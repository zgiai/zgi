package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0079_merge_account_extensions merges account_extensions table into accounts.extensions column
func M0079_merge_account_extensions() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601160079",
		Migrate: func(tx *gorm.DB) error {
			// 1. Add extensions column if not exists
			// We use raw SQL to ensure the column is created regardless of model definition
			if err := tx.Exec(`ALTER TABLE "public"."accounts" ADD COLUMN IF NOT EXISTS "extensions" jsonb`).Error; err != nil {
				return err
			}

			// 2. Migrate data
			// Use jsonb_strip_nulls and explicit NULL handling to avoid {"mobile": null} style payloads
			if err := tx.Exec(`
				UPDATE "public"."accounts" a
				SET "extensions" = (
					SELECT
						CASE
							WHEN ae.mobile IS NULL
								AND ae.wechat IS NULL
								AND ae.address IS NULL
								AND ae.birthdate IS NULL
								AND ae.gender IS NULL
							THEN NULL
							ELSE jsonb_strip_nulls(jsonb_build_object(
								'mobile', ae.mobile,
								'wechat', ae.wechat,
								'address', ae.address,
								'birthdate', CASE WHEN ae.birthdate IS NOT NULL THEN to_char(ae.birthdate, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') ELSE NULL END,
								'gender', ae.gender
							))
						END
					FROM "public"."account_extensions" ae
					WHERE ae.account_id = a.id
				)
				WHERE EXISTS (
					SELECT 1 FROM "public"."account_extensions" ae WHERE ae.account_id = a.id
				)
			`).Error; err != nil {
				return err
			}

			// 3. Drop table
			if err := tx.Exec(`DROP TABLE IF EXISTS "public"."account_extensions"`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// 1. Recreate table
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS "public"."account_extensions" (
					"account_id" uuid NOT NULL,
					"mobile" varchar(20) COLLATE "pg_catalog"."default",
					"wechat" varchar(50) COLLATE "pg_catalog"."default",
					"address" text COLLATE "pg_catalog"."default",
					"birthdate" date,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"gender" varchar(10) COLLATE "pg_catalog"."default",
					CONSTRAINT "account_extension_pkey" PRIMARY KEY ("account_id"),
					CONSTRAINT "account_extensions_account_id_fkey" FOREIGN KEY ("account_id") REFERENCES "public"."accounts" ("id") ON DELETE CASCADE ON UPDATE NO ACTION
				)
			`).Error; err != nil {
				return err
			}

			// 2. Restore data
			if err := tx.Exec(`
				INSERT INTO "public"."account_extensions" (account_id, mobile, wechat, address, birthdate, gender)
				SELECT 
					id, 
					extensions->>'mobile',
					extensions->>'wechat',
					extensions->>'address',
					(extensions->>'birthdate')::date,
					extensions->>'gender'
				FROM "public"."accounts"
				WHERE extensions IS NOT NULL
				ON CONFLICT (account_id) DO NOTHING
			`).Error; err != nil {
				return err
			}

			// 3. Drop column
			if err := tx.Exec(`ALTER TABLE "public"."accounts" DROP COLUMN IF EXISTS "extensions"`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
