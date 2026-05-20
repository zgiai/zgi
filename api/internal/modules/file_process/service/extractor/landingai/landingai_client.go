package landingai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/zgiai/ginext/internal/observability"
	"github.com/zgiai/ginext/pkg/logger"
)

// LandingAIClient is a client for the LandingAI ADE API
type LandingAIClient struct {
	apiKey string
}

// LandingAIElement represents an element returned from the API
type LandingAIElement struct {
	Type     string                 `json:"type"`
	Text     string                 `json:"text"`
	Markdown string                 `json:"markdown"`
	Chunks   []LandingAIChunk       `json:"chunks"`
	Metadata map[string]interface{} `json:"metadata"`
}

// LandingAISplit represents a split of content returned from the API
type LandingAISplit struct {
	ID       string                 `json:"id"`
	Text     string                 `json:"text"`
	Markdown string                 `json:"markdown"`
	Metadata map[string]interface{} `json:"metadata"`
}

// LandingAIResponse represents the response from the API
type LandingAIResponse struct {
	Markdown string                 `json:"markdown"`
	Chunks   []LandingAIChunk       `json:"chunks"`
	Splits   []LandingAISplit       `json:"splits"`
	Metadata map[string]interface{} `json:"metadata"`
}

// LandingAIChunk represents a chunk of content returned from the API
type LandingAIChunk struct {
	ID       string                 `json:"id"`
	Text     string                 `json:"text"`
	Markdown string                 `json:"markdown"`
	Metadata map[string]interface{} `json:"metadata"`
}

// NewLandingAIClient creates a new LandingAI client
func NewLandingAIClient(apiKey string) *LandingAIClient {
	return &LandingAIClient{
		apiKey: apiKey,
	}
}

// ParseFile processes a file using the LandingAI ADE API.
func (c *LandingAIClient) ParseFile(ctx context.Context, filePath string) (*LandingAIResponse, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	fileField, err := writer.CreateFormFile("document", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	_, err = io.Copy(fileField, file)
	if err != nil {
		return nil, fmt.Errorf("failed to copy file to form: %w", err)
	}

	// Add split parameter to split documents at the page level
	err = writer.WriteField("split", "page")
	if err != nil {
		return nil, fmt.Errorf("failed to write split field: %w", err)
	}

	err = writer.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to close writer: %w", err)
	}

	// Use the agentic-doc parse endpoint
	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.va.landing.ai/v1/ade/parse", &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	// Create a custom Transport with appropriate timeout settings
	transport := &http.Transport{
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 10 * time.Minute,
		ExpectContinueTimeout: 5 * time.Second,
	}

	client := &http.Client{
		Transport: observability.HTTPTransport(transport),
		Timeout:   10 * time.Minute,
	}
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("Failed to send request:", err)
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Warn("landing AI API request failed",
			"status", resp.StatusCode,
			"body_bytes", len(body),
		)
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	var response LandingAIResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// GetTextByPage groups elements by page
func (c *LandingAIClient) GetTextByPage(elements []LandingAIElement) []LandingAIElement {
	// For simplicity, we're just returning the elements as they are
	// In a more complex implementation, we might group by page number
	return elements
}
