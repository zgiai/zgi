package migrationsv2

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func M0032_create_prompt_library() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: migrationV2CreatePromptLibraryID,
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				CREATE TABLE IF NOT EXISTS public.app_prompts (
					id UUID PRIMARY KEY,
					organization_id UUID NULL,
					workspace_id UUID NULL,
					owner_account_id UUID NULL,
					source VARCHAR(32) NOT NULL,
					name VARCHAR(255) NOT NULL,
					slug VARCHAR(255) NOT NULL,
					description TEXT NULL,
					locale VARCHAR(32) NOT NULL DEFAULT 'zh-Hans',
					category VARCHAR(128) NULL,
					tags JSONB NOT NULL DEFAULT '[]'::jsonb,
					latest_version INTEGER NOT NULL DEFAULT 1,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				);

				CREATE INDEX IF NOT EXISTS idx_app_prompts_org_workspace_source
				ON public.app_prompts(organization_id, workspace_id, source);

				CREATE INDEX IF NOT EXISTS idx_app_prompts_slug_locale
				ON public.app_prompts(slug, locale);

				CREATE TABLE IF NOT EXISTS public.app_prompt_versions (
					id UUID PRIMARY KEY,
					prompt_id UUID NOT NULL REFERENCES public.app_prompts(id) ON DELETE CASCADE,
					version INTEGER NOT NULL,
					prompt_type VARCHAR(16) NOT NULL,
					content JSONB NOT NULL,
					config JSONB NOT NULL DEFAULT '{}'::jsonb,
					labels JSONB NOT NULL DEFAULT '[]'::jsonb,
					commit_message TEXT NULL,
					created_by UUID NULL,
					created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					UNIQUE(prompt_id, version)
				);

				CREATE INDEX IF NOT EXISTS idx_app_prompt_versions_prompt
				ON public.app_prompt_versions(prompt_id, version DESC);
			`).Error; err != nil {
				return fmt.Errorf("create prompt library tables: %w", err)
			}

			if err := tx.Exec(`
				INSERT INTO public.app_prompts (
					id, organization_id, workspace_id, owner_account_id, source, name, slug, description, locale, category, tags, latest_version, created_at, updated_at
				) VALUES
				(
					'3db60d44-f6a7-4892-8e65-9b7f95f69ab1', NULL, NULL, NULL, 'official',
					'Enterprise Assistant Answer', 'official/enterprise-assistant-answer',
					'Official assistant prompt for internal policy answers, service guidance, and enterprise Q&A.',
					'en-US', 'knowledge-service', '["official","assistant","qa"]'::jsonb, 1, NOW(), NOW()
				),
				(
					'9fa2ec04-0672-4e4f-9af6-ac4630f542ff', NULL, NULL, NULL, 'official',
					'Enterprise Assistant Answer', 'official/enterprise-assistant-answer',
					'Official assistant prompt for internal policy answers, service guidance, and enterprise Q&A.',
					'zh-Hans', 'knowledge-service', '["official","assistant","qa"]'::jsonb, 1, NOW(), NOW()
				),
				(
					'c2b3cc59-36f7-4401-9e0f-c775c478a0f3', NULL, NULL, NULL, 'official',
					'Service Request Triage Classifier', 'official/service-request-triage-classifier',
					'Official classifier prompt for routing service requests by urgency and handling path.',
					'en-US', 'customer-support', '["official","triage","routing"]'::jsonb, 1, NOW(), NOW()
				),
				(
					'ca975ea8-8105-40d0-a9fd-b5865280f906', NULL, NULL, NULL, 'official',
					'Service Request Triage Classifier', 'official/service-request-triage-classifier',
					'Official classifier prompt for routing service requests by urgency and handling path.',
					'zh-Hans', 'customer-support', '["official","triage","routing"]'::jsonb, 1, NOW(), NOW()
				),
				(
					'8e0ca70d-42b9-4727-8ab3-985886fc2a31', NULL, NULL, NULL, 'official',
					'Customer Support Reply', 'official/customer-support-reply',
					'Friendly, policy-aware customer support reply template for common service conversations.',
					'en-US', 'customer-support', '["official","support","reply"]'::jsonb, 1, NOW(), NOW()
				),
				(
					'09f473c0-39a0-49bb-bf77-5d7b2c0baee2', NULL, NULL, NULL, 'official',
					'Customer Support Reply', 'official/customer-support-reply',
					'Friendly, policy-aware customer support reply template for common service conversations.',
					'zh-Hans', 'customer-support', '["official","support","reply"]'::jsonb, 1, NOW(), NOW()
				),
				(
					'14f4b8e4-92d3-494f-b61c-36419fc51f8a', NULL, NULL, NULL, 'official',
					'Meeting Summary Action Items', 'official/meeting-summary-action-items',
					'Turn long meeting notes into concise summary, decisions, risks, and next actions.',
					'en-US', 'meeting-summary', '["official","meeting","summary"]'::jsonb, 1, NOW(), NOW()
				),
				(
					'3756bb26-239b-4ad0-a056-bd4147cb6187', NULL, NULL, NULL, 'official',
					'Meeting Summary Action Items', 'official/meeting-summary-action-items',
					'Turn long meeting notes into concise summary, decisions, risks, and next actions.',
					'zh-Hans', 'meeting-summary', '["official","meeting","summary"]'::jsonb, 1, NOW(), NOW()
				)
				ON CONFLICT (id) DO NOTHING;

				INSERT INTO public.app_prompt_versions (
					id, prompt_id, version, prompt_type, content, config, labels, commit_message, created_by, created_at, updated_at
				) VALUES
				(
					'08bf7b1b-18ef-47dd-88cc-5c1b09d39fea', '3db60d44-f6a7-4892-8e65-9b7f95f69ab1', 1, 'text',
					to_jsonb('You are an enterprise assistant. Answer clearly, ask for missing context when needed, and keep the response actionable. Prefer concise business language and preserve policy boundaries. User question: {{#sys.query#}}'::text),
					'{}'::jsonb,
					'["production","latest"]'::jsonb,
					'Initial official enterprise assistant prompt',
					NULL,
					NOW(),
					NOW()
				),
				(
					'52ba995e-a218-4967-a2e0-5c0f2fdb9574', '9fa2ec04-0672-4e4f-9af6-ac4630f542ff', 1, 'text',
					to_jsonb('你是一名企业助手。请清晰回答问题，在必要时追问缺失上下文，并保持回复可执行、简洁、符合业务表达习惯。严格遵守政策边界。用户问题：{{#sys.query#}}'::text),
					'{}'::jsonb,
					'["production","latest"]'::jsonb,
					'Initial official enterprise assistant prompt',
					NULL,
					NOW(),
					NOW()
				),
				(
					'182fd143-b686-40e9-915e-a5be0fd10e46', 'c2b3cc59-36f7-4401-9e0f-c775c478a0f3', 1, 'text',
					to_jsonb('Classify the latest request as urgent or standard. Return JSON with keys route and reason only. Use route=urgent for service-impacting or time-sensitive incidents; otherwise use route=standard. Request: {{#sys.query#}}'::text),
					'{}'::jsonb,
					'["production","latest"]'::jsonb,
					'Initial official service request triage classifier',
					NULL,
					NOW(),
					NOW()
				),
				(
					'0cc72fa3-0d89-4f7c-9278-341b574c659f', 'ca975ea8-8105-40d0-a9fd-b5865280f906', 1, 'text',
					to_jsonb('请将最新请求分类为 urgent 或 standard，并仅返回包含 route 和 reason 两个字段的 JSON。对于影响服务或时间敏感的问题使用 route=urgent，否则使用 route=standard。请求内容：{{#sys.query#}}'::text),
					'{}'::jsonb,
					'["production","latest"]'::jsonb,
					'Initial official service request triage classifier',
					NULL,
					NOW(),
					NOW()
				),
				(
					'5d518f90-460d-4438-b801-46a82fa5f19f', '8e0ca70d-42b9-4727-8ab3-985886fc2a31', 1, 'text',
					to_jsonb('You are a professional customer support specialist. Reply with empathy, clarity, and actionability. Keep a calm and reassuring tone. Preserve factual policy boundaries, never invent compensation or commitments, and ask concise follow-up questions only when needed. Use the customer''s language. Customer message: {{customer_message}}'::text),
					'{}'::jsonb,
					'["production","latest"]'::jsonb,
					'Initial official support reply prompt',
					NULL,
					NOW(),
					NOW()
				),
				(
					'56db4c26-c7f6-49da-995b-56d2be40cb34', '09f473c0-39a0-49bb-bf77-5d7b2c0baee2', 1, 'text',
					to_jsonb('你是一名专业的客服支持专家。请用同理心、清晰度和可执行建议来回复用户，语气稳定、真诚、不过度承诺。严格遵守事实和政策边界，不捏造补偿方案；只有在必要时才提出简洁的澄清问题。请使用用户当前语言回复。用户消息：{{customer_message}}'::text),
					'{}'::jsonb,
					'["production","latest"]'::jsonb,
					'Initial official support reply prompt',
					NULL,
					NOW(),
					NOW()
				),
				(
					'553403fd-5001-4669-87ef-f6d12a33552c', '14f4b8e4-92d3-494f-b61c-36419fc51f8a', 1, 'text',
					to_jsonb('You are an expert meeting analyst. Transform the meeting notes into: 1) executive summary, 2) decisions made, 3) open risks, 4) action items with owners if mentioned, and 5) recommended follow-ups. Keep it concise, structured, and suitable for sharing with teammates. Meeting notes: {{meeting_notes}}'::text),
					'{}'::jsonb,
					'["production","latest"]'::jsonb,
					'Initial official meeting summary prompt',
					NULL,
					NOW(),
					NOW()
				),
				(
					'77ee85d5-fa47-4695-8f99-db4fb2c27c4c', '3756bb26-239b-4ad0-a056-bd4147cb6187', 1, 'text',
					to_jsonb('你是一名专业的会议分析助手。请把会议记录整理为：1）执行摘要，2）已达成结论，3）待关注风险，4）行动项及负责人（如有提及），5）建议的后续跟进。输出要简洁、结构化，并适合直接分享给同事。会议记录：{{meeting_notes}}'::text),
					'{}'::jsonb,
					'["production","latest"]'::jsonb,
					'Initial official meeting summary prompt',
					NULL,
					NOW(),
					NOW()
				)
				ON CONFLICT (id) DO NOTHING;
			`).Error; err != nil {
				return fmt.Errorf("seed official prompt library entries: %w", err)
			}
			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`
				DROP TABLE IF EXISTS public.app_prompt_versions;
				DROP TABLE IF EXISTS public.app_prompts;
			`).Error; err != nil {
				return fmt.Errorf("drop prompt library tables: %w", err)
			}
			return nil
		},
	}
}
