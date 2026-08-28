package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	doubaoDefaultBaseURL = "https://ark.cn-beijing.volces.com/api/v3"

	doubaoImageGenerationsPath             = "images/generations"
	doubaoVideoTasksPath                   = "contents/generations/tasks"
	doubaoImagePayloadKeyModel             = "model"
	doubaoImagePayloadKeyPrompt            = "prompt"
	doubaoVideoPayloadKeyContent           = "content"
	doubaoVideoPayloadKeyResolution        = "resolution"
	doubaoVideoPayloadKeyRatio             = "ratio"
	doubaoVideoPayloadKeyDuration          = "duration"
	doubaoVideoPayloadKeyGenerateAudio     = "generate_audio"
	doubaoVideoPayloadKeyCallbackURL       = "callback_url"
	doubaoVideoPayloadKeyNegativePrompt    = "negative_prompt"
	doubaoVideoContentTypeText             = "text"
	doubaoVideoContentTypeImageURL         = "image_url"
	doubaoVideoContentTypeVideoURL         = "video_url"
	doubaoVideoContentTypeAudioURL         = "audio_url"
	doubaoVideoContentRoleFirstFrame       = "first_frame"
	doubaoVideoContentRoleReferenceImage   = "reference_image"
	doubaoVideoContentRoleLastFrame        = "last_frame"
	doubaoVideoContentRoleReferenceVideo   = "reference_video"
	doubaoVideoContentRoleReferenceAudio   = "reference_audio"
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
		httpClient: adapter.NewHTTPClientFromConfig(config, timeout, maxRetries),
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
		Capabilities: []string{"chat", "stream", "responses", "embedding", "image", "video", "speech_generation", "transcription"},
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

func (a *DoubaoAdapter) CreateVideo(ctx context.Context, request *adapter.VideoRequest) (*adapter.VideoResponse, error) {
	return createDoubaoArkVideo(ctx, a.httpClient, a.runtimeHeaders(a.config.APIKey), a.baseURL, request)
}

