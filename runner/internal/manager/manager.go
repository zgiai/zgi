package manager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/panjf2000/ants/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/zgiai/zgi/runner/internal/cache"
	"github.com/zgiai/zgi/runner/internal/callback"
	"github.com/zgiai/zgi/runner/internal/config"
	"github.com/zgiai/zgi/runner/internal/dataplane"
	"github.com/zgiai/zgi/runner/internal/plugin"
	"github.com/zgiai/zgi/runner/internal/protocol"
	"github.com/zgiai/zgi/runner/internal/runtime"
	"github.com/zgiai/zgi/runner/internal/storage"
)

type tenantCtxKey struct{}
type roleCtxKey struct{}
type actorCtxKey struct{}

// Manager orchestrates plugin sessions using a runtime implementation.
type Manager struct {
	cfg       *config.Config
	rt        runtime.Runtime
	log       *zap.Logger
	sessions  sync.Map // map[string]*runtime.Session
	store     storage.Store
	installed sync.Map // map[string]Installation
	orm       *gorm.DB
	cache     *cache.Client
	lockMu    sync.Mutex
	lockHeld  map[string]struct{}
	events    chan InstallEvent
	verifier  SignatureVerifier
	pool      *ants.Pool
	cbHandler *callback.Handler

	janitorCancel context.CancelFunc
	janitorDone   chan struct{}
}

// New constructs the manager.
func New(cfg *config.Config, rt runtime.Runtime, store storage.Store, log *zap.Logger, conns *dataplane.Connections, cacheClient *cache.Client, verifier SignatureVerifier) *Manager {
	// Initialize callback handler
	cbHandler := callback.New(log.Named("callback"), callback.Config{
		HTTPTimeout: 30 * time.Second,
	})

	janitorCtx, janitorCancel := context.WithCancel(context.Background())

	m := &Manager{
		cfg:   cfg,
		rt:    rt,
		log:   log,
		store: store,
		orm: func() *gorm.DB {
			if conns != nil {
				return conns.ORM
			}
			return nil
		}(),
		cache:         cacheClient,
		lockHeld:      make(map[string]struct{}),
		events:        make(chan InstallEvent, 32),
		verifier:      verifier,
		cbHandler:     cbHandler,
		janitorCancel: janitorCancel,
		janitorDone:   make(chan struct{}),
	}
	if cfg.MaxConcurrentRuns > 0 {
		if pool, err := ants.NewPool(cfg.MaxConcurrentRuns, ants.WithNonblocking(true)); err == nil {
			m.pool = pool
		}
	}
	if cfg.SessionSweepIntervalSeconds > 0 {
		go m.runSessionJanitor(janitorCtx)
	} else {
		close(m.janitorDone)
	}
	return m
}

// LaunchRequest describes what the caller wants to run.
type LaunchRequest struct {
	Manifest                  plugin.Manifest
	WorkingDir                string
	Env                       map[string]string
	Args                      []string
	TenantID                  uint // Optional: tenant ID for multi-tenant mode
	WorkflowRunID             string
	SessionPolicy             string
	SessionIdleTTLSeconds     int
	SessionMaxLifetimeSeconds int
}

// InstallRequest wraps the payload needed to install a plugin package.
type InstallRequest struct {
	Manifest plugin.Manifest
	Package  []byte
	Operator string
	Source   string
	Checksum string
	Force    bool // Force overwrite even if same version exists with different content
}

// Installation describes a stored plugin package.
type Installation struct {
	Manifest    plugin.Manifest
	Path        string
	InstalledAt time.Time
	Status      string
}

// Launch starts a new plugin session.
func (m *Manager) Launch(ctx context.Context, req LaunchRequest) (*runtime.Snapshot, error) {
	if err := req.Manifest.Validate(); err != nil {
		return nil, err
	}

	if m.pool == nil {
		return m.launchSync(ctx, req)
	}

	type result struct {
		snap *runtime.Snapshot
		err  error
	}
	ch := make(chan result, 1)
	if err := m.pool.Submit(func() {
		snap, err := m.launchSync(ctx, req)
		ch <- result{snap: snap, err: err}
	}); err != nil {
		return nil, err
	}
	res := <-ch
	return res.snap, res.err
}

