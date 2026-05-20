package migrations

import (
	"fmt"
	"gorm.io/gorm"
)

// MigrateExplainVoice adds voice column to explain_results table
func MigrateExplainVoice(db *gorm.DB) error {
	// Add voice column to explain_results table if it doesn't exist
	if err := db.Exec(`
		ALTER TABLE explain_results 
		ADD COLUMN IF NOT EXISTS voice VARCHAR(50)
	`).Error; err != nil {
		return fmt.Errorf("failed to add voice column to explain_results table: %w", err)
	}

	return nil
}
