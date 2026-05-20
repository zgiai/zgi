package migrations

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/zgiai/zgi/api/internal/migrations/baseline"
	"gorm.io/gorm"
)

const initialSchemaMigrationID = "20260520000000_initial_schema"

func init() {
	registerMigration(&gormigrate.Migration{
		ID: initialSchemaMigrationID,
		Migrate: func(tx *gorm.DB) error {
			exists, err := hasExistingPublicSchema(tx)
			if err != nil {
				return err
			}
			if exists {
				return fmt.Errorf("initial schema migration requires an empty public schema; back up existing deployments and migrate them with an explicit cutover plan")
			}
			return applySchemaFiles(tx, baseline.Files)
		},
		Rollback: func(tx *gorm.DB) error {
			return fmt.Errorf("rollback of initial schema is not supported")
		},
	})
}

func hasExistingPublicSchema(tx *gorm.DB) (bool, error) {
	var count int64
	err := tx.Raw(`
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_type = 'BASE TABLE'
		  AND table_name NOT IN ('migrations', 'schema_migrations')
	`).Scan(&count).Error
	if err != nil {
		return false, fmt.Errorf("inspect public schema: %w", err)
	}
	return count > 0, nil
}
