package agentmemory

import "strings"

// TemplateCatalogVersion changes whenever a template's blocks, instructions,
// or samples change, so the editor can tell which catalog a draft came from.
const TemplateCatalogVersion = 1

// Template is a starting point for an Agent's memory (ADR 0013): a set of
// blocks, memory instructions, and sample conversations to preview them with.
// Applying a template copies it into the Agent's draft.
type Template struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Description  string           `json:"description"`
	Instructions string           `json:"instructions"`
	Blocks       []TemplateBlock  `json:"blocks"`
	Samples      []TemplateSample `json:"samples"`
}

type TemplateBlock struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	MaxChars    int    `json:"max_chars"`
	ReadOnly    bool   `json:"read_only"`
	AutoWrite   bool   `json:"auto_write"`
}

// TemplateSample is a short conversation that shows what a template keeps.
type TemplateSample struct {
	Name  string               `json:"name"`
	Turns []TemplateSampleTurn `json:"turns"`
}

type TemplateSampleTurn struct {
	User      string `json:"user"`
	Assistant string `json:"assistant,omitempty"`
}

type TemplateCatalog struct {
	Version   int        `json:"version"`
	Templates []Template `json:"templates"`
}

// text holds one string in each supported locale.
type text struct{ zh, en string }

func (t text) in(chinese bool) string {
	if chinese {
		return t.zh
	}
	return t.en
}

type templateBlockSource struct {
	key         string
	name        text
	description text
	maxChars    int
}

type templateSource struct {
	id           string
	name         text
	description  text
	instructions text
	blocks       []string
	samples      []templateSampleSource
}

type templateSampleSource struct {
	name  text
	turns []struct{ user, assistant text }
}

func turn(userZH, userEN, assistantZH, assistantEN string) struct{ user, assistant text } {
	return struct{ user, assistant text }{text{userZH, userEN}, text{assistantZH, assistantEN}}
}

var templateBlocks = map[string]templateBlockSource{
	"profile": {key: "profile", name: text{"用户资料", "User profile"}, maxChars: 2000, description: text{
		"保存用户本人长期稳定的姓名、用户希望被如何称呼、职业角色、团队角色等。不要保存智能体名称、人设、角色扮演风格、用户对智能体的称呼、联系方式、账号、证件号、密码或临时状态。",
		"Store stable facts about the user only: user name, how the user wants to be addressed, job role, team role. Do not store agent name, assistant persona, roleplay style, what the user calls the agent, contact details, accounts, IDs, passwords, or temporary states.",
	}},
	"preferences": {key: "preferences", name: text{"回答偏好", "Answer preferences"}, maxChars: 2000, description: text{
		"保存用户长期偏好的回答语言、示例技术栈、长短、排版、解释深度、语气风格等。不要保存一次性格式要求、临时心情、当前任务内容或需要智能体长期扮演的身份。",
		"Store durable answer preferences: language, example stack, length, layout, explanation depth, tone, and style. Do not store one-off formatting requests, temporary moods, current task content, or a durable identity the agent should play.",
	}},
	"standing_instructions": {key: "standing_instructions", name: text{"长期协作规则", "Standing instructions"}, maxChars: 2000, description: text{
		"保存用户希望长期遵守的互动规则、协作流程、任务处理规则、称呼用户的方式、用户给智能体的称呼或智能体人设，例如先给结论、先列风险、以某种助手身份互动等。不要保存具体项目事实或一次性任务步骤。",
		"Store long-term interaction rules, collaboration workflows, task procedures, how to address the user, what the user calls the agent, or agent persona instructions, such as conclusion first, risk review, or roleplay as a specific assistant. Do not store project facts or one-off task steps.",
	}},
	"project_context": {key: "project_context", name: text{"项目背景", "Project context"}, maxChars: 2000, description: text{
		"保存用户持续推进的项目、目标、关键背景、工作流和长期约束。仅在用户明确描述项目或目标时更新；不要从职业身份变化推断项目变化。",
		"Store ongoing project, goals, key background, workflow, and durable constraints. Update only when the user describes project or goal changes; do not infer from role changes.",
	}},
	"delivery_preferences": {key: "delivery_preferences", name: text{"交付偏好", "Delivery preferences"}, maxChars: 2000, description: text{
		"保存用户对交付物的长期偏好，例如输出结构、评审深度、文件格式、验收口径等。不要保存单次任务的临时格式要求。",
		"Store durable delivery preferences: output structure, review depth, file format, acceptance criteria. Do not store one-off formatting requests for a single task.",
	}},
	"customer_profile": {key: "customer_profile", name: text{"客户资料", "Customer profile"}, maxChars: 2000, description: text{
		"保存客户长期稳定的身份、账号背景、产品使用情况、合同/关系状态等。不要保存密码、支付信息、证件号或一次性工单细节。",
		"Store stable customer identity, account background, product usage, contract or relationship status. Never store passwords, payment data, IDs, or one-off ticket details.",
	}},
	"communication_preferences": {key: "communication_preferences", name: text{"沟通偏好", "Communication preferences"}, maxChars: 2000, description: text{
		"保存客户长期偏好的语气、沟通渠道、升级措辞、响应节奏等。不要保存临时情绪、一次性投诉内容或单次服务处理细节。",
		"Store durable customer communication preferences: tone, channel, escalation wording, response cadence. Do not store temporary emotions or one-off service handling.",
	}},
	"business_context": {key: "business_context", name: text{"业务背景", "Business context"}, maxChars: 2000, description: text{
		"保存长期业务流程背景、支持范围、政策规则和运营约束。仅保存与该客户/用户长期相关的规则；不要保存通用产品知识或临时处理结论。",
		"Store long-term business process background, support scope, policy rules, and operational constraints tied to this customer/user. Do not store general product knowledge or temporary conclusions.",
	}},
}

