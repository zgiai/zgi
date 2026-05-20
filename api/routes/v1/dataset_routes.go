package v1

import (
	"context"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/zgiai/zgi/api/config"
	contentparseRepo "github.com/zgiai/zgi/api/internal/modules/contentparse/repository"
	contentparseService "github.com/zgiai/zgi/api/internal/modules/contentparse/service"
	graphflow_model "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	graphflow_repo "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/repository"
	datasetHandler "github.com/zgiai/zgi/api/internal/modules/dataset/handler"
	datasetIndexing "github.com/zgiai/zgi/api/internal/modules/dataset/indexing"
	dataset_model "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	datasetRepo "github.com/zgiai/zgi/api/internal/modules/dataset/repository"
	datasetService "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	"github.com/zgiai/zgi/api/internal/modules/dataset/task"

	fileProcessRepo "github.com/zgiai/zgi/api/internal/modules/file_process/repository"
	fileProcess "github.com/zgiai/zgi/api/internal/modules/file_process/service"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/vectordb"

	"github.com/zgiai/zgi/api/pkg/storage"

	// Security and Redis
	"github.com/zgiai/zgi/api/internal/container"
	"github.com/zgiai/zgi/api/internal/util"
	redisPkg "github.com/zgiai/zgi/api/pkg/redis"
	sec "github.com/zgiai/zgi/api/pkg/security"

	// Retrieval
	"github.com/zgiai/zgi/api/internal/modules/dataset/retrieval"

	// Middleware
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/response"
)

