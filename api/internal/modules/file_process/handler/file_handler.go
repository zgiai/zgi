package handler

import (
	"context"
	"errors"
	"io"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/zgiai/zgi/api/config"
	datalibrarymodel "github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	datalibraryservice "github.com/zgiai/zgi/api/internal/modules/datalibrary/service"
	"github.com/zgiai/zgi/api/internal/modules/file_process/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/file_process/model"
	file_model "github.com/zgiai/zgi/api/internal/modules/file_process/model"

	"github.com/zgiai/zgi/api/internal/util"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/response"
)

// FileHandler handles file-related HTTP requests
type FileHandler struct {
	fileService       interfaces.FileService
	fileFolderService service.FileFolderService
	accountService    interfaces.AccountService
	tenantService     interfaces.WorkspaceManagementService
	enterpriseService interfaces.OrganizationService
	assetStateService datalibraryservice.FileAssetProcessingStateService
	processingService datalibraryservice.ProcessingRequestService
	validator         *validator.Validate
}

type FileAssetProcessingServices struct {
	StateService      datalibraryservice.FileAssetProcessingStateService
	ProcessingService datalibraryservice.ProcessingRequestService
}

const (
	UploadProcessingModeStoreOnly  = "store_only"
	UploadProcessingModeProcessNow = "process_now"

	FileProcessingRequestModeParseNow             = "parse_now"
	FileProcessingRequestModeReparse              = "reparse"
	FileProcessingRequestModeGenerateAfterConfirm = "generate_after_confirm"
)

var (
	errInvalidFileProcessingRequestMode   = errors.New("file processing request mode is invalid")
	errFileProcessingRequestStateInvalid  = errors.New("file processing request state is invalid")
	errFileProcessingRequestAlreadyActive = errors.New("file processing request is already active")
)

type fileServiceWithUploadOptions interface {
	UploadFileWithOptions(ctx context.Context, filename string, content []byte, mimeType string, userID, organizationID string, userRole model.CreatedByRole, source *interfaces.FileSource, workspaceID *string, isTemporary bool, isIcon bool, options service.UploadFileOptions) (*dto.UploadFile, error)
}

type fileProcessingRequest struct {
	TargetLevel string `json:"target_level"`
	Mode        string `json:"mode"`
	Force       bool   `json:"force"`
}

type queuedFileProcessingRequest struct {
	Asset             *datalibrarymodel.DocumentAsset
	ProcessingRequest *datalibraryservice.ProcessingRequestView
	ProcessingRunID   *uuid.UUID
	GenerationNo      int64
}

// NewFileHandler creates a new file handler instance
func NewFileHandler(
	fileService interfaces.FileService,
	fileFolderService service.FileFolderService,
	accountService interfaces.AccountService,
	tenantService interfaces.WorkspaceManagementService,
	enterpriseService interfaces.OrganizationService,
	assetProcessingServices ...FileAssetProcessingServices,
) *FileHandler {
	var assetStateService datalibraryservice.FileAssetProcessingStateService
	var processingService datalibraryservice.ProcessingRequestService
	if len(assetProcessingServices) > 0 {
		assetStateService = assetProcessingServices[0].StateService
		processingService = assetProcessingServices[0].ProcessingService
	}
	return &FileHandler{
		fileService:       fileService,
		fileFolderService: fileFolderService,
		accountService:    accountService,
		tenantService:     tenantService,
		enterpriseService: enterpriseService,
		assetStateService: assetStateService,
		processingService: processingService,
		validator:         validator.New(),
	}
}

// businessError is a helper function for business errors
func (h *FileHandler) businessError(c *gin.Context, errorCode response.ErrorCode) {
	response.Fail(c, errorCode)
}

// businessErrorWithMessage is a helper function for business errors with custom message
func (h *FileHandler) businessErrorWithMessage(c *gin.Context, errorCode response.ErrorCode, message string) {
	response.FailWithMessage(c, errorCode, message)
}

func normalizeUploadProcessingMode(raw string) (string, bool) {
	mode := strings.TrimSpace(raw)
	if mode == "" {
		return UploadProcessingModeProcessNow, true
	}
	switch mode {
	case UploadProcessingModeStoreOnly, UploadProcessingModeProcessNow:
		return mode, true
	default:
		return "", false
	}
}

func normalizeFileProcessingRequestMode(raw string) (string, bool) {
	mode := strings.TrimSpace(raw)
	if mode == "" {
		return FileProcessingRequestModeParseNow, true
	}
	switch mode {
	case FileProcessingRequestModeParseNow,
		FileProcessingRequestModeReparse,
		FileProcessingRequestModeGenerateAfterConfirm:
		return mode, true
	default:
		return "", false
	}
}

func normalizeFileProcessingTargetLevel(raw string) string {
	targetLevel := strings.TrimSpace(raw)
	if targetLevel == "" {
		return datalibrarymodel.DocumentProcessingLevelVectorize
	}
	return targetLevel
}

