package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/client"
	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/model"
	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/parser"
	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/repository"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

// yamlManifest represents the new YAML manifest format used by plugins
type yamlManifest struct {
	Name        string            `yaml:"name"`
	Version     string            `yaml:"version"`
	Author      string            `yaml:"author"`
	Description map[string]string `yaml:"description"` // e.g. {"en_US": "...", "zh_Hans": "..."}
	Tags        []string          `yaml:"tags"`
	Meta        yamlManifestMeta  `yaml:"meta"`
}

type yamlManifestMeta struct {
	Version string             `yaml:"version"`
	Runner  yamlManifestRunner `yaml:"runner"`
}

type yamlManifestRunner struct {
	Language   string `yaml:"language"`
	Entrypoint string `yaml:"entrypoint"`
	Version    string `yaml:"version"`
}

// parseManifestFromZip extracts and parses manifest.json or manifest.yaml from a ZIP package
func parseManifestFromZip(zipData []byte) (*model.PluginManifest, error) {
	reader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, fmt.Errorf("failed to open zip: %w", err)
	}

	for _, file := range reader.File {
		// Skip macOS metadata
		if strings.Contains(file.Name, "__MACOSX") {
			continue
		}

		isJSON := strings.HasSuffix(file.Name, "manifest.json")
		isYAML := strings.HasSuffix(file.Name, "manifest.yaml") || strings.HasSuffix(file.Name, "manifest.yml")

		if isJSON || isYAML {
			rc, err := file.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to open manifest file: %w", err)
			}
			defer rc.Close()

			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, fmt.Errorf("failed to read manifest file: %w", err)
			}

			if isJSON {
				var manifest model.PluginManifest
				if err := json.Unmarshal(data, &manifest); err != nil {
					return nil, fmt.Errorf("failed to parse manifest.json: %w", err)
				}
				return &manifest, nil
			}

			// Parse YAML with new format and convert
			var ym yamlManifest
			if err := yaml.Unmarshal(data, &ym); err != nil {
				return nil, fmt.Errorf("failed to parse manifest.yaml: %w", err)
			}

			// Convert to PluginManifest
			description := ""
			if desc, ok := ym.Description["en_US"]; ok {
				description = desc
			} else if desc, ok := ym.Description["zh_Hans"]; ok {
				description = desc
			}

			// Use version from meta if available, otherwise from top level
			version := ym.Version
			if ym.Meta.Version != "" {
				version = ym.Meta.Version
			}

			manifest := &model.PluginManifest{
				Name:        ym.Name,
				Version:     version,
				Description: description,
				Author:      ym.Author,
				Tags:        ym.Tags,
				Runner: model.PluginRunner{
					Language:   ym.Meta.Runner.Language,
					Entrypoint: ym.Meta.Runner.Entrypoint,
				},
			}
			return manifest, nil
		}
	}

	return nil, fmt.Errorf("manifest.json or manifest.yaml not found in package")
}

// ErrPluginInstallationNotFound indicates the plugin is not installed for the organization.
var ErrPluginInstallationNotFound = errors.New("plugin installation not found")

const (
	reusableSessionPolicy         = "reuse_within_run"
	installSelfHealSweepMaxAge    = 60 * time.Minute
	installSelfHealCleanupTimeout = 20 * time.Second
)

