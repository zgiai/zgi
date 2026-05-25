package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/zgiai/zgi/api/pkg/logger"
)

// ToolEngine is the central engine for executing tools
type ToolEngine struct {
	toolManager *ToolManager
}

// NewToolEngine creates a new ToolEngine
func NewToolEngine(toolManager *ToolManager) *ToolEngine {
	return &ToolEngine{
		toolManager: toolManager,
	}
}

// InvokeRequest represents a tool invocation request
type InvokeRequest struct {
	ProviderType      ToolProviderType       `json:"provider_type"`
	ProviderID        string                 `json:"provider_id"`
	ToolName          string                 `json:"tool_name"`
	TenantID          string                 `json:"tenant_id"`
	UserID            string                 `json:"user_id"`
	Parameters        map[string]interface{} `json:"parameters"`
	CredentialID      string                 `json:"credential_id,omitempty"`
	ConversationID    string                 `json:"conversation_id,omitempty"`
	AppID             string                 `json:"app_id,omitempty"`
	MessageID         string                 `json:"message_id,omitempty"`
	InvokeFrom        ToolInvokeFrom         `json:"invoke_from"`
	RuntimeParameters map[string]interface{} `json:"runtime_parameters,omitempty"`
}

// InvokeResult represents the result of a tool invocation
type InvokeResult struct {
	Messages []ToolInvokeMessage `json:"messages"`
	Meta     *ToolInvokeMeta     `json:"meta"`
	Success  bool                `json:"success"`
	Error    string              `json:"error,omitempty"`
}

// Invoke invokes a tool based on the request
func (e *ToolEngine) Invoke(ctx context.Context, req InvokeRequest) (*InvokeResult, error) {
	startTime := time.Now()

	logger.Debug("tool engine invoking tool",
		"provider_type", req.ProviderType,
		"provider_id", req.ProviderID,
		"tool_name", req.ToolName,
		"tenant_id", req.TenantID,
		"user_id", req.UserID)

	// Get tool runtime
	tool, err := e.toolManager.GetToolRuntime(
		ctx,
		req.ProviderType,
		req.ProviderID,
		req.ToolName,
		req.TenantID,
		req.InvokeFrom,
		req.CredentialID,
		req.RuntimeParameters,
	)
	if err != nil {
		return &InvokeResult{
			Success: false,
			Error:   fmt.Sprintf("failed to get tool runtime: %v", err),
			Meta:    NewErrorToolInvokeMeta(err.Error()),
		}, err
	}

	// Prepare optional parameters
	var conversationID, appID, messageID *string
	if req.ConversationID != "" {
		conversationID = &req.ConversationID
	}
	if req.AppID != "" {
		appID = &req.AppID
	}
	if req.MessageID != "" {
		messageID = &req.MessageID
	}

	// Invoke the tool
	messages, err := tool.Invoke(ctx, req.UserID, req.Parameters, conversationID, appID, messageID)
	if err != nil {
		elapsed := time.Since(startTime).Seconds()
		return &InvokeResult{
			Success: false,
			Error:   fmt.Sprintf("tool invocation failed: %v", err),
			Meta: &ToolInvokeMeta{
				TimeCost: elapsed,
				Error:    err.Error(),
			},
		}, err
	}

	elapsed := time.Since(startTime).Seconds()
	return &InvokeResult{
		Messages: messages,
		Success:  true,
		Meta: &ToolInvokeMeta{
			TimeCost: elapsed,
		},
	}, nil
}

// InvokeWithCallback invokes a tool and calls the callback with each message
func (e *ToolEngine) InvokeWithCallback(
	ctx context.Context,
	req InvokeRequest,
	callback func(ToolInvokeMessage) error,
) (*ToolInvokeMeta, error) {
	result, err := e.Invoke(ctx, req)
	if err != nil {
		return result.Meta, err
	}

	// Call callback for each message
	for _, msg := range result.Messages {
		if err := callback(msg); err != nil {
			return result.Meta, fmt.Errorf("callback error: %w", err)
		}
	}

	return result.Meta, nil
}

// WorkflowToolInvokeRequest is a request for invoking a tool from a workflow
type WorkflowToolInvokeRequest struct {
	TenantID           string                 `json:"tenant_id"`
	AppID              string                 `json:"app_id"`
	NodeID             string                 `json:"node_id"`
	UserID             string                 `json:"user_id"`
	ProviderType       ToolProviderType       `json:"provider_type"`
	ProviderID         string                 `json:"provider_id"`
	ToolName           string                 `json:"tool_name"`
	CredentialID       string                 `json:"credential_id,omitempty"`
	ToolConfigurations map[string]interface{} `json:"tool_configurations"`
	ToolCredentials    map[string]interface{} `json:"tool_credentials,omitempty"` // Credentials for plugin runtime
	ConversationID     string                 `json:"conversation_id,omitempty"`
}

