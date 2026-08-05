package integrations

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ConnectionCredentialSource string

const (
	// ConnectionCredentialSourcePlatform is retained only so legacy database
	// rows can be identified and rejected safely. New catalog definitions,
	// connection creation, listing, and execution support organization- and
	// account-owned credentials only.
	ConnectionCredentialSourcePlatform     ConnectionCredentialSource = "platform"
	ConnectionCredentialSourceOrganization ConnectionCredentialSource = "organization"
	ConnectionCredentialSourceAccount      ConnectionCredentialSource = "account"
)

type ConnectionAuthType string

const (
	// ConnectionAuthTypePlatform is a legacy persisted value. It is not a
	// supported authentication type for new or executable connections.
	ConnectionAuthTypePlatform         ConnectionAuthType = "platform"
	ConnectionAuthTypeAPIKey           ConnectionAuthType = "api_key"
	ConnectionAuthTypeOAuth2           ConnectionAuthType = "oauth2"
	ConnectionAuthTypeCustomCredential ConnectionAuthType = "custom_credential"
	ConnectionAuthTypeServiceAccount   ConnectionAuthType = "service_account"
)

type ConnectionHealthStatus string

const (
	ConnectionHealthUnknown   ConnectionHealthStatus = "unknown"
	ConnectionHealthHealthy   ConnectionHealthStatus = "healthy"
	ConnectionHealthDegraded  ConnectionHealthStatus = "degraded"
	ConnectionHealthUnhealthy ConnectionHealthStatus = "unhealthy"
)

type ConnectionAuthStatus string

const (
	ConnectionAuthUnknown           ConnectionAuthStatus = "unknown"
	ConnectionAuthValid             ConnectionAuthStatus = "valid"
	ConnectionAuthReconnectRequired ConnectionAuthStatus = "reconnect_required"
	ConnectionAuthExpired           ConnectionAuthStatus = "expired"
)

type ConnectionScopeStatus string

const (
	ConnectionScopeUnknown  ConnectionScopeStatus = "unknown"
	ConnectionScopeVerified ConnectionScopeStatus = "verified"
	ConnectionScopeDrifted  ConnectionScopeStatus = "drifted"
)

const (
	ConnectionAttentionReconnectRequired   = "reconnect_required"
	ConnectionAttentionScopeUpdateRequired = "scope_update_required"
	ConnectionAttentionBillingRequired     = "billing_required"
	ConnectionAttentionProviderIncident    = "provider_incident"
	ConnectionAttentionAdminCheckRequired  = "admin_check_required"
)

type ConnectionStatus string

const (
	ConnectionStatusPending  ConnectionStatus = "pending"
	ConnectionStatusActive   ConnectionStatus = "active"
	ConnectionStatusInvalid  ConnectionStatus = "invalid"
	ConnectionStatusDisabled ConnectionStatus = "disabled"
)

