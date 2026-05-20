package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0124_fix_llm_credentials_compatibility aligns llm_credentials with the
// organization-scoped schema expected by the current channel/credential services.
func M0124_fix_llm_credentials_compatibility() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260306000124",
		Migrate: func(tx *gorm.DB) error {
			if err := renameTableIfNeeded(tx, "llm_tenant_credentials", "llm_credentials"); err != nil {
				return err
			}

			if err := renameColumnIfNeeded(tx, "llm_credentials", "tenant_id", "organization_id"); err != nil {
				return err
			}

			sqls := []string{
				`ALTER TABLE IF EXISTS llm_credentials
					ADD COLUMN IF NOT EXISTS organization_id UUID`,
				`ALTER TABLE IF EXISTS llm_credentials
					ADD COLUMN IF NOT EXISTS protocol VARCHAR(50)`,
				`UPDATE llm_credentials c
				 SET organization_id = w.organization_id
				 FROM workspaces w
				 WHERE c.organization_id = w.id
				   AND w.organization_id IS NOT NULL`,
				`CREATE INDEX IF NOT EXISTS idx_llm_credentials_organization_id
					ON llm_credentials(organization_id)`,
				`CREATE INDEX IF NOT EXISTS idx_llm_credentials_org_hash
					ON llm_credentials(organization_id, api_key_hash)
					WHERE deleted_at IS NULL`,
				`CREATE INDEX IF NOT EXISTS idx_credential_protocol
					ON llm_credentials(protocol)`,
			}

			for _, sql := range sqls {
				if err := tx.Exec(sql).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Compatibility migration is intentionally one-way.
			return nil
		},
	}
}
