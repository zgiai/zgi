package service

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
	datasetrepo "github.com/zgiai/zgi/api/internal/modules/dataset/repository"
	"github.com/zgiai/zgi/api/pkg/embedding"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
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
	storeCalls   int
	deleteCalls  int
}

func (s *segmentVectorDB) StoreVector(ctx context.Context, id, className string, properties map[string]interface{}, vector []float64) error {
	s.storeCalls++
	s.storedID = id
	s.storedClass = className
	s.storedProps = properties
	s.storedVector = vector
	return nil
}

func (s *segmentVectorDB) DeleteVector(ctx context.Context, id, className string) error {
	s.deleteCalls++
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

func newSegmentVectorTestService(t *testing.T) (*segmentServiceImpl, *segmentVectorDB, *segmentVectorEmbeddingService, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	createSegmentVectorTestTables(t, db)

	vectorDB := &segmentVectorDB{}
	embeddingSvc := &segmentVectorEmbeddingService{}
	service := &segmentServiceImpl{
		chunkService: NewChunkService(datasetrepo.NewChunkRepository(db), nil, db),
		datasetRepo:  datasetrepo.NewDatasetRepository(db),
		vectorDB:     vectorDB,
		embeddingFactory: func(ctx context.Context, dataset *model.Dataset) (embedding.EmbeddingService, error) {
			return embeddingSvc, nil
		},
	}

	return service, vectorDB, embeddingSvc, db
}

func createSegmentVectorTestTables(t *testing.T, db *gorm.DB) {
	t.Helper()

	statements := []string{
		`CREATE TABLE datasets (
			id text PRIMARY KEY,
			organization_id text NOT NULL,
			workspace_id text,
			name text NOT NULL,
			description text,
			provider text NOT NULL,
			enable_graph_flow boolean,
			created_by text NOT NULL,
			created_at datetime,
			updated_by text,
			updated_at datetime,
			owner text,
			embedding_model text,
			embedding_model_provider text,
			entity_model text,
			entity_model_provider text,
			collection_binding_id text,
			retrieval_config text,
			icon_type text,
			icon text,
			icon_background text,
			process_rule text
		)`,
		`CREATE TABLE document_segments (
			id text PRIMARY KEY,
			organization_id text NOT NULL,
			dataset_id text NOT NULL,
			document_id text NOT NULL,
			position integer NOT NULL,
			content text NOT NULL,
			word_count integer NOT NULL,
			tokens integer NOT NULL,
			keywords text,
			index_node_id text,
			index_node_hash text,
			hit_count integer,
			enabled boolean,
			disabled_at datetime,
			disabled_by text,
			status text,
			graph_indexing_status text,
			created_by text NOT NULL,
			created_at datetime,
			indexing_at datetime,
			completed_at datetime,
			error text,
			stopped_at datetime,
			answer text,
			updated_by text,
			updated_at datetime,
			is_deleted boolean,
			deleted_at datetime
		)`,
		`CREATE TABLE child_chunks (
			id text PRIMARY KEY,
			organization_id text NOT NULL,
			dataset_id text NOT NULL,
			document_id text NOT NULL,
			segment_id text NOT NULL,
			position integer NOT NULL,
			content text NOT NULL,
			word_count integer NOT NULL,
			index_node_id text,
			index_node_hash text,
			type text NOT NULL,
			created_by text NOT NULL,
			created_at datetime,
			updated_by text,
			updated_at datetime,
			indexing_at datetime,
			completed_at datetime,
			error text
		)`,
	}

	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create test table: %v", err)
		}
	}
}

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

func TestCreateChildChunkStoresVector(t *testing.T) {
	ctx := context.Background()
	service, vectorDB, embeddingSvc, db := newSegmentVectorTestService(t)
	seedSegmentVectorDatasetAndSegment(t, db)

	childChunk := &model.ChildChunk{
		OrganizationID: "org-1",
		DatasetID:      "dataset-1",
		DocumentID:     "document-1",
		SegmentID:      "segment-1",
		Content:        "new child content",
		WordCount:      len([]rune("new child content")),
		Type:           model.ChildChunkTypeManual,
		CreatedBy:      "user-1",
	}

	response, err := service.CreateChildChunk(ctx, childChunk)
	if err != nil {
		t.Fatalf("CreateChildChunk returned error: %v", err)
	}

	if response.IndexNodeID == nil || *response.IndexNodeID == "" {
		t.Fatalf("expected response index node id")
	}
	if vectorDB.storeCalls != 1 {
		t.Fatalf("store calls = %d, want 1", vectorDB.storeCalls)
	}
	if vectorDB.storedID != *response.IndexNodeID {
		t.Fatalf("stored id = %q, want %q", vectorDB.storedID, *response.IndexNodeID)
	}
	if vectorDB.storedProps["text"] != "new child content" {
		t.Fatalf("stored text = %v", vectorDB.storedProps["text"])
	}
	if len(embeddingSvc.texts) != 1 || embeddingSvc.texts[0] != "new child content" {
		t.Fatalf("embedded texts = %#v", embeddingSvc.texts)
	}
}

