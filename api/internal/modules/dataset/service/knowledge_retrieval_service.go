package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	goRedis "github.com/redis/go-redis/v9"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	dataset_model "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	dataset_repository "github.com/zgiai/zgi/api/internal/modules/dataset/repository"
	"github.com/zgiai/zgi/api/internal/modules/dataset/retrieval"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmruntime "github.com/zgiai/zgi/api/internal/modules/llm/runtime"
	shared_model "github.com/zgiai/zgi/api/internal/modules/shared/model"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/embedding"
	"github.com/zgiai/zgi/api/pkg/logger"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
	"github.com/zgiai/zgi/api/pkg/tokenization"
	"github.com/zgiai/zgi/api/pkg/vectordb"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	defaultKnowledgeListLimit      = 20
	maxKnowledgeListLimit          = 100
	defaultKnowledgeTopK           = 5
	maxKnowledgeTopK               = 10
	defaultKnowledgeContextSep     = "\n"
	defaultKnowledgeMaxContextChar = 12000
	legacyKnowledgeRateLimitPlan   = "default"

	// The knowledge retrieval interface returns parent chunks regardless of the retrieval
	// mode, so merged candidates are reranked once as a single pool. A batch is kept small
	// enough for the gateway request, but the merged pool is never truncated before batching.
	crossDatasetRerankBatchSize      = 32
	crossDatasetRerankMaxContentRune = 1500

	KnowledgeRetrieveStatusSuccess   = "success"
	KnowledgeRetrieveStatusNoResults = "no_results"
	KnowledgeRetrieveStatusNoConfig  = "no_config"
	KnowledgeRetrieveStatusBlocked   = "blocked"

	KnowledgeListStatusSuccess   = "success"
	KnowledgeListStatusNoResults = "no_results"
	KnowledgeListStatusFallback  = "fallback"
)

// KnowledgeScope identifies the caller that is allowed to retrieve knowledge.
type KnowledgeScope struct {
	WorkspaceID    string
	OrganizationID string
	AccountID      string
	AppID          string
}

// KnowledgeDatasetSummary is the model-facing dataset summary used for selection.
type KnowledgeDatasetSummary struct {
	DatasetID       string `json:"dataset_id"`
	WorkspaceID     string `json:"workspace_id,omitempty"`
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	Provider        string `json:"provider,omitempty"`
	EnableGraphFlow bool   `json:"enable_graph_flow"`
}

// KnowledgeListResponse describes accessible knowledge base candidates.
type KnowledgeListResponse struct {
	Query          string                    `json:"query"`
	Status         string                    `json:"status"`
	ResultCount    int                       `json:"result_count"`
	FallbackUsed   bool                      `json:"fallback_used"`
	Limit          int                       `json:"limit"`
	Warnings       []string                  `json:"warnings,omitempty"`
	KnowledgeBases []KnowledgeDatasetSummary `json:"knowledge_bases"`
}

// KnowledgeCandidatePage is a stable page of accessible datasets for an Agent candidate picker.
type KnowledgeCandidatePage struct {
	Page           int                       `json:"page"`
	Limit          int                       `json:"limit"`
	Total          int64                     `json:"total"`
	HasMore        bool                      `json:"has_more"`
	KnowledgeBases []KnowledgeDatasetSummary `json:"knowledge_bases"`
}

// KnowledgeRetrieveRequest describes a knowledge retrieval request.
type KnowledgeRetrieveRequest struct {
	Scope             KnowledgeScope
	Query             string
	DatasetIDs        []string
	AgentBindingGrant bool
	// AllowRuntimeManagedDataset is reserved for DigitalHuman's internal
	// projection bridge. Public tools and resource bindings must leave it false.
	AllowRuntimeManagedDataset bool
	TopK                       int
	// RetrievalMode accepts hybrid, vectorAndBM25 (legacy vector is also accepted), or graph.
	RetrievalMode    string
	FallbackPolicy   string
	RetrievalConfig  map[string]interface{}
	ContextSeparator string
	MaxContextChars  int
}

// KnowledgeRetrieverResource is a compact citation/resource payload for tools.
type KnowledgeRetrieverResource struct {
	Position        int                          `json:"position"`
	DatasetID       string                       `json:"dataset_id,omitempty"`
	DatasetName     string                       `json:"dataset_name,omitempty"`
	DocumentID      string                       `json:"document_id,omitempty"`
	DocumentName    string                       `json:"document_name,omitempty"`
	DataSourceType  string                       `json:"data_source_type,omitempty"`
	SegmentID       string                       `json:"segment_id,omitempty"`
	Score           float64                      `json:"score,omitempty"`
	Content         string                       `json:"content,omitempty"`
	MatchType       string                       `json:"match_type,omitempty"`
	RetrievalSource *dto.RetrievalSourceResponse `json:"retrieval_source,omitempty"`
	DocMetadata     map[string]interface{}       `json:"doc_metadata,omitempty"`
}

// KnowledgeSourceSummary is a model-facing citation summary without internal ids.
type KnowledgeSourceSummary struct {
	Position     int     `json:"position"`
	DatasetName  string  `json:"dataset_name,omitempty"`
	DocumentName string  `json:"document_name,omitempty"`
	MatchType    string  `json:"match_type,omitempty"`
	Score        float64 `json:"score,omitempty"`
}

// KnowledgeContextBlock is a source-marked context block aligned with retriever resources.
type KnowledgeContextBlock struct {
	Position int     `json:"position"`
	Source   string  `json:"source,omitempty"`
	Score    float64 `json:"score,omitempty"`
}

// KnowledgeRetrieveResponse is returned to builtin tools and skill callers.
type KnowledgeRetrieveResponse struct {
	Query           string                         `json:"query"`
	Status          string                         `json:"status"`
	Context         string                         `json:"context"`
	ResultCount     int                            `json:"result_count"`
	TopScore        *float64                       `json:"top_score,omitempty"`
	SourceSummary   []KnowledgeSourceSummary       `json:"source_summary,omitempty"`
	ContextBlocks   []KnowledgeContextBlock        `json:"context_blocks,omitempty"`
	Warnings        []string                       `json:"warnings,omitempty"`
	Control         *KnowledgeRetrievalControl     `json:"control,omitempty"`
	Resources       []KnowledgeRetrieverResource   `json:"retriever_resources"`
	Records         []dto.HitTestingRecordResponse `json:"records,omitempty"`
	GraphExecutions []*dto.GraphExecution          `json:"graph_executions,omitempty"`
}

// KnowledgeRetrievalControl reports the per-execution retrieval budget to Agent callers.
// It is intentionally omitted from AIChat responses that do not run the Agent Knowledge
// loop, preserving the existing non-Agent response contract.
type KnowledgeRetrievalControl struct {
	ContinuationAllowed bool    `json:"continuation_allowed"`
	RemainingRounds     int     `json:"remaining_rounds"`
	Round               int     `json:"round"`
	NewEvidenceCount    int     `json:"new_evidence_count"`
	StopReason          *string `json:"stop_reason"`
}

