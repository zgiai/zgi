package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0070_account_contexts creates the account_contexts table for storing current org/workspace per account
func M0070_account_contexts() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "0070_account_contexts",
		Migrate: func(tx *gorm.DB) error {
			sql := `
				CREATE TABLE IF NOT EXISTS "public"."account_contexts" (
					"account_id"        uuid NOT NULL,
					"current_group_id"  uuid,
					"current_team_id"   uuid,
					"created_at"        timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"updated_at"        timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
				);

				ALTER TABLE "public"."account_contexts"
					ADD CONSTRAINT "account_contexts_pkey" PRIMARY KEY ("account_id");
			`
			return tx.Exec(sql).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE IF EXISTS "public"."account_contexts";`).Error
		},
	}
}
