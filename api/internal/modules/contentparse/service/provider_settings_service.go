package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/contracts"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/model"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/repository"
	llmcrypto "github.com/zgiai/zgi/api/internal/modules/llm/shared/crypto"
)

const (
	ParserProviderReducto = "reducto"
	ParserProviderMineru  = "mineru"

	MineruModeSidecar  = "sidecar"
	MineruModeOfficial = "official"

	DefaultReductoBaseURL = "https://platform.reducto.ai"
	DefaultReductoTimeout = 180

	DefaultMineruSidecarBaseURL       = "http://127.0.0.1:18091"
	DefaultMineruSidecarTimeout       = 800
	DefaultMineruOfficialBaseURL      = "https://mineru.net"
	DefaultMineruOfficialTimeout      = 800
	DefaultMineruOfficialPollInterval = 3
	DefaultMineruOfficialModelVersion = "vlm"
	DefaultReductoProviderPriority    = 100
	DefaultMineruProviderPriority     = 200
)

var (
	ErrUnsupportedParserProvider = errors.New("unsupported parser provider")
	ErrParserConfigInvalid       = errors.New("parser config invalid")
)

type ProviderSettingsService interface {
	List(ctx context.Context, organizationID uuid.UUID) (*ParserSettingsList, error)
	Upsert(ctx context.Context, organizationID uuid.UUID, actorID *uuid.UUID, providerKey string, input ParserSettingsInput) (*ParserProviderSettings, error)
}

type ParserSettingsList struct {
	Items []*ParserProviderSettings `json:"items"`
}

type ParserProviderSettings struct {
	ProviderKey                 string `json:"provider_key"`
	DisplayName                 string `json:"display_name"`
	Enabled                     bool   `json:"enabled"`
	Configured                  bool   `json:"configured"`
	Status                      string `json:"status"`
	BaseURL                     string `json:"base_url,omitempty"`
	TimeoutSec                  int    `json:"timeout_sec,omitempty"`
	APIKeyConfigured            bool   `json:"api_key_configured,omitempty"`
	Mode                        string `json:"mode,omitempty"`
	OfficialTokenConfigured     bool   `json:"official_token_configured,omitempty"`
	OfficialModelVersion        string `json:"official_model_version,omitempty"`
	OfficialPollIntervalSeconds int    `json:"official_poll_interval_seconds,omitempty"`
	RuntimeConfigSource         string `json:"runtime_config_source"`
}

type ParserSettingsInput struct {
	Enabled                     *bool   `json:"enabled"`
	APIKey                      *string `json:"api_key"`
	BaseURL                     *string `json:"base_url"`
	TimeoutSec                  *int    `json:"timeout_sec"`
	Mode                        *string `json:"mode"`
	OfficialToken               *string `json:"official_token"`
	OfficialModelVersion        *string `json:"official_model_version"`
	OfficialPollIntervalSeconds *int    `json:"official_poll_interval_seconds"`
}

type providerSettingsService struct {
	repo   repository.ProviderConfigRepository
	crypto llmcrypto.CryptoService
}

func NewProviderSettingsService(repo repository.ProviderConfigRepository, crypto llmcrypto.CryptoService) ProviderSettingsService {
	return &providerSettingsService{repo: repo, crypto: crypto}
}

func (s *providerSettingsService) List(ctx context.Context, organizationID uuid.UUID) (*ParserSettingsList, error) {
	items := make([]*ParserProviderSettings, 0, 2)
	for _, providerKey := range []string{ParserProviderReducto, ParserProviderMineru} {
		item, err := s.repo.GetByScopeAndKey(ctx, "organization", &organizationID, nil, providerKey)
		if err != nil {
			return nil, err
		}
		items = append(items, s.toView(providerKey, item))
	}
	return &ParserSettingsList{Items: items}, nil
}

func (s *providerSettingsService) Upsert(ctx context.Context, organizationID uuid.UUID, actorID *uuid.UUID, providerKey string, input ParserSettingsInput) (*ParserProviderSettings, error) {
	providerKey = strings.ToLower(strings.TrimSpace(providerKey))
	if providerKey != ParserProviderReducto && providerKey != ParserProviderMineru {
		return nil, ErrUnsupportedParserProvider
	}
	existing, err := s.repo.GetByScopeAndKey(ctx, "organization", &organizationID, nil, providerKey)
	if err != nil {
		return nil, err
	}

	var item *model.ProviderConfig
	switch providerKey {
	case ParserProviderReducto:
		item, err = s.buildReductoConfig(organizationID, actorID, existing, input)
	case ParserProviderMineru:
		item, err = s.buildMineruConfig(organizationID, actorID, existing, input)
	}
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpsertByScopeAndKey(ctx, item); err != nil {
		return nil, err
	}
	saved, err := s.repo.GetByScopeAndKey(ctx, "organization", &organizationID, nil, providerKey)
	if err != nil {
		return nil, err
	}
	return s.toView(providerKey, saved), nil
}

