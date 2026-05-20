package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0029_add_model_governance adds two-layer governance for model visibility
func M0029_add_model_governance() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251207000029",
		Migrate: func(tx *gorm.DB) error {
			// 1. Create llm_tenant_model_settings table
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS llm_tenant_model_settings (
					id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
					tenant_id UUID NOT NULL,
					model_id UUID NOT NULL REFERENCES llm_models(id) ON DELETE CASCADE,

					-- Two-layer governance
					is_connected BOOLEAN DEFAULT false,
					is_visible BOOLEAN DEFAULT false,

					-- Access control
					access_scope VARCHAR(50) DEFAULT 'all',
					visible_groups JSONB DEFAULT '[]',
					visible_users JSONB DEFAULT '[]',

					-- Routing preferences
					routing_weight INT DEFAULT 1,
					routing_mode VARCHAR(20),

					-- Custom configuration
					custom_alias VARCHAR(100),
					custom_pricing JSONB DEFAULT '{}',

					-- Metadata
					notes TEXT,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

					UNIQUE(tenant_id, model_id)
				)
			`).Error; err != nil {
				return err
			}

			// 2. Create indexes
			indexes := []string{
				`CREATE INDEX IF NOT EXISTS idx_tenant_model_setting ON llm_tenant_model_settings(tenant_id, model_id)`,
				`CREATE INDEX IF NOT EXISTS idx_tenant_model_connected ON llm_tenant_model_settings(tenant_id, is_connected)`,
				`CREATE INDEX IF NOT EXISTS idx_tenant_model_visible ON llm_tenant_model_settings(tenant_id, is_visible)`,
				`CREATE INDEX IF NOT EXISTS idx_tenant_model_scope ON llm_tenant_model_settings(access_scope)`,
				`CREATE INDEX IF NOT EXISTS idx_tenant_model_groups ON llm_tenant_model_settings USING GIN (visible_groups)`,
				`CREATE INDEX IF NOT EXISTS idx_tenant_model_users ON llm_tenant_model_settings USING GIN (visible_users)`,
			}

			for _, idx := range indexes {
				if err := tx.Exec(idx).Error; err != nil {
					return err
				}
			}

			// 3. Migrate existing data from llm_tenant_channels
			// For each enabled channel, mark its models as connected and visible
			if err := tx.Exec(`
				INSERT INTO llm_tenant_model_settings (tenant_id, model_id, is_connected, is_visible, access_scope)
				SELECT DISTINCT
					c.tenant_id,
					m.id,
					true,
					true,
					'all'
				FROM llm_tenant_channels c
				CROSS JOIN LATERAL jsonb_array_elements_text(c.models) AS model_name
				JOIN llm_models m ON m.name = model_name
				WHERE c.is_enabled = true
				ON CONFLICT (tenant_id, model_id) DO NOTHING
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE IF EXISTS llm_tenant_model_settings`).Error
		},
	}
}
