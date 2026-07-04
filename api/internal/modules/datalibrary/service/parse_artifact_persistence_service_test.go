package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/contracts"
	contentparsemodel "github.com/zgiai/zgi/api/internal/modules/contentparse/model"
	"github.com/zgiai/zgi/api/internal/modules/contentparse/repository"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/pkg/storage"
)

func TestParseArtifactPersistenceServicePersistsPrivateArtifact(t *testing.T) {
	assetID := uuid.New()
	runID := uuid.New()
	assetRepo := &fileAssetStateAssetRepo{
		asset: &model.DocumentAsset{
			ID:              assetID,
			OrganizationID:  "org-1",
			SourceFileID:    "file-1",
			ProductStatus:   model.DocumentAssetProductStatusParsing,
			ProcessingRunID: &runID,
			GenerationNo:    3,
		},
	}
	artifactRepo := &parseArtifactPersistenceArtifactRepo{}
	store := &parseArtifactMemoryStorage{files: map[string][]byte{}}
	svc := NewParseArtifactPersistenceService(assetRepo, artifactRepo, store)

	result, err := svc.PersistAssetParseArtifact(context.Background(), PersistAssetParseArtifactInput{
		OrganizationID:    "org-1",
		AssetID:           assetID,
		ProcessingRunID:   runID,
		GenerationNo:      3,
		SourceFileID:      "file-1",
		SourceContentHash: "hash-1",
		ParseRequest: contracts.ParseRequest{
			Profile: contracts.ParseProfileDefault,
			Data:    []byte("source"),
		},
		Artifact: &contracts.ParseArtifact{
			SourceType:   contracts.ParseSourceTypeUploadFile,
			SourceRef:    "file-1",
			Profile:      contracts.ParseProfileDefault,
			Status:       contracts.ParseStatusSucceeded,
			QualityLevel: contracts.ParseQualityHigh,
			EngineUsed:   contracts.ParseEngineLocal,
			Text:         "hello",
		},
	})
	if err != nil {
		t.Fatalf("PersistAssetParseArtifact: %v", err)
	}
	if result.Artifact == nil || result.Artifact.ID == uuid.Nil {
		t.Fatalf("artifact not persisted: %+v", result.Artifact)
	}
	if artifactRepo.created == nil || artifactRepo.created.ID != result.Artifact.ID {
		t.Fatalf("created artifact mismatch: %+v", artifactRepo.created)
	}
	if result.Artifact.SourceContentHash != "hash-1" {
		t.Fatalf("source hash=%q", result.Artifact.SourceContentHash)
	}
	if result.Artifact.ProviderSignature == "local:local" || result.Artifact.ProviderSignature[:6] != "asset:" {
		t.Fatalf("provider signature should be asset-scoped, got %q", result.Artifact.ProviderSignature)
	}
	if result.Artifact.ArtifactStorageKey == "" || len(store.files[result.Artifact.ArtifactStorageKey]) == 0 {
		t.Fatalf("artifact storage key not saved: %q", result.Artifact.ArtifactStorageKey)
	}
	if result.Asset == nil || result.Asset.ParseArtifactID == nil || *result.Asset.ParseArtifactID != result.Artifact.ID {
		t.Fatalf("asset parse artifact not updated: %+v", result.Asset)
	}

	loaded, err := svc.LoadParseArtifact(context.Background(), result.Artifact.ArtifactStorageKey)
	if err != nil {
		t.Fatalf("LoadParseArtifact: %v", err)
	}
	if loaded.ArtifactID != result.Artifact.ID.String() || loaded.Text != "hello" {
		t.Fatalf("loaded artifact=%+v", loaded)
	}
}

