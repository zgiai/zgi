package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	"github.com/zgiai/zgi/api/internal/modules/dataset/indexing"
	dataset_model "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	dataset_repository "github.com/zgiai/zgi/api/internal/modules/dataset/repository"
	"github.com/zgiai/zgi/api/internal/modules/dataset/retrieval"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmruntime "github.com/zgiai/zgi/api/internal/modules/llm/runtime"
	shared_model "github.com/zgiai/zgi/api/internal/modules/shared/model"
	"github.com/zgiai/zgi/api/internal/observability"
	"github.com/zgiai/zgi/api/pkg/embedding"
	"github.com/zgiai/zgi/api/pkg/errors"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/tokenization"
	"github.com/zgiai/zgi/api/pkg/vectordb"
	"gorm.io/gorm"
)

type HitTestingService interface {
	Retrieve(ctx context.Context, dataset *dataset_model.Dataset, query string, accountID string, retrievalModel map[string]interface{}, externalRetrievalModel map[string]interface{}, limit int, source string, queryType string, retrievalMode string, recordHistory bool) (*dto.HitTestingResponse, error)
	ExternalRetrieve(ctx context.Context, dataset *dataset_model.Dataset, query string, accountID string, externalRetrievalModel map[string]interface{}) (*dto.HitTestingResponse, error)
	HitTestingArgsCheck(args *dto.HitTestingRequest) error
	EscapeQueryForSearch(query string) string
}

type hitTestingService struct {
	datasetRepo         dataset_repository.DatasetRepository
	datasetQueryService DatasetQueryService
	documentRepo        dataset_repository.DocumentRepository
	vectorClient        *vectordb.WeaviateClient
	embeddingService    embedding.EmbeddingService
	retrievalService    *RetrievalService
	config              *config.Config
	db                  *gorm.DB
	defaultModelSvc     llmdefaultservice.DefaultModelService
	llmClient           llmclient.LLMClient
}

// NewHitTestingService creates a new HitTestingService.
// The llmClient should be obtained from the DI container (ServiceContainer.GetLLMClient()).
func NewHitTestingService(
	datasetRepo dataset_repository.DatasetRepository,
	datasetQueryService DatasetQueryService,
	documentRepo dataset_repository.DocumentRepository,
	vectorClient *vectordb.WeaviateClient,
	cfg *config.Config,
	defaultModelSvc llmdefaultservice.DefaultModelService,
	db *gorm.DB,
	llmClient llmclient.LLMClient,
	graphFlowService *graphflow.Service,
) HitTestingService {
	// Resolve the existing embedding model choice, but execute it via gateway only.
	embeddingFactory := func(dataset *dataset_model.Dataset) embedding.EmbeddingService {
		return buildEmbeddingServiceForHitTesting(llmClient, dataset, defaultModelSvc)
	}

	// Initialize vector retrieval service with a placeholder embedding service.
	// The actual embedding service is set per request via the factory above.
	var vectorRetrieval *retrieval.VectorRetrievalService
	if vectorClient != nil {
		vectorRetrieval = retrieval.NewVectorRetrievalService(nil, vectorClient, "")
		logger.Info("Vector retrieval service initialized successfully", nil)
	} else {
		logger.Warn("Vector retrieval service unavailable: vector client not initialized", nil)
	}

	tokenizer := tokenization.NewTokenizationService()
	keywordRetrieval := retrieval.NewKeywordRetrievalService(tokenizer)
	fullTextRetrieval := retrieval.NewFullTextRetrievalService(tokenizer, 1.5, 0.75, vectorClient)
	hybridRetrieval := retrieval.NewHybridRetrievalService(vectorRetrieval, keywordRetrieval, fullTextRetrieval)
	rerankService := retrieval.NewRerankService()

	// Use the factory-enabled retrieval service
	retrievalService := NewRetrievalServiceWithEmbeddingFactory(
		documentRepo,
		vectorRetrieval,
		keywordRetrieval,
		fullTextRetrieval,
		hybridRetrieval,
		rerankService,
		defaultModelSvc,
		vectorClient,
		graphFlowService,
		embeddingFactory,
	)
	retrievalService.SetLLMClient(llmClient)

	logger.Info("HitTestingService initialization completed", map[string]interface{}{
		"vector_retrieval_available":    vectorRetrieval != nil,
		"keyword_retrieval_available":   keywordRetrieval != nil,
		"full_text_retrieval_available": fullTextRetrieval != nil,
		"hybrid_retrieval_available":    hybridRetrieval != nil,
		"rerank_service_available":      rerankService != nil,
		"gateway_embedding_enabled":     llmClient != nil,
	})

	return &hitTestingService{
		datasetRepo:         datasetRepo,
		datasetQueryService: datasetQueryService,
		documentRepo:        documentRepo,
		vectorClient:        vectorClient,
		embeddingService:    nil,
		retrievalService:    retrievalService,
		config:              cfg,
		db:                  db,
		defaultModelSvc:     defaultModelSvc,
		llmClient:           llmClient,
	}
}

