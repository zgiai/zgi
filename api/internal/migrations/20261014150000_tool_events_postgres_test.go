package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"os"
	"testing"
)

// TestApplyToolEventsLocalMigration is opt-in and applies only this additive migration.
func TestApplyToolEventsLocalMigration(t *testing.T) {
	if os.Getenv("ZGI_T3_APPLY_LOCAL") != "1" {
		t.Skip("explicit local migration opt-in required")
	}
	dsn := os.Getenv("ZGI_TOOL_USAGE_TEST_DSN")
	if dsn == "" {
		t.Fatal("local DSN required")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal("connect to local test DB failed")
	}
	var migration *gormigrate.Migration
	for _, candidate := range registeredMigrations() {
		if candidate.ID == migrationCreateLLMToolUsageEventsID {
			migration = candidate
		}
	}
	if migration == nil {
		t.Fatal("migration not registered")
	}
	opts := gormigrate.DefaultOptions
	copyOpts := *opts
	copyOpts.TableName = "migrations"
	copyOpts.UseTransaction = true
	if err := gormigrate.New(db, &copyOpts, []*gormigrate.Migration{migration}).Migrate(); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasTable("llm_tool_usage_events") {
		t.Fatal("missing event table")
	}
}

func TestToolEventsPostgresUpAndDown(t *testing.T) {
	dsn := os.Getenv("ZGI_TOOL_USAGE_TEST_DSN")
	if dsn == "" {
		t.Skip("local Postgres DSN required")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal("local Postgres connection failed")
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	defer sqlDB.Close()
	schema := "t3_migration_" + uuid.New().String()[:8]
	if err := db.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA " + schema + " CASCADE")
	if err := db.Exec("SET search_path TO " + schema).Error; err != nil {
		t.Fatal(err)
	}
	var migration *gormigrate.Migration
	for _, m := range registeredMigrations() {
		if m.ID == migrationCreateLLMToolUsageEventsID {
			migration = m
		}
	}
	if migration == nil {
		t.Fatal("not registered")
	}
	if err := migration.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasTable("llm_tool_usage_events") {
		t.Fatal("event table missing")
	}
	if err := migration.Rollback(db); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasTable("llm_tool_usage_events") {
		t.Fatal("event table rollback failed")
	}
}

func TestToolEventsLocalNonemptyRollbackGuard(t *testing.T) {
	if os.Getenv("ZGI_T3_ROLLBACK_GUARD") != "1" {
		t.Skip("explicit local audit guard check required")
	}
	db, err := gorm.Open(postgres.Open(os.Getenv("ZGI_TOOL_USAGE_TEST_DSN")), &gorm.Config{})
	if err != nil {
		t.Fatal("connect local DB failed")
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	var before, after int64
	if err := db.Table("llm_tool_usage_events").Count(&before).Error; err != nil || before == 0 {
		t.Fatal("existing local audit facts required")
	}
	var migration *gormigrate.Migration
	for _, m := range registeredMigrations() {
		if m.ID == migrationCreateLLMToolUsageEventsID {
			migration = m
		}
	}
	if migration == nil {
		t.Fatal("not registered")
	}
	if err := migration.Rollback(db); err == nil {
		t.Fatal("nonempty audit rollback must be refused")
	}
	if err := db.Table("llm_tool_usage_events").Count(&after).Error; err != nil || after != before {
		t.Fatalf("audit facts changed: %d -> %d (%v)", before, after, err)
	}
}