// KnowledgeRetrievalService wraps dataset-native retrieval for skill and agent use.
type KnowledgeRetrievalService struct {
	db               *gorm.DB
	datasetRepo      dataset_repository.DatasetRepository
	documentRepo     dataset_repository.DocumentRepository
	retrievalService *RetrievalService
	hitTesting       *hitTestingService
	llmClient        llmclient.LLMClient
	defaultModelSvc  llmdefaultservice.DefaultModelService
}

type scoredKnowledgeRecord struct {
	Record      dto.HitTestingRecordResponse
	DatasetID   string
	DatasetName string
}

// NewKnowledgeRetrievalService creates a knowledge retrieval service backed by dataset retrieval.
func NewKnowledgeRetrievalService(
	db *gorm.DB,
	cfg *config.Config,
	llmClient llmclient.LLMClient,
	defaultModelSvc llmdefaultservice.DefaultModelService,
	graphFlowService *graphflow.Service,
) *KnowledgeRetrievalService {
	datasetRepo := dataset_repository.NewDatasetRepository(db)
	documentRepo := dataset_repository.NewDocumentRepository(db)
	vectorClient := vectordb.NewWeaviateClient(&cfg.VectorStore)
	tokenizer := tokenization.NewTokenizationService()
	vectorRetrieval := retrieval.NewVectorRetrievalService(nil, vectorClient, "")
	keywordRetrieval := retrieval.NewKeywordRetrievalService(tokenizer)
	fullTextRetrieval := retrieval.NewFullTextRetrievalService(tokenizer, 1.5, 0.75, vectorClient)
	hybridRetrieval := retrieval.NewHybridRetrievalService(vectorRetrieval, keywordRetrieval, fullTextRetrieval)
	rerankService := retrieval.NewRerankService()
	embeddingFactory := func(dataset *dataset_model.Dataset) embedding.EmbeddingService {
		return buildEmbeddingServiceForHitTesting(llmClient, dataset, defaultModelSvc)
	}
	retrievalSvc := NewRetrievalServiceWithEmbeddingFactory(
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
	retrievalSvc.SetLLMClient(llmClient)

	ks := &KnowledgeRetrievalService{
		db:               db,
		datasetRepo:      datasetRepo,
		documentRepo:     documentRepo,
		retrievalService: retrievalSvc,
		llmClient:        llmClient,
		defaultModelSvc:  defaultModelSvc,
	}
	ks.hitTesting = &hitTestingService{
		datasetRepo:      datasetRepo,
		documentRepo:     documentRepo,
		vectorClient:     vectorClient,
		retrievalService: retrievalSvc,
		config:           cfg,
		db:               db,
		defaultModelSvc:  defaultModelSvc,
		llmClient:        llmClient,
	}
	return ks
}

// ListAccessibleDatasets returns datasets visible to the current account in the current workspace.
func (s *KnowledgeRetrievalService) ListAccessibleDatasets(ctx context.Context, scope KnowledgeScope, query string, limit int) (*KnowledgeListResponse, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("knowledge retrieval service is not configured")
	}
	limit = normalizeKnowledgeLimit(limit, defaultKnowledgeListLimit, maxKnowledgeListLimit)
	search := strings.TrimSpace(query)
	workspaceID := strings.TrimSpace(scope.WorkspaceID)
	organizationID := strings.TrimSpace(scope.OrganizationID)
	accountID := strings.TrimSpace(scope.AccountID)
	if organizationID == "" && workspaceID == "" {
		return nil, fmt.Errorf("organization_id or workspace_id is required")
	}
	if accountID == "" {
		return nil, fmt.Errorf("account_id is required")
	}

	workspaceIDs := []string{workspaceID}
	if organizationID != "" {
		accessibleWorkspaceIDs, err := s.accessibleKnowledgeWorkspaceIDs(ctx, organizationID, workspaceID, accountID)
		if err != nil {
			return nil, err
		}
		if len(accessibleWorkspaceIDs) == 0 {
			return knowledgeListResponse(search, limit, false, nil), nil
		}
		workspaceIDs = accessibleWorkspaceIDs
	} else {
		allowed, err := s.canAccessKnowledgeWorkspace(ctx, workspaceID, accountID)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return knowledgeListResponse(search, limit, false, nil), nil
		}
	}

	datasets, err := s.findAccessibleDatasets(ctx, workspaceIDs, search, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list accessible datasets: %w", err)
	}
	fallbackUsed := false
	if len(datasets) == 0 && search != "" {
		datasets, err = s.findAccessibleDatasets(ctx, workspaceIDs, "", limit)
		if err != nil {
			return nil, fmt.Errorf("failed to list accessible datasets: %w", err)
		}
		fallbackUsed = len(datasets) > 0
	}

	out := make([]KnowledgeDatasetSummary, 0, len(datasets))
	for _, dataset := range datasets {
		if dataset == nil {
			continue
		}
		out = append(out, KnowledgeDatasetSummary{
			DatasetID:       dataset.ID,
			WorkspaceID:     dataset.WorkspaceID,
			Name:            dataset.Name,
			Description:     stringPtrValue(dataset.Description),
			Provider:        dataset.Provider,
			EnableGraphFlow: dataset.EnableGraphFlow,
		})
	}
	return knowledgeListResponse(search, limit, fallbackUsed, out), nil
}

