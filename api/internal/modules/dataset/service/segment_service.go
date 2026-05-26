package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/internal/dto"
	graphflow_model "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	graphflow_repo "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/repository"
	graphflow_worker "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/worker"
	"github.com/zgiai/zgi/api/internal/modules/dataset/indexing"
	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
	dataset_repo "github.com/zgiai/zgi/api/internal/modules/dataset/repository"
	"github.com/zgiai/zgi/api/internal/modules/dataset/splitter"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	llmruntime "github.com/zgiai/zgi/api/internal/modules/llm/runtime"
	shared_model "github.com/zgiai/zgi/api/internal/modules/shared/model"
	"github.com/zgiai/zgi/api/internal/prompt"
	"github.com/zgiai/zgi/api/pkg/embedding"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
	"github.com/zgiai/zgi/api/pkg/storage"
	"github.com/zgiai/zgi/api/pkg/vectordb"
	"gorm.io/gorm"
)

// SegmentService interface
type SegmentService interface {
	GetSegmentsByDocument(ctx context.Context, datasetID, documentID string, req *dto.SegmentListRequest) (*dto.SegmentListResponse, error)

	CreateSegment(ctx context.Context, documentID, datasetID, accountID, tenantID string, req *dto.SegmentCreateRequest) (*dto.SegmentResponse, error)

	UpdateSegment(ctx context.Context, segmentID string, req *dto.SegmentUpdateRequest) (*dto.SegmentResponse, error)

	DeleteSegment(ctx context.Context, segmentID string) error

	DeleteSegments(ctx context.Context, segmentIDs []string, documentID, datasetID string) error

	GetChunkByID(ctx context.Context, id string) (*model.DocumentSegment, error)

	GetChildChunks(ctx context.Context, segmentID, documentID, datasetID string, page, limit int, keyword string) (*dto.ChildChunkListResponse, error)

	CreateChildChunk(ctx context.Context, childChunk *model.ChildChunk) (*dto.ChildChunkResponse, error)

	UpdateChildChunk(ctx context.Context, childChunk *model.ChildChunk) (*dto.ChildChunkResponse, error)

	DeleteChildChunk(ctx context.Context, childChunkID string) error

	GetChildChunkByID(ctx context.Context, childChunkID string) (*model.ChildChunk, error)

	// Document segment question related methods
	CreateDocumentSegmentQuestion(ctx context.Context, req *dto.DocumentSegmentQuestionCreateRequest, userID, organizationID string) (*dto.DocumentSegmentQuestionResponse, error)
	GetDocumentSegmentQuestionByID(ctx context.Context, questionID string) (*dto.DocumentSegmentQuestionResponse, error)
	ListDocumentSegmentQuestionsBySegment(ctx context.Context, segmentID string, req *dto.DocumentSegmentQuestionListRequest) (*dto.DocumentSegmentQuestionListResponse, error)
	ListDocumentSegmentQuestionsByDocument(ctx context.Context, documentID string, req *dto.DocumentSegmentQuestionListRequest) (*dto.DocumentSegmentQuestionListResponse, error)
	ListDocumentSegmentQuestionsByDataset(ctx context.Context, datasetID string, req *dto.DocumentSegmentQuestionListRequest) (*dto.DocumentSegmentQuestionListResponse, error)
	// GetDocumentSegmentQuestionCountByDataset retrieves the count of document segment questions by dataset ID
	GetDocumentSegmentQuestionCountByDataset(ctx context.Context, datasetID string) (int64, error)
	// RandomDocumentSegmentQuestionsByDataset randomly selects a specified number of questions from a dataset
	RandomDocumentSegmentQuestionsByDataset(ctx context.Context, datasetID string, limit int) ([]dto.DocumentSegmentQuestionResponse, error)
	UpdateDocumentSegmentQuestion(ctx context.Context, questionID string, req *dto.DocumentSegmentQuestionUpdateRequest, userID string) (*dto.DocumentSegmentQuestionResponse, error)
	DeleteDocumentSegmentQuestion(ctx context.Context, questionID string) error
	DeleteDocumentSegmentQuestionsBySegment(ctx context.Context, segmentID string) error
	DeleteDocumentSegmentQuestionsByDocument(ctx context.Context, documentID string) error
	DeleteDocumentSegmentQuestionsByDataset(ctx context.Context, datasetID string) error
	BatchCreateDocumentSegmentQuestions(ctx context.Context, req *dto.DocumentSegmentQuestionBatchCreateRequest, userID, organizationID string, segmentID string) (*dto.DocumentSegmentQuestionBatchCreateResponse, error)
	GenerateQuestionsForSegment(ctx context.Context, segmentID string, count int, userID, organizationID string, model *dto.ModelSpec) (*dto.DocumentSegmentQuestionBatchCreateResponse, error)
}

type segmentServiceImpl struct {
	chunkService      ChunkService
	datasetRepo       dataset_repo.DatasetRepository
	documentRepo      dataset_repo.DocumentRepository
	defaultModelSvc   llmdefaultservice.DefaultModelService
	db                *gorm.DB
	qaIndexProcessor  indexing.BaseIndexProcessor
	vectorDB          vectordb.VectorDB
	llmClient         llmclient.LLMClient
	graphFlowTaskRepo *graphflow_repo.GraphFlowTaskRepository
	taskManager       *queue.TaskManager
	embeddingFactory  func(ctx context.Context, dataset *model.Dataset) (embedding.EmbeddingService, error)
}

// NewSegmentService creates a new SegmentService.
// The llmClient should be obtained from the DI container (ServiceContainer.GetLLMClient()).
func NewSegmentService(chunkService ChunkService, datasetRepo dataset_repo.DatasetRepository, documentRepo dataset_repo.DocumentRepository, defaultModelSvc llmdefaultservice.DefaultModelService, db *gorm.DB, vectorDB vectordb.VectorDB, llmClient llmclient.LLMClient, graphFlowTaskRepo *graphflow_repo.GraphFlowTaskRepository, taskManager *queue.TaskManager) SegmentService {
	// Create index processor
	storageInstance := storage.GetStorage()
	qaIndexProcessor := indexing.NewQAIndexProcessor(storageInstance, defaultModelSvc, llmClient, "")
	return &segmentServiceImpl{
		chunkService:      chunkService,
		datasetRepo:       datasetRepo,
		documentRepo:      documentRepo,
		defaultModelSvc:   defaultModelSvc,
		db:                db,
		qaIndexProcessor:  qaIndexProcessor,
		vectorDB:          vectorDB,
		llmClient:         llmClient,
		graphFlowTaskRepo: graphFlowTaskRepo,
		taskManager:       taskManager,
	}
}

// GetSegmentsByDocument
func (s *segmentServiceImpl) GetSegmentsByDocument(ctx context.Context, datasetID, documentID string, req *dto.SegmentListRequest) (*dto.SegmentListResponse, error) {
	segments, err := s.chunkService.GetChunksByDocumentID(ctx, documentID)
	if err != nil {
		return nil, err
	}

	var segmentResponses []dto.SegmentDetailResponse
	for _, segment := range segments {
		segmentResponses = append(segmentResponses, s.convertSegmentToDetailResponse(segment))
	}

	filteredSegments := s.applyDetailFilters(segmentResponses, req)
	paginatedSegments := s.applyDetailPagination(filteredSegments, req.Page, req.Limit)

	totalPages := int(len(filteredSegments) / req.Limit)
	if len(filteredSegments)%req.Limit > 0 {
		totalPages++
	}

	return &dto.SegmentListResponse{
		Data:       paginatedSegments,
		Total:      int64(len(filteredSegments)),
		Page:       req.Page,
		Limit:      req.Limit,
		TotalPages: totalPages,
	}, nil
}

