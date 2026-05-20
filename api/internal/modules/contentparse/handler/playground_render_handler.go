package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	hyperparseengine "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/pkg/hyperparse"
	"github.com/zgiai/ginext/internal/modules/contentparse/model"
	"github.com/zgiai/ginext/pkg/response"
)

func (h *PlaygroundHandler) RenderSavedRunSource(c *gin.Context) {
	if h == nil || h.runs == nil {
		response.FailWithMessage(c, response.ErrSystemError, "content parse playground history is not initialized")
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid playground run id")
		return
	}
	item, err := h.runs.GetByID(c.Request.Context(), id, playgroundRunScope(c))
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	if item == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "404001", "message": "playground run not found"})
		return
	}
	h.renderPlaygroundRunSource(c, item)
}

func (h *PlaygroundHandler) RenderSharedRunSource(c *gin.Context) {
	if h == nil || h.runs == nil {
		response.FailWithMessage(c, response.ErrSystemError, "content parse playground history is not initialized")
		return
	}
	item, err := h.runs.GetByShareToken(c.Request.Context(), c.Param("token"))
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	if item == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "404001", "message": "shared playground run not found"})
		return
	}
	h.renderPlaygroundRunSource(c, item)
}

func (h *PlaygroundHandler) renderPlaygroundRunSource(c *gin.Context, item *model.PlaygroundRun) {
	if item == nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "404001", "message": "playground run not found"})
		return
	}
	if strings.TrimSpace(item.SourceStorageKey) == "" {
		c.JSON(http.StatusNotFound, gin.H{"code": "404001", "message": "playground source file not saved"})
		return
	}

	store, err := playgroundStorage()
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	data, err := store.Load(item.SourceStorageKey)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	maxPages := parsePositiveInt(c.Query("max_pages"))
	if maxPages <= 0 {
		maxPages = 20
	}
	if maxPages > 50 {
		maxPages = 50
	}

	mimeType := item.SourceMimeType
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	if isPlaygroundPDF(item.FileName, mimeType) {
		pages, engine, err := hyperparseengine.RenderPDFPagesToDataURLs(data, maxPages)
		if err != nil {
			response.FailWithMessage(c, response.ErrSystemError, err.Error())
			return
		}
		response.Success(c, playgroundPDFRenderResponse{
			Engine:    engine,
			PageCount: len(pages),
			Pages:     pages,
		})
		return
	}
	if strings.HasPrefix(mimeType, "image/") {
		response.Success(c, playgroundPDFRenderResponse{
			Engine:    "stored_source_image",
			PageCount: 1,
			Pages:     []string{dataURL(mimeType, data)},
		})
		return
	}

	response.FailWithMessage(c, response.ErrUnsupportedFileType, "saved source preview only supports PDF and image files")
}

func (h *PlaygroundHandler) RenderPDF(c *gin.Context) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.FailWithMessage(c, response.ErrNoFileUploaded, "please upload a pdf file")
		return
	}
	if fileHeader.Size > playgroundMaxFileSize {
		response.FailWithMessage(c, response.ErrFileTooLarge, "file size cannot exceed 64MB")
		return
	}

	data, err := readMultipartFile(fileHeader)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, err.Error())
		return
	}
	maxPages := parsePositiveInt(c.PostForm("max_pages"))
	if maxPages <= 0 {
		maxPages = hyperparseengine.PDFPageCountRelaxed(data)
	}
	if maxPages <= 0 {
		maxPages = 20
	}
	if maxPages > 50 {
		maxPages = 50
	}

	pages, engine, err := hyperparseengine.RenderPDFPagesToDataURLs(data, maxPages)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}
	if len(pages) == 0 {
		response.FailWithMessage(c, response.ErrSystemError, "pdf render returned no pages")
		return
	}

	response.Success(c, playgroundPDFRenderResponse{
		Engine:    engine,
		PageCount: len(pages),
		Pages:     pages,
	})
}
