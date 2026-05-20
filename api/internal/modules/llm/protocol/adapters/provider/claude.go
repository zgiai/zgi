package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	anthropicparam "github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/shopspring/decimal"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

const claudeModelListPageLimit = 1000

// ClaudeAdapter Claude adapter implementation
type ClaudeAdapter struct {
	config     *adapter.AdapterConfig
	httpClient *adapter.HTTPClient
	baseURL    string
}

// NewClaudeAdapter creates a Claude adapter
func NewClaudeAdapter(config *adapter.AdapterConfig) (*ClaudeAdapter, error) {
	if err := validateClaudeConfig(config); err != nil {
		return nil, err
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	maxRetries := config.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	return &ClaudeAdapter{
		config:     config,
		httpClient: adapter.NewHTTPClient(timeout, maxRetries),
		baseURL:    baseURL,
	}, nil
}

func validateClaudeConfig(config *adapter.AdapterConfig) error {
	if config == nil {
		return adapter.ErrInvalidConfig
	}
	if config.APIKey == "" {
		return fmt.Errorf("%w: API key is required", adapter.ErrInvalidConfig)
	}
	return nil
}

// Claude API request/response structures
type claudeMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string or []claudeContentBlock
}

type claudeContentBlock struct {
	Type      string                 `json:"type"` // "text", "image", "tool_use", "tool_result"
	Text      string                 `json:"text,omitempty"`
	Source    *claudeImageSource     `json:"source,omitempty"`
	ID        string                 `json:"id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Input     map[string]interface{} `json:"input,omitempty"`
	ToolUseID string                 `json:"tool_use_id,omitempty"`
	Content   interface{}            `json:"content,omitempty"`
}

type claudeImageSource struct {
	Type      string `json:"type"`       // "base64" or "url"
	MediaType string `json:"media_type"` // "image/jpeg", "image/png", "image/gif", "image/webp"
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

type claudeTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

type claudeRequest struct {
	Model         string          `json:"model"`
	Messages      []claudeMessage `json:"messages"`
	MaxTokens     int             `json:"max_tokens"`
	System        string          `json:"system,omitempty"`
	Temperature   *float64        `json:"temperature,omitempty"`
	TopP          *float64        `json:"top_p,omitempty"`
	TopK          *int            `json:"top_k,omitempty"`
	StopSequences []string        `json:"stop_sequences,omitempty"`
	Stream        bool            `json:"stream,omitempty"`
	Tools         []claudeTool    `json:"tools,omitempty"`
	ToolChoice    interface{}     `json:"tool_choice,omitempty"`
	Metadata      *claudeMetadata `json:"metadata,omitempty"`
}

type claudeMetadata struct {
	UserID string `json:"user_id,omitempty"`
}

type claudeResponse struct {
	ID           string               `json:"id"`
	Type         string               `json:"type"` // "message"
	Role         string               `json:"role"` // "assistant"
	Content      []claudeContentBlock `json:"content"`
	Model        string               `json:"model"`
	StopReason   string               `json:"stop_reason"` // "end_turn", "max_tokens", "stop_sequence", "tool_use"
	StopSequence string               `json:"stop_sequence,omitempty"`
	Usage        claudeUsage          `json:"usage"`
}

type claudeUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

type claudeStreamResponse struct {
	Type         string              `json:"type"` // "message_start", "content_block_start", "content_block_delta", "content_block_stop", "message_delta", "message_stop"
	Message      *claudeResponse     `json:"message,omitempty"`
	Index        int                 `json:"index,omitempty"`
	ContentBlock *claudeContentBlock `json:"content_block,omitempty"`
	Delta        *claudeStreamDelta  `json:"delta,omitempty"`
	Usage        *claudeUsage        `json:"usage,omitempty"`
}

type claudeStreamDelta struct {
	Type         string `json:"type,omitempty"` // "text_delta", "input_json_delta"
	Text         string `json:"text,omitempty"`
	PartialJSON  string `json:"partial_json,omitempty"`
	StopReason   string `json:"stop_reason,omitempty"`
	StopSequence string `json:"stop_sequence,omitempty"`
}

// ChatCompletion executes chat completion request
func (a *ClaudeAdapter) ChatCompletion(ctx context.Context, request *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	claudeReq, err := a.convertToClaude(request)
	if err != nil {
		return nil, fmt.Errorf("failed to convert request: %w", err)
	}

	url := fmt.Sprintf("%s/messages", a.baseURL)
	headers := a.buildHeaders()

	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "POST", url, headers, claudeReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if statusCode != 200 {
		return nil, a.handleError(statusCode, respBody)
	}

	var claudeResp claudeResponse
	if err := json.Unmarshal(respBody, &claudeResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return a.convertFromClaude(&claudeResp), nil
}

// ChatCompletionStream executes streaming chat completion request
func (a *ClaudeAdapter) ChatCompletionStream(ctx context.Context, request *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	claudeReq, err := a.convertToClaude(request)
	if err != nil {
		return nil, fmt.Errorf("failed to convert request: %w", err)
	}
	claudeReq.Stream = true

	url := fmt.Sprintf("%s/messages", a.baseURL)
	headers := a.buildHeaders()

	resp, err := a.httpClient.DoStreamRequest(ctx, "POST", url, headers, claudeReq)
	if err != nil {
		return nil, fmt.Errorf("stream request failed: %w", err)
	}

	respChan := make(chan adapter.StreamResponse, 10)
	dataChan := make(chan string, 10)
	errChan := make(chan error, 1)

	// Start SSE parsing
	go adapter.ParseSSE(resp.Body, dataChan, errChan)

	// Process streaming data
	go func() {
		defer close(respChan)
		defer func() {
			// Drain any remaining data from response body before closing
			// This prevents connection reuse issues with stale data
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}()

		var messageID string
		var model string
		var lastUsage *adapter.Usage
		currentToolCalls := make(map[int]*adapter.ToolCall)

		for {
			select {
			case <-ctx.Done():
				respChan <- adapter.StreamResponse{
					Error: ctx.Err(),
					Done:  true,
					Usage: lastUsage,
				}
				return
			case err := <-errChan:
				if err != nil {
					respChan <- adapter.StreamResponse{
						Error: err,
						Done:  true,
						Usage: lastUsage,
					}
				}
				return
			case data, ok := <-dataChan:
				if !ok {
					// Stream ended
					respChan <- adapter.StreamResponse{
						Done:  true,
						Usage: lastUsage,
					}
					return
				}

				var streamResp claudeStreamResponse
				if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
					respChan <- adapter.StreamResponse{
						Error: fmt.Errorf("failed to parse stream data: %w", err),
						Done:  true,
						Usage: lastUsage,
					}
					return
				}

				// Handle different event types
				switch streamResp.Type {
				case "message_start":
					if streamResp.Message != nil {
						messageID = streamResp.Message.ID
						model = streamResp.Message.Model
						lastUsage = toAdapterUsage(streamResp.Message.Usage)
					}
				case "content_block_start":
					if streamResp.ContentBlock != nil && streamResp.ContentBlock.Type == "tool_use" {
						toolIndex := streamResp.Index
						toolCall := &adapter.ToolCall{
							Index: &toolIndex,
							ID:    streamResp.ContentBlock.ID,
							Type:  "function",
							Function: adapter.FunctionCall{
								Name:      streamResp.ContentBlock.Name,
								Arguments: "",
							},
						}
						currentToolCalls[streamResp.Index] = toolCall
						respChan <- adapter.StreamResponse{
							ID:      messageID,
							Object:  "chat.completion.chunk",
							Created: time.Now().Unix(),
							Model:   model,
							Choices: []adapter.StreamChoice{
								{
									Index: 0,
									Delta: adapter.Message{
										Role:      "assistant",
										ToolCalls: []adapter.ToolCall{*toolCall},
									},
								},
							},
							Usage: lastUsage,
						}
					}
				case "content_block_delta":
					if streamResp.Delta != nil {
						if streamResp.Delta.Type == "text_delta" {
							respChan <- adapter.StreamResponse{
								ID:      messageID,
								Object:  "chat.completion.chunk",
								Created: time.Now().Unix(),
								Model:   model,
								Choices: []adapter.StreamChoice{
									{
										Index: 0,
										Delta: adapter.Message{
											Role:    "assistant",
											Content: streamResp.Delta.Text,
										},
									},
								},
								Usage: lastUsage,
							}
						} else if streamResp.Delta.Type == "input_json_delta" {
							if toolCall, ok := currentToolCalls[streamResp.Index]; ok {
								toolIndex := streamResp.Index
								toolCall.Function.Arguments += streamResp.Delta.PartialJSON
								respChan <- adapter.StreamResponse{
									ID:      messageID,
									Object:  "chat.completion.chunk",
									Created: time.Now().Unix(),
									Model:   model,
									Choices: []adapter.StreamChoice{
										{
											Index: 0,
											Delta: adapter.Message{
												Role: "assistant",
												ToolCalls: []adapter.ToolCall{
													{
														Index: &toolIndex,
														ID:    toolCall.ID,
														Type:  "function",
														Function: adapter.FunctionCall{
															Name:      toolCall.Function.Name,
															Arguments: streamResp.Delta.PartialJSON,
														},
													},
												},
											},
										},
									},
									Usage: lastUsage,
								}
							}
						}
					}
				case "content_block_stop":
					// Content block completed
				case "message_delta":
					if streamResp.Usage != nil {
						lastUsage = toAdapterUsage(*streamResp.Usage)
					}
					if streamResp.Delta != nil && streamResp.Delta.StopReason != "" {
						finishReason := a.convertStopReason(streamResp.Delta.StopReason)
						respChan <- adapter.StreamResponse{
							ID:      messageID,
							Object:  "chat.completion.chunk",
							Created: time.Now().Unix(),
							Model:   model,
							Choices: []adapter.StreamChoice{
								{
									Index:        0,
									Delta:        adapter.Message{},
									FinishReason: finishReason,
								},
							},
							Usage: lastUsage,
						}
					}
				case "message_stop":
					// Message completed
					respChan <- adapter.StreamResponse{
						Done:  true,
						Usage: lastUsage,
					}
					return
				}
			}
		}
	}()

	return respChan, nil
}

// CreateResponse executes response creation request (not natively supported by Claude)
func (a *ClaudeAdapter) CreateResponse(ctx context.Context, request *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, fmt.Errorf("%w: CreateResponse is not supported by Claude API", adapter.ErrCapabilityUnsupported)
}

// CreateAnthropicMessage executes the native Anthropic Messages API without reshaping the JSON body.
func (a *ClaudeAdapter) CreateAnthropicMessage(ctx context.Context, request *adapter.AnthropicMessageRequest) (*adapter.RawResponse, error) {
	body, err := rawRequestBody(request.Body)
	if err != nil {
		return nil, err
	}

	client := anthropic.NewClient(a.anthropicClientOptions(request.Headers)...)
	var params anthropic.MessageNewParams
	anthropicparam.SetJSON(body, &params)

	var raw json.RawMessage
	if _, err := client.Messages.New(ctx, params, anthropicoption.WithResponseBodyInto(&raw)); err != nil {
		return nil, fmt.Errorf("anthropic messages request failed: %w", err)
	}

	return &adapter.RawResponse{
		Body:  raw,
		Usage: anthropicUsageFromRaw(raw, nil),
	}, nil
}

// CreateAnthropicMessageStream executes the native Anthropic Messages streaming API.
func (a *ClaudeAdapter) CreateAnthropicMessageStream(ctx context.Context, request *adapter.AnthropicMessageRequest) (<-chan adapter.RawStreamEvent, error) {
	body, err := rawRequestBody(request.Body)
	if err != nil {
		return nil, err
	}

	client := anthropic.NewClient(a.anthropicClientOptions(request.Headers)...)
	var params anthropic.MessageNewParams
	anthropicparam.SetJSON(body, &params)

	stream := client.Messages.NewStreaming(ctx, params)
	out := make(chan adapter.RawStreamEvent, 10)

	go func() {
		defer close(out)
		defer stream.Close()

		var lastUsage *adapter.Usage
		for stream.Next() {
			event := stream.Current()
			raw := json.RawMessage(event.RawJSON())
			eventType := strings.TrimSpace(event.Type)
			if eventType == "" {
				eventType = rawEventType(raw)
			}
			lastUsage = anthropicUsageFromRaw(raw, lastUsage)
			out <- adapter.RawStreamEvent{
				Event: eventType,
				Data:  raw,
				Usage: lastUsage,
			}
		}
		if err := stream.Err(); err != nil {
			out <- adapter.RawStreamEvent{Error: err, Done: true, Usage: lastUsage}
			return
		}
		out <- adapter.RawStreamEvent{Done: true, Usage: lastUsage}
	}()

	return out, nil
}

// CreateEmbeddings executes embeddings creation request (not supported by Claude)
func (a *ClaudeAdapter) CreateEmbeddings(ctx context.Context, request *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, fmt.Errorf("%w: embeddings are not supported by Claude API", adapter.ErrCapabilityUnsupported)
}

// CreateImage executes image generation request (not supported by Claude)
func (a *ClaudeAdapter) CreateImage(ctx context.Context, request *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, fmt.Errorf("%w: image generation is not supported by Claude API", adapter.ErrCapabilityUnsupported)
}

// Rerank executes rerank request (not supported by Claude)
func (a *ClaudeAdapter) Rerank(ctx context.Context, request *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, fmt.Errorf("%w: rerank is not supported by Claude API", adapter.ErrCapabilityUnsupported)
}

// ListModels gets available model list from Claude
func (a *ClaudeAdapter) ListModels(ctx context.Context, apiKey string) ([]adapter.Model, error) {
	headers := a.buildHeaders()
	if strings.TrimSpace(apiKey) != "" {
		headers["x-api-key"] = strings.TrimSpace(apiKey)
	}

	models := make([]adapter.Model, 0)
	afterID := ""

	for {
		listURL, err := a.buildModelsURL(afterID)
		if err != nil {
			return nil, err
		}

		respBody, statusCode, err := a.httpClient.DoRequest(ctx, "GET", listURL, headers, nil)
		if err != nil {
			return nil, fmt.Errorf("request failed: %w", err)
		}

		if statusCode != 200 {
			return nil, a.handleError(statusCode, respBody)
		}

		var response struct {
			Data []struct {
				ID          string `json:"id"`
				DisplayName string `json:"display_name"`
				CreatedAt   string `json:"created_at"`
				Type        string `json:"type"`
			} `json:"data"`
			HasMore bool   `json:"has_more"`
			LastID  string `json:"last_id"`
		}

		if err := json.Unmarshal(respBody, &response); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		for _, m := range response.Data {
			name := strings.TrimSpace(m.DisplayName)
			if name == "" {
				name = m.ID
			}

			model := adapter.Model{
				ID:           m.ID,
				Name:         name,
				Type:         "chat",
				Capabilities: []string{"chat", "stream", "tools"},
			}

			// Parse created_at timestamp
			if t, err := time.Parse(time.RFC3339, m.CreatedAt); err == nil {
				model.Created = t.Unix()
			}

			// Enrich model information
			a.enrichModelInfo(&model)

			models = append(models, model)
		}

		if !response.HasMore {
			return models, nil
		}
		if strings.TrimSpace(response.LastID) == "" {
			return nil, fmt.Errorf("anthropic models response has_more=true but last_id is empty")
		}

		afterID = response.LastID
	}
}

// GetBalance gets balance information (not directly supported by Claude)
func (a *ClaudeAdapter) GetBalance(ctx context.Context, apiKey string) (*adapter.Balance, error) {
	return nil, fmt.Errorf("%w: balance query is not supported by Claude API", adapter.ErrCapabilityUnsupported)
}

// ValidateConfig validates configuration
func (a *ClaudeAdapter) ValidateConfig(config *adapter.AdapterConfig) error {
	return validateClaudeConfig(config)
}

// GetProviderInfo gets provider information
func (a *ClaudeAdapter) GetProviderInfo() *adapter.ProviderInfo {
	return &adapter.ProviderInfo{
		Name:         "claude",
		Type:         "anthropic",
		DisplayName:  "Claude (Anthropic)",
		Description:  "Anthropic Claude models",
		BaseURL:      a.baseURL,
		Capabilities: []string{"chat", "stream", "tools", "model_listing"},
		Version:      "v1",
	}
}

// buildHeaders builds request headers
func (a *ClaudeAdapter) buildHeaders() map[string]string {
	headers := map[string]string{
		"x-api-key":         a.config.APIKey,
		"anthropic-version": "2023-06-01",
		"content-type":      "application/json",
	}

	// Add custom headers
	for k, v := range a.config.Headers {
		headers[k] = v
	}

	return headers
}

func (a *ClaudeAdapter) anthropicClientOptions(headers map[string]string) []anthropicoption.RequestOption {
	opts := []anthropicoption.RequestOption{
		anthropicoption.WithAPIKey(a.config.APIKey),
		anthropicoption.WithBaseURL(a.anthropicSDKBaseURL()),
		anthropicoption.WithMaxRetries(a.config.MaxRetries),
		anthropicoption.WithRequestTimeout(a.config.Timeout),
	}
	for k, v := range a.config.Headers {
		opts = append(opts, anthropicoption.WithHeader(k, v))
	}
	for k, v := range headers {
		if strings.TrimSpace(v) == "" {
			continue
		}
		opts = append(opts, anthropicoption.WithHeader(k, v))
	}
	return opts
}

func (a *ClaudeAdapter) anthropicSDKBaseURL() string {
	baseURL := strings.TrimRight(strings.TrimSpace(a.baseURL), "/")
	if strings.HasSuffix(baseURL, "/v1") {
		baseURL = strings.TrimSuffix(baseURL, "/v1")
	}
	return baseURL + "/"
}

func (a *ClaudeAdapter) buildModelsURL(afterID string) (string, error) {
	base, err := url.Parse(a.baseURL)
	if err != nil {
		return "", fmt.Errorf("parse base url: %w", err)
	}

	base.Path = strings.TrimRight(base.Path, "/") + "/models"
	query := base.Query()
	query.Set("limit", fmt.Sprintf("%d", claudeModelListPageLimit))
	if strings.TrimSpace(afterID) != "" {
		query.Set("after_id", strings.TrimSpace(afterID))
	}
	base.RawQuery = query.Encode()

	return base.String(), nil
}

// handleError handles error response
func (a *ClaudeAdapter) handleError(statusCode int, body []byte) error {
	var errResp struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &errResp); err != nil {
		return adapter.HandleNonJSONError(statusCode, body)
	}

	switch statusCode {
	case 401:
		return adapter.NewAdapterError(errResp.Error.Type, errResp.Error.Message, statusCode, adapter.ErrAuthFailed)
	case 429:
		return adapter.NewAdapterError(errResp.Error.Type, errResp.Error.Message, statusCode, adapter.ErrRateLimited)
	case 404:
		return adapter.NewAdapterError(errResp.Error.Type, errResp.Error.Message, statusCode, adapter.ErrModelNotFound)
	default:
		return adapter.NewAdapterError(errResp.Error.Type, errResp.Error.Message, statusCode, adapter.ErrUpstreamError)
	}
}

// convertToClaude converts standard request to Claude format
func (a *ClaudeAdapter) convertToClaude(request *adapter.ChatRequest) (*claudeRequest, error) {
	if request == nil {
		return nil, fmt.Errorf("%w: request is required", adapter.ErrInvalidRequest)
	}

	claudeReq := &claudeRequest{
		Model:       request.Model,
		MaxTokens:   1024, // Default value
		Temperature: request.Temperature,
		TopP:        request.TopP,
	}

	// Set max_tokens
	if request.MaxTokens != nil && *request.MaxTokens > 0 {
		claudeReq.MaxTokens = *request.MaxTokens
	}

	// Set stop sequences
	if len(request.Stop) > 0 {
		claudeReq.StopSequences = request.Stop
	}

	// Set user metadata
	if request.User != "" {
		claudeReq.Metadata = &claudeMetadata{
			UserID: request.User,
		}
	}

	// Convert messages
	systemParts := make([]string, 0)
	claudeMessages := make([]claudeMessage, 0)

	for _, msg := range request.Messages {
		normalizedRole := normalizeClaudeRole(msg)

		if normalizedRole == "system" {
			systemText, err := extractClaudeSystemText(msg.Content)
			if err != nil {
				return nil, fmt.Errorf("failed to convert system message: %w", err)
			}
			if systemText != "" {
				systemParts = append(systemParts, systemText)
			}
			continue
		}

		claudeContent, err := a.convertClaudeMessageContent(msg)
		if err != nil {
			return nil, fmt.Errorf("failed to convert %s message: %w", normalizedRole, err)
		}
		claudeMessages = append(claudeMessages, claudeMessage{
			Role:    normalizedRole,
			Content: claudeContent,
		})
	}

	claudeReq.Messages = claudeMessages
	if len(systemParts) > 0 {
		claudeReq.System = strings.Join(systemParts, "\n\n")
	}

	// Convert tools
	if len(request.Tools) > 0 {
		claudeTools := make([]claudeTool, 0)
		for _, tool := range request.Tools {
			claudeTool := claudeTool{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
			}

			// Convert parameters to input_schema
			if params, ok := tool.Function.Parameters.(map[string]interface{}); ok {
				claudeTool.InputSchema = params
			}

			claudeTools = append(claudeTools, claudeTool)
		}
		claudeReq.Tools = claudeTools
	}

	// Convert tool_choice
	if request.ToolChoice != nil {
		claudeReq.ToolChoice = convertToolChoice(request.ToolChoice)
	}

	return claudeReq, nil
}

func normalizeClaudeRole(msg adapter.Message) string {
	if msg.ToolCallID != "" || strings.EqualFold(strings.TrimSpace(msg.Role), "tool") {
		return "user"
	}

	switch strings.ToLower(strings.TrimSpace(msg.Role)) {
	case "assistant":
		return "assistant"
	case "system":
		return "system"
	default:
		return "user"
	}
}

func extractClaudeSystemText(content interface{}) (string, error) {
	switch value := content.(type) {
	case nil:
		return "", nil
	case string:
		return strings.TrimSpace(value), nil
	case []adapter.MessageContentPart:
		parts := make([]string, 0, len(value))
		for _, part := range value {
			if strings.EqualFold(part.Type, "text") && strings.TrimSpace(part.Text) != "" {
				parts = append(parts, strings.TrimSpace(part.Text))
			}
		}
		return strings.Join(parts, "\n"), nil
	case []interface{}:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			m, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if partType, _ := m["type"].(string); strings.EqualFold(partType, "text") {
				if text, ok := m["text"].(string); ok && strings.TrimSpace(text) != "" {
					parts = append(parts, strings.TrimSpace(text))
				}
			}
		}
		return strings.Join(parts, "\n"), nil
	default:
		return "", fmt.Errorf("%w: unsupported system content type %T", adapter.ErrInvalidRequest, content)
	}
}

func (a *ClaudeAdapter) convertClaudeMessageContent(msg adapter.Message) (interface{}, error) {
	if msg.ToolCallID != "" {
		content, err := convertClaudeToolResultContent(msg.Content)
		if err != nil {
			return nil, err
		}
		return []claudeContentBlock{
			{
				Type:      "tool_result",
				ToolUseID: msg.ToolCallID,
				Content:   content,
			},
		}, nil
	}

	blocks, err := convertClaudeBlocksFromContent(msg.Content)
	if err != nil {
		return nil, err
	}

	if len(msg.ToolCalls) > 0 {
		for _, tc := range msg.ToolCalls {
			blocks = append(blocks, claudeContentBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: parseJSON(tc.Function.Arguments),
			})
		}
	} else if msg.FunctionCall != nil {
		blocks = append(blocks, claudeContentBlock{
			Type:  "tool_use",
			ID:    generateID(),
			Name:  msg.FunctionCall.Name,
			Input: parseJSON(msg.FunctionCall.Arguments),
		})
	}

	if len(blocks) == 0 {
		return "", nil
	}
	if len(blocks) == 1 && blocks[0].Type == "text" {
		return blocks[0].Text, nil
	}
	return blocks, nil
}

func convertClaudeBlocksFromContent(content interface{}) ([]claudeContentBlock, error) {
	switch value := content.(type) {
	case nil:
		return nil, nil
	case string:
		text := strings.TrimSpace(value)
		if text == "" {
			return nil, nil
		}
		return []claudeContentBlock{{Type: "text", Text: text}}, nil
	case []adapter.MessageContentPart:
		blocks := make([]claudeContentBlock, 0, len(value))
		for _, part := range value {
			switch strings.ToLower(strings.TrimSpace(part.Type)) {
			case "", "text":
				if strings.TrimSpace(part.Text) != "" {
					blocks = append(blocks, claudeContentBlock{Type: "text", Text: strings.TrimSpace(part.Text)})
				}
			case "image_url":
				source, err := convertClaudeImageSource(part.ImageURL)
				if err != nil {
					return nil, err
				}
				blocks = append(blocks, claudeContentBlock{Type: "image", Source: source})
			default:
				return nil, fmt.Errorf("%w: unsupported content part type %q", adapter.ErrInvalidRequest, part.Type)
			}
		}
		return blocks, nil
	case []interface{}:
		blocks := make([]claudeContentBlock, 0, len(value))
		for _, item := range value {
			switch typed := item.(type) {
			case string:
				if strings.TrimSpace(typed) != "" {
					blocks = append(blocks, claudeContentBlock{Type: "text", Text: strings.TrimSpace(typed)})
				}
			case map[string]interface{}:
				partType, _ := typed["type"].(string)
				switch strings.ToLower(strings.TrimSpace(partType)) {
				case "", "text":
					if text, ok := typed["text"].(string); ok && strings.TrimSpace(text) != "" {
						blocks = append(blocks, claudeContentBlock{Type: "text", Text: strings.TrimSpace(text)})
					}
				case "image_url":
					imageURLValue, _ := typed["image_url"].(map[string]interface{})
					imageURL := &adapter.ImageURL{}
					if imageURLValue != nil {
						if rawURL, ok := imageURLValue["url"].(string); ok {
							imageURL.URL = rawURL
						}
						if detail, ok := imageURLValue["detail"].(string); ok {
							imageURL.Detail = detail
						}
					}
					source, err := convertClaudeImageSource(imageURL)
					if err != nil {
						return nil, err
					}
					blocks = append(blocks, claudeContentBlock{Type: "image", Source: source})
				default:
					return nil, fmt.Errorf("%w: unsupported content part type %q", adapter.ErrInvalidRequest, partType)
				}
			default:
				return nil, fmt.Errorf("%w: unsupported content item type %T", adapter.ErrInvalidRequest, item)
			}
		}
		return blocks, nil
	default:
		return nil, fmt.Errorf("%w: unsupported content type %T", adapter.ErrInvalidRequest, content)
	}
}

func convertClaudeToolResultContent(content interface{}) (interface{}, error) {
	switch value := content.(type) {
	case nil:
		return "", nil
	case string:
		return value, nil
	default:
		blocks, err := convertClaudeBlocksFromContent(value)
		if err != nil {
			return nil, err
		}
		if len(blocks) == 1 && blocks[0].Type == "text" {
			return blocks[0].Text, nil
		}
		return blocks, nil
	}
}

func convertClaudeImageSource(imageURL *adapter.ImageURL) (*claudeImageSource, error) {
	if imageURL == nil || strings.TrimSpace(imageURL.URL) == "" {
		return nil, fmt.Errorf("%w: image_url.url is required", adapter.ErrInvalidRequest)
	}

	raw := strings.TrimSpace(imageURL.URL)
	if strings.HasPrefix(raw, "data:") {
		return parseClaudeDataURL(raw)
	}

	return &claudeImageSource{
		Type: "url",
		URL:  raw,
	}, nil
}

func parseClaudeDataURL(raw string) (*claudeImageSource, error) {
	const prefix = "data:"
	if !strings.HasPrefix(raw, prefix) {
		return nil, fmt.Errorf("%w: invalid data URL", adapter.ErrInvalidRequest)
	}

	comma := strings.Index(raw, ",")
	if comma == -1 {
		return nil, fmt.Errorf("%w: invalid data URL", adapter.ErrInvalidRequest)
	}

	meta := strings.TrimPrefix(raw[:comma], prefix)
	data := raw[comma+1:]
	metaParts := strings.Split(meta, ";")
	if len(metaParts) < 2 || metaParts[len(metaParts)-1] != "base64" {
		return nil, fmt.Errorf("%w: Claude image content requires base64 data URLs", adapter.ErrInvalidRequest)
	}

	mediaType := strings.TrimSpace(metaParts[0])
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}

	return &claudeImageSource{
		Type:      "base64",
		MediaType: mediaType,
		Data:      data,
	}, nil
}

// convertFromClaude converts Claude response to standard format
func (a *ClaudeAdapter) convertFromClaude(response *claudeResponse) *adapter.ChatResponse {
	message := adapter.Message{
		Role: response.Role,
	}

	// Extract content and tool calls
	var textContent strings.Builder
	var toolCalls []adapter.ToolCall

	for _, block := range response.Content {
		switch block.Type {
		case "text":
			textContent.WriteString(block.Text)
		case "tool_use":
			toolCall := adapter.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: adapter.FunctionCall{
					Name:      block.Name,
					Arguments: toJSONString(block.Input),
				},
			}
			toolCalls = append(toolCalls, toolCall)
		}
	}

	message.Content = textContent.String()
	if len(toolCalls) > 0 {
		message.ToolCalls = toolCalls
	}

	return &adapter.ChatResponse{
		ID:      response.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   response.Model,
		Choices: []adapter.Choice{
			{
				Index:        0,
				Message:      message,
				FinishReason: a.convertStopReason(response.StopReason),
			},
		},
		Usage: toAdapterUsage(response.Usage),
	}
}

// convertStopReason converts Claude stop reason to OpenAI format
func (a *ClaudeAdapter) convertStopReason(stopReason string) string {
	switch stopReason {
	case "end_turn":
		return "stop"
	case "max_tokens":
		return "length"
	case "stop_sequence":
		return "stop"
	case "tool_use":
		return "tool_calls"
	case "pause_turn":
		return "stop"
	case "refusal":
		return "content_filter"
	case "model_context_window_exceeded":
		return "length"
	case "compaction":
		return "length"
	default:
		return stopReason
	}
}

func toAdapterUsage(usage claudeUsage) *adapter.Usage {
	if usage.InputTokens == 0 && usage.OutputTokens == 0 {
		return nil
	}
	return &adapter.Usage{
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
		TotalTokens:      usage.InputTokens + usage.OutputTokens,
	}
}

// enrichModelInfo enriches model information with context length and pricing
func (a *ClaudeAdapter) enrichModelInfo(model *adapter.Model) {
	modelID := strings.ToLower(model.ID)

	// Set architecture
	model.Architecture = &adapter.ModelArchitecture{
		Modality:         "text",
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"text"},
		InstructType:     "chat",
	}

	// Set context length and pricing based on model
	switch {
	case strings.Contains(modelID, "opus-4"):
		model.ContextLength = 200000
		model.Pricing = &adapter.Pricing{
			Prompt:     decimal.NewFromFloat(15.0),
			Completion: decimal.NewFromFloat(75.0),
		}
	case strings.Contains(modelID, "sonnet-4"):
		model.ContextLength = 200000
		model.Pricing = &adapter.Pricing{
			Prompt:     decimal.NewFromFloat(3.0),
			Completion: decimal.NewFromFloat(15.0),
		}
	case strings.Contains(modelID, "haiku-4"):
		model.ContextLength = 200000
		model.Pricing = &adapter.Pricing{
			Prompt:     decimal.NewFromFloat(0.8),
			Completion: decimal.NewFromFloat(4.0),
		}
	case strings.Contains(modelID, "3-opus"):
		model.ContextLength = 200000
		model.Pricing = &adapter.Pricing{
			Prompt:     decimal.NewFromFloat(15.0),
			Completion: decimal.NewFromFloat(75.0),
		}
	case strings.Contains(modelID, "3-5-sonnet") || strings.Contains(modelID, "3-7-sonnet"):
		model.ContextLength = 200000
		model.Pricing = &adapter.Pricing{
			Prompt:     decimal.NewFromFloat(3.0),
			Completion: decimal.NewFromFloat(15.0),
		}
	case strings.Contains(modelID, "3-5-haiku"):
		model.ContextLength = 200000
		model.Pricing = &adapter.Pricing{
			Prompt:     decimal.NewFromFloat(0.8),
			Completion: decimal.NewFromFloat(4.0),
		}
	case strings.Contains(modelID, "3-haiku"):
		model.ContextLength = 200000
		model.Pricing = &adapter.Pricing{
			Prompt:     decimal.NewFromFloat(0.25),
			Completion: decimal.NewFromFloat(1.25),
		}
	default:
		model.ContextLength = 200000
		model.Pricing = &adapter.Pricing{
			Prompt:     decimal.NewFromFloat(3.0),
			Completion: decimal.NewFromFloat(15.0),
		}
	}
}

// Helper functions

func parseJSON(jsonStr string) map[string]interface{} {
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return make(map[string]interface{})
	}
	return result
}

func toJSONString(data interface{}) string {
	bytes, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	return string(bytes)
}

func generateID() string {
	return fmt.Sprintf("call_%d", time.Now().UnixNano())
}

func convertToolChoice(toolChoice interface{}) interface{} {
	switch v := toolChoice.(type) {
	case string:
		switch v {
		case "auto":
			return map[string]string{"type": "auto"}
		case "none":
			return map[string]string{"type": "none"}
		case "required":
			return map[string]string{"type": "any"}
		}
	case map[string]interface{}:
		if t, ok := v["type"].(string); ok && t == "function" {
			if name, ok := v["function"].(map[string]interface{})["name"].(string); ok {
				return map[string]interface{}{
					"type": "tool",
					"name": name,
				}
			}
		}
	}
	return map[string]string{"type": "auto"}
}
