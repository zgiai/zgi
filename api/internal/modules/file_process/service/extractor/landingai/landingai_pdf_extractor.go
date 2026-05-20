package landingai

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"
)

type LandingAIPDFExtractor struct {
	filePath string
	client   *LandingAIClient
}

func NewLandingAIPDFExtractor(filePath, apiKey string) *LandingAIPDFExtractor {
	return &LandingAIPDFExtractor{
		filePath: filePath,
		client:   NewLandingAIClient(apiKey),
	}
}

// Extract loads content from a PDF file path.
func (e *LandingAIPDFExtractor) Extract(ctx context.Context) (*dto.ExtractOutput, error) {
	if _, err := os.Stat(e.filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file does not exist: %s", e.filePath)
	}

	extension := strings.ToLower(filepath.Ext(e.filePath))

	// Check if the file extension is supported
	if extension != ".pdf" {
		return nil, fmt.Errorf("unsupported file extension: %s", extension)
	}

	response, err := e.client.ParseFile(ctx, e.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract file: %w", err)
	}

	return e.toExtractOutput(response), nil
}

func (e *LandingAIPDFExtractor) toExtractOutput(response *LandingAIResponse) *dto.ExtractOutput {
	if response == nil {
		return &dto.ExtractOutput{Source: "landingai:ade"}
	}

	output := &dto.ExtractOutput{
		Elements: make([]dto.ExtractElement, 0, landingAIElementCapacity(response)),
		Markdown: strings.TrimSpace(response.Markdown),
		Source:   "landingai:ade",
		Metadata: copyMetadata(response.Metadata),
	}
	output.Metadata["source"] = e.filePath
	output.Metadata["chunk_count"] = len(response.Chunks)
	output.Metadata["split_count"] = len(response.Splits)

	if len(response.Chunks) > 0 {
		for i, chunk := range response.Chunks {
			content := firstNonEmpty(chunk.Markdown, chunk.Text)
			if content == "" {
				continue
			}
			metadata := copyMetadata(chunk.Metadata)
			if chunk.ID != "" {
				metadata["id"] = chunk.ID
			}
			output.Elements = append(output.Elements, dto.ExtractElement{
				Type:     landingAIElementType(metadata, content),
				Page:     landingAIPage(metadata, i),
				Content:  content,
				Ordinal:  i,
				Metadata: metadata,
			})
		}
		return output
	}

	for i, split := range response.Splits {
		content := firstNonEmpty(split.Markdown, split.Text)
		if content == "" {
			continue
		}
		metadata := copyMetadata(split.Metadata)
		if split.ID != "" {
			metadata["id"] = split.ID
		}
		output.Elements = append(output.Elements, dto.ExtractElement{
			Type:     landingAIElementType(metadata, content),
			Page:     landingAIPage(metadata, i),
			Content:  content,
			Ordinal:  i,
			Metadata: metadata,
		})
	}

	if len(output.Elements) == 0 && output.Markdown != "" {
		output.Elements = append(output.Elements, dto.ExtractElement{
			Type:    "text",
			Page:    0,
			Content: output.Markdown,
			Metadata: map[string]any{
				"source": e.filePath,
			},
		})
	}

	return output
}

func landingAIElementCapacity(response *LandingAIResponse) int {
	if response == nil {
		return 0
	}
	if len(response.Chunks) > 0 {
		return len(response.Chunks)
	}
	if len(response.Splits) > 0 {
		return len(response.Splits)
	}
	if strings.TrimSpace(response.Markdown) != "" {
		return 1
	}
	return 0
}

func copyMetadata(metadata map[string]interface{}) map[string]any {
	copied := make(map[string]any, len(metadata))
	for key, value := range metadata {
		copied[key] = value
	}
	return copied
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func landingAIElementType(metadata map[string]any, content string) string {
	for _, key := range []string{"type", "element_type", "chunk_type"} {
		if value, ok := metadata[key].(string); ok && strings.TrimSpace(value) != "" {
			return normalizeLandingAIType(value)
		}
	}
	if looksLikeMarkdownTable(content) {
		return "table"
	}
	return "text"
}

func normalizeLandingAIType(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "table":
		return "table"
	case "figure", "image", "picture":
		return "figure"
	case "formula", "equation":
		return "formula"
	case "title", "heading", "header":
		return "heading"
	case "list", "list_item":
		return "list"
	case "caption":
		return "caption"
	default:
		return normalized
	}
}

func looksLikeMarkdownTable(content string) bool {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") {
			return true
		}
	}
	return false
}

func landingAIPage(metadata map[string]any, fallback int) int {
	for _, key := range []string{"page", "page_number", "page_index"} {
		if value, ok := metadata[key]; ok {
			switch page := value.(type) {
			case int:
				return page
			case int64:
				return int(page)
			case float64:
				return int(page)
			case float32:
				return int(page)
			}
		}
	}
	return fallback
}
