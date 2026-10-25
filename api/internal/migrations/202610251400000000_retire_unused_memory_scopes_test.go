package migrations

import (
	"crypto/md5"
	"encoding/hex"
	"os"
	"testing"

	"github.com/google/uuid"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRetireUnusedMemoryScopesPostgresMovesRowsToTheWorkspace(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run the memory scope migration PostgreSQL integration")
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
	// Restore the wide check so rows in the retired scopes can be seeded.
	if err := downRetireUnusedMemoryScopes(mschema.New(tx)); err != nil {
		t.Fatal(err)
	}
	organizationID, workspaceID, live, deleted := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO organizations (id, name) VALUES (?, ?)", organizationID, "Scope migration")
	exec("INSERT INTO workspaces (id, organization_id, name) VALUES (?, ?, ?)", workspaceID, organizationID, "Scope migration")
	for _, id := range []uuid.UUID{live, deleted} {
		exec("INSERT INTO digital_humans (id, organization_id, workspace_id, name) VALUES (?, ?, ?, ?)", id, organizationID, workspaceID, "Scope migration")
	}
	seq := 0
	memory := func(digitalHumanID uuid.UUID, scopeType, scopeID, key string, supersedes *uuid.UUID) uuid.UUID {
		t.Helper()
		id := uuid.New()
		seq++
		exec(`INSERT INTO digital_human_memories (id, organization_id, workspace_id, digital_human_id, memory_key, memory_class, memory_type,
			scope_type, scope_id, content, content_digest, config_revision, last_operation_id, supersedes_id, created_at)
			VALUES (?, ?, ?, ?, ?, 'business', 'fact', ?, ?, ?, 'sha256:x', 'config', ?, ?, now() + make_interval(secs => ?))`,
			id, organizationID, workspaceID, digitalHumanID, key, scopeType, scopeID, "content "+key, uuid.New(), supersedes, seq)
		return id
	}
	memory(live, "workspace", workspaceID.String(), "pricing", nil)
	collided := memory(live, "project", "p-1", "pricing", nil)
	older := memory(live, "customer", "c-1", "contact_window", nil)
	newer := memory(live, "customer", "c-1", "contact_window_v2", &older)
	memory(live, "campaign", "g-1", "launch_date", nil)
	stranded := memory(deleted, "project", "p-2", "orphan", nil)
	exec("UPDATE digital_humans SET deleted_at = now() WHERE id = ?", deleted)

	if err := upRetireUnusedMemoryScopes(mschema.New(tx)); err != nil {
		t.Fatalf("up migration: %v", err)
	}
	type row struct {
		ID           uuid.UUID
		ScopeType    string
		ScopeID      string
		MemoryKey    string
		Revision     int64
		SupersedesID *uuid.UUID
	}
	var rows []row
	if err := tx.Raw("SELECT id, scope_type, scope_id, memory_key, revision, supersedes_id FROM digital_human_memories WHERE digital_human_id = ? ORDER BY created_at", live).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	digest := md5.Sum([]byte("p-1"))
	wantKeys := []string{"pricing", "pricing-project-" + hex.EncodeToString(digest[:])[:8], "contact_window", "contact_window_v2", "launch_date"}
	for index, got := range rows {
		if got.ScopeType != "workspace" || got.ScopeID != workspaceID.String() || got.MemoryKey != wantKeys[index] {
			t.Fatalf("row %d = %+v, want workspace key %s", index, got, wantKeys[index])
		}
		if index > 0 && got.Revision != 2 {
			t.Fatalf("moved row %d revision = %d", index, got.Revision)
		}
	}
	if rows[3].ID != newer || rows[3].SupersedesID == nil || *rows[3].SupersedesID != older || rows[1].ID != collided {
		t.Fatalf("supersede chain or ids changed: %+v", rows)
	}
	var events int64
	tx.Raw("SELECT count(*) FROM digital_human_memory_events WHERE digital_human_id = ? AND reason_code = 'scope_retired_to_workspace' AND before_snapshot->>'scope_type' <> 'workspace' AND after_snapshot->>'scope_type' = 'workspace'", live).Scan(&events)
	if events != 4 {
		t.Fatalf("audit events = %d, want 4", events)
	}

	// A deleted DigitalHuman's row cannot move, so the check holds new rows
	// only; once it is gone the check is validated.
	var scopeType string
	var validated bool
	tx.Raw("SELECT scope_type FROM digital_human_memories WHERE id = ?", stranded).Scan(&scopeType)
	tx.Raw("SELECT convalidated FROM pg_constraint WHERE conname = 'digital_human_memories_scope_check'").Scan(&validated)
	if scopeType != "project" || validated {
		t.Fatalf("stranded row scope = %q, check validated = %v", scopeType, validated)
	}
	if err := tx.Exec("INSERT INTO digital_human_memories (organization_id, workspace_id, digital_human_id, memory_key, memory_class, memory_type, scope_type, scope_id, content, content_digest, config_revision, last_operation_id) VALUES (?, ?, ?, 'new', 'business', 'fact', 'project', 'p-3', 'x', 'sha256:x', 'config', ?)",
		organizationID, workspaceID, live, uuid.New()).Error; err == nil {
		t.Fatal("a new row was written to a retired scope")
	}
}
