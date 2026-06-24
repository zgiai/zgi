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
	assetProvider := "asset-provider"
	assetModel := "asset-model"
	assetRepo := &fileAssetStateAssetRepo{
		asset: &model.DocumentAsset{
			ID:                assetID,
			OrganizationID:    "org-1",
			SourceFileID:      "file-1",
			ProductStatus:     model.DocumentAssetProductStatusReady,
			ProcessingRunID:   &runID,
			GenerationNo:      5,
			EmbeddingProvider: &assetProvider,
			EmbeddingModel:    &assetModel,
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
	enabled := true

	result, err := svc.UpdateCurrentFileChunk(context.Background(), FileAssetChunkEditInput{
		OrganizationID:    "org-1",
		SourceFileID:      "file-1",
		ChunkID:           chunkID,
		Content:           &content,
		Enabled:           &enabled,
		UpdatedBy:         "user-1",
		EmbeddingProvider: "request-provider",
		EmbeddingModel:    "request-model",
	})
	if err != nil {
		t.Fatalf("UpdateCurrentFileChunk: %v", err)
	}
	if result.Chunk.Content != content ||
		result.Chunk.ContentHash != documentChunkContentHash(content) ||
		!result.Chunk.Enabled ||
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
		chunkEmbed.lastInput.EmbeddingProvider != assetProvider ||
		chunkEmbed.lastInput.EmbeddingModel != assetModel {
		t.Fatalf("embedding input=%+v called=%d", chunkEmbed.lastInput, chunkEmbed.called)
	}
	if chunkRepo.updateCalls != 1 {
		t.Fatalf("updateCalls=%d want 1", chunkRepo.updateCalls)
	}
}

func TestFileAssetChunkEditServiceEnqueuesDatasetRefSyncAfterEdit(t *testing.T) {
	assetID := uuid.New()
	runID := uuid.New()
	chunkID := uuid.New()
	refID := uuid.New()
	documentID := uuid.New()
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
	})
	refStore := &fileAssetStateRefStore{refs: []*model.KnowledgeBaseAssetRef{
		{
			ID:                refID,
			OrganizationID:    "org-1",
			DatasetID:         "dataset-1",
			AssetID:           assetID,
			DatasetDocumentID: &documentID,
			SyncStatus:        model.KnowledgeBaseAssetRefSyncStatusSynced,
		},
	}}
	documentStore := &fileAssetStateDocumentStore{}
	enqueuer := &fileAssetChunkEditDatasetRefSyncEnqueuer{}
	svc := NewFileAssetChunkEditServiceWithDatasetRefs(
		assetRepo,
		chunkRepo,
		nil,
		&fileAssetChunkEditEmbeddingService{},
		nil,
		refStore,
		documentStore,
		enqueuer,
	)
	content := "updated content"

	_, err := svc.UpdateCurrentFileChunk(context.Background(), FileAssetChunkEditInput{
		OrganizationID: "org-1",
		SourceFileID:   "file-1",
		ChunkID:        chunkID,
		Content:        &content,
		UpdatedBy:      "user-1",
	})
	if err != nil {
		t.Fatalf("UpdateCurrentFileChunk: %v", err)
	}
	if len(documentStore.disabledIDs) != 1 || documentStore.disabledIDs[0] != documentID.String() || documentStore.disabledBy != "user-1" {
		t.Fatalf("disabled_ids=%v disabled_by=%s", documentStore.disabledIDs, documentStore.disabledBy)
	}
	if refStore.pendingRefID != refID || refStore.pendingSyncRunID == uuid.Nil {
		t.Fatalf("pending_ref=%s sync_run=%s", refStore.pendingRefID, refStore.pendingSyncRunID)
	}
	if len(enqueuer.items) != 1 {
		t.Fatalf("enqueued=%d want 1", len(enqueuer.items))
	}
	item := enqueuer.items[0]
	if item.refID != refID ||
		item.assetID != assetID ||
		item.datasetID != "dataset-1" ||
		item.generationNo != 5 ||
		item.syncRunID != refStore.pendingSyncRunID {
		t.Fatalf("enqueued item=%+v pending_run=%s", item, refStore.pendingSyncRunID)
	}
}

