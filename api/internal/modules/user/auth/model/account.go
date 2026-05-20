package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// AccountStatus account status enum
type AccountStatus string

const (
	AccountStatusActive        AccountStatus = "active"
	AccountStatusBanned        AccountStatus = "banned"
	AccountStatusPending       AccountStatus = "pending"
	AccountStatusUninitialized AccountStatus = "uninitialized"
	AccountStatusClosed        AccountStatus = "closed"
	AccountStatusFrozen        AccountStatus = "frozen"
)

// Account account model
type Account struct {
	ID                string         `gorm:"type:uuid;primaryKey" json:"id"`
	Name              string         `gorm:"type:varchar(255);not null" json:"name"`
	Email             string         `gorm:"type:varchar(255);index" json:"email"`
	MobileE164        *string        `gorm:"column:mobile_e164;type:varchar(32);index" json:"-"`
	Password          *string        `gorm:"type:varchar(255)" json:"-"`
	PasswordSalt      *string        `gorm:"type:varchar(255)" json:"-"`
	Avatar            *string        `gorm:"type:varchar(255)" json:"avatar"`
	InterfaceLanguage *string        `gorm:"type:varchar(255)" json:"interface_language"`
	InterfaceTheme    *string        `gorm:"type:varchar(255)" json:"interface_theme"`
	Timezone          *string        `gorm:"type:varchar(255)" json:"timezone"`
	LastLoginAt       *time.Time     `json:"last_login_at"`
	LastActiveAt      *time.Time     `gorm:"not null;default:CURRENT_TIMESTAMP" json:"last_active_at"`
	Status            AccountStatus  `gorm:"type:varchar(16);not null;default:'active'" json:"status"`
	IsSuperAdmin      bool           `gorm:"column:is_super_admin;not null;default:false" json:"-"`
	InitializedAt     *time.Time     `gorm:"not null;default:false" json:"initialized_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	LastLoginIp       *string        `gorm:"type:varchar(255)" json:"last_login_ip"`
	Extensions        JSONMap        `gorm:"type:jsonb" json:"extensions"`
	CurrentTenantID   string         `gorm:"-" json:"current_tenant_id"`
	GroupRole         string         `gorm:"-" json:"group_role"`

	AccountIntegrates []AccountIntegrate `gorm:"foreignKey:AccountID" json:"-"`
}

// TableName specifies table name
func (Account) TableName() string {
	return "accounts"
}

// IsActive checks if account is active
func (a *Account) IsActive() bool {
	return a.Status == AccountStatusActive
}

// IsBanned checks if account is banned
func (a *Account) IsBanned() bool {
	return a.Status == AccountStatusBanned
}

// IsPending checks if account is pending
func (a *Account) IsPending() bool {
	return a.Status == AccountStatusPending
}

// GenderEnum gender enum
type GenderEnum string

const (
	GenderMale   GenderEnum = "male"
	GenderFemale GenderEnum = "female"
	GenderOther  GenderEnum = "other"
)

type JSONMap map[string]interface{}

func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

func (j *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into JSONMap", value)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(bytes, &result); err == nil {
		*j = JSONMap(result)
		return nil
	}

	var arrayResult []interface{}
	if err := json.Unmarshal(bytes, &arrayResult); err == nil {
		*j = JSONMap{"keywords": arrayResult}
		return nil
	}

	return json.Unmarshal(bytes, j)
}

// AccountIntegrateProvider third-party integration provider enum
type AccountIntegrateProvider string

const (
	ProviderGoogle AccountIntegrateProvider = "google"
	ProviderGithub AccountIntegrateProvider = "github"
	ProviderOIDC   AccountIntegrateProvider = "oidc"
	ProviderOAuth  AccountIntegrateProvider = "oauth"
)

// AccountIntegrate account third-party integration
type AccountIntegrate struct {
	ID             string                   `gorm:"type:varchar(255);primaryKey" json:"id"`
	AccountID      string                   `gorm:"type:varchar(255);not null;index" json:"account_id"`
	Provider       AccountIntegrateProvider `gorm:"type:varchar(16);not null" json:"provider"`
	OpenID         string                   `gorm:"type:varchar(255);not null" json:"open_id"`
	EncryptedToken string                   `gorm:"type:text;not null" json:"-"`
	CreatedAt      time.Time                `json:"created_at"`
	UpdatedAt      time.Time                `json:"updated_at"`

	// Relationships
	Account Account `gorm:"foreignKey:AccountID" json:"-"`
}

// TableName specifies table name
func (AccountIntegrate) TableName() string {
	return "account_integrates"
}

// InvitationCodeStatus invitation code status enum
type InvitationCodeStatus string

const (
	InvitationStatusPending InvitationCodeStatus = "pending"
	InvitationStatusUsed    InvitationCodeStatus = "used"
	InvitationStatusExpired InvitationCodeStatus = "expired"
)

// InvitationCode invitation code
type InvitationCode struct {
	ID              string               `gorm:"type:varchar(255);primaryKey" json:"id"`
	Batch           string               `gorm:"type:varchar(255);not null" json:"batch"`
	Code            string               `gorm:"type:varchar(32);uniqueIndex;not null" json:"code"`
	Email           string               `gorm:"type:varchar(255);not null" json:"email"`
	Status          InvitationCodeStatus `gorm:"type:varchar(16);not null;default:'pending'" json:"status"`
	UsedAt          *time.Time           `json:"used_at"`
	UsedByTenantID  *string              `gorm:"type:varchar(255)" json:"used_by_tenant_id"`
	UsedByAccountID *string              `gorm:"type:varchar(255)" json:"used_by_account_id"`
	DeprecatedAt    *time.Time           `json:"deprecated_at"`
	CreatedAt       time.Time            `json:"created_at"`

	// Relationships (commented out for modular architecture)
	// UsedByTenant  *Tenant  `gorm:"foreignKey:UsedByTenantID" json:"-"`
	// UsedByAccount *Account `gorm:"foreignKey:UsedByAccountID" json:"-"`
}

// TableName specifies table name
func (InvitationCode) TableName() string {
	return "invitation_codes"
}

// IsUsed checks if invitation code is used
func (ic *InvitationCode) IsUsed() bool {
	return ic.Status == InvitationStatusUsed
}

// IsExpired checks if invitation code is expired
func (ic *InvitationCode) IsExpired() bool {
	return ic.Status == InvitationStatusExpired
}

// IsPending checks if invitation code is pending
func (ic *InvitationCode) IsPending() bool {
	return ic.Status == InvitationStatusPending
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type AccountAndJoin struct {
	Account Account `json:"account"`
	Role    string  `json:"role"`
}

// AccountWithExtensions account with extension information
type AccountWithExtensions struct {
	Account          Account            `json:"account"`
	AccountExtension JSONMap            `json:"account_extension,omitempty"`
	AccountIntegrate []AccountIntegrate `json:"account_integrates,omitempty"`
}

type AccountContext struct {
	AccountID             string    `gorm:"column:account_id;type:uuid;primaryKey" json:"account_id"`
	CurrentOrganizationID *string   `gorm:"column:current_organization_id;type:uuid" json:"current_organization_id,omitempty"`
	CurrentWorkspaceID    *string   `gorm:"column:current_workspace_id;type:uuid" json:"current_workspace_id,omitempty"`
	CreatedAt             time.Time `gorm:"column:created_at;not null" json:"created_at"`
	UpdatedAt             time.Time `gorm:"column:updated_at;not null" json:"updated_at"`
}

func (AccountContext) TableName() string {
	return "account_contexts"
}

// GetCurrentWorkspace is deprecated - use TenantService.GetCurrentWorkspace() instead
// func (a *Account) GetCurrentWorkspace() *Tenant {
//	// This method is deprecated - use TenantService.GetCurrentWorkspace() instead
//	// The actual implementation requires database queries in Service layer
//	return nil
// }
