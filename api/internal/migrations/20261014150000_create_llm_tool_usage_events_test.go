package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/llm/toolusage"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCreateLLMToolUsageEventsMigrationUpAndRollback(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	var migrationIndex = -1
	for i, migration := range registeredMigrations() {
		if migration.ID == migrationCreateLLMToolUsageEventsID {
			migrationIndex = i
			break
		}
	}
	if migrationIndex < 0 {
		t.Fatalf("migration %s is not registered", migrationCreateLLMToolUsageEventsID)
	}
	m := registeredMigrations()[migrationIndex]
	if err := m.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if !db.Migrator().HasTable(&toolusage.Event{}) {
		t.Fatal("tool usage event table missing")
	}
	if err := m.Rollback(db); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasTable(&toolusage.Event{}) {
		t.Fatal("tool usage event table still exists")
	}
}

func TestToolUsageMigrationRollbackPreservesAuditFacts(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	var m *gormigrate.Migration
	for _, candidate := range registeredMigrations() {
		if candidate.ID == migrationCreateLLMToolUsageEventsID {
			m = candidate
		}
	}
	if m == nil {
		t.Fatal("migration not registered")
	}
	if err := m.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO llm_tool_usage_events
 (id,schema_version,organization_id,request_id,attempt_id,event_id,billing_lane,tool,quantity,unit,status,usage_source,dedupe_key,details,first_observed_at,last_observed_at,created_at,updated_at)
 VALUES ('00000000-0000-0000-0000-000000000001',1,'00000000-0000-0000-0000-000000000002','request','attempt','event','private','web_search',1,'call','succeeded','provider_reported','key','{}',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := m.Rollback(db); err == nil {
		t.Fatal("rollback must preserve nonempty audit table")
	}
	var count int64
	if err := db.Table("llm_tool_usage_events").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("audit lost: %d %v", count, err)
	}
}
