package migrationsv2

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0036_create_aichat_custom_skills() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2AIChatCustomSkillsID,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE IF NOT EXISTS public.aichat_custom_skills (
					id UUID PRIMARY KEY,
					organization_id UUID NOT NULL,
					skill_id VARCHAR(128) NOT NULL,
					name VARCHAR(128) NOT NULL,
					description TEXT NOT NULL,
					when_to_use TEXT NOT NULL,
					runtime_type VARCHAR(32) NOT NULL DEFAULT 'prompt',
					display JSONB NOT NULL DEFAULT '{}'::jsonb,
					storage_path TEXT NOT NULL,
					manifest JSONB NOT NULL DEFAULT '{}'::jsonb,
					status VARCHAR(32) NOT NULL DEFAULT 'active',
					validation_error TEXT,
					created_by UUID NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					deleted_at TIMESTAMPTZ
				)`,
				`CREATE UNIQUE INDEX IF NOT EXISTS idx_aichat_custom_skills_org_skill_active
					ON public.aichat_custom_skills(organization_id, skill_id)
					WHERE deleted_at IS NULL`,
				`CREATE INDEX IF NOT EXISTS idx_aichat_custom_skills_org_status
					ON public.aichat_custom_skills(organization_id, status)
					WHERE deleted_at IS NULL`,
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
				`DROP INDEX IF EXISTS public.idx_aichat_custom_skills_org_status`,
				`DROP INDEX IF EXISTS public.idx_aichat_custom_skills_org_skill_active`,
				`DROP TABLE IF EXISTS public.aichat_custom_skills`,
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