func (s *providerSettingsService) buildReductoConfig(organizationID uuid.UUID, actorID *uuid.UUID, existing *model.ProviderConfig, input ParserSettingsInput) (*model.ProviderConfig, error) {
	enabled := boolValue(input.Enabled, existingBool(existing, false))
	baseURL := stringValue(input.BaseURL, existingString(existing, "base_url", DefaultReductoBaseURL))
	timeoutSec := intValue(input.TimeoutSec, existingTimeout(existing, DefaultReductoTimeout))
	credentials := cloneMap(existingCredentials(existing))
	if input.APIKey != nil {
		encrypted, err := s.encryptSecret(strings.TrimSpace(*input.APIKey))
		if err != nil {
			return nil, err
		}
		if encrypted == "" {
			delete(credentials, "api_key")
		} else {
			credentials["api_key"] = encrypted
		}
	}
	if enabled && metadataString(credentials, "api_key") == "" {
		return nil, fmt.Errorf("%w: Reducto API key is required when enabled", ErrParserConfigInvalid)
	}
	return parserConfigRecord(organizationID, actorID, existing, ParserProviderReducto, "Reducto", enabled, baseURL, timeoutSec, credentials, map[string]any{}), nil
}

func (s *providerSettingsService) buildMineruConfig(organizationID uuid.UUID, actorID *uuid.UUID, existing *model.ProviderConfig, input ParserSettingsInput) (*model.ProviderConfig, error) {
	mode := strings.ToLower(strings.TrimSpace(stringValue(input.Mode, existingMetadataString(existing, "mode", MineruModeSidecar))))
	if mode != MineruModeSidecar && mode != MineruModeOfficial {
		return nil, fmt.Errorf("%w: unsupported MinerU mode", ErrParserConfigInvalid)
	}
	enabled := boolValue(input.Enabled, existingBool(existing, false))
	credentials := cloneMap(existingCredentials(existing))
	metadata := cloneMap(existingMetadata(existing))
	metadata["mode"] = mode

	baseDefault := DefaultMineruSidecarBaseURL
	timeoutDefault := DefaultMineruSidecarTimeout
	if mode == MineruModeOfficial {
		baseDefault = DefaultMineruOfficialBaseURL
		timeoutDefault = DefaultMineruOfficialTimeout
	}
	baseURL := stringValue(input.BaseURL, existingString(existing, "base_url", baseDefault))
	timeoutSec := intValue(input.TimeoutSec, existingTimeout(existing, timeoutDefault))

	if input.OfficialToken != nil {
		encrypted, err := s.encryptSecret(strings.TrimSpace(*input.OfficialToken))
		if err != nil {
			return nil, err
		}
		if encrypted == "" {
			delete(credentials, "official_token")
		} else {
			credentials["official_token"] = encrypted
		}
	}
	if mode == MineruModeOfficial {
		metadata["official_model_version"] = stringValue(input.OfficialModelVersion, existingMetadataString(existing, "official_model_version", DefaultMineruOfficialModelVersion))
		metadata["official_poll_interval_seconds"] = intValue(input.OfficialPollIntervalSeconds, existingMetadataInt(existing, "official_poll_interval_seconds", DefaultMineruOfficialPollInterval))
		if enabled && metadataString(credentials, "official_token") == "" {
			return nil, fmt.Errorf("%w: MinerU official token is required when enabled", ErrParserConfigInvalid)
		}
	} else {
		delete(metadata, "official_model_version")
		delete(metadata, "official_poll_interval_seconds")
		if enabled && strings.TrimSpace(baseURL) == "" {
			return nil, fmt.Errorf("%w: MinerU API URL is required when enabled", ErrParserConfigInvalid)
		}
	}
	return parserConfigRecord(organizationID, actorID, existing, ParserProviderMineru, "MinerU", enabled, baseURL, timeoutSec, credentials, metadata), nil
}

func parserConfigRecord(organizationID uuid.UUID, actorID *uuid.UUID, existing *model.ProviderConfig, providerKey, displayName string, enabled bool, baseURL string, timeoutSec int, credentials, metadata map[string]any) *model.ProviderConfig {
	item := &model.ProviderConfig{
		Scope:                 "organization",
		OrganizationID:        &organizationID,
		ProviderKey:           providerKey,
		ProviderType:          parserProviderType(providerKey),
		DisplayName:           displayName,
		Enabled:               enabled,
		Priority:              parserProviderPriority(providerKey, existing),
		AdapterName:           "hyperparse_sdk",
		EngineName:            providerKey,
		BaseURL:               strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		CredentialsCiphertext: credentials,
		TimeoutSec:            timeoutSec,
		SupportsFileTypes:     []string{},
		SupportsProfiles:      []string{},
		Metadata:              metadata,
		UpdatedBy:             actorID,
	}
	if existing != nil {
		item.ID = existing.ID
		item.CreatedBy = existing.CreatedBy
		item.SupportsFileTypes = existing.SupportsFileTypes
		item.SupportsProfiles = existing.SupportsProfiles
		item.CostLevel = existing.CostLevel
		item.PrivacyLevel = existing.PrivacyLevel
		if item.CreatedBy == nil {
			item.CreatedBy = actorID
		}
	} else {
		item.CreatedBy = actorID
	}
	return item
}

