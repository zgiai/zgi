package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
)

func TestFileAssetChunkEditServiceUpdatesLeafChunkAndRegeneratesOnlyThatEmbedding(t *testing.T) {
	assetID := uuid.New()
	runID := uuid.New()
	chunkID := uuid.New()
	assetRepo := &fileAssetStateAssetRepo{
		asset: &model.DocumentAsset{
			ID:              assetID,
			OrganizationID:  "org-1",
			SourceFileID:    "file-1",
			ProductStatus:   model.DocumentAssetProductStatusReady,
			ProcessingRunID: &runID,
			GenerationNo:    5,
		},
	}
	chunkRepo := newFileAssetChunkEditChunkRepo([]*model.DocumentChunk{
		{
			ID:              chunkID,
			OrganizationID:  "org-1",
			AssetID:         assetID,
			ProcessingRunID: runID,
			GenerationNo:    5,
			ChunkType:       model.DocumentChunkTypeChild,
			Content:         "old content",
			ContentHash:     documentChunkContentHash("old content"),
			Enabled:         true,
			Status:          model.DocumentChunkStatusReady,
		},
		{
			ID:              uuid.New(),
			OrganizationID:  "org-1",
			AssetID:         assetID,
			ProcessingRunID: runID,
			GenerationNo:    5,
			ChunkType:       model.DocumentChunkTypeChild,
			Content:         "other content",
			ContentHash:     documentChunkContentHash("other content"),
			Enabled:         true,
			Status:          model.DocumentChunkStatusReady,
		},
	})
	chunkEmbed := &fileAssetChunkEditEmbeddingService{}
	svc := NewFileAssetChunkEditService(assetRepo, chunkRepo, nil, chunkEmbed)
	content := "updated content"
	enabled := false

	result, err := svc.UpdateCurrentFileChunk(context.Background(), FileAssetChunkEditInput{
		OrganizationID:    "org-1",
		SourceFileID:      "file-1",
		ChunkID:           chunkID,
		Content:           &content,
		Enabled:           &enabled,
		UpdatedBy:         "user-1",
		EmbeddingProvider: "provider-1",
		EmbeddingModel:    "model-1",
	})
	if err != nil {
		t.Fatalf("UpdateCurrentFileChunk: %v", err)
	}
	if result.Chunk.Content != content ||
		result.Chunk.ContentHash != documentChunkContentHash(content) ||
		result.Chunk.Enabled ||
		result.Chunk.UpdatedBy != "user-1" {
		t.Fatalf("updated chunk=%+v", result.Chunk)
	}
	if !result.EmbeddingReady || result.Embedding == nil {
		t.Fatalf("embedding result=%+v", result)
	}
	if chunkEmbed.called != 1 ||
		chunkEmbed.lastInput.Chunk == nil ||
		chunkEmbed.lastInput.Chunk.ID != chunkID ||
		chunkEmbed.lastInput.ProcessingRunID != runID ||
		chunkEmbed.lastInput.GenerationNo != 5 ||
		chunkEmbed.lastInput.EmbeddingProvider != "provider-1" ||
		chunkEmbed.lastInput.EmbeddingModel != "model-1" {
		t.Fatalf("embedding input=%+v called=%d", chunkEmbed.lastInput, chunkEmbed.called)
	}
	if chunkRepo.updateCalls != 1 {
		t.Fatalf("updateCalls=%d want 1", chunkRepo.updateCalls)
	}
}

func TestFileAssetChunkEditServiceRejectsParentChunk(t *testing.T) {
	assetID := uuid.New()
	runID := uuid.New()
	chunkID := uuid.New()
	assetRepo := &fileAssetStateAssetRepo{
		asset: &model.DocumentAsset{
			ID:              assetID,
			OrganizationID:  "org-1",
			SourceFileID:    "file-1",
			ProcessingRunID: &runID,
			GenerationNo:    1,
		},
	}
	chunkRepo := newFileAssetChunkEditChunkRepo([]*model.DocumentChunk{
		{
			ID:             chunkID,
			OrganizationID: "org-1",
			AssetID:        assetID,
			GenerationNo:   1,
			ChunkType:      model.DocumentChunkTypeParent,
			Content:        "parent",
			ContentHash:    documentChunkContentHash("parent"),
			Enabled:        true,
			Status:         model.DocumentChunkStatusReady,
		},
	})
	svc := NewFileAssetChunkEditService(assetRepo, chunkRepo, nil, &fileAssetChunkEditEmbeddingService{})
	content := "updated"

	_, err := svc.UpdateCurrentFileChunk(context.Background(), FileAssetChunkEditInput{
		OrganizationID: "org-1",
		SourceFileID:   "file-1",
		ChunkID:        chunkID,
		Content:        &content,
	})
	if !errors.Is(err, ErrFileChunkEditNotAllowed) {
		t.Fatalf("err=%v want ErrFileChunkEditNotAllowed", err)
	}
	if chunkRepo.updateCalls != 0 {
		t.Fatalf("parent chunk should not be updated")
	}
}

