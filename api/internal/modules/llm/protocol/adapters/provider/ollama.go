package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	ollamaDefaultTimeout    = 60 * time.Second
	ollamaDefaultMaxRetries = 3

	ollamaPathAPI              = "/api"
	ollamaPathV1               = "/v1"
	ollamaEndpointEmbed        = "/embed"
	ollamaEndpointTags         = "/tags"
	ollamaEndpointChatComplete = "/chat/completions"

	ollamaUseCaseChat        = "chat"
	ollamaUseCaseEmbedding   = "embedding"
	ollamaUseCaseUnsupported = "unsupported"
)

type ollamaBaseURLs struct {
	native    string
	openAI    string
	exact     string
	exactMode bool
}

// OllamaAdapter implements Ollama private channel support.
type OllamaAdapter struct {
	config     *adapter.AdapterConfig
	httpClient *adapter.HTTPClient
	baseURLs   ollamaBaseURLs
}

func validateOllamaConfig(config *adapter.AdapterConfig) error {
	if config == nil {
		return adapter.ErrInvalidConfig
	}
	if strings.TrimSpace(config.BaseURL) == "" {
		return fmt.Errorf("%w: base_url is required for Ollama", adapter.ErrInvalidConfig)
	}
	return nil
}

func normalizeOllamaBaseURL(raw string) (ollamaBaseURLs, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ollamaBaseURLs{}, fmt.Errorf("%w: base_url is required for Ollama", adapter.ErrInvalidConfig)
	}
	if strings.HasSuffix(trimmed, "#") {
		exact := strings.TrimSpace(strings.TrimSuffix(trimmed, "#"))
		if exact == "" {
			return ollamaBaseURLs{}, fmt.Errorf("%w: base_url before # is required for Ollama", adapter.ErrInvalidConfig)
		}
		return ollamaBaseURLs{
			exact:     exact,
			exactMode: true,
		}, nil
	}

	trimmed = strings.TrimRight(trimmed, "/")

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ollamaBaseURLs{}, fmt.Errorf("%w: invalid Ollama base_url", adapter.ErrInvalidConfig)
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""

	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case path == "":
		return ollamaBaseURLs{
			native: withOllamaPath(parsed, ollamaPathAPI),
			openAI: withOllamaPath(parsed, ollamaPathV1),
		}, nil
	case strings.HasSuffix(path, ollamaPathAPI):
		root := strings.TrimSuffix(path, ollamaPathAPI)
		return ollamaBaseURLs{
			native: withOllamaPath(parsed, path),
			openAI: withOllamaPath(parsed, root+ollamaPathV1),
		}, nil
	case strings.HasSuffix(path, ollamaPathV1):
		root := strings.TrimSuffix(path, ollamaPathV1)
		return ollamaBaseURLs{
			native: withOllamaPath(parsed, root+ollamaPathAPI),
			openAI: withOllamaPath(parsed, path),
		}, nil
	default:
		return ollamaBaseURLs{
			native: withOllamaPath(parsed, path+ollamaPathAPI),
			openAI: withOllamaPath(parsed, path+ollamaPathV1),
		}, nil
	}
}

func withOllamaPath(base *url.URL, path string) string {
	next := *base
	if path == "" {
		path = "/"
	}
	next.Path = path
	return strings.TrimRight(next.String(), "/")
}

// NewOllamaAdapter creates an adapter for Ollama private channels.
func NewOllamaAdapter(config *adapter.AdapterConfig) (*OllamaAdapter, error) {
	if err := validateOllamaConfig(config); err != nil {
		return nil, err
	}

	baseURLs, err := normalizeOllamaBaseURL(config.BaseURL)
	if err != nil {
		return nil, err
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = ollamaDefaultTimeout
	}

	maxRetries := config.MaxRetries
	if maxRetries == 0 {
		maxRetries = ollamaDefaultMaxRetries
	}

	return &OllamaAdapter{
		config:     config,
		httpClient: adapter.NewHTTPClientWithAuthHook(timeout, maxRetries, config.AuthHook),
		baseURLs:   baseURLs,
	}, nil
}

func (a *OllamaAdapter) chatEndpoint() string {
	if a.baseURLs.exactMode {
		return a.baseURLs.exact
	}
	return a.baseURLs.openAI + ollamaEndpointChatComplete
}