func validateFileProcessingRequestState(asset *datalibrarymodel.DocumentAsset, mode string, force bool) error {
	if asset == nil {
		return datalibraryservice.ErrDocumentAssetNotFound
	}
	switch asset.ProductStatus {
	case datalibrarymodel.DocumentAssetProductStatusParsing,
		datalibrarymodel.DocumentAssetProductStatusGenerating:
		if !force {
			return errFileProcessingRequestAlreadyActive
		}
	}

	switch mode {
	case FileProcessingRequestModeParseNow:
		switch asset.ProductStatus {
		case datalibrarymodel.DocumentAssetProductStatusStoredOnly,
			datalibrarymodel.DocumentAssetProductStatusParseFailed:
			return nil
		case datalibrarymodel.DocumentAssetProductStatusParsing,
			datalibrarymodel.DocumentAssetProductStatusGenerating:
			if force {
				return nil
			}
		}
	case FileProcessingRequestModeReparse:
		switch asset.ProductStatus {
		case datalibrarymodel.DocumentAssetProductStatusReady,
			datalibrarymodel.DocumentAssetProductStatusParseFailed,
			datalibrarymodel.DocumentAssetProductStatusConfirming:
			return nil
		case datalibrarymodel.DocumentAssetProductStatusParsing,
			datalibrarymodel.DocumentAssetProductStatusGenerating:
			if force {
				return nil
			}
		}
	case FileProcessingRequestModeGenerateAfterConfirm:
		if asset.ProductStatus == datalibrarymodel.DocumentAssetProductStatusConfirming {
			return nil
		}
	default:
		return errInvalidFileProcessingRequestMode
	}
	return errFileProcessingRequestStateInvalid
}

// GetUploadConfig gets file upload configuration
// GET /files/upload
func (h *FileHandler) GetUploadConfig(c *gin.Context) {
	config := h.fileService.GetUploadConfig()
	response.Success(c, config)
}

// UploadFile handles file upload
// POST /files/upload
func (h *FileHandler) UploadFile(c *gin.Context) {
	// Get current user information
	accountID := c.GetString("account_id")
	if accountID == "" {
		h.businessError(c, response.ErrUnauthorized)
		return
	}

	// Get canonical organization scope
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		h.businessError(c, response.ErrInvalidTenantId)
		return
	}

	// Get is_temporary parameter for temporary file uploads
	isTemporary := c.PostForm("is_temporary") == "true"

	// Get is_icon parameter for icon uploads (will resize to 200x200)
	isIcon := c.PostForm("is_icon") == "true"

	processingMode, ok := normalizeUploadProcessingMode(c.PostForm("processing_mode"))
	if !ok {
		h.businessError(c, response.ErrInvalidParam)
		return
	}

	// Get team_tenant_id parameter and validate permission if provided
	teamTenantIDStr := c.PostForm("team_tenant_id")
	workSpaceIDStr := c.PostForm("workspace_id")
	if workSpaceIDStr != "" {
		teamTenantIDStr = workSpaceIDStr
	}
	var teamTenantID *string
	if teamTenantIDStr != "" {
		if _, err := uuid.Parse(teamTenantIDStr); err != nil {
			h.businessError(c, response.ErrInvalidParam)
			return
		}

		hasAccess := false

		if h.tenantService != nil {
			role, err := h.tenantService.GetUserRole(c.Request.Context(), accountID, teamTenantIDStr)
			if err != nil {
				h.businessError(c, response.ErrSystemError)
				return
			}
			if role != nil {
				hasAccess = true
			}
		}

		if !hasAccess {
			groupRole, err := h.accountService.GetOrganizationRoleByWorkspaceID(c.Request.Context(), accountID, teamTenantIDStr)
			if err == nil && (groupRole == "owner" || groupRole == "admin") {
				hasAccess = true
			}
		}

		if !hasAccess {
			h.businessError(c, response.ErrPermissionDenied)
			return
		}

		if h.enterpriseService != nil {
			hasPermission, err := h.enterpriseService.CheckWorkspacePermission(
				c.Request.Context(),
				organizationID,
				teamTenantIDStr,
				accountID,
				workspace_model.WorkspacePermissionFileUploadCreate,
			)
			if err != nil {
				h.businessError(c, response.ErrSystemError)
				return
			}
			if !hasPermission {
				h.businessError(c, response.ErrPermissionDenied)
				return
			}
		}

		teamTenantID = &teamTenantIDStr
	}

	// Check if file exists
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		h.businessError(c, response.ErrNoFileUploaded)
		return
	}
	defer file.Close()

	// Check if only one file is uploaded
	if err := c.Request.ParseMultipartForm(32 << 20); err == nil { // 32MB
		if len(c.Request.MultipartForm.File) > 1 {
			h.businessError(c, response.ErrTooManyFiles)
			return
		}
	}

	// Check if filename exists
	if header.Filename == "" {
		h.businessError(c, response.ErrFilenameRequired)
		return
	}
	fileExtension := strings.TrimPrefix(strings.ToLower(filepath.Ext(header.Filename)), ".")
	shouldUseAssetProcessing := !isTemporary && !isIcon && model.IsDocumentExtension(fileExtension) && h.assetStateService != nil

	// Get folder_id parameter
	folderID := c.PostForm("folder_id")
	var targetFolderID *string
	if folderID != "" {
		// Validate UUID format
		if _, err := uuid.Parse(folderID); err != nil {
			h.businessError(c, response.ErrInvalidParam)
			return
		}
		targetFolderID = &folderID
	}

	// Get source parameter
	sourceStr := c.PostForm("source")
	var source *interfaces.FileSource
	if sourceStr == "datasets" {
		// Check if user has datasets editing permission
		account, err := h.accountService.GetAccountByID(c.Request.Context(), accountID)
		if err != nil {
			h.businessError(c, response.ErrAccountNotFound)
			return
		}

		// Check if user is a dataset editor (needs to be adjusted based on actual permission logic)
		if !h.isDatasetEditor(account) {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}

		fileSource := interfaces.FileSourceDatasets
		source = &fileSource
	}

	// Read file content
	content, err := io.ReadAll(file)
	if err != nil {
		h.businessError(c, response.ErrFileReadFailed)
		return
	}

	// Get MIME type
	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	uploadFile, err := h.uploadFile(c.Request.Context(), header.Filename, content, mimeType, accountID, organizationID, source, teamTenantID, isTemporary, isIcon, shouldUseAssetProcessing)

	if err != nil {
		switch err {
		case file_model.ErrFileTooLarge:
			h.businessError(c, response.ErrFileTooLarge)
		case file_model.ErrUnsupportedFileType:
			h.businessError(c, response.ErrUnsupportedFileType)
		default:
			// Check if it's a quota error
			errMsg := err.Error()
			if strings.Contains(errMsg, "storage quota exceeded") || strings.Contains(errMsg, "storage space quota insufficient") {
				h.businessErrorWithMessage(c, response.ErrQuotaStorageExceeded, errMsg)
			} else if strings.Contains(errMsg, "OCR") || strings.Contains(errMsg, "ocr_pages") || strings.Contains(errMsg, "quota limit not found") {
				// OCR quota related errors
				h.businessErrorWithMessage(c, response.ErrQuotaOCRPagesExceeded, errMsg)
			} else if strings.Contains(errMsg, "quota") {
				// Generic quota error
				h.businessErrorWithMessage(c, response.ErrQuotaExceeded, errMsg)
			} else {
				// Return the actual error message for debugging
				h.businessErrorWithMessage(c, response.ErrorCode{Code: 210002, Message: "Failed to upload file", UserVisible: true}, errMsg)
			}
		}
		return
	}

	// If folder_id is specified, add file to folder
	// Do not attach temporary files to organization folders
	if targetFolderID != nil && !isTemporary {
		err := h.fileFolderService.AddFileToFolder(c.Request.Context(), uploadFile.ID, *targetFolderID, accountID)
		if err != nil {
			// Log error but don't fail the upload
			logger.WarnContext(c.Request.Context(), "failed to add uploaded file to folder", "file_id", uploadFile.ID, "folder_id", *targetFolderID, err)
		}
	}

	asset, err := h.attachAssetProcessing(c.Request.Context(), uploadFile, organizationID, accountID, processingMode, shouldUseAssetProcessing)
	if err != nil {
		logger.WarnContext(c.Request.Context(), "failed to attach file asset processing", "file_id", uploadFile.ID, "processing_mode", processingMode, err)
		h.businessError(c, response.ErrSystemError)
		return
	}

	// Build response
	fileResponse := dto.NewFileUploadResponse(uploadFile)
	fileResponse.ProcessingMode = processingMode
	if asset != nil {
		fileResponse.AssetID = asset.ID.String()
		fileResponse.ProcessingStatus = asset.ProductStatus
		fileResponse.GenerationNo = asset.GenerationNo
		if asset.ActiveProcessingRequestID != nil {
			fileResponse.ProcessingRequestID = asset.ActiveProcessingRequestID.String()
		}
		if asset.ProcessingRunID != nil {
			fileResponse.ProcessingRunID = asset.ProcessingRunID.String()
		}
	}
	response.Success(c, fileResponse)
}

