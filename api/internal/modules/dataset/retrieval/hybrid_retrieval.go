package retrieval

import (
	"context"
	"math"
	"sort"
	"sync"
)

type HybridRetrievalService struct {
	vectorRetrieval   *VectorRetrievalService
	keywordRetrieval  *KeywordRetrievalService
	fullTextRetrieval *FullTextRetrievalService
}

type HybridWeights struct {
	VectorWeight   float64 `json:"vector_weight"`
	KeywordWeight  float64 `json:"keyword_weight"`
	FullTextWeight float64 `json:"full_text_weight"`
}

type HybridSearchOptions struct {
	SearchOptions SearchOptions
	Weights       HybridWeights `json:"weights"`
}

func NewHybridRetrievalService(
	vectorRetrieval *VectorRetrievalService,
	keywordRetrieval *KeywordRetrievalService,
	fullTextRetrieval *FullTextRetrievalService,
) *HybridRetrievalService {
	return &HybridRetrievalService{
		vectorRetrieval:   vectorRetrieval,
		keywordRetrieval:  keywordRetrieval,
		fullTextRetrieval: fullTextRetrieval,
	}
}

func (s *HybridRetrievalService) Search(ctx context.Context, className string, query string, opts HybridSearchOptions) ([]SearchResult, error) {
	var allResults []SearchResult
	resultMap := make(map[string]*SearchResult)
	var mu sync.Mutex
	var wg sync.WaitGroup

	searchFuncs := []struct {
		name     string
		weight   float64
		enabled  bool
		searchFn func() ([]SearchResult, error)
	}{
		{
			name:    "vector",
			weight:  opts.Weights.VectorWeight,
			enabled: s.vectorRetrieval != nil,
			searchFn: func() ([]SearchResult, error) {
				return s.vectorRetrieval.Search(ctx, className, query, opts.SearchOptions)
			},
		},
		{
			name:    "keyword",
			weight:  opts.Weights.KeywordWeight,
			enabled: s.keywordRetrieval != nil,
			searchFn: func() ([]SearchResult, error) {
				return s.keywordRetrieval.Search(ctx, query, opts.SearchOptions)
			},
		},
		{
			name:    "fulltext",
			weight:  opts.Weights.FullTextWeight,
			enabled: s.fullTextRetrieval != nil,
			searchFn: func() ([]SearchResult, error) {
				return s.fullTextRetrieval.Search(ctx, className, query, opts.SearchOptions)
			},
		},
	}

	for _, searchFunc := range searchFuncs {
		if !searchFunc.enabled || searchFunc.weight <= 0 {
			continue
		}

		wg.Add(1)
		go func(sf struct {
			name     string
			weight   float64
			enabled  bool
			searchFn func() ([]SearchResult, error)
		}) {
			defer wg.Done()

			results, err := sf.searchFn()
			if err != nil {
				return
			}

			mu.Lock()
			defer mu.Unlock()

			for _, result := range results {
				normalizedScore := s.normalizeScore(result.Score, sf.weight)
				if existing, exists := resultMap[result.ID]; exists {
					existing.Score += normalizedScore
				} else {
					newResult := result
					newResult.Score = normalizedScore
					resultMap[result.ID] = &newResult
				}
			}
		}(searchFunc)
	}

	wg.Wait()

	for _, result := range resultMap {
		allResults = append(allResults, *result)
	}

	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].Score > allResults[j].Score
	})

	if len(allResults) > opts.SearchOptions.Limit {
		allResults = allResults[:opts.SearchOptions.Limit]
	}

	return allResults, nil
}

func (s *HybridRetrievalService) normalizeScore(score, weight float64) float64 {
	normalized := math.Max(0, math.Min(1, score))
	return normalized * weight
}

func (s *HybridRetrievalService) GetDefaultWeights() HybridWeights {
	return HybridWeights{
		VectorWeight:   0.6,
		KeywordWeight:  0.2,
		FullTextWeight: 0.2,
	}
}
