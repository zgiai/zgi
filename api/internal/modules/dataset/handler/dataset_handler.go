package handler

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/util"

	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	shared_model "github.com/zgiai/zgi/api/internal/modules/shared/model"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
	"github.com/zgiai/zgi/api/internal/modules/dataset/service"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/gorm"
)

// DatasetHandler handles dataset-related HTTP requests
type DatasetHandler struct {
	accountService      interfaces.AccountService
	datasetService      service.DatasetService
	documentService     service.DocumentService
	tenantService       interfaces.WorkspaceManagementService
	hitTestingService   service.HitTestingService
	datasetQueryService service.DatasetQueryService
	organizationService interfaces.OrganizationService
	billingService      interfaces.BillingService
	defaultModelService llmdefaultservice.DefaultModelService
	segmentService      service.SegmentService
	folderService       service.DatasetFolderService        // Add folder service
	batchTaskManager    *service.BatchHitTestingTaskManager // Add batch task manager
	permissionService   interfaces.ResourcePermissionService
}

// NewDatasetHandler creates a new DatasetHandler instance
func NewDatasetHandler(
	accountService interfaces.AccountService,
	datasetService service.DatasetService,
	documentService service.DocumentService,
	tenantService interfaces.WorkspaceManagementService,
	hitTestingService service.HitTestingService,
	datasetQueryService service.DatasetQueryService,
	enterpriseService interfaces.OrganizationService,
	billingService interfaces.BillingService,
	defaultModelService llmdefaultservice.DefaultModelService,
	segmentService service.SegmentService,
	folderService service.DatasetFolderService, // Add folder service
	batchTaskManager *service.BatchHitTestingTaskManager, // Add batch task manager
	permissionService interfaces.ResourcePermissionService,
) *DatasetHandler {
	return &DatasetHandler{
		accountService:      accountService,
		datasetService:      datasetService,
		documentService:     documentService,
		tenantService:       tenantService,
		hitTestingService:   hitTestingService,
		datasetQueryService: datasetQueryService,
		organizationService: enterpriseService,
		billingService:      billingService,
		defaultModelService: defaultModelService,
		segmentService:      segmentService,
		folderService:       folderService,    // Initialize folder service
		batchTaskManager:    batchTaskManager, // Initialize batch task manager
		permissionService:   permissionService,
	}
}

func failDatasetRead(c *gin.Context, err error, fallback response.ErrorCode) {
	switch {
	case errors.Is(err, service.ErrDatasetAccessDenied):
		response.Fail(c, response.ErrDatasetPermissionDenied)
	case errors.Is(err, gorm.ErrRecordNotFound):
		response.Fail(c, response.ErrDatasetNotFound)
	default:
		response.Fail(c, fallback)
	}
}

// GetDatasets handles GET /datasets
func (h *DatasetHandler) GetDatasets(c *gin.Context) {
	// Get account and tenant IDs from context (already validated by JWTWithTenant middleware)
	accountID := c.GetString("account_id")
	tenantID := c.GetString("tenant_id")

	var req dto.DatasetListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Set default values
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Limit <= 0 {
		req.Limit = 20
	}
	if req.Sort == "" || (req.Sort != "asc" && req.Sort != "desc") {
		req.Sort = "desc"
	}

	// Get group_id from request, if not provided, use current user's group_id
	organizationID := tenantID

	// Get all tenants in the group for all_group permission filtering
	allGroupTenantList, err := h.organizationService.GetOrganizationWorkspacesList(c.Request.Context(), organizationID)
	if err != nil {
		logger.Error("Failed to get all tenants in group for all_group permission", err)
		allGroupTenantList = []*workspace_model.Workspace{}
	}

	tenantIDs := make([]string, 0, len(allGroupTenantList))
	for _, tenant := range allGroupTenantList {
		hasPermission, err := h.organizationService.CheckWorkspaceOrganizationAnyPermission(
			c.Request.Context(),
			organizationID,
			tenant.ID,
			accountID,
			knowledgeBaseViewPermissionCodes()...,
		)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		if hasPermission {
			tenantIDs = append(tenantIDs, tenant.ID)
		}
	}
	allGroupTenantIDs := append([]string(nil), tenantIDs...)

	workspaceID := req.WorkspaceID
	if workspaceID != "" {
		filteredTenantIDs := make([]string, 0, len(tenantIDs))
		for _, id := range tenantIDs {
			if id == workspaceID {
				filteredTenantIDs = append(filteredTenantIDs, id)
				break
			}
		}
		tenantIDs = filteredTenantIDs

		filteredAllGroupTenantIDs := make([]string, 0, len(allGroupTenantIDs))
		for _, id := range allGroupTenantIDs {
			if id == workspaceID {
				filteredAllGroupTenantIDs = append(filteredAllGroupTenantIDs, id)
			}
		}
		allGroupTenantIDs = filteredAllGroupTenantIDs
	}

	if len(tenantIDs) == 0 {
		responseData := dto.DatasetListResponse{
			Data:    []dto.DatasetResponse{},
			HasMore: false,
			Limit:   req.Limit,
			Total:   0,
			Page:    req.Page,
		}
		response.Success(c, responseData)
		return
	}

	// Call service method to get datasets
	paginateReq := &service.GetPaginateDatasetsByTenantIDsRequest{
		TenantIDs:         tenantIDs,
		Page:              req.Page,
		Limit:             req.Limit,
		Search:            stringPointer(req.Keyword),
		DatasetAdmin:      false,
		AccountID:         accountID,
		GroupID:           organizationID,
		IsGroupAdmin:      false,
		AllGroupTenantIDs: allGroupTenantIDs, // Pass all group tenant IDs for all_group permission
		Sort:              req.Sort,
	}

	result, err := h.datasetService.GetPaginateDatasetsByTenantIDs(c.Request.Context(), paginateReq)
	if err != nil {
		response.Fail(c, response.ErrDatasetGetFailed)
		return
	}

	// Convert to response format with editor permission check
	datasetsResponseArr := h.convertDatasetsToResponseWithPermission(c.Request.Context(), result.Data, accountID)
	responseData := dto.DatasetListResponse{
		Data:    datasetsResponseArr,
		HasMore: result.HasMore,
		Limit:   result.Limit,
		Total:   result.Total,
		Page:    result.Page,
	}

	response.Success(c, responseData)
}

// GetDocumentExtractionStrategies handles GET /datasets/extraction-strategies.
func (h *DatasetHandler) GetDocumentExtractionStrategies(c *gin.Context) {
	if h.documentService == nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	items := h.documentService.ListDocumentExtractionStrategyOptions()
	response.Success(c, dto.DocumentExtractionStrategiesResponse{
		Strategies:          h.documentService.ListAvailableDocumentExtractionStrategies(),
		RecommendedStrategy: h.documentService.RecommendedDocumentExtractionStrategy(),
		Items:               items,
	})
}

