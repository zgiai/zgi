package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/zgiai/zgi/api/config"
	hyperparseengine "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/hyperparse"
	"github.com/zgiai/zgi/api/internal/contracts"
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
	fileService                      interfaces.FileService
	fileFolderService                service.FileFolderService
	accountService                   interfaces.AccountService
	tenantService                    interfaces.WorkspaceManagementService
	enterpriseService                interfaces.OrganizationService
	assetStateService                datalibraryservice.FileAssetProcessingStateService
	processingService                datalibraryservice.ProcessingRequestService
	parsePreviewService              datalibraryservice.ParsePreviewService
	parseConfirmationService         datalibraryservice.ParseConfirmationService
	parseArtifactConfirmationService datalibraryservice.ParseArtifactConfirmationService
	fileAssetDetailService           datalibraryservice.FileAssetDetailService
	fileAssetChunkService            datalibraryservice.FileAssetChunkService
	fileAssetChunkEditService        datalibraryservice.FileAssetChunkEditService
	fileAssetQAService               datalibraryservice.FileAssetQAService
	taskEnqueuer                     FileProcessingTaskEnqueuer
	validator                        *validator.Validate
}

type FileAssetProcessingServices struct {
	StateService                     datalibraryservice.FileAssetProcessingStateService
	ProcessingService                datalibraryservice.ProcessingRequestService
	ParsePreviewService              datalibraryservice.ParsePreviewService
	ParseConfirmationService         datalibraryservice.ParseConfirmationService
	ParseArtifactConfirmationService datalibraryservice.ParseArtifactConfirmationService
	FileAssetDetailService           datalibraryservice.FileAssetDetailService
	FileAssetChunkService            datalibraryservice.FileAssetChunkService
	FileAssetChunkEditService        datalibraryservice.FileAssetChunkEditService
	FileAssetQAService               datalibraryservice.FileAssetQAService
	TaskEnqueuer                     FileProcessingTaskEnqueuer
}

