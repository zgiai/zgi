package retrieval

import (
	"context"
	"sort"
	"strings"
	"sync"
	"unicode"
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

type HybridFusionConfig struct {
	RRFK         float64
	VectorWeight float64
	BM25Weight   float64
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
	var vectorResults []SearchResult
	var bm25Results []SearchResult
	var wg sync.WaitGroup
	var mu sync.Mutex

	searchFuncs := []struct {
		name     string
		enabled  bool
		searchFn func() ([]SearchResult, error)
	}{
		{
			name:    "vector",
			enabled: s.vectorRetrieval != nil,
			searchFn: func() ([]SearchResult, error) {
				return s.vectorRetrieval.Search(ctx, className, query, opts.SearchOptions)
			},
		},
		{
			name:    "bm25",
			enabled: s.fullTextRetrieval != nil,
			searchFn: func() ([]SearchResult, error) {
				return s.fullTextRetrieval.Search(ctx, className, query, opts.SearchOptions)
			},
		},
	}

	for _, searchFunc := range searchFuncs {
		if !searchFunc.enabled {
			continue
		}

		wg.Add(1)
		go func(sf struct {
			name     string
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
			if sf.name == "vector" {
				vectorResults = results
			} else {
				bm25Results = results
			}
		}(searchFunc)
	}

	wg.Wait()

	return FuseVectorBM25Results(query, vectorResults, bm25Results, opts.SearchOptions.Limit, HybridFusionConfig{
		RRFK:         DefaultHybridFusionConfig().RRFK,
		VectorWeight: positiveOrDefault(opts.Weights.VectorWeight, DefaultHybridFusionConfig().VectorWeight),
		BM25Weight:   positiveOrDefault(opts.Weights.FullTextWeight, DefaultHybridFusionConfig().BM25Weight),
	}), nil
}

func DefaultHybridFusionConfig() HybridFusionConfig {
	return HybridFusionConfig{
		RRFK:         60,
		VectorWeight: 0.6,
		BM25Weight:   0.4,
	}
}

func FuseVectorBM25Results(query string, vectorResults, bm25Results []SearchResult, limit int, cfg HybridFusionConfig) []SearchResult {
	cfg = normalizeHybridFusionConfig(cfg)
	candidates := make(map[string]*hybridCandidate)

	for rank, result := range vectorResults {
		if result.ID == "" {
			continue
		}
		candidate := getHybridCandidate(candidates, result)
		if !candidate.hasVector || rank+1 < candidate.vectorRank {
			candidate.vectorRank = rank + 1
		}
		candidate.hasVector = true
		if result.Score > candidate.vectorScore {
			candidate.vectorScore = result.Score
		}
		candidate.result = mergeResult(candidate.result, result)
	}

	for rank, result := range bm25Results {
		if result.ID == "" {
			continue
		}
		candidate := getHybridCandidate(candidates, result)
		if !candidate.hasBM25 || rank+1 < candidate.bm25Rank {
			candidate.bm25Rank = rank + 1
		}
		candidate.hasBM25 = true
		bm25Score := result.Score
		if result.Metadata != nil {
			if rawBM25Score, ok := numericBM25Score(result.Metadata["bm25_score"]); ok {
				bm25Score = rawBM25Score
			}
		}
		if bm25Score > candidate.bm25Score {
			candidate.bm25Score = bm25Score
		}
		candidate.result = mergeResult(candidate.result, result)
	}

	fused := make([]SearchResult, 0, len(candidates))
	queryTerms := lexicalTerms(query)
	for _, candidate := range candidates {
		score := 0.0
		if candidate.hasVector {
			score += cfg.VectorWeight / (cfg.RRFK + float64(candidate.vectorRank))
		}
		if candidate.hasBM25 {
			score += cfg.BM25Weight / (cfg.RRFK + float64(candidate.bm25Rank))
		}

		result := candidate.result
		result.Score = score
		if result.Metadata == nil {
			result.Metadata = make(map[string]interface{})
		} else {
			result.Metadata = cloneMetadata(result.Metadata)
		}
		sources := make([]string, 0, 2)
		if candidate.hasVector {
			result.Metadata["vector_score"] = candidate.vectorScore
			result.Metadata["vector_rank"] = candidate.vectorRank
			sources = append(sources, "vector")
		}
		if candidate.hasBM25 {
			result.Metadata["bm25_score"] = candidate.bm25Score
			result.Metadata["bm25_rank"] = candidate.bm25Rank
			sources = append(sources, "bm25")
			result.Metadata["matched_terms"] = matchedTerms(queryTerms, result.Content, result.Metadata)
		}
		result.Metadata["best_rank"] = candidate.bestRank()
		result.Metadata["retrieval_sources"] = sources
		result.Metadata["fusion_score"] = score
		result.Metadata["score"] = score
		fused = append(fused, result)
	}

	sort.SliceStable(fused, func(i, j int) bool {
		if fused[i].Score != fused[j].Score {
			return fused[i].Score > fused[j].Score
		}
		bestRankI := retrievalBestRank(fused[i])
		bestRankJ := retrievalBestRank(fused[j])
		if bestRankI != bestRankJ {
			return bestRankI < bestRankJ
		}
		return fused[i].ID < fused[j].ID
	})

	if limit > 0 && len(fused) > limit {
		fused = fused[:limit]
	}

	return fused
}

func (s *HybridRetrievalService) GetDefaultWeights() HybridWeights {
	return HybridWeights{
		VectorWeight:   0.6,
		KeywordWeight:  0,
		FullTextWeight: 0.4,
	}
}

type hybridCandidate struct {
	result      SearchResult
	hasVector   bool
	hasBM25     bool
	vectorRank  int
	bm25Rank    int
	vectorScore float64
	bm25Score   float64
}

func getHybridCandidate(candidates map[string]*hybridCandidate, result SearchResult) *hybridCandidate {
	if candidate, ok := candidates[result.ID]; ok {
		return candidate
	}
	candidate := &hybridCandidate{result: result}
	candidates[result.ID] = candidate
	return candidate
}

func (c hybridCandidate) bestRank() int {
	best := 0
	if c.hasVector {
		best = c.vectorRank
	}
	if c.hasBM25 && (best == 0 || c.bm25Rank < best) {
		best = c.bm25Rank
	}
	return best
}

func mergeResult(existing, incoming SearchResult) SearchResult {
	result := existing
	if result.ID == "" {
		result.ID = incoming.ID
	}
	if result.Content == "" {
		result.Content = incoming.Content
	}
	if result.Metadata == nil {
		result.Metadata = cloneMetadata(incoming.Metadata)
		return result
	}
	for key, value := range incoming.Metadata {
		if _, exists := result.Metadata[key]; !exists {
			result.Metadata[key] = value
		}
	}
	return result
}

func normalizeHybridFusionConfig(cfg HybridFusionConfig) HybridFusionConfig {
	defaults := DefaultHybridFusionConfig()
	cfg.RRFK = positiveOrDefault(cfg.RRFK, defaults.RRFK)
	cfg.VectorWeight = positiveOrDefault(cfg.VectorWeight, defaults.VectorWeight)
	cfg.BM25Weight = positiveOrDefault(cfg.BM25Weight, defaults.BM25Weight)
	return cfg
}

func positiveOrDefault(value, fallback float64) float64 {
	if value > 0 {
		return value
	}
	return fallback
}

func cloneMetadata(metadata map[string]interface{}) map[string]interface{} {
	if metadata == nil {
		return nil
	}
	cloned := make(map[string]interface{}, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func lexicalTerms(query string) []string {
	seen := make(map[string]struct{})
	terms := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	unique := make([]string, 0, len(terms))
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		unique = append(unique, term)
	}
	return unique
}

func matchedTerms(queryTerms []string, content string, metadata map[string]interface{}) []string {
	if terms := stringSliceMetadata(metadata["matched_terms"]); len(terms) > 0 {
		return terms
	}
	content = strings.ToLower(content)
	matches := make([]string, 0, len(queryTerms))
	for _, term := range queryTerms {
		if strings.Contains(content, term) {
			matches = append(matches, term)
		}
	}
	return matches
}

func stringSliceMetadata(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []interface{}:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && text != "" {
				items = append(items, text)
			}
		}
		return items
	default:
		return nil
	}
}

func retrievalBestRank(result SearchResult) int {
	if result.Metadata == nil {
		return 0
	}
	switch typed := result.Metadata["best_rank"].(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case int32:
		return int(typed)
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	default:
		return 0
	}
}