// buildEmbeddingServiceForHitTesting resolves the existing embedding model choice and executes it via gateway.
func buildEmbeddingServiceForHitTesting(
	llmClient llmclient.LLMClient,
	dataset *dataset_model.Dataset,
	defaultModelSvc llmdefaultservice.DefaultModelService,
) embedding.EmbeddingService {
	if dataset == nil {
		logger.Warn("Dataset is nil, unable to build hit testing embedding service", nil)
		return nil
	}
	if llmClient == nil {
		logger.Warn("LLM client is nil, unable to build hit testing embedding service", map[string]interface{}{
			"dataset_id": dataset.ID,
		})
		return nil
	}

	resolvedModel, err := llmruntime.NewModelResolver(defaultModelSvc).ResolveFromPointers(
		context.Background(),
		dataset.OrganizationID,
		dataset.EmbeddingModelProvider,
		dataset.EmbeddingModel,
		shared_model.ModelTypeEmbedding,
	)
	if err != nil {
		logger.Warn("Failed to resolve embedding model for hit testing", map[string]interface{}{
			"dataset_id": dataset.ID,
			"error":      err.Error(),
		})
		return nil
	}

	accountID := dataset.CreatedBy
	if accountID == "" {
		accountID = dataset.WorkspaceID
	}

	gatewaySvc, err := indexing.NewGatewayEmbeddingService(llmClient, accountID, dataset.ID, "dataset", resolvedModel.Model, dataset.WorkspaceID)
	if err != nil {
		logger.Warn("Failed to build gateway embedding service for hit testing", map[string]interface{}{
			"dataset_id": dataset.ID,
			"model":      resolvedModel.Model,
			"error":      err.Error(),
		})
		return nil
	}

	logger.Info("Using gateway embedding service for hit testing", map[string]interface{}{
		"dataset_id": dataset.ID,
		"model":      resolvedModel.Model,
	})
	return gatewaySvc
}

// Retrieve performs dataset hit testing retrieval
func (s *hitTestingService) Retrieve(ctx context.Context, dataset *dataset_model.Dataset, query string, accountID string, retrievalModel map[string]interface{}, externalRetrievalModel map[string]interface{}, limit int, source string, queryType string, retrievalMode string, recordHistory bool) (*dto.HitTestingResponse, error) {
	start := time.Now()
	logger.Info("HitTesting started", map[string]interface{}{
		"dataset_id": dataset.ID,
		"account_id": accountID,
		"query":      query,
	})

	// Check if dataset is external provider
	if dataset.Provider == "external" {
		return s.ExternalRetrieve(ctx, dataset, query, accountID, externalRetrievalModel)
	}

	// Check if dataset has documents and segments
	documentCount, err := s.documentRepo.GetDocumentCount(ctx, dataset.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get document count: %w", err)
	}

	segmentCount, err := s.documentRepo.GetSegmentCount(ctx, dataset.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get segment count: %w", err)
	}

	// Debug: Print counts to understand the issue
	logger.Debug("HitTesting doc/segment count", map[string]interface{}{
		"dataset_id":     dataset.ID,
		"document_count": documentCount,
		"segment_count":  segmentCount,
	})

	if documentCount == 0 || segmentCount == 0 {
		// Return empty response with query content
		response := &dto.HitTestingResponse{
			Query: dto.HitTestingQueryResponse{
				Content: query,
				TSNEPosition: map[string]interface{}{
					"x": 0.0,
					"y": 0.0,
				},
			},
			Records: []dto.HitTestingRecordResponse{},
		}

		if recordHistory {
			// Save query to database using DatasetQueryService
			hitCount := 0
			createReq := &CreateDatasetQueryRequest{
				DatasetID:     dataset.ID,
				Content:       query,
				Source:        source,
				SourceAppID:   nil,
				CreatedByRole: "account",
				CreatedBy:     accountID,
				Results:       response,
				ElapsedTime:   float64Ptr(0),
				HitCount:      &hitCount,
				QueryType:     queryType,
			}

			if _, err := s.datasetQueryService.CreateDatasetQuery(ctx, createReq); err != nil {
				logger.Error("Failed to save dataset query", err)
			}
		}

		return response, nil
	}

	// Get retrieval parameters
	options := s.getRetrievalOptions(ctx, retrievalModel, dataset)
	options.RetrievalMode = retrievalMode

	if options.TopK > limit {
		options.TopK = limit
	}

	// Log retrieval parameters for observability
	logger.Info("HitTesting retrieval params", map[string]interface{}{
		"dataset_id":              dataset.ID,
		"search_method":           options.SearchMethod,
		"top_k":                   options.TopK,
		"score_threshold_enabled": options.ScoreThresholdEnabled,
		"score_threshold":         options.ScoreThreshold,
		"reranking_enable":        options.RerankingEnable,
		"reranking_mode":          options.RerankingMode,
		"has_rerank_model":        options.RerankingModel != nil,
	})

	// Execute retrieval
	records, execution, err := s.retrievalService.Retrieve(ctx, dataset, query, options)
	if err != nil {
		logger.Error("HitTesting retrieval failed", fmt.Errorf("dataset_id: %s, error: %s", dataset.ID, err.Error()))
		return nil, err
	}

	// If retrieval service is unavailable or no results, use fallback retrieval method
	// if len(records) == 0 {
	// 	logger.Info("Initiating fallback to text-based search", map[string]interface{}{
	// 		"dataset_id": dataset.ID,
	// 	})
	// 	records, err = s.fallbackRetrieve(ctx, dataset, query, options)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// }

	elapsed := time.Since(start)

	// Calculate score statistics
	var minScore, maxScore, avgScore float64
	if len(records) > 0 {
		minScore = records[0].Score
		maxScore = records[0].Score
		totalScore := 0.0
		for _, record := range records {
			if record.Score < minScore {
				minScore = record.Score
			}
			if record.Score > maxScore {
				maxScore = record.Score
			}
			totalScore += record.Score
		}
		avgScore = totalScore / float64(len(records))
	}

	logger.Info("HitTesting finished", map[string]interface{}{
		"dataset_id":    dataset.ID,
		"account_id":    accountID,
		"query":         query,
		"search_method": options.SearchMethod,
		"hit_count":     len(records),
		"elapsed_ms":    elapsed.Milliseconds(),
		"score_min":     minScore,
		"score_max":     maxScore,
		"score_avg":     avgScore,
	})

	response := &dto.HitTestingResponse{
		Query: dto.HitTestingQueryResponse{
			Content: query,
		},
		Records:        records,
		ElapsedTime:    float64(elapsed.Microseconds()) / 1000.0,
		GraphExecution: execution,
	}
	if err := normalizeHitTestingResponseKnowledgeImageURLs(response, config.Current().App.FilesURL); err != nil {
		return nil, err
	}

	if recordHistory {
		// Save query and results to database using DatasetQueryService
		hitCount := len(records)
		createReq := &CreateDatasetQueryRequest{
			DatasetID:     dataset.ID,
			Content:       query,
			Source:        source,
			SourceAppID:   nil,
			CreatedByRole: "account",
			CreatedBy:     accountID,
			Results:       response,
			ElapsedTime:   float64Ptr(response.ElapsedTime),
			HitCount:      &hitCount,
			QueryType:     queryType,
		}

		if _, err := s.datasetQueryService.CreateDatasetQuery(ctx, createReq); err != nil {
			logger.Error("Failed to save dataset query", err)
		}
	}

	return response, nil
}

