package agentbindings

import (
	"sort"
	"strings"
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
	BindingTypeSkill                 BindingType = "skill"
	BindingTypeKnowledgeDataset      BindingType = "knowledge_dataset"
	BindingTypeDatabase              BindingType = "database"
	BindingTypeDatabaseTable         BindingType = "database_table"
	BindingTypeWorkflow              BindingType = "workflow"
	BindingTypeIntegrationConnection BindingType = "integration_connection"
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

// WorkflowConnectionReference is the transport-neutral detail needed to
// explain or revoke a Workflow connector reference. Presentation layers may
// enrich Agent/Workspace names without importing the Workflow module here.
type WorkflowConnectionReference struct {
	OrganizationID       uuid.UUID  `json:"organization_id"`
	WorkspaceID          uuid.UUID  `json:"workspace_id"`
	AgentID              uuid.UUID  `json:"agent_id"`
	WorkflowID           uuid.UUID  `json:"workflow_id"`
	Scope                Scope      `json:"scope"`
	PublishedVersionUUID *uuid.UUID `json:"published_version_uuid,omitempty"`
	NodeID               string     `json:"node_id"`
	NodeTitle            string     `json:"node_title"`
	IntegrationID        string     `json:"integration_id"`
	ConnectionID         string     `json:"connection_id"`
	ActionIDs            []string   `json:"action_ids"`
}

// AsWorkflowConnectionReference projects one Workflow Integration binding
// into a stable impact-detail contract.
func (binding Binding) AsWorkflowConnectionReference() (WorkflowConnectionReference, bool) {
	if binding.ConsumerType != ConsumerTypeWorkflow ||
		binding.ConsumerID == uuid.Nil ||
		binding.BindingType != BindingTypeIntegrationConnection ||
		strings.TrimSpace(binding.BindingInstanceID) == "" {
		return WorkflowConnectionReference{}, false
	}
	return WorkflowConnectionReference{
		OrganizationID:       binding.OrganizationID,
		WorkspaceID:          binding.WorkspaceID,
		AgentID:              binding.AgentID,
		WorkflowID:           binding.ConsumerID,
		Scope:                binding.BindingScope,
		PublishedVersionUUID: cloneUUID(binding.PublishedVersionUUID),
		NodeID:               strings.TrimSpace(binding.BindingInstanceID),
		NodeTitle:            strings.TrimSpace(binding.DisplayName),
		IntegrationID:        strings.TrimSpace(binding.ParentResourceID),
		ConnectionID:         strings.TrimSpace(binding.ResourceID),
		ActionIDs:            IntegrationAllowedActionIDs(binding),
	}, true
}

// Match describes the concrete resource a single Agent tool step is about to use.
type Match struct {
	BindingType      BindingType
	ResourceID       string
	ParentResourceID string
	AccessMode       string
}

const IntegrationAllowedActionIDsMetadataKey = "allowed_action_ids"

func IntegrationAllowedActionIDs(binding Binding) []string {
	if binding.BindingType != BindingTypeIntegrationConnection || binding.Metadata == nil {
		return []string{}
	}
	raw, ok := binding.Metadata[IntegrationAllowedActionIDsMetadataKey]
	if !ok {
		return []string{}
	}
	values := make([]string, 0)
	switch typed := raw.(type) {
	case []string:
		values = append(values, typed...)
	case []interface{}:
		for _, item := range typed {
			if value, ok := item.(string); ok {
				values = append(values, value)
			}
		}
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func (binding Binding) AllowsIntegrationAction(actionID string) bool {
	actionID = strings.ToLower(strings.TrimSpace(actionID))
	if actionID == "" {
		return false
	}
	for _, allowed := range IntegrationAllowedActionIDs(binding) {
		if allowed == actionID {
			return true
		}
	}
	return false
}
