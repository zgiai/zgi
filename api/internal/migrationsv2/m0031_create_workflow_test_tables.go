package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0031_create_workflow_test_tables() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2CreateWorkflowTestTablesID,
		Migrate: func(tx *gorm.DB) error {
			statements := []string{
				`CREATE TABLE IF NOT EXISTS public.workflow_test_settings (
					id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
					agent_id UUID NOT NULL REFERENCES public.agents(id) ON DELETE CASCADE,
					judge_prompt_template TEXT NOT NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					CONSTRAINT uq_workflow_test_settings_agent UNIQUE (agent_id)
				)`,
				`CREATE TABLE IF NOT EXISTS public.workflow_test_scenarios (
					id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
					agent_id UUID NOT NULL REFERENCES public.agents(id) ON DELETE CASCADE,
					name VARCHAR(120) NOT NULL,
					description TEXT NOT NULL DEFAULT '',
					source VARCHAR(32) NOT NULL DEFAULT 'manual',
					case_count INT NOT NULL DEFAULT 0,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS public.workflow_test_cases (
					id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
					agent_id UUID NOT NULL REFERENCES public.agents(id) ON DELETE CASCADE,
					scenario_id UUID REFERENCES public.workflow_test_scenarios(id) ON DELETE SET NULL,
					content TEXT NOT NULL,
					question_type VARCHAR(32) NOT NULL DEFAULT 'core',
					status VARCHAR(32) NOT NULL DEFAULT 'enabled',
					turns JSONB NOT NULL DEFAULT '[]'::jsonb,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS public.workflow_test_batches (
					id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
					agent_id UUID NOT NULL REFERENCES public.agents(id) ON DELETE CASCADE,
					name VARCHAR(160) NOT NULL,
					status VARCHAR(32) NOT NULL DEFAULT 'queued',
					case_count INT NOT NULL DEFAULT 0,
					passed_count INT NOT NULL DEFAULT 0,
					failed_count INT NOT NULL DEFAULT 0,
					review_count INT NOT NULL DEFAULT 0,
					judge_prompt_snapshot TEXT NOT NULL,
					summary TEXT NOT NULL DEFAULT '',
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE TABLE IF NOT EXISTS public.workflow_test_batch_items (
					id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
					agent_id UUID NOT NULL REFERENCES public.agents(id) ON DELETE CASCADE,
					batch_id UUID NOT NULL REFERENCES public.workflow_test_batches(id) ON DELETE CASCADE,
					case_id UUID NOT NULL,
					case_snapshot JSONB NOT NULL,
					status VARCHAR(32) NOT NULL DEFAULT 'pending',
					workflow_run_id VARCHAR(120) NOT NULL DEFAULT '',
					outputs JSONB NOT NULL DEFAULT '{}'::jsonb,
					error TEXT NOT NULL DEFAULT '',
					judge_reason TEXT NOT NULL DEFAULT '',
					judge_suggestion TEXT NOT NULL DEFAULT '',
					judge_confidence NUMERIC(5,4) NOT NULL DEFAULT 0,
					created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
				)`,
				`CREATE INDEX IF NOT EXISTS idx_workflow_test_scenarios_agent
					ON public.workflow_test_scenarios(agent_id, created_at DESC)`,
				`CREATE UNIQUE INDEX IF NOT EXISTS uq_workflow_test_scenarios_agent_name
					ON public.workflow_test_scenarios(agent_id, name)`,
				`CREATE INDEX IF NOT EXISTS idx_workflow_test_cases_agent_status
					ON public.workflow_test_cases(agent_id, status, created_at DESC)`,
				`CREATE INDEX IF NOT EXISTS idx_workflow_test_cases_scenario
					ON public.workflow_test_cases(scenario_id)`,
				`CREATE INDEX IF NOT EXISTS idx_workflow_test_batches_agent_status
					ON public.workflow_test_batches(agent_id, status, created_at DESC)`,
				`CREATE INDEX IF NOT EXISTS idx_workflow_test_batch_items_batch
					ON public.workflow_test_batch_items(batch_id, created_at ASC)`,
				`CREATE INDEX IF NOT EXISTS idx_workflow_test_batch_items_agent
					ON public.workflow_test_batch_items(agent_id, status)`,
				`CREATE INDEX IF NOT EXISTS idx_workflow_test_batch_items_case
					ON public.workflow_test_batch_items(case_id)`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return fmt.Errorf("create workflow test tables: %w", err)
				}
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			statements := []string{
				`DROP INDEX IF EXISTS public.idx_workflow_test_batch_items_agent`,
				`DROP INDEX IF EXISTS public.idx_workflow_test_batch_items_case`,
				`DROP INDEX IF EXISTS public.idx_workflow_test_batch_items_batch`,
				`DROP INDEX IF EXISTS public.idx_workflow_test_batches_agent_status`,
				`DROP INDEX IF EXISTS public.idx_workflow_test_cases_scenario`,
				`DROP INDEX IF EXISTS public.idx_workflow_test_cases_agent_status`,
				`DROP INDEX IF EXISTS public.uq_workflow_test_scenarios_agent_name`,
				`DROP INDEX IF EXISTS public.idx_workflow_test_scenarios_agent`,
				`DROP TABLE IF EXISTS public.workflow_test_batch_items`,
				`DROP TABLE IF EXISTS public.workflow_test_batches`,
				`DROP TABLE IF EXISTS public.workflow_test_cases`,
				`DROP TABLE IF EXISTS public.workflow_test_scenarios`,
				`DROP TABLE IF EXISTS public.workflow_test_settings`,
			}
			for _, statement := range statements {
				if err := tx.Exec(statement).Error; err != nil {
					return fmt.Errorf("rollback workflow test tables: %w", err)
				}
			}
			return nil
		},
	}
}
