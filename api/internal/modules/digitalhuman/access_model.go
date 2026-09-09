package digitalhuman

import (
	"time"

	"github.com/google/uuid"
)

const (
	APIKeyStatusActive  = "active"
	APIKeyStatusRevoked = "revoked"
)

// APIKey is a server-to-server credential bound to exactly one DigitalHuman.
type APIKey struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	DigitalHumanID uuid.UUID  `gorm:"type:uuid;not null;index;column:digital_human_id" json:"digital_human_id"`
	OrganizationID uuid.UUID  `gorm:"type:uuid;not null;index;column:organization_id" json:"organization_id"`
	WorkspaceID    uuid.UUID  `gorm:"type:uuid;not null;index;column:workspace_id" json:"workspace_id"`
	KeyHash        string     `gorm:"type:varchar(64);not null;uniqueIndex;column:key_hash" json:"-"`
	KeyPrefix      string     `gorm:"type:varchar(20);not null;column:key_prefix" json:"key_prefix"`
	Name           string     `gorm:"type:varchar(255);not null" json:"name"`
	Status         string     `gorm:"type:varchar(20);not null;default:'active'" json:"status"`
	ExpiresAt      *time.Time `gorm:"column:expires_at" json:"expires_at,omitempty"`
	UsageCount     int64      `gorm:"not null;default:0;column:usage_count" json:"usage_count"`
	LastUsedAt     *time.Time `gorm:"column:last_used_at" json:"last_used_at,omitempty"`
	CreatedAt      time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"not null" json:"updated_at"`
}

func (APIKey) TableName() string { return "digital_human_api_keys" }

// AccessSettings is the independently mutable external access policy.
type AccessSettings struct {
	WebAppEnabled bool `json:"webapp_enabled"`
	APIEnabled    bool `json:"api_enabled"`
}
