package worker

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
	datalibraryservice "github.com/zgiai/zgi/api/internal/modules/datalibrary/service"
)

func TestFileProcessRunnerStaleRunFailureDoesNotMutateCurrentAsset(t *testing.T) {
	ctx := context.Background()
	assetID := uuid.New()
	staleRunID := uuid.New()
	currentRunID := uuid.New()
	asset := &model.DocumentAsset{
		ID:                 assetID,
		OrganizationID:     "org-1",
		ProductStatus:      model.DocumentAssetProductStatusParsing,
		ProcessingRunID:    &currentRunID,
		GenerationNo:       2,
		ProcessingProgress: 5,
	}
	request := &model.ProcessingRequest{
		ID:             staleRunID,
		OrganizationID: "org-1",
		AssetID:        assetID,
		TargetLevel:    model.DocumentProcessingLevelVectorize,
		Status:         model.ProcessingRequestStatusQueued,
	}
	requestRepo := &singleActiveProcessingRequestRepo{request: request}
	state := &singleActiveAssetStateService{}
	runner := NewFileProcessRunner(FileProcessRunnerDeps{
		ProcessingRequests: requestRepo,
		Assets:             &singleActiveAssetRepo{asset: asset},
		State:              state,
		ProcessingService:  datalibraryservice.NewProcessingRequestService(requestRepo),
	})

	err := runner.Run(ctx, staleRunID)
	if !errors.Is(err, datalibraryservice.ErrProcessingRunMismatch) {
		t.Fatalf("Run err=%v want ErrProcessingRunMismatch", err)
	}
	if state.failedCalled {
		t.Fatalf("stale file process run must not mark current asset failed")
	}
	if request.Status != model.ProcessingRequestStatusFailed || request.ErrorCode != "processing_run_mismatch" {
		t.Fatalf("request status=%s error_code=%s", request.Status, request.ErrorCode)
	}
}

func TestGenerateCurrentResultRunnerStaleGenerationFailureDoesNotMutateCurrentAsset(t *testing.T) {
	ctx := context.Background()
	assetID := uuid.New()
	oldRunID := uuid.New()
	currentRunID := uuid.New()
	parseArtifactID := uuid.New()
	asset := &model.DocumentAsset{
		ID:              assetID,
		OrganizationID:  "org-1",
		ProductStatus:   model.DocumentAssetProductStatusGenerating,
		ProcessingRunID: &currentRunID,
		GenerationNo:    2,
		ParseArtifactID: &parseArtifactID,
	}
	request := &model.ProcessingRequest{
		ID:             uuid.New(),
		OrganizationID: "org-1",
		AssetID:        assetID,
		TargetLevel:    model.DocumentProcessingLevelVectorize,
		Status:         model.ProcessingRequestStatusQueued,
		RequestMetadata: map[string]any{
			"processing_run_id": oldRunID.String(),
			"generation_no":     int64(1),
		},
	}
	requestRepo := &singleActiveProcessingRequestRepo{request: request}
	state := &singleActiveAssetStateService{}
	runner := NewGenerateCurrentResultRunner(GenerateCurrentResultRunnerDeps{
		ProcessingRequests: requestRepo,
		Assets:             &singleActiveAssetRepo{asset: asset},
		State:              state,
		ProcessingService:  datalibraryservice.NewProcessingRequestService(requestRepo),
	})

	err := runner.Run(ctx, request.ID)
	if !errors.Is(err, datalibraryservice.ErrProcessingRunMismatch) {
		t.Fatalf("Run err=%v want ErrProcessingRunMismatch", err)
	}
	if state.failedCalled {
		t.Fatalf("stale generate run must not mark current asset failed")
	}
	if request.Status != model.ProcessingRequestStatusFailed || request.ErrorCode != "processing_run_mismatch" {
		t.Fatalf("request status=%s error_code=%s", request.Status, request.ErrorCode)
	}
}