// PostDatasets handles POST /datasets
func (h *DatasetHandler) PostDatasets(c *gin.Context) {
	// Get account and tenant IDs from context (already validated by JWTWithTenant middleware)
	accountID := c.GetString("account_id")
	tenantID := c.GetString("tenant_id")

	if accountID == "" || tenantID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req dto.DatasetCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Validate name
	if err := h.validateName(req.Name); err != nil {
		response.Fail(c, response.ErrDatasetName)
		return
	}

	// Validate description length
	if err := h.validateDescriptionLength(req.Description); err != nil {
		response.Fail(c, response.ErrDatasetDescriptionLong)
		return
	}

	requestWorkspaceID := req.WorkspaceID
	if requestWorkspaceID == nil || *requestWorkspaceID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if h.organizationService != nil {
		hasPermission, err := h.organizationService.CheckWorkspacePermission(
			c.Request.Context(),
			tenantID,
			*requestWorkspaceID,
			accountID,
			workspace_model.WorkspacePermissionKnowledgeBaseCreate,
		)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		if !hasPermission {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
	}

	// Set defaults
	if req.Provider == "" {
		req.Provider = "vendor"
	}

	// Check if embedding model and provider are provided, if not, get default embedding model
	var embeddingModel, embeddingModelProvider *string
	organizationID := c.GetString("organization_id")
	if organizationID == "" {
		organizationID = tenantID
	}

	if req.EmbeddingModel != nil && *req.EmbeddingModel != "" && req.EmbeddingModelProvider != nil && *req.EmbeddingModelProvider != "" {
		embeddingModel = req.EmbeddingModel
		embeddingModelProvider = req.EmbeddingModelProvider
	} else {
		if h.defaultModelService != nil {
			defaultModel, err := h.defaultModelService.ResolveModelType(c.Request.Context(), organizationID, nil, nil, shared_model.ModelTypeEmbedding)
			if err != nil {
				response.Fail(c, response.ErrSystemError)
				return
			}

			if defaultModel == nil || strings.TrimSpace(defaultModel.Model) == "" || strings.TrimSpace(defaultModel.Provider) == "" {
				response.Fail(c, response.ErrSystemError)
				return
			}
			embeddingModel = &defaultModel.Model
			embeddingModelProvider = &defaultModel.Provider
		} else {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
	}

	if req.IconType == nil || *req.IconType == "" {
		iconTypeStr := string(shared_model.IconTypeText)
		req.IconType = &iconTypeStr
	}

	// Create dataset request
	createReq := &service.CreateDatasetRequest{
		WorkspaceID:            *requestWorkspaceID,
		Name:                   req.Name,
		Description:            &req.Description,
		Provider:               req.Provider,
		Permission:             req.Permission,
		EmbeddingModel:         embeddingModel,
		EmbeddingModelProvider: embeddingModelProvider,
		EntityModel:            req.EntityModel,
		EntityModelProvider:    req.EntityModelProvider,
		RetrievalConfig:        req.RetrievalConfig,
		Icon:                   req.Icon,
		IconType:               req.IconType,
		IconBackground:         req.IconBackground,
		CreatedBy:              accountID,
		EnableGraphFlow:        req.EnableGraphFlow,
	}

	dataset, err := h.datasetService.CreateDataset(c.Request.Context(), createReq)
	if err != nil {
		if errors.Is(err, service.ErrInvalidDatasetPermission) {
			response.Fail(c, response.ErrInvalidPermission)
			return
		}
		if strings.Contains(err.Error(), "duplicate") {
			response.Fail(c, response.ErrDatasetExists)
			return
		}
		if strings.Contains(err.Error(), "quota exceeded") || strings.Contains(err.Error(), "Knowledge bases quota exceeded") {
			response.Fail(c, response.ErrQuotaKnowledgeBasesExceeded)
			return
		}
		response.FailWithMessage(c, response.ErrDatasetCreateFailed, err.Error())
		return
	}

	// If FolderID is provided, move the dataset to the specified folder
	if req.FolderID != nil && *req.FolderID != "" {
		// Validate FolderID format
		if _, err := uuid.Parse(*req.FolderID); err != nil {
			response.Fail(c, response.ErrInvalidParam)
			return
		}

		// Check if folder exists
		_, err := h.folderService.GetFolderByID(c.Request.Context(), *req.FolderID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				response.Fail(c, response.ErrDatasetNotFound)
				return
			}
			response.Fail(c, response.ErrSystemError)
			return
		}

		// Move dataset to folder
		err = h.folderService.MoveDatasetToFolder(c.Request.Context(), dataset.ID, *req.FolderID, accountID)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
	}

	responseData := h.convertDatasetToResponse(dataset)
	response.Success(c, responseData)
}

// GetDataset handles GET /datasets/:id (matches DatasetApi.get)
func (h *DatasetHandler) GetDataset(c *gin.Context) {
	// Get account and tenant IDs from context (already validated by JWTWithTenant middleware)
	accountID := c.GetString("account_id")
	tenantID := c.GetString("tenant_id")
	datasetID := c.Param("dataset_id")

	if datasetID == "" {
		response.Fail(c, response.ErrDatasetIdRequired)
		return
	}

	// Validate UUID format
	if _, err := uuid.Parse(datasetID); err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	// Get dataset with permission check
	dataset, err := h.datasetService.GetDatasetWithPermissionCheck(c.Request.Context(), datasetID, accountID, tenantID)
	if err != nil {
		failDatasetRead(c, err, response.ErrDatasetGetFailed)
		return
	}

	// Fetch dataset stats for document count, segment count, etc.
	datasetWithStats, err := h.datasetService.GetDatasetWithDocumentStats(c.Request.Context(), datasetID)
	if err == nil && datasetWithStats != nil {
		dataset.DocumentCount = int(datasetWithStats.DocumentCount)
		dataset.AvailableDocumentCount = int(datasetWithStats.AvailableDocumentCount)
		dataset.AvailableSegmentCount = int(datasetWithStats.AvailableSegmentCount)
		dataset.WordCount = datasetWithStats.Dataset.WordCount
	}

	responseData := h.convertDatasetToResponseWithPermission(c.Request.Context(), dataset, accountID)
	response.Success(c, responseData)
}

// PatchDataset handles PATCH /datasets/:id (matches DatasetApi.patch)
func (h *DatasetHandler) PatchDataset(c *gin.Context) {
	accountID := c.GetString("account_id")
	tenantID := c.GetString("tenant_id")
	datasetID := c.Param("dataset_id")

	if accountID == "" || tenantID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if datasetID == "" {
		response.Fail(c, response.ErrDatasetIdRequired)
		return
	}

	// Validate UUID format
	if _, err := uuid.Parse(datasetID); err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	var req dto.DatasetUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Validate name if provided
	if req.Name != nil {
		if err := h.validateName(*req.Name); err != nil {
			response.Fail(c, response.ErrDatasetName)
			return
		}
	}

	// Set default IconType if not provided (consistent with PostDatasets)
	if req.IconType == nil || *req.IconType == "" {
		iconTypeStr := string(shared_model.IconTypeText)
		req.IconType = &iconTypeStr
	}

	// Check editor permission with detailed error handling
	hasPermission, err := h.datasetService.CheckEditorPermission(c.Request.Context(), datasetID, accountID, tenantID)
	if err != nil {
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "not found"):
			response.Fail(c, response.ErrDatasetNotFound)
			return
		case strings.Contains(errMsg, "no permission"):
			response.Fail(c, response.ErrDatasetPermissionDenied)
			return
		default:
			response.Fail(c, response.ErrSystemError)
			return
		}
	}
	if !hasPermission {
		response.Fail(c, response.ErrDatasetPermissionDenied)
		return
	}

	// Handle TenantID update - check permissions
	if req.WorkspaceID != nil && *req.WorkspaceID != "" {
		if h.organizationService != nil {
			hasPermission, err := h.organizationService.CheckWorkspacePermission(
				c.Request.Context(),
				tenantID,
				*req.WorkspaceID,
				accountID,
				workspace_model.WorkspacePermissionKnowledgeBaseMove,
			)
			if err != nil {
				response.Fail(c, response.ErrSystemError)
				return
			}
			if !hasPermission {
				response.Fail(c, response.ErrPermissionDenied)
				return
			}
		}
	}

	// Update dataset
	updateReq := &service.UpdateDatasetRequest{
		ID:                     datasetID,
		Name:                   req.Name,
		Description:            req.Description,
		EmbeddingModel:         req.EmbeddingModel,
		EmbeddingModelProvider: req.EmbeddingModelProvider,
		EntityModel:            req.EntityModel,
		EntityModelProvider:    req.EntityModelProvider,
		RetrievalConfig:        req.RetrievalConfig,
		Icon:                   req.Icon,
		IconType:               req.IconType,
		IconBackground:         req.IconBackground,
		WorkspaceID:            req.WorkspaceID,
		UpdatedBy:              accountID,
		EnableGraphFlow:        req.EnableGraphFlow,
	}

	dataset, err := h.datasetService.UpdateDataset(c.Request.Context(), updateReq)
	if err != nil {
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "not found"):
			response.Fail(c, response.ErrDatasetNotFound)
			return
		default:
			response.Fail(c, response.ErrDatasetUpdateFailed)
			return
		}
	}

	responseData := h.convertDatasetToResponse(dataset)
	response.Success(c, responseData)
}

