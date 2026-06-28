package service

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
	"github.com/zgiai/zgi/api/pkg/embedding"
)

func TestDocumentChunkEmbeddingServiceEmbedsOnlyLeafChunks(t *testing.T) {
	assetID := uuid.New()
	runID := uuid.New()
	assetRepo := &fileAssetStateAssetRepo{
		asset: &model.DocumentAsset{
			ID:              assetID,
			OrganizationID:  "org-1",
			SourceFileID:    "file-1",
			ProductStatus:   model.DocumentAssetProductStatusGenerating,
			ProcessingRunID: &runID,
			GenerationNo:    3,
		},
	}
	embeddingRepo := &documentChunkEmbeddingRepo{}
	embeddingSvc := &documentChunkEmbeddingFakeEmbeddingService{
		vectors: [][]float64{
			{1, 1.5, 2},
			{3, 3.5, 4},
		},
	}
	svc := NewDocumentChunkEmbeddingService(
		assetRepo,
		embeddingRepo,
		nil,
		nil,
		WithDocumentChunkEmbeddingFactory(func(ctx context.Context, input GenerateDocumentChunkEmbeddingsInput, asset *model.DocumentAsset, provider string, modelName string) (embedding.EmbeddingService, error) {
			if provider != "provider-1" || modelName != "model-1" {
				t.Fatalf("provider=%q model=%q", provider, modelName)
			}
			return embeddingSvc, nil
		}),
	)
	parentID := uuid.New()
	chunks := []*model.DocumentChunk{
		{
			ID:             parentID,
			OrganizationID: "org-1",
			AssetID:        assetID,
			ChunkType:      model.DocumentChunkTypeParent,
			Content:        "parent",
			ContentHash:    documentChunkContentHash("parent"),
			Enabled:        true,
			Status:         model.DocumentChunkStatusReady,
		},
		{
			ID:             uuid.New(),
			OrganizationID: "org-1",
			AssetID:        assetID,
			ParentChunkID:  &parentID,
			ChunkType:      model.DocumentChunkTypeChild,
			Content:        "child text",
			ContentHash:    documentChunkContentHash("child text"),
			Enabled:        true,
			Status:         model.DocumentChunkStatusReady,
		},
		{
			ID:             uuid.New(),
			OrganizationID: "org-1",
			AssetID:        assetID,
			ChunkType:      model.DocumentChunkTypeManual,
			Content:        "manual text",
			ContentHash:    documentChunkContentHash("manual text"),
			Enabled:        true,
			Status:         model.DocumentChunkStatusReady,
		},
		{
			ID:             uuid.New(),
			OrganizationID: "org-1",
			AssetID:        assetID,
			ChunkType:      model.DocumentChunkTypeAuto,
			Content:        "disabled",
			ContentHash:    documentChunkContentHash("disabled"),
			Enabled:        false,
			Status:         model.DocumentChunkStatusReady,
		},
		{
			ID:             uuid.New(),
			OrganizationID: "org-1",
			AssetID:        assetID,
			ChunkType:      model.DocumentChunkTypeChild,
			Content:        "errored",
			ContentHash:    documentChunkContentHash("errored"),
			Enabled:        true,
			Status:         model.DocumentChunkStatusError,
		},
	}

	result, err := svc.GenerateEmbeddings(context.Background(), GenerateDocumentChunkEmbeddingsInput{
		OrganizationID:    "org-1",
		AssetID:           assetID,
		ProcessingRunID:   runID,
		GenerationNo:      3,
		EmbeddingProvider: "provider-1",
		EmbeddingModel:    "model-1",
		Chunks:            chunks,
	})
	if err != nil {
		t.Fatalf("GenerateEmbeddings: %v", err)
	}
	if !reflect.DeepEqual(embeddingSvc.texts, []string{"child text"}) {
		t.Fatalf("embedded texts=%+v", embeddingSvc.texts)
	}
	if embeddingRepo.deletedCalls != 1 {
		t.Fatalf("deletedCalls=%d want 1", embeddingRepo.deletedCalls)
	}
	if result.EmbeddingCount != 1 || result.EmbeddingDimension != 3 {
		t.Fatalf("result=%+v", result)
	}
	if len(embeddingRepo.items) != 1 {
		t.Fatalf("items=%d want 1", len(embeddingRepo.items))
	}
	if embeddingRepo.items[0].EmbeddingVector[0] != float32(1) ||
		embeddingRepo.items[0].EmbeddingVector[1] != float32(1.5) {
		t.Fatalf("embedding items=%+v", embeddingRepo.items)
	}
}