var templateSources = []templateSource{
	{
		id:          "general_assistant",
		name:        text{"通用助手", "General assistant"},
		description: text{"适合通用助手，记录用户身份、回答偏好、长期协作规则和持续项目背景。", "For general assistants: remember profile, answer preferences, standing rules, and ongoing project context."},
		instructions: text{
			"这是一个通用助手。记住能让以后的回答更贴合这个用户的长期信息：他是谁、喜欢怎样的回答、希望你一直遵守的规则、他持续在做的事。闲聊、一次性的问题和当天的安排不用记。",
			"This is a general assistant. Remember what makes later answers fit this user: who they are, how they like answers, rules they want you to keep, and what they are working on over time. Skip small talk, one-off questions, and today's plans.",
		},
		blocks: []string{"profile", "preferences", "standing_instructions", "project_context"},
		samples: []templateSampleSource{
			{name: text{"自我介绍与偏好", "Introduction and preferences"}, turns: []struct{ user, assistant text }{
				turn("我叫林夏，在一家电商公司做数据分析。以后回答尽量简短，先给结论。", "I'm Lin Xia, a data analyst at an e-commerce company. Keep answers short from now on, conclusion first.",
					"好的，林夏。之后我会先给结论，再简要说明。", "Got it, Lin Xia. I'll lead with the conclusion and keep the rest brief."),
				turn("今天有点累，随便聊聊吧。", "I'm a bit tired today, let's just chat.",
					"没问题，想聊点什么？", "Sure, what's on your mind?"),
			}},
			{name: text{"纠正与遗忘", "Correction and forgetting"}, turns: []struct{ user, assistant text }{
				turn("我上个月转岗了，现在做产品经理。", "I changed roles last month; I'm a product manager now.",
					"了解，之后按产品经理的视角来回答。", "Noted, I'll answer from a product manager's point of view."),
				turn("之前说的先给结论那条不用了，正常回答就行。", "Drop the conclusion-first rule I gave you; answer normally.",
					"好的，那条规则已不再适用。", "Okay, I'll stop applying that rule."),
			}},
		},
	},
	{
		id:          "project_collaboration",
		name:        text{"项目协作", "Project collaboration"},
		description: text{"适合项目协作，记录项目背景、长期流程和交付偏好，减少跨轮重复说明。", "For project collaboration: remember project context, long-term workflow, and delivery preferences."},
		instructions: text{
			"这是一个项目协作助手。重点记住用户长期推进的项目：目标、关键约束、里程碑、决策和团队的协作流程，以及用户对交付物的固定要求。单次任务的中间结果、草稿内容和临时安排不用记。",
			"This is a project collaboration assistant. Focus on the projects the user drives over time: goals, key constraints, milestones, decisions, the team's workflow, and the user's standing requirements for deliverables. Skip intermediate results of a single task, draft contents, and temporary plans.",
		},
		blocks: []string{"profile", "project_context", "standing_instructions", "delivery_preferences"},
		samples: []templateSampleSource{
			{name: text{"项目启动", "Project kickoff"}, turns: []struct{ user, assistant text }{
				turn("我们在做会员积分系统重构，目标是 12 月底上线，数据库只能用 PostgreSQL。", "We're rebuilding the loyalty points system, targeting launch at the end of December, and we can only use PostgreSQL.",
					"明白。我会按 12 月底上线和 PostgreSQL 的约束来给建议。", "Understood. I'll plan around the end-of-December launch and the PostgreSQL constraint."),
				turn("以后给我的方案都用表格列出风险和负责人。", "From now on, list risks and owners in a table in every plan you give me.",
					"好的，之后的方案都会附风险和负责人表格。", "Will do: every plan will include a table of risks and owners."),
			}},
		},
	},
	{
		id:          "support_operations",
		name:        text{"客服运营", "Support operations"},
		description: text{"适合客服/运营场景，记录客户背景、沟通偏好、业务上下文和长期处理规则。", "For support and operations: remember customer background, communication preferences, business context, and standing rules."},
		instructions: text{
			"这是一个客服助手。记住客户的长期背景（公司、套餐、使用的产品）、沟通偏好和与该客户相关的长期业务规则，方便下次接待时不用重复询问。不要记录单次工单的处理过程、客户的临时情绪、订单号或任何账号密码和支付信息。",
			"This is a support assistant. Remember the customer's lasting background (company, plan, products they use), communication preferences, and business rules that apply to this customer, so the next conversation does not ask again. Do not record how a single ticket was handled, temporary emotions, order numbers, or any credentials and payment details.",
		},
		blocks: []string{"customer_profile", "communication_preferences", "business_context", "standing_instructions"},
		samples: []templateSampleSource{
			{name: text{"首次接待", "First contact"}, turns: []struct{ user, assistant text }{
				turn("你好，我是星河科技的运维负责人，我们用的是企业版，主要用你们的日志服务。", "Hi, I lead operations at Xinghe Tech. We're on the Enterprise plan and mainly use your logging service.",
					"您好！已了解您是星河科技企业版客户，主要使用日志服务。请问有什么可以帮您？", "Hello! Noted that you're on the Enterprise plan at Xinghe Tech, mainly using logging. How can I help?"),
				turn("订单 20261025-883 的发票还没收到，挺着急的。以后有事请发邮件通知我，别打电话。", "I still haven't received the invoice for order 20261025-883 and it's urgent. In future, please notify me by email, not by phone.",
					"抱歉让您久等，我马上为您核查发票。之后会通过邮件联系您。", "Sorry for the wait; I'll check the invoice right away. We'll contact you by email from now on."),
			}},
		},
	},
}

