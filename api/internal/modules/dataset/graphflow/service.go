package graphflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/extractor"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/graph"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/repository"
	dataset_repo "github.com/zgiai/zgi/api/internal/modules/dataset/repository"
	"github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	llmruntime "github.com/zgiai/zgi/api/internal/modules/llm/runtime"
	shared_model "github.com/zgiai/zgi/api/internal/modules/shared/model"
	"github.com/zgiai/zgi/api/pkg/embedding"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/queue"
	"github.com/zgiai/zgi/api/pkg/vectordb"
	"gorm.io/gorm"
)

// Service provides GraphFlow functionality
type Service struct {
	// Configuration
	cfg *config.Config
	DB  *gorm.DB

	// LLM Client for Gateway Access
	llmClient client.LLMClient

	// Default model resolver
	DefaultModelSvc llmdefaultservice.DefaultModelService

	// Repositories
	TaskRepo           *repository.GraphFlowTaskRepository
	RunRepo            *repository.GraphFlowRunRepository
	Lifecycle          *LifecycleService
	EntityMentionRepo  *repository.EntityMentionRepository
	TripleMentionRepo  *repository.TripleMentionRepository
	EntityRepo         *repository.EntityRepository
	RelationshipRepo   *repository.RelationshipRepository
	TypeDefinitionRepo *repository.TypeDefinitionRepository
	DocumentRepo       dataset_repo.DocumentRepository
	DatasetRepo        dataset_repo.DatasetRepository

	// Extractors
	ExtractorFactory func(strategy string) extractor.Extractor

	// Graph Client

	// Graph Client
	Neo4jClient    *graph.Neo4jClient
	WeaviateClient *vectordb.WeaviateClient

	// Task Manager
	TaskManager *queue.TaskManager
}

// NewService creates a new GraphFlow service with all dependencies
func NewService(
	cfg *config.Config,
	db *gorm.DB,
	documentRepo dataset_repo.DocumentRepository,
	datasetRepo dataset_repo.DatasetRepository,
	llmClient client.LLMClient,
	defaultModelSvc llmdefaultservice.DefaultModelService,
	taskManager *queue.TaskManager,
) *Service {
	// Initialize repositories
	taskRepo := repository.NewGraphFlowTaskRepository(db)
	runRepo := repository.NewGraphFlowRunRepository(db)
	entityMentionRepo := repository.NewEntityMentionRepository(db)
	tripleMentionRepo := repository.NewTripleMentionRepository(db)
	entityRepo := repository.NewEntityRepository(db)
	relationshipRepo := repository.NewRelationshipRepository(db)
	typeDefinitionRepo := repository.NewTypeDefinitionRepository(db)
	lifecycle := NewLifecycleService(db)
	taskRepo.SetChangeHook(lifecycle.ReconcileTask)

	// Initialize Neo4j client (optional, may be nil if not configured)
	var neo4jClient *graph.Neo4jClient
	if cfg.Neo4j.URI != "" {
		neo4jClient = graph.NewNeo4jClient(cfg.Neo4j.URI, cfg.Neo4j.Username, cfg.Neo4j.Password, cfg.Neo4j.Database)
	}

	// Initialize Weaviate client if configured
	var weaviateClient *vectordb.WeaviateClient
	if cfg.VectorStore.WeaviateEndpoint != "" {
		weaviateClient = vectordb.NewWeaviateClient(&cfg.VectorStore)
	}

	svc := &Service{
		cfg:                cfg,
		DB:                 db,
		llmClient:          llmClient,
		DefaultModelSvc:    defaultModelSvc,
		TaskRepo:           taskRepo,
		RunRepo:            runRepo,
		Lifecycle:          lifecycle,
		EntityMentionRepo:  entityMentionRepo,
		TripleMentionRepo:  tripleMentionRepo,
		EntityRepo:         entityRepo,
		RelationshipRepo:   relationshipRepo,
		TypeDefinitionRepo: typeDefinitionRepo,
		DocumentRepo:       documentRepo,
		DatasetRepo:        datasetRepo,
		Neo4jClient:        neo4jClient,
		TaskManager:        taskManager,
		WeaviateClient:     weaviateClient,
	}

	// Initialize Extractor factory with LLM Client
	svc.ExtractorFactory = func(strategy string) extractor.Extractor {
		return extractor.NewExtractorByStrategy(strategy, llmClient, defaultModelSvc, nil, nil)
	}

	return svc
}