// IntegrationConnection is the persisted, organization-scoped connection
// metadata. EncryptedCredentials is intentionally excluded from JSON views;
// callers should expose ConnectionView instead.
type IntegrationConnection struct {
	ID                      uuid.UUID                  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID          uuid.UUID                  `gorm:"type:uuid;not null;index:idx_integration_connections_org_integration,priority:1" json:"organization_id"`
	IntegrationID           string                     `gorm:"size:64;not null;index:idx_integration_connections_org_integration,priority:2" json:"integration_id"`
	DriverID                string                     `gorm:"size:64;not null" json:"driver_id"`
	Name                    string                     `gorm:"size:128;not null" json:"name"`
	CredentialSource        ConnectionCredentialSource `gorm:"size:32;not null" json:"credential_source"`
	AuthType                ConnectionAuthType         `gorm:"size:32;not null" json:"auth_type"`
	AuthMethodID            string                     `gorm:"size:128;not null" json:"auth_method_id"`
	OwnerAccountID          *uuid.UUID                 `gorm:"type:uuid" json:"owner_account_id,omitempty"`
	EncryptedCredentials    *string                    `gorm:"type:text" json:"-"`
	Config                  map[string]any             `gorm:"type:jsonb;serializer:json;not null;default:'{}'" json:"config"`
	AccountID               *string                    `gorm:"size:255" json:"account_id,omitempty"`
	DisplayName             *string                    `gorm:"size:255" json:"display_name,omitempty"`
	GrantedScopes           []string                   `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"granted_scopes"`
	Status                  ConnectionStatus           `gorm:"size:32;not null" json:"status"`
	IsDefault               bool                       `gorm:"not null;default:false" json:"is_default"`
	CredentialVersion       int                        `gorm:"not null;default:1" json:"credential_version"`
	Revision                int                        `gorm:"not null;default:1" json:"revision"`
	LastTestedAt            *time.Time                 `json:"last_tested_at,omitempty"`
	LastErrorCode           *string                    `gorm:"size:64" json:"last_error_code,omitempty"`
	ExpiresAt               *time.Time                 `json:"expires_at,omitempty"`
	HealthStatus            ConnectionHealthStatus     `gorm:"size:32;not null;default:unknown" json:"health_status"`
	AuthStatus              ConnectionAuthStatus       `gorm:"size:32;not null;default:unknown" json:"auth_status"`
	ScopeStatus             ConnectionScopeStatus      `gorm:"size:32;not null;default:unknown" json:"scope_status"`
	AttentionCode           *string                    `gorm:"size:64" json:"attention_code,omitempty"`
	MissingRequiredScopes   []string                   `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"missing_required_scopes"`
	LastHealthCheckedAt     *time.Time                 `json:"last_health_checked_at,omitempty"`
	LastHealthyAt           *time.Time                 `json:"last_healthy_at,omitempty"`
	LastRuntimeSuccessAt    *time.Time                 `json:"last_runtime_success_at,omitempty"`
	LastRuntimeFailureAt    *time.Time                 `json:"last_runtime_failure_at,omitempty"`
	ScopeCheckedAt          *time.Time                 `json:"scope_checked_at,omitempty"`
	ConsecutiveFailures     int                        `gorm:"not null;default:0" json:"consecutive_failures"`
	HealthRevision          int                        `gorm:"not null;default:1" json:"health_revision"`
	TokenExpiresAt          *time.Time                 `json:"token_expires_at,omitempty"`
	RefreshTokenExpiresAt   *time.Time                 `json:"refresh_token_expires_at,omitempty"`
	NextTokenRefreshAt      *time.Time                 `json:"next_token_refresh_at,omitempty"`
	SetupVersion            int                        `gorm:"not null;default:1" json:"setup_version"`
	SetupCompletedAt        *time.Time                 `json:"setup_completed_at,omitempty"`
	SetupCompletedBy        *uuid.UUID                 `gorm:"type:uuid" json:"-"`
	CreatedBy               *uuid.UUID                 `gorm:"type:uuid" json:"created_by,omitempty"`
	UpdatedBy               *uuid.UUID                 `gorm:"type:uuid" json:"updated_by,omitempty"`
	CreatedAt               time.Time                  `json:"created_at"`
	UpdatedAt               time.Time                  `json:"updated_at"`
	DeletedAt               gorm.DeletedAt             `gorm:"index" json:"-"`
	LoadedCredentialVersion int                        `gorm:"-" json:"-"`
	LoadedRevision          int                        `gorm:"-" json:"-"`
	LoadedHealthRevision    int                        `gorm:"-" json:"-"`
}

func (IntegrationConnection) TableName() string { return "integration_connections" }

