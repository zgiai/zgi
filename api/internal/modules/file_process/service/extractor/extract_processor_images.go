package extractor

import (
	"context"
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/file_process/model"
	defaultmodelservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/storage"
)

const fallbackFigureSummary = "Image summary is unavailable."

type VisionSummaryClient interface {
	Chat(ctx context.Context, organizationID string, req *adapter.ChatRequest) (*adapter.ChatResponse, error)
}

type DefaultVisionModelResolver interface {
	ResolveUseCase(ctx context.Context, organizationID string, useCase llmmodelmodel.UseCase, explicitProvider, explicitModel *string) (*defaultmodelservice.ResolvedModel, error)
}

func (p *ExtractProcessor) processFigureElements(ctx context.Context, output *dto.ExtractOutput, uploadFile *model.UploadFile) *dto.ExtractOutput {
	if output == nil || len(output.Elements) == 0 {
		return output
	}

	changed := false
	for i := range output.Elements {
		element := &output.Elements[i]
		if strings.ToLower(strings.TrimSpace(element.Type)) != "figure" {
			continue
		}

		imagePath := extractMarkdownImagePath(element.Content)
		if imagePath == "" {
			imagePath = extractMarkdownImagePath(metadataString(element.Metadata, "markdown"))
		}
		imageKey := metadataString(element.Metadata, "image_key")
		if imageKey == "" {
			imageKey = metadataNestedString(element.Metadata, "payload", "image_key")
		}
		originalImagePath := metadataString(element.Metadata, "original_img_path")
		if originalImagePath == "" {
			originalImagePath = metadataNestedString(element.Metadata, "payload", "original_img_path")
		}
		if originalImagePath == "" {
			originalImagePath = metadataNestedString(element.Metadata, "payload", "img_path")
		}
		if imagePath == "" {
			imagePath = originalImagePath
		}
		if imagePath == "" && imageKey == "" {
			continue
		}

		imageURL := metadataString(element.Metadata, "image_url")
		if imageURL == "" {
			imageURL = metadataNestedString(element.Metadata, "payload", "image_url")
		}
		if imageURL == "" {
			imageURL = displayImageURL(imagePath)
		}
		summary, err := p.summarizeExtractedImage(ctx, imagePath, imageKey, uploadFile)
		if element.Metadata == nil {
			element.Metadata = map[string]any{}
		}
		if err != nil {
			if imageKey != "" || isLocalFilePath(imagePath) {
				logger.WarnContext(ctx, "failed to summarize extracted figure image", "image_key", imageKey, "image_path", imagePath, err)
			}
			element.Metadata["image_summary_error"] = err.Error()
			summary = fallbackFigureSummary
		}

		element.Content = buildFigureContent(imageURL, summary)
		element.Metadata["image_url"] = imageURL
		element.Metadata["image_summary"] = summary
		element.Metadata["original_image_path"] = imagePath
		if imageKey != "" {
			element.Metadata["image_key"] = imageKey
		}
		changed = true
	}

	if changed {
		output.Markdown = extractOutputMarkdownFromElements(output.Elements)
	}
	return output
}

func (p *ExtractProcessor) summarizeExtractedImage(ctx context.Context, imagePath, imageKey string, uploadFile *model.UploadFile) (string, error) {
	if p.imageSummaryClient == nil {
		return "", fmt.Errorf("image summary client is not initialized")
	}
	if uploadFile == nil || strings.TrimSpace(uploadFile.OrganizationID) == "" {
		return "", fmt.Errorf("organization id is required for image summary")
	}

	dataURL := ""
	var storageErr error
	if strings.TrimSpace(imageKey) != "" {
		dataURL, storageErr = imageDataURLFromStorageKey(p.storage, imageKey)
	}
	if dataURL == "" {
		if !isLocalFilePath(imagePath) {
			if storageErr != nil {
				return "", fmt.Errorf("failed to load image from storage key %q: %w", imageKey, storageErr)
			}
			return "", fmt.Errorf("image summary requires an image_key or a local image path")
		}
		localDataURL, err := imageFileDataURL(imagePath)
		if err != nil {
			if storageErr != nil {
				return "", fmt.Errorf("failed to load image from storage key %q: %v; failed to read local image: %w", imageKey, storageErr, err)
			}
			return "", err
		}
		dataURL = localDataURL
	}

	if p.defaultVisionModelResolver == nil {
		return "", fmt.Errorf("default vision model resolver is not initialized")
	}
	resolved, err := p.defaultVisionModelResolver.ResolveUseCase(ctx, uploadFile.OrganizationID, llmmodelmodel.UseCaseVision, nil, nil)
	if err != nil {
		return "", fmt.Errorf("failed to resolve default vision model: %w", err)
	}
	if resolved == nil || strings.TrimSpace(resolved.Model) == "" {
		return "", fmt.Errorf("default vision model is not configured")
	}

	temp := 0.1
	maxTokens := 160
	req := &adapter.ChatRequest{
		Provider: strings.TrimSpace(resolved.Provider),
		Model:    strings.TrimSpace(resolved.Model),
		Messages: []adapter.Message{
			{
				Role: "user",
				Content: []adapter.MessageContentPart{
					{
						Type: "text",
						Text: "Briefly describe the key semantic content of this image. Focus on what charts, screenshots, formulas, tables, or diagrams communicate. Do not invent unseen details. Keep it under 80 words.",
					},
					{
						Type:     "image_url",
						ImageURL: &adapter.ImageURL{URL: dataURL, Detail: "auto"},
					},
				},
			},
		},
		Temperature: &temp,
		MaxTokens:   &maxTokens,
	}

	resp, err := p.imageSummaryClient.Chat(ctx, uploadFile.OrganizationID, req)
	if err != nil {
		return "", fmt.Errorf("vision model request failed: %w", err)
	}
	if resp == nil || len(resp.Choices) == 0 {
		return "", fmt.Errorf("vision model returned no choices")
	}

	summary := strings.TrimSpace(chatContentText(resp.Choices[0].Message.Content))
	if summary == "" {
		return "", fmt.Errorf("vision model returned empty summary")
	}
	return summary, nil
}