// DeleteDataset handles DELETE /datasets/:id (matches DatasetApi.delete)
func (h *DatasetHandler) DeleteDataset(c *gin.Context) {
	accountID := c.GetString("account_id")
	tenantID := c.GetString("tenant_id")
	groupID := c.GetString("group_id")
	datasetID := c.Param("dataset_id")

	if accountID == "" || tenantID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if datasetID == "" {
		response.Fail(c, response.ErrDatasetIdRequired)
		return
	}

	if groupID == "" {
		groupID = tenantID
	}

	// Validate UUID format
	if _, err := uuid.Parse(datasetID); err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	var datasetTenantID string
	if h.datasetService != nil {
		dataset, err := h.datasetService.GetDatasetByID(c.Request.Context(), datasetID)
		if err != nil || dataset == nil {
			response.Fail(c, response.ErrDatasetNotFound)
			return
		}
		datasetTenantID = dataset.WorkspaceID
	} else {
		datasetTenantID = tenantID
	}

	if h.organizationService != nil {
		hasPermission, err := h.organizationService.CheckWorkspacePermission(
			c.Request.Context(),
			groupID,
			datasetTenantID,
			accountID,
			workspace_model.WorkspacePermissionKnowledgeBaseDelete,
		)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		if !hasPermission {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
	}

	// Check editor permission with detailed error handling
	hasPermission, err := h.datasetService.CheckEditorPermission(c.Request.Context(), datasetID, accountID, tenantID)
	if err != nil {
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "not found"):
			response.Fail(c, response.ErrDatasetNotFound)
			return
		case strings.Contains(errMsg, "no permission"):
			response.Fail(c, response.ErrDatasetPermissionDenied)
			return
		default:
			response.Fail(c, response.ErrSystemError)
			return
		}
	}
	if !hasPermission {
		response.Fail(c, response.ErrDatasetPermissionDenied)
		return
	}

	// Delete dataset
	err = h.datasetService.DeleteDataset(c.Request.Context(), datasetID, accountID, tenantID)
	if err != nil {
		errMsg := err.Error()
		switch {
		case strings.Contains(errMsg, "dataset not found") || strings.Contains(errMsg, "record not found"):
			response.Fail(c, response.ErrDatasetNotFound)
			return
		case strings.Contains(errMsg, "no permission") || strings.Contains(errMsg, "access denied"):
			response.Fail(c, response.ErrDatasetPermissionDenied)
			return
		default:
			response.FailWithMessage(c, response.ErrDatasetDeleteFailed, errMsg)
			return
		}
	}

	response.Success(c, gin.H{"result": "success"})
}

// PostDatasetIndexingEstimate handles POST /datasets/indexing-estimate (matches DatasetIndexingEstimateApi.post)
func (h *DatasetHandler) PostDatasetIndexingEstimate(c *gin.Context) {
	accountID := c.GetString("account_id")
	tenantID := c.GetString("tenant_id")

	if accountID == "" || tenantID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	var req dto.DatasetIndexingEstimateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Set defaults
	if req.DocForm == "" {
		req.DocForm = "text_model"
	}
	if req.DocLanguage == "" {
		req.DocLanguage = "English"
	}

	// Convert to service request
	serviceReq := &service.IndexingEstimateRequest{
		InfoList:          req.InfoList,
		ProcessRule:       req.ProcessRule,
		IndexingTechnique: req.IndexingTechnique,
		DocForm:           req.DocForm,
		DatasetID:         req.DatasetID,
		DocLanguage:       req.DocLanguage,
		TenantID:          tenantID,
		AccountID:         accountID,
	}

	result, err := h.datasetService.EstimateIndexing(c.Request.Context(), serviceReq)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, result)
}

// GetDatasetEditorPermission handles GET /datasets/check-editor-permission/:id (matches DatasetEditorPermissionApi.get)
func (h *DatasetHandler) GetDatasetEditorPermission(c *gin.Context) {
	accountID := c.GetString("account_id")
	tenantID := c.GetString("tenant_id")
	datasetID := c.Param("dataset_id")

	if accountID == "" || tenantID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if datasetID == "" {
		response.Fail(c, response.ErrDatasetIdRequired)
		return
	}

	hasPermission, err := h.datasetService.CheckEditorPermission(c.Request.Context(), datasetID, accountID, tenantID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			response.Fail(c, response.ErrDatasetNotFound)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, dto.DatasetEditorPermissionResponse{
		HasPermission: hasPermission,
	})
}