// ListAccessibleDatasetCandidates returns a permission-scoped page with persisted selections first.
func (s *KnowledgeRetrievalService) ListAccessibleDatasetCandidates(ctx context.Context, scope KnowledgeScope, query string, selectedIDs []string, includeSelected bool, page, limit int) (*KnowledgeCandidatePage, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("knowledge retrieval service is not configured")
	}
	page = normalizeKnowledgeCandidatePage(page)
	limit = normalizeKnowledgeLimit(limit, defaultKnowledgeListLimit, maxKnowledgeListLimit)
	workspaceID := strings.TrimSpace(scope.WorkspaceID)
	organizationID := strings.TrimSpace(scope.OrganizationID)
	accountID := strings.TrimSpace(scope.AccountID)
	if organizationID == "" && workspaceID == "" {
		return nil, fmt.Errorf("organization_id or workspace_id is required")
	}
	if accountID == "" {
		return nil, fmt.Errorf("account_id is required")
	}

	workspaceIDs := []string{workspaceID}
	if organizationID != "" {
		accessibleWorkspaceIDs, err := s.accessibleKnowledgeWorkspaceIDs(ctx, organizationID, workspaceID, accountID)
		if err != nil {
			return nil, err
		}
		workspaceIDs = accessibleWorkspaceIDs
	} else {
		allowed, err := s.canAccessKnowledgeWorkspace(ctx, workspaceID, accountID)
		if err != nil {
			return nil, err
		}
		if !allowed {
			workspaceIDs = nil
		}
	}
	workspaceIDs = normalizeStringList(workspaceIDs)
	if len(workspaceIDs) == 0 {
		return &KnowledgeCandidatePage{
			Page:           page,
			Limit:          limit,
			KnowledgeBases: []KnowledgeDatasetSummary{},
		}, nil
	}

	dbQuery := s.db.WithContext(ctx).
		Model(&dataset_model.Dataset{}).
		Where("workspace_id IN ? AND provider <> '"+dataset_model.ProviderDigitalHumanMemory+"'", workspaceIDs)
	if search := strings.TrimSpace(query); search != "" {
		pattern := "%" + strings.ToLower(search) + "%"
		dbQuery = dbQuery.Where(
			"LOWER(COALESCE(name, '')) LIKE ? OR LOWER(COALESCE(description, '')) LIKE ?",
			pattern,
			pattern,
		)
	}
	selectedIDs = normalizeStringList(selectedIDs)
	if !includeSelected && len(selectedIDs) > 0 {
		dbQuery = dbQuery.Where("id NOT IN ?", selectedIDs)
	}

	var total int64
	if err := dbQuery.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count accessible dataset candidates: %w", err)
	}
	if includeSelected && len(selectedIDs) > 0 {
		dbQuery = dbQuery.Clauses(clause.OrderBy{Expression: clause.Expr{
			SQL:                "CASE WHEN id IN (?) THEN 0 ELSE 1 END, LOWER(name) ASC, id ASC",
			Vars:               []interface{}{selectedIDs},
			WithoutParentheses: true,
		}})
	} else {
		dbQuery = dbQuery.Order("LOWER(name) ASC, id ASC")
	}

	var datasets []*dataset_model.Dataset
	if err := dbQuery.
		Limit(limit).
		Offset((page - 1) * limit).
		Find(&datasets).Error; err != nil {
		return nil, fmt.Errorf("list accessible dataset candidates: %w", err)
	}
	items := make([]KnowledgeDatasetSummary, 0, len(datasets))
	for _, dataset := range datasets {
		if dataset == nil {
			continue
		}
		items = append(items, KnowledgeDatasetSummary{
			DatasetID:       dataset.ID,
			WorkspaceID:     dataset.WorkspaceID,
			Name:            dataset.Name,
			Description:     stringPtrValue(dataset.Description),
			Provider:        dataset.Provider,
			EnableGraphFlow: dataset.EnableGraphFlow,
		})
	}
	return &KnowledgeCandidatePage{
		Page:           page,
		Limit:          limit,
		Total:          total,
		HasMore:        int64(page*limit) < total,
		KnowledgeBases: items,
	}, nil
}

func normalizeKnowledgeCandidatePage(page int) int {
	if page <= 0 {
		return 1
	}
	return page
}

// Retrieve retrieves and merges knowledge from explicitly provided datasets.
func (s *KnowledgeRetrievalService) Retrieve(ctx context.Context, req KnowledgeRetrieveRequest) (*KnowledgeRetrieveResponse, error) {
	if s == nil || s.datasetRepo == nil || s.retrievalService == nil {
		return nil, fmt.Errorf("knowledge retrieval service is not configured")
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if strings.TrimSpace(req.Scope.WorkspaceID) == "" {
		if strings.TrimSpace(req.Scope.OrganizationID) == "" {
			return nil, fmt.Errorf("organization_id or workspace_id is required")
		}
	}
	if strings.TrimSpace(req.Scope.AccountID) == "" {
		return nil, fmt.Errorf("account_id is required")
	}
	if strings.TrimSpace(req.Scope.OrganizationID) == "" && !req.AgentBindingGrant {
		allowed, err := s.canAccessKnowledgeWorkspace(ctx, strings.TrimSpace(req.Scope.WorkspaceID), strings.TrimSpace(req.Scope.AccountID))
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, fmt.Errorf("workspace is not accessible")
		}
	}
	datasetIDs := normalizeStringList(req.DatasetIDs)
	if len(datasetIDs) == 0 {
		return nil, fmt.Errorf("dataset_ids are required")
	}
	rateLimitScopeID := knowledgeRateLimitScopeID(req.Scope)
	if limited, err := s.checkAndUpdateRateLimit(ctx, rateLimitScopeID); err == nil && limited {
		_ = s.recordRateLimitLog(ctx, rateLimitScopeID)
		return nil, fmt.Errorf("knowledge rate limit exceeded")
	}

	topK := normalizeKnowledgeLimit(req.TopK, defaultKnowledgeTopK, maxKnowledgeTopK)
	scoredRecords, graphExecutions, warnings, err := s.retrieveDatasetsParallel(ctx, datasetIDs, req, query, topK)
	if err != nil {
		return nil, err
	}
	scoredRecords = deduplicateKnowledgeRecords(scoredRecords)

	// Per-dataset retrieval owns its own recall width, fusion and rerank configuration and
	// is treated as a fixed contract. When multiple knowledge bases are merged, every record
	// is a parent chunk regardless of retrieval mode and is re-scored once with the tenant
	// default rerank model so their scores are comparable. A single knowledge base keeps its
	// native ordering and avoids a needless second rerank pass.
	if len(datasetIDs) > 1 {
		var rerankWarnings []string
		scoredRecords, rerankWarnings = s.rerankAcrossDatasets(ctx, query, scoredRecords, req.Scope)
		if len(rerankWarnings) > 0 {
			warnings = append(warnings, rerankWarnings...)
		}
	} else {
		sort.SliceStable(scoredRecords, func(i, j int) bool {
			return scoredRecords[i].Record.Score > scoredRecords[j].Record.Score
		})
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if topK > 0 && len(scoredRecords) > topK {
		scoredRecords = scoredRecords[:topK]
	}
	records := make([]dto.HitTestingRecordResponse, 0, len(scoredRecords))
	for _, item := range scoredRecords {
		records = append(records, item.Record)
	}
	resources, contextText, contextBlocks, err := knowledgeResourcesAndContext(scoredRecords, req.ContextSeparator, req.MaxContextChars, config.Current().App.FilesURL)
	if err != nil {
		return nil, err
	}
	return &KnowledgeRetrieveResponse{
		Query:           query,
		Status:          knowledgeRetrieveStatus(resources),
		Context:         contextText,
		ResultCount:     len(resources),
		TopScore:        knowledgeTopScore(resources),
		SourceSummary:   knowledgeSourceSummary(resources),
		ContextBlocks:   contextBlocks,
		Warnings:        warnings,
		Resources:       resources,
		Records:         records,
		GraphExecutions: graphExecutions,
	}, nil
}

type datasetRetrievalResult struct {
	datasetID   string
	datasetName string
	records     []dto.HitTestingRecordResponse
	execution   *dto.GraphExecution
}

type datasetRetrievalFailure struct {
	err error
}

// retrieveDatasetsParallel invokes the knowledge retrieval interface once per dataset and
// keeps merged results aligned with the requested dataset order.
//
// A dataset that fails is reported per dataset instead of aborting the whole retrieval:
// one knowledge base without graph support must not discard the results of the others.
// Authorization failures are fatal; recoverable retrieval failures are downgraded when
// another dataset succeeds (or when the failure is only missing graph capability).
func (s *KnowledgeRetrievalService) retrieveDatasetsParallel(
	ctx context.Context,
	datasetIDs []string,
	req KnowledgeRetrieveRequest,
	query string,
	topK int,
) ([]scoredKnowledgeRecord, []*dto.GraphExecution, []string, error) {
	results := make([]datasetRetrievalResult, len(datasetIDs))
	failures := make([]datasetRetrievalFailure, len(datasetIDs))
	datasetWarnings := make([]string, len(datasetIDs))
	datasets := make([]*dataset_model.Dataset, len(datasetIDs))
	accessFailures := make([]error, len(datasetIDs))

	var wg sync.WaitGroup
	for index, datasetID := range datasetIDs {
		wg.Add(1)
		go func(index int, datasetID string) {
			defer wg.Done()
			dataset, err := s.accessibleKnowledgeDataset(ctx, datasetID, req.Scope, req.AgentBindingGrant, req.AllowRuntimeManagedDataset)
			if err != nil {
				accessFailures[index] = err
				return
			}
			datasets[index] = dataset
		}(index, datasetID)
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, err
	}
	for _, accessFailure := range accessFailures {
		if accessFailure == nil {
			continue
		}
		// Validate every binding before starting retrieval. This keeps a request with one
		// revoked binding from causing side effects (such as hit-count updates) in another
		// binding before the authorization error is returned.
		return nil, nil, nil, fmt.Errorf("knowledge base is not accessible")
	}

	for index, dataset := range datasets {
		if graphRequestedForKnowledge(req.RetrievalMode) && !dataset.EnableGraphFlow {
			datasetWarnings[index] = fmt.Sprintf("knowledge base [%d/%d] does not support graph retrieval; continuing with available sources", index+1, len(datasetIDs))
		}
	}

	for index, dataset := range datasets {
		wg.Add(1)
		go func(index int, dataset *dataset_model.Dataset) {
			defer wg.Done()
			datasetRecords, execution, err := s.retrieveDataset(ctx, dataset, query, req.RetrievalConfig, topK, req.RetrievalMode, req.FallbackPolicy)
			if err != nil {
				failures[index] = datasetRetrievalFailure{err: err}
				return
			}
			if graphRequestedForKnowledge(req.RetrievalMode) && execution != nil && execution.Degraded {
				datasetWarnings[index] = fmt.Sprintf("knowledge base [%d/%d] graph retrieval degraded; continuing with available sources", index+1, len(datasetIDs))
			}
			results[index] = datasetRetrievalResult{
				datasetID:   dataset.ID,
				datasetName: dataset.Name,
				records:     datasetRecords,
				execution:   execution,
			}
		}(index, dataset)
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, err
	}

	warnings := make([]string, 0)
	for _, warning := range datasetWarnings {
		if warning != "" {
			warnings = append(warnings, warning)
		}
	}
	failed := 0
	fatalFailures := 0
	var firstFailure error
	for index, failure := range failures {
		if failure.err == nil {
			continue
		}
		failed++
		if firstFailure == nil {
			firstFailure = failure.err
		}
		if !isRecoverableKnowledgeRetrievalError(failure.err) {
			fatalFailures++
		}
		warnings = append(warnings, knowledgeRetrievalFailureWarning(index, len(datasetIDs), failure.err, graphRequestedForKnowledge(req.RetrievalMode)))
	}
	if firstFailure != nil && failed == len(datasetIDs) && fatalFailures > 0 {
		logger.Warn("All knowledge base retrievals failed", map[string]interface{}{
			"dataset_count": len(datasetIDs),
			"error":         firstFailure.Error(),
		})
		return nil, nil, nil, fmt.Errorf("knowledge retrieval failed")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, err
	}

	scoredRecords := make([]scoredKnowledgeRecord, 0)
	graphExecutions := make([]*dto.GraphExecution, 0)
	for _, result := range results {
		for _, record := range result.records {
			scoredRecords = append(scoredRecords, scoredKnowledgeRecord{
				Record:      record,
				DatasetID:   result.datasetID,
				DatasetName: result.datasetName,
			})
		}
		if result.execution != nil {
			graphExecutions = append(graphExecutions, result.execution)
		}
	}
	return scoredRecords, graphExecutions, warnings, nil
}

func graphRequestedForKnowledge(retrievalMode string) bool {
	switch strings.ToLower(strings.TrimSpace(retrievalMode)) {
	case "", "hybrid", "graph":
		return true
	default:
		return false
	}
}

func isRecoverableKnowledgeRetrievalError(err error) bool {
	if err == nil {
		return true
	}
	// The caller context is checked immediately after the parallel phase. A timeout or
	// cancellation that reaches this helper therefore belongs to an internal retrieval
	// branch (for example, the graph's bounded path timeout) and is safe to downgrade.
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "knowledge graph is not enabled") ||
		strings.Contains(message, "knowledge graph is not ready") ||
		strings.Contains(message, "knowledge graph runtime is unavailable")
}

