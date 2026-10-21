package digitalhuman

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/externalapikey"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
	"gorm.io/gorm"
)

const digitalHumanExternalAccessSource = "digital_human_access"

const (
	accessAuditActionSettingsUpdate = "external_access.updated"
	accessAuditActionAPIKeyCreate   = "api_key.created"
	accessAuditActionAPIKeyUpdate   = "api_key.updated"
	accessAuditActionAPIKeyDelete   = "api_key.deleted"
	accessAuditSourceConsole        = "console"
	accessAuditResultSuccess        = "success"
)

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

// digitalHumanAPIIssuerType identifies API end users derived from a
// DigitalHuman's `user` values. The DigitalHuman is the issuer.
const digitalHumanAPIIssuerType = "digital_human_api"

// EnsureAPIEndUserSubject registers an API end user before any stateful API
// work. Only a caller holding the raw `user` value can derive the subject, so
// the external API may register it. Without it, task and file requests from a
// user who has not chatted yet were rejected as an unknown subject.
func (s *AccessService) EnsureAPIEndUserSubject(ctx context.Context, digitalHumanID uuid.UUID, subject usersubject.Ref) error {
	if s == nil || s.db == nil || digitalHumanID == uuid.Nil || subject.Kind != usersubject.APIEndUser {
		return ErrInvalidRequest
	}
	issuer := digitalHumanID
	if _, err := usersubject.NewService(s.db).Ensure(ctx, usersubject.EnsureRequest{
		Subject: subject, IssuerType: digitalHumanAPIIssuerType, IssuerID: &issuer,
	}); err != nil {
		return fmt.Errorf("ensure DigitalHuman API end user: %w", err)
	}
	return nil
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
	authorization := runtimeauth.ResourceAuthorization{
		ResourceType: runtimeauth.PublishedRuntimeResourceDigitalHuman,
		ResourceID:   definition.ID, OrganizationID: definition.OrganizationID, WorkspaceID: &workspaceID,
		Surfaces: []runtimeauth.SurfaceAuthorization{
			publicSurface(runtimeauth.PublishedRuntimeSurfaceWebApp, settings.WebAppEnabled),
			publicSurface(runtimeauth.PublishedRuntimeSurfaceAPI, settings.APIEnabled),
			{Surface: runtimeauth.PublishedRuntimeSurfaceInternal, Enabled: true, CompatibilitySource: digitalHumanExternalAccessSource,
				Grants: []runtimeauth.SurfaceGrant{{SubjectType: runtimeauth.PublishedRuntimeSubjectInternal, Enabled: true}}},
		},
	}
	details := accessSettingsAuditDetails(accessSettings(auth), settings, request)
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := runtimeauth.NewStore(tx).SaveResourceAuthorization(ctx, authorization); err != nil {
			return err
		}
		return s.recordAccessAuditEvent(ctx, tx, scope, definition, accessAuditActionSettingsUpdate, "access_settings", nil, details)
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
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(record).Error; err != nil {
			return err
		}
		details := map[string]any{"expires_at_set": request.ExpiresAt != nil}
		return s.recordAccessAuditEvent(ctx, tx, scope, definition, accessAuditActionAPIKeyCreate, "api_key", &record.ID, details)
	}); err != nil {
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
	var updated *APIKey
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var previous APIKey
		if err := tx.Where("id = ? AND digital_human_id = ? AND organization_id = ? AND workspace_id = ? AND status <> ?",
			keyID, digitalHumanID, definition.OrganizationID, definition.WorkspaceID, APIKeyStatusRevoked).Take(&previous).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		result := tx.Model(&APIKey{}).
			Where("id = ? AND digital_human_id = ? AND organization_id = ? AND workspace_id = ? AND status <> ?",
				keyID, digitalHumanID, definition.OrganizationID, definition.WorkspaceID, APIKeyStatusRevoked).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		var err error
		updated, err = apiKeyByIDFrom(ctx, tx, definition, keyID)
		if err != nil {
			return err
		}
		details := apiKeyUpdateAuditDetails(&previous, updated, request)
		return s.recordAccessAuditEvent(ctx, tx, scope, definition, accessAuditActionAPIKeyUpdate, "api_key", &keyID, details)
	}); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("update DigitalHuman API key: %w", err)
	}
	return updated, nil
}

func (s *AccessService) DeleteAPIKey(ctx context.Context, scope Scope, digitalHumanID, keyID uuid.UUID) error {
	definition, err := s.repo.Definition(ctx, scope, digitalHumanID)
	if err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where(
			"id = ? AND digital_human_id = ? AND organization_id = ? AND workspace_id = ?",
			keyID, digitalHumanID, definition.OrganizationID, definition.WorkspaceID,
		).Delete(&APIKey{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		return s.recordAccessAuditEvent(ctx, tx, scope, definition, accessAuditActionAPIKeyDelete, "api_key", &keyID, map[string]any{})
	}); err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("delete DigitalHuman API key: %w", err)
	}
	return nil
}

