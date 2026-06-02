package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
)

func TestDocumentChunkGenerationServiceWritesParentChildAndAutoChunks(t *testing.T) {
	assetID := uuid.New()
	runID := uuid.New()
	assetRepo := &fileAssetStateAssetRepo{
		asset: &model.DocumentAsset{
			ID:              assetID,
			OrganizationID:  "org-1",
			SourceFileID:    "file-1",
			ProductStatus:   model.DocumentAssetProductStatusGenerating,
			ProcessingRunID: &runID,
			GenerationNo:    2,
		},
	}
	chunkRepo := &documentChunkGenerationChunkRepo{}
	svc := NewDocumentChunkGenerationService(assetRepo, chunkRepo)

	result, err := svc.GenerateChunks(context.Background(), GenerateDocumentChunksInput{
		OrganizationID:  "org-1",
		AssetID:         assetID,
		ProcessingRunID: runID,
		GenerationNo:    2,
		CreatedBy:       "user-1",
		Chunks: []dto.TransformedChunk{
			{
				Content: "parent",
				Children: []dto.TransformedChildChunk{
					{Content: "child-a"},
					{Content: "child-b"},
				},
			},
			{Content: "auto"},
		},
	})
	if err != nil {
		t.Fatalf("GenerateChunks: %v", err)
	}
	if result.ChunkCount != 2 || result.PrimaryChunkCount != 2 || result.SecondaryChunkCount != 3 {
		t.Fatalf("result=%+v", result)
	}
	if result.Asset == nil || result.Asset.ChunkCount != 2 || result.Asset.VectorStatus != model.DocumentAssetVectorStatusIndexing {
		t.Fatalf("asset=%+v", result.Asset)
	}
	if len(chunkRepo.items) != 5 || chunkRepo.deletedCalls != 1 {
		t.Fatalf("repo items=%+v deleted=%d", chunkRepo.items, chunkRepo.deletedCalls)
	}
	parent := chunkRepo.items[0]
	if parent.ChunkType != model.DocumentChunkTypeParent {
		t.Fatalf("parent=%+v", parent)
	}
	for _, child := range chunkRepo.items[1:3] {
		if child.ChunkType != model.DocumentChunkTypeChild || child.ParentChunkID == nil || *child.ParentChunkID != parent.ID {
			t.Fatalf("child=%+v parent=%+v", child, parent)
		}
	}
	secondParent := chunkRepo.items[3]
	if secondParent.ChunkType != model.DocumentChunkTypeParent {
		t.Fatalf("second parent=%+v", secondParent)
	}
	if chunkRepo.items[4].ChunkType != model.DocumentChunkTypeChild || chunkRepo.items[4].ParentChunkID == nil || *chunkRepo.items[4].ParentChunkID != secondParent.ID {
		t.Fatalf("generated child=%+v parent=%+v", chunkRepo.items[4], secondParent)
	}
}

type documentChunkGenerationChunkRepo struct {
	items        []*model.DocumentChunk
	deletedCalls int
}

func (r *documentChunkGenerationChunkRepo) Create(ctx context.Context, item *model.DocumentChunk) error {
	if item.ID == uuid.Nil {
		item.ID = uuid.New()
	}
	r.items = append(r.items, item)
	return nil
}

func (r *documentChunkGenerationChunkRepo) CreateBatch(ctx context.Context, items []*model.DocumentChunk) error {
	for _, item := range items {
		if err := r.Create(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func (r *documentChunkGenerationChunkRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.DocumentChunk, error) {
	for _, item := range r.items {
		if item.ID == id {
			return item, nil
		}
	}
	return nil, nil
}

func (r *documentChunkGenerationChunkRepo) ListByIDs(ctx context.Context, organizationID string, ids []uuid.UUID) ([]*model.DocumentChunk, error) {
	allowed := map[uuid.UUID]struct{}{}
	for _, id := range ids {
		allowed[id] = struct{}{}
	}
	items := make([]*model.DocumentChunk, 0, len(ids))
	for _, item := range r.items {
		if _, ok := allowed[item.ID]; ok && item.OrganizationID == organizationID {
			items = append(items, item)
		}
	}
	return items, nil
}

func (r *documentChunkGenerationChunkRepo) List(ctx context.Context, filter repository.DocumentChunkListFilter) ([]*model.DocumentChunk, int64, error) {
	return r.items, int64(len(r.items)), nil
}

func (r *documentChunkGenerationChunkRepo) CountByAssetGeneration(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64) (int64, error) {
	return int64(len(r.items)), nil
}

func (r *documentChunkGenerationChunkRepo) CountByAssetGenerationAndTypes(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64, chunkTypes []string) (int64, error) {
	allowed := map[string]struct{}{}
	for _, chunkType := range chunkTypes {
		allowed[chunkType] = struct{}{}
	}
	var count int64
	for _, item := range r.items {
		if len(allowed) == 0 {
			count++
			continue
		}
		if _, ok := allowed[item.ChunkType]; ok {
			count++
		}
	}
	return count, nil
}

func (r *documentChunkGenerationChunkRepo) DeleteByAssetGeneration(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64) error {
	r.deletedCalls++
	r.items = nil
	return nil
}

func (r *documentChunkGenerationChunkRepo) Update(ctx context.Context, id uuid.UUID, patch repository.DocumentChunkPatch) (*model.DocumentChunk, error) {
	return r.GetByID(ctx, id)
}

var _ repository.DocumentChunkRepository = (*documentChunkGenerationChunkRepo)(nil)
