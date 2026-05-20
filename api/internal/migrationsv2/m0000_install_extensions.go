package migrationsv2

import (
	"strings"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0000_install_extensions installs required PostgreSQL extensions for the v2 chain.
func M0000_install_extensions() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2InstallExtensionsID,
		Migrate: func(tx *gorm.DB) error {
			for _, extension := range []string{`"uuid-ossp"`, `pgcrypto`} {
				if err := tx.Exec(`CREATE EXTENSION IF NOT EXISTS ` + extension).Error; err != nil {
					var count int64
					extName := strings.Trim(extension, `"`)
					checkErr := tx.Raw(`SELECT COUNT(*) FROM pg_extension WHERE extname = ?`, extName).Scan(&count).Error
					if checkErr != nil {
						return err
					}
					if count == 0 {
						if strings.Contains(err.Error(), "already exists") {
							continue
						}
						return err
					}
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`DROP EXTENSION IF EXISTS pgcrypto`).Error; err != nil {
				return err
			}
			return tx.Exec(`DROP EXTENSION IF EXISTS "uuid-ossp"`).Error
		},
	}
}