// PluginRunnerService provides high-level operations for the plugin runner
type PluginRunnerService interface {
	// Health check
	IsHealthy(ctx context.Context) bool

	// Plugin management
	RegisterPlugin(ctx context.Context, manifest model.PluginManifest) (*model.Plugin, error)
	ListPlugins(ctx context.Context) ([]model.Plugin, error)
	GetPlugin(ctx context.Context, pluginID string) (*model.Plugin, error)
	DeletePlugin(ctx context.Context, pluginID string) error
	InstallPlugin(ctx context.Context, pluginID string, packageContent []byte, force bool) (*model.Installation, error)
	ListInstalledPlugins(ctx context.Context) ([]model.Installation, error)

	// Install from Marketplace
	InstallFromMarketplace(ctx context.Context, tenantID, marketplacePluginID, versionID, installedBy string, force bool) (*model.Installation, error)
	ReinstallFromMarketplace(ctx context.Context, tenantID, marketplacePluginID, versionID, installedBy string) (*model.Installation, error)

	// Session management
	StartSession(ctx context.Context, req model.StartSessionRequest, opts ...client.RequestOption) (*model.Session, error)
	ListSessions(ctx context.Context) ([]model.Session, error)
	GetSession(ctx context.Context, sessionID string) (*model.Session, error)
	StopSession(ctx context.Context, sessionID string) error
	WaitForSessionReady(ctx context.Context, sessionID string, timeout time.Duration) error

	// Invoke operations
	Invoke(ctx context.Context, req model.InvokeRequest) (*model.InvokeResponse, error)
	InvokeTool(ctx context.Context, req model.ToolInvokeRequest) (*model.InvokeResponse, error)

	// High-level operations
	RunPlugin(ctx context.Context, pluginName, version, entrypoint string, env map[string]string) (*model.Session, error)
	RunPluginAndInvokeTool(ctx context.Context, pluginName, version, entrypoint string, provider, tool string, params map[string]interface{}) (*model.InvokeResponse, error)

	// Multi-Tenant operations
	// Deprecated: tenant bindings are legacy and will be removed.
	CreateWorkspace(ctx context.Context, name string) (*model.Tenant, error)
	// Deprecated: tenant bindings are legacy and will be removed.
	ListPluginTenants(ctx context.Context, pluginID string) ([]model.PluginTenantBinding, error)
	// Deprecated: tenant bindings are legacy and will be removed.
	EnablePluginTenant(ctx context.Context, pluginID string, tenantID uint, config *model.TenantConfig) error
	// Deprecated: tenant bindings are legacy and will be removed.
	DisablePluginTenant(ctx context.Context, pluginID string, tenantID uint) error
}

// pluginRunnerServiceImpl implements PluginRunnerService
type pluginRunnerServiceImpl struct {
	client            *client.Client
	marketplaceClient *client.MarketplaceClient
	installRepo       repository.AccountInstallationRepository
	infoRepo          repository.InstalledPluginInfoRepository
}

// NewPluginRunnerService creates a new plugin runner service
func NewPluginRunnerService(cfg *client.Config) PluginRunnerService {
	return &pluginRunnerServiceImpl{
		client:            client.NewClient(cfg),
		marketplaceClient: client.NewMarketplaceClientFromEnv(),
	}
}

// NewPluginRunnerServiceWithRepos creates a new plugin runner service with database repositories
func NewPluginRunnerServiceWithRepos(
	cfg *client.Config,
	installRepo repository.AccountInstallationRepository,
	infoRepo repository.InstalledPluginInfoRepository) PluginRunnerService {
	return &pluginRunnerServiceImpl{
		client:            client.NewClient(cfg),
		marketplaceClient: client.NewMarketplaceClientFromEnv(),
		installRepo:       installRepo,
		infoRepo:          infoRepo,
	}
}

// NewPluginRunnerServiceWithClient creates a new plugin runner service with existing client
func NewPluginRunnerServiceWithClient(c *client.Client) PluginRunnerService {
	return &pluginRunnerServiceImpl{
		client:            c,
		marketplaceClient: client.NewMarketplaceClientFromEnv(),
	}
}

// ============================================
// Health Check
// ============================================

func (s *pluginRunnerServiceImpl) IsHealthy(ctx context.Context) bool {
	return s.client.IsHealthy(ctx)
}

// ============================================
// Plugin Management
// ============================================

func (s *pluginRunnerServiceImpl) RegisterPlugin(ctx context.Context, manifest model.PluginManifest) (*model.Plugin, error) {
	return s.client.RegisterPlugin(ctx, manifest)
}

func (s *pluginRunnerServiceImpl) ListPlugins(ctx context.Context) ([]model.Plugin, error) {
	return s.client.ListPlugins(ctx)
}

func (s *pluginRunnerServiceImpl) GetPlugin(ctx context.Context, pluginID string) (*model.Plugin, error) {
	return s.client.GetPlugin(ctx, pluginID)
}

func (s *pluginRunnerServiceImpl) DeletePlugin(ctx context.Context, pluginID string) error {
	return s.client.DeletePlugin(ctx, pluginID)
}

