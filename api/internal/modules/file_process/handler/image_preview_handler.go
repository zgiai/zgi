package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/file_process/model"
	file_model "github.com/zgiai/zgi/api/internal/modules/file_process/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/util"
	zgiimage "github.com/zgiai/zgi/api/pkg/image"
	"github.com/zgiai/zgi/api/pkg/response"
	"github.com/zgiai/zgi/api/pkg/storage"
)

// ImagePreviewHandler handles image preview HTTP requests
type ImagePreviewHandler struct {
	fileService       interfaces.FileService
	accountService    interfaces.AccountService
	enterpriseService interfaces.OrganizationService
	storage           storage.Storage
	validator         *validator.Validate
}

// NewImagePreviewHandler creates a new image preview handler instance
func NewImagePreviewHandler(
	fileService interfaces.FileService,
	accountService interfaces.AccountService,
	enterpriseService interfaces.OrganizationService,
	storageClients ...storage.Storage,
) *ImagePreviewHandler {
	var storageClient storage.Storage
	if len(storageClients) > 0 {
		storageClient = storageClients[0]
	}
	return &ImagePreviewHandler{
		fileService:       fileService,
		accountService:    accountService,
		enterpriseService: enterpriseService,
		storage:           storageClient,
		validator:         validator.New(),
	}
}

// GetFilePreview handles file preview requests
// GET /files/:file_id/file-preview
func (h *ImagePreviewHandler) GetFilePreview(c *gin.Context) {
	fileID := c.Param("file_id")
	if fileID == "" {
		response.Fail(c, response.ErrFileIdRequired)
		return
	}

	// Get query parameters
	timestamp := c.Query("timestamp")
	nonce := c.Query("nonce")
	sign := c.Query("sign")
	asAttachmentStr := c.Query("as_attachment")

	hasSignatureParams := timestamp != "" || nonce != "" || sign != ""
	signedAccess := false
	if hasSignatureParams {
		if timestamp == "" || nonce == "" || sign == "" {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
		if !util.VerifyFileSignature(fileID, timestamp, nonce, sign) {
			response.Fail(c, response.ErrFileNotFound) // Return a generic not-found error for security.
			return
		}
		signedAccess = true
	} else if c.GetString("auth_method") != "jwt" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Parse as_attachment parameter
	asAttachment := false
	if asAttachmentStr != "" {
		if parsed, err := strconv.ParseBool(asAttachmentStr); err == nil {
			asAttachment = parsed
		}
	}

	// Get file information
	var uploadFile *dto.UploadFile
	if signedAccess {
		var err error
		uploadFile, err = h.fileService.GetFileByID(c.Request.Context(), fileID)
		if err != nil {
			switch err {
			case file_model.ErrFileNotFound:
				response.Fail(c, response.ErrFileNotFound)
			default:
				response.Fail(c, response.ErrSystemError)
			}
			return
		}
	} else {
		var ok bool
		uploadFile, ok = authorizeFileDownloadAccess(c, h.fileService, h.enterpriseService, fileID)
		if !ok {
			return
		}
	}

	// Get file content
	content, err := h.fileService.DownloadFile(c.Request.Context(), fileID)
	if err != nil {
		switch err {
		case model.ErrFileNotFound:
			response.Fail(c, response.ErrFileNotFound)
		case model.ErrUnsupportedFileType:
			response.Fail(c, response.ErrUnsupportedFileType)
		default:
			response.Fail(c, response.ErrFileDownloadFailed)
		}
		return
	}

	h.writeFilePreview(c, uploadFile, content, asAttachment)
}

func (h *ImagePreviewHandler) writeFilePreview(c *gin.Context, uploadFile *dto.UploadFile, content []byte, asAttachment bool) {
	if !asAttachment && isTextOriginalPreviewFile(uploadFile) {
		h.writeTextFilePreview(c, uploadFile, content)
		return
	}

	if asAttachment {
		c.Header("Content-Disposition", fileAttachmentDisposition(uploadFile.Name))
	}
	c.Header("Content-Length", strconv.Itoa(len(content)))
	c.Data(http.StatusOK, uploadFile.MimeType, content)
}

func (h *ImagePreviewHandler) writeTextFilePreview(c *gin.Context, uploadFile *dto.UploadFile, content []byte) {
	normalized, contentType, err := normalizeTextPreviewContent(content, uploadFile)
	if err != nil {
		response.Fail(c, response.ErrFilePreviewFailed)
		return
	}

	c.Header("Content-Length", strconv.Itoa(len(normalized)))
	c.Data(http.StatusOK, contentType, normalized)
}

func (h *ImagePreviewHandler) GetMinerUImage(c *gin.Context) {
	timestamp := c.Query("timestamp")
	nonce := c.Query("nonce")
	sign := c.Query("sign")
	storageKey := c.Query("key")
	if storageKey != "" {
		var ok bool
		storageKey, ok = normalizeMinerUImageStorageKey(storageKey)
		if !ok {
			response.Fail(c, response.ErrFileNotFound)
			return
		}
		if !verifyOptionalParserImageSignature(c, "key", storageKey, timestamp, nonce, sign) {
			return
		}
		if h.storage == nil {
			response.Fail(c, response.ErrFileNotFound)
			return
		}
		content, err := h.storage.Load(storageKey)
		if err != nil {
			response.Fail(c, response.ErrFileNotFound)
			return
		}

		h.writeCompressedImage(c, content)
		return
	}

	imagePath := c.Query("path")
	if imagePath == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if !allowMinerULocalImagePath(imagePath) {
		response.Fail(c, response.ErrFileNotFound)
		return
	}
	if !verifyOptionalParserImageSignature(c, "path", imagePath, timestamp, nonce, sign) {
		return
	}

	content, err := os.ReadFile(imagePath)
	if err != nil {
		response.Fail(c, response.ErrFileNotFound)
		return
	}

	h.writeCompressedImage(c, content)
}

func (h *ImagePreviewHandler) writeCompressedImage(c *gin.Context, content []byte) {
	compressed, mimeType, err := zgiimage.CompressPreviewImage(content)
	if err != nil {
		response.Fail(c, response.ErrUnsupportedFileType)
		return
	}

	c.Header("Content-Type", mimeType)
	c.Header("Content-Length", strconv.Itoa(len(compressed)))
	c.Data(http.StatusOK, mimeType, compressed)
}

func verifyOptionalParserImageSignature(c *gin.Context, kind, value, timestamp, nonce, sign string) bool {
	hasSignatureParams := timestamp != "" || nonce != "" || sign != ""
	if !hasSignatureParams {
		return true
	}
	if timestamp == "" || nonce == "" || sign == "" {
		response.Fail(c, response.ErrInvalidParam)
		return false
	}
	if !util.VerifyParserImageSignature(kind, value, timestamp, nonce, sign) {
		response.Fail(c, response.ErrFileNotFound)
		return false
	}
	return true
}

func normalizeMinerUImageStorageKey(key string) (string, bool) {
	value := strings.TrimSpace(key)
	if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "\\") {
		return "", false
	}
	normalized := strings.ReplaceAll(value, "\\", "/")
	if hasUnsafePathSegment(normalized) || !hasAllowedMinerUImageKeyPrefix(normalized) || !hasSupportedImageExtension(normalized) {
		return "", false
	}
	return normalized, true
}

