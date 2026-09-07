package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

const migrationCorrectQwen35PlusContextID = "20260907003000_correct_qwen35_plus_context"

func init() {
	registerMigration(&gormigrate.Migration{
		ID:      migrationCorrectQwen35PlusContextID,
		Migrate: upCorrectQwen35PlusContext,
	})
}

func upCorrectQwen35PlusContext(db *gorm.DB) error {
	if !db.Migrator().HasTable("llm_models") {
		return nil
	}
	return db.Exec(`UPDATE llm_models
		SET context_window = 1000000,
			max_output_tokens = CASE WHEN max_output_tokens IS NULL OR max_output_tokens <= 4096 THEN 65536 ELSE max_output_tokens END,
			updated_at = CURRENT_TIMESTAMP
		WHERE name IN ('qwen3.5-plus', 'qwen3.5-plus-2026-02-15')
			AND (context_window IS NULL OR context_window = 100000)`).Error
}
