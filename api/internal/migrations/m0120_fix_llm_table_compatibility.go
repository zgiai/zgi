package migrations

import (
	"fmt"
	"strings"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0120_fix_llm_table_compatibility repairs environments that still use
// llm_tenant_* table names/columns while code expects llm_* with organization_id.
func M0120_fix_llm_table_compatibility() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260306000120",
		Migrate: func(tx *gorm.DB) error {
			if err := renameTableIfNeeded(tx, "llm_tenant_provider_configs", "llm_provider_configs"); err != nil {
				return err
			}
			if err := renameTableIfNeeded(tx, "llm_tenant_model_configs", "llm_model_configs"); err != nil {
				return err
			}
			if err := renameTableIfNeeded(tx, "llm_tenant_custom_providers", "llm_custom_providers"); err != nil {
				return err
			}
			if err := renameTableIfNeeded(tx, "llm_tenant_custom_models", "llm_custom_models"); err != nil {
				return err
			}

			if err := renameColumnIfNeeded(tx, "llm_provider_configs", "tenant_id", "organization_id"); err != nil {
				return err
			}
			if err := renameColumnIfNeeded(tx, "llm_model_configs", "tenant_id", "organization_id"); err != nil {
				return err
			}
			if err := renameColumnIfNeeded(tx, "llm_custom_providers", "tenant_id", "organization_id"); err != nil {
				return err
			}
			if err := renameColumnIfNeeded(tx, "llm_custom_models", "tenant_id", "organization_id"); err != nil {
				return err
			}

			if err := renameColumnIfNeeded(tx, "llm_custom_providers", "name", "provider"); err != nil {
				return err
			}
			if err := renameColumnIfNeeded(tx, "llm_custom_providers", "display_name", "provider_name"); err != nil {
				return err
			}
			if err := tx.Exec(`ALTER TABLE IF EXISTS llm_custom_providers ADD COLUMN IF NOT EXISTS api_base_url VARCHAR(255)`).Error; err != nil {
				return err
			}

			if err := renameColumnIfNeeded(tx, "llm_custom_models", "vision", "supports_vision"); err != nil {
				return err
			}
			if err := renameColumnIfNeeded(tx, "llm_custom_models", "function_calling", "supports_tool_call"); err != nil {
				return err
			}
			if err := renameColumnIfNeeded(tx, "llm_custom_models", "reasoning", "supports_reasoning"); err != nil {
				return err
			}

			sqls := []string{
				`UPDATE llm_provider_configs pc
				 SET organization_id = w.organization_id
				 FROM workspaces w
				 WHERE pc.organization_id = w.id
				   AND w.organization_id IS NOT NULL`,
				`UPDATE llm_model_configs mc
				 SET organization_id = w.organization_id
				 FROM workspaces w
				 WHERE mc.organization_id = w.id
				   AND w.organization_id IS NOT NULL`,
				`UPDATE llm_custom_providers cp
				 SET organization_id = w.organization_id
				 FROM workspaces w
				 WHERE cp.organization_id = w.id
				   AND w.organization_id IS NOT NULL`,
				`UPDATE llm_custom_models cm
				 SET organization_id = w.organization_id
				 FROM workspaces w
				 WHERE cm.organization_id = w.id
				   AND w.organization_id IS NOT NULL`,

				`ALTER TABLE IF EXISTS llm_custom_providers
				 DROP CONSTRAINT IF EXISTS fk_tenant_custom_provider_tenant`,
				`ALTER TABLE IF EXISTS llm_custom_models
				 DROP CONSTRAINT IF EXISTS fk_tenant_custom_model_tenant`,

				`ALTER TABLE IF EXISTS llm_custom_providers
				 ADD CONSTRAINT fk_llm_custom_providers_organization
				 FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE`,
				`ALTER TABLE IF EXISTS llm_custom_models
				 ADD CONSTRAINT fk_llm_custom_models_organization
				 FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE`,

				`CREATE INDEX IF NOT EXISTS idx_llm_provider_configs_organization_id ON llm_provider_configs(organization_id)`,
				`CREATE INDEX IF NOT EXISTS idx_llm_model_configs_organization_id ON llm_model_configs(organization_id)`,
				`CREATE INDEX IF NOT EXISTS idx_llm_custom_providers_organization_id ON llm_custom_providers(organization_id)`,
				`CREATE INDEX IF NOT EXISTS idx_llm_custom_models_organization_id ON llm_custom_models(organization_id)`,
			}

			for _, sql := range sqls {
				if err := tx.Exec(sql).Error; err != nil {
					// ADD CONSTRAINT IF NOT EXISTS is not universally available; ignore duplicate-constraint errors.
					if isDuplicateConstraintError(err) {
						continue
					}
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// This compatibility migration is intentionally one-way.
			return nil
		},
	}
}

func renameTableIfNeeded(tx *gorm.DB, oldName, newName string) error {
	oldExists, err := tableExists(tx, oldName)
	if err != nil {
		return err
	}
	if !oldExists {
		return nil
	}

	newExists, err := tableExists(tx, newName)
	if err != nil {
		return err
	}
	if newExists {
		return nil
	}

	return tx.Exec(fmt.Sprintf(`ALTER TABLE %s RENAME TO %s`, oldName, newName)).Error
}

func renameColumnIfNeeded(tx *gorm.DB, tableName, oldColumn, newColumn string) error {
	tExists, err := tableExists(tx, tableName)
	if err != nil {
		return err
	}
	if !tExists {
		return nil
	}

	oldExists, err := columnExists(tx, tableName, oldColumn)
	if err != nil {
		return err
	}
	if !oldExists {
		return nil
	}

	newExists, err := columnExists(tx, tableName, newColumn)
	if err != nil {
		return err
	}
	if newExists {
		return nil
	}

	return tx.Exec(fmt.Sprintf(`ALTER TABLE %s RENAME COLUMN %s TO %s`, tableName, oldColumn, newColumn)).Error
}

func tableExists(tx *gorm.DB, tableName string) (bool, error) {
	var count int64
	err := tx.Raw(`
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_name = ?
	`, tableName).Scan(&count).Error
	return count > 0, err
}

func columnExists(tx *gorm.DB, tableName, columnName string) (bool, error) {
	var count int64
	err := tx.Raw(`
		SELECT COUNT(*)
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND table_name = ?
		  AND column_name = ?
	`, tableName, columnName).Scan(&count).Error
	return count > 0, err
}

func isDuplicateConstraintError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "already exists") && strings.Contains(msg, "constraint")
}
