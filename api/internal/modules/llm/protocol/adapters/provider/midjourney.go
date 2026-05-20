package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

// MidjourneyAdapter adapter for mj-proxy
type MidjourneyAdapter struct {
	config     *adapter.AdapterConfig
	httpClient *adapter.HTTPClient
	baseURL    string
}

// NewMidjourneyAdapter creates a Midjourney adapter
func NewMidjourneyAdapter(config *adapter.AdapterConfig) (*MidjourneyAdapter, error) {
	baseURL := config.BaseURL
	if baseURL == "" {
		return nil, fmt.Errorf("midjourney adapter requires base_url (mj-proxy endpoint)")
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 300 * time.Second // MJ takes time
	}

	return &MidjourneyAdapter{
		config:     config,
		httpClient: adapter.NewHTTPClientWithAuthHook(timeout, config.MaxRetries, config.AuthHook),
		baseURL:    baseURL,
	}, nil
}

func (a *MidjourneyAdapter) Name() string {
	return "midjourney"
}

func (a *MidjourneyAdapter) ValidateConfig(config *adapter.AdapterConfig) error {
	if config.APIKey == "" {
		return fmt.Errorf("midjourney adapter requires api_key (mj-api-secret)")
	}
	return nil
}

func (a *MidjourneyAdapter) GetProviderInfo() *adapter.ProviderInfo {
	return &adapter.ProviderInfo{
		Name:         "midjourney",
		Type:         "midjourney",
		DisplayName:  "Midjourney",
		Capabilities: []string{"image"},
	}
}

// CreateImage executes image generation request
func (a *MidjourneyAdapter) CreateImage(ctx context.Context, request *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	// Check for action (Upscale/Variation)
	if request.AdditionalParameters != nil {
		action, hasAction := request.AdditionalParameters["action"].(string)
		taskID, hasTaskID := request.AdditionalParameters["task_id"].(string)
		index, hasIndex := request.AdditionalParameters["index"].(float64) // JSON numbers are float64

		if hasAction && hasTaskID && hasIndex {
			return a.handleAction(ctx, action, taskID, int(index))
		}
		// CustomID based action
		customID, hasCustomID := request.AdditionalParameters["custom_id"].(string)
		if hasAction && hasTaskID && hasCustomID {
			return a.handleCustomAction(ctx, taskID, customID)
		}
	}

	return a.handleImagine(ctx, request)
}

func (a *MidjourneyAdapter) handleImagine(ctx context.Context, request *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	url := fmt.Sprintf("%s/mj/submit/imagine", a.baseURL)

	// Append aspect ratio to prompt
	prompt := request.Prompt
	if request.Size != "" {
		ar := mapMJSizeToAspectRatio(request.Size)
		if ar != "1:1" {
			prompt = fmt.Sprintf("%s --ar %s", prompt, ar)
		}
	}

	payload := map[string]interface{}{
		"prompt": prompt,
	}

	// Add other params if needed
	if request.AdditionalParameters != nil {
		if botType, ok := request.AdditionalParameters["botType"]; ok {
			payload["botType"] = botType
		}
	}

	headers := a.buildHeaders()
	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "POST", url, headers, payload)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if statusCode != 200 {
		return nil, fmt.Errorf("midjourney api error (status=%d): %s", statusCode, string(respBody))
	}

	var resp struct {
		Code        int    `json:"code"`
		Description string `json:"description"`
		Result      string `json:"result"` // Task ID
	}

	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if resp.Code != 1 && resp.Code != 22 { // 1=Submitted, 22=InQueue? Depends on proxy implementation. Usually 1 is success.
		return nil, fmt.Errorf("midjourney submission failed: %s (code=%d)", resp.Description, resp.Code)
	}

	taskID := resp.Result
	return a.pollTask(ctx, taskID)
}

