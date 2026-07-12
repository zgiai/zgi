package imageasset

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	workflowfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	defaultImageMIME = "image/png"
	defaultImageExt  = ".png"
	maxImageBytes    = 20 * 1024 * 1024
)

var unsafeFilenamePattern = regexp.MustCompile(`[^a-zA-Z0-9._\-\p{Han}]`)

type SaveRequest struct {
	TenantID       string
	UserID         string
	ConversationID *string
	Item           adapter.ImageItem
	BaseFilename   string
	Index          int
	Lifecycle      tool_file.ToolFileLifecycle
}

type Service interface {
	SaveGeneratedImage(ctx context.Context, req SaveRequest) (map[string]interface{}, error)
}

type service struct{}

func NewService() Service {
	return service{}
}

func SaveGeneratedImage(ctx context.Context, req SaveRequest) (map[string]interface{}, error) {
	return NewService().SaveGeneratedImage(ctx, req)
}

func (service) SaveGeneratedImage(ctx context.Context, req SaveRequest) (map[string]interface{}, error) {
	tenantID := strings.TrimSpace(req.TenantID)
	userID := strings.TrimSpace(req.UserID)
	if tenantID == "" {
		return nil, fmt.Errorf("tenant id is required")
	}
	if userID == "" {
		return nil, fmt.Errorf("user id is required")
	}
	lifecycle := req.Lifecycle
	if lifecycle == "" {
		lifecycle = tool_file.ToolFileLifecyclePersistent
	}

	var toolFile *tool_file.ToolFile
	var err error
	switch {
	case strings.TrimSpace(req.Item.B64JSON) != "":
		data, decodeErr := decodeBase64Image(req.Item.B64JSON)
		if decodeErr != nil {
			return nil, decodeErr
		}
		_, mimeType, extension, validateErr := validateGeneratedImageData(data, "")
		if validateErr != nil {
			return nil, validateErr
		}
		filename := buildImageFilename(req.BaseFilename, req.Index, extension)
		toolFile, err = tool_file.CreateFileByRawGlobal(ctx, tool_file.CreateFileByRawParams{
			UserID:         userID,
			TenantID:       tenantID,
			ConversationID: req.ConversationID,
			FileData:       data,
			MimeType:       mimeType,
			Filename:       &filename,
			Lifecycle:      lifecycle,
		})
	case strings.TrimSpace(req.Item.URL) != "":
		data, mimeType, extension, downloadErr := downloadGeneratedImage(ctx, strings.TrimSpace(req.Item.URL))
		if downloadErr != nil {
			return nil, downloadErr
		}
		filename := buildImageFilename(req.BaseFilename, req.Index, extension)
		toolFile, err = tool_file.CreateFileByRawGlobal(ctx, tool_file.CreateFileByRawParams{
			UserID:         userID,
			TenantID:       tenantID,
			ConversationID: req.ConversationID,
			FileData:       data,
			MimeType:       mimeType,
			Filename:       &filename,
			Lifecycle:      lifecycle,
		})
	default:
		return nil, fmt.Errorf("image item does not contain url or b64_json")
	}
	if err != nil {
		return nil, err
	}

	extension := toolFile.GetFileExtension()
	if extension == "" {
		extension = extensionFromMIME(toolFile.MimeType)
	}
	if extension == "" {
		extension = defaultImageExt
	}
	url, err := tool_file.SignToolFileGlobal(toolFile.ID, extension)
	if err != nil {
		return nil, fmt.Errorf("failed to sign generated image: %w", err)
	}
	downloadURL := appendDownloadQuery(url)
	mimeType := strings.TrimSpace(toolFile.MimeType)
	if mimeType == "" {
		mimeType = defaultImageMIME
	}
	fileObj := workflowfile.NewFile(
		tenantID,
		workflowfile.FileTypeImage,
		workflowfile.FileTransferMethodToolFile,
		workflowfile.WithID(toolFile.ID),
		workflowfile.WithRelatedID(toolFile.ID),
		workflowfile.WithFilename(toolFile.Name),
		workflowfile.WithExtension(extension),
		workflowfile.WithMimeType(mimeType),
		workflowfile.WithSize(int(toolFile.Size)),
		workflowfile.WithURL(url),
	)
	fileMeta := fileObj.ToDict()
	fileMeta["file_id"] = toolFile.ID
	fileMeta["filename"] = toolFile.Name
	fileMeta["format"] = strings.TrimPrefix(extension, ".")
	fileMeta["mime_type"] = mimeType
	fileMeta["url"] = url
	fileMeta["download_url"] = downloadURL
	return fileMeta, nil
}

func decodeBase64Image(raw string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(raw)
	if err == nil {
		return data, nil
	}
	data, rawErr := base64.RawStdEncoding.DecodeString(raw)
	if rawErr == nil {
		return data, nil
	}
	return nil, fmt.Errorf("failed to decode image base64: %w", err)
}

func downloadGeneratedImage(ctx context.Context, rawURL string) ([]byte, string, string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to create generated image download request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to download generated image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", "", fmt.Errorf("failed to download generated image: status %d", resp.StatusCode)
	}
	if resp.ContentLength > maxImageBytes {
		return nil, "", "", fmt.Errorf("generated image exceeds %d bytes", maxImageBytes)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to read generated image: %w", err)
	}
	return validateGeneratedImageData(data, resp.Header.Get("Content-Type"))
}

func validateGeneratedImageData(data []byte, rawContentType string) ([]byte, string, string, error) {
	if len(data) == 0 {
		return nil, "", "", fmt.Errorf("generated image is empty")
	}
	if len(data) > maxImageBytes {
		return nil, "", "", fmt.Errorf("generated image exceeds %d bytes", maxImageBytes)
	}

	headerMIME := ""
	if rawContentType != "" {
		if parsed, _, err := mime.ParseMediaType(rawContentType); err == nil {
			headerMIME = strings.ToLower(strings.TrimSpace(parsed))
		}
	}
	detected := strings.ToLower(strings.TrimSpace(http.DetectContentType(data)))
	if isSupportedImageMIME(detected) {
		return data, detected, extensionFromMIME(detected), nil
	}
	if isSupportedImageMIME(headerMIME) && detected == "application/octet-stream" {
		return data, headerMIME, extensionFromMIME(headerMIME), nil
	}
	return nil, "", "", fmt.Errorf("generated result is not a supported image: detected=%s content_type=%s", detected, headerMIME)
}

func isSupportedImageMIME(mimeType string) bool {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png", "image/jpeg", "image/jpg", "image/webp":
		return true
	default:
		return false
	}
}

func buildImageFilename(raw string, index int, extension string) string {
	name := sanitizeFilename(raw)
	if name == "" {
		name = "generated-image"
	}
	if index > 0 {
		name = fmt.Sprintf("%s-%d", name, index+1)
	}
	currentExt := filepath.Ext(name)
	if currentExt != "" {
		name = strings.TrimSuffix(name, currentExt)
	}
	return name + extension
}

func sanitizeFilename(raw string) string {
	name := strings.TrimSpace(filepath.Base(raw))
	if name == "." || name == string(filepath.Separator) {
		return ""
	}
	name = unsafeFilenamePattern.ReplaceAllString(name, "_")
	name = strings.Trim(name, "._- ")
	if len(name) > 120 {
		name = name[:120]
	}
	return name
}

func extensionFromMIME(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}

func appendDownloadQuery(rawURL string) string {
	if strings.Contains(rawURL, "?") {
		return rawURL + "&download=1"
	}
	return rawURL + "?download=1"
}