type FileProcessingTaskEnqueuer interface {
	EnqueueFileProcess(ctx context.Context, processingRequestID uuid.UUID) error
	EnqueueGenerateCurrentResult(ctx context.Context, processingRequestID uuid.UUID) error
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

type fileSourcePreviewPagesResponse struct {
	Engine    string   `json:"engine"`
	PageCount int      `json:"page_count"`
	Pages     []string `json:"pages"`
}

type fileProcessingRequest struct {
	TargetLevel   string `json:"target_level"`
	Mode          string `json:"mode"`
	Force         bool   `json:"force"`
	ParseProvider string `json:"parse_provider"`
}

type parseConfirmationResolveRequest struct {
	Action       string  `json:"action"`
	FinalContent *string `json:"final_content"`
}

type parseConfirmationBatchIgnoreRequest struct {
	ItemIDs []string `json:"item_ids"`
}

type fileChunkListQuery struct {
	Page          int      `form:"page"`
	Limit         int      `form:"limit"`
	Search        string   `form:"search"`
	Status        string   `form:"status"`
	ChunkType     []string `form:"chunk_type"`
	Enabled       *bool    `form:"enabled"`
	ParentChunkID string   `form:"parent_chunk_id"`
	IncludeTree   bool     `form:"include_tree"`
}

type fileChunkUpdateRequest struct {
	Content *string `json:"content"`
	Enabled *bool   `json:"enabled"`
}

type fileQARequest struct {
	Question string `json:"question"`
	TopK     int    `json:"top_k"`
}

type queuedFileProcessingRequest struct {
	Asset             *datalibrarymodel.DocumentAsset
	ProcessingRequest *datalibraryservice.ProcessingRequestView
	ProcessingRunID   *uuid.UUID
	GenerationNo      int64
}

type fileDetailResponse struct {
	File          *dto.UploadFile                              `json:"file"`
	Asset         *datalibrarymodel.DocumentAsset              `json:"asset,omitempty"`
	Processing    *fileDetailProcessingView                    `json:"processing,omitempty"`
	ArtifactState *datalibraryservice.FileAssetArtifactState   `json:"artifact_state,omitempty"`
	Error         *datalibraryservice.FileAssetProcessingError `json:"error,omitempty"`
}

type fileDetailProcessingView struct {
	LatestRequest            *datalibraryservice.ProcessingRequestView `json:"latest_request,omitempty"`
	Summary                  datalibraryservice.FileAssetSummaryView   `json:"summary"`
	PendingConfirmationCount int64                                     `json:"pending_confirmation_count"`
	ChunkCount               int64                                     `json:"chunk_count"`
	EmbeddingCount           int64                                     `json:"embedding_count"`
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
	var parsePreviewService datalibraryservice.ParsePreviewService
	var parseConfirmationService datalibraryservice.ParseConfirmationService
	var parseArtifactConfirmationService datalibraryservice.ParseArtifactConfirmationService
	var fileAssetDetailService datalibraryservice.FileAssetDetailService
	var fileAssetChunkService datalibraryservice.FileAssetChunkService
	var fileAssetChunkEditService datalibraryservice.FileAssetChunkEditService
	var fileAssetQAService datalibraryservice.FileAssetQAService
	var taskEnqueuer FileProcessingTaskEnqueuer
	if len(assetProcessingServices) > 0 {
		assetStateService = assetProcessingServices[0].StateService
		processingService = assetProcessingServices[0].ProcessingService
		parsePreviewService = assetProcessingServices[0].ParsePreviewService
		parseConfirmationService = assetProcessingServices[0].ParseConfirmationService
		parseArtifactConfirmationService = assetProcessingServices[0].ParseArtifactConfirmationService
		fileAssetDetailService = assetProcessingServices[0].FileAssetDetailService
		fileAssetChunkService = assetProcessingServices[0].FileAssetChunkService
		fileAssetChunkEditService = assetProcessingServices[0].FileAssetChunkEditService
		fileAssetQAService = assetProcessingServices[0].FileAssetQAService
		taskEnqueuer = assetProcessingServices[0].TaskEnqueuer
	}
	return &FileHandler{
		fileService:                      fileService,
		fileFolderService:                fileFolderService,
		accountService:                   accountService,
		tenantService:                    tenantService,
		enterpriseService:                enterpriseService,
		assetStateService:                assetStateService,
		processingService:                processingService,
		parsePreviewService:              parsePreviewService,
		parseConfirmationService:         parseConfirmationService,
		parseArtifactConfirmationService: parseArtifactConfirmationService,
		fileAssetDetailService:           fileAssetDetailService,
		fileAssetChunkService:            fileAssetChunkService,
		fileAssetChunkEditService:        fileAssetChunkEditService,
		fileAssetQAService:               fileAssetQAService,
		taskEnqueuer:                     taskEnqueuer,
		validator:                        validator.New(),
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

func normalizeFileParseProvider(raw string) (string, bool) {
	provider := strings.ToLower(strings.TrimSpace(raw))
	if provider == "" {
		return "auto", true
	}
	switch provider {
	case "auto",
		string(contracts.ParseEngineLocal),
		string(contracts.ParseEngineMineru),
		string(contracts.ParseEngineReducto),
		string(contracts.ParseEngineVLM),
		"hyperparse_api":
		return provider, true
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

func validateFileReplacementState(asset *datalibrarymodel.DocumentAsset) error {
	if asset == nil {
		return datalibraryservice.ErrDocumentAssetNotFound
	}
	switch asset.ProductStatus {
	case datalibrarymodel.DocumentAssetProductStatusStoredOnly,
		datalibrarymodel.DocumentAssetProductStatusReady,
		datalibrarymodel.DocumentAssetProductStatusParseFailed,
		datalibrarymodel.DocumentAssetProductStatusConfirming:
		return nil
	case datalibrarymodel.DocumentAssetProductStatusParsing,
		datalibrarymodel.DocumentAssetProductStatusGenerating:
		return errFileProcessingRequestAlreadyActive
	default:
		return errFileProcessingRequestStateInvalid
	}
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
	parseProvider, ok := normalizeFileParseProvider(c.PostForm("parse_provider"))
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

	asset, err := h.attachAssetProcessing(c.Request.Context(), uploadFile, organizationID, accountID, processingMode, parseProvider, shouldUseAssetProcessing)
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

// ReplaceDocument replaces an existing document file in-place and optionally starts parsing.
// POST /files/:file_id/replacement
func (h *FileHandler) ReplaceDocument(c *gin.Context) {
	if h.assetStateService == nil || h.fileAssetDetailService == nil || h.processingService == nil {
		h.businessErrorWithMessage(c, response.ErrSystemError, "file asset processing service is not available")
		return
	}

	accountID := c.GetString("account_id")
	if accountID == "" {
		h.businessError(c, response.ErrUnauthorized)
		return
	}
	organizationID, currentFile, ok := h.authorizeManageDocumentFile(c)
	if !ok {
		return
	}
	currentDetail, err := h.fileAssetDetailService.GetCurrentFileAssetDetail(c.Request.Context(), datalibraryservice.FileAssetDetailInput{
		OrganizationID: organizationID,
		SourceFileID:   currentFile.ID,
	})
	if err != nil {
		h.handleFileAssetDetailError(c, err)
		return
	}
	if err := validateFileReplacementState(currentDetail.Asset); err != nil {
		h.handleFileProcessingRequestError(c, err)
		return
	}

	processingMode, ok := normalizeUploadProcessingMode(c.PostForm("processing_mode"))
	if !ok {
		h.businessError(c, response.ErrInvalidParam)
		return
	}
	parseProvider, ok := normalizeFileParseProvider(c.PostForm("parse_provider"))
	if !ok {
		h.businessError(c, response.ErrInvalidParam)
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		h.businessError(c, response.ErrNoFileUploaded)
		return
	}
	defer file.Close()
	if err := c.Request.ParseMultipartForm(32 << 20); err == nil {
		if len(c.Request.MultipartForm.File) > 1 {
			h.businessError(c, response.ErrTooManyFiles)
			return
		}
	}
	if header.Filename == "" {
		h.businessError(c, response.ErrFilenameRequired)
		return
	}
	fileExtension := strings.TrimPrefix(strings.ToLower(filepath.Ext(header.Filename)), ".")
	if !model.IsDocumentExtension(fileExtension) {
		h.businessError(c, response.ErrUnsupportedFileType)
		return
	}
	content, err := io.ReadAll(file)
	if err != nil {
		h.businessError(c, response.ErrFileReadFailed)
		return
	}
	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	replacedFile, err := h.fileService.ReplaceFileContent(c.Request.Context(), currentFile.ID, header.Filename, content, mimeType, accountID, organizationID)
	if err != nil {
		switch err {
		case file_model.ErrFileTooLarge:
			h.businessError(c, response.ErrFileTooLarge)
		case file_model.ErrUnsupportedFileType:
			h.businessError(c, response.ErrUnsupportedFileType)
		default:
			errMsg := err.Error()
			if strings.Contains(errMsg, "storage quota exceeded") || strings.Contains(errMsg, "storage space quota insufficient") {
				h.businessErrorWithMessage(c, response.ErrQuotaStorageExceeded, errMsg)
			} else if strings.Contains(errMsg, "quota") {
				h.businessErrorWithMessage(c, response.ErrQuotaExceeded, errMsg)
			} else {
				h.businessErrorWithMessage(c, response.ErrorCode{Code: 210002, Message: "Failed to replace file", UserVisible: true}, errMsg)
			}
		}
		return
	}

	asset, err := h.assetStateService.PrepareFileReplacement(c.Request.Context(), datalibraryservice.FileReplacementInput{
		OrganizationID: organizationID,
		AssetID:        currentDetail.Asset.ID,
		Title:          replacedFile.Name,
		ContentHash:    replacedFile.Hash,
		RequestedBy:    accountID,
		InvalidateRefs: true,
	})
	if err != nil {
		h.handleFileProcessingRequestError(c, err)
		return
	}

	var queued *queuedFileProcessingRequest
	if processingMode == UploadProcessingModeProcessNow {
		queued, err = h.beginAndQueueRunProcessingRequest(
			c.Request.Context(),
			asset,
			replacedFile.ID,
			organizationID,
			accountID,
			datalibrarymodel.DocumentProcessingLevelVectorize,
			FileProcessingRequestModeReparse,
			false,
			parseProvider,
		)
		if err != nil {
			h.handleFileProcessingRequestError(c, err)
			return
		}
		asset = queued.Asset
	}

	fileResponse := dto.NewFileUploadResponse(replacedFile)
	fileResponse.ProcessingMode = processingMode
	fileResponse.AssetID = asset.ID.String()
	fileResponse.ProcessingStatus = asset.ProductStatus
	fileResponse.GenerationNo = asset.GenerationNo
	if asset.ActiveProcessingRequestID != nil {
		fileResponse.ProcessingRequestID = asset.ActiveProcessingRequestID.String()
	}
	if asset.ProcessingRunID != nil {
		fileResponse.ProcessingRunID = asset.ProcessingRunID.String()
	}

	body := gin.H{
		"file":            fileResponse,
		"asset":           asset,
		"processing_mode": processingMode,
	}
	if queued != nil {
		body["processing_request"] = queued.ProcessingRequest
		body["processing_run_id"] = uuidPointerString(queued.ProcessingRunID)
		body["generation_no"] = queued.GenerationNo
	}
	response.Success(c, body)
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
	organizationID, uploadFile, ok := h.authorizeManageDocumentFile(c)
	if !ok {
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
	parseProvider, ok := normalizeFileParseProvider(req.ParseProvider)
	if !ok {
		h.businessError(c, response.ErrInvalidParams)
		return
	}

	result, err := h.createQueuedFileProcessingRequest(c.Request.Context(), uploadFile, organizationID, accountID, targetLevel, mode, req.Force, parseProvider)
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

// GetFileDetail returns file metadata with the current file asset processing state.
// GET /files/:file_id/detail
func (h *FileHandler) GetFileDetail(c *gin.Context) {
	organizationID, uploadFile, ok := h.authorizeDocumentFile(c)
	if !ok {
		return
	}
	if h.fileAssetDetailService == nil {
		response.Success(c, fileDetailResponse{File: uploadFile})
		return
	}
	detail, err := h.fileAssetDetailService.GetCurrentFileAssetDetail(c.Request.Context(), datalibraryservice.FileAssetDetailInput{
		OrganizationID: organizationID,
		SourceFileID:   uploadFile.ID,
	})
	if err != nil {
		h.handleFileAssetDetailError(c, err)
		return
	}
	response.Success(c, fileDetailResponse{
		File:          uploadFile,
		Asset:         detail.Asset,
		ArtifactState: &detail.ArtifactState,
		Error:         detail.Error,
		Processing: &fileDetailProcessingView{
			LatestRequest:            detail.LatestProcessing,
			Summary:                  buildFileDetailAssetSummary(detail),
			PendingConfirmationCount: detail.PendingConfirmationCount,
			ChunkCount:               detail.ChunkCount,
			EmbeddingCount:           detail.EmbeddingCount,
		},
	})
}

func buildFileDetailAssetSummary(detail *datalibraryservice.FileAssetDetailView) datalibraryservice.FileAssetSummaryView {
	if detail == nil || detail.Asset == nil {
		return datalibraryservice.FileAssetSummaryView{}
	}
	asset := detail.Asset
	summary := datalibraryservice.FileAssetSummaryView{
		AssetID:                   asset.ID,
		SourceFileID:              asset.SourceFileID,
		ProductStatus:             asset.ProductStatus,
		ProcessingProgress:        asset.ProcessingProgress,
		ActiveProcessingRequestID: asset.ActiveProcessingRequestID,
		ProcessingRunID:           asset.ProcessingRunID,
		GenerationNo:              asset.GenerationNo,
		PendingConfirmationCount:  detail.PendingConfirmationCount,
		ChunkCount:                detail.ChunkCount,
		EmbeddingCount:            detail.EmbeddingCount,
		EmbeddingDimension:        asset.EmbeddingDimension,
		VectorStatus:              asset.VectorStatus,
	}
	if asset.EmbeddingProvider != nil {
		summary.EmbeddingProvider = *asset.EmbeddingProvider
	}
	if asset.EmbeddingModel != nil {
		summary.EmbeddingModel = *asset.EmbeddingModel
	}
	if asset.ProcessingStage != nil {
		summary.ProcessingStage = *asset.ProcessingStage
	}
	if asset.LastErrorCode != nil {
		summary.LastErrorCode = *asset.LastErrorCode
	}
	if asset.LastErrorMessage != nil {
		summary.LastErrorMessage = *asset.LastErrorMessage
	}
	return summary
}

func (h *FileHandler) handleFileAssetDetailError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, datalibraryservice.ErrDocumentAssetNotFound):
		h.businessError(c, response.ErrNotFound)
	case errors.Is(err, datalibraryservice.ErrOrganizationIDRequired),
		errors.Is(err, datalibraryservice.ErrSourceFileIDRequired):
		h.businessError(c, response.ErrInvalidParams)
	default:
		logger.WarnContext(c.Request.Context(), "failed to get file asset detail", err)
		h.businessError(c, response.ErrSystemError)
	}
}

// ListFileChunks returns current generation chunks for a file asset.
// GET /files/:file_id/chunks
func (h *FileHandler) ListFileChunks(c *gin.Context) {
	if h.fileAssetChunkService == nil {
		h.businessErrorWithMessage(c, response.ErrSystemError, "file asset chunk service is not available")
		return
	}
	organizationID, uploadFile, ok := h.authorizeDocumentFile(c)
	if !ok {
		return
	}
	var query fileChunkListQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		h.businessError(c, response.ErrInvalidParams)
		return
	}
	limit := query.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	page := query.Page
	if page <= 0 {
		page = 1
	}
	parentChunkID, err := parseOptionalChunkParentID(query.ParentChunkID)
	if err != nil {
		h.businessError(c, response.ErrInvalidParams)
		return
	}
	result, err := h.fileAssetChunkService.ListCurrentFileChunks(c.Request.Context(), datalibraryservice.FileAssetChunkListInput{
		OrganizationID: organizationID,
		SourceFileID:   uploadFile.ID,
		Search:         query.Search,
		Status:         query.Status,
		ChunkTypes:     query.ChunkType,
		Enabled:        query.Enabled,
		ParentChunkID:  parentChunkID,
		IncludeTree:    query.IncludeTree,
		Limit:          limit,
		Offset:         (page - 1) * limit,
	})
	if err != nil {
		h.handleFileAssetChunkError(c, err)
		return
	}
	response.Success(c, gin.H{
		"asset":                 result.Asset,
		"items":                 result.Items,
		"tree":                  result.Tree,
		"total":                 result.Total,
		"primary_chunk_count":   result.PrimaryChunkCount,
		"secondary_chunk_count": result.SecondaryChunkCount,
		"embedding_count":       result.EmbeddingCount,
		"limit":                 result.Limit,
		"page":                  page,
		"has_more":              int64(page*limit) < result.Total,
		"generation_no":         result.GenerationNo,
	})
}

func (h *FileHandler) handleFileAssetChunkError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, datalibraryservice.ErrDocumentAssetNotFound):
		h.businessError(c, response.ErrNotFound)
	case errors.Is(err, datalibraryservice.ErrOrganizationIDRequired),
		errors.Is(err, datalibraryservice.ErrSourceFileIDRequired):
		h.businessError(c, response.ErrInvalidParams)
	default:
		logger.WarnContext(c.Request.Context(), "failed to list file asset chunks", err)
		h.businessError(c, response.ErrSystemError)
	}
}

// AskFileQuestion answers one question using the current file's chunk index.
// POST /files/:file_id/qa
func (h *FileHandler) AskFileQuestion(c *gin.Context) {
	if h.fileAssetQAService == nil {
		h.businessErrorWithMessage(c, response.ErrSystemError, "file asset qa service is not available")
		return
	}
	organizationID, uploadFile, ok := h.authorizeDocumentFile(c)
	if !ok {
		return
	}
	var req fileQARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.businessError(c, response.ErrInvalidParams)
		return
	}
	result, err := h.fileAssetQAService.AskCurrentFile(c.Request.Context(), datalibraryservice.FileAssetQAInput{
		OrganizationID: organizationID,
		SourceFileID:   uploadFile.ID,
		Question:       req.Question,
		TopK:           req.TopK,
		AccountID:      c.GetString("account_id"),
	})
	if err != nil {
		h.handleFileAssetQAError(c, err)
		return
	}
	response.Success(c, result)
}

