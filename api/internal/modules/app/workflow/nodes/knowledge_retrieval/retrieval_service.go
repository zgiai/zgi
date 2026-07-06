package knowledgeretrieval

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	datasetmodel "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	drepo "github.com/zgiai/zgi/api/internal/modules/dataset/repository"
	dretrieval "github.com/zgiai/zgi/api/internal/modules/dataset/retrieval"
	dservice "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	"github.com/zgiai/zgi/api/internal/observability"
	"github.com/zgiai/zgi/api/pkg/embedding"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/tokenization"
	"github.com/zgiai/zgi/api/pkg/vectordb"
	"gorm.io/gorm"
)

// RetrievalService defines interfaces for single and multiple retrieval flows.
// It returns a list of DocumentHit which will be formatted by the caller.
type RetrievalService interface {
	// SingleRetrieve performs a single-dataset planning and retrieval using an LLM planning strategy.
	SingleRetrieve(ctx context.Context, params SingleRetrieveParams) ([]DocumentHit, error)
	// MultipleRetrieve performs multi-method retrieval with optional reranking and weighting.
	MultipleRetrieve(ctx context.Context, params MultipleRetrieveParams) ([]DocumentHit, error)
}

type defaultRetrievalService struct {
	db               *gorm.DB
	llmClient        llmclient.LLMClient
	embInvoker       embeddingInvoker
	rerankInvoker    rerankInvoker
	graphFlowService *graphflow.Service
}

// NewRetrievalService creates a default retrieval service implementation.
// The llmClient should be obtained from the DI container (ServiceContainer.GetLLMClient()).
func NewRetrievalService(db *gorm.DB, llmClient llmclient.LLMClient) RetrievalService {
	return &defaultRetrievalService{
		db:        db,
		llmClient: llmClient,
	}
}

// NewRetrievalServiceWithEmbedding creates a retrieval service with a custom embedding invoker.
// The llmClient should be obtained from the DI container (ServiceContainer.GetLLMClient()).
func NewRetrievalServiceWithEmbedding(db *gorm.DB, llmClient llmclient.LLMClient, embInvoker embeddingInvoker, graphFlowService *graphflow.Service) RetrievalService {
	return &defaultRetrievalService{
		db:               db,
		llmClient:        llmClient,
		embInvoker:       embInvoker,
		graphFlowService: graphFlowService,
	}
}

func buildWorkflowRetrieveOptions(base *dservice.RetrievalOptions) *dservice.RetrievalOptions {
	if base == nil {
		return &dservice.RetrievalOptions{}
	}

	opts := *base
	if opts.SearchMethod == "graph_search" {
		opts.RetrievalMode = "hybrid"
		opts.SearchMethod = ""
	}

	return &opts
}

func defaultModelServiceFromGraph(graphService *graphflow.Service) llmdefaultservice.DefaultModelService {
	if graphService == nil {
		return nil
	}
	return graphService.DefaultModelSvc
}

func (s *defaultRetrievalService) buildDatasetRetrievalService(
	embeddingFactory dservice.EmbeddingServiceFactory,
	docRepo drepo.DocumentRepository,
	vectorClient *vectordb.WeaviateClient,
	keywordRetrieval *dretrieval.KeywordRetrievalService,
	fullTextRetrieval *dretrieval.FullTextRetrievalService,
) (*dservice.RetrievalService, error) {
	if embeddingFactory == nil {
		return nil, fmt.Errorf("gateway embedding factory is required")
	}

	vectorRetrieval := dretrieval.NewVectorRetrievalService(nil, vectorClient, "")
	hybridRetrieval := dretrieval.NewHybridRetrievalService(vectorRetrieval, keywordRetrieval, fullTextRetrieval)
	rerankService := dretrieval.NewRerankService()
	retrievalService := dservice.NewRetrievalServiceWithEmbeddingFactory(
		docRepo,
		vectorRetrieval,
		keywordRetrieval,
		fullTextRetrieval,
		hybridRetrieval,
		rerankService,
		defaultModelServiceFromGraph(s.graphFlowService),
		vectorClient,
		s.graphFlowService,
		embeddingFactory,
	)
	retrievalService.SetLLMClient(s.llmClient)

	return retrievalService, nil
}