// CreateProcessingRequest starts parsing, reparsing, or post-confirm generation for a file asset.
// POST /files/:file_id/processing-requests
func (h *FileHandler) CreateProcessingRequest(c *gin.Context) {
	if h.assetStateService == nil || h.processingService == nil {
		h.businessErrorWithMessage(c, response.ErrSystemError, "file asset processing service is not available")
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		h.businessError(c, response.ErrUnauthorized)
		return
	}
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		h.businessError(c, response.ErrInvalidTenantId)
		return
	}

	fileID := c.Param("file_id")
	if fileID == "" {
		h.businessError(c, response.ErrFileIdRequired)
		return
	}
	uploadFile, ok := h.getAuthorizedFileForDownload(c, fileID)
	if !ok {
		return
	}
	if uploadFile.IsTemporary || !model.IsDocumentExtension(strings.TrimPrefix(strings.ToLower(uploadFile.Extension), ".")) {
		h.businessError(c, response.ErrUnsupportedFileType)
		return
	}

	var req fileProcessingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.businessError(c, response.ErrInvalidParams)
		return
	}
	mode, ok := normalizeFileProcessingRequestMode(req.Mode)
	if !ok {
		h.businessError(c, response.ErrInvalidParams)
		return
	}
	targetLevel := normalizeFileProcessingTargetLevel(req.TargetLevel)

	result, err := h.createQueuedFileProcessingRequest(c.Request.Context(), uploadFile, organizationID, accountID, targetLevel, mode, req.Force)
	if err != nil {
		h.handleFileProcessingRequestError(c, err)
		return
	}

	response.Success(c, gin.H{
		"asset":                result.Asset,
		"processing_request":   result.ProcessingRequest,
		"processing_run_id":    uuidPointerString(result.ProcessingRunID),
		"generation_no":        result.GenerationNo,
		"file_id":              uploadFile.ID,
		"target_level":         targetLevel,
		"mode":                 mode,
		"request_queue_status": result.ProcessingRequest.Status,
	})
}

