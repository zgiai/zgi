package tools

import (
	"context"
	"fmt"

	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/client"
	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/service"
)

// ============================================
// Integration Example
// ============================================

// This file demonstrates how to integrate the tool system with the Plugin Runner.
//
// Architecture Overview:
//
//     ┌─────────────────────────────────────────────────────────────────────────┐
//     │                           ZGI API (Current Project)                      │
//     ├─────────────────────────────────────────────────────────────────────────┤
//     │                                                                          │
//     │  ┌──────────────┐     ┌─────────────────┐     ┌────────────────────┐    │
//     │  │  Workflow    │ ──▶ │   ToolEngine    │ ──▶ │    ToolManager     │    │
//     │  │  Agent Node  │     │                 │     │                    │    │
//     │  └──────────────┘     └─────────────────┘     └─────────┬──────────┘    │
//     │                                                          │               │
//     │                                        ┌─────────────────┴───────────┐  │
//     │                                        │                             │  │
//     │                                        ▼                             ▼  │
//     │                            ┌─────────────────────┐   ┌─────────────────┐│
//     │                            │ PluginRunnerAdapter │   │ BuiltinProvider ││
//     │                            └──────────┬──────────┘   └─────────────────┘│
//     │                                       │                                 │
//     └───────────────────────────────────────┼─────────────────────────────────┘
//                                             │
//                                             │ HTTP
//                                             ▼
//                                     ┌───────────────┐
//                                     │ Plugin Runner │
//                                     │ (Go Service)  │
//                                     └───────────────┘
//
// Multi-Tenant Flow:
//
//     1. Request comes with Tenant ID
//     2. ToolManager.GetProvider(ctx, providerType, providerName, tenantID)
//     3. PluginRunnerAdapter starts session with TenantID in request
//     4. Plugin Runner validates tenant access (checkTenantAccess)
//     5. Tool invocation with tenant context
//     6. Audit logging with tenant_id
//
// Key Concepts:
//
//     - ToolProviderType: Defines the source of a tool (Builtin, API, Workflow, PluginRunner)
//     - ToolProvider: Interface for tool providers (each provider has multiple tools)
//     - Tool: Interface for tools (can be invoked with parameters)
//     - ToolEngine: Central engine for tool execution
//     - ToolManager: Registry and factory for tools

// IntegrationExample demonstrates how to use the tool system
type IntegrationExample struct {
	toolManager *ToolManager
	toolEngine  *ToolEngine
}

// NewIntegrationExample creates a new integration example
func NewIntegrationExample(pluginRunnerBaseURL, apiKey string) (*IntegrationExample, error) {
	// 1. Create Plugin Runner service
	pluginRunnerService := service.NewPluginRunnerService(&client.Config{
		BaseURL: pluginRunnerBaseURL, // e.g., "http://localhost:2665"
		APIKey:  apiKey,
	})

	// 2. Create Plugin Runner adapter (implements PluginRunnerToolManagerInterface)
	adapter := NewPluginRunnerToolAdapter(pluginRunnerService, nil)

	// 3. Create ToolManager with the adapter
	toolManager := NewToolManager(adapter)

	// 4. Create ToolEngine
	toolEngine := NewToolEngine(toolManager)

	return &IntegrationExample{
		toolManager: toolManager,
		toolEngine:  toolEngine,
	}, nil
}

// InvokePluginTool demonstrates how to invoke a plugin runner tool
func (e *IntegrationExample) InvokePluginTool(
	ctx context.Context,
	tenantID string,
	userID string,
	providerName string,
	toolName string,
	params map[string]interface{},
) (*InvokeResult, error) {
	return e.toolEngine.Invoke(ctx, InvokeRequest{
		ProviderType: ToolProviderTypePluginRunner,
		ProviderID:   providerName,
		ToolName:     toolName,
		TenantID:     tenantID,
		UserID:       userID,
		Parameters:   params,
		InvokeFrom:   ToolInvokeFromWorkflow,
	})
}

