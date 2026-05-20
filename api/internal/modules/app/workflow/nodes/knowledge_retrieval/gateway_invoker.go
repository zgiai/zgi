package knowledgeretrieval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	llmclient "github.com/zgiai/ginext/internal/modules/llm/client"
	llmadapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

type gatewayLLMInvoker struct {
	client             llmclient.LLMClient
	organizationID     string
	workspaceID        string
	billingSubjectType string
}

// NewGatewayLLMInvoker builds an invoker backed by the LLM client.
// The client should be obtained from the DI container (ServiceContainer.GetLLMClient()).
// workspaceID is a required billing subject for workflow-scoped LLM calls.
// organizationID can be empty and will be resolved by llm client from app context.
func NewGatewayLLMInvoker(client llmclient.LLMClient, organizationID string, workspaceID string, billingSubjectType string) (llmInvoker, error) {
	if client == nil {
		return nil, nil
	}
	orgID := strings.TrimSpace(organizationID)
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required for workflow gateway invoker")
	}
	return &gatewayLLMInvoker{
		client:             client,
		organizationID:     orgID,
		workspaceID:        workspaceID,
		billingSubjectType: strings.TrimSpace(billingSubjectType),
	}, nil
}

func (g *gatewayLLMInvoker) Invoke(ctx context.Context, accountID, appID, appType string, req *InvokeRequest) (*InvokeResult, error) {
	if g == nil || g.client == nil {
		return nil, ErrInvokerNotConfigured
	}

	chatReq := buildChatRequest(req)
	appCtx := &llmclient.AppContext{
		AppID:              appID,
		AppType:            appType,
		AccountID:          accountID,
		OrganizationID:     g.organizationID,
		WorkspaceID:        g.workspaceID,
		BillingSubjectType: g.billingSubjectType,
	}
	resp, err := g.client.AppChat(ctx, appCtx, chatReq)
	if err != nil {
		return nil, err
	}
	if resp == nil || len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty chat response")
	}

	text, _ := resp.Choices[0].Message.Content.(string)
	finish := resp.Choices[0].FinishReason

	return &InvokeResult{
		Text:       text,
		Finish:     finish,
		RawChoices: resp.Choices,
	}, nil
}

func buildChatRequest(req *InvokeRequest) *llmadapter.ChatRequest {
	msgs := make([]llmadapter.Message, 0, len(req.Messages))
	for _, m := range req.Messages {
		msgs = append(msgs, llmadapter.Message{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	out := &llmadapter.ChatRequest{
		Model:    req.ModelSlug,
		Messages: msgs,
		Stop:     req.Stop,
		User:     req.UserID,
		Stream:   false,
	}

	if len(req.Tools) > 0 {
		out.Tools = req.Tools
		out.ToolChoice = req.ToolChoice
	}

	params := req.Parameters
	if v, ok := toFloatPointer(params["temperature"]); ok {
		out.Temperature = v
	}
	if v, ok := toFloatPointer(params["top_p"]); ok {
		out.TopP = v
	}
	if v, ok := toIntPointer(params["max_tokens"]); ok {
		out.MaxTokens = v
	}
	if v, ok := toFloatPointer(params["presence_penalty"]); ok {
		out.PresencePenalty = v
	}
	if v, ok := toFloatPointer(params["frequency_penalty"]); ok {
		out.FrequencyPenalty = v
	}

	if rf, ok := params["response_format"].(string); ok && rf != "" {
		rfLower := strings.ToLower(rf)
		respFmt := &llmadapter.ResponseFormat{Type: rfLower}
		if rfLower == "json_schema" {
			if schema := parseSchema(params["json_schema"]); schema != nil {
				respFmt.Schema = schema
			}
		}
		out.ResponseFormat = respFmt
	}

	return out
}

func parseSchema(raw any) map[string]any {
	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(v), &m); err == nil {
			return m
		}
	case map[string]any:
		return v
	}
	return nil
}

func toFloatPointer(value any) (*float64, bool) {
	if f, ok := toFloat(value); ok {
		return &f, true
	}
	return nil, false
}

func toFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case json.Number:
		if f, err := v.Float64(); err == nil {
			return f, true
		}
		return 0, false
	default:
		return 0, false
	}
}

func toIntPointer(value any) (*int, bool) {
	if i, ok := toInt(value); ok {
		return &i, true
	}
	return nil, false
}

func toInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case int32:
		return int(v), true
	case float64:
		return int(v), true
	case float32:
		return int(v), true
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i), true
		}
		return 0, false
	default:
		return 0, false
	}
}