func hasAllowedMinerUImageKeyPrefix(key string) bool {
	lower := strings.ToLower(key)
	return strings.HasPrefix(lower, "mineru/images/") ||
		strings.HasPrefix(lower, "document-images/")
}

func allowMinerULocalImagePath(path string) bool {
	value := strings.TrimSpace(path)
	if value == "" || hasUnsafePathSegment(strings.ReplaceAll(value, "\\", "/")) {
		return false
	}
	if !filepath.IsAbs(value) && !isWindowsAbsoluteFilePath(value) {
		return false
	}
	normalized := strings.ToLower(strings.ReplaceAll(value, "\\", "/"))
	if !strings.Contains(normalized, "/mineru/images/") &&
		!strings.Contains(normalized, "/hyperparse/mineru/images/") &&
		!strings.Contains(normalized, "/storage/mineru/images/") {
		return false
	}
	return hasSupportedImageExtension(normalized)
}

func hasUnsafePathSegment(value string) bool {
	parts := strings.Split(strings.ReplaceAll(value, "\\", "/"), "/")
	for _, part := range parts {
		if part == ".." {
			return true
		}
	}
	return false
}

func hasSupportedImageExtension(value string) bool {
	switch strings.ToLower(filepath.Ext(strings.SplitN(value, "?", 2)[0])) {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".tif", ".tiff":
		return true
	default:
		return false
	}
}

func isWindowsAbsoluteFilePath(value string) bool {
	return len(value) >= 3 &&
		((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) &&
		value[1] == ':' &&
		(value[2] == '\\' || value[2] == '/')
}
