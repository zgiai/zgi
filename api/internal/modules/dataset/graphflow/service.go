package graphflow

import (
	"context"
	"encoding/json"
	"fmt"
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
	entityMentionRepo := repository.NewEntityMentionRepository(db)
	tripleMentionRepo := repository.NewTripleMentionRepository(db)
	entityRepo := repository.NewEntityRepository(db)
	relationshipRepo := repository.NewRelationshipRepository(db)
	typeDefinitionRepo := repository.NewTypeDefinitionRepository(db)

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

	// Initialize Neo4j vector index asynchronously
	if svc.Neo4jClient != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
			defer cancel()
			if err := svc.Neo4jClient.CreateVectorIndex(ctx, 1536); err != nil {
				logger.Error("Failed to create Neo4j vector index", err)
			} else {
				logger.Info("Neo4j vector index ensured", nil)
			}
		}()
	}

	// Initialize Extractor factory with LLM Client
	svc.ExtractorFactory = func(strategy string) extractor.Extractor {
		return extractor.NewExtractorByStrategy(strategy, llmClient, defaultModelSvc, nil, nil)
	}

	return svc
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
	for docID := range docIDSet {
		if doc, err := s.DocumentRepo.GetByID(ctx, docID); err == nil && doc != nil {
			docInfoCache[docID] = doc.Name
		}
	}

	// 7. Build nodes
	nodes := make([]model.GraphNode, 0, len(entities))
	categorySet := make(map[string]bool)
	for _, entity := range entities {
		nodeID := fmt.Sprintf("ent:%s", entity.ID.String())

		// Build sources array
		var sources []model.GraphNodeSource
		if entityDocs, exists := entitySources[entity.ID]; exists {
			for docID, weight := range entityDocs {
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
				Description: entity.Description,
				Sources:     sources,
			},
		}
		nodes = append(nodes, node)
		categorySet[entity.Type] = true
	}

	// 8. Build edges
	edges := make([]model.GraphEdge, 0, len(relationships))
	for _, rel := range relationships {
		// Only include edges where both nodes exist
		if _, headExists := entityMap[rel.HeadEntityID]; !headExists {
			continue
		}
		if _, tailExists := entityMap[rel.TailEntityID]; !tailExists {
			continue
		}

		edge := model.GraphEdge{
			Source: fmt.Sprintf("ent:%s", rel.HeadEntityID.String()),
			Target: fmt.Sprintf("ent:%s", rel.TailEntityID.String()),
			Label:  rel.RelationType,
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
	}, nil
}
