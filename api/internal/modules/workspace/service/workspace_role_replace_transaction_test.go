package service

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	shared_dto "github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func TestReplaceAndDeleteWorkspaceRoleRollsBackWhenDeleteInvariantFails(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationPermissionRegressionMockDB(t)
	workspaceSvc := newReplaceWorkspaceRoleTestService()
	svc := newOrganizationPermissionRegressionService(db)
	svc.workspaceManagementService = workspaceSvc

	expectWorkspaceRoleReplacementLocksAndTargets(mock)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "workspace_members" WHERE workspace_id IN (SELECT id FROM "workspaces" WHERE organization_id = $1) AND (role_id = $2 OR permission_template_role_id = $3)`)).
		WithArgs("org-1", "role-source", "role-source").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectRollback()

	response, err := svc.ReplaceAndDeleteCustomWorkspaceRole(context.Background(), &shared_dto.ReplaceWorkspaceRoleTemplateRequest{
		OrganizationID:    "org-1",
		RoleID:            "role-source",
		ReplacementRoleID: "role-replacement",
		OperatorID:        "owner-1",
	})

	require.Nil(t, response)
	require.ErrorIs(t, err, ErrWorkspaceRoleInUse)
	require.Equal(t, 1, workspaceSvc.withTxCalls)
	require.Len(t, workspaceSvc.customUpdates, 2)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReplaceAndDeleteWorkspaceRoleRollsBackAllTargetsWhenOneFails(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationPermissionRegressionMockDB(t)
	workspaceSvc := newReplaceWorkspaceRoleTestService()
	workspaceSvc.failByTargetKey = map[string]error{
		"ws-2:member-2": errors.New("no permission"),
	}
	svc := newOrganizationPermissionRegressionService(db)
	svc.workspaceManagementService = workspaceSvc

	expectWorkspaceRoleReplacementLocksAndTargets(mock)
	mock.ExpectRollback()

	response, err := svc.ReplaceAndDeleteCustomWorkspaceRole(context.Background(), &shared_dto.ReplaceWorkspaceRoleTemplateRequest{
		OrganizationID:    "org-1",
		RoleID:            "role-source",
		ReplacementRoleID: "role-replacement",
		OperatorID:        "owner-1",
	})

	require.NoError(t, err)
	require.NotNil(t, response)
	require.False(t, response.Deleted)
	require.Zero(t, response.ReplacedCount)
	require.Equal(t, 2, response.FailedCount)
	require.Len(t, response.Results, 2)
	require.Equal(t, "failed", response.Results[0].Status)
	require.Equal(t, "replacement rolled back", response.Results[0].Message)
	require.Equal(t, "failed", response.Results[1].Status)
	require.Contains(t, response.Results[1].Message, "no permission")
	require.Equal(t, 1, workspaceSvc.withTxCalls)
	require.Len(t, workspaceSvc.customUpdates, 1)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestReplaceAndDeleteWorkspaceRoleCommitsReplacementAndDeleteTogether(t *testing.T) {
	t.Parallel()

	db, mock := newOrganizationPermissionRegressionMockDB(t)
	workspaceSvc := newReplaceWorkspaceRoleTestService()
	svc := newOrganizationPermissionRegressionService(db)
	svc.workspaceManagementService = workspaceSvc

	expectWorkspaceRoleReplacementLocksAndTargets(mock)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "workspace_members" WHERE workspace_id IN (SELECT id FROM "workspaces" WHERE organization_id = $1) AND (role_id = $2 OR permission_template_role_id = $3)`)).
		WithArgs("org-1", "role-source", "role-source").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "roles" WHERE group_id = $1 AND status = $2`)).
		WithArgs("org-1", model.WorkspaceCustomRoleStatusActive).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectExec(`UPDATE "roles" SET .*"status"=\$[0-9]+.*WHERE "id" = \$[0-9]+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	response, err := svc.ReplaceAndDeleteCustomWorkspaceRole(context.Background(), &shared_dto.ReplaceWorkspaceRoleTemplateRequest{
		OrganizationID:    "org-1",
		RoleID:            "role-source",
		ReplacementRoleID: "role-replacement",
		OperatorID:        "owner-1",
	})

	require.NoError(t, err)
	require.NotNil(t, response)
	require.True(t, response.Deleted)
	require.Equal(t, 2, response.ReplacedCount)
	require.Zero(t, response.FailedCount)
	require.Equal(t, 1, workspaceSvc.withTxCalls)
	require.Len(t, workspaceSvc.customUpdates, 2)
	require.NoError(t, mock.ExpectationsWereMet())
}

func expectWorkspaceRoleReplacementLocksAndTargets(mock sqlmock.Sqlmock) {
	now := time.Now().UTC()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "id" FROM "organizations" WHERE id = $1 LIMIT $2 FOR UPDATE`)).
		WithArgs("org-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("org-1"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "roles" WHERE id = $1 AND group_id = $2 ORDER BY "roles"."id" LIMIT $3 FOR UPDATE`)).
		WithArgs("role-source", "org-1", 1).
		WillReturnRows(workspaceRoleReplacementRows(now, "role-source", "Source"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "roles" WHERE id = $1 AND group_id = $2 ORDER BY "roles"."id" LIMIT $3 FOR SHARE`)).
		WithArgs("role-replacement", "org-1", 1).
		WillReturnRows(workspaceRoleReplacementRows(now, "role-replacement", "Replacement"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT workspace_id, account_id FROM "workspace_members" WHERE workspace_id IN (SELECT id FROM "workspaces" WHERE organization_id = $1) AND (role_id = $2 OR permission_template_role_id = $3) ORDER BY workspace_id ASC, account_id ASC FOR UPDATE`)).
		WithArgs("org-1", "role-source", "role-source").
		WillReturnRows(sqlmock.NewRows([]string{"workspace_id", "account_id"}).
			AddRow("ws-1", "member-1").
			AddRow("ws-2", "member-2"))
}

func workspaceRoleReplacementRows(now time.Time, roleID, name string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "group_id", "name", "status", "permissions", "created_by", "created_at", "updated_at",
	}).AddRow(
		roleID,
		"org-1",
		name,
		model.WorkspaceCustomRoleStatusActive,
		[]byte(`[]`),
		"owner-1",
		now,
		now,
	)
}

func newReplaceWorkspaceRoleTestService() *applyTemplateWorkspaceManagementService {
	organizationID := "org-1"
	return &applyTemplateWorkspaceManagementService{
		workspaces: map[string]*model.Workspace{
			"ws-1": {ID: "ws-1", OrganizationID: &organizationID},
			"ws-2": {ID: "ws-2", OrganizationID: &organizationID},
		},
	}
}
