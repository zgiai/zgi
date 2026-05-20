package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0072_fix_llm_tenant_model_settings() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260202000073",
		Migrate: func(tx *gorm.DB) error {
			// Rename tenant_id to organization_id in llm_tenant_model_settings
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_model_settings 
				RENAME COLUMN tenant_id TO organization_id;
			`).Error; err != nil {
				return err
			}

			// Rename indexes
			sqls := []string{
				`ALTER INDEX IF EXISTS idx_tenant_model_connected RENAME TO idx_organization_model_connected;`,
				`ALTER INDEX IF EXISTS idx_tenant_model_setting RENAME TO idx_organization_model_setting;`,
				`ALTER INDEX IF EXISTS idx_tenant_model_visible RENAME TO idx_organization_model_visible;`,
				`ALTER INDEX IF EXISTS llm_tenant_model_settings_tenant_id_model_id_key RENAME TO llm_tenant_model_settings_organization_id_model_id_key;`,
			}

			for _, sql := range sqls {
				if err := tx.Exec(sql).Error; err != nil {
					// Log but don't fail if index doesn't exist
					continue
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Rollback: rename back to tenant_id
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_model_settings 
				RENAME COLUMN organization_id TO tenant_id;
			`).Error; err != nil {
				return err
			}

			// Rename indexes back
			sqls := []string{
				`ALTER INDEX IF EXISTS idx_organization_model_connected RENAME TO idx_tenant_model_connected;`,
				`ALTER INDEX IF EXISTS idx_organization_model_setting RENAME TO idx_tenant_model_setting;`,
				`ALTER INDEX IF EXISTS idx_organization_model_visible RENAME TO idx_tenant_model_visible;`,
				`ALTER INDEX IF EXISTS llm_tenant_model_settings_organization_id_model_id_key RENAME TO llm_tenant_model_settings_tenant_id_model_id_key;`,
			}

			for _, sql := range sqls {
				if err := tx.Exec(sql).Error; err != nil {
					continue
				}
			}

			return nil
		},
	}
}
