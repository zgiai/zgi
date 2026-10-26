package migrations

import (
	"os"
	"testing"

	"github.com/google/uuid"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestOneDigitalHumanMemoryPostgresFoldsScopesIntoInternalMemory(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run the one DigitalHuman memory migration PostgreSQL integration")
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
	// Restore the earlier shape so rows in the old scopes can be seeded.
	if err := downOneDigitalHumanMemory(mschema.New(tx).AllowDestructive()); err != nil {
		t.Fatal(err)
	}
	organizationID, workspaceID, live, deleted := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO organizations (id, name) VALUES (?, ?)", organizationID, "One memory")
	exec("INSERT INTO workspaces (id, organization_id, name) VALUES (?, ?, ?)", workspaceID, organizationID, "One memory")
	for _, id := range []uuid.UUID{live, deleted} {
		exec("INSERT INTO digital_humans (id, organization_id, workspace_id, name) VALUES (?, ?, ?, ?)", id, organizationID, workspaceID, "One memory")
	}
	seq := 0
	memory := func(digitalHumanID uuid.UUID, class, scopeType, scopeID, key, visibility string, supersedes *uuid.UUID) uuid.UUID {
		t.Helper()
		id := uuid.New()
		seq++
		exec(`INSERT INTO digital_human_memories (id, organization_id, workspace_id, digital_human_id, memory_key, memory_class, memory_type,
			scope_type, scope_id, content, content_digest, config_revision, last_operation_id, supersedes_id, visibility, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 'fact', ?, ?, ?, 'sha256:x', 'config', ?, ?, ?, now() + make_interval(secs => ?))`,
			id, organizationID, workspaceID, digitalHumanID, key, class, scopeType, scopeID, "content "+key, uuid.New(), supersedes, visibility, seq)
		return id
	}
	channelA, channelB := uuid.NewString(), uuid.NewString()
	public := memory(live, "self", "", "", "pricing", "workspace", nil)
	promoted := memory(live, "self", "", "", "promoted_note", "restricted", nil)
	team := memory(live, "business", "workspace", workspaceID.String(), "pricing", "workspace", nil)
	older := memory(live, "business", "channel", channelA, "standup", "workspace", nil)
	newer := memory(live, "business", "channel", channelA, "standup_v2", "workspace", &older)
	otherGroup := memory(live, "business", "channel", channelB, "standup", "workspace", nil)
	note := memory(live, "business", "instance", uuid.NewString(), "quote_due", "restricted", nil)
	stranded := memory(deleted, "business", "channel", uuid.NewString(), "orphan", "workspace", nil)
	exec("UPDATE digital_humans SET deleted_at = now() WHERE id = ?", deleted)

	if err := upOneDigitalHumanMemory(mschema.New(tx)); err != nil {
		t.Fatalf("up migration: %v", err)
	}
	type row struct {
		MemoryClass     string
		ScopeType       string
		MemoryKey       string
		Visibility      string
		Origin          string
		OriginChannelID string
		Revision        int64
		SupersedesID    *uuid.UUID
	}
	get := func(id uuid.UUID) row {
		t.Helper()
		var got row
		if err := tx.Raw("SELECT memory_class, scope_type, memory_key, visibility, origin, origin_channel_id, revision, supersedes_id FROM digital_human_memories WHERE id = ?", id).Scan(&got).Error; err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := get(public); got.Visibility != "everyone" || got.Origin != "operator" || got.MemoryKey != "pricing" {
		t.Fatalf("public self memory = %+v", got)
	}
	if got := get(promoted); got.Visibility != "internal" {
		t.Fatalf("a promoted note became public: %+v", got)
	}
	if got := get(team); got.MemoryClass != "self" || got.ScopeType != "" || got.Visibility != "internal" || got.Origin != "operator" || got.MemoryKey == "pricing" || got.Revision != 2 {
		t.Fatalf("workspace memory = %+v", got)
	}
	if got := get(older); got.MemoryClass != "self" || got.Visibility != "internal" || got.Origin != "channel" || got.OriginChannelID != channelA || got.MemoryKey != "standup" {
		t.Fatalf("channel memory = %+v", got)
	}
	if got := get(newer); got.MemoryClass != "self" || got.SupersedesID == nil || *got.SupersedesID != older {
		t.Fatalf("supersede chain = %+v", got)
	}
	if got := get(otherGroup); got.OriginChannelID != channelB || got.MemoryKey == "standup" {
		t.Fatalf("the other group's same key = %+v", got)
	}
	if got := get(note); got.MemoryClass != "business" || got.ScopeType != "instance" || got.Visibility != "internal" || got.Origin != "end_user" {
		t.Fatalf("instance note = %+v", got)
	}
	var events int64
	tx.Raw("SELECT count(*) FROM digital_human_memory_events WHERE digital_human_id = ? AND reason_code = 'merged_into_digital_human_memory'", live).Scan(&events)
	if events != 4 {
		t.Fatalf("audit events = %d, want 4", events)
	}

	// The deleted DigitalHuman's row cannot move, so the checks hold new rows only.
	if got := get(stranded); got.ScopeType != "channel" {
		t.Fatalf("stranded row = %+v", got)
	}
	var validated bool
	tx.Raw("SELECT convalidated FROM pg_constraint WHERE conname = 'digital_human_memories_scope_check'").Scan(&validated)
	if validated {
		t.Fatal("the scope check was validated over a stranded row")
	}
	if err := tx.Exec(`INSERT INTO digital_human_memories (organization_id, workspace_id, digital_human_id, memory_key, memory_class, memory_type, scope_type, scope_id, content, content_digest, config_revision, last_operation_id)
		VALUES (?, ?, ?, 'new', 'business', 'fact', 'channel', 'c', 'x', 'sha256:x', 'config', ?)`, organizationID, workspaceID, live, uuid.New()).Error; err == nil {
		t.Fatal("a new row was written to the channel scope")
	}
}
