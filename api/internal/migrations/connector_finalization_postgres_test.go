package migrations

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/zgiai/zgi/api/internal/migrations/connectorfinalize"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func finalizationTestDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("ZGI_CONNECTOR_FINALIZE_TEST_DSN")
	if dsn == "" {
		t.Skip("requires explicit isolated ZGI_CONNECTOR_FINALIZE_TEST_DSN")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid isolated test DSN")
	}
	if !strings.HasPrefix(config.Database, "connector_finalize_test_") {
		t.Fatal("finalization matrix requires a dedicated connector_finalize_test_ database")
	}
	admin, err := pgx.ConnectConfig(context.Background(), config)
	if err != nil {
		t.Fatal("connect isolated test database")
	}
	name := "connector_finalize_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(context.Background(), "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()+" TEMPLATE template0"); err != nil {
		admin.Close(context.Background())
		t.Fatal(err)
	}
	config.Database = name
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: stdlib.OpenDB(*config)}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool, _ := db.DB()
		pool.Close()
		_, err := admin.Exec(context.Background(), "DROP DATABASE "+pgx.Identifier{name}.Sanitize())
		admin.Close(context.Background())
		if err != nil {
			t.Error("remove isolated matrix database", name)
		}
	})
	return db
}

func applyCheckpointVideoMigration(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, migration := range allMigrations() {
		if migration.ID == "20260903183000_add_video_runtime_task_poster_url" {
			if err := gormigrate.New(db, migrationOptions(false), []*gormigrate.Migration{migration}).Migrate(); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("missing video migration fixture")
}

func TestConnectorFinalizationPostgres(t *testing.T) {
	ctx := context.Background()
	t.Run("fresh_and_repeat", func(t *testing.T) {
		db := finalizationTestDatabase(t)
		if err := RunWithDB(db); err != nil {
			t.Fatal(err)
		}
		if _, err := connectorfinalize.Verify(ctx, db, connectorfinalize.Options{}); err != nil {
			t.Fatal(err)
		}
		if err := RunWithDB(db); err != nil {
			t.Fatal(err)
		}
		for _, drift := range []string{
			`ALTER TABLE public.connector_pending_setups RENAME TO unexpected_pending_setups`,
			`DELETE FROM public.migrations WHERE id='20260904160000_connector_pending_setups'`,
		} {
			tx := db.Begin()
			if err := tx.Exec(drift).Error; err != nil {
				tx.Rollback()
				t.Fatal(err)
			}
			_, err := connectorfinalize.Verify(ctx, tx, connectorfinalize.Options{})
			tx.Rollback()
			if err == nil || !strings.Contains(err.Error(), "creating migration marker disagree") {
				t.Fatalf("future native table drift accepted: %v", err)
			}
		}
	})
	t.Run("ready_v1_proof_dependency_rollback_repeat", func(t *testing.T) {
		db := finalizationTestDatabase(t)
		m := gormigrate.New(db, migrationOptions(true), allMigrations())
		if err := m.MigrateTo(connectorfinalize.CheckpointID); err != nil {
			t.Fatal(err)
		}
		plan, err := connectorfinalize.Plan(ctx, db)
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.Issues) != 0 {
			t.Fatalf("initial proof: %+v", plan.Issues)
		}
		if err := db.Connection(func(conn *gorm.DB) error {
			if err := conn.Exec("SET TIME ZONE 'Asia/Shanghai'").Error; err != nil {
				return err
			}
			defer conn.Exec("SET TIME ZONE 'UTC'")
			regional, err := connectorfinalize.Plan(ctx, conn)
			if err != nil {
				return err
			}
			if regional.Digest != plan.Digest {
				t.Fatal("session timezone changed the retirement proof")
			}
			var zone string
			if err := conn.Raw("SHOW TIME ZONE").Scan(&zone).Error; err != nil {
				return err
			}
			if zone != "Asia/Shanghai" {
				t.Fatal("finalizer leaked transaction timezone")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		applyCheckpointVideoMigration(t, db)
		if err := RunToConnectorCheckpoint(db, connectorfinalize.CheckpointID); err != nil {
			t.Fatal("unrelated video migration blocked the messaging checkpoint", err)
		}
		if _, err := connectorfinalize.Apply(ctx, db, connectorfinalize.ApplyOptions{ExpectedReport: plan.Digest}); err == nil {
			t.Fatal("changed migration history reused a reviewed retirement report")
		}
		updated, err := connectorfinalize.Plan(ctx, db)
		if err != nil || updated.Checkpoint != connectorfinalize.CheckpointID || updated.Digest == plan.Digest || updated.MigrationHistoryDigest == plan.MigrationHistoryDigest {
			t.Fatal("migration history was not bound to the retirement report", err)
		}
		plan = updated
		if _, err := connectorfinalize.Apply(ctx, db, connectorfinalize.ApplyOptions{ExpectedReport: strings.Repeat("0", 64)}); err == nil {
			t.Fatal("wrong report accepted")
		}
		if err := db.Exec(`CREATE VIEW public.finalization_unexpected_view AS SELECT receipt_id FROM public.connector_execution_route_claims`).Error; err != nil {
			t.Fatal(err)
		}
		blocked, err := connectorfinalize.Plan(ctx, db)
		if err != nil {
			t.Fatal(err)
		}
		if len(blocked.Issues) == 0 {
			t.Fatal("unknown view was not rejected")
		}
		if _, err := connectorfinalize.Apply(ctx, db, connectorfinalize.ApplyOptions{ExpectedReport: blocked.Digest}); err == nil {
			t.Fatal("dependency proof ignored")
		}
		if err := db.Exec(`DROP VIEW public.finalization_unexpected_view`).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(`CREATE FUNCTION public.finalization_fail() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected maintenance failure'; END $$; CREATE TRIGGER finalization_fail BEFORE UPDATE ON public.connector_schema_readiness FOR EACH ROW EXECUTE FUNCTION public.finalization_fail()`).Error; err != nil {
			t.Fatal(err)
		}
		plan, err = connectorfinalize.Plan(ctx, db)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := connectorfinalize.Apply(ctx, db, connectorfinalize.ApplyOptions{ExpectedReport: plan.Digest}); err == nil {
			t.Fatal("injected failure did not abort")
		}
		for _, table := range connectorfinalize.RetiredTables() {
			if !db.Migrator().HasTable(table) {
				t.Fatal("DDL did not roll back", table)
			}
		}
		var version int
		db.Table("connector_schema_readiness").Select("schema_version").Scan(&version)
		if version != 1 {
			t.Fatal("readiness escaped transaction")
		}
		if err := db.Exec(`DROP TRIGGER finalization_fail ON public.connector_schema_readiness; DROP FUNCTION public.finalization_fail()`).Error; err != nil {
			t.Fatal(err)
		}
		plan, err = connectorfinalize.Plan(ctx, db)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := connectorfinalize.Apply(ctx, db, connectorfinalize.ApplyOptions{ExpectedReport: plan.Digest}); err != nil {
			t.Fatal(err)
		}
		var before, after int64
		db.Table("connector_conversion_records").Count(&before)
		if _, err := connectorfinalize.Apply(ctx, db, connectorfinalize.ApplyOptions{ExpectedReport: plan.Digest}); err != nil {
			t.Fatal(err)
		}
		db.Table("connector_conversion_records").Count(&after)
		if before != after {
			t.Fatal("repeat wrote additional proofs")
		}
		if _, err := connectorfinalize.Verify(ctx, db, connectorfinalize.Options{}); err != nil {
			t.Fatal(err)
		}
		if err := RunWithDB(db); err != nil {
			t.Fatal(fmt.Errorf("repeat public up: %w", err))
		}
	})
}