func (s *Service) DeleteDatasetProjection(ctx context.Context, datasetID uuid.UUID) error {
	if s == nil || datasetID == uuid.Nil {
		return fmt.Errorf("graph dataset projection scope is required")
	}
	var cleanupErrors []error
	if s.Neo4jClient == nil {
		cleanupErrors = append(cleanupErrors, fmt.Errorf("neo4j client not configured"))
	} else if err := s.Neo4jClient.DeleteDataset(ctx, datasetID.String()); err != nil {
		cleanupErrors = append(cleanupErrors, err)
	}
	if s.WeaviateClient != nil {
		if err := s.WeaviateClient.DeleteClass(ctx, fmt.Sprintf("Entity_%s", datasetID)); err != nil {
			cleanupErrors = append(cleanupErrors, err)
		}
	}
	return errors.Join(cleanupErrors...)
}

// GetExtractor returns an extractor based on the strategy and custom model settings
func (s *Service) GetExtractor(strategy string, model *string, provider *string) extractor.Extractor {
	return extractor.NewExtractorByStrategy(strategy, s.llmClient, s.DefaultModelSvc, model, provider)
}

// GetLLMClient returns the LLM client
func (s *Service) GetLLMClient() client.LLMClient {
	return s.llmClient
}

// GetConfig returns the configuration
func (s *Service) GetConfig() *config.Config {
	return s.cfg
}

// ExtractQueryEntities extracts named entities from a search query using LLM.
// The LLM gateway resolves channels by organization, not workspace.
func (s *Service) ExtractQueryEntities(ctx context.Context, organizationID string, query string, model *string, provider *string) ([]string, error) {
	promptText, err := renderQueryEntityExtractionPrompt(query)
	if err != nil {
		return nil, fmt.Errorf("failed to render query entity extraction prompt: %w", err)
	}

	msgs := []adapter.Message{
		{Role: "user", Content: promptText},
	}
	// Use a low temperature for extraction
	temp := 0.1
	resolvedModel, err := llmruntime.NewModelResolver(s.DefaultModelSvc).ResolveFromPointers(ctx, organizationID, provider, model, shared_model.ModelTypeLLM)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve text model: %w", err)
	}
	if resolvedModel == nil || strings.TrimSpace(resolvedModel.Model) == "" {
		return nil, fmt.Errorf("default text model is not configured")
	}

	req := adapter.ChatRequest{
		Provider:    strings.TrimSpace(resolvedModel.Provider),
		Model:       strings.TrimSpace(resolvedModel.Model),
		Messages:    msgs,
		Temperature: &temp,
		ResponseFormat: &adapter.ResponseFormat{
			Type: "json_object",
		},
	}

	// 2. Call LLM with a 120s timeout to avoid blocking retrieval (increased from 10s)
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	resp, err := s.llmClient.Chat(ctx, organizationID, &req)
	if err != nil {
		return nil, fmt.Errorf("LLM extraction failed: %w", err)
	}

	// 3. Parse result
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices returned from LLM")
	}
	content, ok := resp.Choices[0].Message.Content.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected LLM response content type")
	}

	// Clean up potential markdown code blocks
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var result struct {
		Entities []string `json:"entities"`
	}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w (content: %s)", err, content)
	}

	return result.Entities, nil
}

