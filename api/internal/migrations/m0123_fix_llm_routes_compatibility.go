package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0123_fix_llm_routes_compatibility aligns route table naming/columns with runtime expectations.
// Some environments still keep llm_tenant_routes with tenant_id while code reads llm_routes.organization_id.
func M0123_fix_llm_routes_compatibility() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260306000123",
		Migrate: func(tx *gorm.DB) error {
			if err := renameTableIfNeeded(tx, "llm_tenant_routes", "llm_routes"); err != nil {
				return err
			}

			if err := renameColumnIfNeeded(tx, "llm_routes", "tenant_id", "organization_id"); err != nil {
				return err
			}

			sqls := []string{
				`ALTER TABLE IF EXISTS llm_routes
					ADD COLUMN IF NOT EXISTS organization_id UUID`,
				`UPDATE llm_routes r
				 SET organization_id = w.organization_id
				 FROM workspaces w
				 WHERE r.organization_id = w.id
				   AND w.organization_id IS NOT NULL`,
				`CREATE INDEX IF NOT EXISTS idx_llm_routes_organization_id ON llm_routes(organization_id)`,
			}

			for _, sql := range sqls {
				if err := tx.Exec(sql).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Compatibility migration is intentionally one-way.
			return nil
		},
	}
}
