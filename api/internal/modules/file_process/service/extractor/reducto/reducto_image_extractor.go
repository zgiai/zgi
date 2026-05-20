package reducto

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"
)

type ReductoImageExtractor struct {
	filePath  string
	client    *ReductoClient
	lastUsage *ParseUsage
}

func NewReductoImageExtractor(filePath, apiKey string) *ReductoImageExtractor {
	return &ReductoImageExtractor{
		filePath: filePath,
		client:   NewReductoClient(apiKey),
	}
}

// Extract loads content from an image file path
func (e *ReductoImageExtractor) Extract(ctx context.Context) (*dto.ExtractOutput, error) {
	if _, err := os.Stat(e.filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file does not exist: %s", e.filePath)
	}

	extension := strings.ToLower(filepath.Ext(e.filePath))

	// Check if the file extension is a supported image format
	supportedImageExts := []string{
		".png", ".jpg", ".jpeg", ".gif", ".bmp", ".tiff", ".tif",
		".pcx", ".ppm", ".apng", ".psd", ".cur", ".dcx", ".heic",
	}

	isSupported := false
	for _, ext := range supportedImageExts {
		if extension == ext {
			isSupported = true
			break
		}
	}

	if !isSupported {
		return nil, fmt.Errorf("unsupported image file extension: %s", extension)
	}

	// Use the Reducto API to parse image files
	chunks, usage, err := e.client.ParseFile(ctx, e.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to extract image file: %w", err)
	}

	// Store usage information for later quota recording
	e.lastUsage = usage

	// Create a single document with all extracted content
	var allContent strings.Builder
	for i, chunk := range chunks {
		if i > 0 {
			allContent.WriteString("\n")
		}
		allContent.WriteString(chunk.Content)
	}

	content := strings.TrimSpace(allContent.String())
	if content == "" {
		return nil, fmt.Errorf("no content extracted from image file")
	}

	documents := []dto.Document{
		{
			PageContent: content,
			Metadata: map[string]interface{}{
				"source": e.filePath,
				"type":   "image",
			},
		},
	}

	return dto.NewExtractOutputFromDocumentsWithType("reducto:parse", "figure", documents), nil
}

// GetLastUsage returns the usage information from the last parse operation
func (e *ReductoImageExtractor) GetLastUsage() *ParseUsage {
	return e.lastUsage
}
