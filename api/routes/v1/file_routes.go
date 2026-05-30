package v1

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	toolfilescheduler "github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file/scheduler"
	datalibrarymodule "github.com/zgiai/zgi/api/internal/modules/datalibrary"
	datalibraryworker "github.com/zgiai/zgi/api/internal/modules/datalibrary/worker"
	dataset_repo "github.com/zgiai/zgi/api/internal/modules/dataset/repository"
	fileProcessHandler "github.com/zgiai/zgi/api/internal/modules/file_process/handler"
	fileProcessRepo "github.com/zgiai/zgi/api/internal/modules/file_process/repository"
	fileScheduler "github.com/zgiai/zgi/api/internal/modules/file_process/scheduler"
	fileProcessService "github.com/zgiai/zgi/api/internal/modules/file_process/service"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	jwtMiddleware "github.com/zgiai/zgi/api/middleware"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
	pkgscheduler "github.com/zgiai/zgi/api/pkg/scheduler"
	"github.com/zgiai/zgi/api/pkg/storage"
)

type FileRouteDeps struct {
	DB                         *gorm.DB
	Storage                    storage.Storage
	AccountService             interfaces.AccountService
	WorkspaceManagementService interfaces.WorkspaceManagementService
	OrganizationService        interfaces.OrganizationService
	QuotaService               interfaces.QuotaService
	LLMClient                  llmclient.LLMClient
	DefaultModelService        llmdefaultservice.DefaultModelService
	DataLibraryModule          *datalibrarymodule.Module
	TaskManager                *queue.TaskManager
	Scheduler                  *pkgscheduler.Scheduler
	ScheduledFileService       interfaces.FileService
}

// registerFileRoutesLegacy keeps original implementation details.
func registerFileRoutesLegacy(v1 *gin.RouterGroup, deps FileRouteDeps) {
	fileRepo := fileProcessRepo.NewFileRepository(deps.DB)
	fileFolderRepo := fileProcessRepo.NewFileFolderRepository(deps.DB)
	fileFavoriteRepo := fileProcessRepo.NewFileFavoriteRepository(deps.DB)
	documentRepo := dataset_repo.NewDocumentRepository(deps.DB)
	datasetRepo := dataset_repo.NewDatasetRepository(deps.DB)

	fileService := fileProcessService.NewFileServiceWithVision(fileRepo, deps.Storage, deps.DB, deps.QuotaService, deps.OrganizationService, deps.LLMClient, deps.DefaultModelService)
	fileFolderService := fileProcessService.NewFileResourceService(fileFolderRepo, fileRepo, documentRepo, datasetRepo, deps.AccountService)
	fileFavoriteService := fileProcessService.NewFileFavoriteService(fileFavoriteRepo, fileRepo)

	fileHandler := fileProcessHandler.NewFileHandler(
		fileService,
		fileFolderService,
		deps.AccountService,
		deps.WorkspaceManagementService,
		deps.OrganizationService,
		fileProcessHandler.FileAssetProcessingServices{
			StateService:                     deps.DataLibraryModule.FileAssetProcessingStateService,
			ProcessingService:                deps.DataLibraryModule.ProcessingRequestService,
			ParsePreviewService:              deps.DataLibraryModule.ParsePreviewService,
			ParseConfirmationService:         deps.DataLibraryModule.ParseConfirmationService,
			ParseArtifactConfirmationService: deps.DataLibraryModule.ParseArtifactConfirmationService,
			TaskEnqueuer:                     datalibraryworker.NewFileProcessTaskDispatcher(deps.TaskManager),
		},
	)
	fileResourceHandler := fileProcessHandler.NewFileResourceHandler(fileFolderService, fileService, deps.AccountService, deps.OrganizationService, fileFavoriteService)
	fileFavoriteHandler := fileProcessHandler.NewFileFavoriteHandler(fileFavoriteService, fileService, deps.AccountService)

	// Create image preview handler
	imagePreviewHandler := fileProcessHandler.NewImagePreviewHandler(fileService, deps.AccountService, deps.OrganizationService, deps.Storage)
	toolFileHandler := tool_file.NewHTTPHandler(tool_file.GlobalToolFileManager)

	files := v1.Group("/files",
		jwtMiddleware.JWTWithOrganizationAndService(deps.AccountService),
	)
	{
		files.GET("/upload", fileHandler.GetUploadConfig)

		files.POST("/upload", fileHandler.UploadFile)

		files.POST("/text", fileHandler.CreateTextFile)

		files.GET("/metadata", fileHandler.GetFilesMetadata)

		files.POST("/:file_id/processing-requests", fileHandler.CreateProcessingRequest)

		files.GET("/:file_id/parse-preview", fileHandler.GetFileParsePreview)

		files.GET("/:file_id/parse-confirmation-items", fileHandler.ListParseConfirmationItems)

		files.POST("/:file_id/parse-confirmation-items/batch-ignore", fileHandler.BatchIgnoreParseConfirmationItems)

		files.POST("/:file_id/parse-confirmation-items/:item_id/resolve", fileHandler.ResolveParseConfirmationItem)

		files.GET("/:file_id/preview", fileHandler.GetFilePreview)

		files.GET("/:file_id/preview-url", fileHandler.GetFileOriginalPreviewURL)

		files.GET("/:file_id/download", fileHandler.DownloadFile)

		files.GET("/support-type", fileHandler.GetSupportedFileTypes)

		files.GET("", fileHandler.ListFiles)

		// Add file deletion endpoint
		files.DELETE("", fileHandler.DeleteFiles)

		// Storage usage endpoint
		files.GET("/storage-usage", fileHandler.GetStorageUsage)

		// Add file archive endpoints
		files.POST("/archive", fileResourceHandler.ArchiveFiles)
		files.POST("/unarchive", fileResourceHandler.UnarchiveFiles)

		// Add archived files list endpoint
		files.GET("/archived", fileHandler.ListArchivedFiles)

		// File statistics route
		files.GET("/statistics", fileResourceHandler.GetFileStatistics)

		// File document relation routes
		files.GET("/:file_id/related-documents", fileResourceHandler.GetRelatedDocuments)
		files.GET("/:file_id/related-datasets", fileResourceHandler.GetRelatedDatasets)
		files.GET("/:file_id/related-resources", fileResourceHandler.GetRelatedResources)
	}

	// File folder routes
	fileFolders := v1.Group("/file-folders",
		jwtMiddleware.JWTWithOrganizationAndService(deps.AccountService),
	)
	{
		fileFolders.GET("", fileResourceHandler.GetFolders)
		fileFolders.POST("", fileResourceHandler.PostFolder)
		fileFolders.GET("/:folder_id", fileResourceHandler.GetFolder)
		fileFolders.PATCH("/:folder_id", fileResourceHandler.PatchFolder)
		fileFolders.DELETE("/:folder_id", fileResourceHandler.DeleteFolder)
		fileFolders.GET("/files", fileResourceHandler.GetFilesInFolder)
		fileFolders.GET("/all-files", fileResourceHandler.ListAllFiles)
		fileFolders.GET("/recent-files", fileResourceHandler.ListRecentFiles)
		fileFolders.GET("/favorite-files", fileResourceHandler.ListFavoriteFiles)
		fileFolders.POST("/move-files", fileResourceHandler.MoveFilesToFolder)
		fileFolders.POST("/move-folder", fileResourceHandler.MoveFolderToFolder)

		// File folder permission routes
		fileFolders.GET("/:folder_id/permission-tenants", fileResourceHandler.GetFolderPermissionTenants)
		fileFolders.GET("/:folder_id/permission-tenant-details", fileResourceHandler.GetFolderPermissionTenantDetails)
	}

	// File favorites routes
	fileFavorites := v1.Group("/file-favorites",
		jwtMiddleware.JWTWithOrganizationAndService(deps.AccountService),
	)
	{
		fileFavorites.POST("", fileFavoriteHandler.FavoriteFile)
		fileFavorites.DELETE("/:file_id", fileFavoriteHandler.UnfavoriteFile)
		fileFavorites.POST("/batch", fileFavoriteHandler.BatchFavoriteFiles)
		fileFavorites.POST("/batch-unfavorite", fileFavoriteHandler.BatchUnfavoriteFiles)
		fileFavorites.GET("", fileFavoriteHandler.ListFavorites)
	}

	// File preview route with dual authentication (URL signature or JWT)
	// This route is separate to support both signed URL and authenticated access
	// Priority: URL signature first, then JWT header fallback
	filePreview := v1.Group("/files")
	{
		filePreview.GET("/mineru-images", imagePreviewHandler.GetMinerUImage)
		filePreview.GET("/:file_id/file-preview",
			jwtMiddleware.FilePreviewAuthMiddleware(deps.AccountService),
			imagePreviewHandler.GetFilePreview,
		)
		filePreview.GET("/tools/:tool_file_id", toolFileHandler.GetToolFile)
	}
}

