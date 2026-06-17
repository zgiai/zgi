package datalibrary

import (
	"github.com/zgiai/zgi/api/config"
	contentParseCap "github.com/zgiai/zgi/api/internal/capabilities/contentparse"
	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/routing"
	"github.com/zgiai/zgi/api/internal/contracts"
	contentParseModule "github.com/zgiai/zgi/api/internal/modules/contentparse"
	contentParseRepository "github.com/zgiai/zgi/api/internal/modules/contentparse/repository"
	contentParseService "github.com/zgiai/zgi/api/internal/modules/contentparse/service"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/handler"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/service"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/worker"
	datasetRepository "github.com/zgiai/zgi/api/internal/modules/dataset/repository"
	fileRepository "github.com/zgiai/zgi/api/internal/modules/file_process/repository"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	"github.com/zgiai/zgi/api/pkg/storage"
	"github.com/zgiai/zgi/api/pkg/vectordb"
	"gorm.io/gorm"
)

type Module struct {
	DocumentAssetRepo                  repository.DocumentAssetRepository
	ReuseEventRepo                     repository.ReuseEventRepository
	ProcessingRequestRepo              repository.ProcessingRequestRepository
	ParseConfirmationItemRepo          repository.ParseConfirmationItemRepository
	DocumentChunkRepo                  repository.DocumentChunkRepository
	DocumentChunkEmbeddingRepo         repository.DocumentChunkEmbeddingRepository
	VectorArtifactRepo                 repository.VectorArtifactRepository
	ExtractionArtifactRepo             repository.ExtractionArtifactRepository
	KnowledgeBaseAssetRefRepo          repository.KnowledgeBaseAssetRefRepository
	DatabaseAssetRefRepo               repository.DatabaseAssetRefRepository
	DocumentAssetService               service.DocumentAssetService
	ProcessingRequestService           service.ProcessingRequestService
	FileAssetProcessingStateService    service.FileAssetProcessingStateService
	ParseArtifactPersistenceService    service.ParseArtifactPersistenceService
	ParseArtifactImageAssetService     service.ParseArtifactImageAssetService
	ParseArtifactQualityService        service.ParseArtifactQualityService
	ParsePreviewService                service.ParsePreviewService
	ParseConfirmationService           service.ParseConfirmationService
	ParseArtifactConfirmationService   service.ParseArtifactConfirmationService
	ParseArtifactChunkTransformService service.ParseArtifactChunkTransformService
	DocumentChunkGenerationService     service.DocumentChunkGenerationService
	DocumentChunkEmbeddingService      service.DocumentChunkEmbeddingService
	FileAssetDetailService             service.FileAssetDetailService
	FileAssetSummaryService            service.FileAssetSummaryService
	FileAssetChunkService              service.FileAssetChunkService
	FileAssetChunkEditService          service.FileAssetChunkEditService
	FileAssetVectorIndexService        service.FileAssetVectorIndexService
	FileAssetQAService                 service.FileAssetQAService
	FileAssetDeletionService           service.FileAssetDeletionService
	ProcessingExecutorRegistry         *service.ProcessingExecutorRegistry
	VectorArtifactService              service.VectorArtifactService
	ExtractionArtifactService          service.ExtractionArtifactService
	FileAssetSyncService               service.FileAssetSyncService
	KnowledgeBaseRefService            service.KnowledgeBaseAssetRefService
	DatabaseRefService                 service.DatabaseAssetRefService
	FileProcessRunner                  *worker.FileProcessRunner
	GenerateCurrentResultRunner        *worker.GenerateCurrentResultRunner
	DocumentAssetHandler               *handler.DocumentAssetHandler
	VectorArtifactHandler              *handler.VectorArtifactHandler
	ExtractionArtifactHandler          *handler.ExtractionArtifactHandler
	ProcessingExecutorHandler          *handler.ProcessingExecutorHandler
}

type ContentParseRuntime struct {
	Service         contracts.ContentParseService
	Orchestrator    *contentParseCap.Orchestrator
	Planner         routing.Planner
	CatalogResolver contentParseService.ProviderCatalogResolver
	Catalog         *contracts.ParseProviderCatalog
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
	return NewModuleWithRuntime(db, artifactStorage, contentParseService, nil, nil)
}

func NewModuleWithRuntime(
	db *gorm.DB,
	artifactStorage storage.Storage,
	contentParseService contracts.ContentParseService,
	llmClient llmclient.LLMClient,
	defaultModelSvc llmdefaultservice.DefaultModelService,
) *Module {
	return NewModuleWithContentParseRuntime(db, artifactStorage, ContentParseRuntime{Service: contentParseService}, llmClient, defaultModelSvc)
}