func (s *pluginRunnerServiceImpl) InstallPlugin(ctx context.Context, pluginID string, packageContent []byte, force bool) (*model.Installation, error) {
	return s.installPluginWithSelfHeal(ctx, pluginID, packageContent, force)
}

func (s *pluginRunnerServiceImpl) ListInstalledPlugins(ctx context.Context) ([]model.Installation, error) {
	return s.client.ListInstalledPlugins(ctx)
}

// InstallFromMarketplace downloads plugin from Marketplace and installs to Runner
// Flow: Download -> Single Parse -> Register -> Install -> Write to Database
func (s *pluginRunnerServiceImpl) InstallFromMarketplace(ctx context.Context, tenantID, marketplacePluginID, versionID, installedBy string, force bool) (*model.Installation, error) {
	// 1. Download plugin package from Marketplace
	logger.Info("downloading plugin from marketplace", "plugin_id", marketplacePluginID, "version_id", versionID)

	packageData, err := s.marketplaceClient.DownloadPlugin(ctx, marketplacePluginID, versionID)
	if err != nil {
		return nil, fmt.Errorf("failed to download plugin from marketplace: %w", err)
	}

	logger.Info("plugin downloaded", "size_bytes", len(packageData))

	// 2. Single parse: get both manifest (for Runner) and declaration (for database)
	parseResult, err := parser.ParsePluginFromZipFull(packageData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse plugin package (must follow plugin YAML format): %w", err)
	}

	manifest := buildRunnerManifest(parseResult, marketplacePluginID, versionID)

	// Generate plugin ID for Runner (author:name:version format)
	pluginID := manifest.Author + ":" + manifest.Name + ":" + manifest.Version
	logger.Info("plugin parsed (single parse)", "name", manifest.Name, "version", manifest.Version, "plugin_id", pluginID)

	// 3. Register plugin to Runner first (ignore error if already exists)
	_, regErr := s.client.RegisterPlugin(ctx, *manifest)
	if regErr != nil {
		logger.Debug("register plugin result (may already exist)", "error", regErr)
		return nil, fmt.Errorf("failed to register plugin to runner: %w", regErr)
	}

	// 4. Install to Runner
	installation, err := s.installPluginWithSelfHeal(ctx, pluginID, packageData, force)
	if err != nil {
		return nil, fmt.Errorf("failed to install plugin to runner: %w", err)
	}

	logger.Info("plugin installed to runner successfully", "plugin_id", pluginID, "marketplace_plugin_id", marketplacePluginID)

	// 5. Write declaration to database (using already parsed data)
	if s.infoRepo == nil || s.installRepo == nil || tenantID == "" {
		logger.Warn("infoRepo or installRepo, tenantID is nil")
		return nil, fmt.Errorf("failed to write declaration to database: %w", err)
	}

	declarationJSON, err := parseResult.Declaration.ToJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to convert declaration to JSON: %w", err)
	}

	// Write to installed_plugin_info
	info := &model.InstalledPluginInfo{
		MarketplacePluginID:  marketplacePluginID,
		MarketplaceVersionID: versionID,
		PluginName:           parseResult.Declaration.Provider.Name,
		PluginVersion:        manifest.Version,
		PluginAuthor:         parseResult.Declaration.Provider.Author,
		Declaration:          declarationJSON,
	}

	// Check if already exists
	existing, _ := s.infoRepo.GetByMarketplaceVersionID(ctx, versionID)
	if existing == nil {
		if err := s.infoRepo.Create(ctx, info); err != nil {
			logger.Warn("failed to create installed_plugin_info", "error", err)
			return nil, fmt.Errorf("failed to create installed_plugin_info: %w", err)
		}
		logger.Info("installed_plugin_info created", "version_id", versionID)
	} else {
		// Update declaration
		existing.Declaration = declarationJSON
		if err := s.infoRepo.Update(ctx, existing); err != nil {
			logger.Warn("failed to update installed_plugin_info", "error", err)
			return nil, fmt.Errorf("failed to update installed_plugin_info: %w", err)
		}
	}

	// Write to account_plugin_installations
	instRecord := &model.AccountPluginInstallation{
		TenantID:             tenantID,
		MarketplacePluginID:  marketplacePluginID,
		MarketplaceVersionID: versionID,
		InstalledBy:          installedBy,
		Status:               model.InstallationStatusActive,
	}

	// Check if already exists
	existingInst, _ := s.installRepo.GetByTenantAndVersion(ctx, tenantID, versionID)
	if existingInst == nil {
		if err := s.installRepo.Create(ctx, instRecord); err != nil {
			logger.Warn("failed to create account_plugin_installation", "error", err)
			return nil, fmt.Errorf("failed to create account_plugin_installation: %w", err)
		}
		logger.Info("account_plugin_installation created", "tenant_id", tenantID, "version_id", versionID)
	}

	logger.Info("plugin installed successfully", "plugin_id", pluginID, "marketplace_plugin_id", marketplacePluginID)
	return installation, nil
}