func TestDocumentChunkEmbeddingServiceBatchesTextsByEight(t *testing.T) {
	assetID := uuid.New()
	runID := uuid.New()
	assetRepo := &fileAssetStateAssetRepo{
		asset: &model.DocumentAsset{
			ID:              assetID,
			OrganizationID:  "org-1",
			SourceFileID:    "file-1",
			ProductStatus:   model.DocumentAssetProductStatusGenerating,
			ProcessingRunID: &runID,
			GenerationNo:    6,
		},
	}
	chunks := make([]*model.DocumentChunk, 0, 20)
	vectors := make([][]float64, 0, 20)
	for i := 0; i < 20; i++ {
		content := "child text"
		chunks = append(chunks, &model.DocumentChunk{
			ID:              uuid.New(),
			OrganizationID:  "org-1",
			AssetID:         assetID,
			ProcessingRunID: runID,
			GenerationNo:    6,
			ChunkType:       model.DocumentChunkTypeChild,
			Content:         content,
			ContentHash:     documentChunkContentHash(content),
			Enabled:         true,
			Status:          model.DocumentChunkStatusReady,
		})
		vectors = append(vectors, []float64{float64(i), float64(i + 1)})
	}
	embeddingSvc := &documentChunkEmbeddingFakeEmbeddingService{vectors: vectors}
	svc := NewDocumentChunkEmbeddingService(
		assetRepo,
		&documentChunkEmbeddingRepo{},
		nil,
		nil,
		WithDocumentChunkEmbeddingFactory(func(ctx context.Context, input GenerateDocumentChunkEmbeddingsInput, asset *model.DocumentAsset, provider string, modelName string) (embedding.EmbeddingService, error) {
			return embeddingSvc, nil
		}),
	)

	result, err := svc.GenerateEmbeddings(context.Background(), GenerateDocumentChunkEmbeddingsInput{
		OrganizationID:    "org-1",
		AssetID:           assetID,
		ProcessingRunID:   runID,
		GenerationNo:      6,
		EmbeddingProvider: "provider-1",
		EmbeddingModel:    "model-1",
		Chunks:            chunks,
	})
	if err != nil {
		t.Fatalf("GenerateEmbeddings: %v", err)
	}
	if result.EmbeddingCount != 20 {
		t.Fatalf("embedding count=%d want 20", result.EmbeddingCount)
	}
	gotBatchSizes := make([]int, 0, len(embeddingSvc.calls))
	for _, call := range embeddingSvc.calls {
		gotBatchSizes = append(gotBatchSizes, len(call))
	}
	if !reflect.DeepEqual(gotBatchSizes, []int{8, 8, 4}) {
		t.Fatalf("batch sizes=%v", gotBatchSizes)
	}
}

func TestDocumentChunkEmbeddingServiceReportsBatchProgress(t *testing.T) {
	assetID := uuid.New()
	runID := uuid.New()
	assetRepo := &fileAssetStateAssetRepo{
		asset: &model.DocumentAsset{
			ID:              assetID,
			OrganizationID:  "org-1",
			SourceFileID:    "file-1",
			ProductStatus:   model.DocumentAssetProductStatusGenerating,
			ProcessingRunID: &runID,
			GenerationNo:    6,
		},
	}
	chunks := make([]*model.DocumentChunk, 0, 20)
	vectors := make([][]float64, 0, 20)
	for i := 0; i < 20; i++ {
		content := "child text"
		chunks = append(chunks, &model.DocumentChunk{
			ID:              uuid.New(),
			OrganizationID:  "org-1",
			AssetID:         assetID,
			ProcessingRunID: runID,
			GenerationNo:    6,
			ChunkType:       model.DocumentChunkTypeChild,
			Content:         content,
			ContentHash:     documentChunkContentHash(content),
			Enabled:         true,
			Status:          model.DocumentChunkStatusReady,
		})
		vectors = append(vectors, []float64{float64(i), float64(i + 1)})
	}
	embeddingSvc := &documentChunkEmbeddingFakeEmbeddingService{vectors: vectors}
	svc := NewDocumentChunkEmbeddingService(
		assetRepo,
		&documentChunkEmbeddingRepo{},
		nil,
		nil,
		WithDocumentChunkEmbeddingFactory(func(ctx context.Context, input GenerateDocumentChunkEmbeddingsInput, asset *model.DocumentAsset, provider string, modelName string) (embedding.EmbeddingService, error) {
			return embeddingSvc, nil
		}),
	)

	var progress []GenerateDocumentChunkEmbeddingsProgress
	_, err := svc.GenerateEmbeddings(context.Background(), GenerateDocumentChunkEmbeddingsInput{
		OrganizationID:    "org-1",
		AssetID:           assetID,
		ProcessingRunID:   runID,
		GenerationNo:      6,
		EmbeddingProvider: "provider-1",
		EmbeddingModel:    "model-1",
		Chunks:            chunks,
		OnProgress: func(snapshot GenerateDocumentChunkEmbeddingsProgress) {
			progress = append(progress, snapshot)
		},
	})
	if err != nil {
		t.Fatalf("GenerateEmbeddings: %v", err)
	}
	got := make([][2]int, 0, len(progress))
	for _, snapshot := range progress {
		got = append(got, [2]int{snapshot.Completed, snapshot.Total})
	}
	want := [][2]int{{8, 20}, {16, 20}, {20, 20}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("progress=%v want %v", got, want)
	}
}

