package worker

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	datalibModel "github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	datalibRepo "github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
	datasetModel "github.com/zgiai/zgi/api/internal/modules/dataset/model"
)

func TestDatasetRefSyncRunnerCopiesReadyAssetToDataset(t *testing.T) {
	refID := uuid.New()
	assetID := uuid.New()
	chunkID := uuid.New()
	syncRunID := uuid.New()
	provider := "openai"
	embeddingModel := "text-embedding-3-small"
	refStore := &fakeDatasetRefSyncRefStore{
		ref: &datalibModel.KnowledgeBaseAssetRef{
			ID:             refID,
			OrganizationID: "org-1",
			DatasetID:      "dataset-1",
			AssetID:        assetID,
			SyncRunID:      &syncRunID,
			CreatedBy:      "user-1",
		},
	}
	assetStore := &fakeDatasetRefSyncAssetStore{
		asset: &datalibModel.DocumentAsset{
			ID:                assetID,
			OrganizationID:    "org-1",
			Title:             "Asset A",
			SourceFileID:      "file-1",
			ProductStatus:     datalibModel.DocumentAssetProductStatusReady,
			VectorStatus:      datalibModel.DocumentAssetVectorStatusReady,
			GenerationNo:      5,
			CreatedBy:         "user-1",
			EmbeddingProvider: &provider,
			EmbeddingModel:    &embeddingModel,
		},
	}
	documentStore := &fakeDatasetRefSyncDocumentStore{}
	vectorStore := &fakeDatasetRefSyncVectorStore{}
	runner := NewDatasetRefSyncRunner(DatasetRefSyncRunnerDeps{
		Refs:      refStore,
		Assets:    assetStore,
		Datasets:  &fakeDatasetRefSyncDatasetStore{dataset: &datasetModel.Dataset{ID: "dataset-1", OrganizationID: "org-1"}},
		Documents: documentStore,
		Chunks: &fakeDatasetRefSyncChunkStore{chunks: []*datalibModel.DocumentChunk{{
			ID:             chunkID,
			OrganizationID: "org-1",
			AssetID:        assetID,
			GenerationNo:   5,
			Position:       1,
			ChunkType:      datalibModel.DocumentChunkTypeAuto,
			Content:        "hello dataset",
			Enabled:        true,
			Status:         datalibModel.DocumentChunkStatusReady,
		}}},
		Embeddings: &fakeDatasetRefSyncEmbeddingStore{embeddings: []*datalibModel.DocumentChunkEmbedding{{
			ID:                uuid.New(),
			OrganizationID:    "org-1",
			AssetID:           assetID,
			ChunkID:           chunkID,
			GenerationNo:      5,
			EmbeddingProvider: provider,
			EmbeddingModel:    embeddingModel,
			EmbeddingVector:   datalibModel.Float32Array{0.1, 0.2, 0.3},
			Status:            datalibModel.DocumentChunkEmbeddingStatusReady,
		}}},
		VectorDB: vectorStore,
	})

	err := runner.Run(context.Background(), DatasetRefSyncPayload{
		RefID:        refID.String(),
		AssetID:      assetID.String(),
		DatasetID:    "dataset-1",
		GenerationNo: 5,
		SyncRunID:    syncRunID.String(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if refStore.markSyncingID != refID || refStore.syncedDocumentID == uuid.Nil || refStore.failedCode != "" {
		t.Fatalf("syncing_id=%s synced=%s failed=%s", refStore.markSyncingID, refStore.syncedDocumentID, refStore.failedCode)
	}
	if len(documentStore.createdDocuments) != 1 || !documentStore.createdDocuments[0].Enabled || documentStore.createdDocuments[0].IndexingStatus != datasetModel.DocumentStatusCompleted {
		t.Fatalf("created documents = %#v", documentStore.createdDocuments)
	}
	if len(documentStore.createdSegments) != 1 || documentStore.createdSegments[0].Content != "hello dataset" || !documentStore.createdSegments[0].Enabled {
		t.Fatalf("created segments = %#v", documentStore.createdSegments)
	}
	if len(vectorStore.storedIDs) != 1 || len(vectorStore.storedVectors[0]) != 3 {
		t.Fatalf("stored vectors ids=%v vectors=%v", vectorStore.storedIDs, vectorStore.storedVectors)
	}
}

func TestDatasetRefSyncRunnerSkipsStaleSyncRun(t *testing.T) {
	refID := uuid.New()
	assetID := uuid.New()
	currentSyncRunID := uuid.New()
	staleSyncRunID := uuid.New()
	refStore := &fakeDatasetRefSyncRefStore{
		ref: &datalibModel.KnowledgeBaseAssetRef{
			ID:             refID,
			OrganizationID: "org-1",
			DatasetID:      "dataset-1",
			AssetID:        assetID,
			SyncRunID:      &currentSyncRunID,
		},
	}
	runner := NewDatasetRefSyncRunner(DatasetRefSyncRunnerDeps{Refs: refStore, Assets: &fakeDatasetRefSyncAssetStore{}})

	err := runner.Run(context.Background(), DatasetRefSyncPayload{
		RefID:        refID.String(),
		AssetID:      assetID.String(),
		DatasetID:    "dataset-1",
		GenerationNo: 5,
		SyncRunID:    staleSyncRunID.String(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if refStore.markSyncingID != uuid.Nil || refStore.failedCode != "" {
		t.Fatalf("unexpected writes syncing=%s failed=%s", refStore.markSyncingID, refStore.failedCode)
	}
}

func TestDatasetRefSyncRunnerMarksFailedWhenAssetNotReady(t *testing.T) {
	refID := uuid.New()
	assetID := uuid.New()
	syncRunID := uuid.New()
	refStore := &fakeDatasetRefSyncRefStore{
		ref: &datalibModel.KnowledgeBaseAssetRef{
			ID:             refID,
			OrganizationID: "org-1",
			DatasetID:      "dataset-1",
			AssetID:        assetID,
			SyncRunID:      &syncRunID,
		},
	}
	assetStore := &fakeDatasetRefSyncAssetStore{
		asset: &datalibModel.DocumentAsset{
			ID:             assetID,
			OrganizationID: "org-1",
			ProductStatus:  datalibModel.DocumentAssetProductStatusGenerating,
			VectorStatus:   datalibModel.DocumentAssetVectorStatusIndexing,
			GenerationNo:   5,
		},
	}
	runner := NewDatasetRefSyncRunner(DatasetRefSyncRunnerDeps{Refs: refStore, Assets: assetStore})

	err := runner.Run(context.Background(), DatasetRefSyncPayload{
		RefID:        refID.String(),
		AssetID:      assetID.String(),
		DatasetID:    "dataset-1",
		GenerationNo: 5,
		SyncRunID:    syncRunID.String(),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if refStore.failedCode != "asset_not_ready" || refStore.markSyncingID != uuid.Nil {
		t.Fatalf("failed=%s syncing=%s", refStore.failedCode, refStore.markSyncingID)
	}
}

type fakeDatasetRefSyncRefStore struct {
	ref              *datalibModel.KnowledgeBaseAssetRef
	markSyncingID    uuid.UUID
	markSyncingRunID uuid.UUID
	syncedDocumentID uuid.UUID
	syncedGeneration int64
	failedCode       string
	failedMessage    string
}

func (f *fakeDatasetRefSyncRefStore) GetByID(ctx context.Context, id uuid.UUID) (*datalibModel.KnowledgeBaseAssetRef, error) {
	if f.ref != nil && f.ref.ID == id {
		return f.ref, nil
	}
	return nil, nil
}

func (f *fakeDatasetRefSyncRefStore) MarkSyncing(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID) (*datalibModel.KnowledgeBaseAssetRef, error) {
	f.markSyncingID = id
	f.markSyncingRunID = syncRunID
	return f.ref, nil
}

func (f *fakeDatasetRefSyncRefStore) MarkSynced(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, datasetDocumentID uuid.UUID, generationNo int64, syncedAt time.Time) (*datalibModel.KnowledgeBaseAssetRef, error) {
	f.syncedDocumentID = datasetDocumentID
	f.syncedGeneration = generationNo
	return f.ref, nil
}

func (f *fakeDatasetRefSyncRefStore) MarkFailed(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, errorCode, errorMessage string) (*datalibModel.KnowledgeBaseAssetRef, error) {
	f.failedCode = errorCode
	f.failedMessage = errorMessage
	return f.ref, nil
}

type fakeDatasetRefSyncAssetStore struct {
	asset *datalibModel.DocumentAsset
}

func (f *fakeDatasetRefSyncAssetStore) GetAssetByID(ctx context.Context, id uuid.UUID) (*datalibModel.DocumentAsset, error) {
	if f.asset != nil && f.asset.ID == id {
		return f.asset, nil
	}
	return nil, nil
}

type fakeDatasetRefSyncDatasetStore struct {
	dataset *datasetModel.Dataset
}

func (f *fakeDatasetRefSyncDatasetStore) GetByID(ctx context.Context, id string) (*datasetModel.Dataset, error) {
	if f.dataset != nil && f.dataset.ID == id {
		return f.dataset, nil
	}
	return nil, nil
}

type fakeDatasetRefSyncDocumentStore struct {
	createdDocuments []*datasetModel.Document
	createdSegments  []*datasetModel.DocumentSegment
	createdChildren  []*datasetModel.ChildChunk
	disabledIDs      []string
	deletedIDs       []string
}

func (f *fakeDatasetRefSyncDocumentStore) GetByID(ctx context.Context, id string) (*datasetModel.Document, error) {
	for _, document := range f.createdDocuments {
		if document.ID == id {
			return document, nil
		}
	}
	return nil, nil
}

func (f *fakeDatasetRefSyncDocumentStore) GetNextPosition(ctx context.Context, datasetID string) (int, error) {
	return len(f.createdDocuments) + 1, nil
}

func (f *fakeDatasetRefSyncDocumentStore) Create(ctx context.Context, document *datasetModel.Document) error {
	f.createdDocuments = append(f.createdDocuments, document)
	return nil
}

func (f *fakeDatasetRefSyncDocumentStore) Update(ctx context.Context, document *datasetModel.Document) error {
	return nil
}

func (f *fakeDatasetRefSyncDocumentStore) EnableDocuments(ctx context.Context, datasetID string, documentIDs []string) error {
	for _, document := range f.createdDocuments {
		for _, id := range documentIDs {
			if document.ID == id {
				document.Enabled = true
			}
		}
	}
	for _, segment := range f.createdSegments {
		for _, id := range documentIDs {
			if segment.DocumentID == id {
				segment.Enabled = true
			}
		}
	}
	return nil
}

func (f *fakeDatasetRefSyncDocumentStore) DisableDocuments(ctx context.Context, datasetID string, documentIDs []string, accountID string) error {
	f.disabledIDs = append(f.disabledIDs, documentIDs...)
	return nil
}

func (f *fakeDatasetRefSyncDocumentStore) GetSegmentsByDocumentID(ctx context.Context, documentID string) ([]*datasetModel.DocumentSegment, error) {
	var out []*datasetModel.DocumentSegment
	for _, segment := range f.createdSegments {
		if segment.DocumentID == documentID {
			out = append(out, segment)
		}
	}
	return out, nil
}

func (f *fakeDatasetRefSyncDocumentStore) GetChildChunksBySegmentID(ctx context.Context, segmentID string) ([]datasetModel.ChildChunk, error) {
	var out []datasetModel.ChildChunk
	for _, child := range f.createdChildren {
		if child.SegmentID == segmentID {
			out = append(out, *child)
		}
	}
	return out, nil
}

func (f *fakeDatasetRefSyncDocumentStore) CreateDocumentSegment(ctx context.Context, segment *datasetModel.DocumentSegment) error {
	f.createdSegments = append(f.createdSegments, segment)
	return nil
}

func (f *fakeDatasetRefSyncDocumentStore) CreateChildChunk(ctx context.Context, childChunk *datasetModel.ChildChunk) error {
	f.createdChildren = append(f.createdChildren, childChunk)
	return nil
}

func (f *fakeDatasetRefSyncDocumentStore) DeleteDocumentSegmentQuestionsByDocumentID(ctx context.Context, documentID string) error {
	return nil
}

func (f *fakeDatasetRefSyncDocumentStore) DeleteChildChunksByDocumentID(ctx context.Context, documentID string) error {
	return nil
}

func (f *fakeDatasetRefSyncDocumentStore) DeleteDocumentSegmentsByDocumentID(ctx context.Context, documentID string) error {
	return nil
}

func (f *fakeDatasetRefSyncDocumentStore) Delete(ctx context.Context, id string) error {
	f.deletedIDs = append(f.deletedIDs, id)
	return nil
}

type fakeDatasetRefSyncChunkStore struct {
	chunks []*datalibModel.DocumentChunk
}

func (f *fakeDatasetRefSyncChunkStore) List(ctx context.Context, filter datalibRepo.DocumentChunkListFilter) ([]*datalibModel.DocumentChunk, int64, error) {
	return f.chunks, int64(len(f.chunks)), nil
}

type fakeDatasetRefSyncEmbeddingStore struct {
	embeddings []*datalibModel.DocumentChunkEmbedding
}

func (f *fakeDatasetRefSyncEmbeddingStore) List(ctx context.Context, filter datalibRepo.DocumentChunkEmbeddingListFilter) ([]*datalibModel.DocumentChunkEmbedding, int64, error) {
	return f.embeddings, int64(len(f.embeddings)), nil
}

type fakeDatasetRefSyncVectorStore struct {
	storedIDs      []string
	storedVectors  [][]float64
	deletedIDs     []string
	createdClasses []string
}

func (f *fakeDatasetRefSyncVectorStore) StoreVector(ctx context.Context, id, className string, properties map[string]interface{}, vector []float64) error {
	f.storedIDs = append(f.storedIDs, id)
	f.storedVectors = append(f.storedVectors, vector)
	return nil
}

func (f *fakeDatasetRefSyncVectorStore) DeleteVector(ctx context.Context, id, className string) error {
	f.deletedIDs = append(f.deletedIDs, id)
	return nil
}

func (f *fakeDatasetRefSyncVectorStore) SearchVectors(ctx context.Context, className string, vector []float64, limit int) ([]map[string]interface{}, error) {
	return nil, nil
}

func (f *fakeDatasetRefSyncVectorStore) SearchByFullText(ctx context.Context, className, query string, limit int) ([]map[string]interface{}, error) {
	return nil, nil
}

func (f *fakeDatasetRefSyncVectorStore) CreateClass(ctx context.Context, className string, properties []map[string]interface{}) error {
	f.createdClasses = append(f.createdClasses, className)
	return nil
}

func (f *fakeDatasetRefSyncVectorStore) HealthCheck(ctx context.Context) error {
	return nil
}
