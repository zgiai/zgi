package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0077_org_plugin_subscriptions creates the enterprise_group_plugin_subscriptions table
// for organization-level plugin subscriptions.
func M0077_org_plugin_subscriptions() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601130077",
		Migrate: func(tx *gorm.DB) error {
			// Create organization-level plugin subscriptions table
			return tx.Exec(`
				CREATE TABLE IF NOT EXISTS enterprise_group_plugin_subscriptions (
					id SERIAL PRIMARY KEY,
					group_id UUID NOT NULL,
					plugin_id VARCHAR(255) NOT NULL,
					enabled BOOLEAN NOT NULL DEFAULT true,
					config TEXT,
					subscribed_by UUID,
					created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
					CONSTRAINT idx_group_plugin_unique UNIQUE (group_id, plugin_id)
				);

				CREATE INDEX IF NOT EXISTS idx_egps_group_id 
					ON enterprise_group_plugin_subscriptions(group_id);
				
				CREATE INDEX IF NOT EXISTS idx_egps_plugin_id 
					ON enterprise_group_plugin_subscriptions(plugin_id);
				
				CREATE INDEX IF NOT EXISTS idx_egps_enabled 
					ON enterprise_group_plugin_subscriptions(group_id, enabled) 
					WHERE enabled = true;

				COMMENT ON TABLE enterprise_group_plugin_subscriptions IS 
					'Plugin subscriptions at organization level';
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE IF EXISTS enterprise_group_plugin_subscriptions`).Error
		},
	}
}