func preferWorkflowRecords(records []dto.HitTestingRecordResponse) []dto.HitTestingRecordResponse {
	if len(records) == 0 {
		return records
	}

	graphRecords := make([]dto.HitTestingRecordResponse, 0, len(records))
	vectorRecords := make([]dto.HitTestingRecordResponse, 0, len(records))
	for _, record := range records {
		if isWorkflowGraphRecord(record) {
			graphRecords = append(graphRecords, record)
			continue
		}
		vectorRecords = append(vectorRecords, record)
	}

	if len(graphRecords) > 0 {
		return graphRecords
	}

	return vectorRecords
}

func isWorkflowGraphRecord(record dto.HitTestingRecordResponse) bool {
	if record.MatchType == dto.MatchTypeGraphKnowledge {
		return true
	}

	return record.RetrievalSource != nil && record.RetrievalSource.Method == dto.MatchTypeGraphKnowledge
}

func (s *defaultRetrievalService) SingleRetrieve(ctx context.Context, params SingleRetrieveParams) ([]DocumentHit, error) {

	tools := make([]DatasetTool, 0, len(params.Tools))
	if len(params.AvailableDatasets) > 0 {
		for _, dataset := range params.AvailableDatasets {
			if dataset == nil {
				continue
			}

			desc := *dataset.Description
			if dataset.Description == nil || strings.TrimSpace(*dataset.Description) == "" {
				desc = "useful for when you want to answer queries about the " + dataset.Name
			}

			desc = strings.ReplaceAll(strings.ReplaceAll(desc, "\n", ""), "\r", "")

			tools = append(tools, DatasetTool{
				Name:        dataset.ID,
				Description: desc,
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
					"required":   []string{},
				},
			})
		}
		params.Tools = tools
	}

	var datasetID string
	modelConfig := params.ModelConfig

	if params.Planning == PlanningStrategyREACTRouter {
		var err error
		reactMultiDatasetRouter := NewReactMultiDatasetRouter()
		datasetID, err = reactMultiDatasetRouter.Invoke(ctx, params.Query, params.Tools, modelConfig, params.UserID, params.AppID)
		if err != nil {
			return nil, fmt.Errorf("failed to invoke react multi dataset retrieval: %w", err)
		}
	}

	if params.Planning == PlanningStrategyRouter {
		var err error
		functionCallMultiDatasetRouter := NewFunctionCallMultiDatasetRouter()
		datasetID, err = functionCallMultiDatasetRouter.Invoke(ctx, params.Query, params.Tools, modelConfig, params.UserID, params.AppID)
		if err != nil {
			// Fallback to REACT router if tool-calls are not supported
			reactMultiDatasetRouter := NewReactMultiDatasetRouter()
			datasetID, err = reactMultiDatasetRouter.Invoke(ctx, params.Query, params.Tools, modelConfig, params.UserID, params.AppID)
			if err != nil {
				return nil, err
			}
		}
	}

	if strings.TrimSpace(datasetID) == "" {
		return nil, fmt.Errorf("dataset id empty")
	}

	datasetRepo := drepo.NewDatasetRepository(s.db)
	dataset, err := datasetRepo.GetByID(ctx, datasetID)
	if err != nil {
		return nil, err
	}

	if dataset == nil {
		return nil, fmt.Errorf("dataset not found")
	}

	cond, ok := params.MetadataCond.(*MetadataCondition)
	if !ok {
		return nil, fmt.Errorf("invalid condition")
	}

	var allDocs []DocumentHit

	embeddingFactory := s.makeEmbeddingFactory(params.UserID)
	// Treat typed-nil metadata condition as no filter
	hasMetadataFilter := false
	if params.MetadataCond != nil {
		if c, ok := params.MetadataCond.(*MetadataCondition); !ok || c != nil {
			hasMetadataFilter = true
		}
	}

	if dataset.Provider == "external" {
		// Handle external dataset
		externalDocs, err := s.fetchExternalKnowledgeRetrieval(ctx, dataset, params.Query, cond)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch external knowledge: %w", err)
		}
		// Convert external documents to DocumentHit format
		for _, extDoc := range externalDocs {
			doc := DocumentHit{
				Provider:    "external",
				Score:       extDoc.Score,
				PageContent: extDoc.Content,
				Vector:      nil, // External documents typically don't return vector data
				Metadata:    make(map[string]any),
			}

			// Set metadata
			if extDoc.Metadata != nil {
				doc.Metadata = extDoc.Metadata
			}
			doc.Metadata["score"] = extDoc.Score
			doc.Metadata["title"] = extDoc.Title
			doc.Metadata["dataset_id"] = datasetID
			doc.Metadata["dataset_name"] = dataset.Name

			allDocs = append(allDocs, doc)
		}
	} else {
		if hasMetadataFilter && len(params.MetadataDocIDs) == 0 {
			return nil, nil
		}

		var documentIdsFilter []string
		if hasMetadataFilter {
			if len(params.MetadataDocIDs) == 0 {
				return nil, nil
			}
			documentIds, ok := params.MetadataDocIDs[dataset.ID]
			if !ok {
				return nil, nil
			}
			if len(documentIds) == 0 {
				return nil, nil
			}
			documentIdsFilter = documentIds
		}

		retrievalModelConfig := dataset.RetrievalConfig
		if len(retrievalModelConfig) == 0 {
			retrievalModelConfig = map[string]any{
				"search_method":    "hybrid_search",
				"reranking_enable": false,
				"reranking_model": map[string]string{
					"reranking_provider_name": "",
					"reranking_model_name":    "",
				},
				"top_k":                   4,
				"score_threshold_enabled": false,
			}
		}

		topK := retrievalModelConfig["top_k"]
		retrievalMethod := retrievalModelConfig["search_method"]

		rerankingEnabled := retrievalModelConfig["reranking_enable"].(bool)
		rerankingModel := retrievalModelConfig["reranking_model"]
		if !rerankingEnabled {
			rerankingModel = nil
		}
		rm, ok := rerankingModel.(map[string]any)
		if !ok {
			rm = make(map[string]any)
		}

		weights := retrievalModelConfig["weights"]
		if weights == nil {
			weights = map[string]any{}
		}
		w, ok := weights.(map[string]any)
		if !ok {
			w = map[string]any{}
		}

		// get score threshold
		var scoreThreshold = 0.0
		if retrievalModelConfig["score_threshold_enabled"].(bool) {
			scoreThreshold = retrievalModelConfig["score_threshold"].(float64)
		}

		docRepo := drepo.NewDocumentRepository(s.db)

		vectorClient := vectordb.NewWeaviateClient(&config.GlobalConfig.VectorStore)
		tokenizer := tokenization.NewTokenizationService()
		keywordRetrieval := dretrieval.NewKeywordRetrievalService(tokenizer)
		fullTextRetrieval := dretrieval.NewFullTextRetrievalService(tokenizer, 1.5, 0.75, vectorClient)
		retrievalService, err := s.buildDatasetRetrievalService(
			embeddingFactory,
			docRepo,
			vectorClient,
			keywordRetrieval,
			fullTextRetrieval,
		)
		if err != nil {
			return nil, err
		}

		opts := &dservice.RetrievalOptions{
			SearchMethod:          retrievalMethod.(string),
			TopK:                  topK.(int),
			ScoreThreshold:        scoreThreshold,
			ScoreThresholdEnabled: retrievalModelConfig["score_threshold_enabled"].(bool),
			RerankingEnable:       false, // rerank handled via gateway/fallback at node level
			RerankingModel:        nil,
			Weights:               w,
			DocumentIDsFilter:     documentIdsFilter,
		}

		workflowOpts := buildWorkflowRetrieveOptions(opts)

		logger.Info("[RetrievalService] Starting retrieval", map[string]interface{}{
			"dataset_id":             dataset.ID,
			"query":                  params.Query,
			"search_method":          opts.SearchMethod,
			"workflow_search_method": workflowOpts.SearchMethod,
			"retrieval_mode":         workflowOpts.RetrievalMode,
			"top_k":                  workflowOpts.TopK,
		})

		results, _, err := retrievalService.Retrieve(ctx, dataset, params.Query, workflowOpts)
		if err != nil {
			logger.Error("[RetrievalService] Retrieval failed", err)
			return nil, err
		}
		results = preferWorkflowRecords(results)

		logger.Info("[RetrievalService] Retrieval completed", map[string]interface{}{
			"results_count": len(results),
		})

		// Convert retrieval results to DocumentHit structure
		for _, result := range results {
			doc := DocumentHit{
				Provider:    "zgi",
				Score:       result.Score,
				PageContent: result.Segment.Content,
				Vector:      nil, // TODO: Can fetch from vector database if needed
				Metadata: map[string]any{
					"segment_id":  result.Segment.ID,
					"document_id": result.Segment.DocumentID,
					"position":    result.Segment.Position,
					"word_count":  result.Segment.WordCount,
					"tokens":      result.Segment.Tokens,
					"keywords":    result.Segment.Keywords,
					"hit_count":   result.Segment.HitCount,
					"match_type":  result.MatchType,
					"created_at":  result.Segment.CreatedAt,
					"score":       result.Score,
				},
			}

			// Add document information to metadata
			if result.Segment.Document.ID != "" {
				doc.Metadata["doc_name"] = result.Segment.Document.Name
				doc.Metadata["doc_type"] = result.Segment.Document.DocType
				doc.Metadata["data_source_type"] = result.Segment.Document.DataSourceType
				doc.Metadata["doc_metadata"] = result.Segment.Document.DocMetadata
			}

			if len(result.ChildChunks) > 0 {
				children := make([]ChildDocument, len(result.ChildChunks))
				for i, chunk := range result.ChildChunks {
					children[i] = ChildDocument{
						PageContent: chunk.Content,
						Vector:      nil, // TODO: Can fetch from vector database if needed
						Metadata: map[string]any{
							"id":         chunk.ID,
							"position":   chunk.Position,
							"word_count": chunk.WordCount,
							"type":       chunk.Type,
							"score":      chunk.Score,
							"segment_id": chunk.SegmentID,
						},
					}
				}
				doc.Children = children

				// Maintain backward compatibility, also keep child_chunks in metadata
				childChunkData := make([]map[string]any, len(result.ChildChunks))
				for i, chunk := range result.ChildChunks {
					childChunkData[i] = map[string]any{
						"id":         chunk.ID,
						"content":    chunk.Content,
						"position":   chunk.Position,
						"word_count": chunk.WordCount,
						"type":       chunk.Type,
						"score":      chunk.Score,
					}
				}
				doc.Metadata["child_chunks"] = childChunkData
			}

			allDocs = append(allDocs, doc)
		}

		// Apply rerank via gateway (fallback to legacy) for single retrieval if enabled
		if rerankingEnabled && len(allDocs) > 0 {
			allDocs = s.applyModelReranking(ctx, allDocs, RerankingParams{
				TenantID:           params.TenantID,
				OrganizationID:     params.OrganizationID,
				BillingSubjectType: params.BillingSubjectType,
				UserID:             params.UserID,
				AppID:              params.AppID,
				Query:              params.Query,
				RerankingModel:     rm,
				ScoreThreshold:     scoreThreshold,
				TopK:               topK.(int),
			})
		}

	}

	// Sort by score desc
	sort.SliceStable(allDocs, func(i, j int) bool { return allDocs[i].Score > allDocs[j].Score })
	return allDocs, nil
}

