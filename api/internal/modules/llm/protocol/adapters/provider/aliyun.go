package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
)

// AliyunAdapter Aliyun DashScope adapter
type AliyunAdapter struct {
	config     *adapter.AdapterConfig
	httpClient *adapter.HTTPClient
	baseURL    string
}

// NewAliyunAdapter creates an Aliyun adapter
func NewAliyunAdapter(config *adapter.AdapterConfig) (*AliyunAdapter, error) {
	if config == nil {
		return nil, adapter.ErrInvalidConfig
	}
	if config.APIKey == "" {
		return nil, fmt.Errorf("%w: API key is required", adapter.ErrInvalidConfig)
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com/api/v1"
	}
	// Remove trailing slash if present to avoid double slashes in constructed URLs
	baseURL = strings.TrimSuffix(baseURL, "/")

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second // Longer timeout for image generation
	}

	return &AliyunAdapter{
		config:     config,
		httpClient: adapter.NewHTTPClientFromConfig(config, timeout, 3),
		baseURL:    baseURL,
	}, nil
}

func (a *AliyunAdapter) openAICompatibleBaseURL() string {
	baseURL := a.baseURL
	if strings.Contains(baseURL, "/compatible-mode/v1") {
		return baseURL
	}
	if strings.Contains(baseURL, "/compatible-api/v1") {
		return strings.Replace(baseURL, "/compatible-api/v1", "/compatible-mode/v1", 1)
	}
	if strings.Contains(baseURL, "/api/v1") {
		return strings.Replace(baseURL, "/api/v1", "/compatible-mode/v1", 1)
	}
	if strings.Contains(baseURL, "/api/") && strings.Contains(baseURL, "dashscope.aliyuncs.com") {
		return strings.Replace(baseURL, "/api/", "/compatible-mode/", 1)
	}
	return baseURL
}

func (a *AliyunAdapter) nativeBaseURL() string {
	baseURL := a.baseURL
	if strings.Contains(baseURL, "/compatible-mode/v1") {
		return strings.Replace(baseURL, "/compatible-mode/v1", "/api/v1", 1)
	}
	if strings.Contains(baseURL, "/compatible-api/v1") {
		return strings.Replace(baseURL, "/compatible-api/v1", "/api/v1", 1)
	}
	return baseURL
}

func (a *AliyunAdapter) anthropicMessagesBaseURL() string {
	baseURL := a.baseURL
	if strings.Contains(baseURL, "/apps/anthropic/v1") {
		return baseURL
	}
	if strings.Contains(baseURL, "/compatible-mode/v1") {
		return strings.Replace(baseURL, "/compatible-mode/v1", "/apps/anthropic/v1", 1)
	}
	if strings.Contains(baseURL, "/compatible-api/v1") {
		return strings.Replace(baseURL, "/compatible-api/v1", "/apps/anthropic/v1", 1)
	}
	if strings.Contains(baseURL, "/api/v1") {
		return strings.Replace(baseURL, "/api/v1", "/apps/anthropic/v1", 1)
	}
	return baseURL
}

func (a *AliyunAdapter) compatibleRerankBaseURL() string {
	baseURL := a.baseURL
	if strings.Contains(baseURL, "/compatible-api/v1") {
		return baseURL
	}
	if strings.Contains(baseURL, "/compatible-mode/v1") {
		return strings.Replace(baseURL, "/compatible-mode/v1", "/compatible-api/v1", 1)
	}
	if strings.Contains(baseURL, "/api/v1") {
		return strings.Replace(baseURL, "/api/v1", "/compatible-api/v1", 1)
	}
	return baseURL
}
func (a *AliyunAdapter) openAICompatibleAdapter() (*OpenAIAdapter, error) {
	return newOpenAIAdapterWithOverrides(a.config, a.openAICompatibleBaseURL())
}

// ChatCompletion executes chat completion request.
func (a *AliyunAdapter) ChatCompletion(ctx context.Context, request *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	payload, endpoint, err := a.buildAliyunChatPayload(request, false)
	if err != nil {
		return nil, err
	}

	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "POST", endpoint, a.aliyunJSONHeaders(), payload)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if statusCode != 200 {
		return nil, a.handleError(statusCode, respBody)
	}

	return parseAliyunChatResponse(respBody, request.Model)
}

// ChatCompletionStream executes streaming chat completion request.
func (a *AliyunAdapter) ChatCompletionStream(ctx context.Context, request *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	payload, endpoint, err := a.buildAliyunChatPayload(request, true)
	if err != nil {
		return nil, err
	}

	resp, err := a.httpClient.DoStreamRequest(ctx, "POST", endpoint, a.aliyunSSEHeaders(), payload)
	if err != nil {
		return nil, fmt.Errorf("stream request failed: %w", err)
	}
	if aliyunStreamDebugEnabled() {
		imageCount, dataURLImageCount := aliyunPayloadImageSummary(payload)
		logger.InfoContext(ctx, "aliyun stream opened",
			zap.String("model", request.Model),
			zap.String("endpoint", endpoint),
			zap.String("content_type", resp.Header.Get("Content-Type")),
			zap.Bool("multimodal", strings.Contains(endpoint, "/multimodal-generation/")),
			zap.Int("image_count", imageCount),
			zap.Int("data_url_image_count", dataURLImageCount),
		)
	}

	respChan := make(chan adapter.StreamResponse, 10)
	dataChan := make(chan string, 10)
	errChan := make(chan error, 1)

	go adapter.ParseSSE(resp.Body, dataChan, errChan)

	go func() {
		defer close(respChan)
		defer func() {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}()

		var lastUsage *adapter.Usage
		chunkIndex := 0
		for {
			select {
			case <-ctx.Done():
				respChan <- adapter.StreamResponse{Error: ctx.Err(), Done: true, Usage: lastUsage}
				return
			case err := <-errChan:
				if err != nil {
					if aliyunStreamDebugEnabled() {
						logger.WarnContext(ctx, "aliyun stream parser error", zap.Error(err))
					}
					respChan <- adapter.StreamResponse{Error: err, Done: true, Usage: lastUsage}
				}
				return
			case data, ok := <-dataChan:
				if !ok {
					if aliyunStreamDebugEnabled() {
						logger.InfoContext(ctx, "aliyun stream data channel closed", zap.String("model", request.Model), zap.Int("chunk_count", chunkIndex), zap.Bool("has_usage", lastUsage != nil))
					}
					respChan <- adapter.StreamResponse{Done: true, Usage: lastUsage}
					return
				}

				chunkIndex++
				if aliyunStreamDebugEnabled() {
					logger.InfoContext(ctx, "aliyun stream raw data", zap.String("model", request.Model), zap.Int("chunk_index", chunkIndex), zap.String("data", aliyunDebugSnippet(data, 1000)))
				}
				streamResp, err := parseAliyunChatStreamResponse([]byte(data), request.Model)
				if err != nil {
					respChan <- adapter.StreamResponse{Error: err, Done: true, Usage: lastUsage}
					return
				}
				if streamResp.Usage != nil {
					lastUsage = streamResp.Usage
				}
				if aliyunStreamDebugEnabled() {
					logger.InfoContext(ctx, "aliyun stream parsed chunk",
						zap.String("model", request.Model),
						zap.Int("chunk_index", chunkIndex),
						zap.Int("choices", len(streamResp.Choices)),
						zap.Int("text_len", aliyunStreamResponseTextLen(streamResp)),
						zap.Bool("has_usage", streamResp.Usage != nil),
					)
				}
				respChan <- *streamResp
			}
		}
	}()

	return respChan, nil
}

