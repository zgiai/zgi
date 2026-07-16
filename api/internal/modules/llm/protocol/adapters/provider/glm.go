package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	defaultGLMBaseURL              = "https://open.bigmodel.cn/api/paas/v4"
	defaultGLMAnthropicMessagesURL = "https://api.z.ai/api/anthropic/v1"
)

// GLMAdapter implements the documented GLM OpenAI-compatible endpoints.
type GLMAdapter struct {
	config     *adapter.AdapterConfig
	httpClient *adapter.HTTPClient
	baseURL    string
	openAI     *OpenAIAdapter
}

type glmModelMetadata struct {
	Type         string
	Capabilities []string
	Architecture *adapter.ModelArchitecture
}

var glmDocumentedModelOrder = []string{
	"glm-5.2",
	"glm-5.1",
	"glm-5-turbo",
	"glm-5",
	"glm-4.7",
	"glm-4.7-flash",
	"glm-4.7-flashx",
	"glm-4.6",
	"glm-4.5-air",
	"glm-4.5-airx",
	"glm-4.5-flash",
	"glm-4-flash-250414",
	"glm-4-flashx-250414",
	"glm-4.6v",
	"autoglm-phone",
	"glm-4.6v-flash",
	"glm-4.6v-flashx",
	"glm-4v-flash",
	"glm-4.1v-thinking-flashx",
	"glm-4.1v-thinking-flash",
	"glm-image",
	"cogview-4-250304",
	"cogview-4",
	"cogview-3-flash",
	"embedding-3",
	"embedding-2",
	"rerank",
}

var glmDocumentedModelCatalog = map[string]glmModelMetadata{
	"glm-5.2": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "text",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		},
	},
	"glm-5.1": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "text",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		},
	},
	"glm-5-turbo": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "text",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		},
	},
	"glm-5": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "text",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		},
	},
	"glm-4.7": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "text",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		},
	},
	"glm-4.7-flash": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "text",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		},
	},
	"glm-4.7-flashx": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "text",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		},
	},
	"glm-4.6": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "text",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		},
	},
	"glm-4.5-air": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "text",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		},
	},
	"glm-4.5-airx": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "text",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		},
	},
	"glm-4.5-flash": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "text",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		},
	},
	"glm-4-flash-250414": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "text",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		},
	},
	"glm-4-flashx-250414": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "text",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"text"},
		},
	},
	"glm-4.6v": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "multimodal",
			InputModalities:  []string{"text", "image", "video", "file"},
			OutputModalities: []string{"text"},
		},
	},
	"autoglm-phone": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "multimodal",
			InputModalities:  []string{"text", "image", "video", "file"},
			OutputModalities: []string{"text"},
		},
	},
	"glm-4.6v-flash": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "multimodal",
			InputModalities:  []string{"text", "image", "video", "file"},
			OutputModalities: []string{"text"},
		},
	},
	"glm-4.6v-flashx": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "multimodal",
			InputModalities:  []string{"text", "image", "video", "file"},
			OutputModalities: []string{"text"},
		},
	},
	"glm-4v-flash": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "multimodal",
			InputModalities:  []string{"text", "image"},
			OutputModalities: []string{"text"},
		},
	},
	"glm-4.1v-thinking-flashx": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "multimodal",
			InputModalities:  []string{"text", "image", "video", "file"},
			OutputModalities: []string{"text"},
		},
	},
	"glm-4.1v-thinking-flash": {
		Type:         "chat",
		Capabilities: []string{"chat", "stream"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "multimodal",
			InputModalities:  []string{"text", "image", "video", "file"},
			OutputModalities: []string{"text"},
		},
	},
	"glm-image": {
		Type:         "image",
		Capabilities: []string{"image"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "image",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"image"},
		},
	},
	"cogview-4-250304": {
		Type:         "image",
		Capabilities: []string{"image"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "image",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"image"},
		},
	},
	"cogview-4": {
		Type:         "image",
		Capabilities: []string{"image"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "image",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"image"},
		},
	},
	"cogview-3-flash": {
		Type:         "image",
		Capabilities: []string{"image"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "image",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"image"},
		},
	},
	"embedding-3": {
		Type:         "embedding",
		Capabilities: []string{"embedding"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "embedding",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"embedding"},
		},
	},
	"embedding-2": {
		Type:         "embedding",
		Capabilities: []string{"embedding"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "embedding",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"embedding"},
		},
	},
	"rerank": {
		Type:         "rerank",
		Capabilities: []string{"rerank"},
		Architecture: &adapter.ModelArchitecture{
			Modality:         "rerank",
			InputModalities:  []string{"text"},
			OutputModalities: []string{"score"},
		},
	},
}

