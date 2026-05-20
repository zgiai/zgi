package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0081_member_subscriptions adds account_id and installation_id columns to
// enterprise_group_plugin_subscriptions for member-level plugin subscriptions.
func M0096_member_subscriptions() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601210081",
		Migrate: func(tx *gorm.DB) error {
			// Add account_id for member-level subscriptions
			// Add installation_id to link to account_plugin_installations
			return tx.Exec(`
				-- Add account_id column (member who subscribed)
				ALTER TABLE enterprise_group_plugin_subscriptions
				ADD COLUMN IF NOT EXISTS account_id UUID;

				-- Add installation_id to link to account_plugin_installations
				ALTER TABLE enterprise_group_plugin_subscriptions
				ADD COLUMN IF NOT EXISTS installation_id UUID;

				-- Drop the old unique constraint on (group_id, plugin_id)
				ALTER TABLE enterprise_group_plugin_subscriptions
				DROP CONSTRAINT IF EXISTS idx_group_plugin_unique;

				-- Create new unique constraint: each member can subscribe once per installation
				ALTER TABLE enterprise_group_plugin_subscriptions
				ADD CONSTRAINT idx_group_account_installation_unique
				UNIQUE (group_id, account_id, installation_id);

				-- Index for querying member subscriptions
				CREATE INDEX IF NOT EXISTS idx_egps_account_id
					ON enterprise_group_plugin_subscriptions(account_id);

				-- Index for querying by installation
				CREATE INDEX IF NOT EXISTS idx_egps_installation_id
					ON enterprise_group_plugin_subscriptions(installation_id);

				COMMENT ON COLUMN enterprise_group_plugin_subscriptions.account_id IS
					'Member account ID who subscribed to the plugin';
				COMMENT ON COLUMN enterprise_group_plugin_subscriptions.installation_id IS
					'Reference to account_plugin_installations.id';
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				ALTER TABLE enterprise_group_plugin_subscriptions
				DROP CONSTRAINT IF EXISTS idx_group_account_installation_unique;

				DROP INDEX IF EXISTS idx_egps_account_id;
				DROP INDEX IF EXISTS idx_egps_installation_id;

				ALTER TABLE enterprise_group_plugin_subscriptions
				DROP COLUMN IF EXISTS account_id;

				ALTER TABLE enterprise_group_plugin_subscriptions
				DROP COLUMN IF EXISTS installation_id;

				ALTER TABLE enterprise_group_plugin_subscriptions
				ADD CONSTRAINT idx_group_plugin_unique UNIQUE (group_id, plugin_id);
			`).Error
		},
	}
}
