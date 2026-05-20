package migrationsv2

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0034_add_usage_bill_billing_lane() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2AddUsageBillBillingLaneID,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`ALTER TABLE llm_usage_bills ADD COLUMN IF NOT EXISTS billing_lane VARCHAR(20)`,
				`ALTER TABLE llm_usage_bills ADD COLUMN IF NOT EXISTS remote_deduction_id VARCHAR(120)`,
				`UPDATE llm_usage_bills
				 SET billing_lane = CASE WHEN use_system_provider THEN 'platform' ELSE 'private' END
				 WHERE billing_lane IS NULL OR billing_lane = ''`,
				`ALTER TABLE llm_usage_bills ALTER COLUMN billing_lane SET DEFAULT 'private'`,
				`ALTER TABLE llm_usage_bills ALTER COLUMN billing_lane SET NOT NULL`,
				`DO $$
				 BEGIN
				   IF NOT EXISTS (
				     SELECT 1
				     FROM pg_constraint
				     WHERE conname = 'ck_llm_usage_bills_billing_lane'
				   ) THEN
				     ALTER TABLE llm_usage_bills
				     ADD CONSTRAINT ck_llm_usage_bills_billing_lane
				     CHECK (billing_lane IN ('platform', 'private'));
				   END IF;
				 END $$`,
				`CREATE INDEX IF NOT EXISTS idx_usage_bills_org_lane_created
				 ON llm_usage_bills(organization_id, billing_lane, request_created_at DESC)`,
				`CREATE INDEX IF NOT EXISTS idx_usage_bills_remote_deduction
				 ON llm_usage_bills(remote_deduction_id)
				 WHERE remote_deduction_id IS NOT NULL`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			statements := []string{
				`DROP INDEX IF EXISTS idx_usage_bills_remote_deduction`,
				`DROP INDEX IF EXISTS idx_usage_bills_org_lane_created`,
				`ALTER TABLE llm_usage_bills DROP CONSTRAINT IF EXISTS ck_llm_usage_bills_billing_lane`,
				`ALTER TABLE llm_usage_bills DROP COLUMN IF EXISTS remote_deduction_id`,
				`ALTER TABLE llm_usage_bills DROP COLUMN IF EXISTS billing_lane`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}
