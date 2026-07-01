package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
	"github.com/zgiai/zgi/api/internal/modules/dataset/repository"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

type folderPermissionRepo struct {
	repository.DatasetFolderRepository
	folder *model.DatasetFolder
}

func (r *folderPermissionRepo) GetFolderByID(context.Context, string) (*model.DatasetFolder, error) {
	return r.folder, nil
}

type folderPermissionOrganizationService struct {
	interfaces.OrganizationService
	allowed     bool
	permissions []workspace_model.WorkspacePermissionCode
}

func (s *folderPermissionOrganizationService) CheckWorkspaceOrganizationAnyPermission(_ context.Context, _, _, _ string, permissions ...workspace_model.WorkspacePermissionCode) (bool, error) {
	s.permissions = append([]workspace_model.WorkspacePermissionCode{}, permissions...)
	return s.allowed, nil
}

func TestDatasetFolderServiceCheckFolderPermissionUsesKnowledgeBaseViewPermissions(t *testing.T) {
	orgService := &folderPermissionOrganizationService{allowed: true}
	service := &DatasetFolderServiceImpl{
		folderRepo: &folderPermissionRepo{folder: &model.DatasetFolder{
			ID:             "folder-1",
			OrganizationID: "org-1",
			WorkspaceID:    "workspace-1",
		}},
		organizationService: orgService,
	}

	allowed, err := service.CheckFolderPermission(context.Background(), "folder-1", "account-1", "workspace-1")
	if err != nil {
		t.Fatalf("CheckFolderPermission() error = %v", err)
	}
	if !allowed {
		t.Fatal("CheckFolderPermission() = false, want true")
	}
	want := knowledgeBaseFolderReadPermissionCodes()
	if len(orgService.permissions) != len(want) {
		t.Fatalf("permissions = %v, want %v", orgService.permissions, want)
	}
	for i := range want {
		if orgService.permissions[i] != want[i] {
			t.Fatalf("permissions = %v, want %v", orgService.permissions, want)
		}
	}
	require.NotContains(t, orgService.permissions, workspace_model.WorkspacePermissionKnowledgeBaseDocumentCreate)
}

func TestDatasetFolderServiceCheckFolderEditorPermissionUsesFolderManagePermission(t *testing.T) {
	orgService := &folderPermissionOrganizationService{allowed: true}
	service := &DatasetFolderServiceImpl{
		folderRepo: &folderPermissionRepo{folder: &model.DatasetFolder{
			ID:             "folder-1",
			OrganizationID: "org-1",
			WorkspaceID:    "workspace-1",
		}},
		organizationService: orgService,
	}

	allowed, err := service.CheckFolderEditorPermission(context.Background(), "folder-1", "account-1", "workspace-1")
	if err != nil {
		t.Fatalf("CheckFolderEditorPermission() error = %v", err)
	}
	if !allowed {
		t.Fatal("CheckFolderEditorPermission() = false, want true")
	}
	want := []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionKnowledgeBaseFolderManage}
	if len(orgService.permissions) != len(want) || orgService.permissions[0] != want[0] {
		t.Fatalf("permissions = %v, want %v", orgService.permissions, want)
	}
}
