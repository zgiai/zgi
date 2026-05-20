package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0098_fix_llm_custom_org_fk fixes foreign key constraints for LLM custom tables.
//
// Problem:
// - llm_custom_providers.organization_id and llm_custom_models.organization_id were constrained to workspaces(id)
// - the codebase uses organization_id values from organizations(id)
//
// This migration:
// 1) backfills organization_id if existing rows still store workspace ids
// 2) switches FK targets from workspaces(id) to organizations(id)
func M0098_fix_llm_custom_org_fk() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260202000098",
		Migrate: func(tx *gorm.DB) error {
			// If there are legacy rows where organization_id actually stores a workspace id,
			// convert it to the owning organization id.
			if err := tx.Exec(`
				UPDATE llm_custom_providers cp
				SET organization_id = w.organization_id
				FROM workspaces w
				WHERE cp.organization_id = w.id
				  AND w.organization_id IS NOT NULL
			`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`
				UPDATE llm_custom_models cm
				SET organization_id = w.organization_id
				FROM workspaces w
				WHERE cm.organization_id = w.id
				  AND w.organization_id IS NOT NULL
			`).Error; err != nil {
				return err
			}

			// Drop old constraints referencing workspaces(id)
			if err := tx.Exec(`
				ALTER TABLE llm_custom_providers
				DROP CONSTRAINT IF EXISTS fk_tenant_custom_provider_tenant
			`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`
				ALTER TABLE llm_custom_models
				DROP CONSTRAINT IF EXISTS fk_tenant_custom_model_tenant
			`).Error; err != nil {
				return err
			}

			// Add new constraints referencing organizations(id)
			if err := tx.Exec(`
				ALTER TABLE llm_custom_providers
				ADD CONSTRAINT fk_llm_custom_providers_organization
				FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE
			`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`
				ALTER TABLE llm_custom_models
				ADD CONSTRAINT fk_llm_custom_models_organization
				FOREIGN KEY (organization_id) REFERENCES organizations(id) ON DELETE CASCADE
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Revert FK targets back to workspaces(id)
			if err := tx.Exec(`
				ALTER TABLE llm_custom_providers
				DROP CONSTRAINT IF EXISTS fk_llm_custom_providers_organization
			`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`
				ALTER TABLE llm_custom_models
				DROP CONSTRAINT IF EXISTS fk_llm_custom_models_organization
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				ALTER TABLE llm_custom_providers
				ADD CONSTRAINT fk_tenant_custom_provider_tenant
				FOREIGN KEY (organization_id) REFERENCES workspaces(id) ON DELETE CASCADE
			`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`
				ALTER TABLE llm_custom_models
				ADD CONSTRAINT fk_tenant_custom_model_tenant
				FOREIGN KEY (organization_id) REFERENCES workspaces(id) ON DELETE CASCADE
			`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
