package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
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
		httpClient: adapter.NewHTTPClientWithAuthHook(timeout, 3, config.AuthHook),
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

// ChatCompletion executes chat completion request
func (a *AliyunAdapter) ChatCompletion(ctx context.Context, request *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	openaiAdapter, err := a.openAICompatibleAdapter()
	if err != nil {
		return nil, err
	}
	return openaiAdapter.ChatCompletion(ctx, request)
}

// ChatCompletionStream executes streaming chat completion request
func (a *AliyunAdapter) ChatCompletionStream(ctx context.Context, request *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	openaiAdapter, err := a.openAICompatibleAdapter()
	if err != nil {
		return nil, err
	}
	return openaiAdapter.ChatCompletionStream(ctx, request)
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
