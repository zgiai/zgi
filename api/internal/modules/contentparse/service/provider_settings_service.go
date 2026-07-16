package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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

	ParserValidationSuccess = "success"
	ParserValidationFailed  = "failed"
	ParserValidationUnknown = "unknown"

	parserValidationStatusKey  = "validation_status"
	parserValidationMessageKey = "validation_message"
	parserValidatedAtKey       = "validated_at"
)

var (
	ErrUnsupportedParserProvider = errors.New("unsupported parser provider")
	ErrParserConfigInvalid       = errors.New("parser config invalid")
	ErrParserValidationFailed    = errors.New("parser validation failed")
)

type ProviderSettingsService interface {
	List(ctx context.Context, organizationID uuid.UUID) (*ParserSettingsList, error)
	Upsert(ctx context.Context, organizationID uuid.UUID, actorID *uuid.UUID, providerKey string, input ParserSettingsInput) (*ParserProviderSettings, error)
	Check(ctx context.Context, organizationID uuid.UUID, actorID *uuid.UUID, providerKey string) (*ParserProviderSettings, error)
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
	ValidationStatus            string `json:"validation_status,omitempty"`
	ValidatedAt                 string `json:"validated_at,omitempty"`
	ValidationMessage           string `json:"validation_message,omitempty"`
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
	repo      repository.ProviderConfigRepository
	crypto    llmcrypto.CryptoService
	validator ParserProviderValidator
}

func NewProviderSettingsService(repo repository.ProviderConfigRepository, crypto llmcrypto.CryptoService, validators ...ParserProviderValidator) ProviderSettingsService {
	validator := ParserProviderValidator(defaultParserProviderValidator{})
	if len(validators) > 0 && validators[0] != nil {
		validator = validators[0]
	}
	return &providerSettingsService{repo: repo, crypto: crypto, validator: validator}
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
	if item.Enabled {
		if err := s.validateAndAnnotate(ctx, item); err != nil {
			return nil, err
		}
	} else {
		annotateParserValidation(item, ParserValidationUnknown, "", time.Time{})
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

func (s *providerSettingsService) Check(ctx context.Context, organizationID uuid.UUID, actorID *uuid.UUID, providerKey string) (*ParserProviderSettings, error) {
	providerKey = strings.ToLower(strings.TrimSpace(providerKey))
	if providerKey != ParserProviderReducto && providerKey != ParserProviderMineru {
		return nil, ErrUnsupportedParserProvider
	}
	item, err := s.repo.GetByScopeAndKey(ctx, "organization", &organizationID, nil, providerKey)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, fmt.Errorf("%w: parser provider is not configured", ErrParserConfigInvalid)
	}
	item.UpdatedBy = actorID
	if err := s.validateAndAnnotate(ctx, item); err != nil {
		_ = s.repo.Update(ctx, item)
		return nil, err
	}
	if err := s.repo.Update(ctx, item); err != nil {
		return nil, err
	}
	saved, err := s.repo.GetByScopeAndKey(ctx, "organization", &organizationID, nil, providerKey)
	if err != nil {
		return nil, err
	}
	return s.toView(providerKey, saved), nil
}

func (s *providerSettingsService) validateAndAnnotate(ctx context.Context, item *model.ProviderConfig) error {
	if item == nil {
		return fmt.Errorf("%w: parser provider is not configured", ErrParserConfigInvalid)
	}
	req, err := s.validationRequest(item)
	if err != nil {
		annotateParserValidation(item, ParserValidationFailed, err.Error(), time.Now().UTC())
		return err
	}
	if err := s.validator.Validate(ctx, req); err != nil {
		wrapped := fmt.Errorf("%w: %v", ErrParserValidationFailed, err)
		annotateParserValidation(item, ParserValidationFailed, wrapped.Error(), time.Now().UTC())
		return wrapped
	}
	annotateParserValidation(item, ParserValidationSuccess, "validation succeeded", time.Now().UTC())
	return nil
}

func (s *providerSettingsService) validationRequest(item *model.ProviderConfig) (ParserProviderValidationRequest, error) {
	providerKey := strings.ToLower(strings.TrimSpace(item.ProviderKey))
	req := ParserProviderValidationRequest{
		ProviderKey: providerKey,
		BaseURL:     strings.TrimRight(strings.TrimSpace(item.BaseURL), "/"),
		TimeoutSec:  item.TimeoutSec,
	}
	switch providerKey {
	case ParserProviderReducto:
		req.APIKey = decryptCredential(item.CredentialsCiphertext, "api_key", s.crypto)
		if req.APIKey == "" {
			return req, fmt.Errorf("%w: Reducto API key is required", ErrParserConfigInvalid)
		}
		if req.BaseURL == "" {
			req.BaseURL = DefaultReductoBaseURL
		}
	case ParserProviderMineru:
		req.Mode = strings.ToLower(strings.TrimSpace(metadataString(item.Metadata, "mode")))
		if req.Mode == "" {
			req.Mode = MineruModeSidecar
		}
		if req.Mode == MineruModeOfficial {
			req.OfficialToken = decryptCredential(item.CredentialsCiphertext, "official_token", s.crypto)
			if req.OfficialToken == "" {
				return req, fmt.Errorf("%w: MinerU official token is required", ErrParserConfigInvalid)
			}
			if req.BaseURL == "" {
				req.BaseURL = DefaultMineruOfficialBaseURL
			}
		} else if req.BaseURL == "" {
			return req, fmt.Errorf("%w: MinerU API URL is required", ErrParserConfigInvalid)
		}
	default:
		return req, ErrUnsupportedParserProvider
	}
	return req, nil
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
	view.ValidationStatus = metadataString(item.Metadata, parserValidationStatusKey)
	view.ValidatedAt = metadataString(item.Metadata, parserValidatedAtKey)
	view.ValidationMessage = metadataString(item.Metadata, parserValidationMessageKey)
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
	view.Status = parserStatus(view.Enabled, view.Configured, view.ValidationStatus)
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
			ValidationStatus:    ParserValidationUnknown,
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
			ValidationStatus:            ParserValidationUnknown,
		}
	}
}

func parserStatus(enabled, configured bool, validationStatus string) string {
	if !configured {
		return "not_configured"
	}
	if !enabled {
		return "disabled"
	}
	switch strings.ToLower(strings.TrimSpace(validationStatus)) {
	case ParserValidationSuccess:
		return "available"
	case ParserValidationFailed:
		return "failed"
	default:
		return "unknown"
	}
}

func annotateParserValidation(item *model.ProviderConfig, status string, message string, checkedAt time.Time) {
	if item == nil {
		return
	}
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	item.Metadata[parserValidationStatusKey] = strings.TrimSpace(status)
	item.Metadata[parserValidationMessageKey] = strings.TrimSpace(message)
	if checkedAt.IsZero() {
		delete(item.Metadata, parserValidatedAtKey)
		return
	}
	item.Metadata[parserValidatedAtKey] = checkedAt.UTC().Format(time.RFC3339)
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
