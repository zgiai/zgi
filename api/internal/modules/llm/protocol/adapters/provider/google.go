package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/storage"
)

const defaultGoogleGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// GoogleAdapter supports Gemini text generation and Vertex image generation.
// Which capability is enabled depends on the provided base_url / provider_config.
type GoogleAdapter struct {
	config        *adapter.AdapterConfig
	httpClient    *adapter.HTTPClient
	geminiBaseURL string
	imageBaseURL  string
	geminiEnabled bool
	imageEnabled  bool
}

type googleGeminiPart struct {
	Text string `json:"text,omitempty"`
}

type googleGeminiContent struct {
	Role  string             `json:"role,omitempty"`
	Parts []googleGeminiPart `json:"parts"`
}

type googleGeminiRequest struct {
	Contents          []googleGeminiContent `json:"contents"`
	SystemInstruction *googleGeminiContent  `json:"systemInstruction,omitempty"`
	GenerationConfig  map[string]any        `json:"generationConfig,omitempty"`
}

type googleGeminiCandidate struct {
	Index        int                 `json:"index"`
	Content      googleGeminiContent `json:"content"`
	FinishReason string              `json:"finishReason"`
}

type googleGeminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type googleGeminiResponse struct {
	ResponseID    string                  `json:"responseId"`
	ModelVersion  string                  `json:"modelVersion"`
	Candidates    []googleGeminiCandidate `json:"candidates"`
	UsageMetadata googleGeminiUsage       `json:"usageMetadata"`
}

type googleGeminiErrorResponse struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

type googleGeminiListModelsResponse struct {
	Models []googleGeminiModel `json:"models"`
}

type googleGeminiModel struct {
	Name                       string   `json:"name"`
	DisplayName                string   `json:"displayName"`
	Description                string   `json:"description"`
	InputTokenLimit            int      `json:"inputTokenLimit"`
	OutputTokenLimit           int      `json:"outputTokenLimit"`
	SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
}

