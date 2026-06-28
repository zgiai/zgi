package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
)

func TestFileAssetVectorIndexServiceEnsureAssetIndexedFiltersCurrentAssetEmbeddingModel(t *testing.T) {
	assetID := uuid.New()
	parentID := uuid.New()
	chunkID := uuid.New()
	provider := "qwen"
	modelName := "text-embedding-v4"
	chunk := &model.DocumentChunk{
		ID:             chunkID,
		OrganizationID: "org-1",
		AssetID:        assetID,
		GenerationNo:   3,
		ParentChunkID:  &parentID,
		ChunkType:      model.DocumentChunkTypeChild,
		Content:        "简介 人参为五加科人参属多年生草本植物",
		ContentHash:    "hash-1",
		Enabled:        true,
		Status:         model.DocumentChunkStatusReady,
	}
	embeddingRepo := &fileAssetVectorIndexEmbeddingRepo{items: []*model.DocumentChunkEmbedding{
		{
			ID:                uuid.New(),
			OrganizationID:    "org-1",
			AssetID:           assetID,
			ChunkID:           chunkID,
			GenerationNo:      3,
			EmbeddingProvider: "qwen",
			EmbeddingModel:    "text-embedding-v3",
			EmbeddingVector:   model.Float32Array{3, 3},
			Status:            model.DocumentChunkEmbeddingStatusReady,
		},
		{
			ID:                uuid.New(),
			OrganizationID:    "org-1",
			AssetID:           assetID,
			ChunkID:           chunkID,
			GenerationNo:      3,
			EmbeddingProvider: provider,
			EmbeddingModel:    modelName,
			EmbeddingVector:   model.Float32Array{4, 4},
			Status:            model.DocumentChunkEmbeddingStatusReady,
		},
	}}
	vectorDB := &fileAssetVectorIndexFieldDeleteDB{}
	svc := &fileAssetVectorIndexService{
		chunks: &fileAssetVectorIndexChunkRepo{items: []*model.DocumentChunk{
			{
				ID:             parentID,
				OrganizationID: "org-1",
				AssetID:        assetID,
				GenerationNo:   3,
				ChunkType:      model.DocumentChunkTypeParent,
				Content:        "parent",
				Enabled:        true,
				Status:         model.DocumentChunkStatusReady,
			},
			chunk,
		}},
		embeddings: embeddingRepo,
		vectorDB:   vectorDB,
	}

	err := svc.EnsureAssetIndexed(context.Background(), &model.DocumentAsset{
		ID:                assetID,
		OrganizationID:    "org-1",
		GenerationNo:      3,
		EmbeddingProvider: &provider,
		EmbeddingModel:    &modelName,
	})
	if err != nil {
		t.Fatalf("EnsureAssetIndexed: %v", err)
	}
	if embeddingRepo.lastFilter.EmbeddingProvider != provider ||
		embeddingRepo.lastFilter.EmbeddingModel != modelName {
		t.Fatalf("embedding filter provider=%q model=%q, want %q/%q",
			embeddingRepo.lastFilter.EmbeddingProvider,
			embeddingRepo.lastFilter.EmbeddingModel,
			provider,
			modelName,
		)
	}
	if len(vectorDB.storedVectorIDs) != 1 || vectorDB.storedVectorIDs[0] != chunkID.String() {
		t.Fatalf("stored vector ids=%v", vectorDB.storedVectorIDs)
	}
	if len(vectorDB.storedVectors) != 1 || len(vectorDB.storedVectors[0]) != 2 ||
		vectorDB.storedVectors[0][0] != 4 || vectorDB.storedVectors[0][1] != 4 {
		t.Fatalf("stored vectors=%v", vectorDB.storedVectors)
	}
}