// GetEmbeddingService returns an embedding service for the given dataset
func (s *Service) GetEmbeddingService(ctx context.Context, datasetID string) (embedding.EmbeddingService, error) {
	// 1. Get dataset to check for custom model config
	dataset, err := s.DatasetRepo.GetByID(ctx, datasetID)
	if err != nil {
		return nil, fmt.Errorf("failed to get dataset: %w", err)
	}

	if s.DefaultModelSvc == nil {
		return nil, fmt.Errorf("default model resolver not initialized")
	}

	resolvedModel, err := llmruntime.NewModelResolver(s.DefaultModelSvc).ResolveFromPointers(
		ctx,
		dataset.OrganizationID,
		dataset.EmbeddingModelProvider,
		dataset.EmbeddingModel,
		shared_model.ModelTypeEmbedding,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve embedding model: %w", err)
	}

	accountID := dataset.CreatedBy
	if accountID == "" {
		accountID = dataset.WorkspaceID
	}

	return llmruntime.NewGatewayEmbeddingService(s.llmClient, accountID, dataset.ID, "dataset", resolvedModel.Model, dataset.WorkspaceID)
}

// SegmentDetails holds content and matched entities for a segment
type SegmentDetails struct {
	SegmentID        string
	IndexNodeID      string
	DocumentID       string
	Position         int
	Content          string
	MatchedEntities  []string
	MatchedEntityIDs []string
}

// FindSegmentsByEntities finds document segments that mention the given entities
// FindSegmentsByEntities finds document segments that mention the given entities (by Name)
func (s *Service) FindSegmentsByEntities(ctx context.Context, datasetID string, entities []string, params map[string]interface{}) (map[string]*SegmentDetails, error) {
	if len(entities) == 0 {
		return nil, nil
	}

	kbID, err := uuid.Parse(datasetID)
	if err != nil {
		return nil, fmt.Errorf("invalid dataset ID format: %w", err)
	}

	mentions, err := s.EntityMentionRepo.FindMentionsByEntityNames(ctx, kbID, entities)
	if err != nil {
		return nil, fmt.Errorf("failed to find mentions by names: %w", err)
	}

	return s.processMentionsToSegments(ctx, mentions)
}

// FindSegmentsByEntityIDs finds document segments that mention the given entities (by UUID)
func (s *Service) FindSegmentsByEntityIDs(ctx context.Context, datasetID string, entityIDs []uuid.UUID, params map[string]interface{}) (map[string]*SegmentDetails, error) {
	if len(entityIDs) == 0 {
		return nil, nil
	}

	kbID, err := uuid.Parse(datasetID)
	if err != nil {
		return nil, fmt.Errorf("invalid dataset ID format: %w", err)
	}

	mentions, err := s.EntityMentionRepo.FindMentionsByEntityIDs(ctx, kbID, entityIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to find mentions by IDs: %w", err)
	}

	return s.processMentionsToSegments(ctx, mentions)
}

// processMentionsToSegments groups mentions by segment and fetches segment content
func (s *Service) processMentionsToSegments(ctx context.Context, mentions []*model.EntityMention) (map[string]*SegmentDetails, error) {
	if len(mentions) == 0 {
		return make(map[string]*SegmentDetails), nil
	}

	segmentMap := make(map[string]*SegmentDetails)
	var segmentIDs []string

	for _, m := range mentions {
		segIDStr := m.SegmentID.String()
		if _, exists := segmentMap[segIDStr]; !exists {
			segmentMap[segIDStr] = &SegmentDetails{
				SegmentID:        segIDStr,
				MatchedEntities:  []string{},
				MatchedEntityIDs: []string{},
			}
			segmentIDs = append(segmentIDs, segIDStr)
		}
		segmentMap[segIDStr].MatchedEntities = append(segmentMap[segIDStr].MatchedEntities, m.RawName)
		if m.EntityID != nil {
			segmentMap[segIDStr].MatchedEntityIDs = append(segmentMap[segIDStr].MatchedEntityIDs, m.EntityID.String())
		}
	}

	if len(segmentIDs) > 0 {
		segments, err := s.DocumentRepo.GetSegmentsByIDs(ctx, segmentIDs)
		if err != nil {
			logger.Warn("Failed to fetch segment contents", map[string]interface{}{"error": err})
		} else {
			foundSegmentIDs := make(map[string]bool, len(segments))
			for _, seg := range segments {
				if !seg.Enabled {
					continue
				}
				foundSegmentIDs[seg.ID] = true
				if detail, ok := segmentMap[seg.ID]; ok {
					detail.Content = seg.Content
					detail.DocumentID = seg.DocumentID
					detail.Position = seg.Position
					if seg.IndexNodeID != nil {
						detail.IndexNodeID = *seg.IndexNodeID
					}
				}
			}
			for segmentID := range segmentMap {
				if !foundSegmentIDs[segmentID] {
					delete(segmentMap, segmentID)
				}
			}
		}
	}

	return segmentMap, nil
}

