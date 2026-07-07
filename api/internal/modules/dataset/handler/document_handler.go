package handler

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"

	datasetservice "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/response"
)

// DocumentHandler handles document-related HTTP requests
type DocumentHandler struct {
	documentService   datasetservice.DocumentService
	datasetService    datasetservice.DatasetService
	accountService    interfaces.AccountService
	enterpriseService interfaces.OrganizationService
	authService       interfaces.AuthorizationService
}

// NewDocumentHandler creates a new DocumentHandler instance
func NewDocumentHandler(
	documentService datasetservice.DocumentService,
	datasetService datasetservice.DatasetService,
	accountService interfaces.AccountService,
	enterpriseService interfaces.OrganizationService,
	authServices ...interfaces.AuthorizationService,
) *DocumentHandler {
	var authService interfaces.AuthorizationService
	if len(authServices) > 0 {
		authService = authServices[0]
	}
	return &DocumentHandler{
		documentService:   documentService,
		datasetService:    datasetService,
		accountService:    accountService,
		enterpriseService: enterpriseService,
		authService:       authService,
	}
}

// GetProcessRule handles GET /datasets/process-rule
func (h *DocumentHandler) GetProcessRule(c *gin.Context) {
	documentID := c.Query("document_id")

	var documentIDPtr *string
	if documentID != "" {
		documentIDPtr = &documentID
	}

	result, err := h.documentService.GetProcessRule(c.Request.Context(), documentIDPtr)
	if err != nil {
		response.Fail(c, response.ErrProcessRuleGetFailed)
		return
	}

	response.Success(c, result)
}

// GetDocumentList handles GET /datasets/:dataset_id/documents
func (h *DocumentHandler) GetDocumentList(c *gin.Context) {
	datasetID := c.Param("dataset_id")

	if _, ok := authorizeDatasetViewAccess(c, h.datasetService, h.authService, datasetID); !ok {
		return
	}

	var req dto.DocumentListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	result, err := h.documentService.GetDocumentList(c.Request.Context(), datasetID, &req)
	if err != nil {
		response.Fail(c, response.ErrDocumentListFailed)
		return
	}

	response.Success(c, result)
}

// CreateDocument handles POST /datasets/:dataset_id/documents
func (h *DocumentHandler) CreateDocument(c *gin.Context) {
	accountID := c.GetString("account_id")
	organizationID := c.GetString("tenant_id")

	if accountID == "" || organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	response.FailWithMessage(c, response.ErrDocumentCreateFailed, "dataset documents must be added from ready file assets")
}

