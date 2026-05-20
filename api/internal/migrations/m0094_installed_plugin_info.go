package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0079_installed_plugin_info creates the installed_plugin_info table
// for storing installed plugin metadata (one record per plugin version).
func M0094_installed_plugin_info() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601220079",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(`
				CREATE TABLE IF NOT EXISTS installed_plugin_info (
					id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					marketplace_plugin_id UUID,
					marketplace_version_id UUID NOT NULL UNIQUE,
					plugin_name VARCHAR(100) NOT NULL,
					plugin_version VARCHAR(50) NOT NULL,
					plugin_author VARCHAR(100),
					declaration JSONB NOT NULL,
					created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
				);

				CREATE INDEX IF NOT EXISTS idx_installed_plugin_info_marketplace_plugin
					ON installed_plugin_info(marketplace_plugin_id);

				CREATE INDEX IF NOT EXISTS idx_installed_plugin_info_plugin_name
					ON installed_plugin_info(plugin_name);

				COMMENT ON TABLE installed_plugin_info IS
					'Installed plugin info, one record per marketplace version';
				COMMENT ON COLUMN installed_plugin_info.marketplace_plugin_id IS
					'Marketplace plugin ID for redundancy';
				COMMENT ON COLUMN installed_plugin_info.declaration IS
					'JSONB containing provider and tools definition';
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE IF EXISTS installed_plugin_info`).Error
		},
	}
}
