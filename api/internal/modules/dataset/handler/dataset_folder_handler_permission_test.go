package handler

import (
	"context"
	"errors"
	"reflect"
	"testing"

	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

type datasetFolderPermissionOrganizationService struct {
	interfaces.OrganizationService
	allowedByWorkspaceID map[string]bool
	checkedWorkspaceIDs  []string
	checkedPermissions   []workspace_model.WorkspacePermissionCode
	err                  error
}

func (s *datasetFolderPermissionOrganizationService) CheckWorkspaceOrganizationAnyPermission(_ context.Context, _, workspaceID, _ string, permissions ...workspace_model.WorkspacePermissionCode) (bool, error) {
	s.checkedWorkspaceIDs = append(s.checkedWorkspaceIDs, workspaceID)
	s.checkedPermissions = append([]workspace_model.WorkspacePermissionCode{}, permissions...)
	if s.err != nil {
		return false, s.err
	}
	return s.allowedByWorkspaceID[workspaceID], nil
}

func TestDatasetFolderHandlerFiltersKnowledgeWorkspacesByWorkspacePermissions(t *testing.T) {
	orgService := &datasetFolderPermissionOrganizationService{
		allowedByWorkspaceID: map[string]bool{
			"workspace-allowed": true,
			"workspace-denied":  false,
		},
	}
	handler := &DatasetFolderHandler{organizationService: orgService}

	got, err := handler.filterKnowledgeWorkspaceIDsByPermission(
		context.Background(),
		"org-1",
		"account-1",
		[]string{"workspace-denied", "workspace-allowed", "workspace-allowed", " "},
	)
	if err != nil {
		t.Fatalf("filterKnowledgeWorkspaceIDsByPermission() error = %v", err)
	}

	want := []string{"workspace-allowed"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered workspace IDs = %v, want %v", got, want)
	}

	wantChecked := []string{"workspace-denied", "workspace-allowed"}
	if !reflect.DeepEqual(orgService.checkedWorkspaceIDs, wantChecked) {
		t.Fatalf("checked workspace IDs = %v, want %v", orgService.checkedWorkspaceIDs, wantChecked)
	}

	wantPermissions := datasetFolderKnowledgePermissions()
	if !reflect.DeepEqual(orgService.checkedPermissions, wantPermissions) {
		t.Fatalf("checked permissions = %v, want %v", orgService.checkedPermissions, wantPermissions)
	}
}

func TestDatasetFolderHandlerFilterKnowledgeWorkspacesReturnsPermissionError(t *testing.T) {
	wantErr := errors.New("permission backend failed")
	handler := &DatasetFolderHandler{
		organizationService: &datasetFolderPermissionOrganizationService{err: wantErr},
	}

	_, err := handler.filterKnowledgeWorkspaceIDsByPermission(
		context.Background(),
		"org-1",
		"account-1",
		[]string{"workspace-1"},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("filterKnowledgeWorkspaceIDsByPermission() error = %v, want %v", err, wantErr)
	}
}