// MultipleRetrieve performs multi-method retrieval with optional reranking and weighting.
func (s *defaultRetrievalService) MultipleRetrieve(ctx context.Context, params MultipleRetrieveParams) ([]DocumentHit, error) {
	var allDocs []DocumentHit
	embeddingFactory := s.makeEmbeddingFactory(params.UserID)

	// Handle hybrid retrieval from multiple datasets
	for _, dataset := range params.AvailableDatasets {
		if dataset == nil {
			continue
		}

		// Skip multiple retrieval for external datasets (not currently supported)
		if dataset.Provider == "external" {
			continue
		}

		// Check metadata filtering conditions
		var documentIdsFilter []string
		if len(params.MetadataDocIDs) > 0 {
			documentIds, ok := params.MetadataDocIDs[dataset.ID]
			if !ok || len(documentIds) == 0 {
				continue
			}
			documentIdsFilter = documentIds
		}

		// Get dataset retrieval configuration
		retrievalModelConfig := dataset.RetrievalConfig
		if len(retrievalModelConfig) == 0 {
			retrievalModelConfig = map[string]any{
				"search_method":    "hybrid_search",
				"reranking_enable": false,
				"reranking_model": map[string]string{
					"reranking_provider_name": "",
					"reranking_model_name":    "",
				},
				"top_k":                   params.TopK,
				"score_threshold_enabled": params.ScoreThreshold > 0,
				"score_threshold":         params.ScoreThreshold,
			}
		}

		// Configure retrieval method
		retrievalMethod := retrievalModelConfig["search_method"]

		// Configure weights
		weights := params.Weights
		if weights == nil {
			w, ok := retrievalModelConfig["weights"].(map[string]any)
			if ok {
				weights = w
			}
		}
		if weights == nil {
			weights = map[string]any{}
		}

		// Execute retrieval from single dataset
		docs, err := s.retrieveFromSingleDataset(ctx, dataset, params.Query, &RetrievalOptions{
			SearchMethod:          retrievalMethod.(string),
			TopK:                  params.TopK,
			ScoreThreshold:        params.ScoreThreshold,
			ScoreThresholdEnabled: params.ScoreThreshold > 0,
			RerankingEnable:       false, // rerank is handled globally via gateway/fallback
			RerankingModel:        nil,
			Weights:               weights,
			DocumentIDsFilter:     documentIdsFilter,
		}, embeddingFactory)
		if err != nil {
			// Log error but continue processing other datasets
			continue
		}

		allDocs = append(allDocs, docs...)
	}

	// Execute global reranking and weight processing
	if len(allDocs) > 0 {
		allDocs = s.applyGlobalReranking(ctx, allDocs, params)
	}

	// Sort by score in descending order
	sort.SliceStable(allDocs, func(i, j int) bool {
		return allDocs[i].Score > allDocs[j].Score
	})

	// Apply top_k limit
	if params.TopK > 0 && len(allDocs) > params.TopK {
		allDocs = allDocs[:params.TopK]
	}

	return allDocs, nil
}