func TestFileAssetVectorIndexServiceDeleteChildVectorsByParentUsesFieldBatchDelete(t *testing.T) {
	assetID := uuid.New()
	parentID := uuid.New()
	vectorDB := &fileAssetVectorIndexFieldDeleteDB{}
	chunkRepo := &fileAssetVectorIndexChunkRepo{listErr: errors.New("should not list children")}
	svc := &fileAssetVectorIndexService{
		chunks:   chunkRepo,
		vectorDB: vectorDB,
	}

	err := svc.DeleteChildVectorsByParent(context.Background(), &model.DocumentAsset{
		ID:             assetID,
		OrganizationID: "org-1",
		GenerationNo:   3,
	}, parentID)
	if err != nil {
		t.Fatalf("DeleteChildVectorsByParent: %v", err)
	}
	if chunkRepo.listCalled {
		t.Fatalf("expected batch field delete to avoid listing child chunks")
	}
	if vectorDB.deletedClass != FileAssetVectorCollectionName(assetID) ||
		vectorDB.deletedField != "document_id" ||
		vectorDB.deletedValue != parentID.String() {
		t.Fatalf("delete by field = class:%q field:%q value:%q", vectorDB.deletedClass, vectorDB.deletedField, vectorDB.deletedValue)
	}
	if vectorDB.deleteVectorCalls != 0 {
		t.Fatalf("DeleteVector calls = %d, want 0", vectorDB.deleteVectorCalls)
	}
}

func TestFileAssetVectorIndexServiceDeleteChildVectorsByParentFallsBackWhenFieldBatchDeleteFails(t *testing.T) {
	assetID := uuid.New()
	parentID := uuid.New()
	firstChildID := uuid.New()
	secondChildID := uuid.New()
	vectorDB := &fileAssetVectorIndexFieldDeleteDB{
		deleteFieldErr: errors.New("weaviate delete returned status code: 500"),
	}
	chunkRepo := &fileAssetVectorIndexChunkRepo{items: []*model.DocumentChunk{
		{
			ID:             firstChildID,
			OrganizationID: "org-1",
			AssetID:        assetID,
			GenerationNo:   3,
			ParentChunkID:  &parentID,
			ChunkType:      model.DocumentChunkTypeChild,
		},
		{
			ID:             secondChildID,
			OrganizationID: "org-1",
			AssetID:        assetID,
			GenerationNo:   3,
			ParentChunkID:  &parentID,
			ChunkType:      model.DocumentChunkTypeChild,
		},
	}}
	svc := &fileAssetVectorIndexService{
		chunks:   chunkRepo,
		vectorDB: vectorDB,
	}

	err := svc.DeleteChildVectorsByParent(context.Background(), &model.DocumentAsset{
		ID:             assetID,
		OrganizationID: "org-1",
		GenerationNo:   3,
	}, parentID)
	if err != nil {
		t.Fatalf("DeleteChildVectorsByParent: %v", err)
	}
	if !chunkRepo.listCalled {
		t.Fatalf("expected fallback to list child chunks")
	}
	if len(vectorDB.deletedVectorIDs) != 2 ||
		vectorDB.deletedVectorIDs[0] != firstChildID.String() ||
		vectorDB.deletedVectorIDs[1] != secondChildID.String() {
		t.Fatalf("deleted vector ids = %v", vectorDB.deletedVectorIDs)
	}
}

type fileAssetVectorIndexFieldDeleteDB struct {
	deletedClass      string
	deletedField      string
	deletedValue      string
	deleteVectorCalls int
	deleteFieldErr    error
	deletedVectorIDs  []string
	storedVectorIDs   []string
	storedVectors     [][]float64
}

func (f *fileAssetVectorIndexFieldDeleteDB) StoreVector(ctx context.Context, id, className string, properties map[string]interface{}, vector []float64) error {
	f.storedVectorIDs = append(f.storedVectorIDs, id)
	f.storedVectors = append(f.storedVectors, append([]float64(nil), vector...))
	return nil
}

func (f *fileAssetVectorIndexFieldDeleteDB) DeleteVector(ctx context.Context, id, className string) error {
	f.deleteVectorCalls++
	f.deletedVectorIDs = append(f.deletedVectorIDs, id)
	return nil
}

