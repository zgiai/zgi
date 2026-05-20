package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0117_unique_official_route_per_org ensures one active official route per organization.
func M0117_unique_official_route_per_org() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260212000117",
		Migrate: func(tx *gorm.DB) error {
			var exists bool
			if err := tx.Raw(`
				SELECT EXISTS (
					SELECT 1
					FROM information_schema.tables
					WHERE table_schema = CURRENT_SCHEMA()
					  AND table_name = 'llm_routes'
				)
			`).Scan(&exists).Error; err != nil {
				return err
			}
			if !exists {
				return nil
			}

			return tx.Exec(`
				CREATE UNIQUE INDEX IF NOT EXISTS idx_llm_routes_org_official_unique
				ON llm_routes (organization_id)
				WHERE is_official = true AND deleted_at IS NULL;
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`
				DROP INDEX IF EXISTS idx_llm_routes_org_official_unique;
			`).Error
		},
	}
}
