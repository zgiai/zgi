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

func TestAgentAutoCaptureFollowsAgentTurnsOnOnlyUntouchedAgentRelationships(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run the Agent auto-capture PostgreSQL integration")
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
	// Relationships reference an organization, workspace and subject; this
	// rolled-back transaction skips foreign-key checks instead of building them.
	if err := tx.Exec("SET LOCAL session_replication_role = replica").Error; err != nil {
		t.Skipf("cannot relax foreign keys for the relationship fixture: %v", err)
	}
	now := time.Now().UTC()
	relationship := func(consumerType string, memoryEnabled bool, revision int64) uuid.UUID {
		t.Helper()
		id := uuid.New()
		if err := tx.Exec(`INSERT INTO user_memory_relationships (id, organization_id, workspace_id, subject_id, consumer_type, consumer_id,
				consumer_name_snapshot, status, memory_enabled, auto_capture_enabled, settings_revision, memory_epoch, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, 'assistant', 'active', ?, FALSE, ?, 0, ?, ?)`,
			id, uuid.New(), uuid.New(), uuid.New(), consumerType, uuid.New(), memoryEnabled, revision, now, now).Error; err != nil {
			t.Fatal(err)
		}
		return id
	}
	untouched := relationship("agent", true, 1)
	chosenOff := relationship("agent", true, 2)
	memoryOff := relationship("agent", false, 1)
	digitalHuman := relationship("digital_human", true, 1)

	for run := range 2 {
		if err := upAgentAutoCaptureFollowsAgent(mschema.New(tx)); err != nil {
			t.Fatalf("run %d: %v", run+1, err)
		}
	}
	type row struct {
		AutoCaptureEnabled bool
		SettingsRevision   int64
	}
	read := func(id uuid.UUID) row {
		t.Helper()
		var out row
		if err := tx.Raw(`SELECT auto_capture_enabled, settings_revision FROM user_memory_relationships WHERE id = ?`, id).Scan(&out).Error; err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got := read(untouched); !got.AutoCaptureEnabled || got.SettingsRevision != 2 {
		t.Fatalf("untouched Agent relationship = %+v, want automatic memory on at revision 2", got)
	}
	for name, id := range map[string]uuid.UUID{"chosen off": chosenOff, "memory off": memoryOff, "digital human": digitalHuman} {
		if got := read(id); got.AutoCaptureEnabled {
			t.Fatalf("%s relationship turned on: %+v", name, got)
		}
	}
}