func (a *DoubaoAdapter) GetVideoTask(ctx context.Context, request *adapter.VideoTaskRequest) (*adapter.VideoResponse, error) {
	return getDoubaoArkVideoTask(ctx, a.httpClient, a.runtimeHeaders(a.config.APIKey), a.baseURL, request)
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
	case strings.Contains(lowerID, "seedance"):
		return adapter.Model{
			ID:           id,
			Name:         id,
			Type:         "video",
			Capabilities: []string{"video"},
			Architecture: &adapter.ModelArchitecture{
				Modality:         "video",
				InputModalities:  []string{"text", "image", "video", "audio"},
				OutputModalities: []string{"video"},
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

	size := strings.TrimSpace(request.Size)
	isSeedream := isDoubaoSeedreamImageModel(request.Model)
	if isSeedream && size != "" {
		size = normalizeDoubaoSeedreamSize(request.Size)
	}

	payload := map[string]any{
		doubaoImagePayloadKeyModel:  request.Model,
		doubaoImagePayloadKeyPrompt: request.Prompt,
	}
	referenceImageURL := strings.TrimSpace(request.ReferenceImageURL)
	if referenceImageURL != "" {
		if !isSeedream {
			return nil, fmt.Errorf("%w: reference image is only supported for doubao seedream image models", adapter.ErrCapabilityUnsupported)
		}
		payload["image"] = referenceImageURL
	}
	if size != "" {
		payload[doubaoImagePayloadKeySize] = size
	}
	if isSeedream {
		generationMode, maxImages := seedreamGenerationOptions(request)
		switch generationMode {
		case "single":
			payload[doubaoSeedreamSequentialGenerationKey] = "disabled"
		case "sequence":
			payload[doubaoSeedreamSequentialGenerationKey] = doubaoSeedreamSequentialGenerationAuto
			payload[doubaoSeedreamSequentialOptionsKey] = map[string]any{
				doubaoSeedreamSequentialMaxImagesKey: maxImages,
			}
		}
	} else if request.N != nil {
		payload[doubaoImagePayloadKeyN] = *request.N
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

func createDoubaoArkVideo(
	ctx context.Context,
	httpClient *adapter.HTTPClient,
	headers map[string]string,
	baseURL string,
	request *adapter.VideoRequest,
) (*adapter.VideoResponse, error) {
	if request == nil {
		return nil, fmt.Errorf("%w: request is required", adapter.ErrInvalidRequest)
	}
	if strings.TrimSpace(request.Model) == "" {
		return nil, fmt.Errorf("%w: model is required", adapter.ErrInvalidRequest)
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return nil, fmt.Errorf("%w: prompt is required", adapter.ErrInvalidRequest)
	}

	respBody, statusCode, err := httpClient.DoRequest(ctx, http.MethodPost, doubaoArkVideoTaskURL(baseURL), headers, buildDoubaoArkVideoPayload(request))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		var openAIErr OpenAIAdapter
		return nil, openAIErr.handleError(statusCode, respBody)
	}
	response, err := decodeDoubaoArkVideoResponse(respBody)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return response, nil
}

func getDoubaoArkVideoTask(
	ctx context.Context,
	httpClient *adapter.HTTPClient,
	headers map[string]string,
	baseURL string,
	request *adapter.VideoTaskRequest,
) (*adapter.VideoResponse, error) {
	if request == nil {
		return nil, fmt.Errorf("%w: request is required", adapter.ErrInvalidRequest)
	}
	taskID := strings.TrimSpace(request.TaskID)
	if taskID == "" {
		return nil, fmt.Errorf("%w: task_id is required", adapter.ErrInvalidRequest)
	}

	respBody, statusCode, err := httpClient.DoRequest(ctx, http.MethodGet, doubaoArkVideoTaskURL(baseURL)+"/"+url.PathEscape(taskID), headers, nil)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		var openAIErr OpenAIAdapter
		return nil, openAIErr.handleError(statusCode, respBody)
	}
	response, err := decodeDoubaoArkVideoResponse(respBody)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	return response, nil
}

func buildDoubaoArkVideoPayload(request *adapter.VideoRequest) map[string]any {
	payload := map[string]any{
		doubaoImagePayloadKeyModel:   strings.TrimSpace(request.Model),
		doubaoVideoPayloadKeyContent: doubaoArkVideoContent(request),
	}
	if strings.TrimSpace(request.Resolution) != "" {
		payload[doubaoVideoPayloadKeyResolution] = strings.TrimSpace(request.Resolution)
	}
	if strings.TrimSpace(request.Ratio) != "" {
		payload[doubaoVideoPayloadKeyRatio] = strings.TrimSpace(request.Ratio)
	}
	if request.Duration != nil {
		payload[doubaoVideoPayloadKeyDuration] = *request.Duration
	}
	if request.GenerateAudio != nil {
		payload[doubaoVideoPayloadKeyGenerateAudio] = *request.GenerateAudio
	}
	if strings.TrimSpace(request.CallbackURL) != "" {
		payload[doubaoVideoPayloadKeyCallbackURL] = strings.TrimSpace(request.CallbackURL)
	}
	if strings.TrimSpace(request.NegativePrompt) != "" {
		payload[doubaoVideoPayloadKeyNegativePrompt] = strings.TrimSpace(request.NegativePrompt)
	}
	for key, value := range request.AdditionalParameters {
		payload[key] = value
	}
	return payload
}

func doubaoArkVideoContent(request *adapter.VideoRequest) []map[string]any {
	content := make([]map[string]any, 0, 5+len(request.ImageURLs))
	if prompt := strings.TrimSpace(request.Prompt); prompt != "" {
		content = append(content, map[string]any{"type": doubaoVideoContentTypeText, "text": prompt})
	}
	firstFrameURL := strings.TrimSpace(request.FirstFrameURL)
	lastFrameURL := strings.TrimSpace(request.LastFrameURL)
	hasFrameReference := firstFrameURL != "" || lastFrameURL != ""
	if firstFrameURL != "" {
		content = append(content, doubaoArkVideoURLContent(doubaoVideoContentTypeImageURL, firstFrameURL, doubaoVideoContentRoleFirstFrame))
	}
	if !hasFrameReference {
		content = append(content, doubaoReferenceMediaContent(request)...)
	}
	if lastFrameURL != "" {
		content = append(content, doubaoArkVideoURLContent(doubaoVideoContentTypeImageURL, lastFrameURL, doubaoVideoContentRoleLastFrame))
	}
	return content
}

func doubaoReferenceMediaContent(request *adapter.VideoRequest) []map[string]any {
	content := make([]map[string]any, 0, len(request.ReferenceURLs)+len(request.ImageURLs)+2)
	for index, referenceURL := range request.ReferenceURLs {
		switch doubaoReferenceKindAt(request.ReferenceTypes, index, referenceURL) {
		case "video":
			content = append(content, doubaoArkVideoURLContent(doubaoVideoContentTypeVideoURL, referenceURL, doubaoVideoContentRoleReferenceVideo))
		case "audio":
			content = append(content, doubaoArkVideoURLContent(doubaoVideoContentTypeAudioURL, referenceURL, doubaoVideoContentRoleReferenceAudio))
		default:
			content = append(content, doubaoArkVideoURLContent(doubaoVideoContentTypeImageURL, referenceURL, doubaoVideoContentRoleReferenceImage))
		}
	}
	if len(request.ReferenceURLs) > 0 {
		return content
	}
	for _, imageURL := range doubaoReferenceImageURLs(request) {
		content = append(content, doubaoArkVideoURLContent(doubaoVideoContentTypeImageURL, imageURL, doubaoVideoContentRoleReferenceImage))
	}
	if videoURL := strings.TrimSpace(request.VideoURL); videoURL != "" {
		content = append(content, doubaoArkVideoURLContent(doubaoVideoContentTypeVideoURL, videoURL, doubaoVideoContentRoleReferenceVideo))
	}
	if audioURL := strings.TrimSpace(request.AudioURL); audioURL != "" {
		content = append(content, doubaoArkVideoURLContent(doubaoVideoContentTypeAudioURL, audioURL, doubaoVideoContentRoleReferenceAudio))
	}
	return content
}

func doubaoReferenceKindAt(referenceTypes []string, index int, referenceURL string) string {
	if index >= 0 && index < len(referenceTypes) {
		switch strings.ToLower(strings.TrimSpace(referenceTypes[index])) {
		case "image", "video", "audio":
			return strings.ToLower(strings.TrimSpace(referenceTypes[index]))
		}
	}
	return doubaoReferenceKindFromURL(referenceURL)
}

func doubaoReferenceKindFromURL(referenceURL string) string {
	value := strings.ToLower(strings.TrimSpace(referenceURL))
	if value == "" {
		return "image"
	}
	value = strings.Split(value, "?")[0]
	value = strings.Split(value, "#")[0]
	switch {
	case strings.HasSuffix(value, ".mp4"),
		strings.HasSuffix(value, ".mov"),
		strings.HasSuffix(value, ".webm"),
		strings.HasSuffix(value, ".m4v"),
		strings.HasSuffix(value, ".avi"),
		strings.HasSuffix(value, ".mkv"):
		return "video"
	case strings.HasSuffix(value, ".mp3"),
		strings.HasSuffix(value, ".wav"),
		strings.HasSuffix(value, ".m4a"),
		strings.HasSuffix(value, ".aac"),
		strings.HasSuffix(value, ".flac"),
		strings.HasSuffix(value, ".ogg"):
		return "audio"
	default:
		return "image"
	}
}

func doubaoReferenceImageURLs(request *adapter.VideoRequest) []string {
	seen := map[string]struct{}{}
	skippedFrames := map[string]struct{}{}
	if firstFrameURL := strings.TrimSpace(request.FirstFrameURL); firstFrameURL != "" {
		skippedFrames[firstFrameURL] = struct{}{}
	}
	if lastFrameURL := strings.TrimSpace(request.LastFrameURL); lastFrameURL != "" {
		skippedFrames[lastFrameURL] = struct{}{}
	}
	urls := make([]string, 0, len(request.ImageURLs)+1)
	appendURL := func(raw string) {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return
		}
		if _, isFrame := skippedFrames[trimmed]; isFrame {
			return
		}
		if _, exists := seen[trimmed]; exists {
			return
		}
		seen[trimmed] = struct{}{}
		urls = append(urls, trimmed)
	}
	for _, rawURL := range request.ImageURLs {
		appendURL(rawURL)
	}
	appendURL(request.ImageURL)
	return urls
}

func doubaoArkVideoURLContent(contentType string, rawURL string, role string) map[string]any {
	item := map[string]any{
		"type":      contentType,
		contentType: map[string]any{"url": rawURL},
	}
	if role != "" {
		item["role"] = role
	}
	return item
}

func doubaoArkVideoTaskURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = doubaoDefaultBaseURL
	}
	if strings.HasSuffix(baseURL, "/"+doubaoVideoTasksPath) {
		return baseURL
	}
	return baseURL + "/" + doubaoVideoTasksPath
}

