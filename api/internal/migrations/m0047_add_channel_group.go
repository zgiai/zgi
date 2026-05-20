package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0047_add_channel_group adds channel group fields to llm_system_channels
// This enables aggregating multiple system channels into one "official" channel
// for tenant display, with internal load balancing
func M0047_add_channel_group() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251216000047",
		Migrate: func(tx *gorm.DB) error {
			// Add channel_group fields to llm_system_channels
			if err := tx.Exec(`
				ALTER TABLE llm_system_channels
				ADD COLUMN IF NOT EXISTS channel_group VARCHAR(100),
				ADD COLUMN IF NOT EXISTS channel_group_name VARCHAR(200),
				ADD COLUMN IF NOT EXISTS channel_group_description TEXT,
				ADD COLUMN IF NOT EXISTS channel_group_priority INT DEFAULT 0
			`).Error; err != nil {
				return err
			}

			// Add index for channel_group queries
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_sys_channels_group
				ON llm_system_channels(channel_group)
				WHERE channel_group IS NOT NULL AND deleted_at IS NULL
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`DROP INDEX IF EXISTS idx_sys_channels_group`).Error; err != nil {
				return err
			}
			return tx.Exec(`
				ALTER TABLE llm_system_channels
				DROP COLUMN IF EXISTS channel_group,
				DROP COLUMN IF EXISTS channel_group_name,
				DROP COLUMN IF EXISTS channel_group_description,
				DROP COLUMN IF EXISTS channel_group_priority
			`).Error
		},
	}
}