// GetGraphData returns the complete knowledge graph for frontend visualization
func (s *Service) GetGraphData(ctx context.Context, datasetID string) (*model.GraphDataResponse, error) {
	kbUUID, err := uuid.Parse(datasetID)
	if err != nil {
		return nil, fmt.Errorf("invalid dataset ID: %w", err)
	}

	// 1. Fetch all entities for this dataset
	entities, err := s.EntityRepo.FindByKBID(ctx, kbUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch entities: %w", err)
	}

	// 2. Fetch all relationships for this dataset
	relationships, err := s.RelationshipRepo.FindByKBID(ctx, kbUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch relationships: %w", err)
	}

	// 3. Fetch type definitions for multi-language labels
	typeDefs, err := s.TypeDefinitionRepo.GetTypeKeyMap(ctx, kbUUID)
	if err != nil {
		logger.Warn("Failed to fetch type definitions, using fallback", map[string]interface{}{"error": err})
		typeDefs = make(map[string]*model.TypeDefinition)
	}

	// 4. Build entity ID -> entity map for relationship resolution
	entityMap := make(map[uuid.UUID]*model.Entity)
	for _, ent := range entities {
		if ent.IsDeleted || ent.ActiveSourceCount <= 0 {
			continue
		}
		entityMap[ent.ID] = ent
	}

	// 5. Fetch entity-document associations (via mentions)
	// Group mentions by entity ID to calculate source weights
	entitySources := make(map[uuid.UUID]map[string]int) // entity ID -> doc ID -> count
	mentionsByKB, err := s.EntityMentionRepo.GetByKBID(ctx, kbUUID.String())
	if err != nil {
		logger.Warn("Failed to fetch mentions for source calculation", map[string]interface{}{"error": err})
		mentionsByKB = nil
	}

	// Collect unique segment IDs from mentions for batch fetching
	segmentIDSet := make(map[string]bool)
	for _, mention := range mentionsByKB {
		if mention.EntityID != nil {
			segmentIDSet[mention.SegmentID.String()] = true
		}
	}

	// Batch fetch all segments
	var segmentIDs []string
	for segID := range segmentIDSet {
		segmentIDs = append(segmentIDs, segID)
	}

	segmentDocMap := make(map[string]string) // segment ID -> document ID
	if len(segmentIDs) > 0 {
		segments, err := s.DocumentRepo.GetSegmentsByIDs(ctx, segmentIDs)
		if err != nil {
			logger.Warn("Failed to fetch segments for source calculation", map[string]interface{}{"error": err})
		} else {
			for _, seg := range segments {
				segmentDocMap[seg.ID] = seg.DocumentID
			}
		}
	}

	// Build entity sources map
	for _, mention := range mentionsByKB {
		if mention.EntityID == nil {
			continue
		}
		entityID := *mention.EntityID
		if _, exists := entitySources[entityID]; !exists {
			entitySources[entityID] = make(map[string]int)
		}
		if docID, exists := segmentDocMap[mention.SegmentID.String()]; exists {
			entitySources[entityID][docID]++
		}
	}

	// 6. Fetch document info for sources (batch fetch)
	docIDSet := make(map[string]bool)
	for _, docs := range entitySources {
		for docID := range docs {
			docIDSet[docID] = true
		}
	}

	docInfoCache := make(map[string]string) // doc ID -> doc name
	docEnabledCache := make(map[string]bool)
	for docID := range docIDSet {
		if doc, err := s.DocumentRepo.GetByID(ctx, docID); err == nil && doc != nil {
			docInfoCache[docID] = doc.Name
			docEnabledCache[docID] = doc.Enabled
		}
	}

	// 7. Build nodes
	nodes := make([]model.GraphNode, 0, len(entities))
	categorySet := make(map[string]bool)
	for _, entity := range entities {
		if _, active := entityMap[entity.ID]; !active {
			continue
		}
		nodeID := fmt.Sprintf("ent:%s", entity.ID.String())

		// Build sources array
		var sources []model.GraphNodeSource
		if entityDocs, exists := entitySources[entity.ID]; exists {
			for docID, weight := range entityDocs {
				if !docEnabledCache[docID] {
					continue
				}
				docTitle := docID // Fallback to ID
				if name, cached := docInfoCache[docID]; cached {
					docTitle = name
				}
				sources = append(sources, model.GraphNodeSource{
					Doc: model.GraphSourceDoc{
						ID:    fmt.Sprintf("doc:%s", docID),
						Title: docTitle,
					},
					Weight: weight,
				})
			}
		}

		node := model.GraphNode{
			ID:       nodeID,
			Label:    entity.Name,
			Category: entity.Type,
			Data: model.GraphNodeData{
				Description:       entity.Description,
				Sources:           sources,
				SourceCount:       entity.SourceCount,
				ActiveSourceCount: entity.ActiveSourceCount,
			},
		}
		nodes = append(nodes, node)
		categorySet[entity.Type] = true
	}

	// 8. Build edges
	edges := make([]model.GraphEdge, 0, len(relationships))
	for _, rel := range relationships {
		if rel.IsDeleted || rel.ActiveWeight <= 0 {
			continue
		}
		// Only include edges where both nodes exist
		if _, headExists := entityMap[rel.HeadEntityID]; !headExists {
			continue
		}
		if _, tailExists := entityMap[rel.TailEntityID]; !tailExists {
			continue
		}

		edge := model.GraphEdge{
			Source:       fmt.Sprintf("ent:%s", rel.HeadEntityID.String()),
			Target:       fmt.Sprintf("ent:%s", rel.TailEntityID.String()),
			Label:        rel.RelationType,
			Weight:       rel.Weight,
			ActiveWeight: rel.ActiveWeight,
		}
		edges = append(edges, edge)
	}

	// 9. Build categories with multi-language labels
	categories := make([]model.GraphCategory, 0, len(categorySet))
	for typeKey := range categorySet {
		zhLabel := typeKey
		enLabel := typeKey

		// Get translated labels from type definitions
		if typeDef, exists := typeDefs[typeKey]; exists {
			if typeDef.LabelZh != nil && *typeDef.LabelZh != "" {
				zhLabel = *typeDef.LabelZh
			}
			if typeDef.LabelEn != nil && *typeDef.LabelEn != "" {
				enLabel = *typeDef.LabelEn
			}
		}

		categories = append(categories, model.GraphCategory{
			ID: typeKey,
			Label: model.GraphCategoryLabel{
				ZhHans: zhLabel,
				EnUS:   enLabel,
			},
		})
	}

	return &model.GraphDataResponse{
		Nodes:      nodes,
		Edges:      edges,
		Categories: categories,
		NodeCount:  len(nodes),
		EdgeCount:  len(edges),
		TotalNodes: len(nodes),
		TotalEdges: len(edges),
	}, nil
}