// HitTesting handles POST /datasets/:id/hit-testing (matches HitTestingApi.post)
func (h *DatasetHandler) handleHitTesting(c *gin.Context, forcedMode string) {
	// Get dataset ID from URL parameter
	datasetIDStr := c.Param("dataset_id")
	if datasetIDStr == "" {
		response.Fail(c, response.ErrDatasetIdRequired)
		return
	}

	// Validate UUID format
	datasetID, err := uuid.Parse(datasetIDStr)
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	// Parse request body
	var req dto.HitTestingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Get account and tenant IDs from context (already validated by JWTWithTenant middleware)
	accountID := c.GetString("account_id")
	tenantID := c.GetString("tenant_id")

	// Get and validate dataset with permission check
	dataset, err := h.datasetService.GetDatasetWithPermissionCheck(c.Request.Context(), datasetID.String(), accountID, tenantID)
	if err != nil {
		failDatasetRead(c, err, response.ErrDatasetGetFailed)
		return
	}

	groupID := c.GetString("group_id")
	if groupID == "" {
		groupID = tenantID
	}

	if h.organizationService != nil {
		datasetTenantID := dataset.WorkspaceID

		hasPermission, err := h.organizationService.CheckWorkspaceOrganizationAnyPermission(
			c.Request.Context(),
			groupID,
			datasetTenantID,
			accountID,
			workspace_model.WorkspacePermissionKnowledgeBaseRetrievalTest,
		)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		if !hasPermission {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
	}

	// Validate hit testing arguments
	if err := h.hitTestingService.HitTestingArgsCheck(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	mode := req.RetrievalMode
	if forcedMode != "" {
		mode = forcedMode
	}
	recordHistory := req.RecordHistory == nil || *req.RecordHistory

	result, err := h.hitTestingService.Retrieve(
		c.Request.Context(),
		dataset,
		req.Query,
		accountID,
		nil,
		req.ExternalRetrievalModel,
		10,
		"hit_testing",
		"single",
		mode,
		recordHistory,
	)
	if err != nil {
		// Handle specific error types based on current implementation
		errMsg := err.Error()
		logger.Error("HitTesting failed", err)

		switch {
		case strings.Contains(errMsg, "index not initialized"):
			response.Fail(c, response.ErrDatasetProcessing)
			return
		case strings.Contains(errMsg, "provider not initialized") || strings.Contains(errMsg, "no embedding model"):
			response.Fail(c, response.ErrSystemError)
			return
		case strings.Contains(errMsg, "quota exceeded"):
			response.Fail(c, response.ErrRateLimitExceeded)
			return
		case strings.Contains(errMsg, "model not supported"):
			response.Fail(c, response.ErrInvalidParam)
			return
		case strings.Contains(errMsg, "dataset not found"):
			response.Fail(c, response.ErrDatasetNotFound)
			return
		case strings.Contains(errMsg, "no permission"):
			response.Fail(c, response.ErrDatasetPermissionDenied)
			return
		default:
			logger.Warn("Unhandled hit testing error", map[string]interface{}{
				"error_message": errMsg,
			})
			response.FailWithMessage(c, response.ErrSystemError, errMsg)
			return
		}
	}

	// Return successful response
	response.Success(c, result)
}

// HitTesting handles POST /datasets/:dataset_id/hit-testing
func (h *DatasetHandler) HitTesting(c *gin.Context) {
	h.handleHitTesting(c, "")
}

// RetrieveVector handles POST /datasets/:dataset_id/retrieve/vector
func (h *DatasetHandler) RetrieveVector(c *gin.Context) {
	h.handleHitTesting(c, "vector")
}

// RetrieveGraph handles POST /datasets/:dataset_id/retrieve/graph
func (h *DatasetHandler) RetrieveGraph(c *gin.Context) {
	h.handleHitTesting(c, "graph")
}

// BatchHitTesting handles POST /datasets/:id/batch-hit-testing
// This endpoint performs hit testing for multiple queries at once
func (h *DatasetHandler) BatchHitTesting(c *gin.Context) {
	// Get dataset ID from URL parameter
	datasetIDStr := c.Param("dataset_id")
	if datasetIDStr == "" {
		response.Fail(c, response.ErrDatasetIdRequired)
		return
	}

	// Validate UUID format
	datasetID, err := uuid.Parse(datasetIDStr)
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	// Parse request body
	var req dto.BatchHitTestingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Validate queries
	if len(req.Queries) == 0 {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Limit number of queries in a batch (maximum 20)
	if len(req.Queries) > 20 {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Get account and tenant IDs from context (already validated by JWTWithTenant middleware)
	accountID := c.GetString("account_id")
	tenantID := c.GetString("tenant_id")

	// Get and validate dataset with permission check
	dataset, err := h.datasetService.GetDatasetWithPermissionCheck(c.Request.Context(), datasetID.String(), accountID, tenantID)
	if err != nil {
		failDatasetRead(c, err, response.ErrDatasetGetFailed)
		return
	}

	groupID := c.GetString("group_id")
	if groupID == "" {
		groupID = tenantID
	}

	if h.organizationService != nil {
		datasetTenantID := dataset.WorkspaceID

		hasPermission, err := h.organizationService.CheckWorkspaceOrganizationAnyPermission(
			c.Request.Context(),
			groupID,
			datasetTenantID,
			accountID,
			workspace_model.WorkspacePermissionKnowledgeBaseRetrievalTest,
		)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		if !hasPermission {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
	}

	// Process each query and collect results
	var results []dto.HitTestingResponse
	for _, query := range req.Queries {
		singleReq := &dto.HitTestingRequest{
			Query:                  query,
			ExternalRetrievalModel: req.ExternalRetrievalModel,
		}

		if err := h.hitTestingService.HitTestingArgsCheck(singleReq); err != nil {
			logger.Warn("Invalid hit testing args for query", map[string]interface{}{
				"query": query,
				"error": err.Error(),
			})
			continue
		}

		result, err := h.hitTestingService.Retrieve(
			context.Background(),
			dataset,
			query,
			accountID,
			nil,
			req.ExternalRetrievalModel,
			10,
			"batch_hit_testing",
			"batch",
			"",
			true,
		)
		if err != nil {
			// Handle specific error types based on current implementation
			errMsg := err.Error()
			logger.Error("Batch HitTesting failed for query", fmt.Errorf("query: %s, error: %s", query, errMsg))

			// Continue with other queries
			continue
		}
		results = append(results, *result)
	}

	// Create batch response
	batchResponse := &dto.BatchHitTestingResponse{
		Results: results,
	}

	// Return successful response
	response.Success(c, batchResponse)
}

// AsyncBatchHitTesting handles POST /datasets/:id/async-batch-hit-testing
// This endpoint initiates asynchronous hit testing for multiple queries
func (h *DatasetHandler) AsyncBatchHitTesting(c *gin.Context) {
	// Get dataset ID from URL parameter
	datasetIDStr := c.Param("dataset_id")
	if datasetIDStr == "" {
		response.Fail(c, response.ErrDatasetIdRequired)
		return
	}

	// Validate UUID format
	datasetID, err := uuid.Parse(datasetIDStr)
	if err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	// Parse request body
	var req dto.AsyncBatchHitTestingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Validate queries
	if len(req.Queries) == 0 {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Limit number of queries in a batch (maximum 100)
	if len(req.Queries) > 100 {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Get account and tenant IDs from context (already validated by JWTWithTenant middleware)
	accountID := c.GetString("account_id")
	organizationID := c.GetString("tenant_id")

	// Get and validate dataset with permission check
	dataset, err := h.datasetService.GetDatasetWithPermissionCheck(c.Request.Context(), datasetID.String(), accountID, organizationID)
	if err != nil {
		failDatasetRead(c, err, response.ErrDatasetGetFailed)
		return
	}

	groupID := c.GetString("group_id")
	if groupID == "" {
		groupID = organizationID
	}

	if h.organizationService != nil {
		datasetTenantID := dataset.WorkspaceID

		hasPermission, err := h.organizationService.CheckWorkspaceOrganizationAnyPermission(
			c.Request.Context(),
			groupID,
			datasetTenantID,
			accountID,
			workspace_model.WorkspacePermissionKnowledgeBaseRetrievalTest,
		)
		if err != nil {
			response.Fail(c, response.ErrSystemError)
			return
		}
		if !hasPermission {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}
	}

	// Create async batch task
	taskID := h.batchTaskManager.CreateTask(datasetID.String(), accountID, organizationID, &req)

	// Start processing in a goroutine
	go h.processBatchHitTesting(taskID, dataset, accountID, &req)

	// Return task ID
	asyncResponse := &dto.AsyncBatchHitTestingResponse{
		TaskID: taskID,
	}

	response.Success(c, asyncResponse)
}

// GetBatchHitTestingTaskStatus handles GET /datasets/:id/batch-hit-testing/tasks/:task_id
// This endpoint retrieves the status of an asynchronous batch hit testing task
func (h *DatasetHandler) GetBatchHitTestingTaskStatus(c *gin.Context) {
	_, task, ok := h.authorizeBatchHitTestingTaskAccess(c)
	if !ok {
		return
	}

	// Convert to DTO
	taskStatus := task.ToDTO()

	response.Success(c, taskStatus)
}

// GetBatchHitTestingTaskReport handles GET /datasets/:id/batch-hit-testing/tasks/:task_id/report
// This endpoint retrieves the report of a completed batch hit testing task
func (h *DatasetHandler) GetBatchHitTestingTaskReport(c *gin.Context) {
	_, task, ok := h.authorizeBatchHitTestingTaskAccess(c)
	if !ok {
		return
	}

	// Get task report
	report, err := h.batchTaskManager.GetTaskReport(task.TaskID)
	if err != nil {
		// Check if it's a "not found" error
		if err.Error() == "task not found" {
			response.Fail(c, response.ErrNotFound)
			return
		}

		// For other errors, return a business logic error
		response.Fail(c, response.ErrorCode{
			Code:        202010, // Using dataset module error code range
			Message:     err.Error(),
			UserVisible: true,
		})
		return
	}

	response.Success(c, report)
}

// StopBatchHitTestingTask handles POST /datasets/:id/batch-hit-testing/tasks/:task_id/stop
// This endpoint stops an asynchronous batch hit testing task
func (h *DatasetHandler) StopBatchHitTestingTask(c *gin.Context) {
	_, task, ok := h.authorizeBatchHitTestingTaskAccess(c)
	if !ok {
		return
	}

	// Check if task is in a state that can be stopped
	if task.Status == "completed" || task.Status == "failed" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Stop the task
	// TODO: In a production environment, we might need a more sophisticated way to stop
	// ongoing processing, such as signaling the goroutine to stop.
	if err := h.batchTaskManager.StopTask(task.TaskID); err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{"message": "task stopped successfully"})
}

// processBatchHitTesting processes the batch hit testing asynchronously
func (h *DatasetHandler) processBatchHitTesting(taskID string, dataset *model.Dataset, accountID string, req *dto.AsyncBatchHitTestingRequest) {
	// Create or get a cancellable context for the task
	taskCtx := h.batchTaskManager.CreateTaskContext(taskID)

	// Update task status to processing
	h.batchTaskManager.StartTask(taskID)

	for i, query := range req.Queries {
		if h.batchTaskManager.IsTaskCancelled(taskCtx) {
			for j := i; j < len(req.Queries); j++ {
				errorMsg := "Task was stopped by user"
				h.batchTaskManager.UpdateQueryTaskStatus(taskID, j, "failed", nil, &errorMsg)
			}
			return
		}

		h.batchTaskManager.UpdateQueryTaskStatus(taskID, i, "processing", nil, nil)

		singleReq := &dto.HitTestingRequest{
			Query:                  query,
			ExternalRetrievalModel: req.ExternalRetrievalModel,
		}

		if err := h.hitTestingService.HitTestingArgsCheck(singleReq); err != nil {
			errMsg := err.Error()
			h.batchTaskManager.UpdateQueryTaskStatus(taskID, i, "failed", nil, &errMsg)
			continue
		}

		result, err := h.hitTestingService.Retrieve(
			context.Background(),
			dataset,
			query,
			accountID,
			nil,
			req.ExternalRetrievalModel,
			10,
			"batch_hit_testing",
			"batch",
			"",
			true,
		)
		if err != nil {
			errMsg := err.Error()
			h.batchTaskManager.UpdateQueryTaskStatus(taskID, i, "failed", nil, &errMsg)
			continue
		}

		h.batchTaskManager.UpdateQueryTaskStatus(taskID, i, "completed", result, nil)
	}

	h.batchTaskManager.UpdateProgress(taskID)
}

type DatasetQueryResponse struct {
	ID            string  `json:"id"`
	Content       string  `json:"content"`
	Source        string  `json:"source"`
	SourceAppID   *string `json:"source_app_id"`
	CreatedByRole string  `json:"created_by_role"`
	CreatedBy     string  `json:"created_by"`
	CreatedAt     string  `json:"created_at"`

	Results     *dto.HitTestingResponse `json:"results,omitempty"`
	ElapsedTime *float64                `json:"elapsed_time,omitempty"`
	HitCount    *int                    `json:"hit_count,omitempty"`

	QueryType   string  `json:"query_type"`
	BatchTaskID *string `json:"batch_task_id,omitempty"`
	BatchName   *string `json:"batch_name,omitempty"`
}

type DatasetQueryListResponse struct {
	Data    []DatasetQueryResponse `json:"data"`
	HasMore bool                   `json:"has_more"`
	Limit   int                    `json:"limit"`
	Total   int64                  `json:"total"`
	Page    int                    `json:"page"`
}

// GetDatasetQueries handles GET /datasets/:dataset_id/queries
// This endpoint retrieves dataset queries with filtering capabilities.
// By default, it returns "single" and "batch_saved" query types, excluding "batch" type
// (which represents individual queries within batch testing).
// You can specify a query_type parameter to filter by specific types.
func (h *DatasetHandler) GetDatasetQueries(c *gin.Context) {
	accountID := c.GetString("account_id")
	organizationID := c.GetString("tenant_id")
	datasetID := c.Param("dataset_id")

	// Pagination params
	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "20")
	page, _ := strconv.Atoi(pageStr)
	if page <= 0 {
		page = 1
	}
	limit, _ := strconv.Atoi(limitStr)
	if limit <= 0 {
		limit = 20
	}

	// Query type filter
	queryType := c.Query("query_type")

	req := &service.GetDatasetQueriesRequest{
		DatasetID:      datasetID,
		Page:           page,
		Limit:          limit,
		AccountID:      accountID,
		OrganizationID: organizationID,
	}

	if queryType != "" {
		req.QueryType = &queryType
	}

	result, err := h.datasetQueryService.GetDatasetQueries(c.Request.Context(), req)
	if err != nil {
		if err.Error() == "dataset not found" {
			response.Fail(c, response.ErrDatasetNotFound)
			return
		}
		if err.Error() == "no permission to access this dataset" {
			response.Fail(c, response.ErrDatasetPermissionDenied)
			return
		}
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Convert to response format
	resp := make([]DatasetQueryResponse, 0, len(result.Data))
	for _, q := range result.Data {
		resp = append(resp, DatasetQueryResponse{
			ID:            q.ID,
			Content:       q.Content,
			Source:        q.Source,
			SourceAppID:   q.SourceAppID,
			CreatedByRole: q.CreatedByRole,
			CreatedBy:     q.CreatedBy,
			CreatedAt:     q.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			Results:       q.Results,
			ElapsedTime:   q.ElapsedTime,
			HitCount:      q.HitCount,
			QueryType:     q.QueryType,
			BatchTaskID:   q.BatchTaskID,
			BatchName:     q.BatchName,
		})
	}

	responseData := DatasetQueryListResponse{
		Data:    resp,
		HasMore: result.HasMore,
		Limit:   result.Limit,
		Total:   result.Total,
		Page:    result.Page,
	}
	response.Success(c, responseData)
}

// DeleteDatasetQuery handles DELETE /datasets/:dataset_id/queries/:query_id
func (h *DatasetHandler) DeleteDatasetQuery(c *gin.Context) {
	accountID := c.GetString("account_id")
	tenantID := c.GetString("tenant_id")
	datasetID := c.Param("dataset_id")
	queryID := c.Param("query_id")

	// First check if the user has permission to access the dataset
	_, err := h.datasetService.GetDatasetWithPermissionCheck(c.Request.Context(), datasetID, accountID, tenantID)
	if err != nil {
		failDatasetRead(c, err, response.ErrSystemError)
		return
	}

	// Delete the query
	err = h.datasetQueryService.DeleteDatasetQuery(c.Request.Context(), queryID)
	if err != nil {
		logger.Error("Failed to delete dataset query: ", err)
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, nil)
}

// GetRandomDatasetQuestions handles GET /datasets/:dataset_id/random-questions
// This endpoint randomly selects a specified number of questions from the dataset
func (h *DatasetHandler) GetRandomDatasetQuestions(c *gin.Context) {
	// Get account and tenant IDs from context (already validated by JWTWithTenant middleware)
	accountID := c.GetString("account_id")
	tenantID := c.GetString("tenant_id")
	datasetID := c.Param("dataset_id")

	if datasetID == "" {
		response.Fail(c, response.ErrDatasetIdRequired)
		return
	}

	// Get limit from query parameters (default to 10, max 200)
	limitStr := c.DefaultQuery("limit", "10")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 10
	}
	if limit > 200 {
		limit = 200
	}

	// Get and validate dataset with permission check
	_, err = h.datasetService.GetDatasetWithPermissionCheck(c.Request.Context(), datasetID, accountID, tenantID)
	if err != nil {
		failDatasetRead(c, err, response.ErrDatasetGetFailed)
		return
	}

	// Get random questions
	questions, err := h.segmentService.RandomDocumentSegmentQuestionsByDataset(c.Request.Context(), datasetID, limit)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	// Create response similar to pagination structure
	responseData := dto.DocumentSegmentQuestionListResponse{
		Data:    questions,
		Total:   int64(len(questions)), // For random selection, total is the number of returned questions
		Page:    1,                     // For random selection, we consider it as page 1
		Limit:   limit,
		HasMore: false, // For random selection, there's no concept of "has more"
	}
	response.Success(c, responseData)
}

// SaveBatchHitTestingResults handles POST /datasets/:dataset_id/batch-hit-testing/tasks/:task_id/save
// This endpoint saves the results of a completed batch hit testing task
func (h *DatasetHandler) SaveBatchHitTestingResults(c *gin.Context) {
	datasetID, task, ok := h.authorizeBatchHitTestingTaskAccess(c)
	if !ok {
		return
	}

	// Get account and tenant IDs from context
	accountID := c.GetString("account_id")
	organizationID := util.GetOrganizationIDCompat(c)

	// Check if task is completed
	if task.Status != "completed" && task.Status != "failed" {
		response.Fail(c, response.ErrorCode{
			Code:        202011,
			Message:     "task is not completed yet",
			UserVisible: true,
		})
		return
	}

	// Parse request body after task ownership and dataset access are verified.
	var req dto.SaveBatchHitTestingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Save batch hit testing results
	saveReq := &service.SaveBatchHitTestingResultsRequest{
		BatchTaskID:    task.TaskID,
		BatchName:      req.BatchName,
		DatasetID:      datasetID,
		AccountID:      accountID,
		OrganizationID: organizationID,
		CreatedBy:      accountID,
		StartedAt:      task.StartedAt,
		FinishedAt:     task.FinishedAt,
	}

	if err := h.datasetQueryService.SaveBatchHitTestingResults(c.Request.Context(), saveReq); err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{"message": "batch hit testing results saved successfully"})
}

func (h *DatasetHandler) authorizeBatchHitTestingTaskAccess(c *gin.Context) (string, *service.BatchHitTestingTask, bool) {
	datasetID := c.Param("dataset_id")
	if datasetID == "" {
		response.Fail(c, response.ErrDatasetIdRequired)
		return "", nil, false
	}
	if _, err := uuid.Parse(datasetID); err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return "", nil, false
	}

	taskID := c.Param("task_id")
	if taskID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return "", nil, false
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return "", nil, false
	}

	organizationID := util.GetOrganizationIDCompat(c)
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return "", nil, false
	}

	if h.batchTaskManager == nil {
		response.Fail(c, response.ErrSystemError)
		return "", nil, false
	}

	task, ok := h.batchTaskManager.GetTask(taskID)
	if !ok {
		response.Fail(c, response.ErrNotFound)
		return "", nil, false
	}

	if task.DatasetID != datasetID || task.AccountID != accountID || task.OrganizationID != organizationID {
		response.Fail(c, response.ErrPermissionDenied)
		return "", nil, false
	}

	if h.datasetService == nil {
		response.Fail(c, response.ErrSystemError)
		return "", nil, false
	}
	if _, err := h.datasetService.GetDatasetWithPermissionCheck(c.Request.Context(), datasetID, accountID, organizationID); err != nil {
		failDatasetRead(c, err, response.ErrDatasetGetFailed)
		return "", nil, false
	}

	return datasetID, task, true
}

// GetDatasetErrorDocs handles GET /datasets/:dataset_id/error-docs
// This endpoint retrieves documents with error or paused indexing status for a dataset
func (h *DatasetHandler) GetDatasetErrorDocs(c *gin.Context) {
	accountID := c.GetString("account_id")
	tenantID := c.GetString("tenant_id")
	datasetID := c.Param("dataset_id")

	if datasetID == "" {
		response.Fail(c, response.ErrDatasetIdRequired)
		return
	}

	// Validate UUID format
	if _, err := uuid.Parse(datasetID); err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	// Get dataset with permission check
	_, err := h.datasetService.GetDatasetWithPermissionCheck(c.Request.Context(), datasetID, accountID, tenantID)
	if err != nil {
		failDatasetRead(c, err, response.ErrDatasetGetFailed)
		return
	}

	// Get error documents using the newly added service method
	errorDocuments, err := h.documentService.GetErrorDocumentsByDatasetID(c.Request.Context(), datasetID)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to get error documents for dataset %s", datasetID), err)
		response.Fail(c, response.ErrSystemError)
		return
	}

	// Convert documents to response format
	var documentStatuses []dto.DocumentIndexingStatus
	var failedCount int

	for _, doc := range errorDocuments {
		documentIndexingStatus, err := h.documentService.GetDocumentIndexingStatus(c.Request.Context(), datasetID, doc.ID)
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to get document indexing status: dataset_id=%s, document_id=%s, document_name=%s, indexing_status=%s",
				datasetID, doc.ID, doc.Name, doc.IndexingStatus), err)
			failedCount++

			// Return basic document info even if detailed status fails
			basicStatus := dto.DocumentIndexingStatus{
				ID:             doc.ID,
				IndexingStatus: doc.IndexingStatus,
				Error:          doc.Error,
			}
			documentStatuses = append(documentStatuses, basicStatus)
			continue
		}

		if documentIndexingStatus != nil {
			documentStatuses = append(documentStatuses, *documentIndexingStatus)
		} else {
			logger.Warn(fmt.Sprintf("Document indexing status is nil: dataset_id=%s, document_id=%s",
				datasetID, doc.ID))
			failedCount++
		}
	}

	logger.Info(fmt.Sprintf("GetDatasetErrorDocs completed: dataset_id=%s, total_error_docs=%d, successfully_retrieved=%d, failed_count=%d",
		datasetID, len(errorDocuments), len(documentStatuses), failedCount))

	response.Success(c, gin.H{
		"data":  documentStatuses,
		"total": len(errorDocuments),
	})
}

