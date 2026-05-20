package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0127_fix_llm_organization_api_keys_compatibility repairs environments where
// the tenant->organization rename never reached llm_tenant_api_keys.
func M0127_fix_llm_organization_api_keys_compatibility() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260306000127",
		Migrate: func(tx *gorm.DB) error {
			if err := renameTableIfNeeded(tx, "llm_tenant_api_keys", "llm_organization_api_keys"); err != nil {
				return err
			}

			if err := renameColumnIfNeeded(tx, "llm_organization_api_keys", "tenant_id", "organization_id"); err != nil {
				return err
			}

			sqls := []string{
				`ALTER TABLE IF EXISTS llm_organization_api_keys
					ADD COLUMN IF NOT EXISTS organization_id UUID`,
				`CREATE INDEX IF NOT EXISTS idx_llm_organization_api_keys_organization_id
					ON llm_organization_api_keys(organization_id)`,
				`CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_organization_api_keys_key_hash
					ON llm_organization_api_keys(key_hash)
					WHERE deleted_at IS NULL AND key_hash IS NOT NULL`,
			}

			for _, sql := range sqls {
				if err := tx.Exec(sql).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