func decodeDoubaoArkVideoResponse(body []byte) (*adapter.VideoResponse, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	if upstreamErr := doubaoArkVideoResponseError(raw); upstreamErr != nil {
		return nil, upstreamErr
	}

	var response adapter.VideoResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	response.Raw = raw
	if response.Usage == nil {
		response.Usage = openAIUsageFromRaw(body)
	}
	if response.Created == 0 {
		response.Created = time.Now().Unix()
	}
	if response.TaskID == "" {
		response.TaskID = firstDoubaoVideoStringByKeys(raw, "task_id", "taskId", "id", "name", "operation", "request_id", "requestId")
	}
	if response.Status == "" {
		response.Status = firstDoubaoVideoStringByKeys(raw, "task_status", "taskStatus", "status", "state")
	}
	if response.VideoURL == "" && len(response.Data) > 0 {
		response.VideoURL = response.Data[0].URL
	}
	if response.VideoURL == "" {
		if videoURL := firstDoubaoVideoStringByKeys(raw, "video_url", "videoUrl", "url", "uri"); videoURL != "" {
			response.VideoURL = videoURL
			response.Data = append(response.Data, adapter.VideoItem{URL: videoURL})
		}
	}
	if len(response.Data) == 0 {
		if b64 := firstDoubaoVideoStringByKeys(raw, "b64_json", "b64Json", "bytesBase64Encoded"); b64 != "" {
			response.Data = append(response.Data, adapter.VideoItem{B64JSON: b64})
		}
	}
	return &response, nil
}