// NewGoogleAdapter creates a Google adapter.
func NewGoogleAdapter(config *adapter.AdapterConfig) (*GoogleAdapter, error) {
	if err := validateGoogleConfig(config); err != nil {
		return nil, err
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	baseURL := normalizeGoogleURL(config.BaseURL)
	imageBaseURL := ""
	geminiBaseURL := ""
	imageEnabled := false
	geminiEnabled := false

	switch {
	case baseURL != "":
		if isGoogleVertexModelsBaseURL(baseURL) {
			imageBaseURL = baseURL
			imageEnabled = true
		} else {
			geminiBaseURL = baseURL
			geminiEnabled = true
		}
	default:
		if derived := deriveGoogleImageBaseURL(config); derived != "" {
			imageBaseURL = derived
			imageEnabled = true
		} else {
			geminiBaseURL = defaultGoogleGeminiBaseURL
			geminiEnabled = true
		}
	}

	return &GoogleAdapter{
		config:        config,
		httpClient:    adapter.NewHTTPClientWithAuthHook(timeout, config.MaxRetries, config.AuthHook),
		geminiBaseURL: geminiBaseURL,
		imageBaseURL:  imageBaseURL,
		geminiEnabled: geminiEnabled,
		imageEnabled:  imageEnabled,
	}, nil
}

func validateGoogleConfig(config *adapter.AdapterConfig) error {
	if config == nil {
		return adapter.ErrInvalidConfig
	}
	if config.APIKey == "" && config.AuthHook == nil {
		return fmt.Errorf("%w: API key is required (or set AuthHook for custom auth)", adapter.ErrInvalidConfig)
	}
	return nil
}

func normalizeGoogleURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func deriveGoogleImageBaseURL(config *adapter.AdapterConfig) string {
	if config == nil || config.ProviderConfig == nil {
		return ""
	}
	project, _ := config.ProviderConfig["project_id"].(string)
	location, _ := config.ProviderConfig["location"].(string)
	if project == "" || location == "" {
		return ""
	}
	return fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s/publishers/google/models", location, project, location)
}

func isGoogleVertexModelsBaseURL(baseURL string) bool {
	return strings.Contains(baseURL, "-aiplatform.googleapis.com") || strings.Contains(baseURL, "/publishers/google/models")
}

func (a *GoogleAdapter) Name() string {
	return "google"
}

func (a *GoogleAdapter) ValidateConfig(config *adapter.AdapterConfig) error {
	return validateGoogleConfig(config)
}

func (a *GoogleAdapter) GetProviderInfo() *adapter.ProviderInfo {
	return &adapter.ProviderInfo{
		Name:         "google",
		Type:         "google",
		DisplayName:  "Google Gemini / Vertex AI",
		Capabilities: []string{"chat", "stream", "model_listing", "image", "video"},
	}
}

// ChatCompletion executes Gemini generateContent requests.
func (a *GoogleAdapter) ChatCompletion(ctx context.Context, request *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	if !a.geminiEnabled || a.geminiBaseURL == "" {
		return nil, fmt.Errorf("%w: text generation requires Gemini API configuration", adapter.ErrCapabilityUnsupported)
	}

	geminiReq, err := a.buildGeminiRequest(request)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/%s:generateContent", a.geminiBaseURL, normalizeGeminiModelName(request.Model))
	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "POST", url, a.buildGeminiHeaders(), geminiReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if statusCode != 200 {
		return nil, a.handleGeminiError(statusCode, respBody)
	}

	var geminiResp googleGeminiResponse
	if err := json.Unmarshal(respBody, &geminiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if len(geminiResp.Candidates) == 0 {
		return nil, fmt.Errorf("google gemini returned no candidates")
	}

	choices := make([]adapter.Choice, 0, len(geminiResp.Candidates))
	for i, candidate := range geminiResp.Candidates {
		choices = append(choices, adapter.Choice{
			Index: resolveGoogleCandidateIndex(candidate.Index, i),
			Message: adapter.Message{
				Role:    "assistant",
				Content: joinGoogleGeminiText(candidate.Content.Parts),
			},
			FinishReason: convertGoogleFinishReason(candidate.FinishReason),
		})
	}

	return &adapter.ChatResponse{
		ID:      geminiResp.ResponseID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   resolveGoogleResponseModel(geminiResp.ModelVersion, request.Model),
		Choices: choices,
		Usage:   googleUsageToAdapter(geminiResp.UsageMetadata),
	}, nil
}

// ChatCompletionStream executes Gemini streamGenerateContent requests.
func (a *GoogleAdapter) ChatCompletionStream(ctx context.Context, request *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	if !a.geminiEnabled || a.geminiBaseURL == "" {
		return nil, fmt.Errorf("%w: streaming text generation requires Gemini API configuration", adapter.ErrCapabilityUnsupported)
	}

	geminiReq, err := a.buildGeminiRequest(request)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/%s:streamGenerateContent?alt=sse", a.geminiBaseURL, normalizeGeminiModelName(request.Model))
	resp, err := a.httpClient.DoStreamRequest(ctx, "POST", url, a.buildGeminiHeaders(), geminiReq)
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
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}()

		var lastUsage *adapter.Usage

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
					respChan <- adapter.StreamResponse{
						Done:  true,
						Usage: lastUsage,
					}
					return
				}

				var chunk googleGeminiResponse
				if err := json.Unmarshal([]byte(data), &chunk); err != nil {
					respChan <- adapter.StreamResponse{
						Error: fmt.Errorf("failed to parse stream data: %w", err),
						Done:  true,
						Usage: lastUsage,
					}
					return
				}

				if usage := googleUsageToAdapter(chunk.UsageMetadata); usage != nil {
					lastUsage = usage
				}

				choices := make([]adapter.StreamChoice, 0, len(chunk.Candidates))
				for i, candidate := range chunk.Candidates {
					text := joinGoogleGeminiText(candidate.Content.Parts)
					finishReason := convertGoogleFinishReason(candidate.FinishReason)
					if text == "" && finishReason == "" {
						continue
					}
					choices = append(choices, adapter.StreamChoice{
						Index: resolveGoogleCandidateIndex(candidate.Index, i),
						Delta: adapter.Message{
							Role:    "assistant",
							Content: text,
						},
						FinishReason: finishReason,
					})
				}

				if len(choices) == 0 && lastUsage == nil {
					continue
				}

				respChan <- adapter.StreamResponse{
					ID:      chunk.ResponseID,
					Object:  "chat.completion.chunk",
					Created: time.Now().Unix(),
					Model:   resolveGoogleResponseModel(chunk.ModelVersion, request.Model),
					Choices: choices,
					Usage:   googleUsageToAdapter(chunk.UsageMetadata),
				}
			}
		}
	}()

	return respChan, nil
}

