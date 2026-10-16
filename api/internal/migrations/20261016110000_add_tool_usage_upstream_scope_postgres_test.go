package migrations

import (
	"os"
	"testing"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAddToolUsageUpstreamScopePostgresUpDownGuardAndIndex(t *testing.T) {
	dsn := os.Getenv("ZGI_TOOL_USAGE_TEST_DSN")
	if dsn == "" {
		t.Skip("local Postgres DSN required")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	defer sqlDB.Close()
	schema := "tool_fix_" + uuid.NewString()[:8]
	if err := db.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA " + schema + " CASCADE")
	if err := db.Exec("SET search_path TO " + schema).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE llm_tool_usage_events (id uuid primary key, organization_id uuid, billing_lane text, provider_name text, channel_id uuid, session_id text, status text)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE billing_attempts (attempt_id text primary key, status text, settlement_state text, next_reconcile_at timestamptz, updated_at timestamptz)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE llm_usage_bills (attempt_id text primary key, status text, pricing_snapshot jsonb)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO billing_attempts(attempt_id,status,settlement_state,updated_at) VALUES
		('pending','SETTLED','SETTLED',NOW()),('terminal','SETTLED','SETTLED',NOW());
		INSERT INTO llm_usage_bills(attempt_id,status,pricing_snapshot) VALUES
		('pending','partial','{"tool_settlement":{"usage_state":"pending"}}'),
		('terminal','success','{"tool_settlement":{"usage_state":"settled"}}')`).Error; err != nil {
		t.Fatal(err)
	}
	var migration *gormigrate.Migration
	for _, candidate := range registeredMigrations() {
		if candidate.ID == migrationAddToolUsageUpstreamScopeID {
			migration = candidate
		}
	}
	if migration == nil {
		t.Fatal("migration not registered")
	}
	if err := migration.Migrate(db); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM pg_indexes WHERE schemaname = current_schema() AND indexname = 'idx_tool_events_session_scope'").Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("index count=%d err=%v", count, err)
	}
	if err := db.Table("billing_attempts").Where("attempt_id = ? AND status = ? AND next_reconcile_at IS NOT NULL", "pending", "PARTIAL_SETTLED").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("pending upgrade attempt was not queued: count=%d err=%v", count, err)
	}
	if err := db.Table("billing_attempts").Where("attempt_id = ? AND next_reconcile_at IS NOT NULL", "terminal").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("terminal upgrade attempt was queued: count=%d err=%v", count, err)
	}
	if err := db.Exec("INSERT INTO llm_tool_usage_events(id) VALUES (?)", uuid.New()).Error; err != nil {
		t.Fatal(err)
	}
	if err := migration.Rollback(db); err == nil {
		t.Fatal("rollback guard did not fire")
	}
	if err := db.Exec("DELETE FROM llm_tool_usage_events").Error; err != nil {
		t.Fatal(err)
	}
	if err := migration.Rollback(db); err != nil {
		t.Fatal(err)
	}
}