func knowledgeRetrievalFailureWarning(index, total int, err error, graphRequested bool) string {
	if graphRequested && isRecoverableKnowledgeRetrievalError(err) {
		return fmt.Sprintf("knowledge base [%d/%d] graph retrieval unavailable; continuing with available sources", index+1, total)
	}
	return fmt.Sprintf("knowledge base [%d/%d] retrieval unavailable; continuing with available sources", index+1, total)
}

// deduplicateKnowledgeRecords removes duplicate parent chunks across knowledge bases before
// global reranking. Segment IDs are preferred, while normalized content catches the same
// chunk copied into different knowledge bases. If two copies collide, retain the strongest
// original retrieval score so fallback ordering remains useful.
func deduplicateKnowledgeRecords(records []scoredKnowledgeRecord) []scoredKnowledgeRecord {
	if len(records) < 2 {
		return records
	}
	byID := make(map[string]int, len(records))
	byContent := make(map[string]int, len(records))
	unique := make([]scoredKnowledgeRecord, 0, len(records))
	for _, record := range records {
		id := strings.TrimSpace(record.Record.Segment.ID)
		content := normalizeKnowledgeRecordContent(record.Record.Segment.Content)
		existingIndex := -1
		if id != "" {
			if index, ok := byID[id]; ok {
				existingIndex = index
			}
		}
		if existingIndex < 0 && content != "" {
			if index, ok := byContent[content]; ok {
				existingIndex = index
			}
		}
		if existingIndex >= 0 {
			if record.Record.Score > unique[existingIndex].Record.Score {
				unique[existingIndex] = record
				if replacementID := strings.TrimSpace(record.Record.Segment.ID); replacementID != "" {
					byID[replacementID] = existingIndex
				}
				if replacementContent := normalizeKnowledgeRecordContent(record.Record.Segment.Content); replacementContent != "" {
					byContent[replacementContent] = existingIndex
				}
			}
			continue
		}
		index := len(unique)
		unique = append(unique, record)
		if id != "" {
			byID[id] = index
		}
		if content != "" {
			byContent[content] = index
		}
	}
	return unique
}

func normalizeKnowledgeRecordContent(content string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(content))), " ")
}

