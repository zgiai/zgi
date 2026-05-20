package unstructured

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zgiai/ginext/internal/dto"
)

type UnstructuredPDFExtractor struct {
	filePath string
	client   *UnstructuredClient
	options  *ExtractorOptions
}

// NewUnstructuredPDFExtractor creates a new PDF extractor
// Configure OCR and formula enhancement options through options parameter
func NewUnstructuredPDFExtractor(filePath, apiURL, apiKey string, options *ExtractorOptions) *UnstructuredPDFExtractor {
	extractor := &UnstructuredPDFExtractor{
		filePath: filePath,
		client:   NewUnstructuredClient(apiURL, apiKey),
		options:  &ExtractorOptions{}, // Default options
	}

	if options != nil {
		extractor.options = options
	}

	return extractor
}

// Extract loads content from a PDF file path
func (e *UnstructuredPDFExtractor) Extract(ctx context.Context) (*dto.ExtractOutput, error) {
	if _, err := os.Stat(e.filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file does not exist: %s", e.filePath)
	}

	extension := strings.ToLower(filepath.Ext(e.filePath))

	// Check if the file extension is supported
	if extension != ".pdf" {
		return nil, fmt.Errorf("unsupported file extension: %s", extension)
	}

	// Use the Unstructured API to parse PDF files with OCR and formula enhancement options
	elements, err := e.client.PartitionFile(ctx, e.filePath, e.options)
	if err != nil {
		return nil, fmt.Errorf("failed to extract file: %w", err)
	}

	sortedPages := e.client.GetSortedTextByPage(elements)

	documents := make([]dto.Document, 0, len(sortedPages))
	for _, page := range sortedPages {
		content := strings.TrimSpace(page.Text)
		if content != "" {
			documents = append(documents, dto.Document{
				PageContent: content,
				Metadata:    map[string]interface{}{"source": e.filePath, "page": page.PageNumber},
			})
		}
	}

	// If no page-grouped text, process all text as a single document
	if len(documents) == 0 && len(elements) > 0 {
		var allText strings.Builder
		for i, element := range elements {
			if i > 0 {
				allText.WriteString("\n")
			}
			allText.WriteString(element.Text)
		}

		content := strings.TrimSpace(allText.String())
		if content != "" {
			documents = append(documents, dto.Document{
				PageContent: content,
				Metadata:    map[string]interface{}{"source": e.filePath},
			})
		}
	}

	return dto.NewExtractOutputFromDocuments("unstructured:partition", documents), nil
}
