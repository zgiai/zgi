package retrieval

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/graph"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/repository"
)

// GraphContext represents the graph-based context for retrieved entities
type GraphContext struct {
	Entities      []GraphEntity   `json:"entities"`
	Relationships []GraphRelation `json:"relationships"`
	Summary       string          `json:"summary,omitempty"`
}

// GraphEntity represents an entity with its graph context
type GraphEntity struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	CanonicalName string                 `json:"canonical_name"`
	Type          string                 `json:"type"`
	SourceCount   int                    `json:"source_count"`
	Properties    map[string]interface{} `json:"properties,omitempty"`
}

// GraphRelation represents a relationship between entities
type GraphRelation struct {
	HeadEntity   string `json:"head_entity"`
	TailEntity   string `json:"tail_entity"`
	RelationType string `json:"relation_type"`
	Weight       int    `json:"weight"`
}

// GraphRetrieval handles graph-based context retrieval using Neo4j
type GraphRetrieval struct {
	neo4jClient *graph.Neo4jClient
	entityRepo  *repository.EntityRepository
	relRepo     *repository.RelationshipRepository
}

// NewGraphRetrieval creates a new GraphRetrieval instance
func NewGraphRetrieval(
	neo4jClient *graph.Neo4jClient,
	entityRepo *repository.EntityRepository,
	relRepo *repository.RelationshipRepository,
) *GraphRetrieval {
	return &GraphRetrieval{
		neo4jClient: neo4jClient,
		entityRepo:  entityRepo,
		relRepo:     relRepo,
	}
}

// Retrieve fetches graph context for given entity names
// hopDepth: 1 = direct neighbors, 2 = neighbors of neighbors
func (r *GraphRetrieval) Retrieve(ctx context.Context, kbID uuid.UUID, entityNames []string, hopDepth int) (*GraphContext, error) {
	if len(entityNames) == 0 {
		return &GraphContext{}, nil
	}

	// Limit hop depth to prevent explosion
	if hopDepth < 1 {
		hopDepth = 1
	}
	if hopDepth > 5 {
		hopDepth = 5
	}

	// 1. Try Neo4j first if available
	if r.neo4jClient != nil {
		return r.retrieveFromNeo4j(ctx, kbID, entityNames, hopDepth)
	}

	// 2. Fallback to PostgreSQL-based retrieval
	return r.retrieveFromPostgres(ctx, kbID, entityNames)
}

// retrieveFromNeo4j uses Neo4j for graph traversal with PPR
func (r *GraphRetrieval) retrieveFromNeo4j(ctx context.Context, kbID uuid.UUID, entityNames []string, hopDepth int) (*GraphContext, error) {
	result := &GraphContext{
		Entities:      make([]GraphEntity, 0),
		Relationships: make([]GraphRelation, 0),
	}

	// Run Personalized PageRank to find most relevant nodes
	// We use the extracted entities as seed nodes
	pprResults, err := graph.RunPPR(ctx, r.neo4jClient, entityNames, 0.85, 100)
	if err != nil {
		// Fallback to simple neighbor lookup if PPR fails (e.g. GDS not available)
		return r.retrieveFromNeo4jSimple(ctx, kbID, entityNames)
	}

	// If no results from PPR, try simple lookup
	if len(pprResults) == 0 {
		return r.retrieveFromNeo4jSimple(ctx, kbID, entityNames)
	}

	// Collect top nodes from PPR results
	topNodes := make([]string, 0, len(pprResults))
	for _, res := range pprResults {
		topNodes = append(topNodes, res.NodeName)
	}

	// Get full context (properties and relationships) for these top nodes
	// Use Multi-Hop to capture deeper relationships and paths
	searchResults, _, err := r.neo4jClient.GetEntityContextMultiHop(ctx, kbID.String(), topNodes, hopDepth)
	if err != nil {
		return nil, fmt.Errorf("neo4j context query failed: %w", err)
	}

	seenEntities := make(map[string]bool)
	seenRelations := make(map[string]bool)

	// Process results exactly as before, but now based on PPR-ranked nodes
	for _, sr := range searchResults {
		// Add the main entity
		if name, ok := sr.Entity["name"].(string); ok && !seenEntities[name] {
			seenEntities[name] = true
			entity := GraphEntity{
				Name:       name,
				Properties: sr.Entity,
			}
			if id, ok := sr.Entity["id"].(string); ok {
				entity.ID = id
			}
			if entityType, ok := sr.Entity["type"].(string); ok {
				entity.Type = entityType
			}
			if canonicalName, ok := sr.Entity["canonical_name"].(string); ok {
				entity.CanonicalName = canonicalName
			}
			if sourceCount, ok := sr.Entity["source_count"].(int64); ok {
				entity.SourceCount = int(sourceCount)
			}
			result.Entities = append(result.Entities, entity)
		}

		// Add neighbors and relationships
		for _, neighbor := range sr.Neighbors {
			// Add neighbor entity
			if neighborName, ok := neighbor.Node["name"].(string); ok && !seenEntities[neighborName] {
				seenEntities[neighborName] = true
				neighborEntity := GraphEntity{
					Name:       neighborName,
					Properties: neighbor.Node,
				}
				if id, ok := neighbor.Node["id"].(string); ok {
					neighborEntity.ID = id
				}
				if entityType, ok := neighbor.Node["type"].(string); ok {
					neighborEntity.Type = entityType
				}
				result.Entities = append(result.Entities, neighborEntity)
			}

			// Add relationship
			headName, _ := sr.Entity["name"].(string)
			tailName, _ := neighbor.Node["name"].(string)
			relKey := fmt.Sprintf("%s-%s-%s", headName, neighbor.RelationshipType, tailName)

			if !seenRelations[relKey] {
				seenRelations[relKey] = true
				result.Relationships = append(result.Relationships, GraphRelation{
					HeadEntity:   headName,
					TailEntity:   tailName,
					RelationType: neighbor.RelationshipType,
					Weight:       1,
				})
			}
		}
	}

	// Generate summary
	result.Summary = r.generateSummary(result)

	return result, nil
}