func (a *OllamaAdapter) embeddingEndpoint() string {
	if a.baseURLs.exactMode {
		return a.baseURLs.exact
	}
	return a.baseURLs.native + ollamaEndpointEmbed
}

func (a *OllamaAdapter) openAIBaseURL() string {
	if a.baseURLs.exactMode {
		return strings.TrimSuffix(a.baseURLs.exact, ollamaEndpointChatComplete)
	}
	return a.baseURLs.openAI
}

// ChatCompletion sends a non-streaming chat completion request through Ollama's OpenAI-compatible API.
func (a *OllamaAdapter) ChatCompletion(ctx context.Context, request *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	endpoint := a.chatEndpoint()
	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "POST", endpoint, a.buildHeaders(""), buildOpenAICompatibleChatPayload(request))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if statusCode != http.StatusOK {
		return nil, handleOllamaError(statusCode, respBody)
	}

	var response adapter.ChatResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &response, nil
}

// ChatCompletionStream sends a streaming chat completion request through Ollama's OpenAI-compatible API.
func (a *OllamaAdapter) ChatCompletionStream(ctx context.Context, request *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	request.Stream = true
	endpoint := a.chatEndpoint()
	resp, err := a.httpClient.DoStreamRequest(ctx, "POST", endpoint, a.buildHeaders(""), buildOpenAICompatibleChatPayload(request))
	if err != nil {
		return nil, fmt.Errorf("stream request failed: %w", err)
	}

	respChan := make(chan adapter.StreamResponse, 10)
	dataChan := make(chan string, 10)
	errChan := make(chan error, 1)

	go adapter.ParseSSE(resp.Body, dataChan, errChan)

	go func() {
		defer close(respChan)
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}()

		var lastUsage *adapter.Usage
		for {
			select {
			case <-ctx.Done():
				respChan <- adapter.StreamResponse{Error: ctx.Err(), Done: true, Usage: lastUsage}
				return
			case err := <-errChan:
				if err != nil {
					respChan <- adapter.StreamResponse{Error: err, Done: true, Usage: lastUsage}
				}
				return
			case data, ok := <-dataChan:
				if !ok {
					respChan <- adapter.StreamResponse{Done: true, Usage: lastUsage}
					return
				}

				var streamResp adapter.StreamResponse
				if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
					respChan <- adapter.StreamResponse{Error: fmt.Errorf("failed to parse stream data: %w", err), Done: true, Usage: lastUsage}
					return
				}
				if streamResp.Usage != nil {
					lastUsage = streamResp.Usage
				}
				respChan <- streamResp
			}
		}
	}()

	return respChan, nil
}

// CreateResponse reports that the typed Responses shape is intentionally unsupported.
func (a *OllamaAdapter) CreateResponse(context.Context, *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, fmt.Errorf("%w: Ollama typed responses API is not implemented in this adapter", adapter.ErrCapabilityUnsupported)
}

func (a *OllamaAdapter) CreateResponseRaw(ctx context.Context, request *adapter.RawResponseRequest) (*adapter.RawResponse, error) {
	return rawOpenAIResponseRequest(ctx, a.httpClient, a.openAIBaseURL(), a.buildHeaders(""), request, handleOllamaError)
}

func (a *OllamaAdapter) CreateResponseStream(ctx context.Context, request *adapter.RawResponseRequest) (<-chan adapter.RawStreamEvent, error) {
	return rawOpenAIResponseStream(ctx, a.httpClient, a.openAIBaseURL(), a.buildHeaders(""), request)
}

func (a *OllamaAdapter) CreateAnthropicMessage(ctx context.Context, request *adapter.AnthropicMessageRequest) (*adapter.RawResponse, error) {
	return rawAnthropicMessageRequest(ctx, a.httpClient, a.openAIBaseURL(), buildAnthropicRawHeaders(a.config, request.Headers), request, handleOllamaError)
}

func (a *OllamaAdapter) CreateAnthropicMessageStream(ctx context.Context, request *adapter.AnthropicMessageRequest) (<-chan adapter.RawStreamEvent, error) {
	return rawAnthropicMessageStream(ctx, a.httpClient, a.openAIBaseURL(), buildAnthropicRawHeaders(a.config, request.Headers), request)
}