// StreamFileQuestion answers one question using SSE.
// POST /files/:file_id/qa/stream
func (h *FileHandler) StreamFileQuestion(c *gin.Context) {
	if h.fileAssetQAService == nil {
		h.businessErrorWithMessage(c, response.ErrSystemError, "file asset qa service is not available")
		return
	}
	organizationID, uploadFile, ok := h.authorizeDocumentFile(c)
	if !ok {
		return
	}
	var req fileQARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.businessError(c, response.ErrInvalidParams)
		return
	}
	events, err := h.fileAssetQAService.StreamCurrentFile(c.Request.Context(), datalibraryservice.FileAssetQAInput{
		OrganizationID: organizationID,
		SourceFileID:   uploadFile.ID,
		Question:       req.Question,
		TopK:           req.TopK,
		AccountID:      c.GetString("account_id"),
	})
	if err != nil {
		h.handleFileAssetQAError(c, err)
		return
	}

	writer := c.Writer
	header := writer.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	header.Set("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	for event := range events {
		if err := writeFileQASSEvent(writer, event.Type, event); err != nil {
			logger.WarnContext(c.Request.Context(), "failed to write file qa stream event", err)
			return
		}
		if flusher, ok := writer.(http.Flusher); ok {
			flusher.Flush()
		}
		if event.Type == "done" || event.Type == "error" {
			return
		}
	}
}

