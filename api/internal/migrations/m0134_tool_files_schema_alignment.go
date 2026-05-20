package migrations

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0134_tool_files_schema_alignment repairs legacy tool_files tables that were
// created before lifecycle and timestamp metadata were introduced.
func M0134_tool_files_schema_alignment() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202603150134",
		Migrate: func(tx *gorm.DB) error {
			if !tx.Migrator().HasTable("tool_files") {
				return nil
			}

			if err := addToolFilesColumnIfMissing(tx, "lifecycle", toolFilesLifecycleColumnSQL(tx)); err != nil {
				return err
			}
			if err := addToolFilesColumnIfMissing(tx, "expires_at", toolFilesNullableTimestampColumnSQL(tx, "expires_at")); err != nil {
				return err
			}
			if err := addToolFilesColumnIfMissing(tx, "created_at", toolFilesNullableTimestampColumnSQL(tx, "created_at")); err != nil {
				return err
			}
			if err := addToolFilesColumnIfMissing(tx, "updated_at", toolFilesNullableTimestampColumnSQL(tx, "updated_at")); err != nil {
				return err
			}
			if err := addToolFilesColumnIfMissing(tx, "deleted_at", toolFilesNullableTimestampColumnSQL(tx, "deleted_at")); err != nil {
				return err
			}

			if err := tx.Exec(`
				UPDATE tool_files
				SET lifecycle = COALESCE(lifecycle, 'persistent')
			`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`
				UPDATE tool_files
				SET created_at = COALESCE(created_at, CURRENT_TIMESTAMP),
					updated_at = COALESCE(updated_at, CURRENT_TIMESTAMP)
			`).Error; err != nil {
				return err
			}

			if tx.Dialector.Name() == "postgres" {
				for _, stmt := range []string{
					`ALTER TABLE tool_files ALTER COLUMN lifecycle SET DEFAULT 'persistent'`,
					`ALTER TABLE tool_files ALTER COLUMN lifecycle SET NOT NULL`,
					`ALTER TABLE tool_files ALTER COLUMN created_at SET DEFAULT CURRENT_TIMESTAMP`,
					`ALTER TABLE tool_files ALTER COLUMN created_at SET NOT NULL`,
					`ALTER TABLE tool_files ALTER COLUMN updated_at SET DEFAULT CURRENT_TIMESTAMP`,
					`ALTER TABLE tool_files ALTER COLUMN updated_at SET NOT NULL`,
				} {
					if err := tx.Exec(stmt).Error; err != nil {
						return err
					}
				}
			}

			for _, stmt := range []string{
				`CREATE INDEX IF NOT EXISTS idx_tool_files_lifecycle_expires_at ON tool_files (lifecycle, expires_at)`,
				`CREATE INDEX IF NOT EXISTS idx_tool_files_deleted_at ON tool_files (deleted_at)`,
			} {
				if err := tx.Exec(stmt).Error; err != nil {
					return err
				}
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return fmt.Errorf("migration %s is irreversible", "202603150134")
		},
	}
}

func addToolFilesColumnIfMissing(tx *gorm.DB, columnName, sql string) error {
	if tx.Migrator().HasColumn("tool_files", columnName) {
		return nil
	}
	return tx.Exec(sql).Error
}

func toolFilesLifecycleColumnSQL(tx *gorm.DB) string {
	if tx.Dialector.Name() == "postgres" {
		return `ALTER TABLE tool_files ADD COLUMN lifecycle VARCHAR(32)`
	}
	return `ALTER TABLE tool_files ADD COLUMN lifecycle TEXT`
}

func toolFilesNullableTimestampColumnSQL(tx *gorm.DB, columnName string) string {
	columnType := "TIMESTAMPTZ"
	if tx.Dialector.Name() == "sqlite" {
		columnType = "DATETIME"
	}
	return fmt.Sprintf(`ALTER TABLE tool_files ADD COLUMN %s %s`, columnName, columnType)
}