// CreateEmbeddings creates embeddings through Ollama's native embed API.
func (a *OllamaAdapter) CreateEmbeddings(ctx context.Context, request *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	payload, err := buildOllamaEmbedPayload(request)
	if err != nil {
		return nil, err
	}

	endpoint := a.embeddingEndpoint()
	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "POST", endpoint, a.buildHeaders(""), payload)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if statusCode != http.StatusOK {
		return nil, handleOllamaError(statusCode, respBody)
	}

	var upstream ollamaEmbedResponse
	if err := json.Unmarshal(respBody, &upstream); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return mapOllamaEmbeddingsResponse(request.Model, upstream), nil
}

func buildOllamaEmbedPayload(request *adapter.EmbeddingsRequest) (map[string]interface{}, error) {
	if request == nil {
		return nil, fmt.Errorf("%w: request is required", adapter.ErrInvalidRequest)
	}
	if strings.TrimSpace(request.Model) == "" {
		return nil, fmt.Errorf("%w: model is required", adapter.ErrInvalidRequest)
	}
	if request.Input == nil {
		return nil, fmt.Errorf("%w: input is required", adapter.ErrInvalidRequest)
	}

	payload := map[string]interface{}{
		"model": request.Model,
		"input": request.Input,
	}
	if request.Dimensions > 0 {
		payload["dimensions"] = request.Dimensions
	}
	if request.Truncate != "" {
		truncate, err := strconv.ParseBool(request.Truncate)
		if err != nil {
			return nil, fmt.Errorf("%w: truncate must be a boolean for Ollama", adapter.ErrInvalidRequest)
		}
		payload["truncate"] = truncate
	}
	return payload, nil
}

func mapOllamaEmbeddingsResponse(model string, upstream ollamaEmbedResponse) *adapter.EmbeddingsResponse {
	responseModel := upstream.Model
	if responseModel == "" {
		responseModel = model
	}

	data := make([]adapter.Embedding, 0, len(upstream.Embeddings))
	for i, values := range upstream.Embeddings {
		data = append(data, adapter.Embedding{
			Object:    "embedding",
			Embedding: values,
			Index:     i,
		})
	}

	totalTokens := upstream.PromptEvalCount
	return &adapter.EmbeddingsResponse{
		Object: "list",
		Model:  responseModel,
		Data:   data,
		Usage: adapter.Usage{
			PromptTokens: totalTokens,
			TotalTokens:  totalTokens,
		},
	}
}

// CreateImage reports that Ollama image generation is unsupported.
func (a *OllamaAdapter) CreateImage(context.Context, *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, fmt.Errorf("%w: Ollama image API is not implemented in this adapter", adapter.ErrCapabilityUnsupported)
}

// ListModels lists local Ollama models through the native tags API.
func (a *OllamaAdapter) ListModels(ctx context.Context, apiKey string) ([]adapter.Model, error) {
	if a.baseURLs.exactMode {
		return nil, fmt.Errorf("%w: Ollama model listing is unavailable when base_url ends with #", adapter.ErrCapabilityUnsupported)
	}

	endpoint := a.baseURLs.native + ollamaEndpointTags
	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "GET", endpoint, a.buildHeaders(apiKey), nil)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if statusCode != http.StatusOK {
		return nil, handleOllamaError(statusCode, respBody)
	}

	var response ollamaTagsResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	models := make([]adapter.Model, 0, len(response.Models))
	for _, item := range response.Models {
		name := strings.TrimSpace(item.Model)
		if name == "" {
			name = strings.TrimSpace(item.Name)
		}
		if name == "" {
			continue
		}

		useCase := inferOllamaModelUseCase(name)
		models = append(models, adapter.Model{
			ID:           name,
			Name:         name,
			Type:         useCase,
			Created:      parseOllamaModifiedAt(item.ModifiedAt),
			OwnedBy:      "library",
			Capabilities: ollamaCapabilitiesForUseCase(useCase),
			Endpoints:    ollamaCapabilitiesForUseCase(useCase),
		})
	}

	return models, nil
}