func TestDocumentChunkEmbeddingServiceAdditionalEmbeddingsDoNotIndexFileQACollection(t *testing.T) {
	assetID := uuid.New()
	runID := uuid.New()
	assetRepo := &fileAssetStateAssetRepo{
		asset: &model.DocumentAsset{
			ID:              assetID,
			OrganizationID:  "org-1",
			SourceFileID:    "file-1",
			ProductStatus:   model.DocumentAssetProductStatusReady,
			ProcessingRunID: &runID,
			GenerationNo:    7,
		},
	}
	embeddingRepo := &documentChunkEmbeddingRepo{}
	embeddingSvc := &documentChunkEmbeddingFakeEmbeddingService{
		vectors: [][]float64{{1, 2, 3}},
	}
	vectorIndex := &documentChunkEmbeddingFakeVectorIndex{}
	svc := NewDocumentChunkEmbeddingService(
		assetRepo,
		embeddingRepo,
		nil,
		nil,
		WithDocumentChunkEmbeddingFactory(func(ctx context.Context, input GenerateDocumentChunkEmbeddingsInput, asset *model.DocumentAsset, provider string, modelName string) (embedding.EmbeddingService, error) {
			return embeddingSvc, nil
		}),
		WithDocumentChunkVectorIndex(vectorIndex),
	)
	chunk := &model.DocumentChunk{
		ID:              uuid.New(),
		OrganizationID:  "org-1",
		AssetID:         assetID,
		ProcessingRunID: runID,
		GenerationNo:    7,
		ChunkType:       model.DocumentChunkTypeChild,
		Content:         "child text",
		ContentHash:     documentChunkContentHash("child text"),
		Enabled:         true,
		Status:          model.DocumentChunkStatusReady,
	}

	_, err := svc.GenerateAdditionalEmbeddings(context.Background(), GenerateDocumentChunkEmbeddingsInput{
		OrganizationID:    "org-1",
		AssetID:           assetID,
		ProcessingRunID:   runID,
		GenerationNo:      7,
		EmbeddingProvider: "kb-provider",
		EmbeddingModel:    "kb-model",
		Chunks:            []*model.DocumentChunk{chunk},
	})
	if err != nil {
		t.Fatalf("GenerateAdditionalEmbeddings: %v", err)
	}
	if vectorIndex.indexCalls != 0 {
		t.Fatalf("additional embeddings indexed file QA collection %d times", vectorIndex.indexCalls)
	}
	if vectorIndex.deleteCalls != 0 {
		t.Fatalf("additional embeddings deleted file QA collection %d times", vectorIndex.deleteCalls)
	}
}