func (s *segmentServiceImpl) CreateSegment(ctx context.Context, documentID, datasetID, accountID, tenantID string, req *dto.SegmentCreateRequest) (*dto.SegmentResponse, error) {
	segment := &model.DocumentSegment{
		OrganizationID: tenantID,
		DatasetID:      datasetID,
		DocumentID:     documentID,
		Content:        req.Content,
		Position:       1,
		WordCount:      len(req.Content),
		Tokens:         len(req.Content),
		Status:         "waiting",
		Enabled:        true,
		CreatedBy:      accountID,
	}

	if err := s.chunkService.CreateChunk(ctx, segment); err != nil {
		return nil, err
	}

	// Async processing for embedding and GraphFlow
	backgroundCtx := context.Background()
	go func(seg *model.DocumentSegment) {
		// Recovery
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Panic in async segment generation", fmt.Errorf("%v", r))
			}
		}()

		// 1. Get Dataset info
		dataset, err := s.datasetRepo.GetByID(backgroundCtx, datasetID)
		if err != nil || dataset == nil {
			logger.Error("Failed to fetch dataset in CreateSegment async", err)
			errStr := "Failed to fetch dataset"
			seg.Error = &errStr
			seg.Status = "error"
			s.chunkService.UpdateChunk(backgroundCtx, seg)
			return
		}

		// Update to indexing
		now := time.Now()
		seg.IndexingAt = &now
		seg.Status = "indexing"
		if err := s.chunkService.UpdateChunk(backgroundCtx, seg); err != nil {
			logger.Error("Failed to update segment indexing status", err)
		}

		// 2. Build EmbeddingService
		embeddingService, err := s.buildEmbeddingService(backgroundCtx, dataset)
		if err != nil || embeddingService == nil {
			logger.Error("Failed to build embedding service in CreateSegment async", err)
			errStr := "Failed to build embedding service"
			seg.Error = &errStr
			seg.Status = "error"
			s.chunkService.UpdateChunk(backgroundCtx, seg)
			return
		}

		// 3. Generate Embedding
		var vector []float64
		embeddings, err := embeddingService.EmbedTexts(backgroundCtx, []string{seg.Content})
		if err != nil || len(embeddings) == 0 {
			logger.Error("Failed to generate embedding in CreateSegment async", err)
			errStr := "Failed to generate embedding"
			seg.Error = &errStr
			seg.Status = "error"
			s.chunkService.UpdateChunk(backgroundCtx, seg)
			return
		}
		vector = embeddings[0]

		// 4. Update Segment Index Node ID
		indexNodeID := uuid.New().String()
		seg.IndexNodeID = &indexNodeID

		// Auto-generate child_chunk if DocForm is hierarchical_model
		var isHierarchical bool
		if docObj, docErr := s.documentRepo.GetDocumentByID(backgroundCtx, documentID); docErr == nil && docObj != nil {
			isHierarchical = (docObj.DocForm == "hierarchical_model")
		}

		if isHierarchical {
			childChunk := &model.ChildChunk{
				OrganizationID: tenantID,
				DatasetID:      datasetID,
				DocumentID:     documentID,
				SegmentID:      seg.ID,
				Position:       1,
				Content:        seg.Content,
				WordCount:      len(seg.Content),
				Type:           "manual",
				CreatedBy:      accountID,
				IndexNodeID:    &indexNodeID,
			}
			if childErr := s.chunkService.CreateChildChunk(backgroundCtx, childChunk); childErr != nil {
				logger.Error("Failed to create child chunk for manual segment", childErr)
			} else {
				logger.Info("Auto-generated child chunk for hierarchical segment", map[string]interface{}{
					"segment_id": seg.ID,
				})
			}
		}

		// 5. Store in vector DB
		className := model.GenCollectionNameByID(datasetID)

		classProperties := []map[string]interface{}{
			{"name": "text", "dataType": []string{"text"}},
		}

		if err := s.vectorDB.CreateClass(backgroundCtx, className, classProperties); err != nil && !strings.Contains(err.Error(), "already exists") {
			logger.Error("Failed to create/ensure VectorDB class", err)
		}

		properties := map[string]interface{}{
			"text":        seg.Content,
			"doc_id":      seg.ID,
			"doc_hash":    "",
			"document_id": documentID,
			"dataset_id":  datasetID,
		}

		if err := s.vectorDB.StoreVector(backgroundCtx, indexNodeID, className, properties, vector); err != nil {
			logger.Error("Failed to store vector in DB", err)
			errStr := "Vector Indexing Failed"
			seg.Error = &errStr
			seg.Status = "error"
			s.chunkService.UpdateChunk(backgroundCtx, seg)
			return
		}

		// Success Path
		completedTime := time.Now()
		seg.CompletedAt = &completedTime
		seg.Status = "completed"

		// Update database
		if err := s.chunkService.UpdateChunk(backgroundCtx, seg); err != nil {
			logger.Error("Failed to update index node ID and status in DB", err)
		}

		// 6. Trigger GraphFlow (if enabled)
		if dataset.EnableGraphFlow && s.graphFlowTaskRepo != nil && s.taskManager != nil {
			docUUID, parseErr1 := uuid.Parse(documentID)
			segUUID, parseErr2 := uuid.Parse(seg.ID)
			dsUUID, parseErr3 := uuid.Parse(datasetID)
			tenantUUID, parseErr4 := uuid.Parse(dataset.OrganizationID)

			if parseErr1 == nil && parseErr2 == nil && parseErr3 == nil && parseErr4 == nil {
				graphFlowTask := &graphflow_model.GraphFlowTask{
					ID:         uuid.New(),
					KBID:       dsUUID,
					TenantID:   tenantUUID,
					DocumentID: docUUID,
					SegmentID:  &segUUID,
					TaskType:   "extraction",
					Status:     "waiting",
					Progress:   0,
					CreatedAt:  time.Now(),
				}

				if err := s.graphFlowTaskRepo.CreateTask(backgroundCtx, graphFlowTask); err != nil {
					logger.Error("Failed to create GraphFlow task in async processing", err)
				} else {
					asynqTask, taskErr := graphflow_worker.CreateGraphFlowExtractionTask(graphFlowTask.ID.String(), s.taskManager, 1)
					if taskErr == nil {
						_, enqueueErr := s.taskManager.EnqueueTask(asynqTask, asynq.Queue("graphflow"))
						if enqueueErr != nil {
							logger.Error("Failed to enqueue GraphFlow extraction task", enqueueErr)
						} else {
							logger.Info("GraphFlow extraction task enqueued for new segment", map[string]interface{}{
								"segment_id": segUUID.String(),
								"task_id":    graphFlowTask.ID.String(),
							})
						}
					}
				}
			}
		}
	}(segment)

	response := s.convertSegmentToResponse(segment)
	return &response, nil
}

func (s *segmentServiceImpl) UpdateSegment(ctx context.Context, segmentID string, req *dto.SegmentUpdateRequest) (*dto.SegmentResponse, error) {
	segment, err := s.chunkService.GetChunkByID(ctx, segmentID)
	if err != nil {
		return nil, err
	}

	originalSegment := *segment
	contentChanged := req.Content != "" && segment.Content != req.Content
	var dataset *model.Dataset
	if req.Content != "" {
		segment.Content = req.Content
		segment.WordCount = len([]rune(req.Content))
		segment.Tokens = len([]rune(req.Content))
	}
	if req.Answer != nil {
		segment.Answer = req.Answer
	}

	if contentChanged {
		dataset, err = s.datasetRepo.GetByID(ctx, segment.DatasetID)
		if err != nil {
			return nil, fmt.Errorf("failed to get dataset for segment vector: %w", err)
		}
		if segment.IndexNodeID == nil || strings.TrimSpace(*segment.IndexNodeID) == "" {
			indexNodeID := uuid.New().String()
			segment.IndexNodeID = &indexNodeID
		} else if originalSegment.IndexNodeID != nil && strings.TrimSpace(*originalSegment.IndexNodeID) != "" {
			if err := s.deleteSegmentVector(ctx, originalSegment.DatasetID, *originalSegment.IndexNodeID); err != nil {
				return nil, err
			}
		}
		hash := simpleHash(segment.Content)
		segment.IndexNodeHash = &hash
		if err := s.storeSegmentVector(ctx, segmentVectorTarget{
			Dataset:     dataset,
			DocumentID:  segment.DocumentID,
			IndexNodeID: *segment.IndexNodeID,
			Content:     segment.Content,
			DocHash:     hash,
		}); err != nil {
			if originalSegment.IndexNodeID != nil && strings.TrimSpace(*originalSegment.IndexNodeID) != "" {
				if restoreErr := s.storeSegmentVector(ctx, segmentVectorTarget{
					Dataset:     dataset,
					DocumentID:  originalSegment.DocumentID,
					IndexNodeID: *originalSegment.IndexNodeID,
					Content:     originalSegment.Content,
					DocHash:     valueOrEmpty(originalSegment.IndexNodeHash),
				}); restoreErr != nil {
					return nil, fmt.Errorf("failed to store updated segment vector: %w; restore error: %v", err, restoreErr)
				}
			}
			return nil, err
		}

	}

	if err := s.chunkService.UpdateChunk(ctx, segment); err != nil {
		if contentChanged {
			if segment.IndexNodeID != nil && strings.TrimSpace(*segment.IndexNodeID) != "" {
				_ = s.deleteSegmentVector(ctx, segment.DatasetID, *segment.IndexNodeID)
			}
			if originalSegment.IndexNodeID != nil && strings.TrimSpace(*originalSegment.IndexNodeID) != "" {
				if dataset, datasetErr := s.datasetRepo.GetByID(ctx, originalSegment.DatasetID); datasetErr == nil {
					_ = s.storeSegmentVector(ctx, segmentVectorTarget{
						Dataset:     dataset,
						DocumentID:  originalSegment.DocumentID,
						IndexNodeID: *originalSegment.IndexNodeID,
						Content:     originalSegment.Content,
						DocHash:     valueOrEmpty(originalSegment.IndexNodeHash),
					})
				}
			}
		}
		return nil, err
	}

	if req.RegenerateChildChunks {
		if dataset == nil {
			dataset, err = s.datasetRepo.GetByID(ctx, segment.DatasetID)
			if err != nil {
				return nil, fmt.Errorf("failed to get dataset for child chunk regeneration: %w", err)
			}
		}
		if err := s.regenerateChildChunks(ctx, dataset, segment); err != nil {
			return nil, err
		}
	}

	response := s.convertSegmentToResponse(segment)
	return &response, nil
}

func (s *segmentServiceImpl) DeleteSegment(ctx context.Context, segmentID string) error {
	return s.chunkService.DeleteChunk(ctx, segmentID)
}

func (s *segmentServiceImpl) DeleteSegments(ctx context.Context, segmentIDs []string, documentID, datasetID string) error {
	for _, segmentID := range segmentIDs {
		if err := s.chunkService.DeleteChunk(ctx, segmentID); err != nil {
			return err
		}
	}
	return nil
}

