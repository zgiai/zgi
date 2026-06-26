package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/contracts"
	contentparsemodel "github.com/zgiai/zgi/api/internal/modules/contentparse/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
)

func TestParsePreviewServiceReturnsArtifactWithConfirmationOverlay(t *testing.T) {
	assetID := uuid.New()
	artifactID := uuid.New()
	runID := uuid.New()
	storeKey := "artifact.json"
	assetRepo := &fileAssetStateAssetRepo{
		asset: &model.DocumentAsset{
			ID:              assetID,
			OrganizationID:  "org-1",
			SourceFileID:    "file-1",
			ProductStatus:   model.DocumentAssetProductStatusConfirming,
			ProcessingRunID: &runID,
			GenerationNo:    2,
			ParseArtifactID: &artifactID,
		},
	}
	artifactRepo := &parseArtifactPersistenceArtifactRepo{
		created: &contentparsemodel.Artifact{
			ID:                 artifactID,
			ArtifactStorageKey: storeKey,
		},
	}
	store := &parseArtifactMemoryStorage{files: map[string][]byte{}}
	artifact := contracts.ParseArtifact{
		ArtifactID:   artifactID.String(),
		Status:       contracts.ParseStatusSucceeded,
		QualityLevel: contracts.ParseQualityDegraded,
		EngineUsed:   contracts.ParseEngineLocal,
		Text:         "hello",
		Elements: []contracts.ParsedElement{
			{ID: "el-1", Type: "text", Content: "ok", Ordinal: 1},
			{ID: "el-2", Type: "table", Content: "needs review", Ordinal: 2},
		},
	}
	data, err := json.Marshal(artifact)
	if err != nil {
		t.Fatalf("marshal artifact: %v", err)
	}
	if err := store.Save(storeKey, data); err != nil {
		t.Fatalf("save artifact: %v", err)
	}
	itemRepo := &parseArtifactQualityItemRepo{
		created: []*model.ParseConfirmationItem{
			{
				ID:              uuid.New(),
				OrganizationID:  "org-1",
				AssetID:         assetID,
				ProcessingRunID: runID,
				GenerationNo:    2,
				ItemType:        model.ParseConfirmationItemTypeTable,
				Status:          model.ParseConfirmationItemStatusPending,
				SourceLocatorJSON: map[string]any{
					"artifact_element_id": "el-2",
					"element_index":       1,
				},
				OriginalContent: "needs review",
			},
		},
	}
	svc := NewParsePreviewService(assetRepo, artifactRepo, NewParseArtifactPersistenceService(assetRepo, artifactRepo, store), itemRepo)

	view, err := svc.GetParsePreview(context.Background(), ParsePreviewInput{
		OrganizationID: "org-1",
		SourceFileID:   "file-1",
	})
	if err != nil {
		t.Fatalf("GetParsePreview: %v", err)
	}
	if view.ParseArtifactID != artifactID || view.PendingConfirmationCount != 1 || view.TotalConfirmationCount != 1 {
		t.Fatalf("view counts/artifact mismatch: %+v", view)
	}
	if len(view.Elements) != 2 {
		t.Fatalf("elements=%+v", view.Elements)
	}
	if view.Elements[0].Confirmation != nil {
		t.Fatalf("unexpected confirmation on first element: %+v", view.Elements[0].Confirmation)
	}
	if view.Elements[1].Confirmation == nil || view.Elements[1].Confirmation.ArtifactElementID != "el-2" {
		t.Fatalf("missing confirmation overlay: %+v", view.Elements[1].Confirmation)
	}
}

func TestParsePreviewServiceRequiresParseArtifact(t *testing.T) {
	assetRepo := &fileAssetStateAssetRepo{
		asset: &model.DocumentAsset{
			ID:             uuid.New(),
			OrganizationID: "org-1",
			SourceFileID:   "file-1",
			ProductStatus:  model.DocumentAssetProductStatusParsing,
		},
	}
	svc := NewParsePreviewService(assetRepo, &parseArtifactPersistenceArtifactRepo{}, NewParseArtifactPersistenceService(assetRepo, &parseArtifactPersistenceArtifactRepo{}, &parseArtifactMemoryStorage{files: map[string][]byte{}}), &parseArtifactQualityItemRepo{})

	_, err := svc.GetParsePreview(context.Background(), ParsePreviewInput{
		OrganizationID: "org-1",
		SourceFileID:   "file-1",
	})
	if !errors.Is(err, ErrParsePreviewNotReady) {
		t.Fatalf("err=%v", err)
	}
}
