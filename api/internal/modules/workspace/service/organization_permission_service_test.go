package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
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

func TestOrganizationServiceCheckWorkspacePermissionUsesMemberDirectPermissions(t *testing.T) {
	t.Parallel()

	svc := &organizationService{
		organizationRepo: &permissionCheckOrganizationRepo{
			organization: &model.Organization{ID: "org-1"},
		},
		workspaceRepo: &permissionCheckWorkspaceRepo{organizationID: "org-1"},
		workspaceManagementService: &permissionCheckWorkspaceManagementService{
			join: &model.WorkspaceMember{
				WorkspaceID:      "workspace-1",
				AccountID:        "account-1",
				Role:             model.WorkspaceRoleNormal,
				Permissions:      []string{string(model.WorkspacePermissionAgentManage)},
				PermissionSource: model.WorkspaceMemberPermissionSourceDirect,
			},
		},
	}

	allowed, err := svc.CheckWorkspacePermission(
		context.Background(),
		"org-1",
		"workspace-1",
		"account-1",
		model.WorkspacePermissionAgentCreate,
	)

	require.NoError(t, err)
	require.True(t, allowed)
}

func TestOrganizationServiceCheckWorkspacePermissionRejectsGovernanceFromDirectSnapshot(t *testing.T) {
	t.Parallel()

	svc := &organizationService{
		organizationRepo: &permissionCheckOrganizationRepo{
			organization: &model.Organization{ID: "org-1"},
		},
		workspaceRepo: &permissionCheckWorkspaceRepo{organizationID: "org-1"},
		workspaceManagementService: &permissionCheckWorkspaceManagementService{
			join: &model.WorkspaceMember{
				WorkspaceID:      "workspace-1",
				AccountID:        "account-1",
				Role:             model.WorkspaceRoleNormal,
				Permissions:      []string{string(model.WorkspacePermissionWorkspacePermissionManage)},
				PermissionSource: model.WorkspaceMemberPermissionSourceDirect,
			},
		},
	}

	allowed, err := svc.CheckWorkspacePermission(
		context.Background(),
		"org-1",
		"workspace-1",
		"account-1",
		model.WorkspacePermissionWorkspacePermissionManage,
	)

	require.NoError(t, err)
	require.False(t, allowed)
}

func TestOrganizationServiceCheckWorkspacePermissionAllowsAdminGovernanceWithEmptySnapshot(t *testing.T) {
	t.Parallel()

	svc := &organizationService{
		organizationRepo: &permissionCheckOrganizationRepo{
			organization: &model.Organization{ID: "org-1"},
		},
		workspaceRepo: &permissionCheckWorkspaceRepo{organizationID: "org-1"},
		workspaceManagementService: &permissionCheckWorkspaceManagementService{
			join: &model.WorkspaceMember{
				WorkspaceID:      "workspace-1",
				AccountID:        "account-1",
				Role:             model.WorkspaceRoleAdmin,
				Permissions:      []string{},
				PermissionSource: model.WorkspaceMemberPermissionSourceRoleTemplate,
			},
		},
	}

	allowed, err := svc.CheckWorkspacePermission(
		context.Background(),
		"org-1",
		"workspace-1",
		"account-1",
		model.WorkspacePermissionWorkspaceManage,
	)

	require.NoError(t, err)
	require.True(t, allowed)
}

func TestOrganizationServiceCheckWorkspacePermissionRequiresWorkspaceMembership(t *testing.T) {
	t.Parallel()

	svc := &organizationService{
		organizationRepo: &permissionCheckOrganizationRepo{
			organization: &model.Organization{ID: "org-1"},
		},
		workspaceRepo:              &permissionCheckWorkspaceRepo{organizationID: "org-1"},
		workspaceManagementService: &permissionCheckWorkspaceManagementService{},
	}

	allowed, err := svc.CheckWorkspacePermission(
		context.Background(),
		"org-1",
		"workspace-1",
		"account-1",
		model.WorkspacePermissionWorkspaceManage,
	)

	require.NoError(t, err)
	require.False(t, allowed)
}

func TestOrganizationServiceCheckWorkspacePermissionAllowsAdminAssetAccessWithEmptySnapshot(t *testing.T) {
	t.Parallel()

	svc := &organizationService{
		organizationRepo: &permissionCheckOrganizationRepo{
			organization: &model.Organization{ID: "org-1"},
		},
		workspaceRepo: &permissionCheckWorkspaceRepo{organizationID: "org-1"},
		workspaceManagementService: &permissionCheckWorkspaceManagementService{
			join: &model.WorkspaceMember{
				WorkspaceID:      "workspace-1",
				AccountID:        "account-1",
				Role:             model.WorkspaceRoleAdmin,
				Permissions:      []string{},
				PermissionSource: model.WorkspaceMemberPermissionSourceDirect,
			},
		},
	}

	allowed, err := svc.CheckWorkspacePermission(
		context.Background(),
		"org-1",
		"workspace-1",
		"account-1",
		model.WorkspacePermissionAgentCreate,
	)

	require.NoError(t, err)
	require.True(t, allowed)
}