func (m *Manager) launchSync(ctx context.Context, req LaunchRequest) (*runtime.Snapshot, error) {
	workingDir := req.WorkingDir
	if workingDir == "" {
		workingDir = m.store.Workspace(req.Manifest)
	}

	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		return nil, fmt.Errorf("create working dir: %w", err)
	}

	// NOTE: Multi-tenant access validation has been removed.
	// Permission decisions are now made by zgi-api-go (TenantPluginSubscription table).
	// Runner trusts the caller. TenantID is still used for audit logging in plugin_runs.

	req.Env = m.enrichEnv(req.Manifest, workingDir, req.Env)

	session, err := m.rt.Start(ctx, runtime.StartRequest{
		Manifest:   req.Manifest,
		WorkingDir: workingDir,
		Env:        req.Env,
		Args:       req.Args,
	})
	if err != nil {
		return nil, err
	}

	sessionMetadata := runtime.SessionMetadata{
		WorkflowRunID:             strings.TrimSpace(req.WorkflowRunID),
		SessionPolicy:             strings.TrimSpace(req.SessionPolicy),
		SessionIdleTTLSeconds:     req.SessionIdleTTLSeconds,
		SessionMaxLifetimeSeconds: req.SessionMaxLifetimeSeconds,
	}
	if sessionMetadata.SessionPolicy == "" {
		sessionMetadata.SessionPolicy = string(runtime.SessionPolicyNoReuse)
	}
	if sessionMetadata.SessionIdleTTLSeconds <= 0 {
		sessionMetadata.SessionIdleTTLSeconds = m.cfg.ReuseSessionIdleTTLSeconds
	}
	if sessionMetadata.SessionMaxLifetimeSeconds <= 0 {
		sessionMetadata.SessionMaxLifetimeSeconds = m.cfg.ReuseSessionMaxLifetimeSeconds
	}
	session.SetMetadata(sessionMetadata)

	m.sessions.Store(session.ID(), session)

	// Record plugin run in database (with tenant ID)
	runID := m.recordPluginRun(ctx, req.Manifest, session.ID(), "running", req.TenantID)

	// Configure callback handler for this session
	if m.cbHandler != nil {
		session.SetCallbackHandler(func(ctx context.Context, req *protocol.CallbackRequest) *protocol.CallbackResponse {
			return m.cbHandler.Handle(ctx, req)
		})
	}

	go func(sessionID string, session *runtime.Session) {
		<-session.Done()
		m.sessions.Delete(sessionID)
		m.log.Info("session completed", zap.String("session_id", sessionID))
		// Update plugin run status
		m.updatePluginRunCompleted(context.Background(), runID, session.Snapshot().Status)
	}(session.ID(), session)

	snap := session.Snapshot()
	return &snap, nil
}