// rerankAcrossDatasets re-scores merged candidates with the tenant default rerank model so
// that scores coming from different knowledge bases become comparable. It treats every record
// as a parent chunk and does not distinguish which retrieval mode produced it, including
// graph chunks. When no rerank model is available the merged order degrades to the per-dataset
// retrieval scores.
func (s *KnowledgeRetrievalService) rerankAcrossDatasets(
	ctx context.Context,
	query string,
	records []scoredKnowledgeRecord,
	scope KnowledgeScope,
) ([]scoredKnowledgeRecord, []string) {
	orderByScore := func() {
		sort.SliceStable(records, func(i, j int) bool {
			return records[i].Record.Score > records[j].Record.Score
		})
	}
	if len(records) == 0 {
		return records, nil
	}

	// Every record is a parent chunk no matter which retrieval mode produced it, so graph
	// chunks are reranked together with vector/BM25 chunks instead of being passed through.
	type rerankCandidate struct {
		index   int
		content string
	}
	candidates := make([]rerankCandidate, 0, len(records))
	for index, item := range records {
		content := truncateRerankContent(item.Record.Segment.Content)
		if content == "" {
			orderByScore()
			return records, []string{"cross knowledge base rerank unavailable; results ordered by retrieval score"}
		}
		candidates = append(candidates, rerankCandidate{index: index, content: content})
	}
	if len(candidates) < 2 {
		orderByScore()
		return records, nil
	}

	rerankable := make([]int, 0, len(candidates))
	contents := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		rerankable = append(rerankable, candidate.index)
		contents = append(contents, candidate.content)
	}

	degraded := []string{"cross knowledge base rerank unavailable; results ordered by retrieval score"}
	model, err := s.resolveDefaultRerankModel(ctx, scope)
	if err != nil || model == nil || s.llmClient == nil {
		orderByScore()
		return records, degraded
	}

	workspaceID := strings.TrimSpace(scope.WorkspaceID)
	if workspaceID == "" {
		// Organization-scoped retrieval does not always carry a workspace ID. The
		// rerank gateway still needs a tenant context, so use the organization as
		// that context instead of silently disabling cross-knowledge-base rerank.
		workspaceID = strings.TrimSpace(scope.OrganizationID)
	}
	if workspaceID == "" {
		orderByScore()
		return records, degraded
	}
	accountID := strings.TrimSpace(scope.AccountID)
	if accountID == "" {
		accountID = workspaceID
	}

	rerankSvc, err := retrieval.NewGatewayRerankService(s.llmClient, accountID, strings.TrimSpace(scope.AppID), "dataset", model.Model, workspaceID)
	if err != nil {
		orderByScore()
		return records, degraded
	}

	scores, scored, err := rerankInBatches(ctx, rerankSvc, query, contents)
	if err != nil {
		logger.Warn("Cross knowledge base rerank failed", map[string]interface{}{
			"query": query,
			"model": model.Model,
			"error": err.Error(),
			"count": len(contents),
		})
		orderByScore()
		return records, []string{"cross knowledge base rerank failed; results ordered by retrieval score"}
	}
	if !commitKnowledgeRerankScores(records, rerankable, scores, scored) {
		// Do not mix rerank scores with retrieval scores. The global ordering is either
		// entirely reranked or entirely based on the original retrieval score.
		orderByScore()
		return records, []string{"cross knowledge base rerank returned incomplete scores; results ordered by retrieval score"}
	}

	orderByScore()
	return records, nil
}

// commitKnowledgeRerankScores applies scores atomically. A provider response that omits,
// duplicates, or indexes outside the input is represented by scored=false and causes the
// caller to keep every candidate's original retrieval score.
func commitKnowledgeRerankScores(records []scoredKnowledgeRecord, indexes []int, scores []float64, scored []bool) bool {
	if len(indexes) != len(scores) || len(indexes) != len(scored) {
		return false
	}
	for offset, ok := range scored {
		if !ok || indexes[offset] < 0 || indexes[offset] >= len(records) {
			return false
		}
	}
	for offset, index := range indexes {
		score := scores[offset]
		records[index].Record.Score = score
		if records[index].Record.RetrievalSource == nil {
			records[index].Record.RetrievalSource = &dto.RetrievalSourceResponse{}
		}
		records[index].Record.RetrievalSource.RerankScore = &score
	}
	return true
}

// rerankInBatches scores candidates in fixed size batches so a large merged pool stays
// within the rerank model input limit. Rerank relevance scores are produced per
// (query, document) pair, so batches are scored independently and merged into one ordering.
type knowledgeReranker interface {
	Rerank(ctx context.Context, query string, documents []string, scoreThreshold *float64, topN *int) ([]retrieval.GatewayRerankResult, error)
}

func rerankInBatches(
	ctx context.Context,
	svc knowledgeReranker,
	query string,
	contents []string,
) ([]float64, []bool, error) {
	scores := make([]float64, len(contents))
	scored := make([]bool, len(contents))
	for start := 0; start < len(contents); start += crossDatasetRerankBatchSize {
		end := start + crossDatasetRerankBatchSize
		if end > len(contents) {
			end = len(contents)
		}
		batch := contents[start:end]
		topN := len(batch)
		results, err := svc.Rerank(ctx, query, batch, nil, &topN)
		if err != nil {
			return nil, nil, err
		}
		seenIndexes := make(map[int]struct{}, len(results))
		for _, item := range results {
			if item.Index < 0 || item.Index >= len(batch) {
				return nil, nil, fmt.Errorf("rerank returned out-of-range index %d", item.Index)
			}
			if _, duplicate := seenIndexes[item.Index]; duplicate {
				return nil, nil, fmt.Errorf("rerank returned duplicate index %d", item.Index)
			}
			seenIndexes[item.Index] = struct{}{}
			scores[start+item.Index] = item.Score
			scored[start+item.Index] = true
		}
	}
	return scores, scored, nil
}

// truncateRerankContent bounds a single chunk sent to the rerank model.
func truncateRerankContent(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	runes := []rune(content)
	if len(runes) <= crossDatasetRerankMaxContentRune {
		return content
	}
	return string(runes[:crossDatasetRerankMaxContentRune])
}

func (s *KnowledgeRetrievalService) resolveDefaultRerankModel(ctx context.Context, scope KnowledgeScope) (*llmruntime.ResolvedModel, error) {
	if s.defaultModelSvc == nil {
		return nil, fmt.Errorf("default model service is not configured")
	}
	tenantID := strings.TrimSpace(scope.OrganizationID)
	if tenantID == "" {
		tenantID = strings.TrimSpace(scope.WorkspaceID)
	}
	if tenantID == "" {
		return nil, fmt.Errorf("tenant is required")
	}
	return llmruntime.NewModelResolver(s.defaultModelSvc).ResolveDefault(ctx, tenantID, shared_model.ModelTypeRerank)
}