func inferOllamaModelUseCase(modelName string) string {
	lower := strings.ToLower(strings.TrimSpace(modelName))
	switch {
	case strings.Contains(lower, "rerank"),
		strings.Contains(lower, "reranker"),
		strings.Contains(lower, "bge-reranker"),
		strings.Contains(lower, "bce-reranker"):
		return ollamaUseCaseUnsupported
	case strings.Contains(lower, "embed"),
		strings.Contains(lower, "embedding"),
		strings.Contains(lower, "nomic-embed"),
		strings.Contains(lower, "mxbai-embed"),
		strings.Contains(lower, "all-minilm"):
		return ollamaUseCaseEmbedding
	default:
		return ollamaUseCaseChat
	}
}

func ollamaCapabilitiesForUseCase(useCase string) []string {
	switch useCase {
	case ollamaUseCaseEmbedding:
		return []string{ollamaUseCaseEmbedding}
	case ollamaUseCaseUnsupported:
		return nil
	default:
		return []string{ollamaUseCaseChat, "stream"}
	}
}

func parseOllamaModifiedAt(value string) int64 {
	if value == "" {
		return 0
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return 0
	}
	return parsed.Unix()
}

// GetBalance reports that Ollama balance lookup is unsupported.
func (a *OllamaAdapter) GetBalance(context.Context, string) (*adapter.Balance, error) {
	return nil, fmt.Errorf("%w: Ollama balance API is not supported", adapter.ErrCapabilityUnsupported)
}

// ValidateConfig validates the Ollama adapter configuration.
func (a *OllamaAdapter) ValidateConfig(config *adapter.AdapterConfig) error {
	return validateOllamaConfig(config)
}

// GetProviderInfo returns Ollama provider metadata.
func (a *OllamaAdapter) GetProviderInfo() *adapter.ProviderInfo {
	baseURL := a.baseURLs.native
	if a.baseURLs.exactMode {
		baseURL = a.baseURLs.exact
	}
	return &adapter.ProviderInfo{
		Name:         "ollama",
		Type:         "ollama",
		DisplayName:  "Ollama",
		Description:  "Local Ollama models",
		BaseURL:      baseURL,
		Capabilities: []string{"chat", "stream", "embedding", "model_listing"},
		Version:      "v1",
	}
}

func handleOllamaError(statusCode int, body []byte) error {
	var errResp struct {
		Error interface{} `json:"error"`
	}
	if err := json.Unmarshal(body, &errResp); err != nil {
		return adapter.HandleNonJSONError(statusCode, body)
	}

	message := "Ollama upstream error"
	code := "OLLAMA_UPSTREAM_ERROR"
	switch v := errResp.Error.(type) {
	case string:
		if strings.TrimSpace(v) != "" {
			message = v
		}
	case map[string]interface{}:
		if raw, ok := v["message"].(string); ok && strings.TrimSpace(raw) != "" {
			message = raw
		}
		if raw, ok := v["code"].(string); ok && strings.TrimSpace(raw) != "" {
			code = raw
		}
	}

	switch statusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return adapter.NewAdapterError(code, message, statusCode, adapter.ErrAuthFailed)
	case http.StatusNotFound:
		return adapter.NewAdapterError(code, message, statusCode, adapter.ErrModelNotFound)
	case http.StatusTooManyRequests:
		return adapter.NewAdapterError(code, message, statusCode, adapter.ErrRateLimited)
	case http.StatusBadRequest:
		return adapter.NewAdapterError(code, message, statusCode, adapter.ErrInvalidRequest)
	default:
		return adapter.NewAdapterError(code, message, statusCode, adapter.ErrUpstreamError)
	}
}

func (a *OllamaAdapter) buildHeaders(apiKey string) map[string]string {
	headers := make(map[string]string)
	token := strings.TrimSpace(apiKey)
	if token == "" && a.config != nil {
		token = strings.TrimSpace(a.config.APIKey)
	}
	if token != "" {
		headers["Authorization"] = fmt.Sprintf("Bearer %s", token)
	}
	if a.config != nil {
		for k, v := range a.config.Headers {
			headers[k] = v
		}
	}
	return headers
}

type ollamaEmbedResponse struct {
	Model           string      `json:"model"`
	Embeddings      [][]float32 `json:"embeddings"`
	PromptEvalCount int         `json:"prompt_eval_count"`
}

type ollamaTagsResponse struct {
	Models []ollamaTagModel `json:"models"`
}

type ollamaTagModel struct {
	Name       string `json:"name"`
	Model      string `json:"model"`
	ModifiedAt string `json:"modified_at"`
}