// NewGLMAdapter creates a GLM adapter.
func NewGLMAdapter(config *adapter.AdapterConfig) (*GLMAdapter, error) {
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
		baseURL = defaultGLMBaseURL
	}

	openAICompatible, err := newOpenAIAdapterWithOverrides(config, baseURL)
	if err != nil {
		return nil, err
	}

	return &GLMAdapter{
		config:     config,
		httpClient: adapter.NewHTTPClientFromConfig(config, timeout, maxRetries),
		baseURL:    baseURL,
		openAI:     openAICompatible,
	}, nil
}

func (a *GLMAdapter) anthropicMessagesBaseURL() string {
	baseURL := strings.TrimRight(a.baseURL, "/")
	if baseURL == defaultGLMBaseURL {
		return defaultGLMAnthropicMessagesURL
	}
	if strings.Contains(baseURL, "/api/anthropic") {
		return baseURL
	}
	if strings.Contains(baseURL, "/api/paas/v4") {
		return strings.Replace(baseURL, "/api/paas/v4", "/api/anthropic/v1", 1)
	}
	if strings.HasSuffix(baseURL, "/v1") {
		return strings.TrimSuffix(baseURL, "/v1") + "/api/anthropic/v1"
	}
	return baseURL + "/api/anthropic/v1"
}

func (a *GLMAdapter) ChatCompletion(ctx context.Context, request *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return a.openAI.ChatCompletion(ctx, request)
}

func (a *GLMAdapter) ChatCompletionStream(ctx context.Context, request *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return a.openAI.ChatCompletionStream(ctx, request)
}

func (a *GLMAdapter) CreateResponse(context.Context, *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, fmt.Errorf("%w: GLM /responses API is not documented", adapter.ErrCapabilityUnsupported)
}

func (a *GLMAdapter) CreateAnthropicMessage(ctx context.Context, request *adapter.AnthropicMessageRequest) (*adapter.RawResponse, error) {
	return rawAnthropicMessageRequest(ctx, a.httpClient, a.anthropicMessagesBaseURL(), buildAnthropicBearerHeaders(a.config, request.Headers), request, a.openAI.handleError)
}

func (a *GLMAdapter) CreateAnthropicMessageStream(ctx context.Context, request *adapter.AnthropicMessageRequest) (<-chan adapter.RawStreamEvent, error) {
	return rawAnthropicMessageStream(ctx, a.httpClient, a.anthropicMessagesBaseURL(), buildAnthropicBearerHeaders(a.config, request.Headers), request)
}

func (a *GLMAdapter) CreateEmbeddings(ctx context.Context, request *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return a.openAI.CreateEmbeddings(ctx, request)
}

func (a *GLMAdapter) CreateImage(ctx context.Context, request *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return a.openAI.CreateImage(ctx, request)
}

