package datalibrary

import (
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/handler"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/service"
	fileRepository "github.com/zgiai/zgi/api/internal/modules/file_process/repository"
	"gorm.io/gorm"
)

type Module struct {
	DocumentAssetRepo          repository.DocumentAssetRepository
	ReuseEventRepo             repository.ReuseEventRepository
	ProcessingRequestRepo      repository.ProcessingRequestRepository
	ParseConfirmationItemRepo  repository.ParseConfirmationItemRepository
	VectorArtifactRepo         repository.VectorArtifactRepository
	ExtractionArtifactRepo     repository.ExtractionArtifactRepository
	KnowledgeBaseAssetRefRepo  repository.KnowledgeBaseAssetRefRepository
	DatabaseAssetRefRepo       repository.DatabaseAssetRefRepository
	DocumentAssetService       service.DocumentAssetService
	ProcessingRequestService   service.ProcessingRequestService
	ProcessingExecutorRegistry *service.ProcessingExecutorRegistry
	VectorArtifactService      service.VectorArtifactService
	ExtractionArtifactService  service.ExtractionArtifactService
	FileAssetSyncService       service.FileAssetSyncService
	KnowledgeBaseRefService    service.KnowledgeBaseAssetRefService
	DatabaseRefService         service.DatabaseAssetRefService
	DocumentAssetHandler       *handler.DocumentAssetHandler
	VectorArtifactHandler      *handler.VectorArtifactHandler
	ExtractionArtifactHandler  *handler.ExtractionArtifactHandler
	ProcessingExecutorHandler  *handler.ProcessingExecutorHandler
}

func NewModule(db *gorm.DB) *Module {
	documentAssetRepo := repository.NewDocumentAssetRepository(db)
	reuseEventRepo := repository.NewReuseEventRepository(db)
	processingRequestRepo := repository.NewProcessingRequestRepository(db)
	parseConfirmationItemRepo := repository.NewParseConfirmationItemRepository(db)
	vectorArtifactRepo := repository.NewVectorArtifactRepository(db)
	extractionArtifactRepo := repository.NewExtractionArtifactRepository(db)
	knowledgeBaseAssetRefRepo := repository.NewKnowledgeBaseAssetRefRepository(db)
	databaseAssetRefRepo := repository.NewDatabaseAssetRefRepository(db)
	fileRepo := fileRepository.NewFileRepository(db)
	documentAssetService := service.NewDocumentAssetServiceWithDownstreamRefs(documentAssetRepo, reuseEventRepo, processingRequestRepo, vectorArtifactRepo, knowledgeBaseAssetRefRepo, databaseAssetRefRepo, extractionArtifactRepo)
	processingRequestService := service.NewProcessingRequestService(processingRequestRepo)
	processingExecutorRegistry := service.NewDefaultProcessingExecutorRegistry()
	vectorArtifactService := service.NewVectorArtifactService(vectorArtifactRepo)
	extractionArtifactService := service.NewExtractionArtifactService(extractionArtifactRepo)
	fileAssetSyncService := service.NewFileAssetSyncService(fileRepo, documentAssetRepo, documentAssetService)
	knowledgeBaseRefService := service.NewKnowledgeBaseAssetRefService(knowledgeBaseAssetRefRepo, reuseEventRepo)
	databaseRefService := service.NewDatabaseAssetRefService(databaseAssetRefRepo, reuseEventRepo)

	return &Module{
		DocumentAssetRepo:          documentAssetRepo,
		ReuseEventRepo:             reuseEventRepo,
		ProcessingRequestRepo:      processingRequestRepo,
		ParseConfirmationItemRepo:  parseConfirmationItemRepo,
		VectorArtifactRepo:         vectorArtifactRepo,
		ExtractionArtifactRepo:     extractionArtifactRepo,
		KnowledgeBaseAssetRefRepo:  knowledgeBaseAssetRefRepo,
		DatabaseAssetRefRepo:       databaseAssetRefRepo,
		DocumentAssetService:       documentAssetService,
		ProcessingRequestService:   processingRequestService,
		ProcessingExecutorRegistry: processingExecutorRegistry,
		VectorArtifactService:      vectorArtifactService,
		ExtractionArtifactService:  extractionArtifactService,
		FileAssetSyncService:       fileAssetSyncService,
		KnowledgeBaseRefService:    knowledgeBaseRefService,
		DatabaseRefService:         databaseRefService,
		DocumentAssetHandler:       handler.NewDocumentAssetHandler(documentAssetService, fileAssetSyncService, processingRequestService, knowledgeBaseRefService, databaseRefService),
		VectorArtifactHandler:      handler.NewVectorArtifactHandler(vectorArtifactService, documentAssetService),
		ExtractionArtifactHandler:  handler.NewExtractionArtifactHandler(extractionArtifactService, documentAssetService),
		ProcessingExecutorHandler:  handler.NewProcessingExecutorHandler(processingExecutorRegistry, processingRequestService),
	}
}