// ReinstallFromMarketplace downloads plugin from Marketplace and reinstalls to Runner
// Flow: Validate -> Download -> Parse -> Register -> Install (force) -> Update declaration
func (s *pluginRunnerServiceImpl) ReinstallFromMarketplace(ctx context.Context, tenantID, marketplacePluginID, versionID, installedBy string) (*model.Installation, error) {
	if s.infoRepo == nil || s.installRepo == nil {
		return nil, fmt.Errorf("installation repositories are not configured")
	}
	if tenantID == "" {
		return nil, fmt.Errorf("tenantID is required")
	}

	installation, err := s.installRepo.GetByTenantAndVersion(ctx, tenantID, versionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPluginInstallationNotFound
		}
		return nil, fmt.Errorf("failed to get installation: %w", err)
	}
	if marketplacePluginID == "" {
		marketplacePluginID = installation.MarketplacePluginID
	}
	if installation.MarketplacePluginID != "" && marketplacePluginID != installation.MarketplacePluginID {
		return nil, fmt.Errorf("marketplace plugin ID mismatch")
	}

	info, err := s.infoRepo.GetByMarketplaceVersionID(ctx, versionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get installed plugin info: %w", err)
	}

	logger.Info("reinstalling plugin from marketplace", "plugin_id", marketplacePluginID, "version_id", versionID, "installed_by", installedBy)

	packageData, err := s.marketplaceClient.DownloadPlugin(ctx, marketplacePluginID, versionID)
	if err != nil {
		return nil, fmt.Errorf("failed to download plugin from marketplace: %w", err)
	}

	parseResult, err := parser.ParsePluginFromZipFull(packageData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse plugin package (must follow plugin YAML format): %w", err)
	}
	if err := validateReinstallConsistency(info, parseResult); err != nil {
		return nil, err
	}

	manifest := buildRunnerManifest(parseResult, marketplacePluginID, versionID)
	pluginID, err := buildRunnerPluginID(manifest)
	if err != nil {
		return nil, err
	}

	if err := s.ensurePluginRegistered(ctx, pluginID, *manifest); err != nil {
		return nil, fmt.Errorf("failed to register plugin to runner: %w", err)
	}

	installationResult, err := s.installPluginWithSelfHeal(ctx, pluginID, packageData, true)
	if err != nil {
		return nil, fmt.Errorf("failed to install plugin to runner: %w", err)
	}

	if err := s.updateInstalledPluginInfo(ctx, info, parseResult, manifest); err != nil {
		return nil, err
	}

	logger.Info("plugin reinstalled successfully", "plugin_id", pluginID, "marketplace_plugin_id", marketplacePluginID, "version_id", versionID)
	return installationResult, nil
}

func buildRunnerManifest(parseResult *parser.ParseResult, marketplacePluginID, versionID string) *model.PluginManifest {
	name := parseResult.Manifest.Name
	if parseResult.Declaration != nil && parseResult.Declaration.Provider.Name != "" {
		name = parseResult.Declaration.Provider.Name
	}
	author := parseResult.Manifest.Author
	if parseResult.Declaration != nil && parseResult.Declaration.Provider.Author != "" {
		author = parseResult.Declaration.Provider.Author
	}

	return &model.PluginManifest{
		Name:                 name,
		Version:              parseResult.Manifest.Version,
		Author:               author,
		Description:          parseResult.Manifest.Description,
		Tags:                 parseResult.Manifest.Tags,
		MarketplacePluginID:  marketplacePluginID,
		MarketplaceVersionID: versionID,
		Runner: model.PluginRunner{
			Language:   parseResult.Manifest.Language,
			Entrypoint: parseResult.Manifest.Entrypoint,
		},
	}
}

