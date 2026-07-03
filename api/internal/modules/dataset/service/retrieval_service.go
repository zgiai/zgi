package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/graph"
	graphflow_retrieval "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/retrieval"
	"github.com/zgiai/zgi/api/internal/modules/dataset/indexing"
	dataset_model "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	dataset_repository "github.com/zgiai/zgi/api/internal/modules/dataset/repository"
	"github.com/zgiai/zgi/api/internal/modules/dataset/retrieval"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmruntime "github.com/zgiai/zgi/api/internal/modules/llm/runtime"
	shared_model "github.com/zgiai/zgi/api/internal/modules/shared/model"
	"github.com/zgiai/zgi/api/pkg/embedding"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/vectordb"
	"go.uber.org/zap"
)

// EmbeddingServiceFactory builds an embedding service for a dataset.
type EmbeddingServiceFactory func(dataset *dataset_model.Dataset) embedding.EmbeddingService

// RetrievalService handles retrieval operations
type RetrievalService struct {
	documentRepo      dataset_repository.DocumentRepository
	vectorRetrieval   *retrieval.VectorRetrievalService
	keywordRetrieval  *retrieval.KeywordRetrievalService
	fullTextRetrieval *retrieval.FullTextRetrievalService
	hybridRetrieval   *retrieval.HybridRetrievalService
	rerankService     *retrieval.RerankService
	defaultModelSvc   llmdefaultservice.DefaultModelService
	vectorClient      *vectordb.WeaviateClient
	embeddingFactory  EmbeddingServiceFactory
	llmClient         llmclient.LLMClient
	graphFlowService  *graphflow.Service
	boundaryDetector  *graphflow_retrieval.BoundaryDetector
}

// NewRetrievalService creates a new retrieval service
func NewRetrievalService(
	documentRepo dataset_repository.DocumentRepository,
	vectorRetrieval *retrieval.VectorRetrievalService,
	keywordRetrieval *retrieval.KeywordRetrievalService,
	fullTextRetrieval *retrieval.FullTextRetrievalService,
	hybridRetrieval *retrieval.HybridRetrievalService,
	rerankService *retrieval.RerankService,
	defaultModelSvc llmdefaultservice.DefaultModelService,
	vectorClient *vectordb.WeaviateClient,
	graphFlowService *graphflow.Service,
) *RetrievalService {
	var detector *graphflow_retrieval.BoundaryDetector
	if graphFlowService != nil {
		detector = graphflow_retrieval.NewBoundaryDetector(graphFlowService.EntityRepo, graphFlowService.RelationshipRepo)
	}

	return &RetrievalService{
		documentRepo:      documentRepo,
		vectorRetrieval:   vectorRetrieval,
		keywordRetrieval:  keywordRetrieval,
		fullTextRetrieval: fullTextRetrieval,
		hybridRetrieval:   hybridRetrieval,
		rerankService:     rerankService,
		defaultModelSvc:   defaultModelSvc,
		vectorClient:      vectorClient,
		graphFlowService:  graphFlowService,
		boundaryDetector:  detector,
	}
}

// NewRetrievalServiceWithEmbeddingFactory allows injecting a dataset-specific embedding factory.
func NewRetrievalServiceWithEmbeddingFactory(
	documentRepo dataset_repository.DocumentRepository,
	vectorRetrieval *retrieval.VectorRetrievalService,
	keywordRetrieval *retrieval.KeywordRetrievalService,
	fullTextRetrieval *retrieval.FullTextRetrievalService,
	hybridRetrieval *retrieval.HybridRetrievalService,
	rerankService *retrieval.RerankService,
	defaultModelSvc llmdefaultservice.DefaultModelService,
	vectorClient *vectordb.WeaviateClient,
	graphFlowService *graphflow.Service,
	embeddingFactory EmbeddingServiceFactory,
) *RetrievalService {
	rs := NewRetrievalService(
		documentRepo,
		vectorRetrieval,
		keywordRetrieval,
		fullTextRetrieval,
		hybridRetrieval,
		rerankService,
		defaultModelSvc,
		vectorClient,
		graphFlowService,
	)
	rs.embeddingFactory = embeddingFactory
	return rs
}

// SetLLMClient injects the gateway-backed LLM client used for runtime model invocations.
func (s *RetrievalService) SetLLMClient(client llmclient.LLMClient) {
	if s == nil {
		return
	}
	s.llmClient = client
}

// RetrievalOptions defines retrieval parameters
type RetrievalOptions struct {
	SearchMethod          string
	TopK                  int
	ScoreThreshold        float64
	RerankingEnable       bool
	RerankingModel        map[string]interface{}
	RerankingMode         string
	Weights               map[string]interface{}
	ScoreThresholdEnabled bool
	DocumentIDsFilter     []string
	Filter                map[string]interface{}
	// ReturnFullDoc removed as per requirement
	PreQAExtension        bool    // Whether to retrieve content associated with questions
	RetrievalMode         string  // vector, graph, hybrid (default)
	HopDepth              int     // Knowledge graph search depth (Normal/Single mode)
	AnchoredHopDepth      int     // Knowledge graph search depth (Anchored mode)
	GraphAlpha            float64 // Weight for vector vs graph in hybrid fusion
	SemanticMinScore      float64 // Min score for semantic hits in global mode
	AnchoredMinScore      float64 // Min score for semantic hits in anchored mode
	MentionBoost          float64 // Bonus for multiple entity mentions
	CoveragePenaltyBase   float64 // Base for coverage penalty
	CoveragePenaltyWeight float64 // Weight for coverage penalty
	SemanticWeight        float64 // Final weight for semantic score (e.g., 0.7)
	GraphWeight           float64 // Final weight for graph score (e.g., 0.3)
}

const hybridRecallCandidateLimit = 50

// Retrieve Main retrieval method
func (s *RetrievalService) Retrieve(ctx context.Context, dataset *dataset_model.Dataset, query string, options *RetrievalOptions) ([]dto.HitTestingRecordResponse, *dto.GraphExecution, error) {
	if query == "" {
		return []dto.HitTestingRecordResponse{}, nil, nil
	}

	// check dataset
	if dataset == nil {
		return []dto.HitTestingRecordResponse{}, nil, nil
	}
	logCtx := logger.WithFields(ctx,
		zap.String("dataset_id", dataset.ID),
		zap.String("tenant_id", dataset.WorkspaceID),
		zap.String("organization_id", dataset.OrganizationID),
	)

	// Count only documents and segments that are available for retrieval.
	// Documents need to be completed, enabled and not archived
	// Segments need to be completed and enabled
	availableDocumentCount, err := s.documentRepo.GetDocumentCount(ctx, dataset.ID)
	if err != nil {
		logger.Error("Failed to get available document count", err)
		return nil, nil, fmt.Errorf("failed to get available document count: %w", err)
	}

	availableSegmentCount, err := s.documentRepo.GetSegmentCount(ctx, dataset.ID)
	if err != nil {
		logger.Error("Failed to get available segment count", err)
		return nil, nil, fmt.Errorf("failed to get available segment count: %w", err)
	}

	if availableDocumentCount <= 0 || availableSegmentCount <= 0 {
		return []dto.HitTestingRecordResponse{}, nil, nil
	}

	// store all documents
	var allDocuments []retrieval.SearchResult
	var exceptions []string

	type result struct {
		documents      []retrieval.SearchResult
		graphExecution *dto.GraphExecution
		err            error
	}

	resultChan := make(chan result, 3) // hybrid/vector/BM25 plus graph
	pending := 0

	// Determine retrieval mode
	// RetrievalMode controls which search types to run:
	// - "vector": run vector/BM25 retrieval without graph search
	// - "graph": run only graph search
	// - "hybrid" or "": run both vector and graph searches
	runVector := true
	runGraph := true
	if options.RetrievalMode == "vector" {
		runGraph = false
	} else if options.RetrievalMode == "graph" {
		runVector = false
	}

	searchMethod := normalizeVectorSearchMethod(options.SearchMethod)
	options.SearchMethod = searchMethod

	// Retrieval threshold is applied only after child hits are aggregated back to parent records.
	searchScoreThreshold := 0.0
	searchTopK := options.TopK
	if isHybridVectorBM25Method(searchMethod) {
		searchTopK = hybridRecallCandidateLimit
	}

	// When RetrievalMode is "vector" or "hybrid", run vector-based searches
	// SearchMethod can be used to filter which specific vector searches to run:
	// - "keyword_search": run BM25 lexical search for compatibility
	// - "semantic_search": run only embedding search
	// - "full_text_search": run only BM25 full-text search
	// - "hybrid_search": run vector + BM25 hybrid fusion
	// - "graph_search": run no vector searches (handled separately)
	if runVector {
		runHybrid := isHybridVectorBM25Method(searchMethod)
		runSemantic := searchMethod == "semantic_search"
		runBM25 := isBM25OnlyMethod(searchMethod)

		if runHybrid && (s.vectorRetrieval != nil || s.fullTextRetrieval != nil) {
			pending++
			go func() {
				documents, err := s.hybridVectorBM25Search(ctx, dataset, query, searchTopK, searchScoreThreshold, options)
				logger.DebugContext(logCtx, "vector+BM25 hybrid retrieval completed",
					err,
					zap.Int("documents_count", len(documents)),
				)
				resultChan <- result{documents, nil, err}
			}()
		}

		if runSemantic && s.vectorRetrieval != nil {
			pending++
			go func() {
				documents, err := s.embeddingSearch(ctx, dataset, query, searchTopK, searchScoreThreshold, options)
				logger.DebugContext(logCtx, "semantic retrieval completed",
					err,
					zap.Int("documents_count", len(documents)),
				)
				resultChan <- result{documents, nil, err}
			}()
		}

		if runBM25 && s.fullTextRetrieval != nil {
			pending++
			go func() {
				documents, err := s.fullTextIndexSearch(ctx, dataset, query, searchTopK, searchScoreThreshold, options)
				logger.DebugContext(logCtx, "BM25 retrieval completed",
					err,
					zap.Int("documents_count", len(documents)),
				)
				resultChan <- result{documents, nil, err}
			}()
		}
	}

	// graph_search
	logger.Debug("Graph search check", map[string]interface{}{
		"dataset_enable_graph_flow": dataset.EnableGraphFlow,
		"graph_flow_service_nil":    s.graphFlowService == nil,
	})
	if runGraph && dataset.EnableGraphFlow && s.graphFlowService != nil {
		pending++
		go func() {
			logger.DebugContext(logCtx, "starting graph retrieval")

			// Create a 120s timeout for the graph search specifically (increased from 15s)
			graphCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
			defer cancel()

			documents, execution, err := s.graphSearch(graphCtx, dataset, query, options)
			if err != nil {
				logger.Error("Graph search failed or timed out", err)
				if execution == nil {
					execution = &dto.GraphExecution{
						Summary: fmt.Sprintf("图谱搜索失败: %v", err),
					}
				}
			} else {
				logger.DebugContext(logCtx, "graph retrieval completed",
					zap.Int("documents_count", len(documents)),
				)
			}
			logger.DebugContext(logCtx, "graph retrieval result collected",
				err,
				zap.Int("documents_count", len(documents)),
			)
			resultChan <- result{documents, execution, nil} // Never pass error back to channel to avoid blocking
		}()
	}

	// wait for all goroutines to finish
	// Separate graph_knowledge results from other results
	var graphDocuments []retrieval.SearchResult
	var semanticDocuments []retrieval.SearchResult

	var finalExecution *dto.GraphExecution
	for i := 0; i < pending; i++ {
		res := <-resultChan
		if res.err != nil {
			exceptions = append(exceptions, res.err.Error())
		} else {
			if res.graphExecution != nil {
				finalExecution = res.graphExecution
			}
			for _, doc := range res.documents {
				if source, ok := doc.Metadata["source"].(string); ok && source == "graph_knowledge" {
					graphDocuments = append(graphDocuments, doc)
				} else {
					semanticDocuments = append(semanticDocuments, doc)
				}
			}
		}
	}

	// Fault-tolerant error handling: log warnings but continue with partial results
	// Only fail if ALL methods returned errors and we have zero results
	if len(exceptions) > 0 {
		logger.Warn("Some retrieval methods failed, continuing with partial results", map[string]interface{}{
			"errors":           exceptions,
			"semantic_results": len(semanticDocuments),
			"graph_results":    len(graphDocuments),
		})
		if len(semanticDocuments) == 0 && len(graphDocuments) == 0 {
			return nil, nil, fmt.Errorf("all retrieval methods failed: %v", exceptions)
		}
	}

	// Sort semantic documents by score descending before merging.
	sort.SliceStable(semanticDocuments, func(i, j int) bool {
		return semanticDocuments[i].Score > semanticDocuments[j].Score
	})

	// Deduplicate by segment ID and Content (graph results may overlap with semantic results or contain duplicates)
	seenIDs := make(map[string]bool)
	seenContent := make(map[string]bool)

	// Helper to add unique documents
	addUniqueDoc := func(doc retrieval.SearchResult) {
		// Dedup by ID
		if seenIDs[doc.ID] {
			return
		}

		// Dedup by Content (if substantial length > 50 chars to avoid deduping short common phrases)
		// For short content, we rely on ID. For longer content, strict content match.
		if len(doc.Content) > 50 {
			// Simple content signature (can be hash or just string key)
			if seenContent[doc.Content] {
				return
			}
			seenContent[doc.Content] = true
		}

		seenIDs[doc.ID] = true
		allDocuments = append(allDocuments, doc)
	}

	for _, doc := range semanticDocuments {
		addUniqueDoc(doc)
	}

	// Add graph documents that aren't already in the results
	for _, doc := range graphDocuments {
		addUniqueDoc(doc)
	}

	logger.DebugContext(logCtx, "retrieval results merged before reranking",
		zap.Int("semantic_documents_count", len(semanticDocuments)),
		zap.Int("graph_documents_count", len(graphDocuments)),
		zap.Int("unique_documents_count", len(allDocuments)),
	)

	// Unified Reranking: rerank all fused candidates, then keep final top_k.
	// In hybrid mode this is vector top 50 + BM25 top 50 after RRF dedup.
	if options.RerankingEnable && isValidRerankingModelConfig(options.RerankingModel) && s.rerankService != nil && len(allDocuments) > 0 {
		rerankableDocuments, passthroughDocuments := splitRerankableSearchResults(allDocuments)
		if len(rerankableDocuments) > 0 {
			rerankedDocuments, err := s.applyReranking(ctx, dataset, query, rerankableDocuments, options)
			if err != nil {
				logger.Warn("Unified Reranking failed", map[string]interface{}{
					"dataset_id": dataset.ID,
					"error":      err.Error(),
				})
				// Fallback: continue with original scores (merged list)
			} else {
				allDocuments = append(rerankedDocuments, passthroughDocuments...)
			}
		}
	}

	// Sort all documents by score descending
	sort.SliceStable(allDocuments, func(i, j int) bool {
		return allDocuments[i].Score > allDocuments[j].Score
	})

	// convertSearchResults - real chunks from both semantic and graph search
	records := s.convertSearchResultsToRecords(allDocuments, dataset.ID, options)
	records = filterAndLimitFinalRecords(records, options)

	// Update hit count for final parent records only.
	s.updateRecordHitCount(ctx, records)

	logger.Info("Retrieval results summary", map[string]interface{}{
		"query":             query,
		"semantic_docs":     len(semanticDocuments),
		"graph_docs":        len(graphDocuments),
		"final_docs_count":  len(records),
		"enable_graph_flow": dataset.EnableGraphFlow,
	})
	return records, finalExecution, nil
}

