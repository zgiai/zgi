package migrationsv2

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0030_data_source_excel_import_jobs() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2DataSourceExcelImportJobsID,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`
				CREATE TABLE IF NOT EXISTS data_source_import_jobs (
					id UUID PRIMARY KEY,
					organization_id UUID NOT NULL,
					workspace_id UUID NULL,
					data_source_id UUID NOT NULL,
					table_id UUID NULL,
					upload_file_id UUID NULL,
					source_type VARCHAR(20) NOT NULL,
					source_file_name VARCHAR(512) NOT NULL,
					status VARCHAR(32) NOT NULL,
					total_rows INTEGER NOT NULL DEFAULT 0,
					valid_rows INTEGER NOT NULL DEFAULT 0,
					imported_rows INTEGER NOT NULL DEFAULT 0,
					failed_rows INTEGER NOT NULL DEFAULT 0,
					sheet_name VARCHAR(255) NULL,
					header_row INTEGER NULL,
					start_row INTEGER NULL,
					schema_snapshot JSONB NULL,
					preview_snapshot JSONB NULL,
					error_summary JSONB NULL,
					created_by VARCHAR(36) NOT NULL,
					updated_by VARCHAR(36) NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				)
				`,
				`
				CREATE TABLE IF NOT EXISTS data_source_import_job_errors (
					id UUID PRIMARY KEY,
					job_id UUID NOT NULL REFERENCES data_source_import_jobs(id) ON DELETE CASCADE,
					row_index INTEGER NOT NULL,
					column_name VARCHAR(255) NULL,
					raw_value TEXT NULL,
					error_code VARCHAR(64) NOT NULL,
					error_message TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_source_import_jobs_scope_created_at
				ON data_source_import_jobs (organization_id, data_source_id, created_at DESC)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_source_import_jobs_status_created_at
				ON data_source_import_jobs (status, created_at DESC)
				`,
				`
				CREATE INDEX IF NOT EXISTS idx_data_source_import_job_errors_job_row
				ON data_source_import_job_errors (job_id, row_index)
				`,
			}

			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			statements := []string{
				`DROP INDEX IF EXISTS idx_data_source_import_job_errors_job_row`,
				`DROP INDEX IF EXISTS idx_data_source_import_jobs_status_created_at`,
				`DROP INDEX IF EXISTS idx_data_source_import_jobs_scope_created_at`,
				`DROP TABLE IF EXISTS data_source_import_job_errors`,
				`DROP TABLE IF EXISTS data_source_import_jobs`,
			}

			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}
