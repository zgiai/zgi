package migrations

import (
	"testing"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAddToolUsageUpstreamScopeUpDownAndNonemptyGuard(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE llm_tool_usage_events (id text primary key, organization_id text, billing_lane text, provider_name text, channel_id text, session_id text, status text)").Error; err != nil {
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
	if !db.Migrator().HasColumn("llm_tool_usage_events", "upstream_scope") || !db.Migrator().HasIndex("llm_tool_usage_events", "idx_tool_events_session_scope") {
		t.Fatal("scope column or session index missing")
	}
	if err := db.Exec("INSERT INTO llm_tool_usage_events(id) VALUES ('fact')").Error; err != nil {
		t.Fatal(err)
	}
	if err := migration.Rollback(db); err == nil {
		t.Fatal("nonempty rollback must be refused")
	}
	if err := db.Exec("DELETE FROM llm_tool_usage_events").Error; err != nil {
		t.Fatal(err)
	}
	if err := migration.Rollback(db); err != nil {
		t.Fatal(err)
	}
}