func (h *FileHandler) handleFileProcessingRequestError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errInvalidFileProcessingRequestMode),
		errors.Is(err, errFileProcessingRequestStateInvalid),
		errors.Is(err, datalibraryservice.ErrProcessingLevelRequired),
		errors.Is(err, datalibraryservice.ErrProcessingLevelInvalid),
		errors.Is(err, datalibraryservice.ErrProcessingRequestTransitionInvalid):
		h.businessErrorWithMessage(c, response.ErrInvalidParams, err.Error())
	case errors.Is(err, errFileProcessingRequestAlreadyActive):
		h.businessErrorWithMessage(c, response.ErrInvalidParams, err.Error())
	case errors.Is(err, datalibraryservice.ErrDocumentAssetNotFound),
		errors.Is(err, datalibraryservice.ErrProcessingRequestNotFound):
		h.businessError(c, response.ErrNotFound)
	case errors.Is(err, datalibraryservice.ErrOrganizationIDRequired):
		h.businessError(c, response.ErrUnauthorized)
	default:
		logger.WarnContext(c.Request.Context(), "failed to create file processing request", err)
		h.businessError(c, response.ErrSystemError)
	}
}

func uuidPointerString(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func (h *FileHandler) uploadFile(ctx context.Context, filename string, content []byte, mimeType string, accountID string, organizationID string, source *interfaces.FileSource, teamTenantID *string, isTemporary bool, isIcon bool, useAssetProcessing bool) (*dto.UploadFile, error) {
	if uploadSvc, ok := h.fileService.(fileServiceWithUploadOptions); ok {
		return uploadSvc.UploadFileWithOptions(
			ctx,
			filename,
			content,
			mimeType,
			accountID,
			organizationID,
			model.CreatedByRoleAccount,
			source,
			teamTenantID,
			isTemporary,
			isIcon,
			service.UploadFileOptions{
				StartLegacyContentExtraction: !useAssetProcessing,
			},
		)
	}

	return h.fileService.UploadFile(
		ctx,
		filename,
		content,
		mimeType,
		accountID,
		organizationID,
		model.CreatedByRoleAccount,
		source,
		teamTenantID,
		isTemporary,
		isIcon,
	)
}

func (h *FileHandler) attachAssetProcessing(ctx context.Context, uploadFile *dto.UploadFile, organizationID string, accountID string, processingMode string, useAssetProcessing bool) (*datalibrarymodel.DocumentAsset, error) {
	if !useAssetProcessing || h.assetStateService == nil {
		return nil, nil
	}

	asset, _, err := h.assetStateService.CreateOrReuseStoredAsset(ctx, datalibraryservice.FileAssetCreateInput{
		OrganizationID: organizationID,
		WorkspaceID:    uploadFile.WorkspaceID,
		Title:          uploadFile.Name,
		SourceFileID:   uploadFile.ID,
		ContentHash:    uploadFile.Hash,
		CreatedBy:      accountID,
	})
	if err != nil {
		return nil, err
	}
	if processingMode != UploadProcessingModeProcessNow {
		return asset, nil
	}

	result, err := h.beginAndQueueRunProcessingRequest(ctx, asset, uploadFile.ID, organizationID, accountID, datalibrarymodel.DocumentProcessingLevelVectorize, FileProcessingRequestModeParseNow, false)
	if err != nil {
		return nil, err
	}
	return result.Asset, nil
}

func (h *FileHandler) createQueuedFileProcessingRequest(ctx context.Context, uploadFile *dto.UploadFile, organizationID string, accountID string, targetLevel string, mode string, force bool) (*queuedFileProcessingRequest, error) {
	asset, _, err := h.assetStateService.CreateOrReuseStoredAsset(ctx, datalibraryservice.FileAssetCreateInput{
		OrganizationID: organizationID,
		WorkspaceID:    uploadFile.WorkspaceID,
		Title:          uploadFile.Name,
		SourceFileID:   uploadFile.ID,
		ContentHash:    uploadFile.Hash,
		CreatedBy:      accountID,
	})
	if err != nil {
		return nil, err
	}
	if err := validateFileProcessingRequestState(asset, mode, force); err != nil {
		return nil, err
	}

	switch mode {
	case FileProcessingRequestModeParseNow, FileProcessingRequestModeReparse:
		return h.beginAndQueueRunProcessingRequest(ctx, asset, uploadFile.ID, organizationID, accountID, targetLevel, mode, force)
	case FileProcessingRequestModeGenerateAfterConfirm:
		return h.queueGenerateAfterConfirmRequest(ctx, asset, uploadFile.ID, organizationID, accountID, targetLevel, force)
	default:
		return nil, errInvalidFileProcessingRequestMode
	}
}

func (h *FileHandler) beginAndQueueRunProcessingRequest(ctx context.Context, asset *datalibrarymodel.DocumentAsset, uploadFileID string, organizationID string, accountID string, targetLevel string, mode string, force bool) (*queuedFileProcessingRequest, error) {
	result, err := h.assetStateService.BeginProcessingRequest(ctx, datalibraryservice.BeginProcessingRequestInput{
		OrganizationID: organizationID,
		WorkspaceID:    asset.WorkspaceID,
		AssetID:        asset.ID,
		TargetLevel:    targetLevel,
		RequestedBy:    accountID,
		Force:          force,
		IncrementRun:   true,
		Metadata: map[string]any{
			"source":         "file_processing_request",
			"mode":           mode,
			"upload_file_id": uploadFileID,
		},
	})
	if err != nil {
		return nil, err
	}
	queued, err := h.processingService.QueueRequest(ctx, organizationID, result.ProcessingRequest.ID)
	if err != nil {
		return nil, err
	}
	return &queuedFileProcessingRequest{
		Asset:             result.Asset,
		ProcessingRequest: queued,
		ProcessingRunID:   &result.ProcessingRunID,
		GenerationNo:      result.GenerationNo,
	}, nil
}

func (h *FileHandler) queueGenerateAfterConfirmRequest(ctx context.Context, asset *datalibrarymodel.DocumentAsset, uploadFileID string, organizationID string, accountID string, targetLevel string, force bool) (*queuedFileProcessingRequest, error) {
	if asset.ProcessingRunID == nil || asset.GenerationNo == 0 {
		return nil, errFileProcessingRequestStateInvalid
	}
	planned, err := h.processingService.CreatePlannedRequest(ctx, datalibraryservice.ProcessingRequest{
		OrganizationID: organizationID,
		WorkspaceID:    asset.WorkspaceID,
		AssetID:        asset.ID,
		TargetLevel:    targetLevel,
		RequestedBy:    accountID,
		Force:          force,
		RequestMetadata: map[string]any{
			"source":            "file_processing_request",
			"mode":              FileProcessingRequestModeGenerateAfterConfirm,
			"upload_file_id":    uploadFileID,
			"processing_run_id": asset.ProcessingRunID.String(),
			"generation_no":     asset.GenerationNo,
		},
	})
	if err != nil {
		return nil, err
	}
	queued, err := h.processingService.QueueRequest(ctx, organizationID, planned.ID)
	if err != nil {
		return nil, err
	}
	return &queuedFileProcessingRequest{
		Asset:             asset,
		ProcessingRequest: queued,
		ProcessingRunID:   asset.ProcessingRunID,
		GenerationNo:      asset.GenerationNo,
	}, nil
}

// GetFilePreview gets file preview content
// GET /files/:file_id/preview
func (h *FileHandler) GetFilePreview(c *gin.Context) {
	fileID := c.Param("file_id")
	if fileID == "" {
		h.businessError(c, response.ErrFileIdRequired)
		return
	}

	// Check for ocr parameter
	enableOCR := c.Query("ocr")
	var content string
	var err error

	if enableOCR != "" {
		ocrEnabled := enableOCR == "true"
		// Call service layer to get file preview with OCR control
		content, err = h.fileService.GetFilePreviewWithOCR(c.Request.Context(), fileID, ocrEnabled)
	} else {
		// Call service layer to get file preview
		content, err = h.fileService.GetFilePreview(c.Request.Context(), fileID)
	}

	if err != nil {
		switch err {
		case file_model.ErrFileNotFound:
			h.businessError(c, response.ErrFileNotFound)
		case file_model.ErrUnsupportedFileType:
			h.businessError(c, response.ErrUnsupportedFileType)
		default:
			h.businessError(c, response.ErrFilePreviewFailed)
		}
		return
	}

	// Build response
	previewResponse := &dto.FilePreviewResponse{
		Content: content,
	}
	response.Success(c, previewResponse)
}

// GetSupportedFileTypes gets supported file types
// GET /files/support-type
func (h *FileHandler) GetSupportedFileTypes(c *gin.Context) {
	supportedTypes := h.fileService.GetSupportedFileTypes()

	// Build response
	supportResponse := &dto.FileSupportTypeResponse{
		AllowedExtensions: supportedTypes,
	}
	response.Success(c, supportResponse)
}

// GetFileOriginalPreviewURL returns a signed original-file preview URL.
// GET /files/:file_id/preview-url
func (h *FileHandler) GetFileOriginalPreviewURL(c *gin.Context) {
	fileID := c.Param("file_id")
	if fileID == "" {
		h.businessError(c, response.ErrFileIdRequired)
		return
	}

	uploadFile, ok := h.getAuthorizedFileForDownload(c, fileID)
	if !ok {
		return
	}
	if !isOriginalPreviewSupported(uploadFile) {
		h.businessError(c, response.ErrUnsupportedFileType)
		return
	}

	previewURL, err := h.fileService.GetFileURL(c.Request.Context(), fileID)
	if err != nil {
		h.businessError(c, response.ErrFilePreviewFailed)
		return
	}

	response.Success(c, &dto.FileOriginalPreviewURLResponse{
		URL:       previewURL,
		FileID:    uploadFile.ID,
		Name:      uploadFile.Name,
		Extension: uploadFile.Extension,
		MimeType:  uploadFile.MimeType,
	})
}

// GetFilesMetadata returns authorized metadata for files by ID.
// GET /files/metadata?file_ids=...
func (h *FileHandler) GetFilesMetadata(c *gin.Context) {
	fileIDs := c.QueryArray("file_ids")
	if len(fileIDs) == 0 {
		h.businessError(c, response.ErrFileIdRequired)
		return
	}

	seen := make(map[string]struct{}, len(fileIDs))
	files := make([]dto.UploadFile, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		fileID = strings.TrimSpace(fileID)
		if fileID == "" {
			continue
		}
		if _, ok := seen[fileID]; ok {
			continue
		}
		seen[fileID] = struct{}{}

		uploadFile, ok := h.getAuthorizedFileForDownload(c, fileID)
		if !ok {
			return
		}
		files = append(files, *uploadFile)
	}

	response.Success(c, &dto.FileMetadataListResponse{Data: files})
}

// CreateTextFile creates a text file from provided content and uploads it
// POST /files/text
func (h *FileHandler) CreateTextFile(c *gin.Context) {
	// Get current user information
	accountID := c.GetString("account_id")
	if accountID == "" {
		h.businessError(c, response.ErrUnauthorized)
		return
	}

	// Get canonical organization scope
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		h.businessError(c, response.ErrInvalidTenantId)
		return
	}

	// Parse request
	var req dto.CreateTextFileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.businessError(c, response.ErrInvalidParam)
		return
	}

	// Validate filename
	if req.Filename == "" {
		h.businessError(c, response.ErrFilenameRequired)
		return
	}

	// Ensure the file has a .txt extension
	filename := req.Filename
	if filepath.Ext(filename) == "" {
		filename += ".txt"
	}

	// Convert content to bytes
	content := []byte(req.Content)

	// Set MIME type for text files
	mimeType := "text/plain"

	// Determine whether this is a temporary file
	isTemporary := false

	// Handle source parameter
	var source *interfaces.FileSource
	if req.Source == "datasets" {
		// Check if user has datasets editing permission
		account, err := h.accountService.GetAccountByID(c.Request.Context(), accountID)
		if err != nil {
			h.businessError(c, response.ErrAccountNotFound)
			return
		}

		// Check if user is a dataset editor
		if !h.isDatasetEditor(account) {
			response.Fail(c, response.ErrPermissionDenied)
			return
		}

		fileSource := interfaces.FileSourceDatasets
		source = &fileSource
	}

	// Call service layer to upload file
	var teamTenantID *string

	workspaceID := req.TeamTenantID
	if req.WorkspaceID != nil && *req.WorkspaceID != "" {
		workspaceID = req.WorkspaceID
	}
	if workspaceID != nil && *workspaceID != "" {
		if _, err := uuid.Parse(*workspaceID); err != nil {
			h.businessError(c, response.ErrInvalidParam)
			return
		}
		hasAccess := false
		if h.tenantService != nil {
			role, err := h.tenantService.GetUserRole(c.Request.Context(), accountID, *workspaceID)
			if err != nil {
				h.businessError(c, response.ErrSystemError)
				return
			}
			if role != nil {
				hasAccess = true
			}
		}
		if !hasAccess {
			groupRole, err := h.accountService.GetOrganizationRoleByWorkspaceID(c.Request.Context(), accountID, *workspaceID)
			if err == nil && (groupRole == "owner" || groupRole == "admin") {
				hasAccess = true
			}
		}
		if !hasAccess {
			h.businessError(c, response.ErrPermissionDenied)
			return
		}
		if h.enterpriseService != nil {
			hasPermission, err := h.enterpriseService.CheckWorkspacePermission(
				c.Request.Context(),
				organizationID,
				*workspaceID,
				accountID,
				workspace_model.WorkspacePermissionFileUploadCreate,
			)
			if err != nil {
				h.businessError(c, response.ErrSystemError)
				return
			}
			if !hasPermission {
				h.businessError(c, response.ErrPermissionDenied)
				return
			}
		}
		teamTenantID = workspaceID
	}
	uploadFile, err := h.fileService.UploadFile(
		c.Request.Context(),
		filename,
		content,
		mimeType,
		accountID,
		organizationID,
		model.CreatedByRoleAccount,
		source,
		teamTenantID,
		isTemporary,
		false,
	)

	if err != nil {
		switch err {
		case file_model.ErrFileTooLarge:
			h.businessError(c, response.ErrFileTooLarge)
		case file_model.ErrUnsupportedFileType:
			h.businessError(c, response.ErrUnsupportedFileType)
		default:
			h.businessError(c, response.ErrFileUploadFailed)
		}
		return
	}

	// If folder_id is specified, add file to folder
	// Do not attach temporary files to organization folders
	if req.FolderID != nil && !isTemporary {
		// Validate UUID format
		if _, err := uuid.Parse(*req.FolderID); err != nil {
			h.businessError(c, response.ErrInvalidParam)
			return
		}

		err := h.fileFolderService.AddFileToFolder(c.Request.Context(), uploadFile.ID, *req.FolderID, accountID)
		if err != nil {
			// Log error but don't fail the upload
			logger.WarnContext(c.Request.Context(), "failed to add uploaded file to folder", "file_id", uploadFile.ID, "folder_id", *req.FolderID, err)
		}
	}

	// Build response
	fileResponse := dto.NewFileUploadResponse(uploadFile)
	response.Success(c, fileResponse)
}

