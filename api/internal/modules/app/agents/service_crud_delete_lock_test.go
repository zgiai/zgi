package agents

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type deleteLockAgentsRepository struct {
	AgentsRepository
	agent *Agent
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
	mock.ExpectQuery(`SELECT \* FROM "agents" WHERE id = \$1 AND deleted_at IS NULL LIMIT \$2 FOR UPDATE`).
		WithArgs(agentID.String(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "name", "agent_type", "deleted_at"}).
			AddRow(agentID, lockedWorkspaceID, "assistant", "AGENT", nil))
	mock.ExpectRollback()

	ctx := context.WithValue(context.Background(), "account_id", accountID)
	ctx = context.WithValue(ctx, "tenant_id", organizationID)
	if err := svc.DeleteAgent(ctx, agentID.String()); err == nil {
		t.Fatal("DeleteAgent() error = nil, want locked workspace permission denial")
	}
	if len(organization.checkedWorkspaces) != 2 || organization.checkedWorkspaces[1] != lockedWorkspaceID.String() {
		t.Fatalf("checked workspaces = %#v, want source then locked workspace", organization.checkedWorkspaces)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet SQL expectations: %v", err)
	}
}
