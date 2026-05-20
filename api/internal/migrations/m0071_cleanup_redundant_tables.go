package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0071_cleanup_redundant_tables removes redundant tables and ChannelGroup
func M0071_cleanup_redundant_tables() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260202000071",
		Migrate: func(tx *gorm.DB) error {
			// 1. Drop the channel_group_id column
			if err := tx.Exec(`
				ALTER TABLE llm_system_channels 
				DROP COLUMN IF EXISTS channel_group_id
			`).Error; err != nil {
				return err
			}

			// 2. Drop the ChannelGroup table
			if err := tx.Exec(`
				DROP TABLE IF EXISTS llm_channel_groups CASCADE
			`).Error; err != nil {
				return err
			}

			// 3. Drop redundant configuration tables
			if err := tx.Exec(`
				DROP TABLE IF EXISTS llm_tenant_providers CASCADE
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				DROP TABLE IF EXISTS llm_tenant_models CASCADE
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				DROP TABLE IF EXISTS llm_tenant_channels CASCADE
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Rollback is complex; restore from backup instead
			return nil
		},
	}
}
