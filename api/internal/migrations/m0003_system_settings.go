package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0003_system_settings creates system settings tables
func M0003_system_settings() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251110100200",
		Migrate: func(tx *gorm.DB) error {
			// Create system_settings table
			if err := tx.Exec(`
				CREATE TABLE system_settings (
					id UUID NOT NULL DEFAULT gen_random_uuid(),
					category VARCHAR(50) NOT NULL,
					settings JSONB NOT NULL,
					created_at TIMESTAMPTZ DEFAULT now(),
					updated_at TIMESTAMPTZ DEFAULT now(),
					updated_by UUID,
					PRIMARY KEY (id),
					UNIQUE (category)
				)
			`).Error; err != nil {
				return err
			}

			// Create indexes
			indexes := []string{
				`CREATE INDEX idx_system_settings_category ON system_settings(category)`,
				`CREATE INDEX idx_system_settings_updated_at ON system_settings(updated_at)`,
			}

			for _, indexSQL := range indexes {
				if err := tx.Exec(indexSQL).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable("system_settings")
		},
	}
}
