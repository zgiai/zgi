package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const modelConfigsTable = "llm_model_configs"

type modelConfigPriceField struct {
	legacy  string
	current string
}

func M0022_fix_llm_model_config_price_fields() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2FixModelConfigPriceFieldsID,
		Migrate: func(tx *gorm.DB) error {
			if !tx.Migrator().HasTable(modelConfigsTable) {
				return nil
			}

			fields := []modelConfigPriceField{
				{legacy: "cost_input_override", current: "input_price_override"},
				{legacy: "cost_output_override", current: "output_price_override"},
			}
			for _, field := range fields {
				if err := alignModelConfigPriceField(tx, field); err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}

func alignModelConfigPriceField(tx *gorm.DB, field modelConfigPriceField) error {
	hasLegacy := tx.Migrator().HasColumn(modelConfigsTable, field.legacy)
	hasCurrent := tx.Migrator().HasColumn(modelConfigsTable, field.current)

	switch {
	case hasLegacy && !hasCurrent:
		if err := tx.Migrator().RenameColumn(modelConfigsTable, field.legacy, field.current); err != nil {
			return fmt.Errorf("rename %s.%s to %s: %w", modelConfigsTable, field.legacy, field.current, err)
		}
	case hasLegacy && hasCurrent:
		statement := fmt.Sprintf(`
			UPDATE %s
			SET %s = %s
			WHERE %s IS NULL
			  AND %s IS NOT NULL
		`, modelConfigsTable, field.current, field.legacy, field.current, field.legacy)
		if err := tx.Exec(statement).Error; err != nil {
			return fmt.Errorf("backfill %s.%s from %s: %w", modelConfigsTable, field.current, field.legacy, err)
		}
	case !hasCurrent:
		statement := fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s DECIMAL(10,4)`, modelConfigsTable, field.current)
		if err := tx.Exec(statement).Error; err != nil {
			return fmt.Errorf("add %s.%s: %w", modelConfigsTable, field.current, err)
		}
	}

	return nil
}
