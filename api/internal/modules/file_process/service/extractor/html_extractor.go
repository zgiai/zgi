package extractor

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/zgiai/zgi/api/internal/dto"
)

type HtmlExtractor struct {
	filePath string
}

func NewHtmlExtractor(filePath string) *HtmlExtractor {
	return &HtmlExtractor{
		filePath: filePath,
	}
}

func (e *HtmlExtractor) Extract(ctx context.Context) (*dto.ExtractOutput, error) {
	text, err := e.loadAsText()
	if err != nil {
		return nil, fmt.Errorf("error loading %s: %w", e.filePath, err)
	}

	metadata := map[string]interface{}{"source": e.filePath}
	documents := []dto.Document{
		{
			PageContent: text,
			Metadata:    metadata,
		},
	}
	return dto.NewExtractOutputFromDocuments("zgi:html", documents), nil
}

func (e *HtmlExtractor) loadAsText() (string, error) {
	// Check if file exists
	if _, err := os.Stat(e.filePath); os.IsNotExist(err) {
		return "", fmt.Errorf("file does not exist: %s", e.filePath)
	}

	// Open file
	file, err := os.Open(e.filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Parse HTML with goquery
	doc, err := goquery.NewDocumentFromReader(file)
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML: %w", err)
	}

	// Remove script and style elements
	doc.Find("script, style").Remove()

	// Extract text content
	text := doc.Text()

	// Trim whitespace
	result := strings.TrimSpace(text)
	return result, nil
}