// RetrieveAgentKnowledge retrieves using dataset ids configured on the agent.
func (s *KnowledgeRetrievalService) RetrieveAgentKnowledge(ctx context.Context, req KnowledgeRetrieveRequest) (*KnowledgeRetrieveResponse, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	agentID := strings.TrimSpace(req.Scope.AppID)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	datasetIDs := normalizeStringList(req.DatasetIDs)
	retrievalConfig := req.RetrievalConfig
	if len(datasetIDs) == 0 || retrievalConfig == nil {
		configDatasetIDs, configRetrievalConfig, err := s.agentKnowledgeConfig(ctx, agentID)
		if err != nil {
			return nil, err
		}
		if len(datasetIDs) == 0 {
			datasetIDs = configDatasetIDs
		}
		if retrievalConfig == nil {
			retrievalConfig = configRetrievalConfig
		}
	}
	if len(datasetIDs) == 0 {
		return &KnowledgeRetrieveResponse{
			Query:       query,
			Status:      KnowledgeRetrieveStatusNoConfig,
			ResultCount: 0,
			Warnings:    []string{"agent has no configured knowledge datasets"},
			Resources:   []KnowledgeRetrieverResource{},
		}, nil
	}
	req.DatasetIDs = datasetIDs
	req.RetrievalConfig = retrievalConfig
	return s.Retrieve(ctx, req)
}

// ValidateAccessibleDatasets verifies that accountID can access every dataset in ids now.
func (s *KnowledgeRetrievalService) ValidateAccessibleDatasets(ctx context.Context, scope KnowledgeScope, ids []string) error {
	for _, datasetID := range normalizeStringList(ids) {
		if _, err := s.accessibleKnowledgeDataset(ctx, datasetID, scope, false, false); err != nil {
			return fmt.Errorf("dataset %s is not accessible: %w", datasetID, err)
		}
	}
	return nil
}

func (s *KnowledgeRetrievalService) canAccessKnowledgeWorkspace(ctx context.Context, workspaceID, accountID string) (bool, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	accountID = strings.TrimSpace(accountID)
	if workspaceID == "" || accountID == "" {
		return false, nil
	}

	var member struct {
		Role             string                                          `gorm:"column:role"`
		RoleID           *string                                         `gorm:"column:role_id"`
		Permissions      string                                          `gorm:"column:permissions"`
		PermissionSource workspace_model.WorkspaceMemberPermissionSource `gorm:"column:permission_source"`
	}
	if err := s.db.WithContext(ctx).
		Table("workspace_members").
		Select("role, role_id, COALESCE(permissions::text, '') AS permissions, permission_source").
		Where("workspace_id = ? AND account_id = ?", workspaceID, accountID).
		Take(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check workspace knowledge permission: %w", err)
	}
	return knowledgeWorkspaceMemberCanUseKnowledge(member.Role, member.RoleID, member.Permissions, member.PermissionSource), nil
}

func (s *KnowledgeRetrievalService) findAccessibleDatasets(ctx context.Context, workspaceIDs []string, search string, limit int) ([]*dataset_model.Dataset, error) {
	workspaceIDs = normalizeStringList(workspaceIDs)
	if len(workspaceIDs) == 0 {
		return []*dataset_model.Dataset{}, nil
	}

	// Runtime retrieval intentionally uses organization/workspace membership as its access boundary.
	// Legacy dataset_permissions and per-dataset visibility fields are deprecated for AIChat/Agent tools.
	dbQuery := s.db.WithContext(ctx).
		Model(&dataset_model.Dataset{}).
		Where("workspace_id IN ? AND provider <> '"+dataset_model.ProviderDigitalHumanMemory+"'", workspaceIDs)
	if search != "" {
		pattern := "%" + search + "%"
		dbQuery = dbQuery.Where("LOWER(name) LIKE LOWER(?) OR LOWER(description) LIKE LOWER(?)", pattern, pattern)
	}

	var datasets []*dataset_model.Dataset
	if err := dbQuery.
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&datasets).Error; err != nil {
		return nil, err
	}
	return datasets, nil
}

func knowledgeListResponse(query string, limit int, fallbackUsed bool, datasets []KnowledgeDatasetSummary) *KnowledgeListResponse {
	status := KnowledgeListStatusSuccess
	warnings := []string(nil)
	if fallbackUsed {
		status = KnowledgeListStatusFallback
		warnings = append(warnings, "no knowledge bases matched the query; showing recent accessible knowledge bases")
	} else if len(datasets) == 0 {
		status = KnowledgeListStatusNoResults
	}
	return &KnowledgeListResponse{
		Query:          strings.TrimSpace(query),
		Status:         status,
		ResultCount:    len(datasets),
		FallbackUsed:   fallbackUsed,
		Limit:          limit,
		Warnings:       warnings,
		KnowledgeBases: datasets,
	}
}

func (s *KnowledgeRetrievalService) accessibleKnowledgeWorkspaceIDs(ctx context.Context, organizationID, workspaceID, accountID string) ([]string, error) {
	organizationID = strings.TrimSpace(organizationID)
	workspaceID = strings.TrimSpace(workspaceID)
	accountID = strings.TrimSpace(accountID)
	if organizationID == "" || accountID == "" {
		return nil, nil
	}

	query := s.db.WithContext(ctx).
		Table("workspaces").
		Select("workspaces.id, workspace_members.role, workspace_members.role_id, COALESCE(workspace_members.permissions::text, '') AS permissions, workspace_members.permission_source").
		Joins("JOIN workspace_members ON workspace_members.workspace_id = workspaces.id").
		Where("workspaces.organization_id = ? AND workspace_members.account_id = ?", organizationID, accountID)
	if workspaceID != "" {
		query = query.Where("workspaces.id = ?", workspaceID)
	}

	var rows []struct {
		ID               string                                          `gorm:"column:id"`
		Role             string                                          `gorm:"column:role"`
		RoleID           *string                                         `gorm:"column:role_id"`
		Permissions      string                                          `gorm:"column:permissions"`
		PermissionSource workspace_model.WorkspaceMemberPermissionSource `gorm:"column:permission_source"`
	}
	if err := query.
		Order("workspaces.created_at DESC, workspaces.id DESC").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to list accessible workspaces: %w", err)
	}
	workspaceIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		if knowledgeWorkspaceMemberCanUseKnowledge(row.Role, row.RoleID, row.Permissions, row.PermissionSource) {
			workspaceIDs = append(workspaceIDs, row.ID)
		}
	}
	return normalizeStringList(workspaceIDs), nil
}

func knowledgeWorkspaceMemberCanUseKnowledge(role string, roleID *string, rawPermissions string, permissionSource workspace_model.WorkspaceMemberPermissionSource) bool {
	rawPermissions = strings.TrimSpace(rawPermissions)
	permissions := []string{}
	if rawPermissions != "" && rawPermissions != "null" {
		if err := json.Unmarshal([]byte(rawPermissions), &permissions); err != nil {
			permissions = []string{}
		}
	}

	effectivePermissions := workspace_model.EffectiveWorkspaceMemberPermissionStrings(
		workspace_model.WorkspaceMemberRole(strings.TrimSpace(role)),
		roleID,
		permissions,
		permissionSource,
	)
	for _, permission := range knowledgeBaseReadPermissionCodes() {
		if workspace_model.WorkspacePermissionStringsAllow(effectivePermissions, permission) {
			return true
		}
	}
	return false
}