// getRetrievalOptions Get retrieval options
func (s *hitTestingService) getRetrievalOptions(ctx context.Context, retrievalModel map[string]interface{}, dataset *dataset_model.Dataset) *RetrievalOptions {
	options := &RetrievalOptions{
		TopK:                  10, // Default retrieval limit
		SearchMethod:          "hybrid_search",
		ScoreThreshold:        0.35,
		ScoreThresholdEnabled: true,
		RerankingEnable:       true,
		RerankingMode:         "reranking_model", // Default reranking mode
		PreQAExtension:        false,             // Default value for PreQAExtension
		HopDepth:              3,                 // Knowledge graph search depth (Normal/Single mode)
		AnchoredHopDepth:      2,                 // Knowledge graph search depth (Anchored mode)
		GraphAlpha:            0.7,               // Weight for vector vs graph in hybrid fusion
		SemanticMinScore:      0.5,               // Min score for semantic hits in global mode
		AnchoredMinScore:      0.6,               // Min score for semantic hits in anchored mode
		MentionBoost:          0.1,               // Bonus for multiple entity mentions
		CoveragePenaltyBase:   0.7,               // Base for coverage penalty
		CoveragePenaltyWeight: 0.3,               // Weight for coverage penalty
		SemanticWeight:        0.7,               // Final weight for semantic score
		GraphWeight:           0.3,               // Final weight for graph score
	}

	// Get retrieval configuration from input parameters or dataset
	model := retrievalModel
	if model == nil && dataset.RetrievalConfig != nil {
		model = map[string]interface{}(dataset.RetrievalConfig)
	}

	if model != nil {
		if k, ok := model["top_k"].(float64); ok {
			options.TopK = int(k)
		}
		if method, ok := model["search_method"].(string); ok {
			options.SearchMethod = method
		}
		if threshold, ok := model["score_threshold"].(float64); ok {
			options.ScoreThreshold = threshold
		}
		if enabled, ok := model["score_threshold_enabled"].(bool); ok {
			options.ScoreThresholdEnabled = enabled
		}
		if rerankEnable, ok := model["reranking_enable"].(bool); ok {
			options.RerankingEnable = rerankEnable
		}
		if rerankModel, ok := model["reranking_model"].(map[string]interface{}); ok {
			options.RerankingModel = rerankModel
		}
		if weightsData, ok := model["weights"].(map[string]interface{}); ok {
			options.Weights = weightsData
		}
		if mode, ok := model["reranking_mode"].(string); ok && mode != "" {
			options.RerankingMode = mode
		}
		// ReturnFullDoc handling removed as per requirement
		if preQAExtension, ok := model["pre_qa_extension"].(bool); ok {
			options.PreQAExtension = preQAExtension
		}
		if hd, ok := model["hop_depth"].(float64); ok {
			options.HopDepth = int(hd)
		}
		if ahd, ok := model["anchored_hop_depth"].(float64); ok {
			options.AnchoredHopDepth = int(ahd)
		}
		if ga, ok := model["graph_alpha"].(float64); ok {
			options.GraphAlpha = ga
		}
		if sms, ok := model["semantic_min_score"].(float64); ok {
			options.SemanticMinScore = sms
		}
		if ams, ok := model["anchored_min_score"].(float64); ok {
			options.AnchoredMinScore = ams
		}
		if mb, ok := model["mention_boost"].(float64); ok {
			options.MentionBoost = mb
		}
		if cpb, ok := model["coverage_penalty_base"].(float64); ok {
			options.CoveragePenaltyBase = cpb
		}
		if cpw, ok := model["coverage_penalty_weight"].(float64); ok {
			options.CoveragePenaltyWeight = cpw
		}
		if sw, ok := model["semantic_weight"].(float64); ok {
			options.SemanticWeight = sw
		}
		if gw, ok := model["graph_weight"].(float64); ok {
			options.GraphWeight = gw
		}
	}

	options.SearchMethod = normalizeVectorSearchMethod(options.SearchMethod)

	// Reranking is mandatory for vector/BM25 retrieval. Graph-only results are not
	// doc-backed chunks and cannot be sent to the reranker.
	options.RerankingEnable = options.SearchMethod != "graph_search"
	if options.RerankingEnable && !isValidRerankingModelConfig(options.RerankingModel) {
		resolvedModel, err := llmruntime.NewModelResolver(s.defaultModelSvc).ResolveDefault(ctx, dataset.OrganizationID, shared_model.ModelTypeRerank)
		if err == nil && resolvedModel != nil {
			logger.Info("Using default rerank model", map[string]interface{}{
				"dataset_id": dataset.ID,
				"model":      resolvedModel.Model,
				"provider":   resolvedModel.Provider,
			})
			options.RerankingModel = map[string]interface{}{
				"reranking_provider_name": resolvedModel.Provider,
				"reranking_model_name":    resolvedModel.Model,
			}
		} else if err != nil {
			logger.Warn("Failed to resolve mandatory rerank model; retrieval will fall back to fused ordering if rerank is unavailable", map[string]interface{}{
				"dataset_id": dataset.ID,
				"error":      err.Error(),
			})
			options.RerankingModel = nil
		} else {
			options.RerankingModel = nil
		}
	}

	// Apply score threshold only if enabled
	if !options.ScoreThresholdEnabled {
		options.ScoreThreshold = 0.0
	}

	// Warn when semantic or hybrid search is requested but vector retrieval is unavailable.
	if (options.SearchMethod == "semantic_search" || options.SearchMethod == "hybrid_search") && s.retrievalService.vectorRetrieval == nil {
		logger.Warn("vector retrieval selected but vector retrieval is unavailable; falling back to keyword_search with adjusted threshold", map[string]interface{}{
			"original_search_method":   options.SearchMethod,
			"fallback_search_method":   "keyword_search",
			"original_score_threshold": options.ScoreThreshold,
		})
		// Fallback to keyword search and adjust threshold for better recall
		options.SearchMethod = "keyword_search"
		// Lower the threshold when falling back to text-based search
		if options.ScoreThresholdEnabled && options.ScoreThreshold > 0.1 {
			options.ScoreThreshold = 0.1
			logger.Info("Adjusted score threshold for fallback search", map[string]interface{}{
				"adjusted_score_threshold": options.ScoreThreshold,
			})
		}
	}

	return options
}

