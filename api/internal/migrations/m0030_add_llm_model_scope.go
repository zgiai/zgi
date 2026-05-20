package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0030_add_llm_model_scope adds scope and related fields to llm_models table for L1/L2/L3 architecture
func M0030_add_llm_model_scope() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251207000030",
		Migrate: func(tx *gorm.DB) error {
			sqls := []string{
				// Add scope field for L1/L2/L3 architecture
				`ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS scope VARCHAR(50) NOT NULL DEFAULT 'global'`,
				`CREATE INDEX IF NOT EXISTS idx_model_scope ON llm_models(scope)`,

				// Add source_id field (points to L1 model if subscribed)
				`ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS source_id UUID`,
				`CREATE INDEX IF NOT EXISTS idx_model_source ON llm_models(source_id)`,

				// Add is_custom field
				`ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS is_custom BOOLEAN DEFAULT false`,
				`CREATE INDEX IF NOT EXISTS idx_model_custom ON llm_models(is_custom)`,

				// Add synced_at field
				`ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS synced_at TIMESTAMP`,
				`CREATE INDEX IF NOT EXISTS idx_model_synced ON llm_models(synced_at)`,

				// Add sync_version field
				`ALTER TABLE llm_models ADD COLUMN IF NOT EXISTS sync_version INTEGER DEFAULT 1`,

				// Update existing models to have 'global' scope
				`UPDATE llm_models SET scope = 'global' WHERE scope IS NULL OR scope = ''`,
			}

			for _, sql := range sqls {
				if err := tx.Exec(sql).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			sqls := []string{
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

			for _, sql := range sqls {
				if err := tx.Exec(sql).Error; err != nil {
					return err
				}
			}

			return nil
		},
	}
}