func writeFileQASSEvent(w io.Writer, eventName string, payload datalibraryservice.FileAssetQAStreamEvent) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if eventName == "" {
		eventName = "message"
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventName, data)
	return err
}

func (h *FileHandler) handleFileAssetQAError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, datalibraryservice.ErrDocumentAssetNotFound):
		h.businessError(c, response.ErrNotFound)
	case errors.Is(err, datalibraryservice.ErrFileAssetQAQuestionRequired),
		errors.Is(err, datalibraryservice.ErrOrganizationIDRequired),
		errors.Is(err, datalibraryservice.ErrSourceFileIDRequired):
		h.businessErrorWithMessage(c, response.ErrInvalidParams, err.Error())
	case errors.Is(err, datalibraryservice.ErrFileAssetQAIndexNotReady):
		h.businessErrorWithMessage(c, response.ErrInvalidParams, "文档索引尚未完成")
	default:
		logger.WarnContext(c.Request.Context(), "failed to answer file question", err)
		h.businessError(c, response.ErrSystemError)
	}
}

// UpdateFileChunk edits or enables/disables one current generation leaf chunk.
// PATCH /files/:file_id/chunks/:chunk_id
func (h *FileHandler) UpdateFileChunk(c *gin.Context) {
	if h.fileAssetChunkEditService == nil {
		h.businessErrorWithMessage(c, response.ErrSystemError, "file asset chunk edit service is not available")
		return
	}
	accountID := c.GetString("account_id")
	if accountID == "" {
		h.businessError(c, response.ErrUnauthorized)
		return
	}
	organizationID, uploadFile, ok := h.authorizeManageDocumentFile(c)
	if !ok {
		return
	}
	chunkID, err := uuid.Parse(c.Param("chunk_id"))
	if err != nil || chunkID == uuid.Nil {
		h.businessError(c, response.ErrInvalidParams)
		return
	}
	var req fileChunkUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.businessError(c, response.ErrInvalidParams)
		return
	}
	if req.Content == nil && req.Enabled == nil {
		h.businessError(c, response.ErrInvalidParams)
		return
	}
	result, err := h.fileAssetChunkEditService.UpdateCurrentFileChunk(c.Request.Context(), datalibraryservice.FileAssetChunkEditInput{
		OrganizationID: organizationID,
		SourceFileID:   uploadFile.ID,
		ChunkID:        chunkID,
		Content:        req.Content,
		Enabled:        req.Enabled,
		UpdatedBy:      accountID,
	})
	if err != nil {
		h.handleFileAssetChunkEditError(c, err)
		return
	}
	response.Success(c, result)
}