func isValidRerankingModelConfig(model map[string]interface{}) bool {
	if model == nil {
		return false
	}
	provider, _ := model["reranking_provider_name"].(string)
	name, _ := model["reranking_model_name"].(string)
	return strings.TrimSpace(provider) != "" && strings.TrimSpace(name) != ""
}

// fallbackRetrieve Fallback retrieval method when primary retrieval method fails
func (s *hitTestingService) fallbackRetrieve(ctx context.Context, dataset *dataset_model.Dataset, query string, options *RetrievalOptions) ([]dto.HitTestingRecordResponse, error) {
	// Get segments based on search method
	var segments []*dataset_model.DocumentSegment
	var err error

	if IsSupportSemanticSearch(options.SearchMethod) {
		// For semantic search, get more segments for better filtering
		segments, err = s.documentRepo.GetSegmentsByDatasetID(ctx, dataset.ID, options.TopK*3)
	} else if IsSupportFullTextSearch(options.SearchMethod) {
		// For full text search, get segments with full text indexing
		segments, err = s.documentRepo.GetSegmentsByDatasetID(ctx, dataset.ID, options.TopK*2)
	} else if IsSupportKeywordSearch(options.SearchMethod) {
		// For keyword search, get segments for keyword matching
		segments, err = s.documentRepo.GetSegmentsByDatasetID(ctx, dataset.ID, options.TopK*2)
	} else {
		// Default to semantic search
		segments, err = s.documentRepo.GetSegmentsByDatasetID(ctx, dataset.ID, options.TopK*3)
	}

	if err != nil {
		logger.Error("HitTesting failed to get dataset segments", err)
		if strings.Contains(err.Error(), "provider not initialized") {
			logger.Warn("HitTesting provider not initialized", map[string]interface{}{"dataset_id": dataset.ID, "query": query})
			return nil, errors.New("provider not initialized")
		}
		if strings.Contains(err.Error(), "quota exceeded") || strings.Contains(err.Error(), "配额不足") {
			logger.Warn("HitTesting quota exceeded", map[string]interface{}{"dataset_id": dataset.ID, "query": query})
			return nil, errors.New("配额不足")
		}
		if strings.Contains(err.Error(), "model not supported") {
			logger.Warn("HitTesting model not supported", map[string]interface{}{"dataset_id": dataset.ID, "query": query})
			return nil, errors.New("model not supported")
		}
		if strings.Contains(err.Error(), "no embedding model") {
			logger.Warn("HitTesting no embedding model", map[string]interface{}{"dataset_id": dataset.ID, "query": query})
			return nil, errors.New("no embedding model")
		}
		return nil, fmt.Errorf("failed to get dataset segments: %w", err)
	}

	if len(segments) == 0 {
		logger.Warn("HitTesting index not initialized", map[string]interface{}{"dataset_id": dataset.ID, "query": query})
		return nil, errors.New("index not initialized")
	}

	// Perform search based on method
	var scoredSegments []struct {
		segment *dataset_model.DocumentSegment
		score   float64
	}

	if options.SearchMethod == string(HybridSearch) {
		// Hybrid search: combine multiple search methods
		scoredSegments = s.performHybridSearch(query, segments, options.Weights)
	} else {
		// Single method search
		scoredSegments = s.performSingleMethodSearch(query, segments, options.SearchMethod, options.RerankingModel)
	}

	// Apply reranking if enabled
	if options.RerankingEnable && options.RerankingModel != nil && s.retrievalService.rerankService != nil {
		dtoDocuments := make([]dto.Document, 0, len(scoredSegments))
		for _, scored := range scoredSegments {
			dtoDocuments = append(dtoDocuments, dto.Document{
				PageContent: scored.segment.Content,
				Metadata: map[string]interface{}{
					"id":     scored.segment.ID,
					"doc_id": scored.segment.ID,
					"score":  scored.score,
				},
			})
		}

		rerankMode := retrieval.RERANKING_MODEL
		if options.RerankingMode != "" {
			rerankMode = retrieval.RerankMode(options.RerankingMode)
		}

		accountID := dataset.CreatedBy
		if accountID == "" {
			accountID = dataset.WorkspaceID
		}

		dataPostProcessor := retrieval.NewDataPostProcessorWithGateway(
			ctx,
			dataset.OrganizationID,
			s.defaultModelSvc,
			rerankMode,
			&retrieval.RerankModel{
				RerankingProviderName: options.RerankingModel["reranking_provider_name"].(string),
				RerankingModelName:    options.RerankingModel["reranking_model_name"].(string),
			},
			options.Weights,
			s.llmClient,
			accountID,
			dataset.ID,
		)

		scoreThreshold := options.ScoreThreshold
		topN := options.TopK
		rerankedDocuments, err := dataPostProcessor.Invoke(ctx, query, dtoDocuments, &scoreThreshold, &topN)
		if err == nil && len(rerankedDocuments) > 0 {
			// Update scoredSegments with reranked results
			resultMap := make(map[string]float64, len(rerankedDocuments))
			for _, result := range rerankedDocuments {
				if id, ok := result.Metadata["id"].(string); ok {
					if score, ok := result.Metadata["score"].(float64); ok {
						resultMap[id] = score
					}
				}
			}

			var newScoredSegments []struct {
				segment *dataset_model.DocumentSegment
				score   float64
			}

			for _, scored := range scoredSegments {
				if rerankScore, exists := resultMap[scored.segment.ID]; exists {
					newScoredSegments = append(newScoredSegments, struct {
						segment *dataset_model.DocumentSegment
						score   float64
					}{
						segment: scored.segment,
						score:   rerankScore,
					})
				}
			}

			scoredSegments = newScoredSegments
		}
	}

	// Apply score threshold filtering
	var filteredSegments []struct {
		segment *dataset_model.DocumentSegment
		score   float64
	}

	for _, scored := range scoredSegments {
		if scored.score >= options.ScoreThreshold {
			filteredSegments = append(filteredSegments, scored)
		}
	}

	// Take top results
	maxResults := options.TopK
	if len(filteredSegments) < maxResults {
		maxResults = len(filteredSegments)
	}

	var records []dto.HitTestingRecordResponse
	for i := 0; i < maxResults; i++ {
		seg := filteredSegments[i].segment
		score := filteredSegments[i].score

		// Convert keywords from JSON to slice
		var keywords []string
		if seg.Keywords != nil {
			keywordsMap := map[string]interface{}(seg.Keywords)
			if keywordList, ok := keywordsMap["keywords"].([]interface{}); ok {
				for _, kw := range keywordList {
					if kwStr, ok := kw.(string); ok {
						keywords = append(keywords, kwStr)
					}
				}
			} else if keywordSlice, ok := keywordsMap["keywords"].([]string); ok {
				keywords = keywordSlice
			}
		}

		var answer string
		if seg.Answer != nil {
			answer = *seg.Answer
		}

		var indexNodeID *string
		if seg.IndexNodeID != nil {
			indexNodeID = seg.IndexNodeID
		}

		var indexNodeHash *string
		if seg.IndexNodeHash != nil {
			indexNodeHash = seg.IndexNodeHash
		}

		// Load child chunks for this segment
		childChunks := s.loadChildChunksForSegment(ctx, seg)

		// Load document information
		var documentInfo dto.HitTestingDocumentResponse
		// Load dataset process rule
		var datasetProcessRule map[string]interface{}

		if seg.DocumentID != "" {
			doc, err := s.documentRepo.GetDocumentByID(ctx, seg.DocumentID)
			if err == nil && doc != nil {
				docType := ""
				if doc.DocType != nil {
					docType = *doc.DocType
				}
				documentInfo = dto.HitTestingDocumentResponse{
					ID:             doc.ID,
					DataSourceType: doc.DataSourceType,
					Name:           doc.Name,
					DocType:        docType,
					DocMetadata:    map[string]interface{}(doc.DocMetadata),
				}

				// Get dataset process rule if DatasetProcessRuleID exists
				if doc.DatasetProcessRuleID != nil && *doc.DatasetProcessRuleID != "" {
					if processRule, err := s.documentRepo.GetProcessRuleByID(ctx, *doc.DatasetProcessRuleID); err == nil && processRule != nil {
						datasetProcessRule = map[string]interface{}{
							"id":        processRule.ID,
							"mode":      processRule.Mode,
							"rules":     processRule.Rules,
							"createdBy": processRule.CreatedBy,
							"createdAt": processRule.CreatedAt.Unix(),
						}
					}
				}
			}
		}

		// Convert time fields to Unix timestamps
		createdAt := seg.CreatedAt.Unix()
		var disabledAt, indexingAt, completedAt, stoppedAt *int64
		if seg.DisabledAt != nil {
			disabledAtUnix := seg.DisabledAt.Unix()
			disabledAt = &disabledAtUnix
		}
		if seg.IndexingAt != nil {
			indexingAtUnix := seg.IndexingAt.Unix()
			indexingAt = &indexingAtUnix
		}
		if seg.CompletedAt != nil {
			completedAtUnix := seg.CompletedAt.Unix()
			completedAt = &completedAtUnix
		}
		if seg.StoppedAt != nil {
			stoppedAtUnix := seg.StoppedAt.Unix()
			stoppedAt = &stoppedAtUnix
		}

		// Handle answer field
		answer = ""
		if seg.Answer != nil {
			answer = *seg.Answer
		}

		record := dto.HitTestingRecordResponse{
			Segment: dto.SegmentResponse{
				ID:                 seg.ID,
				Position:           seg.Position,
				DocumentID:         seg.DocumentID,
				Content:            seg.Content,
				SignContent:        seg.Content, // Use content as sign_content for now
				Answer:             &answer,
				WordCount:          seg.WordCount,
				Tokens:             seg.Tokens,
				Keywords:           keywords,
				IndexNodeID:        indexNodeID,
				IndexNodeHash:      indexNodeHash,
				HitCount:           seg.HitCount,
				Enabled:            seg.Enabled,
				DisabledAt:         disabledAt,
				DisabledBy:         seg.DisabledBy,
				Status:             seg.Status,
				CreatedBy:          seg.CreatedBy,
				CreatedAt:          createdAt,
				IndexingAt:         indexingAt,
				CompletedAt:        completedAt,
				Error:              seg.Error,
				StoppedAt:          stoppedAt,
				Document:           documentInfo,
				DatasetProcessRule: datasetProcessRule,
			},
			Score:     score,
			MatchType: dto.MatchTypeOriginal, // TODO: use match type
			TSNEPosition: map[string]interface{}{
				"x": float64(i) * 0.1,
				"y": float64(i) * 0.1,
			},
			ChildChunks: childChunks,
		}
		records = append(records, record)
	}

	return records, nil
}

