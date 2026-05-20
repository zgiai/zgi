package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0025_add_agents_web_app_status() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2AddAgentsWebAppStatusID,
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				ALTER TABLE IF EXISTS public.agents
					ADD COLUMN IF NOT EXISTS web_app_status VARCHAR(20) NOT NULL DEFAULT 'active',
					ADD COLUMN IF NOT EXISTS web_app_offlined_at TIMESTAMPTZ,
					ADD COLUMN IF NOT EXISTS web_app_offlined_by UUID,
					ADD COLUMN IF NOT EXISTS web_app_offline_reason TEXT NOT NULL DEFAULT ''
			`).Error; err != nil {
				return fmt.Errorf("add agents web app status columns: %w", err)
			}

			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_agents_web_app_status
				ON public.agents(web_app_status)
				WHERE deleted_at IS NULL
			`).Error; err != nil {
				return fmt.Errorf("create agents web app status index: %w", err)
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			statements := []string{
				`DROP INDEX IF EXISTS public.idx_agents_web_app_status`,
				`ALTER TABLE IF EXISTS public.agents DROP COLUMN IF EXISTS web_app_offline_reason`,
				`ALTER TABLE IF EXISTS public.agents DROP COLUMN IF EXISTS web_app_offlined_by`,
				`ALTER TABLE IF EXISTS public.agents DROP COLUMN IF EXISTS web_app_offlined_at`,
				`ALTER TABLE IF EXISTS public.agents DROP COLUMN IF EXISTS web_app_status`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return fmt.Errorf("rollback agents web app status: %w", err)
				}
			}
			return nil
		},
	}
}
