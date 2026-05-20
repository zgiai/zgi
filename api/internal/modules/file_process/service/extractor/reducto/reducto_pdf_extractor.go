package reducto

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"
)

type ReductoPDFExtractor struct {
	filePath  string
	client    *ReductoClient
	lastUsage *ParseUsage
}

func NewReductoPDFExtractor(filePath, apiKey string) *ReductoPDFExtractor {
	return &ReductoPDFExtractor{
		filePath: filePath,
		client:   NewReductoClient(apiKey),
	}
}

// Extract loads content from a PDF file path
func (e *ReductoPDFExtractor) Extract(ctx context.Context) (*dto.ExtractOutput, error) {
	if _, err := os.Stat(e.filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file does not exist: %s", e.filePath)
	}

	extension := strings.ToLower(filepath.Ext(e.filePath))

	// Check if the file extension is supported
	if extension != ".pdf" && extension != ".doc" {
		return nil, fmt.Errorf("unsupported file extension: %s", extension)
	}

	// Use the Reducto API to parse PDF files
	chunks, usage, err := e.client.ParseFile(ctx, e.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract file: %w", err)
	}

	// Store usage information for later quota recording
	// This will be accessed by the extract processor
	e.lastUsage = usage

	// Group chunks by page
	pageMap := e.client.GetTextByPage(chunks)

	// Sort pages
	pages := make([]int, 0, len(pageMap))
	for page := range pageMap {
		pages = append(pages, page)
	}
	sort.Ints(pages)

	// Create documents from pages
	documents := make([]dto.Document, 0, len(pages))
	for _, page := range pages {
		pageChunks := pageMap[page]

		// Combine all chunks for this page
		var pageContent strings.Builder
		for i, chunk := range pageChunks {
			if i > 0 {
				pageContent.WriteString("\n")
			}
			// Use content field (which contains the extracted text)
			pageContent.WriteString(chunk.Content)
		}

		content := strings.TrimSpace(pageContent.String())
		if content != "" {
			documents = append(documents, dto.Document{
				PageContent: content,
				Metadata: map[string]interface{}{
					"source": e.filePath,
					"page":   page,
				},
			})
		}
	}

	// If no page-grouped documents, create a single document with all content
	if len(documents) == 0 && len(chunks) > 0 {
		var allContent strings.Builder
		for i, chunk := range chunks {
			if i > 0 {
				allContent.WriteString("\n")
			}
			allContent.WriteString(chunk.Content)
		}

		content := strings.TrimSpace(allContent.String())
		if content != "" {
			documents = append(documents, dto.Document{
				PageContent: content,
				Metadata:    map[string]interface{}{"source": e.filePath},
			})
		}
	}

	return dto.NewExtractOutputFromDocuments("reducto:parse", documents), nil
}

// GetLastUsage returns the usage information from the last parse operation
func (e *ReductoPDFExtractor) GetLastUsage() *ParseUsage {
	return e.lastUsage
}
