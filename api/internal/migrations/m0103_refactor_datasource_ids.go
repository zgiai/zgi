package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0103_refactor_datasource_ids() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601310103",
		Migrate: func(tx *gorm.DB) error {
			// 1. data_sources table
			// Rename group_id -> organization_id
			if tx.Migrator().HasColumn("data_sources", "group_id") {
				if err := tx.Exec("ALTER TABLE data_sources RENAME COLUMN group_id TO organization_id").Error; err != nil {
					return err
				}
			}
			// Rename tenant_id -> workspace_id
			if tx.Migrator().HasColumn("data_sources", "tenant_id") {
				if err := tx.Exec("ALTER TABLE data_sources RENAME COLUMN tenant_id TO workspace_id").Error; err != nil {
					return err
				}
			}
			// Rename indexes
			if err := tx.Exec("ALTER INDEX IF EXISTS idx_data_sources_group_id RENAME TO idx_data_sources_organization_id").Error; err != nil {
				return err
			}
			// Ensure index exists
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_data_sources_organization_id ON data_sources(organization_id)").Error; err != nil {
				return err
			}

			// 2. data_source_tables table
			// Rename group_id -> organization_id
			if tx.Migrator().HasColumn("data_source_tables", "group_id") {
				if err := tx.Exec("ALTER TABLE data_source_tables RENAME COLUMN group_id TO organization_id").Error; err != nil {
					return err
				}
			}
			// Rename indexes
			if err := tx.Exec("ALTER INDEX IF EXISTS idx_data_source_tables_group_id RENAME TO idx_data_source_tables_organization_id").Error; err != nil {
				return err
			}
			// Ensure index exists
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_data_source_tables_organization_id ON data_source_tables(organization_id)").Error; err != nil {
				return err
			}

			// 3. data_source_sql_operations table
			// Rename group_id -> organization_id
			if tx.Migrator().HasColumn("data_source_sql_operations", "group_id") {
				if err := tx.Exec("ALTER TABLE data_source_sql_operations RENAME COLUMN group_id TO organization_id").Error; err != nil {
					return err
				}
			}
			// Rename indexes
			if err := tx.Exec("ALTER INDEX IF EXISTS idx_data_source_sql_operations_group_id RENAME TO idx_data_source_sql_operations_organization_id").Error; err != nil {
				return err
			}
			// Ensure index exists
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_data_source_sql_operations_organization_id ON data_source_sql_operations(organization_id)").Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// 1. data_sources table
			if err := tx.Exec("ALTER TABLE data_sources RENAME COLUMN organization_id TO group_id").Error; err != nil {
				return err
			}
			if err := tx.Exec("ALTER TABLE data_sources RENAME COLUMN workspace_id TO tenant_id").Error; err != nil {
				return err
			}
			// Rename indexes
			if err := tx.Exec("ALTER INDEX IF EXISTS idx_data_sources_organization_id RENAME TO idx_data_sources_group_id").Error; err != nil {
				return err
			}
			// Ensure index exists
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_data_sources_group_id ON data_sources(group_id)").Error; err != nil {
				return err
			}

			// 2. data_source_tables table
			if err := tx.Exec("ALTER TABLE data_source_tables RENAME COLUMN organization_id TO group_id").Error; err != nil {
				return err
			}
			// Rename indexes
			if err := tx.Exec("ALTER INDEX IF EXISTS idx_data_source_tables_organization_id RENAME TO idx_data_source_tables_group_id").Error; err != nil {
				return err
			}
			// Ensure index exists
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_data_source_tables_group_id ON data_source_tables(group_id)").Error; err != nil {
				return err
			}

			// 3. data_source_sql_operations table
			if err := tx.Exec("ALTER TABLE data_source_sql_operations RENAME COLUMN organization_id TO group_id").Error; err != nil {
				return err
			}
			// Rename indexes
			if err := tx.Exec("ALTER INDEX IF EXISTS idx_data_source_sql_operations_organization_id RENAME TO idx_data_source_sql_operations_group_id").Error; err != nil {
				return err
			}
			// Ensure index exists
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_data_source_sql_operations_group_id ON data_source_sql_operations(group_id)").Error; err != nil {
				return err
			}

			return nil
		},
	}
}