// retrieveFromSingleDataset executes retrieval from a single dataset
func (s *defaultRetrievalService) retrieveFromSingleDataset(
	ctx context.Context,
	dataset *datasetmodel.Dataset,
	query string,
	opts *RetrievalOptions,
	embeddingFactory dservice.EmbeddingServiceFactory,
) ([]DocumentHit, error) {
	// Reuse existing retrieval logic
	docRepo := drepo.NewDocumentRepository(s.db)

	vectorClient := vectordb.NewWeaviateClient(&config.GlobalConfig.VectorStore)
	tokenizer := tokenization.NewTokenizationService()
	keywordRetrieval := dretrieval.NewKeywordRetrievalService(tokenizer)
	fullTextRetrieval := dretrieval.NewFullTextRetrievalService(tokenizer, 1.5, 0.75, vectorClient)
	retrievalService, err := s.buildDatasetRetrievalService(
		embeddingFactory,
		docRepo,
		vectorClient,
		keywordRetrieval,
		fullTextRetrieval,
	)
	if err != nil {
		return nil, err
	}

	workflowOpts := buildWorkflowRetrieveOptions(&dservice.RetrievalOptions{
		SearchMethod:          opts.SearchMethod,
		TopK:                  opts.TopK,
		ScoreThreshold:        opts.ScoreThreshold,
		ScoreThresholdEnabled: opts.ScoreThresholdEnabled,
		RerankingEnable:       opts.RerankingEnable,
		RerankingModel:        opts.RerankingModel,
		Weights:               opts.Weights,
		DocumentIDsFilter:     opts.DocumentIDsFilter,
	})
	results, _, err := retrievalService.Retrieve(ctx, dataset, query, workflowOpts)
	if err != nil {
		return nil, err
	}
	results = preferWorkflowRecords(results)

	var docs []DocumentHit
	// Convert retrieval results to DocumentHit structure
	for _, result := range results {
		doc := DocumentHit{
			Provider:    "zgi",
			Score:       result.Score,
			PageContent: result.Segment.Content,
			Vector:      nil,
			Metadata: map[string]any{
				"dataset_id":              dataset.ID,
				"dataset_name":            dataset.Name,
				"document_id":             result.Segment.DocumentID,
				"document_name":           result.Segment.Document.Name,
				"data_source_type":        result.Segment.Document.DataSourceType,
				"segment_id":              result.Segment.ID,
				"segment_position":        result.Segment.Position,
				"segment_hit_count":       result.Segment.HitCount,
				"segment_word_count":      result.Segment.WordCount,
				"segment_index_node_hash": result.Segment.IndexNodeHash,
				"score":                   result.Score,
				"title":                   result.Segment.Document.Name,
			},
		}

		// Add document metadata
		if result.Segment.Document.DocMetadata != nil {
			doc.Metadata["doc_metadata"] = result.Segment.Document.DocMetadata
		}

		// Process child chunks
		if len(result.ChildChunks) > 0 {
			children := make([]ChildDocument, len(result.ChildChunks))
			childChunkData := make([]map[string]any, len(result.ChildChunks))
			for i, chunk := range result.ChildChunks {
				children[i] = ChildDocument{
					PageContent: chunk.Content,
					Vector:      nil,
					Metadata: map[string]any{
						"id":         chunk.ID,
						"position":   chunk.Position,
						"word_count": chunk.WordCount,
						"type":       chunk.Type,
						"score":      chunk.Score,
						"segment_id": chunk.SegmentID,
					},
				}
				childChunkData[i] = map[string]any{
					"id":       chunk.ID,
					"content":  chunk.Content,
					"position": chunk.Position,
					"score":    chunk.Score,
				}
			}
			doc.Children = children
			doc.Metadata["child_chunks"] = childChunkData
		}

		docs = append(docs, doc)
	}

	return docs, nil
}