func TestParseArtifactPersistenceServiceRejectsStaleRunBeforeStorageWrite(t *testing.T) {
	assetID := uuid.New()
	currentRunID := uuid.New()
	inputRunID := uuid.New()
	assetRepo := &fileAssetStateAssetRepo{
		asset: &model.DocumentAsset{
			ID:              assetID,
			OrganizationID:  "org-1",
			SourceFileID:    "file-1",
			ProductStatus:   model.DocumentAssetProductStatusParsing,
			ProcessingRunID: &currentRunID,
			GenerationNo:    3,
		},
	}
	artifactRepo := &parseArtifactPersistenceArtifactRepo{}
	store := &parseArtifactMemoryStorage{files: map[string][]byte{}}
	svc := NewParseArtifactPersistenceService(assetRepo, artifactRepo, store)

	_, err := svc.PersistAssetParseArtifact(context.Background(), PersistAssetParseArtifactInput{
		OrganizationID:  "org-1",
		AssetID:         assetID,
		ProcessingRunID: inputRunID,
		GenerationNo:    3,
		Artifact:        &contracts.ParseArtifact{Text: "stale"},
	})
	if !errors.Is(err, ErrProcessingRunMismatch) {
		t.Fatalf("err=%v", err)
	}
	if len(store.files) != 0 {
		t.Fatalf("stale run wrote storage: %+v", store.files)
	}
	if artifactRepo.created != nil {
		t.Fatalf("stale run created artifact: %+v", artifactRepo.created)
	}
}

type parseArtifactPersistenceArtifactRepo struct {
	created *contentparsemodel.Artifact
}

func (r *parseArtifactPersistenceArtifactRepo) Create(ctx context.Context, item *contentparsemodel.Artifact) error {
	if err := item.BeforeCreate(nil); err != nil {
		return err
	}
	r.created = item
	return nil
}

func (r *parseArtifactPersistenceArtifactRepo) GetByID(ctx context.Context, id uuid.UUID) (*contentparsemodel.Artifact, error) {
	if r.created == nil || r.created.ID != id {
		return nil, nil
	}
	return r.created, nil
}

func (r *parseArtifactPersistenceArtifactRepo) GetBySignature(ctx context.Context, sourceContentHash, profile, canonicalIRVersion, providerSignature string) (*contentparsemodel.Artifact, error) {
	return nil, nil
}

func (r *parseArtifactPersistenceArtifactRepo) UpdateStorageKeyAndSummary(ctx context.Context, id uuid.UUID, storageKey string, summary map[string]any) error {
	if r.created == nil || r.created.ID != id {
		return nil
	}
	r.created.ArtifactStorageKey = storageKey
	r.created.SummaryJSON = summary
	return nil
}

func (r *parseArtifactPersistenceArtifactRepo) Upsert(ctx context.Context, item *contentparsemodel.Artifact) error {
	r.created = item
	return nil
}

var _ repository.ArtifactRepository = (*parseArtifactPersistenceArtifactRepo)(nil)

type parseArtifactMemoryStorage struct {
	files map[string][]byte
}

func (s *parseArtifactMemoryStorage) Save(filename string, data []byte) error {
	copied := append([]byte(nil), data...)
	s.files[filename] = copied
	return nil
}

func (s *parseArtifactMemoryStorage) Load(filename string) ([]byte, error) {
	data, ok := s.files[filename]
	if !ok {
		return nil, errors.New("not found")
	}
	return append([]byte(nil), data...), nil
}

func (s *parseArtifactMemoryStorage) LoadStream(filename string) (<-chan []byte, error) {
	ch := make(chan []byte, 1)
	data, err := s.Load(filename)
	if err != nil {
		return nil, err
	}
	ch <- data
	close(ch)
	return ch, nil
}

func (s *parseArtifactMemoryStorage) Download(filename string, targetPath string) error {
	return nil
}

func (s *parseArtifactMemoryStorage) Exists(filename string) (bool, error) {
	_, ok := s.files[filename]
	return ok, nil
}

func (s *parseArtifactMemoryStorage) Delete(filename string) error {
	delete(s.files, filename)
	return nil
}

func (s *parseArtifactMemoryStorage) List(prefix string) ([]storage.FileInfo, error) {
	items := make([]storage.FileInfo, 0)
	for key, data := range s.files {
		items = append(items, storage.FileInfo{Key: key, Size: int64(len(data)), LastModified: time.Now()})
	}
	return items, nil
}
