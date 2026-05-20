package agents

import (
	"time"

	"github.com/google/uuid"
)

type AgentSource string

const (
	AgentSourceUser AgentSource = "user"
)

type AgentWebAppStatus string

const (
	AgentWebAppStatusActive   AgentWebAppStatus = "active"
	AgentWebAppStatusInactive AgentWebAppStatus = "inactive"
)

func NormalizeAgentWebAppStatus(status AgentWebAppStatus) AgentWebAppStatus {
	if status == "" {
		return AgentWebAppStatusActive
	}
	return status
}

func IsValidAgentWebAppStatus(status AgentWebAppStatus) bool {
	switch status {
	case AgentWebAppStatusActive, AgentWebAppStatusInactive:
		return true
	default:
		return false
	}
}

// Agent represents the agents table schema.
type Agent struct {
	ID                  uuid.UUID         `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	TenantID            uuid.UUID         `gorm:"type:uuid;not null;index:agents_tenant_id_idx;column:tenant_id" json:"tenant_id"`
	Name                string            `gorm:"type:varchar(255);not null" json:"name"`
	Description         string            `gorm:"type:text;not null;default:''" json:"description"`
	AgentsType          string            `gorm:"type:varchar(255);not null;column:agent_type" json:"agent_type"`
	IconType            *string           `gorm:"type:varchar(255);column:icon_type" json:"icon_type"`
	Icon                *string           `gorm:"type:varchar(255);column:icon" json:"icon"`
	AgentsModelConfigID *uuid.UUID        `gorm:"type:uuid;column:agents_model_config_id" json:"agents_model_config_id"`
	WorkflowID          *uuid.UUID        `gorm:"type:uuid;column:workflow_id" json:"workflow_id"`
	WorkflowConfig      *string           `gorm:"type:jsonb;column:workflow_config" json:"workflow_config"` // JSONB field for workflow configuration
	EnableAPI           bool              `gorm:"type:boolean;not null;column:enable_api" json:"enable_api"`
	IsPublic            bool              `gorm:"type:boolean;not null;default:false;column:is_public" json:"is_public"`
	IsUniversal         bool              `gorm:"type:boolean;not null;default:false;column:is_universal" json:"is_universal"`
	Internal            bool              `gorm:"type:boolean;not null;default:false;column:internal" json:"internal"`
	WebAppID            uuid.UUID         `gorm:"type:uuid;not null;unique;column:web_app_id" json:"web_app_id"` // Unique identifier for web application
	WebAppStatus        AgentWebAppStatus `gorm:"type:varchar(20);not null;default:'active';column:web_app_status" json:"web_app_status"`
	WebAppOfflinedAt    *time.Time        `gorm:"column:web_app_offlined_at" json:"web_app_offlined_at"`
	WebAppOfflinedBy    *uuid.UUID        `gorm:"type:uuid;column:web_app_offlined_by" json:"web_app_offlined_by"`
	WebAppOfflineReason string            `gorm:"type:text;not null;default:'';column:web_app_offline_reason" json:"web_app_offline_reason"`
	CreatedBy           *uuid.UUID        `gorm:"type:uuid;column:created_by" json:"created_by"`
	CreatedAt           time.Time         `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedBy           *uuid.UUID        `gorm:"type:uuid;column:updated_by" json:"updated_by"`
	UpdatedAt           time.Time         `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedBy           *uuid.UUID        `gorm:"type:uuid;column:deleted_by" json:"deleted_by"`
	DeletedAt           *time.Time        `gorm:"column:deleted_at" json:"deleted_at"`
	Source              AgentSource       `gorm:"-" json:"-"`
}

// TableName specifies the table name for Agent.
func (Agent) TableName() string {
	return "agents"
}

func (a *Agent) IsWebAppActive() bool {
	if a == nil {
		return false
	}
	return NormalizeAgentWebAppStatus(a.WebAppStatus) == AgentWebAppStatusActive
}

// AgentExtension represents the agent_extensions table schema.
type AgentExtension struct {
	ID                 uuid.UUID `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	AgentID            uuid.UUID `gorm:"type:uuid;not null;uniqueIndex;column:agent_id" json:"agent_id"`
	Permission         *string   `gorm:"type:varchar(32)" json:"permission"`
	ExtendedProperties *string   `gorm:"type:jsonb;column:extended_properties" json:"extended_properties"`
	CreatedAt          time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt          time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// TableName specifies the table name for AgentExtension.
func (AgentExtension) TableName() string {
	return "agent_extensions"
}

// InstalledAgent represents the installed_agents table schema.
type InstalledAgent struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	TenantID           uuid.UUID  `gorm:"type:uuid;not null;index;column:tenant_id" json:"tenant_id"`
	AgentID            uuid.UUID  `gorm:"type:uuid;not null;index;column:agent_id" json:"agent_id"`
	AgentOwnerTenantID uuid.UUID  `gorm:"type:uuid;not null;column:agent_owner_tenant_id" json:"agent_owner_tenant_id"`
	Position           int        `gorm:"type:int;not null;default:0" json:"position"`
	IsPinned           bool       `gorm:"type:boolean;not null;default:false;column:is_pinned" json:"is_pinned"`
	LastUsedAt         *time.Time `gorm:"column:last_used_at" json:"last_used_at"`
	CreatedAt          time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

// TableName specifies the table name for InstalledAgent.
func (InstalledAgent) TableName() string {
	return "installed_agents"
}
