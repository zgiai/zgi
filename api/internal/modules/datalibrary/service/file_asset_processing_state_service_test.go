package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
)

func TestFileAssetProcessingStateServiceCreateOrReuseStoredAsset(t *testing.T) {
	repo := &fileAssetStateAssetRepo{}
	svc := NewFileAssetProcessingStateService(repo, &fileAssetStateProcessingRequestRepo{})

	asset, created, err := svc.CreateOrReuseStoredAsset(context.Background(), FileAssetCreateInput{
		OrganizationID: "org-1",
		Title:          "guide.pdf",
		SourceFileID:   "file-1",
		ContentHash:    "hash-1",
		CreatedBy:      "user-1",
	})
	if err != nil {
		t.Fatalf("CreateOrReuseStoredAsset: %v", err)
	}
	if !created || asset == nil || asset.ProductStatus != model.DocumentAssetProductStatusStoredOnly || asset.VectorStatus != model.DocumentAssetVectorStatusNone {
		t.Fatalf("asset=%+v created=%v", asset, created)
	}

	reused, created, err := svc.CreateOrReuseStoredAsset(context.Background(), FileAssetCreateInput{
		OrganizationID: "org-1",
		SourceFileID:   "file-1",
	})
	if err != nil {
		t.Fatalf("reuse stored asset: %v", err)
	}
	if created || reused.ID != asset.ID {
		t.Fatalf("reused=%+v created=%v", reused, created)
	}
}

func TestFileAssetProcessingStateServiceBeginAndReady(t *testing.T) {
	assetID := uuid.New()
	assetRepo := &fileAssetStateAssetRepo{
		asset: &model.DocumentAsset{
			ID:             assetID,
			OrganizationID: "org-1",
			SourceFileID:   "file-1",
			GenerationNo:   1,
		},
	}
	requestRepo := &fileAssetStateProcessingRequestRepo{}
	svc := NewFileAssetProcessingStateService(assetRepo, requestRepo)

	result, err := svc.BeginProcessingRequest(context.Background(), BeginProcessingRequestInput{
		OrganizationID: "org-1",
		AssetID:        assetID,
		TargetLevel:    model.DocumentProcessingLevelVectorize,
		RequestedBy:    "user-1",
		IncrementRun:   true,
	})
	if err != nil {
		t.Fatalf("BeginProcessingRequest: %v", err)
	}
	if result.ProcessingRunID == uuid.Nil || result.GenerationNo != 2 ||
		result.Asset.ProductStatus != model.DocumentAssetProductStatusParsing ||
		result.Asset.ProcessingRunID == nil || *result.Asset.ProcessingRunID != result.ProcessingRunID {
		t.Fatalf("begin result=%+v", result)
	}

	parseArtifactID := uuid.New()
	chunkArtifactSetID := uuid.New()
	ready, err := svc.MarkReady(context.Background(), ReadyStateInput{
		RunStateInput: RunStateInput{
			OrganizationID:  "org-1",
			AssetID:         assetID,
			ProcessingRunID: result.ProcessingRunID,
			GenerationNo:    result.GenerationNo,
			ParseArtifactID: &parseArtifactID,
		},
		ChunkArtifactSetID: &chunkArtifactSetID,
		ChunkCount:         7,
		EmbeddingProvider:  "openai",
		EmbeddingModel:     "text-embedding-3-small",
		EmbeddingDimension: 1536,
	})
	if err != nil {
		t.Fatalf("MarkReady: %v", err)
	}
	if ready.ProductStatus != model.DocumentAssetProductStatusReady ||
		ready.VectorStatus != model.DocumentAssetVectorStatusReady ||
		ready.ChunkCount != 7 ||
		ready.EmbeddingProvider == nil || *ready.EmbeddingProvider != "openai" {
		t.Fatalf("ready asset=%+v", ready)
	}
}

