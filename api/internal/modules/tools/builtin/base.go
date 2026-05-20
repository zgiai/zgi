package builtin

import (
	"context"
	"fmt"

	"github.com/zgiai/zgi/api/internal/modules/tools"
)

// BuiltinTool is the base struct for all builtin tools
// Concrete tools should embed this struct and implement the Invoke method
type BuiltinTool struct {
	entity   tools.ToolEntity
	tenantID string
	runtime  *tools.ToolRuntime
}

// NewBuiltinTool creates a new builtin tool
func NewBuiltinTool(entity tools.ToolEntity, tenantID string) *BuiltinTool {
	return &BuiltinTool{
		entity:   entity,
		tenantID: tenantID,
	}
}

// GetEntity returns the tool entity
func (t *BuiltinTool) GetEntity() tools.ToolEntity {
	return t.entity
}

// GetProviderType returns the provider type
func (t *BuiltinTool) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeBuiltin
}

// GetTenantID returns the tenant ID
func (t *BuiltinTool) GetTenantID() string {
	return t.tenantID
}

// Runtime returns the tool runtime snapshot injected by ToolManager.
func (t *BuiltinTool) Runtime() *tools.ToolRuntime {
	return t.runtime
}

// GetRuntimeParameters returns the runtime parameters
func (t *BuiltinTool) GetRuntimeParameters(
	ctx context.Context,
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolParameter, error) {
	return t.entity.Parameters, nil
}

// Invoke is the default implementation that should be overridden by concrete tools
func (t *BuiltinTool) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolInvokeMessage, error) {
	return nil, fmt.Errorf("invoke not implemented for tool: %s", t.entity.Identity.Name)
}

// ForkToolRuntime creates a copy of the tool with new runtime parameters
func (t *BuiltinTool) ForkToolRuntime(runtime *tools.ToolRuntime) *BuiltinTool {
	tenantID := t.tenantID
	if runtime != nil && runtime.TenantID != "" {
		tenantID = runtime.TenantID
	}

	return &BuiltinTool{
		entity:   t.entity,
		tenantID: tenantID,
		runtime:  runtime,
	}
}

// ValidateCredentials validates the credentials (builtin tools don't need credentials)
func (t *BuiltinTool) ValidateCredentials(ctx context.Context, credentials map[string]interface{}) error {
	return nil
}

// CreateTextMessage creates a text message response
func CreateTextMessage(text string) tools.ToolInvokeMessage {
	return tools.ToolInvokeMessage{
		Type: tools.ToolInvokeMessageTypeText,
		Text: text,
	}
}

// CreateJSONMessage creates a JSON message response
func CreateJSONMessage(data map[string]interface{}) tools.ToolInvokeMessage {
	return tools.ToolInvokeMessage{
		Type: tools.ToolInvokeMessageTypeJSON,
		Data: data,
	}
}

// BuiltinProvider is the base struct for builtin tool providers
type BuiltinProvider struct {
	entity tools.ToolProviderEntity
	tools  map[string]tools.Tool
}

// NewBuiltinProvider creates a new builtin provider
func NewBuiltinProvider(identity tools.ToolProviderIdentity) *BuiltinProvider {
	return &BuiltinProvider{
		entity: tools.ToolProviderEntity{
			Identity: identity,
		},
		tools: make(map[string]tools.Tool),
	}
}

// RegisterTool registers a tool to the provider
func (p *BuiltinProvider) RegisterTool(tool tools.Tool) {
	entity := tool.GetEntity()
	p.tools[entity.Identity.Name] = tool
	p.entity.Tools = append(p.entity.Tools, entity)
}

// GetEntity returns the provider entity
func (p *BuiltinProvider) GetEntity() tools.ToolProviderEntity {
	return p.entity
}

// GetProviderType returns the provider type
func (p *BuiltinProvider) GetProviderType() tools.ToolProviderType {
	return tools.ToolProviderTypeBuiltin
}

// GetTool returns a tool by name
func (p *BuiltinProvider) GetTool(name string) (tools.Tool, error) {
	if tool, ok := p.tools[name]; ok {
		return tool, nil
	}
	return nil, tools.ErrToolNotFound
}

// GetTools returns all tools
func (p *BuiltinProvider) GetTools() []tools.Tool {
	result := make([]tools.Tool, 0, len(p.tools))
	for _, tool := range p.tools {
		result = append(result, tool)
	}
	return result
}

// ValidateCredentials validates the provider credentials (builtin providers don't need credentials)
func (p *BuiltinProvider) ValidateCredentials(ctx context.Context, credentials map[string]interface{}) error {
	return nil
}

// ============================================
// Builtin Provider Registry
// ============================================

// registry holds all registered builtin providers
var registry []tools.ToolProvider

// Register registers a builtin provider
// This should be called from init() functions in builtin tool packages
func Register(provider tools.ToolProvider) {
	registry = append(registry, provider)
}

// GetAllProviders returns all registered builtin providers
func GetAllProviders() []tools.ToolProvider {
	return registry
}
