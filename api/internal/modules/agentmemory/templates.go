package agentmemory

import "strings"

// TemplateCatalogVersion changes whenever a template's instructions change,
// so the editor can tell which catalog a draft came from.
const TemplateCatalogVersion = 4

// Template is a starting point for an Agent's memory: memory instructions.
// The model organizes the memory files itself (ADR 0017). Applying a
// template copies its instructions into the Agent's draft.
type Template struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Instructions string `json:"instructions"`
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

type templateSource struct {
	id           string
	name         text
	description  text
	instructions text
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
	},
	{
		id:          "project_collaboration",
		name:        text{"项目协作", "Project collaboration"},
		description: text{"适合项目协作，记录项目背景、长期流程和交付偏好，减少跨轮重复说明。", "For project collaboration: remember project context, long-term workflow, and delivery preferences."},
		instructions: text{
			"这是一个项目协作助手。重点记住用户长期推进的项目：目标、关键约束、里程碑、决策和团队的协作流程，以及用户对交付物的固定要求。单次任务的中间结果、草稿内容和临时安排不用记。",
			"This is a project collaboration assistant. Focus on the projects the user drives over time: goals, key constraints, milestones, decisions, the team's workflow, and the user's standing requirements for deliverables. Skip intermediate results of a single task, draft contents, and temporary plans.",
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
	},
	{
		id:          "sales_followup",
		name:        text{"销售跟进", "Sales follow-up"},
		description: text{"适合销售和商机跟进，记录客户需求、预算、决策人、竞品和约定的下一步。", "For sales: remember the customer's needs, budget, decision makers, competitors, and agreed next steps."},
		instructions: text{
			"这是一个销售助手。记住推进这笔生意需要的信息：客户是谁、要解决什么问题、预算和时间线、谁来拍板、正在比较哪些竞品，以及双方约定的下一步。不要记录寒暄、未确认的猜测、我们内部的报价底线，或客户的手机号和邮箱。",
			"This is a sales assistant. Remember what moves the deal forward: who the customer is, the problem they need solved, budget and timeline, who decides, which competitors they are comparing, and the next steps both sides agreed. Do not record small talk, unconfirmed guesses, our internal price floor, or the customer's phone number and email.",
		},
	},
	{
		id:          "tutoring",
		name:        text{"教育辅导", "Tutoring"},
		description: text{"适合学习辅导，记录学员水平、学习目标、薄弱点和学习进度。", "For tutoring: remember the learner's level, goals, weak points, and progress."},
		instructions: text{
			"这是一个学习辅导助手。记住能让下一次辅导接着上一次往下走的信息：学员的水平和目标、考试时间、偏好的讲解方式、已经掌握的内容、反复出错的地方和正在学的单元。学员掌握某个薄弱点后要更新。不要记录学校名称、家庭情况、排名和健康信息，也不用逐题记录答题过程。",
			"This is a tutoring assistant. Remember what lets the next session pick up where this one ended: the learner's level and goals, exam dates, preferred way of explaining, what they have mastered, where they keep making mistakes, and the unit in progress. Update a weak point once it is mastered. Do not record school names, family details, rankings, or health information, and do not log each exercise attempt.",
		},
	},
	{
		id:          "ecommerce_aftersales",
		name:        text{"电商售后", "E-commerce after-sales"},
		description: text{"适合电商售后，记录客户的购物偏好、尺码和长期的退换货与配送偏好。", "For e-commerce after-sales: remember purchase preferences, sizes, and lasting return and delivery preferences."},
		instructions: text{
			"这是一个电商售后助手。记住能让下次服务更顺的长期信息：客户常买的品类和品牌、尺码、对退换货和配送的偏好，以及与这位客户相关的长期售后规则。不要记录订单号、完整收货地址、电话、支付或银行卡信息，也不用记录单次售后的处理过程。",
			"This is an e-commerce after-sales assistant. Remember lasting details that make the next conversation smoother: the categories and brands the customer usually buys, sizes, return and delivery preferences, and after-sales rules that apply to this customer. Do not record order numbers, full delivery addresses, phone numbers, or payment or card details, and do not log how a single case was handled.",
		},
	},
	{
		id:          "internal_helpdesk",
		name:        text{"内部助手（HR/IT）", "Internal helpdesk (HR/IT)"},
		description: text{"适合内部 HR/IT 助手，记录员工的部门、常用系统和反复遇到的问题。", "For internal HR/IT helpdesks: remember the employee's department, the systems they use, and recurring issues."},
		instructions: text{
			"这是一个内部 HR/IT 助手。记住能让下次更快解决问题的信息：员工的部门、岗位和办公地点，常用的系统和设备，反复遇到的问题和已经有效的解决办法，以及还没解决、需要跟进的事。不要记录薪资、绩效、考勤、健康或家庭等隐私，也绝不记录账号密码。",
			"This is an internal HR/IT helpdesk. Remember what helps solve the next request faster: the employee's department, role, and office, the systems and devices they use, recurring problems with fixes that worked, and unresolved issues that need follow-up. Do not record salary, performance, attendance, health, family, or other private details, and never record credentials.",
		},
	},
}

// Templates returns the catalog in the requested locale. Chinese locales
// get Chinese text; every other locale gets English.
func Templates(locale string) TemplateCatalog {
	chinese := strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "zh")
	out := TemplateCatalog{Version: TemplateCatalogVersion, Templates: make([]Template, 0, len(templateSources))}
	for _, source := range templateSources {
		out.Templates = append(out.Templates, Template{
			ID: source.id, Name: source.name.in(chinese), Description: source.description.in(chinese),
			Instructions: source.instructions.in(chinese),
		})
	}
	return out
}
