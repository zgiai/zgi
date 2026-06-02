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
			"file-1": {ID: "file-1", Name: "handbook.pdf"},
		},
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
	if !item.Addable || item.Reason != "" || item.Name != "handbook.pdf" || item.AssetID != assetID || item.ChunkCount != 3 || item.EmbeddingCount != 3 {
		t.Fatalf("candidate=%+v", item)
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
	if len(result.Items) != 1 || !result.Items[0].Success || result.Items[0].SyncRunID == nil || *result.Items[0].SyncRunID == uuid.Nil {
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

type fakeKnowledgeBaseFileRefDeps struct {
	dataset        *datasetModel.Dataset
	assets         []*datalibModel.DocumentAsset
	existingRef    *datalibModel.KnowledgeBaseAssetRef
	files          map[string]*fileModel.UploadFile
	chunkCount     int64
	embeddingCount int64
	created        *datalibModel.KnowledgeBaseAssetRef
}

func newKnowledgeBaseFileRefTestService(deps *fakeKnowledgeBaseFileRefDeps) KnowledgeBaseFileRefService {
	return NewKnowledgeBaseFileRefService(deps, deps, deps, deps, deps, deps)
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

func (f *fakeKnowledgeBaseFileRefDeps) ListByTenantAndIDs(ctx context.Context, tenantID string, ids []string) (map[string]*fileModel.UploadFile, error) {
	return f.files, nil
}

func (f *fakeKnowledgeBaseFileRefDeps) GetByID(ctx context.Context, id string) (*datasetModel.Dataset, error) {
	return f.dataset, nil
}