// RegisterFileRoutes now uses modular services.
func RegisterFileRoutes(v1 *gin.RouterGroup, deps FileRouteDeps) {
	validateFileRouteDeps(deps)
	registerFileRoutesLegacy(v1, deps)

	if err := fileScheduler.RegisterFileTasks(deps.Scheduler, deps.ScheduledFileService); err != nil {
		logger.Error("Failed to register file scheduled tasks", err)
	} else {
		logger.Info("File scheduled tasks registered", nil)
	}

	if err := toolfilescheduler.RegisterToolFileTasks(deps.Scheduler, tool_file.GlobalToolFileManager); err != nil {
		logger.Error("Failed to register tool file scheduled tasks", err)
	} else {
		logger.Info("Tool file scheduled tasks registered", nil)
	}
}

func validateFileRouteDeps(deps FileRouteDeps) {
	if deps.DB == nil {
		panic("file routes require db")
	}
	if deps.Storage == nil {
		panic("file routes require storage")
	}
	if deps.AccountService == nil {
		panic("file routes require account service")
	}
	if deps.WorkspaceManagementService == nil {
		panic("file routes require workspace management service")
	}
	if deps.OrganizationService == nil {
		panic("file routes require organization service")
	}
	if deps.QuotaService == nil {
		panic("file routes require quota service")
	}
	if deps.LLMClient == nil {
		panic("file routes require llm client")
	}
	if deps.DefaultModelService == nil {
		panic("file routes require default model service")
	}
	if deps.DataLibraryModule == nil {
		panic("file routes require data library module")
	}
	if deps.TaskManager == nil {
		panic("file routes require task manager")
	}
	if deps.Scheduler == nil {
		panic("file routes require scheduler")
	}
	if deps.ScheduledFileService == nil {
		panic("file routes require scheduled file service")
	}
}
