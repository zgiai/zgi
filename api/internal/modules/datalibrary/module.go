package datalibrary

import (
	contentParseCap "github.com/zgiai/zgi/api/internal/capabilities/contentparse"
	"github.com/zgiai/zgi/api/internal/contracts"
	contentParseRepository "github.com/zgiai/zgi/api/internal/modules/contentparse/repository"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/handler"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/service"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/worker"
	fileRepository "github.com/zgiai/zgi/api/internal/modules/file_process/repository"
	"github.com/zgiai/zgi/api/pkg/storage"
	"gorm.io/gorm"
)

type Module struct {
	DocumentAssetRepo               repository.DocumentAssetRepository
	ReuseEventRepo                  repository.ReuseEventRepository
	ProcessingRequestRepo           repository.ProcessingRequestRepository
	ParseConfirmationItemRepo       repository.ParseConfirmationItemRepository
	DocumentChunkRepo               repository.DocumentChunkRepository
	DocumentChunkEmbeddingRepo      repository.DocumentChunkEmbeddingRepository
	VectorArtifactRepo              repository.VectorArtifactRepository
	ExtractionArtifactRepo          repository.ExtractionArtifactRepository
	KnowledgeBaseAssetRefRepo       repository.KnowledgeBaseAssetRefRepository
	DatabaseAssetRefRepo            repository.DatabaseAssetRefRepository
	DocumentAssetService            service.DocumentAssetService
	ProcessingRequestService        service.ProcessingRequestService
	FileAssetProcessingStateService service.FileAssetProcessingStateService
	ParseArtifactPersistenceService service.ParseArtifactPersistenceService
	ParseArtifactQualityService     service.ParseArtifactQualityService
	ParsePreviewService             service.ParsePreviewService
	ParseConfirmationService        service.ParseConfirmationService
	ProcessingExecutorRegistry      *service.ProcessingExecutorRegistry
	VectorArtifactService           service.VectorArtifactService
	ExtractionArtifactService       service.ExtractionArtifactService
	FileAssetSyncService            service.FileAssetSyncService
	KnowledgeBaseRefService         service.KnowledgeBaseAssetRefService
	DatabaseRefService              service.DatabaseAssetRefService
	FileProcessRunner               *worker.FileProcessRunner
	DocumentAssetHandler            *handler.DocumentAssetHandler
	VectorArtifactHandler           *handler.VectorArtifactHandler
	ExtractionArtifactHandler       *handler.ExtractionArtifactHandler
	ProcessingExecutorHandler       *handler.ProcessingExecutorHandler
}

func NewModule(db *gorm.DB) *Module {
	contentParseModule := contentParseCap.NewModule()
	return NewModuleWithStorageAndContentParse(db, storage.GetStorage(), contentParseModule.Service)
}

func NewModuleWithStorage(db *gorm.DB, artifactStorage storage.Storage) *Module {
	contentParseModule := contentParseCap.NewModule()
	return NewModuleWithStorageAndContentParse(db, artifactStorage, contentParseModule.Service)
}

