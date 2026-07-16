package workflow

import (
	"context"
	"database/sql/driver"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestBuiltInWorkflowScenarioDetailHonorsBuiltinAppAccountGrant(t *testing.T) {
	ctx := context.Background()
	db, mock, cleanup := openBuiltInRuntimeAuthMockDB(t)
	defer cleanup()

	organizationID := uuid.New()
	accountID := uuid.New()
	otherAccountID := uuid.New()
	agentID := uuid.New()
	surfaceID := uuid.New()
	workflow := dto.BuiltInWorkflowDTO{
		Scenario:   "global_chat",
		AgentID:    agentID,
		AgentName:  "Global chat",
		WorkflowID: uuid.New(),
		WebAppID:   uuid.New(),
	}
	repo := &builtInWorkflowRuntimeAuthRepo{workflow: workflow}
	service := NewBuiltInWorkflowService(repo, runtimeauth.NewStore(db), nil)
	audience := runtimeauth.RuntimeAudience{
		OrganizationID: organizationID,
		AccountID:      accountID,
	}

	expectBuiltInRuntimeAuthCandidateLookup(mock, organizationID, []uuid.UUID{agentID}, []builtInRuntimeCandidateSurfaceRow{{
		resourceID: agentID,
		id:         surfaceID,
		surface:    runtimeauth.PublishedRuntimeSurfaceBuiltinApp,
		enabled:    true,
		source:     runtimeauth.PublishedRuntimeSourceGrant,
	}}, []builtInRuntimeGrantRow{{
		surfaceID:   surfaceID,
		subjectType: runtimeauth.PublishedRuntimeSubjectAccount,
		subjectID:   &otherAccountID,
		enabled:     true,
	}})
	workflows, err := service.GetAllBuiltInWorkflows(ctx, audience)
	if err != nil {
		t.Fatalf("GetAllBuiltInWorkflows error = %v", err)
	}
	if len(workflows) != 0 {
		t.Fatalf("visible workflows = %d, want 0", len(workflows))
	}

	expectBuiltInRuntimeAuthLookup(mock, agentID, organizationID, surfaceID, otherAccountID)
	detail, err := service.GetBuiltInWorkflowByScenario(ctx, workflow.Scenario, audience)
	if err == nil {
		t.Fatalf("GetBuiltInWorkflowByScenario error = nil, want not enabled; detail=%#v", detail)
	}
	if detail != nil {
		t.Fatalf("detail = %#v, want nil", detail)
	}
	if !contains(err.Error(), "not enabled") {
		t.Fatalf("error = %v, want not enabled", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

type builtInRuntimeCandidateSurfaceRow struct {
	resourceID uuid.UUID
	id         uuid.UUID
	surface    runtimeauth.PublishedRuntimeSurface
	enabled    bool
	source     string
}

type builtInRuntimeGrantRow struct {
	surfaceID   uuid.UUID
	subjectType runtimeauth.PublishedRuntimeSubjectType
	subjectID   *uuid.UUID
	enabled     bool
}

type builtInWorkflowRuntimeAuthRepo struct {
	workflow dto.BuiltInWorkflowDTO
}

func (r *builtInWorkflowRuntimeAuthRepo) GetAllBuiltInWorkflows(context.Context) ([]dto.BuiltInWorkflowDTO, error) {
	return []dto.BuiltInWorkflowDTO{r.workflow}, nil
}

func (r *builtInWorkflowRuntimeAuthRepo) GetBuiltInWorkflowByID(_ context.Context, id uuid.UUID) (*dto.BuiltInWorkflowDTO, error) {
	if r.workflow.AgentID == id {
		return &r.workflow, nil
	}
	return nil, errWorkflowRunNotFoundOrDenied
}

func (r *builtInWorkflowRuntimeAuthRepo) GetBuiltInWorkflowByScenario(_ context.Context, scenario string) (*dto.BuiltInWorkflowDTO, error) {
	if r.workflow.Scenario == scenario {
		return &r.workflow, nil
	}
	return nil, errWorkflowRunNotFoundOrDenied
}

func openBuiltInRuntimeAuthMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("open sqlmock: %v", err)
	}
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		sqlDB.Close()
		t.Fatalf("open gorm sqlmock: %v", err)
	}

	return db, mock, func() { _ = sqlDB.Close() }
}

func expectBuiltInRuntimeAuthCandidateLookup(mock sqlmock.Sqlmock, organizationID uuid.UUID, resourceIDs []uuid.UUID, surfaces []builtInRuntimeCandidateSurfaceRow, grants []builtInRuntimeGrantRow) {
	now := time.Now()
	rows := sqlmock.NewRows([]string{
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
	})
	for _, surface := range surfaces {
		rows.AddRow(
			surface.id,
			string(runtimeauth.PublishedRuntimeResourceBuiltinWorkflow),
			surface.resourceID,
			organizationID,
			nil,
			string(surface.surface),
			surface.enabled,
			surface.source,
			now,
			now,
			nil,
		)
	}

	args := make([]driver.Value, 0, 3+len(resourceIDs))
	args = append(args, string(runtimeauth.PublishedRuntimeResourceBuiltinWorkflow), string(runtimeauth.PublishedRuntimeSurfaceBuiltinApp), organizationID)
	for _, resourceID := range resourceIDs {
		args = append(args, resourceID)
	}
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surfaces" WHERE resource_type = \$1 AND surface = \$2 AND organization_id = \$3 AND resource_id IN \(.+\) AND deleted_at IS NULL ORDER BY resource_id ASC`).
		WithArgs(args...).
		WillReturnRows(rows)
	if len(surfaces) == 0 {
		return
	}

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
	for _, grant := range grants {
		var subjectID interface{}
		if grant.subjectID != nil {
			subjectID = grant.subjectID
		}
		grantRows.AddRow(
			uuid.New(),
			grant.surfaceID,
			string(grant.subjectType),
			subjectID,
			grant.enabled,
			now,
			now,
			nil,
		)
	}
	grantArgs := make([]driver.Value, 0, len(surfaces))
	for _, surface := range surfaces {
		grantArgs = append(grantArgs, surface.id)
	}
	mock.ExpectQuery(`SELECT \* FROM "published_runtime_surface_grants" WHERE surface_id IN \(.+\) AND deleted_at IS NULL ORDER BY subject_type ASC, subject_id ASC, created_at ASC`).
		WithArgs(grantArgs...).
		WillReturnRows(grantRows)
}

func expectBuiltInRuntimeAuthLookup(mock sqlmock.Sqlmock, agentID, organizationID, surfaceID, grantAccountID uuid.UUID) {
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "published_runtime_surfaces" WHERE resource_type = $1 AND resource_id = $2 AND deleted_at IS NULL ORDER BY surface ASC`)).
		WithArgs(string(runtimeauth.PublishedRuntimeResourceBuiltinWorkflow), agentID).
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
			surfaceID,
			string(runtimeauth.PublishedRuntimeResourceBuiltinWorkflow),
			agentID,
			organizationID,
			nil,
			string(runtimeauth.PublishedRuntimeSurfaceBuiltinApp),
			true,
			runtimeauth.PublishedRuntimeSourceGrant,
			now,
			now,
			nil,
		))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "published_runtime_surface_grants" WHERE surface_id IN ($1) AND deleted_at IS NULL ORDER BY subject_type ASC, subject_id ASC, created_at ASC`)).
		WithArgs(surfaceID).
		WillReturnRows(sqlmock.NewRows([]string{
			"id",
			"surface_id",
			"subject_type",
			"subject_id",
			"enabled",
			"created_at",
			"updated_at",
			"deleted_at",
		}).AddRow(
			uuid.New(),
			surfaceID,
			string(runtimeauth.PublishedRuntimeSubjectAccount),
			grantAccountID,
			true,
			now,
			now,
			nil,
		))
}
