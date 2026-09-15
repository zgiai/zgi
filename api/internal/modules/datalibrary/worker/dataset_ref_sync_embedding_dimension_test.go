package worker

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	datalibModel "github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
)

func embeddingWithDimension(dimension int) *datalibModel.DocumentChunkEmbedding {
	return &datalibModel.DocumentChunkEmbedding{
		ID:                 uuid.New(),
		EmbeddingProvider:  "qwen",
		EmbeddingModel:     "text-embedding-v4",
		EmbeddingDimension: dimension,
		EmbeddingVector:    datalibModel.Float32Array(make([]float32, dimension)),
	}
}

func TestResolveStoredEmbeddingDimensionUsesSelectionDimension(t *testing.T) {
	// The asset row can still advertise an earlier embedding space; the vectors
	// selected for this sync are the source of truth.
	embeddings := map[uuid.UUID]*datalibModel.DocumentChunkEmbedding{
		uuid.New(): embeddingWithDimension(1024),
		uuid.New(): embeddingWithDimension(1024),
	}

	dimension, err := resolveStoredEmbeddingDimension(embeddings)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dimension != 1024 {
		t.Fatalf("dimension=%d, want 1024", dimension)
	}
}

func TestResolveStoredEmbeddingDimensionRejectsMixedSelection(t *testing.T) {
	embeddings := map[uuid.UUID]*datalibModel.DocumentChunkEmbedding{
		uuid.New(): embeddingWithDimension(1024),
		uuid.New(): embeddingWithDimension(3072),
	}

	_, err := resolveStoredEmbeddingDimension(embeddings)
	if err == nil {
		t.Fatal("expected an error for a mixed-dimension selection")
	}
	if !strings.Contains(err.Error(), "inconsistent embedding dimension") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestResolveStoredEmbeddingDimensionRejectsEmptySelection(t *testing.T) {
	_, err := resolveStoredEmbeddingDimension(map[uuid.UUID]*datalibModel.DocumentChunkEmbedding{})
	if err == nil {
		t.Fatal("expected an error for an empty selection")
	}

	_, err = resolveStoredEmbeddingDimension(map[uuid.UUID]*datalibModel.DocumentChunkEmbedding{
		uuid.New(): embeddingWithDimension(0),
	})
	if err == nil {
		t.Fatal("expected an error for an empty vector")
	}
}
