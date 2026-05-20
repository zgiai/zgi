package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0018_llm_tenant_api_keys_add_key_hash adds key_hash column to llm_tenant_api_keys table
func M0018_llm_tenant_api_keys_add_key_hash() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251126160000",
		Migrate: func(tx *gorm.DB) error {
			// 1. Add key_hash column
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_api_keys 
				ADD COLUMN IF NOT EXISTS key_hash VARCHAR(64)
			`).Error; err != nil {
				return err
			}

			// 2. Create unique index for key_hash
			if err := tx.Exec(`
				CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_tenant_api_keys_key_hash 
				ON llm_tenant_api_keys(key_hash)
			`).Error; err != nil {
				return err
			}

			// 3. Increase key column length to 255 (encrypted key is > 100 chars)
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_api_keys 
				ALTER COLUMN key TYPE VARCHAR(255)
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// 1. Revert key column length to 48
			// Note: This might fail if there are keys longer than 48 chars,
			// but rollback implies reverting state.
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_api_keys 
				ALTER COLUMN key TYPE VARCHAR(48)
			`).Error; err != nil {
				return err
			}

			// 2. Drop index
			if err := tx.Exec(`DROP INDEX IF EXISTS idx_llm_tenant_api_keys_key_hash`).Error; err != nil {
				return err
			}

			// 3. Drop column
			if err := tx.Exec(`ALTER TABLE llm_tenant_api_keys DROP COLUMN IF EXISTS key_hash`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