// PostDatasetErrorDocsRetry handles POST /datasets/:dataset_id/error-docs/retry
func (h *DatasetHandler) PostDatasetErrorDocsRetry(c *gin.Context) {
	accountID := c.GetString("account_id")
	tenantID := c.GetString("tenant_id")
	datasetID := c.Param("dataset_id")

	if datasetID == "" {
		response.Fail(c, response.ErrDatasetIdRequired)
		return
	}

	// Validate UUID format
	if _, err := uuid.Parse(datasetID); err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	// Permission check
	_, err := h.datasetService.GetDatasetWithPermissionCheck(c.Request.Context(), datasetID, accountID, tenantID)
	if err != nil {
		failDatasetRead(c, err, response.ErrDatasetGetFailed)
		return
	}

	var req struct {
		DocumentIDs []string `json:"document_ids"`
	}
	// Body is optional
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		// If body is malformed, but not empty, return error
	}

	documentIDs := req.DocumentIDs
	if len(documentIDs) == 0 {
		// Get all error documents
		errorDocuments, err := h.documentService.GetErrorDocumentsByDatasetID(c.Request.Context(), datasetID)
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to get error documents for retry: dataset_id=%s", datasetID), err)
			response.Fail(c, response.ErrSystemError)
			return
		}
		for _, doc := range errorDocuments {
			documentIDs = append(documentIDs, doc.ID)
		}
	}

	if len(documentIDs) > 0 {
		if err := h.documentService.RetryDocuments(c.Request.Context(), datasetID, documentIDs, accountID); err != nil {
			logger.Error(fmt.Sprintf("Failed to retry error documents: dataset_id=%s, count=%d", datasetID, len(documentIDs)), err)
			response.Fail(c, response.ErrDocumentRetryFailed)
			return
		}
	}

	response.Success(c, gin.H{
		"result": "success",
		"count":  len(documentIDs),
	})
}