const (
	defaultGraphNodeLimit = 300
	defaultGraphEdgeLimit = 900
	maxGraphNodeLimit     = 500
	maxGraphEdgeLimit     = 1500
	maxGraphHopDepth      = 2
)

func (s *Service) QueryGraphData(ctx context.Context, datasetID string, query model.GraphQuery) (*model.GraphDataResponse, error) {
	graphData, err := s.GetGraphData(ctx, datasetID)
	if err != nil {
		return nil, err
	}
	return applyGraphQuery(graphData, query)
}

func applyGraphQuery(graph *model.GraphDataResponse, query model.GraphQuery) (*model.GraphDataResponse, error) {
	query = normalizeGraphQuery(query)
	isFullOverview := query.Overview &&
		strings.TrimSpace(query.Keyword) == "" &&
		strings.TrimSpace(query.Category) == "" &&
		strings.TrimSpace(query.DocumentID) == "" &&
		strings.TrimSpace(query.SeedNodeID) == "" &&
		strings.TrimSpace(query.Cursor) == ""
	if query.HopDepth > maxGraphHopDepth ||
		(!isFullOverview && (query.NodeLimit > maxGraphNodeLimit || query.EdgeLimit > maxGraphEdgeLimit)) {
		return nil, fmt.Errorf("graph query limit exceeded")
	}
	if graph == nil {
		return &model.GraphDataResponse{
			Nodes:      []model.GraphNode{},
			Edges:      []model.GraphEdge{},
			Categories: []model.GraphCategory{},
		}, nil
	}

	nodesByID := make(map[string]model.GraphNode, len(graph.Nodes))
	eligible := make(map[string]bool, len(graph.Nodes))
	keyword := strings.ToLower(strings.TrimSpace(query.Keyword))
	category := strings.TrimSpace(query.Category)
	documentID := strings.TrimPrefix(strings.TrimSpace(query.DocumentID), "doc:")
	for _, node := range graph.Nodes {
		if node.Data.ActiveSourceCount <= 0 {
			continue
		}
		if keyword != "" && !strings.Contains(strings.ToLower(node.Label+" "+node.Data.Description), keyword) {
			continue
		}
		if category != "" && node.Category != category {
			continue
		}
		if documentID != "" && !graphNodeHasDocument(node, documentID) {
			continue
		}
		nodesByID[node.ID] = node
		eligible[node.ID] = true
	}

	activeEdges := make([]model.GraphEdge, 0, len(graph.Edges))
	for _, edge := range graph.Edges {
		if edge.ActiveWeight <= 0 || !eligible[edge.Source] || !eligible[edge.Target] {
			continue
		}
		activeEdges = append(activeEdges, edge)
	}
	if query.SeedNodeID != "" {
		reachable := graphReachableNodes(query.SeedNodeID, query.HopDepth, activeEdges)
		for nodeID := range eligible {
			if !reachable[nodeID] {
				delete(eligible, nodeID)
				delete(nodesByID, nodeID)
			}
		}
	}

	filteredEdges := make([]model.GraphEdge, 0, len(activeEdges))
	for _, edge := range activeEdges {
		if eligible[edge.Source] && eligible[edge.Target] {
			filteredEdges = append(filteredEdges, edge)
		}
	}

	totalNodeCount := len(nodesByID)
	totalEdgeCount := len(filteredEdges)
	ids := make([]string, 0, min(totalNodeCount, query.NodeLimit))
	hasMore := false
	switch {
	case query.Cursor != "":
		for nodeID := range nodesByID {
			if nodeID > query.Cursor {
				ids = append(ids, nodeID)
			}
		}
		sort.Strings(ids)
		hasMore = len(ids) > query.NodeLimit
		if hasMore {
			ids = ids[:query.NodeLimit]
		}
	case query.SeedNodeID != "":
		ids = selectNeighborhoodNodeIDs(
			query.SeedNodeID,
			nodesByID,
			filteredEdges,
			query.NodeLimit,
		)
		hasMore = len(ids) < totalNodeCount
	case query.Overview:
		ids = selectOverviewNodeIDs(nodesByID, filteredEdges, len(nodesByID))
		if len(ids) > query.NodeLimit {
			ids = ids[:query.NodeLimit]
		}
		hasMore = len(ids) < totalNodeCount
	case keyword != "" || category != "" || documentID != "":
		ids = selectRankedNodeIDs(nodesByID, filteredEdges, query.NodeLimit)
		hasMore = len(ids) < totalNodeCount
	default:
		for nodeID := range nodesByID {
			ids = append(ids, nodeID)
		}
		sort.Strings(ids)
		hasMore = len(ids) > query.NodeLimit
		if hasMore {
			ids = ids[:query.NodeLimit]
		}
	}

	pageSet := make(map[string]bool, len(ids))
	nodes := make([]model.GraphNode, 0, len(ids))
	for _, nodeID := range ids {
		pageSet[nodeID] = true
		nodes = append(nodes, nodesByID[nodeID])
	}

	edges := make([]model.GraphEdge, 0)
	for _, edge := range filteredEdges {
		if pageSet[edge.Source] && pageSet[edge.Target] {
			edges = append(edges, edge)
		}
	}
	sort.Slice(edges, func(i, j int) bool {
		if query.Overview || query.SeedNodeID != "" {
			if edges[i].ActiveWeight != edges[j].ActiveWeight {
				return edges[i].ActiveWeight > edges[j].ActiveWeight
			}
		}
		left := edges[i].Source + "\x00" + edges[i].Target + "\x00" + edges[i].Label
		right := edges[j].Source + "\x00" + edges[j].Target + "\x00" + edges[j].Label
		return left < right
	})
	if len(edges) > query.EdgeLimit {
		edges = edges[:query.EdgeLimit]
	}

	categorySet := make(map[string]bool)
	for _, node := range nodes {
		categorySet[node.Category] = true
	}
	categories := make([]model.GraphCategory, 0)
	for _, item := range graph.Categories {
		if categorySet[item.ID] {
			categories = append(categories, item)
		}
	}
	sort.Slice(categories, func(i, j int) bool { return categories[i].ID < categories[j].ID })

	result := &model.GraphDataResponse{
		Nodes:      nodes,
		Edges:      edges,
		Categories: categories,
		NodeCount:  len(nodes),
		EdgeCount:  len(edges),
		TotalNodes: totalNodeCount,
		TotalEdges: totalEdgeCount,
	}
	if hasMore && len(ids) > 0 && !query.Overview && query.SeedNodeID == "" && keyword == "" && category == "" && documentID == "" {
		result.NextCursor = ids[len(ids)-1]
	}
	return result, nil
}