func (a *GLMAdapter) Rerank(ctx context.Context, request *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	if request == nil {
		return nil, fmt.Errorf("%w: request is required", adapter.ErrInvalidRequest)
	}
	if strings.TrimSpace(request.Model) == "" {
		return nil, fmt.Errorf("%w: model is required", adapter.ErrInvalidRequest)
	}
	if strings.TrimSpace(request.Query) == "" {
		return nil, fmt.Errorf("%w: query is required", adapter.ErrInvalidRequest)
	}

	documents, err := glmDocuments(request.Documents)
	if err != nil {
		return nil, err
	}

	payload := map[string]any{
		"model":     request.Model,
		"query":     request.Query,
		"documents": documents,
	}
	if request.TopN != nil {
		payload["top_n"] = *request.TopN
	}
	if request.ReturnDocuments != nil {
		payload["return_documents"] = *request.ReturnDocuments
	}

	url := fmt.Sprintf("%s/rerank", a.baseURL)
	respBody, statusCode, err := a.httpClient.DoRequest(ctx, http.MethodPost, url, a.openAI.buildHeaders(), payload)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if statusCode != http.StatusOK {
		return nil, a.openAI.handleError(statusCode, respBody)
	}

	var resp struct {
		ID      string `json:"id"`
		Results []struct {
			Index          int     `json:"index"`
			Document       string  `json:"document"`
			RelevanceScore float64 `json:"relevance_score"`
		} `json:"results"`
		Usage *struct {
			PromptTokens int `json:"prompt_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	results := make([]adapter.RerankResult, 0, len(resp.Results))
	for _, item := range resp.Results {
		results = append(results, adapter.RerankResult{
			Index:          item.Index,
			RelevanceScore: item.RelevanceScore,
			Document:       item.Document,
			Text:           item.Document,
		})
	}

	response := &adapter.RerankResponse{
		ID:      resp.ID,
		Object:  "list",
		Model:   request.Model,
		Results: results,
	}
	if resp.Usage != nil {
		response.Usage = &adapter.Usage{
			PromptTokens: resp.Usage.PromptTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		}
	}

	return response, nil
}

func (a *GLMAdapter) ListModels(ctx context.Context, apiKey string) ([]adapter.Model, error) {
	models, err := a.fetchRemoteModels(ctx, apiKey)
	if err == nil {
		return models, nil
	}
	if !shouldFallbackGLMListModels(err) {
		return nil, err
	}
	return a.documentedModels(), nil
}

func (a *GLMAdapter) GetBalance(context.Context, string) (*adapter.Balance, error) {
	return nil, fmt.Errorf("%w: GLM balance lookup is not documented", adapter.ErrCapabilityUnsupported)
}

func (a *GLMAdapter) ValidateConfig(config *adapter.AdapterConfig) error {
	return validateOpenAIConfig(config)
}

func (a *GLMAdapter) GetProviderInfo() *adapter.ProviderInfo {
	return &adapter.ProviderInfo{
		Name:         "glm",
		Type:         "glm",
		DisplayName:  "GLM",
		Description:  "Zhipu GLM models",
		BaseURL:      a.baseURL,
		Capabilities: []string{"chat", "stream", "embedding", "image", "rerank", "model_listing"},
		Version:      "paas/v4",
	}
}

func (a *GLMAdapter) fetchRemoteModels(ctx context.Context, apiKey string) ([]adapter.Model, error) {
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
			Object  string `json:"object"`
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

func (a *GLMAdapter) documentedModels() []adapter.Model {
	models := make([]adapter.Model, 0, len(glmDocumentedModelOrder))
	for _, id := range glmDocumentedModelOrder {
		models = append(models, a.normalizeModel(id))
	}
	return models
}

func (a *GLMAdapter) normalizeModel(id string) adapter.Model {
	meta, ok := glmDocumentedModelCatalog[id]
	if !ok {
		return inferGLMModel(id)
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

func inferGLMModel(id string) adapter.Model {
	lowerID := strings.ToLower(strings.TrimSpace(id))

	switch {
	case strings.Contains(lowerID, "embedding"):
		return adapter.Model{
			ID:           id,
			Name:         id,
			Type:         "embedding",
			Capabilities: []string{"embedding"},
			Architecture: &adapter.ModelArchitecture{
				Modality:         "embedding",
				InputModalities:  []string{"text"},
				OutputModalities: []string{"embedding"},
			},
		}
	case strings.Contains(lowerID, "rerank"):
		return adapter.Model{
			ID:           id,
			Name:         id,
			Type:         "rerank",
			Capabilities: []string{"rerank"},
			Architecture: &adapter.ModelArchitecture{
				Modality:         "rerank",
				InputModalities:  []string{"text"},
				OutputModalities: []string{"score"},
			},
		}
	case strings.Contains(lowerID, "image") || strings.Contains(lowerID, "cogview"):
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
	case strings.Contains(lowerID, ".6v") || strings.Contains(lowerID, "4v") || strings.Contains(lowerID, "autoglm") || strings.Contains(lowerID, "ocr"):
		return adapter.Model{
			ID:           id,
			Name:         id,
			Type:         "chat",
			Capabilities: []string{"chat", "stream"},
			Architecture: &adapter.ModelArchitecture{
				Modality:         "multimodal",
				InputModalities:  []string{"text", "image", "video", "file"},
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

func glmDocuments(raw interface{}) ([]string, error) {
	switch value := raw.(type) {
	case []string:
		return value, nil
	case []interface{}:
		result := make([]string, 0, len(value))
		for _, item := range value {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%w: GLM rerank only supports string documents", adapter.ErrInvalidRequest)
			}
			result = append(result, text)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("%w: GLM rerank only supports string documents", adapter.ErrInvalidRequest)
	}
}

func shouldFallbackGLMListModels(err error) bool {
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

func (a *GLMAdapter) runtimeHeaders(apiKey string) map[string]string {
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", apiKey),
	}
	for key, value := range a.config.Headers {
		headers[key] = value
	}
	return headers
}