// CreateImage executes Vertex image generation requests.
func (a *GoogleAdapter) CreateImage(ctx context.Context, request *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	if !a.imageEnabled || a.imageBaseURL == "" {
		return nil, fmt.Errorf("%w: image generation requires Vertex AI configuration", adapter.ErrCapabilityUnsupported)
	}

	endpoint := "predict"
	if strings.Contains(strings.ToLower(request.Model), "veo") {
		endpoint = "predictLongRunning"
	}

	url := fmt.Sprintf("%s/%s:%s", a.imageBaseURL, request.Model, endpoint)
	payload := a.buildImagePayload(request)
	headers := a.buildImageHeaders()

	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "POST", url, headers, payload)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if statusCode != 200 {
		return nil, fmt.Errorf("google image api error (status=%d): %s", statusCode, string(respBody))
	}
	if endpoint == "predictLongRunning" {
		return a.handleLRO(ctx, respBody)
	}

	return a.handlePredictResponse(respBody)
}

func (a *GoogleAdapter) buildGeminiRequest(request *adapter.ChatRequest) (*googleGeminiRequest, error) {
	if request == nil {
		return nil, fmt.Errorf("%w: request is required", adapter.ErrInvalidRequest)
	}
	if strings.TrimSpace(request.Model) == "" {
		return nil, fmt.Errorf("%w: model is required", adapter.ErrInvalidRequest)
	}

	systemParts := make([]string, 0, len(request.Messages))
	contents := make([]googleGeminiContent, 0, len(request.Messages))

	for _, message := range request.Messages {
		text := extractGoogleTextContent(message.Content)
		if strings.TrimSpace(text) == "" {
			continue
		}

		switch normalizeGoogleMessageRole(message.Role) {
		case "system":
			systemParts = append(systemParts, text)
		case "model":
			contents = appendGoogleContent(contents, "model", text)
		default:
			contents = appendGoogleContent(contents, "user", text)
		}
	}

	if len(contents) == 0 {
		return nil, fmt.Errorf("%w: at least one non-system message is required", adapter.ErrInvalidRequest)
	}

	req := &googleGeminiRequest{
		Contents: contents,
	}
	if len(systemParts) > 0 {
		req.SystemInstruction = &googleGeminiContent{
			Parts: []googleGeminiPart{{Text: strings.Join(systemParts, "\n\n")}},
		}
	}

	generationConfig := make(map[string]any)
	if request.Temperature != nil {
		generationConfig["temperature"] = *request.Temperature
	}
	if request.TopP != nil {
		generationConfig["topP"] = *request.TopP
	}
	if request.MaxTokens != nil {
		generationConfig["maxOutputTokens"] = *request.MaxTokens
	}
	if len(request.Stop) > 0 {
		generationConfig["stopSequences"] = request.Stop
	}
	if request.N != nil && *request.N > 0 {
		generationConfig["candidateCount"] = *request.N
	}
	if request.ResponseFormat != nil {
		switch request.ResponseFormat.Type {
		case "json_object":
			generationConfig["responseMimeType"] = "application/json"
			if len(request.ResponseFormat.Schema) > 0 {
				generationConfig["responseSchema"] = request.ResponseFormat.Schema
			}
		case "text":
			generationConfig["responseMimeType"] = "text/plain"
		}
	}
	for key, value := range request.AdditionalParameters {
		generationConfig[key] = value
	}
	if len(generationConfig) > 0 {
		req.GenerationConfig = generationConfig
	}

	return req, nil
}

func appendGoogleContent(contents []googleGeminiContent, role, text string) []googleGeminiContent {
	text = strings.TrimSpace(text)
	if text == "" {
		return contents
	}
	part := googleGeminiPart{Text: text}
	if len(contents) > 0 && contents[len(contents)-1].Role == role {
		contents[len(contents)-1].Parts = append(contents[len(contents)-1].Parts, part)
		return contents
	}
	return append(contents, googleGeminiContent{
		Role:  role,
		Parts: []googleGeminiPart{part},
	})
}

func normalizeGoogleMessageRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant", "model":
		return "model"
	case "system":
		return "system"
	default:
		return "user"
	}
}

func extractGoogleTextContent(content interface{}) string {
	switch value := content.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(value)
	case []adapter.MessageContentPart:
		parts := make([]string, 0, len(value))
		for _, part := range value {
			if strings.EqualFold(part.Type, "text") && strings.TrimSpace(part.Text) != "" {
				parts = append(parts, strings.TrimSpace(part.Text))
			}
		}
		return strings.Join(parts, "\n")
	case []interface{}:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			switch typed := item.(type) {
			case string:
				if strings.TrimSpace(typed) != "" {
					parts = append(parts, strings.TrimSpace(typed))
				}
			case map[string]interface{}:
				partType, _ := typed["type"].(string)
				if strings.EqualFold(partType, "text") {
					if text, ok := typed["text"].(string); ok && strings.TrimSpace(text) != "" {
						parts = append(parts, strings.TrimSpace(text))
					}
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		bytes, err := json.Marshal(value)
		if err != nil {
			return ""
		}
		trimmed := strings.TrimSpace(string(bytes))
		if trimmed == "null" || trimmed == `""` {
			return ""
		}
		return trimmed
	}
}

func normalizeGeminiModelName(model string) string {
	model = strings.TrimSpace(model)
	if strings.HasPrefix(model, "models/") {
		return model
	}
	return "models/" + model
}

func resolveGoogleResponseModel(modelVersion, requestedModel string) string {
	modelVersion = strings.TrimSpace(modelVersion)
	if modelVersion == "" {
		return strings.TrimSpace(requestedModel)
	}
	if strings.HasPrefix(modelVersion, "models/") {
		return strings.TrimPrefix(modelVersion, "models/")
	}
	return modelVersion
}

func resolveGoogleCandidateIndex(index, fallback int) int {
	if index == 0 {
		return fallback
	}
	return index
}

func joinGoogleGeminiText(parts []googleGeminiPart) string {
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part.Text) != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "")
}

func convertGoogleFinishReason(reason string) string {
	switch strings.ToUpper(strings.TrimSpace(reason)) {
	case "", "FINISH_REASON_UNSPECIFIED":
		return ""
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY", "RECITATION", "SPII", "PROHIBITED_CONTENT", "BLOCKLIST":
		return "content_filter"
	default:
		return strings.ToLower(reason)
	}
}

func googleUsageToAdapter(usage googleGeminiUsage) *adapter.Usage {
	if usage.PromptTokenCount == 0 && usage.CandidatesTokenCount == 0 && usage.TotalTokenCount == 0 {
		return nil
	}
	total := usage.TotalTokenCount
	if total == 0 {
		total = usage.PromptTokenCount + usage.CandidatesTokenCount
	}
	return &adapter.Usage{
		PromptTokens:     usage.PromptTokenCount,
		CompletionTokens: usage.CandidatesTokenCount,
		TotalTokens:      total,
	}
}

func (a *GoogleAdapter) buildImagePayload(request *adapter.ImageRequest) map[string]interface{} {
	aspectRatio := "1:1"
	if request.Size != "" {
		aspectRatio = mapSizeToAspectRatio(request.Size)
	}

	sampleCount := 1
	if request.N != nil {
		sampleCount = *request.N
	}

	return map[string]interface{}{
		"instances": []map[string]interface{}{
			{
				"prompt": request.Prompt,
			},
		},
		"parameters": map[string]interface{}{
			"sampleCount": sampleCount,
			"aspectRatio": aspectRatio,
		},
	}
}

func mapSizeToAspectRatio(size string) string {
	switch size {
	case "1024x1024", "512x512", "1:1":
		return "1:1"
	case "1024x768", "4:3":
		return "4:3"
	case "768x1024", "3:4":
		return "3:4"
	case "1920x1080", "16:9":
		return "16:9"
	case "1080x1920", "9:16":
		return "9:16"
	default:
		return "1:1"
	}
}