func buildFigureContent(imageURL, summary string) string {
	imageURL = strings.TrimSpace(imageURL)
	summary = strings.TrimSpace(summary)
	if summary == "" {
		summary = fallbackFigureSummary
	}
	return fmt.Sprintf("![figure](%s)\n\nImage URL: %s\nImage summary: %s", imageURL, imageURL, summary)
}

func displayImageURL(imagePath string) string {
	imagePath = strings.TrimSpace(imagePath)
	if imagePath == "" {
		return ""
	}
	lower := strings.ToLower(imagePath)
	if strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "data:") ||
		strings.HasPrefix(imagePath, "/console/api/") {
		return imagePath
	}
	return buildMinerUImageURL(imagePath)
}

func buildMinerUImageURL(imagePath string) string {
	return util.GetSignedParserImagePathURL(strings.TrimSpace(imagePath))
}

func extractMarkdownImagePath(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}

	start := strings.Index(content, "](")
	if start >= 0 {
		remaining := content[start+2:]
		end := strings.Index(remaining, ")")
		if end >= 0 {
			return strings.TrimSpace(remaining[:end])
		}
	}

	if isSupportedImagePath(content) {
		return content
	}
	return ""
}

func imageFileDataURL(imagePath string) (string, error) {
	if !isLocalFilePath(imagePath) {
		return "", fmt.Errorf("image path is not a local file path: %s", imagePath)
	}
	data, err := os.ReadFile(imagePath)
	if err != nil {
		return "", fmt.Errorf("failed to read image for summary: %w", err)
	}

	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(imagePath)))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(contentType, "image/") {
		return "", fmt.Errorf("unsupported image content type: %s", contentType)
	}

	return fmt.Sprintf("data:%s;base64,%s", contentType, base64.StdEncoding.EncodeToString(data)), nil
}

func imageDataURLFromStorageKey(store storage.Storage, key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("image storage key is empty")
	}
	if store == nil {
		return "", fmt.Errorf("image storage is not initialized")
	}
	data, err := store.Load(key)
	if err != nil {
		return "", fmt.Errorf("failed to load image from storage: %w", err)
	}
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(key)))
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(contentType, "image/") {
		return "", fmt.Errorf("unsupported image content type: %s", contentType)
	}
	return fmt.Sprintf("data:%s;base64,%s", contentType, base64.StdEncoding.EncodeToString(data)), nil
}

func extractOutputMarkdownFromElements(elements []dto.ExtractElement) string {
	contents := make([]string, 0, len(elements))
	for _, element := range elements {
		if content := strings.TrimSpace(element.Content); content != "" {
			contents = append(contents, content)
		}
	}
	return strings.Join(contents, "\n\n")
}

func chatContentText(content any) string {
	switch value := content.(type) {
	case string:
		return value
	case []adapter.MessageContentPart:
		parts := make([]string, 0, len(value))
		for _, part := range value {
			if text := strings.TrimSpace(part.Text); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return fmt.Sprint(value)
	}
}

func metadataString(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	value, ok := metadata[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return text
}

func metadataNestedString(metadata map[string]any, parentKey, childKey string) string {
	if len(metadata) == 0 {
		return ""
	}
	parent, ok := metadata[parentKey].(map[string]any)
	if !ok {
		return ""
	}
	return metadataString(parent, childKey)
}

func isLocalFilePath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "data:") ||
		strings.HasPrefix(value, "/") && strings.HasPrefix(value, "/console/api/") {
		return false
	}
	return filepath.IsAbs(value) || isWindowsAbsolutePath(value)
}

func isWindowsAbsolutePath(value string) bool {
	return len(value) >= 3 &&
		((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) &&
		value[1] == ':' &&
		(value[2] == '\\' || value[2] == '/')
}

func isSupportedImagePath(value string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(value))) {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".tif", ".tiff":
		return true
	default:
		return false
	}
}