func TestOrganizationServiceCheckWorkspacePermissionWorkspaceOwnerHasAllPermissions(t *testing.T) {
	t.Parallel()

	svc := &organizationService{
		organizationRepo: &permissionCheckOrganizationRepo{
			organization: &model.Organization{ID: "org-1"},
		},
		workspaceRepo: &permissionCheckWorkspaceRepo{organizationID: "org-1"},
		workspaceManagementService: &permissionCheckWorkspaceManagementService{
			join: &model.WorkspaceMember{
				WorkspaceID:      "workspace-1",
				AccountID:        "account-1",
				Role:             model.WorkspaceRoleOwner,
				Permissions:      []string{},
				PermissionSource: model.WorkspaceMemberPermissionSourceOwner,
			},
		},
	}

	allowed, err := svc.CheckWorkspacePermission(
		context.Background(),
		"org-1",
		"workspace-1",
		"account-1",
		model.WorkspacePermissionAgentCreate,
	)

	require.NoError(t, err)
	require.True(t, allowed)
}

func TestOrganizationServiceCheckWorkspacePermissionOrganizationAdminBypassesWorkspaceMembership(t *testing.T) {
	t.Parallel()

	svc := &organizationService{
		organizationRepo: &permissionCheckOrganizationRepo{
			organization:     &model.Organization{ID: "org-1"},
			organizationRole: model.OrganizationRoleAdmin,
		},
		workspaceRepo: &permissionCheckWorkspaceRepo{organizationID: "org-1"},
		workspaceManagementService: &permissionCheckWorkspaceManagementService{
			join: nil,
		},
	}

	allowed, err := svc.CheckWorkspacePermission(
		context.Background(),
		"org-1",
		"workspace-1",
		"account-1",
		model.WorkspacePermissionDatabaseDelete,
	)

	require.NoError(t, err)
	require.True(t, allowed)
}

