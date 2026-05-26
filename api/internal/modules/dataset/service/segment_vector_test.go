package service

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
)

type segmentVectorEmbeddingService struct {
	texts []string
	err   error
}

func (s *segmentVectorEmbeddingService) EmbedText(ctx context.Context, text string) ([]float64, error) {
	vectors, err := s.EmbedTexts(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, nil
	}
	return vectors[0], nil
}

func (s *segmentVectorEmbeddingService) EmbedTexts(ctx context.Context, texts []string) ([][]float64, error) {
	s.texts = append(s.texts, texts...)
	if s.err != nil {
		return nil, s.err
	}
	vectors := make([][]float64, len(texts))
	for i := range texts {
		vectors[i] = []float64{0.1, 0.2, 0.3}
	}
	return vectors, nil
}

func (s *segmentVectorEmbeddingService) GetDimension() int { return 3 }
func (s *segmentVectorEmbeddingService) GetModel() string  { return "segment-vector-test" }

type segmentVectorDB struct {
	createdClass string
	storedID     string
	storedClass  string
	storedProps  map[string]interface{}
	storedVector []float64
	deletedID    string
	deletedClass string
}

func (s *segmentVectorDB) StoreVector(ctx context.Context, id, className string, properties map[string]interface{}, vector []float64) error {
	s.storedID = id
	s.storedClass = className
	s.storedProps = properties
	s.storedVector = vector
	return nil
}

func (s *segmentVectorDB) DeleteVector(ctx context.Context, id, className string) error {
	s.deletedID = id
	s.deletedClass = className
	return nil
}

func (s *segmentVectorDB) SearchVectors(ctx context.Context, className string, vector []float64, limit int) ([]map[string]interface{}, error) {
	return nil, nil
}

func (s *segmentVectorDB) SearchByFullText(ctx context.Context, className, query string, limit int) ([]map[string]interface{}, error) {
	return nil, nil
}

func (s *segmentVectorDB) CreateClass(ctx context.Context, className string, properties []map[string]interface{}) error {
	s.createdClass = className
	return nil
}

func (s *segmentVectorDB) HealthCheck(ctx context.Context) error { return nil }

func TestStoreSegmentVector(t *testing.T) {
	ctx := context.Background()
	vectorDB := &segmentVectorDB{}
	embeddingSvc := &segmentVectorEmbeddingService{}
	service := &segmentServiceImpl{vectorDB: vectorDB}
	dataset := &model.Dataset{ID: "dataset-1"}

	err := service.storeSegmentVectorWithEmbedding(ctx, segmentVectorTarget{
		Dataset:     dataset,
		DocumentID:  "document-1",
		IndexNodeID: "node-1",
		Content:     "updated child chunk",
	}, embeddingSvc)
	if err != nil {
		t.Fatalf("storeSegmentVector returned error: %v", err)
	}

	expectedClass := model.GenCollectionNameByID(dataset.ID)
	if vectorDB.createdClass != expectedClass {
		t.Fatalf("created class = %q, want %q", vectorDB.createdClass, expectedClass)
	}
	if vectorDB.storedID != "node-1" || vectorDB.storedClass != expectedClass {
		t.Fatalf("stored vector target = (%q, %q), want (%q, %q)", vectorDB.storedID, vectorDB.storedClass, "node-1", expectedClass)
	}
	if len(vectorDB.storedVector) != 3 {
		t.Fatalf("stored vector length = %d, want 3", len(vectorDB.storedVector))
	}
	if got := vectorDB.storedProps["text"]; got != "updated child chunk" {
		t.Fatalf("stored text = %v", got)
	}
	if got := vectorDB.storedProps["doc_id"]; got != "node-1" {
		t.Fatalf("stored doc_id = %v", got)
	}
	if got := vectorDB.storedProps["document_id"]; got != "document-1" {
		t.Fatalf("stored document_id = %v", got)
	}
	if got := vectorDB.storedProps["dataset_id"]; got != "dataset-1" {
		t.Fatalf("stored dataset_id = %v", got)
	}
	if got := vectorDB.storedProps["doc_hash"]; got == "" {
		t.Fatalf("expected doc_hash to be set")
	}
	if len(embeddingSvc.texts) != 1 || embeddingSvc.texts[0] != "updated child chunk" {
		t.Fatalf("embedded texts = %#v", embeddingSvc.texts)
	}
}

func TestDeleteSegmentVector(t *testing.T) {
	ctx := context.Background()
	vectorDB := &segmentVectorDB{}
	service := &segmentServiceImpl{vectorDB: vectorDB}

	if err := service.deleteSegmentVector(ctx, "dataset-1", "node-1"); err != nil {
		t.Fatalf("deleteSegmentVector returned error: %v", err)
	}

	expectedClass := model.GenCollectionNameByID("dataset-1")
	if vectorDB.deletedID != "node-1" || vectorDB.deletedClass != expectedClass {
		t.Fatalf("deleted vector target = (%q, %q), want (%q, %q)", vectorDB.deletedID, vectorDB.deletedClass, "node-1", expectedClass)
	}
}
