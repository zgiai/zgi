package service

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	shared_service "github.com/zgiai/zgi/api/internal/modules/shared/service"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type stubAssetMoveOrgService struct {
	interfaces.AuthorizationService
	allowed                  bool
	allowedPermissions       map[workspace_model.WorkspacePermissionCode]bool
	workspaceIDsByPermission map[workspace_model.WorkspacePermissionCode][]string
	err                      error
	deniedWorkspace          string
	checks                   []assetMovePermissionCheck
	listChecks               []workspace_model.WorkspacePermissionCode
}

type assetMovePermissionCheck struct {
	organizationID string
	workspaceID    string
	accountID      string
	permission     workspace_model.WorkspacePermissionCode
}

func (s *stubAssetMoveOrgService) RequireOrganizationMember(ctx context.Context, req interfaces.OrganizationScopeRequest) (*interfaces.OrganizationScope, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &interfaces.OrganizationScope{OrganizationID: req.OrganizationID, AccountID: req.AccountID}, nil
}

func (s *stubAssetMoveOrgService) RequireWorkspacePermission(ctx context.Context, req interfaces.WorkspaceScopeRequest) (*interfaces.WorkspaceScope, error) {
	permission := req.PermissionCodes[0]
	s.checks = append(s.checks, assetMovePermissionCheck{
		organizationID: req.OrganizationID,
		workspaceID:    req.WorkspaceID,
		accountID:      req.AccountID,
		permission:     permission,
	})
	if s.err != nil {
		return nil, s.err
	}
	if s.deniedWorkspace != "" && s.deniedWorkspace == req.WorkspaceID {
		return nil, shared_service.ErrAuthorizationDenied
	}
	if s.allowedPermissions != nil {
		if !s.allowedPermissions[permission] {
			return nil, shared_service.ErrAuthorizationDenied
		}
		return &interfaces.WorkspaceScope{WorkspaceID: req.WorkspaceID}, nil
	}
	if !s.allowed {
		return nil, shared_service.ErrAuthorizationDenied
	}
	return &interfaces.WorkspaceScope{WorkspaceID: req.WorkspaceID}, nil
}

func (s *stubAssetMoveOrgService) ListWorkspaceIDsByPermission(ctx context.Context, req interfaces.WorkspaceListPermissionRequest) ([]string, error) {
	if s.err != nil {
		return nil, s.err
	}
	s.listChecks = append(s.listChecks, req.PermissionCode)
	ids := s.workspaceIDsByPermission[req.PermissionCode]
	return append([]string(nil), ids...), nil
}