func (f *fileAssetVectorIndexFieldDeleteDB) DeleteObjectsByField(ctx context.Context, className, fieldName, fieldValue string) error {
	f.deletedClass = className
	f.deletedField = fieldName
	f.deletedValue = fieldValue
	return f.deleteFieldErr
}

func (f *fileAssetVectorIndexFieldDeleteDB) SearchVectors(ctx context.Context, className string, vector []float64, limit int) ([]map[string]interface{}, error) {
	return nil, nil
}

func (f *fileAssetVectorIndexFieldDeleteDB) SearchByFullText(ctx context.Context, className, query string, limit int) ([]map[string]interface{}, error) {
	return nil, nil
}

func (f *fileAssetVectorIndexFieldDeleteDB) CreateClass(ctx context.Context, className string, properties []map[string]interface{}) error {
	return nil
}

func (f *fileAssetVectorIndexFieldDeleteDB) HealthCheck(ctx context.Context) error {
	return nil
}

type fileAssetVectorIndexChunkRepo struct {
	listCalled bool
	listErr    error
	items      []*model.DocumentChunk
}

func (r *fileAssetVectorIndexChunkRepo) Create(ctx context.Context, item *model.DocumentChunk) error {
	return nil
}

func (r *fileAssetVectorIndexChunkRepo) CreateBatch(ctx context.Context, items []*model.DocumentChunk) error {
	return nil
}

func (r *fileAssetVectorIndexChunkRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.DocumentChunk, error) {
	return nil, nil
}

func (r *fileAssetVectorIndexChunkRepo) ListByIDs(ctx context.Context, organizationID string, ids []uuid.UUID) ([]*model.DocumentChunk, error) {
	allowed := map[uuid.UUID]struct{}{}
	for _, id := range ids {
		allowed[id] = struct{}{}
	}
	out := make([]*model.DocumentChunk, 0, len(r.items))
	for _, item := range r.items {
		if item == nil || item.OrganizationID != organizationID {
			continue
		}
		if _, ok := allowed[item.ID]; ok {
			out = append(out, item)
		}
	}
	return out, nil
}

func (r *fileAssetVectorIndexChunkRepo) List(ctx context.Context, filter repository.DocumentChunkListFilter) ([]*model.DocumentChunk, int64, error) {
	r.listCalled = true
	if r.listErr != nil {
		return nil, 0, r.listErr
	}
	return r.items, int64(len(r.items)), nil
}

func (r *fileAssetVectorIndexChunkRepo) CountByAssetGeneration(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64) (int64, error) {
	return 0, nil
}

func (r *fileAssetVectorIndexChunkRepo) CountByAssetGenerationAndTypes(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64, chunkTypes []string) (int64, error) {
	return 0, nil
}

func (r *fileAssetVectorIndexChunkRepo) DeleteByAsset(ctx context.Context, organizationID string, assetID uuid.UUID) error {
	return nil
}

func (r *fileAssetVectorIndexChunkRepo) DeleteByAssetGeneration(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64) error {
	return nil
}

func (r *fileAssetVectorIndexChunkRepo) DeleteChildrenByParent(ctx context.Context, organizationID string, parentChunkID uuid.UUID) error {
	return nil
}

func (r *fileAssetVectorIndexChunkRepo) Update(ctx context.Context, id uuid.UUID, patch repository.DocumentChunkPatch) (*model.DocumentChunk, error) {
	return nil, nil
}

func (r *fileAssetVectorIndexChunkRepo) UpdateEnabledByIDs(ctx context.Context, organizationID string, ids []uuid.UUID, enabled bool, updatedBy string) ([]*model.DocumentChunk, error) {
	return nil, nil
}

func (r *fileAssetVectorIndexChunkRepo) UpdateEnabledByParentIDs(ctx context.Context, organizationID string, parentIDs []uuid.UUID, enabled bool, updatedBy string) (int64, error) {
	return 0, nil
}

var _ repository.DocumentChunkRepository = (*fileAssetVectorIndexChunkRepo)(nil)

