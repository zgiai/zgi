package v1

import (
	"github.com/gin-gonic/gin"

	"github.com/zgiai/ginext/internal/container"
	"github.com/zgiai/ginext/internal/modules/app/workflow/tool_file"
	toolfilescheduler "github.com/zgiai/ginext/internal/modules/app/workflow/tool_file/scheduler"
	dataset_repo "github.com/zgiai/ginext/internal/modules/dataset/repository"
	fileProcessHandler "github.com/zgiai/ginext/internal/modules/file_process/handler"
	fileProcessRepo "github.com/zgiai/ginext/internal/modules/file_process/repository"
	fileScheduler "github.com/zgiai/ginext/internal/modules/file_process/scheduler"
	fileProcessService "github.com/zgiai/ginext/internal/modules/file_process/service"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	jwtMiddleware "github.com/zgiai/ginext/middleware"
	"github.com/zgiai/ginext/pkg/database"
	"github.com/zgiai/ginext/pkg/logger"
	"github.com/zgiai/ginext/pkg/storage"
)

// registerFileRoutesLegacy keeps original implementation details.
func registerFileRoutesLegacy(v1 *gin.RouterGroup, accountService interfaces.AccountService, serviceContainer *container.ServiceContainer) {
	db := database.GetDB()

	storageClient := storage.GetStorage()

	fileRepo := fileProcessRepo.NewFileRepository(db)
	fileFolderRepo := fileProcessRepo.NewFileFolderRepository(db)
	fileFavoriteRepo := fileProcessRepo.NewFileFavoriteRepository(db)
	documentRepo := dataset_repo.NewDocumentRepository(db)
	datasetRepo := dataset_repo.NewDatasetRepository(db)

	// Get quota and enterprise services from service container
	quotaService := serviceContainer.GetQuotaService()
	enterpriseService := serviceContainer.GetOrganizationService()
	tenantService := serviceContainer.GetTenantService()

	fileService := fileProcessService.NewFileServiceWithVision(fileRepo, storageClient, db, quotaService, enterpriseService, serviceContainer.GetLLMClient(), serviceContainer.GetDefaultModelService())
	fileFolderService := fileProcessService.NewFileResourceService(fileFolderRepo, fileRepo, documentRepo, datasetRepo, accountService)
	fileFavoriteService := fileProcessService.NewFileFavoriteService(fileFavoriteRepo, fileRepo)

	fileHandler := fileProcessHandler.NewFileHandler(fileService, fileFolderService, accountService, tenantService, enterpriseService)
	fileResourceHandler := fileProcessHandler.NewFileResourceHandler(fileFolderService, fileService, accountService, enterpriseService, fileFavoriteService)
	fileFavoriteHandler := fileProcessHandler.NewFileFavoriteHandler(fileFavoriteService, fileService, accountService)

	// Create image preview handler
	imagePreviewHandler := fileProcessHandler.NewImagePreviewHandler(fileService, accountService)
	toolFileHandler := tool_file.NewHTTPHandler(tool_file.GlobalToolFileManager)

	files := v1.Group("/files",
		jwtMiddleware.JWTWithOrganizationAndService(accountService),
	)
	{
		files.GET("/upload", fileHandler.GetUploadConfig)

		files.POST("/upload", fileHandler.UploadFile)

		files.POST("/text", fileHandler.CreateTextFile)

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
		jwtMiddleware.JWTWithOrganizationAndService(accountService),
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
		jwtMiddleware.JWTWithOrganizationAndService(accountService),
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
			jwtMiddleware.FilePreviewAuthMiddleware(accountService),
			imagePreviewHandler.GetFilePreview,
		)
		filePreview.GET("/tools/:tool_file_id", toolFileHandler.GetToolFile)
	}
}

// RegisterFileRoutes now uses modular services.
func RegisterFileRoutes(v1 *gin.RouterGroup, accountService interfaces.AccountService, serviceContainer *container.ServiceContainer) {
	registerFileRoutesLegacy(v1, accountService, serviceContainer)

	scheduler := serviceContainer.GetScheduler()
	fileService := serviceContainer.GetFileService()
	if err := fileScheduler.RegisterFileTasks(scheduler, fileService); err != nil {
		logger.Error("Failed to register file scheduled tasks", err)
	} else {
		logger.Info("File scheduled tasks registered", nil)
	}

	if err := toolfilescheduler.RegisterToolFileTasks(scheduler, tool_file.GlobalToolFileManager); err != nil {
		logger.Error("Failed to register tool file scheduled tasks", err)
	} else {
		logger.Info("Tool file scheduled tasks registered", nil)
	}
}
