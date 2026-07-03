package indexing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/config"
	chunkdataset "github.com/zgiai/zgi/api/internal/capabilities/chunking/adapters/dataset"
	chunkexecutor "github.com/zgiai/zgi/api/internal/capabilities/chunking/executor"
	chunkquality "github.com/zgiai/zgi/api/internal/capabilities/chunking/quality"
	contentparsecap "github.com/zgiai/zgi/api/internal/capabilities/contentparse"
	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/routing"
	"github.com/zgiai/zgi/api/internal/contracts"
	"github.com/zgiai/zgi/api/internal/dto"
	contentparsesvc "github.com/zgiai/zgi/api/internal/modules/contentparse/service"
	graphflow_model "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	graphflow_repo "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/repository"
	graphflow_worker "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/worker"
	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
	dataset_repository "github.com/zgiai/zgi/api/internal/modules/dataset/repository"
	"github.com/zgiai/zgi/api/internal/modules/dataset/retrieval"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	llmruntime "github.com/zgiai/zgi/api/internal/modules/llm/runtime"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	shared_model "github.com/zgiai/zgi/api/internal/modules/shared/model"
	"github.com/zgiai/zgi/api/internal/prompt"
	"github.com/zgiai/zgi/api/pkg/embedding"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
	"github.com/zgiai/zgi/api/pkg/storage"

	"github.com/zgiai/zgi/api/pkg/vectordb"
)

const (
	defaultContentParseShadowConcurrency = 2
)

var contentParseShadowSlots = make(chan struct{}, contentParseShadowConcurrencyLimit())

// IndexingRunner handles document indexing and embedding operations.
type IndexingRunner struct {
	storage                   storage.Storage
	fileService               interfaces.FileService
	documentRepo              dataset_repository.DocumentRepository
	datasetRepo               dataset_repository.DatasetRepository
	embeddingService          retrieval.Embedding
	vectorDB                  vectordb.VectorDB
	defaultModelSvc           llmdefaultservice.DefaultModelService
	llmClient                 llmclient.LLMClient
	graphFlowTaskRepo         *graphflow_repo.GraphFlowTaskRepository
	taskManager               *queue.TaskManager
	contentParseSvc           contracts.ContentParseService
	contentParseOrchestrator  *contentparsecap.Orchestrator
	contentParsePlanner       routing.Planner
	contentParseChunkMapper   contracts.ChunkSourceMapper
	contentParseChunkPlanner  contracts.ChunkPlanner
	contentParseCatalog       *contracts.ParseProviderCatalog
	contentParseRunService    contentparsesvc.RunQueryService
	contentParseArtifactSvc   contentparsesvc.ArtifactService
	contentParseChunkArtifact contentparsesvc.ChunkArtifactSetService
	contentParseShadowRunner  contentparsesvc.ShadowPipelineRunner
	contentParseShadowEnabled bool
}

// NewIndexingRunner creates a new IndexingRunner.
// The llmClient should be obtained from the DI container (ServiceContainer.GetLLMClient()).
func NewIndexingRunner(
	storage storage.Storage,
	documentRepo dataset_repository.DocumentRepository,
	datasetRepo dataset_repository.DatasetRepository,
	fileService interfaces.FileService,
	embeddingService retrieval.Embedding,
	vectorDB vectordb.VectorDB,
	defaultModelSvc llmdefaultservice.DefaultModelService,
	llmClient llmclient.LLMClient,
	graphFlowTaskRepo *graphflow_repo.GraphFlowTaskRepository,
	taskManager *queue.TaskManager,
) *IndexingRunner {
	return &IndexingRunner{
		storage:           storage,
		documentRepo:      documentRepo,
		datasetRepo:       datasetRepo,
		fileService:       fileService,
		embeddingService:  embeddingService,
		vectorDB:          vectorDB,
		defaultModelSvc:   defaultModelSvc,
		llmClient:         llmClient,
		graphFlowTaskRepo: graphFlowTaskRepo,
		taskManager:       taskManager,
	}
}

func (ir *IndexingRunner) SetContentParseShadow(
	service contracts.ContentParseService,
	orchestrator *contentparsecap.Orchestrator,
	planner routing.Planner,
	chunkMapper contracts.ChunkSourceMapper,
	chunkPlanner contracts.ChunkPlanner,
	catalog *contracts.ParseProviderCatalog,
	runService contentparsesvc.RunQueryService,
	artifactService contentparsesvc.ArtifactService,
	chunkArtifactService contentparsesvc.ChunkArtifactSetService,
	enabled bool,
) {
	ir.contentParseSvc = service
	ir.contentParseOrchestrator = orchestrator
	ir.contentParsePlanner = planner
	ir.contentParseChunkMapper = chunkMapper
	ir.contentParseChunkPlanner = chunkPlanner
	ir.contentParseCatalog = catalog
	ir.contentParseRunService = runService
	ir.contentParseArtifactSvc = artifactService
	ir.contentParseChunkArtifact = chunkArtifactService
	ir.contentParseShadowRunner = ir
	ir.contentParseShadowEnabled = enabled
}

// IndexingEstimate
type IndexingEstimate struct {
	TotalSegments int                   `json:"total_segments"`
	Preview       []dto.PreviewDetail   `json:"preview"`
	QAPreview     []dto.QAPreviewDetail `json:"qa_preview,omitempty"`
}

// ExtractSetting
type UploadFile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	FilePath string `json:"file_path"`
}

type ExtractSetting struct {
	DataSourceType     string                    `json:"datasource_type"`
	UploadFile         *UploadFile               `json:"upload_file,omitempty"`
	DocumentModel      string                    `json:"document_model"`
	NotionInfo         map[string]interface{}    `json:"notion_info,omitempty"`
	WebsiteInfo        map[string]interface{}    `json:"website_info,omitempty"`
	ProcessRule        *model.DatasetProcessRule `json:"process_rule,omitempty"`
	Content            string                    `json:"content,omitempty"`
	ExtractionStrategy string                    `json:"extraction_strategy,omitempty"` // landingai|local|mineru
	// ExtractionFallbackEnabled controls whether extraction may retry with other parsers.
	ExtractionFallbackEnabled *bool `json:"extraction_fallback_enabled,omitempty"`
}

// IndexingEstimateRequest
type IndexingEstimateRequest struct {
	TenantID          string                    `json:"tenant_id"`
	ExtractSettings   []ExtractSetting          `json:"extract_settings"`
	ProcessRule       *model.DatasetProcessRule `json:"process_rule,omitempty"`
	TmpProcessingRule map[string]interface{}    `json:"tmp_processing_rule"`
	DocForm           string                    `json:"doc_form"`
	DocLanguage       string                    `json:"doc_language"`
	DatasetID         *string                   `json:"dataset_id,omitempty"`
	IndexingTechnique string                    `json:"indexing_technique"`
}

