package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	llmClient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	"github.com/zgiai/zgi/api/internal/modules/llm/multimodal"
	llmAdapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// gatewayLLMInvoker is the default implementation backed by the LLM client.
type gatewayLLMInvoker struct {
	client             llmClient.LLMClient
	organizationID     string
	workspaceID        string
	billingSubjectType string
}

// NewGatewayLLMInvoker constructs an LLMInvoker backed by the LLM client.
// The client should be obtained from the DI container (ServiceContainer.GetLLMClient()).
// workspaceID is a required billing subject for workflow-scoped LLM calls.
// organizationID can be empty and will be resolved by llm client from app context.
func NewGatewayLLMInvoker(client llmClient.LLMClient, organizationID string, workspaceID string, billingSubjectType string) (LLMInvoker, error) {
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

func (g *gatewayLLMInvoker) InvokeStream(ctx context.Context, accountID, appID, appType string, req *LLMInvokeRequest) (<-chan *ResultChunk, <-chan error, error) {
	if g == nil || g.client == nil {
		return nil, nil, fmt.Errorf("llm invoker not configured")
	}
	llmReq, collectStructured := buildChatRequest(req)
	var structuredBuffer strings.Builder

	appCtx := &llmClient.AppContext{
		AppID:              appID,
		AppType:            appType,
		AccountID:          accountID,
		OrganizationID:     g.organizationID,
		WorkspaceID:        g.workspaceID,
		BillingSubjectType: g.billingSubjectType,
		SessionID:          req.SessionID,
		ConversationID:     req.ConversationID,
		WorkflowID:         req.WorkflowID,
		WorkflowRunID:      req.WorkflowRunID,
		NodeID:             req.NodeID,
		NodeType:           req.NodeType,
	}

	streamChan, err := g.client.AppChatStream(ctx, appCtx, llmReq)
	if err != nil {
		return nil, nil, err
	}

	out := make(chan *ResultChunk, 100)
	errChan := make(chan error, 1)

	var lastFinishReason string
	var lastUsage *shared.LLMUsage // Track usage from stream response

	go func() {
		defer close(out)
		defer close(errChan)

		for resp := range streamChan {
			if resp.Error != nil {
				errChan <- resp.Error
				return
			}

			// Capture usage from stream response (usually in last chunk)
			if resp.Usage != nil && resp.Usage.TotalTokens > 0 {
				lastUsage = &shared.LLMUsage{
					PromptTokens:     resp.Usage.PromptTokens,
					CompletionTokens: resp.Usage.CompletionTokens,
					TotalTokens:      resp.Usage.TotalTokens,
				}
			}

			for _, choice := range resp.Choices {
				content, _ := choice.Delta.Content.(string)
				if content != "" {
					if collectStructured {
						structuredBuffer.WriteString(content)
					}

					out <- &ResultChunk{
						Model:          req.ModelSlug,
						PromptMessages: req.Messages,
						Delta: &ResultChunkDelta{
							Message: &PromptMessage{
								Role:    PromptMessageRoleAssistant,
								Content: content,
							},
							FinishReason: choice.FinishReason,
						},
					}
				}
				if choice.FinishReason != "" {
					lastFinishReason = choice.FinishReason
				}
			}

			if resp.Done {
				// Send final chunk with usage data
				finalChunk := &ResultChunk{
					Model:          req.ModelSlug,
					PromptMessages: req.Messages,
					Delta: &ResultChunkDelta{
						FinishReason: lastFinishReason,
						Usage:        lastUsage,
					},
				}

				if collectStructured && structuredBuffer.Len() > 0 {
					var parsed map[string]any
					// Currently, if structured output is not supported,
					// it is ignored directly without returning an error.
					if err := json.Unmarshal([]byte(structuredBuffer.String()), &parsed); err == nil {
						finalChunk.StructuredOutput = parsed
					}
				}

				out <- finalChunk
				return
			}
		}
	}()

	return out, errChan, nil
}

func toAdapterMessages(msgs []PromptMessage) []llmAdapter.Message {
	out := make([]llmAdapter.Message, 0, len(msgs))
	for _, m := range msgs {
		switch content := m.Content.(type) {
		case string:
			// Simple text content
			out = append(out, llmAdapter.Message{
				Role:    string(m.Role),
				Content: content,
			})
		case []PromptMessageContent:
			// Multimodal content - convert to OpenAI format
			parts := make([]llmAdapter.MessageContentPart, 0, len(content))
			for _, c := range content {
				part := convertToAdapterContentPart(c)
				parts = append(parts, part)
			}
			out = append(out, llmAdapter.Message{
				Role:    string(m.Role),
				Content: parts,
			})
		default:
			// Fallback: try to convert to string
			if content != nil {
				out = append(out, llmAdapter.Message{
					Role:    string(m.Role),
					Content: fmt.Sprintf("%v", content),
				})
			}
		}
	}
	return out
}

// convertToAdapterContentPart converts PromptMessageContent to OpenAI-compatible MessageContentPart
func convertToAdapterContentPart(c PromptMessageContent) llmAdapter.MessageContentPart {
	switch c.Type {
	case PromptMessageContentTypeText:
		return multimodal.BuildTextPart(c.Data)
	case PromptMessageContentTypeImage:
		detail := string(c.Detail)
		if c.URL != "" {
			return multimodal.BuildImageURLPart(c.URL, detail)
		}
		if c.Base64 != "" {
			return multimodal.BuildImageDataPart(c.Base64, c.MimeType, detail)
		}
		return multimodal.BuildImageURLPart("", detail)
	default:
		// Fallback to text for unsupported types
		return multimodal.BuildTextPart(c.Data)
	}
}

// buildChatRequest  LLMInvokeRequest parameters to ChatRequest required by the gateway service。
// Map only common inference parameters and response formats (response_format/json_schema)。
func buildChatRequest(req *LLMInvokeRequest) (*llmAdapter.ChatRequest, bool) {
	llmReq := &llmAdapter.ChatRequest{
		Provider: strings.TrimSpace(req.ProviderSlug),
		Model:    req.ModelSlug,
		Messages: toAdapterMessages(req.Messages),
		Stop:     req.Stop,
		Stream:   true,
		User:     req.UserID,
		// Enable usage reporting in stream response
		StreamOptions: &llmAdapter.StreamOptions{IncludeUsage: true},
	}

	params := req.Parameters
	if v, ok := toFloatPointer(params["temperature"]); ok {
		llmReq.Temperature = v
	}
	if v, ok := toFloatPointer(params["top_p"]); ok {
		llmReq.TopP = v
	}
	if v, ok := toIntPointer(params["max_tokens"]); ok {
		llmReq.MaxTokens = v
	}
	if v, ok := toFloatPointer(params["presence_penalty"]); ok {
		llmReq.PresencePenalty = v
	}
	if v, ok := toFloatPointer(params["frequency_penalty"]); ok {
		llmReq.FrequencyPenalty = v
	}

	collectStructured := req.StructuredOutput != nil
	if rf, ok := params["response_format"].(string); ok && rf != "" {
		rfLower := strings.ToLower(rf)
		respFmt := &llmAdapter.ResponseFormat{Type: rfLower}
		if rfLower == "json_schema" {
			if schema := parseSchema(params["json_schema"]); schema != nil {
				respFmt.Schema = schema
				collectStructured = true
			}
		}
		llmReq.ResponseFormat = respFmt
	} else if req.StructuredOutput != nil {
		// Compatible with the StructuredOutput convention,
		// the default request is json_object.
		llmReq.ResponseFormat = &llmAdapter.ResponseFormat{
			Type:   "json_object",
			Schema: req.StructuredOutput,
		}
		collectStructured = true
	}

	return llmReq, collectStructured
}

func buildGatewayRequestSnapshot(req *LLMInvokeRequest) map[string]any {
	if req == nil {
		return nil
	}

	llmReq, _ := buildChatRequest(req)
	snapshot := map[string]any{
		"provider": llmReq.Provider,
		"model":    llmReq.Model,
		"messages": llmReq.Messages,
		"params": map[string]any{
			"stop":              llmReq.Stop,
			"temperature":       optionalFloatValue(llmReq.Temperature),
			"top_p":             optionalFloatValue(llmReq.TopP),
			"max_tokens":        optionalIntValue(llmReq.MaxTokens),
			"presence_penalty":  optionalFloatValue(llmReq.PresencePenalty),
			"frequency_penalty": optionalFloatValue(llmReq.FrequencyPenalty),
			"response_format":   llmReq.ResponseFormat,
		},
	}

	raw, err := json.Marshal(snapshot)
	if err != nil {
		return snapshot
	}

	var normalized map[string]any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return snapshot
	}

	return normalized
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

func optionalFloatValue(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func toFloatPointer(value any) (*float64, bool) {
	if f, ok := toFloat(value); ok {
		return &f, true
	}
	return nil, false
}

func optionalIntValue(value *int) any {
	if value == nil {
		return nil
	}
	return *value
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