// InvokeForWorkflow invokes a tool for workflow execution
func (e *ToolEngine) InvokeForWorkflow(ctx context.Context, req WorkflowToolInvokeRequest) (*InvokeResult, error) {
	// Prepare parameters with credentials injection
	parameters := req.ToolConfigurations
	if parameters == nil {
		parameters = make(map[string]interface{})
	}

	// Inject credentials as __credentials key for Plugin SDK runtime.credentials
	if len(req.ToolCredentials) > 0 {
		parameters["__credentials"] = req.ToolCredentials
	}

	return e.Invoke(ctx, InvokeRequest{
		ProviderType:   req.ProviderType,
		ProviderID:     req.ProviderID,
		ToolName:       req.ToolName,
		TenantID:       req.TenantID,
		UserID:         req.UserID,
		Parameters:     parameters,
		CredentialID:   req.CredentialID,
		ConversationID: req.ConversationID,
		AppID:          req.AppID,
		InvokeFrom:     ToolInvokeFromWorkflow,
	})
}

// AgentToolInvokeRequest is a request for invoking a tool from an agent
type AgentToolInvokeRequest struct {
	TenantID       string                 `json:"tenant_id"`
	AppID          string                 `json:"app_id"`
	UserID         string                 `json:"user_id"`
	ProviderType   ToolProviderType       `json:"provider_type"`
	ProviderID     string                 `json:"provider_id"`
	ToolName       string                 `json:"tool_name"`
	CredentialID   string                 `json:"credential_id,omitempty"`
	Parameters     map[string]interface{} `json:"parameters"`
	ConversationID string                 `json:"conversation_id,omitempty"`
	MessageID      string                 `json:"message_id,omitempty"`
}

// InvokeForAgent invokes a tool for agent execution
func (e *ToolEngine) InvokeForAgent(ctx context.Context, req AgentToolInvokeRequest) (*InvokeResult, error) {
	return e.Invoke(ctx, InvokeRequest{
		ProviderType:   req.ProviderType,
		ProviderID:     req.ProviderID,
		ToolName:       req.ToolName,
		TenantID:       req.TenantID,
		UserID:         req.UserID,
		Parameters:     req.Parameters,
		CredentialID:   req.CredentialID,
		ConversationID: req.ConversationID,
		AppID:          req.AppID,
		MessageID:      req.MessageID,
		InvokeFrom:     ToolInvokeFromAgent,
	})
}

// StopReusableSessionsByWorkflowRunID force-cleans reusable sessions for a workflow run.
func (e *ToolEngine) StopReusableSessionsByWorkflowRunID(ctx context.Context, workflowRunID string) (int, error) {
	if e.toolManager == nil {
		return 0, nil
	}
	return e.toolManager.StopReusableSessionsByWorkflowRunID(ctx, workflowRunID)
}

// SweepStaleReusableSessions sweeps stale reusable sessions older than maxAge.
func (e *ToolEngine) SweepStaleReusableSessions(ctx context.Context, maxAge time.Duration) (int, error) {
	if e.toolManager == nil {
		return 0, nil
	}
	return e.toolManager.SweepStaleReusableSessions(ctx, maxAge)
}

// ValidateProviderCredentials validates provider credentials
func (e *ToolEngine) ValidateProviderCredentials(
	ctx context.Context,
	providerType ToolProviderType,
	providerID string,
	tenantID string,
	credentials map[string]interface{},
) error {
	provider, err := e.toolManager.GetProvider(ctx, providerType, providerID, tenantID)
	if err != nil {
		return fmt.Errorf("failed to get provider: %w", err)
	}
	return provider.ValidateCredentials(ctx, credentials)
}

// GetToolParameters returns the parameters for a tool
func (e *ToolEngine) GetToolParameters(
	ctx context.Context,
	providerType ToolProviderType,
	providerID string,
	toolName string,
	tenantID string,
) ([]ToolParameter, error) {
	tool, err := e.toolManager.GetToolRuntime(
		ctx,
		providerType,
		providerID,
		toolName,
		tenantID,
		ToolInvokeFromAPI,
		"",
		nil,
	)
	if err != nil {
		return nil, err
	}

	return tool.GetRuntimeParameters(ctx, nil, nil, nil)
}