func (a *AliyunAdapter) aliyunJSONHeaders() map[string]string {
	return map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", a.config.APIKey),
		"Content-Type":  "application/json",
		"Accept":        "application/json",
	}
}

func (a *AliyunAdapter) aliyunSSEHeaders() map[string]string {
	headers := a.aliyunJSONHeaders()
	headers["Accept"] = "text/event-stream"
	headers["X-DashScope-SSE"] = "enable"
	return headers
}

func aliyunStreamDebugEnabled() bool {
	return strings.TrimSpace(os.Getenv("ZGI_DEBUG_ALIYUN_STREAM")) == "1"
}

func aliyunDebugSnippet(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

func aliyunStreamResponseTextLen(resp *adapter.StreamResponse) int {
	if resp == nil || len(resp.Choices) == 0 {
		return 0
	}
	text, _ := resp.Choices[0].Delta.Content.(string)
	return len(text)
}

func aliyunPayloadImageSummary(payload map[string]interface{}) (int, int) {
	input, ok := payload["input"].(map[string]interface{})
	if !ok {
		return 0, 0
	}
	messages, ok := input["messages"].([]map[string]interface{})
	if !ok {
		return 0, 0
	}
	imageCount := 0
	dataURLImageCount := 0
	for _, message := range messages {
		content, ok := message["content"].([]map[string]interface{})
		if !ok {
			continue
		}
		for _, part := range content {
			image, ok := part["image"].(string)
			if !ok || strings.TrimSpace(image) == "" {
				continue
			}
			imageCount++
			if strings.HasPrefix(strings.ToLower(image), "data:image/") {
				dataURLImageCount++
			}
		}
	}
	return imageCount, dataURLImageCount
}
func (a *AliyunAdapter) buildAliyunChatPayload(request *adapter.ChatRequest, stream bool) (map[string]interface{}, string, error) {
	if request == nil {
		return nil, "", fmt.Errorf("%w: request is required", adapter.ErrInvalidRequest)
	}
	if strings.TrimSpace(request.Model) == "" {
		return nil, "", fmt.Errorf("%w: model is required", adapter.ErrInvalidRequest)
	}

	hasImage, err := aliyunMessagesHaveImage(request.Messages)
	if err != nil {
		return nil, "", err
	}
	useMultimodal := hasImage || isAliyunMultimodalChatModel(request.Model)
	messages, err := buildAliyunChatMessages(request.Messages, useMultimodal)
	if err != nil {
		return nil, "", err
	}

	parameters := map[string]interface{}{"result_format": "message"}
	if request.Temperature != nil {
		parameters["temperature"] = *request.Temperature
	}
	if request.TopP != nil {
		parameters["top_p"] = *request.TopP
	}
	if request.MaxTokens != nil {
		parameters["max_tokens"] = *request.MaxTokens
	}
	if request.PresencePenalty != nil {
		parameters["presence_penalty"] = *request.PresencePenalty
	}
	if request.FrequencyPenalty != nil {
		parameters["frequency_penalty"] = *request.FrequencyPenalty
	}
	if request.Seed != nil {
		parameters["seed"] = *request.Seed
	}
	if len(request.Stop) > 0 {
		parameters["stop"] = request.Stop
	}
	if request.ResponseFormat != nil {
		parameters["response_format"] = request.ResponseFormat
	}
	if len(request.Tools) > 0 {
		parameters["tools"] = request.Tools
		if request.ToolChoice != nil {
			parameters["tool_choice"] = request.ToolChoice
		}
	}
	if stream {
		parameters["incremental_output"] = true
		parameters["stream_options"] = map[string]interface{}{"include_usage": true}
	}
	for k, v := range request.AdditionalParameters {
		parameters[k] = v
	}

	payload := map[string]interface{}{
		"model": request.Model,
		"input": map[string]interface{}{
			"messages": messages,
		},
		"parameters": parameters,
	}
	if stream {
		payload["stream"] = true
	}

	endpointPath := "/services/aigc/text-generation/generation"
	if useMultimodal {
		endpointPath = "/services/aigc/multimodal-generation/generation"
	}
	return payload, a.nativeBaseURL() + endpointPath, nil
}

func aliyunMessagesHaveImage(messages []adapter.Message) (bool, error) {
	for _, message := range messages {
		hasImage, err := aliyunContentHasImage(message.Content)
		if err != nil {
			return false, err
		}
		if hasImage {
			return true, nil
		}
	}
	return false, nil
}

func aliyunContentHasImage(content interface{}) (bool, error) {
	switch value := content.(type) {
	case nil, string:
		return false, nil
	case []adapter.MessageContentPart:
		for _, part := range value {
			if part.Type == "image_url" {
				return true, nil
			}
			if part.Type != "text" {
				return false, fmt.Errorf("%w: unsupported qwen content part type %q", adapter.ErrInvalidRequest, part.Type)
			}
		}
		return false, nil
	case []interface{}:
		for _, item := range value {
			hasImage, err := aliyunMapContentHasImage(item)
			if err != nil || hasImage {
				return hasImage, err
			}
		}
		return false, nil
	default:
		return false, fmt.Errorf("%w: unsupported qwen message content type %T", adapter.ErrInvalidRequest, content)
	}
}

func aliyunMapContentHasImage(item interface{}) (bool, error) {
	part, ok := item.(map[string]interface{})
	if !ok {
		return false, fmt.Errorf("%w: unsupported qwen content part type %T", adapter.ErrInvalidRequest, item)
	}
	if _, ok := part["image"]; ok {
		return true, nil
	}
	partType, _ := part["type"].(string)
	switch partType {
	case "", "text":
		return false, nil
	case "image_url":
		return true, nil
	default:
		return false, fmt.Errorf("%w: unsupported qwen content part type %q", adapter.ErrInvalidRequest, partType)
	}
}

func buildAliyunChatMessages(messages []adapter.Message, multimodal bool) ([]map[string]interface{}, error) {
	out := make([]map[string]interface{}, 0, len(messages))
	for _, message := range messages {
		content, err := buildAliyunMessageContent(message.Content, multimodal)
		if err != nil {
			return nil, err
		}
		next := map[string]interface{}{
			"role":    message.Role,
			"content": content,
		}
		if message.FunctionCall != nil {
			next["function_call"] = message.FunctionCall
		}
		if len(message.ToolCalls) > 0 {
			next["tool_calls"] = message.ToolCalls
		}
		if strings.TrimSpace(message.ToolCallID) != "" {
			next["tool_call_id"] = message.ToolCallID
		}
		out = append(out, next)
	}
	return out, nil
}

func buildAliyunMessageContent(content interface{}, multimodal bool) (interface{}, error) {
	switch value := content.(type) {
	case string:
		if multimodal {
			return []map[string]interface{}{{"text": value}}, nil
		}
		return value, nil
	case []adapter.MessageContentPart:
		parts, hasImage, err := buildAliyunPartsFromTypedContent(value)
		if err != nil {
			return nil, err
		}
		if multimodal || hasImage {
			return parts, nil
		}
		return joinAliyunTextParts(parts), nil
	case []interface{}:
		parts, hasImage, err := buildAliyunPartsFromInterfaceContent(value)
		if err != nil {
			return nil, err
		}
		if multimodal || hasImage {
			return parts, nil
		}
		return joinAliyunTextParts(parts), nil
	case nil:
		return "", nil
	default:
		return nil, fmt.Errorf("%w: unsupported qwen message content type %T", adapter.ErrInvalidRequest, content)
	}
}

func buildAliyunPartsFromTypedContent(content []adapter.MessageContentPart) ([]map[string]interface{}, bool, error) {
	parts := make([]map[string]interface{}, 0, len(content))
	hasImage := false
	for _, part := range content {
		switch part.Type {
		case "text":
			parts = append(parts, map[string]interface{}{"text": part.Text})
		case "image_url":
			if part.ImageURL == nil || strings.TrimSpace(part.ImageURL.URL) == "" {
				return nil, false, fmt.Errorf("%w: qwen image_url content requires url", adapter.ErrInvalidRequest)
			}
			image, err := normalizeAliyunImageReference(part.ImageURL.URL)
			if err != nil {
				return nil, false, err
			}
			hasImage = true
			parts = append(parts, map[string]interface{}{"image": image})
		default:
			return nil, false, fmt.Errorf("%w: unsupported qwen content part type %q", adapter.ErrInvalidRequest, part.Type)
		}
	}
	return parts, hasImage, nil
}

func buildAliyunPartsFromInterfaceContent(content []interface{}) ([]map[string]interface{}, bool, error) {
	parts := make([]map[string]interface{}, 0, len(content))
	hasImage := false
	for _, item := range content {
		part, ok := item.(map[string]interface{})
		if !ok {
			return nil, false, fmt.Errorf("%w: unsupported qwen content part type %T", adapter.ErrInvalidRequest, item)
		}
		converted, image, err := buildAliyunPartFromMap(part)
		if err != nil {
			return nil, false, err
		}
		if image {
			hasImage = true
		}
		parts = append(parts, converted)
	}
	return parts, hasImage, nil
}

func buildAliyunPartFromMap(part map[string]interface{}) (map[string]interface{}, bool, error) {
	if text, ok := part["text"].(string); ok {
		return map[string]interface{}{"text": text}, false, nil
	}
	if image, ok := part["image"].(string); ok {
		normalized, err := normalizeAliyunImageReference(image)
		if err != nil {
			return nil, false, err
		}
		return map[string]interface{}{"image": normalized}, true, nil
	}

	partType, _ := part["type"].(string)
	switch partType {
	case "text":
		text, _ := part["text"].(string)
		return map[string]interface{}{"text": text}, false, nil
	case "image_url":
		imageURL, ok := part["image_url"].(map[string]interface{})
		if !ok {
			return nil, false, fmt.Errorf("%w: qwen image_url content requires image_url object", adapter.ErrInvalidRequest)
		}
		url, _ := imageURL["url"].(string)
		normalized, err := normalizeAliyunImageReference(url)
		if err != nil {
			return nil, false, err
		}
		return map[string]interface{}{"image": normalized}, true, nil
	default:
		return nil, false, fmt.Errorf("%w: unsupported qwen content part type %q", adapter.ErrInvalidRequest, partType)
	}
}

func normalizeAliyunImageReference(raw string) (string, error) {
	image := strings.TrimSpace(raw)
	if image == "" {
		return "", fmt.Errorf("%w: qwen image content requires url", adapter.ErrInvalidRequest)
	}
	lower := strings.ToLower(image)
	if strings.HasPrefix(lower, "data:image/") {
		return image, nil
	}
	parsed, err := url.Parse(image)
	if err != nil || parsed.Scheme == "" {
		return "", fmt.Errorf("%w: qwen multimodal image must be a public http url or data url", adapter.ErrInvalidRequest)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%w: qwen multimodal image must be a public http url or data url", adapter.ErrInvalidRequest)
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" || isLocalAliyunImageHost(host) {
		return "", fmt.Errorf("%w: qwen multimodal image must be a public http url or data url", adapter.ErrInvalidRequest)
	}
	return image, nil
}

func isLocalAliyunImageHost(host string) bool {
	host = strings.ToLower(strings.Trim(host, "[]"))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}

func joinAliyunTextParts(parts []map[string]interface{}) string {
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if text, ok := part["text"].(string); ok && text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n")
}

func isAliyunMultimodalChatModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return isAliyunQwen36ChatModel(model) || strings.Contains(model, "-vl") || strings.Contains(model, "qvq") || strings.Contains(model, "omni")
}

