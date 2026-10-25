package relationshipmemory

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/migrations"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestInstanceNotesPostgresListOnlyThePersonsActiveNotes(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run instance notes PostgreSQL integration")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.RunWithDB(db); err != nil {
		t.Fatalf("apply repository migrations: %v", err)
	}
	tx := db.Begin()
	t.Cleanup(func() { _ = tx.Rollback().Error })
	exec := func(sql string, args ...any) {
		t.Helper()
		if err := tx.Exec(sql, args...).Error; err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	organizationID, workspaceID, digitalHumanID := uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO organizations (id, name) VALUES (?, ?)", organizationID, "Instance notes")
	exec("INSERT INTO workspaces (id, organization_id, name) VALUES (?, ?, ?)", workspaceID, organizationID, "Instance notes")
	exec("INSERT INTO digital_humans (id, organization_id, workspace_id, name) VALUES (?, ?, ?, ?)", digitalHumanID, organizationID, workspaceID, "Notes")
	person, other := uuid.New(), uuid.New()
	instances := map[uuid.UUID]uuid.UUID{}
	for _, subjectID := range []uuid.UUID{person, other} {
		instanceID := uuid.New()
		instances[subjectID] = instanceID
		exec("INSERT INTO user_subjects (id, kind, issuer_type, issuer_id) VALUES (?, 'api_end_user', 'digital_human_api', ?)", subjectID, digitalHumanID)
		exec(`INSERT INTO digital_human_end_user_instances (id, organization_id, workspace_id, digital_human_id, subject_kind, subject_id, issuer_type, first_seen_at, last_active_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'api_end_user', ?, 'digital_human_api', now(), now(), now(), now())`, instanceID, organizationID, workspaceID, digitalHumanID, subjectID)
	}
	note := func(subjectID uuid.UUID, key, memoryType, status, content string, age int) {
		t.Helper()
		exec(`INSERT INTO digital_human_memories (organization_id, workspace_id, digital_human_id, memory_key, memory_class, memory_type, scope_type, scope_id, content, content_digest, config_revision, last_operation_id, status, updated_at)
			VALUES (?, ?, ?, ?, 'business', ?, 'instance', ?, ?, 'sha256:x', 'config', ?, ?, now() - make_interval(mins => ?))`,
			organizationID, workspaceID, digitalHumanID, key, memoryType, instances[subjectID].String(), content, uuid.New(), status, age)
	}
	note(person, "quote_due", "open_loop", "active", "Promised a quote by Friday.", 10)
	note(person, "plan/enterprise", "fact", "active", "Evaluating the Enterprise plan.", 1)
	note(person, "old_note", "fact", "archived", "Expired.", 20)
	note(other, "someone_else", "fact", "active", "Another person's note.", 1)

	notes, err := NewService(tx).InstanceNotes(t.Context(), workspaceID, digitalHumanID, person)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 || notes[0].Path != "/user/notes/plan_enterprise.md" || notes[1].Path != "/user/notes/quote_due.md" || notes[1].Tags != "open_loop" {
		t.Fatalf("notes = %+v", notes)
	}
	if none, err := NewService(tx).InstanceNotes(t.Context(), workspaceID, uuid.New(), person); err != nil || len(none) != 0 {
		t.Fatalf("another DigitalHuman's notes = %+v, %v", none, err)
	}
}
