package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0051FixSystemChannelProtocol fills the protocol field for system channels
// based on provider mapping (most providers use openai protocol)
func M0051FixSystemChannelProtocol() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251216000051",
		Migrate: func(tx *gorm.DB) error {
			// Check if llm_system_channels table exists
			var exists bool
			err := tx.Raw(`
				SELECT EXISTS (
					SELECT FROM information_schema.tables 
					WHERE table_schema = CURRENT_SCHEMA() 
					AND table_name = 'llm_system_channels'
				)
			`).Scan(&exists).Error
			if err != nil {
				return err
			}

			// Skip if table doesn't exist
			if !exists {
				return nil
			}

			// Check if protocol column exists
			var colExists bool
			err = tx.Raw(`
				SELECT EXISTS (
					SELECT FROM information_schema.columns 
					WHERE table_schema = CURRENT_SCHEMA() 
					AND table_name = 'llm_system_channels'
					AND column_name = 'protocol'
				)
			`).Scan(&colExists).Error
			if err != nil {
				return err
			}

			// Skip if column doesn't exist
			if !colExists {
				return nil
			}

			// Default to 'openai' protocol for most providers (OpenAI-compatible)
			if err := tx.Exec(`
				UPDATE llm_system_channels
				SET protocol = 'openai'
				WHERE protocol IS NULL OR protocol = ''
			`).Error; err != nil {
				return err
			}

			// Set anthropic protocol for anthropic provider
			if err := tx.Exec(`
				UPDATE llm_system_channels
				SET protocol = 'anthropic'
				WHERE provider = 'anthropic'
			`).Error; err != nil {
				return err
			}

			// Set google protocol for google provider
			if err := tx.Exec(`
				UPDATE llm_system_channels
				SET protocol = 'google'
				WHERE provider IN ('google', 'google-vertex')
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				UPDATE llm_system_channels SET protocol = ''
			`).Error
		},
	}
}
