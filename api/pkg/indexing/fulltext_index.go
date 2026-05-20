package indexing

import (
	"sync"

	"github.com/zgiai/ginext/pkg/scoring"
)


type FullTextIndex struct {
	bm25     *scoring.BM25
	inverted map[string]map[string]struct{} // token -> docID set
	mu       sync.RWMutex
}

func NewFullTextIndex(k1, b float64) *FullTextIndex {
	return &FullTextIndex{
		bm25:     scoring.NewBM25(k1, b),
		inverted: make(map[string]map[string]struct{}),
	}
}

func (idx *FullTextIndex) AddDocument(docID string, tokens []string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.bm25.AddDocument(docID, tokens)
	for _, token := range tokens {
		if idx.inverted[token] == nil {
			idx.inverted[token] = make(map[string]struct{})
		}
		idx.inverted[token][docID] = struct{}{}
	}
}

func (idx *FullTextIndex) RemoveDocument(docID string, tokens []string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	for _, token := range tokens {
		if docs, ok := idx.inverted[token]; ok {
			delete(docs, docID)
			if len(docs) == 0 {
				delete(idx.inverted, token)
			}
		}
	}
}

func (idx *FullTextIndex) Search(queryTokens []string, limit int) []struct {
	DocID string
	Score float64
} {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	candidates := make(map[string]struct{})
	for _, token := range queryTokens {
		for docID := range idx.inverted[token] {
			candidates[docID] = struct{}{}
		}
	}
	var results []struct {
		DocID string
		Score float64
	}
	for docID := range candidates {
		score := idx.bm25.CalculateScore(queryTokens, docID)
		if score > 0 {
			results = append(results, struct {
				DocID string
				Score float64
			}{DocID: docID, Score: score})
		}
	}
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[i].Score < results[j].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
	if len(results) > limit {
		results = results[:limit]
	}
	return results
}