func isAliyunQwen36ChatModel(model string) bool {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "qwen3.6-plus", "qwen3.6-flash":
		return true
	default:
		return false
	}
}

type aliyunChatResponse struct {
	RequestID string `json:"request_id"`
	Output    struct {
		Text    string `json:"text"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Role             string                `json:"role"`
				Content          interface{}           `json:"content"`
				ReasoningContent string                `json:"reasoning_content,omitempty"`
				FunctionCall     *adapter.FunctionCall `json:"function_call,omitempty"`
				ToolCalls        []adapter.ToolCall    `json:"tool_calls,omitempty"`
				ToolCallID       string                `json:"tool_call_id,omitempty"`
			} `json:"message"`
			Delta struct {
				Role             string                `json:"role"`
				Content          interface{}           `json:"content"`
				ReasoningContent string                `json:"reasoning_content,omitempty"`
				FunctionCall     *adapter.FunctionCall `json:"function_call,omitempty"`
				ToolCalls        []adapter.ToolCall    `json:"tool_calls,omitempty"`
				ToolCallID       string                `json:"tool_call_id,omitempty"`
			} `json:"delta"`
		} `json:"choices"`
	} `json:"output"`
	Usage   aliyunUsage `json:"usage"`
	Code    string      `json:"code"`
	Message string      `json:"message"`
}

type aliyunUsage struct {
	InputTokens      int `json:"input_tokens"`
	OutputTokens     int `json:"output_tokens"`
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func (u aliyunUsage) toAdapterUsage() *adapter.Usage {
	promptTokens := u.InputTokens
	if promptTokens == 0 {
		promptTokens = u.PromptTokens
	}
	completionTokens := u.OutputTokens
	if completionTokens == 0 {
		completionTokens = u.CompletionTokens
	}
	totalTokens := u.TotalTokens
	if totalTokens == 0 {
		totalTokens = promptTokens + completionTokens
	}
	if promptTokens == 0 && completionTokens == 0 && totalTokens == 0 {
		return nil
	}
	return &adapter.Usage{PromptTokens: promptTokens, CompletionTokens: completionTokens, TotalTokens: totalTokens}
}

func parseAliyunChatResponse(body []byte, model string) (*adapter.ChatResponse, error) {
	var upstream aliyunChatResponse
	if err := json.Unmarshal(body, &upstream); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if upstream.Code != "" {
		return nil, fmt.Errorf("api error: %s - %s", upstream.Code, upstream.Message)
	}

	choices := make([]adapter.Choice, 0, len(upstream.Output.Choices))
	for idx, choice := range upstream.Output.Choices {
		role := strings.TrimSpace(choice.Message.Role)
		if role == "" {
			role = "assistant"
		}
		choices = append(choices, adapter.Choice{
			Index: idx,
			Message: adapter.Message{
				Role:             role,
				Content:          aliyunTextFromContent(choice.Message.Content),
				ReasoningContent: choice.Message.ReasoningContent,
				FunctionCall:     choice.Message.FunctionCall,
				ToolCalls:        choice.Message.ToolCalls,
				ToolCallID:       choice.Message.ToolCallID,
			},
			FinishReason: choice.FinishReason,
		})
	}
	if len(choices) == 0 && upstream.Output.Text != "" {
		choices = append(choices, adapter.Choice{Message: adapter.Message{Role: "assistant", Content: upstream.Output.Text}})
	}
	if len(choices) == 0 {
		return nil, fmt.Errorf("empty choices")
	}

	return &adapter.ChatResponse{
		ID:      upstream.RequestID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: choices,
		Usage:   upstream.Usage.toAdapterUsage(),
	}, nil
}

func parseAliyunChatStreamResponse(body []byte, model string) (*adapter.StreamResponse, error) {
	var upstream aliyunChatResponse
	if err := json.Unmarshal(body, &upstream); err != nil {
		return nil, fmt.Errorf("failed to parse stream data: %w", err)
	}
	if upstream.Code != "" {
		return nil, fmt.Errorf("api error: %s - %s", upstream.Code, upstream.Message)
	}

	choices := make([]adapter.StreamChoice, 0, len(upstream.Output.Choices))
	for idx, choice := range upstream.Output.Choices {
		role := strings.TrimSpace(choice.Delta.Role)
		if role == "" {
			role = strings.TrimSpace(choice.Message.Role)
		}
		if role == "" {
			role = "assistant"
		}
		content := aliyunTextFromContent(choice.Delta.Content)
		if content == "" {
			content = aliyunTextFromContent(choice.Message.Content)
		}
		choices = append(choices, adapter.StreamChoice{
			Index: idx,
			Delta: adapter.Message{
				Role:             role,
				Content:          content,
				ReasoningContent: firstNonEmptyString(choice.Delta.ReasoningContent, choice.Message.ReasoningContent),
				FunctionCall:     firstNonNilFunctionCall(choice.Delta.FunctionCall, choice.Message.FunctionCall),
				ToolCalls:        firstNonEmptyToolCalls(choice.Delta.ToolCalls, choice.Message.ToolCalls),
				ToolCallID:       firstNonEmptyString(choice.Delta.ToolCallID, choice.Message.ToolCallID),
			},
			FinishReason: choice.FinishReason,
		})
	}
	if len(choices) == 0 && upstream.Output.Text != "" {
		choices = append(choices, adapter.StreamChoice{
			Index: 0,
			Delta: adapter.Message{
				Role:    "assistant",
				Content: upstream.Output.Text,
			},
		})
	}

	return &adapter.StreamResponse{
		ID:      upstream.RequestID,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: choices,
		Usage:   upstream.Usage.toAdapterUsage(),
	}, nil
}

func firstNonNilFunctionCall(values ...*adapter.FunctionCall) *adapter.FunctionCall {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func firstNonEmptyToolCalls(values ...[]adapter.ToolCall) []adapter.ToolCall {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func aliyunTextFromContent(content interface{}) string {
	switch value := content.(type) {
	case nil:
		return ""
	case string:
		return value
	case []interface{}:
		texts := make([]string, 0, len(value))
		for _, item := range value {
			part, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if text, ok := part["text"].(string); ok && text != "" {
				texts = append(texts, text)
			}
		}
		return strings.Join(texts, "\n")
	default:
		return fmt.Sprintf("%v", content)
	}
}

func buildAliyunEmbeddingsPayload(request *adapter.EmbeddingsRequest) (map[string]interface{}, error) {
	if request == nil {
		return nil, fmt.Errorf("%w: request is required", adapter.ErrInvalidRequest)
	}
	if strings.TrimSpace(request.Model) == "" {
		return nil, fmt.Errorf("%w: model is required", adapter.ErrInvalidRequest)
	}
	texts, err := normalizeAliyunEmbeddingInput(request.Input)
	if err != nil {
		return nil, err
	}

	if isAliyunMultimodalEmbeddingModel(request.Model) {
		contents := make([]map[string]interface{}, 0, len(texts))
		for _, text := range texts {
			contents = append(contents, map[string]interface{}{"text": text})
		}
		payload := map[string]interface{}{
			"model": request.Model,
			"input": map[string]interface{}{
				"contents": contents,
			},
		}
		parameters := map[string]interface{}{}
		if request.Dimensions > 0 {
			parameters["dimension"] = request.Dimensions
		}
		if len(parameters) > 0 {
			payload["parameters"] = parameters
		}
		return payload, nil
	}

	payload := map[string]interface{}{
		"model": request.Model,
		"input": map[string]interface{}{
			"texts": texts,
		},
	}
	parameters := map[string]interface{}{}
	if request.InputType != "" {
		parameters["text_type"] = request.InputType
	}
	if request.Dimensions > 0 {
		parameters["dimension"] = request.Dimensions
	}
	if len(parameters) > 0 {
		payload["parameters"] = parameters
	}
	return payload, nil
}

func isAliyunMultimodalEmbeddingModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(model, "vl-embedding") ||
		strings.Contains(model, "vision") ||
		strings.Contains(model, "multimodal-embedding")
}
func normalizeAliyunEmbeddingInput(input interface{}) ([]string, error) {
	switch value := input.(type) {
	case string:
		return []string{value}, nil
	case []string:
		return value, nil
	case []interface{}:
		texts := make([]string, 0, len(value))
		for _, item := range value {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%w: aliyun embeddings input must contain strings", adapter.ErrInvalidRequest)
			}
			texts = append(texts, text)
		}
		return texts, nil
	default:
		return nil, fmt.Errorf("%w: unsupported aliyun embeddings input type %T", adapter.ErrInvalidRequest, input)
	}
}

type aliyunEmbeddingsResponse struct {
	Output struct {
		Embeddings []struct {
			Embedding []float32 `json:"embedding"`
			TextIndex *int      `json:"text_index"`
			Index     *int      `json:"index"`
		} `json:"embeddings"`
	} `json:"output"`
	Usage   aliyunUsage `json:"usage"`
	Code    string      `json:"code"`
	Message string      `json:"message"`
}

func parseAliyunEmbeddingsResponse(body []byte, model string) (*adapter.EmbeddingsResponse, error) {
	var upstream aliyunEmbeddingsResponse
	if err := json.Unmarshal(body, &upstream); err != nil {
		return nil, fmt.Errorf("failed to parse embeddings response: %w", err)
	}
	if upstream.Code != "" {
		return nil, fmt.Errorf("api error: %s - %s", upstream.Code, upstream.Message)
	}
	items := make([]adapter.Embedding, 0, len(upstream.Output.Embeddings))
	for i, item := range upstream.Output.Embeddings {
		index := i
		if item.TextIndex != nil {
			index = *item.TextIndex
		} else if item.Index != nil {
			index = *item.Index
		}
		items = append(items, adapter.Embedding{
			Object:    "embedding",
			Embedding: item.Embedding,
			Index:     index,
		})
	}
	usage := adapter.Usage{}
	if u := upstream.Usage.toAdapterUsage(); u != nil {
		usage = *u
	}
	return &adapter.EmbeddingsResponse{
		Object: "list",
		Data:   items,
		Model:  model,
		Usage:  usage,
	}, nil
}

// CreateResponse executes response creation request
func (a *AliyunAdapter) CreateResponse(ctx context.Context, request *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, fmt.Errorf("CreateResponse not implemented for Aliyun adapter")
}

func (a *AliyunAdapter) CreateResponseRaw(ctx context.Context, request *adapter.RawResponseRequest) (*adapter.RawResponse, error) {
	openaiAdapter, err := a.openAICompatibleAdapter()
	if err != nil {
		return nil, err
	}
	return rawOpenAIResponseRequest(ctx, a.httpClient, a.openAICompatibleBaseURL(), openaiAdapter.buildHeaders(), request, openaiAdapter.handleError)
}

func (a *AliyunAdapter) CreateResponseStream(ctx context.Context, request *adapter.RawResponseRequest) (<-chan adapter.RawStreamEvent, error) {
	openaiAdapter, err := a.openAICompatibleAdapter()
	if err != nil {
		return nil, err
	}
	return rawOpenAIResponseStream(ctx, a.httpClient, a.openAICompatibleBaseURL(), openaiAdapter.buildHeaders(), request)
}

func (a *AliyunAdapter) CreateAnthropicMessage(ctx context.Context, request *adapter.AnthropicMessageRequest) (*adapter.RawResponse, error) {
	openaiAdapter, err := a.openAICompatibleAdapter()
	if err != nil {
		return nil, err
	}
	return rawAnthropicMessageRequest(ctx, a.httpClient, a.anthropicMessagesBaseURL(), buildAnthropicRawHeaders(a.config, request.Headers), request, openaiAdapter.handleError)
}

func (a *AliyunAdapter) CreateAnthropicMessageStream(ctx context.Context, request *adapter.AnthropicMessageRequest) (<-chan adapter.RawStreamEvent, error) {
	return rawAnthropicMessageStream(ctx, a.httpClient, a.anthropicMessagesBaseURL(), buildAnthropicRawHeaders(a.config, request.Headers), request)
}

// CreateEmbeddings executes embeddings creation request
func (a *AliyunAdapter) CreateEmbeddings(ctx context.Context, request *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	openaiAdapter, err := a.openAICompatibleAdapter()
	if err != nil {
		return nil, err
	}
	return openaiAdapter.CreateEmbeddings(ctx, request)
}

// CreateImage executes image generation request (Wanx)
func (a *AliyunAdapter) CreateImage(ctx context.Context, request *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	// Handle URL compatibility: if user configured OpenAI-compatible URL, switch to native API URL for image generation
	// https://dashscope.aliyuncs.com/compatible-mode/v1 -> https://dashscope.aliyuncs.com/api/v1
	baseURL := a.nativeBaseURL()

	// Dispatch to Qwen Image models (new API) if model name starts with "qwen-image"
	if strings.HasPrefix(request.Model, "qwen-image") {
		return a.createQwenImage(ctx, baseURL, request)
	}

	url := fmt.Sprintf("%s/services/aigc/text2image/image-synthesis", baseURL)
	headers := map[string]string{
		"Authorization":     fmt.Sprintf("Bearer %s", a.config.APIKey),
		"X-DashScope-Async": "enable",
		"Content-Type":      "application/json",
	}

	// Format size: 1024x1024 -> 1024*1024
	size := request.Size
	if strings.Contains(size, "x") {
		size = strings.ReplaceAll(size, "x", "*")
	}

	// Build payload for Wanx
	// Ref: https://help.aliyun.com/zh/dashscope/developer-reference/api-details-9
	payload := map[string]interface{}{
		"model": request.Model,
		"input": map[string]interface{}{
			"prompt": request.Prompt,
		},
		"parameters": map[string]interface{}{
			"size": size,
			"n":    1, // Wanx usually generates 1 image per task, but let's check N
		},
	}

	if request.N != nil && *request.N > 1 {
		payload["parameters"].(map[string]interface{})["n"] = *request.N
	}

	// Add style if present
	if request.Style != "" {
		payload["parameters"].(map[string]interface{})["style"] = request.Style
	}

	// Add additional parameters
	for k, v := range request.AdditionalParameters {
		payload["parameters"].(map[string]interface{})[k] = v
	}

	// Step 1: Submit Task
	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "POST", url, headers, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to submit task: %w", err)
	}
	if statusCode != 200 {
		return nil, a.handleError(statusCode, respBody)
	}

	var taskResp struct {
		Output struct {
			TaskID     string `json:"task_id"`
			TaskStatus string `json:"task_status"`
		} `json:"output"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(respBody, &taskResp); err != nil {
		return nil, fmt.Errorf("failed to parse task response: %w", err)
	}

	if taskResp.Code != "" {
		return nil, fmt.Errorf("api error: %s - %s", taskResp.Code, taskResp.Message)
	}

	taskID := taskResp.Output.TaskID
	if taskID == "" {
		return nil, fmt.Errorf("no task_id returned")
	}

	// Step 2: Poll Task Status
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Wait up to timeout (e.g. 60s or from context)
	// We use the context deadline if set, or a default loop limit

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			// Check status
			statusResp, err := a.checkTaskStatus(ctx, taskID)
			if err != nil {
				return nil, err
			}

			switch statusResp.Output.TaskStatus {
			case "SUCCEEDED":
				// Convert to standard ImageResponse
				return a.convertToImageResponse(statusResp)
			case "FAILED", "CANCELED":
				return nil, fmt.Errorf("task failed with status: %s, code: %s, message: %s",
					statusResp.Output.TaskStatus, statusResp.Output.Code, statusResp.Output.Message)
			case "PENDING", "RUNNING":
				// Continue polling
				continue
			default:
				return nil, fmt.Errorf("unknown task status: %s", statusResp.Output.TaskStatus)
			}
		}
	}
}

