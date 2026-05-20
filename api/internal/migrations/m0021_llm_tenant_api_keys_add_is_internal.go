package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0021LLMTenantAPIKeysAddIsInternal() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251127000002",
		Migrate: func(tx *gorm.DB) error {
			// Add is_internal column
			if err := tx.Exec(`ALTER TABLE llm_tenant_api_keys ADD COLUMN IF NOT EXISTS is_internal BOOLEAN NOT NULL DEFAULT FALSE`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop is_internal column
			if err := tx.Exec(`ALTER TABLE llm_tenant_api_keys DROP COLUMN is_internal`).Error; err != nil {
				return err
			}
			return nil
		},
	}
}