// ExternalRetrieve performs external dataset retrieval
func (s *hitTestingService) ExternalRetrieve(ctx context.Context, dataset *dataset_model.Dataset, query string, accountID string, externalRetrievalModel map[string]interface{}) (*dto.HitTestingResponse, error) {
	start := time.Now()

	// 1. Query table to get binding
	binding, err := s.datasetRepo.GetExternalKnowledgeBindingByDatasetID(ctx, dataset.ID)
	if err != nil {
		return nil, fmt.Errorf("external knowledge binding not found: %w", err)
	}
	// 2. Query table to get API configuration
	apiCfg, err := s.datasetRepo.GetExternalKnowledgeApiByID(ctx, binding.ExternalKnowledgeApiID)
	if err != nil {
		return nil, fmt.Errorf("external knowledge api not found: %w", err)
	}
	apiURL := apiCfg.Endpoint
	apiKey := apiCfg.ApiKey
	externalKnowledgeID := binding.ExternalKnowledgeID

	if apiURL == "" || apiKey == "" || externalKnowledgeID == "" {
		return nil, fmt.Errorf("external knowledge API config missing (url/key/id)")
	}

	// Escape query for external search to match default behavior
	escapedQuery := s.EscapeQueryForSearch(query)
	logger.Info("External retrieval started", map[string]interface{}{
		"dataset_id":            dataset.ID,
		"account_id":            accountID,
		"original_query":        query,
		"escaped_query":         escapedQuery,
		"external_knowledge_id": externalKnowledgeID,
		"api_url":               apiURL,
	})

	// 3. Construct request body
	requestBody := map[string]interface{}{
		"query":        escapedQuery,
		"knowledge_id": externalKnowledgeID,
	}
	if externalRetrievalModel != nil {
		requestBody["retrieval_setting"] = externalRetrievalModel
	}

	// 4. Make HTTP POST request
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL+"/retrieval", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := observability.HTTPClient(&http.Client{Timeout: 15 * time.Second})
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("external knowledge API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("external knowledge API returned status %d: %s", resp.StatusCode, string(body))
	}

	// 5. Parse response
	var extResp struct {
		Query   map[string]interface{}   `json:"query"`
		Records []map[string]interface{} `json:"records"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&extResp); err != nil {
		return nil, fmt.Errorf("failed to decode external API response: %w", err)
	}

	elapsed := time.Since(start)

	// 6. Convert to dto.HitTestingResponse
	response := &dto.HitTestingResponse{
		Query: dto.HitTestingQueryResponse{
			Content: query,
		},
		Records:     []dto.HitTestingRecordResponse{},
		ElapsedTime: float64(elapsed.Microseconds()) / 1000.0,
	}
	if extResp.Query != nil {
		if content, ok := extResp.Query["content"].(string); ok {
			response.Query.Content = content
		}
		if tsne, ok := extResp.Query["tsne_position"].(map[string]interface{}); ok {
			response.Query.TSNEPosition = tsne
		}
	}
	for _, rec := range extResp.Records {
		seg := dto.SegmentResponse{}
		if segment, ok := rec["segment"].(map[string]interface{}); ok {
			if id, ok := segment["id"].(string); ok {
				seg.ID = id
			}
			if pos, ok := segment["position"].(float64); ok {
				seg.Position = int(pos)
			}
			if docID, ok := segment["document_id"].(string); ok {
				seg.DocumentID = docID
			}
			if content, ok := segment["content"].(string); ok {
				seg.Content = content
			}
			if answer, ok := segment["answer"].(string); ok {
				seg.Answer = &answer
			}
			if wc, ok := segment["word_count"].(float64); ok {
				seg.WordCount = int(wc)
			}
			if tokens, ok := segment["tokens"].(float64); ok {
				seg.Tokens = int(tokens)
			}
			if keywords, ok := segment["keywords"].([]interface{}); ok {
				for _, kw := range keywords {
					if kwStr, ok := kw.(string); ok {
						seg.Keywords = append(seg.Keywords, kwStr)
					}
				}
			}
			if idxID, ok := segment["index_node_id"].(string); ok {
				seg.IndexNodeID = &idxID
			}
			if idxHash, ok := segment["index_node_hash"].(string); ok {
				seg.IndexNodeHash = &idxHash
			}
			if hitCount, ok := segment["hit_count"].(float64); ok {
				seg.HitCount = int(hitCount)
			}
			if enabled, ok := segment["enabled"].(bool); ok {
				seg.Enabled = enabled
			}
			if createdAtStr, ok := segment["created_at"].(string); ok {
				// Parse Unix timestamp from string
				if timestamp, err := strconv.ParseInt(createdAtStr, 10, 64); err == nil {
					seg.CreatedAt = timestamp
				}
			}
			if status, ok := segment["status"].(string); ok {
				seg.Status = status
			}
		}
		record := dto.HitTestingRecordResponse{
			Segment:   seg,
			Score:     0,
			MatchType: dto.MatchTypeOriginal, // TODO: use match type
		}
		if score, ok := rec["score"].(float64); ok {
			record.Score = score
		}
		if tsne, ok := rec["tsne_position"].(map[string]interface{}); ok {
			record.TSNEPosition = tsne
		}
		// child_chunks
		if childChunks, ok := rec["child_chunks"].([]interface{}); ok {
			for _, cc := range childChunks {
				if ccMap, ok := cc.(map[string]interface{}); ok {
					chunk := dto.ChildChunkResponse{}
					if id, ok := ccMap["id"].(string); ok {
						chunk.ID = id
					}
					if content, ok := ccMap["content"].(string); ok {
						chunk.Content = content
					}
					if pos, ok := ccMap["position"].(float64); ok {
						chunk.Position = int(pos)
					}
					// if score, ok := ccMap["score"].(float64); ok {
					// 	// chunk.Score = score // ChildChunkResponse has no Score field, ignore
					// }
					record.ChildChunks = append(record.ChildChunks, chunk)
				}
			}
		}
		response.Records = append(response.Records, record)
	}
	if err := normalizeHitTestingResponseKnowledgeImageURLs(response, config.Current().App.FilesURL); err != nil {
		return nil, err
	}

	return response, nil
}

// HitTestingArgsCheck validates hit testing arguments
func (s *hitTestingService) HitTestingArgsCheck(args *dto.HitTestingRequest) error {
	if args.Query == "" {
		return errors.NewBadRequestError("Query is required")
	}

	if utf8.RuneCountInString(args.Query) > 250 {
		return errors.NewBadRequestError("Query cannot exceed 250 characters")
	}

	return nil
}

// EscapeQueryForSearch escapes special characters in query for search
func (s *hitTestingService) EscapeQueryForSearch(query string) string {
	// Remove or escape special characters that might interfere with search.

	// Remove extra whitespace
	query = strings.TrimSpace(query)
	query = regexp.MustCompile(`\s+`).ReplaceAllString(query, " ")

	// Escape special regex characters
	specialChars := []string{"[", "]", "(", ")", "{", "}", "*", "+", "?", "^", "$", "|", "\\", "."}
	for _, char := range specialChars {
		query = strings.ReplaceAll(query, char, "\\"+char)
	}

	return query
}

// escapeQueryForSearch applies basic escaping for quotes to be safe with external services
func escapeQueryForSearch(q string) string {
	// Simple escaping of double quotes; extend as needed for search syntax.
	return strings.ReplaceAll(q, "\"", "\\\"")
}

// truncateString Truncate string to specified length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// performSingleMethodSearch Use single method retrieval (fallback path)
func (s *hitTestingService) performSingleMethodSearch(query string, segments []*dataset_model.DocumentSegment, searchMethod string, retrievalModel map[string]interface{}) []struct {
	segment *dataset_model.DocumentSegment
	score   float64
} {
	// If available services exist, try to use services; otherwise use simple text similarity strategy
	if searchMethod == string(KeywordSearch) && s.retrievalService.keywordRetrieval != nil {
		segmentMap := make(map[string]string)
		for _, seg := range segments {
			segmentMap[seg.ID] = seg.Content
		}
		s.retrievalService.keywordRetrieval.ClearIndex()
		s.retrievalService.keywordRetrieval.IndexSegments(segmentMap)
		results, _ := s.retrievalService.keywordRetrieval.Search(context.Background(), query, retrieval.SearchOptions{Limit: len(segments)})
		var scoredSegments []struct {
			segment *dataset_model.DocumentSegment
			score   float64
		}
		for _, r := range results {
			for _, segPtr := range segments {
				if segPtr.ID == r.ID {
					scoredSegments = append(scoredSegments, struct {
						segment *dataset_model.DocumentSegment
						score   float64
					}{segment: segPtr, score: r.Score})
					break
				}
			}
		}
		// Descending order
		for i := 0; i < len(scoredSegments)-1; i++ {
			for j := i + 1; j < len(scoredSegments); j++ {
				if scoredSegments[i].score < scoredSegments[j].score {
					scoredSegments[i], scoredSegments[j] = scoredSegments[j], scoredSegments[i]
				}
			}
		}
		return scoredSegments
	}

	if searchMethod == string(FullTextSearch) && s.retrievalService.fullTextRetrieval != nil {
		segmentMap := make(map[string]string)
		for _, seg := range segments {
			segmentMap[seg.ID] = seg.Content
		}
		s.retrievalService.fullTextRetrieval.ClearIndex()
		s.retrievalService.fullTextRetrieval.IndexSegments(segmentMap)
		results, _ := s.retrievalService.fullTextRetrieval.Search(context.Background(), "", query, retrieval.SearchOptions{Limit: len(segments)})
		var scoredSegments []struct {
			segment *dataset_model.DocumentSegment
			score   float64
		}
		for _, r := range results {
			for _, segPtr := range segments {
				if segPtr.ID == r.ID {
					scoredSegments = append(scoredSegments, struct {
						segment *dataset_model.DocumentSegment
						score   float64
					}{segment: segPtr, score: r.Score})
					break
				}
			}
		}
		for i := 0; i < len(scoredSegments)-1; i++ {
			for j := i + 1; j < len(scoredSegments); j++ {
				if scoredSegments[i].score < scoredSegments[j].score {
					scoredSegments[i], scoredSegments[j] = scoredSegments[j], scoredSegments[i]
				}
			}
		}
		return scoredSegments
	}

	// Simple text similarity fallback
	queryLower := strings.ToLower(query)
	queryWords := strings.Fields(queryLower)
	var scoredSegments []struct {
		segment *dataset_model.DocumentSegment
		score   float64
	}
	for _, segment := range segments {
		contentLower := strings.ToLower(segment.Content)
		score := s.calculateTextSimilarity(queryLower, queryWords, contentLower)
		scoredSegments = append(scoredSegments, struct {
			segment *dataset_model.DocumentSegment
			score   float64
		}{segment: segment, score: score})
	}
	for i := 0; i < len(scoredSegments)-1; i++ {
		for j := i + 1; j < len(scoredSegments); j++ {
			if scoredSegments[i].score < scoredSegments[j].score {
				scoredSegments[i], scoredSegments[j] = scoredSegments[j], scoredSegments[i]
			}
		}
	}
	return scoredSegments
}

// performHybridSearch Hybrid retrieval fallback: combine semantic/keyword scores
func (s *hitTestingService) performHybridSearch(query string, segments []*dataset_model.DocumentSegment, weights map[string]interface{}) []struct {
	segment *dataset_model.DocumentSegment
	score   float64
} {
	queryLower := strings.ToLower(query)
	queryWords := strings.Fields(queryLower)
	vectorWeight := 0.7
	keywordWeight := 0.3
	if weights != nil {
		if vectorSetting, ok := weights["vector_setting"].(map[string]interface{}); ok {
			if w, ok := vectorSetting["vector_weight"].(float64); ok {
				vectorWeight = w
			}
		}
		if keywordSetting, ok := weights["keyword_setting"].(map[string]interface{}); ok {
			if w, ok := keywordSetting["keyword_weight"].(float64); ok {
				keywordWeight = w
			}
		}
	}
	var scoredSegments []struct {
		segment *dataset_model.DocumentSegment
		score   float64
	}
	for _, segment := range segments {
		contentLower := strings.ToLower(segment.Content)
		semanticScore := s.calculateTextSimilarity(queryLower, queryWords, contentLower)
		keywordScore := s.calculateKeywordSimilarity(queryLower, queryWords, contentLower)
		combined := semanticScore*vectorWeight + keywordScore*keywordWeight
		scoredSegments = append(scoredSegments, struct {
			segment *dataset_model.DocumentSegment
			score   float64
		}{segment: segment, score: combined})
	}
	for i := 0; i < len(scoredSegments)-1; i++ {
		for j := i + 1; j < len(scoredSegments); j++ {
			if scoredSegments[i].score < scoredSegments[j].score {
				scoredSegments[i], scoredSegments[j] = scoredSegments[j], scoredSegments[i]
			}
		}
	}
	return scoredSegments
}

// loadChildChunksForSegment Load child chunks for a segment from database
func (s *hitTestingService) loadChildChunksForSegment(ctx context.Context, segment *dataset_model.DocumentSegment) []dto.ChildChunkResponse {
	var childChunks []dto.ChildChunkResponse
	items, err := s.documentRepo.GetChildChunksBySegmentID(ctx, segment.ID)
	if err != nil {
		logger.Error("Failed to load child chunks for segment", err)
		return childChunks
	}
	for _, cc := range items {
		childChunks = append(childChunks, dto.ChildChunkResponse{
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
	return childChunks
}

// calculateTextSimilarity Calculate text similarity (improved algorithm, combining Jaccard and TF-IDF ideas)
func (s *hitTestingService) calculateTextSimilarity(queryLower string, queryWords []string, contentLower string) float64 {
	if len(queryLower) == 0 || len(contentLower) == 0 {
		return 0
	}

	// Normalize content text to tokens; use provided queryWords for query side
	normContent := func(in string) []string {
		// Keep Chinese characters, English letters and numbers
		in = regexp.MustCompile(`[^a-zA-Z0-9\p{Han}]+`).ReplaceAllString(in, " ")
		in = regexp.MustCompile(`\s+`).ReplaceAllString(in, " ")
		in = strings.TrimSpace(in)
		if in == "" {
			return nil
		}

		// For Chinese, split by character; for English, split by space
		var tokens []string
		words := strings.Split(in, " ")
		for _, word := range words {
			if word == "" {
				continue
			}
			// If contains Chinese characters, split by character
			if regexp.MustCompile(`\p{Han}`).MatchString(word) {
				for _, char := range word {
					if char != ' ' {
						tokens = append(tokens, string(char))
					}
				}
			} else {
				// Add English words directly
				tokens = append(tokens, word)
			}
		}
		return tokens
	}

	// Process query tokens, if queryWords is empty or is English tokenization, re-process as Chinese
	var qTokens []string
	if len(queryWords) == 0 {
		qTokens = normContent(queryLower)
	} else {
		qTokens = queryWords
	}

	// Process content tokens
	cTokens := normContent(contentLower)

	if len(qTokens) == 0 || len(cTokens) == 0 {
		return 0
	}

	// Calculate Jaccard similarity
	tokenSetQ := make(map[string]bool)
	tokenSetC := make(map[string]bool)
	for _, t := range qTokens {
		tokenSetQ[t] = true
	}
	for _, t := range cTokens {
		tokenSetC[t] = true
	}

	intersection := 0
	union := 0
	for t := range tokenSetQ {
		if tokenSetC[t] {
			intersection++
		}
		union++
	}
	for t := range tokenSetC {
		if !tokenSetQ[t] {
			union++
		}
	}

	if union == 0 {
		return 0
	}

	jaccard := float64(intersection) / float64(union)

	// Calculate TF-IDF similarity (simplified version)
	// For each query token, calculate its frequency in content
	totalMatches := 0
	for _, qToken := range qTokens {
		for _, cToken := range cTokens {
			if qToken == cToken {
				totalMatches++
			}
		}
	}

	// Normalize by total tokens
	tfidf := float64(totalMatches) / float64(len(cTokens))

	// Combine Jaccard and TF-IDF scores
	combined := 0.7*jaccard + 0.3*tfidf

	return combined
}

// calculateKeywordSimilarity Calculate keyword similarity (simplified TF-IDF)
func (s *hitTestingService) calculateKeywordSimilarity(queryLower string, queryWords []string, contentLower string) float64 {
	if len(queryLower) == 0 || len(contentLower) == 0 {
		return 0
	}

	// Count occurrences of each query word in content
	matches := 0
	for _, qWord := range queryWords {
		// Simple substring matching for keyword similarity
		if strings.Contains(contentLower, qWord) {
			matches++
		}
	}

	// Normalize by query word count
	if len(queryWords) == 0 {
		return 0
	}

	return float64(matches) / float64(len(queryWords))
}

func float64Ptr(f float64) *float64 {
	return &f
}
