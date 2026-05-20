package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0091_backfill_workspace_member_role_id() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601240091",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				UPDATE workspace_members
				SET role_id = CASE lower(role)
					WHEN 'owner' THEN '00000000-0000-0000-0000-000000000001'::uuid
					WHEN 'admin' THEN '00000000-0000-0000-0000-000000000002'::uuid
					WHEN 'viewer' THEN '00000000-0000-0000-0000-000000000004'::uuid
					WHEN 'normal' THEN '00000000-0000-0000-0000-000000000003'::uuid
					WHEN 'member' THEN '00000000-0000-0000-0000-000000000003'::uuid
					WHEN 'editor' THEN '00000000-0000-0000-0000-000000000003'::uuid
					ELSE '00000000-0000-0000-0000-000000000003'::uuid
				END
				WHERE role_id IS NULL
				  AND role IS NOT NULL
				  AND role <> ''
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return nil
		},
	}
}
