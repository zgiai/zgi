package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

type customModelRuntimeColumn struct {
	name       string
	definition string
	legacy     string
}

func M0027_fix_custom_model_runtime_schema() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2FixCustomModelRuntimeID,
		Migrate: func(tx *gorm.DB) error {
			if !tx.Migrator().HasTable(customModelPriceTable) {
				return nil
			}

			for _, column := range customModelRuntimeColumns(tx.Dialector.Name()) {
				if err := addCustomModelRuntimeColumnIfMissing(tx, column); err != nil {
					return err
				}
				if err := backfillCustomModelRuntimeColumn(tx, column); err != nil {
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

func customModelRuntimeColumns(dialect string) []customModelRuntimeColumn {
	jsonType := "JSONB"
	if dialect == "sqlite" {
		jsonType = "TEXT"
	}

	return []customModelRuntimeColumn{
		{name: "context_window", definition: "INTEGER DEFAULT 0", legacy: "context_limit"},
		{name: "max_output_tokens", definition: "INTEGER DEFAULT 0", legacy: "output_limit"},
		{name: "max_input_tokens", definition: "INTEGER DEFAULT 0"},
		{name: "supported_parameters", definition: fmt.Sprintf("%s DEFAULT '[]'", jsonType)},
		{name: "config_parameters", definition: fmt.Sprintf("%s DEFAULT '[]'", jsonType)},
		{name: "default_parameters", definition: fmt.Sprintf("%s DEFAULT '{}'", jsonType)},
	}
}

func addCustomModelRuntimeColumnIfMissing(tx *gorm.DB, column customModelRuntimeColumn) error {
	exists, err := hasExactColumnV2(tx, customModelPriceTable, column.name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	statement := fmt.Sprintf(
		`ALTER TABLE %s ADD COLUMN %s %s`,
		customModelPriceTable,
		column.name,
		column.definition,
	)
	if err := tx.Exec(statement).Error; err != nil {
		return fmt.Errorf("add %s.%s: %w", customModelPriceTable, column.name, err)
	}
	return nil
}

func backfillCustomModelRuntimeColumn(tx *gorm.DB, column customModelRuntimeColumn) error {
	if column.legacy == "" {
		return nil
	}
	legacyExists, err := hasExactColumnV2(tx, customModelPriceTable, column.legacy)
	if err != nil {
		return err
	}
	if !legacyExists {
		return nil
	}

	statement := fmt.Sprintf(
		`UPDATE %s SET %s = %s WHERE %s IS NOT NULL AND (%s IS NULL OR %s = 0)`,
		customModelPriceTable,
		column.name,
		column.legacy,
		column.legacy,
		column.name,
		column.name,
	)
	if err := tx.Exec(statement).Error; err != nil {
		return fmt.Errorf("backfill %s.%s from %s: %w", customModelPriceTable, column.name, column.legacy, err)
	}
	return nil
}
