package runtimeworkspace

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestCreateCredentialRejectsExpiredAndOversizedInput(t *testing.T) {
	service := NewService(&gorm.DB{})
	expired := time.Now().Add(-time.Minute)
	if _, err := service.CreateCredential(t.Context(), uuid.New(), uuid.New(), "expired", &expired, uuid.NewString()); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("expired credential error = %v, want ErrInvalidCredential", err)
	}
	if _, err := service.CreateCredential(t.Context(), uuid.New(), uuid.New(), strings.Repeat("界", 256), nil, uuid.NewString()); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("oversized credential name error = %v, want ErrInvalidCredential", err)
	}
}

func TestBindAgentRejectsMissingScopeBeforeDatabaseAccess(t *testing.T) {
	service := NewService(&gorm.DB{})
	if _, err := service.BindAgent(t.Context(), uuid.Nil, uuid.New()); !errors.Is(err, ErrInvalidBinding) {
		t.Fatalf("missing Runtime Workspace error = %v, want ErrInvalidBinding", err)
	}
	if _, err := service.BindAgent(t.Context(), uuid.New(), uuid.Nil); !errors.Is(err, ErrInvalidBinding) {
		t.Fatalf("missing Agent error = %v, want ErrInvalidBinding", err)
	}
}

func TestLoadBoundAgentsAllowsAuthorizedAgentFromAnotherWorkspace(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	runtimeWorkspaceID, agentWorkspaceID, organizationID, agentID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	mock.ExpectQuery(`(?s)SELECT agents\.id, agents\.tenant_id, workspaces\.organization_id.*FROM agents AS agents.*JOIN workspaces AS workspaces ON workspaces\.id = agents\.tenant_id.*agents\.id IN \(\$1\)`).
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "organization_id", "name", "description", "enable_api", "web_app_status", "published"}).
			AddRow(agentID, agentWorkspaceID, organizationID, "Shared Agent", "Cross-workspace published Agent", true, "active", true))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "published_runtime_surfaces" WHERE resource_type = $1 AND surface = $2 AND organization_id = $3 AND resource_id IN ($4) AND deleted_at IS NULL ORDER BY resource_id ASC`)).
		WithArgs("agent", "api", organizationID, agentID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "resource_id"}))
	workspace := &RuntimeWorkspace{WorkspaceID: runtimeWorkspaceID, OrganizationID: organizationID}
	agents, authorized, err := NewService(db).loadBoundAgents(t.Context(), workspace, []AgentBinding{{RuntimeWorkspaceID: runtimeWorkspaceID, AgentID: agentID}})
	if err != nil {
		t.Fatal(err)
	}
	if agents[agentID].TenantID != agentWorkspaceID {
		t.Fatalf("agent workspace = %s, want %s", agents[agentID].TenantID, agentWorkspaceID)
	}
	if _, ok := authorized[agentID]; !ok {
		t.Fatal("cross-workspace Agent was not authorized")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeAgentUnavailableReason(t *testing.T) {
	active := runtimeAgentRecord{Published: true, EnableAPI: true, WebAppStatus: "active"}
	for _, test := range []struct {
		name       string
		record     runtimeAgentRecord
		found      bool
		authorized bool
		want       string
	}{
		{name: "missing", found: false, want: "agent_not_found"},
		{name: "unpublished", record: runtimeAgentRecord{EnableAPI: true, WebAppStatus: "active"}, found: true, want: "agent_not_published"},
		{name: "disabled", record: runtimeAgentRecord{Published: true, WebAppStatus: "active"}, found: true, want: "agent_disabled"},
		{name: "unauthorized", record: active, found: true, want: "agent_not_authorized"},
		{name: "available", record: active, found: true, authorized: true, want: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := runtimeAgentUnavailableReason(test.record, test.found, test.authorized); got != test.want {
				t.Fatalf("reason = %q, want %q", got, test.want)
			}
		})
	}
}
