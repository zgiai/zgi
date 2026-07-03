package tools

import (
	"context"
	"errors"
)

// ============================================
// Errors
// ============================================

var (
	// ErrToolNotFound is returned when a tool is not found
	ErrToolNotFound = errors.New("tool not found")
	// ErrProviderNotFound is returned when a provider is not found
	ErrProviderNotFound = errors.New("provider not found")
)

// ============================================
// Tool Provider Types
// ============================================

// ToolProviderType represents the type of tool provider
type ToolProviderType string

const (
	// Supported types
	ToolProviderTypeBuiltin      ToolProviderType = "builtin"       // Built-in tools provided by the system
	ToolProviderTypePluginRunner ToolProviderType = "plugin_runner" // Compatibility value for marketplace tools
	ToolProviderTypeRunner       ToolProviderType = "runner"        // Preferred public alias for marketplace tools

	// Not supported - reserved for future extension
	// ToolProviderTypeAPI              ToolProviderType = "api"               // Custom OpenAPI tools - not supported
	// ToolProviderTypeWorkflow         ToolProviderType = "workflow"          // Workflow as tool - not supported
	// ToolProviderTypeApp              ToolProviderType = "app"               // App as tool - not supported
	// ToolProviderTypeDatasetRetrieval ToolProviderType = "dataset-retrieval" // Dataset retrieval - not supported
	// ToolProviderTypeMCP              ToolProviderType = "mcp"               // MCP protocol tools - not supported
	// ToolProviderTypePlugin           ToolProviderType = "plugin"            // Legacy plugin type - not supported
)

// String returns the string representation
func (t ToolProviderType) String() string {
	return string(t)
}

// NormalizeToolProviderType maps public aliases to the internal compatibility value.
func NormalizeToolProviderType(providerType ToolProviderType) ToolProviderType {
	switch providerType {
	case ToolProviderTypeRunner, "plugin":
		return ToolProviderTypePluginRunner
	default:
		return providerType
	}
}

// ============================================
// Tool Invoke Context
// ============================================

// ToolInvokeFrom represents where the tool is invoked from
type ToolInvokeFrom string

const (
	ToolInvokeFromAgent    ToolInvokeFrom = "agent"
	ToolInvokeFromWorkflow ToolInvokeFrom = "workflow"
	ToolInvokeFromAPI      ToolInvokeFrom = "api"
	ToolInvokeFromAIChat   ToolInvokeFrom = "aichat"
)

// ============================================
// i18n Types
// ============================================

// I18nText represents internationalized text with language codes as keys
// e.g., {"en_US": "Current Time", "zh_Hans": "Current time"}
type I18nText map[string]string

// Get returns the text for the specified language, falling back to en_US or first available
func (t I18nText) Get(lang string) string {
	if v, ok := t[lang]; ok {
		return v
	}
	if v, ok := t["en_US"]; ok {
		return v
	}
	for _, v := range t {
		return v
	}
	return ""
}

// ToolDescription represents tool description with human-readable and LLM-specific variants
type ToolDescription struct {
	Human I18nText `json:"human"`
	LLM   string   `json:"llm,omitempty"`
}

// ============================================
// Tool Identity
// ============================================

// ToolIdentity represents the identity of a tool
type ToolIdentity struct {
	Name     string   `json:"name"`
	Author   string   `json:"author,omitempty"`
	Provider string   `json:"provider,omitempty"`
	Label    I18nText `json:"label"`
	Icon     string   `json:"icon,omitempty"`
}

// ============================================
// Tool Parameter
// ============================================

// ToolParameterType represents the type of tool parameter
type ToolParameterType string

const (
	ToolParameterTypeString  ToolParameterType = "string"
	ToolParameterTypeNumber  ToolParameterType = "number"
	ToolParameterTypeBoolean ToolParameterType = "boolean"
	ToolParameterTypeSelect  ToolParameterType = "select"
	ToolParameterTypeFile    ToolParameterType = "file"
)

// ToolParameterForm represents the form type of tool parameter
type ToolParameterForm string

const (
	ToolParameterFormLLM  ToolParameterForm = "llm"
	ToolParameterFormForm ToolParameterForm = "form"
)

// ToolParameterOption represents an option for select type parameter
type ToolParameterOption struct {
	Value string   `json:"value"`
	Label I18nText `json:"label"`
}

// ToolParameter represents a tool parameter definition
type ToolParameter struct {
	Name             string                `json:"name"`
	Label            I18nText              `json:"label"`
	HumanDescription I18nText              `json:"human_description,omitempty"`
	LLMDescription   string                `json:"llm_description,omitempty"`
	Type             ToolParameterType     `json:"type"`
	Form             ToolParameterForm     `json:"form"`
	Required         bool                  `json:"required"`
	Default          interface{}           `json:"default,omitempty"`
	Options          []ToolParameterOption `json:"options,omitempty"`
	MinValue         *float64              `json:"min,omitempty"`
	MaxValue         *float64              `json:"max,omitempty"`
	Placeholder      I18nText              `json:"placeholder,omitempty"`
	SupportVariable  bool                  `json:"support_variable"` // Controls whether the {x} variable input button is shown
}

// ============================================
// Tool Entity
// ============================================

