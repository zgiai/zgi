package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0033_create_prompt_optimization_runs() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2PromptOptimizationRunsID,
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS public.app_prompt_optimization_runs (
					id UUID PRIMARY KEY,
					organization_id UUID NOT NULL,
					workspace_id UUID NULL,
					prompt_id UUID NULL REFERENCES public.app_prompts(id) ON DELETE SET NULL,
					account_id UUID NOT NULL,
					goal VARCHAR(32) NOT NULL DEFAULT 'general',
					provider VARCHAR(128) NULL,
					model VARCHAR(255) NULL,
					preserve_variables BOOLEAN NOT NULL DEFAULT TRUE,
					detected_variables JSONB NOT NULL DEFAULT '[]'::jsonb,
					raw_prompt TEXT NOT NULL,
					safe_output TEXT NOT NULL,
					balanced_output TEXT NOT NULL,
					advanced_output TEXT NOT NULL,
					adopted_variant VARCHAR(16) NULL,
					adopted_prompt_version_id UUID NULL REFERENCES public.app_prompt_versions(id) ON DELETE SET NULL,
					adopted_at TIMESTAMPTZ NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				CREATE INDEX IF NOT EXISTS idx_prompt_opt_runs_account_prompt_created
				ON public.app_prompt_optimization_runs(account_id, prompt_id, created_at DESC);

				CREATE INDEX IF NOT EXISTS idx_prompt_opt_runs_org_workspace_created
				ON public.app_prompt_optimization_runs(organization_id, workspace_id, created_at DESC);
			`).Error; err != nil {
				return fmt.Errorf("create prompt optimization runs table: %w", err)
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				DROP TABLE IF EXISTS public.app_prompt_optimization_runs;
			`).Error; err != nil {
				return fmt.Errorf("drop prompt optimization runs table: %w", err)
			}
			return nil
		},
	}
}
