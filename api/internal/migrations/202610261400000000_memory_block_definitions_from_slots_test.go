package migrations

import (
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestMemoryBlockDefinitionsFromSlotsCopiesTheSlots(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run the memory block definition PostgreSQL integration")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := RunWithDB(db); err != nil {
		t.Fatalf("apply repository migrations: %v", err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })
	// Slots reference an Agent; this rolled-back transaction skips
	// foreign-key checks instead of building one.
	if err := tx.Exec("SET LOCAL session_replication_role = replica").Error; err != nil {
		t.Skipf("cannot relax foreign keys for the slot fixture: %v", err)
	}
	workspaceID, agentID, now := uuid.New(), uuid.New(), time.Now().UTC()
	slot := func(key string, enabled, readOnly, manualOnly bool) {
		t.Helper()
		if err := tx.Exec(`INSERT INTO agent_memory_slots (id, workspace_id, agent_id, key, name, description, max_chars, enabled, sort_order, read_only, manual_only, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, 'From the slot.', 700, ?, 3, ?, ?, ?, ?)`, uuid.New(), workspaceID, agentID, key, "Name "+key, enabled, readOnly, manualOnly, now, now).Error; err != nil {
			t.Fatal(err)
		}
	}
	slot("manual", false, false, true)
	slot("missing", true, true, false)
	// The mirror stored "manual" with the column defaults.
	definitionID := uuid.New()
	if err := tx.Exec(`INSERT INTO memory_block_definitions (id, consumer_type, consumer_id, label, name, description, limit_chars, partition, owner, enabled, auto_write, created_at, updated_at)
		VALUES (?, 'agent', ?, 'manual', 'Name manual', 'From the slot.', 700, 'person', 'user', true, true, ?, ?)`, definitionID, agentID, now, now).Error; err != nil {
		t.Fatal(err)
	}

	for run := range 2 {
		if err := upMemoryBlockDefinitionsFromSlots(mschema.New(tx)); err != nil {
			t.Fatalf("run %d: %v", run+1, err)
		}
	}
	type definition struct {
		ID                           uuid.UUID
		Label, Name                  string
		LimitChars, SortOrder        int
		Enabled, ReadOnly, AutoWrite bool
	}
	var rows []definition
	if err := tx.Raw(`SELECT id, label, name, limit_chars, sort_order, enabled, read_only, auto_write FROM memory_block_definitions
		WHERE consumer_id = ? AND deleted_at IS NULL ORDER BY label`, agentID).Scan(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("definitions = %+v", rows)
	}
	manual, missing := rows[0], rows[1]
	if manual.ID != definitionID || manual.Enabled || manual.AutoWrite || manual.SortOrder != 3 {
		t.Fatalf("manual = %+v, want the slot's disabled manual-only block under the same ID", manual)
	}
	if missing.Label != "missing" || !missing.ReadOnly || !missing.AutoWrite || missing.LimitChars != 700 || missing.Name != "Name missing" {
		t.Fatalf("missing = %+v, want a definition created from the slot", missing)
	}
}
