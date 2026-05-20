package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

const defaultMiniMaxBaseURL = "https://api.minimaxi.com/v1"

// MiniMaxAdapter implements the documented MiniMax OpenAI-compatible text API
// and the native image generation API.
type MiniMaxAdapter struct {
	config     *adapter.AdapterConfig
	httpClient *adapter.HTTPClient
	baseURL    string
	openAI     *OpenAIAdapter
}

type miniMaxModelMetadata struct {
	Type         string
	Capabilities []string
	Architecture *adapter.ModelArchitecture
}

var miniMaxDocumentedModelOrder = []string{
	"MiniMax-M2.5",
	"MiniMax-M2.5-highspeed",
	"MiniMax-M2.1",
	"MiniMax-M2.1-highspeed",
	"MiniMax-M2",
	"image-01",
	"image-01-live",
}

var miniMaxDocumentedModelCatalog = map[string]miniMaxModelMetadata{
	"MiniMax-M2.5": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "text",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		},
	},
	"MiniMax-M2.5-highspeed": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "text",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		},
	},
	"MiniMax-M2.1": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "text",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		},
	},
	"MiniMax-M2.1-highspeed": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "text",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		},
	},
	"MiniMax-M2": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "text",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		},
	},
	"image-01": {
		Type:         "image",
		Capabilities: []string{"image"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "image",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"image"},
		},
	},
	"image-01-live": {
		Type:         "image",
		Capabilities: []string{"image"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "image",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"image"},
		},
	},
	// Keep legacy or remotely returned MiniMax models classified safely.
	"MiniMax-M1": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "text",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		},
	},
	"MiniMax-Text-01": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "text",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		},
	},
	"MiniMax-VL-01": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "multimodal",
			InputModalities:  []string{"text", "image", "video"},
			OutputModalities: []string{"text"},
		},
	},
	"MiniMax-VL-01-direct": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "multimodal",
			InputModalities:  []string{"text", "image", "video"},
			OutputModalities: []string{"text"},
		},
	},
}

// NewMiniMaxAdapter creates a MiniMax adapter.
func NewMiniMaxAdapter(config *adapter.AdapterConfig) (*MiniMaxAdapter, error) {
	if err := validateOpenAIConfig(config); err != nil {
		return nil, err
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	maxRetries := config.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}

	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultMiniMaxBaseURL
	}

	openAICompatible, err := newOpenAIAdapterWithOverrides(config, baseURL)
	if err != nil {
		return nil, err
	}

	return &MiniMaxAdapter{
		config:     config,
		httpClient: adapter.NewHTTPClientWithAuthHook(timeout, maxRetries, config.AuthHook),
		baseURL:    baseURL,
		openAI:     openAICompatible,
	}, nil
}

func (a *MiniMaxAdapter) ChatCompletion(ctx context.Context, request *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return a.openAI.ChatCompletion(ctx, request)
}

func (a *MiniMaxAdapter) ChatCompletionStream(ctx context.Context, request *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return a.openAI.ChatCompletionStream(ctx, request)
}

func (a *MiniMaxAdapter) CreateResponse(context.Context, *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, fmt.Errorf("%w: MiniMax /responses API is not documented", adapter.ErrCapabilityUnsupported)
}

func (a *MiniMaxAdapter) CreateAnthropicMessage(ctx context.Context, request *adapter.AnthropicMessageRequest) (*adapter.RawResponse, error) {
	return rawAnthropicMessageRequest(ctx, a.httpClient, a.baseURL, buildAnthropicRawHeaders(a.config, request.Headers), request, a.openAI.handleError)
}

func (a *MiniMaxAdapter) CreateAnthropicMessageStream(ctx context.Context, request *adapter.AnthropicMessageRequest) (<-chan adapter.RawStreamEvent, error) {
	return rawAnthropicMessageStream(ctx, a.httpClient, a.baseURL, buildAnthropicRawHeaders(a.config, request.Headers), request)
}

func (a *MiniMaxAdapter) CreateEmbeddings(context.Context, *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, fmt.Errorf("%w: MiniMax embeddings API is not documented", adapter.ErrCapabilityUnsupported)
}