// getEmbeddingService helper to get or create embedding service for a dataset
func (s *RetrievalService) getEmbeddingService(ctx context.Context, dataset *dataset_model.Dataset) embedding.EmbeddingService {
	// Prefer injected embedding factory (gateway) if provided
	if s.embeddingFactory != nil {
		if embeddingService := s.embeddingFactory(dataset); embeddingService != nil {
			return embeddingService
		}
	}

	if dataset == nil || s.llmClient == nil {
		if s.vectorRetrieval != nil {
			return s.vectorRetrieval.GetEmbeddingService()
		}
		return nil
	}

	resolvedModel, err := llmruntime.NewModelResolver(s.defaultModelSvc).ResolveFromPointers(
		ctx,
		dataset.OrganizationID,
		dataset.EmbeddingModelProvider,
		dataset.EmbeddingModel,
		shared_model.ModelTypeEmbedding,
	)
	if err != nil {
		logger.Error("Failed to resolve embedding model", err)
		return nil
	}

	accountID := dataset.CreatedBy
	if accountID == "" {
		accountID = dataset.WorkspaceID
	}

	embeddingService, err := indexing.NewGatewayEmbeddingService(s.llmClient, accountID, dataset.ID, "dataset", resolvedModel.Model, dataset.WorkspaceID)
	if err != nil {
		logger.Error("Failed to build gateway embedding service", err)
		return nil
	}

	return embeddingService
}

// keywordSearch Keyword search
func (s *RetrievalService) keywordSearch(ctx context.Context, dataset *dataset_model.Dataset, query string, options *RetrievalOptions) ([]retrieval.SearchResult, error) {
	if s.keywordRetrieval == nil {
		return []retrieval.SearchResult{}, nil
	}

	searchOpts := retrieval.SearchOptions{
		Limit:          options.TopK,
		ScoreThreshold: 0.0,
		DocumentIDs:    options.DocumentIDsFilter,
		Filter:         options.Filter,
	}

	// Segments
	segments, err := s.documentRepo.GetSegmentsByDatasetID(ctx, dataset.ID, 0) // 0 means fetch all
	if err != nil {
		return nil, err
	}

	// segmentMap for indexing, segmentDetailMap for metadata enrichment
	segmentMap := make(map[string]string)
	segmentDetailMap := make(map[string]*dataset_model.DocumentSegment)
	for _, segment := range segments {
		segmentMap[segment.ID] = segment.Content
		segmentDetailMap[segment.ID] = segment
	}

	// IndexSegments
	s.keywordRetrieval.ClearIndex()
	err = s.keywordRetrieval.IndexSegments(segmentMap)
	if err != nil {
		return nil, err
	}

	// Search
	results, err := s.keywordRetrieval.Search(ctx, query, searchOpts)
	if err != nil {
		return nil, err
	}

	// Enrich results with metadata (doc_id, document_id) so that
	// convertSearchResultsToRecords can look up documents and segments
	for i, r := range results {
		if seg, ok := segmentDetailMap[r.ID]; ok {
			results[i].Metadata = map[string]interface{}{
				"doc_id":      seg.ID,
				"document_id": seg.DocumentID,
				"dataset_id":  dataset.ID,
			}
		}
	}

	return results, nil
}

func (s *RetrievalService) hybridVectorBM25Search(ctx context.Context, dataset *dataset_model.Dataset, query string, topK int, scoreThreshold float64, options *RetrievalOptions) ([]retrieval.SearchResult, error) {
	type branchResult struct {
		source    string
		documents []retrieval.SearchResult
		err       error
	}

	resultChan := make(chan branchResult, 2)
	pending := 0

	if s.vectorRetrieval != nil {
		pending++
		go func() {
			documents, err := s.embeddingSearch(ctx, dataset, query, topK, scoreThreshold, options)
			resultChan <- branchResult{source: "vector", documents: documents, err: err}
		}()
	}

	if s.fullTextRetrieval != nil {
		pending++
		go func() {
			documents, err := s.fullTextIndexSearch(ctx, dataset, query, topK, scoreThreshold, options)
			resultChan <- branchResult{source: "bm25", documents: documents, err: err}
		}()
	}

	var vectorDocuments []retrieval.SearchResult
	var bm25Documents []retrieval.SearchResult
	var errors []string
	for i := 0; i < pending; i++ {
		res := <-resultChan
		if res.err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", res.source, res.err))
			continue
		}
		if res.source == "vector" {
			vectorDocuments = res.documents
		} else {
			bm25Documents = res.documents
		}
	}

	if len(vectorDocuments) == 0 && len(bm25Documents) == 0 && len(errors) > 0 {
		return nil, fmt.Errorf("vector+BM25 retrieval failed: %s", strings.Join(errors, "; "))
	}
	if len(errors) > 0 {
		logger.Warn("Vector+BM25 retrieval continuing with partial results", map[string]interface{}{
			"dataset_id":     dataset.ID,
			"errors":         errors,
			"vector_results": len(vectorDocuments),
			"bm25_results":   len(bm25Documents),
		})
	}

	return retrieval.FuseVectorBM25Results(query, vectorDocuments, bm25Documents, 0, retrieval.DefaultHybridFusionConfig()), nil
}

// embeddingSearch Vector search
func (s *RetrievalService) embeddingSearch(ctx context.Context, dataset *dataset_model.Dataset, query string, topK int, scoreThreshold float64, options *RetrievalOptions) ([]retrieval.SearchResult, error) {
	if s.vectorRetrieval == nil {
		return []retrieval.SearchResult{}, nil
	}

	// Prefer injected embedding factory (gateway) if provided
	overrideApplied := false
	if s.embeddingFactory != nil {

		// Temporarily specify the model
		//*dataset.EmbeddingModel = "text-embedding-3-large"
		if embeddingService := s.embeddingFactory(dataset); embeddingService != nil {
			s.vectorRetrieval.SetEmbeddingService(embeddingService)
			overrideApplied = true
		}
	}

	if !overrideApplied {
		embeddingService := s.getEmbeddingService(ctx, dataset)
		if embeddingService == nil {
			return nil, fmt.Errorf("failed to resolve embedding service")
		}
		s.vectorRetrieval.SetEmbeddingService(embeddingService)
	}

	// className
	className := dataset_model.GenCollectionNameByID(dataset.ID)

	searchOpts := retrieval.SearchOptions{
		Limit:          topK,
		ScoreThreshold: 0.0,
		DocumentIDs:    options.DocumentIDsFilter,
		Filter:         options.Filter,
		PreQAExtension: options.PreQAExtension, // Pass the PreQAExtension option
	}

	// Reranking
	useRankingModel := options.RerankingEnable &&
		options.RerankingModel != nil &&
		options.SearchMethod == "semantic_search"

	vectorTopK := topK
	if useRankingModel {
		vectorTopK = 10
		searchOpts.Limit = vectorTopK
	}

	// Search
	documents, err := s.vectorRetrieval.Search(ctx, className, query, searchOpts)
	if err != nil {
		return nil, err
	}

	for i := range documents {
		documents[i].Metadata = cloneSearchMetadata(documents[i].Metadata)
		documents[i].Metadata["vector_score"] = documents[i].Score
		documents[i].Metadata["retrieval_sources"] = []string{"vector"}
		documents[i].Metadata["fusion_score"] = documents[i].Score
		documents[i].Metadata["score"] = documents[i].Score
	}

	return documents, nil
}