func buildRunnerPluginID(manifest *model.PluginManifest) (string, error) {
	if manifest == nil {
		return "", fmt.Errorf("manifest is required")
	}
	if manifest.Name == "" || manifest.Version == "" {
		return "", fmt.Errorf("manifest is missing name or version")
	}
	return fmt.Sprintf("%s:%s:%s", manifest.Author, manifest.Name, manifest.Version), nil
}

func validateReinstallConsistency(info *model.InstalledPluginInfo, parseResult *parser.ParseResult) error {
	if info == nil || parseResult == nil || parseResult.Declaration == nil || parseResult.Manifest == nil {
		return fmt.Errorf("invalid plugin data for reinstall")
	}
	if parseResult.Declaration.Provider.Name == "" {
		return fmt.Errorf("plugin declaration missing provider name")
	}
	if parseResult.Manifest.Name == "" || parseResult.Manifest.Version == "" {
		return fmt.Errorf("plugin manifest missing name or version")
	}
	if info.PluginName != "" && parseResult.Declaration.Provider.Name != "" && info.PluginName != parseResult.Declaration.Provider.Name {
		return fmt.Errorf("plugin name mismatch")
	}
	if info.PluginVersion != "" && parseResult.Manifest.Version != "" && info.PluginVersion != parseResult.Manifest.Version {
		return fmt.Errorf("plugin version mismatch")
	}
	if info.PluginAuthor != "" {
		if parseResult.Declaration.Provider.Author == "" || info.PluginAuthor != parseResult.Declaration.Provider.Author {
			return fmt.Errorf("plugin author mismatch")
		}
		if parseResult.Manifest.Author != "" && info.PluginAuthor != parseResult.Manifest.Author {
			return fmt.Errorf("plugin author mismatch")
		}
	}
	return nil
}

func (s *pluginRunnerServiceImpl) ensurePluginRegistered(ctx context.Context, pluginID string, manifest model.PluginManifest) error {
	if pluginID == "" {
		return fmt.Errorf("plugin ID is required")
	}
	plugin, err := s.client.GetPlugin(ctx, pluginID)
	if err == nil && plugin != nil {
		return nil
	}
	if _, err := s.client.RegisterPlugin(ctx, manifest); err != nil {
		plugin, getErr := s.client.GetPlugin(ctx, pluginID)
		if getErr == nil && plugin != nil {
			return nil
		}
		return err
	}
	return nil
}

func (s *pluginRunnerServiceImpl) updateInstalledPluginInfo(ctx context.Context, info *model.InstalledPluginInfo, parseResult *parser.ParseResult, manifest *model.PluginManifest) error {
	if info == nil || parseResult == nil || parseResult.Declaration == nil || manifest == nil {
		return fmt.Errorf("invalid plugin data for update")
	}

	declarationJSON, err := parseResult.Declaration.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to convert declaration to JSON: %w", err)
	}

	info.Declaration = declarationJSON
	info.PluginName = parseResult.Declaration.Provider.Name
	info.PluginVersion = manifest.Version
	info.PluginAuthor = parseResult.Declaration.Provider.Author

	if err := s.infoRepo.Update(ctx, info); err != nil {
		return fmt.Errorf("failed to update installed_plugin_info: %w", err)
	}
	return nil
}

