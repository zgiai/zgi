package service

import (
	"context"
	"testing"

	"github.com/google/uuid"
	datalibModel "github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	datalibRepo "github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
	datasetModel "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	fileModel "github.com/zgiai/zgi/api/internal/modules/file_process/model"
)

func TestKnowledgeBaseFileRefServiceListsAddableCandidates(t *testing.T) {
	assetID := uuid.New()
	provider := "openai"
	embeddingModel := "text-embedding-3-small"
	svc := newKnowledgeBaseFileRefTestService(&fakeKnowledgeBaseFileRefDeps{
		dataset: &datasetModel.Dataset{
			ID:                     "dataset-1",
			OrganizationID:         "org-1",
			EmbeddingModelProvider: &provider,
			EmbeddingModel:         &embeddingModel,
		},
		assets: []*datalibModel.DocumentAsset{
			{
				ID:                assetID,
				OrganizationID:    "org-1",
				SourceFileID:      "file-1",
				Title:             "Asset title",
				ProductStatus:     datalibModel.DocumentAssetProductStatusReady,
				VectorStatus:      datalibModel.DocumentAssetVectorStatusReady,
				GenerationNo:      7,
				EmbeddingProvider: &provider,
				EmbeddingModel:    &embeddingModel,
			},
		},
		files: map[string]*fileModel.UploadFile{
			"file-1": {ID: "file-1", Name: "handbook.pdf", Extension: "pdf", Size: 2048},
		},
		referenceCount: 2,
		chunkCount:     3,
		embeddingCount: 3,
	})

	result, err := svc.ListCandidates(context.Background(), KnowledgeBaseFileCandidateRequest{
		OrganizationID: "org-1",
		DatasetID:      "dataset-1",
		Filter:         FileCandidateFilterAddable,
		Limit:          20,
	})
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("result=%+v", result)
	}
	item := result.Items[0]
	if !item.Addable ||
		item.Reason != "" ||
		item.Name != "handbook.pdf" ||
		item.FileExtension != "pdf" ||
		item.FileSize == nil ||
		*item.FileSize != 2048 ||
		item.ReferenceCount != 2 ||
		item.AssetID != assetID ||
		item.ChunkCount != 3 ||
		item.EmbeddingCount != 3 {
		t.Fatalf("candidate=%+v", item)
	}
}

func TestKnowledgeBaseFileRefServiceSearchesCandidateAssetTitleWhenFileMissing(t *testing.T) {
	assetID := uuid.New()
	svc := newKnowledgeBaseFileRefTestService(&fakeKnowledgeBaseFileRefDeps{
		dataset: &datasetModel.Dataset{
			ID:             "dataset-1",
			OrganizationID: "org-1",
		},
		assets: []*datalibModel.DocumentAsset{
			{
				ID:             assetID,
				OrganizationID: "org-1",
				SourceFileID:   "missing-file",
				Title:          "Asset Fallback Title",
				ProductStatus:  datalibModel.DocumentAssetProductStatusReady,
				VectorStatus:   datalibModel.DocumentAssetVectorStatusReady,
				GenerationNo:   1,
			},
		},
		files:          map[string]*fileModel.UploadFile{},
		chunkCount:     1,
		embeddingCount: 1,
	})

	result, err := svc.ListCandidates(context.Background(), KnowledgeBaseFileCandidateRequest{
		OrganizationID: "org-1",
		DatasetID:      "dataset-1",
		Filter:         FileCandidateFilterAll,
		Keyword:        "fallback",
	})
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].AssetID != assetID || result.Items[0].Name != "Asset Fallback Title" {
		t.Fatalf("items=%+v", result.Items)
	}
}

