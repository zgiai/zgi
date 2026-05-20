package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

const (
	doubaoDefaultBaseURL = "https://ark.cn-beijing.volces.com/api/v3"

	doubaoImageGenerationsPath             = "images/generations"
	doubaoImagePayloadKeyModel             = "model"
	doubaoImagePayloadKeyPrompt            = "prompt"
	doubaoImagePayloadKeySize              = "size"
	doubaoImagePayloadKeyN                 = "n"
	doubaoImagePayloadKeyQuality           = "quality"
	doubaoImagePayloadKeyStyle             = "style"
	doubaoImagePayloadKeyResponseFormat    = "response_format"
	doubaoImagePayloadKeyUser              = "user"
	doubaoSeedreamModelPrefix              = "doubao-seedream"
	doubaoSeedreamSequentialGenerationKey  = "sequential_image_generation"
	doubaoSeedreamSequentialOptionsKey     = "sequential_image_generation_options"
	doubaoSeedreamSequentialMaxImagesKey   = "max_images"
	doubaoSeedreamSequentialGenerationAuto = "auto"
	doubaoSeedreamMultiImagePromptFormat   = "%s\n\nGenerate exactly %d images as separate image results."
)

// DoubaoAdapter implements the documented Ark API endpoints for Doubao.
type DoubaoAdapter struct {
	config     *adapter.AdapterConfig
	httpClient *adapter.HTTPClient
	baseURL    string
	openAI     *OpenAIAdapter
}