func (s *defaultRetrievalService) makeEmbeddingFactory(accountID string) dservice.EmbeddingServiceFactory {
	if s.embInvoker == nil {
		logger.Warn("[RetrievalService] embInvoker is nil, embedding factory will be nil", nil)
		return nil
	}

	logger.Info("[RetrievalService] Creating embedding factory with gateway embedding service", nil)
	return func(dataset *datasetmodel.Dataset) embedding.EmbeddingService {
		if dataset == nil {
			return nil
		}

		modelName := "text-embedding-3-large"
		if dataset.EmbeddingModel != nil && *dataset.EmbeddingModel != "" {
			modelName = *dataset.EmbeddingModel
		}

		return newGatewayEmbeddingService(s.embInvoker, accountID, dataset.ID, "dataset", modelName)
	}
}

// RerankingParams carries the context needed to run reranking.
type RerankingParams struct {
	TenantID           string
	OrganizationID     string
	BillingSubjectType string
	UserID             string
	AppID              string
	Query              string
	RerankingModel     map[string]any
	ScoreThreshold     float64
	TopK               int
}

// applyGlobalReranking applies global reranking logic
func (s *defaultRetrievalService) applyGlobalReranking(
	ctx context.Context,
	docs []DocumentHit,
	params MultipleRetrieveParams,
) []DocumentHit {
	// Process based on reranking mode
	switch params.RerankingMode {
	case RerankingModeWeightedScore:
		return s.applyWeightedScoring(docs, params.Weights)
	case RerankingModeModel:
		return s.applyModelReranking(ctx, docs, RerankingParams{
			TenantID:           params.TenantID,
			OrganizationID:     params.OrganizationID,
			BillingSubjectType: params.BillingSubjectType,
			UserID:             params.UserID,
			AppID:              params.AppID,
			Query:              params.Query,
			RerankingModel:     params.RerankingModel,
			ScoreThreshold:     params.ScoreThreshold,
			TopK:               params.TopK,
		})
	default:
		return docs
	}
}

