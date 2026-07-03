package retrieval

import (
	"math"
	"testing"
)

func TestFuseVectorBM25ResultsFusesVectorAndBM25Ranks(t *testing.T) {
	vectorResults := []SearchResult{
		{
			ID:      "vector-only",
			Content: "semantic match",
			Score:   0.99,
			Metadata: map[string]interface{}{
				"doc_id": "vector-only",
			},
		},
		{
			ID:      "both",
			Content: "invoice 2026 semantic and lexical match",
			Score:   0.88,
			Metadata: map[string]interface{}{
				"doc_id": "both",
			},
		},
	}
	bm25Results := []SearchResult{
		{
			ID:      "both",
			Content: "invoice 2026 semantic and lexical match",
			Score:   4.2,
			Metadata: map[string]interface{}{
				"document_id": "doc-both",
			},
		},
		{
			ID:      "bm25-only",
			Content: "invoice 2026 lexical match",
			Score:   4.0,
		},
	}

	results := FuseVectorBM25Results("invoice 2026", vectorResults, bm25Results, 10, HybridFusionConfig{
		RRFK:         60,
		VectorWeight: 0.6,
		BM25Weight:   0.4,
	})

	if len(results) != 3 {
		t.Fatalf("expected 3 fused results, got %d", len(results))
	}
	if results[0].ID != "both" {
		t.Fatalf("expected high-ranked vector+BM25 candidate to rank first by pure RRF, got %#v", results)
	}
	wantScore := 0.6/62 + 0.4/61
	if math.Abs(results[0].Score-wantScore) > 1e-12 {
		t.Fatalf("score = %v, want pure RRF score %v", results[0].Score, wantScore)
	}

	metadata := results[0].Metadata
	if metadata["vector_score"] != 0.88 {
		t.Fatalf("vector_score = %#v, want 0.88", metadata["vector_score"])
	}
	if metadata["bm25_score"] != 4.2 {
		t.Fatalf("bm25_score = %#v, want 4.2", metadata["bm25_score"])
	}
	if metadata["fusion_score"] != results[0].Score {
		t.Fatalf("fusion_score = %#v, want %v", metadata["fusion_score"], results[0].Score)
	}
	if metadata["vector_rank"] != 2 || metadata["bm25_rank"] != 1 || metadata["best_rank"] != 1 {
		t.Fatalf("unexpected rank metadata: %#v", metadata)
	}
	if got := metadata["retrieval_sources"]; !sameStrings(got, []string{"vector", "bm25"}) {
		t.Fatalf("retrieval_sources = %#v, want vector+bm25", got)
	}
	if got := metadata["matched_terms"]; !sameStrings(got, []string{"invoice", "2026"}) {
		t.Fatalf("matched_terms = %#v, want invoice+2026", got)
	}
	if metadata["doc_id"] != "both" || metadata["document_id"] != "doc-both" {
		t.Fatalf("expected metadata from both sources to be preserved, got %#v", metadata)
	}
}

func TestFuseVectorBM25ResultsUsesStableTieBreakers(t *testing.T) {
	vectorResults := []SearchResult{
		{ID: "b", Content: "same", Score: 0.5},
		{ID: "a", Content: "same", Score: 0.5},
	}

	first := FuseVectorBM25Results("same", vectorResults, nil, 10, HybridFusionConfig{})
	second := FuseVectorBM25Results("same", vectorResults, nil, 10, HybridFusionConfig{})

	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("unexpected result counts: %d %d", len(first), len(second))
	}
	if first[0].ID != second[0].ID || first[1].ID != second[1].ID {
		t.Fatalf("ordering changed between runs: %#v vs %#v", first, second)
	}
	if first[0].ID != "b" || first[1].ID != "a" {
		t.Fatalf("expected rank order to be stable for single-source ties, got %#v", first)
	}
}

func TestFuseVectorBM25ResultsDoesNotUseRawScoreAsCrossSourceTieBreaker(t *testing.T) {
	vectorResults := []SearchResult{
		{ID: "a-vector", Content: "same", Score: 0.1},
	}
	bm25Results := []SearchResult{
		{ID: "z-bm25", Content: "same", Score: 999},
	}

	results := FuseVectorBM25Results("same", vectorResults, bm25Results, 10, HybridFusionConfig{
		RRFK:         60,
		VectorWeight: 1,
		BM25Weight:   1,
	})

	if len(results) != 2 {
		t.Fatalf("result count = %d, want 2", len(results))
	}
	if results[0].ID != "a-vector" {
		t.Fatalf("expected ID tie-breaker after equal RRF/source/rank, got %#v", results)
	}
}

func TestFuseVectorBM25ResultsDoesNotTruncateWhenLimitIsZero(t *testing.T) {
	vectorResults := []SearchResult{
		{ID: "v1", Content: "vector one", Score: 0.9},
		{ID: "shared", Content: "shared", Score: 0.8},
	}
	bm25Results := []SearchResult{
		{ID: "shared", Content: "shared", Score: 3.0},
		{ID: "b1", Content: "bm25 one", Score: 2.0},
	}

	results := FuseVectorBM25Results("shared", vectorResults, bm25Results, 0, HybridFusionConfig{
		RRFK:         60,
		VectorWeight: 0.6,
		BM25Weight:   0.4,
	})

	if len(results) != 3 {
		t.Fatalf("result count = %d, want all 3 deduped candidates", len(results))
	}
}

func sameStrings(value interface{}, want []string) bool {
	got, ok := value.([]string)
	if !ok || len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
