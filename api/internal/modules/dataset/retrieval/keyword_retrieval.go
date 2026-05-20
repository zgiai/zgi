package retrieval

import (
	"context"

	"github.com/zgiai/zgi/api/pkg/indexing"
	"github.com/zgiai/zgi/api/pkg/tokenization"
)

// SearchResult represents a single search result

// SearchOptions defines search parameters

// KeywordRetrievalService handles keyword-based retrieval operations

type KeywordRetrievalService struct {
	tokenizer    tokenization.TokenizationService
	keywordIndex *indexing.KeywordIndex
}

// NewKeywordRetrievalService creates a new keyword retrieval service
func NewKeywordRetrievalService(tokenizer tokenization.TokenizationService) *KeywordRetrievalService {
	return &KeywordRetrievalService{
		tokenizer:    tokenizer,
		keywordIndex: indexing.NewKeywordIndex(),
	}
}

// Search performs keyword-based search
func (s *KeywordRetrievalService) Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error) {
	tokens, err := s.tokenizer.Tokenize(query)
	if err != nil {
		return nil, err
	}
	results := s.keywordIndex.Search(tokens, opts.Limit)
	searchResults := make([]SearchResult, 0, len(results))
	for _, r := range results {
		searchResults = append(searchResults, SearchResult{
			ID:      r.ID,
			Content: r.Content,
			Score:   r.Score,
		})
	}
	return searchResults, nil
}

// IndexSegment adds a segment to the keyword index
func (s *KeywordRetrievalService) IndexSegment(segmentID, content string) error {
	tokens, err := s.tokenizer.Tokenize(content)
	if err != nil {
		return err
	}
	return s.keywordIndex.AddSegment(segmentID, content, tokens)
}

func (s *KeywordRetrievalService) IndexSegments(segments map[string]string) error {
	for id, content := range segments {
		err := s.IndexSegment(id, content)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *KeywordRetrievalService) ClearIndex() {
	s.keywordIndex = indexing.NewKeywordIndex()
}