// applyWeightedScoring applies weighted scoring
func (s *defaultRetrievalService) applyWeightedScoring(docs []DocumentHit, weights map[string]any) []DocumentHit {
	if weights == nil {
		return docs
	}

	// Get weight configuration
	vectorWeight := 0.7  // Default weight
	keywordWeight := 0.3 // Default weight

	if vectorSetting, ok := weights["vector_setting"].(map[string]any); ok {
		if vw, ok := vectorSetting["vector_weight"].(float64); ok {
			vectorWeight = vw
		}
	}

	if keywordSetting, ok := weights["keyword_setting"].(map[string]any); ok {
		if kw, ok := keywordSetting["keyword_weight"].(float64); ok {
			keywordWeight = kw
		}
	}

	// Recalculate weighted scores
	for i := range docs {
		// Need to calculate based on actual vector and keyword scores
		// Currently using original score as base, applying weight adjustment
		docs[i].Score = docs[i].Score * (vectorWeight + keywordWeight)
	}

	return docs
}

// applyModelReranking executes reranking through the gateway when available.
func (s *defaultRetrievalService) applyModelReranking(
	ctx context.Context,
	docs []DocumentHit,
	params RerankingParams,
) []DocumentHit {
	if params.RerankingModel == nil || len(docs) == 0 {
		return docs
	}

	if reranked, ok := s.applyGatewayRerank(ctx, docs, params); ok {
		return reranked
	}

	return docs
}