func parserProviderType(providerKey string) string {
	switch providerKey {
	case ParserProviderReducto:
		return string(contracts.ParseProviderTypeRemote)
	default:
		return string(contracts.ParseProviderTypeBuiltin)
	}
}

func parserProviderPriority(providerKey string, existing *model.ProviderConfig) int {
	if existing != nil && existing.Priority > 0 {
		return existing.Priority
	}
	switch providerKey {
	case ParserProviderReducto:
		return DefaultReductoProviderPriority
	case ParserProviderMineru:
		return DefaultMineruProviderPriority
	default:
		return 0
	}
}

func (s *providerSettingsService) toView(providerKey string, item *model.ProviderConfig) *ParserProviderSettings {
	if item == nil {
		return defaultParserSettings(providerKey)
	}
	view := defaultParserSettings(providerKey)
	view.Enabled = item.Enabled
	view.BaseURL = item.BaseURL
	view.TimeoutSec = item.TimeoutSec
	view.RuntimeConfigSource = "database"
	switch providerKey {
	case ParserProviderReducto:
		view.APIKeyConfigured = metadataString(item.CredentialsCiphertext, "api_key") != ""
		view.Configured = view.APIKeyConfigured
	case ParserProviderMineru:
		view.Mode = existingMetadataString(item, "mode", MineruModeSidecar)
		view.Configured = strings.TrimSpace(item.BaseURL) != ""
		if view.Mode == MineruModeOfficial {
			view.OfficialTokenConfigured = metadataString(item.CredentialsCiphertext, "official_token") != ""
			view.OfficialModelVersion = existingMetadataString(item, "official_model_version", DefaultMineruOfficialModelVersion)
			view.OfficialPollIntervalSeconds = existingMetadataInt(item, "official_poll_interval_seconds", DefaultMineruOfficialPollInterval)
			view.Configured = view.OfficialTokenConfigured
		}
	}
	view.Status = parserStatus(view.Enabled, view.Configured)
	return view
}

func defaultParserSettings(providerKey string) *ParserProviderSettings {
	switch providerKey {
	case ParserProviderReducto:
		return &ParserProviderSettings{
			ProviderKey:         ParserProviderReducto,
			DisplayName:         "Reducto",
			Enabled:             false,
			Configured:          false,
			Status:              "not_configured",
			BaseURL:             DefaultReductoBaseURL,
			TimeoutSec:          DefaultReductoTimeout,
			RuntimeConfigSource: "default",
		}
	default:
		return &ParserProviderSettings{
			ProviderKey:                 ParserProviderMineru,
			DisplayName:                 "MinerU",
			Enabled:                     false,
			Configured:                  false,
			Status:                      "not_configured",
			Mode:                        MineruModeSidecar,
			BaseURL:                     DefaultMineruSidecarBaseURL,
			TimeoutSec:                  DefaultMineruSidecarTimeout,
			OfficialModelVersion:        DefaultMineruOfficialModelVersion,
			OfficialPollIntervalSeconds: DefaultMineruOfficialPollInterval,
			RuntimeConfigSource:         "default",
		}
	}
}

func parserStatus(enabled, configured bool) string {
	if !configured {
		return "not_configured"
	}
	if !enabled {
		return "disabled"
	}
	return "available"
}

func (s *providerSettingsService) encryptSecret(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if s.crypto == nil {
		return "", fmt.Errorf("%w: crypto service is not configured", ErrParserConfigInvalid)
	}
	return s.crypto.Encrypt(value)
}

func existingCredentials(item *model.ProviderConfig) map[string]any {
	if item == nil {
		return nil
	}
	return item.CredentialsCiphertext
}

func existingMetadata(item *model.ProviderConfig) map[string]any {
	if item == nil {
		return nil
	}
	return item.Metadata
}

func cloneMap(input map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range input {
		out[key] = value
	}
	return out
}

func boolValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func stringValue(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	return strings.TrimSpace(*value)
}

func intValue(value *int, fallback int) int {
	if value == nil || *value <= 0 {
		return fallback
	}
	return *value
}

func existingBool(item *model.ProviderConfig, fallback bool) bool {
	if item == nil {
		return fallback
	}
	return item.Enabled
}

func existingString(item *model.ProviderConfig, field, fallback string) string {
	if item == nil {
		return fallback
	}
	switch field {
	case "base_url":
		if item.BaseURL != "" {
			return item.BaseURL
		}
	}
	return fallback
}

func existingTimeout(item *model.ProviderConfig, fallback int) int {
	if item == nil || item.TimeoutSec <= 0 {
		return fallback
	}
	return item.TimeoutSec
}

func existingMetadataString(item *model.ProviderConfig, key, fallback string) string {
	if item == nil {
		return fallback
	}
	if value := metadataString(item.Metadata, key); value != "" {
		return value
	}
	return fallback
}

func existingMetadataInt(item *model.ProviderConfig, key string, fallback int) int {
	if item == nil {
		return fallback
	}
	raw := metadataString(item.Metadata, key)
	if raw == "" {
		return fallback
	}
	var parsed int
	if _, err := fmt.Sscanf(raw, "%d", &parsed); err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
