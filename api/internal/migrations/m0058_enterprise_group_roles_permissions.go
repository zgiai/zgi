package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0058_enterprise_group_roles_permissions() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202512210058",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				CREATE TABLE "public"."enterprise_group_roles" (
					"id" uuid NOT NULL DEFAULT uuid_generate_v4(),
					"group_id" uuid NOT NULL,
					"name" varchar(255) NOT NULL,
					"description" text,
					"status" varchar(16) NOT NULL DEFAULT 'active',
					"created_by" uuid NOT NULL,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY ("id"),
					CONSTRAINT fk_eg_roles_group FOREIGN KEY (group_id)
						REFERENCES enterprise_groups(id) ON DELETE CASCADE,
					CONSTRAINT fk_eg_roles_created_by FOREIGN KEY (created_by)
						REFERENCES accounts(id) ON DELETE RESTRICT,
					CONSTRAINT uk_eg_roles_group_name UNIQUE (group_id, name)
				);

				CREATE INDEX IF NOT EXISTS idx_eg_roles_group_id ON enterprise_group_roles(group_id);
				CREATE INDEX IF NOT EXISTS idx_eg_roles_status ON enterprise_group_roles(status);
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				CREATE TABLE "public"."enterprise_group_role_permissions" (
					"role_id" uuid NOT NULL,
					"permission_code" varchar(128) NOT NULL,
					"created_by" uuid NOT NULL,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY ("role_id", "permission_code"),
					CONSTRAINT fk_eg_role_perm_role FOREIGN KEY (role_id)
						REFERENCES enterprise_group_roles(id) ON DELETE CASCADE,
					CONSTRAINT fk_eg_role_perm_created_by FOREIGN KEY (created_by)
						REFERENCES accounts(id) ON DELETE RESTRICT
				);

				CREATE INDEX IF NOT EXISTS idx_eg_role_perm_role_id ON enterprise_group_role_permissions(role_id);
				CREATE INDEX IF NOT EXISTS idx_eg_role_perm_perm_code ON enterprise_group_role_permissions(permission_code);

				ALTER TABLE tenant_account_joins
				ADD COLUMN IF NOT EXISTS role_id uuid NULL;

				CREATE INDEX IF NOT EXISTS idx_tenant_account_role_id ON tenant_account_joins(role_id);
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				DROP TABLE IF EXISTS enterprise_group_role_permissions;
				DROP TABLE IF EXISTS enterprise_group_roles;

				ALTER TABLE tenant_account_joins
				DROP COLUMN IF EXISTS role_id;
			`).Error; err != nil {
				return err
			}
			return nil
		},
	}
}