func (s *pluginRunnerServiceImpl) installPluginWithSelfHeal(
	ctx context.Context,
	pluginID string,
	packageContent []byte,
	force bool,
	opts ...client.RequestOption,
) (*model.Installation, error) {
	installation, err := s.client.InstallPluginWithFile(ctx, pluginID, packageContent, force, opts...)
	if err == nil {
		return installation, nil
	}
	if !isActiveSessionInstallConflict(err) {
		return nil, err
	}

	logger.Warn("plugin install blocked by active sessions, starting stale-session self-heal",
		"plugin_id", pluginID,
		"error", err)

	cleanupCtx, cancel := context.WithTimeout(context.Background(), installSelfHealCleanupTimeout)
	defer cancel()
	cleanedCount, cleanupErr := s.cleanupExpiredReusableSessionsForPlugin(cleanupCtx, pluginID, installSelfHealSweepMaxAge)
	if cleanupErr != nil {
		logger.Warn("stale session cleanup encountered errors before install retry",
			"plugin_id", pluginID,
			"cleaned_count", cleanedCount,
			"error", cleanupErr)
	}

	retryInstallation, retryErr := s.client.InstallPluginWithFile(ctx, pluginID, packageContent, force, opts...)
	if retryErr != nil {
		if cleanupErr != nil {
			return nil, fmt.Errorf("install retry failed after stale-session cleanup (cleaned=%d): %w (cleanup error: %v)", cleanedCount, retryErr, cleanupErr)
		}
		return nil, fmt.Errorf("install retry failed after stale-session cleanup (cleaned=%d): %w", cleanedCount, retryErr)
	}

	logger.Info("plugin install self-heal retry succeeded",
		"plugin_id", pluginID,
		"cleaned_count", cleanedCount)
	return retryInstallation, nil
}

func (s *pluginRunnerServiceImpl) cleanupExpiredReusableSessionsForPlugin(
	ctx context.Context,
	pluginID string,
	maxAge time.Duration,
) (int, error) {
	sessions, err := s.client.ListSessions(ctx)
	if err != nil {
		return 0, fmt.Errorf("list sessions for cleanup failed: %w", err)
	}

	targetName, targetVersion, hasPluginFilter := parsePluginNameVersionFromID(pluginID)
	now := time.Now()
	stoppedCount := 0
	var firstErr error

	for _, session := range sessions {
		if session.Status != model.SessionStatusRunning {
			continue
		}

		if hasPluginFilter {
			if session.Manifest.Name != targetName || session.Manifest.Version != targetVersion {
				continue
			}
		}

		if session.Metadata != nil {
			policy := strings.TrimSpace(session.Metadata.SessionPolicy)
			if policy != "" && policy != reusableSessionPolicy {
				continue
			}
		}

		lastActivityAt := session.StartedAt
		if session.LastActivityAt != nil {
			lastActivityAt = *session.LastActivityAt
		}
		if now.Sub(lastActivityAt) <= maxAge {
			continue
		}

		if err := s.client.StopSession(ctx, session.ID); err != nil {
			if isRunnerSessionNotFound(err) {
				continue
			}
			logger.Warn("failed to stop stale session during install self-heal",
				"session_id", session.ID,
				"plugin_id", pluginID,
				"error", err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		stoppedCount++
	}

	return stoppedCount, firstErr
}

func parsePluginNameVersionFromID(pluginID string) (string, string, bool) {
	parts := strings.Split(strings.TrimSpace(pluginID), ":")
	if len(parts) < 2 {
		return "", "", false
	}
	version := strings.TrimSpace(parts[len(parts)-1])
	name := strings.TrimSpace(parts[len(parts)-2])
	if name == "" || version == "" {
		return "", "", false
	}
	return name, version, true
}

func isActiveSessionInstallConflict(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "has active sessions")
}

func isRunnerSessionNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "status 404") || strings.Contains(msg, "not found")
}

// ============================================
// Session Management
// ============================================

func (s *pluginRunnerServiceImpl) StartSession(ctx context.Context, req model.StartSessionRequest, opts ...client.RequestOption) (*model.Session, error) {
	return s.client.StartSession(ctx, req, opts...)
}

func (s *pluginRunnerServiceImpl) ListSessions(ctx context.Context) ([]model.Session, error) {
	return s.client.ListSessions(ctx)
}

func (s *pluginRunnerServiceImpl) GetSession(ctx context.Context, sessionID string) (*model.Session, error) {
	return s.client.GetSession(ctx, sessionID)
}

func (s *pluginRunnerServiceImpl) StopSession(ctx context.Context, sessionID string) error {
	return s.client.StopSession(ctx, sessionID)
}

// WaitForSessionReady waits for a session to become ready
func (s *pluginRunnerServiceImpl) WaitForSessionReady(ctx context.Context, sessionID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for session %s to be ready", sessionID)
			}

			resp, err := s.client.IsSessionReady(ctx, sessionID)
			if err != nil {
				logger.Warn("check session ready failed", "session_id", sessionID, "error", err)
				continue
			}

			if resp.Ready {
				return nil
			}

			// Check if session failed
			if resp.Status == model.SessionStatusFailed || resp.Status == model.SessionStatusExited {
				return fmt.Errorf("session %s is in %s state", sessionID, resp.Status)
			}
		}
	}
}