func selectOverviewNodeIDs(
	nodesByID map[string]model.GraphNode,
	edges []model.GraphEdge,
	limit int,
) []string {
	if limit <= 0 || len(nodesByID) == 0 {
		return []string{}
	}
	sortedEdges := append([]model.GraphEdge(nil), edges...)
	sort.Slice(sortedEdges, func(i, j int) bool {
		if sortedEdges[i].ActiveWeight != sortedEdges[j].ActiveWeight {
			return sortedEdges[i].ActiveWeight > sortedEdges[j].ActiveWeight
		}
		leftEvidence := nodesByID[sortedEdges[i].Source].Data.ActiveSourceCount +
			nodesByID[sortedEdges[i].Target].Data.ActiveSourceCount
		rightEvidence := nodesByID[sortedEdges[j].Source].Data.ActiveSourceCount +
			nodesByID[sortedEdges[j].Target].Data.ActiveSourceCount
		if leftEvidence != rightEvidence {
			return leftEvidence > rightEvidence
		}
		left := sortedEdges[i].Source + "\x00" + sortedEdges[i].Target + "\x00" + sortedEdges[i].Label
		right := sortedEdges[j].Source + "\x00" + sortedEdges[j].Target + "\x00" + sortedEdges[j].Label
		return left < right
	})

	selected := make(map[string]bool, min(len(nodesByID), limit))
	ids := make([]string, 0, min(len(nodesByID), limit))
	addNode := func(nodeID string) bool {
		if selected[nodeID] || len(ids) >= limit {
			return false
		}
		selected[nodeID] = true
		ids = append(ids, nodeID)
		return true
	}
	for _, edge := range sortedEdges {
		missing := 0
		if !selected[edge.Source] {
			missing++
		}
		if !selected[edge.Target] {
			missing++
		}
		if missing > limit-len(ids) {
			continue
		}
		addNode(edge.Source)
		addNode(edge.Target)
		if len(ids) >= limit {
			return ids
		}
	}

	for _, nodeID := range selectRankedNodeIDs(nodesByID, edges, limit) {
		addNode(nodeID)
		if len(ids) >= limit {
			break
		}
	}
	return ids
}

