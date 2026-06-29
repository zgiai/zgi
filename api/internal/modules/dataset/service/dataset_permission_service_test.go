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
	allowed    bool
	permission []workspace_model.WorkspacePermissionCode
}

func (s *datasetPermissionOrganizationService) CheckWorkspaceOrganizationAnyPermission(_ context.Context, _, _, _ string, permissions ...workspace_model.WorkspacePermissionCode) (bool, error) {
	s.permission = append([]workspace_model.WorkspacePermissionCode{}, permissions...)
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

func TestSetOrganizationServiceAllowsDatasetReadByKnowledgeBasePermission(t *testing.T) {
	svc := &datasetService{}
	dataset := &model.Dataset{
		ID:             "dataset-1",
		OrganizationID: "org-1",
		WorkspaceID:    "workspace-1",
		CreatedBy:      "owner-1",
		Permission:     string(model.DatasetPermissionOnlyMe),
	}

	canRead, err := svc.canReadDataset(context.Background(), dataset, "member-1")
	if err != nil {
		t.Fatalf("canReadDataset returned error before injection: %v", err)
	}
	if canRead {
		t.Fatal("dataset should not be readable when organization service is missing")
	}

	orgService := &datasetPermissionOrganizationService{allowed: true}
	svc.SetOrganizationService(orgService)
	canRead, err = svc.canReadDataset(context.Background(), dataset, "member-1")
	if err != nil {
		t.Fatalf("canReadDataset returned error after injection: %v", err)
	}
	if !canRead {
		t.Fatal("dataset should be readable when organization service grants knowledge-base permission")
	}
	want := knowledgeBaseReadPermissionCodes()
	if len(orgService.permission) != len(want) {
		t.Fatalf("permissions = %v, want %v", orgService.permission, want)
	}
	for i := range want {
		if orgService.permission[i] != want[i] {
			t.Fatalf("permissions = %v, want %v", orgService.permission, want)
		}
	}
}

func TestCanEditDatasetUsesDatasetUpdatePermission(t *testing.T) {
	dataset := &model.Dataset{
		ID:             "dataset-1",
		OrganizationID: "org-1",
		WorkspaceID:    "workspace-1",
		CreatedBy:      "owner-1",
		Permission:     string(model.DatasetPermissionAllTeam),
	}
	orgService := &datasetPermissionOrganizationService{allowed: true}
	svc := &datasetService{enterpriseService: orgService}

	if !svc.canEditDataset(context.Background(), dataset, "member-1") {
		t.Fatal("dataset should be editable when organization service grants knowledge-base update permission")
	}
	want := []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionKnowledgeBaseUpdate}
	if len(orgService.permission) != len(want) {
		t.Fatalf("permissions = %v, want %v", orgService.permission, want)
	}
	for i := range want {
		if orgService.permission[i] != want[i] {
			t.Fatalf("permissions = %v, want %v", orgService.permission, want)
		}
	}
	for _, permission := range orgService.permission {
		if permission == workspace_model.WorkspacePermissionKnowledgeBaseManage {
			t.Fatalf("permissions = %v should not include legacy knowledge_base.manage", orgService.permission)
		}
		if permission == workspace_model.WorkspacePermissionKnowledgeBaseDocumentUpdate ||
			permission == workspace_model.WorkspacePermissionKnowledgeBaseSegmentUpdate ||
			permission == workspace_model.WorkspacePermissionKnowledgeBaseIndexManage {
			t.Fatalf("permissions = %v should not include content-edit permissions for dataset metadata editing", orgService.permission)
		}
	}
}