func NewModuleWithContentParseModule(
	db *gorm.DB,
	artifactStorage storage.Storage,
	contentParse *contentParseModule.Module,
	llmClient llmclient.LLMClient,
	defaultModelSvc llmdefaultservice.DefaultModelService,
) *Module {
	runtime := ContentParseRuntime{}
	if contentParse != nil {
		runtime = ContentParseRuntime{
			Service:         contentParse.ContentParseService,
			Orchestrator:    contentParse.Orchestrator,
			Planner:         contentParse.Planner,
			CatalogResolver: contentParse.ProviderCatalogs,
			Catalog:         contentParse.Catalog,
		}
	}
	return NewModuleWithContentParseRuntime(db, artifactStorage, runtime, llmClient, defaultModelSvc)
}

func NewModuleWithContentParseRuntime(
	db *gorm.DB,
	artifactStorage storage.Storage,
	contentParseRuntime ContentParseRuntime,
	llmClient llmclient.LLMClient,
	defaultModelSvc llmdefaultservice.DefaultModelService,
) *Module {
	contentParseService := contentParseRuntime.Service
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
	datasetRepo := datasetRepository.NewDatasetRepository(db)
	datasetDocumentRepo := datasetRepository.NewDocumentRepository(db)
	contentParseArtifactRepo := contentParseRepository.NewArtifactRepository(db)
	fileRepo := fileRepository.NewFileRepository(db)
	documentAssetService := service.NewDocumentAssetServiceWithDownstreamRefs(documentAssetRepo, reuseEventRepo, processingRequestRepo, vectorArtifactRepo, knowledgeBaseAssetRefRepo, databaseAssetRefRepo, extractionArtifactRepo)
	processingRequestService := service.NewProcessingRequestService(processingRequestRepo)
	fileAssetProcessingStateService := service.NewFileAssetProcessingStateServiceWithDatasetRefs(documentAssetRepo, processingRequestRepo, knowledgeBaseAssetRefRepo, datasetDocumentRepo)
	parseArtifactPersistenceService := service.NewParseArtifactPersistenceService(documentAssetRepo, contentParseArtifactRepo, artifactStorage)
	parseArtifactImageAssetService := service.NewParseArtifactImageAssetService(artifactStorage)
	parseArtifactQualityService := service.NewParseArtifactQualityService(parseConfirmationItemRepo)
	parsePreviewService := service.NewParsePreviewService(documentAssetRepo, contentParseArtifactRepo, parseArtifactPersistenceService, parseConfirmationItemRepo)
	parseConfirmationService := service.NewParseConfirmationService(documentAssetRepo, parseConfirmationItemRepo)
	parseArtifactConfirmationService := service.NewParseArtifactConfirmationService(documentAssetRepo, contentParseArtifactRepo, parseArtifactPersistenceService, parseConfirmationItemRepo)
	parseArtifactChunkTransformService := service.NewParseArtifactChunkTransformService(artifactStorage, nil, defaultModelSvc, llmClient)
	documentChunkGenerationService := service.NewDocumentChunkGenerationService(documentAssetRepo, documentChunkRepo)
	var vectorDB vectordb.VectorDB
	if config.GlobalConfig != nil {
		if db, err := vectordb.NewVectorDB(&config.GlobalConfig.VectorStore); err == nil {
			vectorDB = db
		}
	}
	var fileAssetVectorIndexService service.FileAssetVectorIndexService
	if vectorDB != nil {
		fileAssetVectorIndexService = service.NewFileAssetVectorIndexService(documentChunkRepo, documentChunkEmbeddingRepo, vectorDB)
	}
	documentChunkEmbeddingService := service.NewDocumentChunkEmbeddingService(
		documentAssetRepo,
		documentChunkEmbeddingRepo,
		llmClient,
		defaultModelSvc,
		service.WithDocumentChunkVectorIndex(fileAssetVectorIndexService),
	)
	fileAssetDetailService := service.NewFileAssetDetailService(documentAssetRepo, processingRequestRepo, parseConfirmationItemRepo, documentChunkRepo, documentChunkEmbeddingRepo)
	fileAssetSummaryService := service.NewFileAssetSummaryService(documentAssetRepo, parseConfirmationItemRepo, documentChunkRepo, documentChunkEmbeddingRepo)
	fileAssetChunkService := service.NewFileAssetChunkService(documentAssetRepo, documentChunkRepo, documentChunkEmbeddingRepo)
	fileAssetChunkEditService := service.NewFileAssetChunkEditServiceWithDatasetRefs(
		documentAssetRepo,
		documentChunkRepo,
		documentChunkEmbeddingRepo,
		documentChunkEmbeddingService,
		fileAssetVectorIndexService,
		knowledgeBaseAssetRefRepo,
		datasetDocumentRepo,
		nil,
		datasetRepo,
	)
	fileAssetQAService := service.NewFileAssetQAService(documentAssetRepo, documentChunkRepo, documentChunkEmbeddingRepo, fileAssetVectorIndexService, llmClient, defaultModelSvc)
	fileAssetDeletionService := service.NewFileAssetDeletionService(db, fileAssetVectorIndexService)
	processingExecutorRegistry := service.NewDefaultProcessingExecutorRegistry()
	vectorArtifactService := service.NewVectorArtifactService(vectorArtifactRepo)
	extractionArtifactService := service.NewExtractionArtifactService(extractionArtifactRepo)
	fileAssetSyncService := service.NewFileAssetSyncService(fileRepo, documentAssetRepo, documentAssetService)
	knowledgeBaseRefService := service.NewKnowledgeBaseAssetRefService(knowledgeBaseAssetRefRepo, reuseEventRepo)
	databaseRefService := service.NewDatabaseAssetRefService(databaseAssetRefRepo, reuseEventRepo)
	fileProcessRunner := worker.NewFileProcessRunner(worker.FileProcessRunnerDeps{
		ProcessingRequests:       processingRequestRepo,
		Assets:                   documentAssetRepo,
		Files:                    fileRepo,
		Storage:                  artifactStorage,
		ContentParse:             contentParseService,
		ContentParseOrchestrator: contentParseRuntime.Orchestrator,
		ContentParsePlanner:      contentParseRuntime.Planner,
		ProviderCatalogs:         contentParseRuntime.CatalogResolver,
		ContentParseCatalog:      contentParseRuntime.Catalog,
		State:                    fileAssetProcessingStateService,
		ImageAssets:              parseArtifactImageAssetService,
		ArtifactPersistence:      parseArtifactPersistenceService,
		Quality:                  parseArtifactQualityService,
		ProcessingService:        processingRequestService,
	})
	generateCurrentResultRunner := worker.NewGenerateCurrentResultRunner(worker.GenerateCurrentResultRunnerDeps{
		ProcessingRequests:  processingRequestRepo,
		Assets:              documentAssetRepo,
		Artifacts:           contentParseArtifactRepo,
		State:               fileAssetProcessingStateService,
		ArtifactPersistence: parseArtifactPersistenceService,
		ParseConfirmations:  parseConfirmationItemRepo,
		Transform:           parseArtifactChunkTransformService,
		ChunkGeneration:     documentChunkGenerationService,
		Embedding:           documentChunkEmbeddingService,
		EmbeddingTargets:    documentChunkEmbeddingRepo,
		ProcessingService:   processingRequestService,
		Refs:                knowledgeBaseAssetRefRepo,
		Datasets:            datasetRepo,
	})

	return &Module{
		DocumentAssetRepo:                  documentAssetRepo,
		ReuseEventRepo:                     reuseEventRepo,
		ProcessingRequestRepo:              processingRequestRepo,
		ParseConfirmationItemRepo:          parseConfirmationItemRepo,
		DocumentChunkRepo:                  documentChunkRepo,
		DocumentChunkEmbeddingRepo:         documentChunkEmbeddingRepo,
		VectorArtifactRepo:                 vectorArtifactRepo,
		ExtractionArtifactRepo:             extractionArtifactRepo,
		KnowledgeBaseAssetRefRepo:          knowledgeBaseAssetRefRepo,
		DatabaseAssetRefRepo:               databaseAssetRefRepo,
		DocumentAssetService:               documentAssetService,
		ProcessingRequestService:           processingRequestService,
		FileAssetProcessingStateService:    fileAssetProcessingStateService,
		ParseArtifactPersistenceService:    parseArtifactPersistenceService,
		ParseArtifactImageAssetService:     parseArtifactImageAssetService,
		ParseArtifactQualityService:        parseArtifactQualityService,
		ParsePreviewService:                parsePreviewService,
		ParseConfirmationService:           parseConfirmationService,
		ParseArtifactConfirmationService:   parseArtifactConfirmationService,
		ParseArtifactChunkTransformService: parseArtifactChunkTransformService,
		DocumentChunkGenerationService:     documentChunkGenerationService,
		DocumentChunkEmbeddingService:      documentChunkEmbeddingService,
		FileAssetDetailService:             fileAssetDetailService,
		FileAssetSummaryService:            fileAssetSummaryService,
		FileAssetChunkService:              fileAssetChunkService,
		FileAssetChunkEditService:          fileAssetChunkEditService,
		FileAssetVectorIndexService:        fileAssetVectorIndexService,
		FileAssetQAService:                 fileAssetQAService,
		FileAssetDeletionService:           fileAssetDeletionService,
		ProcessingExecutorRegistry:         processingExecutorRegistry,
		VectorArtifactService:              vectorArtifactService,
		ExtractionArtifactService:          extractionArtifactService,
		FileAssetSyncService:               fileAssetSyncService,
		KnowledgeBaseRefService:            knowledgeBaseRefService,
		DatabaseRefService:                 databaseRefService,
		FileProcessRunner:                  fileProcessRunner,
		GenerateCurrentResultRunner:        generateCurrentResultRunner,
		DocumentAssetHandler:               handler.NewDocumentAssetHandler(documentAssetService, fileAssetSyncService, processingRequestService, knowledgeBaseRefService, databaseRefService),
		VectorArtifactHandler:              handler.NewVectorArtifactHandler(vectorArtifactService, documentAssetService),
		ExtractionArtifactHandler:          handler.NewExtractionArtifactHandler(extractionArtifactService, documentAssetService),
		ProcessingExecutorHandler:          handler.NewProcessingExecutorHandler(processingExecutorRegistry, processingRequestService),
	}
}