// Install stores the plugin package to the workspace.
func (m *Manager) Install(ctx context.Context, req InstallRequest) (*Installation, error) {
	if err := req.Manifest.Validate(); err != nil {
		return nil, err
	}
	if m.cfg.RequireManifestSignature && strings.TrimSpace(req.Manifest.Signature) == "" {
		return nil, fmt.Errorf("manifest signature required")
	}
	if m.cfg.RequireManifestSignature && m.verifier != nil {
		if err := m.verifier.Verify(req.Manifest); err != nil {
			return nil, fmt.Errorf("verify signature failed: %w", err)
		}
	}
	if m.cfg.MaxPackageSize > 0 && len(req.Package) > int(m.cfg.MaxPackageSize) {
		return nil, fmt.Errorf("package exceeds max size %d bytes", m.cfg.MaxPackageSize)
	}

	// Compute checksum early for duplicate detection
	newChecksum := computeChecksum(req.Package)
	if req.Checksum != "" && req.Checksum != newChecksum {
		return nil, fmt.Errorf("package checksum mismatch")
	}

	// Check if plugin is already installed
	existingInstall, _ := m.getExistingInstall(ctx, req.Manifest)
	if existingInstall != nil {
		// Same checksum - skip installation, return existing
		if existingInstall.PackageChecksum == newChecksum {
			m.log.Info("plugin already installed with same content, skipping",
				zap.String("plugin", manifestKey(req.Manifest)),
				zap.String("checksum", newChecksum))
			// Return existing installation
			return m.buildInstallationFromRecord(ctx, existingInstall, req.Manifest)
		}

		// Different checksum - require force flag
		if !req.Force {
			return nil, fmt.Errorf("plugin %s already installed with different content (existing: %s, new: %s), use force=true to overwrite",
				manifestKey(req.Manifest), existingInstall.PackageChecksum[:8], newChecksum[:8])
		}

		// Check for active sessions before overwriting
		if m.hasActiveSessions(req.Manifest) {
			return nil, fmt.Errorf("cannot overwrite plugin %s: has active sessions, stop them first",
				manifestKey(req.Manifest))
		}

		m.log.Warn("force overwriting existing plugin installation",
			zap.String("plugin", manifestKey(req.Manifest)),
			zap.String("old_checksum", existingInstall.PackageChecksum),
			zap.String("new_checksum", newChecksum))
	}

	path, err := m.store.SavePackage(ctx, req.Manifest, req.Package)
	if err != nil {
		return nil, err
	}

	lockKey := fmt.Sprintf("lock:install:%s", manifestKey(req.Manifest))
	unlock, err := m.acquireLock(ctx, lockKey, 2*time.Minute)
	if err != nil {
		return nil, err
	}
	defer unlock()

	installedBy := req.Operator
	if installedBy == "" {
		installedBy = "system"
	}
	source := req.Source
	if source == "" {
		source = "local"
	}
	pkgSize := int64(len(req.Package))

	_ = m.recordInstall(ctx, req.Manifest, path, "pending", newChecksum, "upload", "uploaded", nil, installedBy, source, pkgSize)
	_ = m.publishEvent(req.Manifest, "pending", "pending", newChecksum, "")

	if err := m.installDependencies(ctx, req.Manifest, path); err != nil {
		_ = m.recordInstall(ctx, req.Manifest, path, "failed", newChecksum, "install", err.Error(), err, installedBy, source, pkgSize)
		_ = m.publishEvent(req.Manifest, "install", "failed", newChecksum, err.Error())
		return nil, err
	}

	// Parse requirements.txt and update manifest packages
	manifest := req.Manifest
	if manifest.Runner.Language == plugin.LanguagePython {
		reqPath := filepath.Join(path, "requirements.txt")
		if pkgs, err := ParseRequirementsTxt(reqPath); err == nil && len(pkgs) > 0 {
			manifest.Requirements.Packages = pkgs
		}
	}

	ins := Installation{
		Manifest:    manifest,
		Path:        path,
		InstalledAt: time.Now(),
		Status:      "installed",
	}
	m.installed.Store(manifestKey(manifest), ins)
	_ = m.recordInstall(ctx, manifest, path, "installed", newChecksum, "complete", "ok", nil, installedBy, source, pkgSize)
	_ = m.publishEvent(manifest, "complete", "installed", newChecksum, "")
	_ = m.recordAudit(ctx, installedBy, "install", manifestKey(manifest), fmt.Sprintf("source=%s stage=complete", source))
	return &ins, nil
}

// buildInstallationFromRecord constructs an Installation from a database record.
func (m *Manager) buildInstallationFromRecord(ctx context.Context, record *dataplane.PluginInstall, manifest plugin.Manifest) (*Installation, error) {
	// Try to parse requirements if Python
	if manifest.Runner.Language == plugin.LanguagePython {
		reqPath := filepath.Join(record.WorkspacePath, "requirements.txt")
		if pkgs, err := ParseRequirementsTxt(reqPath); err == nil && len(pkgs) > 0 {
			manifest.Requirements.Packages = pkgs
		}
	}
	return &Installation{
		Manifest:    manifest,
		Path:        record.WorkspacePath,
		InstalledAt: record.InstalledAt,
		Status:      record.Status,
	}, nil
}