func selectNeighborhoodNodeIDs(
	seedNodeID string,
	nodesByID map[string]model.GraphNode,
	edges []model.GraphEdge,
	limit int,
) []string {
	if limit <= 0 {
		return []string{}
	}
	if _, exists := nodesByID[seedNodeID]; !exists {
		return []string{}
	}

	selected := map[string]bool{seedNodeID: true}
	ids := []string{seedNodeID}
	for len(ids) < limit {
		bestNodeID := ""
		bestWeight := -1
		bestEvidence := -1
		for _, edge := range edges {
			var candidate string
			switch {
			case selected[edge.Source] && !selected[edge.Target]:
				candidate = edge.Target
			case selected[edge.Target] && !selected[edge.Source]:
				candidate = edge.Source
			default:
				continue
			}
			evidence := nodesByID[candidate].Data.ActiveSourceCount
			if edge.ActiveWeight > bestWeight ||
				(edge.ActiveWeight == bestWeight && evidence > bestEvidence) ||
				(edge.ActiveWeight == bestWeight && evidence == bestEvidence &&
					(bestNodeID == "" || candidate < bestNodeID)) {
				bestNodeID = candidate
				bestWeight = edge.ActiveWeight
				bestEvidence = evidence
			}
		}
		if bestNodeID == "" {
			break
		}
		selected[bestNodeID] = true
		ids = append(ids, bestNodeID)
	}
	return ids
}

