package unstructured

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"
)

type UnstructuredPPTExtractor struct {
	filePath string
	client   *UnstructuredClient
	options  *ExtractorOptions
}

// NewUnstructuredPPTExtractor creates a new PPT extractor
// Configure OCR and formula enhancement options through options parameter
func NewUnstructuredPPTExtractor(filePath, apiURL, apiKey string, options *ExtractorOptions) *UnstructuredPPTExtractor {
	extractor := &UnstructuredPPTExtractor{
		filePath: filePath,
		client:   NewUnstructuredClient(apiURL, apiKey),
		options:  &ExtractorOptions{}, // Default options
	}

	if options != nil {
		extractor.options = options
	}

	return extractor
}

// Extract loads content from a PPT/PPTX file path
func (e *UnstructuredPPTExtractor) Extract(ctx context.Context) (*dto.ExtractOutput, error) {
	if _, err := os.Stat(e.filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file does not exist: %s", e.filePath)
	}

	extension := strings.ToLower(filepath.Ext(e.filePath))

	// Check if the file extension is supported
	if extension != ".ppt" && extension != ".pptx" {
		return nil, fmt.Errorf("unsupported file extension: %s", extension)
	}

	// Use the Unstructured API to parse PPT/PPTX files
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