// Estimate
// IndexingRunner.indexing_estimate
func (ir *IndexingRunner) Estimate(ctx context.Context, req *IndexingEstimateRequest) (*IndexingEstimate, error) {
	// TODO: check document limit
	// features := FeatureService.get_features(tenant_id)
	// if features.billing.enabled:
	//     count := len(extract_settings)
	//     if count exceeds the configured upload limit:
	//         raise an error before starting the batch job

	// TODO: embeddingModelInstance
	// var embeddingModelInstance interface{} // ModelInstance
	// if req.DatasetID != nil && *req.DatasetID != "" {
	// TODO: get dataset embedding_model_instance
	// dataset = Dataset.query.filter_by(id=dataset_id).first()
	// if not dataset:
	//     raise ValueError("Dataset not found.")
	// if dataset.indexing_technique == "high_quality" or indexing_technique == "high_quality":
	//     if dataset.embedding_model_provider:
	//         embedding_model_instance = self.model_manager.get_model_instance(
	//             tenant_id=tenant_id,
	//             provider=dataset.embedding_model_provider,
	//             model_type=ModelType.TEXT_EMBEDDING,
	//             model=dataset.embedding_model,
	//         )
	//     else:
	//         embedding_model_instance = self.model_manager.get_default_model_instance(
	//             tenant_id=tenant_id,
	//             model_type=ModelType.TEXT_EMBEDDING,
	//         )
	// } else {
	// 	if req.IndexingTechnique == "high_quality" {
	// TODO: default_embedding_model_instance
	// embedding_model_instance = self.model_manager.get_default_model_instance(
	//     tenant_id=tenant_id,
	//     model_type=ModelType.TEXT_EMBEDDING,
	// )
	// 	}
	// }

	// TODO:
	// var embeddingModelInstance interface{} // ModelInstance

	if req.DatasetID != nil && *req.DatasetID != "" {
		// TODO:
		// dataset = Dataset.query.filter_by(id=dataset_id).first()
		// if not dataset:
		//     raise ValueError("Dataset not found.")
		// if dataset.indexing_technique == "high_quality" or indexing_technique == "high_quality":
		//     if dataset.embedding_model_provider:
		//         embedding_model_instance = self.model_manager.get_model_instance(
		//             tenant_id=tenant_id,
		//             provider=dataset.embedding_model_provider,
		//             model_type=ModelType.TEXT_EMBEDDING,
		//             model=dataset.embedding_model,
		//         )
		//     else:
		//         embedding_model_instance = self.model_manager.get_default_model_instance(
		//             tenant_id=tenant_id,
		//             model_type=ModelType.TEXT_EMBEDDING,
		//         )
	} else {
		if req.IndexingTechnique == "high_quality" {
			// TODO:
			// embedding_model_instance = self.model_manager.get_default_model_instance(
			//     tenant_id=tenant_id,
			//     model_type=ModelType.TEXT_EMBEDDING,
			// )
		}
	}

	previewTexts := make([]interface{}, 0)
	qaPreviewTexts := make([]dto.QAPreviewDetail, 0)

	totalSegments := 0
	indexType := req.DocForm

	factory := NewIndexProcessorFactory(IndexType(indexType), ir.storage, ir.documentRepo, ir.defaultModelSvc, ir.llmClient, req.TenantID)
	indexProcessor, err := factory.CreateIndexProcessor()
	if err != nil {
		return nil, fmt.Errorf("failed to create index processor: %w", err)
	}

	for _, extractSetting := range req.ExtractSettings {
		var mode string
		var rules map[string]interface{}

		// Use process rule from extractSetting if available, otherwise fallback to request-level rules
		if extractSetting.ProcessRule != nil {
			mode = extractSetting.ProcessRule.Mode
			rules = extractSetting.ProcessRule.Rules
		} else if req.ProcessRule != nil {
			mode = req.ProcessRule.Mode
			rules = req.ProcessRule.Rules
		} else {
			mode, _ = req.TmpProcessingRule["mode"].(string)
			if req.TmpProcessingRule["rules"] != nil {
				rules, _ = req.TmpProcessingRule["rules"].(map[string]interface{})
			}
		}

		rulesJSON, _ := json.Marshal(rules)

		_ = map[string]interface{}{
			"mode":  mode,
			"rules": string(rulesJSON),
		}

		// Create a copy of extractSetting and set the ProcessRule correctly
		settingWithProcessRule := extractSetting
		if extractSetting.ProcessRule == nil && req.ProcessRule != nil {
			settingWithProcessRule.ProcessRule = req.ProcessRule
		} else if extractSetting.ProcessRule == nil && req.ProcessRule == nil {
			// Create a temporary process rule from TmpProcessingRule if needed
			tmpProcessRule := &model.DatasetProcessRule{
				Mode:  mode,
				Rules: rules,
			}
			settingWithProcessRule.ProcessRule = tmpProcessRule
		}

		extractOutput, err := indexProcessor.Extract(ctx, &settingWithProcessRule, &ProcessOptions{
			Mode:        mode,
			ProcessRule: rules,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to extract documents: %w", err)
		}
		transformedChunks, err := indexProcessor.Transform(ctx, extractOutput, &ProcessOptions{
			Mode:        mode,
			ProcessRule: rules,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to transform documents: %w", err)
		}
		transformedChunks = cleanDatasetTransformedChunks(transformedChunks)

		totalSegments += len(transformedChunks)

		for _, chunk := range transformedChunks {
			// TODO: may use other preview text limit
			// if len(previewTexts) >= 10 {
			// 	break
			// }

			if req.DocForm == "qa_model" {
				// QA
				answer := ""
				if metadata, ok := chunk.Metadata["answer"]; ok {
					if str, ok := metadata.(string); ok {
						answer = str
					}
				}

				qaPreviewDetail := dto.QAPreviewDetail{
					Question: chunk.Content,
					Answer:   answer,
				}
				qaPreviewTexts = append(qaPreviewTexts, qaPreviewDetail)
			} else {
				previewDetail := dto.PreviewDetail{
					Content: chunk.Content,
				}

				if chunk.Children != nil {
					childChunks := make([]string, len(chunk.Children))
					for i, child := range chunk.Children {
						childChunks[i] = child.Content
					}
					previewDetail.ChildChunks = childChunks
				}

				previewTexts = append(previewTexts, previewDetail)
			}

			// TODO: delete image files
			// image_upload_file_ids = get_image_upload_file_ids(document.page_content)
			// for upload_file_id in image_upload_file_ids:
			//     image_file = db.session.query(UploadFile).filter(UploadFile.id == upload_file_id).first()
			//     try:
			//         if image_file:
			//             storage.delete(image_file.key)
			//     except Exception:
			//         logging.exception(
			//             "Delete image_files failed while indexing_estimate, \
			//                           image_upload_file_is: {}".format(upload_file_id)
			//         )
			//     db.session.delete(image_file)
		}
	}

	if req.DocForm == "qa_model" {
		return &IndexingEstimate{
			TotalSegments: totalSegments * 20,
			QAPreview:     qaPreviewTexts,
			Preview:       []dto.PreviewDetail{},
		}, nil
	}

	previewDetails := make([]dto.PreviewDetail, 0, len(previewTexts))
	for _, preview := range previewTexts {
		if detail, ok := preview.(dto.PreviewDetail); ok {
			previewDetails = append(previewDetails, detail)
		} else {
			continue
		}
	}

	return &IndexingEstimate{
		TotalSegments: totalSegments,
		Preview:       previewDetails,
		QAPreview:     []dto.QAPreviewDetail{},
	}, nil
}

// Run executes the document indexing process
func (ir *IndexingRunner) Run(ctx context.Context, datasetDocument *model.Document) error {
	// KEY: Configurable indexing timeout to prevent stuck documents
	indexingTimeoutMinutes := 60 // default 1 hour
	if config.GlobalConfig != nil && config.GlobalConfig.VectorStore.IndexingTimeout > 0 {
		indexingTimeoutMinutes = config.GlobalConfig.VectorStore.IndexingTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(indexingTimeoutMinutes)*time.Minute)
	defer cancel()

	// KEY: Deferred guard ensures document status is always updated on failure.
	// Uses context.Background() because the original ctx may have been cancelled
	// by the timeout, which would cause DB updates to silently fail.
	var runErr error
	defer func() {
		if runErr != nil {
			errCtx, errCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer errCancel()

			stopTime := time.Now()
			if updateErr := ir.documentRepo.UpdateDocumentError(errCtx, datasetDocument.ID, runErr.Error(), &stopTime); updateErr != nil {
				logger.Error("Failed to update document error status in deferred guard", updateErr)
			} else {
				logger.Info("Document error status updated via deferred guard", map[string]interface{}{
					"document_id": datasetDocument.ID,
					"error":       runErr.Error(),
				})
			}
		}
	}()

	// Update document status to indicate indexing has started
	if err := ir.documentRepo.UpdateDocumentIndexingStatus(ctx, datasetDocument.ID, model.DocumentStatusParsing); err != nil {
		logger.Error("Failed to update document indexing status", err)
		runErr = fmt.Errorf("failed to update document status to parsing: %w", err)
		return runErr
	}

	// Set processing started time
	startTime := time.Now()
	if err := ir.documentRepo.UpdateDocumentProcessingStarted(ctx, datasetDocument.ID, &startTime); err != nil {
		logger.Error("Failed to update document processing started time", err)
		// Non-critical, continue execution
	}

	// Get document data source info
	dataSourceInfo := make(map[string]interface{})
	if datasetDocument.DataSourceInfo != nil {
		if err := json.Unmarshal([]byte(*datasetDocument.DataSourceInfo), &dataSourceInfo); err != nil {
			runErr = fmt.Errorf("failed to parse data source info: %w", err)
			return runErr
		}
	}

	// Create extract setting based on data source type
	var (
		extractSetting     *ExtractSetting
		uploadFileExtOrKey string
	)
	switch datasetDocument.DataSourceType {
	case "upload_file":
		uploadFileID, ok := dataSourceInfo["upload_file_id"].(string)
		if !ok {
			runErr = fmt.Errorf("upload_file_id not found in data source info")
			return runErr
		}

		// get dtoFile FileService
		dtoFile, err := ir.fileService.GetFileByID(ctx, uploadFileID)
		if err != nil {
			runErr = fmt.Errorf("failed to get file info for %s: %w", uploadFileID, err)
			return runErr
		}
		uploadFileExtOrKey = dtoFile.Extension
		if strings.TrimSpace(uploadFileExtOrKey) == "" {
			uploadFileExtOrKey = dtoFile.Name
		}
		// Create a minimal extract setting for processing
		extractSetting = &ExtractSetting{
			DataSourceType: "upload_file",
			UploadFile: &UploadFile{
				ID:       dtoFile.ID,
				Name:     dtoFile.Name,
				Size:     dtoFile.Size,
				FilePath: dtoFile.Key,
			},
			DocumentModel: datasetDocument.DocForm,
		}
	case "reading":
		content, ok := dataSourceInfo["content"].(string)
		if !ok {
			runErr = fmt.Errorf("content not found in data source info for reading type")
			return runErr
		}
		extractSetting = &ExtractSetting{
			DataSourceType: "reading",
			Content:        content,
			DocumentModel:  datasetDocument.DocForm,
		}
	case "text_input":
		content, ok := dataSourceInfo["text"].(string)
		if !ok {
			runErr = fmt.Errorf("text not found in data source info for text_input type")
			return runErr
		}
		extractSetting = &ExtractSetting{
			DataSourceType: "text_input",
			Content:        content,
			DocumentModel:  datasetDocument.DocForm,
		}
	default:
		runErr = fmt.Errorf("[DEBUG RUNNER] unsupported data source type: '%s'", datasetDocument.DataSourceType)
		return runErr
	}

	// Get process rule
	var processRule *model.DatasetProcessRule
	if datasetDocument.DatasetProcessRuleID == nil || strings.TrimSpace(*datasetDocument.DatasetProcessRuleID) == "" {
		runErr = fmt.Errorf("document process rule id is required")
		return runErr
	}

	rule, err := ir.documentRepo.GetProcessRuleByID(ctx, *datasetDocument.DatasetProcessRuleID)
	if err != nil {
		runErr = fmt.Errorf("failed to get document process rule %s: %w", *datasetDocument.DatasetProcessRuleID, err)
		return runErr
	}
	processRule = rule

	if processRule == nil {
		runErr = fmt.Errorf("document process rule %s not found", *datasetDocument.DatasetProcessRuleID)
		return runErr
	}

	extractSetting.ProcessRule = processRule
	if processRule.Rules == nil {
		runErr = fmt.Errorf("document process rule %s has empty rules", processRule.ID)
		return runErr
	}
	strategy, ok := processRule.Rules["user_choose_extraction_strategy"].(string)
	if !ok || strings.TrimSpace(strategy) == "" {
		runErr = fmt.Errorf("document process rule %s missing user_choose_extraction_strategy", processRule.ID)
		return runErr
	}
	extractSetting.ExtractionStrategy = strings.ToLower(strings.TrimSpace(strategy))
	if enabled, ok := processRule.Rules["extraction_fallback_enabled"].(bool); ok {
		extractSetting.ExtractionFallbackEnabled = &enabled
	}

	selectedDocForm := datasetDocument.DocForm
	selectedProcessRule := processRule

	// Use a single shared extraction pass before runtime routing.
	baseProcessor := NewBaseIndexProcessorImpl(ir.storage, ir.defaultModelSvc, ir.llmClient, datasetDocument.OrganizationID)
	extractOutput, err := baseProcessor.Extract(ctx, extractSetting, &ProcessOptions{
		Mode:        selectedProcessRule.Mode,
		ProcessRule: selectedProcessRule.Rules,
	})
	if err != nil {
		runErr = fmt.Errorf("failed to extract document: %w", err)
		return runErr
	}
	if extractOutput == nil || len(extractOutput.Elements) == 0 {
		runErr = fmt.Errorf("no documents found")
		return runErr
	}

	// Validate extracted content and filter out empty elements
	filteredElements := make([]dto.ExtractElement, 0, len(extractOutput.Elements))
	for i, element := range extractOutput.Elements {
		if strings.TrimSpace(element.Content) != "" {
			filteredElements = append(filteredElements, element)
		} else {
			logger.Warn(fmt.Sprintf("Extracted element %d has empty content, skipping", i), map[string]interface{}{
				"doc_id":   datasetDocument.ID,
				"metadata": element.Metadata,
			})
		}
	}
	extractOutput.Elements = filteredElements

	if len(extractOutput.Elements) == 0 {
		runErr = fmt.Errorf("extracted content is empty (possibly due to empty file, image-only PDF without OCR, or extraction failure)")
		return runErr
	}

	// Calculate word count and update document status to splitting with word_count and parsing_completed_at
	wordCount := len([]rune(dto.ExtractOutputText(extractOutput)))

	// Update document status to splitting
	if err := ir.documentRepo.UpdateDocumentIndexingStatus(ctx, datasetDocument.ID, model.DocumentStatusSplitting); err != nil {
		logger.Error("Failed to update document indexing status to splitting", err)
		runErr = fmt.Errorf("failed to update document status to splitting: %w", err)
		return runErr
	}

	parsingCompletedAt := time.Now()
	if err := ir.documentRepo.UpdateDocumentParsingCompleted(ctx, datasetDocument.ID, &parsingCompletedAt); err != nil {
		logger.Error("Failed to update document parsing completed time", err)
	}

	// Update document with word count
	if err := ir.documentRepo.UpdateDocumentWordCount(ctx, datasetDocument.ID, wordCount); err != nil {
		logger.Error("Failed to update document word count", err)
	}

	if extractionMeta, ok := extractOutput.Metadata["extraction"].(map[string]interface{}); ok {
		if err := ir.documentRepo.UpdateDocumentExtractionMetadata(ctx, datasetDocument.ID, extractionMeta); err != nil {
			logger.Warn("Failed to update document extraction metadata", map[string]interface{}{
				"document_id": datasetDocument.ID,
				"error":       err.Error(),
			})
		} else {
			if datasetDocument.DocMetadata == nil {
				datasetDocument.DocMetadata = model.JSONMap{}
			}
			datasetDocument.DocMetadata["extraction"] = extractionMeta
		}
	}

	if len(extractOutput.Elements) > 0 {
		for i := range extractOutput.Elements {
			if extractOutput.Elements[i].Metadata == nil {
				extractOutput.Elements[i].Metadata = make(map[string]interface{})
			}
			if _, ok := extractOutput.Elements[i].Metadata["document_id"]; !ok {
				extractOutput.Elements[i].Metadata["document_id"] = datasetDocument.ID
			}
			if _, ok := extractOutput.Elements[i].Metadata["dataset_id"]; !ok {
				extractOutput.Elements[i].Metadata["dataset_id"] = datasetDocument.DatasetID
			}
		}
	}

	if datasetDocument.DataSourceType == "upload_file" {
		router := NewRuntimeRouter(ctx, ir.llmClient, ir.defaultModelSvc, datasetDocument.OrganizationID)
		decision, routeErr := router.Route(RouterInput{
			DocumentID:      datasetDocument.ID,
			DatasetID:       datasetDocument.DatasetID,
			DataSourceType:  datasetDocument.DataSourceType,
			DocExt:          uploadFileExtOrKey,
			ExtractedOutput: extractOutput,
		})
		if routeErr != nil {
			logger.Warn("Runtime routing failed, using legacy path", map[string]interface{}{
				"document_id": datasetDocument.ID,
				"error":       routeErr.Error(),
			})
		} else if decision != nil && decision.Matched {
			routingMeta := map[string]interface{}{
				"version":           "v1",
				"matched":           true,
				"route_name":        decision.RouteName,
				"original_doc_form": datasetDocument.DocForm,
				"target_doc_form":   decision.TargetDocForm,
				"reason":            decision.Reason,
			}
			for key, value := range decision.RouteMeta {
				routingMeta[key] = value
			}

			routedRule := *selectedProcessRule
			routedRule.Mode = decision.TargetMode
			routedRule.Rules = mergeProcessRuleRules(selectedProcessRule.Rules, decision.TargetRules)

			if err := ir.documentRepo.ApplyDocumentRouting(ctx, datasetDocument.ID, decision.TargetDocForm, &routedRule, routingMeta); err != nil {
				logger.Warn("Failed to persist runtime routing, falling back to legacy path", map[string]interface{}{
					"document_id":     datasetDocument.ID,
					"route_name":      decision.RouteName,
					"target_doc_form": decision.TargetDocForm,
					"error":           err.Error(),
				})
			} else {
				selectedDocForm = decision.TargetDocForm
				selectedProcessRule = &routedRule
				datasetDocument.DocForm = decision.TargetDocForm
				if datasetDocument.DocMetadata == nil {
					datasetDocument.DocMetadata = model.JSONMap{}
				}
				datasetDocument.DocMetadata["routing"] = routingMeta

				logger.Info("Document runtime route matched", map[string]interface{}{
					"document_id":       datasetDocument.ID,
					"route_name":        decision.RouteName,
					"original_doc_form": routingMeta["original_doc_form"],
					"target_doc_form":   decision.TargetDocForm,
					"reason":            decision.Reason,
				})
			}
		} else if decision != nil {
			logger.Info("Document runtime route not matched", map[string]interface{}{
				"document_id": datasetDocument.ID,
				"doc_ext":     normalizeDocExt(uploadFileExtOrKey),
				"reason":      decision.Reason,
			})
		}
	}

	// Create the final index processor after runtime routing has had a chance to correct DocForm.
	factory := NewIndexProcessorFactory(IndexType(selectedDocForm), ir.storage, ir.documentRepo, ir.defaultModelSvc, ir.llmClient, datasetDocument.OrganizationID)
	indexProcessor, err := factory.CreateIndexProcessor()
	if err != nil {
		runErr = fmt.Errorf("failed to create index processor: %w", err)
		return runErr
	}

	// Transform extracted output into chunks.
	transformedChunks, err := indexProcessor.Transform(ctx, extractOutput, &ProcessOptions{
		Mode:        selectedProcessRule.Mode,
		ProcessRule: selectedProcessRule.Rules,
	})
	if err != nil {
		runErr = fmt.Errorf("failed to transform documents: %w", err)
		return runErr
	}
	rawChunkCount := len(transformedChunks)
	transformedChunks = cleanDatasetTransformedChunks(transformedChunks)
	if len(transformedChunks) != rawChunkCount {
		logger.Info("Filtered low-value dataset chunks before indexing", map[string]interface{}{
			"document_id": datasetDocument.ID,
			"dataset_id":  datasetDocument.DatasetID,
			"before":      rawChunkCount,
			"after":       len(transformedChunks),
		})
	}
	if len(transformedChunks) == 0 {
		runErr = fmt.Errorf("no segments generated after transformation")
		return runErr
	}

	ir.startContentParseShadow(datasetDocument, extractSetting, extractOutput, transformedChunks)

	// Load segments - this is where we update cleaning_completed_at and splitting_completed_at
	savedSegmentsCount, err := ir.loadSegments(ctx, datasetDocument, transformedChunks)
	if err != nil {
		runErr = fmt.Errorf("failed to load segments: %w", err)
		return runErr
	}

	// Process segments for vectorization if high quality indexing is enabled
	dataset, err := ir.datasetRepo.GetByID(ctx, datasetDocument.DatasetID)
	if err != nil {
		logger.Error("Failed to get dataset", err)
		runErr = fmt.Errorf("failed to get dataset: %w", err)
		return runErr
	}

	if err := ir.load(ctx, indexProcessor, dataset, datasetDocument, transformedChunks); err != nil {
		runErr = fmt.Errorf("failed to process segments for vectorization: %w", err)
		return runErr
	}

	// Check if we need to generate recommend questions
	if selectedProcessRule != nil && ir.shouldGenerateRecommendQuestions(selectedProcessRule.Rules) {
		// Generate recommend questions for each segment
		if err := ir.generateRecommendQuestions(ctx, datasetDocument, dataset.WorkspaceID, datasetDocument.CreatedBy); err != nil {
			logger.Error("Failed to generate recommend questions", err)
			// Continue execution as this is not critical
		}
	}

	// Update document status to completed

	logger.Info("Document indexing completed", map[string]interface{}{
		"document_id":   datasetDocument.ID,
		"segment_count": len(transformedChunks),
	})

	// Trigger GraphFlow task if enabled
	if dataset.EnableGraphFlow {
		logger.Info("GraphFlow enabled for dataset, creating extraction task", map[string]interface{}{
			"dataset_id":  dataset.ID,
			"document_id": datasetDocument.ID,
		})

		// safe UUID conversion

		// safe UUID conversion
		kbid, _ := uuid.Parse(dataset.ID)
		docID, _ := uuid.Parse(datasetDocument.ID)
		tenantID, _ := uuid.Parse(dataset.OrganizationID)

		// Determine extraction strategy from process rules
		strategy := "llm"
		if selectedProcessRule != nil && selectedProcessRule.Rules != nil {
			if s, ok := selectedProcessRule.Rules["extraction_strategy"].(string); ok && s != "" {
				strategy = s
			}
		}

		// Create GraphFlow task
		graphFlowTask := &graphflow_model.GraphFlowTask{
			TenantID:           tenantID,
			KBID:               kbid,
			DocumentID:         docID,
			TaskType:           "extraction",
			Status:             "pending",
			ExtractionStrategy: strategy,
		}

		if err := ir.graphFlowTaskRepo.CreateTask(ctx, graphFlowTask); err != nil {
			logger.Error("Failed to create GraphFlow task", err)
			runErr = fmt.Errorf("failed to create GraphFlow task: %w", err)
			return runErr
		} else {
			logger.Info("GraphFlow extraction task created successfully", map[string]interface{}{
				"task_id":  graphFlowTask.ID.String(),
				"strategy": strategy,
			})

			// Enqueue task for async processing using Asynq
			// Fix race condition: Pass expected segment count
			expectedSegments := savedSegmentsCount
			asynqTask, err := graphflow_worker.CreateGraphFlowExtractionTask(graphFlowTask.ID.String(), ir.taskManager, expectedSegments)
			if err != nil {
				logger.Error("Failed to create GraphFlow asynq task", err)
				runErr = fmt.Errorf("failed to create GraphFlow asynq task: %w", err)
				return runErr
			} else {
				_, enqueueErr := ir.taskManager.EnqueueTask(asynqTask, asynq.Queue("graphflow"))
				if enqueueErr != nil {
					logger.Error("Failed to enqueue GraphFlow task to Asynq", enqueueErr)
					runErr = fmt.Errorf("failed to enqueue GraphFlow task: %w", enqueueErr)
					return runErr
				}
			}
		}
	}

	return nil
}

func (ir *IndexingRunner) startContentParseShadow(datasetDocument *model.Document, extractSetting *ExtractSetting, primaryOutput *dto.ExtractOutput, legacyChunks []dto.TransformedChunk) {
	if ir == nil || !ir.contentParseShadowEnabled || ir.contentParseShadowRunner == nil {
		return
	}
	if datasetDocument == nil || datasetDocument.FileID == nil || strings.TrimSpace(*datasetDocument.FileID) == "" {
		return
	}
	if extractSetting == nil || extractSetting.DataSourceType != "upload_file" {
		return
	}
	if ir.fileService == nil || ir.documentRepo == nil {
		return
	}
	primarySnapshot := cloneContentParseShadowExtractOutput(primaryOutput)
	legacyChunkSnapshot := cloneContentParseShadowTransformedChunks(legacyChunks)
	ir.contentParseShadowRunner.EnqueueDatasetIndexingShadow(context.Background(), contentparsesvc.DatasetShadowInput{
		DocumentID:     datasetDocument.ID,
		DatasetID:      datasetDocument.DatasetID,
		OrganizationID: datasetDocument.OrganizationID,
		FileID:         *datasetDocument.FileID,
		FileName:       datasetDocument.Name,
		EngineHint:     toContentParseEngineHint(extractSetting.ExtractionStrategy),
		PrimaryOutput:  primarySnapshot,
		LegacyChunks:   legacyChunkSnapshot,
		InitialSummary: map[string]interface{}{
			"enabled":                 true,
			"captured_at":             time.Now().Unix(),
			"legacy_extract_baseline": summarizePrimaryExtractOutput(primarySnapshot),
			"legacy_chunk_baseline":   summarizeLegacyTransformedChunks(legacyChunkSnapshot),
		},
		RecognitionSource: "dataset_indexing_shadow",
		Source:            "dataset_indexing",
	})
}

func cloneContentParseShadowExtractOutput(input *dto.ExtractOutput) *dto.ExtractOutput {
	if input == nil {
		return nil
	}
	out := &dto.ExtractOutput{
		Markdown: input.Markdown,
		Source:   input.Source,
		Metadata: cloneContentParseShadowMetadata(input.Metadata),
	}
	if len(input.Elements) > 0 {
		out.Elements = make([]dto.ExtractElement, len(input.Elements))
		for i, element := range input.Elements {
			out.Elements[i] = element
			out.Elements[i].Metadata = cloneContentParseShadowMetadata(element.Metadata)
			if element.BBox != nil {
				box := *element.BBox
				out.Elements[i].BBox = &box
			}
		}
	}
	return out
}

func cloneContentParseShadowTransformedChunks(input []dto.TransformedChunk) []dto.TransformedChunk {
	if len(input) == 0 {
		return nil
	}
	out := make([]dto.TransformedChunk, len(input))
	for i, chunk := range input {
		out[i] = chunk
		out[i].Metadata = cloneContentParseShadowMetadata(chunk.Metadata)
		if chunk.BBox != nil {
			box := *chunk.BBox
			out[i].BBox = &box
		}
		if len(chunk.Children) > 0 {
			out[i].Children = make([]dto.TransformedChildChunk, len(chunk.Children))
			for j, child := range chunk.Children {
				out[i].Children[j] = child
				out[i].Children[j].Metadata = cloneContentParseShadowMetadata(child.Metadata)
				if child.BBox != nil {
					box := *child.BBox
					out[i].Children[j].BBox = &box
				}
			}
		}
	}
	return out
}

func cloneContentParseShadowMetadata(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func (ir *IndexingRunner) runContentParseShadow(ctx context.Context, req contracts.ParseRequest, routePlan *routing.RoutePlan) (*contracts.ParseArtifact, error) {
	if ir == nil || ir.contentParseSvc == nil {
		return nil, fmt.Errorf("content parse shadow service is not configured")
	}
	if routePlan != nil && routePlan.Primary != nil && ir.contentParseOrchestrator != nil {
		candidates := make([]routing.RouteCandidate, 0, len(routePlan.FallbackCandidates)+1)
		candidates = append(candidates, *routePlan.Primary)
		candidates = append(candidates, routePlan.FallbackCandidates...)
		var lastErr error
		attemptedProviders := make([]string, 0, len(candidates))
		attemptedAdapters := make([]string, 0, len(candidates))
		for index, candidate := range candidates {
			if strings.TrimSpace(candidate.AdapterName) == "" {
				continue
			}
			if strings.TrimSpace(candidate.ProviderKey) != "" {
				attemptedProviders = append(attemptedProviders, candidate.ProviderKey)
			}
			attemptedAdapters = append(attemptedAdapters, candidate.AdapterName)
			attemptReq := req
			attemptReq.EngineHint = candidate.EngineName
			artifact, err := ir.contentParseOrchestrator.ParseWithAdapter(ctx, candidate.AdapterName, attemptReq)
			if err != nil {
				lastErr = err
				continue
			}
			if artifact != nil {
				contentparsesvc.ApplyRouteExecutionMetadata(artifact, candidate, attemptedProviders, attemptedAdapters, index > 0)
			}
			return artifact, nil
		}
		if lastErr != nil {
			return nil, fmt.Errorf("content parse shadow route failed: %w", lastErr)
		}
		return nil, fmt.Errorf("content parse shadow route has no executable provider")
	}
	return ir.contentParseSvc.Parse(ctx, req)
}

func contentParseShadowConcurrencyLimit() int {
	return contentparsesvc.ShadowPipelineOptionsFromEnv().Concurrency
}

func contentParseChunkShadowWorkers() int {
	return contentparsesvc.ShadowPipelineOptionsFromEnv().ChunkWorkers
}

func contentParseChunkShadowPartitionSize() int {
	return contentparsesvc.ShadowPipelineOptionsFromEnv().ChunkPartitionSize
}

func tryAcquireContentParseShadowSlot() bool {
	select {
	case contentParseShadowSlots <- struct{}{}:
		return true
	default:
		return false
	}
}

func releaseContentParseShadowSlot() {
	select {
	case <-contentParseShadowSlots:
	default:
	}
}

func (ir *IndexingRunner) persistContentParseRunShadow(
	ctx context.Context,
	documentID string,
	datasetID string,
	workspaceID string,
	organizationID string,
	fileID string,
	req contracts.ParseRequest,
	routePlan *routing.RoutePlan,
	artifact *contracts.ParseArtifact,
	summary map[string]interface{},
) (*uuid.UUID, *uuid.UUID) {
	if ir == nil || ir.contentParseRunService == nil {
		return nil, nil
	}

	var artifactID *uuid.UUID
	if artifact != nil && ir.contentParseArtifactSvc != nil {
		build := contentparsesvc.BuildParseArtifactItem(contentparsesvc.ParseArtifactBuildInput{
			Request:   req,
			RoutePlan: routePlan,
			Artifact:  artifact,
			Summary:   summary,
		})
		if err := ir.contentParseArtifactSvc.Upsert(ctx, build.Item); err != nil {
			summary["artifact_persist_error"] = err.Error()
		} else if persisted, err := ir.contentParseArtifactSvc.GetBySignature(ctx, build.SourceContentHash, string(req.Profile), "v1", build.ProviderSignature); err != nil {
			summary["artifact_lookup_error"] = err.Error()
		} else if persisted != nil {
			artifactID = &persisted.ID
		}
	}

	run := contentparsesvc.BuildDatasetParseRun(contentparsesvc.DatasetParseRunBuildInput{
		WorkspaceID:    parseOptionalUUID(workspaceID),
		DatasetID:      parseOptionalUUID(datasetID),
		DocumentID:     parseOptionalUUID(documentID),
		FileID:         parseOptionalUUID(fileID),
		ArtifactID:     artifactID,
		Request:        req,
		RoutePlan:      routePlan,
		Artifact:       artifact,
		Summary:        summary,
		OrganizationID: strings.TrimSpace(organizationID),
	})

	if err := ir.contentParseRunService.CreateParseRun(ctx, run); err != nil {
		summary["run_persist_error"] = err.Error()
		return nil, artifactID
	}
	return &run.ID, artifactID
}

func (ir *IndexingRunner) persistChunkingShadow(
	ctx context.Context,
	parseRunID uuid.UUID,
	parseArtifactID *uuid.UUID,
	artifact *contracts.ParseArtifact,
	primary *dto.ExtractOutput,
	legacyChunks []dto.TransformedChunk,
	summary map[string]interface{},
) {
	if ir == nil || ir.contentParseRunService == nil || ir.contentParseChunkMapper == nil || ir.contentParseChunkPlanner == nil || artifact == nil {
		return
	}

	doc, err := ir.contentParseChunkMapper.FromParseArtifact(artifact)
	if err != nil {
		summary["chunking_shadow_error"] = fmt.Sprintf("map artifact: %v", err)
		return
	}

	plan, err := ir.contentParseChunkPlanner.Plan(doc, contracts.ChunkUseCaseDatasetIndex)
	if err != nil {
		summary["chunking_shadow_error"] = fmt.Sprintf("plan chunks: %v", err)
		return
	}

	chunkingSummary := summarizeChunkingShadow(doc, plan, nil, primary, legacyChunks, 0)
	executeStartedAt := time.Now()
	executeResult, executeErr := chunkexecutor.New(chunkexecutor.WithLimits(chunkexecutor.Limits{
		MaxWorkers:       contentParseChunkShadowWorkers(),
		MaxPartitionSize: contentParseChunkShadowPartitionSize(),
	})).Execute(ctx, doc, plan)
	executeDurationMS := time.Since(executeStartedAt).Milliseconds()
	if executeErr != nil {
		chunkingSummary["execution_error"] = executeErr.Error()
	} else {
		chunkingSummary = summarizeChunkingShadow(doc, plan, executeResult, primary, legacyChunks, executeDurationMS)
	}
	summary["chunking_shadow"] = chunkingSummary

	unitCount := len(doc.Elements)
	var chunkArtifactSetID *uuid.UUID
	if executeResult != nil {
		unitCount = len(executeResult.Units)
		chunkArtifactSetID = ir.persistChunkArtifactSet(ctx, parseRunID, parseArtifactID, artifact, plan, executeResult, chunkingSummary, summary)
	}
	run := contentparsesvc.BuildChunkingRun(contentparsesvc.ChunkingRunBuildInput{
		ParseRunID:         parseRunID,
		ChunkArtifactSetID: chunkArtifactSetID,
		Plan:               plan,
		UnitCount:          unitCount,
		PlanJSON:           chunkingSummary,
	})
	if err := ir.contentParseRunService.CreateChunkingRun(ctx, run); err != nil {
		summary["chunking_run_persist_error"] = err.Error()
		return
	}
	summary["chunking_run_id"] = run.ID.String()
}

func (ir *IndexingRunner) persistChunkArtifactSet(
	ctx context.Context,
	parseRunID uuid.UUID,
	parseArtifactID *uuid.UUID,
	artifact *contracts.ParseArtifact,
	plan *contracts.ChunkPlan,
	result *chunkexecutor.Result,
	chunkingSummary map[string]interface{},
	summary map[string]interface{},
) *uuid.UUID {
	if ir == nil || ir.contentParseChunkArtifact == nil || artifact == nil || plan == nil || result == nil {
		return nil
	}

	build := contentparsesvc.BuildChunkArtifactSetItem(contentparsesvc.ChunkArtifactSetBuildInput{
		ParseRunID:        parseRunID,
		ParseArtifactID:   parseArtifactID,
		Artifact:          artifact,
		Plan:              plan,
		Units:             result.Units,
		ChunkingSummary:   chunkingSummary,
		SourceContentHash: readStringMap(summary, "source_content_hash"),
	})
	if err := ir.contentParseChunkArtifact.Upsert(ctx, build.Item); err != nil {
		summary["chunk_artifact_set_persist_error"] = err.Error()
		return nil
	}
	persisted, err := ir.contentParseChunkArtifact.GetBySignature(ctx, build.Signature)
	if err != nil {
		summary["chunk_artifact_set_lookup_error"] = err.Error()
		return nil
	}
	if persisted == nil {
		return nil
	}
	contentparsesvc.ApplyChunkArtifactSetSummary(summary, chunkingSummary, persisted.ID)
	return &persisted.ID
}

func summarizeChunkingShadow(
	doc *contracts.ChunkSourceDocument,
	plan *contracts.ChunkPlan,
	execution *chunkexecutor.Result,
	primary *dto.ExtractOutput,
	legacyChunks []dto.TransformedChunk,
	executeDurationMS int64,
) map[string]interface{} {
	if doc == nil || plan == nil {
		return nil
	}
	out := map[string]interface{}{
		"document_id":       doc.DocumentID,
		"dataset_id":        doc.DatasetID,
		"file_id":           doc.FileID,
		"source":            doc.Source,
		"title":             doc.Title,
		"language":          doc.Language,
		"element_count":     len(doc.Elements),
		"chunk_use_case":    string(plan.UseCase),
		"parent_mode":       plan.ParentMode,
		"segmentation":      plan.Segmentation,
		"preserve_order":    plan.PreserveOrder,
		"target_kinds":      chunkKindsToStrings(plan.TargetKinds),
		"plan_metadata":     cloneStringAnyMap(plan.Metadata),
		"document_overview": summarizeChunkSourceDocument(doc),
	}
	if primary != nil {
		out["legacy_extract_baseline"] = summarizePrimaryExtractOutput(primary)
	}
	if len(legacyChunks) > 0 {
		out["legacy_chunk_baseline"] = summarizeLegacyTransformedChunks(legacyChunks)
	}
	if execution != nil {
		out["execution"] = summarizeChunkExecution(execution, executeDurationMS)
		out["comparison"] = compareChunkExecutionToLegacyChunks(legacyChunks, execution)
		out["extract_comparison"] = compareChunkExecutionToPrimary(primary, execution)
		out["quality_score"] = summarizeChunkQualityScore(execution, legacyChunks)
		adaptedChunks := chunkdataset.UnitsToTransformedChunksWithOptions(execution.Units, chunkdataset.AdapterOptions{
			BuildChildren: len(legacyChunks) > 0 && collectLegacyTransformedChunkStats(legacyChunks).ChildCount > 0,
		})
		out["dataset_adapter"] = summarizeDatasetAdapterShadow(adaptedChunks, legacyChunks)
	}
	return out
}

func summarizeChunkSourceDocument(doc *contracts.ChunkSourceDocument) map[string]interface{} {
	if doc == nil {
		return nil
	}
	headings := 0
	tables := 0
	figures := 0
	formulas := 0
	pages := make(map[int]struct{})
	for _, element := range doc.Elements {
		pages[element.Page] = struct{}{}
		switch strings.ToLower(strings.TrimSpace(element.Type)) {
		case "heading", "title":
			headings++
		case "table":
			tables++
		case "figure", "image":
			figures++
		case "formula", "equation":
			formulas++
		}
	}
	return map[string]interface{}{
		"page_count":    len(pages),
		"heading_count": headings,
		"table_count":   tables,
		"figure_count":  figures,
		"formula_count": formulas,
	}
}

func summarizeChunkExecution(result *chunkexecutor.Result, elapsedMS int64) map[string]interface{} {
	if result == nil {
		return nil
	}
	kindCounts := make(map[string]int)
	for _, unit := range result.Units {
		kindCounts[string(unit.Kind)]++
	}
	stats := collectChunkExecutionStats(result)
	return map[string]interface{}{
		"elapsed_ms":                    elapsedMS,
		"partition_count":               result.Metrics.PartitionCount,
		"worker_count":                  result.Metrics.WorkerCount,
		"unit_count":                    len(result.Units),
		"metric_unit_count":             result.Metrics.UnitCount,
		"filtered_unit_count":           result.Metrics.FilteredUnitCount,
		"source_element_filtered_count": result.Metrics.SourceElementFilteredCount,
		"stable_order":                  result.Metrics.StableOrder,
		"partition_kind_count":          result.Metrics.PartitionKindCount,
		"filter_reasons":                result.Metrics.FilterReasons,
		"source_element_filter_reasons": result.Metrics.SourceElementFilterReasons,
		"chunk_kind_count":              kindCounts,
		"avg_chunk_chars":               averageInt(stats.TotalChars, stats.UnitCount),
		"total_chunk_chars":             stats.TotalChars,
		"bbox_coverage":                 ratio(stats.BBoxCount, stats.UnitCount),
		"pages_covered":                 stats.PageCount,
	}
}

type chunkUnitStats struct {
	UnitCount  int
	TotalChars int
	BBoxCount  int
	EmptyCount int
	MaxChars   int
	PageCount  int
}

type transformedChunkStats struct {
	ParentCount int
	ChildCount  int
	UnitCount   int
	TotalChars  int
	BBoxCount   int
	EmptyCount  int
	MaxChars    int
}

func summarizeLegacyTransformedChunks(chunks []dto.TransformedChunk) map[string]interface{} {
	stats := collectLegacyTransformedChunkStats(chunks)
	return map[string]interface{}{
		"parent_chunk_count": stats.ParentCount,
		"child_chunk_count":  stats.ChildCount,
		"unit_count":         stats.UnitCount,
		"total_text_chars":   stats.TotalChars,
		"avg_chars":          averageInt(stats.TotalChars, stats.UnitCount),
		"max_chars":          stats.MaxChars,
		"empty_count":        stats.EmptyCount,
		"bbox_coverage":      ratio(stats.BBoxCount, stats.UnitCount),
	}
}

func summarizeDatasetAdapterShadow(adaptedChunks []dto.TransformedChunk, legacyChunks []dto.TransformedChunk) map[string]interface{} {
	adapted := collectLegacyTransformedChunkStats(adaptedChunks)
	legacy := collectLegacyTransformedChunkStats(legacyChunks)
	return map[string]interface{}{
		"adapted_chunk_baseline": summarizeLegacyTransformedChunks(adaptedChunks),
		"legacy_parent_count":    legacy.ParentCount,
		"adapted_parent_count":   adapted.ParentCount,
		"parent_count_delta":     adapted.ParentCount - legacy.ParentCount,
		"legacy_child_count":     legacy.ChildCount,
		"adapted_child_count":    adapted.ChildCount,
		"child_count_delta":      adapted.ChildCount - legacy.ChildCount,
		"legacy_unit_count":      legacy.UnitCount,
		"adapted_unit_count":     adapted.UnitCount,
		"unit_count_delta":       adapted.UnitCount - legacy.UnitCount,
		"text_retention_ratio":   ratio(adapted.TotalChars, legacy.TotalChars),
		"bbox_coverage_delta":    ratio(adapted.BBoxCount, adapted.UnitCount) - ratio(legacy.BBoxCount, legacy.UnitCount),
		"main_business_wired":    false,
	}
}

func compareChunkExecutionToLegacyChunks(legacyChunks []dto.TransformedChunk, result *chunkexecutor.Result) map[string]interface{} {
	legacy := collectLegacyTransformedChunkStats(legacyChunks)
	current := collectChunkExecutionStats(result)
	return map[string]interface{}{
		"baseline":                  "legacy_transformed_chunks",
		"legacy_parent_chunk_count": legacy.ParentCount,
		"legacy_child_chunk_count":  legacy.ChildCount,
		"legacy_unit_count":         legacy.UnitCount,
		"new_unit_count":            current.UnitCount,
		"unit_count_delta":          current.UnitCount - legacy.UnitCount,
		"legacy_text_length":        legacy.TotalChars,
		"new_text_length":           current.TotalChars,
		"text_length_delta":         current.TotalChars - legacy.TotalChars,
		"text_retention_ratio":      ratio(current.TotalChars, legacy.TotalChars),
		"legacy_avg_chars":          averageInt(legacy.TotalChars, legacy.UnitCount),
		"new_avg_chunk_chars":       averageInt(current.TotalChars, current.UnitCount),
		"legacy_bbox_coverage":      ratio(legacy.BBoxCount, legacy.UnitCount),
		"new_bbox_coverage":         ratio(current.BBoxCount, current.UnitCount),
		"bbox_coverage_delta":       ratio(current.BBoxCount, current.UnitCount) - ratio(legacy.BBoxCount, legacy.UnitCount),
		"low_value_removed_count":   resultFilteredCount(result),
		"stable_order":              resultStableOrder(result),
		"more_compact_than_legacy":  current.UnitCount > 0 && legacy.UnitCount > 0 && current.UnitCount < legacy.UnitCount,
	}
}

func summarizeChunkQualityScore(result *chunkexecutor.Result, legacyChunks []dto.TransformedChunk) map[string]interface{} {
	current := collectChunkExecutionStats(result)
	legacy := collectLegacyTransformedChunkStats(legacyChunks)
	score := chunkquality.EvaluateChunkScore(chunkquality.ScoreInput{
		UnitCount:            current.UnitCount,
		TotalChars:           current.TotalChars,
		AvgChars:             averageInt(current.TotalChars, current.UnitCount),
		BBoxCoverage:         ratio(current.BBoxCount, current.UnitCount),
		PageCoverage:         current.PageCount,
		StableOrder:          resultStableOrder(result),
		LowValueRemovedCount: resultFilteredCount(result),
		LegacyUnitCount:      legacy.UnitCount,
		LegacyTotalChars:     legacy.TotalChars,
	})
	return map[string]interface{}{
		"overall":  score.Overall,
		"label":    score.Label,
		"signals":  score.Signals,
		"warnings": score.Warnings,
	}
}

func collectChunkExecutionStats(result *chunkexecutor.Result) chunkUnitStats {
	if result == nil {
		return chunkUnitStats{}
	}
	pages := make(map[int]struct{})
	stats := chunkUnitStats{UnitCount: len(result.Units)}
	for _, unit := range result.Units {
		length := len([]rune(chunkUnitText(unit)))
		stats.TotalChars += length
		if length == 0 {
			stats.EmptyCount++
		}
		if length > stats.MaxChars {
			stats.MaxChars = length
		}
		if unit.BBox != nil {
			stats.BBoxCount++
		}
		for _, page := range unit.Pages {
			if page > 0 {
				pages[page] = struct{}{}
			}
		}
	}
	stats.PageCount = len(pages)
	return stats
}

func collectLegacyTransformedChunkStats(chunks []dto.TransformedChunk) transformedChunkStats {
	stats := transformedChunkStats{ParentCount: len(chunks)}
	for _, chunk := range chunks {
		addLegacyChunkStats(chunk.Content, chunk.BBox != nil, &stats)
		for _, child := range chunk.Children {
			stats.ChildCount++
			addLegacyChunkStats(child.Content, child.BBox != nil, &stats)
		}
	}
	return stats
}

func addLegacyChunkStats(content string, hasBBox bool, stats *transformedChunkStats) {
	if stats == nil {
		return
	}
	stats.UnitCount++
	length := len([]rune(strings.TrimSpace(content)))
	stats.TotalChars += length
	if length == 0 {
		stats.EmptyCount++
	}
	if length > stats.MaxChars {
		stats.MaxChars = length
	}
	if hasBBox {
		stats.BBoxCount++
	}
}

func compareChunkExecutionToPrimary(primary *dto.ExtractOutput, result *chunkexecutor.Result) map[string]interface{} {
	primaryElementCount := 0
	primaryTextLength := 0
	primaryBBoxCount := 0
	if primary != nil {
		primaryElementCount = len(primary.Elements)
		primaryTextLength = len([]rune(dto.ExtractOutputText(primary)))
		for _, element := range primary.Elements {
			if element.BBox != nil {
				primaryBBoxCount++
			}
		}
	}

	newUnitCount := 0
	newTextLength := 0
	newBBoxCount := 0
	if result != nil {
		newUnitCount = len(result.Units)
		for _, unit := range result.Units {
			newTextLength += len([]rune(chunkUnitText(unit)))
			if unit.BBox != nil {
				newBBoxCount++
			}
		}
	}

	return map[string]interface{}{
		"baseline":                               "legacy_extract_output",
		"primary_element_count":                  primaryElementCount,
		"new_unit_count":                         newUnitCount,
		"unit_count_delta":                       newUnitCount - primaryElementCount,
		"primary_text_length":                    primaryTextLength,
		"new_text_length":                        newTextLength,
		"text_length_delta":                      newTextLength - primaryTextLength,
		"primary_avg_chars":                      averageInt(primaryTextLength, primaryElementCount),
		"new_avg_chunk_chars":                    averageInt(newTextLength, newUnitCount),
		"primary_bbox_coverage":                  ratio(primaryBBoxCount, primaryElementCount),
		"new_bbox_coverage":                      ratio(newBBoxCount, newUnitCount),
		"bbox_coverage_delta":                    ratio(newBBoxCount, newUnitCount) - ratio(primaryBBoxCount, primaryElementCount),
		"low_value_removed_count":                resultFilteredCount(result),
		"low_value_unit_removed_count":           resultUnitFilteredCount(result),
		"low_value_source_element_removed_count": resultSourceElementFilteredCount(result),
		"stable_order":                           resultStableOrder(result),
		"more_compact_than_primary":              newUnitCount > 0 && primaryElementCount > 0 && newUnitCount < primaryElementCount,
	}
}

func averageInt(total, count int) float64 {
	if count <= 0 {
		return 0
	}
	return float64(total) / float64(count)
}

func ratio(part, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(part) / float64(total)
}

func chunkUnitText(unit contracts.ChunkUnit) string {
	content := strings.TrimSpace(unit.Content)
	if content != "" {
		return content
	}
	return strings.TrimSpace(unit.Markdown)
}

func resultFilteredCount(result *chunkexecutor.Result) int {
	if result == nil {
		return 0
	}
	return result.Metrics.FilteredUnitCount + result.Metrics.SourceElementFilteredCount
}

func resultUnitFilteredCount(result *chunkexecutor.Result) int {
	if result == nil {
		return 0
	}
	return result.Metrics.FilteredUnitCount
}

func resultSourceElementFilteredCount(result *chunkexecutor.Result) int {
	if result == nil {
		return 0
	}
	return result.Metrics.SourceElementFilteredCount
}

func resultStableOrder(result *chunkexecutor.Result) bool {
	return result == nil || result.Metrics.StableOrder
}

func chunkKindsToStrings(items []contracts.ChunkKind) []string {
	if len(items) == 0 {
		return nil
	}
	values := make([]string, 0, len(items))
	for _, item := range items {
		values = append(values, string(item))
	}
	return values
}

func parseOptionalUUID(raw string) *uuid.UUID {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return nil
	}
	return &id
}

func readStringMap(summary map[string]interface{}, key string) string {
	if len(summary) == 0 {
		return ""
	}
	value, ok := summary[key]
	if !ok || value == nil {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func cloneStringAnyMap(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func optionalInt(value interface{}) *int {
	switch typed := value.(type) {
	case int:
		return &typed
	case int32:
		converted := int(typed)
		return &converted
	case int64:
		converted := int(typed)
		return &converted
	case float64:
		converted := int(typed)
		return &converted
	default:
		return nil
	}
}

func (ir *IndexingRunner) persistContentParseShadow(ctx context.Context, documentID string, summary map[string]interface{}) {
	if err := ir.documentRepo.UpdateDocumentMetadataField(ctx, documentID, "contentparse_shadow", summary); err != nil {
		logger.Warn("Failed to persist content parse shadow metadata", map[string]interface{}{
			"document_id": documentID,
			"error":       err.Error(),
		})
	}
}

func toContentParseEngineHint(strategy string) contracts.ParseEngine {
	switch strings.ToLower(strings.TrimSpace(strategy)) {
	case "mineru":
		return contracts.ParseEngineMineru
	case "reducto":
		return contracts.ParseEngineReducto
	case "vlm", "gemini":
		return contracts.ParseEngineVLM
	default:
		return contracts.ParseEngineLocal
	}
}

func summarizePrimaryExtractOutput(output *dto.ExtractOutput) map[string]interface{} {
	if output == nil {
		return map[string]interface{}{
			"source":                      "",
			"element_count":               0,
			"markdown_length":             0,
			"text_length":                 0,
			"avg_chars_per_element":       0,
			"element_bbox_coverage_ratio": 0,
		}
	}
	textLength := len([]rune(dto.ExtractOutputText(output)))
	bboxCount := 0
	for _, element := range output.Elements {
		if element.BBox != nil {
			bboxCount++
		}
	}
	return map[string]interface{}{
		"source":                      output.Source,
		"element_count":               len(output.Elements),
		"markdown_length":             len(output.Markdown),
		"text_length":                 textLength,
		"avg_chars_per_element":       averageInt(textLength, len(output.Elements)),
		"element_bbox_coverage_ratio": ratio(bboxCount, len(output.Elements)),
	}
}

func (ir *IndexingRunner) load(ctx context.Context, indexProcessor BaseIndexProcessor, dataset *model.Dataset, datasetDocument *model.Document, chunks []dto.TransformedChunk) error {
	var embeddingService embedding.EmbeddingService
	var err error

	// Always use high_quality mode for embedding
	embeddingService, err = ir.buildEmbeddingService(ctx, dataset)
	if err != nil {
		logger.Error("Failed to get embedding service", err)
		return fmt.Errorf("failed to get embedding service: %w", err)
	}

	// Always call indexProcessor.Load to ensure segments are marked as completed
	tokens, err := indexProcessor.Load(ctx, dataset, chunks, true, embeddingService, ir.documentRepo, ir.vectorDB)
	if err != nil {
		logger.Error("Failed to load documents", err)
		return fmt.Errorf("failed to load documents: %w", err)
	}

	// Update document tokens (0 if economy mode)
	if err := ir.documentRepo.UpdateDocumentTokens(ctx, datasetDocument.ID, tokens); err != nil {
		logger.Error("Failed to update document tokens", err)
	}

	// Update document to completed status
	completedAt := time.Now()
	if err := ir.documentRepo.UpdateDocumentCompleted(ctx, datasetDocument.ID, &completedAt); err != nil {
		logger.Error("Failed to update document completion status", err)
	} else {
		logger.Info("Document indexing completed", map[string]interface{}{
			"document_id": datasetDocument.ID,
		})
	}

	return nil
}

// storeInVectorDatabase stores embeddings in the vector database
func (ir *IndexingRunner) storeInVectorDatabase(ctx context.Context, datasetDocument *model.Document, segment *model.DocumentSegment, embeddings []float64) (string, error) {
	// Generate unique UUID for this vector
	indexNodeID := segment.ID

	// Prepare metadata properties for vector storage.
	properties := map[string]interface{}{
		"text":        segment.Content,    // Main content field (defined in schema)
		"doc_id":      segment.ID,         // segment ID as doc_id (API format)
		"doc_hash":    "",                 // Empty for now, could add hash if needed
		"document_id": segment.DocumentID, // document ID
		"dataset_id":  segment.DatasetID,  // dataset ID
	}

	// Generate collection name
	className := model.GenCollectionNameByID(segment.DatasetID)

	// Ensure the class exists in the vector database
	if err := ir.ensureVectorClass(ctx, className); err != nil {
		logger.Error("Failed to ensure vector class exists", err)
		// Continue with storage attempt anyway
	}

	// Store the vector in the database
	if err := ir.vectorDB.StoreVector(ctx, indexNodeID, className, properties, embeddings); err != nil {
		return "", fmt.Errorf("failed to store vector in database: %w", err)
	}

	logger.Info("Successfully stored embeddings in vector database", map[string]interface{}{
		"segment_id":     segment.ID,
		"index_node_id":  indexNodeID,
		"embedding_size": len(embeddings),
		"class_name":     className,
	})

	return indexNodeID, nil
}

// ensureVectorClass ensures that the required class exists in the vector database
func (ir *IndexingRunner) ensureVectorClass(ctx context.Context, className string) error {
	// Define the schema properties
	// Other fields (doc_id, doc_hash, document_id, dataset_id) are stored as dynamic properties
	properties := []map[string]interface{}{
		{
			"name":            "text",
			"dataType":        []string{"text"},
			"tokenization":    "gse_ch",
			"indexSearchable": true,
		},
	}

	return ir.vectorDB.CreateClass(ctx, className, properties)
}

// simpleHash generates a simple hash for content
func simpleHash(content string) string {
	h := sha256.New()
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}

func mergeProcessRuleRules(base model.JSONMap, overrides map[string]interface{}) model.JSONMap {
	merged := model.JSONMap{}
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range overrides {
		merged[key] = value
	}
	return merged
}

// shouldGenerateRecommendQuestions checks if we should generate recommend questions based on process rules
func (ir *IndexingRunner) shouldGenerateRecommendQuestions(rules map[string]interface{}) bool {
	// Check if pre_processing_rules exists and is an array
	if preProcessingRules, ok := rules["pre_processing_rules"].([]interface{}); ok {
		// Iterate through pre_processing_rules to find generate_recommend_questions
		for _, rule := range preProcessingRules {
			if ruleMap, ok := rule.(map[string]interface{}); ok {
				if id, ok := ruleMap["id"].(string); ok && id == "generate_recommend_questions" {
					if enabled, ok := ruleMap["enabled"].(bool); ok {
						return enabled
					}
				}
			}
		}
	}
	return false
}

// generateRecommendQuestions generates recommend questions for segments
func (ir *IndexingRunner) generateRecommendQuestions(ctx context.Context, datasetDocument *model.Document, tenantID, userID string) error {
	// Asynchronously process all segments to avoid blocking the main indexing process
	go func() {
		// Create a background context since the original context might be cancelled
		backgroundCtx := context.Background()

		// Get all segments for this document
		segments, err := ir.documentRepo.GetSegmentsByDocumentID(backgroundCtx, datasetDocument.ID)
		if err != nil {
			logger.Error("Failed to get segments", err)
			return
		}

		// Process each segment to generate questions
		for _, segment := range segments {
			// Generate 3 questions for each segment (can be adjusted)
			_, err := ir.generateQuestionsForSegment(backgroundCtx, segment.ID, 3, userID, tenantID)
			if err != nil {
				logger.Error("Failed to generate questions for segment", err)
				// Continue with other segments even if one fails
			}
		}
	}()

	return nil
}

func (ir *IndexingRunner) buildEmbeddingService(ctx context.Context, dataset *model.Dataset) (embedding.EmbeddingService, error) {
	if dataset == nil {
		return nil, fmt.Errorf("dataset is nil")
	}

	resolvedModel, err := llmruntime.NewModelResolver(ir.defaultModelSvc).ResolveFromPointers(
		ctx,
		dataset.OrganizationID,
		dataset.EmbeddingModelProvider,
		dataset.EmbeddingModel,
		shared_model.ModelTypeEmbedding,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve embedding model: %w", err)
	}

	gatewaySvc, err := ir.newGatewayEmbeddingService(dataset, resolvedModel.Model)
	if err != nil {
		return nil, fmt.Errorf("failed to build gateway embedding service: %w", err)
	}

	logger.Info("Using gateway embedding service for indexing", map[string]interface{}{
		"dataset_id": dataset.ID,
		"model":      resolvedModel.Model,
	})
	return gatewaySvc, nil
}

func (ir *IndexingRunner) newGatewayEmbeddingService(dataset *model.Dataset, modelName string) (embedding.EmbeddingService, error) {
	if ir.llmClient == nil {
		return nil, fmt.Errorf("llm client is nil")
	}

	accountID := dataset.CreatedBy
	return NewGatewayEmbeddingService(ir.llmClient, accountID, dataset.ID, "dataset", modelName, dataset.WorkspaceID)
}

// generateQuestionsForSegment generates questions for a specific segment using the segment service
func (ir *IndexingRunner) generateQuestionsForSegment(ctx context.Context, segmentID string, count int, userID, tenantID string) (*dto.DocumentSegmentQuestionBatchCreateResponse, error) {
	// Get segment to verify it exists
	segment, err := ir.documentRepo.GetDocumentSegmentByID(ctx, segmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get segment: %w", err)
	}
	if segment == nil {
		return nil, fmt.Errorf("segment not found")
	}

	// Get document to verify it exists and get dataset ID
	document, err := ir.documentRepo.GetByID(ctx, segment.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}
	if document == nil {
		return nil, fmt.Errorf("document not found")
	}

	// Generate questions using LLM
	questions, err := ir.generateQuestionsWithLLM(ctx, segment.Content, segmentID, segment.DocumentID, document.DatasetID, count, userID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate questions with LLM: %w", err)
	}

	// Save to database
	if err := ir.documentRepo.BatchCreateDocumentSegmentQuestions(ctx, questions); err != nil {
		return nil, fmt.Errorf("failed to batch create questions: %w", err)
	}

	// Convert to response DTOs
	var questionResponses []dto.DocumentSegmentQuestionResponse
	for _, question := range questions {
		questionResponses = append(questionResponses, *ir.convertQuestionToResponse(question))
	}

	// Synchronously index the questions to ensure proper resource management
	// Create QA documents for indexing
	var qaDocs []dto.Document
	for _, question := range questions {
		qaDoc := dto.Document{
			PageContent: question.Question,
			Metadata: map[string]interface{}{
				"doc_id":      *segment.IndexNodeID,
				"doc_hash":    fmt.Sprintf("question/%s", question.ID),
				"segment_id":  segmentID,
				"document_id": segment.DocumentID,
				"dataset_id":  document.DatasetID,
				"question_id": question.ID,
			},
		}
		qaDocs = append(qaDocs, qaDoc)
	}

	dataset, err := ir.datasetRepo.GetByID(ctx, document.DatasetID)
	if err != nil {
		return nil, fmt.Errorf("failed to get dataset for QA indexing: %w", err)
	}

	// Use fixed QA index processor instance
	indexProcessor := NewQAIndexProcessor(ir.storage, ir.defaultModelSvc, ir.llmClient, tenantID)

	embeddingService, err := ir.buildEmbeddingService(ctx, dataset)
	if err != nil {
		return nil, fmt.Errorf("failed to get embedding service for QA indexing: %w", err)
	}

	_, err = indexProcessor.Load(ctx, dataset, dto.DocumentsToTransformedChunks(qaDocs), true, embeddingService, ir.documentRepo, ir.vectorDB)
	if err != nil {
		return nil, fmt.Errorf("failed to index QA documents: %w", err)
	}

	return &dto.DocumentSegmentQuestionBatchCreateResponse{
		Questions: questionResponses,
		Count:     len(questionResponses),
	}, nil
}

// generateQuestionsWithLLM generates questions using a language model
func (ir *IndexingRunner) generateQuestionsWithLLM(ctx context.Context, content, segmentID, documentID, datasetID string, count int, userID, tenantID string) ([]*model.DocumentSegmentQuestion, error) {
	resolvedModel, err := llmruntime.NewModelResolver(ir.defaultModelSvc).ResolveDefault(ctx, tenantID, shared_model.ModelTypeLLM)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve chat model: %w", err)
	}

	// Get prompt template
	tmpl, err := prompt.GetTemplate(prompt.DatasetQuestionGeneration)
	if err != nil {
		return nil, fmt.Errorf("failed to get prompt template: %w", err)
	}

	// Prepare template data
	templateData := struct {
		Content string
		Count   int
	}{
		Content: content,
		Count:   count,
	}

	// Render prompt
	promptText, err := tmpl.Render(templateData)
	if err != nil {
		return nil, fmt.Errorf("failed to render prompt template: %w", err)
	}

	resp, err := ir.llmClient.Chat(ctx, tenantID, &llmadapter.ChatRequest{
		Model: resolvedModel.Model,
		Messages: []llmadapter.Message{
			{Role: "user", Content: promptText},
		},
		Stream: false,
		User:   userID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate questions with LLM: %w", err)
	}

	if resp == nil || len(resp.Choices) == 0 {
		return nil, fmt.Errorf("failed to generate questions with LLM: empty chat response")
	}
	generatedContent, ok := resp.Choices[0].Message.Content.(string)
	if !ok || strings.TrimSpace(generatedContent) == "" {
		return nil, fmt.Errorf("failed to generate questions with LLM: empty chat result")
	}

	// Parse the generated content into questions
	questions := ir.parseGeneratedQuestions(generatedContent, segmentID, documentID, datasetID, count, userID, tenantID)

	// If parsing failed to produce questions, return error
	if len(questions) == 0 {
		return nil, fmt.Errorf("failed to parse generated questions")
	}

	// Return the generated questions (without saving to database)
	return questions, nil
}

// parseGeneratedQuestions parses the LLM generated content into questions
func (ir *IndexingRunner) parseGeneratedQuestions(content, segmentID, documentID, datasetID string, count int, userID, tenantID string) []*model.DocumentSegmentQuestion {
	var questions []*model.DocumentSegmentQuestion

	// Split content by newlines to get individual questions
	lines := strings.Split(content, "\n")

	// Filter out empty lines
	var validLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			validLines = append(validLines, trimmed)
		}
	}

	// Take only the requested number of questions
	actualCount := count
	if actualCount > len(validLines) {
		actualCount = len(validLines)
	}

	// Create question models
	for i := 0; i < actualCount; i++ {
		question := &model.DocumentSegmentQuestion{
			ID:             uuid.New().String(),
			OrganizationID: tenantID,
			DatasetID:      datasetID,
			DocumentID:     documentID,
			SegmentID:      segmentID,
			Question:       validLines[i],
			CreatedBy:      userID,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		questions = append(questions, question)
	}

	return questions
}

// convertQuestionToResponse converts a DocumentSegmentQuestion model to DTO response
func (ir *IndexingRunner) convertQuestionToResponse(question *model.DocumentSegmentQuestion) *dto.DocumentSegmentQuestionResponse {
	createdAt := question.CreatedAt.Unix()
	updatedAt := question.UpdatedAt.Unix()

	response := &dto.DocumentSegmentQuestionResponse{
		ID:             question.ID,
		OrganizationID: question.OrganizationID,
		DatasetID:      question.DatasetID,
		DocumentID:     question.DocumentID,
		SegmentID:      question.SegmentID,
		Question:       question.Question,
		CreatedBy:      question.CreatedBy,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}

	return response
}

// loadSegments creates document segments in the database and updates document status.
func (ir *IndexingRunner) loadSegments(ctx context.Context, datasetDocument *model.Document, chunks []dto.TransformedChunk) (int, error) {
	// Ensure idempotency by clearing existing segments and child chunks for this document
	if err := ir.documentRepo.DeleteChildChunksByDocumentID(ctx, datasetDocument.ID); err != nil {
		logger.Error("Failed to delete existing child chunks by document", err)
		return 0, fmt.Errorf("failed to delete existing child chunks: %w", err)
	}
	if err := ir.documentRepo.DeleteDocumentSegmentsByDocumentID(ctx, datasetDocument.ID); err != nil {
		logger.Error("Failed to delete existing document segments", err)
		return 0, fmt.Errorf("failed to delete existing segments: %w", err)
	}

	// Create document segments in database
	segments := make([]*model.DocumentSegment, 0, len(chunks))
	var lastErr error
	currentPosition := 0
	for i := range chunks {
		chunk := &chunks[i]

		// Skip empty content
		if strings.TrimSpace(chunk.Content) == "" {
			continue
		}
		currentPosition++

		// Calculate word count and tokens (simplified)
		wordCount := len([]rune(chunk.Content))
		tokens := wordCount / 4 // Rough estimation

		if chunk.Metadata == nil {
			chunk.Metadata = make(map[string]any)
		}
		indexNodeID := uuid.New().String()
		indexNodeHash := simpleHash(chunk.Content)
		chunk.Metadata["doc_id"] = indexNodeID
		chunk.Metadata["doc_hash"] = indexNodeHash

		segmentModel := &model.DocumentSegment{
			OrganizationID: datasetDocument.OrganizationID,
			DatasetID:      datasetDocument.DatasetID,
			DocumentID:     datasetDocument.ID,
			Position:       currentPosition,
			Content:        chunk.Content,
			WordCount:      wordCount,
			Tokens:         tokens,
			IndexNodeID:    &indexNodeID,
			IndexNodeHash:  &indexNodeHash,
			Status:         model.DocumentStatusWaiting,
			CreatedBy:      datasetDocument.CreatedBy,
			CreatedAt:      time.Now(),
		}

		if err := ir.documentRepo.CreateDocumentSegment(ctx, segmentModel); err != nil {
			logger.Error("Failed to create document segment", err)
			lastErr = err
			continue
		}

		segments = append(segments, segmentModel)

		// Save child chunks if they exist
		if chunk.Children != nil && len(chunk.Children) > 0 {
			childPosition := 0
			for childIndex := range chunk.Children {
				child := &chunk.Children[childIndex]

				// Skip empty child chunks
				if strings.TrimSpace(child.Content) == "" {
					continue
				}
				childPosition++

				// Calculate word count for child chunk
				childWordCount := len([]rune(child.Content))
				if child.Metadata == nil {
					child.Metadata = make(map[string]any)
				}
				childIndexNodeID := uuid.New().String()
				childIndexNodeHash := simpleHash(child.Content)
				child.Metadata["doc_id"] = childIndexNodeID
				child.Metadata["doc_hash"] = childIndexNodeHash

				childChunk := &model.ChildChunk{
					OrganizationID: datasetDocument.OrganizationID,
					DatasetID:      datasetDocument.DatasetID,
					DocumentID:     datasetDocument.ID,
					SegmentID:      segmentModel.ID,
					Position:       childPosition,
					Content:        child.Content,
					WordCount:      childWordCount,
					Type:           "automatic",
					IndexNodeID:    &childIndexNodeID,
					IndexNodeHash:  &childIndexNodeHash,
					CreatedBy:      datasetDocument.CreatedBy,
					CreatedAt:      time.Now(),
				}

				// Save child chunk to database
				if err := ir.documentRepo.CreateChildChunk(ctx, childChunk); err != nil {
					logger.Error("Failed to create child chunk", err)
					continue
				}
			}
		}
	}

	if len(segments) == 0 {
		errMsg := "failed to save any segments to database"
		if lastErr != nil {
			errMsg = fmt.Sprintf("%s: %v", errMsg, lastErr)
		}
		return 0, fmt.Errorf("%s", errMsg)
	}

	// Update document status to indexing and set cleaning_completed_at and splitting_completed_at
	if err := ir.documentRepo.UpdateDocumentIndexingStatus(ctx, datasetDocument.ID, model.DocumentStatusIndexing); err != nil {
		logger.Error("Failed to update document indexing status to indexing", err)
		return 0, fmt.Errorf("failed to update document indexing status: %w", err)
	}

	curTime := time.Now()
	if err := ir.documentRepo.UpdateDocumentCleaningCompleted(ctx, datasetDocument.ID, &curTime); err != nil {
		logger.Error("Failed to update document cleaning completed time", err)
		return 0, fmt.Errorf("failed to update document cleaning completed time: %w", err)
	}

	if err := ir.documentRepo.UpdateDocumentSplittingCompleted(ctx, datasetDocument.ID, &curTime); err != nil {
		logger.Error("Failed to update document splitting completed time", err)
		return 0, fmt.Errorf("failed to update document splitting completed time: %w", err)
	}

	// Update segment status to indexing
	for _, segment := range segments {
		indexingTime := time.Now()
		if err := ir.documentRepo.UpdateSegmentIndexingStatus(ctx, segment.ID, model.SegmentStatusIndexing, &indexingTime); err != nil {
			logger.Error("Failed to update segment indexing status", err)
			// Continue with other segments even if one fails
		}
	}

	return len(segments), nil
}
