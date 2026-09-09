package digitalhuman

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/externalapikey"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
	"gorm.io/gorm"
)

const digitalHumanExternalAccessSource = "digital_human_access"

// APIKeyCreateRequest creates one DigitalHuman-bound API credential.
type APIKeyCreateRequest struct {
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// APIKeyUpdateRequest updates the mutable metadata of an API credential.
type APIKeyUpdateRequest struct {
	Name      *string    `json:"name,omitempty"`
	Status    *string    `json:"status,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// APIKeyCreated is returned exactly once with the plaintext credential.
type APIKeyCreated struct {
	*APIKey
	Key string `json:"api_key"`
}

// AccessSettingsUpdate changes external surfaces independently of publication.
type AccessSettingsUpdate struct {
	WebAppEnabled *bool `json:"webapp_enabled,omitempty"`
	APIEnabled    *bool `json:"api_enabled,omitempty"`
}

// PublishedAccess contains the public-safe published snapshot and access state.
type PublishedAccess struct {
	Definition *Definition
	Version    *Version
	Settings   AccessSettings
}

// AccessService owns DigitalHuman external surfaces and API credentials.
type AccessService struct {
	db          *gorm.DB
	repo        *GormRepository
	runtimeAuth *runtimeauth.Store
	now         func() time.Time
}

func NewAccessService(db *gorm.DB) *AccessService {
	if db == nil {
		panic("digital human access service requires db")
	}
	return &AccessService{db: db, repo: NewRepository(db), runtimeAuth: runtimeauth.NewStore(db), now: func() time.Time { return time.Now().UTC() }}
}

func (s *AccessService) GetSettings(ctx context.Context, scope Scope, digitalHumanID uuid.UUID) (*AccessSettings, error) {
	definition, err := s.repo.Definition(ctx, scope, digitalHumanID)
	if err != nil {
		return nil, err
	}
	auth, err := s.authorization(ctx, definition)
	if err != nil {
		return nil, err
	}
	settings := accessSettings(auth)
	return &settings, nil
}

func (s *AccessService) UpdateSettings(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, request AccessSettingsUpdate) (*AccessSettings, error) {
	if request.WebAppEnabled == nil && request.APIEnabled == nil {
		return nil, ErrInvalidRequest
	}
	definition, err := s.repo.Definition(ctx, scope, digitalHumanID)
	if err != nil {
		return nil, err
	}
	if ((request.WebAppEnabled != nil && *request.WebAppEnabled) || (request.APIEnabled != nil && *request.APIEnabled)) &&
		(definition.Status != StatusActive || definition.PublishedVersionID == nil) {
		return nil, ErrPublishInvalid
	}
	auth, err := s.authorization(ctx, definition)
	if err != nil {
		return nil, err
	}
	settings := accessSettings(auth)
	if request.WebAppEnabled != nil {
		settings.WebAppEnabled = *request.WebAppEnabled
	}
	if request.APIEnabled != nil {
		settings.APIEnabled = *request.APIEnabled
	}
	workspaceID := definition.WorkspaceID
	if err := s.runtimeAuth.SaveResourceAuthorization(ctx, runtimeauth.ResourceAuthorization{
		ResourceType: runtimeauth.PublishedRuntimeResourceDigitalHuman,
		ResourceID:   definition.ID, OrganizationID: definition.OrganizationID, WorkspaceID: &workspaceID,
		Surfaces: []runtimeauth.SurfaceAuthorization{
			publicSurface(runtimeauth.PublishedRuntimeSurfaceWebApp, settings.WebAppEnabled),
			publicSurface(runtimeauth.PublishedRuntimeSurfaceAPI, settings.APIEnabled),
			{Surface: runtimeauth.PublishedRuntimeSurfaceInternal, Enabled: true, CompatibilitySource: digitalHumanExternalAccessSource,
				Grants: []runtimeauth.SurfaceGrant{{SubjectType: runtimeauth.PublishedRuntimeSubjectInternal, Enabled: true}}},
		},
	}); err != nil {
		return nil, fmt.Errorf("save DigitalHuman external access: %w", err)
	}
	return &settings, nil
}

func publicSurface(surface runtimeauth.PublishedRuntimeSurface, enabled bool) runtimeauth.SurfaceAuthorization {
	return runtimeauth.SurfaceAuthorization{Surface: surface, Enabled: enabled, CompatibilitySource: digitalHumanExternalAccessSource,
		Grants: []runtimeauth.SurfaceGrant{{SubjectType: runtimeauth.PublishedRuntimeSubjectPublic, Enabled: enabled}}}
}

func (s *AccessService) ListAPIKeys(ctx context.Context, scope Scope, digitalHumanID uuid.UUID) ([]*APIKey, error) {
	definition, err := s.repo.Definition(ctx, scope, digitalHumanID)
	if err != nil {
		return nil, err
	}
	items := make([]*APIKey, 0)
	if err := s.db.WithContext(ctx).Where("digital_human_id = ? AND organization_id = ? AND workspace_id = ?",
		digitalHumanID, definition.OrganizationID, definition.WorkspaceID).
		Order("created_at DESC, id DESC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list DigitalHuman API keys: %w", err)
	}
	return items, nil
}

func (s *AccessService) CreateAPIKey(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, request APIKeyCreateRequest) (*APIKeyCreated, error) {
	definition, err := s.repo.Definition(ctx, scope, digitalHumanID)
	if err != nil {
		return nil, err
	}
	request.Name = strings.TrimSpace(request.Name)
	if definition.PublishedVersionID == nil || request.Name == "" || len([]rune(request.Name)) > 255 ||
		(request.ExpiresAt != nil && !request.ExpiresAt.After(s.now())) {
		return nil, ErrInvalidRequest
	}
	key, prefix, hash, err := externalapikey.Generate()
	if err != nil {
		return nil, fmt.Errorf("generate DigitalHuman API key: %w", err)
	}
	now := s.now()
	record := &APIKey{ID: uuid.New(), DigitalHumanID: digitalHumanID, OrganizationID: definition.OrganizationID,
		WorkspaceID: definition.WorkspaceID, KeyHash: hash, KeyPrefix: prefix, Name: request.Name,
		Status: APIKeyStatusActive, ExpiresAt: request.ExpiresAt, CreatedAt: now, UpdatedAt: now}
	if err := s.db.WithContext(ctx).Create(record).Error; err != nil {
		return nil, fmt.Errorf("create DigitalHuman API key: %w", err)
	}
	return &APIKeyCreated{APIKey: record, Key: key}, nil
}

func (s *AccessService) UpdateAPIKey(ctx context.Context, scope Scope, digitalHumanID, keyID uuid.UUID, request APIKeyUpdateRequest) (*APIKey, error) {
	definition, err := s.repo.Definition(ctx, scope, digitalHumanID)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{"updated_at": s.now()}
	if request.Name != nil {
		name := strings.TrimSpace(*request.Name)
		if name == "" || len([]rune(name)) > 255 {
			return nil, ErrInvalidRequest
		}
		updates["name"] = name
	}
	if request.Status != nil {
		status := strings.TrimSpace(*request.Status)
		if status != APIKeyStatusActive && status != APIKeyStatusRevoked {
			return nil, ErrInvalidRequest
		}
		updates["status"] = status
	}
	if request.ExpiresAt != nil {
		if !request.ExpiresAt.After(s.now()) {
			return nil, ErrInvalidRequest
		}
		updates["expires_at"] = request.ExpiresAt.UTC()
	}
	if len(updates) == 1 {
		return nil, ErrInvalidRequest
	}
	result := s.db.WithContext(ctx).Model(&APIKey{}).
		Where("id = ? AND digital_human_id = ? AND organization_id = ? AND workspace_id = ? AND status <> ?",
			keyID, digitalHumanID, definition.OrganizationID, definition.WorkspaceID, APIKeyStatusRevoked).
		Updates(updates)
	if result.Error != nil {
		return nil, fmt.Errorf("update DigitalHuman API key: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return nil, ErrNotFound
	}
	return s.apiKeyByID(ctx, definition, keyID)
}

func (s *AccessService) DeleteAPIKey(ctx context.Context, scope Scope, digitalHumanID, keyID uuid.UUID) error {
	definition, err := s.repo.Definition(ctx, scope, digitalHumanID)
	if err != nil {
		return err
	}
	result := s.db.WithContext(ctx).Where(
		"id = ? AND digital_human_id = ? AND organization_id = ? AND workspace_id = ?",
		keyID, digitalHumanID, definition.OrganizationID, definition.WorkspaceID,
	).Delete(&APIKey{})
	if result.Error != nil {
		return fmt.Errorf("delete DigitalHuman API key: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrNotFound
	}
	return nil
}

func (s *AccessService) ResolveAPIKey(ctx context.Context, rawKey string) (*APIKey, *PublishedAccess, error) {
	if !ExternalAPIRuntimeEnabled() {
		return nil, nil, ErrPublishInvalid
	}
	var key APIKey
	err := s.db.WithContext(ctx).Where("key_hash = ? AND status = ?", externalapikey.Hash(rawKey), APIKeyStatusActive).
		Where("expires_at IS NULL OR expires_at > ?", s.now()).Take(&key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("resolve DigitalHuman API key: %w", err)
	}
	access, err := s.publishedAccess(ctx, key.DigitalHumanID, key.OrganizationID, key.WorkspaceID)
	if err != nil || !access.Settings.APIEnabled {
		if err != nil {
			return nil, nil, err
		}
		return nil, nil, ErrPublishInvalid
	}
	now := s.now()
	_ = s.db.WithContext(ctx).Model(&APIKey{}).Where("id = ?", key.ID).Updates(map[string]any{
		"last_used_at": now, "usage_count": gorm.Expr("usage_count + 1"), "updated_at": now,
	}).Error
	return &key, access, nil
}

func (s *AccessService) ResolveWebApp(ctx context.Context, webAppID uuid.UUID) (*PublishedAccess, error) {
	if !PublicWebAppRuntimeEnabled() || webAppID == uuid.Nil {
		return nil, ErrNotFound
	}
	var definition Definition
	if err := s.db.WithContext(ctx).Where("web_app_id = ? AND status = ? AND deleted_at IS NULL", webAppID, StatusActive).Take(&definition).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("resolve DigitalHuman webapp: %w", err)
	}
	access, err := s.publishedAccess(ctx, definition.ID, definition.OrganizationID, definition.WorkspaceID)
	if err != nil || !access.Settings.WebAppEnabled {
		return nil, ErrNotFound
	}
	return access, nil
}

func (s *AccessService) publishedAccess(ctx context.Context, digitalHumanID, organizationID, workspaceID uuid.UUID) (*PublishedAccess, error) {
	var definition Definition
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ? AND workspace_id = ? AND status = ? AND deleted_at IS NULL",
		digitalHumanID, organizationID, workspaceID, StatusActive).Take(&definition).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if definition.PublishedVersionID == nil {
		return nil, ErrPublishInvalid
	}
	var version Version
	if err := s.db.WithContext(ctx).Where("id = ? AND digital_human_id = ? AND state = ?", *definition.PublishedVersionID, definition.ID, VersionStatePublished).Take(&version).Error; err != nil {
		return nil, ErrVersionNotFound
	}
	auth, err := s.authorization(ctx, &definition)
	if err != nil {
		return nil, err
	}
	return &PublishedAccess{Definition: &definition, Version: &version, Settings: accessSettings(auth)}, nil
}

func (s *AccessService) authorization(ctx context.Context, definition *Definition) (*runtimeauth.ResourceAuthorization, error) {
	if definition == nil {
		return nil, ErrNotFound
	}
	return s.runtimeAuth.GetResourceAuthorization(ctx, runtimeauth.PublishedRuntimeResourceDigitalHuman, definition.ID,
		runtimeauth.PublishedRuntimePolicy{WebAppStatus: runtimeauth.WebAppStatusInactive, APIEnabled: false, InternalInvocation: true})
}

func accessSettings(auth *runtimeauth.ResourceAuthorization) AccessSettings {
	public := runtimeauth.RuntimeAudience{}
	return AccessSettings{WebAppEnabled: auth != nil && auth.Allows(runtimeauth.PublishedRuntimeSurfaceWebApp, public),
		APIEnabled: auth != nil && auth.Allows(runtimeauth.PublishedRuntimeSurfaceAPI, public)}
}

func (s *AccessService) apiKeyByID(ctx context.Context, definition *Definition, keyID uuid.UUID) (*APIKey, error) {
	var key APIKey
	if err := s.db.WithContext(ctx).Where("id = ? AND digital_human_id = ? AND organization_id = ? AND workspace_id = ?",
		keyID, definition.ID, definition.OrganizationID, definition.WorkspaceID).Take(&key).Error; err != nil {
		return nil, ErrNotFound
	}
	return &key, nil
}

func presentationPolicy(version *Version, definition *Definition) PresentationPolicy {
	policy := PresentationPolicy{}
	if version != nil && len(version.PresentationPolicy) > 0 {
		_ = json.Unmarshal(version.PresentationPolicy, &policy)
	}
	if definition != nil {
		if strings.TrimSpace(policy.HomeTitle) == "" {
			policy.HomeTitle = definition.Name
		}
		if strings.TrimSpace(policy.OpeningStatement) == "" {
			policy.OpeningStatement = definition.Description
		}
	}
	return policy
}

func externalFeatureEnabled(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func ExternalAPIRuntimeEnabled() bool {
	return externalFeatureEnabled("ZGI_DIGITAL_HUMAN_EXTERNAL_API_ENABLED")
}
func PublicWebAppRuntimeEnabled() bool {
	return externalFeatureEnabled("ZGI_DIGITAL_HUMAN_PUBLIC_WEBAPP_ENABLED")
}
