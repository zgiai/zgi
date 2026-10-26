package migrations

import (
	"os"
	"testing"

	"github.com/google/uuid"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPersonNotesPostgresMoveInstanceMemoryIntoRelationshipMemory(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run the person notes migration PostgreSQL integration")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := RunWithDB(db); err != nil {
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
	count := func(sql string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := tx.Raw(sql, args...).Scan(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	organizationID, workspaceID, digitalHumanID, subjectID, instanceID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO organizations (id, name) VALUES (?, 'Notes')", organizationID)
	exec("INSERT INTO workspaces (id, organization_id, name) VALUES (?, ?, 'Notes')", workspaceID, organizationID)
	exec("INSERT INTO digital_humans (id, organization_id, workspace_id, name) VALUES (?, ?, ?, 'Keeper')", digitalHumanID, organizationID, workspaceID)
	exec("INSERT INTO user_subjects (id, kind, issuer_type, issuer_id) VALUES (?, 'api_end_user', 'digital_human_api', ?)", subjectID, digitalHumanID)
	exec(`INSERT INTO digital_human_end_user_instances (id, organization_id, workspace_id, digital_human_id, subject_kind, subject_id, issuer_type, issuer_id, status, quota_policy, first_seen_at, last_active_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'api_end_user', ?, 'digital_human_api', ?, 'active', '{}', now(), now(), now(), now())`, instanceID, organizationID, workspaceID, digitalHumanID, subjectID, digitalHumanID)
	memory := func(key, memoryType, status string) uuid.UUID {
		t.Helper()
		id := uuid.New()
		exec(`INSERT INTO digital_human_memories (id, organization_id, workspace_id, digital_human_id, memory_key, memory_class, memory_type, scope_type, scope_id,
			content, content_digest, config_revision, last_operation_id, status, visibility, origin)
			VALUES (?, ?, ?, ?, ?, 'business', ?, 'instance', ?, ?, 'sha256:x', 'config', ?, ?, 'internal', 'end_user')`,
			id, organizationID, workspaceID, digitalHumanID, key, memoryType, instanceID.String(), "content "+key, uuid.New(), status)
		exec(`INSERT INTO digital_human_memory_events (operation_id, organization_id, workspace_id, digital_human_id, memory_id, action, result, evidence)
			VALUES (?, ?, ?, ?, ?, 'upsert', 'committed', '[]')`, uuid.New(), organizationID, workspaceID, digitalHumanID, id)
		return id
	}
	memory("quote_due", "open_loop", "active")
	memory("plan/enterprise", "fact", "active")
	memory("old_promise", "fact", "archived")
	exec(`INSERT INTO digital_human_memories (organization_id, workspace_id, digital_human_id, memory_key, memory_class, memory_type, scope_type, scope_id,
		content, content_digest, config_revision, last_operation_id, visibility)
		VALUES (?, ?, ?, 'own', 'self', 'fact', '', '', 'own memory', 'sha256:x', 'config', ?, 'everyone')`, organizationID, workspaceID, digitalHumanID, uuid.New())

	if err := upPersonNotesInRelationshipMemory(mschema.New(tx)); err != nil {
		t.Fatalf("up migration: %v", err)
	}
	if n := count("SELECT count(*) FROM digital_human_memories WHERE digital_human_id = ? AND scope_type = 'instance'", digitalHumanID); n != 0 {
		t.Fatalf("%d instance memories stayed", n)
	}
	if n := count("SELECT count(*) FROM digital_human_memories WHERE digital_human_id = ? AND memory_key = 'own'", digitalHumanID); n != 1 {
		t.Fatal("the DigitalHuman's own memory moved")
	}
	if n := count("SELECT count(*) FROM digital_human_memory_events WHERE digital_human_id = ?", digitalHumanID); n != 0 {
		t.Fatalf("%d events of moved notes stayed", n)
	}
	var keys []string
	if err := tx.Raw(`SELECT item.memory_key || ':' || item.category FROM user_memory_items item JOIN user_memory_relationships relationship ON relationship.id = item.relationship_id
		WHERE relationship.subject_id = ? AND relationship.consumer_id = ? ORDER BY item.memory_key`, subjectID, digitalHumanID).Scan(&keys).Error; err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0] != "notes/plan_enterprise:fact" || keys[1] != "notes/quote_due:open_loop" {
		t.Fatalf("notes = %v", keys)
	}
	if n := count(`SELECT count(*) FROM user_memory_archive_entries entry JOIN user_memory_relationships relationship ON relationship.id = entry.relationship_id
		WHERE relationship.subject_id = ? AND entry.source_key = 'notes/old_promise'`, subjectID); n != 1 {
		t.Fatalf("archived note entries = %d", n)
	}
}