// Templates returns the catalog in the requested locale. Chinese locales
// get Chinese text; every other locale gets English.
func Templates(locale string) TemplateCatalog {
	chinese := strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "zh")
	out := TemplateCatalog{Version: TemplateCatalogVersion, Templates: make([]Template, 0, len(templateSources))}
	for _, source := range templateSources {
		template := Template{
			ID: source.id, Name: source.name.in(chinese), Description: source.description.in(chinese),
			Instructions: source.instructions.in(chinese),
			Blocks:       make([]TemplateBlock, 0, len(source.blocks)),
			Samples:      make([]TemplateSample, 0, len(source.samples)),
		}
		for _, key := range source.blocks {
			block := templateBlocks[key]
			template.Blocks = append(template.Blocks, TemplateBlock{
				Key: block.key, Name: block.name.in(chinese), Description: block.description.in(chinese),
				MaxChars: block.maxChars, AutoWrite: true,
			})
		}
		for _, sample := range source.samples {
			turns := make([]TemplateSampleTurn, 0, len(sample.turns))
			for _, t := range sample.turns {
				turns = append(turns, TemplateSampleTurn{User: t.user.in(chinese), Assistant: t.assistant.in(chinese)})
			}
			template.Samples = append(template.Samples, TemplateSample{Name: sample.name.in(chinese), Turns: turns})
		}
		out.Templates = append(out.Templates, template)
	}
	return out
}
