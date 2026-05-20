package model

import (
	"encoding/json"
	"time"
)

// InstalledPluginInfo represents installed plugin info stored in database.
// This is the metadata for a specific plugin version, shared across all accounts.
type InstalledPluginInfo struct {
	ID                   string          `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	MarketplacePluginID  string          `gorm:"type:uuid" json:"marketplace_plugin_id,omitempty"`
	MarketplaceVersionID string          `gorm:"type:uuid;not null;uniqueIndex" json:"marketplace_version_id"`
	PluginName           string          `gorm:"size:100;not null" json:"plugin_name"`
	PluginVersion        string          `gorm:"size:50;not null" json:"plugin_version"`
	PluginAuthor         string          `gorm:"size:100" json:"plugin_author,omitempty"`
	Declaration          json.RawMessage `gorm:"type:jsonb;not null" json:"declaration"`
	CreatedAt            time.Time       `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt            time.Time       `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName specifies the database table name
func (InstalledPluginInfo) TableName() string {
	return "installed_plugin_info"
}

// PluginDeclaration represents the declaration structure stored in JSONB
type PluginDeclaration struct {
	Provider ProviderDeclaration `json:"provider"`
	Tools    []ToolDeclaration   `json:"tools"`
}

// ProviderDeclaration represents provider info
type ProviderDeclaration struct {
	Name            string                 `json:"name"`
	Author          string                 `json:"author,omitempty"`
	Label           map[string]string      `json:"label"`
	Description     map[string]string      `json:"description"`
	Icon            string                 `json:"icon,omitempty"`
	Tags            []string               `json:"tags,omitempty"`
	ExecutionPolicy *PluginExecutionPolicy `json:"execution_policy,omitempty"`
}

// PluginExecutionPolicy defines plugin-side execution semantics for tool invocation.
type PluginExecutionPolicy struct {
	WaitMode              string `json:"wait_mode,omitempty"`
	StreamMode            string `json:"stream_mode,omitempty"`
	SessionPolicy         string `json:"session_policy,omitempty"`
	SessionIdleTTLSeconds int    `json:"session_idle_ttl_seconds,omitempty"`
	SerializeInvocations  *bool  `json:"serialize_invocations,omitempty"`
}

// ToolDeclaration represents a tool definition
type ToolDeclaration struct {
	Name           string                 `json:"name"`
	Label          map[string]string      `json:"label"`
	Description    ToolDescription        `json:"description"`
	Parameters     []ParameterDeclare     `json:"parameters"`
	Configurations []ConfigurationDeclare `json:"configurations,omitempty"`
}

// ToolDescription represents tool description with human and LLM variants.
type ToolDescription struct {
	Human map[string]string `json:"human"`
	LLM   string            `json:"llm"`
}

// ParameterDeclare represents a tool parameter
type ParameterDeclare struct {
	Name             string            `json:"name"`
	Type             string            `json:"type"`
	Required         bool              `json:"required"`
	Label            map[string]string `json:"label"`
	HumanDescription map[string]string `json:"human_description,omitempty"`
	LLMDescription   string            `json:"llm_description,omitempty"`
	Form             string            `json:"form"` // "llm" or "form"
	Default          interface{}       `json:"default,omitempty"`
	Options          []OptionDeclare   `json:"options,omitempty"`
}

// ConfigurationDeclare represents a tool configuration (credentials)
type ConfigurationDeclare struct {
	Name             string            `json:"name"`
	Type             string            `json:"type"` // text-input, secret-input, select
	Required         bool              `json:"required"`
	Label            map[string]string `json:"label"`
	Help             map[string]string `json:"help,omitempty"`
	Placeholder      map[string]string `json:"placeholder,omitempty"`
	Default          interface{}       `json:"default,omitempty"`
	Options          []OptionDeclare   `json:"options,omitempty"`
	HumanDescription map[string]string `json:"human_description,omitempty"`
}

// OptionDeclare represents a select option
type OptionDeclare struct {
	Value interface{}       `json:"value"`
	Label map[string]string `json:"label"`
}
