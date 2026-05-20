package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0053RemoveDeprecatedFields removes deprecated fields from LLM tables:
// - llm_models: scope, source_id, is_custom, synced_at, sync_version (L1/L2/L3 architecture)
// - llm_tenant_routes: provider_type, global_provider_id, custom_provider_id (unused fields)
func M0053RemoveDeprecatedFields() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251216000053",
		Migrate: func(tx *gorm.DB) error {
			// Remove L1/L2/L3 architecture fields from llm_models
			modelDropStatements := []string{
				`DROP INDEX IF EXISTS idx_model_scope`,
				`DROP INDEX IF EXISTS idx_model_source`,
				`DROP INDEX IF EXISTS idx_model_custom`,
				`DROP INDEX IF EXISTS idx_model_synced`,
				`ALTER TABLE llm_models DROP COLUMN IF EXISTS scope`,
				`ALTER TABLE llm_models DROP COLUMN IF EXISTS source_id`,
				`ALTER TABLE llm_models DROP COLUMN IF EXISTS is_custom`,
				`ALTER TABLE llm_models DROP COLUMN IF EXISTS synced_at`,
				`ALTER TABLE llm_models DROP COLUMN IF EXISTS sync_version`,
			}

			for _, stmt := range modelDropStatements {
				if err := tx.Exec(stmt).Error; err != nil {
					return err
				}
			}

			// Remove unused provider reference fields from llm_tenant_routes
			routeDropStatements := []string{
				`DROP INDEX IF EXISTS idx_llm_tenant_routes_provider_type`,
				`DROP INDEX IF EXISTS idx_llm_tenant_routes_global_provider_id`,
				`DROP INDEX IF EXISTS idx_llm_tenant_routes_custom_provider_id`,
				`ALTER TABLE llm_tenant_routes DROP COLUMN IF EXISTS provider_type`,
				`ALTER TABLE llm_tenant_routes DROP COLUMN IF EXISTS global_provider_id`,
				`ALTER TABLE llm_tenant_routes DROP COLUMN IF EXISTS custom_provider_id`,
			}

			for _, stmt := range routeDropStatements {
				if err := tx.Exec(stmt).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Restore L1/L2/L3 architecture fields to llm_models
			modelRestoreStatements := []string{
				`ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS scope VARCHAR(50) NOT NULL DEFAULT 'global'`,
				`ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS source_id UUID`,
				`ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS is_custom BOOLEAN DEFAULT false`,
				`ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS synced_at TIMESTAMP`,
				`ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS sync_version INTEGER DEFAULT 1`,
				`CREATE INDEX IF NOT EXISTS idx_model_scope ON llm_models(scope)`,
				`CREATE INDEX IF NOT EXISTS idx_model_source ON llm_models(source_id)`,
				`CREATE INDEX IF NOT EXISTS idx_model_custom ON llm_models(is_custom)`,
				`CREATE INDEX IF NOT EXISTS idx_model_synced ON llm_models(synced_at)`,
			}

			for _, stmt := range modelRestoreStatements {
				if err := tx.Exec(stmt).Error; err != nil {
					return err
				}
			}

			// Restore provider reference fields to llm_tenant_routes
			routeRestoreStatements := []string{
				`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS provider_type VARCHAR(20)`,
				`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS global_provider_id UUID`,
				`ALTER TABLE llm_tenant_routes ADD COLUMN IF NOT EXISTS custom_provider_id UUID`,
				`CREATE INDEX IF NOT EXISTS idx_llm_tenant_routes_provider_type ON llm_tenant_routes(provider_type)`,
				`CREATE INDEX IF NOT EXISTS idx_llm_tenant_routes_global_provider_id ON llm_tenant_routes(global_provider_id)`,
				`CREATE INDEX IF NOT EXISTS idx_llm_tenant_routes_custom_provider_id ON llm_tenant_routes(custom_provider_id)`,
			}

			for _, stmt := range routeRestoreStatements {
				if err := tx.Exec(stmt).Error; err != nil {
					return err
				}
			}

			return nil
		},
	}
}