func NewModuleWithStorageAndContentParse(db *gorm.DB, artifactStorage storage.Storage, contentParseService contracts.ContentParseService) *Module {
	documentAssetRepo := repository.NewDocumentAssetRepository(db)
	reuseEventRepo := repository.NewReuseEventRepository(db)
	processingRequestRepo := repository.NewProcessingRequestRepository(db)
	parseConfirmationItemRepo := repository.NewParseConfirmationItemRepository(db)
	documentChunkRepo := repository.NewDocumentChunkRepository(db)
	documentChunkEmbeddingRepo := repository.NewDocumentChunkEmbeddingRepository(db)
	vectorArtifactRepo := repository.NewVectorArtifactRepository(db)
	extractionArtifactRepo := repository.NewExtractionArtifactRepository(db)
	knowledgeBaseAssetRefRepo := repository.NewKnowledgeBaseAssetRefRepository(db)
	databaseAssetRefRepo := repository.NewDatabaseAssetRefRepository(db)
	contentParseArtifactRepo := contentParseRepository.NewArtifactRepository(db)
	fileRepo := fileRepository.NewFileRepository(db)
	documentAssetService := service.NewDocumentAssetServiceWithDownstreamRefs(documentAssetRepo, reuseEventRepo, processingRequestRepo, vectorArtifactRepo, knowledgeBaseAssetRefRepo, databaseAssetRefRepo, extractionArtifactRepo)
	processingRequestService := service.NewProcessingRequestService(processingRequestRepo)
	fileAssetProcessingStateService := service.NewFileAssetProcessingStateService(documentAssetRepo, processingRequestRepo)
	parseArtifactPersistenceService := service.NewParseArtifactPersistenceService(documentAssetRepo, contentParseArtifactRepo, artifactStorage)
	parseArtifactQualityService := service.NewParseArtifactQualityService(parseConfirmationItemRepo)
	parsePreviewService := service.NewParsePreviewService(documentAssetRepo, contentParseArtifactRepo, parseArtifactPersistenceService, parseConfirmationItemRepo)
	parseConfirmationService := service.NewParseConfirmationService(documentAssetRepo, parseConfirmationItemRepo)
	processingExecutorRegistry := service.NewDefaultProcessingExecutorRegistry()
	vectorArtifactService := service.NewVectorArtifactService(vectorArtifactRepo)
	extractionArtifactService := service.NewExtractionArtifactService(extractionArtifactRepo)
	fileAssetSyncService := service.NewFileAssetSyncService(fileRepo, documentAssetRepo, documentAssetService)
	knowledgeBaseRefService := service.NewKnowledgeBaseAssetRefService(knowledgeBaseAssetRefRepo, reuseEventRepo)
	databaseRefService := service.NewDatabaseAssetRefService(databaseAssetRefRepo, reuseEventRepo)
	fileProcessRunner := worker.NewFileProcessRunner(worker.FileProcessRunnerDeps{
		ProcessingRequests:  processingRequestRepo,
		Assets:              documentAssetRepo,
		Files:               fileRepo,
		Storage:             artifactStorage,
		ContentParse:        contentParseService,
		State:               fileAssetProcessingStateService,
		ArtifactPersistence: parseArtifactPersistenceService,
		Quality:             parseArtifactQualityService,
		ProcessingService:   processingRequestService,
	})

	return &Module{
		DocumentAssetRepo:               documentAssetRepo,
		ReuseEventRepo:                  reuseEventRepo,
		ProcessingRequestRepo:           processingRequestRepo,
		ParseConfirmationItemRepo:       parseConfirmationItemRepo,
		DocumentChunkRepo:               documentChunkRepo,
		DocumentChunkEmbeddingRepo:      documentChunkEmbeddingRepo,
		VectorArtifactRepo:              vectorArtifactRepo,
		ExtractionArtifactRepo:          extractionArtifactRepo,
		KnowledgeBaseAssetRefRepo:       knowledgeBaseAssetRefRepo,
		DatabaseAssetRefRepo:            databaseAssetRefRepo,
		DocumentAssetService:            documentAssetService,
		ProcessingRequestService:        processingRequestService,
		FileAssetProcessingStateService: fileAssetProcessingStateService,
		ParseArtifactPersistenceService: parseArtifactPersistenceService,
		ParseArtifactQualityService:     parseArtifactQualityService,
		ParsePreviewService:             parsePreviewService,
		ParseConfirmationService:        parseConfirmationService,
		ProcessingExecutorRegistry:      processingExecutorRegistry,
		VectorArtifactService:           vectorArtifactService,
		ExtractionArtifactService:       extractionArtifactService,
		FileAssetSyncService:            fileAssetSyncService,
		KnowledgeBaseRefService:         knowledgeBaseRefService,
		DatabaseRefService:              databaseRefService,
		FileProcessRunner:               fileProcessRunner,
		DocumentAssetHandler:            handler.NewDocumentAssetHandler(documentAssetService, fileAssetSyncService, processingRequestService, knowledgeBaseRefService, databaseRefService),
		VectorArtifactHandler:           handler.NewVectorArtifactHandler(vectorArtifactService, documentAssetService),
		ExtractionArtifactHandler:       handler.NewExtractionArtifactHandler(extractionArtifactService, documentAssetService),
		ProcessingExecutorHandler:       handler.NewProcessingExecutorHandler(processingExecutorRegistry, processingRequestService),
	}
}
