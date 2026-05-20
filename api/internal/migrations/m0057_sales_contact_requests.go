package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0057_sales_contact_requests() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251220000057",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				CREATE TABLE "public"."sales_contact_requests" (
					"id" varchar(255) NOT NULL,
					"account_id" uuid,
					"company_name" varchar(255) NOT NULL,
					"contact_name" varchar(100) NOT NULL,
					"phone" varchar(50) NOT NULL,
					"email" varchar(255),
					"extra_meta" jsonb NOT NULL DEFAULT '{}'::jsonb,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY ("id")
				);

				CREATE INDEX IF NOT EXISTS idx_sales_contact_account
					ON sales_contact_requests(account_id);

				CREATE INDEX IF NOT EXISTS idx_sales_contact_created_at
					ON sales_contact_requests(created_at);

				CREATE UNIQUE INDEX IF NOT EXISTS idx_sales_contact_account_unique
					ON sales_contact_requests(account_id)
					WHERE account_id IS NOT NULL;
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				DROP INDEX IF EXISTS idx_sales_contact_account_unique;
				DROP INDEX IF EXISTS idx_sales_contact_created_at;
				DROP INDEX IF EXISTS idx_sales_contact_account;
				DROP TABLE IF EXISTS sales_contact_requests;
			`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}

