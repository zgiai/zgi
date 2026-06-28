package worker

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestNewDatasetRefSyncTaskBuildsPayload(t *testing.T) {
	refID := uuid.New()
	assetID := uuid.New()
	syncRunID := uuid.New()

	task, err := NewDatasetRefSyncTask(refID, assetID, "dataset-1", 12, syncRunID, nil)
	if err != nil {
		t.Fatalf("NewDatasetRefSyncTask: %v", err)
	}
	if task.Type() != TypeDataLibraryDatasetRefSync {
		t.Fatalf("task type=%q", task.Type())
	}

	var payload DatasetRefSyncPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.RefID != refID.String() ||
		payload.AssetID != assetID.String() ||
		payload.DatasetID != "dataset-1" ||
		payload.GenerationNo != 12 ||
		payload.SyncRunID != syncRunID.String() {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestNewDatasetRefSyncTaskRequiresFields(t *testing.T) {
	refID := uuid.New()
	assetID := uuid.New()
	syncRunID := uuid.New()

	tests := []struct {
		name         string
		refID        uuid.UUID
		assetID      uuid.UUID
		datasetID    string
		generationNo int64
		syncRunID    uuid.UUID
	}{
		{name: "ref", assetID: assetID, datasetID: "dataset-1", generationNo: 1, syncRunID: syncRunID},
		{name: "asset", refID: refID, datasetID: "dataset-1", generationNo: 1, syncRunID: syncRunID},
		{name: "dataset", refID: refID, assetID: assetID, generationNo: 1, syncRunID: syncRunID},
		{name: "generation", refID: refID, assetID: assetID, datasetID: "dataset-1", syncRunID: syncRunID},
		{name: "sync_run", refID: refID, assetID: assetID, datasetID: "dataset-1", generationNo: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewDatasetRefSyncTask(tt.refID, tt.assetID, tt.datasetID, tt.generationNo, tt.syncRunID, nil); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