func (connection *IntegrationConnection) BeforeCreate(_ *gorm.DB) error {
	if connection.ID == uuid.Nil {
		connection.ID = uuid.New()
	}
	if connection.CredentialVersion < 1 {
		connection.CredentialVersion = 1
	}
	if connection.Revision < 1 {
		connection.Revision = 1
	}
	if connection.HealthRevision < 1 {
		connection.HealthRevision = 1
	}
	if connection.SetupVersion < 1 {
		connection.SetupVersion = 1
	}
	if connection.HealthStatus == "" {
		connection.HealthStatus = ConnectionHealthUnknown
	}
	if connection.AuthStatus == "" {
		connection.AuthStatus = ConnectionAuthUnknown
	}
	if connection.ScopeStatus == "" {
		connection.ScopeStatus = ConnectionScopeUnknown
	}
	if connection.AuthMethodID == "" {
		connection.AuthMethodID = string(connection.AuthType)
	}
	connection.AuthMethodID = strings.ToLower(strings.TrimSpace(connection.AuthMethodID))
	if !integrationIdentifierPattern.MatchString(connection.AuthMethodID) {
		return invalidInput("integration authentication method is invalid", nil)
	}
	connection.LoadedCredentialVersion = connection.CredentialVersion
	connection.LoadedRevision = connection.Revision
	connection.LoadedHealthRevision = connection.HealthRevision
	if connection.Config == nil {
		connection.Config = map[string]any{}
	}
	if connection.GrantedScopes == nil {
		connection.GrantedScopes = []string{}
	}
	if connection.MissingRequiredScopes == nil {
		connection.MissingRequiredScopes = []string{}
	}
	return nil
}

func (connection *IntegrationConnection) AfterFind(_ *gorm.DB) error {
	connection.LoadedCredentialVersion = connection.CredentialVersion
	connection.LoadedRevision = connection.Revision
	connection.LoadedHealthRevision = connection.HealthRevision
	return nil
}

// ConnectionView is safe for management APIs. It reports only whether a
// credential exists and never exposes the encrypted envelope.
type ConnectionView struct {
	ID                    uuid.UUID                    `json:"id"`
	OrganizationID        uuid.UUID                    `json:"organization_id"`
	IntegrationID         string                       `json:"integration_id"`
	DriverID              string                       `json:"driver_id"`
	Name                  string                       `json:"name"`
	CredentialSource      ConnectionCredentialSource   `json:"credential_source"`
	AuthType              ConnectionAuthType           `json:"auth_type"`
	AuthMethodID          string                       `json:"auth_method_id"`
	OwnerAccountID        *uuid.UUID                   `json:"owner_account_id,omitempty"`
	CredentialConfigured  bool                         `json:"credential_configured"`
	Config                map[string]any               `json:"config"`
	AccountID             *string                      `json:"account_id,omitempty"`
	DisplayName           *string                      `json:"display_name,omitempty"`
	GrantedScopes         []string                     `json:"granted_scopes"`
	PermissionSummary     *ConnectionPermissionSummary `json:"permission_summary,omitempty"`
	Status                ConnectionStatus             `json:"status"`
	IsDefault             bool                         `json:"is_default"`
	CredentialVersion     int                          `json:"credential_version"`
	Revision              int                          `json:"revision"`
	LastTestedAt          *time.Time                   `json:"last_tested_at,omitempty"`
	LastErrorCode         *string                      `json:"last_error_code,omitempty"`
	ExpiresAt             *time.Time                   `json:"expires_at,omitempty"`
	HealthStatus          ConnectionHealthStatus       `json:"health_status"`
	AuthStatus            ConnectionAuthStatus         `json:"auth_status"`
	ScopeStatus           ConnectionScopeStatus        `json:"scope_status"`
	AttentionCode         *string                      `json:"attention_code,omitempty"`
	MissingRequiredScopes []string                     `json:"missing_required_scopes"`
	LastHealthCheckedAt   *time.Time                   `json:"last_health_checked_at,omitempty"`
	LastHealthyAt         *time.Time                   `json:"last_healthy_at,omitempty"`
	LastRuntimeSuccessAt  *time.Time                   `json:"last_runtime_success_at,omitempty"`
	LastRuntimeFailureAt  *time.Time                   `json:"last_runtime_failure_at,omitempty"`
	ScopeCheckedAt        *time.Time                   `json:"scope_checked_at,omitempty"`
	ConsecutiveFailures   int                          `json:"consecutive_failures"`
	HealthRevision        int                          `json:"health_revision"`
	TokenExpiresAt        *time.Time                   `json:"token_expires_at,omitempty"`
	RefreshTokenExpiresAt *time.Time                   `json:"refresh_token_expires_at,omitempty"`
	NextTokenRefreshAt    *time.Time                   `json:"next_token_refresh_at,omitempty"`
	SetupVersion          int                          `json:"setup_version"`
	SetupCompletedAt      *time.Time                   `json:"setup_completed_at,omitempty"`
	CreatedAt             time.Time                    `json:"created_at"`
	UpdatedAt             time.Time                    `json:"updated_at"`
}