// applyGatewayRerank uses LLM gateway rerank; returns ok=false if not executed or failed.
func (s *defaultRetrievalService) applyGatewayRerank(
	ctx context.Context,
	docs []DocumentHit,
	params RerankingParams,
) ([]DocumentHit, bool) {
	modelName, ok := params.RerankingModel["reranking_model_name"].(string)
	if !ok || modelName == "" {
		return nil, false
	}

	inv := s.rerankInvoker
	if inv == nil && s.llmClient != nil {
		gatewayInvoker, invErr := NewGatewayRerankInvoker(s.llmClient, params.OrganizationID, params.TenantID, params.BillingSubjectType)
		if invErr != nil {
			return nil, false
		}
		inv = gatewayInvoker
	}
	if inv == nil {
		return nil, false
	}

	topN := params.TopK
	if topN <= 0 || topN > len(docs) {
		topN = len(docs)
	}

	docContents := make([]string, len(docs))
	for i, doc := range docs {
		docContents[i] = doc.PageContent
	}

	results, err := inv.Rerank(ctx, params.UserID, params.AppID, AppType, params.Query, docContents, modelName, topN)
	if err != nil || len(results) == 0 {
		return nil, false
	}

	rerankedDocs := make([]DocumentHit, 0, len(results))
	for _, result := range results {
		if result.Index < 0 || result.Index >= len(docs) {
			continue
		}

		doc := docs[result.Index]
		doc.Score = result.RelevanceScore
		if doc.Metadata == nil {
			doc.Metadata = map[string]any{}
		}
		doc.Metadata["score"] = result.RelevanceScore
		doc.Metadata["rerank_score"] = result.RelevanceScore

		rerankedDocs = append(rerankedDocs, doc)
	}

	if len(rerankedDocs) == 0 {
		return nil, false
	}

	sort.SliceStable(rerankedDocs, func(i, j int) bool {
		return rerankedDocs[i].Score > rerankedDocs[j].Score
	})

	return rerankedDocs, true
}