// Uninstall removes the plugin from disk.
func (m *Manager) Uninstall(ctx context.Context, name, version string) error {
	actor := actorFromCtx(ctx)
	//FIXME: should be updated to author:name:version, but the author information is currently missing in the manifest
	manifest := plugin.Manifest{Name: name, Version: version}
	if err := m.store.Remove(ctx, manifest); err != nil {
		return err
	}
	m.installed.Delete(manifestKey(manifest))
	_ = m.publishEvent(manifest, "cleanup", "removed", "", "")
	_ = m.recordAudit(ctx, actor, "uninstall", fmt.Sprintf("%s:%s", name, version), "")
	return nil
}

// ListInstalled returns all known installations from database.
// Falls back to in-memory data if database is unavailable.
func (m *Manager) ListInstalled() []Installation {
	// Try to load from database first
	if m.orm != nil {
		var results []struct {
			dataplane.PluginInstall
			ManifestJSON string
		}
		err := m.orm.Table("plugin_installs").
			Select("plugin_installs.*, plugins.manifest_json").
			Joins("JOIN plugins ON plugins.id = plugin_installs.plugin_id").
			Where("plugin_installs.status = ?", "installed").
			Find(&results).Error
		if err == nil && len(results) > 0 {
			var items []Installation
			for _, r := range results {
				var manifest plugin.Manifest
				if parseErr := json.Unmarshal([]byte(r.ManifestJSON), &manifest); parseErr != nil {
					m.log.Warn("failed to parse manifest", zap.Error(parseErr))
					continue
				}
				items = append(items, Installation{
					Manifest:    manifest,
					Path:        r.WorkspacePath,
					InstalledAt: r.InstalledAt,
					Status:      r.Status,
				})
			}
			return items
		}
	}

	// Fallback to in-memory data
	var items []Installation
	m.installed.Range(func(_, value any) bool {
		if ins, ok := value.(Installation); ok {
			items = append(items, ins)
		}
		return true
	})
	return items
}

func manifestKey(manifest plugin.Manifest) string {
	return fmt.Sprintf("%s:%s", manifest.Name, manifest.Version)
}

// List returns snapshots for all sessions.
func (m *Manager) List() []runtime.Snapshot {
	var snapshots []runtime.Snapshot
	m.sessions.Range(func(key, value any) bool {
		if s, ok := value.(*runtime.Session); ok {
			snapshots = append(snapshots, s.Snapshot())
		}
		return true
	})
	return snapshots
}

// Get returns a snapshot for a session if present.
func (m *Manager) Get(id string) (*runtime.Snapshot, bool) {
	raw, ok := m.sessions.Load(id)
	if !ok {
		return nil, false
	}
	session, ok := raw.(*runtime.Session)
	if !ok {
		return nil, false
	}
	snap := session.Snapshot()
	return &snap, true
}

// Stop attempts to gracefully stop the specified session.
func (m *Manager) Stop(ctx context.Context, id string) error {
	raw, ok := m.sessions.Load(id)
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}
	session, ok := raw.(*runtime.Session)
	if !ok {
		return fmt.Errorf("session %s corrupt entry", id)
	}
	return session.Stop(ctx)
}

// GetSession returns the raw session for invoke operations.
func (m *Manager) GetSession(id string) (*runtime.Session, bool) {
	raw, ok := m.sessions.Load(id)
	if !ok {
		return nil, false
	}
	session, ok := raw.(*runtime.Session)
	return session, ok
}