func (s *segmentServiceImpl) regenerateChildChunks(ctx context.Context, dataset *model.Dataset, segment *model.DocumentSegment) error {
	if dataset == nil {
		return fmt.Errorf("dataset is required")
	}
	if segment == nil {
		return fmt.Errorf("segment is required")
	}

	if err := s.deleteChildChunksForSegment(ctx, segment); err != nil {
		return err
	}

	rule, err := s.resolveSegmentChildChunkRule(ctx, segment)
	if err != nil {
		return err
	}
	chunks := s.splitSegmentChildChunkContents(ctx, segment.Content, rule)
	for i, content := range chunks {
		indexNodeID := uuid.New().String()
		hash := simpleHash(content)
		childChunk := &model.ChildChunk{
			OrganizationID: segment.OrganizationID,
			DatasetID:      segment.DatasetID,
			DocumentID:     segment.DocumentID,
			SegmentID:      segment.ID,
			Position:       i + 1,
			Content:        content,
			WordCount:      len([]rune(content)),
			Type:           model.ChildChunkTypeAutomatic,
			CreatedBy:      segment.CreatedBy,
			UpdatedBy:      segment.UpdatedBy,
			IndexNodeID:    &indexNodeID,
			IndexNodeHash:  &hash,
		}
		if err := s.chunkService.CreateChildChunk(ctx, childChunk); err != nil {
			_ = s.deleteChildChunksForSegment(ctx, segment)
			return fmt.Errorf("failed to create regenerated child chunk: %w", err)
		}
		if err := s.storeChildChunkVector(ctx, dataset, childChunk); err != nil {
			_ = s.chunkService.DeleteChildChunkByID(ctx, childChunk.ID)
			_ = s.deleteChildChunksForSegment(ctx, segment)
			return err
		}
	}

	return nil
}

func (s *segmentServiceImpl) deleteChildChunksForSegment(ctx context.Context, segment *model.DocumentSegment) error {
	if segment == nil {
		return fmt.Errorf("segment is required")
	}
	childChunks, err := s.chunkService.GetChildChunksBySegmentID(ctx, segment.ID)
	if err != nil {
		return fmt.Errorf("failed to get child chunks for segment %s: %w", segment.ID, err)
	}
	for _, childChunk := range childChunks {
		if childChunk.IndexNodeID != nil && strings.TrimSpace(*childChunk.IndexNodeID) != "" {
			if segment.IndexNodeID != nil && *childChunk.IndexNodeID == *segment.IndexNodeID {
				continue
			}
			if err := s.deleteSegmentVector(ctx, childChunk.DatasetID, *childChunk.IndexNodeID); err != nil {
				return err
			}
		}
	}
	if err := s.chunkService.DeleteChildChunksBySegmentID(ctx, segment.ID); err != nil {
		return fmt.Errorf("failed to delete child chunks for segment %s: %w", segment.ID, err)
	}
	return nil
}

func (s *segmentServiceImpl) resolveSegmentChildChunkRule(ctx context.Context, segment *model.DocumentSegment) (*indexing.Rule, error) {
	var rules map[string]interface{}
	if s.documentRepo != nil {
		document, err := s.documentRepo.GetDocumentByID(ctx, segment.DocumentID)
		if err != nil {
			return nil, fmt.Errorf("failed to get document for child chunk regeneration: %w", err)
		}
		if document != nil && document.DatasetProcessRuleID != nil && strings.TrimSpace(*document.DatasetProcessRuleID) != "" {
			processRule, err := s.documentRepo.GetProcessRuleByID(ctx, *document.DatasetProcessRuleID)
			if err != nil {
				return nil, fmt.Errorf("failed to get document process rule for child chunk regeneration: %w", err)
			}
			if processRule != nil {
				rules = map[string]interface{}(processRule.Rules)
			}
		}
		if rules == nil {
			processRule, err := s.documentRepo.GetLatestProcessRule(ctx, segment.DatasetID)
			if err != nil {
				return nil, fmt.Errorf("failed to get dataset process rule for child chunk regeneration: %w", err)
			}
			if processRule != nil {
				rules = map[string]interface{}(processRule.Rules)
			}
		}
	}

	if rules == nil {
		dataset, err := s.datasetRepo.GetByID(ctx, segment.DatasetID)
		if err != nil {
			return nil, fmt.Errorf("failed to get dataset process rule for child chunk regeneration: %w", err)
		}
		if dataset != nil && dataset.ProcessRule != nil {
			rules = map[string]interface{}(dataset.ProcessRule)
		}
	}

	rule, err := indexing.ParseRule(rules)
	if err != nil {
		return nil, fmt.Errorf("failed to parse child chunk process rule: %w", err)
	}
	return rule, nil
}

func (s *segmentServiceImpl) splitSegmentChildChunkContents(ctx context.Context, content string, rule *indexing.Rule) []string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return []string{}
	}

	if rule == nil || rule.SubchunkSegmentation == nil {
		return []string{trimmed}
	}
	fixedSeparator, separators := buildSegmentSubchunkSeparators(rule.SubchunkSegmentation.Separator)
	textSplitter := splitter.NewFixedRecursiveCharacterTextSplitter(
		fixedSeparator,
		separators,
		rule.SubchunkSegmentation.MaxTokens,
		rule.SubchunkSegmentation.ChunkOverlap,
		nil,
		false,
		false,
	)

	rawChunks := textSplitter.SplitText(trimmed)
	chunks := make([]string, 0, len(rawChunks))
	for _, chunk := range rawChunks {
		if ctx.Err() != nil {
			return chunks
		}
		if trimmedChunk := strings.TrimSpace(chunk); trimmedChunk != "" {
			chunks = append(chunks, trimmedChunk)
		}
	}
	if len(chunks) == 0 {
		return []string{trimmed}
	}
	return chunks
}