func (a *MidjourneyAdapter) handleAction(ctx context.Context, action string, taskID string, index int) (*adapter.ImageResponse, error) {
	// Map action + index to customId or use simple action endpoint if supported
	// Many mj-proxy support /mj/submit/action or /mj/submit/simple-change
	// Let's assume /mj/submit/simple-change for U1, V1, etc.
	
	url := fmt.Sprintf("%s/mj/submit/simple-change", a.baseURL)
	
	content := fmt.Sprintf("%s%d", strings.ToUpper(string(action[0])), index) // e.g. U1, V2
	
	payload := map[string]interface{}{
		"content": content,
		"taskId":  taskID,
	}
	
	headers := a.buildHeaders()
	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "POST", url, headers, payload)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	
	if statusCode != 200 {
		return nil, fmt.Errorf("midjourney action error (status=%d): %s", statusCode, string(respBody))
	}
	
	var resp struct {
		Code        int    `json:"code"`
		Description string `json:"description"`
		Result      string `json:"result"`
	}
	
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	
	if resp.Code != 1 {
		return nil, fmt.Errorf("midjourney action failed: %s", resp.Description)
	}
	
	return a.pollTask(ctx, resp.Result)
}

func (a *MidjourneyAdapter) handleCustomAction(ctx context.Context, taskID string, customID string) (*adapter.ImageResponse, error) {
	url := fmt.Sprintf("%s/mj/submit/action", a.baseURL)
	
	payload := map[string]interface{}{
		"customId": customID,
		"taskId":   taskID,
	}
	
	headers := a.buildHeaders()
	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "POST", url, headers, payload)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	
	if statusCode != 200 {
		return nil, fmt.Errorf("midjourney action error (status=%d): %s", statusCode, string(respBody))
	}
	
	var resp struct {
		Code        int    `json:"code"`
		Description string `json:"description"`
		Result      string `json:"result"`
	}
	
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	
	if resp.Code != 1 {
		return nil, fmt.Errorf("midjourney action failed: %s", resp.Description)
	}
	
	return a.pollTask(ctx, resp.Result)
}

func (a *MidjourneyAdapter) pollTask(ctx context.Context, taskID string) (*adapter.ImageResponse, error) {
	url := fmt.Sprintf("%s/mj/task/%s/fetch", a.baseURL, taskID)
	headers := a.buildHeaders()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	timeout := time.After(600 * time.Second) // MJ can be slow

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			return nil, fmt.Errorf("polling timeout")
		case <-ticker.C:
			respBody, statusCode, err := a.httpClient.DoRequest(ctx, "GET", url, headers, nil)
			if err != nil {
				continue
			}
			if statusCode != 200 {
				return nil, fmt.Errorf("poll failed: %d", statusCode)
			}

			var task struct {
				ID          string `json:"id"`
				Status      string `json:"status"` // SUBMITTED, IN_PROGRESS, SUCCESS, FAILURE
				Progress    string `json:"progress"`
				Description string `json:"description"`
				FailReason  string `json:"failReason"`
				ImageUrl    string `json:"imageUrl"`
				Action      string `json:"action"` // IMAGINE, UPSCALE, VARIATION
			}

			if err := json.Unmarshal(respBody, &task); err != nil {
				continue
			}

			if task.Status == "FAILURE" {
				return nil, fmt.Errorf("task failed: %s", task.FailReason)
			}

			if task.Status == "SUCCESS" {
				// Return result
				return &adapter.ImageResponse{
					Created: time.Now().Unix(),
					Data: []adapter.ImageItem{
						{
							URL:           task.ImageUrl,
							RevisedPrompt: task.Description, // MJ doesn't strictly have revised prompt, use description?
						},
					},
				}, nil
			}
		}
	}
}

func (a *MidjourneyAdapter) buildHeaders() map[string]string {
	return map[string]string{
		"mj-api-secret": a.config.APIKey,
		"Content-Type":  "application/json",
	}
}

func mapMJSizeToAspectRatio(size string) string {
	switch size {
	case "1024x1024", "1:1":
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

// Implement other interface methods
func (a *MidjourneyAdapter) ChatCompletion(ctx context.Context, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (a *MidjourneyAdapter) ChatCompletionStream(ctx context.Context, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (a *MidjourneyAdapter) CreateEmbeddings(ctx context.Context, req *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (a *MidjourneyAdapter) CreateResponse(ctx context.Context, req *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (a *MidjourneyAdapter) Rerank(ctx context.Context, req *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
func (a *MidjourneyAdapter) ListModels(ctx context.Context, apiKey string) ([]adapter.Model, error) {
	return nil, fmt.Errorf("not implemented")
}
func (a *MidjourneyAdapter) GetBalance(ctx context.Context, apiKey string) (*adapter.Balance, error) {
	return nil, fmt.Errorf("not implemented")
}
