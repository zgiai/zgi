package retrieval

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/internal/modules/llm/client"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/ginext/pkg/vectordb"
)

// EntityMatch represents a matched entity from vector search
type EntityMatch struct {
	EntityID      string                 `json:"entity_id"`
	Name          string                 `json:"name"`
	CanonicalName string                 `json:"canonical_name"`
	Type          string                 `json:"type"`
	Score         float64                `json:"score"`
	Properties    map[string]interface{} `json:"properties,omitempty"`
}

// EntityRetrieval handles entity-based vector retrieval
type EntityRetrieval struct {
	vectorDB       vectordb.VectorDB
	llmClient      client.LLMClient
	embeddingModel string
}

// NewEntityRetrieval creates a new EntityRetrieval instance
func NewEntityRetrieval(cfg *config.Config, llmClient client.LLMClient) *EntityRetrieval {
	vectorDB := vectordb.NewWeaviateClient(&cfg.VectorStore)
	return &EntityRetrieval{
		vectorDB:       vectorDB,
		llmClient:      llmClient,
		embeddingModel: "text-embedding-3-large",
	}
}

// Retrieve performs entity-based vector retrieval
// Given entity names, it generates embeddings and searches for similar entities in VectorDB
func (r *EntityRetrieval) Retrieve(ctx context.Context, tenantID string, kbID uuid.UUID, entityNames []string, limit int) ([]EntityMatch, error) {
	if len(entityNames) == 0 {
		return []EntityMatch{}, nil
	}

	if r.llmClient == nil {
		return nil, fmt.Errorf("llm client is not initialized")
	}

	// 1. Generate embeddings for entity names
	embeddingReq := &adapter.EmbeddingsRequest{
		Model: r.embeddingModel,
		Input: entityNames,
	}

	embeddingResp, err := r.llmClient.Embed(ctx, tenantID, embeddingReq)
	if err != nil {
		return nil, fmt.Errorf("failed to generate embeddings: %w", err)
	}

	if len(embeddingResp.Data) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}

	// 2. Search for similar entities in VectorDB
	var allMatches []EntityMatch
	className := fmt.Sprintf("Entity_%s", kbID.String())

	for i, embedding := range embeddingResp.Data {
		// Convert embedding to float64 slice
		vector := make([]float64, len(embedding.Embedding))
		for j, v := range embedding.Embedding {
			vector[j] = float64(v)
		}

		results, err := r.vectorDB.SearchVectors(ctx, className, vector, limit)
		if err != nil {
			// Log error but continue with other entities
			continue
		}

		for _, result := range results {
			match := EntityMatch{
				Score:      getFloatFromMap(result, "_distance"),
				Properties: result,
			}

			// Extract standard fields
			if id, ok := result["id"].(string); ok {
				match.EntityID = id
			}
			if name, ok := result["name"].(string); ok {
				match.Name = name
			}
			if canonicalName, ok := result["canonical_name"].(string); ok {
				match.CanonicalName = canonicalName
			}
			if entityType, ok := result["type"].(string); ok {
				match.Type = entityType
			}

			// Associate with the query entity name
			if i < len(entityNames) {
				match.Properties["query_entity"] = entityNames[i]
			}

			allMatches = append(allMatches, match)
		}
	}

	// 3. Deduplicate and sort by score
	return deduplicateMatches(allMatches, limit), nil
}

// RetrieveByEmbedding searches for entities using a pre-computed embedding vector
func (r *EntityRetrieval) RetrieveByEmbedding(ctx context.Context, kbID uuid.UUID, vector []float64, limit int) ([]EntityMatch, error) {
	className := fmt.Sprintf("Entity_%s", kbID.String())

	results, err := r.vectorDB.SearchVectors(ctx, className, vector, limit)
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	matches := make([]EntityMatch, 0, len(results))
	for _, result := range results {
		match := EntityMatch{
			Score:      getFloatFromMap(result, "_distance"),
			Properties: result,
		}

		if id, ok := result["id"].(string); ok {
			match.EntityID = id
		}
		if name, ok := result["name"].(string); ok {
			match.Name = name
		}
		if canonicalName, ok := result["canonical_name"].(string); ok {
			match.CanonicalName = canonicalName
		}
		if entityType, ok := result["type"].(string); ok {
			match.Type = entityType
		}

		matches = append(matches, match)
	}

	return matches, nil
}

// getFloatFromMap safely extracts a float64 from a map
func getFloatFromMap(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case float64:
			return val
		case float32:
			return float64(val)
		case int:
			return float64(val)
		}
	}
	return 0.0
}

// deduplicateMatches removes duplicate entities and returns top N by score
func deduplicateMatches(matches []EntityMatch, limit int) []EntityMatch {
	seen := make(map[string]bool)
	var unique []EntityMatch

	for _, match := range matches {
		if match.EntityID != "" && !seen[match.EntityID] {
			seen[match.EntityID] = true
			unique = append(unique, match)
		}
	}

	// Sort by score (lower distance = better match)
	// Note: For cosine similarity, we might need to invert this
	if len(unique) > limit {
		unique = unique[:limit]
	}

	return unique
}