// DownloadFile handles file download requests
// GET /files/:file_id/download
func (h *FileHandler) DownloadFile(c *gin.Context) {
	fileID := c.Param("file_id")
	if fileID == "" {
		h.businessError(c, response.ErrFileIdRequired)
		return
	}

	uploadFile, ok := h.getAuthorizedFileForDownload(c, fileID)
	if !ok {
		return
	}

	// Call service layer to download file
	content, err := h.fileService.DownloadFile(c.Request.Context(), fileID)
	if err != nil {
		if errors.Is(err, model.ErrFileNotFound) {
			response.Fail(c, response.ErrFileNotFound)
		} else {
			h.businessError(c, response.ErrFileDownloadFailed)
		}
		return
	}

	// Set response headers for file download
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Type", uploadFile.MimeType)

	c.Header("Content-Disposition", fileAttachmentDisposition(uploadFile.Name))

	c.Header("Content-Length", strconv.FormatInt(uploadFile.Size, 10))
	c.Header("Expires", "0")
	c.Header("Cache-Control", "must-revalidate")
	c.Header("Pragma", "public")

	// Write file content to response
	c.Data(200, uploadFile.MimeType, content)
}

func (h *FileHandler) getAuthorizedFileForDownload(c *gin.Context, fileID string) (*dto.UploadFile, bool) {
	return authorizeFileDownloadAccess(c, h.fileService, h.enterpriseService, fileID)
}