// retrieveFromNeo4jSimple uses simple neighbor lookup (fallback behavior)
func (r *GraphRetrieval) retrieveFromNeo4jSimple(ctx context.Context, kbID uuid.UUID, entityNames []string) (*GraphContext, error) {
	result := &GraphContext{
		Entities:      make([]GraphEntity, 0),
		Relationships: make([]GraphRelation, 0),
	}

	searchResults, err := r.neo4jClient.GetEntityContext(ctx, kbID.String(), entityNames)
	if err != nil {
		return nil, fmt.Errorf("neo4j query failed: %w", err)
	}

	seenEntities := make(map[string]bool)
	seenRelations := make(map[string]bool)

	for _, sr := range searchResults {
		if name, ok := sr.Entity["name"].(string); ok && !seenEntities[name] {
			seenEntities[name] = true
			entity := GraphEntity{
				Name:       name,
				Properties: sr.Entity,
			}
			if id, ok := sr.Entity["id"].(string); ok {
				entity.ID = id
			}
			if entityType, ok := sr.Entity["type"].(string); ok {
				entity.Type = entityType
			}
			if canonicalName, ok := sr.Entity["canonical_name"].(string); ok {
				entity.CanonicalName = canonicalName
			}
			if sourceCount, ok := sr.Entity["source_count"].(int64); ok {
				entity.SourceCount = int(sourceCount)
			}
			result.Entities = append(result.Entities, entity)
		}

		for _, neighbor := range sr.Neighbors {
			if neighborName, ok := neighbor.Node["name"].(string); ok && !seenEntities[neighborName] {
				seenEntities[neighborName] = true
				neighborEntity := GraphEntity{
					Name:       neighborName,
					Properties: neighbor.Node,
				}
				if id, ok := neighbor.Node["id"].(string); ok {
					neighborEntity.ID = id
				}
				if entityType, ok := neighbor.Node["type"].(string); ok {
					neighborEntity.Type = entityType
				}
				result.Entities = append(result.Entities, neighborEntity)
			}

			headName, _ := sr.Entity["name"].(string)
			tailName, _ := neighbor.Node["name"].(string)
			relKey := fmt.Sprintf("%s-%s-%s", headName, neighbor.RelationshipType, tailName)

			if !seenRelations[relKey] {
				seenRelations[relKey] = true
				result.Relationships = append(result.Relationships, GraphRelation{
					HeadEntity:   headName,
					TailEntity:   tailName,
					RelationType: neighbor.RelationshipType,
					Weight:       1,
				})
			}
		}
	}

	result.Summary = r.generateSummary(result)
	return result, nil
}