// ExternalDocument represents a document returned from external knowledge API
type ExternalDocument struct {
	Content  string         `json:"content"`
	Score    float64        `json:"score"`
	Title    string         `json:"title"`
	Metadata map[string]any `json:"metadata"`
}

func (s *defaultRetrievalService) fetchExternalKnowledgeRetrieval(
	ctx context.Context,
	dataset *datasetmodel.Dataset,
	query string,
	metadataCondition *MetadataCondition,
) ([]ExternalDocument, error) {
	datasetRepo := drepo.NewDatasetRepository(s.db)

	// Get external knowledge binding information
	binding, err := datasetRepo.GetExternalKnowledgeBindingByDatasetID(ctx, dataset.ID)
	if err != nil {
		return nil, fmt.Errorf("external knowledge binding not found: %w", err)
	}

	// Get external knowledge API configuration
	api, err := datasetRepo.GetExternalKnowledgeApiByID(ctx, binding.ExternalKnowledgeApiID)
	if err != nil {
		return nil, fmt.Errorf("external knowledge api not found: %w", err)
	}

	// Parse API settings
	var settings map[string]any
	if err := json.Unmarshal([]byte(api.Settings), &settings); err != nil {
		return nil, fmt.Errorf("failed to parse API settings: %w", err)
	}

	endpoint, ok := settings["endpoint"].(string)
	if !ok || endpoint == "" {
		return nil, fmt.Errorf("endpoint not found in API settings")
	}

	apiKey, ok := settings["api_key"].(string)
	if !ok || apiKey == "" {
		return nil, fmt.Errorf("api_key not found in API settings")
	}

	// Build request parameters
	requestParams := map[string]any{
		"retrieval_setting": map[string]any{
			"top_k":           dataset.RetrievalConfig["top_k"],
			"score_threshold": 0.0,
		},
		"query":        query,
		"knowledge_id": binding.ExternalKnowledgeID,
	}

	// Set score threshold
	if scoreThresholdEnabled, ok := dataset.RetrievalConfig["score_threshold_enabled"].(bool); ok && scoreThresholdEnabled {
		if scoreThreshold, ok := dataset.RetrievalConfig["score_threshold"].(float64); ok {
			requestParams["retrieval_setting"].(map[string]any)["score_threshold"] = scoreThreshold
		}
	}

	// Set metadata condition
	if metadataCondition != nil {
		requestParams["metadata_condition"] = metadataCondition
	}

	// Send HTTP request
	return s.processExternalAPIRequest(ctx, endpoint+"/retrieval", apiKey, requestParams)
}

// processExternalAPIRequest sends HTTP request to external knowledge API
func (s *defaultRetrievalService) processExternalAPIRequest(
	ctx context.Context,
	url string,
	apiKey string,
	params map[string]interface{},
) ([]ExternalDocument, error) {
	// Serialize request parameters
	jsonData, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request params: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set request headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	// Send request
	client := observability.HTTPClient(&http.Client{Timeout: 30 * time.Second})
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			logger.Warn("failed to close external API response body", map[string]interface{}{
				"error": closeErr.Error(),
			})
		}
	}()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var response struct {
		Records []map[string]any `json:"records"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert to ExternalDocument structure
	var documents []ExternalDocument
	for _, record := range response.Records {
		doc := ExternalDocument{
			Content:  getStringFromMap(record, "content"),
			Score:    getFloatFromMap(record, "score"),
			Title:    getStringFromMap(record, "title"),
			Metadata: getMetadataFromMap(record, "metadata"),
		}
		documents = append(documents, doc)
	}

	return documents, nil
}

// Helper function to safely get values from map
func getStringFromMap(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func getFloatFromMap(m map[string]any, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	if v, ok := m[key].(int); ok {
		return float64(v)
	}
	return 0.0
}

func getMetadataFromMap(m map[string]any, key string) map[string]any {
	if v, ok := m[key].(map[string]any); ok {
		return v
	}
	return make(map[string]any)
}
