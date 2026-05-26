package seeders

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/zgiai/zgi/api/pkg/logger"
	pkguuid "github.com/zgiai/zgi/api/pkg/uuid"
)

type officialPromptSeed struct {
	ID          string
	Name        string
	Slug        string
	Description string
	Locale      string
	Category    string
	Tags        []string
	Content     string
}

var officialPromptSeeds = []officialPromptSeed{
	{
		ID:          "9c6ff0a8-c53f-42b7-87c2-1d2f9f7f1d08",
		Name:        "通用工作流助手模板",
		Slug:        "official/workflow-task-assistant",
		Description: "适用于任务型工作流执行节点的官方默认提示词。",
		Locale:      "zh-Hans",
		Category:    "workflow",
		Tags:        []string{"workflow", "task", "assistant"},
		Content:     "你是一名工作流任务助手。请严格根据提供的输入完成任务，输出清晰、可执行、可复用的结果；只有在确实缺少必要上下文时才提出澄清问题，不要虚构事实、结论或承诺。",
	},
	{
		ID:          "2d35f08e-5c52-43ef-bf63-bca3d5ae86ab",
		Name:        "Workflow Task Assistant",
		Slug:        "official/workflow-task-assistant",
		Description: "Official default prompt for task-oriented workflow execution nodes.",
		Locale:      "en-US",
		Category:    "workflow",
		Tags:        []string{"workflow", "task", "assistant"},
		Content:     "You are a workflow task assistant. Follow the provided input carefully, produce clear and actionable output, ask for missing context only when absolutely necessary, and avoid fabricating facts or decisions.",
	},
	{
		ID:          "9fa2ec04-0672-4e4f-9af6-ac4630f542ff",
		Name:        "企业助手回复模板",
		Slug:        "official/enterprise-assistant-answer",
		Description: "适用于内部制度问答、服务指引和企业问答的官方助手提示词。",
		Locale:      "zh-Hans",
		Category:    "assistant",
		Tags:        []string{"assistant", "policy", "qa"},
		Content:     "你是一名企业助手。请清晰回答问题，在必要时追问缺失上下文，并保持回复可执行、简洁、符合业务表达习惯。严格遵守政策边界。用户问题：{{#sys.query#}}",
	},
	{
		ID:          "3db60d44-f6a7-4892-8e65-9b7f95f69ab1",
		Name:        "Enterprise Assistant Answer",
		Slug:        "official/enterprise-assistant-answer",
		Description: "Official assistant prompt for internal policy answers, service guidance, and enterprise Q&A.",
		Locale:      "en-US",
		Category:    "assistant",
		Tags:        []string{"assistant", "policy", "qa"},
		Content:     "You are an enterprise assistant. Answer clearly, ask for missing context when needed, and keep the response actionable. Prefer concise business language and preserve policy boundaries. User question: {{#sys.query#}}",
	},
	{
		ID:          "ca975ea8-8105-40d0-a9fd-b5865280f906",
		Name:        "服务请求分诊分类器",
		Slug:        "official/service-request-triage-classifier",
		Description: "按紧急程度和处理路径路由服务请求的官方分类提示词。",
		Locale:      "zh-Hans",
		Category:    "support",
		Tags:        []string{"triage", "service", "routing"},
		Content:     "你是一名服务请求分诊分类器。请阅读用户请求，判断紧急程度、问题类型和建议处理路径。输出应简洁、结构化，并说明分类理由；不要承诺无法确认的处理结果。",
	},
	{
		ID:          "c2b3cc59-36f7-4401-9e0f-c775c478a0f3",
		Name:        "Service Request Triage Classifier",
		Slug:        "official/service-request-triage-classifier",
		Description: "Official classifier prompt for routing service requests by urgency and handling path.",
		Locale:      "en-US",
		Category:    "support",
		Tags:        []string{"triage", "service", "routing"},
		Content:     "You are a service request triage classifier. Read the user request, determine urgency, issue type, and recommended handling path. Keep the output concise and structured, explain the classification rationale, and avoid promising outcomes you cannot verify.",
	},
	{
		ID:          "09f473c0-39a0-49bb-bf77-5d7b2c0baee2",
		Name:        "客服回复模板",
		Slug:        "official/customer-support-reply",
		Description: "适用于常见服务对话的友好、合规客服回复模板。",
		Locale:      "zh-Hans",
		Category:    "support",
		Tags:        []string{"support", "reply", "tone"},
		Content:     "你是一名客服回复助手。请根据用户问题生成友好、清晰、符合政策边界的回复。先回应核心诉求，再给出可执行步骤；如信息不足，请提出必要的澄清问题。",
	},
	{
		ID:          "8e0ca70d-42b9-4727-8ab3-985886fc2a31",
		Name:        "Customer Support Reply",
		Slug:        "official/customer-support-reply",
		Description: "Friendly, policy-aware customer support reply template for common service conversations.",
		Locale:      "en-US",
		Category:    "support",
		Tags:        []string{"support", "reply", "tone"},
		Content:     "You are a customer support reply assistant. Generate a friendly, clear, policy-aware response. Address the core request first, then provide actionable next steps. If information is missing, ask the necessary clarifying questions.",
	},
	{
		ID:          "3756bb26-239b-4ad0-a056-bd4147cb6187",
		Name:        "会议纪要行动项模板",
		Slug:        "official/meeting-summary-action-items",
		Description: "把长篇会议记录整理成简明摘要、决策、风险和下一步行动。",
		Locale:      "zh-Hans",
		Category:    "meeting",
		Tags:        []string{"meeting", "summary", "actions"},
		Content:     "请将会议记录整理为简明摘要，提取关键决策、风险、待办事项、负责人和截止时间。若负责人或时间不明确，请标记为未明确，不要自行编造。",
	},
	{
		ID:          "14f4b8e4-92d3-494f-b61c-36419fc51f8a",
		Name:        "Meeting Summary Action Items",
		Slug:        "official/meeting-summary-action-items",
		Description: "Turn long meeting notes into concise summary, decisions, risks, and next actions.",
		Locale:      "en-US",
		Category:    "meeting",
		Tags:        []string{"meeting", "summary", "actions"},
		Content:     "Turn the meeting notes into a concise summary. Extract key decisions, risks, action items, owners, and due dates. If an owner or date is unclear, mark it as unspecified instead of inventing details.",
	},
}

