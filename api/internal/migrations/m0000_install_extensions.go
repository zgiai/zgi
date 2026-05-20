package migrations

import (
	"strings"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0000_install_extensions installs required PostgreSQL extensions
func M0000_install_extensions() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251108000000",
		Migrate: func(tx *gorm.DB) error {
			// Install uuid-ossp extension
			// Ignore permission errors if extension already exists
			if err := tx.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`).Error; err != nil {
				// Check if extension already exists by querying pg_extension
				var count int64
				checkErr := tx.Raw(`SELECT COUNT(*) FROM pg_extension WHERE extname = 'uuid-ossp'`).Scan(&count).Error
				if checkErr != nil {
					return err // Return original error if we can't check
				}
				if count == 0 {
					// Check for specific error about uuid_nil already existing
					// This happens when functions exist but extension is not registered
					if strings.Contains(err.Error(), "already exists") {
						return nil
					}
					return err // Extension doesn't exist and we can't create it
				}
				// Extension exists, ignore the permission error
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop extension on rollback (note: not recommended in production environments)
			return tx.Exec(`DROP EXTENSION IF EXISTS "uuid-ossp"`).Error
		},
	}
}
