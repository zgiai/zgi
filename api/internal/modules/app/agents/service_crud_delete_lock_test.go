package agents

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	connectorbindings "github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type deleteLockAgentsRepository struct {
	AgentsRepository
	agent *Agent
}

func TestDeleteWorkflowAgentRollsBackCanonicalLineageWhenSoftDeleteFails(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}

	organizationID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	workspaceID := uuid.MustParse("00000000-0000-4000-8000-000000000002")
	agentID := uuid.MustParse("00000000-0000-4000-8000-000000000003")
	draftID := uuid.MustParse("00000000-0000-4000-8000-000000000010")
	publishedVersionID := uuid.MustParse("00000000-0000-4000-8000-000000000020")
	publishedRowID := uuid.MustParse("00000000-0000-4000-8000-000000000030")
	ownerID, connectionID := uuid.New(), uuid.New()
	accountID := uuid.NewString()
	now := time.Now().UTC()
	organization := &deleteLockOrganizationService{
		organizationID: organizationID.String(),
		permissionsByWorkspace: map[string]bool{
			workspaceID.String(): true,
		},
	}
	svc := &agentsService{
		agentsRepo: &deleteLockAgentsRepository{agent: &Agent{
			ID: agentID, TenantID: workspaceID, Name: "workflow", AgentsType: "WORKFLOW",
		}},
		enterpriseService:       organization,
		db:                      db,
		connectorTargetBindings: connectorbindings.NewGormRepository(db),
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
		WithArgs("connector-workflow-lineage:" + organizationID.String() + ":" + agentID.String()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT id, version, version_uuid FROM "workflows" WHERE agent_id = \$1 AND tenant_id = \$2 ORDER BY id ASC`).
		WithArgs(agentID, workspaceID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "version", "version_uuid"}).
			AddRow(draftID, "draft", nil).
			AddRow(publishedRowID, "20260901000000", publishedVersionID))
	for _, targetID := range []uuid.UUID{draftID, publishedVersionID} {
		mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
			WithArgs("connector-workflow-target:" + organizationID.String() + ":" + targetID.String()).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	for _, targetID := range []uuid.UUID{draftID, publishedVersionID} {
		scope, version := connectorcore.ScopeDraft, ""
		if targetID == publishedVersionID {
			scope, version = connectorcore.ScopePublished, publishedVersionID.String()
		}
		mock.ExpectQuery(`SELECT \* FROM "connector_target_bindings" WHERE organization_id = \$1 AND target_type = \$2 AND target_id = \$3 AND target_scope = \$4 AND target_version_id = \$5 ORDER BY owner_account_id ASC, connector_id ASC, binding_instance_id ASC, id ASC`).
			WithArgs(organizationID, connectorcore.TargetTypeWorkflow, targetID.String(), scope, version).
			WillReturnRows(sqlmock.NewRows(workflowConnectorCanonicalColumns()).AddRow(
				uuid.New(), organizationID, ownerID, connectionID, "github", connectorcore.TargetTypeWorkflow,
				targetID.String(), "node-1", connectorcore.BindingStatusActive,
				1, nil, now, now, scope, version, agentWorkflowTestConstraint(ownerID),
			))
	}
	expectAgentWorkflowCanonicalAuthority(mock, organizationID, ownerID, connectionID, "github")
	for _, targetID := range []uuid.UUID{draftID, publishedVersionID} {
		scope, version := connectorcore.ScopeDraft, ""
		if targetID == publishedVersionID {
			scope, version = connectorcore.ScopePublished, publishedVersionID.String()
		}
		mock.ExpectQuery(`SELECT \* FROM "connector_target_bindings" WHERE organization_id = \$1 AND target_type = \$2 AND target_id = \$3 AND target_scope = \$4 AND target_version_id = \$5 ORDER BY owner_account_id ASC, connector_id ASC, binding_instance_id ASC, id ASC FOR UPDATE`).
			WithArgs(organizationID, connectorcore.TargetTypeWorkflow, targetID.String(), scope, version).
			WillReturnRows(sqlmock.NewRows(workflowConnectorCanonicalColumns()).AddRow(
				uuid.New(), organizationID, ownerID, connectionID, "github", connectorcore.TargetTypeWorkflow,
				targetID.String(), "node-1", connectorcore.BindingStatusActive,
				1, nil, now, now, scope, version, agentWorkflowTestConstraint(ownerID),
			))
		mock.ExpectExec(`UPDATE "connector_target_bindings" SET .* WHERE id = \$[0-9]+ AND organization_id = \$[0-9]+ AND revision = \$[0-9]+`).
			WillReturnResult(sqlmock.NewResult(0, 1))
		expectAgentWorkflowCanonicalLifecycle(mock, organizationID, ownerID, connectionID, "github")
	}
	mock.ExpectQuery(`SELECT \* FROM "agents" WHERE id = \$1 AND deleted_at IS NULL LIMIT \$2 FOR UPDATE`).
		WithArgs(agentID.String(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "name", "agent_type", "deleted_at"}).
			AddRow(agentID, workspaceID, "workflow", "WORKFLOW", nil))
	mock.ExpectExec(`UPDATE "agents" SET .* WHERE id = \$[0-9]+`).
		WillReturnError(errors.New("soft delete unavailable"))
	mock.ExpectRollback()

	ctx := context.WithValue(context.Background(), "account_id", accountID)
	ctx = context.WithValue(ctx, "tenant_id", organizationID.String())
	if err := svc.DeleteAgent(ctx, agentID.String()); err == nil {
		t.Fatal("DeleteAgent() error = nil, want transaction failure")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func workflowConnectorCanonicalColumns() []string {
	return []string{
		"id", "organization_id", "owner_account_id", "connection_id", "connector_id",
		"target_type", "target_id", "binding_instance_id", "status",
		"revision", "disabled_at", "created_at", "updated_at", "target_scope", "target_version_id", "binding_constraint",
	}
}

func expectAgentWorkflowCanonicalLifecycle(
	mock sqlmock.Sqlmock,
	organizationID, ownerID, connectionID uuid.UUID,
	connectorID string,
) {
	mock.ExpectExec(`INSERT INTO "connector_lifecycle_decisions"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectAgentWorkflowCanonicalAuthority(
	mock sqlmock.Sqlmock,
	organizationID, ownerID, connectionID uuid.UUID,
	connectorID string,
) {
	mock.ExpectQuery(`SELECT "id" FROM "connector_connections"`).WithArgs(connectionID, organizationID, ownerID, connectorID, 1).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(connectionID))
}

func (r *deleteLockAgentsRepository) GetByID(context.Context, string) (*Agent, error) {
	copy := *r.agent
	return &copy, nil
}

type deleteLockOrganizationService struct {
	interfaces.OrganizationService
	organizationID         string
	permissionsByWorkspace map[string]bool
	checkedWorkspaces      []string
}

func (s *deleteLockOrganizationService) GetOrganizationByWorkspaceID(context.Context, string) (*workspace_model.Organization, error) {
	return &workspace_model.Organization{ID: s.organizationID}, nil
}

func (s *deleteLockOrganizationService) CheckWorkspaceOrganizationAnyPermission(_ context.Context, _, workspaceID, _ string, _ ...workspace_model.WorkspacePermissionCode) (bool, error) {
	s.checkedWorkspaces = append(s.checkedWorkspaces, workspaceID)
	return s.permissionsByWorkspace[workspaceID], nil
}

func TestDeleteAgentReauthorizesLockedWorkspaceAfterLifecycleLock(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}

	organizationID := uuid.NewString()
	agentID := uuid.New()
	accountID := uuid.NewString()
	sourceWorkspaceID := uuid.New()
	lockedWorkspaceID := uuid.New()
	organization := &deleteLockOrganizationService{
		organizationID: organizationID,
		permissionsByWorkspace: map[string]bool{
			sourceWorkspaceID.String(): true,
			lockedWorkspaceID.String(): false,
		},
	}
	svc := &agentsService{
		agentsRepo: &deleteLockAgentsRepository{agent: &Agent{
			ID:         agentID,
			TenantID:   sourceWorkspaceID,
			Name:       "assistant",
			AgentsType: "AGENT",
		}},
		enterpriseService: organization,
		db:                db,
		agentBindings:     agentbindings.NewRepositoryWithTokenSecret(db, "delete-lock-test-secret"),
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
		WithArgs("agent-binding-resource:" + organizationID + ":workflow::" + agentID.String()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT \* FROM "agent_resource_bindings" WHERE agent_id = \$1`).
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
		WithArgs("agent-binding-agent:" + agentID.String()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM "agent_resource_bindings" WHERE agent_id = \$1`).
		WithArgs(agentID).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`SELECT \* FROM "agents" WHERE id = \$1 AND deleted_at IS NULL LIMIT \$2 FOR UPDATE`).
		WithArgs(agentID.String(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "name", "agent_type", "deleted_at"}).
			AddRow(agentID, lockedWorkspaceID, "assistant", "AGENT", nil))
	mock.ExpectRollback()

	ctx := context.WithValue(context.Background(), "account_id", accountID)
	ctx = context.WithValue(ctx, "tenant_id", organizationID)
	err = svc.DeleteAgent(ctx, agentID.String())
	if !errors.Is(err, errAgentPermissionDenied) {
		t.Fatalf("DeleteAgent() error = %v, want errAgentPermissionDenied", err)
	}
	if len(organization.checkedWorkspaces) != 2 || organization.checkedWorkspaces[1] != lockedWorkspaceID.String() {
		t.Fatalf("checked workspaces = %#v, want source then locked workspace", organization.checkedWorkspaces)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}

func agentWorkflowTestConstraint(owner uuid.UUID) string {
	encoded, _ := json.Marshal(connectorcore.OwnerConstraint(owner, []connectorcore.Identifier{"github.user.get"}))
	return string(encoded)
}