func getUploadFileWorkspaceID(uploadFile *dto.UploadFile) string {
	if uploadFile.WorkspaceID != nil {
		return *uploadFile.WorkspaceID
	}
	if uploadFile.TeamTenantID != nil {
		return *uploadFile.TeamTenantID
	}
	return ""
}

func fileAttachmentDisposition(filename string) string {
	filename = sanitizeAttachmentFilename(filename)
	if filename == "" {
		filename = "download"
	}
	fallback := asciiAttachmentFilenameFallback(filename)
	encoded := url.PathEscape(filename)
	return `attachment; filename="` + fallback + `"; filename*=UTF-8''` + encoded
}

func sanitizeAttachmentFilename(filename string) string {
	filename = strings.TrimSpace(filename)
	filename = strings.ReplaceAll(filename, "\r", "_")
	filename = strings.ReplaceAll(filename, "\n", "_")
	filename = strings.ReplaceAll(filename, "\\", "/")
	if index := strings.LastIndex(filename, "/"); index >= 0 {
		filename = filename[index+1:]
	}
	return strings.Trim(filename, ". ")
}

func asciiAttachmentFilenameFallback(filename string) string {
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

func isOriginalPreviewSupported(uploadFile *dto.UploadFile) bool {
	mimeType := strings.TrimSpace(strings.ToLower(strings.Split(uploadFile.MimeType, ";")[0]))
	if mimeType != "" && mimeType != "application/octet-stream" {
		return mimeType == "application/pdf" ||
			strings.HasPrefix(mimeType, "image/") ||
			isOfficeOriginalPreviewMIMEType(mimeType) ||
			isTextOriginalPreviewMIMEType(mimeType)
	}

	extension := strings.ToLower(strings.TrimPrefix(uploadFile.Extension, "."))
	if extension == "pdf" || file_model.IsImageExtension(extension) {
		return true
	}
	if isOfficeOriginalPreviewExtension(extension) {
		return true
	}
	if isTextOriginalPreviewExtension(extension) {
		return true
	}
	return false
}

func isTextOriginalPreviewExtension(extension string) bool {
	switch extension {
	case "txt", "md", "markdown", "mdx", "json", "csv", "html", "htm", "xml":
		return true
	default:
		return false
	}
}

func isTextOriginalPreviewMIMEType(mimeType string) bool {
	switch strings.TrimSpace(strings.Split(mimeType, ";")[0]) {
	case "text/plain",
		"text/markdown",
		"text/html",
		"application/json",
		"text/csv",
		"application/csv",
		"text/xml",
		"application/xml":
		return true
	default:
		return false
	}
}

func isOfficeOriginalPreviewExtension(extension string) bool {
	switch extension {
	case "docx", "xlsx":
		return true
	default:
		return false
	}
}

func isOfficeOriginalPreviewMIMEType(mimeType string) bool {
	switch strings.TrimSpace(strings.Split(mimeType, ";")[0]) {
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return true
	default:
		return false
	}
}

// isDatasetEditor checks if user is a dataset editor
// This needs to be implemented based on actual user permission model
func (h *FileHandler) isDatasetEditor(account interface{}) bool {
	// Temporary implementation: assume all users have dataset editing permission
	// In production, this should be determined based on user roles or permissions
	return true
}

// ListFiles handles paginated file listing request - GET /files
// GET /files
func (h *FileHandler) ListFiles(c *gin.Context) {
	// Get canonical organization scope
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		h.businessError(c, response.ErrInvalidTenantId)
		return
	}

	// Parse request parameters
	var req dto.FileListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		h.businessError(c, response.ErrInvalidParam)
		return
	}

	// Set default values
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Limit <= 0 {
		req.Limit = 20
	} else if req.Limit > 100 {
		req.Limit = 100
	}

	accountID := c.GetString("account_id")

	visibleWorkspaceIDs, err := resolveVisibleWorkspaceIDs(
		c.Request.Context(),
		h.enterpriseService,
		organizationID,
		accountID,
		req.WorkspaceID,
		workspace_model.WorkspacePermissionFileView,
		workspace_model.WorkspacePermissionFileManage,
		workspace_model.WorkspacePermissionFileDownload,
	)
	if err != nil {
		h.businessError(c, response.ErrSystemError)
		return
	}
	if len(visibleWorkspaceIDs) == 0 {
		response.Success(c, &dto.FileListResponse{
			Data:    []dto.UploadFile{},
			HasMore: false,
			Limit:   req.Limit,
			Total:   0,
			Page:    req.Page,
		})
		return
	}

	// Call service layer to get file list
	result, err := h.fileService.ListFiles(c.Request.Context(), organizationID, accountID, &req, visibleWorkspaceIDs)
	if err != nil {
		h.businessError(c, response.ErrFileSystemError)
		return
	}

	response.Success(c, result)
}