type aliyunTaskResult struct {
	Output struct {
		TaskID     string `json:"task_id"`
		TaskStatus string `json:"task_status"`
		Code       string `json:"code"`
		Message    string `json:"message"`
		Results    []struct {
			URL string `json:"url"`
		} `json:"results"`
	} `json:"output"`
	Usage struct {
		ImageCount int `json:"image_count"`
	} `json:"usage"`
}

// checkTaskStatus checks the status of an asynchronous task
func (a *AliyunAdapter) checkTaskStatus(ctx context.Context, taskID string) (*aliyunTaskResult, error) {
	url := fmt.Sprintf("%s/tasks/%s", a.nativeBaseURL(), taskID)
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", a.config.APIKey),
	}

	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "GET", url, headers, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to check task status: %w", err)
	}
	if statusCode != 200 {
		return nil, a.handleError(statusCode, respBody)
	}

	var res aliyunTaskResult
	if err := json.Unmarshal(respBody, &res); err != nil {
		return nil, fmt.Errorf("failed to parse task status: %w", err)
	}

	return &res, nil
}

// handleError parses Aliyun API errors
func (a *AliyunAdapter) handleError(statusCode int, body []byte) error {
	if len(body) == 0 {
		return fmt.Errorf("upstream service returned status %d with empty body", statusCode)
	}

	var errResp struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
		// Some APIs return error object (OpenAI compatible or others)
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &errResp); err == nil {
		code := errResp.Code
		msg := errResp.Message
		if code == "" && errResp.Error.Code != "" {
			code = errResp.Error.Code
			msg = errResp.Error.Message
		} else if code == "" && errResp.Error.Message != "" {
			// OpenAI compatible might have empty code but message
			msg = errResp.Error.Message
			code = "API_ERROR"
		}

		if code != "" {
			// Map common errors to typed errors
			if statusCode == 401 {
				return adapter.NewAdapterError(code, msg, statusCode, adapter.ErrAuthFailed)
			}
			if statusCode == 429 {
				return adapter.NewAdapterError(code, msg, statusCode, adapter.ErrRateLimited)
			}
			if strings.Contains(strings.ToLower(msg), "balance") || strings.Contains(strings.ToLower(code), "balance") {
				return adapter.NewAdapterError(code, msg, statusCode, adapter.ErrInsufficientBalance)
			}

			return adapter.NewAdapterError(code, msg, statusCode, adapter.ErrUpstreamError)
		}
	}

	return adapter.HandleNonJSONError(statusCode, body)
}

