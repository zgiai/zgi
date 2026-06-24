package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
)

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
}

func (f *fileAssetVectorIndexFieldDeleteDB) StoreVector(ctx context.Context, id, className string, properties map[string]interface{}, vector []float64) error {
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
	return nil, nil
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

var _ repository.DocumentChunkRepository = (*fileAssetVectorIndexChunkRepo)(nil)