func (s *KnowledgeRetrievalService) accessibleKnowledgeDataset(ctx context.Context, datasetID string, scope KnowledgeScope, agentBindingGrant bool, runtimeManaged ...bool) (*dataset_model.Dataset, error) {
	datasetID = strings.TrimSpace(datasetID)
	if datasetID == "" {
		return nil, fmt.Errorf("dataset_id is required")
	}
	workspaceID := strings.TrimSpace(scope.WorkspaceID)
	organizationID := strings.TrimSpace(scope.OrganizationID)
	if organizationID == "" && workspaceID == "" {
		return nil, fmt.Errorf("organization_id or workspace_id is required")
	}

	query := s.db.WithContext(ctx).Where("id = ?", datasetID)
	if organizationID != "" {
		if agentBindingGrant {
			query = query.Where("organization_id = ?", organizationID)
			if workspaceID != "" {
				query = query.Where("workspace_id = ?", workspaceID)
			}
		} else {
			workspaceIDs, err := s.accessibleKnowledgeWorkspaceIDs(ctx, organizationID, workspaceID, scope.AccountID)
			if err != nil {
				return nil, err
			}
			if len(workspaceIDs) == 0 {
				return nil, gorm.ErrRecordNotFound
			}
			query = query.Where("organization_id = ? AND workspace_id IN ?", organizationID, workspaceIDs)
		}
	} else {
		query = query.Where("workspace_id = ?", workspaceID)
	}

	var dataset dataset_model.Dataset
	if err := query.First(&dataset).Error; err != nil {
		return nil, err
	}
	allowRuntimeManaged := len(runtimeManaged) > 0 && runtimeManaged[0]
	if dataset.Provider == dataset_model.ProviderDigitalHumanMemory && !allowRuntimeManaged {
		return nil, gorm.ErrRecordNotFound
	}
	return &dataset, nil
}

func knowledgeRateLimitScopeID(scope KnowledgeScope) string {
	if organizationID := strings.TrimSpace(scope.OrganizationID); organizationID != "" {
		return organizationID
	}
	return strings.TrimSpace(scope.WorkspaceID)
}

func (s *KnowledgeRetrievalService) retrieveDataset(ctx context.Context, dataset *dataset_model.Dataset, query string, retrievalConfig map[string]interface{}, topK int, retrievalMode string, fallbackPolicy string) ([]dto.HitTestingRecordResponse, *dto.GraphExecution, error) {
	effectiveRetrievalConfig := mergeKnowledgeRetrievalConfig(dataset.RetrievalConfig, retrievalConfig)
	if dataset.Provider == "external" {
		response, err := s.hitTesting.ExternalRetrieve(ctx, dataset, query, "", effectiveRetrievalConfig)
		if err != nil {
			return nil, nil, err
		}
		if response == nil {
			return nil, nil, nil
		}
		for i := range response.Records {
			if response.Records[i].Segment.Document.Name == "" {
				response.Records[i].Segment.Document.Name = dataset.Name
			}
			if response.Records[i].Segment.Document.DataSourceType == "" {
				response.Records[i].Segment.Document.DataSourceType = "external"
			}
		}
		return response.Records, response.GraphExecution, nil
	}
	options := s.hitTesting.getRetrievalOptions(ctx, effectiveRetrievalConfig, dataset)
	options.TopK = topK
	options.RetrievalMode = normalizeRetrievalMode(retrievalMode)
	options.FallbackPolicy = strings.TrimSpace(fallbackPolicy)
	// The merged pool is reranked once at this layer, so keep a per source quota instead of
	// letting in-dataset fusion truncate graph chunks away.
	options.SourceQuotaTopK = true
	if options.FallbackPolicy == "" {
		if configuredPolicy, ok := effectiveRetrievalConfig["fallback_policy"].(string); ok {
			options.FallbackPolicy = configuredPolicy
		}
	}
	records, graphExecution, err := s.retrievalService.Retrieve(ctx, dataset, query, options)
	if err != nil {
		return nil, nil, err
	}
	return records, graphExecution, nil
}

func mergeKnowledgeRetrievalConfig(base map[string]interface{}, overrides map[string]interface{}) map[string]interface{} {
	if base == nil && overrides == nil {
		return nil
	}
	merged := make(map[string]interface{}, len(base)+len(overrides))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range overrides {
		merged[key] = value
	}
	return merged
}

func (s *KnowledgeRetrievalService) agentKnowledgeConfig(ctx context.Context, agentID string) ([]string, map[string]interface{}, error) {
	var row struct {
		DatasetConfigs *string `gorm:"column:dataset_configs"`
		Configs        *string `gorm:"column:configs"`
		Retriever      *string `gorm:"column:retriever_resource"`
		AgentMode      *string `gorm:"column:agent_mode"`
	}
	err := s.db.WithContext(ctx).
		Table("agents_configs").
		Where("agents_id = ? AND deleted_at IS NULL", agentID).
		Order("updated_at DESC").
		Limit(1).
		Scan(&row).Error
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load agent knowledge config: %w", err)
	}
	rawConfigs := []*string{row.AgentMode, row.DatasetConfigs, row.Configs, row.Retriever}
	var datasetIDs []string
	var retrievalConfig map[string]interface{}
	for _, rawConfig := range rawConfigs {
		if rawConfig == nil || strings.TrimSpace(*rawConfig) == "" {
			continue
		}
		var raw interface{}
		if err := json.Unmarshal([]byte(*rawConfig), &raw); err != nil {
			return nil, nil, fmt.Errorf("invalid agent dataset config: %w", err)
		}
		if len(datasetIDs) == 0 {
			datasetIDs = extractDatasetIDsFromValue(raw)
		}
		if retrievalConfig == nil {
			retrievalConfig = extractRetrievalConfigFromValue(raw)
		}
	}
	return normalizeStringList(datasetIDs), retrievalConfig, nil
}

func extractDatasetIDsFromValue(raw interface{}) []string {
	switch typed := raw.(type) {
	case map[string]interface{}:
		return extractDatasetIDs(typed)
	case []interface{}:
		return stringsFromValue(typed)
	default:
		return nil
	}
}

func extractDatasetIDs(raw map[string]interface{}) []string {
	if len(raw) == 0 {
		return nil
	}
	candidates := [][]string{
		stringsFromValue(raw["dataset_ids"]),
		stringsFromValue(raw["datasetIds"]),
		stringsFromValue(raw["datasets"]),
		stringsFromValue(raw["knowledge_bases"]),
		stringsFromValue(raw["knowledgeBases"]),
		stringsFromValue(raw["knowledge_dataset_ids"]),
		stringsFromValue(raw["knowledgeDatasetIds"]),
	}
	if nested, ok := raw["datasets"].(map[string]interface{}); ok {
		candidates = append(candidates, stringsFromValue(nested["datasets"]), stringsFromValue(nested["dataset_ids"]), stringsFromValue(nested["datasetIds"]))
	}
	for _, ids := range candidates {
		if len(ids) > 0 {
			return normalizeStringList(ids)
		}
	}
	return nil
}

func extractRetrievalConfigFromValue(raw interface{}) map[string]interface{} {
	if typed, ok := raw.(map[string]interface{}); ok {
		return extractRetrievalConfig(typed)
	}
	return nil
}

