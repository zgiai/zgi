package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
	datasetservice "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/response"
)

// SegmentHandler handles segment-related HTTP requests
type SegmentHandler struct {
	segmentService    datasetservice.SegmentService
	datasetService    datasetservice.DatasetService
	documentService   datasetservice.DocumentService
	accountService    interfaces.AccountService
	enterpriseService interfaces.OrganizationService
	authService       interfaces.AuthorizationService
}

// NewSegmentHandler creates a new SegmentHandler instance
func NewSegmentHandler(
	segmentService datasetservice.SegmentService,
	datasetService datasetservice.DatasetService,
	documentService datasetservice.DocumentService,
	accountService interfaces.AccountService,
	enterpriseService interfaces.OrganizationService,
	authServices ...interfaces.AuthorizationService,
) *SegmentHandler {
	var authService interfaces.AuthorizationService
	if len(authServices) > 0 {
		authService = authServices[0]
	}
	return &SegmentHandler{
		segmentService:    segmentService,
		datasetService:    datasetService,
		documentService:   documentService,
		accountService:    accountService,
		enterpriseService: enterpriseService,
		authService:       authService,
	}
}

func rejectDatasetSegmentMutation(c *gin.Context) {
	response.FailWithMessage(c, response.ErrSegmentUpdateFailed, "dataset segments must be edited from file management")
}

// GetDocumentSegments handles GET /datasets/{dataset_id}/documents/{document_id}/segments
func (h *SegmentHandler) GetDocumentSegments(c *gin.Context) {
	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")

	if datasetID == "" || documentID == "" {
		response.Fail(c, response.ErrDatasetIdRequired)
		return
	}

	if _, _, ok := authorizeDatasetDocumentViewAccess(c, h.datasetService, h.documentService, h.authService, datasetID, documentID); !ok {
		return
	}

	// Check dataset exists
	_, err := h.datasetService.GetDatasetByID(c.Request.Context(), datasetID)
	if err != nil {
		response.Fail(c, response.ErrDatasetNotFound)
		return
	}

	// Check document exists (using document service's GetDocumentDetail method)
	_, err = h.documentService.GetDocumentDetail(c.Request.Context(), datasetID, documentID, "")
	if err != nil {
		response.Fail(c, response.ErrDocumentNotFound)
		return
	}

	// Parse query parameters
	var req dto.SegmentListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Set defaults to match default behavior
	if req.Limit == 0 {
		req.Limit = 20
	}
	if req.Page == 0 {
		req.Page = 1
	}
	if req.Enabled == "" {
		req.Enabled = "all"
	}

	// Get segments
	segmentList, err := h.segmentService.GetSegmentsByDocument(c.Request.Context(), datasetID, documentID, &req)
	if err != nil {
		response.Fail(c, response.ErrSegmentGetFailed)
		return
	}

	// Return response
	response.Success(c, segmentList)
}

