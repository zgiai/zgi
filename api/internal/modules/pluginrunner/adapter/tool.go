package adapter

import (
	"context"
	"fmt"

	"github.com/zgiai/zgi/api/pkg/logger"
)

// PluginRunnerTool represents a tool that can be invoked through the plugin runner
type PluginRunnerTool struct {
	entity                 ToolEntity
	tenantID               string
	provider               string
	pluginUniqueIdentifier string
	manager                *PluginRunnerToolManager
}

// NewPluginRunnerTool creates a new PluginRunnerTool
func NewPluginRunnerTool(
	entity ToolEntity,
	tenantID string,
	provider string,
	pluginUniqueIdentifier string,
	manager *PluginRunnerToolManager,
) *PluginRunnerTool {
	return &PluginRunnerTool{
		entity:                 entity,
		tenantID:               tenantID,
		provider:               provider,
		pluginUniqueIdentifier: pluginUniqueIdentifier,
		manager:                manager,
	}
}

// GetEntity returns the tool entity
func (t *PluginRunnerTool) GetEntity() ToolEntity {
	return t.entity
}

// GetTenantID returns the tenant ID
func (t *PluginRunnerTool) GetTenantID() string {
	return t.tenantID
}

// GetProvider returns the provider name
func (t *PluginRunnerTool) GetProvider() string {
	return t.provider
}

// GetPluginUniqueIdentifier returns the plugin unique identifier
func (t *PluginRunnerTool) GetPluginUniqueIdentifier() string {
	return t.pluginUniqueIdentifier
}

// Invoke invokes the tool with the given parameters
func (t *PluginRunnerTool) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]ToolInvokeMessage, error) {
	logger.Debug("invoking plugin runner tool",
		"tenant_id", t.tenantID,
		"user_id", userID,
		"provider", t.provider,
		"tool_name", t.entity.Identity.Name)

	if t.manager == nil {
		return nil, fmt.Errorf("plugin runner tool manager not set")
	}

	// Use the manager to invoke the tool
	return t.manager.Invoke(
		ctx,
		t.tenantID,
		userID,
		t.provider,
		t.entity.Identity.Name,
		nil, // credentials
		CredentialTypeAPIKey,
		toolParameters,
		conversationID,
		appID,
		messageID,
	)
}

// ForkToolRuntime forks the tool runtime with new parameters
func (t *PluginRunnerTool) ForkToolRuntime(
	toolParameters map[string]interface{},
) *PluginRunnerTool {
	// Create a new tool with the same properties but potentially different parameters
	return &PluginRunnerTool{
		entity:                 t.entity,
		tenantID:               t.tenantID,
		provider:               t.provider,
		pluginUniqueIdentifier: t.pluginUniqueIdentifier,
		manager:                t.manager,
	}
}

// GetRuntimeParameters gets the runtime parameters for the tool
func (t *PluginRunnerTool) GetRuntimeParameters(
	conversationID *string,
	appID *string,
	messageID *string,
) ([]map[string]interface{}, error) {
	// In our simplified implementation, we'll return empty parameters
	// A more complete implementation would fetch tool schema from the plugin
	return []map[string]interface{}{}, nil
}
