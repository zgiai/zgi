package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0048_create_channel_groups_table creates the llm_channel_groups table
func M0048_create_channel_groups_table() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251216000048",
		Migrate: func(tx *gorm.DB) error {
			// Create channel groups table
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS llm_channel_groups (
					id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
					name VARCHAR(100) NOT NULL UNIQUE,
					display_name VARCHAR(200) NOT NULL,
					description TEXT,
					priority INT NOT NULL DEFAULT 10,
					is_active BOOLEAN NOT NULL DEFAULT true,
					created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
					deleted_at TIMESTAMP WITH TIME ZONE
				)
			`).Error; err != nil {
				return err
			}

			// Create index on name
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_llm_channel_groups_name ON llm_channel_groups(name)
			`).Error; err != nil {
				return err
			}

			// Create index on is_active
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_llm_channel_groups_is_active ON llm_channel_groups(is_active)
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE IF EXISTS llm_channel_groups`).Error
		},
	}
}
