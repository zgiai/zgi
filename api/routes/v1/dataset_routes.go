package v1

import (
	"context"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/zgiai/zgi/api/config"
	contentparseRepo "github.com/zgiai/zgi/api/internal/modules/contentparse/repository"
	contentparseService "github.com/zgiai/zgi/api/internal/modules/contentparse/service"
	graphflow "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
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
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
	"github.com/zgiai/zgi/api/pkg/vectordb"

	"github.com/zgiai/zgi/api/pkg/storage"

	// Security and Redis
	"github.com/zgiai/zgi/api/internal/util"
	redisPkg "github.com/zgiai/zgi/api/pkg/redis"
	sec "github.com/zgiai/zgi/api/pkg/security"

	// Retrieval
	"github.com/zgiai/zgi/api/internal/modules/dataset/retrieval"

	// Middleware
	"github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/response"
)

type datasetTaskHandlerRegistry interface {
	Register(taskType string, handler func(context.Context, *asynq.Task) error) bool
}

type DatasetRouteDeps struct {
	DB                         *gorm.DB
	Storage                    storage.Storage
	AccountService             interfaces.AccountService
	WorkspaceManagementService interfaces.WorkspaceManagementService
	OrganizationService        interfaces.OrganizationService
	BillingService             interfaces.BillingService
	QuotaService               interfaces.QuotaService
	LLMClient                  llmclient.LLMClient
	DefaultModelService        llmdefaultservice.DefaultModelService
	TaskManager                *queue.TaskManager
	GraphFlowService           *graphflow.Service
	TaskHandlerRegistry        datasetTaskHandlerRegistry
	ResourcePermissionService  interfaces.ResourcePermissionService
	AuthorizationService       interfaces.AuthorizationService
}