func newConnectionView(connection *IntegrationConnection) ConnectionView {
	if connection == nil {
		return ConnectionView{}
	}
	return ConnectionView{
		ID:                    connection.ID,
		OrganizationID:        connection.OrganizationID,
		IntegrationID:         connection.IntegrationID,
		DriverID:              connection.DriverID,
		Name:                  connection.Name,
		CredentialSource:      connection.CredentialSource,
		AuthType:              connection.AuthType,
		AuthMethodID:          connection.AuthMethodID,
		OwnerAccountID:        cloneUUIDPointer(connection.OwnerAccountID),
		CredentialConfigured:  connection.EncryptedCredentials != nil && *connection.EncryptedCredentials != "",
		Config:                cloneAnyMap(connection.Config),
		AccountID:             cloneStringPointer(connection.AccountID),
		DisplayName:           cloneStringPointer(connection.DisplayName),
		GrantedScopes:         cloneStringSlice(connection.GrantedScopes),
		Status:                connection.Status,
		IsDefault:             connection.IsDefault,
		CredentialVersion:     connection.CredentialVersion,
		Revision:              connection.Revision,
		LastTestedAt:          cloneTimePointer(connection.LastTestedAt),
		LastErrorCode:         cloneStringPointer(connection.LastErrorCode),
		ExpiresAt:             cloneTimePointer(connection.ExpiresAt),
		HealthStatus:          connection.HealthStatus,
		AuthStatus:            connection.AuthStatus,
		ScopeStatus:           connection.ScopeStatus,
		AttentionCode:         cloneStringPointer(connection.AttentionCode),
		MissingRequiredScopes: cloneStringSlice(connection.MissingRequiredScopes),
		LastHealthCheckedAt:   cloneTimePointer(connection.LastHealthCheckedAt),
		LastHealthyAt:         cloneTimePointer(connection.LastHealthyAt),
		LastRuntimeSuccessAt:  cloneTimePointer(connection.LastRuntimeSuccessAt),
		LastRuntimeFailureAt:  cloneTimePointer(connection.LastRuntimeFailureAt),
		ScopeCheckedAt:        cloneTimePointer(connection.ScopeCheckedAt),
		ConsecutiveFailures:   connection.ConsecutiveFailures,
		HealthRevision:        connection.HealthRevision,
		TokenExpiresAt:        cloneTimePointer(connection.TokenExpiresAt),
		RefreshTokenExpiresAt: cloneTimePointer(connection.RefreshTokenExpiresAt),
		NextTokenRefreshAt:    cloneTimePointer(connection.NextTokenRefreshAt),
		SetupVersion:          connection.SetupVersion,
		SetupCompletedAt:      cloneTimePointer(connection.SetupCompletedAt),
		CreatedAt:             connection.CreatedAt,
		UpdatedAt:             connection.UpdatedAt,
	}
}

func supportedConnectionCredentialSource(source ConnectionCredentialSource) bool {
	switch source {
	case ConnectionCredentialSourceOrganization, ConnectionCredentialSourceAccount:
		return true
	default:
		return false
	}
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneStringSlice(value []string) []string {
	if len(value) == 0 {
		return []string{}
	}
	return append([]string{}, value...)
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneAnyMap(value map[string]any) map[string]any {
	if len(value) == 0 {
		return map[string]any{}
	}
	copyValue := make(map[string]any, len(value))
	for key, item := range value {
		copyValue[key] = item
	}
	return copyValue
}
