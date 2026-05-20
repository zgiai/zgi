package handler

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	appconfig "github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/contracts"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/model"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/service"
	"github.com/zgiai/zgi/api/pkg/response"
	"github.com/zgiai/zgi/api/pkg/storage"
)

func persistPlaygroundSourceFile(run *model.PlaygroundRun, exec *playgroundExecution) error {
	if run == nil || exec == nil || len(exec.SourceData) == 0 {
		return nil
	}
	if run.ID == uuid.Nil {
		run.ID = uuid.New()
	}
	store, err := playgroundStorage()
	if err != nil {
		return fmt.Errorf("content parse source storage unavailable: %w", err)
	}
	if run.SourceMimeType == "" {
		run.SourceMimeType = exec.SourceMimeType
	}
	if run.SourceFileExt == "" {
		run.SourceFileExt = exec.SourceFileExt
	}
	run.SourceStorageType = appconfig.Current().Storage.Type
	run.SourceStorageKey = buildPlaygroundSourceStorageKey(run)
	if err := store.Save(run.SourceStorageKey, exec.SourceData); err != nil {
		return fmt.Errorf("save content parse source file: %w", err)
	}
	return nil
}

func cleanupPlaygroundSourceFile(run *model.PlaygroundRun) {
	if run == nil || strings.TrimSpace(run.SourceStorageKey) == "" {
		return
	}
	store, err := playgroundStorage()
	if err != nil {
		return
	}
	_ = store.Delete(run.SourceStorageKey)
}

func playgroundStorage() (store storage.Storage, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%v", recovered)
		}
	}()
	return storage.GetStorage(), nil
}

func buildPlaygroundSourceStorageKey(run *model.PlaygroundRun) string {
	scope := "system"
	if run.WorkspaceID != nil && *run.WorkspaceID != uuid.Nil {
		scope = run.WorkspaceID.String()
	}
	ext := normalizeStoredFileExt(run.SourceFileExt)
	return fmt.Sprintf(
		"content_parse/playground/%s/%s/%s%s",
		scope,
		run.SourceContentHash,
		run.ID.String(),
		ext,
	)
}

func toJSONMap(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	if item, ok := value.(map[string]any); ok {
		return item
	}
	data, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]any{}
	}
	if out == nil {
		return map[string]any{}
	}
	return out
}

func failPlaygroundRequest(c *gin.Context, err error) {
	var requestErr *playgroundRequestError
	if errors.As(err, &requestErr) {
		response.FailWithMessage(c, requestErr.code, requestErr.Error())
		return
	}
	response.FailWithMessage(c, response.ErrSystemError, err.Error())
}

func newPlaygroundRequestError(code response.ErrorCode, message string) error {
	return &playgroundRequestError{code: code, err: errors.New(message)}
}

func readMultipartFile(fileHeader *multipart.FileHeader) ([]byte, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()

	limited := io.LimitReader(file, playgroundMaxFileSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(data) > playgroundMaxFileSize {
		return nil, errors.New("file size cannot exceed 64MB")
	}
	if len(data) == 0 {
		return nil, errors.New("uploaded file is empty")
	}
	return data, nil
}

func detectPlaygroundSourceMimeType(fileHeader *multipart.FileHeader, data []byte) string {
	if fileHeader != nil {
		if contentType := strings.TrimSpace(fileHeader.Header.Get("Content-Type")); contentType != "" {
			return contentType
		}
	}
	if len(data) > 0 {
		return http.DetectContentType(data)
	}
	return "application/octet-stream"
}

func normalizePlaygroundFileExt(fileName, mimeType string) string {
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(fileName)))
	if ext != "" {
		return normalizeStoredFileExt(ext)
	}
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "application/pdf":
		return ".pdf"
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "image/bmp":
		return ".bmp"
	case "image/tiff":
		return ".tiff"
	default:
		return ""
	}
}

func normalizeStoredFileExt(ext string) string {
	ext = strings.ToLower(strings.TrimSpace(ext))
	if ext == "" {
		return ""
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	if len(ext) > 16 {
		return ""
	}
	for _, r := range ext[1:] {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return ""
		}
	}
	return ext
}

func isPlaygroundPDF(fileName, mimeType string) bool {
	return strings.EqualFold(strings.TrimSpace(mimeType), "application/pdf") ||
		strings.EqualFold(filepath.Ext(fileName), ".pdf")
}

func dataURL(mimeType string, data []byte) string {
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data))
}

func buildPlaygroundShareURL(c *gin.Context, token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	origin := strings.TrimRight(c.GetHeader("Origin"), "/")
	if origin == "" {
		scheme := "http"
		if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		host := c.Request.Host
		if forwardedHost := strings.TrimSpace(c.GetHeader("X-Forwarded-Host")); forwardedHost != "" {
			host = forwardedHost
		}
		if host != "" {
			origin = scheme + "://" + host
		}
	}
	if origin == "" {
		return ""
	}
	return origin + "/console/developer/content-parse?share=" + token
}

func parseProfile(value string) contracts.ParseProfile {
	switch contracts.ParseProfile(strings.TrimSpace(value)) {
	case contracts.ParseProfileHighQuality,
		contracts.ParseProfileFast,
		contracts.ParseProfileLocalFirst,
		contracts.ParseProfileDefault,
		contracts.ParseProfileFastPreview,
		contracts.ParseProfileLayoutFirst,
		contracts.ParseProfileTextFirst,
		contracts.ParseProfileDatasetIndex:
		return contracts.ParseProfile(strings.TrimSpace(value))
	default:
		return contracts.ParseProfileAuto
	}
}

func parseIntent(value string) contracts.ParseIntent {
	switch contracts.ParseIntent(strings.TrimSpace(value)) {
	case contracts.ParseIntentDatasetIndex, contracts.ParseIntentChatContext:
		return contracts.ParseIntent(strings.TrimSpace(value))
	default:
		return contracts.ParseIntentPreview
	}
}

func parsePositiveInt(value string) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 1 {
		return 0
	}
	return parsed
}

func parseListLimit(value string, fallback int, max int) int {
	limit := fallback
	if parsed := parsePositiveInt(value); parsed > 0 {
		limit = parsed
	}
	if limit <= 0 {
		limit = 20
	}
	if max > 0 && limit > max {
		return max
	}
	return limit
}

func parseContextUUID(c *gin.Context, keys ...string) *uuid.UUID {
	for _, key := range keys {
		raw := strings.TrimSpace(c.GetString(key))
		if raw == "" {
			value, exists := c.Get(key)
			if !exists || value == nil {
				continue
			}
			raw = strings.TrimSpace(fmt.Sprint(value))
		}
		if raw == "" {
			continue
		}
		id, err := uuid.Parse(raw)
		if err == nil {
			return &id
		}
	}
	return nil
}

func playgroundRunScope(c *gin.Context) service.PlaygroundRunListFilter {
	scope := service.PlaygroundRunListFilter{
		WorkspaceID: parseContextUUID(c, "workspace_id", "tenant_id"),
		AccountID:   parseContextUUID(c, "account_id"),
	}
	if scope.WorkspaceID == nil && scope.AccountID == nil && c.GetBool(contentParseInternalRouteKey) {
		scope.AllowUnscoped = true
	}
	return scope
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func fileSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