func TestFileAssetChunkEditServiceBatchUpdatesChunksAndEnqueuesDatasetRefSyncOnce(t *testing.T) {
	assetID := uuid.New()
	runID := uuid.New()
	firstChunkID := uuid.New()
	secondChunkID := uuid.New()
	refID := uuid.New()
	documentID := uuid.New()
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
			ID:              firstChunkID,
			OrganizationID:  "org-1",
			AssetID:         assetID,
			ProcessingRunID: runID,
			GenerationNo:    5,
			ChunkType:       model.DocumentChunkTypeChild,
			Content:         "first",
			ContentHash:     documentChunkContentHash("first"),
			Enabled:         true,
			Status:          model.DocumentChunkStatusReady,
		},
		{
			ID:              secondChunkID,
			OrganizationID:  "org-1",
			AssetID:         assetID,
			ProcessingRunID: runID,
			GenerationNo:    5,
			ChunkType:       model.DocumentChunkTypeChild,
			Content:         "second",
			ContentHash:     documentChunkContentHash("second"),
			Enabled:         true,
			Status:          model.DocumentChunkStatusReady,
		},
	})
	refStore := &fileAssetStateRefStore{refs: []*model.KnowledgeBaseAssetRef{
		{
			ID:                refID,
			OrganizationID:    "org-1",
			DatasetID:         "dataset-1",
			AssetID:           assetID,
			DatasetDocumentID: &documentID,
			SyncStatus:        model.KnowledgeBaseAssetRefSyncStatusSynced,
		},
	}}
	documentStore := &fileAssetStateDocumentStore{}
	enqueuer := &fileAssetChunkEditDatasetRefSyncEnqueuer{}
	svc := NewFileAssetChunkEditServiceWithDatasetRefs(
		assetRepo,
		chunkRepo,
		nil,
		&fileAssetChunkEditEmbeddingService{},
		nil,
		refStore,
		documentStore,
		enqueuer,
	)

	result, err := svc.BatchUpdateCurrentFileChunks(context.Background(), FileAssetChunkBatchEditInput{
		OrganizationID: "org-1",
		SourceFileID:   "file-1",
		ChunkIDs:       []uuid.UUID{firstChunkID, secondChunkID, firstChunkID},
		Enabled:        false,
		UpdatedBy:      "user-1",
	})
	if err != nil {
		t.Fatalf("BatchUpdateCurrentFileChunks: %v", err)
	}
	if result.UpdatedCount != 2 || len(result.Chunks) != 2 {
		t.Fatalf("result=%+v", result)
	}
	if chunkRepo.updateCalls != 2 {
		t.Fatalf("updateCalls=%d want 2", chunkRepo.updateCalls)
	}
	for _, chunkID := range []uuid.UUID{firstChunkID, secondChunkID} {
		chunk, err := chunkRepo.GetByID(context.Background(), chunkID)
		if err != nil {
			t.Fatalf("GetByID: %v", err)
		}
		if chunk == nil || chunk.Enabled {
			t.Fatalf("chunk %s enabled=%v", chunkID, chunk != nil && chunk.Enabled)
		}
	}
	if len(documentStore.disabledIDs) != 1 || documentStore.disabledIDs[0] != documentID.String() || documentStore.disabledBy != "user-1" {
		t.Fatalf("disabled_ids=%v disabled_by=%s", documentStore.disabledIDs, documentStore.disabledBy)
	}
	if refStore.pendingRefID != refID || refStore.pendingSyncRunID == uuid.Nil {
		t.Fatalf("pending_ref=%s sync_run=%s", refStore.pendingRefID, refStore.pendingSyncRunID)
	}
	if len(enqueuer.items) != 1 {
		t.Fatalf("enqueued=%d want 1", len(enqueuer.items))
	}
}

func TestFileAssetChunkEditServiceUpdatesParentChunkAndRegeneratesChildren(t *testing.T) {
	assetID := uuid.New()
	runID := uuid.New()
	chunkID := uuid.New()
	childID := uuid.New()
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
			ID:              chunkID,
			OrganizationID:  "org-1",
			AssetID:         assetID,
			ProcessingRunID: runID,
			GenerationNo:    1,
			ChunkType:       model.DocumentChunkTypeParent,
			Content:         "parent",
			ContentHash:     documentChunkContentHash("parent"),
			Enabled:         true,
			Status:          model.DocumentChunkStatusReady,
		},
		{
			ID:              childID,
			OrganizationID:  "org-1",
			AssetID:         assetID,
			ProcessingRunID: runID,
			GenerationNo:    1,
			ParentChunkID:   &chunkID,
			ChunkType:       model.DocumentChunkTypeChild,
			Content:         "old child",
			ContentHash:     documentChunkContentHash("old child"),
			Enabled:         true,
			Status:          model.DocumentChunkStatusReady,
		},
	})
	chunkEmbed := &fileAssetChunkEditEmbeddingService{}
	vectorIndex := &fileAssetChunkEditVectorIndex{}
	svc := NewFileAssetChunkEditService(assetRepo, chunkRepo, nil, chunkEmbed, vectorIndex)
	content := "updated parent content"

	result, err := svc.UpdateCurrentFileChunk(context.Background(), FileAssetChunkEditInput{
		OrganizationID: "org-1",
		SourceFileID:   "file-1",
		ChunkID:        chunkID,
		Content:        &content,
		UpdatedBy:      "user-1",
	})
	if err != nil {
		t.Fatalf("UpdateCurrentFileChunk: %v", err)
	}
	if result.Chunk.Content != content || result.Chunk.ContentHash != documentChunkContentHash(content) || !result.EmbeddingReady {
		t.Fatalf("result=%+v", result)
	}
	if _, ok := chunkRepo.items[childID]; ok {
		t.Fatalf("old child should be deleted")
	}
	var newChildren []*model.DocumentChunk
	for _, item := range chunkRepo.items {
		if item.ParentChunkID != nil && *item.ParentChunkID == chunkID {
			newChildren = append(newChildren, item)
		}
	}
	if len(newChildren) == 0 {
		t.Fatalf("expected regenerated child chunks")
	}
	if chunkEmbed.called != len(newChildren) {
		t.Fatalf("embedding called=%d children=%d", chunkEmbed.called, len(newChildren))
	}
	if len(vectorIndex.deletedParentIDs) != 1 || vectorIndex.deletedParentIDs[0] != chunkID {
		t.Fatalf("deleted parent vectors=%v", vectorIndex.deletedParentIDs)
	}
}

