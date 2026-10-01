package agentbindings

import (
	"time"

	"github.com/google/uuid"
)

type Scope string

const (
	ScopeDraft     Scope = "draft"
	ScopePublished Scope = "published"
)

// ConsumerType identifies the runtime surface that owns a resource binding.
// AgentID remains the owning application/root Agent for authorization and
// workspace lifecycle purposes; ConsumerID identifies the concrete Agent or
// Workflow graph that contains the binding.
type ConsumerType string

const (
	ConsumerTypeAgent    ConsumerType = "agent"
	ConsumerTypeWorkflow ConsumerType = "workflow"
)

type BindingType string

const (
	BindingTypeSkill            BindingType = "skill"
	BindingTypeKnowledgeDataset BindingType = "knowledge_dataset"
	BindingTypeDatabase         BindingType = "database"
	BindingTypeDatabaseTable    BindingType = "database_table"
	BindingTypeWorkflow         BindingType = "workflow"
)

type Binding struct {
	ID                   uuid.UUID              `gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	AgentID              uuid.UUID              `gorm:"type:uuid;not null;column:agent_id"`
	ConsumerType         ConsumerType           `gorm:"type:varchar(16);not null;default:'agent';column:consumer_type"`
	ConsumerID           uuid.UUID              `gorm:"type:uuid;not null;column:consumer_id"`
	BindingInstanceID    string                 `gorm:"type:varchar(255);not null;default:'';column:binding_instance_id"`
	BindingScope         Scope                  `gorm:"type:varchar(16);not null;column:binding_scope"`
	OrganizationID       uuid.UUID              `gorm:"type:uuid;not null;column:organization_id"`
	WorkspaceID          uuid.UUID              `gorm:"type:uuid;not null;column:workspace_id"`
	PublishedVersionUUID *uuid.UUID             `gorm:"type:uuid;column:published_version_uuid"`
	BindingType          BindingType            `gorm:"type:varchar(32);not null;column:binding_type"`
	ResourceID           string                 `gorm:"type:varchar(255);not null;column:resource_id"`
	ParentResourceID     string                 `gorm:"type:varchar(255);not null;default:'';column:parent_resource_id"`
	DisplayName          string                 `gorm:"type:varchar(255);not null;default:'';column:display_name"`
	AccessMode           string                 `gorm:"type:varchar(16);not null;default:'';column:access_mode"`
	Metadata             map[string]interface{} `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:metadata"`
	AuthorizedBy         *uuid.UUID             `gorm:"type:uuid;column:authorized_by"`
	AuthorizedAt         *time.Time             `gorm:"column:authorized_at"`
	CreatedAt            time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt            time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP"`
}

func (Binding) TableName() string {
	return "agent_resource_bindings"
}

type ScopeRef struct {
	AgentID              uuid.UUID
	Scope                Scope
	PublishedVersionUUID *uuid.UUID
}

// WorkflowScopeRef selects one concrete Workflow graph scope. WorkflowID is
// workflows.id (the draft or published graph row), while AgentID is the owning
// Workflow application's agents.id. PublishedVersionUUID is workflows.version_uuid.
type WorkflowScopeRef struct {
	AgentID              uuid.UUID
	WorkflowID           uuid.UUID
	Scope                Scope
	PublishedVersionUUID *uuid.UUID
}

type ResourceRef struct {
	OrganizationID    uuid.UUID
	WorkspaceID       *uuid.UUID
	AgentID           *uuid.UUID
	ConsumerType      ConsumerType
	ConsumerID        *uuid.UUID
	BindingInstanceID string
	Scope             *Scope
	BindingType       BindingType
	ResourceID        string
	ParentResourceID  string
}

// Match describes the concrete resource a single Agent tool step is about to use.
type Match struct {
	BindingType      BindingType
	ResourceID       string
	ParentResourceID string
	AccessMode       string
}
