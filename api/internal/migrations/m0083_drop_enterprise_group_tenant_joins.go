package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0083_drop_enterprise_group_tenant_joins() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601160083",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec("DROP TABLE IF EXISTS enterprise_group_tenant_joins").Error
		},
		Rollback: func(tx *gorm.DB) error {
			// Recreate the table
			return tx.Exec(`
				CREATE TABLE IF NOT EXISTS enterprise_group_tenant_joins (
					group_id varchar(255) NOT NULL,
					tenant_id varchar(255) NOT NULL,
					department_id uuid,
					api_key_id uuid,
					created_at timestamptz DEFAULT CURRENT_TIMESTAMP,
					CONSTRAINT eg_tenant_pkey PRIMARY KEY (group_id, tenant_id),
					CONSTRAINT unique_tenant_in_group UNIQUE (tenant_id)
				);
				CREATE INDEX IF NOT EXISTS idx_enterprise_group_tenant_joins_group_id ON enterprise_group_tenant_joins(group_id);
				CREATE INDEX IF NOT EXISTS idx_enterprise_group_tenant_joins_tenant_id ON enterprise_group_tenant_joins(tenant_id);

				-- Restore data from workspaces
				INSERT INTO enterprise_group_tenant_joins (group_id, tenant_id, department_id, api_key_id)
				SELECT organization_id, id, department_id, api_key_id
				FROM workspaces
				WHERE organization_id IS NOT NULL AND organization_id != '';
			`).Error
		},
	}
}
