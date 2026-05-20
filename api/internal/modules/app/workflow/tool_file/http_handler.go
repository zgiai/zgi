package tool_file

import (
	"context"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/pkg/response"
)

type HTTPHandler struct {
	manager   *ToolFileManager
	signature *FileSignature
}

func NewHTTPHandler(manager *ToolFileManager) *HTTPHandler {
	return &HTTPHandler{
		manager:   manager,
		signature: GlobalFileSignature,
	}
}

func (h *HTTPHandler) GetToolFile(c *gin.Context) {
	if h.manager == nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	rawToolFileID := c.Param("tool_file_id")
	toolFileID := strings.TrimSuffix(rawToolFileID, path.Ext(rawToolFileID))
	if toolFileID == "" {
		response.Fail(c, response.ErrFileIdRequired)
		return
	}

	expiresAt := c.Query("expires_at")
	nonce := c.Query("nonce")
	sign := c.Query("sign")
	if expiresAt != "" {
		if nonce == "" || sign == "" {
			response.Fail(c, response.ErrInvalidParam)
			return
		}

		if !h.verifyExpirySignature(toolFileID, expiresAt, nonce, sign) {
			response.Fail(c, response.ErrFileNotFound)
			return
		}
	} else {
		timestamp := c.Query("timestamp")
		if timestamp == "" || nonce == "" || sign == "" {
			response.Fail(c, response.ErrInvalidParam)
			return
		}

		if !h.verifySignature(toolFileID, timestamp, nonce, sign) {
			response.Fail(c, response.ErrFileNotFound)
			return
		}
	}

	fileData, mimeType, err := h.manager.GetFileBinary(c.Request.Context(), toolFileID)
	if err != nil {
		response.Fail(c, response.ErrFileNotFound)
		return
	}

	contentType := toolFileResponseContentType(mimeType)
	c.Header("Content-Type", contentType)
	if shouldDownloadToolFile(c.Query("download")) {
		filename := toolFileDownloadFilename(c.Request.Context(), h.manager, toolFileID, rawToolFileID)
		c.Header("Content-Disposition", toolFileContentDisposition(filename))
	}
	c.Header("Content-Length", strconv.Itoa(len(fileData)))
	c.Data(http.StatusOK, contentType, fileData)
}

func toolFileResponseContentType(mimeType string) string {
	contentType := strings.TrimSpace(mimeType)
	if contentType == "" {
		return "application/octet-stream"
	}

	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		if strings.Contains(strings.ToLower(contentType), "charset=") {
			return contentType
		}
		if shouldAttachUTF8Charset(contentType) {
			return contentType + "; charset=utf-8"
		}
		return contentType
	}

	if _, ok := params["charset"]; ok {
		return contentType
	}
	if shouldAttachUTF8Charset(mediaType) {
		return mime.FormatMediaType(mediaType, map[string]string{"charset": "utf-8"})
	}
	return contentType
}

func shouldAttachUTF8Charset(mediaType string) bool {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if strings.HasPrefix(mediaType, "text/") {
		return true
	}
	switch mediaType {
	case "application/json", "application/xml", "application/javascript", "application/x-javascript":
		return true
	default:
		return strings.HasSuffix(mediaType, "+json") || strings.HasSuffix(mediaType, "+xml")
	}
}

func shouldDownloadToolFile(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func toolFileDownloadFilename(ctx context.Context, manager *ToolFileManager, toolFileID string, rawToolFileID string) string {
	if manager != nil {
		if toolFile, err := manager.GetToolFileByID(ctx, toolFileID); err == nil && strings.TrimSpace(toolFile.Name) != "" {
			return toolFile.Name
		}
	}
	if filename := path.Base(rawToolFileID); filename != "." && filename != "/" && strings.TrimSpace(filename) != "" {
		return filename
	}
	return toolFileID
}

func toolFileContentDisposition(filename string) string {
	filename = sanitizeDispositionFilename(filename)
	if filename == "" {
		filename = "download"
	}
	fallback := asciiDispositionFallback(filename)
	encoded := url.PathEscape(filename)
	return `attachment; filename="` + fallback + `"; filename*=utf-8''` + encoded
}

func sanitizeDispositionFilename(filename string) string {
	filename = strings.TrimSpace(path.Base(filename))
	filename = strings.ReplaceAll(filename, "\r", "_")
	filename = strings.ReplaceAll(filename, "\n", "_")
	filename = strings.Trim(filename, ". ")
	return filename
}

func asciiDispositionFallback(filename string) string {
	var builder strings.Builder
	for _, r := range filename {
		switch {
		case r == '"' || r == '\\':
			builder.WriteByte('_')
		case r >= 0x20 && r <= 0x7e:
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}
	fallback := strings.TrimSpace(builder.String())
	if fallback == "" {
		return "download"
	}
	return fallback
}

func (h *HTTPHandler) verifySignature(toolFileID, timestamp, nonce, sign string) bool {
	if h.signature != nil {
		return h.signature.VerifyToolFileSignature(toolFileID, timestamp, nonce, sign)
	}

	if cfg := config.GlobalConfig; cfg != nil {
		return NewFileSignature(cfg).VerifyToolFileSignature(toolFileID, timestamp, nonce, sign)
	}

	return false
}

func (h *HTTPHandler) verifyExpirySignature(toolFileID, expiresAt, nonce, sign string) bool {
	if h.signature != nil {
		return h.signature.VerifyToolFileSignatureWithExpiry(toolFileID, expiresAt, nonce, sign)
	}

	if cfg := config.GlobalConfig; cfg != nil {
		return NewFileSignature(cfg).VerifyToolFileSignatureWithExpiry(toolFileID, expiresAt, nonce, sign)
	}

	return false
}