func (s *AccessService) ResolveAPIKey(ctx context.Context, rawKey string) (*APIKey, *PublishedAccess, error) {
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
	if webAppID == uuid.Nil {
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

func apiKeyByIDFrom(ctx context.Context, db *gorm.DB, definition *Definition, keyID uuid.UUID) (*APIKey, error) {
	var key APIKey
	if err := db.WithContext(ctx).Where("id = ? AND digital_human_id = ? AND organization_id = ? AND workspace_id = ?",
		keyID, definition.ID, definition.OrganizationID, definition.WorkspaceID).Take(&key).Error; err != nil {
		return nil, ErrNotFound
	}
	return &key, nil
}

func (s *AccessService) recordAccessAuditEvent(
	ctx context.Context,
	tx *gorm.DB,
	scope Scope,
	definition *Definition,
	action string,
	targetType string,
	targetID *uuid.UUID,
	details map[string]any,
) error {
	if tx == nil || definition == nil || scope.ActorID == uuid.Nil || strings.TrimSpace(action) == "" || strings.TrimSpace(targetType) == "" {
		return ErrInvalidRequest
	}
	payload, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("encode DigitalHuman access audit details: %w", err)
	}
	actorID := scope.ActorID
	event := &AccessAuditEvent{
		ID: uuid.New(), DigitalHumanID: definition.ID, OrganizationID: definition.OrganizationID, WorkspaceID: definition.WorkspaceID,
		ActorID: &actorID, Action: action, TargetType: targetType, TargetID: targetID,
		Source: accessAuditSourceConsole, Result: accessAuditResultSuccess, Details: payload, CreatedAt: s.now(),
	}
	if err := tx.WithContext(ctx).Create(event).Error; err != nil {
		return fmt.Errorf("record DigitalHuman access audit event: %w", err)
	}
	return nil
}

func accessSettingsAuditDetails(previous, next AccessSettings, request AccessSettingsUpdate) map[string]any {
	details := make(map[string]any, 2)
	if request.WebAppEnabled != nil {
		details["webapp_enabled"] = map[string]bool{"from": previous.WebAppEnabled, "to": next.WebAppEnabled}
	}
	if request.APIEnabled != nil {
		details["api_enabled"] = map[string]bool{"from": previous.APIEnabled, "to": next.APIEnabled}
	}
	return details
}

func apiKeyUpdateAuditDetails(previous, next *APIKey, request APIKeyUpdateRequest) map[string]any {
	details := make(map[string]any, 3)
	if request.Name != nil {
		details["name_changed"] = true
	}
	if request.Status != nil && previous != nil && next != nil {
		details["status"] = map[string]string{"from": previous.Status, "to": next.Status}
	}
	if request.ExpiresAt != nil {
		details["expires_at_changed"] = true
	}
	return details
}

// BindExternalFile assigns a newly uploaded temporary file to exactly one
// DigitalHuman user subject and public surface.
func (s *AccessService) BindExternalFile(ctx context.Context, scope Scope, digitalHumanID, fileID uuid.UUID, surface string) error {
	if fileID == uuid.Nil || digitalHumanID == uuid.Nil || scope.OrganizationID == uuid.Nil || scope.WorkspaceID == uuid.Nil ||
		scope.UserSubjectID == nil || *scope.UserSubjectID == uuid.Nil || strings.TrimSpace(scope.UserSubjectKind) == "" ||
		(surface != SurfaceAPI && surface != SurfaceWebApp) || scope.Surface != surface {
		return ErrInvalidRequest
	}
	record := &ExternalFileBinding{
		FileID: fileID, DigitalHumanID: digitalHumanID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		UserSubjectKind: scope.UserSubjectKind, UserSubjectID: *scope.UserSubjectID, Surface: surface, CreatedAt: s.now(),
	}
	if err := s.db.WithContext(ctx).Create(record).Error; err != nil {
		return fmt.Errorf("bind DigitalHuman external file: %w", err)
	}
	return nil
}

// ValidateExternalFiles ensures every attachment was uploaded by the current
// user subject for this exact DigitalHuman and public surface.
func (s *AccessService) ValidateExternalFiles(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, fileIDs []string, surface string) error {
	if len(fileIDs) == 0 {
		return nil
	}
	if digitalHumanID == uuid.Nil || scope.UserSubjectID == nil || *scope.UserSubjectID == uuid.Nil ||
		strings.TrimSpace(scope.UserSubjectKind) == "" || (surface != SurfaceAPI && surface != SurfaceWebApp) || scope.Surface != surface {
		return ErrNotFound
	}
	parsed := make([]uuid.UUID, 0, len(fileIDs))
	seen := make(map[uuid.UUID]struct{}, len(fileIDs))
	for _, raw := range fileIDs {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil || id == uuid.Nil {
			return ErrInvalidRequest
		}
		if _, exists := seen[id]; exists {
			return ErrInvalidRequest
		}
		seen[id] = struct{}{}
		parsed = append(parsed, id)
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&ExternalFileBinding{}).
		Where("file_id IN ? AND digital_human_id = ? AND organization_id = ? AND workspace_id = ?", parsed, digitalHumanID, scope.OrganizationID, scope.WorkspaceID).
		Where("user_subject_kind = ? AND user_subject_id = ? AND surface = ?", scope.UserSubjectKind, *scope.UserSubjectID, surface).
		Count(&count).Error; err != nil {
		return fmt.Errorf("validate DigitalHuman external files: %w", err)
	}
	if count != int64(len(parsed)) {
		return ErrNotFound
	}
	return nil
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
