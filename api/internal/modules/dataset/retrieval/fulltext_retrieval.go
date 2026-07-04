package retrieval

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/zgiai/zgi/api/pkg/indexing"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/tokenization"
	"github.com/zgiai/zgi/api/pkg/vectordb"
	"go.uber.org/zap"
)

// FullTextRetrievalService handles BM25-based full text retrieval
type FullTextRetrievalService struct {
	tokenizer  tokenization.TokenizationService
	index      *indexing.FullTextIndex
	vectorDB   vectordb.VectorDB
	segmentMap map[string]string // Map segment ID to content
}

func NewFullTextRetrievalService(tokenizer tokenization.TokenizationService, k1, b float64, vectorDB vectordb.VectorDB) *FullTextRetrievalService {
	return &FullTextRetrievalService{
		tokenizer: tokenizer,
		index:     indexing.NewFullTextIndex(k1, b),
		vectorDB:  vectorDB, // Initialize vectorDB in constructor
	}
}

// IndexSegment adds a segment to the fulltext index
func (s *FullTextRetrievalService) IndexSegment(segmentID, content string) error {
	tokens, err := s.tokenizer.Tokenize(content)
	if err != nil {
		return err
	}
	s.index.AddDocument(segmentID, tokens)

	// Store content in segment map for local BM25 fallback
	if s.segmentMap == nil {
		s.segmentMap = make(map[string]string)
	}
	s.segmentMap[segmentID] = content

	return nil
}

func (s *FullTextRetrievalService) IndexSegments(segments map[string]string) error {
	// Store content in segment map for local BM25 fallback
	s.segmentMap = segments

	for id, content := range segments {
		err := s.IndexSegment(id, content)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *FullTextRetrievalService) ClearIndex() {
	s.index = indexing.NewFullTextIndex(1.5, 0.75)
	s.segmentMap = nil
}

// Search performs BM25-based full text search
// This method now tries to use Weaviate's BM25 search, falling back to local BM25 if needed
func (s *FullTextRetrievalService) Search(ctx context.Context, className string, query string, opts SearchOptions) ([]SearchResult, error) {
	// Try to use Weaviate's BM25 search if vectorDB is available
	if s.vectorDB != nil && className != "" {
		results, err := s.vectorDB.SearchByFullText(ctx, className, query, opts.Limit)
		if err == nil {
			// Successfully used Weaviate BM25 search
			var searchResults []SearchResult
			for i, r := range results {
				docID, ok := r["doc_id"].(string)
				if !ok || docID == "" {
					continue
				}
				bm25Score := extractBM25Score(r, i)
				score := fallbackBM25RankScore(i)
				result := SearchResult{
					ID:    docID,
					Score: score,
				}

				// Add content if available
				if text, ok := r["text"].(string); ok {
					result.Content = text
				}

				// Add metadata
				result.Metadata = make(map[string]interface{})
				result.Metadata["doc_id"] = docID
				result.Metadata["bm25_score"] = bm25Score
				result.Metadata["bm25_rank_score"] = score
				if datasetID, ok := r["dataset_id"].(string); ok {
					result.Metadata["dataset_id"] = datasetID
				}
				if documentID, ok := r["document_id"].(string); ok {
					result.Metadata["document_id"] = documentID
				}
				if docHash, ok := r["doc_hash"].(string); ok {
					result.Metadata["doc_hash"] = docHash
				}

				searchResults = append(searchResults, result)
			}
			return searchResults, nil
		}

		// If Weaviate search failed, fall back to local BM25 with warning
		logger.WarnContext(ctx, "weaviate full-text search failed, falling back to local BM25",
			err,
			zap.String("class_name", className),
			zap.Int("limit", opts.Limit),
		)
	}

	// Fallback to local BM25 implementation
	tokens, err := s.tokenizer.Tokenize(query)
	if err != nil {
		return nil, err
	}
	results := s.index.Search(tokens, opts.Limit)
	var searchResults []SearchResult
	for i, r := range results {
		content := ""
		if s.segmentMap != nil {
			content = s.segmentMap[r.DocID]
		}

		searchResults = append(searchResults, SearchResult{
			ID:    r.DocID,
			Score: fallbackBM25RankScore(i),
			Metadata: map[string]interface{}{
				"bm25_score":      r.Score,
				"bm25_rank_score": fallbackBM25RankScore(i),
			},
			Content: content,
		})
	}
	return searchResults, nil
}

func extractBM25Score(result map[string]interface{}, rank int) float64 {
	if additional, ok := result["_additional"].(map[string]interface{}); ok {
		if score, ok := numericBM25Score(additional["score"]); ok {
			return score
		}
	}
	if score, ok := numericBM25Score(result["score"]); ok {
		return score
	}
	return fallbackBM25RankScore(rank)
}

func numericBM25Score(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case json.Number:
		score, err := typed.Float64()
		return score, err == nil
	case string:
		score, err := strconv.ParseFloat(typed, 64)
		return score, err == nil
	default:
		return 0, false
	}
}

func fallbackBM25RankScore(rank int) float64 {
	score := 1.0 - float64(rank)*0.05
	if score < 0.1 {
		return 0.1
	}
	return score
}