// GetDatasetQuestionCount handles GET /datasets/:dataset_id/question-count
// This endpoint returns the total count of questions for a dataset
func (h *DatasetHandler) GetDatasetQuestionCount(c *gin.Context) {
	// Get account and tenant IDs from context (already validated by JWTWithTenant middleware)
	accountID := c.GetString("account_id")
	tenantID := c.GetString("tenant_id")
	datasetID := c.Param("dataset_id")

	if datasetID == "" {
		response.Fail(c, response.ErrDatasetIdRequired)
		return
	}

	// Validate UUID format
	if _, err := uuid.Parse(datasetID); err != nil {
		response.Fail(c, response.ErrInvalidUuid)
		return
	}

	// Get dataset with permission check
	_, err := h.datasetService.GetDatasetWithPermissionCheck(c.Request.Context(), datasetID, accountID, tenantID)
	if err != nil {
		failDatasetRead(c, err, response.ErrDatasetGetFailed)
		return
	}

	// Get question count
	count, err := h.segmentService.GetDocumentSegmentQuestionCountByDataset(c.Request.Context(), datasetID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, gin.H{
		"count": count,
	})
}

// Helper methods for validation
func (h *DatasetHandler) validateName(name string) error {
	if len(name) < 1 || len(name) > 40 {
		return fmt.Errorf("Name must be between 1 to 40 characters")
	}
	return nil
}