func TestUpdateChildChunkStoresUpdatedVectorAndHash(t *testing.T) {
	ctx := context.Background()
	service, vectorDB, embeddingSvc, db := newSegmentVectorTestService(t)
	seedSegmentVectorDatasetAndSegment(t, db)
	oldHash := simpleHash("old child content")
	indexNodeID := "child-node-1"
	childChunk := &model.ChildChunk{
		ID:             "child-1",
		OrganizationID: "org-1",
		DatasetID:      "dataset-1",
		DocumentID:     "document-1",
		SegmentID:      "segment-1",
		Position:       1,
		Content:        "old child content",
		WordCount:      len([]rune("old child content")),
		Type:           model.ChildChunkTypeAutomatic,
		IndexNodeID:    &indexNodeID,
		IndexNodeHash:  &oldHash,
		CreatedBy:      "user-1",
	}
	if err := db.Create(childChunk).Error; err != nil {
		t.Fatalf("seed child chunk: %v", err)
	}

	childChunk.Content = "updated child content"
	childChunk.WordCount = len([]rune(childChunk.Content))
	response, err := service.UpdateChildChunk(ctx, childChunk)
	if err != nil {
		t.Fatalf("UpdateChildChunk returned error: %v", err)
	}

	if vectorDB.storeCalls != 1 {
		t.Fatalf("store calls = %d, want 1", vectorDB.storeCalls)
	}
	if vectorDB.deleteCalls != 1 {
		t.Fatalf("delete calls = %d, want 1", vectorDB.deleteCalls)
	}
	if vectorDB.deletedID != indexNodeID {
		t.Fatalf("deleted id = %q, want %q", vectorDB.deletedID, indexNodeID)
	}
	if vectorDB.storedID != indexNodeID {
		t.Fatalf("stored id = %q, want %q", vectorDB.storedID, indexNodeID)
	}
	if vectorDB.storedProps["text"] != "updated child content" {
		t.Fatalf("stored text = %v", vectorDB.storedProps["text"])
	}
	if response.IndexNodeHash == nil || *response.IndexNodeHash != simpleHash("updated child content") {
		t.Fatalf("response hash = %v", response.IndexNodeHash)
	}
	if len(embeddingSvc.texts) != 1 || embeddingSvc.texts[0] != "updated child content" {
		t.Fatalf("embedded texts = %#v", embeddingSvc.texts)
	}
}

func TestDeleteChildChunkDeletesVector(t *testing.T) {
	ctx := context.Background()
	service, vectorDB, _, db := newSegmentVectorTestService(t)
	seedSegmentVectorDatasetAndSegment(t, db)
	hash := simpleHash("child content")
	indexNodeID := "child-node-1"
	childChunk := &model.ChildChunk{
		ID:             "child-1",
		OrganizationID: "org-1",
		DatasetID:      "dataset-1",
		DocumentID:     "document-1",
		SegmentID:      "segment-1",
		Position:       1,
		Content:        "child content",
		WordCount:      len([]rune("child content")),
		Type:           model.ChildChunkTypeAutomatic,
		IndexNodeID:    &indexNodeID,
		IndexNodeHash:  &hash,
		CreatedBy:      "user-1",
	}
	if err := db.Create(childChunk).Error; err != nil {
		t.Fatalf("seed child chunk: %v", err)
	}

	if err := service.DeleteChildChunk(ctx, "child-1"); err != nil {
		t.Fatalf("DeleteChildChunk returned error: %v", err)
	}

	if vectorDB.deleteCalls != 1 {
		t.Fatalf("delete calls = %d, want 1", vectorDB.deleteCalls)
	}
	if vectorDB.deletedID != indexNodeID {
		t.Fatalf("deleted id = %q, want %q", vectorDB.deletedID, indexNodeID)
	}
	var count int64
	if err := db.Model(&model.ChildChunk{}).Where("id = ?", "child-1").Count(&count).Error; err != nil {
		t.Fatalf("count child chunks: %v", err)
	}
	if count != 0 {
		t.Fatalf("child chunk count = %d, want 0", count)
	}
}

func seedSegmentVectorDatasetAndSegment(t *testing.T, db *gorm.DB) {
	t.Helper()

	dataset := &model.Dataset{
		ID:             "dataset-1",
		OrganizationID: "org-1",
		WorkspaceID:    "workspace-1",
		Name:           "Dataset",
		Provider:       "vendor",
		CreatedBy:      "user-1",
	}
	if err := db.Create(dataset).Error; err != nil {
		t.Fatalf("seed dataset: %v", err)
	}
	segment := &model.DocumentSegment{
		ID:             "segment-1",
		OrganizationID: "org-1",
		DatasetID:      "dataset-1",
		DocumentID:     "document-1",
		Position:       1,
		Content:        "parent content",
		WordCount:      len([]rune("parent content")),
		Tokens:         len([]rune("parent content")),
		Status:         model.SegmentStatusCompleted,
		Enabled:        true,
		CreatedBy:      "user-1",
	}
	if err := db.Create(segment).Error; err != nil {
		t.Fatalf("seed segment: %v", err)
	}
}
