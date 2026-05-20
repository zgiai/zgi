package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0034_payment_query_indexes adds indexes to optimize high-frequency payment scheduler queries.
func M0034_payment_query_indexes() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251214000034",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				-- Optimize order expire task query:
				-- SELECT * FROM orders WHERE status = 'pending' AND created_at < ?
				CREATE INDEX IF NOT EXISTS idx_orders_pending_created_at
					ON orders(created_at)
					WHERE status = 'pending';

				-- Optimize subscription renewal task query:
				-- SELECT * FROM group_subscriptions
				-- WHERE auto_renew = true AND next_billing_date IS NOT NULL
				--   AND next_billing_date <= ? AND next_billing_date > ?
				--   AND status = 'active'
				--   AND COALESCE(external_binding->>'provider','') = ''
				CREATE INDEX IF NOT EXISTS idx_group_subscriptions_renewal_next_billing_date
					ON group_subscriptions(next_billing_date)
					WHERE status = 'active'
					  AND auto_renew = true
					  AND next_billing_date IS NOT NULL
					  AND COALESCE(external_binding->>'provider','') = '';
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				DROP INDEX IF EXISTS idx_group_subscriptions_renewal_next_billing_date;
				DROP INDEX IF EXISTS idx_orders_pending_created_at;
			`).Error; err != nil {
				return err
			}
			return nil
		},
	}
}
