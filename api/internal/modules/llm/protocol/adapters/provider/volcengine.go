package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/credentials"
	v4 "github.com/aws/aws-sdk-go/aws/signer/v4"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

const defaultVolcengineVisualBaseURL = "https://visual.volcengineapi.com"

// VolcengineAdapter adapter for Volcengine Visual CV APIs.
type VolcengineAdapter struct {
	config     *adapter.AdapterConfig
	httpClient *adapter.HTTPClient
	baseURL    string
	ak         string
	sk         string
	region     string
	service    string
}

// NewVolcengineAdapter creates a CV-only Volcengine adapter.
func NewVolcengineAdapter(config *adapter.AdapterConfig) (*VolcengineAdapter, error) {
	if config == nil {
		return nil, adapter.ErrInvalidConfig
	}

	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = defaultVolcengineVisualBaseURL
	}

	ak, _ := config.ProviderConfig["access_key"].(string)
	sk, _ := config.ProviderConfig["secret_key"].(string)
	region, _ := config.ProviderConfig["region"].(string)
	if region == "" {
		region = "cn-north-1"
	}
	service := "cv"

	// Allow a compact `access_key|secret_key` api_key format because the current
	// channel contract only exposes a single credential field.
	if ak == "" || sk == "" {
		parts := strings.Split(config.APIKey, "|")
		if len(parts) == 2 {
			ak = strings.TrimSpace(parts[0])
			sk = strings.TrimSpace(parts[1])
		}
	}

	if ak == "" || sk == "" {
		return nil, fmt.Errorf("%w: volcengine visual requires access_key and secret_key", adapter.ErrInvalidConfig)
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}

	return &VolcengineAdapter{
		config:     config,
		httpClient: adapter.NewHTTPClientWithAuthHook(timeout, config.MaxRetries, config.AuthHook),
		baseURL:    baseURL,
		ak:         ak,
		sk:         sk,
		region:     region,
		service:    service,
	}, nil
}

func (a *VolcengineAdapter) Name() string {
	return "volcengine"
}

func (a *VolcengineAdapter) ValidateConfig(config *adapter.AdapterConfig) error {
	_, err := NewVolcengineAdapter(config)
	return err
}

func (a *VolcengineAdapter) GetProviderInfo() *adapter.ProviderInfo {
	return &adapter.ProviderInfo{
		Name:         "volcengine",
		Type:         "volcengine",
		DisplayName:  "Volcengine",
		Description:  "Volcengine Visual CV APIs",
		BaseURL:      a.baseURL,
		Capabilities: []string{"image"},
		Version:      "visual",
	}
}

// CreateImage executes a signed Volcengine Visual CV request.
func (a *VolcengineAdapter) CreateImage(ctx context.Context, request *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	if strings.HasPrefix(request.Model, "doubao-seedream") {
		return nil, fmt.Errorf("%w: doubao seedream models belong to the doubao adapter", adapter.ErrCapabilityUnsupported)
	}
	if a.ak == "" || a.sk == "" {
		return nil, fmt.Errorf("volcengine cv image generation requires access_key and secret_key")
	}

	// Default to the documented generic Visual action.
	action := "CVProcess"
	version := "2022-08-31"

	reqKey := request.Model

	payload := map[string]interface{}{
		"req_key": reqKey,
		"prompt":  request.Prompt,
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/?Action=%s&Version=%s", a.baseURL, action, version)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	// Sign request
	creds := credentials.NewStaticCredentials(a.ak, a.sk, "")
	signer := v4.NewSigner(creds)
	_, err = signer.Sign(req, bytes.NewReader(bodyBytes), a.service, a.region, time.Now())
	if err != nil {
		return nil, fmt.Errorf("failed to sign request: %w", err)
	}

	// Use httpClient to execute (copy headers)
	headers := make(map[string]string)
	for k, v := range req.Header {
		headers[k] = v[0]
	}

	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "POST", url, headers, payload)
	if err != nil {
		return nil, err
	}

	if statusCode != 200 {
		return nil, fmt.Errorf("volcengine error: %d %s", statusCode, string(respBody))
	}

	// Parse response
	var resp struct {
		Code int `json:"code"`
		Data *struct {
			Status           string `json:"status"`             // "success"?
			Result           string `json:"result"`             // Base64? URL?
			BinaryDataPrefix string `json:"binary_data_base64"` // Some return this
		} `json:"data"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if resp.Code != 10000 { // 10000 is usually success
		return nil, fmt.Errorf("volcengine api error: %s", resp.Message)
	}

	// Extract image
	if resp.Data == nil {
		return nil, fmt.Errorf("no data in response")
	}

	return &adapter.ImageResponse{
		Created: time.Now().Unix(),
		Data: []adapter.ImageItem{
			{
				URL: resp.Data.Result,
			},
		},
	}, nil
}

func (a *VolcengineAdapter) ChatCompletion(context.Context, *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return nil, fmt.Errorf("%w: volcengine visual does not support chat", adapter.ErrCapabilityUnsupported)
}

func (a *VolcengineAdapter) ChatCompletionStream(context.Context, *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, fmt.Errorf("%w: volcengine visual does not support streaming chat", adapter.ErrCapabilityUnsupported)
}

func (a *VolcengineAdapter) CreateEmbeddings(context.Context, *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, fmt.Errorf("%w: volcengine visual does not support embeddings", adapter.ErrCapabilityUnsupported)
}

func (a *VolcengineAdapter) CreateResponse(context.Context, *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, fmt.Errorf("%w: volcengine visual does not support responses", adapter.ErrCapabilityUnsupported)
}

func (a *VolcengineAdapter) Rerank(context.Context, *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, fmt.Errorf("%w: volcengine visual does not support rerank", adapter.ErrCapabilityUnsupported)
}

func (a *VolcengineAdapter) ListModels(context.Context, string) ([]adapter.Model, error) {
	return nil, fmt.Errorf("%w: volcengine visual does not expose model listing", adapter.ErrCapabilityUnsupported)
}

func (a *VolcengineAdapter) GetBalance(context.Context, string) (*adapter.Balance, error) {
	return nil, fmt.Errorf("%w: volcengine visual does not expose balance lookup", adapter.ErrCapabilityUnsupported)
}