func (h *FileHandler) handleFileAssetChunkEditError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, datalibraryservice.ErrDocumentAssetNotFound):
		h.businessError(c, response.ErrNotFound)
	case errors.Is(err, datalibraryservice.ErrOrganizationIDRequired),
		errors.Is(err, datalibraryservice.ErrSourceFileIDRequired),
		errors.Is(err, datalibraryservice.ErrAssetIDRequired),
		errors.Is(err, datalibraryservice.ErrProcessingRunMismatch),
		errors.Is(err, datalibraryservice.ErrFileChunkEditNotAllowed),
		errors.Is(err, datalibraryservice.ErrDocumentChunkEmbeddingsRequired):
		h.businessErrorWithMessage(c, response.ErrInvalidParams, err.Error())
	default:
		logger.WarnContext(c.Request.Context(), "failed to update file asset chunk", err)
		h.businessError(c, response.ErrSystemError)
	}
}

func parseOptionalChunkParentID(raw string) (*uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("parent_chunk_id is nil")
	}
	return &id, nil
}

// GetFileParsePreview returns the current parsed elements with confirmation overlay.
// GET /files/:file_id/parse-preview
func (h *FileHandler) GetFileParsePreview(c *gin.Context) {
	if h.parsePreviewService == nil {
		h.businessErrorWithMessage(c, response.ErrSystemError, "file parse preview service is not available")
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

	preview, err := h.parsePreviewService.GetParsePreview(c.Request.Context(), datalibraryservice.ParsePreviewInput{
		OrganizationID: organizationID,
		SourceFileID:   uploadFile.ID,
	})
	if err != nil {
		h.handleFileParsePreviewError(c, err)
		return
	}
	response.Success(c, preview)
}

func (h *FileHandler) handleFileParsePreviewError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, datalibraryservice.ErrDocumentAssetNotFound),
		errors.Is(err, datalibraryservice.ErrParsePreviewNotReady):
		h.businessError(c, response.ErrNotFound)
	case errors.Is(err, datalibraryservice.ErrOrganizationIDRequired),
		errors.Is(err, datalibraryservice.ErrSourceFileIDRequired):
		h.businessError(c, response.ErrInvalidParams)
	default:
		logger.WarnContext(c.Request.Context(), "failed to get file parse preview", err)
		h.businessError(c, response.ErrSystemError)
	}
}