// ListArchivedFiles handles paginated archived file listing request - GET /files/archived
// GET /files/archived
func (h *FileHandler) ListArchivedFiles(c *gin.Context) {
	// Get canonical organization scope
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		h.businessError(c, response.ErrInvalidTenantId)
		return
	}

	// Parse request parameters
	var req dto.FileListRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		h.businessError(c, response.ErrInvalidParam)
		return
	}

	// Set default values
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Limit <= 0 {
		req.Limit = 20
	} else if req.Limit > 100 {
		req.Limit = 100
	}

	accountID := c.GetString("account_id")

	visibleWorkspaceIDs, err := resolveVisibleWorkspaceIDs(
		c.Request.Context(),
		h.enterpriseService,
		organizationID,
		accountID,
		req.WorkspaceID,
		workspace_model.WorkspacePermissionFileView,
		workspace_model.WorkspacePermissionFileManage,
		workspace_model.WorkspacePermissionFileDownload,
	)
	if err != nil {
		h.businessError(c, response.ErrSystemError)
		return
	}
	if len(visibleWorkspaceIDs) == 0 {
		response.Success(c, &dto.FileListResponse{
			Data:    []dto.UploadFile{},
			HasMore: false,
			Limit:   req.Limit,
			Total:   0,
			Page:    req.Page,
		})
		return
	}

	// Call service layer to get archived file list
	result, err := h.fileService.ListArchivedFiles(c.Request.Context(), organizationID, accountID, &req, visibleWorkspaceIDs)
	if err != nil {
		h.businessError(c, response.ErrFileSystemError)
		return
	}

	response.Success(c, result)
}