func doubaoArkVideoResponseError(raw map[string]interface{}) error {
	if raw == nil {
		return nil
	}
	if errValue, ok := raw["error"]; ok && errValue != nil {
		if message := doubaoArkVideoErrorMessage(errValue); message != "" {
			return fmt.Errorf("upstream error: %s", message)
		}
		return fmt.Errorf("upstream error: %v", errValue)
	}
	if code, ok := raw["code"]; ok && !isDoubaoArkSuccessCode(code) {
		message := firstDoubaoVideoStringByOrderedKeys(raw, "message", "msg", "error_message", "errorMessage")
		if message == "" {
			message = fmt.Sprint(code)
		}
		return fmt.Errorf("upstream error: %s", message)
	}
	return nil
}

func doubaoArkVideoErrorMessage(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]interface{}:
		message := firstDoubaoVideoStringByOrderedKeys(typed, "message", "msg", "error_message", "errorMessage", "code", "type")
		return strings.TrimSpace(message)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func isDoubaoArkSuccessCode(value interface{}) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case float64:
		return typed == 0
	case int:
		return typed == 0
	case string:
		text := strings.TrimSpace(strings.ToLower(typed))
		return text == "" || text == "0" || text == "success" || text == "ok"
	default:
		return false
	}
}
func firstDoubaoVideoStringByOrderedKeys(value interface{}, keys ...string) string {
	for _, key := range keys {
		if text := firstDoubaoVideoStringByKeys(value, key); text != "" {
			return text
		}
	}
	return ""
}
func firstDoubaoVideoStringByKeys(value interface{}, keys ...string) string {
	keySet := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		keySet[key] = struct{}{}
	}
	return firstDoubaoVideoStringByKeySet(value, keySet)
}

func firstDoubaoVideoStringByKeySet(value interface{}, keys map[string]struct{}) string {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, item := range typed {
			if _, ok := keys[key]; ok {
				if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
					return strings.TrimSpace(text)
				}
			}
		}
		for _, item := range typed {
			if text := firstDoubaoVideoStringByKeySet(item, keys); text != "" {
				return text
			}
		}
	case []interface{}:
		for _, item := range typed {
			if text := firstDoubaoVideoStringByKeySet(item, keys); text != "" {
				return text
			}
		}
	}
	return ""
}
func seedreamGenerationOptions(request *adapter.ImageRequest) (string, int) {
	mode := strings.TrimSpace(request.GenerationMode)
	if mode != "" {
		maxImages := 0
		if request.MaxImages != nil {
			maxImages = *request.MaxImages
		}
		return mode, maxImages
	}
	if request.N != nil && *request.N > 1 {
		return "sequence", *request.N
	}
	if request.N != nil {
		return "single", 0
	}
	return "", 0
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