// ListParseConfirmationItems returns current confirmation items for a parsed file.
// GET /files/:file_id/parse-confirmation-items
func (h *FileHandler) ListParseConfirmationItems(c *gin.Context) {
	if h.parseConfirmationService == nil {
		h.businessErrorWithMessage(c, response.ErrSystemError, "file parse confirmation service is not available")
		return
	}
	organizationID, uploadFile, ok := h.authorizeDocumentFile(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	result, err := h.parseConfirmationService.ListCurrentConfirmationItems(c.Request.Context(), datalibraryservice.ParseConfirmationListInput{
		OrganizationID: organizationID,
		SourceFileID:   uploadFile.ID,
		Status:         c.Query("status"),
		Limit:          limit,
		Offset:         offset,
	})
	if err != nil {
		h.handleFileParseConfirmationError(c, err)
		return
	}
	response.Success(c, result)
}

// ResolveParseConfirmationItem applies keep/edit/ignore to one pending confirmation item.
// POST /files/:file_id/parse-confirmation-items/:item_id/resolve
func (h *FileHandler) ResolveParseConfirmationItem(c *gin.Context) {
	if h.parseConfirmationService == nil {
		h.businessErrorWithMessage(c, response.ErrSystemError, "file parse confirmation service is not available")
		return
	}
	accountID := c.GetString("account_id")
	if accountID == "" {
		h.businessError(c, response.ErrUnauthorized)
		return
	}
	organizationID, uploadFile, ok := h.authorizeManageDocumentFile(c)
	if !ok {
		return
	}
	itemID, err := uuid.Parse(c.Param("item_id"))
	if err != nil || itemID == uuid.Nil {
		h.businessError(c, response.ErrInvalidParams)
		return
	}
	var req parseConfirmationResolveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.businessError(c, response.ErrInvalidParams)
		return
	}
	result, err := h.parseConfirmationService.ResolveCurrentConfirmationItem(c.Request.Context(), datalibraryservice.ParseConfirmationResolveInput{
		OrganizationID: organizationID,
		SourceFileID:   uploadFile.ID,
		ItemID:         itemID,
		Action:         strings.TrimSpace(req.Action),
		FinalContent:   req.FinalContent,
		UpdatedBy:      accountID,
	})
	if err != nil {
		h.handleFileParseConfirmationError(c, err)
		return
	}
	generationRequest, err := h.queueGenerateAfterConfirmationIfNeeded(c.Request.Context(), result.Asset, uploadFile.ID, organizationID, accountID, result.ShouldGenerate)
	if err != nil {
		h.handleFileParseConfirmationError(c, err)
		return
	}
	response.Success(c, gin.H{
		"item":               result.Item,
		"pending_count":      result.PendingCount,
		"should_generate":    result.ShouldGenerate,
		"generation_request": generationRequest,
	})
}

// BatchIgnoreParseConfirmationItems ignores selected pending confirmation items, or all pending items when item_ids is empty.
// POST /files/:file_id/parse-confirmation-items/batch-ignore
func (h *FileHandler) BatchIgnoreParseConfirmationItems(c *gin.Context) {
	if h.parseConfirmationService == nil {
		h.businessErrorWithMessage(c, response.ErrSystemError, "file parse confirmation service is not available")
		return
	}
	accountID := c.GetString("account_id")
	if accountID == "" {
		h.businessError(c, response.ErrUnauthorized)
		return
	}
	organizationID, uploadFile, ok := h.authorizeManageDocumentFile(c)
	if !ok {
		return
	}
	var req parseConfirmationBatchIgnoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.businessError(c, response.ErrInvalidParams)
		return
	}
	itemIDs, err := parseUUIDList(req.ItemIDs)
	if err != nil {
		h.businessError(c, response.ErrInvalidParams)
		return
	}
	result, err := h.parseConfirmationService.BatchIgnoreCurrentConfirmationItems(c.Request.Context(), datalibraryservice.ParseConfirmationBatchIgnoreInput{
		OrganizationID: organizationID,
		SourceFileID:   uploadFile.ID,
		ItemIDs:        itemIDs,
		UpdatedBy:      accountID,
	})
	if err != nil {
		h.handleFileParseConfirmationError(c, err)
		return
	}
	generationRequest, err := h.queueGenerateAfterConfirmationIfNeeded(c.Request.Context(), result.Asset, uploadFile.ID, organizationID, accountID, result.ShouldGenerate)
	if err != nil {
		h.handleFileParseConfirmationError(c, err)
		return
	}
	response.Success(c, gin.H{
		"items":              result.Items,
		"resolved_count":     len(result.Items),
		"pending_count":      result.PendingCount,
		"should_generate":    result.ShouldGenerate,
		"generation_request": generationRequest,
	})
}

