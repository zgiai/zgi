package migrations

import (
	"encoding/json"

	"github.com/go-gormigrate/gormigrate/v2"
	llmmodel "github.com/zgiai/ginext/internal/modules/llm/llmmodel/model"
	"gorm.io/gorm"
)

const migration0137ID = "20260326000137"

func M0137_normalize_supported_parameters_shape() *gormigrate.Migration {
	type modelRow struct {
		ID                  string `gorm:"column:id"`
		SupportedParameters []byte `gorm:"column:supported_parameters"`
	}

	return &gormigrate.Migration{
		ID: migration0137ID,
		Migrate: func(tx *gorm.DB) error {
			if !tx.Migrator().HasTable("llm_models") || !tx.Migrator().HasColumn("llm_models", "supported_parameters") {
				return nil
			}

			var rows []modelRow
			if err := tx.Table("llm_models").
				Select("id", "supported_parameters").
				Find(&rows).Error; err != nil {
				return err
			}

			for _, row := range rows {
				normalized, err := llmmodel.NormalizeParameterDefinitionsJSON(row.SupportedParameters)
				if err != nil {
					return err
				}
				serialized, err := marshalParameterDefinitionsForMigration(normalized)
				if err != nil {
					return err
				}
				if err := tx.Table("llm_models").
					Where("id = ?", row.ID).
					Update("supported_parameters", string(serialized)).Error; err != nil {
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

func marshalParameterDefinitionsForMigration(params llmmodel.ParameterDefinitions) ([]byte, error) {
	if len(params) == 0 {
		return []byte("[]"), nil
	}
	return json.Marshal(params)
}
