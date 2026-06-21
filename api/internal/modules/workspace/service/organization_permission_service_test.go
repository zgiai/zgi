package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	workspace_repo "github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	"gorm.io/gorm"
)

func TestOrganizationServiceCheckWorkspacePermissionFailsClosedForMissingOrganizationScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		organizationID  string
		organizationErr error
		wantRepoLookup  bool
	}{
		{
			name:           "empty organization",
			organizationID: "",
		},
		{
			name:           "blank organization",
			organizationID: "   ",
		},
		{
			name:            "missing organization",
			organizationID:  "missing-org",
			organizationErr: gorm.ErrRecordNotFound,
			wantRepoLookup:  true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			orgRepo := &permissionCheckOrganizationRepo{
				err: tt.organizationErr,
			}
			workspaceRepo := &permissionCheckWorkspaceRepo{}
			svc := &organizationService{
				organizationRepo: orgRepo,
				workspaceRepo:    workspaceRepo,
			}

			allowed, err := svc.CheckWorkspacePermission(
				context.Background(),
				tt.organizationID,
				"workspace-1",
				"account-1",
				model.WorkspacePermissionWorkspaceView,
			)

			require.NoError(t, err)
			require.False(t, allowed)
			require.Equal(t, tt.wantRepoLookup, orgRepo.getByIDCalled)
			require.False(t, workspaceRepo.getWorkspaceOrganizationIDCalled)
		})
	}
}

func TestOrganizationServiceCheckWorkspaceOrganizationAnyPermissionFailsClosedForMissingOrganizationScope(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		organizationID  string
		organizationErr error
		wantRepoLookup  bool
	}{
		{
			name:           "empty organization",
			organizationID: "",
		},
		{
			name:           "blank organization",
			organizationID: "   ",
		},
		{
			name:            "missing organization",
			organizationID:  "missing-org",
			organizationErr: gorm.ErrRecordNotFound,
			wantRepoLookup:  true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			orgRepo := &permissionCheckOrganizationRepo{
				err: tt.organizationErr,
			}
			workspaceRepo := &permissionCheckWorkspaceRepo{}
			svc := &organizationService{
				organizationRepo: orgRepo,
				workspaceRepo:    workspaceRepo,
			}

			allowed, err := svc.CheckWorkspaceOrganizationAnyPermission(
				context.Background(),
				tt.organizationID,
				"workspace-1",
				"account-1",
				model.WorkspacePermissionWorkspaceView,
				model.WorkspacePermissionWorkspaceManage,
			)

			require.NoError(t, err)
			require.False(t, allowed)
			require.Equal(t, tt.wantRepoLookup, orgRepo.getByIDCalled)
			require.False(t, workspaceRepo.getWorkspaceOrganizationIDCalled)
		})
	}
}

type permissionCheckOrganizationRepo struct {
	workspace_repo.OrganizationRepository

	err           error
	getByIDCalled bool
}

func (r *permissionCheckOrganizationRepo) GetByID(context.Context, string) (*model.Organization, error) {
	r.getByIDCalled = true
	if r.err != nil {
		return nil, r.err
	}
	return nil, gorm.ErrRecordNotFound
}

type permissionCheckWorkspaceRepo struct {
	workspace_repo.WorkspaceRepository

	getWorkspaceOrganizationIDCalled bool
}

func (r *permissionCheckWorkspaceRepo) GetWorkspaceOrganizationID(context.Context, string) (string, error) {
	r.getWorkspaceOrganizationIDCalled = true
	return "org-1", nil
}