// DeleteDocumentSegments handles DELETE /datasets/{dataset_id}/documents/{document_id}/segments
func (h *SegmentHandler) DeleteDocumentSegments(c *gin.Context) {
	rejectDatasetSegmentMutation(c)
	return

	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")

	if datasetID == "" || documentID == "" {
		response.Fail(c, response.ErrDatasetIdRequired)
		return
	}

	// Get current user from context
	accountID := c.GetString("account_id")
	tenantID := c.GetString("tenant_id")
	groupID := c.GetString("group_id")
	if accountID == "" || tenantID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if groupID == "" {
		groupID = tenantID
	}

	if _, _, ok := authorizeDatasetDocumentSegmentDeleteAccess(c, h.datasetService, h.documentService, h.authService, datasetID, documentID); !ok {
		return
	}

	// Check dataset exists
	dataset, err := h.datasetService.GetDatasetByID(c.Request.Context(), datasetID)
	if err != nil {
		response.Fail(c, response.ErrDatasetNotFound)
		return
	}

	var datasetTenantID = dataset.WorkspaceID

	if h.enterpriseService != nil {
		hasPermission, err := h.enterpriseService.CheckWorkspacePermission(
			c.Request.Context(),
			groupID,
			datasetTenantID,
			accountID,
			workspace_model.WorkspacePermissionKnowledgeBaseSegmentDelete,
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

	// Check dataset permission
	hasPermission, err := h.datasetService.CheckDatasetPermission(c.Request.Context(), datasetID, accountID, tenantID)
	if err != nil || !hasPermission {
		response.Fail(c, response.ErrDatasetPermissionDenied)
		return
	}

	// Check document exists
	_, err = h.documentService.GetDocumentDetail(c.Request.Context(), datasetID, documentID, "")
	if err != nil {
		response.Fail(c, response.ErrDocumentNotFound)
		return
	}

	// Get segment IDs from query parameters
	segmentIDs := c.QueryArray("segment_id")
	if len(segmentIDs) == 0 {
		response.Fail(c, response.ErrNoSegmentIds)
		return
	}

	// Delete segments
	if err := h.segmentService.DeleteSegments(c.Request.Context(), segmentIDs, documentID, datasetID); err != nil {
		response.Fail(c, response.ErrSegmentDeleteFailed)
		return
	}

	// Return success response
	response.Success(c, gin.H{"result": "success"})
}

// CreateDocumentSegment handles POST /datasets/{dataset_id}/documents/{document_id}/segments
func (h *SegmentHandler) CreateDocumentSegment(c *gin.Context) {
	rejectDatasetSegmentMutation(c)
	return

	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")

	if datasetID == "" || documentID == "" {
		response.Fail(c, response.ErrDatasetIdRequired)
		return
	}

	// Get current user from context
	accountID := c.GetString("account_id")
	tenantID := c.GetString("tenant_id")
	groupID := c.GetString("group_id")
	if accountID == "" || tenantID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if groupID == "" {
		groupID = tenantID
	}

	if _, _, ok := authorizeDatasetDocumentSegmentUpdateAccess(c, h.datasetService, h.documentService, h.authService, datasetID, documentID); !ok {
		return
	}

	dataset, err := h.datasetService.GetDatasetByID(c.Request.Context(), datasetID)
	if err != nil || dataset == nil {
		response.Fail(c, response.ErrDatasetNotFound)
		return
	}

	var datasetTenantID = dataset.WorkspaceID

	if h.enterpriseService != nil {
		hasPermission, err := h.enterpriseService.CheckWorkspacePermission(
			c.Request.Context(),
			groupID,
			datasetTenantID,
			accountID,
			workspace_model.WorkspacePermissionKnowledgeBaseSegmentUpdate,
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

	// Check dataset permission
	hasPermission, err := h.datasetService.CheckDatasetPermission(c.Request.Context(), datasetID, accountID, tenantID)
	if err != nil || !hasPermission {
		response.Fail(c, response.ErrDatasetPermissionDenied)
		return
	}

	// Parse request body
	var req dto.SegmentCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Create segment
	segment, err := h.segmentService.CreateSegment(c.Request.Context(), documentID, datasetID, accountID, tenantID, &req)
	if err != nil {
		response.Fail(c, response.ErrSegmentCreateFailed)
		return
	}

	// Return created segment
	response.Success(c, segment)
}

// UpdateDocumentSegment handles PATCH /datasets/{dataset_id}/documents/{document_id}/segments/{segment_id}
func (h *SegmentHandler) UpdateDocumentSegment(c *gin.Context) {
	rejectDatasetSegmentMutation(c)
	return

	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")
	segmentID := c.Param("segment_id")

	if datasetID == "" || documentID == "" || segmentID == "" {
		response.Fail(c, response.ErrSegmentIdRequired)
		return
	}

	// Get current user from context
	accountID := c.GetString("account_id")
	tenantID := c.GetString("tenant_id")
	groupID := c.GetString("group_id")
	if accountID == "" || tenantID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if groupID == "" {
		groupID = tenantID
	}

	if _, _, _, ok := authorizeDatasetSegmentUpdateAccess(c, h.datasetService, h.documentService, h.segmentService, h.authService, datasetID, documentID, segmentID); !ok {
		return
	}

	// Check dataset exists
	dataset, err := h.datasetService.GetDatasetByID(c.Request.Context(), datasetID)
	if err != nil {
		response.Fail(c, response.ErrDatasetNotFound)
		return
	}

	var datasetTenantID = dataset.WorkspaceID

	if h.enterpriseService != nil {
		hasPermission, err := h.enterpriseService.CheckWorkspacePermission(
			c.Request.Context(),
			groupID,
			datasetTenantID,
			accountID,
			workspace_model.WorkspacePermissionKnowledgeBaseSegmentUpdate,
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

	// Check dataset model setting
	// TODO: DatasetService.CheckDatasetModelSetting
	// DatasetService.check_dataset_model_setting(dataset)

	// Check document exists
	// document, err := h.documentService.GetDocumentDetail(c.Request.Context(), datasetID, documentID, "")
	// if err != nil {
	// 	response.Fail(c, response.ErrDocumentNotFound)
	// 	return
	// }

	// Embedding model check removed - always use high quality mode

	// Check segment exists
	// TODO:
	// segment = DocumentSegment.query.filter(
	//     DocumentSegment.id == str(segment_id), DocumentSegment.tenant_id == current_user.current_tenant_id
	// ).first()
	// if not segment:
	//     raise NotFound("Segment not found.")

	// Check user permission - must be admin, owner, or editor
	// TODO:
	// if not current_user.is_editor:
	//     raise Forbidden()

	// Check dataset permission
	hasPermission, err := h.datasetService.CheckDatasetPermission(c.Request.Context(), datasetID, accountID, tenantID)
	if err != nil || !hasPermission {
		response.Fail(c, response.ErrDatasetPermissionDenied)
		return
	}

	// Parse request body
	var req dto.SegmentUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Validate segment args
	// TODO: SegmentService.segment_create_args_validate
	// SegmentService.segment_create_args_validate(args, document)

	// Update segment
	// TODO:
	// segment = SegmentService.update_segment(SegmentUpdateArgs(**args), segment, document, dataset)
	segment, err := h.segmentService.UpdateSegment(c.Request.Context(), segmentID, &req)
	if err != nil {
		response.Fail(c, response.ErrSegmentUpdateFailed)
		return
	}

	// Return updated segment
	// TODO: doc_form
	response.Success(c, segment)
}

// DeleteDocumentSegment handles DELETE /datasets/{dataset_id}/documents/{document_id}/segments/{segment_id}
func (h *SegmentHandler) DeleteDocumentSegment(c *gin.Context) {
	rejectDatasetSegmentMutation(c)
	return

	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")
	segmentID := c.Param("segment_id")

	if datasetID == "" || documentID == "" || segmentID == "" {
		response.Fail(c, response.ErrSegmentIdRequired)
		return
	}

	// Get current user from context
	accountID := c.GetString("account_id")
	tenantID := c.GetString("tenant_id")
	groupID := c.GetString("group_id")
	if accountID == "" || tenantID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if groupID == "" {
		groupID = tenantID
	}

	if _, _, _, ok := authorizeDatasetSegmentDeleteAccess(c, h.datasetService, h.documentService, h.segmentService, h.authService, datasetID, documentID, segmentID); !ok {
		return
	}

	dataset, err := h.datasetService.GetDatasetByID(c.Request.Context(), datasetID)
	if err != nil || dataset == nil {
		response.Fail(c, response.ErrDatasetNotFound)
		return
	}

	var datasetTenantID = dataset.WorkspaceID

	if h.enterpriseService != nil {
		hasPermission, err := h.enterpriseService.CheckWorkspacePermission(
			c.Request.Context(),
			groupID,
			datasetTenantID,
			accountID,
			workspace_model.WorkspacePermissionKnowledgeBaseSegmentDelete,
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

	// Check dataset exists
	// dataset, err := h.datasetService.GetDatasetByID(c.Request.Context(), datasetID)
	// if err != nil {
	// 	response.Fail(c, response.ErrDatasetNotFound)
	// 	return
	// }

	// Check dataset model setting
	// TODO: DatasetService.CheckDatasetModelSetting
	// DatasetService.check_dataset_model_setting(dataset)

	// Check document exists
	// _, err = h.documentService.GetDocumentDetail(c.Request.Context(), datasetID, documentID, "")
	// if err != nil {
	// 	response.Fail(c, response.ErrDocumentNotFound)
	// 	return
	// }

	// Check segment exists
	// TODO:
	// segment = DocumentSegment.query.filter(
	//     DocumentSegment.id == str(segment_id), DocumentSegment.tenant_id == current_user.current_tenant_id
	// ).first()
	// if not segment:
	//     raise NotFound("Segment not found.")

	// Check user permission - must be admin or owner
	// TODO:
	// if not current_user.is_editor:
	//     raise Forbidden()

	// Check dataset permission
	hasPermission, err := h.datasetService.CheckDatasetPermission(c.Request.Context(), datasetID, accountID, tenantID)
	if err != nil || !hasPermission {
		response.Fail(c, response.ErrDatasetPermissionDenied)
		return
	}

	// Delete segment
	// TODO:
	// SegmentService.delete_segment(segment, document, dataset)
	if err := h.segmentService.DeleteSegment(c.Request.Context(), segmentID); err != nil {
		response.Fail(c, response.ErrSegmentDeleteFailed)
		return
	}

	// Return success response
	response.Success(c, gin.H{"result": "success"})
}

// UpdateDocumentSegmentStatus handles PATCH /datasets/{dataset_id}/documents/{document_id}/segment/{action}
func (h *SegmentHandler) UpdateDocumentSegmentStatus(c *gin.Context) {
	rejectDatasetSegmentMutation(c)
	return

	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")
	action := c.Param("action")
	// Get segment IDs from query parameters
	segmentIDs := c.QueryArray("segment_id")

	if datasetID == "" || documentID == "" || action == "" {
		response.Fail(c, response.ErrDatasetIdRequired)
		return
	}

	if len(segmentIDs) == 0 {
		response.Fail(c, response.ErrSegmentIdRequired)
		return
	}

	// Get current user from context
	accountID := c.GetString("account_id")
	tenantID := c.GetString("tenant_id")
	if accountID == "" || tenantID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if _, _, ok := authorizeDatasetDocumentSegmentUpdateAccess(c, h.datasetService, h.documentService, h.authService, datasetID, documentID); !ok {
		return
	}
	for _, segmentID := range segmentIDs {
		if _, _, _, ok := authorizeDatasetSegmentUpdateAccess(c, h.datasetService, h.documentService, h.segmentService, h.authService, datasetID, documentID, segmentID); !ok {
			return
		}
	}

	// Check dataset exists
	dataset, err := h.datasetService.GetDatasetByID(c.Request.Context(), datasetID)
	if err != nil {
		response.Fail(c, response.ErrDatasetNotFound)
		return
	}

	// Check document exists
	document, err := h.documentService.GetDocumentByID(c.Request.Context(), documentID)
	if err != nil {
		response.Fail(c, response.ErrDocumentNotFound)
		return
	}

	// Check dataset model setting
	// TODO: DatasetService.CheckDatasetModelSetting
	// DatasetService.check_dataset_model_setting(dataset)

	// Check dataset permission
	hasPermission, err := h.datasetService.CheckDatasetPermission(c.Request.Context(), datasetID, accountID, tenantID)
	if err != nil || !hasPermission {
		response.Fail(c, response.ErrDatasetPermissionDenied)
		return
	}

	// Embedding model check removed - always use high quality mode

	// Check if document is being indexed
	// TODO:
	// document_indexing_cache_key = "document_{}_indexing".format(document.id)
	// cache_result = redis_client.get(document_indexing_cache_key)
	// if cache_result is not None:
	//     raise InvalidActionError("Document is being indexed, please try again later")

	// Update segments status using dataset service
	err = h.datasetService.UpdateSegmentsStatus(c.Request.Context(), segmentIDs, action, dataset, document)
	if err != nil {
		// TODO: InvalidActionError
		// if isinstance(err, InvalidActionError):
		//     raise InvalidActionError(str(e))
		response.Fail(c, response.ErrSegmentUpdateFailed)
		return
	}

	response.Success(c, gin.H{"result": "success"})
}

// CreateChildChunk handles POST /datasets/{dataset_id}/documents/{document_id}/segments/{segment_id}/child_chunks
func (h *SegmentHandler) CreateChildChunk(c *gin.Context) {
	rejectDatasetSegmentMutation(c)
	return

	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")
	segmentID := c.Param("segment_id")

	if datasetID == "" || documentID == "" || segmentID == "" {
		response.Fail(c, response.ErrSegmentIdRequired)
		return
	}

	// Get current user from context
	accountID := c.GetString("account_id")
	tenantID := c.GetString("tenant_id")
	if accountID == "" || tenantID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if _, _, _, ok := authorizeDatasetSegmentUpdateAccess(c, h.datasetService, h.documentService, h.segmentService, h.authService, datasetID, documentID, segmentID); !ok {
		return
	}

	// Check dataset exists
	_, err := h.datasetService.GetDatasetByID(c.Request.Context(), datasetID)
	if err != nil {
		response.Fail(c, response.ErrDatasetNotFound)
		return
	}

	// Check dataset permission
	hasPermission, err := h.datasetService.CheckDatasetPermission(c.Request.Context(), datasetID, accountID, tenantID)
	if err != nil || !hasPermission {
		response.Fail(c, response.ErrDatasetPermissionDenied)
		return
	}

	// Check document exists
	_, err = h.documentService.GetDocumentDetail(c.Request.Context(), datasetID, documentID, "")
	if err != nil {
		response.Fail(c, response.ErrDocumentNotFound)
		return
	}

	// Check segment exists
	segment, err := h.segmentService.GetChunkByID(c.Request.Context(), segmentID)
	if err != nil || segment == nil {
		response.Fail(c, response.ErrSegmentNotFound)
		return
	}

	// Parse request body
	var req dto.ChildChunkCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Create child chunk model
	childChunk := &model.ChildChunk{
		OrganizationID: tenantID,
		DatasetID:      datasetID,
		DocumentID:     documentID,
		SegmentID:      segmentID,
		Content:        req.Content,
		Position:       1, // Default position
		WordCount:      len([]rune(req.Content)),
		Type:           "customized", // Default type
		CreatedBy:      accountID,
	}

	// Create child chunk using segment service
	createdChildChunk, err := h.segmentService.CreateChildChunk(c.Request.Context(), childChunk)
	if err != nil {
		response.Fail(c, response.ErrChildChunkCreateFailed)
		return
	}

	// Return created child chunk
	response.Success(c, createdChildChunk)
}

// GetChildChunks handles GET /datasets/{dataset_id}/documents/{document_id}/segments/{segment_id}/child_chunks
func (h *SegmentHandler) GetChildChunks(c *gin.Context) {
	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")
	segmentID := c.Param("segment_id")

	if datasetID == "" || documentID == "" || segmentID == "" {
		response.Fail(c, response.ErrSegmentIdRequired)
		return
	}

	if _, _, _, ok := authorizeDatasetSegmentViewAccess(c, h.datasetService, h.documentService, h.segmentService, h.authService, datasetID, documentID, segmentID); !ok {
		return
	}

	// Check dataset exists
	_, err := h.datasetService.GetDatasetByID(c.Request.Context(), datasetID)
	if err != nil {
		response.Fail(c, response.ErrDatasetNotFound)
		return
	}

	// Check document exists
	_, err = h.documentService.GetDocumentDetail(c.Request.Context(), datasetID, documentID, "")
	if err != nil {
		response.Fail(c, response.ErrDocumentNotFound)
		return
	}

	// Check segment exists
	_, err = h.segmentService.GetChunkByID(c.Request.Context(), segmentID)
	if err != nil {
		response.Fail(c, response.ErrSegmentNotFound)
		return
	}

	// Parse query parameters
	var req dto.ChildChunkListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Set defaults
	if req.Limit == 0 {
		req.Limit = 20
	}
	if req.Page == 0 {
		req.Page = 1
	}

	// Get child chunks list
	childChunks, err := h.segmentService.GetChildChunks(c.Request.Context(), segmentID, documentID, datasetID, req.Page, req.Limit, req.Keyword)
	if err != nil {
		response.Fail(c, response.ErrChildChunkCreateFailed)
		return
	}

	response.Success(c, childChunks)
}

// GetChildChunk handles GET /datasets/{dataset_id}/documents/{document_id}/segments/{segment_id}/child_chunks/{child_chunk_id}
func (h *SegmentHandler) GetChildChunk(c *gin.Context) {
	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")
	segmentID := c.Param("segment_id")
	childChunkID := c.Param("child_chunk_id")

	if datasetID == "" || documentID == "" || segmentID == "" || childChunkID == "" {
		response.Fail(c, response.ErrSegmentIdRequired)
		return
	}

	if _, ok := authorizeDatasetChildChunkAccess(
		c,
		h.datasetService,
		h.documentService,
		h.segmentService,
		h.authService,
		datasetID,
		documentID,
		segmentID,
		childChunkID,
		knowledgeBaseSegmentViewPermissionCodes()...,
	); !ok {
		return
	}

	// Check dataset exists
	_, err := h.datasetService.GetDatasetByID(c.Request.Context(), datasetID)
	if err != nil {
		response.Fail(c, response.ErrDatasetNotFound)
		return
	}

	// Check document exists
	_, err = h.documentService.GetDocumentDetail(c.Request.Context(), datasetID, documentID, "")
	if err != nil {
		response.Fail(c, response.ErrDocumentNotFound)
		return
	}

	// Check segment exists
	_, err = h.segmentService.GetChunkByID(c.Request.Context(), segmentID)
	if err != nil {
		response.Fail(c, response.ErrSegmentNotFound)
		return
	}

	// Check child chunk exists
	childChunk, err := h.segmentService.GetChildChunkByID(c.Request.Context(), childChunkID)
	if err != nil || childChunk == nil {
		response.Fail(c, response.ErrChildChunkNotFound)
		return
	}

	// Convert to response DTO
	responseDTO := &dto.ChildChunkResponse{
		ID:            childChunk.ID,
		SegmentID:     childChunk.SegmentID,
		Content:       childChunk.Content,
		Position:      childChunk.Position,
		WordCount:     childChunk.WordCount,
		Type:          childChunk.Type,
		IndexNodeID:   childChunk.IndexNodeID,
		IndexNodeHash: childChunk.IndexNodeHash,
		CreatedAt:     childChunk.CreatedAt.Unix(),
		UpdatedAt:     childChunk.UpdatedAt.Unix(),
	}

	// Return child chunk
	response.Success(c, responseDTO)
}

// UpdateChildChunk handles PATCH /datasets/{dataset_id}/documents/{document_id}/segments/{segment_id}/child_chunks/{child_chunk_id}
func (h *SegmentHandler) UpdateChildChunk(c *gin.Context) {
	rejectDatasetSegmentMutation(c)
	return

	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")
	segmentID := c.Param("segment_id")
	childChunkID := c.Param("child_chunk_id")

	if datasetID == "" || documentID == "" || segmentID == "" || childChunkID == "" {
		response.Fail(c, response.ErrSegmentIdRequired)
		return
	}

	// Get current user from context
	accountID := c.GetString("account_id")
	tenantID := c.GetString("tenant_id")
	if accountID == "" || tenantID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if _, ok := authorizeDatasetChildChunkAccess(
		c,
		h.datasetService,
		h.documentService,
		h.segmentService,
		h.authService,
		datasetID,
		documentID,
		segmentID,
		childChunkID,
		workspace_model.WorkspacePermissionKnowledgeBaseSegmentUpdate,
	); !ok {
		return
	}

	// Check dataset exists
	_, err := h.datasetService.GetDatasetByID(c.Request.Context(), datasetID)
	if err != nil {
		response.Fail(c, response.ErrDatasetNotFound)
		return
	}

	// Check dataset permission
	hasPermission, err := h.datasetService.CheckDatasetPermission(c.Request.Context(), datasetID, accountID, tenantID)
	if err != nil || !hasPermission {
		response.Fail(c, response.ErrDatasetPermissionDenied)
		return
	}

	// Check document exists
	_, err = h.documentService.GetDocumentDetail(c.Request.Context(), datasetID, documentID, "")
	if err != nil {
		response.Fail(c, response.ErrDocumentNotFound)
		return
	}

	// Check segment exists
	segment, err := h.segmentService.GetChunkByID(c.Request.Context(), segmentID)
	if err != nil || segment == nil {
		response.Fail(c, response.ErrSegmentNotFound)
		return
	}

	// Check child chunk exists
	childChunk, err := h.segmentService.GetChildChunkByID(c.Request.Context(), childChunkID)
	if err != nil || childChunk == nil {
		response.Fail(c, response.ErrChildChunkNotFound)
		return
	}

	// Parse request body
	var req dto.ChildChunkUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Update fields if provided
	if req.Content != nil {
		childChunk.Content = *req.Content
		childChunk.WordCount = len([]rune(*req.Content))
	}

	if req.Position != nil {
		childChunk.Position = *req.Position
	}

	if req.Type != nil {
		childChunk.Type = *req.Type
	}

	// Set updated by
	childChunk.UpdatedBy = &accountID

	// Update child chunk using segment service
	updatedChildChunk, err := h.segmentService.UpdateChildChunk(c.Request.Context(), childChunk)
	if err != nil {
		response.Fail(c, response.ErrChildChunkUpdateFailed)
		return
	}

	// Return updated child chunk
	response.Success(c, updatedChildChunk)
}

// DeleteChildChunk handles DELETE /datasets/{dataset_id}/documents/{document_id}/segments/{segment_id}/child_chunks/{child_chunk_id}
func (h *SegmentHandler) DeleteChildChunk(c *gin.Context) {
	rejectDatasetSegmentMutation(c)
	return

	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")
	segmentID := c.Param("segment_id")
	childChunkID := c.Param("child_chunk_id")

	if datasetID == "" || documentID == "" || segmentID == "" || childChunkID == "" {
		response.Fail(c, response.ErrSegmentIdRequired)
		return
	}

	// Get current user from context
	accountID := c.GetString("account_id")
	tenantID := c.GetString("tenant_id")
	if accountID == "" || tenantID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	if _, ok := authorizeDatasetChildChunkAccess(
		c,
		h.datasetService,
		h.documentService,
		h.segmentService,
		h.authService,
		datasetID,
		documentID,
		segmentID,
		childChunkID,
		workspace_model.WorkspacePermissionKnowledgeBaseSegmentDelete,
	); !ok {
		return
	}

	// Check dataset exists
	_, err := h.datasetService.GetDatasetByID(c.Request.Context(), datasetID)
	if err != nil {
		response.Fail(c, response.ErrDatasetNotFound)
		return
	}

	// Check dataset permission
	hasPermission, err := h.datasetService.CheckDatasetPermission(c.Request.Context(), datasetID, accountID, tenantID)
	if err != nil || !hasPermission {
		response.Fail(c, response.ErrDatasetPermissionDenied)
		return
	}

	// Check document exists
	_, err = h.documentService.GetDocumentDetail(c.Request.Context(), datasetID, documentID, "")
	if err != nil {
		response.Fail(c, response.ErrDocumentNotFound)
		return
	}

	// Check segment exists
	_, err = h.segmentService.GetChunkByID(c.Request.Context(), segmentID)
	if err != nil {
		response.Fail(c, response.ErrSegmentNotFound)
		return
	}

	// Check child chunk exists
	childChunk, err := h.segmentService.GetChildChunkByID(c.Request.Context(), childChunkID)
	if err != nil || childChunk == nil {
		response.Fail(c, response.ErrChildChunkNotFound)
		return
	}

	// Delete child chunk using segment service
	err = h.segmentService.DeleteChildChunk(c.Request.Context(), childChunkID)
	if err != nil {
		response.Fail(c, response.ErrChildChunkUpdateFailed)
		return
	}

	// Return success response
	response.Success(c, gin.H{"result": "success"})
}

// GenerateQuestionsForSegment handles POST /datasets/{dataset_id}/documents/{document_id}/segments/{segment_id}/questions/generate
func (h *SegmentHandler) GenerateQuestionsForSegment(c *gin.Context) {
	// Get user info
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Get tenant ID
	organizationID := c.GetString("tenant_id")
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	// Get segment ID from path
	segmentID := c.Param("segment_id")
	if segmentID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	if _, _, _, ok := authorizeDatasetSegmentUpdateAccess(c, h.datasetService, h.documentService, h.segmentService, h.authService, c.Param("dataset_id"), c.Param("document_id"), segmentID); !ok {
		return
	}

	// Parse optional JSON body for model/count
	var genReq dto.DocumentSegmentQuestionGenerateRequest
	_ = c.ShouldBindJSON(&genReq) // ignore bind error to allow empty body

	// Get count from query parameters or request body
	count := 5 // default
	if genReq.Count != nil {
		count = *genReq.Count
	}
	if c.Query("count") != "" {
		parsedCount, err := strconv.Atoi(c.Query("count"))
		if err != nil {
			response.Fail(c, response.ErrInvalidParam)
			return
		}
		count = parsedCount
	}

	// Limit count to maximum of 10
	if count > 10 {
		count = 10
	}

	// Generate questions
	result, err := h.segmentService.GenerateQuestionsForSegment(c.Request.Context(), segmentID, count, accountID, organizationID, genReq.Model)
	if err != nil {
		response.FailWithMessage(c, response.ErrSystemError, err.Error())
		return
	}

	response.Success(c, result)
}

// DocumentSegmentQuestion handlers

// CreateDocumentSegmentQuestion handles POST /datasets/{dataset_id}/documents/{document_id}/segments/{segment_id}/questions
func (h *SegmentHandler) CreateDocumentSegmentQuestion(c *gin.Context) {
	// Get user info
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Get tenant ID
	organizationID := c.GetString("tenant_id")
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	// Get segment ID from path
	segmentID := c.Param("segment_id")
	if segmentID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if _, _, _, ok := authorizeDatasetSegmentUpdateAccess(c, h.datasetService, h.documentService, h.segmentService, h.authService, c.Param("dataset_id"), c.Param("document_id"), segmentID); !ok {
		return
	}

	// Parse request
	var req dto.DocumentSegmentQuestionCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	req.SegmentID = segmentID

	// Create question
	result, err := h.segmentService.CreateDocumentSegmentQuestion(c.Request.Context(), &req, accountID, organizationID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, result)
}

// GetDocumentSegmentQuestion handles GET /datasets/{dataset_id}/documents/{document_id}/segments/{segment_id}/questions/{question_id}
func (h *SegmentHandler) GetDocumentSegmentQuestion(c *gin.Context) {
	// Get question ID from path
	questionID := c.Param("question_id")
	if questionID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")
	segmentID := c.Param("segment_id")
	if _, _, _, ok := authorizeDatasetSegmentViewAccess(c, h.datasetService, h.documentService, h.segmentService, h.authService, datasetID, documentID, segmentID); !ok {
		return
	}

	// Get question
	result, err := h.segmentService.GetDocumentSegmentQuestionByID(c.Request.Context(), questionID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if !documentSegmentQuestionMatchesPath(result, datasetID, documentID, segmentID) {
		response.Fail(c, response.ErrSegmentNotFound)
		return
	}

	response.Success(c, result)
}

// ListDocumentSegmentQuestionsBySegment handles GET /datasets/{dataset_id}/documents/{document_id}/segments/{segment_id}/questions
func (h *SegmentHandler) ListDocumentSegmentQuestionsBySegment(c *gin.Context) {
	// Get segment ID from path
	segmentID := c.Param("segment_id")
	if segmentID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if _, _, _, ok := authorizeDatasetSegmentViewAccess(c, h.datasetService, h.documentService, h.segmentService, h.authService, c.Param("dataset_id"), c.Param("document_id"), segmentID); !ok {
		return
	}

	// Parse query parameters
	var req dto.DocumentSegmentQuestionListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// List questions
	result, err := h.segmentService.ListDocumentSegmentQuestionsBySegment(c.Request.Context(), segmentID, &req)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, result)
}

// ListDocumentSegmentQuestionsByDocument handles GET /datasets/{dataset_id}/documents/{document_id}/questions
func (h *SegmentHandler) ListDocumentSegmentQuestionsByDocument(c *gin.Context) {
	// Get document ID from path
	documentID := c.Param("document_id")
	if documentID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if _, _, ok := authorizeDatasetDocumentViewAccess(c, h.datasetService, h.documentService, h.authService, c.Param("dataset_id"), documentID); !ok {
		return
	}

	// Parse query parameters
	var req dto.DocumentSegmentQuestionListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// List questions
	result, err := h.segmentService.ListDocumentSegmentQuestionsByDocument(c.Request.Context(), documentID, &req)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, result)
}

// ListDocumentSegmentQuestionsByDataset handles GET /datasets/{dataset_id}/questions
func (h *SegmentHandler) ListDocumentSegmentQuestionsByDataset(c *gin.Context) {
	// Get dataset ID from path
	datasetID := c.Param("dataset_id")
	if datasetID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if _, ok := authorizeDatasetViewAccess(c, h.datasetService, h.authService, datasetID); !ok {
		return
	}

	// Parse query parameters
	var req dto.DocumentSegmentQuestionListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// List questions
	result, err := h.segmentService.ListDocumentSegmentQuestionsByDataset(c.Request.Context(), datasetID, &req)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, result)
}

// UpdateDocumentSegmentQuestion handles PUT /datasets/{dataset_id}/documents/{document_id}/segments/{segment_id}/questions/{question_id}
func (h *SegmentHandler) UpdateDocumentSegmentQuestion(c *gin.Context) {
	// Get user info
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Get question ID from path
	questionID := c.Param("question_id")
	if questionID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")
	segmentID := c.Param("segment_id")
	if _, _, _, ok := authorizeDatasetSegmentUpdateAccess(c, h.datasetService, h.documentService, h.segmentService, h.authService, datasetID, documentID, segmentID); !ok {
		return
	}
	question, err := h.segmentService.GetDocumentSegmentQuestionByID(c.Request.Context(), questionID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if !documentSegmentQuestionMatchesPath(question, datasetID, documentID, segmentID) {
		response.Fail(c, response.ErrSegmentNotFound)
		return
	}

	// Parse request
	var req dto.DocumentSegmentQuestionUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Update question
	result, err := h.segmentService.UpdateDocumentSegmentQuestion(c.Request.Context(), questionID, &req, accountID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, result)
}

// DeleteDocumentSegmentQuestion handles DELETE /datasets/{dataset_id}/documents/{document_id}/segments/{segment_id}/questions/{question_id}
func (h *SegmentHandler) DeleteDocumentSegmentQuestion(c *gin.Context) {
	// Get question ID from path
	questionID := c.Param("question_id")
	if questionID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	datasetID := c.Param("dataset_id")
	documentID := c.Param("document_id")
	segmentID := c.Param("segment_id")
	if _, _, _, ok := authorizeDatasetSegmentDeleteAccess(c, h.datasetService, h.documentService, h.segmentService, h.authService, datasetID, documentID, segmentID); !ok {
		return
	}
	question, err := h.segmentService.GetDocumentSegmentQuestionByID(c.Request.Context(), questionID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}
	if !documentSegmentQuestionMatchesPath(question, datasetID, documentID, segmentID) {
		response.Fail(c, response.ErrSegmentNotFound)
		return
	}

	// Delete question
	if err := h.segmentService.DeleteDocumentSegmentQuestion(c.Request.Context(), questionID); err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, nil)
}

// DeleteDocumentSegmentQuestionsBySegment handles DELETE /datasets/{dataset_id}/documents/{document_id}/segments/{segment_id}/questions
func (h *SegmentHandler) DeleteDocumentSegmentQuestionsBySegment(c *gin.Context) {
	// Get segment ID from path
	segmentID := c.Param("segment_id")
	if segmentID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if _, _, _, ok := authorizeDatasetSegmentUpdateAccess(c, h.datasetService, h.documentService, h.segmentService, h.authService, c.Param("dataset_id"), c.Param("document_id"), segmentID); !ok {
		return
	}

	// Delete questions
	if err := h.segmentService.DeleteDocumentSegmentQuestionsBySegment(c.Request.Context(), segmentID); err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, nil)
}

// DeleteDocumentSegmentQuestionsByDocument handles DELETE /datasets/{dataset_id}/documents/{document_id}/questions
func (h *SegmentHandler) DeleteDocumentSegmentQuestionsByDocument(c *gin.Context) {
	// Get document ID from path
	documentID := c.Param("document_id")
	if documentID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if _, _, ok := authorizeDatasetDocumentSegmentDeleteAccess(c, h.datasetService, h.documentService, h.authService, c.Param("dataset_id"), documentID); !ok {
		return
	}

	// Delete questions
	if err := h.segmentService.DeleteDocumentSegmentQuestionsByDocument(c.Request.Context(), documentID); err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, nil)
}

// DeleteDocumentSegmentQuestionsByDataset handles DELETE /datasets/{dataset_id}/questions
func (h *SegmentHandler) DeleteDocumentSegmentQuestionsByDataset(c *gin.Context) {
	// Get dataset ID from path
	datasetID := c.Param("dataset_id")
	if datasetID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if _, ok := authorizeDatasetSegmentDeleteAccessByDataset(c, h.datasetService, h.authService, datasetID); !ok {
		return
	}

	// Delete questions
	if err := h.segmentService.DeleteDocumentSegmentQuestionsByDataset(c.Request.Context(), datasetID); err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, nil)
}

// BatchCreateDocumentSegmentQuestions handles POST /datasets/{dataset_id}/documents/{document_id}/segments/{segment_id}/questions/batch
func (h *SegmentHandler) BatchCreateDocumentSegmentQuestions(c *gin.Context) {
	// Get user info
	accountID := c.GetString("account_id")
	if accountID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return
	}

	// Get tenant ID
	organizationID := c.GetString("tenant_id")
	if organizationID == "" {
		response.Fail(c, response.ErrInvalidTenantId)
		return
	}

	// Get segment ID from path
	segmentID := c.Param("segment_id")
	if segmentID == "" {
		response.Fail(c, response.ErrInvalidParam)
		return
	}
	if _, _, _, ok := authorizeDatasetSegmentUpdateAccess(c, h.datasetService, h.documentService, h.segmentService, h.authService, c.Param("dataset_id"), c.Param("document_id"), segmentID); !ok {
		return
	}

	// Parse request
	var req dto.DocumentSegmentQuestionBatchCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam)
		return
	}

	// Batch create questions
	result, err := h.segmentService.BatchCreateDocumentSegmentQuestions(c.Request.Context(), &req, accountID, organizationID, segmentID)
	if err != nil {
		response.Fail(c, response.ErrSystemError)
		return
	}

	response.Success(c, result)
}

