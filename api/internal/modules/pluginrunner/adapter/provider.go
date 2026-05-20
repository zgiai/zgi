package adapter

import (
	"fmt"
)

// PluginRunnerToolProviderController manages plugin tool providers
type PluginRunnerToolProviderController struct {
	entity                 PluginToolProviderEntity
	tenantID               string
	pluginID               string
	pluginUniqueIdentifier string
	manager                *PluginRunnerToolManager
}

// ToolIdentity represents the identity of a tool
type ToolIdentity struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
}

// ToolEntity represents a tool entity
type ToolEntity struct {
	Identity    ToolIdentity             `json:"identity"`
	Description string                   `json:"description,omitempty"`
	Parameters  []map[string]interface{} `json:"parameters,omitempty"`
}

// NewPluginRunnerToolProviderController creates a new PluginRunnerToolProviderController
func NewPluginRunnerToolProviderController(
	entity PluginToolProviderEntity,
	pluginID string,
	pluginUniqueIdentifier string,
	tenantID string,
	manager *PluginRunnerToolManager,
) *PluginRunnerToolProviderController {
	return &PluginRunnerToolProviderController{
		entity:                 entity,
		tenantID:               tenantID,
		pluginID:               pluginID,
		pluginUniqueIdentifier: pluginUniqueIdentifier,
		manager:                manager,
	}
}

// GetEntity returns the provider entity
func (p *PluginRunnerToolProviderController) GetEntity() PluginToolProviderEntity {
	return p.entity
}

// GetPluginID returns the plugin ID
func (p *PluginRunnerToolProviderController) GetPluginID() string {
	return p.pluginID
}

// GetPluginUniqueIdentifier returns the plugin unique identifier
func (p *PluginRunnerToolProviderController) GetPluginUniqueIdentifier() string {
	return p.pluginUniqueIdentifier
}

// GetTenantID returns the tenant ID
func (p *PluginRunnerToolProviderController) GetTenantID() string {
	return p.tenantID
}

// ValidateCredentials validates the credentials of the provider
func (p *PluginRunnerToolProviderController) ValidateCredentials(userID string, credentials map[string]interface{}) error {
	// In our simplified implementation, we'll just check if the plugin exists
	// A more complete implementation would validate actual credentials
	return nil
}

// GetTool returns a specific tool by name
func (p *PluginRunnerToolProviderController) GetTool(toolName string) (*PluginRunnerTool, error) {
	// Create a tool entity based on the plugin manifest
	toolEntity := ToolEntity{
		Identity: ToolIdentity{
			Name:     toolName,
			Provider: p.entity.Declaration.Name,
		},
		Description: fmt.Sprintf("Tool %s from plugin %s", toolName, p.entity.Declaration.Name),
	}

	return NewPluginRunnerTool(
		toolEntity,
		p.tenantID,
		p.entity.Declaration.Name, // plugin name as provider
		p.pluginUniqueIdentifier,
		p.manager,
	), nil
}

// GetTools returns all tools provided by this plugin
func (p *PluginRunnerToolProviderController) GetTools() []*PluginRunnerTool {
	// For now, we'll return an empty slice since our plugin runner doesn't expose
	// individual tools in its manifest
	// A more complete implementation would parse the plugin manifest to extract tools
	return []*PluginRunnerTool{}
}

// ToolProviderType returns the type of the provider
func (p *PluginRunnerToolProviderController) ToolProviderType() string {
	return "plugin_runner"
}
