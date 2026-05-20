package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0101_add_is_official_to_routes adds is_official column to llm_routes table
// for mixed load balancing support
func M0101_add_is_official_to_routes() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260203000101",
		Migrate: func(tx *gorm.DB) error {
			// Add is_official column
			if err := tx.Exec(`
				ALTER TABLE llm_routes 
				ADD COLUMN IF NOT EXISTS is_official BOOLEAN NOT NULL DEFAULT false;
				
				CREATE INDEX IF NOT EXISTS idx_routes_is_official ON llm_routes(is_official);
				
				COMMENT ON COLUMN llm_routes.is_official IS 'Flag for mixed load balancing: true = official Console channel, false = local channel';
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Remove is_official column
			if err := tx.Exec(`
				DROP INDEX IF EXISTS idx_routes_is_official;
				
				ALTER TABLE llm_routes 
				DROP COLUMN IF EXISTS is_official;
			`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
