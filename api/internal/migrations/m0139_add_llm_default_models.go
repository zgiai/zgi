package migrations

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migration0139ID = "20260331000139"

func M0139_add_llm_default_models() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migration0139ID,
		Migrate: func(tx *gorm.DB) error {
			if tx.Migrator().HasTable("llm_default_models") {
				return nil
			}

			jsonType := "JSONB"
			if tx.Dialector.Name() == "sqlite" {
				jsonType = "TEXT"
			}

			createSQL := fmt.Sprintf(`
				CREATE TABLE llm_default_models (
					id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
					organization_id UUID NOT NULL,
					use_case VARCHAR(50) NOT NULL,
					provider VARCHAR(100) NOT NULL,
					model VARCHAR(100) NOT NULL,
					params %s NOT NULL DEFAULT '{}',
					created_by UUID NULL,
					updated_by UUID NULL,
					created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
					deleted_at TIMESTAMP NULL
				)
			`, jsonType)
			if err := tx.Exec(createSQL).Error; err != nil {
				return err
			}

			indexes := []string{
				`CREATE UNIQUE INDEX idx_llm_default_models_org_use_case ON llm_default_models(organization_id, use_case)`,
				`CREATE INDEX idx_llm_default_models_organization_id ON llm_default_models(organization_id)`,
				`CREATE INDEX idx_llm_default_models_deleted_at ON llm_default_models(deleted_at)`,
			}
			for _, sql := range indexes {
				if err := tx.Exec(sql).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if tx.Migrator().HasTable("llm_default_models") {
				return tx.Migrator().DropTable("llm_default_models")
			}
			return nil
		},
	}
}
