package migrations

import (
	"fmt"

	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

const migrationAddToolUsageUpstreamScopeID = "20261016110000_add_tool_usage_upstream_scope"

func init() {
	registerSchemaMigration(migrationAddToolUsageUpstreamScopeID, upAddToolUsageUpstreamScope, downAddToolUsageUpstreamScope)
}

// The checks use GORM's migrator, which follows the session search_path, so
// the migration behaves the same in the public schema and in isolated tests.
func upAddToolUsageUpstreamScope(schema *mschema.Builder) error {
	return schema.DataFix("add tool usage upstream scope and requeue pending embedded tool usage", func(db *gorm.DB) error {
		if !db.Migrator().HasColumn("llm_tool_usage_events", "upstream_scope") {
			if err := db.Exec("ALTER TABLE llm_tool_usage_events ADD COLUMN upstream_scope varchar(128) NOT NULL DEFAULT ''").Error; err != nil {
				return err
			}
		}
		if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_tool_events_session_scope ON llm_tool_usage_events (organization_id, billing_lane, provider_name, channel_id, upstream_scope, session_id, status)").Error; err != nil {
			return err
		}
		if !db.Migrator().HasTable("billing_attempts") || !db.Migrator().HasTable("llm_usage_bills") {
			return nil
		}
		// One-time upgrade handoff for tool bills that the old full-history
		// sweep kept alive. Move only embedded pending usage back to the
		// structured due queue; terminal usage and rolled-back attempts remain
		// untouched.
		pendingUsage := `CAST(b.pricing_snapshot AS TEXT) LIKE '%"tool_settlement"%' AND CAST(b.pricing_snapshot AS TEXT) LIKE '%"usage_state"%' AND CAST(b.pricing_snapshot AS TEXT) LIKE '%"pending"%'`
		if db.Dialector.Name() == "postgres" {
			pendingUsage = `(b.pricing_snapshot::jsonb -> 'tool_settlement' ->> 'usage_state') = 'pending'`
		}
		return db.Exec(fmt.Sprintf(`UPDATE billing_attempts AS a
			SET status = 'PARTIAL_SETTLED', settlement_state = 'PARTIAL_SETTLED',
			    next_reconcile_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
			FROM llm_usage_bills AS b
			WHERE b.attempt_id = a.attempt_id
			  AND b.status = 'partial'
			  AND %s
			  AND a.status NOT IN ('ROLLED_BACK','PREDEDUCT_FAILED','COMPENSATED','FAILED_FINAL')`, pendingUsage)).Error
	})
}

func downAddToolUsageUpstreamScope(schema *mschema.Builder) error {
	return schema.DataFix("drop tool usage upstream scope when no audit facts exist", func(db *gorm.DB) error {
		var count int64
		if err := db.Table("llm_tool_usage_events").Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("tool usage events contain %d audit facts; preserve upstream scope and roll back the application only", count)
		}
		if err := db.Exec("DROP INDEX IF EXISTS idx_tool_events_session_scope").Error; err != nil {
			return err
		}
		if db.Migrator().HasColumn("llm_tool_usage_events", "upstream_scope") {
			return db.Exec("ALTER TABLE llm_tool_usage_events DROP COLUMN upstream_scope").Error
		}
		return nil
	})
}
