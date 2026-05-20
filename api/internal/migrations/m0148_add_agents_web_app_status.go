package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0148_add_agents_web_app_status() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "20260426000148",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				ALTER TABLE agents
				ADD COLUMN IF NOT EXISTS web_app_status VARCHAR(20) NOT NULL DEFAULT 'active',
				ADD COLUMN IF NOT EXISTS web_app_offlined_at TIMESTAMPTZ,
				ADD COLUMN IF NOT EXISTS web_app_offlined_by UUID,
				ADD COLUMN IF NOT EXISTS web_app_offline_reason TEXT NOT NULL DEFAULT ''
			`).Error; err != nil {
				return err
			}

			return tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_agents_web_app_status
				ON agents(web_app_status)
				WHERE deleted_at IS NULL
			`).Error
		},
		Rollback: func(tx *gorm.DB) error {
			sqls := []string{
				`DROP INDEX IF EXISTS idx_agents_web_app_status`,
				`ALTER TABLE agents DROP COLUMN IF EXISTS web_app_offline_reason`,
				`ALTER TABLE agents DROP COLUMN IF EXISTS web_app_offlined_by`,
				`ALTER TABLE agents DROP COLUMN IF EXISTS web_app_offlined_at`,
				`ALTER TABLE agents DROP COLUMN IF EXISTS web_app_status`,
			}

			for _, sql := range sqls {
				if err := tx.Exec(sql).Error; err != nil {
					return err
				}
			}
			return nil
		},
	}
}
