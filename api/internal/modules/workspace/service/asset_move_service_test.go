package service

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/dto"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type stubAssetMoveOrgService struct {
	allowed bool
	err     error
}

func (s stubAssetMoveOrgService) IsOrganizationAdminOrOwner(ctx context.Context, organizationID, accountID string) (bool, error) {
	return s.allowed, s.err
}

func TestWorkspaceAssetMovePreviewRequiresOrganizationAdmin(t *testing.T) {
	db, mock := newAssetMoveMockDB(t)
	svc := NewWorkspaceAssetMoveService(db, stubAssetMoveOrgService{allowed: false})

	_, err := svc.Preview(context.Background(), "org-1", "acct-1", dto.WorkspaceAssetMoveRequest{
		TargetWorkspaceID: "ws-2",
		Items:             []dto.WorkspaceAssetMoveItem{{Type: AssetMoveTypeDataset, ID: "dataset-1"}},
	})
	require.ErrorIs(t, err, ErrAssetMovePermissionDenied)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceAssetMovePreviewIgnoresOnlyMeLegacyPermission(t *testing.T) {
	db, mock := newAssetMoveMockDB(t)
	expectWorkspaceLookup(mock, "ws-2", "org-1", "normal")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, organization_id, workspace_id FROM "datasets" WHERE id = $1 ORDER BY "datasets"."id" LIMIT $2`)).
		WithArgs("dataset-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "workspace_id"}).AddRow("dataset-1", "org-1", "ws-1"))
	expectWorkspaceLookup(mock, "ws-1", "org-1", "normal")

	svc := NewWorkspaceAssetMoveService(db, stubAssetMoveOrgService{allowed: true})
	preview, err := svc.Preview(context.Background(), "org-1", "acct-1", dto.WorkspaceAssetMoveRequest{
		TargetWorkspaceID: "ws-2",
		Items:             []dto.WorkspaceAssetMoveItem{{Type: AssetMoveTypeDataset, ID: "dataset-1"}},
	})

	require.NoError(t, err)
	require.True(t, preview.Movable)
	require.Empty(t, preview.Items[0].Blockers)
	require.Empty(t, preview.Items[0].Warnings)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceAssetMoveMoveRevalidatesAndBlocksArchivedTarget(t *testing.T) {
	db, mock := newAssetMoveMockDB(t)
	mock.ExpectBegin()
	expectWorkspaceLookup(mock, "ws-2", "org-1", "archived")
	mock.ExpectRollback()

	svc := NewWorkspaceAssetMoveService(db, stubAssetMoveOrgService{allowed: true})
	result, err := svc.Move(context.Background(), "org-1", "acct-1", dto.WorkspaceAssetMoveRequest{
		TargetWorkspaceID: "ws-2",
		Items:             []dto.WorkspaceAssetMoveItem{{Type: AssetMoveTypeDataset, ID: "dataset-1"}},
	})

	require.ErrorIs(t, err, ErrAssetMoveBlocked)
	require.NotNil(t, result)
	require.False(t, result.Preview.Movable)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceAssetMovePreviewRejectsWorkflowAssetType(t *testing.T) {
	db, mock := newAssetMoveMockDB(t)
	expectWorkspaceLookup(mock, "ws-2", "org-1", "normal")

	svc := NewWorkspaceAssetMoveService(db, stubAssetMoveOrgService{allowed: true})
	preview, err := svc.Preview(context.Background(), "org-1", "acct-1", dto.WorkspaceAssetMoveRequest{
		TargetWorkspaceID: "ws-2",
		Items:             []dto.WorkspaceAssetMoveItem{{Type: "workflow", ID: "workflow-1"}},
	})

	require.NoError(t, err)
	require.False(t, preview.Movable)
	require.Equal(t, []string{"unsupported asset type"}, preview.Items[0].Blockers)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceAssetMovePreviewBlocksTargetOutsideOrganization(t *testing.T) {
	db, mock := newAssetMoveMockDB(t)
	expectWorkspaceLookup(mock, "ws-2", "org-2", "normal")

	svc := NewWorkspaceAssetMoveService(db, stubAssetMoveOrgService{allowed: true})
	preview, err := svc.Preview(context.Background(), "org-1", "acct-1", dto.WorkspaceAssetMoveRequest{
		TargetWorkspaceID: "ws-2",
		Items:             []dto.WorkspaceAssetMoveItem{{Type: AssetMoveTypeDataset, ID: "dataset-1"}},
	})

	require.NoError(t, err)
	require.False(t, preview.Movable)
	require.Contains(t, preview.Items[0].Blockers, "target workspace is outside current organization")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceAssetMovePreviewBlocksSourceOutsideOrganization(t *testing.T) {
	db, mock := newAssetMoveMockDB(t)
	expectWorkspaceLookup(mock, "ws-2", "org-1", "normal")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tenant_id FROM "agents" WHERE id = $1 AND deleted_at IS NULL ORDER BY "agents"."id" LIMIT $2`)).
		WithArgs("agent-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id"}).AddRow("agent-1", "ws-1"))
	expectWorkspaceLookup(mock, "ws-1", "org-2", "normal")
	expectWorkspaceLookup(mock, "ws-1", "org-2", "normal")

	svc := NewWorkspaceAssetMoveService(db, stubAssetMoveOrgService{allowed: true})
	preview, err := svc.Preview(context.Background(), "org-1", "acct-1", dto.WorkspaceAssetMoveRequest{
		TargetWorkspaceID: "ws-2",
		Items:             []dto.WorkspaceAssetMoveItem{{Type: AssetMoveTypeAgent, ID: "agent-1"}},
	})

	require.NoError(t, err)
	require.False(t, preview.Movable)
	require.Contains(t, preview.Items[0].Blockers, "agent is outside current organization")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceAssetMovePreviewBlocksDatasetTargetFolderOutsideTargetWorkspace(t *testing.T) {
	db, mock := newAssetMoveMockDB(t)
	expectWorkspaceLookup(mock, "ws-2", "org-1", "normal")
	expectDatasetPreview(mock, "dataset-1", "org-1", "ws-1")
	expectWorkspaceLookup(mock, "ws-1", "org-1", "normal")
	mock.ExpectQuery(`SELECT count\(\*\) FROM "dataset_folders" WHERE id = \$1 AND organization_id = \$2 AND workspace_id = \$3`).
		WithArgs("folder-1", "org-1", "ws-2").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	svc := NewWorkspaceAssetMoveService(db, stubAssetMoveOrgService{allowed: true})
	preview, err := svc.Preview(context.Background(), "org-1", "acct-1", dto.WorkspaceAssetMoveRequest{
		TargetWorkspaceID: "ws-2",
		TargetFolderID:    "folder-1",
		Items:             []dto.WorkspaceAssetMoveItem{{Type: AssetMoveTypeDataset, ID: "dataset-1"}},
	})

	require.NoError(t, err)
	require.False(t, preview.Movable)
	require.Contains(t, preview.Items[0].Blockers, "target folder is not in target workspace")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceAssetMoveAgentMoveUpdatesRelatedTablesAndAudit(t *testing.T) {
	db, mock := newAssetMoveMockDB(t)
	mock.ExpectBegin()
	expectWorkspaceLookup(mock, "ws-2", "org-1", "normal")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tenant_id FROM "agents" WHERE id = $1 AND deleted_at IS NULL ORDER BY "agents"."id" LIMIT $2`)).
		WithArgs("agent-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id"}).AddRow("agent-1", "ws-1"))
	expectWorkspaceLookup(mock, "ws-1", "org-1", "normal")
	expectWorkspaceLookup(mock, "ws-1", "org-1", "normal")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, graph FROM "workflows" WHERE agent_id = $1 OR app_id = $2`)).
		WithArgs("agent-1", "agent-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "graph"}).AddRow("workflow-1", "{}"))
	mock.ExpectExec(`UPDATE "agents" SET .* WHERE id = .* AND deleted_at IS NULL`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "workflows" SET .* WHERE agent_id = .* OR app_id = .*`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "installed_agents" SET .* WHERE agent_id = .*`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "workspace_asset_move_events"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	svc := NewWorkspaceAssetMoveService(db, stubAssetMoveOrgService{allowed: true})
	result, err := svc.Move(context.Background(), "org-1", "acct-1", dto.WorkspaceAssetMoveRequest{
		TargetWorkspaceID: "ws-2",
		Items:             []dto.WorkspaceAssetMoveItem{{Type: AssetMoveTypeAgent, ID: "agent-1"}},
	})

	require.NoError(t, err)
	require.True(t, result.Moved)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceAssetMoveDatasetMoveWithTargetFolderUpdatesJoinAndAudit(t *testing.T) {
	db, mock := newAssetMoveMockDB(t)
	mock.ExpectBegin()
	expectWorkspaceLookup(mock, "ws-2", "org-1", "normal")
	expectDatasetPreview(mock, "dataset-1", "org-1", "ws-1")
	expectWorkspaceLookup(mock, "ws-1", "org-1", "normal")
	mock.ExpectQuery(`SELECT count\(\*\) FROM "dataset_folders" WHERE id = \$1 AND organization_id = \$2 AND workspace_id = \$3`).
		WithArgs("folder-1", "org-1", "ws-2").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec(`UPDATE "datasets" SET .* WHERE id = .*`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "dataset_permissions" SET .* WHERE dataset_id = .*`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM dataset_folder_joins WHERE dataset_id = .*`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "dataset_folder_joins"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "workspace_asset_move_events"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	svc := NewWorkspaceAssetMoveService(db, stubAssetMoveOrgService{allowed: true})
	result, err := svc.Move(context.Background(), "org-1", "acct-1", dto.WorkspaceAssetMoveRequest{
		TargetWorkspaceID: "ws-2",
		TargetFolderID:    "folder-1",
		Items:             []dto.WorkspaceAssetMoveItem{{Type: AssetMoveTypeDataset, ID: "dataset-1"}},
	})

	require.NoError(t, err)
	require.True(t, result.Moved)
	require.NoError(t, mock.ExpectationsWereMet())
}

func newAssetMoveMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	mock.MatchExpectationsInOrder(false)

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{SkipDefaultTransaction: true})
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	return db, mock
}

func expectWorkspaceLookup(mock sqlmock.Sqlmock, workspaceID, orgID, status string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workspaces" WHERE id = $1 ORDER BY "workspaces"."id" LIMIT $2`)).
		WithArgs(workspaceID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "organization_id", "status"}).AddRow(workspaceID, workspaceID, orgID, status))
}

func expectDatasetPreview(mock sqlmock.Sqlmock, datasetID, orgID, workspaceID string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, organization_id, workspace_id FROM "datasets" WHERE id = $1 ORDER BY "datasets"."id" LIMIT $2`)).
		WithArgs(datasetID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "workspace_id"}).AddRow(datasetID, orgID, workspaceID))
}