func (h *DatasetHandler) validateDescriptionLength(description string) error {
	if len(description) > 400 {
		return fmt.Errorf("Description cannot exceed 400 characters")
	}
	return nil
}

// Helper methods for response conversion

// generateIconURL generates the icon URL if the icon is an image file ID
func generateIconURL(icon *string, iconType *string) string {
	if icon == nil || iconType == nil || *iconType != string(shared_model.IconTypeImage) {
		return ""
	}
	signedURL, err := util.GetSignedFileURL(*icon)
	if err != nil {
		return ""
	}
	return signedURL
}

func (h *DatasetHandler) convertDatasetToResponse(dataset *model.Dataset) dto.DatasetResponse {
	response := dto.DatasetResponse{
		ID:                     dataset.ID,
		WorkspaceID:            dataset.WorkspaceID,
		Name:                   dataset.Name,
		Description:            dataset.Description,
		Provider:               dataset.Provider,
		CreatedBy:              dataset.CreatedBy,
		CreatedAt:              dataset.CreatedAt,
		UpdatedBy:              dataset.UpdatedBy,
		UpdatedAt:              dataset.UpdatedAt,
		Owner:                  dataset.Owner,
		EmbeddingModel:         dataset.EmbeddingModel,
		EmbeddingModelProvider: dataset.EmbeddingModelProvider,
		EntityModel:            dataset.EntityModel,
		EntityModelProvider:    dataset.EntityModelProvider,
		CollectionBindingID:    dataset.CollectionBindingID,
		Icon:                   dataset.Icon,
		IconType:               dataset.IconType,
		IconBackground:         dataset.IconBackground,
		IconURL:                generateIconURL(dataset.Icon, dataset.IconType),
		AppCount:               dataset.AppCount,
		DocumentCount:          dataset.DocumentCount,
		AvailableDocumentCount: dataset.AvailableDocumentCount,
		AvailableSegmentCount:  dataset.AvailableSegmentCount,
		WordCount:              dataset.WordCount,
		EmbeddingAvailable:     true,
		PartialMemberList:      []interface{}{},
		Tags:                   dataset.Tags,
		DocForm:                dataset.DocForm,
		CanEdit:                false, // Default to false, will be set by caller if needed
		EnableGraphFlow:        dataset.EnableGraphFlow,
	}

	// Convert RetrievalConfig
	if dataset.RetrievalConfig != nil {
		response.RetrievalConfig = map[string]interface{}(dataset.RetrievalConfig)
	}

	return response
}