// SeedOfficialPrompts ensures built-in official prompt templates exist.
func SeedOfficialPrompts(ctx context.Context, db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("seed database is not initialized")
	}

	logger.Info("Seeding official prompts...")

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin official prompt seed transaction: %w", err)
	}
	defer tx.Rollback()

	for _, prompt := range officialPromptSeeds {
		if err := seedOfficialPrompt(ctx, tx, prompt); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit official prompt seeds: %w", err)
	}

	logger.Info("Official prompts seeding completed", "total", len(officialPromptSeeds))
	return nil
}

func seedOfficialPrompt(ctx context.Context, tx *sql.Tx, prompt officialPromptSeed) error {
	tags, err := json.Marshal(prompt.Tags)
	if err != nil {
		return fmt.Errorf("marshal official prompt tags %s: %w", prompt.ID, err)
	}

	content, err := json.Marshal(prompt.Content)
	if err != nil {
		return fmt.Errorf("marshal official prompt content %s: %w", prompt.ID, err)
	}

	versionID := pkguuid.GenerateBuiltInWorkflowUUID("official_prompt_version_" + prompt.ID).String()
	labels := `["production","latest"]`

	_, err = tx.ExecContext(
		ctx,
		`
			INSERT INTO app_prompts (
				id, source, name, slug, description, locale, category, tags,
				latest_version, created_at, updated_at
			) VALUES (
				$1, 'official', $2, $3, $4, $5, $6, $7::jsonb,
				1, NOW(), NOW()
			)
			ON CONFLICT (id) DO UPDATE SET
				source = 'official',
				name = EXCLUDED.name,
				slug = EXCLUDED.slug,
				description = EXCLUDED.description,
				locale = EXCLUDED.locale,
				category = EXCLUDED.category,
				tags = EXCLUDED.tags,
				latest_version = 1,
				updated_at = NOW()
		`,
		prompt.ID,
		prompt.Name,
		prompt.Slug,
		prompt.Description,
		prompt.Locale,
		prompt.Category,
		string(tags),
	)
	if err != nil {
		return fmt.Errorf("upsert official prompt %s: %w", prompt.ID, err)
	}

	_, err = tx.ExecContext(
		ctx,
		`
			INSERT INTO app_prompt_versions (
				id, prompt_id, version, prompt_type, content, config, labels,
				commit_message, created_at, updated_at
			) VALUES (
				$1, $2, 1, 'text', $3::jsonb, '{}'::jsonb, $4::jsonb,
				'Initial official prompt template', NOW(), NOW()
			)
			ON CONFLICT (prompt_id, version) DO UPDATE SET
				prompt_type = 'text',
				content = EXCLUDED.content,
				config = EXCLUDED.config,
				labels = EXCLUDED.labels,
				commit_message = EXCLUDED.commit_message,
				updated_at = NOW()
		`,
		versionID,
		prompt.ID,
		string(content),
		labels,
	)
	if err != nil {
		return fmt.Errorf("upsert official prompt version %s: %w", prompt.ID, err)
	}

	return nil
}
