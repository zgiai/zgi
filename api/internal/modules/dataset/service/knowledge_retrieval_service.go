package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
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
	shared_model "github.com/zgiai/zgi/api/internal/modules/shared/model"
	"github.com/zgiai/zgi/api/pkg/embedding"
	"github.com/zgiai/zgi/api/pkg/logger"
	redisutil "github.com/zgiai/zgi/api/pkg/redis"
	"github.com/zgiai/zgi/api/pkg/tokenization"
	"github.com/zgiai/zgi/api/pkg/vectordb"
	"gorm.io/gorm"
)

const (
	defaultKnowledgeListLimit      = 20
	maxKnowledgeListLimit          = 100
	defaultKnowledgeTopK           = 5
	maxKnowledgeTopK               = 20
	defaultKnowledgeContextSep     = "\n"
	defaultKnowledgeMaxContextChar = 12000
	legacyKnowledgeRateLimitPlan   = "default"
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
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	Provider        string `json:"provider,omitempty"`
	EnableGraphFlow bool   `json:"enable_graph_flow"`
}

// KnowledgeRetrieveRequest describes a knowledge retrieval request.
type KnowledgeRetrieveRequest struct {
	Scope             KnowledgeScope
	Query             string
	DatasetIDs        []string
	AgentBindingGrant bool
	TopK              int
	RetrievalMode     string
	RetrievalConfig   map[string]interface{}
	ContextSeparator  string
	MaxContextChars   int
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

// KnowledgeRetrieveResponse is returned to builtin tools and skill callers.
type KnowledgeRetrieveResponse struct {
	Query           string                         `json:"query"`
	Context         string                         `json:"context"`
	Resources       []KnowledgeRetrieverResource   `json:"retriever_resources"`
	Records         []dto.HitTestingRecordResponse `json:"records,omitempty"`
	GraphExecutions []*dto.GraphExecution          `json:"graph_executions,omitempty"`
}

// KnowledgeRetrievalService wraps dataset-native retrieval for skill and agent use.
type KnowledgeRetrievalService struct {
	db               *gorm.DB
	datasetRepo      dataset_repository.DatasetRepository
	documentRepo     dataset_repository.DocumentRepository
	retrievalService *RetrievalService
	hitTesting       *hitTestingService
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
func (s *KnowledgeRetrievalService) ListAccessibleDatasets(ctx context.Context, scope KnowledgeScope, query string, limit int) ([]KnowledgeDatasetSummary, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("knowledge retrieval service is not configured")
	}
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
			return []KnowledgeDatasetSummary{}, nil
		}
		workspaceIDs = accessibleWorkspaceIDs
	} else {
		allowed, err := s.canAccessKnowledgeWorkspace(ctx, workspaceID, accountID)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return []KnowledgeDatasetSummary{}, nil
		}
	}

	limit = normalizeKnowledgeLimit(limit, defaultKnowledgeListLimit, maxKnowledgeListLimit)
	search := strings.TrimSpace(query)
	datasets, err := s.findAccessibleDatasets(ctx, workspaceIDs, search, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list accessible datasets: %w", err)
	}
	if len(datasets) == 0 && search != "" {
		datasets, err = s.findAccessibleDatasets(ctx, workspaceIDs, "", limit)
		if err != nil {
			return nil, fmt.Errorf("failed to list accessible datasets: %w", err)
		}
	}

	out := make([]KnowledgeDatasetSummary, 0, len(datasets))
	for _, dataset := range datasets {
		if dataset == nil {
			continue
		}
		out = append(out, KnowledgeDatasetSummary{
			DatasetID:       dataset.ID,
			Name:            dataset.Name,
			Description:     stringPtrValue(dataset.Description),
			Provider:        dataset.Provider,
			EnableGraphFlow: dataset.EnableGraphFlow,
		})
	}
	return out, nil
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
	scoredRecords := make([]scoredKnowledgeRecord, 0)
	graphExecutions := make([]*dto.GraphExecution, 0)
	for _, datasetID := range datasetIDs {
		dataset, err := s.accessibleKnowledgeDataset(ctx, datasetID, req.Scope, req.AgentBindingGrant)
		if err != nil {
			return nil, fmt.Errorf("dataset %s is not accessible: %w", datasetID, err)
		}
		datasetRecords, graphExecution, err := s.retrieveDataset(ctx, dataset, query, req.RetrievalConfig, topK, req.RetrievalMode)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve dataset %s: %w", datasetID, err)
		}
		for _, record := range datasetRecords {
			scoredRecords = append(scoredRecords, scoredKnowledgeRecord{
				Record:      record,
				DatasetID:   dataset.ID,
				DatasetName: dataset.Name,
			})
		}
		if graphExecution != nil {
			graphExecutions = append(graphExecutions, graphExecution)
		}
	}
	sort.SliceStable(scoredRecords, func(i, j int) bool {
		return scoredRecords[i].Record.Score > scoredRecords[j].Record.Score
	})
	if topK > 0 && len(scoredRecords) > topK {
		scoredRecords = scoredRecords[:topK]
	}
	records := make([]dto.HitTestingRecordResponse, 0, len(scoredRecords))
	for _, item := range scoredRecords {
		records = append(records, item.Record)
	}
	resources, contextText := knowledgeResourcesAndContext(scoredRecords, req.ContextSeparator, req.MaxContextChars)
	return &KnowledgeRetrieveResponse{
		Query:           query,
		Context:         contextText,
		Resources:       resources,
		Records:         records,
		GraphExecutions: graphExecutions,
	}, nil
}