func (a *MiniMaxAdapter) CreateImage(ctx context.Context, request *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	if request == nil {
		return nil, fmt.Errorf("%w: request is required", adapter.ErrInvalidRequest)
	}
	if strings.TrimSpace(request.Model) == "" {
		return nil, fmt.Errorf("%w: model is required", adapter.ErrInvalidRequest)
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return nil, fmt.Errorf("%w: prompt is required", adapter.ErrInvalidRequest)
	}

	payload := map[string]any{
		"model":  request.Model,
		"prompt": request.Prompt,
	}
	if request.N != nil {
		payload["n"] = *request.N
	}
	if request.ResponseFormat != "" {
		payload["response_format"] = normalizeMiniMaxResponseFormat(request.ResponseFormat)
	}
	if request.Style != "" {
		payload["style"] = request.Style
	}
	if width, height, ok := parseMiniMaxSize(request.Size); ok {
		payload["width"] = width
		payload["height"] = height
	}
	for key, value := range request.AdditionalParameters {
		payload[key] = value
	}

	url := fmt.Sprintf("%s/image_generation", a.baseURL)
	respBody, statusCode, err := a.httpClient.DoRequest(ctx, http.MethodPost, url, a.openAI.buildHeaders(), payload)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if statusCode != http.StatusOK {
		return nil, a.openAI.handleError(statusCode, respBody)
	}

	var response struct {
		BaseResp struct {
			StatusCode int    `json:"status_code"`
			StatusMsg  string `json:"status_msg"`
		} `json:"base_resp"`
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if response.BaseResp.StatusCode != 0 {
		return nil, adapter.NewAdapterError(
			strconv.Itoa(response.BaseResp.StatusCode),
			response.BaseResp.StatusMsg,
			http.StatusBadGateway,
			adapter.ErrUpstreamError,
		)
	}

	items := make([]adapter.ImageItem, 0)
	items = append(items, miniMaxImageItems(response.Data["image_url"], false)...)
	items = append(items, miniMaxImageItems(response.Data["image_urls"], false)...)
	items = append(items, miniMaxImageItems(response.Data["image_base64"], true)...)
	items = append(items, miniMaxImageItems(response.Data["image_base64s"], true)...)
	items = append(items, miniMaxImageItems(response.Data["images"], false)...)
	if len(items) == 0 {
		return nil, fmt.Errorf("failed to parse response: no generated image data returned")
	}

	return &adapter.ImageResponse{
		Created: time.Now().Unix(),
		Data:    items,
	}, nil
}

func (a *MiniMaxAdapter) Rerank(context.Context, *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, fmt.Errorf("%w: MiniMax rerank API is not documented", adapter.ErrCapabilityUnsupported)
}

func (a *MiniMaxAdapter) ListModels(ctx context.Context, apiKey string) ([]adapter.Model, error) {
	models, err := a.fetchRemoteModels(ctx, apiKey)
	if err == nil {
		return models, nil
	}
	if !shouldFallbackMiniMaxListModels(err) {
		return nil, err
	}
	return a.documentedModels(), nil
}

func (a *MiniMaxAdapter) GetBalance(context.Context, string) (*adapter.Balance, error) {
	return nil, fmt.Errorf("%w: MiniMax balance lookup is not documented", adapter.ErrCapabilityUnsupported)
}

func (a *MiniMaxAdapter) ValidateConfig(config *adapter.AdapterConfig) error {
	return validateOpenAIConfig(config)
}

func (a *MiniMaxAdapter) GetProviderInfo() *adapter.ProviderInfo {
	return &adapter.ProviderInfo{
		Name:         "minimax",
		Type:         "minimax",
		DisplayName:  "MiniMax",
		Description:  "MiniMax models",
		BaseURL:      a.baseURL,
		Capabilities: []string{"chat", "stream", "image", "model_listing"},
		Version:      "v1",
	}
}

func (a *MiniMaxAdapter) fetchRemoteModels(ctx context.Context, apiKey string) ([]adapter.Model, error) {
	url := fmt.Sprintf("%s/models", a.baseURL)
	headers := a.runtimeHeaders(apiKey)

	respBody, statusCode, err := a.httpClient.DoRequest(ctx, http.MethodGet, url, headers, nil)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if statusCode != http.StatusOK {
		return nil, a.openAI.handleError(statusCode, respBody)
	}

	var response struct {
		Data []struct {
			ID      string `json:"id"`
			Created int64  `json:"created"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	models := make([]adapter.Model, 0, len(response.Data))
	for _, item := range response.Data {
		model := a.normalizeModel(item.ID)
		model.Created = item.Created
		model.OwnedBy = item.OwnedBy
		models = append(models, model)
	}
	return models, nil
}

func (a *MiniMaxAdapter) documentedModels() []adapter.Model {
	models := make([]adapter.Model, 0, len(miniMaxDocumentedModelOrder))
	for _, id := range miniMaxDocumentedModelOrder {
		models = append(models, a.normalizeModel(id))
	}
	return models
}

func (a *MiniMaxAdapter) normalizeModel(id string) adapter.Model {
	meta, ok := miniMaxDocumentedModelCatalog[id]
	if !ok {
		return inferMiniMaxModel(id)
	}

	model := adapter.Model{
		ID:           id,
		Name:         id,
		Type:         meta.Type,
		Capabilities: append([]string(nil), meta.Capabilities...),
	}
	if meta.Architecture != nil {
		model.Architecture = &adapter.ModelArchitecture{
			Modality:         meta.Architecture.Modality,
			InputModalities:  append([]string(nil), meta.Architecture.InputModalities...),
			OutputModalities: append([]string(nil), meta.Architecture.OutputModalities...),
			Tokenizer:        meta.Architecture.Tokenizer,
			InstructType:     meta.Architecture.InstructType,
		}
	}
	return model
}

func inferMiniMaxModel(id string) adapter.Model {
	lowerID := strings.ToLower(strings.TrimSpace(id))

	switch {
	case strings.Contains(lowerID, "image-"):
		return adapter.Model{
			ID:           id,
			Name:         id,
			Type:         "image",
			Capabilities: []string{"image"},
			Architecture: &adapter.ModelArchitecture{
				Modality:         "image",
				InputModalities:  []string{"text"},
				OutputModalities: []string{"image"},
			},
		}
	case strings.Contains(lowerID, "vl") || strings.Contains(lowerID, "vision"):
		return adapter.Model{
			ID:           id,
			Name:         id,
			Type:         "chat",
			Capabilities: []string{"chat", "stream"},
			Architecture: &adapter.ModelArchitecture{
				Modality:         "multimodal",
				InputModalities:  []string{"text", "image", "video"},
				OutputModalities: []string{"text"},
			},
		}
	default:
		return adapter.Model{
			ID:           id,
			Name:         id,
			Type:         "chat",
			Capabilities: []string{"chat", "stream"},
			Architecture: &adapter.ModelArchitecture{
				Modality:         "text",
				InputModalities:  []string{"text"},
				OutputModalities: []string{"text"},
			},
		}
	}
}

func miniMaxImageItems(raw json.RawMessage, isBase64 bool) []adapter.ImageItem {
	if len(raw) == 0 {
		return nil
	}

	values, ok := miniMaxStringSlice(raw)
	if ok {
		items := make([]adapter.ImageItem, 0, len(values))
		for _, value := range values {
			if isBase64 {
				items = append(items, adapter.ImageItem{B64JSON: value})
				continue
			}
			items = append(items, adapter.ImageItem{URL: value})
		}
		return items
	}

	var objects []struct {
		URL           string `json:"url"`
		ImageURL      string `json:"image_url"`
		B64JSON       string `json:"b64_json"`
		ImageBase64   string `json:"image_base64"`
		RevisedPrompt string `json:"revised_prompt"`
	}
	if err := json.Unmarshal(raw, &objects); err != nil {
		return nil
	}

	items := make([]adapter.ImageItem, 0, len(objects))
	for _, object := range objects {
		item := adapter.ImageItem{
			URL:           firstNonEmpty(object.URL, object.ImageURL),
			B64JSON:       firstNonEmpty(object.B64JSON, object.ImageBase64),
			RevisedPrompt: object.RevisedPrompt,
		}
		if item.URL != "" || item.B64JSON != "" {
			items = append(items, item)
		}
	}
	return items
}

func miniMaxStringSlice(raw json.RawMessage) ([]string, bool) {
	var single string
	if err := json.Unmarshal(raw, &single); err == nil && single != "" {
		return []string{single}, true
	}

	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		return many, true
	}
	return nil, false
}

func parseMiniMaxSize(size string) (int, int, bool) {
	widthText, heightText, ok := strings.Cut(strings.TrimSpace(size), "x")
	if !ok {
		return 0, 0, false
	}

	width, err := strconv.Atoi(strings.TrimSpace(widthText))
	if err != nil || width <= 0 {
		return 0, 0, false
	}
	height, err := strconv.Atoi(strings.TrimSpace(heightText))
	if err != nil || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

func normalizeMiniMaxResponseFormat(format string) string {
	if strings.EqualFold(strings.TrimSpace(format), "b64_json") {
		return "base64"
	}
	return format
}

func shouldFallbackMiniMaxListModels(err error) bool {
	var adapterErr *adapter.AdapterError
	if errors.As(err, &adapterErr) {
		switch adapterErr.StatusCode {
		case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented:
			return true
		default:
			return false
		}
	}

	return strings.Contains(err.Error(), "failed to parse response")
}

func (a *MiniMaxAdapter) runtimeHeaders(apiKey string) map[string]string {
	if strings.TrimSpace(apiKey) == "" {
		apiKey = a.config.APIKey
	}

	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", apiKey),
	}
	for key, value := range a.config.Headers {
		headers[key] = value
	}
	return headers
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
