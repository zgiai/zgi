package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddOrganizationLLMRoutingStrategyID = "20260904100000_add_organization_llm_routing_strategy"

func init() {
	registerSchemaMigration(
		migrationAddOrganizationLLMRoutingStrategyID,
		upAddOrganizationLLMRoutingStrategy,
		downAddOrganizationLLMRoutingStrategy,
	)
}

func upAddOrganizationLLMRoutingStrategy(schema *mschema.Builder) error {
	return schema.WhenTableDoesntHaveColumn("organizations", "llm_routing_strategy", func() error {
		return schema.Table("organizations", func(table *mschema.Blueprint) {
			table.String("llm_routing_strategy", 16).DefaultSQL("'stability'").NotNull()
		})
	})
}

func downAddOrganizationLLMRoutingStrategy(schema *mschema.Builder) error {
	return schema.WhenTableHasColumn("organizations", "llm_routing_strategy", func() error {
		return schema.Table("organizations", func(table *mschema.Blueprint) {
			table.DropColumn("llm_routing_strategy")
		})
	})
}
