package parameterextractor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

// LLMInvoker defines the interface for invoking LLM services
type LLMInvoker interface {
	// Invoke performs a non-streaming LLM invocation
	Invoke(ctx context.Context, accountID, appID, appType string, req *InvokeRequest) (*InvokeResult, error)

	// InvokeStream performs a streaming LLM invocation
	InvokeStream(ctx context.Context, accountID, appID, appType string, req *InvokeRequest) (<-chan *ResultChunk, <-chan error, error)
}

// InvokeRequest represents a request to invoke the LLM
type InvokeRequest struct {
	ModelSlug  string          // Model name (without provider prefix)
	Messages   []PromptMessage // Prompt messages list
	Parameters map[string]any  // Model parameters (temperature, max_tokens, etc.)
	Stop       []string        // Stop sequences
	UserID     string          // User ID for tracking
	Stream     bool            // Whether to use streaming output
}

// InvokeResult represents the result from LLM invocation
type InvokeResult struct {
	Text   string     // Response text
	Finish string     // Finish reason
	Usage  *UsageInfo // Usage information
}

// ResultChunk represents a chunk of streaming result
type ResultChunk struct {
	Delta *ResultChunkDelta // Incremental content
	Usage *UsageInfo        // Usage information (only in final chunk)
}

// ResultChunkDelta represents the incremental content in a streaming chunk
type ResultChunkDelta struct {
	Content      string // Incremental text content
	FinishReason string // Finish reason (only in final chunk)
}

// UsageInfo represents token usage information
type UsageInfo struct {
	PromptTokens     int    // Number of prompt tokens
	CompletionTokens int    // Number of completion tokens
	TotalTokens      int    // Total tokens used
	TotalPrice       string // Total price in decimal string format
	Currency         string // Currency code (e.g., "USD")
}

// gatewayLLMInvoker is the implementation backed by the LLM client
type gatewayLLMInvoker struct {
	client             llmclient.LLMClient
	organizationID     string
	workspaceID        string
	billingSubjectType string
}

// NewGatewayLLMInvoker creates a new Gateway LLM invoker
// The client should be obtained from the DI container (ServiceContainer.GetLLMClient()).
// workspaceID is a required billing subject for workflow-scoped LLM calls.
// organizationID can be empty and will be resolved by llm client from app context.
func NewGatewayLLMInvoker(client llmclient.LLMClient, organizationID string, workspaceID string, billingSubjectType string) (LLMInvoker, error) {
	if client == nil {
		return nil, nil
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required for workflow gateway invoker")
	}
	return &gatewayLLMInvoker{
		client:             client,
		organizationID:     strings.TrimSpace(organizationID),
		workspaceID:        workspaceID,
		billingSubjectType: strings.TrimSpace(billingSubjectType),
	}, nil
}

// Invoke performs a non-streaming LLM invocation
func (g *gatewayLLMInvoker) Invoke(ctx context.Context, accountID, appID, appType string, req *InvokeRequest) (*InvokeResult, error) {
	if g == nil || g.client == nil {
		return nil, fmt.Errorf("LLM invoker not configured")
	}

	// Build chat request
	chatReq := g.buildChatRequest(req)
	chatReq.Stream = false

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
		return nil, fmt.Errorf("LLM invocation failed: %w", err)
	}

	if resp == nil {
		return nil, fmt.Errorf("empty chat response: response is nil")
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty chat response: no choices returned")
	}

	// Extract result
	text, _ := resp.Choices[0].Message.Content.(string)
	result := &InvokeResult{
		Text:   text,
		Finish: resp.Choices[0].FinishReason,
	}

	// Extract usage information
	if resp.Usage != nil {
		result.Usage = &UsageInfo{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
			TotalPrice:       "0.00", // Price calculation will be done at a higher level
			Currency:         "USD",
		}
	}

	return result, nil
}

// InvokeStream performs a streaming LLM invocation
func (g *gatewayLLMInvoker) InvokeStream(ctx context.Context, accountID, appID, appType string, req *InvokeRequest) (<-chan *ResultChunk, <-chan error, error) {
	if g == nil || g.client == nil {
		return nil, nil, fmt.Errorf("LLM invoker not configured")
	}

	// Build chat request
	chatReq := g.buildChatRequest(req)
	chatReq.Stream = true

	appCtx := &llmclient.AppContext{
		AppID:              appID,
		AppType:            appType,
		AccountID:          accountID,
		OrganizationID:     g.organizationID,
		WorkspaceID:        g.workspaceID,
		BillingSubjectType: g.billingSubjectType,
	}

	streamChan, err := g.client.AppChatStream(ctx, appCtx, chatReq)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start streaming: %w", err)
	}

	// Create output channels
	out := make(chan *ResultChunk, 100)
	errChan := make(chan error, 1)

	// Process stream in goroutine
	go func() {
		defer close(out)
		defer close(errChan)

		for resp := range streamChan {
			// Check for errors
			if resp.Error != nil {
				errChan <- resp.Error
				return
			}

			// Process each choice
			for _, choice := range resp.Choices {
				content, _ := choice.Delta.Content.(string)

				if content != "" {
					out <- &ResultChunk{
						Delta: &ResultChunkDelta{
							Content:      content,
							FinishReason: choice.FinishReason,
						},
					}
				}
			}

			// Note: Usage information is not available in streaming responses
			// It will need to be calculated at a higher level if needed
		}
	}()

	return out, errChan, nil
}

// buildChatRequest converts InvokeRequest to ChatRequest for the gateway
func (g *gatewayLLMInvoker) buildChatRequest(req *InvokeRequest) *llmadapter.ChatRequest {
	// Convert messages
	// Support both simple text content and multi-part content (for vision)
	msgs := make([]llmadapter.Message, 0, len(req.Messages))
	for _, m := range req.Messages {
		// Convert content to string
		var content string
		if str, ok := m.Content.(string); ok {
			content = str
		} else {
			// For structured content (vision), convert to JSON string
			contentBytes, _ := json.Marshal(m.Content)
			content = string(contentBytes)
		}

		msgs = append(msgs, llmadapter.Message{
			Role:    string(m.Role),
			Content: content,
		})
	}

	// Build base request
	chatReq := &llmadapter.ChatRequest{
		Model:    req.ModelSlug,
		Messages: msgs,
		Stop:     req.Stop,
		User:     req.UserID,
	}

	// Map parameters
	params := req.Parameters
	if v, ok := toFloatPointer(params["temperature"]); ok {
		chatReq.Temperature = v
	}
	if v, ok := toFloatPointer(params["top_p"]); ok {
		chatReq.TopP = v
	}
	if v, ok := toIntPointer(params["max_tokens"]); ok {
		chatReq.MaxTokens = v
	}
	if v, ok := toFloatPointer(params["presence_penalty"]); ok {
		chatReq.PresencePenalty = v
	}
	if v, ok := toFloatPointer(params["frequency_penalty"]); ok {
		chatReq.FrequencyPenalty = v
	}

	// Handle response format
	if rf, ok := params["response_format"].(string); ok && rf != "" {
		rfLower := strings.ToLower(rf)
		respFmt := &llmadapter.ResponseFormat{Type: rfLower}
		if rfLower == "json_schema" {
			if schema := parseSchema(params["json_schema"]); schema != nil {
				respFmt.Schema = schema
			}
		}
		chatReq.ResponseFormat = respFmt
	}

	return chatReq
}

// Helper functions for type conversion

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
