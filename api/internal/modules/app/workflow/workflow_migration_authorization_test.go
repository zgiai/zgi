package workflow

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/app/agents"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func TestWebAppMigrationAuthorizer_AllowsPublicGrantWithoutOrganizationMembership(t *testing.T) {
	db, mock := setWorkflowWebAppRuntimeMockDB(t)
	agentID := uuid.New()
	webAppID := uuid.New()
	workspaceID := uuid.New()
	organizationID := uuid.New()
	accountID := uuid.New()

	expectWorkflowMigrationAgentByWebApp(mock, agentID, webAppID, workspaceID, "active")
	expectWorkflowMigrationRuntimeSurface(mock, agentID, organizationID, workspaceID, true, runtimeauth.PublishedRuntimeSubjectPublic, uuid.Nil)

	authorizer := NewWebAppMigrationAuthorizer(agents.NewAgentsRepository(db), db)
	err := authorizer.AuthorizeWebAppMigration(context.Background(), webAppID.String(), accountID.String())
	if err != nil {
		t.Fatalf("AuthorizeWebAppMigration error = %v, want nil", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestWebAppMigrationAuthorizer_AllowsOrganizationGrantForActiveMember(t *testing.T) {
	db, mock := setWorkflowWebAppRuntimeMockDB(t)
	agentID := uuid.New()
	webAppID := uuid.New()
	workspaceID := uuid.New()
	organizationID := uuid.New()
	accountID := uuid.New()

	expectWorkflowMigrationAgentByWebApp(mock, agentID, webAppID, workspaceID, "active")
	expectWorkflowMigrationRuntimeSurface(mock, agentID, organizationID, workspaceID, true, runtimeauth.PublishedRuntimeSubjectOrganization, organizationID)
	expectWorkflowMigrationAudience(mock, accountID, organizationID, nil, 1)

	authorizer := NewWebAppMigrationAuthorizer(agents.NewAgentsRepository(db), db)
	err := authorizer.AuthorizeWebAppMigration(context.Background(), webAppID.String(), accountID.String())
	if err != nil {
		t.Fatalf("AuthorizeWebAppMigration error = %v, want nil", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestWebAppMigrationAuthorizer_RejectsOrganizationGrantForNonMember(t *testing.T) {
	db, mock := setWorkflowWebAppRuntimeMockDB(t)
	agentID := uuid.New()
	webAppID := uuid.New()
	workspaceID := uuid.New()
	organizationID := uuid.New()
	accountID := uuid.New()

	expectWorkflowMigrationAgentByWebApp(mock, agentID, webAppID, workspaceID, "active")
	expectWorkflowMigrationRuntimeSurface(mock, agentID, organizationID, workspaceID, true, runtimeauth.PublishedRuntimeSubjectOrganization, organizationID)
	expectWorkflowMigrationAudience(mock, accountID, organizationID, nil, 0)

	authorizer := NewWebAppMigrationAuthorizer(agents.NewAgentsRepository(db), db)
	err := authorizer.AuthorizeWebAppMigration(context.Background(), webAppID.String(), accountID.String())
	if !errors.Is(err, errWebAppMigrationAccessDenied) {
		t.Fatalf("AuthorizeWebAppMigration error = %v, want %v", err, errWebAppMigrationAccessDenied)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestWebAppMigrationAuthorizer_RejectsDisabledWebApp(t *testing.T) {
	db, mock := setWorkflowWebAppRuntimeMockDB(t)
	agentID := uuid.New()
	webAppID := uuid.New()
	workspaceID := uuid.New()
	organizationID := uuid.New()
	accountID := uuid.New()

	expectWorkflowMigrationAgentByWebApp(mock, agentID, webAppID, workspaceID, "active")
	expectWorkflowMigrationRuntimeSurface(mock, agentID, organizationID, workspaceID, false, runtimeauth.PublishedRuntimeSubjectOrganization, organizationID)

	authorizer := NewWebAppMigrationAuthorizer(agents.NewAgentsRepository(db), db)
	err := authorizer.AuthorizeWebAppMigration(context.Background(), webAppID.String(), accountID.String())
	if !errors.Is(err, errWebAppMigrationOffline) {
		t.Fatalf("AuthorizeWebAppMigration error = %v, want %v", err, errWebAppMigrationOffline)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func expectWorkflowMigrationAgentByWebApp(mock sqlmock.Sqlmock, agentID, webAppID, workspaceID uuid.UUID, webAppStatus string) {
	now := time.Now().UTC().Truncate(time.Second)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "agents"`)).
		WithArgs(webAppID.String(), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"tenant_id",
			"name",
			"description",
			"agent_type",
			"enable_api",
			"web_app_id",
			"web_app_status",
			"created_by",
			"created_at",
			"updated_at",
			"deleted_at",
		}).AddRow(
			agentID.String(),
			workspaceID.String(),
			"private-workflow-app",
			"",
			"AGENT",
			true,
			webAppID.String(),
			webAppStatus,
			uuid.NewString(),
			now,
			now,
			nil,
		))
}

func expectWorkflowMigrationRuntimeSurface(mock sqlmock.Sqlmock, agentID, organizationID, workspaceID uuid.UUID, enabled bool, subjectType runtimeauth.PublishedRuntimeSubjectType, subjectID uuid.UUID) {
	surfaceID := uuid.New()
	now := time.Now().UTC().Truncate(time.Second)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "published_runtime_surfaces" WHERE resource_type = $1 AND resource_id = $2 AND deleted_at IS NULL ORDER BY surface ASC`)).
		WithArgs(string(runtimeauth.PublishedRuntimeResourceAgent), agentID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"resource_type",
			"resource_id",
			"organization_id",
			"workspace_id",
			"surface",
			"enabled",
			"compatibility_source",
			"created_at",
			"updated_at",
			"deleted_at",
		}).AddRow(
			surfaceID.String(),
			string(runtimeauth.PublishedRuntimeResourceAgent),
			agentID.String(),
			organizationID.String(),
			workspaceID.String(),
			string(runtimeauth.PublishedRuntimeSurfaceWebApp),
			enabled,
			runtimeauth.PublishedRuntimeSourceGrant,
			now,
			now,
			nil,
		))

	grantRows := sqlmock.NewRows([]string{
		"id",
		"surface_id",
		"subject_type",
		"subject_id",
		"enabled",
		"created_at",
		"updated_at",
		"deleted_at",
	})
	var grantSubjectID any
	if subjectID != uuid.Nil {
		grantSubjectID = subjectID.String()
	}
	grantRows.AddRow(
		uuid.NewString(),
		surfaceID.String(),
		string(subjectType),
		grantSubjectID,
		true,
		now,
		now,
		nil,
	)
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surface_grants" WHERE surface_id IN \(.+\) AND deleted_at IS NULL ORDER BY subject_type ASC, subject_id ASC, created_at ASC`).
		WillReturnRows(grantRows)
}

func expectWorkflowMigrationAudience(mock sqlmock.Sqlmock, accountID, organizationID uuid.UUID, departmentIDs []uuid.UUID, memberCount int64) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "members" WHERE organization_id = $1 AND account_id = $2 AND status = $3`)).
		WithArgs(organizationID.String(), accountID.String(), workspacemodel.OrganizationMemberStatusActive).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(memberCount))
	if memberCount == 0 {
		return
	}

	rows := sqlmock.NewRows([]string{"department_id"})
	for _, departmentID := range departmentIDs {
		rows.AddRow(departmentID.String())
	}
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT department_members.department_id FROM "department_members" JOIN departments ON departments.id = department_members.department_id WHERE department_members.account_id = $1 AND departments.group_id = $2 AND departments.status = $3`)).
		WithArgs(accountID, organizationID, workspacemodel.DepartmentStatusActive).
		WillReturnRows(rows)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT workspace_members.workspace_id FROM "workspace_members" JOIN workspaces ON workspaces.id = workspace_members.workspace_id WHERE workspace_members.account_id = $1 AND workspaces.organization_id = $2 AND workspaces.status = $3`)).
		WithArgs(accountID, organizationID, workspacemodel.WorkspaceStatusNormal).
		WillReturnRows(sqlmock.NewRows([]string{"workspace_id"}))
}