func buildSegmentSubchunkSeparators(preferredSeparator string) (string, []string) {
	defaultSeparators := []string{"\n\n", "\n", "。", "！", "？", "；", "：", ". ", "! ", "? ", "; ", ": ", ".", "!", "?", ";", ":", "，", ",", "、", " ", ""}
	fixedSeparator := preferredSeparator
	if fixedSeparator == "" {
		fixedSeparator = "\n"
	}
	separators := make([]string, 0, len(defaultSeparators)+1)
	seen := make(map[string]struct{}, len(defaultSeparators)+1)
	for _, separator := range append([]string{fixedSeparator}, defaultSeparators...) {
		if _, ok := seen[separator]; ok {
			continue
		}
		seen[separator] = struct{}{}
		separators = append(separators, separator)
	}
	return fixedSeparator, separators
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

type segmentVectorTarget struct {
	Dataset     *model.Dataset
	DocumentID  string
	IndexNodeID string
	Content     string
	DocHash     string
}

func (s *segmentServiceImpl) storeSegmentVector(ctx context.Context, target segmentVectorTarget) error {
	if s.vectorDB == nil {
		return fmt.Errorf("vector database is not configured")
	}
	if target.Dataset == nil {
		return fmt.Errorf("dataset is required")
	}
	if strings.TrimSpace(target.IndexNodeID) == "" {
		return fmt.Errorf("index node id is required")
	}
	if strings.TrimSpace(target.Content) == "" {
		return fmt.Errorf("content is required")
	}

	embeddingService, err := s.resolveSegmentEmbeddingService(ctx, target.Dataset)
	if err != nil {
		return err
	}

	return s.storeSegmentVectorWithEmbedding(ctx, target, embeddingService)
}

func (s *segmentServiceImpl) resolveSegmentEmbeddingService(ctx context.Context, dataset *model.Dataset) (embedding.EmbeddingService, error) {
	if s.embeddingFactory != nil {
		return s.embeddingFactory(ctx, dataset)
	}
	return s.buildEmbeddingService(ctx, dataset)
}

func (s *segmentServiceImpl) storeSegmentVectorWithEmbedding(ctx context.Context, target segmentVectorTarget, embeddingService embedding.EmbeddingService) error {
	if s.vectorDB == nil {
		return fmt.Errorf("vector database is not configured")
	}
	if target.Dataset == nil {
		return fmt.Errorf("dataset is required")
	}
	if strings.TrimSpace(target.IndexNodeID) == "" {
		return fmt.Errorf("index node id is required")
	}
	if strings.TrimSpace(target.Content) == "" {
		return fmt.Errorf("content is required")
	}
	if embeddingService == nil {
		return fmt.Errorf("embedding service is not configured")
	}

	embeddings, err := embeddingService.EmbedTexts(ctx, []string{target.Content})
	if err != nil {
		return fmt.Errorf("failed to generate embedding: %w", err)
	}
	if len(embeddings) == 0 || len(embeddings[0]) == 0 {
		return fmt.Errorf("empty embedding vector for index node %s", target.IndexNodeID)
	}

	className := model.GenCollectionNameByID(target.Dataset.ID)
	if err := s.vectorDB.CreateClass(ctx, className, defaultSegmentVectorClassProperties()); err != nil {
		return fmt.Errorf("failed to ensure vector class %s: %w", className, err)
	}

	docHash := target.DocHash
	if docHash == "" {
		docHash = simpleHash(target.Content)
	}
	properties := map[string]interface{}{
		"text":        target.Content,
		"doc_id":      target.IndexNodeID,
		"doc_hash":    docHash,
		"document_id": target.DocumentID,
		"dataset_id":  target.Dataset.ID,
	}

	if err := s.vectorDB.StoreVector(ctx, target.IndexNodeID, className, properties, embeddings[0]); err != nil {
		return fmt.Errorf("failed to store vector %s: %w", target.IndexNodeID, err)
	}

	return nil
}

func (s *segmentServiceImpl) deleteSegmentVector(ctx context.Context, datasetID, indexNodeID string) error {
	if s.vectorDB == nil {
		return fmt.Errorf("vector database is not configured")
	}
	if strings.TrimSpace(datasetID) == "" {
		return fmt.Errorf("dataset id is required")
	}
	if strings.TrimSpace(indexNodeID) == "" {
		return nil
	}

	className := model.GenCollectionNameByID(datasetID)
	if err := s.vectorDB.DeleteVector(ctx, indexNodeID, className); err != nil {
		return fmt.Errorf("failed to delete vector %s: %w", indexNodeID, err)
	}

	return nil
}

func defaultSegmentVectorClassProperties() []map[string]interface{} {
	return []map[string]interface{}{
		{"name": "text", "dataType": []string{"text"}},
	}
}

// GetChunkByID
func (s *segmentServiceImpl) GetChunkByID(ctx context.Context, id string) (*model.DocumentSegment, error) {
	return s.chunkService.GetChunkByID(ctx, id)
}

// GetChildChunks
func (s *segmentServiceImpl) GetChildChunks(ctx context.Context, segmentID, documentID, datasetID string, page, limit int, keyword string) (*dto.ChildChunkListResponse, error) {
	childChunks, err := s.chunkService.GetChildChunksBySegmentID(ctx, segmentID)
	if err != nil {
		return nil, err
	}

	var childChunkResponses []dto.ChildChunkResponse
	for _, childChunk := range childChunks {
		childChunkResponses = append(childChunkResponses, dto.ChildChunkResponse{
			ID:            childChunk.ID,
			SegmentID:     childChunk.SegmentID,
			Content:       childChunk.Content,
			Position:      childChunk.Position,
			WordCount:     childChunk.WordCount,
			Type:          childChunk.Type,
			IndexNodeID:   childChunk.IndexNodeID,
			IndexNodeHash: childChunk.IndexNodeHash,
			CreatedAt:     childChunk.CreatedAt.Unix(),
			UpdatedAt:     childChunk.UpdatedAt.Unix(),
		})
	}

	filteredChildChunks := s.applyChildChunkFilters(childChunkResponses, keyword)
	paginatedChildChunks := s.applyChildChunkPagination(filteredChildChunks, page, limit)

	totalPages := int(len(filteredChildChunks) / limit)
	if len(filteredChildChunks)%limit > 0 {
		totalPages++
	}

	return &dto.ChildChunkListResponse{
		Data:       paginatedChildChunks,
		Total:      int64(len(filteredChildChunks)),
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
	}, nil
}

// CreateChildChunk
func (s *segmentServiceImpl) CreateChildChunk(ctx context.Context, childChunk *model.ChildChunk) (*dto.ChildChunkResponse, error) {
	maxPosition, err := s.chunkService.GetMaxChildChunkPosition(ctx, childChunk.DatasetID, childChunk.DocumentID, childChunk.SegmentID)
	if err != nil {
		return nil, err
	}

	childChunk.Position = maxPosition + 1

	// UUID
	indexNodeID := uuid.New().String()
	childChunk.IndexNodeID = &indexNodeID

	// todo: use a more secure hash algorithm
	hash := simpleHash(childChunk.Content)
	childChunk.IndexNodeHash = &hash

	if err := s.chunkService.CreateChildChunk(ctx, childChunk); err != nil {
		return nil, err
	}

	dataset, err := s.datasetRepo.GetByID(ctx, childChunk.DatasetID)
	if err != nil {
		_ = s.chunkService.DeleteChildChunkByID(ctx, childChunk.ID)
		return nil, fmt.Errorf("failed to get dataset for child chunk vector: %w", err)
	}
	if err := s.storeChildChunkVector(ctx, dataset, childChunk); err != nil {
		_ = s.chunkService.DeleteChildChunkByID(ctx, childChunk.ID)
		return nil, err
	}

	return childChunkResponse(childChunk), nil
}

// UpdateChildChunk
func (s *segmentServiceImpl) UpdateChildChunk(ctx context.Context, childChunk *model.ChildChunk) (*dto.ChildChunkResponse, error) {
	existingChildChunk, err := s.chunkService.GetChildChunkByID(ctx, childChunk.ID)
	if err != nil {
		return nil, err
	}
	dataset, err := s.datasetRepo.GetByID(ctx, childChunk.DatasetID)
	if err != nil {
		return nil, fmt.Errorf("failed to get dataset for child chunk vector: %w", err)
	}

	contentChanged := existingChildChunk.Content != childChunk.Content
	if contentChanged {
		if childChunk.IndexNodeID == nil || strings.TrimSpace(*childChunk.IndexNodeID) == "" {
			indexNodeID := uuid.New().String()
			childChunk.IndexNodeID = &indexNodeID
		}
		hash := simpleHash(childChunk.Content)
		childChunk.IndexNodeHash = &hash
		if existingChildChunk.IndexNodeID != nil && strings.TrimSpace(*existingChildChunk.IndexNodeID) != "" {
			if err := s.deleteSegmentVector(ctx, existingChildChunk.DatasetID, *existingChildChunk.IndexNodeID); err != nil {
				return nil, err
			}
		}
		if err := s.storeChildChunkVector(ctx, dataset, childChunk); err != nil {
			if restoreErr := s.storeChildChunkVector(ctx, dataset, existingChildChunk); restoreErr != nil {
				return nil, fmt.Errorf("failed to store updated child chunk vector: %w; restore error: %v", err, restoreErr)
			}
			return nil, err
		}
	}

	if err := s.chunkService.UpdateChildChunk(ctx, childChunk); err != nil {
		if contentChanged {
			if childChunk.IndexNodeID != nil && strings.TrimSpace(*childChunk.IndexNodeID) != "" {
				_ = s.deleteSegmentVector(ctx, childChunk.DatasetID, *childChunk.IndexNodeID)
			}
			if restoreErr := s.storeChildChunkVector(ctx, dataset, existingChildChunk); restoreErr != nil {
				return nil, fmt.Errorf("failed to update child chunk and restore vector: %w; restore error: %v", err, restoreErr)
			}
		}
		return nil, err
	}

	return childChunkResponse(childChunk), nil
}

// DeleteChildChunk
func (s *segmentServiceImpl) DeleteChildChunk(ctx context.Context, childChunkID string) error {
	childChunk, err := s.chunkService.GetChildChunkByID(ctx, childChunkID)
	if err != nil {
		return err
	}

	if childChunk.IndexNodeID != nil {
		if err := s.deleteSegmentVector(ctx, childChunk.DatasetID, *childChunk.IndexNodeID); err != nil {
			return err
		}
	}

	if err := s.chunkService.DeleteChildChunkByID(ctx, childChunkID); err != nil {
		dataset, datasetErr := s.datasetRepo.GetByID(ctx, childChunk.DatasetID)
		if datasetErr != nil {
			return fmt.Errorf("failed to delete child chunk after vector deletion: %w; failed to load dataset for vector restore: %v", err, datasetErr)
		}
		if restoreErr := s.storeChildChunkVector(ctx, dataset, childChunk); restoreErr != nil {
			return fmt.Errorf("failed to delete child chunk and restore vector: %w; restore error: %v", err, restoreErr)
		}
		return err
	}

	return nil
}

// GetChildChunkByID
func (s *segmentServiceImpl) GetChildChunkByID(ctx context.Context, childChunkID string) (*model.ChildChunk, error) {
	return s.chunkService.GetChildChunkByID(ctx, childChunkID)
}

func (s *segmentServiceImpl) storeChildChunkVector(ctx context.Context, dataset *model.Dataset, childChunk *model.ChildChunk) error {
	if childChunk == nil {
		return fmt.Errorf("child chunk is required")
	}
	if childChunk.IndexNodeID == nil || strings.TrimSpace(*childChunk.IndexNodeID) == "" {
		return fmt.Errorf("child chunk index node id is required")
	}

	docHash := ""
	if childChunk.IndexNodeHash != nil {
		docHash = *childChunk.IndexNodeHash
	}

	return s.storeSegmentVector(ctx, segmentVectorTarget{
		Dataset:     dataset,
		DocumentID:  childChunk.DocumentID,
		IndexNodeID: *childChunk.IndexNodeID,
		Content:     childChunk.Content,
		DocHash:     docHash,
	})
}

func childChunkResponse(childChunk *model.ChildChunk) *dto.ChildChunkResponse {
	return &dto.ChildChunkResponse{
		ID:            childChunk.ID,
		SegmentID:     childChunk.SegmentID,
		Content:       childChunk.Content,
		Position:      childChunk.Position,
		WordCount:     childChunk.WordCount,
		Type:          childChunk.Type,
		IndexNodeID:   childChunk.IndexNodeID,
		IndexNodeHash: childChunk.IndexNodeHash,
		CreatedAt:     childChunk.CreatedAt.Unix(),
		UpdatedAt:     childChunk.UpdatedAt.Unix(),
	}
}

// simpleHash
func simpleHash(content string) string {
	// todo : use a more secure hash algorithm
	h := sha256.New()
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}

// convertSegmentToResponse
func (s *segmentServiceImpl) convertSegmentToResponse(segment *model.DocumentSegment) dto.SegmentResponse {
	return dto.SegmentResponse{
		ID:            segment.ID,
		Position:      segment.Position,
		DocumentID:    segment.DocumentID,
		Content:       segment.Content,
		WordCount:     segment.WordCount,
		Tokens:        segment.Tokens,
		Status:        segment.Status,
		Enabled:       segment.Enabled,
		CreatedAt:     segment.CreatedAt.Unix(),
		HitCount:      segment.HitCount,
		IndexNodeID:   segment.IndexNodeID,
		IndexNodeHash: segment.IndexNodeHash,
		Answer:        segment.Answer,
	}
}

func (s *segmentServiceImpl) convertSegmentToDetailResponse(segment *model.DocumentSegment) dto.SegmentDetailResponse {
	childChunks, err := s.chunkService.GetChildChunksBySegmentID(context.Background(), segment.ID)
	if err != nil {
		logger.Error("Failed to load child chunks for segment: "+segment.ID, err)
		childChunks = []*model.ChildChunk{}
	}

	var childChunkResponses []dto.ChildChunkResponse
	for _, cc := range childChunks {
		childChunkResponses = append(childChunkResponses, dto.ChildChunkResponse{
			ID:        cc.ID,
			SegmentID: cc.SegmentID,
			Content:   cc.Content,
			Position:  cc.Position,
			WordCount: cc.WordCount,
			Type:      cc.Type,
			Score:     0.0,
			CreatedAt: cc.CreatedAt.Unix(),
			UpdatedAt: cc.UpdatedAt.Unix(),
		})
	}

	var keywords []string
	if segment.Keywords != nil {
		if keywordsArray, ok := segment.Keywords["keywords"]; ok {
			if keywordsList, ok := keywordsArray.([]interface{}); ok {
				for _, kw := range keywordsList {
					if kwStr, ok := kw.(string); ok {
						keywords = append(keywords, kwStr)
					}
				}
			}
		} else {
			for _, v := range segment.Keywords {
				if kwStr, ok := v.(string); ok {
					keywords = append(keywords, kwStr)
				}
			}
		}
	}

	return dto.SegmentDetailResponse{
		ID:            segment.ID,
		Position:      segment.Position,
		DocumentID:    segment.DocumentID,
		Content:       segment.Content,
		SignContent:   &segment.Content,
		Answer:        segment.Answer,
		WordCount:     segment.WordCount,
		Tokens:        segment.Tokens,
		Keywords:      keywords,
		IndexNodeID:   segment.IndexNodeID,
		IndexNodeHash: segment.IndexNodeHash,
		HitCount:      segment.HitCount,
		Enabled:       segment.Enabled,
		DisabledAt:    segment.DisabledAt,
		DisabledBy:    segment.DisabledBy,
		Status:        segment.Status,
		CreatedBy:     segment.CreatedBy,
		CreatedAt:     segment.CreatedAt,
		UpdatedAt:     segment.UpdatedAt,
		UpdatedBy:     segment.UpdatedBy,
		IndexingAt:    segment.IndexingAt,
		CompletedAt:   segment.CompletedAt,
		Error:         segment.Error,
		StoppedAt:     segment.StoppedAt,
		ChildChunks:   childChunkResponses,
	}
}

// applyDetailFilters
func (s *segmentServiceImpl) applyDetailFilters(segments []dto.SegmentDetailResponse, req *dto.SegmentListRequest) []dto.SegmentDetailResponse {
	var filtered []dto.SegmentDetailResponse

	for _, segment := range segments {
		if req.Enabled != "all" {
			if req.Enabled == "true" && !segment.Enabled {
				continue
			}
			if req.Enabled == "false" && segment.Enabled {
				continue
			}
		}

		if len(req.Status) > 0 {
			statusMatch := false
			for _, status := range req.Status {
				if segment.Status == status {
					statusMatch = true
					break
				}
			}
			if !statusMatch {
				continue
			}
		}

		if req.HitCountGte != nil && segment.HitCount < *req.HitCountGte {
			continue
		}

		if req.Keyword != "" {
			if !s.matchesKeyword(segment.Content, req.Keyword, req.SearchMethod) {
				continue
			}
		}

		filtered = append(filtered, segment)
	}

	if req.Keyword != "" {
		filtered = s.sortByRelevanceDetail(filtered, req.Keyword, req.SearchMethod)
	}

	return filtered
}

func (s *segmentServiceImpl) applyFilters(segments []dto.SegmentResponse, req *dto.SegmentListRequest) []dto.SegmentResponse {
	var filtered []dto.SegmentResponse

	for _, segment := range segments {
		if req.Enabled != "all" {
			if req.Enabled == "true" && !segment.Enabled {
				continue
			}
			if req.Enabled == "false" && segment.Enabled {
				continue
			}
		}

		if len(req.Status) > 0 {
			statusMatch := false
			for _, status := range req.Status {
				if segment.Status == status {
					statusMatch = true
					break
				}
			}
			if !statusMatch {
				continue
			}
		}

		if req.HitCountGte != nil && segment.HitCount < *req.HitCountGte {
			continue
		}

		if req.Keyword != "" {
			if !s.matchesKeyword(segment.Content, req.Keyword, req.SearchMethod) {
				continue
			}
		}

		filtered = append(filtered, segment)
	}

	return filtered
}

func (s *segmentServiceImpl) matchesKeyword(content, keyword, searchMethod string) bool {
	if keyword == "" {
		return true
	}

	score := s.calculateSimilarity(content, keyword, searchMethod)

	return score > 0.1
}

func (s *segmentServiceImpl) calculateSimilarity(content, query, searchMethod string) float64 {
	queryLower := strings.ToLower(s.normalizeText(query))
	contentLower := strings.ToLower(s.normalizeText(content))
	queryWords := s.splitIntoWords(queryLower)

	switch RetrievalMethod(searchMethod) {
	case SemanticSearch:
		return s.calculateTextSimilarity(queryLower, queryWords, contentLower)
	case FullTextSearch:
		return s.calculateFullTextSimilarity(queryLower, contentLower)
	case KeywordSearch:
		return s.calculateKeywordSimilarity(queryLower, queryWords, contentLower)
	default:
		return s.calculateTextSimilarity(queryLower, queryWords, contentLower)
	}
}

func (s *segmentServiceImpl) calculateTextSimilarity(queryLower string, queryWords []string, contentLower string) float64 {
	score := 0.0

	if strings.Contains(contentLower, queryLower) {
		score += 0.8
	}

	contentWords := s.splitIntoWords(contentLower)
	matchCount := 0

	for _, queryWord := range queryWords {
		if len(queryWord) < 2 {
			continue
		}
		for _, contentWord := range contentWords {
			if strings.Contains(contentWord, queryWord) || strings.Contains(queryWord, contentWord) {
				matchCount++
				break
			}
		}
	}

	if len(queryWords) > 0 {
		wordOverlapRatio := float64(matchCount) / float64(len(queryWords))
		score += wordOverlapRatio * 0.6
	}

	contentLength := len(contentWords)
	if contentLength > 0 && contentLength < 100 {
		score += 0.1
	}

	if score > 1.0 {
		score = 1.0
	}

	return score
}

func (s *segmentServiceImpl) calculateFullTextSimilarity(queryLower, contentLower string) float64 {
	score := 0.0

	if strings.Contains(contentLower, queryLower) {
		score += 0.9
	}

	queryWords := s.splitIntoWords(queryLower)
	contentWords := s.splitIntoWords(contentLower)

	if len(queryWords) > 1 {
		for i := 0; i <= len(contentWords)-len(queryWords); i++ {
			match := true
			for j, queryWord := range queryWords {
				if contentWords[i+j] != queryWord {
					match = false
					break
				}
			}
			if match {
				score += 0.8
				break
			}
		}
	}

	wordMatches := 0
	for _, queryWord := range queryWords {
		for _, contentWord := range contentWords {
			if contentWord == queryWord {
				wordMatches++
				break
			}
		}
	}

	if len(queryWords) > 0 {
		score += float64(wordMatches) / float64(len(queryWords)) * 0.6
	}

	if score > 1.0 {
		score = 1.0
	}

	return score
}

func (s *segmentServiceImpl) calculateKeywordSimilarity(queryLower string, queryWords []string, contentLower string) float64 {
	score := 0.0
	contentWords := s.splitIntoWords(contentLower)

	exactMatches := 0
	for _, queryWord := range queryWords {
		if len(queryWord) < 2 {
			continue
		}
		for _, contentWord := range contentWords {
			if contentWord == queryWord {
				exactMatches++
				break
			}
		}
	}

	if len(queryWords) > 0 {
		score += float64(exactMatches) / float64(len(queryWords)) * 0.8
	}

	partialMatches := 0
	for _, queryWord := range queryWords {
		if len(queryWord) < 2 {
			continue
		}
		for _, contentWord := range contentWords {
			if strings.Contains(contentWord, queryWord) || strings.Contains(queryWord, contentWord) {
				partialMatches++
				break
			}
		}
	}

	if len(queryWords) > 0 {
		score += float64(partialMatches) / float64(len(queryWords)) * 0.4
	}

	if score > 1.0 {
		score = 1.0
	}

	return score
}

func (s *segmentServiceImpl) sortByRelevance(segments []dto.SegmentResponse, keyword, searchMethod string) []dto.SegmentResponse {
	type scoredSegment struct {
		segment dto.SegmentResponse
		score   float64
	}

	var scoredSegments []scoredSegment
	for _, segment := range segments {
		score := s.calculateSimilarity(segment.Content, keyword, searchMethod)
		scoredSegments = append(scoredSegments, scoredSegment{
			segment: segment,
			score:   score,
		})
	}

	sort.Slice(scoredSegments, func(i, j int) bool {
		return scoredSegments[i].score > scoredSegments[j].score
	})

	var result []dto.SegmentResponse
	for _, scored := range scoredSegments {
		result = append(result, scored.segment)
	}

	return result
}

func (s *segmentServiceImpl) sortByRelevanceDetail(segments []dto.SegmentDetailResponse, keyword, searchMethod string) []dto.SegmentDetailResponse {
	type scoredSegment struct {
		segment dto.SegmentDetailResponse
		score   float64
	}

	var scoredSegments []scoredSegment
	for _, segment := range segments {
		score := s.calculateSimilarity(segment.Content, keyword, searchMethod)
		scoredSegments = append(scoredSegments, scoredSegment{
			segment: segment,
			score:   score,
		})
	}

	sort.Slice(scoredSegments, func(i, j int) bool {
		return scoredSegments[i].score > scoredSegments[j].score
	})

	var result []dto.SegmentDetailResponse
	for _, scored := range scoredSegments {
		result = append(result, scored.segment)
	}

	return result
}

func (s *segmentServiceImpl) normalizeText(text string) string {
	text = strings.TrimSpace(text)
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\t", " ")

	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}

	return text
}

