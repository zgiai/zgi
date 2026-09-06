package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func rawEventType(raw json.RawMessage) string {
	var payload struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	return strings.TrimSpace(payload.Type)
}

func openAIUsageFromRaw(raw json.RawMessage) *adapter.Usage {
	if len(raw) == 0 {
		return nil
	}

	var direct struct {
		Usage openAIUsageShape `json:"usage"`
	}
	if err := json.Unmarshal(raw, &direct); err == nil {
		if usage := direct.Usage.toAdapterUsage(); usage != nil {
			return usage
		}
	}

	var event struct {
		Response struct {
			Usage openAIUsageShape `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal(raw, &event); err != nil {
		return nil
	}
	return event.Response.Usage.toAdapterUsage()
}

type openAIUsageShape struct {
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	TotalTokens        int `json:"total_tokens"`
	VideoTokens        int `json:"video_tokens"`
	TotalVideoTokens   int `json:"total_video_tokens"`
	PromptTokens       int `json:"prompt_tokens"`
	CompletionTokens   int `json:"completion_tokens"`
	InputTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	PromptTokensDetails struct {
		CachedTokens        int `json:"cached_tokens"`
		CacheCreationTokens int `json:"cache_creation_tokens"`
	} `json:"prompt_tokens_details"`
	OutputTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
	CompletionTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
}

func (u openAIUsageShape) toAdapterUsage() *adapter.Usage {
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
		totalTokens = u.VideoTokens
	}
	if totalTokens == 0 {
		totalTokens = u.TotalVideoTokens
	}
	if totalTokens == 0 {
		totalTokens = promptTokens + completionTokens
	}
	if promptTokens == 0 && completionTokens == 0 && totalTokens == 0 {
		return nil
	}
	usage := &adapter.Usage{
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
	}
	usage.CacheReadTokens = maxRawUsageInt(u.InputTokensDetails.CachedTokens, u.PromptTokensDetails.CachedTokens)
	usage.CacheWriteTokens = u.PromptTokensDetails.CacheCreationTokens
	usage.CompletionTokensDetails.ReasoningTokens = maxRawUsageInt(u.OutputTokensDetails.ReasoningTokens, u.CompletionTokensDetails.ReasoningTokens)
	usage.NormalizeCacheTokens()
	return usage
}

func maxRawUsageInt(values ...int) int {
	best := 0
	for _, value := range values {
		if value > best {
			best = value
		}
	}
	return best
}

func geminiUsageFromRaw(raw json.RawMessage, previous *adapter.Usage) *adapter.Usage {
	if len(raw) == 0 {
		return previous
	}
	var payload struct {
		Usage struct {
			PromptTokens     int `json:"promptTokenCount"`
			CompletionTokens int `json:"candidatesTokenCount"`
			TotalTokens      int `json:"totalTokenCount"`
			CachedTokens     int `json:"cachedContentTokenCount"`
			ReasoningTokens  int `json:"thoughtsTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return previous
	}
	u := payload.Usage
	if previous == nil && u.PromptTokens == 0 && u.CompletionTokens == 0 && u.TotalTokens == 0 && u.CachedTokens == 0 && u.ReasoningTokens == 0 {
		return nil
	}
	usage := &adapter.Usage{}
	if previous != nil {
		*usage = *previous
	}
	if u.PromptTokens > 0 {
		usage.PromptTokens = u.PromptTokens
	}
	if u.CompletionTokens > 0 {
		usage.CompletionTokens = u.CompletionTokens
	}
	if u.TotalTokens > 0 {
		usage.TotalTokens = u.TotalTokens
	}
	if u.CachedTokens > 0 {
		usage.CacheReadTokens = u.CachedTokens
	}
	if u.ReasoningTokens > 0 {
		usage.CompletionTokensDetails.ReasoningTokens = u.ReasoningTokens
	}
	usage.NormalizeCacheTokens()
	return usage
}

func anthropicUsageFromRaw(raw json.RawMessage, previous *adapter.Usage) *adapter.Usage {
	if len(raw) == 0 {
		return previous
	}

	usage := previous

	var direct struct {
		Usage anthropicUsageShape `json:"usage"`
	}
	if err := json.Unmarshal(raw, &direct); err == nil {
		usage = mergeAnthropicTokenUsage(usage, direct.Usage)
	}

	var event struct {
		Message struct {
			Usage anthropicUsageShape `json:"usage"`
		} `json:"message"`
	}
	if err := json.Unmarshal(raw, &event); err == nil {
		usage = mergeAnthropicTokenUsage(usage, event.Message.Usage)
	}

	return usage
}

type anthropicUsageShape struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreation            struct {
		Ephemeral5mInputTokens int `json:"ephemeral_5m_input_tokens"`
		Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens"`
	} `json:"cache_creation"`
}