// fullTextIndexSearch Full-text index search
func (s *RetrievalService) fullTextIndexSearch(ctx context.Context, dataset *dataset_model.Dataset, query string, topK int, scoreThreshold float64, options *RetrievalOptions) ([]retrieval.SearchResult, error) {
	if s.fullTextRetrieval == nil {
		return []retrieval.SearchResult{}, nil
	}

	// Escape query for search
	escapedQuery := s.escapeQueryForSearch(query)

	// Set up class name for Weaviate BM25 search
	className := dataset_model.GenCollectionNameByID(dataset.ID)

	searchOpts := retrieval.SearchOptions{
		Limit:          topK,
		ScoreThreshold: 0.0,
		DocumentIDs:    options.DocumentIDsFilter,
		Filter:         options.Filter,
		PreQAExtension: options.PreQAExtension, // Pass the PreQAExtension option
	}

	// Search
	results, err := s.fullTextRetrieval.Search(ctx, className, escapedQuery, searchOpts)
	if err != nil {
		return nil, err
	}

	// Check if we need to populate content from segmentMap
	// This is only needed when using local BM25 fallback (when Weaviate search fails)
	needSegmentMap := false
	for _, result := range results {
		if result.Content == "" {
			needSegmentMap = true
			break
		}
	}

	var segmentMap map[string]string
	if needSegmentMap {
		// segments
		segments, err := s.documentRepo.GetSegmentsByDatasetID(ctx, dataset.ID, 0) // 0 means fetch all
		if err != nil {
			return nil, err
		}

		// segmentMap
		segmentMap = make(map[string]string)
		for _, segment := range segments {
			segmentMap[segment.ID] = segment.Content
		}

		// IndexSegments
		s.fullTextRetrieval.ClearIndex()
		err = s.fullTextRetrieval.IndexSegments(segmentMap)
		if err != nil {
			return nil, err
		}

		// Fill in content for search results
		for i := range results {
			if content, exists := segmentMap[results[i].ID]; exists && results[i].Content == "" {
				results[i].Content = content
			}
		}
	}

	for i := range results {
		results[i].Metadata = cloneSearchMetadata(results[i].Metadata)
		if results[i].Metadata["bm25_score"] == nil {
			results[i].Metadata["bm25_score"] = results[i].Score
		}
		results[i].Metadata["bm25_rank_score"] = results[i].Score
		results[i].Metadata["retrieval_sources"] = []string{"bm25"}
		results[i].Metadata["matched_terms"] = matchedQueryTerms(query, results[i].Content)
		results[i].Metadata["fusion_score"] = results[i].Score
		results[i].Metadata["score"] = results[i].Score
	}

	return results, nil
}

func cloneSearchMetadata(metadata map[string]interface{}) map[string]interface{} {
	if metadata == nil {
		return make(map[string]interface{})
	}
	cloned := make(map[string]interface{}, len(metadata)+4)
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func normalizeVectorSearchMethod(searchMethod string) string {
	if searchMethod == "" || !isKnownSearchMethod(searchMethod) {
		return "hybrid_search"
	}
	return searchMethod
}

func isHybridVectorBM25Method(searchMethod string) bool {
	return searchMethod == "hybrid_search"
}

func isBM25OnlyMethod(searchMethod string) bool {
	return searchMethod == "full_text_search" || searchMethod == "keyword_search"
}

func isKnownSearchMethod(searchMethod string) bool {
	switch searchMethod {
	case "", "hybrid_search", "semantic_search", "full_text_search", "keyword_search", "graph_search":
		return true
	default:
		return false
	}
}

func splitRerankableSearchResults(documents []retrieval.SearchResult) ([]retrieval.SearchResult, []retrieval.SearchResult) {
	rerankable := make([]retrieval.SearchResult, 0, len(documents))
	passthrough := make([]retrieval.SearchResult, 0)
	for _, doc := range documents {
		if isRerankableSearchResult(doc) {
			rerankable = append(rerankable, doc)
		} else {
			passthrough = append(passthrough, doc)
		}
	}
	return rerankable, passthrough
}

func isRerankableSearchResult(doc retrieval.SearchResult) bool {
	if doc.Metadata == nil {
		return false
	}
	if source, _ := doc.Metadata["source"].(string); source == "graph_knowledge" {
		return false
	}
	docID, _ := doc.Metadata["doc_id"].(string)
	return strings.TrimSpace(docID) != ""
}

func matchedQueryTerms(query, content string) []string {
	content = strings.ToLower(content)
	seen := make(map[string]struct{})
	terms := make([]string, 0)
	for _, term := range strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		if strings.Contains(content, term) {
			terms = append(terms, term)
		}
	}
	return terms
}

// escapeQueryForSearch escapes special characters in query for search
func (s *RetrievalService) escapeQueryForSearch(query string) string {
	// Simple escaping of double quotes; extend as needed for search syntax.
	return strings.ReplaceAll(query, "\"", "\\\"")
}

// applyReranking Apply reranking
func (s *RetrievalService) applyReranking(ctx context.Context, dataset *dataset_model.Dataset, query string, documents []retrieval.SearchResult, options *RetrievalOptions) ([]retrieval.SearchResult, error) {
	// Convert retrieval.SearchResult to dto.Document
	dtoDocuments := make([]dto.Document, len(documents))
	for i, doc := range documents {
		dtoDocuments[i] = dto.Document{
			PageContent: doc.Content,
			Metadata: map[string]interface{}{
				"id":    doc.ID,
				"score": doc.Score,
			},
		}
		// Copy original metadata
		for k, v := range doc.Metadata {
			dtoDocuments[i].Metadata[k] = v
		}
	}

	rerankMode := retrieval.RERANKING_MODEL
	if options.RerankingMode != "" {
		rerankMode = retrieval.RerankMode(options.RerankingMode)
	}

	accountID := dataset.CreatedBy
	if accountID == "" {
		accountID = dataset.WorkspaceID
	}

	// Create DataPostProcessor with gateway support
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

	// Apply reranking only. Threshold and top_k are applied later at parent-record level.
	rerankedDocuments, rerankErr := dataPostProcessor.Invoke(ctx, query, dtoDocuments, nil, nil)
	if rerankErr != nil {
		logger.Warn("Reranking failed in embedding search", map[string]interface{}{
			"dataset_id": dataset.ID,
			"error":      rerankErr.Error(),
		})
		// If reranking fails, return the original documents
		// This matches the default behavior where it continues with original documents
		return documents, nil
	}

	// Convert back from dto.Document to retrieval.SearchResult
	resultDocuments := make([]retrieval.SearchResult, len(rerankedDocuments))
	for i, doc := range rerankedDocuments {
		score := 0.0
		if s, ok := doc.Metadata["score"].(float64); ok {
			score = s
		}
		if doc.Metadata == nil {
			doc.Metadata = map[string]interface{}{}
		}
		doc.Metadata["final_score"] = score

		id := ""
		if idVal, ok := doc.Metadata["id"].(string); ok {
			id = idVal
		}

		resultDocuments[i] = retrieval.SearchResult{
			ID:       id,
			Content:  doc.PageContent,
			Score:    score,
			Metadata: doc.Metadata,
		}
	}

	return resultDocuments, nil
}

func retrievalSourceResponseForDoc(doc retrieval.SearchResult, fallbackMethod, fallbackReason string) *dto.RetrievalSourceResponse {
	source := &dto.RetrievalSourceResponse{
		Method: fallbackMethod,
		Reason: fallbackReason,
	}
	if doc.Metadata == nil {
		return source
	}

	sources := searchMetadataStringSlice(doc.Metadata["retrieval_sources"])
	matchedTerms := searchMetadataStringSlice(doc.Metadata["matched_terms"])
	source.RetrievalSources = sources
	source.MatchedTerms = matchedTerms
	source.VectorScore = searchMetadataFloatPtr(doc.Metadata["vector_score"])
	source.BM25Score = searchMetadataFloatPtr(doc.Metadata["bm25_score"])
	source.VectorRank = searchMetadataIntPtr(doc.Metadata["vector_rank"])
	source.BM25Rank = searchMetadataIntPtr(doc.Metadata["bm25_rank"])
	source.BestRank = searchMetadataIntPtr(doc.Metadata["best_rank"])
	source.FusionScore = searchMetadataFloatPtr(doc.Metadata["fusion_score"])
	if source.FusionScore == nil {
		source.FusionScore = searchMetadataFloatPtr(doc.Metadata["score"])
	}
	source.RerankScore = searchMetadataFloatPtr(doc.Metadata["rerank_score"])
	source.FinalScore = searchMetadataFloatPtr(doc.Metadata["final_score"])
	if source.FinalScore == nil {
		finalScore := doc.Score
		source.FinalScore = &finalScore
	}

	hasVector := containsString(sources, "vector")
	hasBM25 := containsString(sources, "bm25")
	switch {
	case hasVector && hasBM25:
		source.Method = "hybrid_search"
		source.Reason = "向量语义与BM25词面混合匹配"
	case hasBM25:
		source.Method = "full_text_search"
		source.Reason = "BM25全文匹配"
	case hasVector:
		source.Method = "semantic_search"
		source.Reason = "语义相似度匹配"
	}
	return source
}

func searchMetadataFloatPtr(value interface{}) *float64 {
	switch typed := value.(type) {
	case float64:
		return &typed
	case float32:
		v := float64(typed)
		return &v
	case int:
		v := float64(typed)
		return &v
	case int64:
		v := float64(typed)
		return &v
	case int32:
		v := float64(typed)
		return &v
	default:
		return nil
	}
}

func searchMetadataIntPtr(value interface{}) *int {
	switch typed := value.(type) {
	case int:
		return &typed
	case int64:
		v := int(typed)
		return &v
	case int32:
		v := int(typed)
		return &v
	case float64:
		if typed == math.Trunc(typed) {
			v := int(typed)
			return &v
		}
		return nil
	case float32:
		floatValue := float64(typed)
		if floatValue == math.Trunc(floatValue) {
			v := int(floatValue)
			return &v
		}
		return nil
	default:
		return nil
	}
}

func searchMetadataStringSlice(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []interface{}:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && text != "" {
				items = append(items, text)
			}
		}
		return items
	default:
		return nil
	}
}

func containsString(items []string, item string) bool {
	for _, candidate := range items {
		if candidate == item {
			return true
		}
	}
	return false
}

func filterAndLimitFinalRecords(records []dto.HitTestingRecordResponse, options *RetrievalOptions) []dto.HitTestingRecordResponse {
	if len(records) == 0 {
		return records
	}

	sort.SliceStable(records, func(i, j int) bool {
		return records[i].Score > records[j].Score
	})

	filtered := records
	if options != nil && options.ScoreThresholdEnabled {
		filtered = make([]dto.HitTestingRecordResponse, 0, len(records))
		for _, record := range records {
			if !shouldApplyFinalScoreThreshold(record) || record.Score >= options.ScoreThreshold {
				filtered = append(filtered, record)
			}
		}
	}

	if options != nil && options.TopK > 0 && len(filtered) > options.TopK {
		filtered = filtered[:options.TopK]
	}
	return filtered
}