func (s *segmentServiceImpl) splitIntoWords(text string) []string {
	var words []string
	var currentWord strings.Builder

	for _, r := range text {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			if currentWord.Len() > 0 {
				words = append(words, currentWord.String())
				currentWord.Reset()
			}
		} else if unicode.Is(unicode.Han, r) {
			if currentWord.Len() > 0 {
				words = append(words, currentWord.String())
				currentWord.Reset()
			}
			words = append(words, string(r))
		} else {
			currentWord.WriteRune(r)
		}
	}

	if currentWord.Len() > 0 {
		words = append(words, currentWord.String())
	}

	return words
}

func (s *segmentServiceImpl) applyDetailPagination(segments []dto.SegmentDetailResponse, page, limit int) []dto.SegmentDetailResponse {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}

	start := (page - 1) * limit
	end := start + limit

	if start >= len(segments) {
		return []dto.SegmentDetailResponse{}
	}

	if end > len(segments) {
		end = len(segments)
	}

	return segments[start:end]
}

func (s *segmentServiceImpl) applyPagination(segments []dto.SegmentResponse, page, limit int) []dto.SegmentResponse {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}

	start := (page - 1) * limit
	end := start + limit

	if start >= len(segments) {
		return []dto.SegmentResponse{}
	}

	if end > len(segments) {
		end = len(segments)
	}

	return segments[start:end]
}

