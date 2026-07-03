package retrieval

import (
	"context"
	"errors"
	"testing"

	"github.com/zgiai/zgi/api/pkg/tokenization"
	"github.com/zgiai/zgi/api/pkg/vectordb"
)

func TestFullTextRetrievalUsesWeaviateBM25Score(t *testing.T) {
	service := NewFullTextRetrievalService(
		tokenization.NewTokenizationService(),
		1.5,
		0.75,
		&fullTextVectorDBStub{
			results: []map[string]interface{}{
				{
					"doc_id":      "chunk-1",
					"dataset_id":  "dataset-1",
					"document_id": "document-1",
					"text":        "invoice 2026",
					"_additional": map[string]interface{}{
						"score": "7.25",
					},
				},
			},
		},
	)

	results, err := service.Search(context.Background(), "Dataset_1", "invoice", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("result count = %d, want 1", len(results))
	}
	if results[0].Score != 7.25 {
		t.Fatalf("score = %v, want 7.25", results[0].Score)
	}
	if results[0].Metadata["bm25_score"] != 7.25 {
		t.Fatalf("bm25_score = %#v, want 7.25", results[0].Metadata["bm25_score"])
	}
}

func TestFullTextRetrievalFallsBackToRankScoreWhenWeaviateScoreMissing(t *testing.T) {
	result := map[string]interface{}{
		"_additional": map[string]interface{}{},
	}

	if score := extractBM25Score(result, 2); score != 0.9 {
		t.Fatalf("fallback score = %v, want 0.9", score)
	}
}

func TestFullTextRetrievalFallsBackToLocalBM25OnWeaviateError(t *testing.T) {
	service := NewFullTextRetrievalService(
		tokenization.NewTokenizationService(),
		1.5,
		0.75,
		&fullTextVectorDBStub{err: errors.New("weaviate unavailable")},
	)
	if err := service.IndexSegment("chunk-1", "invoice 2026"); err != nil {
		t.Fatalf("IndexSegment returned error: %v", err)
	}

	results, err := service.Search(context.Background(), "Dataset_1", "invoice", SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("result count = %d, want 1", len(results))
	}
	if results[0].ID != "chunk-1" {
		t.Fatalf("id = %q, want chunk-1", results[0].ID)
	}
	if results[0].Score <= 0 {
		t.Fatalf("local BM25 score should be positive, got %v", results[0].Score)
	}
}

type fullTextVectorDBStub struct {
	vectordb.MockVectorDB
	results []map[string]interface{}
	err     error
}

func (s *fullTextVectorDBStub) SearchByFullText(ctx context.Context, className, query string, limit int) ([]map[string]interface{}, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.results, nil
}
