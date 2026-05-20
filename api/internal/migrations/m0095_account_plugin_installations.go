package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0080_account_plugin_installations creates the account_plugin_installations table
// for storing the relationship between accounts and installed plugins.
func M0095_account_plugin_installations() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260123000080",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				CREATE TABLE IF NOT EXISTS account_plugin_installations (
					id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					tenant_id UUID NOT NULL,
					marketplace_plugin_id UUID NOT NULL,
					marketplace_version_id UUID NOT NULL,
					installed_by UUID NOT NULL,
					installed_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
					status VARCHAR(20) DEFAULT 'active',
					CONSTRAINT uq_account_plugin_installations_tenant_version
						UNIQUE (tenant_id, marketplace_version_id)
				);

				CREATE INDEX IF NOT EXISTS idx_account_plugin_installations_tenant
					ON account_plugin_installations(tenant_id);

				CREATE INDEX IF NOT EXISTS idx_account_plugin_installations_plugin
					ON account_plugin_installations(tenant_id, marketplace_plugin_id);

				CREATE INDEX IF NOT EXISTS idx_account_plugin_installations_version
					ON account_plugin_installations(marketplace_version_id);

				COMMENT ON TABLE account_plugin_installations IS
					'Tenant to plugin installation relationships';
				COMMENT ON COLUMN account_plugin_installations.tenant_id IS
					'Organization/Tenant ID where the plugin is installed';
				COMMENT ON COLUMN account_plugin_installations.marketplace_plugin_id IS
					'Marketplace plugin ID for redundancy and easy querying';
				COMMENT ON COLUMN account_plugin_installations.marketplace_version_id IS
					'Marketplace version ID - lookup declaration via plugin_declarations table';
				COMMENT ON COLUMN account_plugin_installations.installed_by IS
					'User ID who installed this plugin';
				COMMENT ON COLUMN account_plugin_installations.status IS
					'Installation status: active, disabled';
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE IF EXISTS account_plugin_installations`).Error
		},
	}
}