// ============================================
// Invoke Operations
// ============================================

func (s *pluginRunnerServiceImpl) Invoke(ctx context.Context, req model.InvokeRequest) (*model.InvokeResponse, error) {
	return s.client.Invoke(ctx, req)
}

func (s *pluginRunnerServiceImpl) InvokeTool(ctx context.Context, req model.ToolInvokeRequest) (*model.InvokeResponse, error) {
	return s.client.InvokeTool(ctx, req)
}

// ============================================
// High-Level Operations
// ============================================

// RunPlugin starts a plugin session and waits for it to be ready
func (s *pluginRunnerServiceImpl) RunPlugin(ctx context.Context, pluginName, version, entrypoint string, env map[string]string) (*model.Session, error) {
	// TODO: enforce subscription checks for runner plugins before execution.
	// Check if plugin is installed
	plugins, err := s.ListInstalledPlugins(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list installed plugins: %w", err)
	}

	found := false
	for _, p := range plugins {
		if p.Manifest.Name == pluginName && p.Manifest.Version == version {
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("plugin %s@%s is not installed", pluginName, version)
	}

	// Start session
	session, err := s.StartSession(ctx, model.StartSessionRequest{
		Name:       pluginName,
		Version:    version,
		Entrypoint: entrypoint,
		Env:        env,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start session: %w", err)
	}

	// Wait for session to be ready (default 30s timeout)
	if err := s.WaitForSessionReady(ctx, session.ID, 30*time.Second); err != nil {
		// Try to stop the session on failure
		_ = s.StopSession(ctx, session.ID)
		return nil, fmt.Errorf("session failed to become ready: %w", err)
	}

	return session, nil
}

// RunPluginAndInvokeTool is a convenience method that starts a session, invokes a tool, and stops the session
func (s *pluginRunnerServiceImpl) RunPluginAndInvokeTool(ctx context.Context, pluginName, version, entrypoint string, provider, tool string, params map[string]interface{}) (*model.InvokeResponse, error) {
	// TODO: enforce subscription checks for runner plugins before execution.
	// Start session
	session, err := s.RunPlugin(ctx, pluginName, version, entrypoint, nil)
	if err != nil {
		return nil, err
	}

	// Ensure session is stopped when done
	defer func() {
		if stopErr := s.StopSession(ctx, session.ID); stopErr != nil {
			logger.Warn("failed to stop session", "session_id", session.ID, "error", stopErr)
		}
	}()

	// Invoke tool
	resp, err := s.InvokeTool(ctx, model.ToolInvokeRequest{
		SessionID:  session.ID,
		Provider:   provider,
		Tool:       tool,
		Parameters: params,
		Timeout:    30,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to invoke tool: %w", err)
	}

	return resp, nil
}

// ============================================
// Multi-Tenant Operations
// ============================================

func (s *pluginRunnerServiceImpl) CreateWorkspace(ctx context.Context, name string) (*model.Tenant, error) {
	// Deprecated: tenant bindings are legacy and will be removed.
	return s.client.CreateWorkspace(ctx, name)
}

func (s *pluginRunnerServiceImpl) ListPluginTenants(ctx context.Context, pluginID string) ([]model.PluginTenantBinding, error) {
	// Deprecated: tenant bindings are legacy and will be removed.
	return s.client.ListPluginTenants(ctx, pluginID)
}

func (s *pluginRunnerServiceImpl) EnablePluginTenant(ctx context.Context, pluginID string, tenantID uint, config *model.TenantConfig) error {
	// Deprecated: tenant bindings are legacy and will be removed.
	return s.client.EnablePluginTenant(ctx, pluginID, tenantID, config)
}

func (s *pluginRunnerServiceImpl) DisablePluginTenant(ctx context.Context, pluginID string, tenantID uint) error {
	// Deprecated: tenant bindings are legacy and will be removed.
	return s.client.DisablePluginTenant(ctx, pluginID, tenantID)
}
