package migrationsv2

import (
	"fmt"
	"strings"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const customModelPriceTable = "llm_custom_models"

type customModelPriceColumn struct {
	current string
	legacy  string
}

func M0026_add_custom_model_price_columns() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2AddCustomModelPricesID,
		Migrate: func(tx *gorm.DB) error {
			if !tx.Migrator().HasTable(customModelPriceTable) {
				return nil
			}

			columns := []customModelPriceColumn{
				{current: "input_price", legacy: "cost_input"},
				{current: "output_price", legacy: "cost_output"},
			}
			for _, column := range columns {
				if err := alignCustomModelPriceColumn(tx, column); err != nil {
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

func alignCustomModelPriceColumn(tx *gorm.DB, column customModelPriceColumn) error {
	added, err := ensureCustomModelPriceColumn(tx, column.current)
	if err != nil {
		return err
	}
	if !added {
		return nil
	}

	return backfillCustomModelPriceColumn(tx, column.current, column.legacy)
}

func ensureCustomModelPriceColumn(tx *gorm.DB, column string) (bool, error) {
	exists, err := hasExactColumnV2(tx, customModelPriceTable, column)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}

	statement := fmt.Sprintf(
		`ALTER TABLE %s ADD COLUMN %s DECIMAL(10,4) DEFAULT 0`,
		customModelPriceTable,
		column,
	)
	if err := tx.Exec(statement).Error; err != nil {
		return false, fmt.Errorf("add %s.%s: %w", customModelPriceTable, column, err)
	}
	return true, nil
}

func backfillCustomModelPriceColumn(tx *gorm.DB, targetColumn, legacyColumn string) error {
	legacyExists, err := hasExactColumnV2(tx, customModelPriceTable, legacyColumn)
	if err != nil {
		return err
	}
	if !legacyExists {
		return nil
	}

	statement := fmt.Sprintf(
		`UPDATE %s SET %s = COALESCE(%s, 0)`,
		customModelPriceTable,
		targetColumn,
		legacyColumn,
	)
	if err := tx.Exec(statement).Error; err != nil {
		return fmt.Errorf("backfill %s.%s from %s: %w", customModelPriceTable, targetColumn, legacyColumn, err)
	}
	return nil
}

func hasExactColumnV2(tx *gorm.DB, tableName, columnName string) (bool, error) {
	switch tx.Dialector.Name() {
	case "postgres":
		var count int64
		err := tx.Raw(`
			SELECT COUNT(*)
			FROM information_schema.columns
			WHERE table_schema = CURRENT_SCHEMA()
			  AND table_name = ?
			  AND column_name = ?
		`, tableName, columnName).Scan(&count).Error
		return count > 0, err
	case "sqlite":
		var columns []struct {
			Name string `gorm:"column:name"`
		}
		if err := tx.Raw(fmt.Sprintf(`PRAGMA table_info(%s)`, tableName)).Scan(&columns).Error; err != nil {
			return false, err
		}
		for _, column := range columns {
			if strings.EqualFold(column.Name, columnName) {
				return true, nil
			}
		}
		return false, nil
	default:
		columnTypes, err := tx.Migrator().ColumnTypes(tableName)
		if err != nil {
			return false, err
		}
		for _, columnType := range columnTypes {
			if strings.EqualFold(columnType.Name(), columnName) {
				return true, nil
			}
		}
		return false, nil
	}
}
