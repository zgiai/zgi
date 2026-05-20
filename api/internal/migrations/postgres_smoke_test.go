package migrations

import (
	"os"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPublicInitialSchemaAppliesToPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run PostgreSQL migration smoke test")
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}

	if err := RunWithDB(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	var tableCount int64
	if err := db.Raw(`
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_type = 'BASE TABLE'
		  AND table_name <> 'migrations'
	`).Scan(&tableCount).Error; err != nil {
		t.Fatalf("count public tables: %v", err)
	}
	if tableCount < 140 {
		t.Fatalf("expected initial schema to create at least 140 public tables, got %d", tableCount)
	}
}