func TestFileAssetChunkEditServiceRejectsStaleGeneration(t *testing.T) {
	assetID := uuid.New()
	runID := uuid.New()
	chunkID := uuid.New()
	assetRepo := &fileAssetStateAssetRepo{
		asset: &model.DocumentAsset{
			ID:              assetID,
			OrganizationID:  "org-1",
			SourceFileID:    "file-1",
			ProcessingRunID: &runID,
			GenerationNo:    2,
		},
	}
	chunkRepo := newFileAssetChunkEditChunkRepo([]*model.DocumentChunk{
		{
			ID:             chunkID,
			OrganizationID: "org-1",
			AssetID:        assetID,
			GenerationNo:   1,
			ChunkType:      model.DocumentChunkTypeChild,
			Content:        "old generation",
			ContentHash:    documentChunkContentHash("old generation"),
			Enabled:        true,
			Status:         model.DocumentChunkStatusReady,
		},
	})
	svc := NewFileAssetChunkEditService(assetRepo, chunkRepo, nil, &fileAssetChunkEditEmbeddingService{})
	content := "updated"

	_, err := svc.UpdateCurrentFileChunk(context.Background(), FileAssetChunkEditInput{
		OrganizationID: "org-1",
		SourceFileID:   "file-1",
		ChunkID:        chunkID,
		Content:        &content,
	})
	if !errors.Is(err, ErrProcessingRunMismatch) {
		t.Fatalf("err=%v want ErrProcessingRunMismatch", err)
	}
	if chunkRepo.updateCalls != 0 {
		t.Fatalf("stale chunk should not be updated")
	}
}

type fileAssetChunkEditChunkRepo struct {
	items       map[uuid.UUID]*model.DocumentChunk
	updateCalls int
}

func newFileAssetChunkEditChunkRepo(items []*model.DocumentChunk) *fileAssetChunkEditChunkRepo {
	repo := &fileAssetChunkEditChunkRepo{items: map[uuid.UUID]*model.DocumentChunk{}}
	for _, item := range items {
		cloned := *item
		repo.items[item.ID] = &cloned
	}
	return repo
}

func (r *fileAssetChunkEditChunkRepo) Create(ctx context.Context, item *model.DocumentChunk) error {
	r.items[item.ID] = item
	return nil
}

func (r *fileAssetChunkEditChunkRepo) CreateBatch(ctx context.Context, items []*model.DocumentChunk) error {
	for _, item := range items {
		if err := r.Create(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func (r *fileAssetChunkEditChunkRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.DocumentChunk, error) {
	item, ok := r.items[id]
	if !ok {
		return nil, nil
	}
	cloned := *item
	return &cloned, nil
}

func (r *fileAssetChunkEditChunkRepo) List(ctx context.Context, filter repository.DocumentChunkListFilter) ([]*model.DocumentChunk, int64, error) {
	items := make([]*model.DocumentChunk, 0, len(r.items))
	for _, item := range r.items {
		cloned := *item
		items = append(items, &cloned)
	}
	return items, int64(len(items)), nil
}

func (r *fileAssetChunkEditChunkRepo) CountByAssetGeneration(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64) (int64, error) {
	return int64(len(r.items)), nil
}

func (r *fileAssetChunkEditChunkRepo) CountByAssetGenerationAndTypes(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64, chunkTypes []string) (int64, error) {
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

func (r *fileAssetChunkEditChunkRepo) DeleteByAssetGeneration(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64) error {
	r.items = map[uuid.UUID]*model.DocumentChunk{}
	return nil
}

func (r *fileAssetChunkEditChunkRepo) Update(ctx context.Context, id uuid.UUID, patch repository.DocumentChunkPatch) (*model.DocumentChunk, error) {
	item, ok := r.items[id]
	if !ok || item.OrganizationID != patch.OrganizationID {
		return nil, nil
	}
	r.updateCalls++
	if patch.Content != nil {
		item.Content = *patch.Content
	}
	if patch.ContentHash != nil {
		item.ContentHash = *patch.ContentHash
	}
	if patch.Enabled != nil {
		item.Enabled = *patch.Enabled
	}
	if patch.Status != nil {
		item.Status = *patch.Status
	}
	if patch.UpdatedBy != "" {
		item.UpdatedBy = patch.UpdatedBy
	}
	cloned := *item
	return &cloned, nil
}

var _ repository.DocumentChunkRepository = (*fileAssetChunkEditChunkRepo)(nil)

type fileAssetChunkEditEmbeddingService struct {
	called    int
	lastInput GenerateDocumentChunkEmbeddingInput
}

func (s *fileAssetChunkEditEmbeddingService) GenerateEmbeddings(ctx context.Context, input GenerateDocumentChunkEmbeddingsInput) (*GenerateDocumentChunkEmbeddingsResult, error) {
	return nil, nil
}

func (s *fileAssetChunkEditEmbeddingService) GenerateChunkEmbedding(ctx context.Context, input GenerateDocumentChunkEmbeddingInput) (*model.DocumentChunkEmbedding, error) {
	s.called++
	s.lastInput = input
	return &model.DocumentChunkEmbedding{
		ID:                uuid.New(),
		OrganizationID:    input.OrganizationID,
		AssetID:           input.AssetID,
		ChunkID:           input.Chunk.ID,
		ProcessingRunID:   input.ProcessingRunID,
		GenerationNo:      input.GenerationNo,
		EmbeddingProvider: input.EmbeddingProvider,
		EmbeddingModel:    input.EmbeddingModel,
		ContentHash:       input.Chunk.ContentHash,
		Status:            model.DocumentChunkEmbeddingStatusReady,
	}, nil
}

var _ DocumentChunkEmbeddingService = (*fileAssetChunkEditEmbeddingService)(nil)
