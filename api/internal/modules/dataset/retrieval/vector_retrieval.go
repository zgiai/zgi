package retrieval

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/zgiai/zgi/api/pkg/embedding"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/vectordb"
)

// VectorRetrievalService handles vector-based retrieval operations
type VectorRetrievalService struct {
	// embedding       Embedding
	embeddingService embedding.EmbeddingService
	vectorDB         vectordb.VectorDB
	embeddingConfig  map[string]interface{}
}

// NewVectorRetrievalService creates a new vector retrieval service
func NewVectorRetrievalService(
	// embedding Embedding,
	embeddingService embedding.EmbeddingService,
	vectorDB vectordb.VectorDB,
	className string,
) *VectorRetrievalService {
	return &VectorRetrievalService{
		// embedding: embedding,
		embeddingService: embeddingService,
		vectorDB:         vectorDB,
	}
}

// SearchResult represents a single search result
type SearchResult struct {
	ID       string                 `json:"id"`
	Content  string                 `json:"content"`
	Score    float64                `json:"score"`
	Metadata map[string]interface{} `json:"metadata"`
}

// SearchOptions defines search parameters
type SearchOptions struct {
	Limit          int                    `json:"limit"`
	ScoreThreshold float64                `json:"score_threshold"`
	DocumentIDs    []string               `json:"document_ids,omitempty"`
	Filter         map[string]interface{} `json:"filter,omitempty"`
	PreQAExtension bool                   `json:"pre_qa_extension"` // Whether to include question-related data in search
}

// Search performs vector similarity search
func (s *VectorRetrievalService) Search(
	ctx context.Context,
	className string,
	query string,
	opts SearchOptions,
) ([]SearchResult, error) {
	// 1. Convert query to vector
	// queryVector, err := s.embedding.EmbedQuery(ctx, query)
	queryVector, err := s.embeddingService.EmbedText(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	// 2. Search in vector database
	var results []map[string]interface{}

	// Check if we need to include question-related data
	if opts.PreQAExtension {
		// Generate the question class name
		// Extract dataset ID from className (format: Vector_index_{dataset_id}_Node)
		// We need to convert it back to get the original dataset ID
		questionClassName := strings.Replace(className, "_Node", "_Question_Node", 1)

		// Check if the vectorDB implements ExtendedVectorDB interface
		if extendedDB, ok := s.vectorDB.(interface {
			SearchVectorsWithQuestions(ctx context.Context, className, questionClassName string, vector []float64, limit int) ([]map[string]interface{}, error)
		}); ok {
			// Use the method that searches both regular segments and questions
			results, err = extendedDB.SearchVectorsWithQuestions(ctx, className, questionClassName, queryVector, opts.Limit)
		} else {
			// Fallback to regular search if the method is not implemented
			logger.Warn("SearchVectorsWithQuestions not implemented, falling back to regular search", map[string]interface{}{
				"class_name":          className,
				"question_class_name": questionClassName,
			})
			results, err = s.vectorDB.SearchVectors(ctx, className, queryVector, opts.Limit)
		}
	} else {
		// Use regular search method
		results, err = s.vectorDB.SearchVectors(ctx, className, queryVector, opts.Limit)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to search vectors: %w", err)
	}

	// 3. Convert and filter results
	logger.Info("Vector search raw results from Weaviate", map[string]interface{}{
		"raw_count":       len(results),
		"class_name":      className,
		"score_threshold": opts.ScoreThreshold,
	})

	searchResults := make([]SearchResult, 0, len(results))
	filteredCount := 0
	for _, result := range results {
		// Extract score from metadata
		score := s.extractScore(result)

		// Apply score threshold
		if score < opts.ScoreThreshold {
			filteredCount++
			logger.Debug("Result filtered by score threshold", map[string]interface{}{
				"score":     score,
				"threshold": opts.ScoreThreshold,
				"id":        s.extractID(result),
			})
			continue
		}

		// Apply document IDs filter if specified
		if len(opts.DocumentIDs) > 0 {
			// TODO: document IDs filter
		}

		// Apply custom filter if specified
		if opts.Filter != nil && len(opts.Filter) > 0 {
			// TODO: custom filter
		}

		searchResult := SearchResult{
			ID:       s.extractID(result),
			Content:  s.extractContent(result),
			Score:    score,
			Metadata: result,
		}
		searchResults = append(searchResults, searchResult)
	}

	// 4. Sort by score (descending)
	sort.Slice(searchResults, func(i, j int) bool {
		return searchResults[i].Score > searchResults[j].Score
	})

	logger.Info("Vector search completed", map[string]interface{}{
		"query":            query[:min(len(query), 100)],
		"results_count":    len(searchResults),
		"raw_count":        len(results),
		"filtered_count":   filteredCount,
		"score_threshold":  opts.ScoreThreshold,
		"model":            s.embeddingService.GetModel(),
		"pre_qa_extension": opts.PreQAExtension,
	})

	return searchResults, nil
}

// StoreDocument stores a document in the vector database
func (s *VectorRetrievalService) StoreDocument(
	ctx context.Context,
	className string,
	id string,
	content string,
	metadata map[string]interface{},
) error {
	// 1. Convert content to vector
	// vector, err := s.embedding.EmbedQuery(ctx, content)
	vector, err := s.embeddingService.EmbedText(ctx, content)
	if err != nil {
		return fmt.Errorf("failed to embed content: %w", err)
	}

	// 2. Store in vector database
	err = s.vectorDB.StoreVector(ctx, id, className, metadata, vector)
	if err != nil {
		return fmt.Errorf("failed to store vector: %w", err)
	}

	logger.Info("Document stored in vector database", map[string]interface{}{
		"id":             id,
		"content_length": len(content),
		"vector_size":    len(vector),
		// "model":          s.embedding.GetModel(),
		"model": s.embeddingService.GetModel(),
	})

	return nil
}

// SetEmbeddingService sets the embedding service for vector operations
func (s *VectorRetrievalService) SetEmbeddingService(embeddingService embedding.EmbeddingService) {
	s.embeddingService = embeddingService
}

// GetEmbeddingService returns the currently configured embedding service.
func (s *VectorRetrievalService) GetEmbeddingService() embedding.EmbeddingService {
	if s == nil {
		return nil
	}
	return s.embeddingService
}

// extractScore extracts score from search result metadata
func (s *VectorRetrievalService) extractScore(result map[string]interface{}) float64 {
	if score, ok := result["score"].(float64); ok {
		return score
	}
	if score, ok := result["_additional"].(map[string]interface{}); ok {
		if distance, ok := score["distance"].(float64); ok {
			// Convert distance to similarity score (1 - distance)
			return 1.0 - distance
		}
	}
	return 0.0
}

// extractID extracts ID from search result metadata
func (s *VectorRetrievalService) extractID(result map[string]interface{}) string {
	if id, ok := result["id"].(string); ok {
		return id
	}
	if additional, ok := result["_additional"].(map[string]interface{}); ok {
		if id, ok := additional["id"].(string); ok {
			return id
		}
	}
	return ""
}

// extractContent extracts content from search result metadata
func (s *VectorRetrievalService) extractContent(result map[string]interface{}) string {
	// Use 'text' as the primary content field
	if text, ok := result["text"].(string); ok {
		return text
	}
	// Fallback to 'content' for compatibility
	if content, ok := result["content"].(string); ok {
		return content
	}
	return ""
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