func TestFileAssetProcessingStateServiceRejectsStaleRun(t *testing.T) {
	assetID := uuid.New()
	currentRunID := uuid.New()
	assetRepo := &fileAssetStateAssetRepo{
		asset: &model.DocumentAsset{
			ID:              assetID,
			OrganizationID:  "org-1",
			SourceFileID:    "file-1",
			ProductStatus:   model.DocumentAssetProductStatusParsing,
			ProcessingRunID: &currentRunID,
			GenerationNo:    4,
		},
	}
	svc := NewFileAssetProcessingStateService(assetRepo, &fileAssetStateProcessingRequestRepo{})

	_, err := svc.MarkGenerating(context.Background(), RunStateInput{
		OrganizationID:  "org-1",
		AssetID:         assetID,
		ProcessingRunID: uuid.New(),
		GenerationNo:    4,
	})
	if !errors.Is(err, ErrProcessingRunMismatch) {
		t.Fatalf("err=%v", err)
	}
}

type fileAssetStateAssetRepo struct {
	asset *model.DocumentAsset
}

func (r *fileAssetStateAssetRepo) CreateAsset(ctx context.Context, item *model.DocumentAsset) error {
	if err := item.BeforeCreate(nil); err != nil {
		return err
	}
	r.asset = item
	return nil
}

func (r *fileAssetStateAssetRepo) CreateAssetWithVersion(ctx context.Context, asset *model.DocumentAsset, version *model.DocumentVersion) error {
	r.asset = asset
	return nil
}

func (r *fileAssetStateAssetRepo) GetAssetByID(ctx context.Context, id uuid.UUID) (*model.DocumentAsset, error) {
	if r.asset == nil || r.asset.ID != id {
		return nil, nil
	}
	return cloneAsset(r.asset), nil
}

func (r *fileAssetStateAssetRepo) FindAssetBySourceFileID(ctx context.Context, organizationID string, sourceFileID string) (*model.DocumentAsset, error) {
	if r.asset == nil || r.asset.OrganizationID != organizationID || r.asset.SourceFileID != sourceFileID {
		return nil, nil
	}
	return cloneAsset(r.asset), nil
}

func (r *fileAssetStateAssetRepo) FindAssetsBySourceFileIDs(ctx context.Context, organizationID string, sourceFileIDs []string) (map[string]*model.DocumentAsset, error) {
	return map[string]*model.DocumentAsset{}, nil
}

func (r *fileAssetStateAssetRepo) ListAssets(ctx context.Context, filter repository.DocumentAssetListFilter) ([]*model.DocumentAsset, int64, error) {
	if r.asset == nil {
		return nil, 0, nil
	}
	return []*model.DocumentAsset{cloneAsset(r.asset)}, 1, nil
}