type SegmentServiceImpl struct {
	chunkService ChunkService
}

func (s *SegmentServiceImpl) someMethod() {
	var err error
	logger.Error("Error message", err)
}

// applyChildChunkFilters
func (s *segmentServiceImpl) applyChildChunkFilters(childChunks []dto.ChildChunkResponse, keyword string) []dto.ChildChunkResponse {
	if keyword == "" {
		return childChunks
	}

	var filtered []dto.ChildChunkResponse
	for _, childChunk := range childChunks {
		if strings.Contains(childChunk.Content, keyword) {
			filtered = append(filtered, childChunk)
		}
	}
	return filtered
}

// applyChildChunkPagination
func (s *segmentServiceImpl) applyChildChunkPagination(childChunks []dto.ChildChunkResponse, page, limit int) []dto.ChildChunkResponse {
	start := (page - 1) * limit
	if start >= len(childChunks) {
		return []dto.ChildChunkResponse{}
	}

	end := start + limit
	if end > len(childChunks) {
		end = len(childChunks)
	}

	return childChunks[start:end]
}

// DocumentSegmentQuestion related methods

// CreateDocumentSegmentQuestion creates a new document segment question
func (s *segmentServiceImpl) CreateDocumentSegmentQuestion(ctx context.Context, req *dto.DocumentSegmentQuestionCreateRequest, userID, organizationID string) (*dto.DocumentSegmentQuestionResponse, error) {
	// Validate request
	if err := s.validateCreateQuestionRequest(req); err != nil {
		return nil, err
	}

	// Check permissions
	if err := s.checkSegmentPermission(ctx, req.SegmentID, userID); err != nil {
		return nil, err
	}

	// Get segment to verify it exists and get document/dataset IDs
	segment, err := s.documentRepo.GetDocumentSegmentByID(ctx, req.SegmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get segment: %w", err)
	}
	if segment == nil {
		return nil, fmt.Errorf("segment not found")
	}

	// Get document to verify it exists and get dataset ID
	document, err := s.documentRepo.GetByID(ctx, segment.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}
	if document == nil {
		return nil, fmt.Errorf("document not found")
	}

	// Create the question model
	question := &model.DocumentSegmentQuestion{
		ID:             uuid.New().String(),
		OrganizationID: organizationID,
		DatasetID:      document.DatasetID,
		DocumentID:     segment.DocumentID,
		SegmentID:      req.SegmentID,
		Question:       req.Question,
		CreatedBy:      userID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	// Save to database
	if err := s.documentRepo.CreateDocumentSegmentQuestion(ctx, question); err != nil {
		return nil, fmt.Errorf("failed to create question: %w", err)
	}

	// Create QA document for indexing
	qaDoc := dto.Document{
		PageContent: req.Question,
		Metadata: map[string]interface{}{
			"doc_id":      *segment.IndexNodeID,
			"doc_hash":    fmt.Sprintf("question/%s", question.ID),
			"segment_id":  req.SegmentID,
			"document_id": segment.DocumentID,
			"dataset_id":  document.DatasetID,
			"question_id": question.ID,
		},
	}
	dataset, err := s.datasetRepo.GetByID(ctx, document.DatasetID)
	if err != nil {
		logger.Error("Failed to get dataset for QA indexing", err)
	} else {
		embeddingService, err := s.buildEmbeddingService(ctx, dataset)
		if err != nil {
			logger.Error("Failed to get embedding service for QA indexing", err)
		} else {
			_, err = s.qaIndexProcessor.Load(ctx, dataset, dto.DocumentsToTransformedChunks([]dto.Document{qaDoc}), true, embeddingService, s.documentRepo, s.vectorDB)
			if err != nil {
				logger.Error("Failed to index QA document", err)
			}
		}
	}

	// Convert to response DTO
	return s.convertQuestionToResponse(question), nil
}

// GetDocumentSegmentQuestionByID retrieves a document segment question by ID
func (s *segmentServiceImpl) GetDocumentSegmentQuestionByID(ctx context.Context, questionID string) (*dto.DocumentSegmentQuestionResponse, error) {
	question, err := s.documentRepo.GetDocumentSegmentQuestionByID(ctx, questionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get question: %w", err)
	}
	if question == nil {
		return nil, fmt.Errorf("question not found")
	}

	return s.convertQuestionToResponse(question), nil
}

// ListDocumentSegmentQuestionsBySegment lists document segment questions by segment ID
func (s *segmentServiceImpl) ListDocumentSegmentQuestionsBySegment(ctx context.Context, segmentID string, req *dto.DocumentSegmentQuestionListRequest) (*dto.DocumentSegmentQuestionListResponse, error) {
	// Set default values
	page := req.Page
	if page <= 0 {
		page = 1
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	// Get questions from repository
	questions, total, err := s.documentRepo.ListDocumentSegmentQuestionsBySegmentID(ctx, segmentID, page, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list questions: %w", err)
	}

	// Convert to response DTOs
	var questionResponses []dto.DocumentSegmentQuestionResponse
	for _, question := range questions {
		questionResponses = append(questionResponses, *s.convertQuestionToResponse(question))
	}

	return &dto.DocumentSegmentQuestionListResponse{
		Data:    questionResponses,
		Total:   total,
		Page:    page,
		Limit:   limit,
		HasMore: int64(page*limit) < total,
	}, nil
}

// ListDocumentSegmentQuestionsByDocument lists document segment questions by document ID
func (s *segmentServiceImpl) ListDocumentSegmentQuestionsByDocument(ctx context.Context, documentID string, req *dto.DocumentSegmentQuestionListRequest) (*dto.DocumentSegmentQuestionListResponse, error) {
	// Set default values
	page := req.Page
	if page <= 0 {
		page = 1
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	// Get questions from repository
	questions, total, err := s.documentRepo.ListDocumentSegmentQuestionsByDocumentID(ctx, documentID, page, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list questions: %w", err)
	}

	// Convert to response DTOs
	var questionResponses []dto.DocumentSegmentQuestionResponse
	for _, question := range questions {
		questionResponses = append(questionResponses, *s.convertQuestionToResponse(question))
	}

	return &dto.DocumentSegmentQuestionListResponse{
		Data:    questionResponses,
		Total:   total,
		Page:    page,
		Limit:   limit,
		HasMore: int64(page*limit) < total,
	}, nil
}

// ListDocumentSegmentQuestionsByDataset lists document segment questions by dataset ID
func (s *segmentServiceImpl) ListDocumentSegmentQuestionsByDataset(ctx context.Context, datasetID string, req *dto.DocumentSegmentQuestionListRequest) (*dto.DocumentSegmentQuestionListResponse, error) {
	// Set default values
	page := req.Page
	if page <= 0 {
		page = 1
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	// Get questions from repository
	questions, total, err := s.documentRepo.ListDocumentSegmentQuestionsByDatasetID(ctx, datasetID, page, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list questions: %w", err)
	}

	// Convert to response DTOs
	var questionResponses []dto.DocumentSegmentQuestionResponse
	for _, question := range questions {
		questionResponses = append(questionResponses, *s.convertQuestionToResponse(question))
	}

	return &dto.DocumentSegmentQuestionListResponse{
		Data:    questionResponses,
		Total:   total,
		Page:    page,
		Limit:   limit,
		HasMore: int64(page*limit) < total,
	}, nil
}

// UpdateDocumentSegmentQuestion updates a document segment question
func (s *segmentServiceImpl) UpdateDocumentSegmentQuestion(ctx context.Context, questionID string, req *dto.DocumentSegmentQuestionUpdateRequest, userID string) (*dto.DocumentSegmentQuestionResponse, error) {
	// Get existing question
	question, err := s.documentRepo.GetDocumentSegmentQuestionByID(ctx, questionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get question: %w", err)
	}
	if question == nil {
		return nil, fmt.Errorf("question not found")
	}

	// Check permissions
	if err := s.checkSegmentPermission(ctx, question.SegmentID, userID); err != nil {
		return nil, err
	}

	// If the question content is the same, no need to update
	if req.Question != "" && req.Question == question.Question {
		// Convert to response DTO directly without updating
		return s.convertQuestionToResponse(question), nil
	}

	// Update fields
	if req.Question != "" {
		question.Question = req.Question
	}
	question.UpdatedBy = &userID

	// Save to database
	if err := s.documentRepo.UpdateDocumentSegmentQuestion(ctx, question); err != nil {
		return nil, fmt.Errorf("failed to update question: %w", err)
	}

	// Update vector database - delete old and recreate with new content
	// Only proceed if question content actually changed
	if req.Question != "" && req.Question != question.Question {
		go func() {
			// Create a background context since the original context might be cancelled
			backgroundCtx := context.Background()

			// Get segment to verify it exists and get document/dataset IDs
			segment, err := s.documentRepo.GetDocumentSegmentByID(backgroundCtx, question.SegmentID)
			if err != nil {
				logger.Error("Failed to get segment for QA indexing", err)
				return
			}
			if segment == nil || segment.IndexNodeID == nil {
				logger.Error("Segment not found or missing index node ID", nil)
				return
			}

			// Get document to verify it exists and get dataset ID
			document, err := s.documentRepo.GetByID(backgroundCtx, segment.DocumentID)
			if err != nil {
				logger.Error("Failed to get document for QA indexing", err)
				return
			}
			if document == nil {
				logger.Error("Document not found", nil)
				return
			}

			// Delete the old vector data
			dataset, err := s.datasetRepo.GetByID(backgroundCtx, document.DatasetID)
			if err != nil {
				logger.Error("Failed to get dataset for QA indexing", err)
				return
			}

			// Delete old vector data
			if questionsOnlyCleaner, ok := s.qaIndexProcessor.(indexing.QAIndexProcessorExtension); ok {
				// Use the extension method to clean only questions
				err = questionsOnlyCleaner.CleanQuestionsOnly(backgroundCtx, dataset, []string{question.ID}, true, true)
			} else {
				// Fallback to standard clean method
				err = s.qaIndexProcessor.Clean(backgroundCtx, dataset, []string{question.ID}, true, true)
			}
			if err != nil {
				logger.Error("Failed to delete old QA vector data", err)
				// Continue anyway, as we still want to try to create the new data
			}

			// Create new vector data with updated content
			qaDoc := dto.Document{
				PageContent: question.Question,
				Metadata: map[string]interface{}{
					"doc_id":      *segment.IndexNodeID,
					"doc_hash":    fmt.Sprintf("question/%s", question.ID),
					"segment_id":  question.SegmentID,
					"document_id": segment.DocumentID,
					"dataset_id":  document.DatasetID,
					"question_id": question.ID,
				},
			}

			embeddingService, err := s.buildEmbeddingService(backgroundCtx, dataset)
			if err != nil {
				logger.Error("Failed to get embedding service for QA indexing", err)
				return
			}

			_, err = s.qaIndexProcessor.Load(backgroundCtx, dataset, dto.DocumentsToTransformedChunks([]dto.Document{qaDoc}), true, embeddingService, s.documentRepo, s.vectorDB)
			if err != nil {
				logger.Error("Failed to index updated QA document", err)
			}
		}()
	}

	// Convert to response DTO
	return s.convertQuestionToResponse(question), nil
}

// DeleteDocumentSegmentQuestion deletes a document segment question
func (s *segmentServiceImpl) DeleteDocumentSegmentQuestion(ctx context.Context, questionID string) error {
	// Get existing question
	question, err := s.documentRepo.GetDocumentSegmentQuestionByID(ctx, questionID)
	if err != nil {
		return fmt.Errorf("failed to get question: %w", err)
	}
	if question == nil {
		return fmt.Errorf("question not found")
	}

	// TODO: Check permissions

	// Delete from database
	if err := s.documentRepo.DeleteDocumentSegmentQuestion(ctx, questionID); err != nil {
		return fmt.Errorf("failed to delete question: %w", err)
	}

	dataset, err := s.datasetRepo.GetByID(ctx, question.DatasetID)
	if err != nil {
		logger.Error("Failed to get dataset for QA indexing", err)
	} else {
		// Check if the QA index processor implements the extension interface for cleaning questions only
		if questionsOnlyCleaner, ok := s.qaIndexProcessor.(indexing.QAIndexProcessorExtension); ok {
			// Use the extension method to clean only questions
			err = questionsOnlyCleaner.CleanQuestionsOnly(ctx, dataset, []string{question.ID}, true, true)
		} else {
			// Fallback to standard clean method
			err = s.qaIndexProcessor.Clean(ctx, dataset, []string{question.ID}, true, true)
		}
		if err != nil {
			logger.Error("Failed to clean QA document", err)
		}
	}

	return nil
}

// DeleteDocumentSegmentQuestionsBySegment deletes all document segment questions for a segment
func (s *segmentServiceImpl) DeleteDocumentSegmentQuestionsBySegment(ctx context.Context, segmentID string) error {
	// TODO: Check permissions

	// Delete from database
	if err := s.documentRepo.DeleteDocumentSegmentQuestionsBySegmentID(ctx, segmentID); err != nil {
		return fmt.Errorf("failed to delete questions by segment: %w", err)
	}

	return nil
}

// DeleteDocumentSegmentQuestionsByDocument deletes all document segment questions for a document
func (s *segmentServiceImpl) DeleteDocumentSegmentQuestionsByDocument(ctx context.Context, documentID string) error {
	// TODO: Check permissions

	// Delete from database
	if err := s.documentRepo.DeleteDocumentSegmentQuestionsByDocumentID(ctx, documentID); err != nil {
		return fmt.Errorf("failed to delete questions by document: %w", err)
	}

	return nil
}

// DeleteDocumentSegmentQuestionsByDataset deletes all document segment questions for a dataset
func (s *segmentServiceImpl) DeleteDocumentSegmentQuestionsByDataset(ctx context.Context, datasetID string) error {
	// TODO: Check permissions

	// Delete from database
	if err := s.documentRepo.DeleteDocumentSegmentQuestionsByDatasetID(ctx, datasetID); err != nil {
		return fmt.Errorf("failed to delete questions by dataset: %w", err)
	}

	return nil
}

// BatchCreateDocumentSegmentQuestions creates multiple document segment questions
func (s *segmentServiceImpl) BatchCreateDocumentSegmentQuestions(ctx context.Context, req *dto.DocumentSegmentQuestionBatchCreateRequest, userID, organizationID string, segmentID string) (*dto.DocumentSegmentQuestionBatchCreateResponse, error) {
	// Validate request
	if err := s.validateBatchCreateQuestionRequest(req); err != nil {
		return nil, err
	}

	// TODO: Check permissions for all segments

	// Get segment to verify it exists and get document/dataset IDs
	segment, err := s.documentRepo.GetDocumentSegmentByID(ctx, segmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get segment %s: %w", segmentID, err)
	}
	if segment == nil {
		return nil, fmt.Errorf("segment %s not found", segmentID)
	}

	// Get document to verify it exists and get dataset ID
	document, err := s.documentRepo.GetByID(ctx, segment.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get document %s: %w", segment.DocumentID, err)
	}
	if document == nil {
		return nil, fmt.Errorf("document %s not found", segment.DocumentID)
	}

	// Create question models
	var questions []*model.DocumentSegmentQuestion
	for _, questionReq := range req.Questions {
		question := &model.DocumentSegmentQuestion{
			ID:             uuid.New().String(),
			OrganizationID: organizationID,
			DatasetID:      document.DatasetID,
			DocumentID:     segment.DocumentID,
			SegmentID:      segmentID,
			Question:       questionReq.Question,
			CreatedBy:      userID,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		questions = append(questions, question)
	}

	// Save to database
	if err := s.documentRepo.BatchCreateDocumentSegmentQuestions(ctx, questions); err != nil {
		return nil, fmt.Errorf("failed to batch create questions: %w", err)
	}

	// Convert to response DTOs
	var questionResponses []dto.DocumentSegmentQuestionResponse
	for _, question := range questions {
		questionResponses = append(questionResponses, *s.convertQuestionToResponse(question))
	}

	// Asynchronously index the questions to avoid blocking the API
	go func() {
		// Create a background context since the original context might be cancelled
		backgroundCtx := context.Background()

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

		dataset, err := s.datasetRepo.GetByID(backgroundCtx, document.DatasetID)
		if err != nil {
			logger.Error("Failed to get dataset for QA indexing", err)
			return
		}

		embeddingService, err := s.buildEmbeddingService(backgroundCtx, dataset)
		if err != nil {
			logger.Error("Failed to get embedding service for QA indexing", err)
			return
		}

		_, err = s.qaIndexProcessor.Load(backgroundCtx, dataset, dto.DocumentsToTransformedChunks(qaDocs), true, embeddingService, s.documentRepo, s.vectorDB)
		if err != nil {
			logger.Error("Failed to index QA documents", err)
		}
	}()

	return &dto.DocumentSegmentQuestionBatchCreateResponse{
		Questions: questionResponses,
		Count:     len(questionResponses),
	}, nil
}

// GenerateQuestionsForSegment generates mock questions for a segment
func (s *segmentServiceImpl) GenerateQuestionsForSegment(ctx context.Context, segmentID string, count int, userID, organizationID string, model *dto.ModelSpec) (*dto.DocumentSegmentQuestionBatchCreateResponse, error) {
	// Get segment to verify it exists
	segment, err := s.documentRepo.GetDocumentSegmentByID(ctx, segmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get segment: %w", err)
	}
	if segment == nil {
		return nil, fmt.Errorf("segment not found")
	}

	// Get document to verify it exists and get dataset ID
	document, err := s.documentRepo.GetByID(ctx, segment.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}
	if document == nil {
		return nil, fmt.Errorf("document not found")
	}

	// TODO: Check permissions

	// Generate questions using LLM (optional model selection)
	questions, err := s.generateQuestionsWithLLM(ctx, segment.Content, segmentID, segment.DocumentID, document.DatasetID, count, userID, organizationID, model)
	if err != nil {
		return nil, fmt.Errorf("failed to generate questions with LLM: %w", err)
	}

	// Save to database
	if err := s.documentRepo.BatchCreateDocumentSegmentQuestions(ctx, questions); err != nil {
		return nil, fmt.Errorf("failed to batch create questions: %w", err)
	}

	// Convert to response DTOs
	var questionResponses []dto.DocumentSegmentQuestionResponse
	for _, question := range questions {
		questionResponses = append(questionResponses, *s.convertQuestionToResponse(question))
	}

	// Asynchronously index the questions to avoid blocking the API
	go func() {
		// Create a background context since the original context might be cancelled
		backgroundCtx := context.Background()

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

		dataset, err := s.datasetRepo.GetByID(backgroundCtx, document.DatasetID)
		if err != nil {
			logger.Error("Failed to get dataset for QA indexing", err)
			return
		}

		embeddingService, err := s.buildEmbeddingService(backgroundCtx, dataset)
		if err != nil {
			logger.Error("Failed to get embedding service for QA indexing", err)
			return
		}

		_, err = s.qaIndexProcessor.Load(backgroundCtx, dataset, dto.DocumentsToTransformedChunks(qaDocs), true, embeddingService, s.documentRepo, s.vectorDB)
		if err != nil {
			logger.Error("Failed to index QA documents", err)
		}
	}()

	return &dto.DocumentSegmentQuestionBatchCreateResponse{
		Questions: questionResponses,
		Count:     len(questionResponses),
	}, nil
}

// generateQuestionsWithLLM generates questions using a language model
func (s *segmentServiceImpl) generateQuestionsWithLLM(ctx context.Context, content, segmentID, documentID, datasetID string, count int, userID, tenantID string, model *dto.ModelSpec) ([]*model.DocumentSegmentQuestion, error) {
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

	resolvedModel, err := llmruntime.NewModelResolver(s.defaultModelSvc).Resolve(
		ctx,
		tenantID,
		"",
		func() string {
			if model != nil {
				return model.Name
			}
			return ""
		}(),
		shared_model.ModelTypeLLM,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve chat model: %w", err)
	}
	resp, err := s.llmClient.Chat(ctx, tenantID, &llmadapter.ChatRequest{
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
	questions := s.parseGeneratedQuestions(generatedContent, segmentID, documentID, datasetID, count, userID, tenantID)

	// If parsing failed to produce questions, fall back to mock questions
	if len(questions) == 0 {
		// return s.generateMockQuestions(content, segmentID, documentID, datasetID, count, userID, tenantID), nil
		return nil, fmt.Errorf("failed to parse generated questions")
	}

	// Return the generated questions (without saving to database)
	return questions, nil
}

// parseGeneratedQuestions parses the LLM generated content into questions
func (s *segmentServiceImpl) parseGeneratedQuestions(content, segmentID, documentID, datasetID string, count int, userID, tenantID string) []*model.DocumentSegmentQuestion {
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

	// If we don't have enough questions, fall back to mock questions for the remainder
	// if len(questions) < count {
	// 	mockQuestions := s.generateMockQuestions("", segmentID, documentID, datasetID, count-len(questions), userID, tenantID)
	// 	questions = append(questions, mockQuestions...)
	// }

	return questions
}

// generateMockQuestions generates mock questions when LLM is not available or fails
func (s *segmentServiceImpl) generateMockQuestions(content, segmentID, documentID, datasetID string, count int, userID, tenantID string) []*model.DocumentSegmentQuestion {
	var questions []*model.DocumentSegmentQuestion

	mockQuestions := []string{
		"What is the main topic of this segment?",
		"What key information does this segment provide?",
		"How does this segment relate to the overall document?",
		"What are the important details mentioned in this segment?",
		"Can you summarize the content of this segment?",
		"What questions might a user have about this content?",
		"What is the purpose of this section?",
		"Are there any specific terms or concepts explained here?",
		"How could this information be useful?",
		"What actions might a reader take after reading this?",
	}

	actualCount := count
	if actualCount > len(mockQuestions) {
		actualCount = len(mockQuestions)
	}

	for i := 0; i < actualCount; i++ {
		question := &model.DocumentSegmentQuestion{
			ID:             uuid.New().String(),
			OrganizationID: tenantID,
			DatasetID:      datasetID,
			DocumentID:     documentID,
			SegmentID:      segmentID,
			Question:       mockQuestions[i],
			CreatedBy:      userID,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		questions = append(questions, question)
	}

	return questions
}

// RandomDocumentSegmentQuestionsByDataset randomly selects a specified number of questions from a dataset
func (s *segmentServiceImpl) RandomDocumentSegmentQuestionsByDataset(ctx context.Context, datasetID string, limit int) ([]dto.DocumentSegmentQuestionResponse, error) {
	questions, err := s.documentRepo.RandomDocumentSegmentQuestionsByDatasetID(ctx, datasetID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get random questions: %w", err)
	}

	// Convert to response DTOs
	var questionResponses []dto.DocumentSegmentQuestionResponse
	for _, question := range questions {
		questionResponses = append(questionResponses, *s.convertQuestionToResponse(question))
	}

	return questionResponses, nil
}

// GetDocumentSegmentQuestionCountByDataset returns the total count of questions for a dataset
func (s *segmentServiceImpl) GetDocumentSegmentQuestionCountByDataset(ctx context.Context, datasetID string) (int64, error) {
	count, err := s.documentRepo.GetDocumentSegmentQuestionCountByDatasetID(ctx, datasetID)
	if err != nil {
		return 0, fmt.Errorf("failed to get question count: %w", err)
	}
	return count, nil
}

// validateCreateQuestionRequest validates the create question request
func (s *segmentServiceImpl) validateCreateQuestionRequest(req *dto.DocumentSegmentQuestionCreateRequest) error {
	if req == nil {
		return fmt.Errorf("request cannot be nil")
	}
	if req.SegmentID == "" {
		return fmt.Errorf("segment_id is required")
	}
	if req.Question == "" {
		return fmt.Errorf("question is required")
	}
	return nil
}

// validateBatchCreateQuestionRequest validates the batch create question request
func (s *segmentServiceImpl) validateBatchCreateQuestionRequest(req *dto.DocumentSegmentQuestionBatchCreateRequest) error {
	if req == nil {
		return fmt.Errorf("request cannot be nil")
	}
	if len(req.Questions) == 0 {
		return fmt.Errorf("at least one question is required")
	}
	if len(req.Questions) > 100 {
		return fmt.Errorf("maximum 100 questions allowed in a batch")
	}
	return nil
}

// checkSegmentPermission checks if the user has permission to modify questions for the segment
func (s *segmentServiceImpl) checkSegmentPermission(ctx context.Context, segmentID, userID string) error {
	// Get segment to verify it exists and get document/dataset IDs
	segment, err := s.documentRepo.GetDocumentSegmentByID(ctx, segmentID)
	if err != nil {
		return fmt.Errorf("failed to get segment: %w", err)
	}
	if segment == nil {
		return fmt.Errorf("segment not found")
	}

	// Get document to verify it exists and get dataset ID
	document, err := s.documentRepo.GetByID(ctx, segment.DocumentID)
	if err != nil {
		return fmt.Errorf("failed to get document: %w", err)
	}
	if document == nil {
		return fmt.Errorf("document not found")
	}

	// TODO: Check if user has permission to access this tenant
	// hasPermission := s.tenantSvc.CheckPermission(ctx, document.TenantID, userID, "write")
	// if !hasPermission {
	// 	return fmt.Errorf("access denied")
	// }

	return nil
}

// convertQuestionToResponse converts a DocumentSegmentQuestion model to response DTO
func (s *segmentServiceImpl) convertQuestionToResponse(question *model.DocumentSegmentQuestion) *dto.DocumentSegmentQuestionResponse {
	response := &dto.DocumentSegmentQuestionResponse{
		ID:             question.ID,
		OrganizationID: question.OrganizationID,
		DatasetID:      question.DatasetID,
		DocumentID:     question.DocumentID,
		SegmentID:      question.SegmentID,
		Question:       question.Question,
		CreatedBy:      question.CreatedBy,
		CreatedAt:      question.CreatedAt.Unix(),
		UpdatedAt:      question.UpdatedAt.Unix(),
	}

	if question.UpdatedBy != nil {
		response.UpdatedBy = *question.UpdatedBy
	}

	return response
}

func (s *segmentServiceImpl) buildEmbeddingService(ctx context.Context, dataset *model.Dataset) (embedding.EmbeddingService, error) {
	if dataset == nil {
		return nil, fmt.Errorf("dataset is nil")
	}

	resolvedModel, err := llmruntime.NewModelResolver(s.defaultModelSvc).ResolveFromPointers(
		ctx,
		dataset.WorkspaceID,
		dataset.EmbeddingModelProvider,
		dataset.EmbeddingModel,
		shared_model.ModelTypeEmbedding,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve embedding model: %w", err)
	}

	accountID := dataset.CreatedBy
	gatewaySvc, err := indexing.NewGatewayEmbeddingService(s.llmClient, accountID, dataset.ID, "dataset", resolvedModel.Model, dataset.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to build gateway embedding service: %w", err)
	}

	logger.Info("Using gateway embedding service for segment service", map[string]interface{}{
		"dataset_id": dataset.ID,
		"model":      resolvedModel.Model,
	})
	return gatewaySvc, nil
}
