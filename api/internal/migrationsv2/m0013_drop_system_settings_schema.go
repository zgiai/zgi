package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0013_drop_system_settings_schema() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2DropSystemSettingsID,
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				DROP TABLE IF EXISTS public.settings_audit_logs CASCADE;
				DROP TABLE IF EXISTS public.system_settings CASCADE;
			`).Error; err != nil {
				return fmt.Errorf("drop retired system settings schema: %w", err)
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
