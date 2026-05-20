package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0029_contacts_module creates the contacts module tables (departments, department_members)
// and adds status field to enterprise_group_account_joins
func M0029_contacts_module() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251211000000",
		Migrate: func(tx *gorm.DB) error {
			// 1. Add status field to enterprise_group_account_joins
			if err := tx.Exec(`
				ALTER TABLE "public"."enterprise_group_account_joins" 
				ADD COLUMN IF NOT EXISTS "status" varchar(16) NOT NULL DEFAULT 'active';

				CREATE INDEX IF NOT EXISTS idx_egaj_status ON enterprise_group_account_joins(status);

				COMMENT ON COLUMN enterprise_group_account_joins.status IS 'Member status: active/inactive';
			`).Error; err != nil {
				return err
			}

			// 2. Create departments table
			if err := tx.Exec(`
				CREATE TABLE "public"."departments" (
					"id" uuid NOT NULL DEFAULT uuid_generate_v4(),
					"group_id" uuid NOT NULL,
					"parent_id" uuid,
					"name" varchar(255) NOT NULL,
					"sort_order" int NOT NULL DEFAULT 0,
					"status" varchar(16) NOT NULL DEFAULT 'active',
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"updated_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					"created_by" uuid,
					PRIMARY KEY ("id"),
					CONSTRAINT fk_dept_group FOREIGN KEY (group_id) 
						REFERENCES enterprise_groups(id) ON DELETE CASCADE,
					CONSTRAINT fk_dept_parent FOREIGN KEY (parent_id) 
						REFERENCES departments(id) ON DELETE SET NULL,
					CONSTRAINT uk_dept_name_parent UNIQUE (group_id, parent_id, name)
				);

				-- Indexes
				CREATE INDEX idx_dept_group_id ON departments(group_id);
				CREATE INDEX idx_dept_parent_id ON departments(parent_id);
				CREATE INDEX idx_dept_status ON departments(status);
				CREATE INDEX idx_dept_sort ON departments(group_id, parent_id, sort_order);

				-- Comments
				COMMENT ON TABLE departments IS 'Department table for contacts module';
				COMMENT ON COLUMN departments.group_id IS 'Enterprise group ID';
				COMMENT ON COLUMN departments.parent_id IS 'Parent department ID, NULL for root department';
				COMMENT ON COLUMN departments.name IS 'Department name';
				COMMENT ON COLUMN departments.sort_order IS 'Sort order within same parent';
				COMMENT ON COLUMN departments.status IS 'Department status: active/archived';
			`).Error; err != nil {
				return err
			}

			// 3. Create department_members table
			if err := tx.Exec(`
				CREATE TABLE "public"."department_members" (
					"id" uuid NOT NULL DEFAULT uuid_generate_v4(),
					"department_id" uuid NOT NULL,
					"account_id" uuid NOT NULL,
					"created_at" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY ("id"),
					CONSTRAINT fk_dm_department FOREIGN KEY (department_id) 
						REFERENCES departments(id) ON DELETE CASCADE,
					CONSTRAINT fk_dm_account FOREIGN KEY (account_id) 
						REFERENCES accounts(id) ON DELETE CASCADE,
					CONSTRAINT uk_dept_member UNIQUE (department_id, account_id)
				);

				-- Indexes
				CREATE INDEX idx_dm_department_id ON department_members(department_id);
				CREATE INDEX idx_dm_account_id ON department_members(account_id);

				-- Comments
				COMMENT ON TABLE department_members IS 'Department member association table';
				COMMENT ON COLUMN department_members.department_id IS 'Department ID';
				COMMENT ON COLUMN department_members.account_id IS 'Account ID';
			`).Error; err != nil {
				return err
			}

			// 4. Extend enterprise_group_tenant_joins with department and api key
			if err := tx.Exec(`
				ALTER TABLE "public"."enterprise_group_tenant_joins"
				ADD COLUMN IF NOT EXISTS "department_id" uuid,
				ADD COLUMN IF NOT EXISTS "api_key_id" uuid;

				CREATE INDEX IF NOT EXISTS idx_egtj_department_id ON enterprise_group_tenant_joins(department_id);
				CREATE INDEX IF NOT EXISTS idx_egtj_api_key_id ON enterprise_group_tenant_joins(api_key_id);

				COMMENT ON COLUMN enterprise_group_tenant_joins.department_id IS 'Department ID of this tenant under the enterprise group';
				COMMENT ON COLUMN enterprise_group_tenant_joins.api_key_id IS 'LLM API key ID assigned to this tenant in the group';
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop tables in reverse order
			if err := tx.Exec(`
				DROP TABLE IF EXISTS "public"."department_members";
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				DROP TABLE IF EXISTS "public"."departments";
			`).Error; err != nil {
				return err
			}

			// Remove status column from enterprise_group_account_joins
			if err := tx.Exec(`
				DROP INDEX IF EXISTS idx_egtj_department_id;
				DROP INDEX IF EXISTS idx_egtj_api_key_id;
				ALTER TABLE "public"."enterprise_group_tenant_joins"
				DROP COLUMN IF EXISTS "department_id",
				DROP COLUMN IF EXISTS "api_key_id";

				DROP INDEX IF EXISTS idx_egaj_status;
				ALTER TABLE "public"."enterprise_group_account_joins" 
				DROP COLUMN IF EXISTS "status";
			`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
