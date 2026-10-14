package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationCreateZGIMarketCallLogsID = "202610140900000000_create_zgi_market_call_logs"

func init() {
	registerSchemaMigration(migrationCreateZGIMarketCallLogsID, upCreateZGIMarketCallLogs, downCreateZGIMarketCallLogs)
}

// upCreateZGIMarketCallLogs keeps one row per ZGI Market tool call this instance made for an agent
// or a digital human: what was called, its input and output, how it ended, and what the market
// charged for it, read back by the request id the gateway answered with. It belongs to the
// zgimarket module only.
func upCreateZGIMarketCallLogs(schema *mschema.Builder) error {
	return schema.Create("zgi_market_call_logs", func(table *mschema.Blueprint) {
		table.UUID("id").Primary()
		table.UUID("organization_id").NotNull()
		table.String("subject_type", 32).NotNull()
		table.UUID("subject_id").NotNull()
		table.String("item_kind", 8).NotNull()
		table.String("listing_slug", 128).NotNull()
		table.String("listing_title", 200).Default("").NotNull()
		table.String("offering_slug", 128).Default("").NotNull()
		table.String("tool_name", 128).NotNull()
		table.Text("input").NotNull()
		table.Text("output").NotNull()
		table.String("status", 16).NotNull()
		table.String("error_message", 500).Default("").NotNull()
		table.BigInteger("duration_ms").NotNull()
		table.String("request_id", 64).Default("").NotNull()
		table.String("charge_state", 16).NotNull()
		table.BigInteger("charge_micros").Default(0).NotNull()
		table.JSONB("charge_lines").DefaultSQL("'[]'::jsonb").NotNull()
		table.TimestampTz("charge_checked_at").Nullable()
		table.TimestampTz("created_at").NotNull()
		table.Index("idx_zgi_market_call_logs_organization_created", "organization_id", "created_at")
		table.Index("idx_zgi_market_call_logs_subject", "organization_id", "subject_type", "subject_id", "created_at")
	})
}

func downCreateZGIMarketCallLogs(schema *mschema.Builder) error {
	return schema.DropIfExists("zgi_market_call_logs")
}
