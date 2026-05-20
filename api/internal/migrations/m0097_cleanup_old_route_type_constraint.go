package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0097_cleanup_old_route_type_constraint removes the obsolete chk_route_type_new constraint
// that still references old RouteType values (USER_OWNED, SYSTEM_REF)
func M0097_cleanup_old_route_type_constraint() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260131000097",
		Migrate: func(tx *gorm.DB) error {
			// Drop the obsolete chk_route_type_new constraint
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_routes 
				DROP CONSTRAINT IF EXISTS chk_route_type_new
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Restore the old constraint (for rollback purposes only)
			if err := tx.Exec(`
				ALTER TABLE llm_tenant_routes 
				ADD CONSTRAINT chk_route_type_new CHECK (route_type IS NULL OR route_type IN ('USER_OWNED', 'SYSTEM_REF'))
			`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