func TestWorkspaceAssetMoveEligibleTargetsUsesResolvedMovePermission(t *testing.T) {
	db, mock := newAssetMoveMockDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT tenant_id, agent_type FROM "agents" WHERE id = $1 AND deleted_at IS NULL LIMIT $2`)).
		WithArgs("workflow-agent-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "agent_type"}).AddRow("ws-source", "WORKFLOW"))
	mock.ExpectQuery(`SELECT count\(\*\) FROM "workspaces" WHERE organization_id = \$1 AND status = \$2 AND id IN \(\$3\)`).
		WithArgs("org-1", workspace_model.WorkspaceStatusNormal, "ws-target").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT id, name FROM "workspaces" WHERE organization_id = \$1 AND status = \$2 AND id IN \(\$3\) ORDER BY LOWER\(name\) ASC,id ASC LIMIT \$4`).
		WithArgs("org-1", workspace_model.WorkspaceStatusNormal, "ws-target", 100).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow("ws-target", "Target workspace"))

	authorization := &stubAssetMoveOrgService{
		allowed: true,
		workspaceIDsByPermission: map[workspace_model.WorkspacePermissionCode][]string{
			workspace_model.WorkspacePermissionWorkflowMove: {"ws-source", "ws-target"},
		},
	}
	svc := NewWorkspaceAssetMoveService(db, authorization, nil)

	result, err := svc.EligibleTargets(context.Background(), "org-1", "acct-1", dto.WorkspaceAssetMoveEligibleTargetsRequest{
		Items: []dto.WorkspaceAssetMoveItem{{Type: AssetMoveTypeAgent, ID: "workflow-agent-1"}},
	})

	require.NoError(t, err)
	require.Equal(t, []dto.WorkspaceAssetMoveWorkspace{{ID: "ws-target", Name: "Target workspace"}}, result.Data)
	require.Equal(t, int64(1), result.Total)
	require.False(t, result.HasMore)
	require.Equal(t, []workspace_model.WorkspacePermissionCode{workspace_model.WorkspacePermissionWorkflowMove}, authorization.listChecks)
	require.Equal(t, []assetMovePermissionCheck{{
		organizationID: "org-1",
		workspaceID:    "ws-source",
		accountID:      "acct-1",
		permission:     workspace_model.WorkspacePermissionWorkflowMove,
	}}, authorization.checks)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceAssetMoveDependencyPreflightChecksSourceWithoutTarget(t *testing.T) {
	db, mock := newAssetMoveMockDB(t)
	organizationID := uuid.NewString()
	workspaceID := uuid.NewString()
	datasetID := uuid.NewString()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT organization_id, workspace_id FROM "datasets" WHERE id = $1 LIMIT $2`)).
		WithArgs(datasetID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"organization_id", "workspace_id"}).AddRow(organizationID, workspaceID))
	authorization := &stubAssetMoveOrgService{allowed: true}
	svc := NewWorkspaceAssetMoveService(db, authorization, nil)

	result, err := svc.PreviewDependencies(context.Background(), organizationID, uuid.NewString(), dto.WorkspaceAssetMoveDependencyPreviewRequest{
		Items: []dto.WorkspaceAssetMoveItem{{Type: AssetMoveTypeDataset, ID: datasetID}},
	})

	require.NoError(t, err)
	require.Nil(t, result.AgentBindingImpact)
	require.Len(t, authorization.checks, 1)
	require.Equal(t, workspaceID, authorization.checks[0].workspaceID)
	require.Equal(t, workspace_model.WorkspacePermissionKnowledgeBaseMove, authorization.checks[0].permission)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceAssetMoveEligibleTargetsIntersectsBatchPermissions(t *testing.T) {
	db, mock := newAssetMoveMockDB(t)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT organization_id, workspace_id FROM "datasets" WHERE id = $1 LIMIT $2`)).
		WithArgs("dataset-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"organization_id", "workspace_id"}).AddRow("org-1", "ws-source-1"))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT organization_id, workspace_id FROM "data_sources" WHERE id = $1 LIMIT $2`)).
		WithArgs("database-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"organization_id", "workspace_id"}).AddRow("org-1", "ws-source-2"))

	authorization := &stubAssetMoveOrgService{
		allowed: true,
		workspaceIDsByPermission: map[workspace_model.WorkspacePermissionCode][]string{
			workspace_model.WorkspacePermissionKnowledgeBaseMove: {"ws-target-1"},
			workspace_model.WorkspacePermissionDatabaseMove:      {"ws-target-2"},
		},
	}
	svc := NewWorkspaceAssetMoveService(db, authorization, nil)

	result, err := svc.EligibleTargets(context.Background(), "org-1", "acct-1", dto.WorkspaceAssetMoveEligibleTargetsRequest{
		Items: []dto.WorkspaceAssetMoveItem{
			{Type: AssetMoveTypeDataset, ID: "dataset-1"},
			{Type: AssetMoveTypeDatabase, ID: "database-1"},
		},
	})

	require.NoError(t, err)
	require.Empty(t, result.Data)
	require.Zero(t, result.Total)
	require.ElementsMatch(t, []workspace_model.WorkspacePermissionCode{
		workspace_model.WorkspacePermissionKnowledgeBaseMove,
		workspace_model.WorkspacePermissionDatabaseMove,
	}, authorization.listChecks)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceAssetMovePreviewRequiresTargetMovePermission(t *testing.T) {
	db, mock := newAssetMoveMockDB(t)
	expectWorkspaceLookup(mock, "ws-2", "org-1", "normal")
	orgService := &stubAssetMoveOrgService{allowed: false}
	svc := NewWorkspaceAssetMoveService(db, orgService, nil)

	_, err := svc.Preview(context.Background(), "org-1", "acct-1", dto.WorkspaceAssetMoveRequest{
		TargetWorkspaceID: "ws-2",
		Items:             []dto.WorkspaceAssetMoveItem{{Type: AssetMoveTypeDataset, ID: "dataset-1"}},
	})
	require.ErrorIs(t, err, ErrAssetMovePermissionDenied)
	require.Equal(t, []assetMovePermissionCheck{{
		organizationID: "org-1",
		workspaceID:    "ws-2",
		accountID:      "acct-1",
		permission:     workspace_model.WorkspacePermissionKnowledgeBaseMove,
	}}, orgService.checks)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceAssetMovePreviewIgnoresOnlyMeLegacyPermission(t *testing.T) {
	db, mock := newAssetMoveMockDB(t)
	expectWorkspaceLookup(mock, "ws-2", "org-1", "normal")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, organization_id, workspace_id FROM "datasets" WHERE id = $1 ORDER BY "datasets"."id" LIMIT $2`)).
		WithArgs("dataset-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "workspace_id"}).AddRow("dataset-1", "org-1", "ws-1"))
	expectWorkspaceLookup(mock, "ws-1", "org-1", "normal")

	orgService := &stubAssetMoveOrgService{allowed: true}
	svc := NewWorkspaceAssetMoveService(db, orgService, nil)
	preview, err := svc.Preview(context.Background(), "org-1", "acct-1", dto.WorkspaceAssetMoveRequest{
		TargetWorkspaceID: "ws-2",
		Items:             []dto.WorkspaceAssetMoveItem{{Type: AssetMoveTypeDataset, ID: "dataset-1"}},
	})

	require.NoError(t, err)
	require.True(t, preview.Movable)
	require.Empty(t, preview.Items[0].Blockers)
	require.Empty(t, preview.Items[0].Warnings)
	require.Equal(t, []assetMovePermissionCheck{
		{organizationID: "org-1", workspaceID: "ws-2", accountID: "acct-1", permission: workspace_model.WorkspacePermissionKnowledgeBaseMove},
		{organizationID: "org-1", workspaceID: "ws-1", accountID: "acct-1", permission: workspace_model.WorkspacePermissionKnowledgeBaseMove},
	}, orgService.checks)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceAssetMoveMoveRevalidatesAndBlocksArchivedTarget(t *testing.T) {
	db, mock := newAssetMoveMockDB(t)
	mock.ExpectBegin()
	expectAssetMoveLock(mock, AssetMoveTypeDataset, "dataset-1")
	expectWorkspaceLookup(mock, "ws-2", "org-1", "archived")
	mock.ExpectRollback()

	svc := NewWorkspaceAssetMoveService(db, &stubAssetMoveOrgService{allowed: true}, nil)
	result, err := svc.Move(context.Background(), "org-1", "acct-1", dto.WorkspaceAssetMoveRequest{
		TargetWorkspaceID: "ws-2",
		Items:             []dto.WorkspaceAssetMoveItem{{Type: AssetMoveTypeDataset, ID: "dataset-1"}},
	})

	require.ErrorIs(t, err, ErrAssetMoveBlocked)
	require.NotNil(t, result)
	require.False(t, result.Preview.Movable)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceAssetMoveLocksAssetBeforeAuthorizingLatestSourceWorkspace(t *testing.T) {
	db, mock := newAssetMoveMockDB(t)
	mock.MatchExpectationsInOrder(true)
	mock.ExpectBegin()
	expectAssetMoveLock(mock, AssetMoveTypeDataset, "dataset-1")
	expectWorkspaceLookup(mock, "ws-c", "org-1", "normal")
	expectDatasetPreview(mock, "dataset-1", "org-1", "ws-b")
	expectWorkspaceLookup(mock, "ws-b", "org-1", "normal")
	mock.ExpectRollback()

	authorization := &stubAssetMoveOrgService{allowed: true, deniedWorkspace: "ws-b"}
	svc := NewWorkspaceAssetMoveService(db, authorization, nil)

	_, err := svc.Move(context.Background(), "org-1", "acct-1", dto.WorkspaceAssetMoveRequest{
		TargetWorkspaceID: "ws-c",
		Items:             []dto.WorkspaceAssetMoveItem{{Type: AssetMoveTypeDataset, ID: "dataset-1"}},
	})

	require.ErrorIs(t, err, ErrAssetMovePermissionDenied)
	require.Equal(t, []assetMovePermissionCheck{
		{organizationID: "org-1", workspaceID: "ws-c", accountID: "acct-1", permission: workspace_model.WorkspacePermissionKnowledgeBaseMove},
		{organizationID: "org-1", workspaceID: "ws-b", accountID: "acct-1", permission: workspace_model.WorkspacePermissionKnowledgeBaseMove},
	}, authorization.checks)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceAssetMovePreviewRejectsWorkflowAssetType(t *testing.T) {
	db, mock := newAssetMoveMockDB(t)
	expectWorkspaceLookup(mock, "ws-2", "org-1", "normal")

	svc := NewWorkspaceAssetMoveService(db, &stubAssetMoveOrgService{allowed: true}, nil)
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

	svc := NewWorkspaceAssetMoveService(db, &stubAssetMoveOrgService{allowed: true}, nil)
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
	expectAgentPreview(mock, "agent-1", "ws-1", "AGENT")
	expectWorkspaceLookup(mock, "ws-1", "org-2", "normal")
	expectWorkspaceLookup(mock, "ws-1", "org-2", "normal")

	svc := NewWorkspaceAssetMoveService(db, &stubAssetMoveOrgService{allowed: true}, nil)
	preview, err := svc.Preview(context.Background(), "org-1", "acct-1", dto.WorkspaceAssetMoveRequest{
		TargetWorkspaceID: "ws-2",
		Items:             []dto.WorkspaceAssetMoveItem{{Type: AssetMoveTypeAgent, ID: "agent-1"}},
	})

	require.NoError(t, err)
	require.False(t, preview.Movable)
	require.Contains(t, preview.Items[0].Blockers, "agent is outside current organization")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceAssetMovePreviewWorkflowAgentRequiresWorkflowMovePermission(t *testing.T) {
	db, mock := newAssetMoveMockDB(t)
	expectWorkspaceLookup(mock, "ws-2", "org-1", "normal")
	expectAgentPreview(mock, "workflow-agent-1", "ws-1", "WORKFLOW")
	expectWorkspaceLookup(mock, "ws-1", "org-1", "normal")
	expectWorkspaceLookup(mock, "ws-1", "org-1", "normal")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, graph FROM "workflows" WHERE agent_id = $1 OR app_id = $2`)).
		WithArgs("workflow-agent-1", "workflow-agent-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "graph"}))

	orgService := &stubAssetMoveOrgService{
		allowedPermissions: map[workspace_model.WorkspacePermissionCode]bool{
			workspace_model.WorkspacePermissionAgentMove:    true,
			workspace_model.WorkspacePermissionWorkflowMove: false,
		},
	}
	svc := NewWorkspaceAssetMoveService(db, orgService, nil)

	_, err := svc.Preview(context.Background(), "org-1", "acct-1", dto.WorkspaceAssetMoveRequest{
		TargetWorkspaceID: "ws-2",
		Items:             []dto.WorkspaceAssetMoveItem{{Type: AssetMoveTypeAgent, ID: "workflow-agent-1"}},
	})

	require.ErrorIs(t, err, ErrAssetMovePermissionDenied)
	require.Equal(t, []assetMovePermissionCheck{
		{organizationID: "org-1", workspaceID: "ws-2", accountID: "acct-1", permission: workspace_model.WorkspacePermissionAgentMove},
		{organizationID: "org-1", workspaceID: "ws-2", accountID: "acct-1", permission: workspace_model.WorkspacePermissionWorkflowMove},
	}, orgService.checks)
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

	svc := NewWorkspaceAssetMoveService(db, &stubAssetMoveOrgService{allowed: true}, nil)
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

func TestWorkspaceAssetMovePreviewBlocksSameWorkspaceForAgent(t *testing.T) {
	db, mock := newAssetMoveMockDB(t)
	expectWorkspaceLookup(mock, "ws-1", "org-1", "normal")
	expectAgentPreview(mock, "agent-1", "ws-1", "AGENT")
	expectWorkspaceLookup(mock, "ws-1", "org-1", "normal")
	expectWorkspaceLookup(mock, "ws-1", "org-1", "normal")

	svc := NewWorkspaceAssetMoveService(db, &stubAssetMoveOrgService{allowed: true}, nil)
	preview, err := svc.Preview(context.Background(), "org-1", "acct-1", dto.WorkspaceAssetMoveRequest{
		TargetWorkspaceID: "ws-1",
		Items:             []dto.WorkspaceAssetMoveItem{{Type: AssetMoveTypeAgent, ID: "agent-1"}},
	})

	require.NoError(t, err)
	require.False(t, preview.Movable)
	require.Contains(t, preview.Items[0].Blockers, "asset is already in target workspace")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceAssetMoveDatasetMoveBlocksSameWorkspaceWithoutClearingFolderJoin(t *testing.T) {
	db, mock := newAssetMoveMockDB(t)
	mock.ExpectBegin()
	expectAssetMoveLock(mock, AssetMoveTypeDataset, "dataset-1")
	expectWorkspaceLookup(mock, "ws-1", "org-1", "normal")
	expectDatasetPreview(mock, "dataset-1", "org-1", "ws-1")
	expectWorkspaceLookup(mock, "ws-1", "org-1", "normal")
	mock.ExpectRollback()

	svc := NewWorkspaceAssetMoveService(db, &stubAssetMoveOrgService{allowed: true}, nil)
	result, err := svc.Move(context.Background(), "org-1", "acct-1", dto.WorkspaceAssetMoveRequest{
		TargetWorkspaceID: "ws-1",
		Items:             []dto.WorkspaceAssetMoveItem{{Type: AssetMoveTypeDataset, ID: "dataset-1"}},
	})

	require.ErrorIs(t, err, ErrAssetMoveBlocked)
	require.NotNil(t, result)
	require.False(t, result.Preview.Movable)
	require.Contains(t, result.Preview.Items[0].Blockers, "asset is already in target workspace")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceAssetMoveFileMoveBlocksSameWorkspaceWithoutClearingFolderJoin(t *testing.T) {
	db, mock := newAssetMoveMockDB(t)
	mock.ExpectBegin()
	expectAssetMoveLock(mock, AssetMoveTypeFile, "file-1")
	expectWorkspaceLookup(mock, "ws-1", "org-1", "normal")
	expectFilePreview(mock, "file-1", "org-1", "ws-1")
	expectWorkspaceLookup(mock, "ws-1", "org-1", "normal")
	mock.ExpectRollback()

	svc := NewWorkspaceAssetMoveService(db, &stubAssetMoveOrgService{allowed: true}, nil)
	result, err := svc.Move(context.Background(), "org-1", "acct-1", dto.WorkspaceAssetMoveRequest{
		TargetWorkspaceID: "ws-1",
		Items:             []dto.WorkspaceAssetMoveItem{{Type: AssetMoveTypeFile, ID: "file-1"}},
	})

	require.ErrorIs(t, err, ErrAssetMoveBlocked)
	require.NotNil(t, result)
	require.False(t, result.Preview.Movable)
	require.Contains(t, result.Preview.Items[0].Blockers, "asset is already in target workspace")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceAssetMovePreviewBlocksSameWorkspaceForDatabase(t *testing.T) {
	db, mock := newAssetMoveMockDB(t)
	expectWorkspaceLookup(mock, "ws-1", "org-1", "normal")
	expectDatabasePreview(mock, "database-1", "org-1", "ws-1")
	expectWorkspaceLookup(mock, "ws-1", "org-1", "normal")

	svc := NewWorkspaceAssetMoveService(db, &stubAssetMoveOrgService{allowed: true}, nil)
	preview, err := svc.Preview(context.Background(), "org-1", "acct-1", dto.WorkspaceAssetMoveRequest{
		TargetWorkspaceID: "ws-1",
		Items:             []dto.WorkspaceAssetMoveItem{{Type: AssetMoveTypeDatabase, ID: "database-1"}},
	})

	require.NoError(t, err)
	require.False(t, preview.Movable)
	require.Contains(t, preview.Items[0].Blockers, "asset is already in target workspace")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkspaceAssetMoveAgentMoveUpdatesRelatedTablesAndAudit(t *testing.T) {
	db, mock := newAssetMoveMockDB(t)
	mock.ExpectBegin()
	expectAssetMoveLock(mock, AssetMoveTypeAgent, "agent-1")
	expectWorkspaceLookup(mock, "ws-2", "org-1", "normal")
	expectAgentPreview(mock, "agent-1", "ws-1", "AGENT")
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

	svc := NewWorkspaceAssetMoveService(db, &stubAssetMoveOrgService{allowed: true}, nil)
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
	expectAssetMoveLock(mock, AssetMoveTypeDataset, "dataset-1")
	expectWorkspaceLookup(mock, "ws-2", "org-1", "normal")
	expectDatasetPreview(mock, "dataset-1", "org-1", "ws-1")
	expectWorkspaceLookup(mock, "ws-1", "org-1", "normal")
	mock.ExpectQuery(`SELECT count\(\*\) FROM "dataset_folders" WHERE id = \$1 AND organization_id = \$2 AND workspace_id = \$3`).
		WithArgs("folder-1", "org-1", "ws-2").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectExec(`UPDATE "datasets" SET .* WHERE id = .*`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM dataset_folder_joins WHERE dataset_id = .*`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "dataset_folder_joins"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "workspace_asset_move_events"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	svc := NewWorkspaceAssetMoveService(db, &stubAssetMoveOrgService{allowed: true}, nil)
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

func expectAgentPreview(mock sqlmock.Sqlmock, agentID, workspaceID, agentType string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, tenant_id, agent_type FROM "agents" WHERE id = $1 AND deleted_at IS NULL ORDER BY "agents"."id" LIMIT $2`)).
		WithArgs(agentID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "agent_type"}).AddRow(agentID, workspaceID, agentType))
}

func expectDatasetPreview(mock sqlmock.Sqlmock, datasetID, orgID, workspaceID string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, organization_id, workspace_id FROM "datasets" WHERE id = $1 ORDER BY "datasets"."id" LIMIT $2`)).
		WithArgs(datasetID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "workspace_id"}).AddRow(datasetID, orgID, workspaceID))
}

func expectFilePreview(mock sqlmock.Sqlmock, fileID, orgID, workspaceID string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, organization_id, workspace_id FROM "upload_files" WHERE id = $1 ORDER BY "upload_files"."id" LIMIT $2`)).
		WithArgs(fileID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "workspace_id"}).AddRow(fileID, orgID, workspaceID))
}

func expectDatabasePreview(mock sqlmock.Sqlmock, databaseID, orgID, workspaceID string) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, organization_id, workspace_id FROM "data_sources" WHERE id = $1 ORDER BY "data_sources"."id" LIMIT $2`)).
		WithArgs(databaseID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "workspace_id"}).AddRow(databaseID, orgID, workspaceID))
}

func expectAssetMoveLock(mock sqlmock.Sqlmock, assetType, assetID string) {
	table := assetMoveTable(assetType)
	query := `SELECT "id" FROM "` + table + `" WHERE id = \$1`
	if assetType == AssetMoveTypeAgent {
		query += ` AND deleted_at IS NULL`
	}
	query += ` LIMIT \$2 FOR UPDATE`
	mock.ExpectQuery(query).
		WithArgs(assetID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(assetID))
}