func extractRetrievalConfig(raw map[string]interface{}) map[string]interface{} {
	for _, key := range []string{"retrieval_config", "retrieval_model_config", "retrieval_model", "knowledge_retrieval_config"} {
		if cfg, ok := raw[key].(map[string]interface{}); ok {
			return cfg
		}
	}
	return nil
}

func stringsFromValue(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			switch v := item.(type) {
			case string:
				out = append(out, v)
			case map[string]interface{}:
				if id, ok := v["id"].(string); ok {
					out = append(out, id)
				} else if id, ok := v["dataset_id"].(string); ok {
					out = append(out, id)
				} else if id, ok := v["datasetId"].(string); ok {
					out = append(out, id)
				}
			}
		}
		return out
	case map[string]interface{}:
		return stringsFromValue(typed["datasets"])
	default:
		return nil
	}
}

func knowledgeResourcesAndContext(records []scoredKnowledgeRecord, separator string, maxChars int, filesBaseURL string) ([]KnowledgeRetrieverResource, string, []KnowledgeContextBlock, error) {
	if separator == "" {
		separator = defaultKnowledgeContextSep
	}
	if maxChars <= 0 {
		maxChars = defaultKnowledgeMaxContextChar
	}
	resources := make([]KnowledgeRetrieverResource, 0, len(records))
	contextParts := make([]string, 0, len(records))
	contextBlocks := make([]KnowledgeContextBlock, 0, len(records))
	for i, scored := range records {
		record := scored.Record
		content, err := NormalizeKnowledgeImageURLs(record.Segment.Content, filesBaseURL)
		if err != nil {
			return nil, "", nil, err
		}
		resource := KnowledgeRetrieverResource{
			Position:        i + 1,
			DatasetID:       scored.DatasetID,
			DatasetName:     scored.DatasetName,
			DocumentID:      record.Segment.DocumentID,
			DocumentName:    record.Segment.Document.Name,
			DataSourceType:  record.Segment.Document.DataSourceType,
			SegmentID:       record.Segment.ID,
			Score:           record.Score,
			Content:         content,
			MatchType:       record.MatchType,
			RetrievalSource: record.RetrievalSource,
			DocMetadata:     record.Segment.Document.DocMetadata,
		}
		resources = append(resources, resource)
		if strings.TrimSpace(content) != "" {
			block := KnowledgeContextBlock{
				Position: resource.Position,
				Source:   knowledgeSourceLabel(resource),
				Score:    resource.Score,
			}
			remaining := maxChars - len(strings.Join(contextParts, separator))
			if len(contextParts) > 0 {
				remaining -= len(separator)
			}
			if remaining <= 0 {
				continue
			}
			contextBlockText := knowledgeContextBlockText(block, resource.MatchType, content, remaining)
			contextParts = append(contextParts, contextBlockText)
			contextBlocks = append(contextBlocks, block)
		}
	}
	return resources, strings.Join(contextParts, separator), contextBlocks, nil
}

func knowledgeRetrieveStatus(resources []KnowledgeRetrieverResource) string {
	if len(resources) == 0 {
		return KnowledgeRetrieveStatusNoResults
	}
	return KnowledgeRetrieveStatusSuccess
}

func knowledgeTopScore(resources []KnowledgeRetrieverResource) *float64 {
	if len(resources) == 0 {
		return nil
	}
	score := resources[0].Score
	return &score
}

func knowledgeSourceSummary(resources []KnowledgeRetrieverResource) []KnowledgeSourceSummary {
	if len(resources) == 0 {
		return nil
	}
	out := make([]KnowledgeSourceSummary, 0, len(resources))
	for _, resource := range resources {
		out = append(out, KnowledgeSourceSummary{
			Position:     resource.Position,
			DatasetName:  resource.DatasetName,
			DocumentName: resource.DocumentName,
			MatchType:    resource.MatchType,
			Score:        resource.Score,
		})
	}
	return out
}

func knowledgeSourceLabel(resource KnowledgeRetrieverResource) string {
	parts := make([]string, 0, 2)
	if strings.TrimSpace(resource.DatasetName) != "" {
		parts = append(parts, strings.TrimSpace(resource.DatasetName))
	}
	if strings.TrimSpace(resource.DocumentName) != "" && strings.TrimSpace(resource.DocumentName) != strings.TrimSpace(resource.DatasetName) {
		parts = append(parts, strings.TrimSpace(resource.DocumentName))
	}
	if len(parts) == 0 {
		return "Unknown source"
	}
	return strings.Join(parts, " / ")
}

func knowledgeContextBlockText(block KnowledgeContextBlock, matchType string, content string, maxChars int) string {
	headerParts := []string{
		"[" + strconv.Itoa(block.Position) + "] Source: " + block.Source,
	}
	if block.Score != 0 {
		headerParts = append(headerParts, "Score: "+strconv.FormatFloat(block.Score, 'f', 4, 64))
	}
	if strings.TrimSpace(matchType) != "" {
		headerParts = append(headerParts, "Match: "+strings.TrimSpace(matchType))
	}
	header := strings.Join(headerParts, "; ") + "\n"
	if maxChars <= 0 {
		return ""
	}
	if len(header) >= maxChars {
		return header[:maxChars]
	}
	available := maxChars - len(header)
	if len(content) > available {
		content = content[:available]
	}
	return header + content
}

func normalizeKnowledgeLimit(value int, defaultValue int, maxValue int) int {
	if value <= 0 {
		return defaultValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func normalizeStringList(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func normalizeRetrievalMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "vector", "vectorandbm25", "vector_and_bm25":
		// vectorAndBM25 is the public name for the textual Vector+BM25 route;
		// RetrievalService keeps the shorter internal mode for compatibility.
		return RetrievalModeVector
	case "graph":
		return RetrievalModeGraph
	case "hybrid":
		return RetrievalModeHybrid
	default:
		return RetrievalModeHybrid
	}
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (s *KnowledgeRetrievalService) checkAndUpdateRateLimit(ctx context.Context, workspaceID string) (bool, error) {
	knowledgeCfg := config.Current().Knowledge
	if !knowledgeCfg.RateLimitEnabled {
		return false, nil
	}
	client := redisutil.GetClient()
	if client == nil {
		return false, nil
	}
	key := fmt.Sprintf("rate_limit_%s", workspaceID)
	nowMs := time.Now().UnixMilli()
	member := fmt.Sprintf("%d_%s", nowMs, uuid.NewString())
	pipe := client.Pipeline()
	pipe.ZAdd(ctx, key, goRedis.Z{Score: float64(nowMs), Member: member})
	pipe.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", nowMs-knowledgeCfg.RateLimitWindowMS))
	countCmd := pipe.ZCard(ctx, key)
	pipe.Expire(ctx, key, time.Minute)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, err
	}
	return countCmd.Val() > knowledgeCfg.RateLimitMax, nil
}

func (s *KnowledgeRetrievalService) recordRateLimitLog(ctx context.Context, workspaceID string) error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.WithContext(ctx).Create(&shared_model.RateLimitLog{
		TenantID:         workspaceID,
		SubscriptionPlan: legacyKnowledgeRateLimitPlan,
		Operation:        "knowledge",
		CreatedAt:        time.Now(),
	}).Error
}

var _ = logger.Debug
