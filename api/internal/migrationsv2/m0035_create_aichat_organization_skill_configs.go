package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0035_create_aichat_organization_skill_configs() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2AIChatOrganizationSkillsID,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE IF NOT EXISTS public.aichat_organization_skill_configs (
					organization_id UUID NOT NULL,
					skill_id VARCHAR(128) NOT NULL,
					enabled BOOLEAN NOT NULL DEFAULT true,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					PRIMARY KEY (organization_id, skill_id)
				)`,
				`CREATE INDEX IF NOT EXISTS idx_aichat_organization_skill_configs_enabled
					ON public.aichat_organization_skill_configs(organization_id, enabled)`,
				`INSERT INTO public.aichat_organization_skill_configs (organization_id, skill_id, enabled, created_at, updated_at)
					SELECT o.id, skill.skill_id, true, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
					FROM public.organizations o
					CROSS JOIN (VALUES ('time'), ('calculator'), ('file-generator')) AS skill(skill_id)
					ON CONFLICT (organization_id, skill_id) DO NOTHING`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return fmt.Errorf("create aichat organization skill configs: %w", err)
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			statements := []string{
				`DROP INDEX IF EXISTS public.idx_aichat_organization_skill_configs_enabled`,
				`DROP TABLE IF EXISTS public.aichat_organization_skill_configs`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return fmt.Errorf("rollback aichat organization skill configs: %w", err)
				}
			}
			return nil
		},
	}
}
