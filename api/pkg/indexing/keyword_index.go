package indexing

import (
	"sort"
	"sync"
)


type SegmentInfo struct {
	ID      string
	Content string
	Tokens  []string
	TF      map[string]float64 // term frequency
}

// keyword -> segment_id -> score

type KeywordIndex struct {
	mu       sync.RWMutex
	inverted map[string]map[string]float64 // keyword -> segment_id -> score
	segments map[string]*SegmentInfo       // segment_id -> segment info
}

func NewKeywordIndex() *KeywordIndex {
	return &KeywordIndex{
		inverted: make(map[string]map[string]float64),
		segments: make(map[string]*SegmentInfo),
	}
}

func (idx *KeywordIndex) AddSegment(segmentID, content string, tokens []string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	tf := make(map[string]float64)
	for _, token := range tokens {
		tf[token]++
	}
	total := float64(len(tokens))
	for token, count := range tf {
		tf[token] = count / total
	}

	idx.segments[segmentID] = &SegmentInfo{
		ID:      segmentID,
		Content: content,
		Tokens:  tokens,
		TF:      tf,
	}

	for token, score := range tf {
		if idx.inverted[token] == nil {
			idx.inverted[token] = make(map[string]float64)
		}
		idx.inverted[token][segmentID] = score
	}

	return nil
}

func (idx *KeywordIndex) Search(queryTokens []string, limit int) []SearchResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	scores := make(map[string]float64)
	for _, token := range queryTokens {
		if segments, exists := idx.inverted[token]; exists {
			for segmentID, score := range segments {
				scores[segmentID] += score
			}
		}
	}

	var results []SearchResult
	for segmentID, score := range scores {
		if segment, exists := idx.segments[segmentID]; exists {
			results = append(results, SearchResult{
				ID:      segmentID,
				Content: segment.Content,
				Score:   score,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > limit {
		results = results[:limit]
	}

	return results
}


type SearchResult struct {
	ID      string
	Content string
	Score   float64
}