// RetrieveAgentKnowledge retrieves using dataset ids configured on the agent.
func (s *KnowledgeRetrievalService) RetrieveAgentKnowledge(ctx context.Context, req KnowledgeRetrieveRequest) (*KnowledgeRetrieveResponse, error) {
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
		return nil, fmt.Errorf("agent has no configured knowledge datasets")
	}
	req.DatasetIDs = datasetIDs
	req.RetrievalConfig = retrievalConfig
	return s.Retrieve(ctx, req)
}

// ValidateAccessibleDatasets verifies that accountID can access every dataset in ids now.
func (s *KnowledgeRetrievalService) ValidateAccessibleDatasets(ctx context.Context, scope KnowledgeScope, ids []string) error {
	for _, datasetID := range normalizeStringList(ids) {
		if _, err := s.accessibleKnowledgeDataset(ctx, datasetID, scope, false); err != nil {
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

	var workspace struct {
		OrganizationID string `gorm:"column:organization_id"`
	}
	if err := s.db.WithContext(ctx).
		Table("workspaces").
		Select("organization_id").
		Where("id = ?", workspaceID).
		Take(&workspace).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to load workspace: %w", err)
	}

	var memberCount int64
	if err := s.db.WithContext(ctx).
		Table("workspace_members").
		Where("workspace_id = ? AND account_id = ?", workspaceID, accountID).
		Count(&memberCount).Error; err != nil {
		return false, fmt.Errorf("failed to check workspace membership: %w", err)
	}
	if memberCount > 0 {
		return true, nil
	}

	if strings.TrimSpace(workspace.OrganizationID) == "" {
		return false, nil
	}
	var adminCount int64
	if err := s.db.WithContext(ctx).
		Table("members").
		Where("organization_id = ? AND account_id = ? AND role IN ?", workspace.OrganizationID, accountID, []string{"owner", "admin"}).
		Count(&adminCount).Error; err != nil {
		return false, fmt.Errorf("failed to check organization membership: %w", err)
	}
	return adminCount > 0, nil
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
		Where("workspace_id IN ?", workspaceIDs)
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

func (s *KnowledgeRetrievalService) accessibleKnowledgeWorkspaceIDs(ctx context.Context, organizationID, workspaceID, accountID string) ([]string, error) {
	organizationID = strings.TrimSpace(organizationID)
	workspaceID = strings.TrimSpace(workspaceID)
	accountID = strings.TrimSpace(accountID)
	if organizationID == "" || accountID == "" {
		return nil, nil
	}

	query := s.db.WithContext(ctx).
		Table("workspaces").
		Select("workspaces.id").
		Where("workspaces.organization_id = ?", organizationID)
	if workspaceID != "" {
		query = query.Where("workspaces.id = ?", workspaceID)
	}

	var adminCount int64
	if err := s.db.WithContext(ctx).
		Table("members").
		Where("organization_id = ? AND account_id = ? AND role IN ?", organizationID, accountID, []string{"owner", "admin"}).
		Count(&adminCount).Error; err != nil {
		return nil, fmt.Errorf("failed to check organization membership: %w", err)
	}
	if adminCount == 0 {
		query = query.Joins("JOIN workspace_members ON workspace_members.workspace_id = workspaces.id").
			Where("workspace_members.account_id = ?", accountID)
	}

	var workspaceIDs []string
	if err := query.
		Order("workspaces.created_at DESC, workspaces.id DESC").
		Pluck("workspaces.id", &workspaceIDs).Error; err != nil {
		return nil, fmt.Errorf("failed to list accessible workspaces: %w", err)
	}
	return normalizeStringList(workspaceIDs), nil
}

func (s *KnowledgeRetrievalService) accessibleKnowledgeDataset(ctx context.Context, datasetID string, scope KnowledgeScope, agentBindingGrant bool) (*dataset_model.Dataset, error) {
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
	return &dataset, nil
}

func knowledgeRateLimitScopeID(scope KnowledgeScope) string {
	if organizationID := strings.TrimSpace(scope.OrganizationID); organizationID != "" {
		return organizationID
	}
	return strings.TrimSpace(scope.WorkspaceID)
}

func (s *KnowledgeRetrievalService) retrieveDataset(ctx context.Context, dataset *dataset_model.Dataset, query string, retrievalConfig map[string]interface{}, topK int, retrievalMode string) ([]dto.HitTestingRecordResponse, *dto.GraphExecution, error) {
	if dataset.Provider == "external" {
		response, err := s.hitTesting.ExternalRetrieve(ctx, dataset, query, "", retrievalConfig)
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
	options := s.hitTesting.getRetrievalOptions(ctx, retrievalConfig, dataset)
	options.TopK = topK
	options.RetrievalMode = normalizeRetrievalMode(retrievalMode)
	records, graphExecution, err := s.retrievalService.Retrieve(ctx, dataset, query, options)
	if err != nil {
		return nil, nil, err
	}
	return records, graphExecution, nil
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

func knowledgeResourcesAndContext(records []scoredKnowledgeRecord, separator string, maxChars int) ([]KnowledgeRetrieverResource, string) {
	if separator == "" {
		separator = defaultKnowledgeContextSep
	}
	if maxChars <= 0 {
		maxChars = defaultKnowledgeMaxContextChar
	}
	resources := make([]KnowledgeRetrieverResource, 0, len(records))
	contextParts := make([]string, 0, len(records))
	for i, scored := range records {
		record := scored.Record
		content := record.Segment.Content
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
			contextParts = append(contextParts, content)
		}
	}
	contextText := ""
	for _, part := range contextParts {
		if contextText == "" {
			contextText = part
		} else {
			contextText += separator + part
		}
		if len(contextText) >= maxChars {
			if len(contextText) > maxChars {
				contextText = contextText[:maxChars]
			}
			break
		}
	}
	return resources, contextText
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
	case "vector", "graph", "hybrid":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "hybrid"
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
