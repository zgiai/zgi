package service

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
)

func TestCanEditDatasetDoesNotFollowWorkspaceVisiblePermission(t *testing.T) {
	svc := &datasetService{}
	dataset := &model.Dataset{
		ID:          "dataset-1",
		WorkspaceID: "workspace-1",
		CreatedBy:   "owner-1",
		Permission:  string(model.DatasetPermissionAllTeam),
	}

	if svc.canEditDataset(context.Background(), dataset, "member-1") {
		t.Fatal("workspace-visible dataset must not be editable by a regular non-creator member")
	}
	if !svc.canEditDataset(context.Background(), dataset, "owner-1") {
		t.Fatal("dataset creator should be able to edit")
	}
}