func TestDocumentChunkEmbeddingServiceRegeneratesSingleChunkWithoutClearingGeneration(t *testing.T) {
	assetID := uuid.New()
	runID := uuid.New()
	assetRepo := &fileAssetStateAssetRepo{
		asset: &model.DocumentAsset{
			ID:              assetID,
			OrganizationID:  "org-1",
			SourceFileID:    "file-1",
			ProductStatus:   model.DocumentAssetProductStatusReady,
			ProcessingRunID: &runID,
			GenerationNo:    4,
		},
	}
	embeddingRepo := &documentChunkEmbeddingRepo{}
	embeddingSvc := &documentChunkEmbeddingFakeEmbeddingService{
		vectors: [][]float64{{9, 8, 7}},
	}
	svc := NewDocumentChunkEmbeddingService(
		assetRepo,
		embeddingRepo,
		nil,
		nil,
		WithDocumentChunkEmbeddingFactory(func(ctx context.Context, input GenerateDocumentChunkEmbeddingsInput, asset *model.DocumentAsset, provider string, modelName string) (embedding.EmbeddingService, error) {
			return embeddingSvc, nil
		}),
	)
	chunk := &model.DocumentChunk{
		ID:              uuid.New(),
		OrganizationID:  "org-1",
		AssetID:         assetID,
		ProcessingRunID: runID,
		GenerationNo:    4,
		ChunkType:       model.DocumentChunkTypeChild,
		Content:         "edited chunk",
		ContentHash:     documentChunkContentHash("edited chunk"),
		Enabled:         true,
		Status:          model.DocumentChunkStatusReady,
	}

	item, err := svc.GenerateChunkEmbedding(context.Background(), GenerateDocumentChunkEmbeddingInput{
		OrganizationID:    "org-1",
		AssetID:           assetID,
		ProcessingRunID:   runID,
		GenerationNo:      4,
		EmbeddingProvider: "provider-1",
		EmbeddingModel:    "model-1",
		Chunk:             chunk,
	})
	if err != nil {
		t.Fatalf("GenerateChunkEmbedding: %v", err)
	}
	if embeddingRepo.deletedCalls != 0 {
		t.Fatalf("single chunk regenerate cleared generation embeddings")
	}
	if len(embeddingRepo.items) != 1 || item.ChunkID != chunk.ID {
		t.Fatalf("item=%+v repo=%+v", item, embeddingRepo.items)
	}
	if !reflect.DeepEqual(embeddingSvc.texts, []string{"edited chunk"}) {
		t.Fatalf("embedded texts=%+v", embeddingSvc.texts)
	}
}

type documentChunkEmbeddingFakeEmbeddingService struct {
	texts   []string
	calls   [][]string
	vectors [][]float64
	offset  int
}

func (s *documentChunkEmbeddingFakeEmbeddingService) EmbedText(ctx context.Context, text string) ([]float64, error) {
	vectors, err := s.EmbedTexts(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	return vectors[0], nil
}

func (s *documentChunkEmbeddingFakeEmbeddingService) EmbedTexts(ctx context.Context, texts []string) ([][]float64, error) {
	s.texts = append(s.texts, texts...)
	s.calls = append(s.calls, append([]string(nil), texts...))
	if len(s.vectors) == 0 {
		return nil, nil
	}
	end := s.offset + len(texts)
	if end > len(s.vectors) {
		end = len(s.vectors)
	}
	out := append([][]float64(nil), s.vectors[s.offset:end]...)
	s.offset = end
	return out, nil
}

func (s *documentChunkEmbeddingFakeEmbeddingService) GetDimension() int {
	if len(s.vectors) == 0 {
		return 0
	}
	return len(s.vectors[0])
}

func (s *documentChunkEmbeddingFakeEmbeddingService) GetModel() string {
	return "model-1"
}

type documentChunkEmbeddingRepo struct {
	items        []*model.DocumentChunkEmbedding
	deletedCalls int
}

func (r *documentChunkEmbeddingRepo) Create(ctx context.Context, item *model.DocumentChunkEmbedding) error {
	r.items = append(r.items, item)
	return nil
}

func (r *documentChunkEmbeddingRepo) Upsert(ctx context.Context, item *model.DocumentChunkEmbedding) error {
	r.items = append(r.items, item)
	return nil
}

func (r *documentChunkEmbeddingRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.DocumentChunkEmbedding, error) {
	for _, item := range r.items {
		if item.ID == id {
			return item, nil
		}
	}
	return nil, nil
}

func (r *documentChunkEmbeddingRepo) FindByChunkModel(ctx context.Context, chunkID uuid.UUID, provider string, embeddingModel string) (*model.DocumentChunkEmbedding, error) {
	for _, item := range r.items {
		if item.ChunkID == chunkID && item.EmbeddingProvider == provider && item.EmbeddingModel == embeddingModel {
			return item, nil
		}
	}
	return nil, nil
}

func (r *documentChunkEmbeddingRepo) List(ctx context.Context, filter repository.DocumentChunkEmbeddingListFilter) ([]*model.DocumentChunkEmbedding, int64, error) {
	return r.items, int64(len(r.items)), nil
}

func (r *documentChunkEmbeddingRepo) ListModelTargetsByAsset(ctx context.Context, organizationID string, assetID uuid.UUID) ([]repository.DocumentChunkEmbeddingModelTarget, error) {
	seen := map[string]struct{}{}
	var out []repository.DocumentChunkEmbeddingModelTarget
	for _, item := range r.items {
		if item.OrganizationID != organizationID || item.AssetID != assetID || item.EmbeddingModel == "" {
			continue
		}
		key := item.EmbeddingProvider + "\x00" + item.EmbeddingModel
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, repository.DocumentChunkEmbeddingModelTarget{
			EmbeddingProvider: item.EmbeddingProvider,
			EmbeddingModel:    item.EmbeddingModel,
		})
	}
	return out, nil
}

