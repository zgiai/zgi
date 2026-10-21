package digitalhumantask

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestTaskVisibilityPostgresReadRulesOccupancyAndConstraints(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run DigitalHuman task visibility PostgreSQL integration")
	}
	t.Setenv("ZGI_DIGITAL_HUMAN_TASK_VISIBILITY_ENABLED", "true")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })

	organizationID, workspaceID, digitalHumanID := uuid.New(), uuid.New(), uuid.New()
	accountA, accountB := uuid.New(), uuid.New()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO organizations (id, name) VALUES (?, ?)", []any{organizationID, "Task visibility integration"}},
		{"INSERT INTO workspaces (id, organization_id, name) VALUES (?, ?, ?)", []any{workspaceID, organizationID, "Task visibility integration"}},
		{"INSERT INTO accounts (id, name, email) VALUES (?, ?, ?)", []any{accountA, "Requester A", "visibility-" + accountA.String() + "@example.invalid"}},
		{"INSERT INTO accounts (id, name, email) VALUES (?, ?, ?)", []any{accountB, "Requester B", "visibility-" + accountB.String() + "@example.invalid"}},
		{`INSERT INTO digital_humans (id, organization_id, workspace_id, name, created_by, updated_by) VALUES (?, ?, ?, ?, ?, ?)`, []any{digitalHumanID, organizationID, workspaceID, "Task visibility integration", accountA, accountA}},
	} {
		if err := tx.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(tx, &recordingDispatcher{}, nil)
	shared := digitalhuman.TaskAccessContext{
		OrganizationID: organizationID, WorkspaceID: workspaceID, DigitalHumanID: digitalHumanID,
		Principal: invocation.PrincipalRef{Type: invocation.PrincipalAccount, ID: accountA.String()},
		Owner:     digitalhuman.SharedTaskOwner(), Surface: digitalhuman.SurfaceConsole,
	}
	fixture := newVisibilityFixtureFor(t, service, shared, accountA, accountB)

	assertNames(t, "task center", listNames(t, service, fixture.taskCenter), "shared")
	assertNames(t, "task center manager", listNames(t, service, fixture.manager), "shared", "private A", "private B", "conversation")
	assertNames(t, "member A", listNames(t, service, fixture.memberA), "shared", "private A")
	assertNames(t, "channel", listNames(t, service, fixture.channel), "conversation")
	assertNames(t, "external", listNames(t, service, fixture.external), "private A")

	// Rows written by a build without visibility support resolve from owner_kind.
	if err := tx.Exec("UPDATE automation_tasks SET visibility = NULL WHERE id IN (?, ?)", fixture.shared.ID, fixture.privateB.ID).Error; err != nil {
		t.Fatal(err)
	}
	assertNames(t, "member A over legacy rows", listNames(t, service, fixture.memberA), "shared", "private A")

	occupancy, err := service.ListTaskOccupancy(t.Context(), fixture.memberA, 0)
	if err != nil || len(occupancy) != 2 {
		t.Fatalf("member A occupancy = %#v, %v", occupancy, err)
	}
	for _, item := range occupancy {
		if item.NextRunAt == nil || item.NextRunAt.Before(time.Now()) {
			t.Fatalf("occupancy item = %#v", item)
		}
	}

	for name, statement := range map[string]string{
		"unknown visibility":                      "UPDATE automation_tasks SET visibility = 'everyone' WHERE id = '" + fixture.shared.ID.String() + "'",
		"generic automation task with visibility": "UPDATE automation_tasks SET task_kind = 'automation', owner_kind = NULL, owner_subject_kind = NULL, owner_subject_id = NULL, owner_conversation_id = NULL, origin_surface = NULL, subject_type = NULL, subject_id = NULL WHERE id = '" + fixture.conversation.ID.String() + "'",
	} {
		savepoint := "visibility_check"
		if err := tx.SavePoint(savepoint).Error; err != nil {
			t.Fatal(err)
		}
		if err := tx.Exec(statement).Error; err == nil {
			t.Fatalf("%s was admitted", name)
		}
		if err := tx.RollbackTo(savepoint).Error; err != nil {
			t.Fatal(err)
		}
	}
}
