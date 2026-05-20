package reducto

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/observability"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// ReductoClient is a client for the Reducto API
type ReductoClient struct {
	apiKey  string
	baseURL string
}

// UploadResponse represents the response from the upload endpoint
type UploadResponse struct {
	FileID       string  `json:"file_id"`
	PresignedURL *string `json:"presigned_url"`
}

// BoundingBox represents the bounding box of a block
type BoundingBox struct {
	Left         float64 `json:"left"`
	Top          float64 `json:"top"`
	Width        float64 `json:"width"`
	Height       float64 `json:"height"`
	Page         int     `json:"page"`
	OriginalPage int     `json:"original_page"`
}

// ParseBlock represents a block in the parsed document
type ParseBlock struct {
	Type       string                 `json:"type"`
	BBox       BoundingBox            `json:"bbox"`
	Content    string                 `json:"content"`
	ImageURL   *string                `json:"image_url"`
	Confidence *string                `json:"confidence"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// ParseChunk represents a chunk of content returned from the API
type ParseChunk struct {
	Content           string       `json:"content"`
	Embed             string       `json:"embed"`
	Enriched          *string      `json:"enriched"`
	EnrichmentSuccess bool         `json:"enrichment_success"`
	Blocks            []ParseBlock `json:"blocks"`
}

// ParseUsage represents usage information
type ParseUsage struct {
	NumPages int      `json:"num_pages"`
	Credits  *float64 `json:"credits"`
}

// FullResult represents the full result from the parse endpoint
type FullResult struct {
	Type   string       `json:"type"` // "full" or "url"
	URL    string       `json:"url,omitempty"`
	Chunks []ParseChunk `json:"chunks"`
}

// ParseResponse represents the response from the parse endpoint
type ParseResponse struct {
	JobID      string     `json:"job_id"`
	Duration   float64    `json:"duration"`
	PDFURL     *string    `json:"pdf_url"`
	StudioLink *string    `json:"studio_link"`
	Usage      ParseUsage `json:"usage"`
	Result     FullResult `json:"result"`
}

// NewReductoClient creates a new Reducto client
func NewReductoClient(apiKey string) *ReductoClient {
	return &ReductoClient{
		apiKey:  apiKey,
		baseURL: "https://platform.reducto.ai",
	}
}

// UploadFile uploads a file to Reducto and returns the file_id
func (c *ReductoClient) UploadFile(ctx context.Context, filePath string) (*UploadResponse, error) {
	// Request a presigned URL instead of direct multipart upload
	ext := filepath.Ext(filePath)
	ext = strings.TrimPrefix(ext, ".")
	if ext == "" {
		ext = "pdf" // default to pdf if no extension
	}

	reqBody := map[string]string{
		"extension": ext,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal upload request: %w", err)
	}

	// Step 1: Request presigned URL
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/upload", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	transport := &http.Transport{
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 2 * time.Minute,
		ExpectContinueTimeout: 5 * time.Second,
	}

	client := &http.Client{
		Transport: observability.HTTPTransport(transport),
		Timeout:   2 * time.Minute,
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.Warn("Reducto upload request failed (network issue): %v", err)
		return nil, fmt.Errorf("reducto upload unavailable (network error): %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Warn("Reducto upload failed",
			"status", resp.StatusCode,
			"body_bytes", len(body),
		)
		return nil, fmt.Errorf("upload failed with status %d", resp.StatusCode)
	}

	var uploadResp UploadResponse
	err = json.NewDecoder(resp.Body).Decode(&uploadResp)
	if err != nil {
		return nil, fmt.Errorf("failed to decode upload response: %w", err)
	}

	// Step 1.5: Upload to presigned URL if provided
	if uploadResp.PresignedURL != nil && *uploadResp.PresignedURL != "" {
		logger.Info("Reducto upload returned presigned URL, uploading file to it...")
		err = c.uploadToPresignedURL(ctx, *uploadResp.PresignedURL, filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to upload to presigned URL: %w", err)
		}
	} else {
		return nil, fmt.Errorf("reducto failed to return a presigned url for upload")
	}

	return &uploadResp, nil
}

func (c *ReductoClient) uploadToPresignedURL(ctx context.Context, presignedURL, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file for presigned upload: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", presignedURL, file)
	if err != nil {
		return fmt.Errorf("failed to create PUT request: %w", err)
	}

	req.ContentLength = stat.Size()

	client := observability.HTTPClient(&http.Client{
		Timeout: 5 * time.Minute,
	})

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute PUT request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("presigned upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// ParseFile processes a file using the Reducto API (two-step: upload then parse)
// Returns chunks, usage information, and error
func (c *ReductoClient) ParseFile(ctx context.Context, filePath string) ([]ParseChunk, *ParseUsage, error) {
	// Step 1: Upload file
	uploadResp, err := c.UploadFile(ctx, filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to upload file: %w", err)
	}

	logger.Info("File uploaded to Reducto, file_id: %s", uploadResp.FileID)

	// Step 2: Parse the uploaded file
	logger.Info("Starting Reducto parse for file_id: %s", uploadResp.FileID)
	chunks, usage, err := c.ParseByFileID(ctx, uploadResp.FileID)
	if err != nil {
		logger.Error("Reducto parse failed: %v", err)
		return nil, nil, err
	}
	logger.Info("Reducto parse successful, got %d chunks", len(chunks))
	return chunks, usage, nil
}

// ParseByFileID parses a file that has already been uploaded to Reducto
// Returns chunks, usage information, and error
func (c *ReductoClient) ParseByFileID(ctx context.Context, fileID string) ([]ParseChunk, *ParseUsage, error) {
	// Create parse request body
	inputID := fileID
	if !strings.HasPrefix(fileID, "reducto://") {
		inputID = "reducto://" + fileID
	}

	parseReq := map[string]interface{}{
		"input": inputID,
	}

	reqBody, err := json.Marshal(parseReq)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal parse request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/parse", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create parse request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	transport := &http.Transport{
		TLSHandshakeTimeout:   30 * time.Second,
		ResponseHeaderTimeout: 2 * time.Minute,
		ExpectContinueTimeout: 5 * time.Second,
	}

	client := &http.Client{
		Transport: observability.HTTPTransport(transport),
		Timeout:   10 * time.Minute,
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.Warn("Reducto parse request failed: %v", err)
		return nil, nil, fmt.Errorf("reducto parse unavailable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Warn("Reducto parse failed",
			"status", resp.StatusCode,
			"body_bytes", len(body),
		)
		return nil, nil, fmt.Errorf("parse failed with status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read response body: %w", err)
	}

	logger.Debug("Reducto response read", "body_bytes", len(bodyBytes))

	var parseResp ParseResponse
	err = json.Unmarshal(bodyBytes, &parseResp)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode parse response: %w", err)
	}

	logger.Info("Reducto parse completed, job_id: %s, pages: %d, duration: %.2fs",
		parseResp.JobID, parseResp.Usage.NumPages, parseResp.Duration)

	if parseResp.Result.Type == "url" && parseResp.Result.URL != "" {
		logger.Info("Reducto returned type: url, downloading chunks from S3 presigned URL...")
		chunks, err := c.downloadChunksFromURL(ctx, parseResp.Result.URL)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to download chunks from url: %w", err)
		}
		return chunks, &parseResp.Usage, nil
	}

	return parseResp.Result.Chunks, &parseResp.Usage, nil
}

func (c *ReductoClient) downloadChunksFromURL(ctx context.Context, downloadURL string) ([]ParseChunk, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return nil, err
	}

	client := observability.HTTPClient(&http.Client{Timeout: 5 * time.Minute})
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download chunks, status: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Chunks []ParseChunk `json:"chunks"`
	}
	if err := json.Unmarshal(bodyBytes, &result); err == nil && len(result.Chunks) > 0 {
		return result.Chunks, nil
	}

	var chunks []ParseChunk
	if err := json.Unmarshal(bodyBytes, &chunks); err == nil {
		return chunks, nil
	}

	return nil, fmt.Errorf("failed to decode chunks payload from url")
}

// GetTextByPage groups chunks by page number
func (c *ReductoClient) GetTextByPage(chunks []ParseChunk) map[int][]ParseChunk {
	pageMap := make(map[int][]ParseChunk)

	for _, chunk := range chunks {
		// Get page number from the first block in the chunk
		if len(chunk.Blocks) > 0 {
			page := chunk.Blocks[0].BBox.Page
			pageMap[page] = append(pageMap[page], chunk)
		}
	}

	return pageMap
}
