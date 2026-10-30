package service

import (
	"context"
	"testing"

	dataset_model "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	"github.com/zgiai/zgi/api/internal/modules/dataset/retrieval"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/pkg/embedding"
)

// The retrieval failure used to be flattened into a string, so an unrouted
// embedding model could only be reported as a generic server error.
func TestHybridRetrievalFailureKeepsEmbeddingModelIdentity(t *testing.T) {
	mockEmb := &mockEmbeddingService{err: llmerrors.NewModelNotFoundErrorWithName("text-embedding-v4")}
	vectorRetrieval := retrieval.NewVectorRetrievalService(mockEmb, &stubVectorDB{}, "")
	rs := NewRetrievalServiceWithEmbeddingFactory(nil, vectorRetrieval, nil, nil, nil, nil, nil, nil, nil,
		func(*dataset_model.Dataset) embedding.EmbeddingService { return mockEmb })

	_, err := rs.hybridVectorBM25Search(context.Background(), &dataset_model.Dataset{ID: "ds-1", WorkspaceID: "ws-1"}, "hello", 5, 0,
		&RetrievalOptions{SearchMethod: "hybrid_search", TopK: 5})
	if err == nil {
		t.Fatal("expected retrieval error")
	}
	if !IsEmbeddingModelUnavailable(err) {
		t.Fatalf("error lost the model-not-found identity: %v", err)
	}
}
