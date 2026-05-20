package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0015_agents_permission_refactor_indexes creates additional indexes for the refactored agent permission system
// This migration adds indexes that complement the existing m0012 indexes for optimal query performance
func M0015_agents_permission_refactor_indexes() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251125100000",
		Migrate: func(tx *gorm.DB) error {
			// 1. Create index for sorting agents by creation date (descending)
			// Used in: ORDER BY agents.created_at DESC
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_agents_created_at_desc 
				ON agents(created_at DESC) 
				WHERE deleted_at IS NULL
			`).Error; err != nil {
				return err
			}

			// 2. Create composite index for tenant + created_at (for department-scoped queries)
			// Used in: Filtering by tenant_id and sorting by created_at
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_agents_tenant_created_at 
				ON agents(tenant_id, created_at DESC) 
				WHERE deleted_at IS NULL
			`).Error; err != nil {
				return err
			}

			// 3. Create index for soft delete filtering
			// Used in: WHERE deleted_at IS NULL
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_agents_deleted_at 
				ON agents(deleted_at)
			`).Error; err != nil {
				return err
			}

			// 4. Create index for agent_extensions permission filtering
			// Used in: WHERE permission = 'all_team' / 'all_group' / 'private'
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_agent_extensions_permission 
				ON agent_extensions(permission)
			`).Error; err != nil {
				return err
			}

			// 5. Create index for finding user's department memberships (current=false)
			// Used in: WHERE account_id = ? AND current = false
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_taj_account_not_current 
				ON tenant_account_joins(account_id) 
				WHERE current = false
			`).Error; err != nil {
				return err
			}

			// 6. Create composite index for account + tenant + role lookups
			// Used in: Permission context building with role checks
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_taj_account_tenant_role 
				ON tenant_account_joins(account_id, tenant_id, role)
			`).Error; err != nil {
				return err
			}

			// 7. Create index for finding organization by department
			// Used in: WHERE tenant_id = ? to find which organization a department belongs to
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_egtj_tenant 
				ON enterprise_group_tenant_joins(tenant_id)
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop all indexes in reverse order
			indexes := []string{
				"idx_egtj_tenant",
				"idx_taj_account_tenant_role",
				"idx_taj_account_not_current",
				"idx_agent_extensions_permission",
				"idx_agents_deleted_at",
				"idx_agents_tenant_created_at",
				"idx_agents_created_at_desc",
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
