package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0017_internal_flag_indexes creates indexes for internal flag queries on agents and workflows tables
// This migration supports efficient querying of built-in workflows and agents
func M0017_internal_flag_indexes() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251125120000",
		Migrate: func(tx *gorm.DB) error {
			// 1. Create partial index for internal agents
			// Used in: WHERE internal = true
			// Partial index is more efficient as we only index internal=true rows
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_agents_internal 
				ON agents(internal) 
				WHERE internal = true
			`).Error; err != nil {
				return err
			}

			// 2. Create partial index for internal workflows
			// Used in: WHERE internal = true
			// Partial index is more efficient as we only index internal=true rows
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_workflows_internal 
				ON workflows(internal) 
				WHERE internal = true
			`).Error; err != nil {
				return err
			}

			// 3. Create composite index for internal agents with tenant filtering
			// Used in: WHERE internal = true AND tenant_id = ?
			// Supports queries for built-in workflows in specific tenant contexts
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_agents_internal_tenant 
				ON agents(internal, tenant_id) 
				WHERE internal = true
			`).Error; err != nil {
				return err
			}

			// 4. Create composite index for internal workflows with tenant filtering
			// Used in: WHERE internal = true AND tenant_id = ?
			// Supports queries for built-in workflows in specific tenant contexts
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_workflows_internal_tenant 
				ON workflows(internal, tenant_id) 
				WHERE internal = true
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop all indexes in reverse order
			indexes := []string{
				"idx_workflows_internal_tenant",
				"idx_agents_internal_tenant",
				"idx_workflows_internal",
				"idx_agents_internal",
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