// convertDatasetToResponseWithPermission converts a single dataset to response with permission check
func (h *DatasetHandler) convertDatasetToResponseWithPermission(ctx context.Context, dataset *model.Dataset, accountID string) dto.DatasetResponse {
	response := h.convertDatasetToResponse(dataset)

	// Check single resource edit permission
	canEdit, err := h.permissionService.CheckSingleResourceEditPermission(ctx, interfaces.SingleResourcePermissionParams{
		AccountID: accountID,
		TenantID:  dataset.WorkspaceID,
		CreatedBy: dataset.CreatedBy,
		GroupID:   nil, // Datasets are workspace-scoped and have no organization compatibility override
		PermissionCodes: []workspace_model.WorkspacePermissionCode{
			workspace_model.WorkspacePermissionKnowledgeBaseUpdate,
		},
	})
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to check edit permission for dataset %s", dataset.ID), err)
		canEdit = false
	}

	response.CanEdit = canEdit
	response.IsEditor = canEdit // Keep for backward compatibility

	return response
}

func (h *DatasetHandler) convertDatasetsToResponse(datasets []*model.Dataset) []dto.DatasetResponse {
	result := make([]dto.DatasetResponse, len(datasets))
	for i, dataset := range datasets {
		result[i] = h.convertDatasetToResponse(dataset)
	}
	return result
}

// convertDatasetsToResponseWithPermission converts datasets to response with editor permission check
func (h *DatasetHandler) convertDatasetsToResponseWithPermission(ctx context.Context, datasets []*model.Dataset, accountID string) []dto.DatasetResponse {
	result := make([]dto.DatasetResponse, len(datasets))

	// Prepare resources for batch permission check
	resources := make([]interfaces.ResourcePermissionInfo, len(datasets))
	for i, dataset := range datasets {
		resources[i] = interfaces.ResourcePermissionInfo{
			ResourceID:  dataset.ID,
			WorkspaceID: dataset.WorkspaceID,
			CreatedBy:   dataset.CreatedBy,
			GroupID:     nil, // Datasets are workspace-scoped and have no organization compatibility override
			PermissionCodes: []workspace_model.WorkspacePermissionCode{
				workspace_model.WorkspacePermissionKnowledgeBaseUpdate,
			},
		}
	}

	// Batch check permissions
	permissionMap, err := h.permissionService.CheckBatchResourceEditPermission(ctx, interfaces.BatchResourcePermissionParams{
		AccountID: accountID,
		Resources: resources,
	})
	if err != nil {
		logger.Error("Failed to batch check edit permissions for datasets", err)
		// On error, default to false for all
		permissionMap = make(map[string]bool)
	}

	// Convert datasets to response with permission info
	for i, dataset := range datasets {
		response := h.convertDatasetToResponse(dataset)

		// Set can_edit from batch permission check
		canEdit, exists := permissionMap[dataset.ID]
		if !exists {
			canEdit = false
		}
		response.CanEdit = canEdit

		// Keep IsEditor for backward compatibility
		response.IsEditor = canEdit

		result[i] = response
	}
	return result
}

// Helper function to create string pointer
func stringPointer(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// RegisterRoutes registers all active dataset routes.
func (h *DatasetHandler) RegisterRoutes(router *gin.RouterGroup) {
	authWithTenant := router.Group("", middleware.JWTWithOrganizationAndService(h.accountService))

	// DatasetListApi routes: /datasets
	authWithTenant.GET("/datasets", h.GetDatasets)
	authWithTenant.POST("/datasets", h.PostDatasets)
	authWithTenant.GET("/datasets/extraction-strategies", h.GetDocumentExtractionStrategies)

	// DatasetApi routes: /datasets/:dataset_id
	authWithTenant.GET("/datasets/:dataset_id", h.GetDataset)
	authWithTenant.PATCH("/datasets/:dataset_id", h.PatchDataset)
	authWithTenant.DELETE("/datasets/:dataset_id", h.DeleteDataset)

	// DatasetIndexingEstimateApi route: /datasets/indexing-estimate
	authWithTenant.POST("/datasets/indexing-estimate", h.PostDatasetIndexingEstimate)

	// DatasetEditorPermissionApi route: /datasets/check-editor-permission/:dataset_id
	authWithTenant.GET("/datasets/check-editor-permission/:dataset_id", h.GetDatasetEditorPermission)

	// HitTestingApi route: /datasets/:dataset_id/hit-testing
	authWithTenant.POST("/datasets/:dataset_id/hit-testing", h.HitTesting)
	authWithTenant.POST("/datasets/:dataset_id/retrieve/vector", h.RetrieveVector)
	authWithTenant.POST("/datasets/:dataset_id/retrieve/graph", h.RetrieveGraph)

	// Batch hit testing API: /datasets/:dataset_id/batch-hit-testing
	authWithTenant.POST("/datasets/:dataset_id/batch-hit-testing", h.BatchHitTesting)

	// Async batch hit testing API: /datasets/:dataset_id/async-batch-hit-testing
	authWithTenant.POST("/datasets/:dataset_id/async-batch-hit-testing", h.AsyncBatchHitTesting)

	// Batch hit testing task status API: /datasets/:dataset_id/batch-hit-testing/tasks/:task_id
	authWithTenant.GET("/datasets/:dataset_id/batch-hit-testing/tasks/:task_id", h.GetBatchHitTestingTaskStatus)

	// Batch hit testing task report API: /datasets/:dataset_id/batch-hit-testing/tasks/:task_id/report
	authWithTenant.GET("/datasets/:dataset_id/batch-hit-testing/tasks/:task_id/report", h.GetBatchHitTestingTaskReport)

	// Stop batch hit testing task API: /datasets/:dataset_id/batch-hit-testing/tasks/:task_id/stop
	authWithTenant.POST("/datasets/:dataset_id/batch-hit-testing/tasks/:task_id/stop", h.StopBatchHitTestingTask)

	authWithTenant.GET("/datasets/:dataset_id/queries", h.GetDatasetQueries)
	authWithTenant.DELETE("/datasets/:dataset_id/queries/:query_id", h.DeleteDatasetQuery)

	// Random questions API: /datasets/:dataset_id/random-questions
	authWithTenant.GET("/datasets/:dataset_id/random-questions", h.GetRandomDatasetQuestions)

	// Dataset questions count API: /datasets/:dataset_id/question-count
	authWithTenant.GET("/datasets/:dataset_id/question-count", h.GetDatasetQuestionCount)

	// Save batch hit testing results API: /datasets/:dataset_id/batch-hit-testing/tasks/:task_id/save
	authWithTenant.POST("/datasets/:dataset_id/batch-hit-testing/tasks/:task_id/save", h.SaveBatchHitTestingResults)

	// DatasetErrorDocs route: /datasets/:dataset_id/error-docs
	authWithTenant.GET("/datasets/:dataset_id/error-docs", h.GetDatasetErrorDocs)
	// DatasetErrorDocs retry route: /datasets/:dataset_id/error-docs/retry
	authWithTenant.POST("/datasets/:dataset_id/error-docs/retry", h.PostDatasetErrorDocsRetry)
}