// ToolEntity represents the metadata of a tool
type ToolEntity struct {
	Identity    ToolIdentity    `json:"identity"`
	Description ToolDescription `json:"description"`
	Parameters  []ToolParameter `json:"parameters,omitempty"`
	OutputType  string          `json:"output_type,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
}

// ============================================
// Tool Runtime
// ============================================

// ToolRuntime represents the runtime configuration for a tool
type ToolRuntime struct {
	TenantID          string                 `json:"tenant_id"`
	Credentials       map[string]interface{} `json:"credentials,omitempty"`
	RuntimeParameters map[string]interface{} `json:"runtime_parameters,omitempty"`
	InvokeFrom        ToolInvokeFrom         `json:"invoke_from"`
}

// ============================================
// Tool Invoke Message Types
// ============================================

// ToolInvokeMessageType represents the type of tool invoke message
type ToolInvokeMessageType string

const (
	ToolInvokeMessageTypeText               ToolInvokeMessageType = "text"
	ToolInvokeMessageTypeJSON               ToolInvokeMessageType = "json"
	ToolInvokeMessageTypeImage              ToolInvokeMessageType = "image"
	ToolInvokeMessageTypeLink               ToolInvokeMessageType = "link"
	ToolInvokeMessageTypeFile               ToolInvokeMessageType = "file"
	ToolInvokeMessageTypeBlob               ToolInvokeMessageType = "blob"
	ToolInvokeMessageTypeVariable           ToolInvokeMessageType = "variable"
	ToolInvokeMessageTypeLog                ToolInvokeMessageType = "log"
	ToolInvokeMessageTypeImageLink          ToolInvokeMessageType = "image_link"
	ToolInvokeMessageTypeBinaryLink         ToolInvokeMessageType = "binary_link"
	ToolInvokeMessageTypeBlobChunk          ToolInvokeMessageType = "blob_chunk"
	ToolInvokeMessageTypeRetrieverResources ToolInvokeMessageType = "retriever_resources"
)

// ToolInvokeMessage represents a message returned from tool invocation
type ToolInvokeMessage struct {
	Type ToolInvokeMessageType  `json:"type"`
	Text string                 `json:"text,omitempty"`
	Data map[string]interface{} `json:"data,omitempty"`
	Meta map[string]interface{} `json:"meta,omitempty"`
}

// ============================================
// Tool Invoke Meta
// ============================================

// ToolInvokeMeta contains metadata about a tool invocation
type ToolInvokeMeta struct {
	TimeCost   float64                `json:"time_cost"`
	Error      string                 `json:"error,omitempty"`
	ToolConfig map[string]interface{} `json:"tool_config,omitempty"`
}

// NewEmptyToolInvokeMeta creates an empty tool invoke meta
func NewEmptyToolInvokeMeta() *ToolInvokeMeta {
	return &ToolInvokeMeta{
		TimeCost:   0,
		ToolConfig: make(map[string]interface{}),
	}
}

// NewErrorToolInvokeMeta creates a tool invoke meta with error
func NewErrorToolInvokeMeta(err string) *ToolInvokeMeta {
	return &ToolInvokeMeta{
		TimeCost:   0,
		Error:      err,
		ToolConfig: make(map[string]interface{}),
	}
}

// ============================================
// Tool Interface
// ============================================

// Tool is the interface that all tools must implement
type Tool interface {
	// GetEntity returns the tool entity
	GetEntity() ToolEntity

	// GetProviderType returns the provider type
	GetProviderType() ToolProviderType

	// GetTenantID returns the tenant ID
	GetTenantID() string

	// Invoke invokes the tool with the given parameters
	Invoke(
		ctx context.Context,
		userID string,
		toolParameters map[string]interface{},
		conversationID *string,
		appID *string,
		messageID *string,
	) ([]ToolInvokeMessage, error)

	// GetRuntimeParameters gets the runtime parameters for the tool
	GetRuntimeParameters(
		ctx context.Context,
		conversationID *string,
		appID *string,
		messageID *string,
	) ([]ToolParameter, error)

	// ForkToolRuntime creates a copy of the tool with new runtime parameters
	ForkToolRuntime(runtime *ToolRuntime) Tool

	// ValidateCredentials validates the credentials
	ValidateCredentials(ctx context.Context, credentials map[string]interface{}) error
}

// ToolGovernanceArgumentEnricher is an optional extension for tools that can
// make governance approval payloads more user-readable before invocation is
// frozen.
type ToolGovernanceArgumentEnricher interface {
	EnrichGovernanceArguments(ctx context.Context, userID string, toolParameters map[string]interface{}) map[string]interface{}
}

// ============================================
// Tool Provider Interface
// ============================================

// ToolProviderIdentity represents the identity of a tool provider
type ToolProviderIdentity struct {
	Name        string   `json:"name"`
	Author      string   `json:"author,omitempty"`
	Label       I18nText `json:"label"`
	Description I18nText `json:"description"`
	Icon        string   `json:"icon,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// ToolProviderEntity represents a tool provider entity
type ToolProviderEntity struct {
	Identity          ToolProviderIdentity `json:"identity"`
	ProviderType      ToolProviderType     `json:"type"`
	CredentialsSchema []ToolParameter      `json:"credentials_schema,omitempty"`
	Tools             []ToolEntity         `json:"tools,omitempty"`
}

// ToolProvider is the interface that all tool providers must implement
type ToolProvider interface {
	// GetEntity returns the provider entity
	GetEntity() ToolProviderEntity

	// GetProviderType returns the provider type
	GetProviderType() ToolProviderType

	// GetTool returns a tool by name
	GetTool(name string) (Tool, error)

	// GetTools returns all tools
	GetTools() []Tool

	// ValidateCredentials validates the provider credentials
	ValidateCredentials(ctx context.Context, credentials map[string]interface{}) error
}
