package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	appconfig "github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/model"
	"github.com/zgiai/zgi/api/internal/observability"
)

// Config holds the configuration for the plugin runner client
type Config struct {
	BaseURL string        // Base URL of the plugin runner service (e.g., http://localhost:2665)
	APIKey  string        // API key for authentication
	Timeout time.Duration // Request timeout
}

// NewConfigFromEnv keeps the legacy name for compatibility, but now reads
// plugin runner settings from the loaded application config.
func NewConfigFromEnv() *Config {
	runnerCfg := appconfig.Current().PluginRunner
	baseURL := runnerCfg.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:2665"
	}

	apiKey := runnerCfg.APIKey

	timeout := 30 * time.Second
	if runnerCfg.Timeout > 0 {
		timeout = time.Duration(runnerCfg.Timeout) * time.Second
	}

	return &Config{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Timeout: timeout,
	}
}

// Client is the HTTP client for interacting with the plugin runner service
type Client struct {
	cfg        *Config
	httpClient *http.Client
}

// NewClient creates a new plugin runner client
func NewClient(cfg *Config) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	// Ensure BaseURL doesn't end with slash
	cfg.BaseURL = strings.TrimSuffix(cfg.BaseURL, "/")

	return &Client{
		cfg: cfg,
		httpClient: observability.HTTPClient(&http.Client{
			Timeout: cfg.Timeout,
		}),
	}
}

// RequestOption allows customizing individual requests
type RequestOption func(*http.Request)

// WithTenantID sets the X-Tenant-ID header
func WithTenantID(tenantID string) RequestOption {
	return func(req *http.Request) {
		req.Header.Set("X-Tenant-ID", tenantID)
	}
}

// WithActor sets the X-Actor header
func WithActor(actor string) RequestOption {
	return func(req *http.Request) {
		req.Header.Set("X-Actor", actor)
	}
}

// WithWorkflowRunID sets the workflow run identifier for session lifecycle tracking.
func WithWorkflowRunID(workflowRunID string) RequestOption {
	return func(req *http.Request) {
		if strings.TrimSpace(workflowRunID) == "" {
			return
		}
		req.Header.Set("X-Workflow-Run-ID", workflowRunID)
	}
}

// WithSessionPolicy sets the session policy header.
func WithSessionPolicy(policy string) RequestOption {
	return func(req *http.Request) {
		if strings.TrimSpace(policy) == "" {
			return
		}
		req.Header.Set("X-Session-Policy", policy)
	}
}

// WithSessionIdleTTLSeconds sets the session idle TTL in seconds.
func WithSessionIdleTTLSeconds(seconds int) RequestOption {
	return func(req *http.Request) {
		if seconds <= 0 {
			return
		}
		req.Header.Set("X-Session-Idle-TTL-Seconds", strconv.Itoa(seconds))
	}
}

// WithSessionMaxLifetimeSeconds sets the session max lifetime in seconds.
func WithSessionMaxLifetimeSeconds(seconds int) RequestOption {
	return func(req *http.Request) {
		if seconds <= 0 {
			return
		}
		req.Header.Set("X-Session-Max-Lifetime-Seconds", strconv.Itoa(seconds))
	}
}

// doRequest performs an HTTP request and decodes the response
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}, result interface{}, opts ...RequestOption) error {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	url := c.cfg.BaseURL + path

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set default headers
	if c.cfg.APIKey != "" {
		req.Header.Set("X-API-Key", c.cfg.APIKey)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	// Apply request options
	for _, opt := range opts {
		opt(req)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for error responses
	if resp.StatusCode >= 400 {
		var errResp model.ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error != "" {
			return fmt.Errorf("plugin runner error (status %d): %s", resp.StatusCode, errResp.Error)
		}
		return fmt.Errorf("plugin runner error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Decode response if result is provided
	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// doMultipartRequest performs a multipart form request
func (c *Client) doMultipartRequest(ctx context.Context, path string, fieldName string, fileContent []byte, fileName string, formFields map[string]string, result interface{}, opts ...RequestOption) error {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add file field
	part, err := writer.CreateFormFile(fieldName, fileName)
	if err != nil {
		return fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(fileContent); err != nil {
		return fmt.Errorf("failed to write file content: %w", err)
	}

	// Add other form fields
	for key, value := range formFields {
		if err := writer.WriteField(key, value); err != nil {
			return fmt.Errorf("failed to write form field %s: %w", key, err)
		}
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close multipart writer: %w", err)
	}

	url := c.cfg.BaseURL + path

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	if c.cfg.APIKey != "" {
		req.Header.Set("X-API-Key", c.cfg.APIKey)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	// Apply request options
	for _, opt := range opts {
		opt(req)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for error responses
	if resp.StatusCode >= 400 {
		var errResp model.ErrorResponse
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error != "" {
			return fmt.Errorf("plugin runner error (status %d): %s", resp.StatusCode, errResp.Error)
		}
		return fmt.Errorf("plugin runner error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Decode response if result is provided
	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// ============================================
// Health Check
// ============================================

// Health checks the health of the plugin runner service
func (c *Client) Health(ctx context.Context) (*model.HealthResponse, error) {
	var result model.HealthResponse
	if err := c.doRequest(ctx, http.MethodGet, "/healthz", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// IsHealthy returns true if the plugin runner service is healthy
func (c *Client) IsHealthy(ctx context.Context) bool {
	resp, err := c.Health(ctx)
	return err == nil && resp != nil && resp.Status == "ok"
}