type fileAssetVectorIndexEmbeddingRepo struct {
	items      []*model.DocumentChunkEmbedding
	lastFilter repository.DocumentChunkEmbeddingListFilter
}

func (r *fileAssetVectorIndexEmbeddingRepo) Create(ctx context.Context, item *model.DocumentChunkEmbedding) error {
	r.items = append(r.items, item)
	return nil
}

func (r *fileAssetVectorIndexEmbeddingRepo) Upsert(ctx context.Context, item *model.DocumentChunkEmbedding) error {
	r.items = append(r.items, item)
	return nil
}

func (r *fileAssetVectorIndexEmbeddingRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.DocumentChunkEmbedding, error) {
	for _, item := range r.items {
		if item != nil && item.ID == id {
			return item, nil
		}
	}
	return nil, nil
}

func (r *fileAssetVectorIndexEmbeddingRepo) FindByChunkModel(ctx context.Context, chunkID uuid.UUID, provider string, embeddingModel string) (*model.DocumentChunkEmbedding, error) {
	for _, item := range r.items {
		if item != nil && item.ChunkID == chunkID && item.EmbeddingProvider == provider && item.EmbeddingModel == embeddingModel {
			return item, nil
		}
	}
	return nil, nil
}

func (r *fileAssetVectorIndexEmbeddingRepo) List(ctx context.Context, filter repository.DocumentChunkEmbeddingListFilter) ([]*model.DocumentChunkEmbedding, int64, error) {
	r.lastFilter = filter
	out := make([]*model.DocumentChunkEmbedding, 0, len(r.items))
	for _, item := range r.items {
		if item == nil {
			continue
		}
		if filter.OrganizationID != "" && item.OrganizationID != filter.OrganizationID {
			continue
		}
		if filter.AssetID != uuid.Nil && item.AssetID != filter.AssetID {
			continue
		}
		if filter.ChunkID != uuid.Nil && item.ChunkID != filter.ChunkID {
			continue
		}
		if filter.GenerationNo != nil && item.GenerationNo != *filter.GenerationNo {
			continue
		}
		if filter.EmbeddingProvider != "" && item.EmbeddingProvider != filter.EmbeddingProvider {
			continue
		}
		if filter.EmbeddingModel != "" && item.EmbeddingModel != filter.EmbeddingModel {
			continue
		}
		if filter.Status != "" && item.Status != filter.Status {
			continue
		}
		out = append(out, item)
	}
	return out, int64(len(out)), nil
}

func (r *fileAssetVectorIndexEmbeddingRepo) ListModelTargetsByAsset(ctx context.Context, organizationID string, assetID uuid.UUID) ([]repository.DocumentChunkEmbeddingModelTarget, error) {
	return nil, nil
}

func (r *fileAssetVectorIndexEmbeddingRepo) ListModelTargetsByChunkIDs(ctx context.Context, organizationID string, chunkIDs []uuid.UUID) ([]repository.DocumentChunkEmbeddingModelTarget, error) {
	return nil, nil
}

func (r *fileAssetVectorIndexEmbeddingRepo) CountReadyByAssetGeneration(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64) (int64, error) {
	return 0, nil
}

func (r *fileAssetVectorIndexEmbeddingRepo) CountReadyByAssetGenerationModel(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64, provider string, embeddingModel string) (int64, error) {
	return 0, nil
}

func (r *fileAssetVectorIndexEmbeddingRepo) DeleteByChunkID(ctx context.Context, organizationID string, chunkID uuid.UUID) error {
	return nil
}

func (r *fileAssetVectorIndexEmbeddingRepo) DeleteByAsset(ctx context.Context, organizationID string, assetID uuid.UUID) error {
	return nil
}

func (r *fileAssetVectorIndexEmbeddingRepo) DeleteByAssetGeneration(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64) error {
	return nil
}

var _ repository.DocumentChunkEmbeddingRepository = (*fileAssetVectorIndexEmbeddingRepo)(nil)
