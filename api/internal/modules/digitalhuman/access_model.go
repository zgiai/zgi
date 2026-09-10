package digitalhuman

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
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

// ExternalFileBinding records the immutable public-entry ownership boundary
// for a temporary chat upload. The generic file service still owns the file;
// this record prevents the same account from reusing it with another
// DigitalHuman or through another external surface.
type ExternalFileBinding struct {
	FileID          uuid.UUID `gorm:"type:uuid;primaryKey;column:file_id" json:"-"`
	DigitalHumanID  uuid.UUID `gorm:"type:uuid;not null;index;column:digital_human_id" json:"-"`
	OrganizationID  uuid.UUID `gorm:"type:uuid;not null;index;column:organization_id" json:"-"`
	WorkspaceID     uuid.UUID `gorm:"type:uuid;not null;index;column:workspace_id" json:"-"`
	UserSubjectKind string    `gorm:"type:varchar(32);not null;column:user_subject_kind" json:"-"`
	UserSubjectID   uuid.UUID `gorm:"type:uuid;not null;index;column:user_subject_id" json:"-"`
	Surface         string    `gorm:"type:varchar(32);not null;column:surface" json:"-"`
	CreatedAt       time.Time `gorm:"not null" json:"-"`
}

func (ExternalFileBinding) TableName() string { return "digital_human_external_files" }

// AccessAuditEvent is an append-only record of a successful external-access
// management mutation. Details are deliberately limited to non-secret state
// transitions; API key material, key names, external user IDs, and content are
// never stored here.
type AccessAuditEvent struct {
	ID             uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	DigitalHumanID uuid.UUID      `gorm:"type:uuid;not null;index;column:digital_human_id" json:"digital_human_id"`
	OrganizationID uuid.UUID      `gorm:"type:uuid;not null;index;column:organization_id" json:"organization_id"`
	WorkspaceID    uuid.UUID      `gorm:"type:uuid;not null;index;column:workspace_id" json:"workspace_id"`
	ActorID        *uuid.UUID     `gorm:"type:uuid;column:actor_id" json:"actor_id,omitempty"`
	Action         string         `gorm:"type:varchar(64);not null;column:action" json:"action"`
	TargetType     string         `gorm:"type:varchar(32);not null;column:target_type" json:"target_type"`
	TargetID       *uuid.UUID     `gorm:"type:uuid;column:target_id" json:"target_id,omitempty"`
	Source         string         `gorm:"type:varchar(32);not null;column:source" json:"source"`
	Result         string         `gorm:"type:varchar(16);not null;column:result" json:"result"`
	Details        datatypes.JSON `gorm:"type:jsonb;not null;default:'{}';column:details" json:"details"`
	CreatedAt      time.Time      `gorm:"not null" json:"created_at"`
}

func (AccessAuditEvent) TableName() string { return "digital_human_access_events" }