func mergeAnthropicTokenUsage(previous *adapter.Usage, u anthropicUsageShape) *adapter.Usage {
	cacheWrite5mTokens := maxUsageToken(u.CacheCreation.Ephemeral5mInputTokens)
	cacheWrite1hTokens := maxUsageToken(u.CacheCreation.Ephemeral1hInputTokens)
	cacheWriteTokens := maxUsageToken(u.CacheCreationInputTokens)
	if cacheWriteTokens == 0 {
		cacheWriteTokens = cacheWrite5mTokens + cacheWrite1hTokens
	}
	if previous == nil && u.InputTokens == 0 && u.OutputTokens == 0 && cacheWriteTokens == 0 && u.CacheReadInputTokens == 0 {
		return nil
	}

	usage := &adapter.Usage{}
	if previous != nil {
		*usage = *previous
	}
	if u.InputTokens > 0 {
		usage.UncachedInputTokens = u.InputTokens
	}
	if u.CacheReadInputTokens > 0 {
		usage.CacheReadTokens = u.CacheReadInputTokens
	}
	if cacheWrite5mTokens > 0 {
		usage.CacheWrite5mTokens = cacheWrite5mTokens
	}
	if cacheWrite1hTokens > 0 {
		usage.CacheWrite1hTokens = cacheWrite1hTokens
	}
	if cacheWriteTokens > 0 {
		usage.CacheWriteTokens = cacheWriteTokens
	}
	if u.OutputTokens > 0 {
		usage.CompletionTokens = u.OutputTokens
	}
	usage.PromptTokens = usage.UncachedInputTokens + usage.CacheReadTokens + usage.CacheWriteTokens
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	usage.NormalizeCacheTokens()
	return usage
}

func maxUsageToken(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func streamRawHTTPEvents(
	ctx context.Context,
	respBody io.ReadCloser,
	usageFromRaw func(json.RawMessage, *adapter.Usage) *adapter.Usage,
	responses ...*http.Response,
) <-chan adapter.RawStreamEvent {
	out := make(chan adapter.RawStreamEvent, 10)
	eventChan := make(chan adapter.RawStreamEvent, 10)
	errChan := make(chan error, 1)

	go adapter.ParseSSEEvents(respBody, eventChan, errChan)

	go func() {
		defer close(out)
		defer respBody.Close()

		var lastUsage *adapter.Usage
		var settlement *adapter.SettlementResult
		for {
			select {
			case <-ctx.Done():
				out <- adapter.RawStreamEvent{Error: ctx.Err(), Done: true, Usage: lastUsage, Settlement: settlement}
				return
			case err := <-errChan:
				if err != nil {
					out <- adapter.RawStreamEvent{Error: err, Done: true, Usage: lastUsage, Settlement: settlement}
				}
				return
			case event, ok := <-eventChan:
				if !ok {
					if len(responses) > 0 && responses[0] != nil {
						if parsed := settlementFromHeaders(responses[0].Trailer); parsed != nil {
							settlement = parsed
						} else if parsed := settlementFromHeaders(responses[0].Header); parsed != nil {
							settlement = parsed
						}
					}
					out <- adapter.RawStreamEvent{Done: true, Usage: lastUsage, Settlement: settlement}
					return
				}
				if event.Event == eventZGISettlement {
					var parsed adapter.SettlementResult
					if err := json.Unmarshal(event.Data, &parsed); err == nil {
						settlement = &parsed
					}
					continue
				}
				if event.Event == eventZGISettlementError {
					var parsed adapter.SettlementError
					if err := json.Unmarshal(event.Data, &parsed); err == nil {
						out <- adapter.RawStreamEvent{
							Error:      settlementErrorToError(&parsed),
							Done:       true,
							Usage:      lastUsage,
							Settlement: settlement,
						}
						return
					}
					out <- adapter.RawStreamEvent{
						Error:      fmt.Errorf("console proxy settlement failed: invalid settlement error event"),
						Done:       true,
						Usage:      lastUsage,
						Settlement: settlement,
					}
					return
				}
				if event.Event == "" {
					event.Event = rawEventType(event.Data)
				}
				lastUsage = usageFromRaw(event.Data, lastUsage)
				event.Usage = lastUsage
				event.Settlement = settlement
				out <- event
			}
		}
	}()

	return out
}

func rawRequestBody(requestBody json.RawMessage) (json.RawMessage, error) {
	if len(requestBody) == 0 {
		return nil, fmt.Errorf("%w: request body is required", adapter.ErrInvalidRequest)
	}
	if !json.Valid(requestBody) {
		return nil, fmt.Errorf("%w: request body must be valid JSON", adapter.ErrInvalidRequest)
	}
	return requestBody, nil
}

func rawOpenAIResponseRequest(
	ctx context.Context,
	httpClient *adapter.HTTPClient,
	baseURL string,
	headers map[string]string,
	request *adapter.RawResponseRequest,
	handleError func(int, []byte) error,
) (*adapter.RawResponse, error) {
	body, err := rawRequestBody(request.Body)
	if err != nil {
		return nil, err
	}

	url := strings.TrimRight(baseURL, "/") + "/responses"
	respBody, statusCode, err := httpClient.DoRequest(ctx, http.MethodPost, url, headers, body)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if statusCode != http.StatusOK {
		return nil, handleError(statusCode, respBody)
	}

	return &adapter.RawResponse{
		Body:  respBody,
		Usage: openAIUsageFromRaw(respBody),
	}, nil
}

func rawOpenAIResponseStream(
	ctx context.Context,
	httpClient *adapter.HTTPClient,
	baseURL string,
	headers map[string]string,
	request *adapter.RawResponseRequest,
	handleError ...func(int, []byte) error,
) (<-chan adapter.RawStreamEvent, error) {
	body, err := rawRequestBody(request.Body)
	if err != nil {
		return nil, err
	}

	url := strings.TrimRight(baseURL, "/") + "/responses"
	resp, err := httpClient.DoStreamRequest(ctx, http.MethodPost, url, headers, body)
	if err != nil {
		if parsed := parseRawStreamHTTPError(err, handleError); parsed != nil {
			return nil, parsed
		}
		return nil, fmt.Errorf("stream request failed: %w", err)
	}

	return streamRawHTTPEvents(ctx, resp.Body, func(raw json.RawMessage, _ *adapter.Usage) *adapter.Usage {
		return openAIUsageFromRaw(raw)
	}), nil
}

func rawAnthropicMessageRequest(
	ctx context.Context,
	httpClient *adapter.HTTPClient,
	baseURL string,
	headers map[string]string,
	request *adapter.AnthropicMessageRequest,
	handleError func(int, []byte) error,
) (*adapter.RawResponse, error) {
	body, err := rawRequestBody(request.Body)
	if err != nil {
		return nil, err
	}

	url := rawAnthropicMessagesURL(baseURL)
	respBody, statusCode, err := httpClient.DoRequest(ctx, http.MethodPost, url, headers, body)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if statusCode != http.StatusOK {
		return nil, handleError(statusCode, respBody)
	}

	return &adapter.RawResponse{
		Body:  respBody,
		Usage: anthropicUsageFromRaw(respBody, nil),
	}, nil
}

func rawAnthropicMessageStream(
	ctx context.Context,
	httpClient *adapter.HTTPClient,
	baseURL string,
	headers map[string]string,
	request *adapter.AnthropicMessageRequest,
	handleError ...func(int, []byte) error,
) (<-chan adapter.RawStreamEvent, error) {
	body, err := rawRequestBody(request.Body)
	if err != nil {
		return nil, err
	}

	url := rawAnthropicMessagesURL(baseURL)
	resp, err := httpClient.DoStreamRequest(ctx, http.MethodPost, url, headers, body)
	if err != nil {
		if parsed := parseRawStreamHTTPError(err, handleError); parsed != nil {
			return nil, parsed
		}
		return nil, fmt.Errorf("stream request failed: %w", err)
	}

	return streamRawHTTPEvents(ctx, resp.Body, anthropicUsageFromRaw), nil
}

func parseRawStreamHTTPError(err error, handlers []func(int, []byte) error) error {
	if len(handlers) == 0 || handlers[0] == nil {
		return nil
	}
	var statusErr *adapter.HTTPStatusError
	if !errors.As(err, &statusErr) {
		return nil
	}
	return handlers[0](statusErr.StatusCode, statusErr.Body)
}

func rawAnthropicMessagesURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	switch {
	case strings.HasSuffix(baseURL, "/v1/messages"):
		return baseURL
	case strings.HasSuffix(baseURL, "/messages"):
		return baseURL
	case strings.HasSuffix(baseURL, "/v1"):
		return baseURL + "/messages"
	default:
		return baseURL + "/v1/messages"
	}
}