func (r *documentChunkEmbeddingRepo) ListModelTargetsByChunkIDs(ctx context.Context, organizationID string, chunkIDs []uuid.UUID) ([]repository.DocumentChunkEmbeddingModelTarget, error) {
	allowed := map[uuid.UUID]struct{}{}
	for _, id := range chunkIDs {
		allowed[id] = struct{}{}
	}
	seen := map[string]struct{}{}
	var out []repository.DocumentChunkEmbeddingModelTarget
	for _, item := range r.items {
		if item.OrganizationID != organizationID || item.EmbeddingModel == "" {
			continue
		}
		if _, ok := allowed[item.ChunkID]; !ok {
			continue
		}
		key := item.EmbeddingProvider + "\x00" + item.EmbeddingModel
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, repository.DocumentChunkEmbeddingModelTarget{
			EmbeddingProvider: item.EmbeddingProvider,
			EmbeddingModel:    item.EmbeddingModel,
		})
	}
	return out, nil
}

func (r *documentChunkEmbeddingRepo) CountReadyByAssetGeneration(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64) (int64, error) {
	var count int64
	for _, item := range r.items {
		if item.OrganizationID == organizationID &&
			item.AssetID == assetID &&
			item.GenerationNo == generationNo &&
			item.Status == model.DocumentChunkEmbeddingStatusReady {
			count++
		}
	}
	return count, nil
}

func (r *documentChunkEmbeddingRepo) CountReadyByAssetGenerationModel(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64, provider string, embeddingModel string) (int64, error) {
	var count int64
	for _, item := range r.items {
		if item.OrganizationID == organizationID &&
			item.AssetID == assetID &&
			item.GenerationNo == generationNo &&
			item.EmbeddingProvider == provider &&
			item.EmbeddingModel == embeddingModel &&
			item.Status == model.DocumentChunkEmbeddingStatusReady {
			count++
		}
	}
	return count, nil
}

func (r *documentChunkEmbeddingRepo) DeleteByChunkID(ctx context.Context, organizationID string, chunkID uuid.UUID) error {
	filtered := r.items[:0]
	for _, item := range r.items {
		if item.OrganizationID == organizationID && item.ChunkID == chunkID {
			continue
		}
		filtered = append(filtered, item)
	}
	r.items = filtered
	return nil
}

func (r *documentChunkEmbeddingRepo) DeleteByAsset(ctx context.Context, organizationID string, assetID uuid.UUID) error {
	r.deletedCalls++
	r.items = nil
	return nil
}

func (r *documentChunkEmbeddingRepo) DeleteByAssetGeneration(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64) error {
	r.deletedCalls++
	r.items = nil
	return nil
}

var _ repository.DocumentChunkEmbeddingRepository = (*documentChunkEmbeddingRepo)(nil)

type documentChunkEmbeddingFakeVectorIndex struct {
	indexCalls  int
	deleteCalls int
}

func (f *documentChunkEmbeddingFakeVectorIndex) EnsureAssetIndexed(ctx context.Context, asset *model.DocumentAsset) error {
	return nil
}

func (f *documentChunkEmbeddingFakeVectorIndex) RebuildAssetIndex(ctx context.Context, asset *model.DocumentAsset) (int, error) {
	return 0, nil
}

func (f *documentChunkEmbeddingFakeVectorIndex) IndexChunkEmbeddings(ctx context.Context, asset *model.DocumentAsset, chunks []*model.DocumentChunk, embeddings []*model.DocumentChunkEmbedding, resetAsset bool) error {
	f.indexCalls++
	return nil
}

func (f *documentChunkEmbeddingFakeVectorIndex) DeleteAssetIndex(ctx context.Context, asset *model.DocumentAsset) error {
	f.deleteCalls++
	return nil
}

func (f *documentChunkEmbeddingFakeVectorIndex) DeleteChunkVector(ctx context.Context, asset *model.DocumentAsset, chunkID uuid.UUID) error {
	return nil
}

func (f *documentChunkEmbeddingFakeVectorIndex) DeleteChildVectorsByParent(ctx context.Context, asset *model.DocumentAsset, parentChunkID uuid.UUID) error {
	return nil
}

func (f *documentChunkEmbeddingFakeVectorIndex) Search(ctx context.Context, asset *model.DocumentAsset, queryVector []float64, limit int) ([]map[string]interface{}, error) {
	return nil, nil
}

var _ FileAssetVectorIndexService = (*documentChunkEmbeddingFakeVectorIndex)(nil)