func shouldApplyFinalScoreThreshold(record dto.HitTestingRecordResponse) bool {
	source := record.RetrievalSource
	if source == nil {
		return true
	}
	return !(source.RerankScore == nil && source.FusionScore != nil && source.BestRank != nil)
}

func (s *RetrievalService) updateRecordHitCount(ctx context.Context, records []dto.HitTestingRecordResponse) {
	if len(records) == 0 {
		return
	}

	seen := make(map[string]struct{}, len(records))
	segmentIDs := make([]string, 0, len(records))
	for _, record := range records {
		segmentID := strings.TrimSpace(record.Segment.ID)
		if segmentID == "" || record.MatchType == dto.MatchTypeGraphKnowledge {
			continue
		}
		if _, ok := seen[segmentID]; ok {
			continue
		}
		seen[segmentID] = struct{}{}
		segmentIDs = append(segmentIDs, segmentID)
	}
	if len(segmentIDs) == 0 {
		return
	}
	if err := s.incrementSegmentHitCount(ctx, segmentIDs); err != nil {
		logger.Error("Failed to increment final record hit count", err)
	}
}

// convertSearchResultsToRecords Convert search results to record responses
func (s *RetrievalService) convertSearchResultsToRecords(documents []retrieval.SearchResult, datasetID string, options *RetrievalOptions) []dto.HitTestingRecordResponse {
	records := make([]dto.HitTestingRecordResponse, 0, len(documents))

	// Collect document IDs for batch query
	documentIDs := make(map[string]bool)
	for _, doc := range documents {
		// Extract document_id from metadata
		if doc.Metadata != nil {
			if docID, ok := doc.Metadata["document_id"].(string); ok && docID != "" {
				documentIDs[docID] = true
			}
		}
	}

	// Batch query dataset documents
	datasetDocuments := make(map[string]*dataset_model.Document)
	if len(documentIDs) > 0 {
		ids := make([]string, 0, len(documentIDs))
		for id := range documentIDs {
			ids = append(ids, id)
		}

		docs, err := s.documentRepo.GetDocumentsByIDs(context.Background(), ids)
		if err == nil {
			for _, doc := range docs {
				datasetDocuments[doc.ID] = doc
			}
		} else {
			logger.Warn("Failed to batch query dataset documents", map[string]interface{}{
				"error":        err.Error(),
				"document_ids": ids,
			})
		}
	}

	logger.Debug("convertSearchResultsToRecords debug", map[string]interface{}{
		"total_search_results":    len(documents),
		"unique_document_ids":     len(documentIDs),
		"found_dataset_documents": len(datasetDocuments),
	})

	// Track processed segment IDs and child chunk mapping
	processedSegmentIDs := make(map[string]bool)
	segmentChildMap := make(map[string]*struct {
		MaxScore      float64
		ChildChunks   []dto.ChildChunkResponse
		ChildChunkIDs map[string]struct{}
	})

	// Process documents
	for i, doc := range documents {
		// Handle Graph Knowledge - Special Case
		if source, ok := doc.Metadata["source"].(string); ok && source == "graph_knowledge" {
			matchedEntitiesRaw, _ := doc.Metadata["matched_entities"].([]string)
			matchedEntities := matchedEntitiesRaw
			if matchedEntities == nil {
				// Fallback to interface slice if necessary
				if rawList, ok := doc.Metadata["matched_entities"].([]interface{}); ok {
					for _, item := range rawList {
						if str, ok := item.(string); ok {
							matchedEntities = append(matchedEntities, str)
						}
					}
				}
			}
			if len(matchedEntities) > 0 {
				_ = matchedEntities[0] // Place holder
			}
			record := dto.HitTestingRecordResponse{
				Segment: dto.SegmentResponse{
					ID:          doc.ID,
					Content:     doc.Content,
					SignContent: doc.Content,
					WordCount:   len(doc.Content),
					Status:      "completed",
					Enabled:     true,
					Document: dto.HitTestingDocumentResponse{
						ID:             "graph_knowledge",
						Name:           "Graph Knowledge",
						DataSourceType: "graph_knowledge",
					},
					Keywords: matchedEntities,
				},
				Score:     doc.Score,
				MatchType: dto.MatchTypeGraphKnowledge,
				TSNEPosition: map[string]interface{}{
					"x": float64(i) * 0.1,
					"y": float64(i) * 0.1,
				},
				RetrievalSource: &dto.RetrievalSourceResponse{
					Method:          "graph_knowledge",
					Reason:          "通过知识图谱实体关联找到",
					MatchedEntities: matchedEntities,
				},
			}
			records = append(records, record)
			continue
		}

		// Get document_id
		var documentID string
		if doc.Metadata != nil {
			if docID, ok := doc.Metadata["document_id"].(string); ok {
				documentID = docID
			}
		}

		// Check if dataset document exists
		datasetDocument, datasetDocumentExists := datasetDocuments[documentID]
		if !datasetDocumentExists {
			logger.Warn("Dataset document not found, skipping result", map[string]interface{}{
				"document_id": documentID,
				"doc_id":      doc.Metadata["doc_id"],
				"dataset_id":  datasetID,
			})
			continue
		}

		logger.Debug("Processing search result", map[string]interface{}{
			"document_id": documentID,
			"doc_id":      doc.Metadata["doc_id"],
			"doc_form":    datasetDocument.DocForm,
			"doc_hash":    doc.Metadata["doc_hash"],
		})

		// use doc_hash to check if question document
		isQuestion := false
		// var questionID string
		if docHashVal, exists := doc.Metadata["doc_hash"]; exists && docHashVal != nil {
			if docHashStr, ok := docHashVal.(string); ok {
				// check doc_hash is "question/" prefix
				if strings.HasPrefix(docHashStr, "question/") {
					// questionID = strings.TrimPrefix(docHashStr, "question/")
					isQuestion = true
				}
			}
		}

		if isQuestion {
			// Handle question documents
			indexNodeID, _ := doc.Metadata["doc_id"].(string)
			// Query segment by IndexNodeID directly
			segment, err := s.documentRepo.GetDocumentSegmentByIndexNodeID(context.Background(), indexNodeID)
			if err != nil || segment == nil {
				logger.Warn("Segment not found by IndexNodeID (question path)", map[string]interface{}{
					"index_node_id": indexNodeID,
					"document_id":   documentID,
				})
				continue
			}
			if !segment.Enabled {
				continue
			}

			// Convert keywords
			var keywords []string
			if segment.Keywords != nil {
				keywordsMap := map[string]interface{}(segment.Keywords)
				if keywordList, ok := keywordsMap["keywords"].([]interface{}); ok {
					for _, kw := range keywordList {
						if kwStr, ok := kw.(string); ok {
							keywords = append(keywords, kwStr)
						}
					}
				}
			}

			// Handle optional fields
			var answer string
			if segment.Answer != nil {
				answer = *segment.Answer
			}

			var indexNodeIDPtr *string
			if segment.IndexNodeID != nil {
				indexNodeIDPtr = segment.IndexNodeID
			}

			var indexNodeHashPtr *string
			if segment.IndexNodeHash != nil {
				indexNodeHashPtr = segment.IndexNodeHash
			}

			var disabledAt, indexingAt, completedAt, stoppedAt *int64
			if segment.DisabledAt != nil {
				t := segment.DisabledAt.Unix()
				disabledAt = &t
			}
			if segment.IndexingAt != nil {
				t := segment.IndexingAt.Unix()
				indexingAt = &t
			}
			if segment.CompletedAt != nil {
				t := segment.CompletedAt.Unix()
				completedAt = &t
			}
			if segment.StoppedAt != nil {
				t := segment.StoppedAt.Unix()
				stoppedAt = &t
			}

			// Load child chunk information
			childChunks := s.loadChildChunksForSegment(segment)

			// Load document information
			var documentInfo dto.HitTestingDocumentResponse
			// Load dataset process rule
			var datasetProcessRule map[string]interface{}
			if segment.DocumentID != "" {
				docInfo, err := s.documentRepo.GetDocumentByID(context.Background(), segment.DocumentID)
				if err == nil && docInfo != nil {
					docType := ""
					if docInfo.DocType != nil {
						docType = *docInfo.DocType
					}
					documentInfo = dto.HitTestingDocumentResponse{
						ID:             docInfo.ID,
						DataSourceType: docInfo.DataSourceType,
						Name:           docInfo.Name,
						DocType:        docType,
						DocMetadata:    map[string]interface{}(docInfo.DocMetadata),
					}

					// Get dataset process rule if DatasetProcessRuleID exists
					if docInfo.DatasetProcessRuleID != nil && *docInfo.DatasetProcessRuleID != "" {
						if processRule, err := s.documentRepo.GetProcessRuleByID(context.Background(), *docInfo.DatasetProcessRuleID); err == nil && processRule != nil {
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
			matchType := dto.MatchTypeQuestion
			// Create record
			record := dto.HitTestingRecordResponse{
				Segment: dto.SegmentResponse{
					ID:                 segment.ID,
					Position:           segment.Position,
					DocumentID:         segment.DocumentID,
					Content:            segment.Content,
					SignContent:        segment.Content,
					Answer:             &answer,
					WordCount:          segment.WordCount,
					Tokens:             segment.Tokens,
					Keywords:           keywords,
					IndexNodeID:        indexNodeIDPtr,
					IndexNodeHash:      indexNodeHashPtr,
					HitCount:           segment.HitCount,
					Enabled:            segment.Enabled,
					DisabledAt:         disabledAt,
					DisabledBy:         segment.DisabledBy,
					Status:             segment.Status,
					CreatedBy:          segment.CreatedBy,
					CreatedAt:          segment.CreatedAt.Unix(),
					IndexingAt:         indexingAt,
					CompletedAt:        completedAt,
					Error:              segment.Error,
					StoppedAt:          stoppedAt,
					Document:           documentInfo,
					DatasetProcessRule: datasetProcessRule,
				},
				Score: doc.Score,
				TSNEPosition: map[string]interface{}{
					"x": 0.1,
					"y": 0.1,
					// TODO: Implement complete TSNE position calculation
				},
				ChildChunks:     childChunks,
				MatchType:       matchType,
				RetrievalSource: retrievalSourceResponseForDoc(doc, "semantic_search", "语义相似度匹配（问答对）"),
			}

			// If the segment has a child chunk mapping, update the score and child chunk information
			if segmentChildMap[segment.ID] != nil {
				record.ChildChunks = segmentChildMap[segment.ID].ChildChunks
				record.Score = segmentChildMap[segment.ID].MaxScore
			}

			records = append(records, record)
		} else {
			// Determine if this should be handled as hierarchical (parent-child) document
			useHierarchical := false
			var hierarchicalChildChunk *dataset_model.ChildChunk
			if datasetDocument.DocForm == "hierarchical_model" || datasetDocument.DocForm == "table_model" {
				// doc_id in vector DB is the child chunk's index_node_id, not segment_id
				indexNodeID, _ := doc.Metadata["doc_id"].(string)
				childChunk, err := s.documentRepo.GetChildChunkByIndexNodeID(context.Background(), indexNodeID)
				if err == nil && childChunk != nil {
					logger.Debug("Found child chunk by index_node_id", map[string]interface{}{
						"child_chunk_id": childChunk.ID,
						"segment_id":     childChunk.SegmentID,
					})
					useHierarchical = true
					hierarchicalChildChunk = childChunk
				} else {
					logger.Warn("Child chunk not found by index_node_id, trying segment_id fallback", map[string]interface{}{
						"index_node_id": indexNodeID,
						"error":         err,
					})
					// Fallback: if no child chunk found by index_node_id, try as segment_id for backward compatibility
					segmentID := indexNodeID
					childChunks, err := s.documentRepo.GetChildChunksBySegmentID(context.Background(), segmentID)
					if err == nil && len(childChunks) > 0 {
						useHierarchical = true
						hierarchicalChildChunk = &childChunks[0]
					} else {
						logger.Debug("No child chunks found for hierarchical_model, falling back to regular segment handling", map[string]interface{}{
							"index_node_id": indexNodeID,
							"document_id":   documentID,
						})
					}
				}
			}

			if useHierarchical && hierarchicalChildChunk != nil {
				// Handle parent-child documents
				childChunk := hierarchicalChildChunk

				// Query segment
				segment, err := s.documentRepo.GetDocumentSegmentByID(context.Background(), childChunk.SegmentID)
				if err != nil || segment == nil {
					continue
				}
				if !segment.Enabled {
					continue
				}
				// Check if segment has been processed
				if !processedSegmentIDs[segment.ID] {
					processedSegmentIDs[segment.ID] = true

					// Convert keywords
					var keywords []string
					if segment.Keywords != nil {
						keywordsMap := map[string]interface{}(segment.Keywords)
						if keywordList, ok := keywordsMap["keywords"].([]interface{}); ok {
							for _, kw := range keywordList {
								if kwStr, ok := kw.(string); ok {
									keywords = append(keywords, kwStr)
								}
							}
						}
					}

					// Handle optional fields
					var answer string
					if segment.Answer != nil {
						answer = *segment.Answer
					}

					var indexNodeIDPtr *string
					if segment.IndexNodeID != nil {
						indexNodeIDPtr = segment.IndexNodeID
					}

					var indexNodeHashPtr *string
					if segment.IndexNodeHash != nil {
						indexNodeHashPtr = segment.IndexNodeHash
					}

					var disabledAt, indexingAt, completedAt, stoppedAt *int64
					if segment.DisabledAt != nil {
						t := segment.DisabledAt.Unix()
						disabledAt = &t
					}
					if segment.IndexingAt != nil {
						t := segment.IndexingAt.Unix()
						indexingAt = &t
					}
					if segment.CompletedAt != nil {
						t := segment.CompletedAt.Unix()
						completedAt = &t
					}
					if segment.StoppedAt != nil {
						t := segment.StoppedAt.Unix()
						stoppedAt = &t
					}

					// Load document information
					var documentInfo dto.HitTestingDocumentResponse
					if segment.DocumentID != "" {
						docInfo, err := s.documentRepo.GetDocumentByID(context.Background(), segment.DocumentID)
						if err == nil && docInfo != nil {
							docType := ""
							if docInfo.DocType != nil {
								docType = *docInfo.DocType
							}
							documentInfo = dto.HitTestingDocumentResponse{
								ID:             docInfo.ID,
								DataSourceType: docInfo.DataSourceType,
								Name:           docInfo.Name,
								DocType:        docType,
								DocMetadata:    map[string]interface{}(docInfo.DocMetadata),
							}
						}
					}

					// Create child chunk response
					childChunkResp := dto.ChildChunkResponse{
						ID:        childChunk.ID,
						SegmentID: childChunk.SegmentID,
						Content:   childChunk.Content,
						Position:  childChunk.Position,
						WordCount: childChunk.WordCount,
						Type:      childChunk.Type,
						Score:     doc.Score,
						CreatedAt: childChunk.CreatedAt.Unix(),
						UpdatedAt: childChunk.UpdatedAt.Unix(),
					}

					// Add to segment child chunk mapping
					segmentChildMap[segment.ID] = &struct {
						MaxScore      float64
						ChildChunks   []dto.ChildChunkResponse
						ChildChunkIDs map[string]struct{}
					}{
						MaxScore:      doc.Score,
						ChildChunks:   []dto.ChildChunkResponse{childChunkResp},
						ChildChunkIDs: map[string]struct{}{childChunk.ID: {}},
					}

					record := dto.HitTestingRecordResponse{
						Segment: dto.SegmentResponse{
							ID:            segment.ID,
							Position:      segment.Position,
							DocumentID:    segment.DocumentID,
							Content:       segment.Content,
							SignContent:   segment.Content,
							Answer:        &answer,
							WordCount:     segment.WordCount,
							Tokens:        segment.Tokens,
							Keywords:      keywords,
							IndexNodeID:   indexNodeIDPtr,
							IndexNodeHash: indexNodeHashPtr,
							HitCount:      segment.HitCount,
							Enabled:       segment.Enabled,
							DisabledAt:    disabledAt,
							DisabledBy:    segment.DisabledBy,
							Status:        segment.Status,
							CreatedBy:     segment.CreatedBy,
							CreatedAt:     segment.CreatedAt.Unix(),
							IndexingAt:    indexingAt,
							CompletedAt:   completedAt,
							Error:         segment.Error,
							StoppedAt:     stoppedAt,
							Document:      documentInfo,
						},
						Score: doc.Score,
						TSNEPosition: map[string]interface{}{
							"x": 0.1,
							"y": 0.1,
						},
						ChildChunks:     []dto.ChildChunkResponse{childChunkResp},
						MatchType:       dto.MatchTypeOriginal,
						RetrievalSource: retrievalSourceResponseForDoc(doc, "semantic_search", "语义相似度匹配（父子索引）"),
					}
					records = append(records, record)
				} else {
					// Segment already exists, update child chunk information
					if segmentChildMap[segment.ID] != nil {
						childChunkResp := dto.ChildChunkResponse{
							ID:        childChunk.ID,
							SegmentID: childChunk.SegmentID,
							Content:   childChunk.Content,
							Position:  childChunk.Position,
							WordCount: childChunk.WordCount,
							Type:      childChunk.Type,
							Score:     doc.Score,
							CreatedAt: childChunk.CreatedAt.Unix(),
							UpdatedAt: childChunk.UpdatedAt.Unix(),
						}

						if _, exists := segmentChildMap[segment.ID].ChildChunkIDs[childChunk.ID]; !exists {
							segmentChildMap[segment.ID].ChildChunks = append(segmentChildMap[segment.ID].ChildChunks, childChunkResp)
							segmentChildMap[segment.ID].ChildChunkIDs[childChunk.ID] = struct{}{}
						}
						if doc.Score > segmentChildMap[segment.ID].MaxScore {
							segmentChildMap[segment.ID].MaxScore = doc.Score
						}
					}
				}
			} else {
				// Handle regular documents
				indexNodeID, _ := doc.Metadata["doc_id"].(string)

				// Query segment by IndexNodeID directly
				segment, err := s.documentRepo.GetDocumentSegmentByIndexNodeID(context.Background(), indexNodeID)
				if err != nil || segment == nil {
					logger.Warn("Segment not found by IndexNodeID", map[string]interface{}{
						"index_node_id": indexNodeID,
						"document_id":   documentID,
					})
					continue
				}
				if !segment.Enabled {
					continue
				}

				// Convert keywords
				var keywords []string
				if segment.Keywords != nil {
					keywordsMap := map[string]interface{}(segment.Keywords)
					if keywordList, ok := keywordsMap["keywords"].([]interface{}); ok {
						for _, kw := range keywordList {
							if kwStr, ok := kw.(string); ok {
								keywords = append(keywords, kwStr)
							}
						}
					}
				}

				// Handle optional fields
				var answer string
				if segment.Answer != nil {
					answer = *segment.Answer
				}

				var indexNodeIDPtr *string
				if segment.IndexNodeID != nil {
					indexNodeIDPtr = segment.IndexNodeID
				}

				var indexNodeHashPtr *string
				if segment.IndexNodeHash != nil {
					indexNodeHashPtr = segment.IndexNodeHash
				}

				var disabledAt, indexingAt, completedAt, stoppedAt *int64
				if segment.DisabledAt != nil {
					t := segment.DisabledAt.Unix()
					disabledAt = &t
				}
				if segment.IndexingAt != nil {
					t := segment.IndexingAt.Unix()
					indexingAt = &t
				}
				if segment.CompletedAt != nil {
					t := segment.CompletedAt.Unix()
					completedAt = &t
				}
				if segment.StoppedAt != nil {
					t := segment.StoppedAt.Unix()
					stoppedAt = &t
				}

				// Load child chunk information
				childChunks := s.loadChildChunksForSegment(segment)

				// Load document information
				var documentInfo dto.HitTestingDocumentResponse
				// Load dataset process rule
				var datasetProcessRule map[string]interface{}
				if segment.DocumentID != "" {
					docInfo, err := s.documentRepo.GetDocumentByID(context.Background(), segment.DocumentID)
					if err == nil && docInfo != nil {
						docType := ""
						if docInfo.DocType != nil {
							docType = *docInfo.DocType
						}
						documentInfo = dto.HitTestingDocumentResponse{
							ID:             docInfo.ID,
							DataSourceType: docInfo.DataSourceType,
							Name:           docInfo.Name,
							DocType:        docType,
							DocMetadata:    map[string]interface{}(docInfo.DocMetadata),
						}

						// Get dataset process rule if DatasetProcessRuleID exists
						if docInfo.DatasetProcessRuleID != nil && *docInfo.DatasetProcessRuleID != "" {
							if processRule, err := s.documentRepo.GetProcessRuleByID(context.Background(), *docInfo.DatasetProcessRuleID); err == nil && processRule != nil {
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
				matchType := dto.MatchTypeOriginal
				// Create record
				record := dto.HitTestingRecordResponse{
					Segment: dto.SegmentResponse{
						ID:                 segment.ID,
						Position:           segment.Position,
						DocumentID:         segment.DocumentID,
						Content:            segment.Content,
						SignContent:        segment.Content,
						Answer:             &answer,
						WordCount:          segment.WordCount,
						Tokens:             segment.Tokens,
						Keywords:           keywords,
						IndexNodeID:        indexNodeIDPtr,
						IndexNodeHash:      indexNodeHashPtr,
						HitCount:           segment.HitCount,
						Enabled:            segment.Enabled,
						DisabledAt:         disabledAt,
						DisabledBy:         segment.DisabledBy,
						Status:             segment.Status,
						CreatedBy:          segment.CreatedBy,
						CreatedAt:          segment.CreatedAt.Unix(),
						IndexingAt:         indexingAt,
						CompletedAt:        completedAt,
						Error:              segment.Error,
						StoppedAt:          stoppedAt,
						Document:           documentInfo,
						DatasetProcessRule: datasetProcessRule,
					},
					Score: doc.Score,
					TSNEPosition: map[string]interface{}{
						"x": 0.1,
						"y": 0.1,
						// TODO: Implement complete TSNE position calculation
					},
					ChildChunks:     childChunks,
					MatchType:       matchType,
					RetrievalSource: retrievalSourceResponseForDoc(doc, "semantic_search", "语义相似度匹配"),
				}

				// If the segment has a child chunk mapping, update the score and child chunk information
				if segmentChildMap[segment.ID] != nil {
					record.ChildChunks = segmentChildMap[segment.ID].ChildChunks
					record.Score = segmentChildMap[segment.ID].MaxScore
				}

				records = append(records, record)
			}
		}
	}

	// Update child chunk information and scores in records
	for i := range records {
		segmentID := records[i].Segment.ID
		if segmentChildMap[segmentID] != nil {
			records[i].ChildChunks = segmentChildMap[segmentID].ChildChunks
			records[i].Score = segmentChildMap[segmentID].MaxScore
		}
	}
	// If we need to return full documents, process them accordingly
	// Full document return logic removed as per requirement

	return records
}

// loadChildChunksForSegment Load child chunk information for a segment
func (s *RetrievalService) loadChildChunksForSegment(segment *dataset_model.DocumentSegment) []dto.ChildChunkResponse {
	var childChunks []dto.ChildChunkResponse

	items, err := s.documentRepo.GetChildChunksBySegmentID(context.Background(), segment.ID)
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

// incrementSegmentHitCount increments the hit count for the given segment IDs
func (s *RetrievalService) incrementSegmentHitCount(ctx context.Context, segmentIDs []string) error {
	// Use repository method to increment hit counts for multiple segments
	if err := s.documentRepo.IncrementSegmentHitCount(ctx, segmentIDs); err != nil {
		return fmt.Errorf("failed to increment segment hit count: %w", err)
	}
	return nil
}

// updateSegmentHitCount updates the hit count for segments in the search results
func (s *RetrievalService) updateSegmentHitCount(ctx context.Context, documents []retrieval.SearchResult) {
	// Collect segment IDs and child chunk segment IDs from the search results
	segmentIDs := make([]string, 0, len(documents))
	childChunkSegmentIDs := make([]string, 0, len(documents))

	// Process each document in the search results
	for _, doc := range documents {
		if indexNodeID, ok := doc.Metadata["doc_id"].(string); ok && indexNodeID != "" {
			// Get the document to check its form type
			datasetDocumentID, docIDExists := doc.Metadata["document_id"].(string)
			if !docIDExists || datasetDocumentID == "" {
				// If document_id is not in metadata, try to get segment by IndexNodeID
				segment, err := s.documentRepo.GetDocumentSegmentByIndexNodeID(ctx, indexNodeID)
				if err != nil || segment == nil {
					continue
				}

				// Get the document to check its form type
				document, docErr := s.documentRepo.GetDocumentByID(ctx, segment.DocumentID)
				if docErr != nil || document == nil {
					// If we can't get the document, just increment the segment hit count
					segmentIDs = append(segmentIDs, segment.ID)
					continue
				}

				// Check if this is a parent-child index type document
				if document.DocForm == "hierarchical_model" || document.DocForm == "table_model" {
					// For parent-child index, we need to increment the child chunk hit count
					childChunkSegmentIDs = append(childChunkSegmentIDs, segment.ID)
				} else {
					// For regular documents, increment the segment hit count
					segmentIDs = append(segmentIDs, segment.ID)
				}
			} else {
				// Check if dataset document is parent-child index type
				datasetDocument, datasetDocErr := s.documentRepo.GetDocumentByID(ctx, datasetDocumentID)
				if datasetDocErr != nil || datasetDocument == nil {
					continue
				}

				if datasetDocument.DocForm == "hierarchical_model" || datasetDocument.DocForm == "table_model" {
					// For parent-child index, get child chunk and increment its hit count
					childChunk, childErr := s.documentRepo.GetChildChunkByIndexNodeID(ctx, indexNodeID)
					if childErr != nil || childChunk == nil {
						continue
					}

					// Add child chunk's segment ID to childChunkSegmentIDs for hit count increment
					childChunkSegmentIDs = append(childChunkSegmentIDs, childChunk.SegmentID)
				} else {
					// For regular documents, get segment and increment its hit count
					segment, segErr := s.documentRepo.GetDocumentSegmentByIndexNodeID(ctx, indexNodeID)
					if segErr != nil || segment == nil {
						continue
					}

					segmentIDs = append(segmentIDs, segment.ID)
				}
			}
		}
	}

	// Increment hit count for segments
	if len(segmentIDs) > 0 {
		if err := s.incrementSegmentHitCount(ctx, segmentIDs); err != nil {
			logger.Error("Failed to increment segment hit count", err)
		}
	}

	// Increment hit count for child chunks (which are stored as segments in the hierarchical model)
	if len(childChunkSegmentIDs) > 0 {
		if err := s.incrementSegmentHitCount(ctx, childChunkSegmentIDs); err != nil {
			logger.Error("Failed to increment child chunk hit count", err)
		}
	}
}

// graphSearch (GraphFlow) - Enhanced retrieval using knowledge graph
func (s *RetrievalService) graphSearch(ctx context.Context, dataset *dataset_model.Dataset, query string, options *RetrievalOptions) ([]retrieval.SearchResult, *dto.GraphExecution, error) {
	if s.graphFlowService == nil {
		return []retrieval.SearchResult{}, nil, nil
	}
	graphLogCtx := logger.WithFields(ctx,
		zap.String("dataset_id", dataset.ID),
		zap.String("tenant_id", dataset.WorkspaceID),
		zap.String("organization_id", dataset.OrganizationID),
		zap.Int("query_length", len(query)),
	)

	execution := &dto.GraphExecution{
		Steps: []dto.GraphExecutionStep{},
	}

	// 1. Extract entities from query
	// Use 120s timeout for LLM extraction because the gateway is currently very slow
	extractCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	entities, err := s.graphFlowService.ExtractQueryEntities(extractCtx, dataset.OrganizationID, query, dataset.EntityModel, dataset.EntityModelProvider)
	if err != nil {
		logger.WarnContext(graphLogCtx, "failed to extract entities for graph search", err)
		execution.Steps = append(execution.Steps, dto.GraphExecutionStep{
			Step: 1, Action: "extract_entities", Description: "从查询中提取实体", Result: fmt.Sprintf("提取失败: %v", err),
		})
		execution.Summary = fmt.Sprintf("由于实体提取超时或失败，跳过了图谱搜索。详细原因: %v", err)
		return []retrieval.SearchResult{}, execution, err
	}

	// Filter and clean entities
	var cleanEntities []string
	seenEntities := make(map[string]bool)
	for _, e := range entities {
		e = strings.TrimSpace(e)
		if e != "" && !seenEntities[e] && len([]rune(e)) > 1 {
			seenEntities[e] = true
			cleanEntities = append(cleanEntities, e)
		}
	}
	entities = cleanEntities

	logger.Info("Extracted and cleaned entities for graph search", map[string]interface{}{
		"query":    query,
		"entities": entities,
	})

	// Start Step 1
	execution.Entities = entities
	execution.Steps = append(execution.Steps, dto.GraphExecutionStep{
		Step: 1, Action: "extract_entities", Description: "从查询中提取实体", Result: strings.Join(entities, ", "),
	})

	if len(entities) == 0 {
		return []retrieval.SearchResult{}, &dto.GraphExecution{
			Steps:   []dto.GraphExecutionStep{{Step: 1, Action: "extract_entities", Description: "从查询中提取实体", Result: "未提取到实体"}},
			Summary: "未能从查询中提取到相关实体。",
		}, nil
	}

	// 1.5 Detect Retrieval Boundary
	kbUUID, _ := uuid.Parse(dataset.ID)
	boundary, err := s.boundaryDetector.Detect(ctx, kbUUID, entities)
	if err != nil {
		logger.Warn("[GRAPH_SEARCH] Boundary detection failed, falling back to global", map[string]interface{}{"error": err})
		boundary = &graphflow_retrieval.Boundary{Type: graphflow_retrieval.BoundaryTypeGlobal}
	}

	// Adjust hop depth based on boundary precision
	effectiveHopDepth := options.HopDepth
	if boundary.Type == graphflow_retrieval.BoundaryTypeAnchored {
		// For anchored queries (known relations), stay closer to maintain precision
		if effectiveHopDepth > 2 {
			effectiveHopDepth = 2
		}
	} else if boundary.Type == graphflow_retrieval.BoundaryTypeSingle {
		// For single entity, allow more exploration but with pruning (handled in Neo4j)
		if effectiveHopDepth < 2 {
			effectiveHopDepth = 2
		}
	}

	logger.Info("[GRAPH_SEARCH] Detected boundary", map[string]interface{}{
		"type":       boundary.Type,
		"entity_ids": len(boundary.EntityIDs),
		"hop_depth":  effectiveHopDepth,
	})

	execution.Steps = append(execution.Steps, dto.GraphExecutionStep{
		Step: 2, Action: "detect_boundary", Description: "检测检索边界", Result: fmt.Sprintf("边界类型: %v, 实体数: %d", boundary.Type, len(boundary.EntityIDs)),
	})

	// 2. Vector-Based Entity Matching & Multi-hop Traversal
	// Combine both Vector Search (semantic) and Exact Match (structural)
	// Update execution with extracted entities

	// 2. Async Pre-compute query embedding to prevent blocking main routine
	queryEmbeddingChan := make(chan []float64, 2)
	go func() {
		var qEmb []float64
		embeddingService := s.getEmbeddingService(ctx, dataset)
		if embeddingService != nil {
			if emb, err := embeddingService.EmbedText(ctx, query); err == nil && len(emb) > 0 {
				qEmb = emb
			} else {
				logger.Warn("[GRAPH_SEARCH] Failed to extract query embedding", map[string]interface{}{"error": err})
			}
		}
		queryEmbeddingChan <- qEmb
		queryEmbeddingChan <- qEmb // Send twice for multiple consumers to prevent race condition
	}()

	// Async launch Weaviate database vector extraction for candidate chunks reuse
	weaviateScoreMap := make(map[string]float64)
	var weaviateMu sync.Mutex
	weaviateDone := make(chan bool, 1)

	go func() {
		qEmb := <-queryEmbeddingChan

		if len(qEmb) > 0 && s.vectorRetrieval != nil && s.vectorClient != nil {
			className := dataset_model.GenCollectionNameByID(dataset.ID)

			// Use a reasonably high top-k for candidate chunks
			results, err := s.vectorClient.SearchVectors(ctx, className, qEmb, 100)
			if err == nil {
				minScore := options.SemanticMinScore
				if minScore <= 0 {
					minScore = 0.5
				}
				if boundary.Type == graphflow_retrieval.BoundaryTypeAnchored && options.AnchoredMinScore > 0 {
					minScore = options.AnchoredMinScore
				}

				weaviateMu.Lock()
				for _, res := range results {
					docID := ""
					if id, ok := res["doc_id"].(string); ok {
						docID = id
					} else if additional, ok := res["_additional"].(map[string]interface{}); ok {
						if aid, ok := additional["id"].(string); ok {
							docID = aid
						}
					}

					score := 0.0
					if additional, ok := res["_additional"].(map[string]interface{}); ok {
						if distance, ok := additional["distance"].(float64); ok {
							score = 1.0 - distance
						}
					}

					// FILTER: Only keep chunks that pass the semantic floor
					if docID != "" && score >= minScore {
						weaviateScoreMap[docID] = score
					}
				}
				weaviateMu.Unlock()
			} else {
				logger.Warn("[GRAPH_SEARCH] Failed to fetch Weaviate chunk scores", map[string]interface{}{"error": err})
			}
		}
		weaviateDone <- true
	}()

	// 3. Parallel Vector-Based and Multi-hop Traversal
	var finalContextResults []graph.EntitySearchResult
	seenResultEntities := make(map[string]bool)
	uniqueEntityMap := make(map[string]bool)
	var allGraphEntities []string
	var vectorResults []graphflow_retrieval.EntityMatch
	var mu sync.Mutex

	type graphPathResult struct {
		cResults []graph.EntitySearchResult
		entities []string
		source   string
		err      error
	}
	resultChan := make(chan graphPathResult, 3)

	// Helper to add results with deduplication (Thread-safe)
	addResultsLocked := func(results []graph.EntitySearchResult, source string, gEntities []string) {
		mu.Lock()
		defer mu.Unlock()

		for _, res := range results {
			id, _ := res.Entity["id"].(string)
			name, _ := res.Entity["name"].(string)
			key := id
			if key == "" {
				key = name
			}

			if key != "" {
				logger.DebugContext(graphLogCtx, "processing graph entity retrieval result",
					zap.String("source", source),
					zap.String("entity_key", key),
					zap.Int("neighbors_count", len(res.Neighbors)),
				)
				if !seenResultEntities[key] {
					seenResultEntities[key] = true
					finalContextResults = append(finalContextResults, res)
				} else {
					// MERGE LOGIC: If entity already exists, add its neighbors to the existing entry
					// This ensures that different paths (vector, multihop, smart) complement each other.
					for i, existing := range finalContextResults {
						existingID, _ := existing.Entity["id"].(string)
						existingName, _ := existing.Entity["name"].(string)
						existingKey := existingID
						if existingKey == "" {
							existingKey = existingName
						}

						if existingKey == key {
							// Deduplicate and merge neighbors
							seenNbr := make(map[string]bool)
							for _, n := range finalContextResults[i].Neighbors {
								nID, _ := n.Node["id"].(string)
								nName, _ := n.Node["name"].(string)
								k := nID
								if k == "" {
									k = nName
								}
								seenNbr[n.RelationshipType+":"+k] = true
							}
							for _, n := range res.Neighbors {
								nID, _ := n.Node["id"].(string)
								nName, _ := n.Node["name"].(string)
								k := nID
								if k == "" {
									k = nName
								}
								if k != "" && !seenNbr[n.RelationshipType+":"+k] {
									seenNbr[n.RelationshipType+":"+k] = true
									finalContextResults[i].Neighbors = append(finalContextResults[i].Neighbors, n)
								}
							}

							// Also merge scores if the new one is higher
							if res.Score > finalContextResults[i].Score {
								finalContextResults[i].Score = res.Score
							}

							if smartScore, ok := res.Entity["_smart_score"].(float64); ok {
								finalContextResults[i].Entity["_smart_score"] = smartScore
							}
							break
						}
					}
				}
			}

			if source == "vector" && res.Score > 0 {
				vectorResults = append(vectorResults, graphflow_retrieval.EntityMatch{
					EntityID: id,
					Name:     name,
					Score:    res.Score,
				})
			}
		}

		for _, e := range gEntities {
			if e != "" && !uniqueEntityMap[e] {
				uniqueEntityMap[e] = true
				allGraphEntities = append(allGraphEntities, e)
			}
		}
	}

	// Path A: Multi-hop exact match traversal
	go func() {
		results, entities, err := s.graphFlowService.Neo4jClient.GetEntityContextMultiHop(ctx, dataset.ID, entities, effectiveHopDepth)
		resultChan <- graphPathResult{cResults: results, entities: entities, source: "multi_hop", err: err}
	}()

	// Path B: Vector similarity expansion
	go func() {
		qEmb := <-queryEmbeddingChan
		if len(qEmb) == 0 {
			resultChan <- graphPathResult{source: "vector", err: fmt.Errorf("no query embedding")}
			return
		}
		// Convert []float64 to []float32 for Neo4j
		qEmb32 := make([]float32, len(qEmb))
		for i, v := range qEmb {
			qEmb32[i] = float32(v)
		}

		// EXTRACT: Boundary Anchors
		anchorIDs := make([]string, len(boundary.EntityIDs))
		for i, id := range boundary.EntityIDs {
			anchorIDs[i] = id.String()
		}

		// FIX: Entity-level vector matching naturally yields lower cosine similarities
		// than chunk-level matching because entity names/short descriptions lack context.
		// Therefore, we use a much more relaxed threshold here to avoid starvation.
		minScore := options.SemanticMinScore
		if minScore <= 0 {
			minScore = 0.55
		}
		if boundary.Type == graphflow_retrieval.BoundaryTypeAnchored && options.AnchoredMinScore > 0 {
			minScore = options.AnchoredMinScore
		}

		// Topic extraction: use non-primary entities as topic keywords to improve intent matching (for example, "university")
		topicKeywords := []string{}
		if len(entities) > 1 {
			topicKeywords = entities[1:]
		}

		cRes, gEnts, err := s.graphFlowService.Neo4jClient.GetEntityContextByVector(ctx, dataset.ID, qEmb32, 10, effectiveHopDepth, anchorIDs, minScore, topicKeywords)
		if err != nil {
			logger.Warn("[GRAPH_SEARCH] Vector search failed", map[string]interface{}{"error": err})
		}

		resultChan <- graphPathResult{cRes, gEnts, "vector", err}
	}()

	// Path B: Smart Expansion (Anchor-based + Adaptive Pruning)
	go func() {
		// Use Smart Expansion to retrieve enriched context
		scoredNodes, err := s.graphFlowService.Neo4jClient.RetrieveEnrichedContext(ctx, dataset.ID, entities)
		if err != nil {
			resultChan <- graphPathResult{nil, nil, "smart_expansion", err}
			return
		}

		// Convert ScoredNodes to EntitySearchResult + Reconstruct Edges
		// 1. Collect all nodes and edges
		nodePropsMap := make(map[string]map[string]interface{})
		allEdges := make([]graph.Edge, 0)

		for _, sn := range scoredNodes {
			if sn.Node.Name != "" {
				// Inject smart score
				sn.Node.Props["_smart_score"] = sn.Score
				nodePropsMap[sn.Node.Name] = sn.Node.Props
			}
			allEdges = append(allEdges, sn.Edges...)
		}

		// Also add neighbor nodes to nodePropsMap (they may not be in scoredNodes)
		for _, sn := range scoredNodes {
			for _, edge := range sn.Edges {
				// Add head node if not present
				if _, ok := nodePropsMap[edge.Head]; !ok {
					nodePropsMap[edge.Head] = map[string]interface{}{"name": edge.Head}
				}
				// Add tail node if not present
				if _, ok := nodePropsMap[edge.Tail]; !ok {
					nodePropsMap[edge.Tail] = map[string]interface{}{"name": edge.Tail}
				}
			}
		}

		var cRes []graph.EntitySearchResult
		seenEntityNames := make(map[string]bool)

		for _, sn := range scoredNodes {
			name := sn.Node.Name
			if name == "" || seenEntityNames[name] {
				continue
			}
			seenEntityNames[name] = true

			// Find neighbors for this node from allEdges
			var neighbors []graph.Neighbor
			seenNeighbors := make(map[string]bool)

			for _, edge := range allEdges {
				var targetName string
				var relType string

				if edge.Head == name {
					targetName = edge.Tail
					relType = edge.Type
				} else if edge.Tail == name {
					targetName = edge.Head
					relType = edge.Type
				} else {
					continue
				}

				if targetProps, ok := nodePropsMap[targetName]; ok && !seenNeighbors[targetName] {
					seenNeighbors[targetName] = true
					neighbors = append(neighbors, graph.Neighbor{
						RelationshipType: relType,
						Node:             targetProps,
					})
				}
			}

			cRes = append(cRes, graph.EntitySearchResult{
				Entity:    sn.Node.Props,
				Neighbors: neighbors,
			})
		}

		resultChan <- graphPathResult{cRes, nil, "smart_expansion", nil}
	}()

	// Wait for all 3 paths
	for i := 0; i < 3; i++ {
		res := <-resultChan
		if res.err != nil {
			logger.Warn(fmt.Sprintf("Graph path %s failed", res.source), map[string]interface{}{"error": res.err.Error()})
		} else if len(res.cResults) > 0 || len(res.entities) > 0 {
			addResultsLocked(res.cResults, res.source, res.entities)
		}
	}

	// If MultiHop failed or found nothing, ensure original extracted entities are still in allGraphEntities
	mu.Lock()
	for _, e := range entities {
		if e != "" && !uniqueEntityMap[e] {
			uniqueEntityMap[e] = true
			allGraphEntities = append(allGraphEntities, e)
		}
	}
	mu.Unlock()

	contextResults := finalContextResults

	// Start step 3
	execution.Steps = append(execution.Steps, dto.GraphExecutionStep{
		Step:        3,
		Action:      "query_graph",
		Description: "在知识图谱中查找实体",
		Result:      fmt.Sprintf("找到 %d 个匹配实体", len(allGraphEntities)),
	})

	// Collect triples and matched entities from graph results
	triples := make([]dto.TripleResponse, 0)
	matchedEntities := make([]string, 0)

	for _, res := range contextResults {
		// COMPREHENSIVE FALLBACK: Check all common identifier properties
		subject := ""
		for _, prop := range []string{"name", "title", "id", "fileName", "canonical_name", "content"} {
			if n, ok := res.Entity[prop].(string); ok && n != "" {
				subject = n
				break
			}
		}

		if subject != "" {
			matchedEntities = append(matchedEntities, subject)
		}

		for _, neighbor := range res.Neighbors {
			object := ""
			for _, prop := range []string{"name", "title", "id", "fileName", "canonical_name", "content"} {
				if n, ok := neighbor.Node[prop].(string); ok && n != "" {
					object = n
					break
				}
			}

			if object != "" && subject != "" {
				triples = append(triples, dto.TripleResponse{
					Subject:   subject,
					Predicate: neighbor.RelationshipType,
					Object:    object,
				})
			} else {
				logger.DebugContext(graphLogCtx, "skipped graph triple with incomplete fields",
					zap.Bool("has_subject", subject != ""),
					zap.Bool("has_object", object != ""),
					zap.String("predicate", neighbor.RelationshipType),
				)
			}
		}
	}

	logger.DebugContext(graphLogCtx, "graph triples assembled",
		zap.Int("triples_count", len(triples)),
		zap.Int("matched_entities_count", len(matchedEntities)),
	)

	execution.Steps = append(execution.Steps, dto.GraphExecutionStep{
		Step:        4,
		Action:      "find_relations",
		Description: "获取实体关联关系",
		Result:      fmt.Sprintf("找到 %d 条关系三元组", len(triples)),
	})
	execution.Triples = triples

	// Update HybridFusion to use the full expanded context (including neighbors)
	expandedCtx := extractGraphContext(finalContextResults)
	// Extract External Scores from Smart Expansion results
	externalScores := make(map[string]float64)
	for _, res := range finalContextResults {
		if name, ok := res.Entity["name"].(string); ok {
			if score, ok := res.Entity["_smart_score"].(float64); ok {
				externalScores[name] = score
			}
		}
	}

	fusedResults := graphflow_retrieval.HybridFusion(vectorResults, expandedCtx, options.GraphAlpha, entities, externalScores)

	var results []retrieval.SearchResult

	if len(fusedResults) > 0 {
		// Collect entity IDs/Names to query
		var targetIDs []uuid.UUID
		entityScoreMap := make(map[string]float64)

		for _, res := range fusedResults {
			// CRITICAL: We now use EntityID (UUID) for looking up segments in Postgres.
			// This matches kb_entity_mentions.entity_id and is much more robust than names.
			if res.EntityID != "" {
				if id, err := uuid.Parse(res.EntityID); err == nil {
					// DEDUPLICATE: Prevent redundant SQL lookups
					found := false
					for _, existingID := range targetIDs {
						if existingID == id {
							found = true
							break
						}
					}
					if !found {
						targetIDs = append(targetIDs, id)
					}
					// Store score by UUID string for later lookup
					entityScoreMap[res.EntityID] = res.FinalScore
				}
			}
		}

		// Batch get segments for these entities using the new ID-based method
		params := map[string]interface{}{}
		segmentMap, err := s.graphFlowService.FindSegmentsByEntityIDs(ctx, dataset.ID, targetIDs, params)
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				logger.Error("[RETRIEVAL_CRITICAL] Graph Search Timeout during Entity Resolution", err)
			} else {
				logger.Error("Failed to find segments by entity IDs", err)
			}
		}

		var validSegmentIDs []string

		// 1. Filter out short segments
		for segmentID := range segmentMap {
			validSegmentIDs = append(validSegmentIDs, segmentID)
		}

		// Wait for Weaviate Async Search to finish
		<-weaviateDone

		uniqueSegmentMap := make(map[string]retrieval.SearchResult)

		for _, segmentID := range validSegmentIDs {
			details := segmentMap[segmentID]
			maxScore := 0.0

			// Hard Constraint Check:
			// Calculate how many ORIGINAL query 'entities' are covered by this segment's matched graph entities.
			coveredQueryEntities := make(map[string]bool)
			if len(entities) > 1 {
				for _, me := range details.MatchedEntities {
					meLower := strings.ToLower(me)
					for _, qe := range entities {
						qeLower := strings.ToLower(qe)
						// Check if graph entity IS or CONTAINS query entity (fuzzy match)
						if strings.Contains(meLower, qeLower) || strings.Contains(qeLower, meLower) {
							coveredQueryEntities[qe] = true
						}
					}
				}
			}

			// Use EntityID (UUID) for score lookup as it is more unique/stable than Name
			for _, matchedID := range details.MatchedEntityIDs {
				if sc, ok := entityScoreMap[matchedID]; ok {
					if sc > maxScore {
						maxScore = sc
					}
				}
			}

			// Fallback to name if score is still 0 (e.g. for legacy or unlinked entities)
			if maxScore == 0 {
				for _, matchedName := range details.MatchedEntities {
					if sc, ok := entityScoreMap[matchedName]; ok {
						if sc > maxScore {
							maxScore = sc
						}
					}
				}
			}

			// Apply Penalty for partial coverage
			penaltyFactor := 1.0
			if len(entities) > 1 {
				coverageCount := len(coveredQueryEntities)
				// Use Soft Decay for partial matches instead of strict halving.
				// e.g. covering 2 out of 3 gives 0.8 + 0.2*(2/3) ~ 0.933 penalty
				if coverageCount < len(entities) {
					coverageRatio := float64(coverageCount) / float64(len(entities))
					penaltyFactor = options.CoveragePenaltyBase +
						(1.0-options.CoveragePenaltyBase)*coverageRatio
				}
			}

			graphScore := maxScore * penaltyFactor
			logger.Debug("Coverage penalty", map[string]interface{}{
				"max_score":      maxScore,
				"graphScore":     graphScore,
				"penalty_factor": penaltyFactor,
				"details":        details,
			})
			mentionCount := len(details.MatchedEntities)
			if mentionCount > 1 {
				// Use Logarithmic Saturation to prevent "entity-stuffing" from unfairly dominating semantic scores.
				// This ensures segments with specific, unique matches outrank those with many common labels.
				graphScore = graphScore * (1.0 + options.MentionBoost*math.Log2(float64(mentionCount)))
			}

			finalSegmentScore := graphScore

			// Adaptive Re-score: blend with semantic score only if both exist
			semanticScore := 0.0
			weaviateMu.Lock()
			lookupID := details.IndexNodeID
			if lookupID == "" {
				lookupID = segmentID // Fallback if IndexNodeID is somehow empty
			}
			if sScore, ok := weaviateScoreMap[lookupID]; ok && sScore > 0 {
				semanticScore = sScore
				finalSegmentScore = (semanticScore * options.SemanticWeight) + (graphScore * options.GraphWeight)
			}
			weaviateMu.Unlock()

			uniqueSegmentMap[segmentID] = retrieval.SearchResult{
				ID:      segmentID, // We need the ID
				Content: details.Content,
				Score:   finalSegmentScore,
				Metadata: map[string]interface{}{
					"source":           "graph_knowledge",
					"matched_entities": details.MatchedEntities,
					"penalty_applied":  penaltyFactor < 1.0,
					"semantic_score":   semanticScore,
					"graph_score":      graphScore,
					"mention_count":    mentionCount,
					"document_id":      details.DocumentID,
					"position":         details.Position,
				},
			}

			logger.Debug("Graph Knowledge Coverage penalty", map[string]interface{}{
				"source":           "graph_knowledge",
				"matched_entities": details.MatchedEntities,
				"penalty_applied":  penaltyFactor < 1.0,
				"semantic_score":   semanticScore,
				"graph_score":      graphScore,
				"mention_count":    mentionCount,
				"document_id":      details.DocumentID,
				"position":         details.Position,
			})
		}

		// Convert map to slice
		for _, v := range uniqueSegmentMap {
			results = append(results, v)
		}
	}

	// 5. Build final execution summary
	execution.Steps = append(execution.Steps, dto.GraphExecutionStep{
		Step:        5,
		Action:      "find_chunks",
		Description: "通过实体提及找到相关切片",
		Result:      fmt.Sprintf("找到 %d 个相关切片", len(results)),
	})

	// Generate summary
	if len(results) > 0 {
		execution.Summary = fmt.Sprintf("从查询中识别到实体「%s」，通过知识图谱关联找到了 %d 个相关的文档切片，以及 %d 条关系三元组。",
			strings.Join(entities, "、"), len(results), len(triples))
	} else if len(matchedEntities) > 0 {
		execution.Summary = fmt.Sprintf("从查询中识别到实体「%s」，在知识图谱中找到了 %d 条相关关系，但未找到关联的文档切片。",
			strings.Join(matchedEntities, "、"), len(triples))
	} else {
		execution.Summary = "未在知识图谱中找到与查询相关的实体信息。"
	}

	// Deduplicate final entities list
	finalEntitiesSet := make(map[string]bool)
	var finalEntities []string
	for _, e := range append(entities, matchedEntities...) {
		if e != "" && !finalEntitiesSet[e] {
			finalEntitiesSet[e] = true
			finalEntities = append(finalEntities, e)
		}
	}

	execution.Entities = finalEntities
	execution.DebugInfo = map[string]interface{}{
		"seeds":          entities,
		"triples_count":  len(triples),
		"entities_count": len(finalEntities),
		"chunks_count":   len(results),
		"hop_depth":      3,
	}

	// Sort results by score descending, with ID fallback to ensure absolute stable ordering across identical scores
	sort.SliceStable(results, func(i, j int) bool {
		// handle floating point precision
		diff := math.Abs(results[i].Score - results[j].Score)
		if diff < 1e-6 {
			return results[i].ID > results[j].ID // Fallback to ID for true stability
		}
		return results[i].Score > results[j].Score
	})

	logger.Debug("Sort results by score descending to ensure stable ordering", map[string]interface{}{
		"source": results,
	})

	// Clamp scores to a maximum of 1.0 to preserve true absolute differences for final display
	for i := range results {
		if results[i].Score > 1.0 {
			results[i].Score = 1.0
		}
	}

	logger.Info("Graph search finished", map[string]interface{}{
		"extracted_entities": len(entities),
		"matched_entities":   len(matchedEntities),
		"triples_found":      len(triples),
		"chunks_found":       len(results),
	})

	return results, execution, nil
}

// extractGraphContext converts EntitySearchResult slice to a full GraphContext
// Including both the seed entities and their multi-hop neighbors
func extractGraphContext(results []graph.EntitySearchResult) *graphflow_retrieval.GraphContext {
	seenEntities := make(map[string]bool)
	seenRelations := make(map[string]bool)
	ctx := &graphflow_retrieval.GraphContext{
		Entities:      make([]graphflow_retrieval.GraphEntity, 0),
		Relationships: make([]graphflow_retrieval.GraphRelation, 0),
	}

	for _, res := range results {
		// 1. Add root entity
		rootName, _ := res.Entity["name"].(string)
		if rootName != "" && !seenEntities[rootName] {
			seenEntities[rootName] = true
			ctx.Entities = append(ctx.Entities, convertToGraphEntity(res.Entity))
		}

		// 2. Add neighbors and relationships
		for _, nb := range res.Neighbors {
			neighborName, _ := nb.Node["name"].(string)
			if neighborName == "" {
				continue
			}

			// Add neighbor entity if new
			if !seenEntities[neighborName] {
				seenEntities[neighborName] = true
				ctx.Entities = append(ctx.Entities, convertToGraphEntity(nb.Node))
			}

			// Add relationship if new
			if rootName != "" {
				relKey := fmt.Sprintf("%s-%s-%s", rootName, nb.RelationshipType, neighborName)
				if !seenRelations[relKey] {
					seenRelations[relKey] = true
					ctx.Relationships = append(ctx.Relationships, graphflow_retrieval.GraphRelation{
						HeadEntity:   rootName,
						TailEntity:   neighborName,
						RelationType: nb.RelationshipType,
						Weight:       1,
					})
				}
			}
		}
	}
	return ctx
}

// convertToGraphEntity converts a map of Neo4j properties to a GraphEntity
func convertToGraphEntity(props map[string]interface{}) graphflow_retrieval.GraphEntity {
	name, _ := props["name"].(string)
	id, _ := props["id"].(string)
	eType, _ := props["type"].(string)
	if eType == "" {
		eType = "Entity"
	}

	canonical, _ := props["canonical_name"].(string)

	sourceCount := 1
	// Neo4j driver returns int64 for integers
	if sc, ok := props["source_count"].(int64); ok {
		sourceCount = int(sc)
	} else if sc, ok := props["source_count"].(float64); ok {
		sourceCount = int(sc)
	}

	return graphflow_retrieval.GraphEntity{
		ID:            id,
		Name:          name,
		CanonicalName: canonical,
		Type:          eType,
		SourceCount:   sourceCount,
		Properties:    props,
	}
}

// calculateCosineSimilarity calculates the cosine similarity between two vectors
func calculateCosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}
	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0.0
	}
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}