func (a *GoogleAdapter) buildGeminiHeaders() map[string]string {
	headers := map[string]string{}
	if strings.TrimSpace(a.config.APIKey) != "" {
		headers["x-goog-api-key"] = strings.TrimSpace(a.config.APIKey)
	}
	return headers
}

func (a *GoogleAdapter) buildImageHeaders() map[string]string {
	headers := map[string]string{}
	if strings.TrimSpace(a.config.APIKey) != "" {
		headers["Authorization"] = fmt.Sprintf("Bearer %s", strings.TrimSpace(a.config.APIKey))
	}
	return headers
}

func (a *GoogleAdapter) handlePredictResponse(body []byte) (*adapter.ImageResponse, error) {
	var resp struct {
		Predictions []struct {
			BytesBase64Encoded string `json:"bytesBase64Encoded"`
			MimeType           string `json:"mimeType"`
		} `json:"predictions"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	if len(resp.Predictions) == 0 {
		return nil, fmt.Errorf("no predictions returned")
	}

	store := storage.GetStorage()
	if store == nil {
		return nil, fmt.Errorf("storage service not initialized (STORAGE_TYPE not set)")
	}

	imageItems := make([]adapter.ImageItem, 0, len(resp.Predictions))
	for _, pred := range resp.Predictions {
		data, err := base64.StdEncoding.DecodeString(pred.BytesBase64Encoded)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 image: %w", err)
		}

		ext := ".png"
		if pred.MimeType == "image/jpeg" {
			ext = ".jpg"
		}

		filename := fmt.Sprintf("generated/%s/%s%s", time.Now().Format("2006/01/02"), uuid.New().String(), ext)
		if err := store.Save(filename, data); err != nil {
			return nil, fmt.Errorf("failed to save image to storage: %w", err)
		}

		url := filename
		if urlGetter, ok := store.(interface{ URL(string) string }); ok {
			url = urlGetter.URL(filename)
		} else if urlGetter, ok := store.(interface{ GetFileURL(string) string }); ok {
			url = urlGetter.GetFileURL(filename)
		}

		imageItems = append(imageItems, adapter.ImageItem{URL: url})
	}

	return &adapter.ImageResponse{
		Created: time.Now().Unix(),
		Data:    imageItems,
	}, nil
}

func (a *GoogleAdapter) handleLRO(ctx context.Context, body []byte) (*adapter.ImageResponse, error) {
	var lroResp struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &lroResp); err != nil {
		return nil, fmt.Errorf("failed to parse LRO response: %w", err)
	}
	if lroResp.Name == "" {
		return nil, fmt.Errorf("invalid LRO response: missing name")
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	timeout := time.After(300 * time.Second)

	parts := strings.Split(a.imageBaseURL, "/v1/")
	if len(parts) < 1 {
		return nil, fmt.Errorf("invalid base url format for LRO construction")
	}
	host := parts[0]
	opURL := fmt.Sprintf("%s/v1/%s", host, lroResp.Name)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			return nil, fmt.Errorf("polling timeout")
		case <-ticker.C:
			respBody, statusCode, err := a.httpClient.DoRequest(ctx, "GET", opURL, a.buildImageHeaders(), nil)
			if err != nil {
				continue
			}
			if statusCode != 200 {
				return nil, fmt.Errorf("poll failed: %d", statusCode)
			}

			var opResp struct {
				Done     bool                   `json:"done"`
				Response map[string]interface{} `json:"response"`
				Error    *struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(respBody, &opResp); err != nil {
				continue
			}

			if opResp.Error != nil {
				return nil, fmt.Errorf("operation failed: %s", opResp.Error.Message)
			}
			if opResp.Done {
				respBytes, _ := json.Marshal(opResp.Response)
				return a.handlePredictResponse(respBytes)
			}
		}
	}
}

// CreateEmbeddings is not implemented for this adapter family yet.
func (a *GoogleAdapter) CreateEmbeddings(ctx context.Context, req *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, fmt.Errorf("%w: Google embeddings are not implemented in this adapter", adapter.ErrCapabilityUnsupported)
}

// CreateResponse is not implemented for this adapter family yet.
func (a *GoogleAdapter) CreateResponse(ctx context.Context, req *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, fmt.Errorf("%w: Google /responses API is not implemented in this adapter", adapter.ErrCapabilityUnsupported)
}

// Rerank is not implemented for this adapter family yet.
func (a *GoogleAdapter) Rerank(ctx context.Context, req *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, fmt.Errorf("%w: Google rerank is not implemented in this adapter", adapter.ErrCapabilityUnsupported)
}

// ListModels lists Gemini models when the adapter is configured for Gemini API mode.
func (a *GoogleAdapter) ListModels(ctx context.Context, apiKey string) ([]adapter.Model, error) {
	if !a.geminiEnabled || a.geminiBaseURL == "" {
		return nil, fmt.Errorf("%w: model listing requires Gemini API configuration", adapter.ErrCapabilityUnsupported)
	}

	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "GET", fmt.Sprintf("%s/models", a.geminiBaseURL), a.buildGeminiHeaders(), nil)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if statusCode != 200 {
		return nil, a.handleGeminiError(statusCode, respBody)
	}

	var listResp googleGeminiListModelsResponse
	if err := json.Unmarshal(respBody, &listResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	models := make([]adapter.Model, 0, len(listResp.Models))
	for _, item := range listResp.Models {
		id := strings.TrimPrefix(strings.TrimSpace(item.Name), "models/")
		name := strings.TrimSpace(item.DisplayName)
		if name == "" {
			name = id
		}
		capabilities := googleCapabilitiesFromMethods(item.SupportedGenerationMethods)
		modelType := ""
		switch {
		case containsString(capabilities, "chat"):
			modelType = "chat"
		case containsString(capabilities, "embedding"):
			modelType = "embedding"
		}

		endpoints := append([]string(nil), item.SupportedGenerationMethods...)
		sort.Strings(endpoints)

		models = append(models, adapter.Model{
			ID:            id,
			Name:          name,
			Type:          modelType,
			Description:   item.Description,
			ContextLength: item.InputTokenLimit,
			Capabilities:  capabilities,
			Endpoints:     endpoints,
			OwnedBy:       "google",
		})
	}

	return models, nil
}

func googleCapabilitiesFromMethods(methods []string) []string {
	set := map[string]struct{}{}
	for _, method := range methods {
		switch strings.TrimSpace(method) {
		case "generateContent":
			set["chat"] = struct{}{}
			set["stream"] = struct{}{}
		case "embedContent":
			set["embedding"] = struct{}{}
		case "generateImage":
			set["image"] = struct{}{}
		}
	}

	capabilities := make([]string, 0, len(set))
	for capability := range set {
		capabilities = append(capabilities, capability)
	}
	sort.Strings(capabilities)
	return capabilities
}

// GetBalance is not implemented for this adapter family yet.
func (a *GoogleAdapter) GetBalance(ctx context.Context, apiKey string) (*adapter.Balance, error) {
	return nil, fmt.Errorf("%w: Google balance lookup is not implemented in this adapter", adapter.ErrCapabilityUnsupported)
}

func (a *GoogleAdapter) handleGeminiError(statusCode int, body []byte) error {
	var errResp googleGeminiErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		return adapter.HandleNonJSONError(statusCode, body)
	}
	if strings.TrimSpace(errResp.Error.Message) == "" {
		return adapter.HandleNonJSONError(statusCode, body)
	}

	baseErr := adapter.ErrUpstreamError
	code := "GOOGLE_API_ERROR"
	switch statusCode {
	case 400:
		baseErr = adapter.ErrInvalidRequest
		code = "INVALID_REQUEST"
	case 401, 403:
		baseErr = adapter.ErrAuthFailed
		code = "AUTH_FAILED"
	case 404:
		baseErr = adapter.ErrModelNotFound
		code = "MODEL_NOT_FOUND"
	case 408:
		baseErr = adapter.ErrTimeout
		code = "TIMEOUT"
	case 429:
		baseErr = adapter.ErrRateLimited
		code = "RATE_LIMITED"
	default:
		if statusCode >= 500 {
			baseErr = adapter.ErrUpstreamError
			code = "UPSTREAM_ERROR"
		}
	}

	return adapter.NewAdapterError(code, errResp.Error.Message, statusCode, baseErr)
}