// retrieveFromPostgres uses PostgreSQL tables for graph traversal (fallback)
func (r *GraphRetrieval) retrieveFromPostgres(ctx context.Context, kbID uuid.UUID, entityNames []string) (*GraphContext, error) {
	result := &GraphContext{
		Entities:      make([]GraphEntity, 0),
		Relationships: make([]GraphRelation, 0),
	}

	if r.entityRepo == nil {
		return result, nil
	}

	// Get all entities for this KB
	entities, err := r.entityRepo.FindByKBID(ctx, kbID)
	if err != nil {
		return nil, fmt.Errorf("failed to get entities: %w", err)
	}

	// Filter entities matching the query names (case-insensitive)
	queryNamesLower := make(map[string]bool)
	for _, name := range entityNames {
		queryNamesLower[strings.ToLower(name)] = true
	}

	matchedEntityIDs := make(map[uuid.UUID]bool)
	for _, entity := range entities {
		if queryNamesLower[strings.ToLower(entity.Name)] || queryNamesLower[strings.ToLower(entity.CanonicalName)] {
			matchedEntityIDs[entity.ID] = true
			result.Entities = append(result.Entities, GraphEntity{
				ID:            entity.ID.String(),
				Name:          entity.Name,
				CanonicalName: entity.CanonicalName,
				Type:          entity.Type,
				SourceCount:   entity.SourceCount,
			})
		}
	}

	// Get relationships for matched entities
	if r.relRepo != nil {
		relationships, err := r.relRepo.FindByKBID(ctx, kbID)
		if err == nil {
			for _, rel := range relationships {
				// Include relationships where either head or tail is in matched entities
				if matchedEntityIDs[rel.HeadEntityID] || matchedEntityIDs[rel.TailEntityID] {
					// Find entity names
					headName := findEntityName(entities, rel.HeadEntityID)
					tailName := findEntityName(entities, rel.TailEntityID)

					result.Relationships = append(result.Relationships, GraphRelation{
						HeadEntity:   headName,
						TailEntity:   tailName,
						RelationType: rel.RelationType,
						Weight:       rel.Weight,
					})

					// Also add the connected entity if not already present
					if !matchedEntityIDs[rel.HeadEntityID] {
						for _, e := range entities {
							if e.ID == rel.HeadEntityID {
								matchedEntityIDs[e.ID] = true
								result.Entities = append(result.Entities, GraphEntity{
									ID:            e.ID.String(),
									Name:          e.Name,
									CanonicalName: e.CanonicalName,
									Type:          e.Type,
									SourceCount:   e.SourceCount,
								})
								break
							}
						}
					}
					if !matchedEntityIDs[rel.TailEntityID] {
						for _, e := range entities {
							if e.ID == rel.TailEntityID {
								matchedEntityIDs[e.ID] = true
								result.Entities = append(result.Entities, GraphEntity{
									ID:            e.ID.String(),
									Name:          e.Name,
									CanonicalName: e.CanonicalName,
									Type:          e.Type,
									SourceCount:   e.SourceCount,
								})
								break
							}
						}
					}
				}
			}
		}
	}

	result.Summary = r.generateSummary(result)
	return result, nil
}

// generateSummary creates a human-readable summary of the graph context
func (r *GraphRetrieval) generateSummary(gc *GraphContext) string {
	if len(gc.Entities) == 0 {
		return ""
	}

	var parts []string
	for _, rel := range gc.Relationships {
		parts = append(parts, fmt.Sprintf("%s %s %s", rel.HeadEntity, rel.RelationType, rel.TailEntity))
	}

	if len(parts) == 0 {
		entityNames := make([]string, 0, len(gc.Entities))
		for _, e := range gc.Entities {
			entityNames = append(entityNames, e.Name)
		}
		return fmt.Sprintf("Found entities: %s", strings.Join(entityNames, ", "))
	}

	return strings.Join(parts, "; ")
}

// findEntityName looks up entity name by ID
func findEntityName(entities []*model.Entity, id uuid.UUID) string {
	for _, e := range entities {
		if e.ID == id {
			return e.Name
		}
	}
	return id.String()
}