// GetStorageUsage gets storage usage for the tenant
// GET /files/storage-usage
func (h *FileHandler) GetStorageUsage(c *gin.Context) {
	// Get canonical organization scope
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		h.businessError(c, response.ErrInvalidTenantId)
		return
	}

	// Get unit parameter, default to GB
	unit := c.Query("unit")
	if unit == "" {
		unit = "GB"
	}

	// Get used storage size
	usedSize, err := h.fileService.GetStorageUsage(c.Request.Context(), organizationID)
	if err != nil {
		h.businessError(c, response.ErrFileSystemError)
		return
	}

	// Get total storage quota from config
	totalSize := config.GlobalConfig.Upload.EnterpriseStorageQuota

	// Convert sizes based on unit
	usedSizeConverted, totalSizeConverted := convertBytesToUnitFloat(usedSize, totalSize, unit)

	// Build response
	response.Success(c, &dto.StorageUsageResponse{
		Used:  usedSizeConverted,
		Total: totalSizeConverted,
		Unit:  unit,
	})
}

// DeleteFiles handles DELETE /files endpoint for deleting files
// DELETE /files
func (h *FileHandler) DeleteFiles(c *gin.Context) {
	// Get canonical organization scope
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		h.businessError(c, response.ErrInvalidTenantId)
		return
	}

	// Get account ID
	accountID := c.GetString("account_id")
	if accountID == "" {
		h.businessError(c, response.ErrUnauthorized)
		return
	}

	// Parse request - file IDs are passed as query parameters
	fileIDs := c.QueryArray("file_ids")
	if len(fileIDs) == 0 {
		h.businessError(c, response.ErrInvalidParam)
		return
	}

	// Validate UUIDs
	for _, fileID := range fileIDs {
		if _, err := uuid.Parse(fileID); err != nil {
			h.businessError(c, response.ErrInvalidParam)
			return
		}
	}

	// Call service to delete files
	err := h.fileService.DeleteFiles(c.Request.Context(), fileIDs)
	if err != nil {
		// Check if it's a usage error (file is used by documents)
		if strings.Contains(err.Error(), "is used by documents") {
			h.businessError(c, response.ErrFileInUse)
			return
		}

		// Check if it's a file not found error
		if strings.Contains(err.Error(), "not found") {
			h.businessError(c, response.ErrFileNotFound)
			return
		}

		logger.Error("Failed to delete files", err)
		h.businessErrorWithMessage(c, response.ErrFileSystemError, "Failed to delete file from storage")
		return
	}

	response.Success(c, gin.H{"result": "success"})
}

// convertBytesToUnitFloat converts bytes to the specified unit with floating point precision
func convertBytesToUnitFloat(usedBytes, totalBytes int64, unit string) (float64, float64) {
	switch unit {
	case "B":
		return float64(usedBytes), float64(totalBytes)
	case "KB":
		return float64(usedBytes) / 1024, float64(totalBytes) / 1024
	case "MB":
		return float64(usedBytes) / (1024 * 1024), float64(totalBytes) / (1024 * 1024)
	case "GB":
		return float64(usedBytes) / (1024 * 1024 * 1024), float64(totalBytes) / (1024 * 1024 * 1024)
	case "TB":
		return float64(usedBytes) / (1024 * 1024 * 1024 * 1024), float64(totalBytes) / (1024 * 1024 * 1024 * 1024)
	default:
		// Default to GB if unknown unit
		return float64(usedBytes) / (1024 * 1024 * 1024), float64(totalBytes) / (1024 * 1024 * 1024)
	}
}
