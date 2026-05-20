package migrationsv2

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0010_drop_payment_subscription_schema() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2DropPaymentSubscriptionID,
		Migrate: func(tx *gorm.DB) error {
			if tx.Migrator().HasTable("group_ai_credit_accounts") && tx.Migrator().HasColumn("group_ai_credit_accounts", "subscription_credits") {
				if err := tx.Exec(`
					UPDATE group_ai_credit_accounts
					SET subscription_credits = 0
				`).Error; err != nil {
					return err
				}
			}

			if err := tx.Exec(`DROP TABLE IF EXISTS subscription_history CASCADE`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`DROP TABLE IF EXISTS group_subscriptions CASCADE`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`DROP TABLE IF EXISTS subscription_plans CASCADE`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`DROP TABLE IF EXISTS group_quotas CASCADE`).Error; err != nil {
				return err
			}

			if tx.Migrator().HasTable("group_ai_credit_accounts") {
				if err := tx.Exec(`
					ALTER TABLE group_ai_credit_accounts
						DROP COLUMN IF EXISTS subscription_credits,
						DROP COLUMN IF EXISTS last_reset_at,
						DROP COLUMN IF EXISTS next_reset_at
				`).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
