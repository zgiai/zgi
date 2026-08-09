package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddLLMInvocationSourceID = "20260808120000_add_llm_invocation_source"

func init() {
	registerSchemaMigration(
		migrationAddLLMInvocationSourceID,
		upAddLLMInvocationSource,
		downAddLLMInvocationSource,
	)
}

func upAddLLMInvocationSource(schema *mschema.Builder) error {
	return schema.WhenTableDoesntHaveColumn("llm_usage_bills", "invocation_source", func() error {
		return schema.Table("llm_usage_bills", func(table *mschema.Blueprint) {
			table.String("invocation_source", 20).DefaultSQL("'unknown'").NotNull()
		})
	})
}

func downAddLLMInvocationSource(schema *mschema.Builder) error {
	return schema.WhenTableHasColumn("llm_usage_bills", "invocation_source", func() error {
		return schema.Table("llm_usage_bills", func(table *mschema.Blueprint) {
			table.DropColumn("invocation_source")
		})
	})
}