// InvokeForWorkflowNode demonstrates how to use in a workflow node
func (e *IntegrationExample) InvokeForWorkflowNode(
	ctx context.Context,
	tenantID string,
	appID string,
	nodeID string,
	userID string,
	providerName string,
	toolName string,
	params map[string]interface{},
) (*InvokeResult, error) {
	return e.toolEngine.InvokeForWorkflow(ctx, WorkflowToolInvokeRequest{
		TenantID:           tenantID,
		AppID:              appID,
		NodeID:             nodeID,
		UserID:             userID,
		ProviderType:       ToolProviderTypePluginRunner,
		ProviderID:         providerName,
		ToolName:           toolName,
		ToolConfigurations: params,
	})
}

// InvokeForAgentTool demonstrates how to use in an agent
func (e *IntegrationExample) InvokeForAgentTool(
	ctx context.Context,
	tenantID string,
	appID string,
	userID string,
	providerName string,
	toolName string,
	params map[string]interface{},
	conversationID string,
	messageID string,
) (*InvokeResult, error) {
	return e.toolEngine.InvokeForAgent(ctx, AgentToolInvokeRequest{
		TenantID:       tenantID,
		AppID:          appID,
		UserID:         userID,
		ProviderType:   ToolProviderTypePluginRunner,
		ProviderID:     providerName,
		ToolName:       toolName,
		Parameters:     params,
		ConversationID: conversationID,
		MessageID:      messageID,
	})
}

// ListAvailableTools demonstrates how to list available tools
func (e *IntegrationExample) ListAvailableTools(ctx context.Context, tenantID string) ([]ToolProviderEntity, error) {
	return e.toolManager.ListAllProviders(ctx, tenantID)
}

// BatchInvoke demonstrates how to invoke multiple tools efficiently
func (e *IntegrationExample) BatchInvoke(
	ctx context.Context,
	tenantID string,
	userID string,
	providerName string,
	tools []struct {
		Name   string
		Params map[string]interface{}
	},
) ([]BatchToolResult, error) {
	// Get the adapter
	adapter, ok := e.toolManager.pluginRunnerManager.(*PluginRunnerToolAdapter)
	if !ok {
		return nil, fmt.Errorf("plugin runner manager is not of expected type")
	}

	// Build batch request
	batchTools := make([]BatchToolRequest, len(tools))
	for i, t := range tools {
		batchTools[i] = BatchToolRequest{
			Tool:       t.Name,
			Parameters: t.Params,
		}
	}

	// Execute batch
	result, err := adapter.BatchInvoke(ctx, BatchInvokeRequest{
		TenantID: tenantID,
		UserID:   userID,
		Provider: providerName,
		Tools:    batchTools,
	})
	if err != nil {
		return nil, err
	}

	return result.Results, nil
}

// ============================================
// Usage in Workflow Node
// ============================================

// WorkflowToolNode is an example of how a workflow tool node would use the tool system
type WorkflowToolNode struct {
	toolEngine *ToolEngine
}

// Run executes the tool node
func (n *WorkflowToolNode) Run(
	ctx context.Context,
	tenantID string,
	appID string,
	nodeID string,
	userID string,
	providerType ToolProviderType,
	providerID string,
	toolName string,
	parameters map[string]interface{},
	conversationID string,
) (*InvokeResult, error) {
	return n.toolEngine.InvokeForWorkflow(ctx, WorkflowToolInvokeRequest{
		TenantID:           tenantID,
		AppID:              appID,
		NodeID:             nodeID,
		UserID:             userID,
		ProviderType:       providerType,
		ProviderID:         providerID,
		ToolName:           toolName,
		ToolConfigurations: parameters,
		ConversationID:     conversationID,
	})
}

// ============================================
// Usage in Agent
// ============================================

// AgentToolExecutor is an example of how an agent would use the tool system
type AgentToolExecutor struct {
	toolEngine *ToolEngine
}

// ExecuteTool executes a tool for the agent
func (e *AgentToolExecutor) ExecuteTool(
	ctx context.Context,
	tenantID string,
	appID string,
	userID string,
	providerType ToolProviderType,
	providerID string,
	toolName string,
	parameters map[string]interface{},
	conversationID string,
	messageID string,
) (*InvokeResult, error) {
	return e.toolEngine.InvokeForAgent(ctx, AgentToolInvokeRequest{
		TenantID:       tenantID,
		AppID:          appID,
		UserID:         userID,
		ProviderType:   providerType,
		ProviderID:     providerID,
		ToolName:       toolName,
		Parameters:     parameters,
		ConversationID: conversationID,
		MessageID:      messageID,
	})
}