// RegisterRoutes registers segment-related routes with JWT authentication
func (h *SegmentHandler) RegisterRoutes(router *gin.RouterGroup) {
	// Import middleware package
	// Setup middleware groups - same as dataset handler
	authWithTenant := router.Group("", middleware.JWTWithOrganizationAndService(h.accountService))

	// Document segments routes
	segments := authWithTenant.Group("/datasets/:dataset_id/documents/:document_id/segments")
	{
		segments.GET("", h.GetDocumentSegments)       // GET segments list
		segments.DELETE("", h.DeleteDocumentSegments) // DELETE multiple segments
		segments.POST("", h.CreateDocumentSegment)    // POST create segment
	}

	// Individual segment routes with authentication
	segment := authWithTenant.Group("/datasets/:dataset_id/documents/:document_id/segments/:segment_id")
	{
		segment.PATCH("", h.UpdateDocumentSegment)  // PATCH update segment
		segment.DELETE("", h.DeleteDocumentSegment) // DELETE single segment
	}

	// Document segment status update route with authentication
	// /datasets/<uuid:dataset_id>/documents/<uuid:document_id>/segment/<string:action>
	segmentAction := authWithTenant.Group("/datasets/:dataset_id/documents/:document_id/segment/:action")
	{
		segmentAction.PATCH("", h.UpdateDocumentSegmentStatus) // PATCH update segment status
	}

	// Child chunks routes with authentication
	childChunks := authWithTenant.Group("/datasets/:dataset_id/documents/:document_id/segments/:segment_id/child_chunks")
	{
		childChunks.GET("", h.GetChildChunks)                      // GET list child chunks
		childChunks.POST("", h.CreateChildChunk)                   // POST create child chunk
		childChunks.PATCH("/:child_chunk_id", h.UpdateChildChunk)  // PATCH update child chunk
		childChunks.DELETE("/:child_chunk_id", h.DeleteChildChunk) // DELETE delete child chunk
	}

	// Document segment questions routes with authentication
	questions := authWithTenant.Group("/datasets/:dataset_id/documents/:document_id/segments/:segment_id/questions")
	{
		questions.POST("", h.CreateDocumentSegmentQuestion)
		questions.GET("", h.ListDocumentSegmentQuestionsBySegment)
		questions.GET("/:question_id", h.GetDocumentSegmentQuestion)
		questions.PUT("/:question_id", h.UpdateDocumentSegmentQuestion)
		questions.DELETE("/:question_id", h.DeleteDocumentSegmentQuestion)
		questions.DELETE("", h.DeleteDocumentSegmentQuestionsBySegment)
		questions.POST("/batch", h.BatchCreateDocumentSegmentQuestions)
		questions.POST("/generate", h.GenerateQuestionsForSegment)
	}

	documentQuestions := authWithTenant.Group("/datasets/:dataset_id/documents/:document_id/questions")
	{
		documentQuestions.GET("", h.ListDocumentSegmentQuestionsByDocument)
		documentQuestions.DELETE("", h.DeleteDocumentSegmentQuestionsByDocument)
	}

	datasetQuestions := authWithTenant.Group("/datasets/:dataset_id/questions")
	{
		datasetQuestions.GET("", h.ListDocumentSegmentQuestionsByDataset)
		datasetQuestions.DELETE("", h.DeleteDocumentSegmentQuestionsByDataset)
	}
}

func documentSegmentQuestionMatchesPath(question *dto.DocumentSegmentQuestionResponse, datasetID, documentID, segmentID string) bool {
	return question != nil &&
		question.DatasetID == datasetID &&
		question.DocumentID == documentID &&
		question.SegmentID == segmentID
}
