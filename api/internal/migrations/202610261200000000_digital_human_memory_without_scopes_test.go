package migrations

import (
	"os"
	"testing"

	"github.com/google/uuid"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestDigitalHumanMemoryWithoutScopesPostgresFoldsScopesAndMovesNotes(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run the DigitalHuman memory scope removal PostgreSQL integration")
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
	// Restore the scoped shape so rows in the old scopes can be seeded.
	if err := downDigitalHumanMemoryWithoutScopes(mschema.New(tx).AllowDestructive()); err != nil {
		t.Fatal(err)
	}
	organizationID, workspaceID, live, deleted, subjectID, instanceID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO organizations (id, name) VALUES (?, 'Scopes')", organizationID)
	exec("INSERT INTO workspaces (id, organization_id, name) VALUES (?, ?, 'Scopes')", workspaceID, organizationID)
	for _, id := range []uuid.UUID{live, deleted} {
		exec("INSERT INTO digital_humans (id, organization_id, workspace_id, name) VALUES (?, ?, ?, 'Keeper')", id, organizationID, workspaceID)
	}
	exec("INSERT INTO user_subjects (id, kind, issuer_type, issuer_id) VALUES (?, 'api_end_user', 'digital_human_api', ?)", subjectID, live)
	exec(`INSERT INTO digital_human_end_user_instances (id, organization_id, workspace_id, digital_human_id, subject_kind, subject_id, issuer_type, issuer_id, status, quota_policy, first_seen_at, last_active_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'api_end_user', ?, 'digital_human_api', ?, 'active', '{}', now(), now(), now(), now())`, instanceID, organizationID, workspaceID, live, subjectID, live)
	seq := 0
	memory := func(digitalHumanID uuid.UUID, class, scopeType, scopeID, key, status, visibility string, supersedes *uuid.UUID) uuid.UUID {
		t.Helper()
		id := uuid.New()
		seq++
		exec(`INSERT INTO digital_human_memories (id, organization_id, workspace_id, digital_human_id, memory_key, memory_class, memory_type,
			scope_type, scope_id, content, content_digest, config_revision, last_operation_id, supersedes_id, status, visibility, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 'fact', ?, ?, ?, 'sha256:x', 'config', ?, ?, ?, ?, now() + make_interval(secs => ?))`,
			id, organizationID, workspaceID, digitalHumanID, key, class, scopeType, scopeID, "content "+key, uuid.New(), supersedes, status, visibility, seq)
		exec(`INSERT INTO digital_human_memory_events (operation_id, organization_id, workspace_id, digital_human_id, memory_id, action, result, evidence)
			VALUES (?, ?, ?, ?, ?, 'upsert', 'committed', '[]')`, uuid.New(), organizationID, workspaceID, digitalHumanID, id)
		return id
	}
	instance := instanceID.String()
	channelA, channelB := uuid.NewString(), uuid.NewString()
	public := memory(live, "self", "", "", "pricing", "active", "workspace", nil)
	promoted := memory(live, "self", "", "", "promoted_note", "active", "restricted", nil)
	team := memory(live, "business", "workspace", workspaceID.String(), "pricing", "active", "workspace", nil)
	older := memory(live, "business", "channel", channelA, "standup", "active", "workspace", nil)
	newer := memory(live, "business", "channel", channelA, "standup_v2", "active", "workspace", &older)
	otherGroup := memory(live, "business", "channel", channelB, "standup", "active", "workspace", nil)
	project := memory(live, "business", "project", "p-1", "launch_date", "active", "workspace", nil)
	memory(live, "business", "instance", instance, "quote_due", "active", "restricted", nil)
	memory(live, "business", "instance", instance, "plan/enterprise", "active", "restricted", nil)
	memory(live, "business", "instance", instance, "old_promise", "archived", "restricted", nil)
	memory(live, "business", "instance", instance, "dropped", "invalidated", "restricted", nil)
	orphanNote := memory(live, "business", "instance", uuid.NewString(), "orphan_note", "active", "restricted", nil)
	deletedSelf := memory(deleted, "self", "", "", "orphan", "active", "workspace", nil)
	deletedChannel := memory(deleted, "business", "channel", uuid.NewString(), "orphan", "active", "workspace", nil)
	exec("UPDATE digital_humans SET deleted_at = now() WHERE id = ?", deleted)

	if err := upDigitalHumanMemoryWithoutScopes(mschema.New(tx)); err != nil {
		t.Fatalf("up migration: %v", err)
	}
	// The retired class and scope columns stay behind, the same for every row.
	if n := count("SELECT count(*) FROM digital_human_memories WHERE digital_human_id IN (?, ?) AND (memory_class <> 'self' OR scope_type <> '' OR scope_id <> '')", live, deleted); n != 0 {
		t.Fatalf("%d rows kept a class or scope", n)
	}
	type row struct {
		MemoryKey       string
		Status          string
		Visibility      string
		Origin          string
		OriginChannelID string
		Revision        int64
		SupersedesID    *uuid.UUID
	}
	get := func(id uuid.UUID) row {
		t.Helper()
		var got row
		if err := tx.Raw("SELECT memory_key, status, visibility, origin, origin_channel_id, revision, supersedes_id FROM digital_human_memories WHERE id = ?", id).Scan(&got).Error; err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := get(public); got.Visibility != "everyone" || got.Origin != "operator" || got.MemoryKey != "pricing" {
		t.Fatalf("public memory = %+v", got)
	}
	if got := get(promoted); got.Visibility != "internal" {
		t.Fatalf("a promoted note became public: %+v", got)
	}
	if got := get(team); got.Visibility != "internal" || got.Origin != "operator" || got.MemoryKey == "pricing" || got.Revision != 2 {
		t.Fatalf("workspace memory = %+v", got)
	}
	if got := get(project); got.Visibility != "internal" || got.MemoryKey != "launch_date" {
		t.Fatalf("project memory = %+v", got)
	}
	if got := get(older); got.Visibility != "internal" || got.Origin != "channel" || got.OriginChannelID != channelA || got.MemoryKey != "standup" {
		t.Fatalf("channel memory = %+v", got)
	}
	if got := get(newer); got.SupersedesID == nil || *got.SupersedesID != older {
		t.Fatalf("supersede chain = %+v", got)
	}
	if got := get(otherGroup); got.OriginChannelID != channelB || got.MemoryKey == "standup" {
		t.Fatalf("the other group's same key = %+v", got)
	}
	if n := count("SELECT count(*) FROM digital_human_memory_events WHERE digital_human_id = ? AND reason_code = 'merged_into_digital_human_memory'", live); n != 5 {
		t.Fatalf("audit events = %d, want 5", n)
	}

	// Instance memory moved to the person's relationship memory, with its
	// events; the invalidated row is gone.
	if n := count("SELECT count(*) FROM digital_human_memories WHERE digital_human_id = ? AND memory_key IN ('quote_due', 'plan/enterprise', 'old_promise', 'dropped')", live); n != 0 {
		t.Fatalf("%d instance memories stayed", n)
	}
	if n := count("SELECT count(*) FROM digital_human_memory_events event LEFT JOIN digital_human_memories memory ON memory.id = event.memory_id WHERE event.digital_human_id = ? AND memory.id IS NULL", live); n != 0 {
		t.Fatalf("%d events of moved notes stayed", n)
	}
	var keys []string
	if err := tx.Raw(`SELECT item.memory_key || ':' || item.category FROM user_memory_items item JOIN user_memory_relationships relationship ON relationship.id = item.relationship_id
		WHERE relationship.subject_id = ? AND relationship.consumer_id = ? ORDER BY item.memory_key`, subjectID, live).Scan(&keys).Error; err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0] != "notes/plan_enterprise:fact" || keys[1] != "notes/quote_due:fact" {
		t.Fatalf("notes = %v", keys)
	}
	if n := count(`SELECT count(*) FROM user_memory_archive_entries entry JOIN user_memory_relationships relationship ON relationship.id = entry.relationship_id
		WHERE relationship.subject_id = ? AND entry.source_key = 'notes/old_promise'`, subjectID); n != 1 {
		t.Fatalf("archived note entries = %d", n)
	}

	// Rows that could not move stay as internal entries no one recalls.
	if got := get(orphanNote); got.Status != "invalidated" || got.Visibility != "internal" || got.Origin != "end_user" {
		t.Fatalf("note of a missing instance = %+v", got)
	}
	selfRow, channelRow := get(deletedSelf), get(deletedChannel)
	if selfRow.Visibility != "everyone" || channelRow.Visibility != "internal" || selfRow.MemoryKey == channelRow.MemoryKey {
		t.Fatalf("deleted DigitalHuman rows = %+v %+v", selfRow, channelRow)
	}

	var invalid int64
	tx.Raw("SELECT count(*) FROM pg_constraint WHERE conrelid = 'public.digital_human_memories'::regclass AND NOT convalidated").Scan(&invalid)
	if invalid != 0 {
		t.Fatalf("%d memory checks hold new rows only", invalid)
	}
	if err := tx.Exec(`SAVEPOINT duplicate_key`).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(`INSERT INTO digital_human_memories (organization_id, workspace_id, digital_human_id, memory_key, memory_type, content, content_digest, config_revision, last_operation_id)
		VALUES (?, ?, ?, 'pricing', 'fact', 'x', 'sha256:x', 'config', ?)`, organizationID, workspaceID, live, uuid.New()).Error; err == nil {
		t.Fatal("a second entry was written under a used key")
	}
	exec("ROLLBACK TO SAVEPOINT duplicate_key")
}