func (r *fileAssetStateAssetRepo) UpdateCurrentResult(ctx context.Context, id uuid.UUID, patch repository.DocumentAssetCurrentResultPatch) (*model.DocumentAsset, error) {
	if r.asset == nil || r.asset.ID != id || r.asset.OrganizationID != patch.OrganizationID {
		return nil, nil
	}
	if patch.RequireProcessingRunID != nil {
		if r.asset.ProcessingRunID == nil || *r.asset.ProcessingRunID != *patch.RequireProcessingRunID {
			return cloneAsset(r.asset), nil
		}
	}
	if patch.RequireGenerationNo != nil && r.asset.GenerationNo != *patch.RequireGenerationNo {
		return cloneAsset(r.asset), nil
	}
	if patch.ProductStatus != nil {
		r.asset.ProductStatus = *patch.ProductStatus
	}
	if patch.ProcessingStage != nil {
		r.asset.ProcessingStage = patch.ProcessingStage
	}
	if patch.ProcessingProgress != nil {
		r.asset.ProcessingProgress = *patch.ProcessingProgress
	}
	if patch.ActiveProcessingRequestID != nil {
		r.asset.ActiveProcessingRequestID = patch.ActiveProcessingRequestID
	}
	if patch.ProcessingRunID != nil {
		r.asset.ProcessingRunID = patch.ProcessingRunID
	}
	if patch.GenerationNo != nil {
		r.asset.GenerationNo = *patch.GenerationNo
	}
	if patch.ParseArtifactID != nil {
		r.asset.ParseArtifactID = patch.ParseArtifactID
	}
	if patch.ChunkArtifactSetID != nil {
		r.asset.ChunkArtifactSetID = patch.ChunkArtifactSetID
	}
	if patch.ChunkCount != nil {
		r.asset.ChunkCount = *patch.ChunkCount
	}
	if patch.EmbeddingProvider != nil {
		r.asset.EmbeddingProvider = patch.EmbeddingProvider
	}
	if patch.EmbeddingModel != nil {
		r.asset.EmbeddingModel = patch.EmbeddingModel
	}
	if patch.EmbeddingDimension != nil {
		r.asset.EmbeddingDimension = patch.EmbeddingDimension
	}
	if patch.VectorStatus != nil {
		r.asset.VectorStatus = *patch.VectorStatus
	}
	if patch.LastErrorCode != nil {
		r.asset.LastErrorCode = patch.LastErrorCode
	}
	if patch.LastErrorMessage != nil {
		r.asset.LastErrorMessage = patch.LastErrorMessage
	}
	return cloneAsset(r.asset), nil
}

func (r *fileAssetStateAssetRepo) CreateVersion(ctx context.Context, item *model.DocumentVersion) error {
	return nil
}

func (r *fileAssetStateAssetRepo) GetVersionByID(ctx context.Context, id uuid.UUID) (*model.DocumentVersion, error) {
	return nil, nil
}

func (r *fileAssetStateAssetRepo) ListVersionsByAssetID(ctx context.Context, assetID uuid.UUID) ([]*model.DocumentVersion, error) {
	return nil, nil
}

var _ repository.DocumentAssetRepository = (*fileAssetStateAssetRepo)(nil)

type fileAssetStateProcessingRequestRepo struct {
	created *model.ProcessingRequest
}

func (r *fileAssetStateProcessingRequestRepo) Create(ctx context.Context, item *model.ProcessingRequest) error {
	if err := item.BeforeCreate(nil); err != nil {
		return err
	}
	r.created = item
	return nil
}

func (r *fileAssetStateProcessingRequestRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.ProcessingRequest, error) {
	return r.created, nil
}

func (r *fileAssetStateProcessingRequestRepo) List(ctx context.Context, filter repository.ProcessingRequestListFilter) ([]*model.ProcessingRequest, int64, error) {
	return nil, 0, nil
}

func (r *fileAssetStateProcessingRequestRepo) StatusSummaryByAssetID(ctx context.Context, organizationID string, assetID uuid.UUID) ([]repository.ProcessingRequestStatusSummary, error) {
	return nil, nil
}

func (r *fileAssetStateProcessingRequestRepo) QueueSummary(ctx context.Context, filter repository.ProcessingRequestQueueSummaryFilter) ([]repository.ProcessingRequestQueueSummary, error) {
	return nil, nil
}

func (r *fileAssetStateProcessingRequestRepo) TransitionStatus(ctx context.Context, id uuid.UUID, patch repository.ProcessingRequestStatusPatch) (*model.ProcessingRequest, error) {
	return r.created, nil
}

func (r *fileAssetStateProcessingRequestRepo) ClaimNextQueued(ctx context.Context, filter repository.ProcessingRequestClaimFilter) (*model.ProcessingRequest, error) {
	return nil, nil
}

var _ repository.ProcessingRequestRepository = (*fileAssetStateProcessingRequestRepo)(nil)

func cloneAsset(item *model.DocumentAsset) *model.DocumentAsset {
	if item == nil {
		return nil
	}
	cloned := *item
	return &cloned
}