func TestGenerateCurrentResultRunnerEnqueuesDatasetRefSyncs(t *testing.T) {
	ctx := context.Background()
	assetID := uuid.New()
	refID := uuid.New()
	refStore := &fakeGenerateCurrentResultRefStore{
		refs: []*model.KnowledgeBaseAssetRef{
			{
				ID:             refID,
				OrganizationID: "org-1",
				DatasetID:      "dataset-1",
				AssetID:        assetID,
				SyncStatus:     model.KnowledgeBaseAssetRefSyncStatusSynced,
			},
		},
	}
	enqueuer := &fakeGenerateCurrentResultDatasetRefSyncEnqueuer{}
	runner := NewGenerateCurrentResultRunner(GenerateCurrentResultRunnerDeps{
		Refs:           refStore,
		DatasetRefSync: enqueuer,
	})
	asset := &model.DocumentAsset{
		ID:             assetID,
		OrganizationID: "org-1",
		GenerationNo:   9,
	}

	err := runner.enqueueDatasetRefSyncs(ctx, asset, asset.GenerationNo)
	if err != nil {
		t.Fatalf("enqueueDatasetRefSyncs: %v", err)
	}
	if refStore.pendingRefID != refID || refStore.pendingSyncRunID == uuid.Nil {
		t.Fatalf("pending_ref=%s sync_run=%s", refStore.pendingRefID, refStore.pendingSyncRunID)
	}
	if len(enqueuer.items) != 1 ||
		enqueuer.items[0].refID != refID ||
		enqueuer.items[0].assetID != assetID ||
		enqueuer.items[0].datasetID != "dataset-1" ||
		enqueuer.items[0].generationNo != 9 ||
		enqueuer.items[0].syncRunID != refStore.pendingSyncRunID {
		t.Fatalf("items=%+v pending_sync=%s", enqueuer.items, refStore.pendingSyncRunID)
	}
}

func TestGenerateCurrentResultRunnerGeneratesEmbeddingsForAllTargets(t *testing.T) {
	embedding := &fakeGenerateCurrentResultEmbeddingService{}
	runner := &GenerateCurrentResultRunner{embedding: embedding}
	assetID := uuid.New()
	runID := uuid.New()

	result, total, err := runner.generateEmbeddingsForTargets(context.Background(), datalibraryservice.GenerateDocumentChunkEmbeddingsInput{
		OrganizationID:  "org-1",
		AssetID:         assetID,
		ProcessingRunID: runID,
		GenerationNo:    3,
		RequestedBy:     "user-1",
	}, []datalibraryservice.EmbeddingTarget{
		{Provider: "provider-a", Model: "model-a"},
		{Provider: "provider-b", Model: "model-b"},
	})
	if err != nil {
		t.Fatalf("generateEmbeddingsForTargets: %v", err)
	}
	if result == nil || result.EmbeddingProvider != "provider-a" || result.EmbeddingModel != "model-a" {
		t.Fatalf("first result=%+v", result)
	}
	if total != 2 {
		t.Fatalf("total=%d want 2", total)
	}
	if len(embedding.calls) != 2 {
		t.Fatalf("calls=%+v", embedding.calls)
	}
	if embedding.calls[0].method != "generate" ||
		embedding.calls[0].provider != "provider-a" ||
		embedding.calls[0].model != "model-a" {
		t.Fatalf("first call=%+v", embedding.calls[0])
	}
	if embedding.calls[1].method != "additional" ||
		embedding.calls[1].provider != "provider-b" ||
		embedding.calls[1].model != "model-b" {
		t.Fatalf("second call=%+v", embedding.calls[1])
	}
}

type singleActiveAssetRepo struct {
	asset *model.DocumentAsset
}

func (r *singleActiveAssetRepo) CreateAsset(ctx context.Context, item *model.DocumentAsset) error {
	r.asset = item
	return nil
}

func (r *singleActiveAssetRepo) CreateAssetWithVersion(ctx context.Context, asset *model.DocumentAsset, version *model.DocumentVersion) error {
	r.asset = asset
	return nil
}

func (r *singleActiveAssetRepo) GetAssetByID(ctx context.Context, id uuid.UUID) (*model.DocumentAsset, error) {
	if r.asset == nil || r.asset.ID != id {
		return nil, nil
	}
	cloned := *r.asset
	return &cloned, nil
}

func (r *singleActiveAssetRepo) FindAssetBySourceFileID(ctx context.Context, organizationID string, sourceFileID string) (*model.DocumentAsset, error) {
	return nil, nil
}

func (r *singleActiveAssetRepo) FindAssetsBySourceFileIDs(ctx context.Context, organizationID string, sourceFileIDs []string) (map[string]*model.DocumentAsset, error) {
	return map[string]*model.DocumentAsset{}, nil
}

func (r *singleActiveAssetRepo) ListAssets(ctx context.Context, filter repository.DocumentAssetListFilter) ([]*model.DocumentAsset, int64, error) {
	return nil, 0, nil
}

func (r *singleActiveAssetRepo) UpdateCurrentResult(ctx context.Context, id uuid.UUID, patch repository.DocumentAssetCurrentResultPatch) (*model.DocumentAsset, error) {
	if r.asset == nil || r.asset.ID != id {
		return nil, nil
	}
	if patch.RequireProcessingRunID != nil {
		if r.asset.ProcessingRunID == nil || *r.asset.ProcessingRunID != *patch.RequireProcessingRunID {
			cloned := *r.asset
			return &cloned, nil
		}
	}
	if patch.RequireGenerationNo != nil && r.asset.GenerationNo != *patch.RequireGenerationNo {
		cloned := *r.asset
		return &cloned, nil
	}
	cloned := *r.asset
	return &cloned, nil
}

