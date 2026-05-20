package migrations

import (
	"fmt"
	"strings"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0071_fix_capability_naming_cleanup fixes the issues caused by M0068
// It properly handles duplicate columns and ensures clean migration state
func M0072_fix_capability_naming_cleanup() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "0071_fix_capability_naming_cleanup",
		Migrate: func(tx *gorm.DB) error {
			// Define all the capability column mappings
			capabilityColumns := map[string]string{
				"supports_vision":            "vision",
				"supports_reasoning":         "reasoning",
				"supports_streaming":         "streaming",
				"supports_tool_call":         "function_calling",
				"supports_structured_output": "structured_output",
				"supports_json_mode":         "json_mode",
				"supports_function_call":     "legacy_function_call",
				"supports_audio":             "audio",
				"supports_attachment":        "attachment",
				"supports_temperature":       "temperature",
			}

			// Process each column mapping
			for oldCol, newCol := range capabilityColumns {
				// Check if old column exists
				var oldExists bool
				err := tx.Raw(`
					SELECT EXISTS (
						SELECT 1 FROM information_schema.columns
						WHERE table_name = 'llm_models' AND column_name = ?
					)
				`, oldCol).Scan(&oldExists).Error

				if err != nil {
					return fmt.Errorf("failed to check column %s: %w", oldCol, err)
				}

				// Check if new column exists
				var newExists bool
				err = tx.Raw(`
					SELECT EXISTS (
						SELECT 1 FROM information_schema.columns
						WHERE table_name = 'llm_models' AND column_name = ?
					)
				`, newCol).Scan(&newExists).Error

				if err != nil {
					return fmt.Errorf("failed to check column %s: %w", newCol, err)
				}

				// Handle different scenarios
				if oldExists && newExists {
					// Both columns exist - we need to migrate data and drop old column
					// First, check if they have the same type
					var oldType, newType string
					if err := tx.Raw(`
						SELECT data_type FROM information_schema.columns
						WHERE table_name = 'llm_models' AND column_name = ?
					`, oldCol).Scan(&oldType).Error; err != nil {
						return fmt.Errorf("failed to get type for column %s: %w", oldCol, err)
					}

					if err := tx.Raw(`
						SELECT data_type FROM information_schema.columns
						WHERE table_name = 'llm_models' AND column_name = ?
					`, newCol).Scan(&newType).Error; err != nil {
						return fmt.Errorf("failed to get type for column %s: %w", newCol, err)
					}

					if oldType == newType {
						// Migrate data from old to new if new is NULL
						tx.Exec(fmt.Sprintf(`
							UPDATE llm_models
							SET %s = %s
							WHERE %s IS NOT NULL AND %s IS NULL
						`, newCol, oldCol, oldCol, newCol))
					}

					// Drop the old column
					if err := tx.Exec(fmt.Sprintf(
						"ALTER TABLE llm_models DROP COLUMN IF EXISTS %s", oldCol,
					)).Error; err != nil {
						return fmt.Errorf("failed to drop column %s: %w", oldCol, err)
					}

					fmt.Printf("Dropped duplicate column: %s (kept %s)\n", oldCol, newCol)

				} else if oldExists && !newExists {
					// Only old column exists - rename it
					if err := tx.Exec(fmt.Sprintf(
						"ALTER TABLE llm_models RENAME COLUMN %s TO %s", oldCol, newCol,
					)).Error; err != nil {
						return fmt.Errorf("failed to rename column %s to %s: %w", oldCol, newCol, err)
					}

					fmt.Printf("Renamed column: %s -> %s\n", oldCol, newCol)

				} else if !oldExists && newExists {
					// New column already exists, old column doesn't - this is good
					fmt.Printf("Column already migrated: %s\n", newCol)
				}
				// If neither exists, do nothing
			}

			// Fix use_cases column type - ensure it's TEXT[]
			var useCasesExists bool
			if err := tx.Raw(`
				SELECT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_name = 'llm_models' AND column_name = 'use_cases'
				)
			`).Scan(&useCasesExists).Error; err != nil {
				return fmt.Errorf("failed to check use_cases column: %w", err)
			}

			if useCasesExists {
				// Check current type using udt_name for better array detection
				var udtName string
				if err := tx.Raw(`
					SELECT udt_name FROM information_schema.columns
					WHERE table_name = 'llm_models' AND column_name = 'use_cases'
				`).Scan(&udtName).Error; err != nil {
					return fmt.Errorf("failed to get use_cases column type: %w", err)
				}

				// If it's not an array type (udt_name starts with '_'), convert it
				if !strings.HasPrefix(udtName, "_") {
					// Backup data if needed
					if err := tx.Exec("CREATE TEMP TABLE use_cases_backup AS SELECT id, use_cases FROM llm_models WHERE use_cases IS NOT NULL").Error; err != nil {
						return fmt.Errorf("failed to backup use_cases data: %w", err)
					}

					// Drop and recreate with correct type
					if err := tx.Exec("ALTER TABLE llm_models DROP COLUMN IF EXISTS use_cases").Error; err != nil {
						return fmt.Errorf("failed to drop use_cases column: %w", err)
					}

					if err := tx.Exec(`
						ALTER TABLE llm_models
						ADD COLUMN IF NOT EXISTS use_cases TEXT[] DEFAULT '{}' NOT NULL
					`).Error; err != nil {
						return fmt.Errorf("failed to add use_cases column: %w", err)
					}

					fmt.Printf("Fixed use_cases column type to TEXT[]\n")
				}
			}

			// Add indexes for the new capability columns if they don't exist
			capabilityIndexes := []string{
				"CREATE INDEX IF NOT EXISTS idx_llm_models_vision ON llm_models(vision) WHERE vision = true",
				"CREATE INDEX IF NOT EXISTS idx_llm_models_reasoning ON llm_models(reasoning) WHERE reasoning = true",
				"CREATE INDEX IF NOT EXISTS idx_llm_models_streaming ON llm_models(streaming) WHERE streaming = true",
				"CREATE INDEX IF NOT EXISTS idx_llm_models_function_calling ON llm_models(function_calling) WHERE function_calling = true",
				"CREATE INDEX IF NOT EXISTS idx_llm_models_audio ON llm_models(audio) WHERE audio = true",
			}

			for _, indexSQL := range capabilityIndexes {
				if err := tx.Exec(indexSQL).Error; err != nil {
					// Log error but don't fail the migration
					fmt.Printf("Warning: Failed to create index: %v\n", err)
				}
			}

			// Verify final state
			fmt.Println("\nFinal column state verification:")
			for _, col := range []string{"vision", "reasoning", "streaming", "function_calling", "structured_output", "json_mode", "audio"} {
				var exists bool
				tx.Raw(`
					SELECT EXISTS (
						SELECT 1 FROM information_schema.columns
						WHERE table_name = 'llm_models' AND column_name = ?
					)
				`, col).Scan(&exists)
				fmt.Printf("  %s: %v\n", col, exists)
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// This is a cleanup migration, rollback would be complex
			// Best approach is to restore from backup if needed
			fmt.Println("Warning: This migration cannot be safely rolled back")
			fmt.Println("Please restore from database backup if needed")
			return nil
		},
	}
}
