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

type ResourceRef struct {
	OrganizationID   uuid.UUID
	WorkspaceID      *uuid.UUID
	AgentID          *uuid.UUID
	Scope            *Scope
	BindingType      BindingType
	ResourceID       string
	ParentResourceID string
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