// DeleteDocuments handles DELETE /datasets/:dataset_id/documents
func (h *DocumentHandler) DeleteDocuments(c *gin.Context) {
	datasetID := c.Param("dataset_id")

	if _, ok := authorizeDatasetViewAccess(c, h.datasetService, h.authService, datasetID); !ok {
		return
	}

	// Get document IDs from query parameters
	documentIDs := c.QueryArray("document_id")
	if len(documentIDs) == 0 {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	for _, documentID := range documentIDs {
		if _, _, ok := authorizeDatasetDocumentDeleteAccess(c, h.datasetService, h.documentService, h.authService, datasetID, documentID); !ok {
			return
		}
	}

	err := h.documentService.DeleteDocuments(c.Request.Context(), datasetID, documentIDs)
	if err != nil {
		response.Fail(c, response.ErrDocumentDeleteFailed)
		return
	}

	response.Success(c, gin.H{"result": "success"})
}

// GetDocumentDetail handles GET /datasets/:dataset_id/documents/:document_id
func (h *DocumentHandler) GetDocumentDetail(c *gin.Context) {
	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")
	metadata := c.Query("metadata")

	if metadata == "" {
		metadata = "all"
	}

	if _, _, ok := authorizeDatasetDocumentViewAccess(c, h.datasetService, h.documentService, h.authService, datasetID, documentID); !ok {
		return
	}

	result, err := h.documentService.GetDocumentDetail(c.Request.Context(), datasetID, documentID, metadata)
	if err != nil {
		response.Fail(c, response.ErrDocumentGetFailed)
		return
	}

	response.Success(c, result)
}

// UpdateDocument handles PATCH /datasets/:dataset_id/documents/:document_id
func (h *DocumentHandler) UpdateDocument(c *gin.Context) {
	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")

	if _, _, ok := authorizeDatasetDocumentUpdateAccess(c, h.datasetService, h.documentService, h.authService, datasetID, documentID); !ok {
		return
	}

	var req struct {
		Enabled *bool  `json:"enabled"`
		Name    string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	request := &dto.DocumentUpdateRequest{
		Enabled: req.Enabled,
		Name:    req.Name,
	}
	err := h.documentService.UpdateDocument(c.Request.Context(), request, datasetID, documentID)
	if err != nil {
		response.Fail(c, response.ErrDocumentUpdateFailed)
		return
	}
	response.Success(c, gin.H{"result": "success"})
}

// DeleteDocument handles DELETE /datasets/:dataset_id/documents/:document_id
func (h *DocumentHandler) DeleteDocument(c *gin.Context) {
	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")

	if _, _, ok := authorizeDatasetDocumentDeleteAccess(c, h.datasetService, h.documentService, h.authService, datasetID, documentID); !ok {
		return
	}

	err := h.documentService.DeleteDocuments(c.Request.Context(), datasetID, []string{documentID})
	if err != nil {
		response.Fail(c, response.ErrDocumentDeleteFailed)
		return
	}

	response.Success(c, gin.H{"result": "success"})
}

// GetBatchIndexingStatus handles GET /datasets/:dataset_id/batch/:batch/indexing-status
func (h *DocumentHandler) GetBatchIndexingStatus(c *gin.Context) {
	datasetID := c.Param("dataset_id")
	batch := c.Param("batch")

	if _, ok := authorizeDatasetViewAccess(c, h.datasetService, h.authService, datasetID); !ok {
		return
	}

	result, err := h.documentService.GetBatchIndexingStatus(c.Request.Context(), datasetID, batch)
	if err != nil {
		response.Fail(c, response.ErrBatchIndexingStatusFailed)
		return
	}

	response.Success(c, result)
}

// GetDocumentIndexingStatus handles GET /datasets/:dataset_id/documents/:document_id/indexing-status
func (h *DocumentHandler) GetDocumentIndexingStatus(c *gin.Context) {
	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")

	if _, _, ok := authorizeDatasetDocumentViewAccess(c, h.datasetService, h.documentService, h.authService, datasetID, documentID); !ok {
		return
	}

	result, err := h.documentService.GetDocumentIndexingStatus(c.Request.Context(), datasetID, documentID)
	if err != nil {
		response.Fail(c, response.ErrDocumentNotFound)
		return
	}

	response.Success(c, result)
}

// GetDocumentProgress handles GET /datasets/:dataset_id/documents/:document_id/progress
func (h *DocumentHandler) GetDocumentProgress(c *gin.Context) {
	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")

	if _, _, ok := authorizeDatasetDocumentViewAccess(c, h.datasetService, h.documentService, h.authService, datasetID, documentID); !ok {
		return
	}

	result, err := h.documentService.GetDocumentProgress(c.Request.Context(), datasetID, documentID)
	if err != nil {
		response.Fail(c, response.ErrDocumentNotFound)
		return
	}

	response.Success(c, result)
}

// RetryDocument handles POST /datasets/:dataset_id/retry
func (h *DocumentHandler) RetryDocument(c *gin.Context) {
	datasetID := c.Param("dataset_id")

	// Get user info from context
	accountID := c.GetString("account_id")
	tenantID := c.GetString("tenant_id")

	if accountID == "" || tenantID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}
	if _, ok := authorizeDatasetIndexManageAccess(c, h.datasetService, h.authService, datasetID); !ok {
		return
	}

	var req struct {
		DocumentIDs []string `json:"document_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	for _, documentID := range req.DocumentIDs {
		if _, _, ok := authorizeDatasetDocumentUpdateAccess(c, h.datasetService, h.documentService, h.authService, datasetID, documentID); !ok {
			return
		}
	}

	// Retry documents
	if err := h.documentService.RetryDocuments(c.Request.Context(), datasetID, req.DocumentIDs, accountID); err != nil {
		response.Fail(c, response.ErrDocumentRetryFailed)
		return
	}

	response.Success(c, gin.H{"result": "success"})
}

// UpdateDocumentStatus handles PATCH /datasets/:dataset_id/documents/status/:action/batch
func (h *DocumentHandler) UpdateDocumentStatus(c *gin.Context) {
	datasetID := c.Param("dataset_id")
	action := c.Param("action")

	// Validate dataset ID format
	if _, err := uuid.Parse(datasetID); err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	// Get account and tenant IDs from context
	accountID := c.GetString("account_id")
	tenantID := c.GetString("tenant_id")

	if accountID == "" || tenantID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if _, ok := authorizeDatasetDocumentBatchUpdateAccess(c, h.datasetService, h.authService, datasetID); !ok {
		return
	}

	// Validate action
	validActions := map[string]bool{
		"enable":     true,
		"disable":    true,
		"archive":    true,
		"un_archive": true,
	}

	if !validActions[action] {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Get document IDs from query parameters
	documentIDs := c.QueryArray("document_id")
	if len(documentIDs) == 0 {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Validate document IDs format
	for _, docID := range documentIDs {
		if _, err := uuid.Parse(docID); err != nil {
			response.Fail(c, response.ErrInvalidUuid)
			return
		}
	}

	for _, documentID := range documentIDs {
		if _, _, ok := authorizeDatasetDocumentUpdateAccess(c, h.datasetService, h.documentService, h.authService, datasetID, documentID); !ok {
			return
		}
	}

	// Call document service to update document status
	if err := h.documentService.UpdateDocumentStatus(c.Request.Context(), datasetID, action, documentIDs, accountID); err != nil {
		// Handle specific errors
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "invalid action"):
			response.Fail(c, response.ErrInvalidParam)
			return
		case strings.Contains(errMsg, "invalid document ID format"):
			response.Fail(c, response.ErrInvalidUuid)
			return
		case strings.Contains(errMsg, "failed to get dataset"):
			response.Fail(c, response.ErrDatasetNotFound)
			return
		default:
			response.Fail(c, response.ErrDocumentUpdateFailed)
			return
		}
	}

	response.Success(c, gin.H{"result": "success"})
}

// RegisterRoutes registers all document routes
func (h *DocumentHandler) RegisterRoutes(router *gin.RouterGroup) {
	// Process rule routes (no auth required for basic queries)
	router.GET("/datasets/process-rule", h.GetProcessRule)

	// Document routes with authentication - using the same middleware as DatasetHandler
	authWithTenant := router.Group("", middleware.JWTWithOrganizationAndService(h.accountService))
	datasetDocs := authWithTenant.Group("/datasets/:dataset_id")
	{
		// Document list and creation
		datasetDocs.GET("/documents", h.GetDocumentList)
		datasetDocs.POST("/documents", h.CreateDocument)
		datasetDocs.DELETE("/documents", h.DeleteDocuments)

		// Document detail operations
		datasetDocs.GET("/documents/:document_id", h.GetDocumentDetail)
		datasetDocs.PATCH("/documents/:document_id", h.UpdateDocument)
		datasetDocs.DELETE("/documents/:document_id", h.DeleteDocument)
		datasetDocs.GET("/documents/:document_id/indexing-status", h.GetDocumentIndexingStatus)
		datasetDocs.GET("/documents/:document_id/progress", h.GetDocumentProgress)

		// Document retry operation
		datasetDocs.POST("/retry", h.RetryDocument)

		// Document status operations
		datasetDocs.PATCH("/documents/status/:action/batch", h.UpdateDocumentStatus)
	}
}