func TestWorkspaceMemberAllowsPermissionRoleMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		role             model.WorkspaceMemberRole
		roleID           *string
		permissionSource model.WorkspaceMemberPermissionSource
		permissions      []string
		allowed          []model.WorkspacePermissionCode
		denied           []model.WorkspacePermissionCode
	}{
		{
			name:             "viewer can inspect assets but not mutate",
			role:             model.WorkspaceRoleViewer,
			roleID:           stringPtr(model.WorkspaceBuiltinRoleViewerID),
			permissionSource: model.WorkspaceMemberPermissionSourceRoleTemplate,
			permissions: model.DefaultWorkspaceMemberPermissionStrings(
				model.WorkspaceRoleViewer,
				stringPtr(model.WorkspaceBuiltinRoleViewerID),
			),
			allowed: []model.WorkspacePermissionCode{
				model.WorkspacePermissionWorkspaceView,
				model.WorkspacePermissionWorkspaceMemberView,
				model.WorkspacePermissionAgentLogsView,
				model.WorkspacePermissionWorkflowView,
				model.WorkspacePermissionKnowledgeBaseDocumentView,
				model.WorkspacePermissionDatabaseSchemaView,
				model.WorkspacePermissionFileMetadataView,
			},
			denied: []model.WorkspacePermissionCode{
				model.WorkspacePermissionAgentCreate,
				model.WorkspacePermissionWorkflowPublish,
				model.WorkspacePermissionKnowledgeBaseDocumentCreate,
				model.WorkspacePermissionDatabaseRecordCreate,
				model.WorkspacePermissionFileUpload,
				model.WorkspacePermissionCode("dashboard.view"),
			},
		},
		{
			name:             "member has runtime helper permissions but not builders",
			role:             model.WorkspaceRoleMember,
			roleID:           stringPtr(model.WorkspaceBuiltinRoleMemberID),
			permissionSource: model.WorkspaceMemberPermissionSourceRoleTemplate,
			permissions: model.DefaultWorkspaceMemberPermissionStrings(
				model.WorkspaceRoleMember,
				stringPtr(model.WorkspaceBuiltinRoleMemberID),
			),
			allowed: []model.WorkspacePermissionCode{
				model.WorkspacePermissionWorkspaceView,
				model.WorkspacePermissionWorkspaceMemberView,
				model.WorkspacePermissionKnowledgeBaseRetrievalTest,
				model.WorkspacePermissionDatabaseAIQueryRead,
				model.WorkspacePermissionFileUpload,
				model.WorkspacePermissionFileDownload,
			},
			denied: []model.WorkspacePermissionCode{
				model.WorkspacePermissionAgentCreate,
				model.WorkspacePermissionWorkflowCreate,
				model.WorkspacePermissionKnowledgeBaseCreate,
				model.WorkspacePermissionDatabaseCreate,
				model.WorkspacePermissionFileDelete,
				model.WorkspacePermissionCode("dashboard.view"),
			},
		},
		{
			name:             "admin has builder asset permissions from its member snapshot",
			role:             model.WorkspaceRoleAdmin,
			roleID:           stringPtr(model.WorkspaceBuiltinRoleAdminID),
			permissionSource: model.WorkspaceMemberPermissionSourceRoleTemplate,
			permissions: model.DefaultWorkspaceMemberPermissionStrings(
				model.WorkspaceRoleAdmin,
				stringPtr(model.WorkspaceBuiltinRoleAdminID),
			),
			allowed: []model.WorkspacePermissionCode{
				model.WorkspacePermissionWorkspaceView,
				model.WorkspacePermissionWorkspaceMemberView,
				model.WorkspacePermissionWorkspaceTransfer,
				model.WorkspacePermissionWorkspaceArchive,
				model.WorkspacePermissionAgentCreate,
				model.WorkspacePermissionWorkflowPublish,
				model.WorkspacePermissionKnowledgeBaseDocumentCreate,
				model.WorkspacePermissionDatabaseRecordCreate,
				model.WorkspacePermissionFileDelete,
			},
			denied: []model.WorkspacePermissionCode{
				model.WorkspacePermissionCode("dashboard.stats.view"),
			},
		},
		{
			name:             "owner has all permissions even with an empty snapshot",
			role:             model.WorkspaceRoleOwner,
			roleID:           stringPtr(model.WorkspaceBuiltinRoleOwnerID),
			permissionSource: model.WorkspaceMemberPermissionSourceOwner,
			permissions:      nil,
			allowed: []model.WorkspacePermissionCode{
				model.WorkspacePermissionAgentCreate,
				model.WorkspacePermissionWorkflowPublish,
				model.WorkspacePermissionKnowledgeBaseDocumentCreate,
				model.WorkspacePermissionDatabaseRecordCreate,
				model.WorkspacePermissionFileDelete,
				model.WorkspacePermissionWorkspaceTransfer,
			},
		},
		{
			name:             "custom template uses only its saved snapshot",
			role:             model.WorkspaceRoleMember,
			roleID:           stringPtr("10000000-0000-0000-0000-000000000001"),
			permissionSource: model.WorkspaceMemberPermissionSourceRoleTemplate,
			permissions: []string{
				string(model.WorkspacePermissionDatabaseSchemaManage),
				string(model.WorkspacePermissionFileMetadataView),
			},
			allowed: []model.WorkspacePermissionCode{
				model.WorkspacePermissionWorkspaceView,
				model.WorkspacePermissionWorkspaceMemberView,
				model.WorkspacePermissionDatabaseSchemaManage,
				model.WorkspacePermissionFileMetadataView,
			},
			denied: []model.WorkspacePermissionCode{
				model.WorkspacePermissionDatabaseManage,
				model.WorkspacePermissionFileManage,
				model.WorkspacePermissionAgentCreate,
				model.WorkspacePermissionCode("dashboard.view"),
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			for _, permission := range tt.allowed {
				require.Truef(t,
					workspaceMemberAllowsPermission(tt.role, tt.roleID, tt.permissions, tt.permissionSource, permission),
					"expected %s to be allowed",
					permission,
				)
			}
			for _, permission := range tt.denied {
				require.Falsef(t,
					workspaceMemberAllowsPermission(tt.role, tt.roleID, tt.permissions, tt.permissionSource, permission),
					"expected %s to be denied",
					permission,
				)
			}
		})
	}
}

type permissionCheckOrganizationRepo struct {
	workspace_repo.OrganizationRepository

	organization     *model.Organization
	organizationRole model.OrganizationRole
	err              error
	getByIDCalled    bool
}

func (r *permissionCheckOrganizationRepo) GetByID(context.Context, string) (*model.Organization, error) {
	r.getByIDCalled = true
	if r.organization != nil {
		return r.organization, nil
	}
	if r.err != nil {
		return nil, r.err
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *permissionCheckOrganizationRepo) GetAccountJoin(context.Context, string, string) (*model.OrganizationMember, error) {
	if r.organizationRole != "" {
		return &model.OrganizationMember{Role: r.organizationRole}, nil
	}
	return nil, gorm.ErrRecordNotFound
}

type permissionCheckWorkspaceRepo struct {
	workspace_repo.WorkspaceRepository

	organizationID                   string
	getWorkspaceOrganizationIDCalled bool
}

func (r *permissionCheckWorkspaceRepo) GetWorkspaceOrganizationID(context.Context, string) (string, error) {
	r.getWorkspaceOrganizationIDCalled = true
	if r.organizationID != "" {
		return r.organizationID, nil
	}
	return "org-1", nil
}

type permissionCheckWorkspaceManagementService struct {
	interfaces.WorkspaceManagementService

	join *model.WorkspaceMember
}

func (s *permissionCheckWorkspaceManagementService) GetByWorkspaceAndMember(context.Context, string, string) (*model.WorkspaceMember, error) {
	return s.join, nil
}
