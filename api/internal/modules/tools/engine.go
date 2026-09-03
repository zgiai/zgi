package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
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

// ResolveToolEntity returns the authoritative catalog contract used by a Tool
// node. Workflow execution uses this as a compatibility fallback for drafts
// saved before structured output snapshots were introduced.
func (e *ToolEngine) ResolveToolEntity(
	ctx context.Context,
	providerType ToolProviderType,
	providerID, toolName, tenantID string,
) (*ToolEntity, error) {
	if e == nil || e.toolManager == nil {
		return nil, fmt.Errorf("tool manager is not configured")
	}
	provider, err := e.toolManager.GetProvider(ctx, providerType, providerID, tenantID)
	if err != nil {
		return nil, err
	}
	tool, err := provider.GetTool(toolName)
	if err != nil {
		return nil, err
	}
	entity := tool.GetEntity()
	return &entity, nil
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
	// ConnectorIdentity is an internal, typed, non-secret authorization and
	// idempotency identity. It is never decoded from a public Tool payload.
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
	if NormalizeToolProviderType(req.ProviderType) == ToolProviderTypeConnector {
		err := fmt.Errorf("Connector operations require native dispatch")
		return &InvokeResult{Success: false, Error: err.Error(), Meta: NewErrorToolInvokeMeta(err.Error())}, err
	}
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
			Messages: safeConnectorRuntimeErrorMessages(req.ProviderType, tool, messages, err),
			Success:  false,
			Error:    fmt.Sprintf("tool invocation failed: %v", err),
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

// connectorRuntimeErrorMessageProjector is intentionally implemented only by
// the server-owned Connector metadata type. ToolEngine never forwards the
// original message on an error path; the projector must rebuild a bounded,
// credential-free control message from typed fields.
type connectorRuntimeErrorMessageProjector interface {
	SafeConnectorRuntimeErrorMessage() (ToolInvokeMessage, bool)
}

func safeConnectorRuntimeErrorMessages(
	providerType ToolProviderType,
	tool Tool,
	messages []ToolInvokeMessage,
	err error,
) []ToolInvokeMessage {
	if NormalizeToolProviderType(providerType) != ToolProviderTypeConnector ||
		!connectorcore.IsErrorCode(err, connectorcore.ErrorCodeConfirmationRequired) ||
		len(messages) != 1 || tool == nil {
		return nil
	}
	native, nativeOK := tool.(ConnectorNativeTool)
	if !nativeOK || !native.IsConnectorNativeTool() {
		return nil
	}
	message := messages[0]
	if message.Type != ToolInvokeMessageTypeJSON || len(message.Meta) != 1 {
		return nil
	}
	for _, rawMetadata := range message.Meta {
		projector, ok := rawMetadata.(connectorRuntimeErrorMessageProjector)
		if !ok {
			return nil
		}
		projected, valid := projector.SafeConnectorRuntimeErrorMessage()
		if !valid || !validSafeConnectorRuntimeErrorMessage(projected) {
			return nil
		}
		return []ToolInvokeMessage{projected}
	}
	return nil
}

func validSafeConnectorRuntimeErrorMessage(message ToolInvokeMessage) bool {
	if message.Type != ToolInvokeMessageTypeJSON || message.Text != "" || len(message.Data) != 2 ||
		len(message.Meta) != 1 || message.Data["state"] != "blocked" {
		return false
	}
	decisionID, ok := message.Data["decision_id"].(string)
	if !ok {
		return false
	}
	parsedDecisionID, err := uuid.Parse(decisionID)
	if err != nil || parsedDecisionID == uuid.Nil {
		return false
	}
	rawMetadata, ok := message.Meta["connector_execution"]
	if !ok {
		return false
	}
	encoded, err := json.Marshal(rawMetadata)
	if err != nil {
		return false
	}
	metadata := map[string]interface{}{}
	if err := json.Unmarshal(encoded, &metadata); err != nil || len(metadata) != 2 ||
		metadata["state"] != "blocked" || metadata["decision_id"] != decisionID {
		return false
	}
	return true
}

// EnrichGovernanceArguments gives a runtime tool a chance to add display-only
// context, such as asset names, before the governance decision freezes a call.
func (e *ToolEngine) EnrichGovernanceArguments(ctx context.Context, req InvokeRequest) (map[string]interface{}, error) {
	if NormalizeToolProviderType(req.ProviderType) == ToolProviderTypeConnector {
		return req.Parameters, fmt.Errorf("Connector operations require native dispatch")
	}
	if e == nil || e.toolManager == nil {
		return req.Parameters, nil
	}
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
		return req.Parameters, err
	}
	if enricher, ok := tool.(ToolGovernanceArgumentEnricherWithError); ok {
		enriched, enrichErr := enricher.EnrichGovernanceArgumentsWithError(ctx, req.UserID, req.Parameters)
		if enrichErr != nil {
			return req.Parameters, enrichErr
		}
		if enriched == nil {
			return req.Parameters, nil
		}
		return enriched, nil
	}
	enricher, ok := tool.(ToolGovernanceArgumentEnricher)
	if !ok {
		return req.Parameters, nil
	}
	enriched := enricher.EnrichGovernanceArguments(ctx, req.UserID, req.Parameters)
	if enriched == nil {
		return req.Parameters, nil
	}
	return enriched, nil
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
	RuntimeParameters  map[string]interface{} `json:"runtime_parameters,omitempty"`
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
		ProviderType:      req.ProviderType,
		ProviderID:        req.ProviderID,
		ToolName:          req.ToolName,
		TenantID:          req.TenantID,
		UserID:            req.UserID,
		Parameters:        parameters,
		CredentialID:      req.CredentialID,
		ConversationID:    req.ConversationID,
		AppID:             req.AppID,
		InvokeFrom:        ToolInvokeFromWorkflow,
		RuntimeParameters: req.RuntimeParameters,
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
	UserScope      string                 `json:"user_scope,omitempty"`
}

// InvokeForAgent invokes a tool for agent execution
func (e *ToolEngine) InvokeForAgent(ctx context.Context, req AgentToolInvokeRequest) (*InvokeResult, error) {
	return e.Invoke(ctx, InvokeRequest{
		ProviderType:      req.ProviderType,
		ProviderID:        req.ProviderID,
		ToolName:          req.ToolName,
		TenantID:          req.TenantID,
		UserID:            req.UserID,
		Parameters:        req.Parameters,
		CredentialID:      req.CredentialID,
		ConversationID:    req.ConversationID,
		AppID:             req.AppID,
		MessageID:         req.MessageID,
		InvokeFrom:        ToolInvokeFromAgent,
		RuntimeParameters: map[string]interface{}{"user_scope": req.UserScope},
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