func (r *singleActiveAssetRepo) CreateVersion(ctx context.Context, item *model.DocumentVersion) error {
	return nil
}

func (r *singleActiveAssetRepo) GetVersionByID(ctx context.Context, id uuid.UUID) (*model.DocumentVersion, error) {
	return nil, nil
}

type fakeGenerateCurrentResultRefStore struct {
	refs             []*model.KnowledgeBaseAssetRef
	pendingRefID     uuid.UUID
	pendingSyncRunID uuid.UUID
}

func (r *fakeGenerateCurrentResultRefStore) ListActiveByAsset(ctx context.Context, organizationID string, assetID uuid.UUID) ([]*model.KnowledgeBaseAssetRef, error) {
	var out []*model.KnowledgeBaseAssetRef
	for _, ref := range r.refs {
		if ref.OrganizationID == organizationID && ref.AssetID == assetID {
			out = append(out, ref)
		}
	}
	return out, nil
}

func (r *fakeGenerateCurrentResultRefStore) MarkPending(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, errorCode, errorMessage *string) (*model.KnowledgeBaseAssetRef, error) {
	r.pendingRefID = id
	r.pendingSyncRunID = syncRunID
	for _, ref := range r.refs {
		if ref.ID == id && ref.OrganizationID == organizationID {
			ref.SyncStatus = model.KnowledgeBaseAssetRefSyncStatusPending
			ref.SyncRunID = &syncRunID
			return ref, nil
		}
	}
	return nil, nil
}

type fakeGenerateCurrentResultDatasetRefSyncEnqueuer struct {
	items []fakeGenerateCurrentResultDatasetRefSyncItem
}

type fakeGenerateCurrentResultDatasetRefSyncItem struct {
	refID        uuid.UUID
	assetID      uuid.UUID
	datasetID    string
	generationNo int64
	syncRunID    uuid.UUID
}

func (e *fakeGenerateCurrentResultDatasetRefSyncEnqueuer) EnqueueDatasetRefSync(ctx context.Context, refID uuid.UUID, assetID uuid.UUID, datasetID string, generationNo int64, syncRunID uuid.UUID) error {
	e.items = append(e.items, fakeGenerateCurrentResultDatasetRefSyncItem{
		refID:        refID,
		assetID:      assetID,
		datasetID:    datasetID,
		generationNo: generationNo,
		syncRunID:    syncRunID,
	})
	return nil
}

func (r *singleActiveAssetRepo) ListVersionsByAssetID(ctx context.Context, assetID uuid.UUID) ([]*model.DocumentVersion, error) {
	return nil, nil
}

var _ repository.DocumentAssetRepository = (*singleActiveAssetRepo)(nil)

type fakeGenerateCurrentResultEmbeddingService struct {
	calls []fakeGenerateCurrentResultEmbeddingCall
}

type fakeGenerateCurrentResultEmbeddingCall struct {
	method   string
	provider string
	model    string
}

func (s *fakeGenerateCurrentResultEmbeddingService) GenerateEmbeddings(ctx context.Context, input datalibraryservice.GenerateDocumentChunkEmbeddingsInput) (*datalibraryservice.GenerateDocumentChunkEmbeddingsResult, error) {
	s.calls = append(s.calls, fakeGenerateCurrentResultEmbeddingCall{method: "generate", provider: input.EmbeddingProvider, model: input.EmbeddingModel})
	return &datalibraryservice.GenerateDocumentChunkEmbeddingsResult{
		EmbeddingCount:    1,
		EmbeddingProvider: input.EmbeddingProvider,
		EmbeddingModel:    input.EmbeddingModel,
	}, nil
}

func (s *fakeGenerateCurrentResultEmbeddingService) GenerateAdditionalEmbeddings(ctx context.Context, input datalibraryservice.GenerateDocumentChunkEmbeddingsInput) (*datalibraryservice.GenerateDocumentChunkEmbeddingsResult, error) {
	s.calls = append(s.calls, fakeGenerateCurrentResultEmbeddingCall{method: "additional", provider: input.EmbeddingProvider, model: input.EmbeddingModel})
	return &datalibraryservice.GenerateDocumentChunkEmbeddingsResult{
		EmbeddingCount:    1,
		EmbeddingProvider: input.EmbeddingProvider,
		EmbeddingModel:    input.EmbeddingModel,
	}, nil
}

func (s *fakeGenerateCurrentResultEmbeddingService) GenerateChunkEmbedding(ctx context.Context, input datalibraryservice.GenerateDocumentChunkEmbeddingInput) (*model.DocumentChunkEmbedding, error) {
	return nil, nil
}

