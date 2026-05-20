package migrationsv2

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0033_harden_content_parse_playground_sharing() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2ContentParseShareHardeningID,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`ALTER TABLE content_parse_playground_runs ALTER COLUMN is_share_enabled SET DEFAULT FALSE`,
				`UPDATE content_parse_playground_runs SET is_share_enabled = FALSE WHERE is_share_enabled = TRUE`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return err
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`ALTER TABLE content_parse_playground_runs ALTER COLUMN is_share_enabled SET DEFAULT FALSE`).Error
		},
	}
}