func TestKnowledgeBaseFileRefServiceRejectsMismatchedEmbeddingModel(t *testing.T) {
	assetID := uuid.New()
	datasetProvider := "openai"
	datasetModelName := "text-embedding-3-small"
	assetProvider := "openai"
	assetModelName := "other-model"
	svc := newKnowledgeBaseFileRefTestService(&fakeKnowledgeBaseFileRefDeps{
		dataset: &datasetModel.Dataset{
			ID:                     "dataset-1",
			OrganizationID:         "org-1",
			EmbeddingModelProvider: &datasetProvider,
			EmbeddingModel:         &datasetModelName,
		},
		assets: []*datalibModel.DocumentAsset{
			{
				ID:                assetID,
				OrganizationID:    "org-1",
				SourceFileID:      "file-1",
				ProductStatus:     datalibModel.DocumentAssetProductStatusReady,
				VectorStatus:      datalibModel.DocumentAssetVectorStatusReady,
				GenerationNo:      1,
				EmbeddingProvider: &assetProvider,
				EmbeddingModel:    &assetModelName,
			},
		},
		files: map[string]*fileModel.UploadFile{
			"file-1": {ID: "file-1", Name: "handbook.pdf"},
		},
		chunkCount:     1,
		embeddingCount: 1,
	})

	result, err := svc.ListCandidates(context.Background(), KnowledgeBaseFileCandidateRequest{
		OrganizationID: "org-1",
		DatasetID:      "dataset-1",
		Filter:         FileCandidateFilterAll,
	})
	if err != nil {
		t.Fatalf("ListCandidates: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].Addable || result.Items[0].Reason != FileCandidateReasonEmbeddingModelMismatch {
		t.Fatalf("items=%+v", result.Items)
	}
}

func TestKnowledgeBaseFileRefServiceCreatesPendingRefs(t *testing.T) {
	assetID := uuid.New()
	provider := "openai"
	embeddingModel := "text-embedding-3-small"
	deps := &fakeKnowledgeBaseFileRefDeps{
		dataset: &datasetModel.Dataset{
			ID:                     "dataset-1",
			OrganizationID:         "org-1",
			EmbeddingModelProvider: &provider,
			EmbeddingModel:         &embeddingModel,
		},
		assets: []*datalibModel.DocumentAsset{
			{
				ID:                assetID,
				OrganizationID:    "org-1",
				SourceFileID:      "file-1",
				ProductStatus:     datalibModel.DocumentAssetProductStatusReady,
				VectorStatus:      datalibModel.DocumentAssetVectorStatusReady,
				GenerationNo:      1,
				EmbeddingProvider: &provider,
				EmbeddingModel:    &embeddingModel,
			},
		},
		chunkCount:     2,
		embeddingCount: 2,
	}
	svc := newKnowledgeBaseFileRefTestService(deps)

	result, err := svc.CreateRefs(context.Background(), KnowledgeBaseFileRefCreateRequest{
		OrganizationID: "org-1",
		DatasetID:      "dataset-1",
		AssetIDs:       []uuid.UUID{assetID},
		CreatedBy:      "account-1",
	})
	if err != nil {
		t.Fatalf("CreateRefs: %v", err)
	}
	if len(result.Items) != 1 ||
		!result.Items[0].Success ||
		result.Items[0].SyncRunID == nil ||
		*result.Items[0].SyncRunID == uuid.Nil ||
		result.Items[0].GenerationNo != 1 {
		t.Fatalf("result=%+v", result)
	}
	if deps.created == nil ||
		deps.created.AssetID != assetID ||
		deps.created.DatasetID != "dataset-1" ||
		deps.created.SyncStatus != datalibModel.KnowledgeBaseAssetRefSyncStatusPending ||
		deps.created.SyncRunID == nil ||
		deps.created.CreatedBy != "account-1" {
		t.Fatalf("created=%+v", deps.created)
	}
}

func TestKnowledgeBaseFileRefServiceListsRefsWithAssetAndFileState(t *testing.T) {
	assetID := uuid.New()
	refID := uuid.New()
	documentID := uuid.New()
	syncedGeneration := int64(3)
	deps := &fakeKnowledgeBaseFileRefDeps{
		dataset: &datasetModel.Dataset{
			ID:             "dataset-1",
			OrganizationID: "org-1",
		},
		assets: []*datalibModel.DocumentAsset{
			{
				ID:             assetID,
				OrganizationID: "org-1",
				SourceFileID:   "file-1",
				Title:          "Asset title",
				ProductStatus:  datalibModel.DocumentAssetProductStatusReady,
				GenerationNo:   3,
			},
		},
		files: map[string]*fileModel.UploadFile{
			"file-1": {ID: "file-1", Name: "handbook.pdf"},
		},
		refs: []*datalibModel.KnowledgeBaseAssetRef{
			{
				ID:                 refID,
				OrganizationID:     "org-1",
				DatasetID:          "dataset-1",
				AssetID:            assetID,
				DatasetDocumentID:  &documentID,
				SyncStatus:         datalibModel.KnowledgeBaseAssetRefSyncStatusSynced,
				SyncedGenerationNo: &syncedGeneration,
			},
		},
		documents: []*datasetModel.Document{
			{
				ID:           documentID.String(),
				Enabled:      false,
				SegmentCount: 24,
			},
		},
	}
	svc := newKnowledgeBaseFileRefTestService(deps)

	result, err := svc.ListRefs(context.Background(), KnowledgeBaseFileRefListRequest{
		OrganizationID: "org-1",
		DatasetID:      "dataset-1",
		SyncStatus:     datalibModel.KnowledgeBaseAssetRefSyncStatusSynced,
	})
	if err != nil {
		t.Fatalf("ListRefs: %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 {
		t.Fatalf("result=%+v", result)
	}
	item := result.Items[0]
	if item.ID != refID ||
		item.AssetID != assetID ||
		item.DatasetDocumentID == nil ||
		*item.DatasetDocumentID != documentID ||
		item.FileName != "handbook.pdf" ||
		item.SyncStatus != datalibModel.KnowledgeBaseAssetRefSyncStatusSynced ||
		item.SyncedGenerationNo == nil ||
		*item.SyncedGenerationNo != syncedGeneration ||
		item.DatasetDocumentEnabled == nil ||
		*item.DatasetDocumentEnabled ||
		item.DatasetDocumentSegmentCount == nil ||
		*item.DatasetDocumentSegmentCount != 24 {
		t.Fatalf("item=%+v", item)
	}
	if deps.lastRefFilter.SyncStatus != datalibModel.KnowledgeBaseAssetRefSyncStatusSynced {
		t.Fatalf("filter=%+v", deps.lastRefFilter)
	}
}

func TestKnowledgeBaseFileRefServiceRetriesRef(t *testing.T) {
	assetID := uuid.New()
	refID := uuid.New()
	oldSyncRunID := uuid.New()
	provider := "openai"
	embeddingModel := "text-embedding-3-small"
	deps := &fakeKnowledgeBaseFileRefDeps{
		dataset: &datasetModel.Dataset{
			ID:                     "dataset-1",
			OrganizationID:         "org-1",
			EmbeddingModelProvider: &provider,
			EmbeddingModel:         &embeddingModel,
		},
		assets: []*datalibModel.DocumentAsset{
			{
				ID:                assetID,
				OrganizationID:    "org-1",
				SourceFileID:      "file-1",
				ProductStatus:     datalibModel.DocumentAssetProductStatusReady,
				VectorStatus:      datalibModel.DocumentAssetVectorStatusReady,
				GenerationNo:      7,
				EmbeddingProvider: &provider,
				EmbeddingModel:    &embeddingModel,
			},
		},
		refs: []*datalibModel.KnowledgeBaseAssetRef{
			{
				ID:             refID,
				OrganizationID: "org-1",
				DatasetID:      "dataset-1",
				AssetID:        assetID,
				SyncStatus:     datalibModel.KnowledgeBaseAssetRefSyncStatusFailed,
				SyncRunID:      &oldSyncRunID,
			},
		},
		chunkCount:     1,
		embeddingCount: 1,
	}
	svc := newKnowledgeBaseFileRefTestService(deps)

	result, err := svc.RetryRef(context.Background(), KnowledgeBaseFileRefRetryRequest{
		OrganizationID: "org-1",
		DatasetID:      "dataset-1",
		RefID:          refID,
	})
	if err != nil {
		t.Fatalf("RetryRef: %v", err)
	}
	if result == nil ||
		!result.Success ||
		result.SyncRunID == nil ||
		*result.SyncRunID == uuid.Nil ||
		*result.SyncRunID == oldSyncRunID ||
		result.GenerationNo != 7 ||
		deps.pendingSyncRunID == uuid.Nil {
		t.Fatalf("result=%+v pending=%s", result, deps.pendingSyncRunID)
	}
}

func TestKnowledgeBaseFileRefServiceMarksRefSyncFailed(t *testing.T) {
	assetID := uuid.New()
	refID := uuid.New()
	syncRunID := uuid.New()
	deps := &fakeKnowledgeBaseFileRefDeps{
		dataset: &datasetModel.Dataset{
			ID:             "dataset-1",
			OrganizationID: "org-1",
		},
		refs: []*datalibModel.KnowledgeBaseAssetRef{
			{
				ID:             refID,
				OrganizationID: "org-1",
				DatasetID:      "dataset-1",
				AssetID:        assetID,
				SyncStatus:     datalibModel.KnowledgeBaseAssetRefSyncStatusPending,
				SyncRunID:      &syncRunID,
			},
		},
	}
	svc := newKnowledgeBaseFileRefTestService(deps)

	result, err := svc.MarkRefSyncFailed(context.Background(), KnowledgeBaseFileRefSyncFailureRequest{
		OrganizationID: "org-1",
		DatasetID:      "dataset-1",
		RefID:          refID,
		SyncRunID:      syncRunID,
		ErrorCode:      "enqueue_failed",
		ErrorMessage:   "queue unavailable",
	})
	if err != nil {
		t.Fatalf("MarkRefSyncFailed: %v", err)
	}
	if result == nil ||
		result.SyncStatus != datalibModel.KnowledgeBaseAssetRefSyncStatusFailed ||
		result.SyncErrorCode == nil ||
		*result.SyncErrorCode != "enqueue_failed" ||
		result.SyncErrorMessage == nil ||
		*result.SyncErrorMessage != "queue unavailable" {
		t.Fatalf("result=%+v", result)
	}
	if deps.failedSyncRunID != syncRunID {
		t.Fatalf("failed_sync_run_id=%s", deps.failedSyncRunID)
	}
}

func TestKnowledgeBaseFileRefServiceRemovesRef(t *testing.T) {
	assetID := uuid.New()
	refID := uuid.New()
	deps := &fakeKnowledgeBaseFileRefDeps{
		dataset: &datasetModel.Dataset{
			ID:             "dataset-1",
			OrganizationID: "org-1",
		},
		refs: []*datalibModel.KnowledgeBaseAssetRef{
			{
				ID:             refID,
				OrganizationID: "org-1",
				DatasetID:      "dataset-1",
				AssetID:        assetID,
				SyncStatus:     datalibModel.KnowledgeBaseAssetRefSyncStatusSynced,
			},
		},
	}
	svc := newKnowledgeBaseFileRefTestService(deps)

	removed, err := svc.RemoveRef(context.Background(), KnowledgeBaseFileRefGetRequest{
		OrganizationID: "org-1",
		DatasetID:      "dataset-1",
		RefID:          refID,
	})
	if err != nil {
		t.Fatalf("RemoveRef: %v", err)
	}
	if removed == nil || removed.ID != refID || deps.removedRefID != refID {
		t.Fatalf("removed=%+v removed_id=%s", removed, deps.removedRefID)
	}
}

type fakeKnowledgeBaseFileRefDeps struct {
	dataset          *datasetModel.Dataset
	assets           []*datalibModel.DocumentAsset
	existingRef      *datalibModel.KnowledgeBaseAssetRef
	refs             []*datalibModel.KnowledgeBaseAssetRef
	documents        []*datasetModel.Document
	files            map[string]*fileModel.UploadFile
	referenceCount   int64
	chunkCount       int64
	embeddingCount   int64
	created          *datalibModel.KnowledgeBaseAssetRef
	lastRefFilter    datalibRepo.KnowledgeBaseAssetRefListFilter
	pendingSyncRunID uuid.UUID
	failedSyncRunID  uuid.UUID
	removedRefID     uuid.UUID
}

func newKnowledgeBaseFileRefTestService(deps *fakeKnowledgeBaseFileRefDeps) KnowledgeBaseFileRefService {
	return NewKnowledgeBaseFileRefService(deps, deps, deps, fakeKnowledgeBaseFileRefStore{deps: deps}, deps, deps, deps)
}

func (f *fakeKnowledgeBaseFileRefDeps) GetAssetByID(ctx context.Context, id uuid.UUID) (*datalibModel.DocumentAsset, error) {
	for _, asset := range f.assets {
		if asset.ID == id {
			return asset, nil
		}
	}
	return nil, nil
}

func (f *fakeKnowledgeBaseFileRefDeps) ListAssets(ctx context.Context, filter datalibRepo.DocumentAssetListFilter) ([]*datalibModel.DocumentAsset, int64, error) {
	return f.assets, int64(len(f.assets)), nil
}

func (f *fakeKnowledgeBaseFileRefDeps) CountByAssetGenerationAndTypes(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64, chunkTypes []string) (int64, error) {
	return f.chunkCount, nil
}

func (f *fakeKnowledgeBaseFileRefDeps) CountReadyByAssetGeneration(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64) (int64, error) {
	return f.embeddingCount, nil
}

func (f *fakeKnowledgeBaseFileRefDeps) Create(ctx context.Context, item *datalibModel.KnowledgeBaseAssetRef) error {
	f.created = item
	return nil
}

func (f *fakeKnowledgeBaseFileRefDeps) FindActiveByAsset(ctx context.Context, organizationID string, datasetID string, assetID uuid.UUID) (*datalibModel.KnowledgeBaseAssetRef, error) {
	return f.existingRef, nil
}

func (f *fakeKnowledgeBaseFileRefDeps) List(ctx context.Context, filter datalibRepo.KnowledgeBaseAssetRefListFilter) ([]*datalibModel.KnowledgeBaseAssetRef, int64, error) {
	f.lastRefFilter = filter
	return f.refs, int64(len(f.refs)), nil
}

func (f *fakeKnowledgeBaseFileRefDeps) ListByTenantAndIDs(ctx context.Context, tenantID string, ids []string) (map[string]*fileModel.UploadFile, error) {
	return f.files, nil
}

func (f *fakeKnowledgeBaseFileRefDeps) GetByID(ctx context.Context, id string) (*datasetModel.Dataset, error) {
	return f.dataset, nil
}

func (f *fakeKnowledgeBaseFileRefDeps) GetDocumentsByIDs(ctx context.Context, ids []string) ([]*datasetModel.Document, error) {
	allowed := map[string]struct{}{}
	for _, id := range ids {
		allowed[id] = struct{}{}
	}
	result := make([]*datasetModel.Document, 0, len(f.documents))
	for _, document := range f.documents {
		if _, ok := allowed[document.ID]; ok {
			result = append(result, document)
		}
	}
	return result, nil
}

type fakeKnowledgeBaseFileRefStore struct {
	deps *fakeKnowledgeBaseFileRefDeps
}

func (f fakeKnowledgeBaseFileRefStore) Create(ctx context.Context, item *datalibModel.KnowledgeBaseAssetRef) error {
	f.deps.created = item
	return nil
}

func (f fakeKnowledgeBaseFileRefStore) GetByID(ctx context.Context, id uuid.UUID) (*datalibModel.KnowledgeBaseAssetRef, error) {
	for _, ref := range f.deps.refs {
		if ref.ID == id {
			return ref, nil
		}
	}
	if f.deps.created != nil && f.deps.created.ID == id {
		return f.deps.created, nil
	}
	return nil, nil
}

func (f fakeKnowledgeBaseFileRefStore) FindActiveByAsset(ctx context.Context, organizationID string, datasetID string, assetID uuid.UUID) (*datalibModel.KnowledgeBaseAssetRef, error) {
	return f.deps.existingRef, nil
}

func (f fakeKnowledgeBaseFileRefStore) List(ctx context.Context, filter datalibRepo.KnowledgeBaseAssetRefListFilter) ([]*datalibModel.KnowledgeBaseAssetRef, int64, error) {
	f.deps.lastRefFilter = filter
	return f.deps.refs, int64(len(f.deps.refs)), nil
}

func (f fakeKnowledgeBaseFileRefStore) CountActiveByAssetID(ctx context.Context, organizationID string, assetID uuid.UUID) (int64, error) {
	return f.deps.referenceCount, nil
}

func (f fakeKnowledgeBaseFileRefStore) MarkPending(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, errorCode, errorMessage *string) (*datalibModel.KnowledgeBaseAssetRef, error) {
	ref, err := f.GetByID(ctx, id)
	if err != nil || ref == nil {
		return ref, err
	}
	ref.SyncStatus = datalibModel.KnowledgeBaseAssetRefSyncStatusPending
	ref.SyncRunID = &syncRunID
	ref.SyncErrorCode = errorCode
	ref.SyncErrorMessage = errorMessage
	f.deps.pendingSyncRunID = syncRunID
	return ref, nil
}

func (f fakeKnowledgeBaseFileRefStore) MarkFailed(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, errorCode, errorMessage string) (*datalibModel.KnowledgeBaseAssetRef, error) {
	ref, err := f.GetByID(ctx, id)
	if err != nil || ref == nil {
		return ref, err
	}
	if ref.OrganizationID != organizationID || ref.SyncRunID == nil || *ref.SyncRunID != syncRunID {
		return nil, nil
	}
	ref.SyncStatus = datalibModel.KnowledgeBaseAssetRefSyncStatusFailed
	ref.SyncErrorCode = &errorCode
	ref.SyncErrorMessage = &errorMessage
	f.deps.failedSyncRunID = syncRunID
	return ref, nil
}

func (f fakeKnowledgeBaseFileRefStore) SoftDelete(ctx context.Context, organizationID string, id uuid.UUID) (*datalibModel.KnowledgeBaseAssetRef, error) {
	ref, err := f.GetByID(ctx, id)
	if err != nil || ref == nil || ref.OrganizationID != organizationID {
		return ref, err
	}
	f.deps.removedRefID = id
	return ref, nil
}
