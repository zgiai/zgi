package migrations

import (
	mschema "github.com/zgiai/zgi/api/internal/migrations/schema"
	"gorm.io/gorm"
)

// migrationHardenUsageBillsOperationID tightens llm_usage_bills.operation after
// the original additive migration: legacy rows are classified as the empty
// default (historical operations cannot be inferred), then the column becomes
// NOT NULL with a default. The append-only rule forbids editing the already
// applied 20260916120000 migration, so the hardening is a separate migration.
const migrationHardenUsageBillsOperationID = "20260916153000_harden_usage_bills_operation"

func init() {
	registerSchemaMigration(migrationHardenUsageBillsOperationID, upHardenUsageBillsOperation, downHardenUsageBillsOperation)
}

func upHardenUsageBillsOperation(schema *mschema.Builder) error {
	if err := schema.Raw(`
		ALTER TABLE public.llm_usage_bills
			ALTER COLUMN operation SET DEFAULT '';
	`); err != nil {
		return err
	}
	if err := schema.DataFix("backfill llm_usage_bills.operation NULL rows with the empty default", func(db *gorm.DB) error {
		return db.Exec(`UPDATE public.llm_usage_bills SET operation = '' WHERE operation IS NULL`).Error
	}); err != nil {
		return err
	}
	return schema.Raw(`
		ALTER TABLE public.llm_usage_bills
			ALTER COLUMN operation SET NOT NULL;
	`)
}

func downHardenUsageBillsOperation(schema *mschema.Builder) error {
	if err := schema.Raw(`
		ALTER TABLE public.llm_usage_bills
			ALTER COLUMN operation DROP DEFAULT;
	`); err != nil {
		return err
	}
	return schema.Raw(`
		ALTER TABLE public.llm_usage_bills
			ALTER COLUMN operation DROP NOT NULL;
	`)
}
