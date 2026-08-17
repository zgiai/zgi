package toolprovider

import (
	"context"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

type Tool struct {
	entity      tools.ToolEntity
	action      integrations.ActionDefinition
	executor    *integrations.Executor
	integration string
	runtime     *tools.ToolRuntime
}

func (t *Tool) GetEntity() tools.ToolEntity { return t.entity }

func (t *Tool) GetProviderType() tools.ToolProviderType { return tools.ToolProviderTypeConnector }

func (t *Tool) GetTenantID() string {
	if t.runtime == nil {
		return ""
	}
	return t.runtime.TenantID
}

func (t *Tool) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolInvokeMessage, error) {
	if t.runtime == nil {
		return nil, fmt.Errorf("connector tool runtime is not configured")
	}
	connectionID := strings.TrimSpace(t.runtime.ConnectionID)
	if connectionID == "" {
		connectionID = runtimeIntegrationConnectionID(t.runtime.RuntimeParameters, t.integration)
	}
	request := integrations.ActionRequest{
		OrganizationID: t.runtime.TenantID,
		WorkspaceID:    runtimeString(t.runtime.RuntimeParameters, "workspace_id"),
		UserID:         strings.TrimSpace(userID),
		AgentID:        runtimeString(t.runtime.RuntimeParameters, "agent_id"),
		ConversationID: optionalString(conversationID),
		AppID:          optionalString(appID),
		MessageID:      optionalString(messageID),
		ConnectionID:   connectionID,
		InvokeFrom:     t.runtime.InvokeFrom,
		IntegrationID:  t.integration,
		ActionID:       t.action.ID,
		Input:          toolParameters,
	}
	if verifier := skills.AgentBindingVerifierFromRuntimeParameters(t.runtime.RuntimeParameters); verifier != nil {
		accessMode := "read"
		if t.action.Effect != toolgovernance.EffectRead {
			accessMode = "write"
		}
		request.VerifyAgentConnection = func(ctx context.Context, authorization integrations.AgentConnectionAuthorizationRequest) (bool, error) {
			return verifier(ctx, skills.AgentBindingCheck{
				BindingType:      "integration_connection",
				ResourceID:       authorization.ConnectionID,
				ParentResourceID: authorization.IntegrationID,
				AccessMode:       accessMode,
				ActionID:         authorization.ActionID,
			})
		}
	}
	result, err := t.executor.Execute(ctx, request)
	if err != nil {
		return nil, err
	}
	return []tools.ToolInvokeMessage{{Type: tools.ToolInvokeMessageTypeJSON, Data: result.Output}}, nil
}

func runtimeIntegrationConnectionID(values map[string]interface{}, integrationID string) string {
	if len(values) == 0 {
		return ""
	}
	if value := runtimeString(values, "integration_connection_id"); value != "" {
		return value
	}
	raw := values["integration_connection_ids"]
	switch typed := raw.(type) {
	case map[string]string:
		return strings.TrimSpace(typed[strings.TrimSpace(integrationID)])
	case map[string]interface{}:
		return strings.TrimSpace(fmt.Sprint(typed[strings.TrimSpace(integrationID)]))
	default:
		return ""
	}
}

func (t *Tool) GetRuntimeParameters(context.Context, *string, *string, *string) ([]tools.ToolParameter, error) {
	return nil, nil
}

func (t *Tool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	clone := *t
	clone.runtime = runtime
	return &clone
}

func (t *Tool) ValidateCredentials(context.Context, map[string]interface{}) error { return nil }

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func runtimeString(values map[string]interface{}, key string) string {
	if len(values) == 0 {
		return ""
	}
	value, _ := values[key].(string)
	return strings.TrimSpace(value)
}
