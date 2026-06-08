package service

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

type datasetPermissionOrganizationService struct {
	interfaces.OrganizationService
	allowed bool
}

func (s *datasetPermissionOrganizationService) CheckWorkspaceOrganizationAnyPermission(_ context.Context, _, _, _ string, _ ...workspace_model.WorkspacePermissionCode) (bool, error) {
	return s.allowed, nil
}

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

func TestSetOrganizationServiceAllowsWorkspaceVisibleDatasetRead(t *testing.T) {
	svc := &datasetService{}
	dataset := &model.Dataset{
		ID:             "dataset-1",
		OrganizationID: "org-1",
		WorkspaceID:    "workspace-1",
		CreatedBy:      "owner-1",
		Permission:     string(model.DatasetPermissionAllTeam),
	}

	canRead, err := svc.canReadDataset(context.Background(), dataset, "member-1")
	if err != nil {
		t.Fatalf("canReadDataset returned error before injection: %v", err)
	}
	if canRead {
		t.Fatal("workspace-visible dataset should not be readable when organization service is missing")
	}

	svc.SetOrganizationService(&datasetPermissionOrganizationService{allowed: true})
	canRead, err = svc.canReadDataset(context.Background(), dataset, "member-1")
	if err != nil {
		t.Fatalf("canReadDataset returned error after injection: %v", err)
	}
	if !canRead {
		t.Fatal("workspace-visible dataset should be readable when organization service grants knowledge-base permission")
	}
}