func selectRankedNodeIDs(
	nodesByID map[string]model.GraphNode,
	edges []model.GraphEdge,
	limit int,
) []string {
	if limit <= 0 {
		return []string{}
	}
	degrees := make(map[string]int, len(nodesByID))
	for _, edge := range edges {
		degrees[edge.Source] += edge.ActiveWeight
		degrees[edge.Target] += edge.ActiveWeight
	}
	ids := make([]string, 0, len(nodesByID))
	for nodeID := range nodesByID {
		ids = append(ids, nodeID)
	}
	sort.Slice(ids, func(i, j int) bool {
		left := nodesByID[ids[i]]
		right := nodesByID[ids[j]]
		if left.Data.ActiveSourceCount != right.Data.ActiveSourceCount {
			return left.Data.ActiveSourceCount > right.Data.ActiveSourceCount
		}
		if degrees[ids[i]] != degrees[ids[j]] {
			return degrees[ids[i]] > degrees[ids[j]]
		}
		if left.Label != right.Label {
			return left.Label < right.Label
		}
		return ids[i] < ids[j]
	})
	if len(ids) > limit {
		ids = ids[:limit]
	}
	return ids
}

func normalizeGraphQuery(query model.GraphQuery) model.GraphQuery {
	if query.NodeLimit <= 0 {
		query.NodeLimit = defaultGraphNodeLimit
	}
	if query.EdgeLimit <= 0 {
		query.EdgeLimit = defaultGraphEdgeLimit
	}
	if query.HopDepth < 0 {
		query.HopDepth = 0
	}
	if query.SeedNodeID != "" && query.HopDepth == 0 {
		query.HopDepth = 1
	}
	return query
}

func graphNodeHasDocument(node model.GraphNode, documentID string) bool {
	for _, source := range node.Data.Sources {
		if strings.TrimPrefix(source.Doc.ID, "doc:") == documentID {
			return true
		}
	}
	return false
}

func graphReachableNodes(seedNodeID string, hopDepth int, edges []model.GraphEdge) map[string]bool {
	reachable := map[string]bool{seedNodeID: true}
	frontier := map[string]bool{seedNodeID: true}
	for hop := 0; hop < hopDepth; hop++ {
		next := make(map[string]bool)
		for _, edge := range edges {
			if frontier[edge.Source] && !reachable[edge.Target] {
				next[edge.Target] = true
			}
			if frontier[edge.Target] && !reachable[edge.Source] {
				next[edge.Source] = true
			}
		}
		for nodeID := range next {
			reachable[nodeID] = true
		}
		frontier = next
	}
	return reachable
}
