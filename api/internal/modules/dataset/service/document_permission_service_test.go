package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	dataset_model "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	dataset_repo "github.com/zgiai/zgi/api/internal/modules/dataset/repository"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func TestDocumentServiceCheckDatasetPermissionUsesWorkspaceKnowledgePermissions(t *testing.T) {
	t.Parallel()

	organizationService := &documentPermissionOrganizationService{allowed: true}
	service := &DocumentServiceImpl{
		datasetRepo: &documentPermissionDatasetRepo{dataset: &dataset_model.Dataset{
			ID:             "dataset-1",
			OrganizationID: "org-1",
			WorkspaceID:    "workspace-1",
		}},
		organizationSvc: organizationService,
	}

	err := service.CheckDatasetPermission(context.Background(), "dataset-1", "account-1")

	require.NoError(t, err)
	require.Equal(t, "org-1", organizationService.organizationID)
	require.Equal(t, "workspace-1", organizationService.workspaceID)
	require.Equal(t, "account-1", organizationService.accountID)
	require.Equal(t, knowledgeBaseReadPermissionCodes(), organizationService.permissions)
	require.NotContains(t, organizationService.permissions, workspace_model.WorkspacePermissionKnowledgeBaseView)
	require.NotContains(t, organizationService.permissions, workspace_model.WorkspacePermissionKnowledgeBaseManage)
	require.Contains(t, organizationService.permissions, workspace_model.WorkspacePermissionKnowledgeBaseDocumentView)
}

func TestDocumentServiceCheckEditPermissionUsesWorkspaceDocumentMutationPermissions(t *testing.T) {
	t.Parallel()

	organizationService := &documentPermissionOrganizationService{allowed: true}
	service := &DocumentServiceImpl{
		datasetRepo: &documentPermissionDatasetRepo{dataset: &dataset_model.Dataset{
			ID:             "dataset-1",
			OrganizationID: "org-1",
			WorkspaceID:    "workspace-1",
		}},
		organizationSvc: organizationService,
	}

	err := service.CheckEditPermission(context.Background(), "dataset-1", "account-1")

	require.NoError(t, err)
	require.Equal(t, knowledgeBaseEditPermissionCodes(), organizationService.permissions)
	require.NotContains(t, organizationService.permissions, workspace_model.WorkspacePermissionKnowledgeBaseManage)
	require.Contains(t, organizationService.permissions, workspace_model.WorkspacePermissionKnowledgeBaseDocumentCreate)
	require.Contains(t, organizationService.permissions, workspace_model.WorkspacePermissionKnowledgeBaseDocumentUpdate)
	require.Contains(t, organizationService.permissions, workspace_model.WorkspacePermissionKnowledgeBaseIndexManage)
}

func TestDocumentServiceCheckEditPermissionFailsClosedWithoutOrganizationService(t *testing.T) {
	t.Parallel()

	service := &DocumentServiceImpl{
		datasetRepo: &documentPermissionDatasetRepo{dataset: &dataset_model.Dataset{
			ID:             "dataset-1",
			OrganizationID: "org-1",
			WorkspaceID:    "workspace-1",
		}},
	}

	err := service.CheckEditPermission(context.Background(), "dataset-1", "account-1")

	require.ErrorContains(t, err, "workspace permission service unavailable")
}

type documentPermissionDatasetRepo struct {
	dataset_repo.DatasetRepository

	dataset *dataset_model.Dataset
}

func (r *documentPermissionDatasetRepo) GetByID(context.Context, string) (*dataset_model.Dataset, error) {
	return r.dataset, nil
}

type documentPermissionOrganizationService struct {
	interfaces.OrganizationService

	allowed        bool
	organizationID string
	workspaceID    string
	accountID      string
	permissions    []workspace_model.WorkspacePermissionCode
}

func (s *documentPermissionOrganizationService) CheckWorkspaceOrganizationAnyPermission(
	_ context.Context,
	organizationID, workspaceID, accountID string,
	permissions ...workspace_model.WorkspacePermissionCode,
) (bool, error) {
	s.organizationID = organizationID
	s.workspaceID = workspaceID
	s.accountID = accountID
	s.permissions = permissions
	return s.allowed, nil
}