func RegisterDatasetRoutes(router *gin.RouterGroup, container *container.ServiceContainer) {
	tenantService := container.GetTenantServiceAdapter()
	accountService := container.GetAccountServiceAdapter()
	enterpriseService := container.GetOrganizationService()
	billingService := container.GetBillingService()

	datasetRepoObj := datasetRepo.NewDatasetRepository(container.GetDB())
	documentRepoObj := datasetRepo.NewDocumentRepository(container.GetDB())
	chunkRepoObj := datasetRepo.NewChunkRepository(container.GetDB())
	datasetQueryRepoObj := datasetRepo.NewDatasetQueryRepository(container.GetDB())

	fileRepo := fileProcessRepo.NewFileRepository(container.GetDB())
	storageInstance := storage.GetStorage()
	// Get quota and enterprise services from container
	quotaSvc := container.GetQuotaService()
	enterpriseSvc := container.GetOrganizationService()
	fileServiceObj := fileProcess.NewFileServiceWithVision(fileRepo, storageInstance, container.GetDB(), quotaSvc, enterpriseSvc, container.GetLLMClient(), container.GetDefaultModelService())

	vectorClient := vectordb.NewWeaviateClient(&config.GlobalConfig.VectorStore)
	redisClient := redisPkg.GetClient()
	var encrypter *sec.Encrypter
	if redisClient != nil {
		if enc, err := sec.NewEncrypter(redisClient); err == nil {
			encrypter = enc
		}
	}

	// Get LLM client from container for dataset services
	llmClient := container.GetLLMClient()
	defaultModelService := container.GetDefaultModelService()

	indexingServiceObj := datasetService.NewDocumentIndexingService(
		documentRepoObj,
		datasetRepoObj,
		fileServiceObj,
		storageInstance,
		config.GlobalConfig,
		container.GetDB(),
		redisClient,
		encrypter,
		llmClient,
		defaultModelService,
		container.GetTaskManager(),
	)

	// Create embedding service for dataset service
	var embeddingService retrieval.Embedding

	// Get GraphFlowService from container
	graphFlowServiceObj := container.GetGraphFlowService()
	taskManager := container.GetTaskManager()

	graphFlowTaskRepoObj := graphflow_repo.NewGraphFlowTaskRepository(container.GetDB())

	datasetServiceObj := datasetService.NewDatasetService(datasetRepoObj, documentRepoObj, chunkRepoObj, tenantService, fileServiceObj, embeddingService, vectorClient, defaultModelService, storageInstance, container.GetDB(), quotaSvc, enterpriseSvc, llmClient, taskManager)
	documentServiceObj := datasetService.NewDocumentService(documentRepoObj, datasetRepoObj, tenantService, indexingServiceObj, fileServiceObj, taskManager, graphFlowTaskRepoObj)

	datasetQueryServiceObj := datasetService.NewDatasetQueryService(datasetQueryRepoObj, datasetServiceObj)

	hitTestingServiceObj := datasetService.NewHitTestingService(datasetRepoObj, datasetQueryServiceObj, documentRepoObj, vectorClient, config.GlobalConfig, defaultModelService, container.GetDB(), llmClient, graphFlowServiceObj)
	chunkServiceObj := datasetService.NewChunkService(chunkRepoObj, documentRepoObj, container.GetDB())
	segmentServiceObj := datasetService.NewSegmentService(chunkServiceObj, datasetRepoObj, documentRepoObj, defaultModelService, container.GetDB(), vectorClient, llmClient, graphFlowTaskRepoObj, taskManager)
	folderRepo := datasetRepo.NewDatasetFolderRepository(container.GetDB())
	folderService := datasetService.NewDatasetFolderService(folderRepo, accountService, tenantService)

	// Create BatchHitTestingTaskRepository instance
	batchHitTestingTaskRepo := datasetRepo.NewBatchHitTestingTaskRepository(container.GetDB())

	// Create BatchHitTestingTaskManager instance
	batchTaskManager := datasetService.NewBatchHitTestingTaskManager(batchHitTestingTaskRepo)

	// Get permission service from container
	permissionService := container.GetResourcePermissionService()

	datasetHandlerObj := datasetHandler.NewDatasetHandler(
		accountService,
		datasetServiceObj,
		documentServiceObj,
		tenantService,
		hitTestingServiceObj,
		datasetQueryServiceObj,
		enterpriseService, // EnterpriseService from container
		billingService,    // BillingService from container
		defaultModelService,
		segmentServiceObj,
		folderService,     // Add folder service
		batchTaskManager,  // Add batch task manager
		permissionService, // Add permission service
	)
	documentHandlerObj := datasetHandler.NewDocumentHandler(
		documentServiceObj,
		datasetServiceObj,
		accountService,
		enterpriseService,
	)
	segmentHandlerObj := datasetHandler.NewSegmentHandler(
		segmentServiceObj,
		datasetServiceObj,
		documentServiceObj,
		accountService,
		enterpriseService,
	)

	folderHandler := datasetHandler.NewDatasetFolderHandler(datasetServiceObj, folderService, tenantService, accountService, enterpriseService, permissionService)

	datasetHandlerObj.RegisterRoutes(router)
	documentHandlerObj.RegisterRoutes(router)
	segmentHandlerObj.RegisterRoutes(router)
	folderHandler.RegisterRoutes(router)

	// Get the task type with prefix
	documentIndexingTaskType := task.TypeDocumentIndexing
	if taskManager != nil {
		documentIndexingTaskType = taskManager.GetTaskTypeWithPrefix(task.TypeDocumentIndexing)
	}

	// Register the handler with the centralized task handler registry instead of starting a separate server
	// Check if the handler was newly registered to avoid duplicate registration
	if isNew := container.GetTaskHandlerRegistry().Register(documentIndexingTaskType, task.HandleDocumentIndexingTask(documentServiceObj)); !isNew {
		logger.Warn("Document indexing task handler was replaced", map[string]interface{}{
			"task_type": documentIndexingTaskType,
		})
	}

	// Register Graph API endpoint for knowledge graph visualization
	router.GET("/datasets/:dataset_id/graph", middleware.JWTWithOrganizationAndService(accountService), newDatasetGraphHandler(datasetServiceObj, graphFlowServiceObj))

	contentParseRunService := contentparseService.NewRunQueryService(
		contentparseRepo.NewParseRunRepository(container.GetDB()),
		contentparseRepo.NewChunkingRunRepository(container.GetDB()),
	)
	router.GET(
		"/datasets/:dataset_id/content-parse/shadow-quality",
		middleware.JWTWithOrganizationAndService(accountService),
		newDatasetContentParseShadowQualityHandler(datasetServiceObj, contentParseRunService),
	)
	router.POST(
		"/datasets/:dataset_id/content-parse/shadow-quality/sample",
		middleware.JWTWithOrganizationAndService(accountService),
		newDatasetContentParseShadowSamplingHandler(datasetServiceObj, indexingServiceObj),
	)
}

