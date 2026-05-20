package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0113_align_global_provider_columns renames columns in llm_providers
// to align with ModelMeta standard and CustomProvider naming:
//   - name → provider
//   - display_name → provider_name
func M0113_align_global_provider_columns() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260208000113",
		Migrate: func(tx *gorm.DB) error {
			sqls := []string{
				`ALTER TABLE llm_providers RENAME COLUMN name TO provider`,
				`ALTER TABLE llm_providers RENAME COLUMN display_name TO provider_name`,
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
				`ALTER TABLE llm_providers RENAME COLUMN provider TO name`,
				`ALTER TABLE llm_providers RENAME COLUMN provider_name TO display_name`,
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
