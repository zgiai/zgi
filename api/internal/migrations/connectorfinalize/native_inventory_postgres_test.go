package connectorfinalize

import (
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestNativeInventoryPostgresMatrix proves the four creation-marker/relation
// combinations against a real PostgreSQL catalog. It requires an explicitly
// isolated database and rolls back every change.
func TestNativeInventoryPostgresMatrix(t *testing.T) {
	dsn := os.Getenv("ZGI_CONNECTOR_FINALIZE_TEST_DSN")
	if dsn == "" {
		t.Skip("requires explicit isolated ZGI_CONNECTOR_FINALIZE_TEST_DSN")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("open isolated PostgreSQL database")
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()

	if err := tx.Exec(`CREATE TABLE IF NOT EXISTS public.migrations (id text PRIMARY KEY)`).Error; err != nil {
		t.Fatal(err)
	}
	setMarker := func(applied bool) {
		t.Helper()
		if applied {
			if err := tx.Exec(`INSERT INTO public.migrations (id) VALUES (?) ON CONFLICT (id) DO NOTHING`, pendingSetupsMigration).Error; err != nil {
				t.Fatal(err)
			}
			return
		}
		if err := tx.Exec(`DELETE FROM public.migrations WHERE id = ?`, pendingSetupsMigration).Error; err != nil {
			t.Fatal(err)
		}
	}
	setRelation := func(exists bool) {
		t.Helper()
		if exists {
			if err := tx.Exec(`CREATE TABLE IF NOT EXISTS public.connector_pending_setups (id uuid PRIMARY KEY)`).Error; err != nil {
				t.Fatal(err)
			}
			return
		}
		if err := tx.Exec(`DROP TABLE IF EXISTS public.connector_pending_setups CASCADE`).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, testCase := range []struct {
		name           string
		markerApplied  bool
		relationExists bool
		wantDeferred   bool
		wantErr        bool
	}{
		{name: "creator_not_applied_and_relation_absent", markerApplied: false, relationExists: false, wantDeferred: true},
		{name: "creator_applied_but_relation_missing", markerApplied: true, relationExists: false, wantErr: true},
		{name: "creator_applied_and_relation_present", markerApplied: true, relationExists: true, wantDeferred: false},
		{name: "relation_present_but_creator_not_applied", markerApplied: false, relationExists: true, wantErr: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			setMarker(testCase.markerApplied)
			setRelation(testCase.relationExists)
			tables, deferred, err := resolveNativeInventory(tx)
			if testCase.wantErr {
				if err == nil || tables != nil || deferred != nil {
					t.Fatalf("resolve = (%v, %v, %v), want a fail-closed error", tables, deferred, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if (len(deferred) == 1 && deferred[0] == pendingSetupsTable) != testCase.wantDeferred {
				t.Fatalf("deferred = %v, want %v", deferred, testCase.wantDeferred)
			}
			if len(tables)+len(deferred) != len(NativeTables()) {
				t.Fatalf("inventory incomplete: tables=%d deferred=%d", len(tables), len(deferred))
			}
		})
	}
}
