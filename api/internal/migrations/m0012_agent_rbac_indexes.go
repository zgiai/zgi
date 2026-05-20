package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0012_agent_rbac_indexes creates indexes for agent RBAC query optimization
func M0012_agent_rbac_indexes() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251120100000",
		Migrate: func(tx *gorm.DB) error {
			// 1. Create index on enterprise_group_account_joins(account_id, role)
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_egaj_account_role 
				ON enterprise_group_account_joins(account_id, role)
			`).Error; err != nil {
				return err
			}

			// 2. Create index on tenant_account_joins(account_id, current)
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_taj_account_current 
				ON tenant_account_joins(account_id, current)
			`).Error; err != nil {
				return err
			}

			// 3. Create index on enterprise_group_tenant_joins(group_id, tenant_id)
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_egtj_group_tenant 
				ON enterprise_group_tenant_joins(group_id, tenant_id)
			`).Error; err != nil {
				return err
			}

			// 4. Create index on agents(tenant_id) for non-deleted agents
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_agents_tenant 
				ON agents(tenant_id) WHERE deleted_at IS NULL
			`).Error; err != nil {
				return err
			}

			// 5. Create index on agents(created_by) for non-deleted agents
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_agents_created_by 
				ON agents(created_by) WHERE deleted_at IS NULL
			`).Error; err != nil {
				return err
			}

			// 6. Create index on agent_extensions(agent_id, permission)
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_ae_agent_permission 
				ON agent_extensions(agent_id, permission)
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop all indexes in reverse order
			indexes := []string{
				"idx_ae_agent_permission",
				"idx_agents_created_by",
				"idx_agents_tenant",
				"idx_egtj_group_tenant",
				"idx_taj_account_current",
				"idx_egaj_account_role",
			}

			for _, idx := range indexes {
				if err := tx.Exec(`DROP INDEX IF EXISTS ` + idx).Error; err != nil {
					return err
				}
			}

			return nil
		},
	}
}
