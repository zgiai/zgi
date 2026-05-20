package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0072_add_route_sync_mode adds sync_mode and last_synced_at fields to llm_tenant_routes
func M0073_add_route_sync_mode() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260107000072",
		Migrate: func(tx *gorm.DB) error {
			// Add sync_mode field (default: snapshot)
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_routes
				ADD COLUMN IF NOT EXISTS sync_mode VARCHAR(20) DEFAULT 'snapshot'
			`).Error; err != nil {
				return err
			}

			// Add last_synced_at field
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_routes
				ADD COLUMN IF NOT EXISTS last_synced_at TIMESTAMP
			`).Error; err != nil {
				return err
			}

			// Initialize existing routes with snapshot mode
			if err := tx.Exec(`
				UPDATE llm_tenant_routes
				SET sync_mode = 'snapshot',
				    last_synced_at = NOW()
				WHERE sync_mode IS NULL
			`).Error; err != nil {
				return err
			}

			// Copy system channel models to routes with empty models
			if err := tx.Exec(`
				UPDATE llm_tenant_routes tr
				SET
					models = sc.models,
					last_synced_at = NOW()
				FROM llm_system_channels sc
				WHERE tr.system_channel_id = sc.id
				  AND tr.system_channel_id IS NOT NULL
				  AND (tr.models IS NULL OR jsonb_array_length(tr.models) = 0)
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_routes DROP COLUMN IF EXISTS sync_mode
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				ALTER TABLE llm_tenant_routes DROP COLUMN IF EXISTS last_synced_at
			`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