type datasetGraphPermissionChecker interface {
	GetDatasetByID(ctx context.Context, id string) (*dataset_model.Dataset, error)
	CheckDatasetPermission(ctx context.Context, datasetID, accountID, workspaceID string) (bool, error)
}

type datasetGraphReader interface {
	GetGraphData(ctx context.Context, datasetID string) (*graphflow_model.GraphDataResponse, error)
}

type datasetContentParseShadowSampler interface {
	StartContentParseShadowSampling(ctx context.Context, datasetID, organizationID string, limit int, documentIDs []string) (*datasetIndexing.ContentParseShadowSamplingResult, error)
}

func newDatasetGraphHandler(datasetService datasetGraphPermissionChecker, graphService datasetGraphReader) gin.HandlerFunc {
	return func(c *gin.Context) {
		datasetID := c.Param("dataset_id")
		if datasetID == "" {
			response.Fail(c, response.ErrDatasetIdRequired)
			return
		}

		if _, err := uuid.Parse(datasetID); err != nil {
			response.Fail(c, response.ErrInvalidUuid)
			return
		}

		accountID := c.GetString("account_id")
		organizationID := util.GetOrganizationID(c)
		if accountID == "" || organizationID == "" {
			response.Fail(c, response.ErrUnauthorized)
			return
		}

		dataset, err := datasetService.GetDatasetByID(c.Request.Context(), datasetID)
		if err != nil {
			handleDatasetGraphPermissionError(c, err)
			return
		}

		if dataset.OrganizationID != organizationID || dataset.WorkspaceID == "" {
			response.Fail(c, response.ErrDatasetPermissionDenied)
			return
		}

		hasPermission, err := datasetService.CheckDatasetPermission(c.Request.Context(), datasetID, accountID, dataset.WorkspaceID)
		if err != nil {
			handleDatasetGraphPermissionError(c, err)
			return
		}
		if !hasPermission {
			response.Fail(c, response.ErrDatasetPermissionDenied)
			return
		}

		graphData, err := graphService.GetGraphData(c.Request.Context(), datasetID)
		if err != nil {
			logger.Error("Failed to get graph data", err)
			c.JSON(500, gin.H{"error": "Failed to retrieve graph data"})
			return
		}

		response.Success(c, graphData)
	}
}

func handleDatasetGraphPermissionError(c *gin.Context, err error) {
	errMsg := err.Error()
	switch {
	case strings.Contains(errMsg, "not found"):
		response.Fail(c, response.ErrDatasetNotFound)
	case strings.Contains(errMsg, "no permission"), strings.Contains(errMsg, "access denied"):
		response.Fail(c, response.ErrDatasetPermissionDenied)
	default:
		logger.Error("Failed to check dataset graph permission", err)
		response.Fail(c, response.ErrDatasetGetFailed)
	}
}

