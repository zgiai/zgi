package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/contracts"
	contentparsemodel "github.com/zgiai/zgi/api/internal/modules/contentparse/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
)

func TestParseArtifactConfirmationServiceAppliesEditedContentToSameArtifact(t *testing.T) {
	assetID := uuid.New()
	runID := uuid.New()
	artifactID := uuid.New()
	assetRepo := &fileAssetStateAssetRepo{
		asset: &model.DocumentAsset{
			ID:              assetID,
			OrganizationID:  "org-1",
			SourceFileID:    "file-1",
			ProductStatus:   model.DocumentAssetProductStatusConfirming,
			ProcessingRunID: &runID,
			GenerationNo:    3,
			ParseArtifactID: &artifactID,
		},
	}
	artifactRepo := &parseArtifactPersistenceArtifactRepo{
		created: &contentparsemodel.Artifact{
			ID:                 artifactID,
			ArtifactStorageKey: "original.json",
			SummaryJSON:        map[string]any{"asset_id": assetID.String()},
		},
	}
	store := &parseArtifactMemoryStorage{files: map[string][]byte{}}
	artifact := contracts.ParseArtifact{
		ArtifactID: artifactID.String(),
		Elements: []contracts.ParsedElement{
			{ID: "el-1", Type: "text", Content: "teh"},
			{ID: "el-2", Type: "text", Content: "unchanged"},
		},
	}
	payload, err := json.Marshal(artifact)
	if err != nil {
		t.Fatalf("marshal artifact: %v", err)
	}
	if err := store.Save("original.json", payload); err != nil {
		t.Fatalf("save artifact: %v", err)
	}
	finalContent := "the"
	itemRepo := newParseConfirmationServiceItemRepo([]*model.ParseConfirmationItem{
		{
			ID:              uuid.New(),
			OrganizationID:  "org-1",
			AssetID:         assetID,
			ProcessingRunID: runID,
			GenerationNo:    3,
			Status:          model.ParseConfirmationItemStatusEdited,
			OriginalContent: "teh",
			FinalContent:    &finalContent,
			SourceLocatorJSON: map[string]any{
				"artifact_element_id": "el-1",
			},
		},
		{
			ID:              uuid.New(),
			OrganizationID:  "org-1",
			AssetID:         assetID,
			ProcessingRunID: runID,
			GenerationNo:    3,
			Status:          model.ParseConfirmationItemStatusKept,
			OriginalContent: "unchanged",
			SourceLocatorJSON: map[string]any{
				"artifact_element_id": "el-2",
			},
		},
	})
	persistence := NewParseArtifactPersistenceService(assetRepo, artifactRepo, store)
	svc := NewParseArtifactConfirmationService(assetRepo, artifactRepo, persistence, itemRepo)

	result, err := svc.ApplyResolvedConfirmations(context.Background(), ApplyResolvedConfirmationsInput{
		OrganizationID: "org-1",
		SourceFileID:   "file-1",
		UpdatedBy:      "user-1",
	})
	if err != nil {
		t.Fatalf("ApplyResolvedConfirmations: %v", err)
	}
	if result.ParseArtifactID != artifactID || result.AppliedItemCount != 2 || result.EditedItemCount != 1 {
		t.Fatalf("result=%+v", result)
	}
	if artifactRepo.created.ArtifactStorageKey == "original.json" {
		t.Fatalf("artifact storage key was not updated")
	}
	loaded, err := persistence.LoadParseArtifact(context.Background(), artifactRepo.created.ArtifactStorageKey)
	if err != nil {
		t.Fatalf("load confirmed artifact: %v", err)
	}
	if loaded.ArtifactID != artifactID.String() || loaded.Elements[0].Content != "the" || loaded.Elements[1].Content != "unchanged" {
		t.Fatalf("loaded=%+v", loaded)
	}
	if artifactRepo.created.SummaryJSON["confirmed"] != true {
		t.Fatalf("summary=%+v", artifactRepo.created.SummaryJSON)
	}
}
