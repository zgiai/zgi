package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// M0112_migrate_old_permissions replaces legacy "group.*" permission codes
// with "workspace.*" in the roles.permissions JSONB column, and removes duplicates.
func M0112_migrate_old_permissions() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260209000112",
		Migrate: func(tx *gorm.DB) error {
			// Replace old permission strings in the JSONB array:
			//   group.view             -> workspace.view
			//   group.manage           -> workspace.manage
			//   group.billing_audit    -> workspace.billing_audit
			//   group.transfer_archive -> workspace.transfer_archive
			// Then remove duplicates that may result from the replacement.
			return tx.Exec(`
				UPDATE roles
				SET permissions = (
					SELECT jsonb_agg(DISTINCT elem)
					FROM (
						SELECT
							CASE elem::text
								WHEN '"group.view"'             THEN '"workspace.view"'::jsonb
								WHEN '"group.manage"'           THEN '"workspace.manage"'::jsonb
								WHEN '"group.billing_audit"'    THEN '"workspace.billing_audit"'::jsonb
								WHEN '"group.transfer_archive"' THEN '"workspace.transfer_archive"'::jsonb
								ELSE elem
							END AS elem
						FROM jsonb_array_elements(permissions) AS elem
					) sub
				)
				WHERE permissions::text LIKE '%group.%';
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			// Reverse: workspace.* -> group.*
			return tx.Exec(`
				UPDATE roles
				SET permissions = (
					SELECT jsonb_agg(DISTINCT elem)
					FROM (
						SELECT
							CASE elem::text
								WHEN '"workspace.view"'             THEN '"group.view"'::jsonb
								WHEN '"workspace.manage"'           THEN '"group.manage"'::jsonb
								WHEN '"workspace.billing_audit"'    THEN '"group.billing_audit"'::jsonb
								WHEN '"workspace.transfer_archive"' THEN '"group.transfer_archive"'::jsonb
								ELSE elem
							END AS elem
						FROM jsonb_array_elements(permissions) AS elem
					) sub
				)
				WHERE permissions::text LIKE '%workspace.view%'
				   OR permissions::text LIKE '%workspace.manage%'
				   OR permissions::text LIKE '%workspace.billing_audit%'
				   OR permissions::text LIKE '%workspace.transfer_archive%';
			`).Error
		},
	}
}
