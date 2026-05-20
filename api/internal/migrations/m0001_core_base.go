package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0001_core_base creates core base tables
func M0001_core_base() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251110100000",
		Migrate: func(tx *gorm.DB) error {
			// Create tenants table
			// Drop tables if they exist to ensure clean state (dev fix)
			if err := tx.Exec(`DROP TABLE IF EXISTS "public"."account_roles", "public"."account_integrates", "public"."account_extensions", "public"."accounts", "public"."tenants" CASCADE`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				CREATE TABLE "public"."tenants" (
					"id" uuid NOT NULL DEFAULT uuid_generate_v4(),
					"name" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
					"encrypt_public_key" text COLLATE "pg_catalog"."default",
					"plan" varchar(255) COLLATE "pg_catalog"."default" NOT NULL DEFAULT 'basic'::character varying,
					"status" varchar(255) COLLATE "pg_catalog"."default" NOT NULL DEFAULT 'normal'::character varying,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"custom_config" text COLLATE "pg_catalog"."default"
				);
			`).Error; err != nil {
				return err
			}

			// Create accounts table
			if err := tx.Exec(`
				CREATE TABLE "public"."accounts" (
					"id" uuid NOT NULL DEFAULT uuid_generate_v4(),
					"name" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
					"email" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
					"password" varchar(255) COLLATE "pg_catalog"."default",
					"password_salt" varchar(255) COLLATE "pg_catalog"."default",
					"avatar" varchar(255) COLLATE "pg_catalog"."default",
					"interface_language" varchar(255) COLLATE "pg_catalog"."default",
					"interface_theme" varchar(255) COLLATE "pg_catalog"."default",
					"timezone" varchar(255) COLLATE "pg_catalog"."default",
					"last_login_at" timestamptz,
					"last_login_ip" varchar(255) COLLATE "pg_catalog"."default",
					"status" varchar(16) COLLATE "pg_catalog"."default" NOT NULL DEFAULT 'active'::character varying,
					"initialized_at" timestamptz,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"last_active_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
				);
			`).Error; err != nil {
				return err
			}

			// Create account_extensions table
			if err := tx.Exec(`
				CREATE TABLE "public"."account_extensions" (
					"account_id" uuid NOT NULL,
					"mobile" varchar(20) COLLATE "pg_catalog"."default",
					"wechat" varchar(50) COLLATE "pg_catalog"."default",
					"address" text COLLATE "pg_catalog"."default",
					"birthdate" date,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"gender" varchar(10) COLLATE "pg_catalog"."default"
				);
			`).Error; err != nil {
				return err
			}

			// Create account_integrates table
			if err := tx.Exec(`
				CREATE TABLE "public"."account_integrates" (
					"id" uuid NOT NULL DEFAULT uuid_generate_v4(),
					"account_id" uuid NOT NULL,
					"provider" varchar(16) COLLATE "pg_catalog"."default" NOT NULL,
					"open_id" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
					"encrypted_token" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
				);
			`).Error; err != nil {
				return err
			}

			// Create account_roles table
			if err := tx.Exec(`
				CREATE TABLE "public"."account_roles" (
					"account_id" uuid NOT NULL,
					"role_type" varchar(32) COLLATE "pg_catalog"."default",
					"assigned_by" uuid NOT NULL,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
				);
			`).Error; err != nil {
				return err
			}

			// Create indexes and constraints
			statements := []string{
				`CREATE INDEX "account_email_idx" ON "public"."accounts" USING btree ("email" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST)`,
				`ALTER TABLE "public"."tenants" ADD CONSTRAINT "tenant_pkey" PRIMARY KEY ("id")`,
				`ALTER TABLE "public"."accounts" ADD CONSTRAINT "account_pkey" PRIMARY KEY ("id")`,
				`ALTER TABLE "public"."account_extensions" ADD CONSTRAINT "account_extension_pkey" PRIMARY KEY ("account_id")`,
				`ALTER TABLE "public"."account_integrates" ADD CONSTRAINT "account_integrate_pkey" PRIMARY KEY ("id")`,
				`ALTER TABLE "public"."account_integrates" ADD CONSTRAINT "unique_account_provider" UNIQUE ("account_id", "provider")`,
				`ALTER TABLE "public"."account_integrates" ADD CONSTRAINT "unique_provider_open_id" UNIQUE ("provider", "open_id")`,
				`ALTER TABLE "public"."account_roles" ADD CONSTRAINT "account_role_pkey" PRIMARY KEY ("account_id")`,

				// Add foreign key constraints
				`ALTER TABLE "public"."account_extensions" ADD CONSTRAINT "account_extensions_account_id_fkey" FOREIGN KEY ("account_id") REFERENCES "public"."accounts" ("id") ON DELETE CASCADE ON UPDATE NO ACTION`,
				`ALTER TABLE "public"."account_roles" ADD CONSTRAINT "account_roles_account_id_fkey" FOREIGN KEY ("account_id") REFERENCES "public"."accounts" ("id") ON DELETE CASCADE ON UPDATE NO ACTION`,
			}

			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop tables in reverse order
			tables := []string{"account_roles", "account_integrates", "account_extensions", "accounts", "tenants"}
			for _, table := range tables {
				if err := tx.Exec(`DROP TABLE IF EXISTS "public"."` + table + `";`).Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}
