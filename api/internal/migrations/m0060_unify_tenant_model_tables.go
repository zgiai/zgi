package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0060_unify_tenant_model_tables migrates data from legacy tables to the new unified tables:
// - llm_tenant_models -> llm_tenant_model_configs (uses model_id UUID instead of provider+model strings)
// - llm_tenant_providers -> llm_tenant_provider_configs (uses provider_id UUID instead of provider string)
// This fixes the FK design issue where string-based foreign keys can break when names change.
func M0060_unify_tenant_model_tables() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251222000060",
		Migrate: func(tx *gorm.DB) error {
			// ============================================================
			// Part 1: Migrate Model data
			// ============================================================
			// Migrate records from llm_tenant_models to llm_tenant_model_configs
			// Only migrate records that don't already exist in the new table
			if err := tx.Exec(`
				INSERT INTO llm_tenant_model_configs (
					id,
					tenant_id,
					model_id,
					is_enabled,
					sort_order,
					created_at,
					updated_at
				)
				SELECT
					uuid_generate_v4(),
					tm.tenant_id,
					m.id,
					tm.is_enabled,
					0,
					tm.created_at,
					tm.updated_at
				FROM llm_tenant_models tm
				JOIN llm_models m ON m.provider = tm.provider AND m.name = tm.model
				LEFT JOIN llm_tenant_model_configs tmc
					ON tmc.tenant_id = tm.tenant_id AND tmc.model_id = m.id
				WHERE tm.deleted_at IS NULL
					AND m.deleted_at IS NULL
					AND tmc.id IS NULL
			`).Error; err != nil {
				return err
			}

			// ============================================================
			// Part 2: Migrate Provider data
			// ============================================================
			// Migrate records from llm_tenant_providers to llm_tenant_provider_configs
			// Only migrate records that don't already exist in the new table
			if err := tx.Exec(`
				INSERT INTO llm_tenant_provider_configs (
					id,
					tenant_id,
					provider_id,
					is_enabled,
					sort_order,
					created_at,
					updated_at
				)
				SELECT
					uuid_generate_v4(),
					tp.tenant_id,
					p.id,
					tp.is_enabled,
					COALESCE(tp.sort_order, 0),
					tp.created_at,
					tp.updated_at
				FROM llm_tenant_providers tp
				JOIN llm_providers p ON p.name = tp.provider
				LEFT JOIN llm_tenant_provider_configs tpc
					ON tpc.tenant_id = tp.tenant_id AND tpc.provider_id = p.id
				WHERE tp.deleted_at IS NULL
					AND p.deleted_at IS NULL
					AND tpc.id IS NULL
			`).Error; err != nil {
				return err
			}

			// ============================================================
			// Part 3: Mark old tables as deprecated
			// ============================================================
			// We don't drop the tables yet to allow for rollback
			_ = tx.Exec(`
				COMMENT ON TABLE llm_tenant_models IS
				'DEPRECATED: Use llm_tenant_model_configs instead. This table will be removed in a future version.'
			`)

			_ = tx.Exec(`
				COMMENT ON TABLE llm_tenant_providers IS
				'DEPRECATED: Use llm_tenant_provider_configs instead. This table will be removed in a future version.'
			`)

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Remove the deprecation comments
			_ = tx.Exec(`COMMENT ON TABLE llm_tenant_models IS NULL`)
			_ = tx.Exec(`COMMENT ON TABLE llm_tenant_providers IS NULL`)

			// Note: We don't delete the migrated records because they may have been
			// modified through the new table. Manual cleanup may be needed if rollback is required.
			return nil
		},
	}
}
