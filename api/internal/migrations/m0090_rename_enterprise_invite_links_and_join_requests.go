package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0090_rename_enterprise_invite_links_and_join_requests() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601190090",
		Migrate: func(tx *gorm.DB) error {
			// 1. Rename table enterprise_invite_links -> organization_invite_links
			if err := tx.Exec("ALTER TABLE enterprise_invite_links RENAME TO organization_invite_links").Error; err != nil {
				return err
			}
			// 2. Create view enterprise_invite_links for backward compatibility
			if err := tx.Exec("CREATE VIEW enterprise_invite_links AS SELECT * FROM organization_invite_links").Error; err != nil {
				return err
			}

			// 3. Rename table enterprise_join_requests -> organization_join_requests
			if err := tx.Exec("ALTER TABLE enterprise_join_requests RENAME TO organization_join_requests").Error; err != nil {
				return err
			}
			// 4. Create view enterprise_join_requests for backward compatibility
			if err := tx.Exec("CREATE VIEW enterprise_join_requests AS SELECT * FROM organization_join_requests").Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// 1. Drop view enterprise_join_requests
			if err := tx.Exec("DROP VIEW IF EXISTS enterprise_join_requests").Error; err != nil {
				return err
			}
			// 2. Rename table back to enterprise_join_requests
			if err := tx.Exec("ALTER TABLE organization_join_requests RENAME TO enterprise_join_requests").Error; err != nil {
				return err
			}

			// 3. Drop view enterprise_invite_links
			if err := tx.Exec("DROP VIEW IF EXISTS enterprise_invite_links").Error; err != nil {
				return err
			}
			// 4. Rename table back to enterprise_invite_links
			if err := tx.Exec("ALTER TABLE organization_invite_links RENAME TO enterprise_invite_links").Error; err != nil {
				return err
			}

			return nil
		},
	}
}
