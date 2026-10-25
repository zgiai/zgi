package migrations

import (
	"strings"
	"testing"
	"time"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/migrations/connectorfinalize"
	"gorm.io/gorm"
)

func TestConnectorPendingInventoryPostgres(t *testing.T) {
	// Exercise normal retirement without loading local deployment configuration.
	previous := config.GlobalConfig
	config.GlobalConfig = &config.Config{}
	t.Cleanup(func() { config.GlobalConfig = previous })
	const pendingMigration = "20260904160000_connector_pending_setups"
	const pendingTable = "connector_pending_setups"
	checkpointDatabase := func(t *testing.T) *gorm.DB {
		t.Helper()
		db := finalizationTestDatabase(t)
		if err := gormigrate.New(db, migrationOptions(true), allMigrations()).MigrateTo(connectorfinalize.CheckpointID); err != nil {
			t.Fatal(err)
		}
		return db
	}
	hasPendingProof := func(report connectorfinalize.Report) bool {
		for _, proof := range report.Native {
			if proof.Table == pendingTable {
				return true
			}
		}
		return false
	}

	t.Run("checkpoint_apply_verify_and_later_creation", func(t *testing.T) {
		db := checkpointDatabase(t)
		plan, err := connectorfinalize.Plan(t.Context(), db)
		if err != nil {
			t.Fatal(err)
		}
		if hasPendingProof(plan) {
			t.Fatal("not-yet-created table received a native proof")
		}
		if _, err := connectorfinalize.Apply(t.Context(), db, connectorfinalize.ApplyOptions{ExpectedReport: plan.Digest}); err != nil {
			t.Fatal(err)
		}
		verified, err := connectorfinalize.Verify(t.Context(), db, connectorfinalize.Options{})
		if err != nil || hasPendingProof(verified) {
			t.Fatalf("verify before pending-setups creation: %v", err)
		}
		if err := RunWithDB(db); err != nil {
			t.Fatal(err)
		}
		assertMigrationRecorded(t, db, pendingMigration)
		now := time.Now().UTC()
		if err := db.Table(pendingTable).Create(map[string]any{
			"id": uuid.New(), "organization_id": uuid.New(), "owner_account_id": uuid.New(),
			"connector_id": "feishu", "region": "feishu", "app_id": "inventory_test",
			"source_flow_id": uuid.New(), "template_id": "test", "template_digest": strings.Repeat("a", 64),
			"accepted_definition_digest": strings.Repeat("b", 64), "state": "verification_required",
			"resume_expires_at": now.Add(time.Hour), "created_at": now, "updated_at": now,
		}).Error; err != nil {
			t.Fatal(err)
		}
		before, err := connectorfinalize.Verify(t.Context(), db, connectorfinalize.Options{})
		if err != nil || !hasPendingProof(before) {
			t.Fatalf("created pending-setups table was not verified: %v", err)
		}
		if err := RunWithDB(db); err != nil {
			t.Fatal(err)
		}
		after, err := connectorfinalize.Verify(t.Context(), db, connectorfinalize.Options{})
		if err != nil || !hasPendingProof(after) {
			t.Fatalf("repeat omitted the pending-setups inventory: %v", err)
		}
		for _, proof := range before.Native {
			if proof.Table != pendingTable {
				continue
			}
			if proof.Rows != 1 {
				t.Fatalf("pending row was not inventoried: %+v", proof)
			}
			for _, repeated := range after.Native {
				if repeated.Table == pendingTable && repeated != proof {
					t.Fatal("repeat migration changed protected pending setup data")
				}
			}
		}
	})

	for _, test := range []struct {
		name, statement string
	}{
		{"marker_without_table", "INSERT INTO public.migrations (id) VALUES ('" + pendingMigration + "')"},
		{"table_without_marker", "CREATE TABLE public.connector_pending_setups (id uuid PRIMARY KEY)"},
		{"other_native_table_missing", "ALTER TABLE public.connector_chat_preferences RENAME TO connector_chat_preferences_missing"},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := checkpointDatabase(t)
			plan, err := connectorfinalize.Plan(t.Context(), db)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Exec(test.statement).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := connectorfinalize.Plan(t.Context(), db); err == nil {
				t.Fatal("plan accepted inconsistent inventory")
			}
			if _, err := connectorfinalize.Apply(t.Context(), db, connectorfinalize.ApplyOptions{ExpectedReport: plan.Digest}); err == nil {
				t.Fatal("apply accepted inconsistent inventory")
			}
			if !db.Migrator().HasTable("integration_connections") {
				t.Fatal("rejected finalization dropped a legacy table")
			}
		})
	}

	t.Run("verify_rejects_missing_previously_created_table", func(t *testing.T) {
		db := finalizationTestDatabase(t)
		if err := RunWithDB(db); err != nil {
			t.Fatal(err)
		}
		if err := db.Exec("ALTER TABLE public.connector_pending_setups RENAME TO connector_pending_setups_missing").Error; err != nil {
			t.Fatal(err)
		}
		if _, err := connectorfinalize.Verify(t.Context(), db, connectorfinalize.Options{}); err == nil {
			t.Fatal("verify accepted a missing previously created table")
		}
	})
}
