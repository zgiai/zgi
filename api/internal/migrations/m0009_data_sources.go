package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0009_add_data_sources adds data_sources table for managing user data sources
func M0009_data_sources() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20251110100800",
		Migrate: func(tx *gorm.DB) error {
			// Create data_sources table
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS data_sources (
					id uuid PRIMARY KEY,
					group_id uuid NOT NULL,
					tenant_id uuid,
					name VARCHAR(255) NOT NULL,
					schema_id INTEGER NOT NULL DEFAULT 0,
					schema_name VARCHAR(255) NOT NULL,
					description TEXT,
					permission VARCHAR(50) NOT NULL DEFAULT 'only_me',
					status VARCHAR(50) NOT NULL DEFAULT 'active',
					icon_type VARCHAR(255),
					icon TEXT,
					icon_background VARCHAR(255),
					created_by VARCHAR(36) NOT NULL,
					updated_by VARCHAR(36) NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)
			`).Error; err != nil {
				return err
			}

			// Add indexes to data_sources table
			if err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_data_sources_group_id ON data_sources(group_id)`).Error; err != nil {
				return err
			}

			// Create the data_source_tables table
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS data_source_tables (
					id uuid PRIMARY KEY,
					group_id uuid NOT NULL,
					data_source_id uuid NOT NULL,
					name VARCHAR(255) NOT NULL,
					table_id INTEGER NOT NULL,
					table_name VARCHAR(255) NOT NULL,
					description TEXT,
					created_by VARCHAR(36) NOT NULL,
					updated_by VARCHAR(36) NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)
			`).Error; err != nil {
				return err
			}

			// Create indexes
			indexes := []string{
				"CREATE INDEX IF NOT EXISTS idx_data_source_tables_group_id ON data_source_tables(group_id)",
				"CREATE INDEX IF NOT EXISTS idx_data_source_tables_data_source_id ON data_source_tables(data_source_id)",
				"CREATE INDEX IF NOT EXISTS idx_data_source_tables_name ON data_source_tables(name)",
			}

			for _, indexQuery := range indexes {
				if err := tx.Exec(indexQuery).Error; err != nil {
					return err
				}
			}

			// Create the table_prompts table
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS data_source_table_prompts (
					id uuid PRIMARY KEY,
					table_id uuid NOT NULL,
					prompt TEXT NOT NULL,
					created_by VARCHAR(36) NOT NULL,
					updated_by VARCHAR(36) NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)
			`).Error; err != nil {
				return err
			}

			// Add indexes to table_prompts table
			if err := tx.Exec(`CREATE INDEX IF NOT EXISTS idx_table_prompts_table_id ON data_source_table_prompts(table_id)`).Error; err != nil {
				return err
			}

			// Create data_source_sql_operations table (previously operation_logs and data_source_table_operations)
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS data_source_sql_operations (
					id uuid PRIMARY KEY,
					group_id uuid NOT NULL,
					data_source_id uuid NOT NULL,
					table_id uuid,
					data_source_name VARCHAR(255),
					table_name VARCHAR(255),
					sql_statement TEXT NOT NULL,
					operation_type VARCHAR(20) NOT NULL,
					start_time TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					end_time TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					status VARCHAR(10) NOT NULL,
					created_by VARCHAR(36) NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)
			`).Error; err != nil {
				return err
			}

			// Create indexes for data_source_sql_operations table
			indexes = []string{
				"CREATE INDEX IF NOT EXISTS idx_data_source_sql_operations_group_id ON data_source_sql_operations(group_id)",
				"CREATE INDEX IF NOT EXISTS idx_data_source_sql_operations_data_source_id ON data_source_sql_operations(data_source_id)",
				"CREATE INDEX IF NOT EXISTS idx_data_source_sql_operations_table_id ON data_source_sql_operations(table_id)",
				"CREATE INDEX IF NOT EXISTS idx_data_source_sql_operations_created_at ON data_source_sql_operations(created_at)",
			}

			for _, indexQuery := range indexes {
				if err := tx.Exec(indexQuery).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop data_sources table
			if err := tx.Exec("DROP TABLE IF EXISTS data_sources").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP TABLE IF EXISTS data_source_tables").Error; err != nil {
				return err
			}
			// Drop table_prompts table
			if err := tx.Exec("DROP TABLE IF EXISTS data_source_table_prompts").Error; err != nil {
				return err
			}
			// Drop data_source_sql_operations table
			if err := tx.Exec("DROP TABLE IF EXISTS data_source_sql_operations").Error; err != nil {
				return err
			}
			return nil
		},
	}
}
