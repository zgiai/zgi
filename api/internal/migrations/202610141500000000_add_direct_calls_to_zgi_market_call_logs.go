package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddDirectCallsToZGIMarketCallLogsID = "202610141500000000_add_direct_calls_to_zgi_market_call_logs"

func init() {
	registerSchemaMigration(migrationAddDirectCallsToZGIMarketCallLogsID, upAddDirectCallsToZGIMarketCallLogs, downAddDirectCallsToZGIMarketCallLogs)
}

// upAddDirectCallsToZGIMarketCallLogs lets the call log hold the calls made with a market
// credential outside this instance, read back from the market: the credential each was made
// with, and per organization how far the market's call records have been read.
func upAddDirectCallsToZGIMarketCallLogs(schema *mschema.Builder) error {
	if err := schema.Table("zgi_market_call_logs", func(table *mschema.Blueprint) {
		table.BigInteger("credential_id").Default(0).NotNull()
		table.String("credential_name", 100).Default("").NotNull()
		table.Index("idx_zgi_market_call_logs_request", "organization_id", "request_id")
	}); err != nil {
		return err
	}
	return schema.Create("zgi_market_call_log_cursors", func(table *mschema.Blueprint) {
		table.UUID("organization_id").Primary()
		table.BigInteger("after_id").NotNull()
		table.TimestampTz("updated_at").NotNull()
	})
}

func downAddDirectCallsToZGIMarketCallLogs(schema *mschema.Builder) error {
	if err := schema.DropIfExists("zgi_market_call_log_cursors"); err != nil {
		return err
	}
	// Dropping the columns leaves the request index, which belongs to no dropped column.
	return schema.Table("zgi_market_call_logs", func(table *mschema.Blueprint) {
		table.DropColumn("credential_id")
		table.DropColumn("credential_name")
	})
}