// createQwenImage handles image generation for Qwen Image models (qwen-image-*, wan2.*)
func (a *AliyunAdapter) createQwenImage(ctx context.Context, baseURL string, request *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	url := fmt.Sprintf("%s/services/aigc/multimodal-generation/generation", baseURL)
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", a.config.APIKey),
		"Content-Type":  "application/json",
	}

	// Format size: 1024x1024 -> 1024*1024 (if not already formatted)
	size := request.Size
	if strings.Contains(size, "x") {
		size = strings.ReplaceAll(size, "x", "*")
	}

	// Build payload for Qwen/Wan2 (Chat-like format)
	// Ref: https://help.aliyun.com/zh/model-studio/developer-reference/qwen-image-edit-api
	payload := map[string]interface{}{
		"model": request.Model,
		"input": map[string]interface{}{
			"messages": []map[string]interface{}{
				{
					"role": "user",
					"content": []map[string]string{
						{"text": request.Prompt},
					},
				},
			},
		},
		"parameters": map[string]interface{}{
			"size":          size,
			"n":             1,
			"prompt_extend": true,  // Enable prompt extension by default for better results
			"watermark":     false, // Disable watermark by default
		},
	}

	if request.N != nil && *request.N > 1 {
		payload["parameters"].(map[string]interface{})["n"] = *request.N
	}

	// Add additional parameters
	for k, v := range request.AdditionalParameters {
		payload["parameters"].(map[string]interface{})[k] = v
	}

	// Set default negative prompt only if not provided by user
	params := payload["parameters"].(map[string]interface{})
	if _, ok := params["negative_prompt"]; !ok {
		// Use standard English negative prompts for better compatibility
		params["negative_prompt"] = "low resolution, low quality, bad anatomy, deformed, oversaturated, wax figure, no face details, overly smooth, AI look, messy composition, blurry text, distorted"
	}

	// Send synchronous request
	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "POST", url, headers, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to generate image: %w", err)
	}
	if statusCode != 200 {
		return nil, a.handleError(statusCode, respBody)
	}

	// Parse Qwen Image response (Chat-like)
	var qwenResp struct {
		Output struct {
			Choices []struct {
				Message struct {
					Content []struct {
						Image string `json:"image"`
						Text  string `json:"text,omitempty"`
					} `json:"content"`
					Role string `json:"role"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		} `json:"output"`
		Usage struct {
			ImageCount int `json:"image_count"`
			Height     int `json:"height"`
			Width      int `json:"width"`
		} `json:"usage"`
		RequestID string `json:"request_id"`
		Code      string `json:"code"`
		Message   string `json:"message"`
	}

	if err := json.Unmarshal(respBody, &qwenResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if qwenResp.Code != "" {
		return nil, fmt.Errorf("api error: %s - %s", qwenResp.Code, qwenResp.Message)
	}

	if len(qwenResp.Output.Choices) == 0 || len(qwenResp.Output.Choices[0].Message.Content) == 0 {
		return nil, fmt.Errorf("no image generated in response")
	}

	// Convert to standard ImageResponse
	response := &adapter.ImageResponse{
		Created: time.Now().Unix(),
		Data:    make([]adapter.ImageItem, 0),
	}

	for _, choice := range qwenResp.Output.Choices {
		for _, content := range choice.Message.Content {
			if content.Image != "" {
				response.Data = append(response.Data, adapter.ImageItem{
					URL: content.Image,
				})
			}
		}
	}

	return response, nil
}

func (a *AliyunAdapter) convertToImageResponse(res *aliyunTaskResult) (*adapter.ImageResponse, error) {
	response := &adapter.ImageResponse{
		Created: time.Now().Unix(),
		Data:    make([]adapter.ImageItem, 0, len(res.Output.Results)),
	}

	for _, item := range res.Output.Results {
		response.Data = append(response.Data, adapter.ImageItem{
			URL: item.URL,
		})
	}

	return response, nil
}

// Rerank executes rerank request
func (a *AliyunAdapter) Rerank(ctx context.Context, request *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	if request == nil {
		return nil, fmt.Errorf("%w: request is required", adapter.ErrInvalidRequest)
	}
	if strings.TrimSpace(request.Model) == "" {
		return nil, fmt.Errorf("%w: model is required", adapter.ErrInvalidRequest)
	}
	if strings.TrimSpace(request.Query) == "" {
		return nil, fmt.Errorf("%w: query is required", adapter.ErrInvalidRequest)
	}
	if request.Documents == nil {
		return nil, fmt.Errorf("%w: documents are required", adapter.ErrInvalidRequest)
	}

	url := a.aliyunRerankURL(request.Model)
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", a.config.APIKey),
		"Content-Type":  "application/json",
		"Accept":        "application/json",
	}

	payload, err := a.buildAliyunRerankPayload(request)
	if err != nil {
		return nil, err
	}

	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "POST", url, headers, payload)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if statusCode != 200 {
		return nil, a.handleError(statusCode, respBody)
	}

	var upstreamResp aliyunRerankResponse
	if err := json.Unmarshal(respBody, &upstreamResp); err != nil {
		return nil, fmt.Errorf("failed to parse rerank response: %w", err)
	}

	results := upstreamResp.Results
	if len(results) == 0 {
		results = upstreamResp.Output.Results
	}

	response := &adapter.RerankResponse{
		ID:      upstreamResp.ID,
		Object:  "list",
		Model:   request.Model,
		Results: make([]adapter.RerankResult, 0, len(results)),
	}

	for _, result := range results {
		item := adapter.RerankResult{
			Index:          result.Index,
			RelevanceScore: result.RelevanceScore,
			Document:       result.Document,
		}

		switch doc := result.Document.(type) {
		case string:
			item.Text = doc
		case map[string]interface{}:
			if text, ok := doc["text"].(string); ok {
				item.Text = text
			}
		}

		response.Results = append(response.Results, item)
	}

	if upstreamResp.Usage.TotalTokens > 0 {
		response.Usage = &adapter.Usage{
			TotalTokens: upstreamResp.Usage.TotalTokens,
		}
	}

	return response, nil
}

// ListModels gets available model list
func (a *AliyunAdapter) ListModels(ctx context.Context, apiKey string) ([]adapter.Model, error) {
	if strings.TrimSpace(apiKey) == "" {
		apiKey = a.config.APIKey
	}

	url := fmt.Sprintf("%s/models", a.openAICompatibleBaseURL())
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", apiKey),
		"Accept":        "application/json",
	}

	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "GET", url, headers, nil)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	switch statusCode {
	case 200:
	case 404, 405, 501:
		return aliyunDocumentedModelCatalog(), nil
	default:
		return nil, a.handleError(statusCode, respBody)
	}

	var response struct {
		Data []struct {
			ID      string `json:"id"`
			Created int64  `json:"created"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse models response: %w", err)
	}

	models := make([]adapter.Model, 0, len(response.Data))
	for _, model := range response.Data {
		models = append(models, aliyunModelFromID(model.ID, model.Created, model.OwnedBy))
	}

	return models, nil
}

// GetBalance queries API Key balance
func (a *AliyunAdapter) GetBalance(ctx context.Context, apiKey string) (*adapter.Balance, error) {
	return nil, fmt.Errorf("GetBalance not implemented for Aliyun adapter")
}

// ValidateConfig validates configuration
func (a *AliyunAdapter) ValidateConfig(config *adapter.AdapterConfig) error {
	if config.APIKey == "" {
		return fmt.Errorf("api key is required for Aliyun adapter")
	}
	return nil
}

// GetProviderInfo gets provider metadata
func (a *AliyunAdapter) GetProviderInfo() *adapter.ProviderInfo {
	return &adapter.ProviderInfo{
		Name:         "dashscope",
		Type:         "aliyun",
		DisplayName:  "Aliyun DashScope",
		Description:  "Aliyun DashScope (Wanx, Qwen)",
		BaseURL:      a.baseURL,
		Capabilities: []string{"chat", "stream", "embedding", "image", "rerank", "model_listing"},
		Version:      "v1",
	}
}

type aliyunRerankResponse struct {
	ID      string               `json:"id"`
	Model   string               `json:"model"`
	Results []aliyunRerankResult `json:"results"`
	Output  struct {
		Results []aliyunRerankResult `json:"results"`
	} `json:"output"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

type aliyunRerankResult struct {
	Index          int         `json:"index"`
	RelevanceScore float64     `json:"relevance_score"`
	Document       interface{} `json:"document"`
}

func (a *AliyunAdapter) aliyunRerankURL(model string) string {
	if strings.EqualFold(strings.TrimSpace(model), "qwen3-rerank") {
		return fmt.Sprintf("%s/reranks", a.compatibleRerankBaseURL())
	}
	return fmt.Sprintf("%s/services/rerank/text-rerank/text-rerank", a.nativeBaseURL())
}

func (a *AliyunAdapter) buildAliyunRerankPayload(request *adapter.RerankRequest) (map[string]interface{}, error) {
	documents, err := normalizeAliyunRerankDocuments(request.Documents)
	if err != nil {
		return nil, err
	}

	if strings.EqualFold(strings.TrimSpace(request.Model), "qwen3-rerank") {
		payload := map[string]interface{}{
			"model":     request.Model,
			"query":     request.Query,
			"documents": documents,
		}
		if request.TopN != nil {
			payload["top_n"] = *request.TopN
		}
		return payload, nil
	}

	payload := map[string]interface{}{
		"model": request.Model,
		"input": map[string]interface{}{
			"query":     request.Query,
			"documents": documents,
		},
	}

	parameters := map[string]interface{}{}
	if request.TopN != nil {
		parameters["top_n"] = *request.TopN
	}
	if request.ReturnDocuments != nil {
		parameters["return_documents"] = *request.ReturnDocuments
	}
	if len(parameters) > 0 {
		payload["parameters"] = parameters
	}

	return payload, nil
}

func normalizeAliyunRerankDocuments(documents interface{}) (interface{}, error) {
	switch value := documents.(type) {
	case []string:
		return value, nil
	case []interface{}:
		return value, nil
	case []map[string]interface{}:
		items := make([]interface{}, 0, len(value))
		for _, item := range value {
			items = append(items, item)
		}
		return items, nil
	default:
		return nil, fmt.Errorf("%w: invalid documents type %T", adapter.ErrInvalidRequest, documents)
	}
}

func aliyunDocumentedModelCatalog() []adapter.Model {
	ids := []string{
		"qwen-max",
		"qwen-plus",
		"qwen-turbo",
		"qwen-long",
		"qwen-vl-max",
		"qwen-vl-plus",
		"qvq-max",
		"qwq-plus",
		"qwq-32b",
		"qwen-omni-turbo",
		"qwen-coder-plus",
		"qwen-coder-turbo",
		"qwen-image-2.0-pro",
		"qwen-image-2.0",
		"qwen-image-max",
		"qwen-image-plus",
		"qwen-image-edit-max",
		"qwen-image-edit-plus",
		"qwen-image-edit",
		"wanx2.1-t2i-plus",
		"wanx2.1-t2i-turbo",
		"wanx2.0-t2i-turbo",
		"wanx-v1",
		"wanx2.1-imageedit",
		"text-embedding-v4",
		"text-embedding-v3",
		"text-embedding-v2",
		"text-embedding-v1",
		"qwen3-vl-embedding",
		"qwen2.5-vl-embedding",
		"tongyi-embedding-vision-plus",
		"tongyi-embedding-vision-flash",
		"multimodal-embedding-v1",
		"qwen3-rerank",
		"gte-rerank-v2",
		"qwen3-vl-rerank",
	}

	models := make([]adapter.Model, 0, len(ids))
	for _, id := range ids {
		models = append(models, aliyunModelFromID(id, 0, "dashscope"))
	}
	return models
}

func aliyunModelFromID(id string, created int64, ownedBy string) adapter.Model {
	capabilities := aliyunCapabilitiesForModel(id)
	modelType := ""
	if len(capabilities) > 0 {
		modelType = capabilities[0]
		if modelType == "stream" && len(capabilities) > 1 {
			modelType = capabilities[1]
		}
	}

	if ownedBy == "" {
		ownedBy = "dashscope"
	}

	return adapter.Model{
		ID:           id,
		Name:         id,
		Type:         modelType,
		Created:      created,
		OwnedBy:      ownedBy,
		Capabilities: capabilities,
	}
}

func aliyunCapabilitiesForModel(id string) []string {
	modelID := strings.ToLower(strings.TrimSpace(id))
	if modelID == "" {
		return nil
	}

	switch {
	case strings.Contains(modelID, "rerank"):
		return []string{"rerank"}
	case strings.Contains(modelID, "embedding"):
		return []string{"embedding"}
	case strings.HasPrefix(modelID, "qwen-image"),
		strings.HasPrefix(modelID, "wanx"),
		strings.Contains(modelID, "imageedit"):
		return []string{"image"}
	case strings.HasPrefix(modelID, "qwen"),
		strings.HasPrefix(modelID, "qwq"),
		strings.HasPrefix(modelID, "qvq"):
		return []string{"chat", "stream"}
	default:
		return nil
	}
}
