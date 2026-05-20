package migrations

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0141_add_llm_model_config_parameters() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260410000141",
		Migrate: func(tx *gorm.DB) error {
			jsonType := "JSONB"
			defaultValue := "'[]'::jsonb"
			if tx.Dialector.Name() == "sqlite" {
				jsonType = "TEXT"
				defaultValue = "'[]'"
			}

			type tableColumn struct {
				table  string
				column string
			}
			targets := []tableColumn{
				{table: "llm_models", column: "config_parameters"},
				{table: "llm_custom_models", column: "config_parameters"},
			}
			for _, target := range targets {
				if !tx.Migrator().HasTable(target.table) || tx.Migrator().HasColumn(target.table, target.column) {
					continue
				}
				statement := fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s NOT NULL DEFAULT %s`, target.table, target.column, jsonType, defaultValue)
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			targets := []string{
				"llm_models",
				"llm_custom_models",
			}
			for _, table := range targets {
				if !tx.Migrator().HasTable(table) || !tx.Migrator().HasColumn(table, "config_parameters") {
					continue
				}
				if err := tx.Migrator().DropColumn(table, "config_parameters"); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
