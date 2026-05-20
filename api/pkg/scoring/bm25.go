package scoring

import (
	"math"
	"sync"
)


type BM25 struct {
	k1 float64
	b  float64

	docCount    int
	totalDocLen int
	avgDocLen   float64

	idf  map[string]float64
	tf   map[string]map[string]int // docID -> token -> count
	dlen map[string]int            // docID -> docLen

	mu sync.RWMutex
}

func NewBM25(k1, b float64) *BM25 {
	return &BM25{
		k1:   k1,
		b:    b,
		idf:  make(map[string]float64),
		tf:   make(map[string]map[string]int),
		dlen: make(map[string]int),
	}
}

func (bm *BM25) AddDocument(docID string, tokens []string) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	bm.docCount++
	bm.totalDocLen += len(tokens)
	bm.avgDocLen = float64(bm.totalDocLen) / float64(bm.docCount)
	bm.dlen[docID] = len(tokens)

	if bm.tf[docID] == nil {
		bm.tf[docID] = make(map[string]int)
	}
	seen := make(map[string]bool)
	for _, token := range tokens {
		bm.tf[docID][token]++
		seen[token] = true
	}
	for token := range seen {
		df := 0
		for _, tfMap := range bm.tf {
			if tfMap[token] > 0 {
				df++
			}
		}
		bm.idf[token] = math.Log(1 + (float64(bm.docCount)-float64(df)+0.5)/(float64(df)+0.5))
	}
}

func (bm *BM25) CalculateScore(queryTokens []string, docID string) float64 {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	score := 0.0
	docLen := float64(bm.dlen[docID])
	avgDocLen := bm.avgDocLen
	for _, token := range queryTokens {
		idf := bm.idf[token]
		freq := float64(bm.tf[docID][token])
		denom := freq + bm.k1*(1-bm.b+bm.b*docLen/avgDocLen)
		if denom > 0 {
			score += idf * (freq * (bm.k1 + 1) / denom)
		}
	}
	return score
}
