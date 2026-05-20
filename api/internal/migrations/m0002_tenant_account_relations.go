package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0002_tenant_account_relations creates tenant-account relation tables
func M0002_tenant_account_relations() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251110100100",
		Migrate: func(tx *gorm.DB) error {
			// Create enterprise_groups table
			if err := tx.Exec(`
				CREATE TABLE "public"."enterprise_groups" (
					"id" uuid NOT NULL DEFAULT uuid_generate_v4(),
					"name" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
					"status" varchar(16) COLLATE "pg_catalog"."default" NOT NULL DEFAULT 'active'::character varying,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"short_name" varchar(100) COLLATE "pg_catalog"."default"
				);
			`).Error; err != nil {
				return err
			}

			// Create end_users table
			if err := tx.Exec(`
				CREATE TABLE "public"."end_users" (
					"id" uuid NOT NULL DEFAULT uuid_generate_v4(),
					"tenant_id" uuid NOT NULL,
					"app_id" uuid,
					"type" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
					"external_user_id" varchar(255) COLLATE "pg_catalog"."default",
					"name" varchar(255) COLLATE "pg_catalog"."default",
					"is_anonymous" bool NOT NULL DEFAULT true,
					"session_id" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
				);
			`).Error; err != nil {
				return err
			}

			// Create enterprise_group_account_joins table
			if err := tx.Exec(`
				CREATE TABLE "public"."enterprise_group_account_joins" (
					"group_id" uuid NOT NULL,
					"account_id" uuid NOT NULL,
					"role" varchar(16) COLLATE "pg_catalog"."default" NOT NULL DEFAULT 'normal'::character varying,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
				);
			`).Error; err != nil {
				return err
			}

			// Create enterprise_group_providers table
			if err := tx.Exec(`
				CREATE TABLE "public"."enterprise_group_providers" (
					"id" uuid NOT NULL DEFAULT uuid_generate_v4(),
					"group_id" uuid NOT NULL,
					"provider_name" varchar(255) COLLATE "pg_catalog"."default" NOT NULL,
					"provider_type" varchar(40) COLLATE "pg_catalog"."default" NOT NULL DEFAULT 'custom'::character varying,
					"encrypted_config" text COLLATE "pg_catalog"."default",
					"is_valid" bool NOT NULL DEFAULT false,
					"last_used" timestamptz,
					"quota_type" varchar(40) COLLATE "pg_catalog"."default" DEFAULT ''::character varying,
					"quota_limit" int8,
					"quota_used" int8,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
				);
			`).Error; err != nil {
				return err
			}

			// Create enterprise_group_tenant_joins table
			if err := tx.Exec(`
				CREATE TABLE "public"."enterprise_group_tenant_joins" (
					"group_id" uuid NOT NULL,
					"tenant_id" uuid NOT NULL,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
				);
			`).Error; err != nil {
				return err
			}

			// Create tenant_account_joins table
			if err := tx.Exec(`
				CREATE TABLE "public"."tenant_account_joins" (
					"id" uuid NOT NULL DEFAULT uuid_generate_v4(),
					"tenant_id" uuid NOT NULL,
					"account_id" uuid NOT NULL,
					"role" varchar(16) COLLATE "pg_catalog"."default" NOT NULL DEFAULT 'normal'::character varying,
					"invited_by" uuid,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"current" bool NOT NULL DEFAULT false
				);
			`).Error; err != nil {
				return err
			}

			// Create tenant_account_extensions table
			if err := tx.Exec(`
				CREATE TABLE "public"."tenant_account_extensions" (
					"tenant_account_join_id" uuid NOT NULL,
					"position" varchar(100) COLLATE "pg_catalog"."default",
					"permissions" varchar(32)[] COLLATE "pg_catalog"."default" DEFAULT '{}'::character varying[],
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
				);
			`).Error; err != nil {
				return err
			}

			// Create indexes and constraints
			statements := []string{
				// end_users indexes and constraints
				`CREATE INDEX "end_user_session_id_idx" ON "public"."end_users" USING btree ("session_id" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST, "type" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST)`,
				`CREATE INDEX "end_user_tenant_session_id_idx" ON "public"."end_users" USING btree ("tenant_id" "pg_catalog"."uuid_ops" ASC NULLS LAST, "session_id" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST, "type" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST)`,
				`ALTER TABLE "public"."end_users" ADD CONSTRAINT "end_user_pkey" PRIMARY KEY ("id")`,

				// enterprise_group_account_joins indexes and constraints
				`CREATE INDEX "eg_account_account_idx" ON "public"."enterprise_group_account_joins" USING btree ("account_id" "pg_catalog"."uuid_ops" ASC NULLS LAST)`,
				`CREATE INDEX "eg_account_group_idx" ON "public"."enterprise_group_account_joins" USING btree ("group_id" "pg_catalog"."uuid_ops" ASC NULLS LAST)`,
				`ALTER TABLE "public"."enterprise_group_account_joins" ADD CONSTRAINT "eg_account_pkey" PRIMARY KEY ("group_id", "account_id")`,

				// enterprise_group_providers indexes and constraints
				`CREATE INDEX "provider_group_id_provider_idx" ON "public"."enterprise_group_providers" USING btree ("group_id" "pg_catalog"."uuid_ops" ASC NULLS LAST, "provider_name" COLLATE "pg_catalog"."default" "pg_catalog"."text_ops" ASC NULLS LAST)`,
				`ALTER TABLE "public"."enterprise_group_providers" ADD CONSTRAINT "unique_enterprise_group_provider_name_type_quota" UNIQUE ("group_id", "provider_name", "provider_type", "quota_type")`,
				`ALTER TABLE "public"."enterprise_group_providers" ADD CONSTRAINT "enterprise_group_provider_pkey" PRIMARY KEY ("id")`,

				// enterprise_group_tenant_joins indexes and constraints
				`ALTER TABLE "public"."enterprise_group_tenant_joins" ADD CONSTRAINT "unique_tenant_in_group" UNIQUE ("tenant_id")`,
				`ALTER TABLE "public"."enterprise_group_tenant_joins" ADD CONSTRAINT "eg_tenant_pkey" PRIMARY KEY ("group_id", "tenant_id")`,

				// enterprise_groups constraints
				`ALTER TABLE "public"."enterprise_groups" ADD CONSTRAINT "enterprise_group_pkey" PRIMARY KEY ("id")`,

				// tenant_account_joins indexes and constraints
				`CREATE INDEX "idx_tenant_account_joins_account_current" ON "public"."tenant_account_joins" USING btree ("account_id" "pg_catalog"."uuid_ops" ASC NULLS LAST, "current" "pg_catalog"."bool_ops" ASC NULLS LAST)`,
				`CREATE INDEX "tenant_account_join_account_id_idx" ON "public"."tenant_account_joins" USING btree ("account_id" "pg_catalog"."uuid_ops" ASC NULLS LAST)`,
				`CREATE INDEX "tenant_account_join_tenant_id_idx" ON "public"."tenant_account_joins" USING btree ("tenant_id" "pg_catalog"."uuid_ops" ASC NULLS LAST)`,
				`ALTER TABLE "public"."tenant_account_joins" ADD CONSTRAINT "unique_tenant_account_join" UNIQUE ("tenant_id", "account_id")`,
				`ALTER TABLE "public"."tenant_account_joins" ADD CONSTRAINT "tenant_account_join_pkey" PRIMARY KEY ("id")`,

				// tenant_account_extensions constraints
				`ALTER TABLE "public"."tenant_account_extensions" ADD CONSTRAINT "tenant_account_extension_pkey" PRIMARY KEY ("tenant_account_join_id")`,
			}

			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}

			// Add foreign key constraints
			foreignKeyConstraints := []string{
				// enterprise_group_account_joins foreign keys
				`ALTER TABLE "public"."enterprise_group_account_joins" ADD CONSTRAINT "enterprise_group_account_joins_account_id_fkey" FOREIGN KEY ("account_id") REFERENCES "public"."accounts" ("id") ON DELETE CASCADE ON UPDATE NO ACTION`,
				`ALTER TABLE "public"."enterprise_group_account_joins" ADD CONSTRAINT "enterprise_group_account_joins_group_id_fkey" FOREIGN KEY ("group_id") REFERENCES "public"."enterprise_groups" ("id") ON DELETE CASCADE ON UPDATE NO ACTION`,

				// enterprise_group_tenant_joins foreign keys
				`ALTER TABLE "public"."enterprise_group_tenant_joins" ADD CONSTRAINT "enterprise_group_tenant_joins_group_id_fkey" FOREIGN KEY ("group_id") REFERENCES "public"."enterprise_groups" ("id") ON DELETE CASCADE ON UPDATE NO ACTION`,
				`ALTER TABLE "public"."enterprise_group_tenant_joins" ADD CONSTRAINT "enterprise_group_tenant_joins_tenant_id_fkey" FOREIGN KEY ("tenant_id") REFERENCES "public"."tenants" ("id") ON DELETE CASCADE ON UPDATE NO ACTION`,

				// tenant_account_extensions foreign keys
				`ALTER TABLE "public"."tenant_account_extensions" ADD CONSTRAINT "tenant_account_extensions_tenant_account_join_id_fkey" FOREIGN KEY ("tenant_account_join_id") REFERENCES "public"."tenant_account_joins" ("id") ON DELETE CASCADE ON UPDATE NO ACTION`,
			}

			for _, constraintSQL := range foreignKeyConstraints {
				if err := tx.Exec(constraintSQL).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop tables in reverse order
			tables := []string{
				"tenant_account_extensions",
				"tenant_account_joins",
				"enterprise_group_tenant_joins",
				"enterprise_group_providers",
				"enterprise_group_account_joins",
				"end_users",
				"enterprise_groups",
			}
			for _, table := range tables {
				if err := tx.Exec(`DROP TABLE IF EXISTS "public"."` + table + `";`).Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}
