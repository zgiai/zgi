package unstructured

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
	"sort"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/observability"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// UnstructuredClient is a generic client for the Unstructured API
type UnstructuredClient struct {
	apiURL string
	apiKey string
}

// ExtractorOptions defines common options for all extractors
type ExtractorOptions struct {
	EnableOCR      bool
	EnhanceFormula bool
}

// UnstructuredElement represents an element returned from the API
type UnstructuredElement struct {
	Type     string                 `json:"type"`
	Text     string                 `json:"text"`
	Metadata map[string]interface{} `json:"metadata"`
}

// NewUnstructuredClient creates a new Unstructured client
func NewUnstructuredClient(apiURL, apiKey string) *UnstructuredClient {
	apiURL = strings.TrimSuffix(apiURL, "/general/v0/general")
	return &UnstructuredClient{
		apiURL: apiURL,
		apiKey: apiKey,
	}
}

// PartitionFile processes a file using the Unstructured API
// Configure OCR and formula enhancement options through options parameter
func (c *UnstructuredClient) PartitionFile(ctx context.Context, filePath string, options *ExtractorOptions) ([]UnstructuredElement, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	fileField, err := writer.CreateFormFile("files", filepath.Base(filePath))
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	_, err = io.Copy(fileField, file)
	if err != nil {
		return nil, fmt.Errorf("failed to copy file to form: %w", err)
	}

	// Set options to default values if not provided
	opts := &ExtractorOptions{}
	if options != nil {
		opts = options
	}

	// Set strategy based on OCR setting
	if opts.EnableOCR {
		_ = writer.WriteField("strategy", "hi_res")
	} else {
		_ = writer.WriteField("strategy", "fast")
	}

	// Set languages - always include default languages per Unstructured best practices
	languages := []string{"chi_sim", "eng"}

	// Add formula enhancement language if enabled
	if opts.EnhanceFormula {
		languages = append(languages, "equ")
	}

	// Write all languages
	for _, lang := range languages {
		writer.WriteField("languages", lang)
	}

	// OCR parameters - only when OCR is enabled
	if opts.EnableOCR {
		// other OCR parameters
		_ = writer.WriteField("extract_images_in_pdf", "true")
		_ = writer.WriteField("pdf_infer_table_structure", "true")
	}

	_ = writer.WriteField("include_page_breaks", "true")
	_ = writer.WriteField("skip_infer_table_types", "[]")

	err = writer.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to close writer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.apiURL+"/general/v0/general", &buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	if c.apiKey != "" {
		req.Header.Set("unstructured-api-key", c.apiKey)
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
		logger.Error("Failed to send request:", err)
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		logger.Warn("unstructured API request failed",
			"status", resp.StatusCode,
			"body_bytes", len(body),
		)
		return nil, fmt.Errorf("API request failed with status %d", resp.StatusCode)
	}

	var elements []UnstructuredElement
	err = json.NewDecoder(resp.Body).Decode(&elements)
	if err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return elements, nil
}

// GetTextByPage organizes text content by page number
func (c *UnstructuredClient) GetTextByPage(elements []UnstructuredElement) map[int]string {
	textByPage := make(map[int]string)

	for _, element := range elements {
		var page int
		if pageNum, ok := element.Metadata["page_number"]; ok {
			if pageNumFloat, ok := pageNum.(float64); ok {
				page = int(pageNumFloat)
			} else {
				continue
			}
		} else {
			continue
		}

		text := element.Text
		if existingText, exists := textByPage[page]; exists {
			textByPage[page] = existingText + "\n" + text
		} else {
			textByPage[page] = text
		}
	}

	return textByPage
}

// GetSortedTextByPage organizes text content by page number and returns pages in sorted order
func (c *UnstructuredClient) GetSortedTextByPage(elements []UnstructuredElement) []struct {
	PageNumber int
	Text       string
} {
	textByPage := c.GetTextByPage(elements)

	// Get sorted page numbers
	pages := make([]int, 0, len(textByPage))
	for page := range textByPage {
		pages = append(pages, page)
	}
	sort.Ints(pages)

	// Create sorted result
	result := make([]struct {
		PageNumber int
		Text       string
	}, len(pages))

	for i, page := range pages {
		result[i] = struct {
			PageNumber int
			Text       string
		}{
			PageNumber: page,
			Text:       textByPage[page],
		}
	}

	return result
}