func authorizeDatasetContentParseAccess(c *gin.Context, datasetService datasetGraphPermissionChecker, datasetID string) (*dataset_model.Dataset, string, bool) {
	accountID := c.GetString("account_id")
	organizationID := util.GetOrganizationID(c)
	if accountID == "" || organizationID == "" {
		response.Fail(c, response.ErrUnauthorized)
		return nil, "", false
	}

	dataset, err := datasetService.GetDatasetByID(c.Request.Context(), datasetID)
	if err != nil {
		handleDatasetGraphPermissionError(c, err)
		return nil, "", false
	}

	if dataset.OrganizationID != organizationID || dataset.WorkspaceID == "" {
		response.Fail(c, response.ErrDatasetPermissionDenied)
		return nil, "", false
	}

	hasPermission, err := datasetService.CheckDatasetPermission(c.Request.Context(), datasetID, accountID, dataset.WorkspaceID)
	if err != nil {
		handleDatasetGraphPermissionError(c, err)
		return nil, "", false
	}
	if !hasPermission {
		response.Fail(c, response.ErrDatasetPermissionDenied)
		return nil, "", false
	}

	return dataset, organizationID, true
}

func newDatasetContentParseShadowQualityHandler(datasetService datasetGraphPermissionChecker, runQuery contentparseService.RunQueryService) gin.HandlerFunc {
	return func(c *gin.Context) {
		datasetID := c.Param("dataset_id")
		if datasetID == "" {
			response.Fail(c, response.ErrDatasetIdRequired)
			return
		}

		if _, err := uuid.Parse(datasetID); err != nil {
			response.Fail(c, response.ErrInvalidUuid)
			return
		}
		if runQuery == nil {
			response.FailWithMessage(c, response.ErrSystemError, "content parse shadow quality service is not available")
			return
		}

		if _, _, ok := authorizeDatasetContentParseAccess(c, datasetService, datasetID); !ok {
			return
		}

		limit := 200
		if raw := c.Query("limit"); raw != "" {
			if parsed, parseErr := strconv.Atoi(raw); parseErr == nil && parsed > 0 {
				limit = parsed
			}
		}
		if limit > 500 {
			limit = 500
		}

		parsedDatasetID, _ := uuid.Parse(datasetID)
		summary, err := runQuery.GetLatestDatasetShadowSummary(c.Request.Context(), parsedDatasetID, limit)
		if err != nil {
			logger.Error("Failed to get dataset content parse shadow quality", err)
			response.FailWithMessage(c, response.ErrSystemError, err.Error())
			return
		}
		response.Success(c, summary)
	}
}

type datasetContentParseShadowSamplingRequest struct {
	Limit       int      `json:"limit"`
	DocumentIDs []string `json:"document_ids"`
}

func newDatasetContentParseShadowSamplingHandler(datasetService datasetGraphPermissionChecker, sampler datasetContentParseShadowSampler) gin.HandlerFunc {
	return func(c *gin.Context) {
		datasetID := c.Param("dataset_id")
		if datasetID == "" {
			response.Fail(c, response.ErrDatasetIdRequired)
			return
		}
		if _, err := uuid.Parse(datasetID); err != nil {
			response.Fail(c, response.ErrInvalidUuid)
			return
		}
		if sampler == nil {
			response.FailWithMessage(c, response.ErrSystemError, "content parse shadow sampler is not available")
			return
		}

		_, organizationID, ok := authorizeDatasetContentParseAccess(c, datasetService, datasetID)
		if !ok {
			return
		}

		var req datasetContentParseShadowSamplingRequest
		if c.Request.ContentLength > 0 {
			if err := c.ShouldBindJSON(&req); err != nil {
				response.Fail(c, response.ErrInvalidParams)
				return
			}
		}
		for _, documentID := range req.DocumentIDs {
			if _, err := uuid.Parse(documentID); err != nil {
				response.Fail(c, response.ErrInvalidUuid)
				return
			}
		}

		result, err := sampler.StartContentParseShadowSampling(c.Request.Context(), datasetID, organizationID, req.Limit, req.DocumentIDs)
		if err != nil {
			logger.Error("Failed to start dataset content parse shadow sampling", err)
			response.FailWithMessage(c, response.ErrSystemError, err.Error())
			return
		}
		response.Success(c, result)
	}
}
