package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0108_kb_type_definitions creates the kb_type_definitions table for multi-language entity type labels
func M0108_kb_type_definitions() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260203180000", // 2026-02-03 18:00:00
		Migrate: func(tx *gorm.DB) error {
			// Create kb_type_definitions table
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS kb_type_definitions (
					id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
					dataset_id UUID NOT NULL,
					type_key VARCHAR(100) NOT NULL,
					label_zh VARCHAR(100),
					label_en VARCHAR(100),
					style_config JSONB DEFAULT '{}',
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					CONSTRAINT uq_type_per_dataset UNIQUE (dataset_id, type_key)
				);
			`).Error; err != nil {
				return err
			}

			// Add foreign key constraint
			if err := tx.Exec(`
				ALTER TABLE kb_type_definitions 
				ADD CONSTRAINT fk_type_definitions_dataset 
				FOREIGN KEY (dataset_id) REFERENCES datasets(id) ON DELETE CASCADE;
			`).Error; err != nil {
				return err
			}

			// Add index for faster lookups
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_type_definitions_dataset 
				ON kb_type_definitions(dataset_id);
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec("DROP TABLE IF EXISTS kb_type_definitions CASCADE").Error
		},
	}
}
