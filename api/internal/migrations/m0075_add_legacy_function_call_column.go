package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0070_add_legacy_function_call_column ensures llm_models has legacy_function_call column
// This column is required by the new canonical llmmodel.LLMModel struct.
// Some environments may have skipped or partially applied capability renames.
func M0075_add_legacy_function_call_column() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260108000070",
		Migrate: func(tx *gorm.DB) error {
			// Ensure the column exists on llm_models
			if err := tx.Exec(`
				ALTER TABLE llm_models
				ADD COLUMN IF NOT EXISTS legacy_function_call BOOLEAN NOT NULL DEFAULT false
			`).Error; err != nil {
				return err
			}

			// If the old column exists, copy values over
			tx.Exec(`
				DO $$
				BEGIN
					IF EXISTS (
						SELECT 1
						FROM information_schema.columns
						WHERE table_name = 'llm_models' AND column_name = 'supports_function_call'
					) THEN
						UPDATE llm_models
						SET legacy_function_call = supports_function_call
						WHERE legacy_function_call = false;
					END IF;
				END $$;
			`)

			// Keep tenant custom models schema consistent if present
			tx.Exec(`
				DO $$
				BEGIN
					IF EXISTS (
						SELECT 1
						FROM information_schema.tables
						WHERE table_name = 'llm_tenant_custom_models'
					) THEN
						ALTER TABLE llm_tenant_custom_models
						ADD COLUMN IF NOT EXISTS legacy_function_call BOOLEAN NOT NULL DEFAULT false;

						IF EXISTS (
							SELECT 1
							FROM information_schema.columns
							WHERE table_name = 'llm_tenant_custom_models' AND column_name = 'supports_function_call'
						) THEN
							UPDATE llm_tenant_custom_models
							SET legacy_function_call = supports_function_call
							WHERE legacy_function_call = false;
						END IF;
					END IF;
				END $$;
			`)

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Safe rollback: only drop the added columns
			if err := tx.Exec(`
				ALTER TABLE llm_models
				DROP COLUMN IF EXISTS legacy_function_call
			`).Error; err != nil {
				return err
			}

			tx.Exec(`
				DO $$
				BEGIN
					IF EXISTS (
						SELECT 1
						FROM information_schema.tables
						WHERE table_name = 'llm_tenant_custom_models'
					) THEN
						ALTER TABLE llm_tenant_custom_models
						DROP COLUMN IF EXISTS legacy_function_call;
					END IF;
				END $$;
			`)

			return nil
		},
	}
}