func (h *FileHandler) handleFileParseConfirmationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, datalibraryservice.ErrDocumentAssetNotFound),
		errors.Is(err, datalibraryservice.ErrParseConfirmationItemNotFound),
		errors.Is(err, datalibraryservice.ErrParseConfirmationPatchTargetNotFound):
		h.businessError(c, response.ErrNotFound)
	case errors.Is(err, datalibraryservice.ErrOrganizationIDRequired),
		errors.Is(err, datalibraryservice.ErrSourceFileIDRequired),
		errors.Is(err, datalibraryservice.ErrProcessingRunMismatch),
		errors.Is(err, datalibraryservice.ErrParseConfirmationStateInvalid),
		errors.Is(err, datalibraryservice.ErrParseConfirmationActionInvalid),
		errors.Is(err, datalibraryservice.ErrParseConfirmationFinalContentRequired):
		h.businessErrorWithMessage(c, response.ErrInvalidParams, err.Error())
	default:
		logger.WarnContext(c.Request.Context(), "failed to update file parse confirmation", err)
		h.businessError(c, response.ErrSystemError)
	}
}

func (h *FileHandler) queueGenerateAfterConfirmationIfNeeded(ctx context.Context, asset *datalibrarymodel.DocumentAsset, uploadFileID string, organizationID string, accountID string, shouldGenerate bool) (*queuedFileProcessingRequest, error) {
	if !shouldGenerate {
		return nil, nil
	}
	if h.parseArtifactConfirmationService != nil {
		applied, err := h.parseArtifactConfirmationService.ApplyResolvedConfirmations(ctx, datalibraryservice.ApplyResolvedConfirmationsInput{
			OrganizationID: organizationID,
			SourceFileID:   uploadFileID,
			UpdatedBy:      accountID,
		})
		if err != nil {
			return nil, err
		}
		if applied != nil && applied.Asset != nil {
			asset = applied.Asset
		}
	}
	queued, err := h.queueGenerateAfterConfirmRequest(ctx, asset, uploadFileID, organizationID, accountID, datalibrarymodel.DocumentProcessingLevelVectorize, false)
	if err != nil {
		return nil, err
	}
	if h.assetStateService != nil && asset != nil && asset.ProcessingRunID != nil {
		generating, err := h.assetStateService.MarkGenerating(ctx, datalibraryservice.RunStateInput{
			OrganizationID:     organizationID,
			AssetID:            asset.ID,
			ProcessingRunID:    *asset.ProcessingRunID,
			GenerationNo:       asset.GenerationNo,
			ProcessingProgress: 50,
			ParseArtifactID:    asset.ParseArtifactID,
		})
		if err != nil {
			return nil, err
		}
		queued.Asset = generating
	}
	return queued, nil
}

func (h *FileHandler) authorizeDocumentFile(c *gin.Context) (string, *dto.UploadFile, bool) {
	return h.authorizeDocumentFileWith(c, h.getAuthorizedFileForDownload)
}

func (h *FileHandler) authorizeManageDocumentFile(c *gin.Context) (string, *dto.UploadFile, bool) {
	return h.authorizeDocumentFileWith(c, h.getAuthorizedFileForManage)
}

func (h *FileHandler) authorizeDocumentFileWith(c *gin.Context, authorize func(*gin.Context, string) (*dto.UploadFile, bool)) (string, *dto.UploadFile, bool) {
	organizationID := util.GetOrganizationID(c)
	if organizationID == "" {
		h.businessError(c, response.ErrInvalidTenantId)
		return "", nil, false
	}
	fileID := c.Param("file_id")
	if fileID == "" {
		h.businessError(c, response.ErrFileIdRequired)
		return "", nil, false
	}
	uploadFile, ok := authorize(c, fileID)
	if !ok {
		return "", nil, false
	}
	if uploadFile.IsTemporary || !model.IsDocumentExtension(strings.TrimPrefix(strings.ToLower(uploadFile.Extension), ".")) {
		h.businessError(c, response.ErrUnsupportedFileType)
		return "", nil, false
	}
	return organizationID, uploadFile, true
}

func parseUUIDList(raw []string) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(raw))
	for _, value := range raw {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		id, err := uuid.Parse(value)
		if err != nil || id == uuid.Nil {
			if err != nil {
				return nil, err
			}
			return nil, errors.New("uuid is nil")
		}
		ids = append(ids, id)
	}
	return ids, nil
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

