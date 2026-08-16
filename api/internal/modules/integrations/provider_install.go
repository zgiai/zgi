package integrations

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ProviderInstallFlowStatus string

const (
	ProviderInstallFlowPending   ProviderInstallFlowStatus = "pending"
	ProviderInstallFlowSucceeded ProviderInstallFlowStatus = "succeeded"
	ProviderInstallFlowFailed    ProviderInstallFlowStatus = "failed"
	ProviderInstallFlowExpired   ProviderInstallFlowStatus = "expired"
	ProviderInstallFlowCancelled ProviderInstallFlowStatus = "cancelled"
)

type IntegrationProviderInstallFlow struct {
	ID                    uuid.UUID                  `gorm:"type:uuid;primaryKey"`
	FlowDigest            string                     `gorm:"size:64;not null;uniqueIndex"`
	StateDigest           string                     `gorm:"size:64;not null;uniqueIndex"`
	BrowserBindingDigest  string                     `gorm:"size:64;not null"`
	OrganizationID        uuid.UUID                  `gorm:"type:uuid;not null;index"`
	AccountID             uuid.UUID                  `gorm:"type:uuid;not null;index"`
	CompletedConnectionID *uuid.UUID                 `gorm:"type:uuid"`
	IntegrationID         string                     `gorm:"size:64;not null"`
	DriverID              string                     `gorm:"size:64;not null"`
	AuthMethodID          string                     `gorm:"size:128;not null"`
	CredentialSource      ConnectionCredentialSource `gorm:"size:32;not null"`
	ConnectionName        string                     `gorm:"size:128;not null"`
	ReturnPath            string                     `gorm:"size:2048;not null"`
	Status                ProviderInstallFlowStatus  `gorm:"size:32;not null;index"`
	FailureCode           *string                    `gorm:"size:64"`
	AccountDisplayName    *string                    `gorm:"size:255"`
	ExpiresAt             time.Time                  `gorm:"not null;index"`
	CompletedAt           *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

func (IntegrationProviderInstallFlow) TableName() string { return "integration_provider_install_flows" }

func (flow *IntegrationProviderInstallFlow) BeforeCreate(_ *gorm.DB) error {
	if flow.ID == uuid.Nil {
		flow.ID = uuid.New()
	}
	return nil
}

type IntegrationProviderAuthorization struct {
	ID                 uuid.UUID `gorm:"type:uuid;primaryKey"`
	IntegrationID      string    `gorm:"size:64;not null;uniqueIndex:idx_provider_authorization_tenant,priority:1"`
	ExternalTenantHash string    `gorm:"size:64;not null;uniqueIndex:idx_provider_authorization_tenant,priority:2"`
	OrganizationID     uuid.UUID `gorm:"type:uuid;not null;index"`
	ConnectionID       uuid.UUID `gorm:"type:uuid;not null;uniqueIndex"`
	Active             bool      `gorm:"not null;default:true"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (IntegrationProviderAuthorization) TableName() string {
	return "integration_provider_authorizations"
}

func (authorization *IntegrationProviderAuthorization) BeforeCreate(_ *gorm.DB) error {
	if authorization.ID == uuid.Nil {
		authorization.ID = uuid.New()
	}
	return nil
}

type ProviderInstallStartRequest struct {
	OrganizationID       uuid.UUID
	AccountID            uuid.UUID
	BrowserBindingDigest string
	IntegrationID        string
	AuthMethodID         string
	CredentialSource     ConnectionCredentialSource
	ConnectionName       string
	RedirectURI          string
	ReturnPath           string
}

type ProviderInstallAuthorizationRequest struct {
	RedirectURI string
	State       string
}

type ProviderInstallExchangeRequest struct {
	AuthorizationCode string
}

type ProviderInstallGrant struct {
	ExternalTenantHash string
	AccountDisplayName string
	Credentials        map[string]string
	Config             map[string]any
	GrantedScopes      []string
}

func (grant *ProviderInstallGrant) Destroy() {
	if grant == nil {
		return
	}
	destroyCredentialMap(grant.Credentials)
	grant.Credentials = nil
	grant.ExternalTenantHash = ""
	grant.AccountDisplayName = ""
	grant.Config = nil
	for index := range grant.GrantedScopes {
		grant.GrantedScopes[index] = ""
	}
	grant.GrantedScopes = nil
}

type ProviderInstallProvider interface {
	BuildProviderInstallAuthorizationURL(context.Context, ProviderInstallAuthorizationRequest) (string, error)
	ExchangeProviderInstallCode(context.Context, ProviderInstallExchangeRequest) (ProviderInstallGrant, error)
}

type ProviderInstallFlowStartResult struct {
	FlowID           string                    `json:"flow_id"`
	AuthorizationURL string                    `json:"authorization_url"`
	Status           ProviderInstallFlowStatus `json:"status"`
	ExpiresAt        time.Time                 `json:"expires_at"`
	NextPollAfterMS  int                       `json:"next_poll_after_ms"`
}

type ProviderInstallFlowView struct {
	FlowID                string                     `json:"flow_id"`
	IntegrationID         string                     `json:"integration_id"`
	AuthMethodID          string                     `json:"auth_method_id"`
	CredentialSource      ConnectionCredentialSource `json:"credential_source"`
	Intent                OAuthFlowIntent            `json:"intent"`
	Status                ProviderInstallFlowStatus  `json:"status"`
	ConnectionName        string                     `json:"connection_name,omitempty"`
	AccountDisplayName    string                     `json:"account_display_name,omitempty"`
	ErrorCode             string                     `json:"error_code,omitempty"`
	ExpiresAt             time.Time                  `json:"expires_at"`
	CompletedAt           *time.Time                 `json:"completed_at,omitempty"`
	CompletedConnectionID *uuid.UUID                 `json:"completed_connection_id,omitempty"`
	UsageRulesRequired    bool                       `json:"usage_rules_required"`
	NextPollAfterMS       int                        `json:"next_poll_after_ms"`
}

type ProviderInstallCallbackResult struct {
	FlowID     string
	ReturnPath string
	Status     ProviderInstallFlowStatus
	ErrorCode  string
}

type ProviderInstallFlowService struct {
	db       *gorm.DB
	cipher   CredentialCipher
	registry *Registry
	ttl      time.Duration
	now      func() time.Time
}

func NewProviderInstallFlowService(db *gorm.DB, cipher CredentialCipher, registry *Registry, ttl time.Duration) *ProviderInstallFlowService {
	if ttl <= 0 || ttl > 30*time.Minute {
		ttl = 10 * time.Minute
	}
	return &ProviderInstallFlowService{db: db, cipher: cipher, registry: registry, ttl: ttl, now: time.Now}
}

func (service *ProviderInstallFlowService) Start(ctx context.Context, request ProviderInstallStartRequest) (ProviderInstallFlowStartResult, error) {
	if service == nil || service.db == nil || service.cipher == nil || service.registry == nil {
		return ProviderInstallFlowStartResult{}, fmt.Errorf("provider install flow service is unavailable")
	}
	definition, method, provider, err := service.resolveMethod(request.IntegrationID, request.AuthMethodID)
	if err != nil {
		return ProviderInstallFlowStartResult{}, err
	}
	if request.OrganizationID == uuid.Nil || request.AccountID == uuid.Nil || !sha256HexPattern.MatchString(request.BrowserBindingDigest) {
		return ProviderInstallFlowStartResult{}, invalidInput("provider install actor binding is invalid", nil)
	}
	if request.CredentialSource == "" {
		request.CredentialSource = method.CredentialSource
	}
	if request.CredentialSource != method.CredentialSource || request.CredentialSource != ConnectionCredentialSourceOrganization {
		return ProviderInstallFlowStartResult{}, invalidInput("provider install credential ownership is invalid", nil)
	}
	name, err := normalizeConnectionName(request.ConnectionName)
	if err != nil {
		return ProviderInstallFlowStartResult{}, err
	}
	flowRef, err := randomProviderInstallReference()
	if err != nil {
		return ProviderInstallFlowStartResult{}, err
	}
	state, err := randomProviderInstallState()
	if err != nil {
		return ProviderInstallFlowStartResult{}, err
	}
	authorizationURL, err := provider.BuildProviderInstallAuthorizationURL(ctx, ProviderInstallAuthorizationRequest{
		RedirectURI: strings.TrimSpace(request.RedirectURI), State: state,
	})
	if err != nil {
		return ProviderInstallFlowStartResult{}, err
	}
	now := service.now().UTC()
	flow := &IntegrationProviderInstallFlow{
		ID: uuid.New(), FlowDigest: providerInstallDigest(flowRef), StateDigest: providerInstallDigest(state),
		BrowserBindingDigest: request.BrowserBindingDigest, OrganizationID: request.OrganizationID,
		AccountID: request.AccountID, IntegrationID: definition.ID, DriverID: definition.DriverID,
		AuthMethodID: method.ID, CredentialSource: request.CredentialSource, ConnectionName: name,
		ReturnPath: safeProviderInstallReturnPath(request.ReturnPath), Status: ProviderInstallFlowPending,
		ExpiresAt: now.Add(service.ttl),
	}
	if err := service.db.WithContext(ctx).Create(flow).Error; err != nil {
		return ProviderInstallFlowStartResult{}, fmt.Errorf("create provider install flow: %w", err)
	}
	return ProviderInstallFlowStartResult{
		FlowID: flowRef, AuthorizationURL: authorizationURL, Status: flow.Status,
		ExpiresAt: flow.ExpiresAt, NextPollAfterMS: 1500,
	}, nil
}

func (service *ProviderInstallFlowService) Poll(ctx context.Context, flowRef string, organizationID, accountID uuid.UUID) (ProviderInstallFlowView, error) {
	flow, err := service.getForActor(ctx, flowRef, organizationID, accountID)
	if err != nil {
		return ProviderInstallFlowView{}, err
	}
	if flow.Status == ProviderInstallFlowPending && !flow.ExpiresAt.After(service.now().UTC()) {
		now := service.now().UTC()
		failure := ErrorCodeAuthInvalid
		result := service.db.WithContext(ctx).Model(&IntegrationProviderInstallFlow{}).
			Where("id = ? AND status = ?", flow.ID, ProviderInstallFlowPending).
			Updates(map[string]any{"status": ProviderInstallFlowExpired, "failure_code": failure, "completed_at": now, "updated_at": now})
		if result.Error != nil {
			return ProviderInstallFlowView{}, fmt.Errorf("expire provider install flow: %w", result.Error)
		}
		if result.RowsAffected == 1 {
			flow.Status, flow.FailureCode, flow.CompletedAt = ProviderInstallFlowExpired, &failure, &now
		}
	}
	return providerInstallFlowView(flowRef, flow), nil
}

func (service *ProviderInstallFlowService) Cancel(ctx context.Context, flowRef string, organizationID, accountID uuid.UUID) error {
	flow, err := service.getForActor(ctx, flowRef, organizationID, accountID)
	if err != nil {
		return err
	}
	if flow.Status != ProviderInstallFlowPending {
		return nil
	}
	now := service.now().UTC()
	result := service.db.WithContext(ctx).Model(&IntegrationProviderInstallFlow{}).
		Where("id = ? AND status = ?", flow.ID, ProviderInstallFlowPending).
		Updates(map[string]any{"status": ProviderInstallFlowCancelled, "completed_at": now, "updated_at": now})
	if result.Error != nil {
		return fmt.Errorf("cancel provider install flow: %w", result.Error)
	}
	return nil
}

func (service *ProviderInstallFlowService) Callback(ctx context.Context, state, browserBindingDigest, authorizationCode string) (ProviderInstallCallbackResult, error) {
	result := ProviderInstallCallbackResult{Status: ProviderInstallFlowFailed}
	if service == nil || service.db == nil || service.cipher == nil || service.registry == nil {
		return result, fmt.Errorf("provider install flow service is unavailable")
	}
	stateDigest := providerInstallDigest(strings.TrimSpace(state))
	if strings.TrimSpace(state) == "" || !sha256HexPattern.MatchString(browserBindingDigest) || strings.TrimSpace(authorizationCode) == "" {
		return result, NewError(ErrorCodeAuthInvalid, "provider install callback is invalid", nil)
	}
	var flow IntegrationProviderInstallFlow
	err := service.db.WithContext(ctx).Where("state_digest = ?", stateDigest).First(&flow).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return result, NewError(ErrorCodeAuthInvalid, "provider install state is expired or already used", nil)
	}
	if err != nil {
		return result, fmt.Errorf("get provider install state: %w", err)
	}
	result.FlowID = ""
	result.ReturnPath = flow.ReturnPath
	if flow.Status != ProviderInstallFlowPending || !flow.ExpiresAt.After(service.now().UTC()) || flow.BrowserBindingDigest != browserBindingDigest {
		return result, NewError(ErrorCodeAuthInvalid, "provider install state is expired or already used", nil)
	}
	_, _, provider, err := service.resolveMethod(flow.IntegrationID, flow.AuthMethodID)
	if err != nil {
		_ = service.failFlow(ctx, flow.ID, ErrorCodeDisabled)
		result.ErrorCode = ErrorCodeDisabled
		return result, err
	}
	grant, err := provider.ExchangeProviderInstallCode(ctx, ProviderInstallExchangeRequest{AuthorizationCode: authorizationCode})
	if err != nil {
		code := ErrorCode(err)
		if code == "" {
			code = ErrorCodeAuthInvalid
		}
		_ = service.failFlow(ctx, flow.ID, code)
		result.ErrorCode = code
		return result, err
	}
	defer grant.Destroy()
	if !sha256HexPattern.MatchString(grant.ExternalTenantHash) || len(grant.Credentials) == 0 {
		_ = service.failFlow(ctx, flow.ID, ErrorCodeResponseInvalid)
		result.ErrorCode = ErrorCodeResponseInvalid
		return result, NewError(ErrorCodeResponseInvalid, "provider install grant is incomplete", nil)
	}
	connectionID := uuid.New()
	envelope, err := service.cipher.EncryptCredentials(grant.Credentials, CredentialAAD{
		OrganizationID: flow.OrganizationID, ConnectionID: connectionID,
		IntegrationID: flow.IntegrationID, CredentialVersion: 1,
	})
	if err != nil {
		_ = service.failFlow(ctx, flow.ID, ErrorCodeConnectionInvalid)
		result.ErrorCode = ErrorCodeConnectionInvalid
		return result, NewError(ErrorCodeConnectionInvalid, "provider install credentials could not be encrypted", err)
	}
	completedAt := service.now().UTC()
	displayName := boundedProviderInstallText(grant.AccountDisplayName, 255)
	accountID := grant.ExternalTenantHash
	connection := &IntegrationConnection{
		ID: connectionID, OrganizationID: flow.OrganizationID, IntegrationID: flow.IntegrationID,
		DriverID: flow.DriverID, Name: flow.ConnectionName,
		CredentialSource: flow.CredentialSource, AuthType: ConnectionAuthTypeProviderInstall,
		AuthMethodID: flow.AuthMethodID, EncryptedCredentials: &envelope,
		Config: cloneAnyMap(grant.Config), AccountID: &accountID, DisplayName: &displayName,
		GrantedScopes: normalizeScopes(grant.GrantedScopes), Status: ConnectionStatusPending,
		HealthStatus: ConnectionHealthUnknown, AuthStatus: ConnectionAuthValid,
		ScopeStatus: ConnectionScopeUnknown, MissingRequiredScopes: []string{},
		VerifiedActionIDs: []string{}, DeniedActionIDs: []string{}, CredentialVersion: 1,
		Revision: 1, HealthRevision: 1, SetupVersion: 1, CreatedBy: &flow.AccountID, UpdatedBy: &flow.AccountID,
	}
	err = service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked IntegrationProviderInstallFlow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", flow.ID).First(&locked).Error; err != nil {
			return err
		}
		if locked.Status != ProviderInstallFlowPending || !locked.ExpiresAt.After(completedAt) {
			return ErrConnectionChanged
		}
		var existing int64
		if err := tx.Model(&IntegrationProviderAuthorization{}).
			Where("integration_id = ? AND external_tenant_hash = ? AND active = ?", flow.IntegrationID, grant.ExternalTenantHash, true).
			Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			return NewError(ErrorCodeConnectionConflict, "provider tenant is already connected", nil)
		}
		if err := tx.Create(connection).Error; err != nil {
			return err
		}
		authorization := &IntegrationProviderAuthorization{
			IntegrationID: flow.IntegrationID, ExternalTenantHash: grant.ExternalTenantHash,
			OrganizationID: flow.OrganizationID, ConnectionID: connection.ID, Active: true,
		}
		createdAuthorization := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(authorization)
		if createdAuthorization.Error != nil {
			return createdAuthorization.Error
		}
		if createdAuthorization.RowsAffected == 0 {
			reactivated := tx.Model(&IntegrationProviderAuthorization{}).
				Where("integration_id = ? AND external_tenant_hash = ? AND active = ?", flow.IntegrationID, grant.ExternalTenantHash, false).
				Updates(map[string]any{
					"organization_id": flow.OrganizationID,
					"connection_id":   connection.ID,
					"active":          true,
					"updated_at":      completedAt,
				})
			if reactivated.Error != nil {
				return reactivated.Error
			}
			if reactivated.RowsAffected != 1 {
				return NewError(ErrorCodeConnectionConflict, "provider tenant is already connected", nil)
			}
		}
		return tx.Model(&IntegrationProviderInstallFlow{}).Where("id = ? AND status = ?", flow.ID, ProviderInstallFlowPending).
			Updates(map[string]any{
				"status": ProviderInstallFlowSucceeded, "completed_connection_id": connection.ID,
				"account_display_name": displayName, "failure_code": nil,
				"completed_at": completedAt, "updated_at": completedAt,
			}).Error
	})
	if err != nil {
		code := ErrorCode(err)
		if code == "" {
			code = ErrorCodeConnectionInvalid
		}
		_ = service.failFlow(ctx, flow.ID, code)
		result.ErrorCode = code
		return result, err
	}
	result.Status = ProviderInstallFlowSucceeded
	return result, nil
}

func (service *ProviderInstallFlowService) DisableAuthorization(ctx context.Context, integrationID, externalTenantHash, errorCode string) error {
	if service == nil || service.db == nil || !sha256HexPattern.MatchString(externalTenantHash) {
		return invalidInput("provider authorization identity is invalid", nil)
	}
	return service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var authorization IntegrationProviderAuthorization
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("integration_id = ? AND external_tenant_hash = ? AND active = ?", strings.ToLower(strings.TrimSpace(integrationID)), externalTenantHash, true).
			First(&authorization).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		attention := ConnectionAttentionReconnectRequired
		if strings.TrimSpace(errorCode) == "" {
			errorCode = ErrorCodeReconnectRequired
		}
		if err := tx.Model(&IntegrationConnection{}).Where("id = ? AND organization_id = ?", authorization.ConnectionID, authorization.OrganizationID).
			Updates(map[string]any{
				"status": ConnectionStatusDisabled, "auth_status": ConnectionAuthReconnectRequired,
				"attention_code": attention, "last_error_code": errorCode,
				"encrypted_credentials": nil, "is_default": false,
				"credential_version": gorm.Expr("credential_version + 1"), "revision": gorm.Expr("revision + 1"),
				"updated_at": gorm.Expr("CURRENT_TIMESTAMP"),
			}).Error; err != nil {
			return err
		}
		return tx.Model(&authorization).Updates(map[string]any{"active": false, "updated_at": gorm.Expr("CURRENT_TIMESTAMP")}).Error
	})
}

func (service *ProviderInstallFlowService) MarkAuthorizationChanged(ctx context.Context, integrationID, externalTenantHash string) error {
	if service == nil || service.db == nil || !sha256HexPattern.MatchString(externalTenantHash) {
		return invalidInput("provider authorization identity is invalid", nil)
	}
	return service.db.WithContext(ctx).Model(&IntegrationConnection{}).
		Where("id = (?)", service.db.Model(&IntegrationProviderAuthorization{}).
			Select("connection_id").Where("integration_id = ? AND external_tenant_hash = ? AND active = ?", strings.ToLower(strings.TrimSpace(integrationID)), externalTenantHash, true)).
		Updates(map[string]any{
			"status":              ConnectionStatusPending,
			"is_default":          false,
			"health_status":       ConnectionHealthUnknown,
			"scope_status":        ConnectionScopeUnknown,
			"attention_code":      ConnectionAttentionAdminCheckRequired,
			"verified_action_ids": datatypes.JSON([]byte("[]")),
			"denied_action_ids":   datatypes.JSON([]byte("[]")),
			"revision":            gorm.Expr("revision + 1"),
			"health_revision":     gorm.Expr("health_revision + 1"),
			"updated_at":          gorm.Expr("CURRENT_TIMESTAMP"),
		}).Error
}

func (service *ProviderInstallFlowService) resolveMethod(integrationID, authMethodID string) (ProviderDefinition, AuthMethodDefinition, ProviderInstallProvider, error) {
	integrationID = strings.ToLower(strings.TrimSpace(integrationID))
	authMethodID = strings.ToLower(strings.TrimSpace(authMethodID))
	definition, ok := service.registry.ProviderDefinition(integrationID)
	if !ok {
		return ProviderDefinition{}, AuthMethodDefinition{}, nil, NewError(ErrorCodeDisabled, "provider install integration is unavailable", nil)
	}
	var method AuthMethodDefinition
	for _, candidate := range definition.AuthMethods {
		if candidate.ID == authMethodID && candidate.Type == AuthMethodTypeProviderInstall && candidate.Available {
			method = candidate
			break
		}
	}
	if method.ID == "" {
		return ProviderDefinition{}, AuthMethodDefinition{}, nil, NewError(ErrorCodeDisabled, "provider install method is unavailable", nil)
	}
	provider, ok := service.registry.ProviderInstallProvider(definition.ID, definition.DriverID)
	if !ok {
		return ProviderDefinition{}, AuthMethodDefinition{}, nil, NewError(ErrorCodeDisabled, "provider installer is unavailable", nil)
	}
	return definition, method, provider, nil
}

func (service *ProviderInstallFlowService) getForActor(ctx context.Context, flowRef string, organizationID, accountID uuid.UUID) (*IntegrationProviderInstallFlow, error) {
	flowRef = strings.TrimSpace(flowRef)
	if flowRef == "" || len(flowRef) > 256 || organizationID == uuid.Nil || accountID == uuid.Nil {
		return nil, ErrConnectionNotFound
	}
	var flow IntegrationProviderInstallFlow
	err := service.db.WithContext(ctx).
		Where("flow_digest = ? AND organization_id = ? AND account_id = ?", providerInstallDigest(flowRef), organizationID, accountID).
		First(&flow).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrConnectionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get provider install flow: %w", err)
	}
	return &flow, nil
}

func (service *ProviderInstallFlowService) failFlow(ctx context.Context, flowID uuid.UUID, code string) error {
	now := service.now().UTC()
	return service.db.WithContext(ctx).Model(&IntegrationProviderInstallFlow{}).
		Where("id = ? AND status = ?", flowID, ProviderInstallFlowPending).
		Updates(map[string]any{"status": ProviderInstallFlowFailed, "failure_code": strings.TrimSpace(code), "completed_at": now, "updated_at": now}).Error
}

func providerInstallFlowView(flowRef string, flow *IntegrationProviderInstallFlow) ProviderInstallFlowView {
	view := ProviderInstallFlowView{FlowID: flowRef, Intent: OAuthFlowIntentConnect, NextPollAfterMS: 1500}
	if flow == nil {
		return view
	}
	view.IntegrationID, view.AuthMethodID, view.CredentialSource = flow.IntegrationID, flow.AuthMethodID, flow.CredentialSource
	view.Status, view.ConnectionName, view.ExpiresAt = flow.Status, flow.ConnectionName, flow.ExpiresAt
	view.CompletedAt, view.CompletedConnectionID = cloneTimePointer(flow.CompletedAt), cloneUUIDPointer(flow.CompletedConnectionID)
	if flow.AccountDisplayName != nil {
		view.AccountDisplayName = *flow.AccountDisplayName
	}
	if flow.FailureCode != nil {
		view.ErrorCode = *flow.FailureCode
	}
	view.UsageRulesRequired = flow.Status == ProviderInstallFlowSucceeded
	return view
}

var sha256HexPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func randomProviderInstallReference() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", NewError(ErrorCodeUpstream, "provider install reference could not be generated", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func randomProviderInstallState() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", NewError(ErrorCodeUpstream, "provider install state could not be generated", err)
	}
	return hex.EncodeToString(raw), nil
}

func providerInstallDigest(value string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(digest[:])
}

func safeProviderInstallReturnPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 2048 || !strings.HasPrefix(value, "/console/") || strings.HasPrefix(value, "//") || strings.ContainsAny(value, "\r\n") {
		return "/console/integrations"
	}
	return value
}

func boundedProviderInstallText(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}