func RegisterDatasetRoutes(router *gin.RouterGroup, deps DatasetRouteDeps) {
	validateDatasetRouteDeps(deps)

	datasetRepoObj := datasetRepo.NewDatasetRepository(deps.DB)
	documentRepoObj := datasetRepo.NewDocumentRepository(deps.DB)
	chunkRepoObj := datasetRepo.NewChunkRepository(deps.DB)
	datasetQueryRepoObj := datasetRepo.NewDatasetQueryRepository(deps.DB)

	fileRepo := fileProcessRepo.NewFileRepository(deps.DB)
	storageInstance := deps.Storage
	fileServiceObj := fileProcess.NewFileServiceWithVision(fileRepo, storageInstance, deps.DB, deps.QuotaService, deps.OrganizationService, deps.LLMClient, deps.DefaultModelService)

	vectorClient := vectordb.NewWeaviateClient(&config.GlobalConfig.VectorStore)
	redisClient := redisPkg.GetClient()
	var encrypter *sec.Encrypter
	if redisClient != nil {
		if enc, err := sec.NewEncrypter(redisClient); err == nil {
			encrypter = enc
		}
	}

	indexingServiceObj := datasetService.NewDocumentIndexingService(
		documentRepoObj,
		datasetRepoObj,
		fileServiceObj,
		storageInstance,
		config.GlobalConfig,
		deps.DB,
		redisClient,
		encrypter,
		deps.LLMClient,
		deps.DefaultModelService,
		deps.TaskManager,
	)

	// Create embedding service for dataset service
	var embeddingService retrieval.Embedding

	graphFlowServiceObj := deps.GraphFlowService
	taskManager := deps.TaskManager

	graphFlowTaskRepoObj := graphflow_repo.NewGraphFlowTaskRepository(deps.DB)

	datasetServiceObj := datasetService.NewDatasetService(datasetRepoObj, documentRepoObj, chunkRepoObj, deps.WorkspaceManagementService, fileServiceObj, embeddingService, vectorClient, deps.DefaultModelService, storageInstance, deps.DB, deps.QuotaService, deps.OrganizationService, deps.LLMClient, taskManager)
	documentServiceObj := datasetService.NewDocumentService(documentRepoObj, datasetRepoObj, deps.WorkspaceManagementService, indexingServiceObj, fileServiceObj, vectorClient, taskManager, graphFlowTaskRepoObj)

	datasetQueryServiceObj := datasetService.NewDatasetQueryService(datasetQueryRepoObj, datasetServiceObj)

	hitTestingServiceObj := datasetService.NewHitTestingService(datasetRepoObj, datasetQueryServiceObj, documentRepoObj, vectorClient, config.GlobalConfig, deps.DefaultModelService, deps.DB, deps.LLMClient, graphFlowServiceObj)
	chunkServiceObj := datasetService.NewChunkService(chunkRepoObj, documentRepoObj, deps.DB)
	segmentServiceObj := datasetService.NewSegmentService(chunkServiceObj, datasetRepoObj, documentRepoObj, deps.DefaultModelService, deps.DB, vectorClient, deps.LLMClient, graphFlowTaskRepoObj, taskManager)
	folderRepo := datasetRepo.NewDatasetFolderRepository(deps.DB)
	folderService := datasetService.NewDatasetFolderService(folderRepo, deps.AccountService, deps.WorkspaceManagementService)

	// Create BatchHitTestingTaskRepository instance
	batchHitTestingTaskRepo := datasetRepo.NewBatchHitTestingTaskRepository(deps.DB)

	// Create BatchHitTestingTaskManager instance
	batchTaskManager := datasetService.NewBatchHitTestingTaskManager(batchHitTestingTaskRepo)

	datasetHandlerObj := datasetHandler.NewDatasetHandler(
		deps.AccountService,
		datasetServiceObj,
		documentServiceObj,
		deps.WorkspaceManagementService,
		hitTestingServiceObj,
		datasetQueryServiceObj,
		deps.OrganizationService,
		deps.BillingService,
		deps.DefaultModelService,
		segmentServiceObj,
		folderService,
		batchTaskManager,
		deps.ResourcePermissionService,
	)
	documentHandlerObj := datasetHandler.NewDocumentHandler(
		documentServiceObj,
		datasetServiceObj,
		deps.AccountService,
		deps.OrganizationService,
		deps.AuthorizationService,
	)
	segmentHandlerObj := datasetHandler.NewSegmentHandler(
		segmentServiceObj,
		datasetServiceObj,
		documentServiceObj,
		deps.AccountService,
		deps.OrganizationService,
		deps.AuthorizationService,
	)

	folderHandler := datasetHandler.NewDatasetFolderHandler(datasetServiceObj, folderService, deps.WorkspaceManagementService, deps.AccountService, deps.OrganizationService, deps.ResourcePermissionService, deps.AuthorizationService)

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
	if isNew := deps.TaskHandlerRegistry.Register(documentIndexingTaskType, task.HandleDocumentIndexingTask(documentServiceObj)); !isNew {
		logger.Warn("Document indexing task handler was replaced", map[string]interface{}{
			"task_type": documentIndexingTaskType,
		})
	}

	// Register Graph API endpoint for knowledge graph visualization
	router.GET("/datasets/:dataset_id/graph", middleware.JWTWithOrganizationAndService(deps.AccountService), newDatasetGraphHandler(datasetServiceObj, graphFlowServiceObj))

	contentParseRunService := contentparseService.NewRunQueryService(
		contentparseRepo.NewParseRunRepository(deps.DB),
		contentparseRepo.NewChunkingRunRepository(deps.DB),
	)
	router.GET(
		"/datasets/:dataset_id/content-parse/shadow-quality",
		middleware.JWTWithOrganizationAndService(deps.AccountService),
		newDatasetContentParseShadowQualityHandler(datasetServiceObj, contentParseRunService),
	)
	router.POST(
		"/datasets/:dataset_id/content-parse/shadow-quality/sample",
		middleware.JWTWithOrganizationAndService(deps.AccountService),
		newDatasetContentParseShadowSamplingHandler(datasetServiceObj, indexingServiceObj),
	)
}

func validateDatasetRouteDeps(deps DatasetRouteDeps) {
	if deps.DB == nil {
		panic("dataset routes require db")
	}
	if deps.Storage == nil {
		panic("dataset routes require storage")
	}
	if deps.AccountService == nil {
		panic("dataset routes require account service")
	}
	if deps.WorkspaceManagementService == nil {
		panic("dataset routes require workspace management service")
	}
	if deps.OrganizationService == nil {
		panic("dataset routes require organization service")
	}
	if deps.BillingService == nil {
		panic("dataset routes require billing service")
	}
	if deps.QuotaService == nil {
		panic("dataset routes require quota service")
	}
	if deps.LLMClient == nil {
		panic("dataset routes require llm client")
	}
	if deps.DefaultModelService == nil {
		panic("dataset routes require default model service")
	}
	if deps.TaskManager == nil {
		panic("dataset routes require task manager")
	}
	if deps.GraphFlowService == nil {
		panic("dataset routes require graph flow service")
	}
	if deps.TaskHandlerRegistry == nil {
		panic("dataset routes require task handler registry")
	}
	if deps.ResourcePermissionService == nil {
		panic("dataset routes require resource permission service")
	}
	if deps.AuthorizationService == nil {
		panic("dataset routes require authorization service")
	}
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