// NewDoubaoAdapter creates a Doubao adapter backed by Ark's OpenAI-compatible APIs.
func NewDoubaoAdapter(config *adapter.AdapterConfig) (*DoubaoAdapter, error) {
	if err := validateOpenAIConfig(config); err != nil {
		return nil, err
	}

	baseURL := strings.TrimSpace(config.BaseURL)
	if baseURL == "" {
		baseURL = doubaoDefaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	openAIAdapter, err := newOpenAIAdapterWithOverrides(config, baseURL)
	if err != nil {
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

	return &DoubaoAdapter{
		config:     config,
		httpClient: adapter.NewHTTPClientWithAuthHook(timeout, maxRetries, config.AuthHook),
		baseURL:    baseURL,
		openAI:     openAIAdapter,
	}, nil
}

func (a *DoubaoAdapter) ValidateConfig(config *adapter.AdapterConfig) error {
	return validateOpenAIConfig(config)
}

func (a *DoubaoAdapter) GetProviderInfo() *adapter.ProviderInfo {
	return &adapter.ProviderInfo{
		Name:         "doubao",
		Type:         "doubao",
		DisplayName:  "Doubao",
		Description:  "ByteDance Ark Doubao models",
		BaseURL:      a.baseURL,
		Capabilities: []string{"chat", "stream", "responses", "embedding", "image"},
		Version:      "api/v3",
	}
}

func (a *DoubaoAdapter) ChatCompletion(ctx context.Context, request *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return a.openAI.ChatCompletion(ctx, request)
}

func (a *DoubaoAdapter) ChatCompletionStream(ctx context.Context, request *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return a.openAI.ChatCompletionStream(ctx, request)
}

func (a *DoubaoAdapter) CreateResponse(ctx context.Context, request *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return a.openAI.CreateResponse(ctx, request)
}

func (a *DoubaoAdapter) CreateResponseRaw(ctx context.Context, request *adapter.RawResponseRequest) (*adapter.RawResponse, error) {
	return rawOpenAIResponseRequest(ctx, a.httpClient, a.baseURL, a.runtimeHeaders(a.config.APIKey), request, a.openAI.handleError)
}

func (a *DoubaoAdapter) CreateResponseStream(ctx context.Context, request *adapter.RawResponseRequest) (<-chan adapter.RawStreamEvent, error) {
	return rawOpenAIResponseStream(ctx, a.httpClient, a.baseURL, a.runtimeHeaders(a.config.APIKey), request)
}

func (a *DoubaoAdapter) CreateEmbeddings(ctx context.Context, request *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return a.openAI.CreateEmbeddings(ctx, request)
}

func (a *DoubaoAdapter) CreateImage(ctx context.Context, request *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return createDoubaoArkImage(ctx, a.httpClient, a.runtimeHeaders(a.config.APIKey), a.baseURL, request)
}

func (a *DoubaoAdapter) Rerank(context.Context, *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, fmt.Errorf("%w: doubao rerank is not documented by Ark", adapter.ErrCapabilityUnsupported)
}

func (a *DoubaoAdapter) ListModels(ctx context.Context, apiKey string) ([]adapter.Model, error) {
	url := fmt.Sprintf("%s/models", a.baseURL)
	headers := a.runtimeHeaders(apiKey)

	respBody, statusCode, err := a.httpClient.DoRequest(ctx, http.MethodGet, url, headers, nil)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if statusCode != http.StatusOK {
		adapterErr := a.openAI.handleError(statusCode, respBody)
		if shouldTreatDoubaoModelListingAsUnsupported(adapterErr) {
			return nil, fmt.Errorf("%w: doubao upstream does not expose /models", adapter.ErrCapabilityUnsupported)
		}
		return nil, adapterErr
	}

	var response struct {
		Data []struct {
			ID      string `json:"id"`
			Created int64  `json:"created"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("%w: failed to parse doubao model list", adapter.ErrCapabilityUnsupported)
	}

	models := make([]adapter.Model, 0, len(response.Data))
	for _, item := range response.Data {
		model := normalizeDoubaoModel(item.ID)
		model.Created = item.Created
		model.OwnedBy = item.OwnedBy
		models = append(models, model)
	}
	return models, nil
}

func (a *DoubaoAdapter) GetBalance(context.Context, string) (*adapter.Balance, error) {
	return nil, fmt.Errorf("%w: doubao balance lookup is not documented by Ark", adapter.ErrCapabilityUnsupported)
}

func (a *DoubaoAdapter) runtimeHeaders(apiKey string) map[string]string {
	token := strings.TrimSpace(apiKey)
	if token == "" {
		token = strings.TrimSpace(a.config.APIKey)
	}

	headers := map[string]string{}
	if token != "" {
		headers["Authorization"] = fmt.Sprintf("Bearer %s", token)
	}
	for key, value := range a.config.Headers {
		headers[key] = value
	}
	return headers
}

func shouldTreatDoubaoModelListingAsUnsupported(err error) bool {
	if err == nil {
		return false
	}

	var adapterErr *adapter.AdapterError
	if errors.As(err, &adapterErr) {
		switch adapterErr.StatusCode {
		case http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusNotImplemented:
			return true
		}
	}

	return strings.Contains(strings.ToLower(err.Error()), "failed to parse")
}

func normalizeDoubaoModel(id string) adapter.Model {
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
	case strings.Contains(lowerID, "seedream"), strings.Contains(lowerID, "t2i"), strings.Contains(lowerID, "image"):
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
	case strings.Contains(lowerID, "vision"), strings.Contains(lowerID, "-vl"), strings.Contains(lowerID, "video"):
		return adapter.Model{
			ID:           id,
			Name:         id,
			Type:         "chat",
			Capabilities: []string{"chat", "stream", "responses"},
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
			Capabilities: []string{"chat", "stream", "responses"},
			Architecture: &adapter.ModelArchitecture{
				Modality:         "text",
				InputModalities:  []string{"text"},
				OutputModalities: []string{"text"},
			},
		}
	}
}

func createDoubaoArkImage(
	ctx context.Context,
	httpClient *adapter.HTTPClient,
	headers map[string]string,
	baseURL string,
	request *adapter.ImageRequest,
) (*adapter.ImageResponse, error) {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = doubaoDefaultBaseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	size := request.Size
	isSeedream := isDoubaoSeedreamImageModel(request.Model)
	if isSeedream {
		size = normalizeDoubaoSeedreamSize(request.Size)
	}

	imageCount := 1
	if request.N != nil && *request.N > 0 {
		imageCount = *request.N
	}
	prompt := buildDoubaoSeedreamImagePrompt(request.Prompt, imageCount, isSeedream)

	payload := map[string]any{
		doubaoImagePayloadKeyModel:  request.Model,
		doubaoImagePayloadKeyPrompt: prompt,
		doubaoImagePayloadKeySize:   size,
		doubaoImagePayloadKeyN:      imageCount,
	}
	if isSeedream && imageCount > 1 {
		payload[doubaoSeedreamSequentialGenerationKey] = doubaoSeedreamSequentialGenerationAuto
		payload[doubaoSeedreamSequentialOptionsKey] = map[string]any{
			doubaoSeedreamSequentialMaxImagesKey: imageCount,
		}
	}
	if request.Quality != "" {
		payload[doubaoImagePayloadKeyQuality] = request.Quality
	}
	if request.Style != "" {
		payload[doubaoImagePayloadKeyStyle] = request.Style
	}
	if request.ResponseFormat != "" {
		payload[doubaoImagePayloadKeyResponseFormat] = request.ResponseFormat
	}
	if request.User != "" {
		payload[doubaoImagePayloadKeyUser] = request.User
	}
	for key, value := range request.AdditionalParameters {
		payload[key] = value
	}

	url := fmt.Sprintf("%s/%s", baseURL, doubaoImageGenerationsPath)
	respBody, statusCode, err := httpClient.DoRequest(ctx, http.MethodPost, url, headers, payload)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if statusCode != http.StatusOK {
		var openAIErr OpenAIAdapter
		return nil, openAIErr.handleError(statusCode, respBody)
	}

	var response adapter.ImageResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return &response, nil
}

func buildDoubaoSeedreamImagePrompt(prompt string, imageCount int, isSeedream bool) string {
	if !isSeedream || imageCount <= 1 {
		return prompt
	}
	return fmt.Sprintf(doubaoSeedreamMultiImagePromptFormat, prompt, imageCount)
}

func isDoubaoSeedreamImageModel(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), doubaoSeedreamModelPrefix)
}

func normalizeDoubaoSeedreamSize(size string) string {
	if strings.TrimSpace(size) == "" {
		return "2048x2048"
	}

	w, h, ok := parseWxH(size)
	if !ok || w <= 0 || h <= 0 {
		return size
	}

	const minPixels int64 = 3686400
	current := int64(w) * int64(h)
	if current >= minPixels {
		return size
	}

	scale := math.Sqrt(float64(minPixels) / float64(current))
	nw := int(math.Ceil(float64(w) * scale))
	nh := int(math.Ceil(float64(h) * scale))

	nw = roundUpToMultiple(nw, 64)
	nh = roundUpToMultiple(nh, 64)

	if int64(nw)*int64(nh) < minPixels {
		nw = roundUpToMultiple(int(math.Ceil(float64(nw)*1.01)), 64)
		nh = roundUpToMultiple(int(math.Ceil(float64(nh)*1.01)), 64)
	}

	return fmt.Sprintf("%dx%d", nw, nh)
}

func parseWxH(s string) (int, int, bool) {
	parts := strings.Split(strings.TrimSpace(s), "x")
	if len(parts) != 2 {
		return 0, 0, false
	}
	w, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, false
	}
	h, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, false
	}
	return w, h, true
}

func roundUpToMultiple(v, m int) int {
	if m <= 0 {
		return v
	}
	if v%m == 0 {
		return v
	}
	return ((v / m) + 1) * m
}
