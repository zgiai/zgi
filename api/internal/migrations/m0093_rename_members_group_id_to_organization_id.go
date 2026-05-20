package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0093_rename_members_group_id_to_organization_id() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601270093",
		Migrate: func(tx *gorm.DB) error {
			var orgIdCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'members' AND column_name = 'organization_id'").Scan(&orgIdCount).Error; err != nil {
				return err
			}
			var groupIdCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'members' AND column_name = 'group_id'").Scan(&groupIdCount).Error; err != nil {
				return err
			}

			if groupIdCount > 0 && orgIdCount == 0 {
				if err := tx.Exec("ALTER TABLE members RENAME COLUMN group_id TO organization_id").Error; err != nil {
					return err
				}
			} else if groupIdCount == 0 && orgIdCount == 0 {
				if err := tx.Exec("ALTER TABLE members ADD COLUMN IF NOT EXISTS organization_id UUID").Error; err != nil {
					return err
				}
			}

			var oldIndexCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'members' AND indexname = 'members_group_id_idx'").Scan(&oldIndexCount).Error; err != nil {
				return err
			}
			if oldIndexCount > 0 {
				if err := tx.Exec("ALTER INDEX IF EXISTS members_group_id_idx RENAME TO members_organization_id_idx").Error; err != nil {
					return err
				}
			}

			if err := tx.Exec("DROP VIEW IF EXISTS enterprise_group_account_joins").Error; err != nil {
				return err
			}
			if err := tx.Exec(`
				CREATE VIEW enterprise_group_account_joins AS
				SELECT
					organization_id AS group_id,
					account_id,
					role,
					status,
					created_at,
					updated_at
				FROM members
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			var orgIdExists int64
			if err := tx.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = 'members' AND column_name = 'organization_id'").Scan(&orgIdExists).Error; err != nil {
				return err
			}
			if orgIdExists == 0 {
				return nil
			}

			if err := tx.Exec("DROP VIEW IF EXISTS enterprise_group_account_joins").Error; err != nil {
				return err
			}

			var newIndexCount int64
			if err := tx.Raw("SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'members' AND indexname = 'members_organization_id_idx'").Scan(&newIndexCount).Error; err != nil {
				return err
			}
			if newIndexCount > 0 {
				if err := tx.Exec("ALTER INDEX IF EXISTS members_organization_id_idx RENAME TO members_group_id_idx").Error; err != nil {
					return err
				}
			}

			if err := tx.Exec("ALTER TABLE members RENAME COLUMN organization_id TO group_id").Error; err != nil {
				return err
			}

			if err := tx.Exec("CREATE VIEW enterprise_group_account_joins AS SELECT * FROM members").Error; err != nil {
				return err
			}

			return nil
		},
	}
}