var _ datalibraryservice.DocumentChunkEmbeddingService = (*fakeGenerateCurrentResultEmbeddingService)(nil)

type singleActiveProcessingRequestRepo struct {
	request *model.ProcessingRequest
}

func (r *singleActiveProcessingRequestRepo) Create(ctx context.Context, item *model.ProcessingRequest) error {
	r.request = item
	return nil
}

func (r *singleActiveProcessingRequestRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.ProcessingRequest, error) {
	if r.request == nil || r.request.ID != id {
		return nil, nil
	}
	return r.request, nil
}

func (r *singleActiveProcessingRequestRepo) List(ctx context.Context, filter repository.ProcessingRequestListFilter) ([]*model.ProcessingRequest, int64, error) {
	return nil, 0, nil
}

func (r *singleActiveProcessingRequestRepo) StatusSummaryByAssetID(ctx context.Context, organizationID string, assetID uuid.UUID) ([]repository.ProcessingRequestStatusSummary, error) {
	return nil, nil
}

func (r *singleActiveProcessingRequestRepo) QueueSummary(ctx context.Context, filter repository.ProcessingRequestQueueSummaryFilter) ([]repository.ProcessingRequestQueueSummary, error) {
	return nil, nil
}

func (r *singleActiveProcessingRequestRepo) TransitionStatus(ctx context.Context, id uuid.UUID, patch repository.ProcessingRequestStatusPatch) (*model.ProcessingRequest, error) {
	if r.request == nil || r.request.ID != id || r.request.OrganizationID != patch.OrganizationID {
		return nil, nil
	}
	allowed := len(patch.AllowedFrom) == 0
	for _, status := range patch.AllowedFrom {
		if r.request.Status == status {
			allowed = true
			break
		}
	}
	if !allowed {
		return r.request, nil
	}
	r.request.Status = patch.Status
	if patch.ExecutorKey != nil {
		r.request.ExecutorKey = *patch.ExecutorKey
	}
	if patch.ErrorCode != nil {
		r.request.ErrorCode = *patch.ErrorCode
	}
	if patch.ErrorMessage != nil {
		r.request.ErrorMessage = *patch.ErrorMessage
	}
	r.request.AttemptCount += patch.AttemptCountDelta
	r.request.QueuedAt = patch.QueuedAt
	r.request.StartedAt = patch.StartedAt
	r.request.CompletedAt = patch.CompletedAt
	r.request.FailedAt = patch.FailedAt
	r.request.CanceledAt = patch.CanceledAt
	if patch.ExecutionMetadata != nil {
		r.request.ExecutionMetadata = patch.ExecutionMetadata
	}
	return r.request, nil
}

func (r *singleActiveProcessingRequestRepo) ClaimNextQueued(ctx context.Context, filter repository.ProcessingRequestClaimFilter) (*model.ProcessingRequest, error) {
	return nil, nil
}

var _ repository.ProcessingRequestRepository = (*singleActiveProcessingRequestRepo)(nil)

type singleActiveAssetStateService struct {
	failedCalled bool
}

func (s *singleActiveAssetStateService) CreateOrReuseStoredAsset(ctx context.Context, input datalibraryservice.FileAssetCreateInput) (*model.DocumentAsset, bool, error) {
	return nil, false, nil
}

func (s *singleActiveAssetStateService) PrepareFileReplacement(ctx context.Context, input datalibraryservice.FileReplacementInput) (*model.DocumentAsset, error) {
	return nil, nil
}

func (s *singleActiveAssetStateService) BeginProcessingRequest(ctx context.Context, input datalibraryservice.BeginProcessingRequestInput) (*datalibraryservice.BeginProcessingRequestResult, error) {
	return nil, nil
}

func (s *singleActiveAssetStateService) MarkParsing(ctx context.Context, input datalibraryservice.RunStateInput) (*model.DocumentAsset, error) {
	return nil, nil
}

func (s *singleActiveAssetStateService) MarkConfirming(ctx context.Context, input datalibraryservice.RunStateInput) (*model.DocumentAsset, error) {
	return nil, nil
}

func (s *singleActiveAssetStateService) MarkGenerating(ctx context.Context, input datalibraryservice.RunStateInput) (*model.DocumentAsset, error) {
	return nil, nil
}

func (s *singleActiveAssetStateService) MarkReady(ctx context.Context, input datalibraryservice.ReadyStateInput) (*model.DocumentAsset, error) {
	return nil, nil
}

func (s *singleActiveAssetStateService) MarkFailed(ctx context.Context, input datalibraryservice.FailedStateInput) (*model.DocumentAsset, error) {
	s.failedCalled = true
	return nil, nil
}

var _ datalibraryservice.FileAssetProcessingStateService = (*singleActiveAssetStateService)(nil)
