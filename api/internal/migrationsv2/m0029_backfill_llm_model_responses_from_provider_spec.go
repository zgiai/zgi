package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

var modelResponsesProviderSpecs = []string{
	"openai",
	"qwen",
	"doubao",
	"openrouter",
	"ollama",
	"agicto",
}

func M0029_backfill_llm_model_responses_from_provider_spec() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2BackfillModelResponsesID,
		Migrate: func(tx *gorm.DB) error {
			if !tx.Migrator().HasTable("llm_models") {
				return fmt.Errorf("llm_models table is required")
			}
			exists, err := hasExactColumnV2(tx, "llm_models", "responses")
			if err != nil {
				return fmt.Errorf("check llm_models.responses: %w", err)
			}
			if !exists {
				return fmt.Errorf("llm_models.responses column is required")
			}

			return tx.Exec(`
				UPDATE llm_models
				SET responses = TRUE,
					updated_at = CURRENT_TIMESTAMP
				WHERE deleted_at IS NULL
					AND responses = FALSE
					AND provider IN ?
			`, modelResponsesProviderSpecs).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
