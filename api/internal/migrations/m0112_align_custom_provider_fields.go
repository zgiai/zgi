package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0112_align_custom_provider_fields renames columns in llm_custom_providers
// to align with global Provider model (ModelMeta standard):
//   - name → provider
//   - display_name → provider_name
//   - DROP protocol column
func M0112_align_custom_provider_fields() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260208000112",
		Migrate: func(tx *gorm.DB) error {
			var tableExists bool
			if err := tx.Raw(`
				SELECT EXISTS (
					SELECT 1
					FROM information_schema.tables
					WHERE table_schema = CURRENT_SCHEMA()
					  AND table_name = 'llm_custom_providers'
				)
			`).Scan(&tableExists).Error; err != nil {
				return err
			}
			if !tableExists {
				return nil
			}

			sqls := []string{
				`DO $$
				BEGIN
					IF EXISTS (
						SELECT 1
						FROM information_schema.columns
						WHERE table_schema = CURRENT_SCHEMA()
						  AND table_name = 'llm_custom_providers'
						  AND column_name = 'name'
					) THEN
						ALTER TABLE llm_custom_providers RENAME COLUMN name TO provider;
					END IF;
				END
				$$;`,
				`DO $$
				BEGIN
					IF EXISTS (
						SELECT 1
						FROM information_schema.columns
						WHERE table_schema = CURRENT_SCHEMA()
						  AND table_name = 'llm_custom_providers'
						  AND column_name = 'display_name'
					) THEN
						ALTER TABLE llm_custom_providers RENAME COLUMN display_name TO provider_name;
					END IF;
				END
				$$;`,
				`ALTER TABLE llm_custom_providers DROP COLUMN IF EXISTS protocol`,
			}
			for _, sql := range sqls {
				if err := tx.Exec(sql).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			var tableExists bool
			if err := tx.Raw(`
				SELECT EXISTS (
					SELECT 1
					FROM information_schema.tables
					WHERE table_schema = CURRENT_SCHEMA()
					  AND table_name = 'llm_custom_providers'
				)
			`).Scan(&tableExists).Error; err != nil {
				return err
			}
			if !tableExists {
				return nil
			}

			sqls := []string{
				`DO $$
				BEGIN
					IF EXISTS (
						SELECT 1
						FROM information_schema.columns
						WHERE table_schema = CURRENT_SCHEMA()
						  AND table_name = 'llm_custom_providers'
						  AND column_name = 'provider'
					) THEN
						ALTER TABLE llm_custom_providers RENAME COLUMN provider TO name;
					END IF;
				END
				$$;`,
				`DO $$
				BEGIN
					IF EXISTS (
						SELECT 1
						FROM information_schema.columns
						WHERE table_schema = CURRENT_SCHEMA()
						  AND table_name = 'llm_custom_providers'
						  AND column_name = 'provider_name'
					) THEN
						ALTER TABLE llm_custom_providers RENAME COLUMN provider_name TO display_name;
					END IF;
				END
				$$;`,
				`ALTER TABLE llm_custom_providers ADD COLUMN IF NOT EXISTS protocol VARCHAR(50) DEFAULT 'openai'`,
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
