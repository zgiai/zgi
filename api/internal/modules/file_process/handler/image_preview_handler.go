package handler

import (
	"net/http"
	"os"
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
	storageKey := c.Query("key")
	if storageKey != "" {
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

	content, err := os.ReadFile(imagePath)
	if err != nil {
		response.Fail(c, response.ErrFileNotFound)
		return
	}

	h.writeCompressedImage(c, content)
}

func (h *ImagePreviewHandler) GetDocumentImage(c *gin.Context) {
	storageKey := strings.TrimSpace(c.Query("key"))
	if storageKey == "" || !strings.HasPrefix(storageKey, "document-images/") {
		response.Fail(c, response.ErrInvalidParam)
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