func buildAnthropicRawHeaders(config *adapter.AdapterConfig, requestHeaders map[string]string) map[string]string {
	headers := map[string]string{
		"anthropic-version": "2023-06-01",
	}
	if config != nil {
		if strings.TrimSpace(config.APIKey) != "" {
			headers["x-api-key"] = strings.TrimSpace(config.APIKey)
		}
		for k, v := range config.Headers {
			headers[k] = v
		}
	}
	for k, v := range requestHeaders {
		if strings.TrimSpace(v) == "" {
			continue
		}
		headers[k] = v
	}
	return headers
}

func buildAnthropicBearerHeaders(config *adapter.AdapterConfig, requestHeaders map[string]string) map[string]string {
	headers := map[string]string{
		"anthropic-version": "2023-06-01",
	}
	if config != nil {
		if strings.TrimSpace(config.APIKey) != "" {
			headers["Authorization"] = fmt.Sprintf("Bearer %s", strings.TrimSpace(config.APIKey))
		}
		for k, v := range config.Headers {
			headers[k] = v
		}
	}
	return mergeHeaders(headers, requestHeaders)
}

func mergeHeaders(base map[string]string, overrides map[string]string) map[string]string {
	headers := make(map[string]string, len(base)+len(overrides))
	for k, v := range base {
		if strings.TrimSpace(v) == "" {
			continue
		}
		headers[k] = v
	}
	for k, v := range overrides {
		if strings.TrimSpace(v) == "" {
			continue
		}
		headers[k] = v
	}
	return headers
}
