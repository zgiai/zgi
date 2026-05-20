package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0034_seed_workflow_default_prompts() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2WorkflowDefaultPromptsID,
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				INSERT INTO public.app_prompts (
					id, organization_id, workspace_id, owner_account_id, source, name, slug, description, locale, category, tags, latest_version, created_at, updated_at
				) VALUES
				(
					'2d35f08e-5c52-43ef-bf63-bca3d5ae86ab', NULL, NULL, NULL, 'official',
					'Workflow Task Assistant', 'official/workflow-task-assistant',
					'Official default prompt for task-oriented workflow execution nodes.',
					'en-US', 'workflow-default', '["official","workflow","default"]'::jsonb, 1, NOW(), NOW()
				),
				(
					'9c6ff0a8-c53f-42b7-87c2-1d2f9f7f1d08', NULL, NULL, NULL, 'official',
					'Workflow Task Assistant', 'official/workflow-task-assistant',
					'Official default prompt for task-oriented workflow execution nodes.',
					'zh-Hans', 'workflow-default', '["official","workflow","default"]'::jsonb, 1, NOW(), NOW()
				)
				ON CONFLICT (id) DO NOTHING;

				INSERT INTO public.app_prompt_versions (
					id, prompt_id, version, prompt_type, content, config, labels, commit_message, created_by, created_at, updated_at
				) VALUES
				(
					'ecf3b3de-dfb0-4a60-af03-6e48c8fb8e0d', '2d35f08e-5c52-43ef-bf63-bca3d5ae86ab', 1, 'text',
					to_jsonb('You are a workflow task assistant. Follow the provided input carefully, produce clear and actionable output, ask for missing context only when absolutely necessary, and avoid fabricating facts or decisions.'::text),
					'{}'::jsonb,
					'["production","latest"]'::jsonb,
					'Initial official workflow default prompt',
					NULL,
					NOW(),
					NOW()
				),
				(
					'658faf9a-5364-4d43-a6f3-dca596d54f12', '9c6ff0a8-c53f-42b7-87c2-1d2f9f7f1d08', 1, 'text',
					to_jsonb('你是一名工作流任务助手。请严格根据提供的输入完成任务，输出清晰、可执行、可复用的结果；只有在确实缺少必要上下文时才提出澄清问题，不要虚构事实、结论或承诺。'::text),
					'{}'::jsonb,
					'["production","latest"]'::jsonb,
					'Initial official workflow default prompt',
					NULL,
					NOW(),
					NOW()
				)
				ON CONFLICT (id) DO NOTHING;
			`).Error; err != nil {
				return fmt.Errorf("seed workflow default prompts: %w", err)
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				DELETE FROM public.app_prompt_versions
				WHERE id IN (
					'ecf3b3de-dfb0-4a60-af03-6e48c8fb8e0d',
					'658faf9a-5364-4d43-a6f3-dca596d54f12'
				);

				DELETE FROM public.app_prompts
				WHERE id IN (
					'2d35f08e-5c52-43ef-bf63-bca3d5ae86ab',
					'9c6ff0a8-c53f-42b7-87c2-1d2f9f7f1d08'
				);
			`).Error; err != nil {
				return fmt.Errorf("rollback workflow default prompts seed: %w", err)
			}
			return nil
		},
	}
}