func TestFileAssetChunkEditServiceRegeneratesParentChildrenForExistingEmbeddingModels(t *testing.T) {
	assetID := uuid.New()
	runID := uuid.New()
	parentID := uuid.New()
	childID := uuid.New()
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
			ID:              parentID,
			OrganizationID:  "org-1",
			AssetID:         assetID,
			ProcessingRunID: runID,
			GenerationNo:    1,
			ChunkType:       model.DocumentChunkTypeParent,
			Content:         "parent",
			ContentHash:     documentChunkContentHash("parent"),
			Enabled:         true,
			Status:          model.DocumentChunkStatusReady,
		},
		{
			ID:              childID,
			OrganizationID:  "org-1",
			AssetID:         assetID,
			ProcessingRunID: runID,
			GenerationNo:    1,
			ParentChunkID:   &parentID,
			ChunkType:       model.DocumentChunkTypeChild,
			Content:         "old child",
			ContentHash:     documentChunkContentHash("old child"),
			Enabled:         true,
			Status:          model.DocumentChunkStatusReady,
		},
	})
	embeddingRepo := &documentChunkEmbeddingRepo{items: []*model.DocumentChunkEmbedding{
		{
			ID:                uuid.New(),
			OrganizationID:    "org-1",
			AssetID:           assetID,
			ChunkID:           childID,
			ProcessingRunID:   runID,
			GenerationNo:      1,
			EmbeddingProvider: "provider-a",
			EmbeddingModel:    "model-a",
			Status:            model.DocumentChunkEmbeddingStatusReady,
		},
		{
			ID:                uuid.New(),
			OrganizationID:    "org-1",
			AssetID:           assetID,
			ChunkID:           childID,
			ProcessingRunID:   runID,
			GenerationNo:      1,
			EmbeddingProvider: "provider-b",
			EmbeddingModel:    "model-b",
			Status:            model.DocumentChunkEmbeddingStatusReady,
		},
	}}
	chunkEmbed := &fileAssetChunkEditEmbeddingService{}
	svc := NewFileAssetChunkEditService(assetRepo, chunkRepo, embeddingRepo, chunkEmbed)
	content := "updated parent content"

	if _, err := svc.UpdateCurrentFileChunk(context.Background(), FileAssetChunkEditInput{
		OrganizationID: "org-1",
		SourceFileID:   "file-1",
		ChunkID:        parentID,
		Content:        &content,
		UpdatedBy:      "user-1",
	}); err != nil {
		t.Fatalf("UpdateCurrentFileChunk: %v", err)
	}

	var newChildIDs []uuid.UUID
	for _, item := range chunkRepo.items {
		if item.ParentChunkID != nil && *item.ParentChunkID == parentID {
			newChildIDs = append(newChildIDs, item.ID)
		}
	}
	if len(newChildIDs) == 0 {
		t.Fatalf("expected regenerated child chunks")
	}
	if chunkEmbed.called != len(newChildIDs)*2 {
		t.Fatalf("embedding called=%d want %d", chunkEmbed.called, len(newChildIDs)*2)
	}
	seen := map[string]bool{}
	for _, input := range chunkEmbed.inputs {
		seen[input.Chunk.ID.String()+"|"+input.EmbeddingProvider+"|"+input.EmbeddingModel] = true
	}
	for _, childID := range newChildIDs {
		if !seen[childID.String()+"|provider-a|model-a"] {
			t.Fatalf("missing provider-a/model-a embedding for child %s", childID)
		}
		if !seen[childID.String()+"|provider-b|model-b"] {
			t.Fatalf("missing provider-b/model-b embedding for child %s", childID)
		}
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

func (r *fileAssetChunkEditChunkRepo) ListByIDs(ctx context.Context, organizationID string, ids []uuid.UUID) ([]*model.DocumentChunk, error) {
	allowed := map[uuid.UUID]struct{}{}
	for _, id := range ids {
		allowed[id] = struct{}{}
	}
	items := make([]*model.DocumentChunk, 0, len(ids))
	for _, item := range r.items {
		if _, ok := allowed[item.ID]; !ok || item.OrganizationID != organizationID {
			continue
		}
		cloned := *item
		items = append(items, &cloned)
	}
	return items, nil
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

func (r *fileAssetChunkEditChunkRepo) DeleteByAsset(ctx context.Context, organizationID string, assetID uuid.UUID) error {
	r.items = map[uuid.UUID]*model.DocumentChunk{}
	return nil
}

func (r *fileAssetChunkEditChunkRepo) DeleteByAssetGeneration(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64) error {
	r.items = map[uuid.UUID]*model.DocumentChunk{}
	return nil
}

func (r *fileAssetChunkEditChunkRepo) DeleteChildrenByParent(ctx context.Context, organizationID string, parentChunkID uuid.UUID) error {
	for id, item := range r.items {
		if item.OrganizationID == organizationID && item.ParentChunkID != nil && *item.ParentChunkID == parentChunkID {
			delete(r.items, id)
		}
	}
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
	inputs    []GenerateDocumentChunkEmbeddingInput
}

func (s *fileAssetChunkEditEmbeddingService) GenerateEmbeddings(ctx context.Context, input GenerateDocumentChunkEmbeddingsInput) (*GenerateDocumentChunkEmbeddingsResult, error) {
	return nil, nil
}

func (s *fileAssetChunkEditEmbeddingService) GenerateAdditionalEmbeddings(ctx context.Context, input GenerateDocumentChunkEmbeddingsInput) (*GenerateDocumentChunkEmbeddingsResult, error) {
	return nil, nil
}

func (s *fileAssetChunkEditEmbeddingService) GenerateChunkEmbedding(ctx context.Context, input GenerateDocumentChunkEmbeddingInput) (*model.DocumentChunkEmbedding, error) {
	s.called++
	s.lastInput = input
	s.inputs = append(s.inputs, input)
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

type fileAssetChunkEditVectorIndex struct {
	deletedParentIDs []uuid.UUID
}

func (v *fileAssetChunkEditVectorIndex) EnsureAssetIndexed(ctx context.Context, asset *model.DocumentAsset) error {
	return nil
}

func (v *fileAssetChunkEditVectorIndex) IndexChunkEmbeddings(ctx context.Context, asset *model.DocumentAsset, chunks []*model.DocumentChunk, embeddings []*model.DocumentChunkEmbedding, resetAsset bool) error {
	return nil
}

func (v *fileAssetChunkEditVectorIndex) DeleteAssetIndex(ctx context.Context, asset *model.DocumentAsset) error {
	return nil
}

func (v *fileAssetChunkEditVectorIndex) DeleteChunkVector(ctx context.Context, asset *model.DocumentAsset, chunkID uuid.UUID) error {
	return nil
}

func (v *fileAssetChunkEditVectorIndex) DeleteChildVectorsByParent(ctx context.Context, asset *model.DocumentAsset, parentChunkID uuid.UUID) error {
	v.deletedParentIDs = append(v.deletedParentIDs, parentChunkID)
	return nil
}

func (v *fileAssetChunkEditVectorIndex) Search(ctx context.Context, asset *model.DocumentAsset, queryVector []float64, limit int) ([]map[string]interface{}, error) {
	return nil, nil
}

var _ FileAssetVectorIndexService = (*fileAssetChunkEditVectorIndex)(nil)

type fileAssetChunkEditDatasetRefSyncEnqueuer struct {
	items []fileAssetChunkEditDatasetRefSyncItem
}

type fileAssetChunkEditDatasetRefSyncItem struct {
	refID        uuid.UUID
	assetID      uuid.UUID
	datasetID    string
	generationNo int64
	syncRunID    uuid.UUID
}

func (e *fileAssetChunkEditDatasetRefSyncEnqueuer) EnqueueDatasetRefSync(ctx context.Context, refID uuid.UUID, assetID uuid.UUID, datasetID string, generationNo int64, syncRunID uuid.UUID) error {
	e.items = append(e.items, fileAssetChunkEditDatasetRefSyncItem{
		refID:        refID,
		assetID:      assetID,
		datasetID:    datasetID,
		generationNo: generationNo,
		syncRunID:    syncRunID,
	})
	return nil
}

var _ FileAssetChunkEditDatasetRefSyncEnqueuer = (*fileAssetChunkEditDatasetRefSyncEnqueuer)(nil)
