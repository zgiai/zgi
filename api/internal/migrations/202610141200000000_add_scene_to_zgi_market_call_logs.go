package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddSceneToZGIMarketCallLogsID = "202610141200000000_add_scene_to_zgi_market_call_logs"

func init() {
	registerSchemaMigration(migrationAddSceneToZGIMarketCallLogsID, upAddSceneToZGIMarketCallLogs, downAddSceneToZGIMarketCallLogs)
}

// upAddSceneToZGIMarketCallLogs records where a market call was made from: the account it ran
// for, the conversation, message and application, and the surface the conversation came from.
func upAddSceneToZGIMarketCallLogs(schema *mschema.Builder) error {
	return schema.Table("zgi_market_call_logs", func(table *mschema.Blueprint) {
		table.String("source", 32).Default("").NotNull()
		table.String("user_id", 64).Default("").NotNull()
		table.String("external_user_id", 128).Default("").NotNull()
		table.String("conversation_id", 64).Default("").NotNull()
		table.String("message_id", 64).Default("").NotNull()
		table.String("app_id", 64).Default("").NotNull()
		table.Index("idx_zgi_market_call_logs_source", "organization_id", "source", "created_at")
	})
}

func downAddSceneToZGIMarketCallLogs(schema *mschema.Builder) error {
	// Dropping the source column drops its index with it.
	return schema.Table("zgi_market_call_logs", func(table *mschema.Blueprint) {
		for _, column := range []string{"source", "user_id", "external_user_id", "conversation_id", "message_id", "app_id"} {
			table.DropColumn(column)
		}
	})
}
