package migrations

import mschema "github.com/zgiai/zgi/api/internal/migrations/schema"

const migrationAddOperationToUsageBillsID = "20260916120000_add_operation_to_usage_bills"

const addOperationToUsageBillsSQL = `
	ALTER TABLE public.llm_usage_bills
		ADD COLUMN IF NOT EXISTS operation varchar(64);

	CREATE INDEX IF NOT EXISTS idx_usage_bills_org_operation_created
		ON public.llm_usage_bills (organization_id, operation, request_created_at);
`

const rollbackOperationFromUsageBillsSQL = `
	DROP INDEX IF EXISTS idx_usage_bills_org_operation_created;

	ALTER TABLE public.llm_usage_bills
		DROP COLUMN IF EXISTS operation;
`

func init() {
	registerSchemaMigration(
		migrationAddOperationToUsageBillsID,
		upAddOperationToUsageBills,
		downAddOperationToUsageBills,
	)
}

func upAddOperationToUsageBills(schema *mschema.Builder) error {
	return schema.Raw(addOperationToUsageBillsSQL)
}

func downAddOperationToUsageBills(schema *mschema.Builder) error {
	return schema.Raw(rollbackOperationFromUsageBillsSQL)
}