// Close releases manager-owned resources.
func (m *Manager) Close(ctx context.Context) error {
	if m.janitorCancel != nil {
		m.janitorCancel()
	}
	if m.janitorDone != nil {
		select {
		case <-m.janitorDone:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if m.pool != nil {
		m.pool.Release()
	}
	return nil
}

func (m *Manager) runSessionJanitor(ctx context.Context) {
	defer close(m.janitorDone)

	interval := time.Duration(m.cfg.SessionSweepIntervalSeconds) * time.Second
	if interval <= 0 {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.sweepReusableSessions()
		}
	}
}

func (m *Manager) sweepReusableSessions() {
	now := time.Now()

	m.sessions.Range(func(_, value any) bool {
		session, ok := value.(*runtime.Session)
		if !ok {
			return true
		}

		snap := session.Snapshot()
		if snap.Status != runtime.SessionStatusRunning || snap.Metadata == nil {
			return true
		}
		if snap.Metadata.SessionPolicy != string(runtime.SessionPolicyReuseWithinRun) {
			return true
		}

		idleTTLSeconds := snap.Metadata.SessionIdleTTLSeconds
		if idleTTLSeconds <= 0 {
			idleTTLSeconds = m.cfg.ReuseSessionIdleTTLSeconds
		}
		maxLifetimeSeconds := snap.Metadata.SessionMaxLifetimeSeconds
		if maxLifetimeSeconds <= 0 {
			maxLifetimeSeconds = m.cfg.ReuseSessionMaxLifetimeSeconds
		}

		lastActivityAt := snap.StartedAt
		if snap.LastActivityAt != nil {
			lastActivityAt = *snap.LastActivityAt
		}

		idleExpired := idleTTLSeconds > 0 && now.Sub(lastActivityAt) > time.Duration(idleTTLSeconds)*time.Second
		lifetimeExpired := maxLifetimeSeconds > 0 && now.Sub(snap.StartedAt) > time.Duration(maxLifetimeSeconds)*time.Second
		if !idleExpired && !lifetimeExpired {
			return true
		}

		reason := "idle_timeout"
		if lifetimeExpired {
			reason = "max_lifetime_timeout"
		}
		if idleExpired && lifetimeExpired {
			reason = "idle_and_max_lifetime_timeout"
		}

		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := session.Stop(stopCtx)
		cancel()
		if err != nil {
			m.log.Warn("session janitor stop failed",
				zap.String("session_id", snap.ID),
				zap.String("workflow_run_id", snap.Metadata.WorkflowRunID),
				zap.String("reason", reason),
				zap.Error(err))
			return true
		}

		m.log.Info("session janitor stopped stale reusable session",
			zap.String("session_id", snap.ID),
			zap.String("workflow_run_id", snap.Metadata.WorkflowRunID),
			zap.String("reason", reason))
		return true
	})
}

func (m *Manager) acquireLock(ctx context.Context, key string, ttl time.Duration) (func() error, error) {
	if m.cache != nil && m.cache.Enabled() {
		return m.cache.Lock(ctx, key, ttl)
	}

	m.lockMu.Lock()
	if _, exists := m.lockHeld[key]; exists {
		m.lockMu.Unlock()
		return nil, fmt.Errorf("lock %s already held", key)
	}
	m.lockHeld[key] = struct{}{}
	m.lockMu.Unlock()

	return func() error {
		m.lockMu.Lock()
		delete(m.lockHeld, key)
		m.lockMu.Unlock()
		return nil
	}, nil
}

func (m *Manager) recordInstall(ctx context.Context, manifest plugin.Manifest, path, status, checksum, stage, message string, installErr error, installedBy string, source string, pkgSize int64) error {
	if m.orm == nil {
		return nil
	}

	var pluginRec dataplane.PluginRecord
	if err := m.orm.WithContext(ctx).
		Where("name = ? AND version = ?", manifest.Name, manifest.Version).
		First(&pluginRec).Error; err != nil {
		return err
	}

	record := dataplane.PluginInstall{
		PluginID:        pluginRec.ID,
		WorkspacePath:   path,
		InstalledBy:     installedBy,
		Status:          status,
		Stage:           stage,
		Source:          source,
		PackageChecksum: checksum,
		ErrorMessage:    message,
		PackageSize:     pkgSize,
		InstalledAt:     time.Now(),
	}
	if installErr != nil {
		record.ErrorMessage = installErr.Error()
	}

	return m.orm.WithContext(ctx).
		Where("plugin_id = ?", pluginRec.ID).
		Assign(record).
		FirstOrCreate(&dataplane.PluginInstall{}).Error
}

func computeChecksum(data []byte) string {
	if len(data) == 0 {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (m *Manager) publishEvent(manifest plugin.Manifest, stage, status, checksum, message string) error {
	if m.cache == nil || !m.cache.Enabled() {
		return nil
	}
	event := InstallEvent{
		Name:      manifest.Name,
		Version:   manifest.Version,
		Stage:     stage,
		Status:    status,
		Checksum:  checksum,
		Message:   message,
		Timestamp: time.Now(),
	}
	return m.cache.Publish(context.Background(), "install", event)
}

func (m *Manager) recordAudit(ctx context.Context, actor, action, resource, detail string) error {
	if m.orm == nil {
		return nil
	}
	tenant := tenantFromCtx(ctx)
	entry := dataplane.AuditLog{
		Actor:    actor,
		Action:   action,
		Resource: resource,
		Tenant:   tenant,
		Detail:   detail,
	}
	return m.orm.WithContext(ctx).Create(&entry).Error
}

func tenantFromCtx(ctx context.Context) string {
	tenant := "default"
	if v := ctx.Value(tenantCtxKey{}); v != nil {
		if s, ok := v.(string); ok && s != "" {
			tenant = s
		}
	}
	return tenant
}

// WithTenant annotates context with tenant for audit logging.
func WithTenant(ctx context.Context, tenant string) context.Context {
	if tenant == "" {
		return ctx
	}
	return context.WithValue(ctx, tenantCtxKey{}, tenant)
}

func WithRole(ctx context.Context, role string) context.Context {
	if role == "" {
		return ctx
	}
	return context.WithValue(ctx, roleCtxKey{}, role)
}

func WithActor(ctx context.Context, actor string) context.Context {
	if actor == "" {
		return ctx
	}
	return context.WithValue(ctx, actorCtxKey{}, actor)
}

func actorFromCtx(ctx context.Context) string {
	if v := ctx.Value(actorCtxKey{}); v != nil {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return "system"
}

// RecordAudit exposes audit logging for handlers.
func (m *Manager) RecordAudit(ctx context.Context, action, resource, detail string) {
	actor := actorFromCtx(ctx)
	_ = m.recordAudit(ctx, actor, action, resource, detail)
}

// getExistingInstall returns the existing installation record for a plugin if it exists.
func (m *Manager) getExistingInstall(ctx context.Context, manifest plugin.Manifest) (*dataplane.PluginInstall, error) {
	if m.orm == nil {
		return nil, nil
	}

	var pluginRec dataplane.PluginRecord
	if err := m.orm.WithContext(ctx).
		Where("name = ? AND version = ?", manifest.Name, manifest.Version).
		First(&pluginRec).Error; err != nil {
		return nil, nil // Plugin not registered yet
	}

	var install dataplane.PluginInstall
	if err := m.orm.WithContext(ctx).
		Where("plugin_id = ? AND status = ?", pluginRec.ID, "installed").
		First(&install).Error; err != nil {
		return nil, nil // Not installed yet
	}
	return &install, nil
}

// hasActiveSessions checks if there are any running sessions for a plugin.
func (m *Manager) hasActiveSessions(manifest plugin.Manifest) bool {
	key := manifestKey(manifest)
	hasActive := false
	m.sessions.Range(func(_, value any) bool {
		if s, ok := value.(*runtime.Session); ok {
			snap := s.Snapshot()
			sessionKey := fmt.Sprintf("%s:%s", snap.Manifest.Name, snap.Manifest.Version)
			if sessionKey == key && snap.Status == runtime.SessionStatusRunning {
				hasActive = true
				return false // Stop iteration
			}
		}
		return true
	})
	return hasActive
}
func (m *Manager) enrichEnv(manifest plugin.Manifest, workingDir string, env map[string]string) map[string]string {
	if env == nil {
		env = make(map[string]string)
	}
	if manifest.Runner.Language == plugin.LanguageNode {
		nodePath := filepath.Join(workingDir, ".node_env", "node_modules")
		if existing, ok := env["NODE_PATH"]; ok && strings.TrimSpace(existing) != "" {
			env["NODE_PATH"] = nodePath + string(os.PathListSeparator) + existing
		} else {
			env["NODE_PATH"] = nodePath
		}
		opts := "--no-addons --disable-proto=delete --no-warnings"
		if existing, ok := env["NODE_OPTIONS"]; ok && strings.TrimSpace(existing) != "" {
			env["NODE_OPTIONS"] = opts + " " + existing
		} else {
			env["NODE_OPTIONS"] = opts
		}
	}
	return env
}

// recordPluginRun creates a PluginRun record when a session starts.
func (m *Manager) recordPluginRun(ctx context.Context, manifest plugin.Manifest, sessionID, status string, tenantID uint) uint {
	if m.orm == nil {
		return 0
	}

	var pluginRec dataplane.PluginRecord
	if err := m.orm.WithContext(ctx).
		Where("name = ? AND version = ?", manifest.Name, manifest.Version).
		First(&pluginRec).Error; err != nil {
		m.log.Warn("failed to find plugin record for run", zap.Error(err))
		return 0
	}

	run := dataplane.PluginRun{
		PluginID:  pluginRec.ID,
		SessionID: sessionID,
		Status:    status,
		StartedAt: time.Now(),
	}
	// Set tenant ID if provided
	if tenantID > 0 {
		run.TenantID = &tenantID
	}

	if err := m.orm.WithContext(ctx).Create(&run).Error; err != nil {
		m.log.Warn("failed to record plugin run", zap.Error(err))
		return 0
	}

	m.log.Debug("plugin run recorded",
		zap.Uint("run_id", run.ID),
		zap.String("session_id", sessionID),
		zap.String("status", status),
		zap.Uint("tenant_id", tenantID),
	)
	return run.ID
}

// updatePluginRunCompleted updates a PluginRun record when the session completes.
func (m *Manager) updatePluginRunCompleted(ctx context.Context, runID uint, status runtime.SessionStatus) {
	if m.orm == nil || runID == 0 {
		return
	}

	now := time.Now()
	if err := m.orm.WithContext(ctx).
		Model(&dataplane.PluginRun{}).
		Where("id = ?", runID).
		Updates(map[string]interface{}{
			"status":       string(status),
			"completed_at": &now,
		}).Error; err != nil {
		m.log.Warn("failed to update plugin run completion", zap.Uint("run_id", runID), zap.Error(err))
	}
}

// checkTenantAccess verifies if a tenant has enabled access to a plugin.
// Returns the binding if access is granted, otherwise returns an error.
func (m *Manager) checkTenantAccess(ctx context.Context, pluginName, pluginVersion string, tenantID uint) (*dataplane.PluginTenantBinding, error) {
	if m.orm == nil {
		return nil, nil // No database, allow access (single-tenant mode)
	}

	// Find plugin record first
	var pluginRec dataplane.PluginRecord
	if err := m.orm.WithContext(ctx).
		Where("name = ? AND version = ?", pluginName, pluginVersion).
		First(&pluginRec).Error; err != nil {
		return nil, fmt.Errorf("plugin %s:%s not found", pluginName, pluginVersion)
	}

	// Check tenant binding
	var binding dataplane.PluginTenantBinding
	err := m.orm.WithContext(ctx).
		Where("plugin_id = ? AND tenant_id = ?", pluginRec.ID, tenantID).
		First(&binding).Error
	if err != nil {
		return nil, fmt.Errorf("tenant %d is not bound to plugin %s:%s", tenantID, pluginName, pluginVersion)
	}
	if !binding.Enabled {
		return nil, fmt.Errorf("tenant %d access to plugin %s:%s is disabled", tenantID, pluginName, pluginVersion)
	}
	return &binding, nil
}
