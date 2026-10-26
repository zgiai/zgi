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

func TestPurgeReasonsOfPurgedMemoryVersionsClearsOnlyPurgedRows(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run the memory version purge PostgreSQL integration")
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
	// Versions reference a commit and a relationship; this rolled-back
	// transaction skips foreign-key checks instead of building them.
	if err := tx.Exec("SET LOCAL session_replication_role = replica").Error; err != nil {
		t.Skipf("cannot relax foreign keys for the version fixture: %v", err)
	}
	now := time.Now().UTC()
	version := func(purgedAt *time.Time) uuid.UUID {
		t.Helper()
		id := uuid.New()
		if err := tx.Exec(`INSERT INTO memory_file_versions (id, commit_id, relationship_id, memory_key, action, before_payload, after_payload,
				before_display_name, evidence, reason, content_purged_at, created_at)
			VALUES (?, ?, ?, 'profile', 'update', '', '', 'Phone 13800001111', 'my phone is 13800001111', 'the user said their phone is 13800001111', ?, ?)`,
			id, uuid.New(), uuid.New(), purgedAt, now).Error; err != nil {
			t.Fatal(err)
		}
		return id
	}
	purged, kept := version(&now), version(nil)

	for run := range 2 {
		if err := upPurgeReasonsOfPurgedMemoryVersions(mschema.New(tx)); err != nil {
			t.Fatalf("run %d: %v", run+1, err)
		}
	}
	type row struct{ Evidence, Reason, BeforeDisplayName string }
	read := func(id uuid.UUID) row {
		t.Helper()
		var out row
		if err := tx.Raw(`SELECT evidence, reason, before_display_name FROM memory_file_versions WHERE id = ?`, id).Scan(&out).Error; err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got := read(purged); got != (row{}) {
		t.Fatalf("purged version still quotes %+v", got)
	}
	if got := read(kept); got.Reason == "" || got.Evidence == "" || got.BeforeDisplayName == "" {
		t.Fatalf("a version that was not purged lost its text: %+v", got)
	}
}
