package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	PrincipalTypeUser           = "user"
	PrincipalTypeServiceAccount = "service_account"

	AccessModeSelfService      = "self_service"
	AccessModeApprovalRequired = "approval_required"
	AccessModeDisabled         = "disabled"

	RequestStatusPending   = "pending"
	RequestStatusApproved  = "approved"
	RequestStatusRejected  = "rejected"
	RequestStatusCancelled = "cancelled"

	GrantStatusActive   = "active"
	GrantStatusDisabled = "disabled"
	GrantStatusRevoked  = "revoked"
)

type Policy struct {
	ID                 string         `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID     string         `gorm:"type:uuid;not null;index" json:"organization_id"`
	WorkspaceID        string         `gorm:"type:uuid;not null;uniqueIndex" json:"workspace_id"`
	Mode               string         `gorm:"type:varchar(32);not null" json:"mode"`
	DefaultQuota       *int64         `json:"default_quota,omitempty"`
	MaxQuota           *int64         `json:"max_quota,omitempty"`
	MaxKeys            int            `gorm:"not null;default:3" json:"max_keys"`
	DefaultTTLSeconds  *int64         `json:"default_ttl_seconds,omitempty"`
	MaxTTLSeconds      *int64         `json:"max_ttl_seconds,omitempty"`
	AllowedModels      []string       `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"allowed_models"`
	Version            int64          `gorm:"not null;default:1" json:"version"`
	UpdatedByAccountID *string        `gorm:"type:uuid" json:"updated_by_account_id,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Policy) TableName() string { return "llm_developer_access_policies" }
func (p *Policy) BeforeCreate(*gorm.DB) error {
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	return nil
}

type AccessRequest struct {
	ID                  string         `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID      string         `gorm:"type:uuid;not null;index" json:"organization_id"`
	WorkspaceID         string         `gorm:"type:uuid;not null;index" json:"workspace_id"`
	RequesterAccountID  string         `gorm:"type:uuid;not null;index" json:"requester_account_id"`
	RequesterName       string         `gorm:"-" json:"requester_name,omitempty"`
	RequesterEmail      string         `gorm:"-" json:"requester_email,omitempty"`
	Purpose             string         `gorm:"type:text;not null" json:"purpose"`
	Environment         string         `gorm:"type:varchar(20);not null;default:'development'" json:"environment"`
	RequestedQuota      *int64         `json:"requested_quota,omitempty"`
	RequestedModels     []string       `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"requested_models"`
	RequestedTTLSeconds *int64         `json:"requested_ttl_seconds,omitempty"`
	Status              string         `gorm:"type:varchar(20);not null;index" json:"status"`
	ReviewerAccountID   *string        `gorm:"type:uuid" json:"reviewer_account_id,omitempty"`
	ReviewReason        *string        `gorm:"type:text" json:"review_reason,omitempty"`
	ReviewedAt          *time.Time     `json:"reviewed_at,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"-"`
}

func (AccessRequest) TableName() string { return "llm_developer_access_requests" }
func (r *AccessRequest) BeforeCreate(*gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.NewString()
	}
	return nil
}

type Grant struct {
	ID                   string         `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID       string         `gorm:"type:uuid;not null;index" json:"organization_id"`
	WorkspaceID          string         `gorm:"type:uuid;not null;index" json:"workspace_id"`
	PrincipalType        string         `gorm:"type:varchar(32);not null" json:"principal_type"`
	PrincipalID          string         `gorm:"type:varchar(255);not null;index" json:"principal_id"`
	Source               string         `gorm:"type:varchar(32);not null" json:"source"`
	Status               string         `gorm:"type:varchar(20);not null;index" json:"status"`
	QuotaLimit           *int64         `json:"quota_limit,omitempty"`
	UsedQuota            int64          `gorm:"not null;default:0" json:"used_quota"`
	RemainQuota          int64          `gorm:"not null;default:0" json:"remain_quota"`
	MaxKeys              int            `gorm:"not null;default:3" json:"max_keys"`
	AllowedModels        []string       `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"allowed_models"`
	ExpiresAt            *time.Time     `json:"expires_at,omitempty"`
	AuthorizationVersion int64          `gorm:"not null;default:1" json:"authorization_version"`
	CreatedByAccountID   *string        `gorm:"type:uuid" json:"created_by_account_id,omitempty"`
	UpdatedByAccountID   *string        `gorm:"type:uuid" json:"updated_by_account_id,omitempty"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
	DeletedAt            gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Grant) TableName() string { return "llm_developer_access_grants" }
func (g *Grant) BeforeCreate(*gorm.DB) error {
	if g.ID == "" {
		g.ID = uuid.NewString()
	}
	return nil
}

func (g *Grant) IsActive(now time.Time) bool {
	return g.Status == GrantStatusActive && (g.ExpiresAt == nil || g.ExpiresAt.After(now))
}