func (h *FileHandler) attachAssetProcessing(ctx context.Context, uploadFile *dto.UploadFile, organizationID string, accountID string, processingMode string, parseProvider string, useAssetProcessing bool) (*datalibrarymodel.DocumentAsset, error) {
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

	result, err := h.beginAndQueueRunProcessingRequest(ctx, asset, uploadFile.ID, organizationID, accountID, datalibrarymodel.DocumentProcessingLevelVectorize, FileProcessingRequestModeParseNow, false, parseProvider)
	if err != nil {
		return nil, err
	}
	return result.Asset, nil
}

func (h *FileHandler) createQueuedFileProcessingRequest(ctx context.Context, uploadFile *dto.UploadFile, organizationID string, accountID string, targetLevel string, mode string, force bool, parseProvider string) (*queuedFileProcessingRequest, error) {
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
		return h.beginAndQueueRunProcessingRequest(ctx, asset, uploadFile.ID, organizationID, accountID, targetLevel, mode, force, parseProvider)
	case FileProcessingRequestModeGenerateAfterConfirm:
		return h.queueGenerateAfterConfirmRequest(ctx, asset, uploadFile.ID, organizationID, accountID, targetLevel, force)
	default:
		return nil, errInvalidFileProcessingRequestMode
	}
}

func (h *FileHandler) beginAndQueueRunProcessingRequest(ctx context.Context, asset *datalibrarymodel.DocumentAsset, uploadFileID string, organizationID string, accountID string, targetLevel string, mode string, force bool, parseProvider string) (*queuedFileProcessingRequest, error) {
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
			"parse_provider": parseProvider,
		},
	})
	if err != nil {
		return nil, err
	}
	if h.taskEnqueuer != nil {
		if err := h.taskEnqueuer.EnqueueFileProcess(ctx, result.ProcessingRequest.ID); err != nil {
			return nil, err
		}
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
	if h.taskEnqueuer != nil {
		if err := h.taskEnqueuer.EnqueueGenerateCurrentResult(ctx, planned.ID); err != nil {
			return nil, err
		}
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

// GetFileSourcePreviewPages renders source pages for parse-review overlays.
// GET /files/:file_id/source-preview
func (h *FileHandler) GetFileSourcePreviewPages(c *gin.Context) {
	fileID := c.Param("file_id")
	if fileID == "" {
		h.businessError(c, response.ErrFileIdRequired)
		return
	}

	uploadFile, ok := h.getAuthorizedFileForDownload(c, fileID)
	if !ok {
		return
	}

	content, err := h.fileService.DownloadFile(c.Request.Context(), fileID)
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

	maxPages := parseFileSourcePreviewMaxPages(c.Query("max_pages"), content)
	mimeType := strings.TrimSpace(uploadFile.MimeType)
	if mimeType == "" {
		mimeType = http.DetectContentType(content)
	}

	if isFileSourcePreviewPDF(uploadFile.Name, uploadFile.Extension, mimeType) {
		pages, engine, err := hyperparseengine.RenderPDFPreviewPagesToDataURLs(content, maxPages)
		if err != nil {
			logger.WarnContext(c.Request.Context(), "failed to render file source preview pdf", err)
			h.businessError(c, response.ErrFilePreviewFailed)
			return
		}
		if len(pages) == 0 {
			h.businessError(c, response.ErrFilePreviewFailed)
			return
		}
		response.Success(c, fileSourcePreviewPagesResponse{
			Engine:    engine,
			PageCount: len(pages),
			Pages:     pages,
		})
		return
	}

	if strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		response.Success(c, fileSourcePreviewPagesResponse{
			Engine:    "stored_source_image",
			PageCount: 1,
			Pages:     []string{fileSourcePreviewDataURL(mimeType, content)},
		})
		return
	}

	h.businessError(c, response.ErrUnsupportedFileType)
}

func parseFileSourcePreviewMaxPages(raw string, content []byte) int {
	maxPages := 0
	if raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			maxPages = parsed
		}
	}
	if maxPages <= 0 {
		maxPages = hyperparseengine.PDFPageCountRelaxed(content)
	}
	if maxPages <= 0 {
		maxPages = 20
	}
	if maxPages > 50 {
		maxPages = 50
	}
	return maxPages
}

func isFileSourcePreviewPDF(fileName string, extension string, mimeType string) bool {
	normalizedMIME := strings.ToLower(strings.TrimSpace(mimeType))
	if normalizedMIME == "application/pdf" || strings.Contains(normalizedMIME, "/pdf") {
		return true
	}
	normalizedExt := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(extension)), ".")
	if normalizedExt == "pdf" {
		return true
	}
	return strings.HasSuffix(strings.ToLower(strings.TrimSpace(fileName)), ".pdf")
}

func fileSourcePreviewDataURL(mimeType string, content []byte) string {
	if strings.TrimSpace(mimeType) == "" {
		mimeType = http.DetectContentType(content)
	}
	return fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(content))
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

func (h *FileHandler) getAuthorizedFileForManage(c *gin.Context, fileID string) (*dto.UploadFile, bool) {
	return authorizeFileManageAccess(c, h.fileService, h.enterpriseService, fileID)
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
	case "docx", "xlsx", "xls":
		return true
	default:
		return false
	}
}

func isOfficeOriginalPreviewMIMEType(mimeType string) bool {
	switch strings.TrimSpace(strings.Split(mimeType, ";")[0]) {
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/vnd.ms-excel":
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
