package digitalhuman

import "strings"

// MemoryInstructionsCatalogVersion changes whenever a template's text or
// samples change.
const MemoryInstructionsCatalogVersion = 1

// MemoryInstructionsTemplate is a starting point for a DigitalHuman's memory
// instructions (ADR 0015 §5): the instructions for one kind of work, and
// sample conversations to preview them with. Applying one copies the
// instructions into the draft.
type MemoryInstructionsTemplate struct {
	ID           string                    `json:"id"`
	Name         string                    `json:"name"`
	Description  string                    `json:"description"`
	Instructions string                    `json:"instructions"`
	Samples      []MemoryInstructionSample `json:"samples"`
}

// MemoryInstructionSample is a short conversation that shows what a template
// keeps.
type MemoryInstructionSample struct {
	Name  string              `json:"name"`
	Turns []MemoryPreviewTurn `json:"turns"`
}

// MemoryInstructionsCatalog lists the templates in one locale.
type MemoryInstructionsCatalog struct {
	Version   int                          `json:"version"`
	Templates []MemoryInstructionsTemplate `json:"templates"`
}

// localized holds one string in each supported locale.
type localized struct{ zh, en string }

func (l localized) in(chinese bool) string {
	if chinese {
		return l.zh
	}
	return l.en
}

type memoryTemplateSource struct {
	id                              string
	name, description, instructions localized
	samples                         []memoryTemplateSampleSource
}

type memoryTemplateSampleSource struct {
	name  localized
	turns [][2]localized
}

func exchange(userZH, userEN, replyZH, replyEN string) [2]localized {
	return [2]localized{{userZH, userEN}, {replyZH, replyEN}}
}

var memoryTemplateSources = []memoryTemplateSource{
	{
		id:          "support_desk",
		name:        localized{"客户服务", "Customer support"},
		description: localized{"记住客户的长期背景、反馈的问题和我们承诺的处理。", "Remember each customer's lasting background, the issues they report, and what we promised."},
		instructions: localized{
			"这是一个客服数字人。对每位客户，记下他们的公司和套餐、正在使用的产品、反馈过的问题以及我们承诺的处理和时间；问题解决后更新。对自己的工作，记下反复出现的问题和已经验证有效的解决办法。不要记录寒暄、客户的临时情绪、订单号或任何账号密码和支付信息。",
			"This is a customer support digital human. For each customer, note their company and plan, the products they use, the issues they reported, and the fix and date we promised; update it once resolved. For its own work, note recurring problems and fixes that proved effective. Do not record small talk, temporary emotions, order numbers, or any credentials and payment details.",
		},
		samples: []memoryTemplateSampleSource{
			{name: localized{"反馈问题", "Reporting an issue"}, turns: [][2]localized{
				exchange("我们是星河科技，企业版。今天天气不错。对了，你们的导出功能这周第二次失败了，周五前能修好吗？", "We're Xinghe Tech on the Enterprise plan. Nice weather today. By the way, your export feature failed for the second time this week; can it be fixed by Friday?",
					"抱歉给您带来不便，我已提交工程团队，周五前给您答复。", "Sorry for the trouble. I've escalated it to engineering and will get back to you by Friday."),
			}},
		},
	},
	{
		id:          "sales_followup",
		name:        localized{"销售跟进", "Sales follow-up"},
		description: localized{"记住客户需求、预算、决策人、竞品和约定的下一步。", "Remember needs, budget, decision makers, competitors, and agreed next steps."},
		instructions: localized{
			"这是一个销售数字人。对每位客户，记下要解决的问题、预算和时间线、谁来拍板、正在用或在比较的竞品及价格，以及双方约定的下一步和截止时间；事情完成后更新。不要记录寒暄、未经客户确认的猜测、我们内部的报价底线，或客户的手机号和邮箱。",
			"This is a sales digital human. For each customer, note the problem to solve, budget and timeline, who decides, competitors they use or compare with prices, and the next steps both sides agreed with their deadlines; update them once done. Do not record small talk, guesses the customer has not confirmed, our internal price floor, or the customer's phone number and email.",
		},
		samples: []memoryTemplateSampleSource{
			{name: localized{"首次沟通", "First call"}, turns: [][2]localized{
				exchange("我们 200 人，现在用钉钉，每人每月 30 块，想换能对接 ERP 的平台，预算一年 10 万以内，CTO 拍板。下周三前把方案发我。", "We're 200 people on DingTalk at 30 yuan per person per month, want a platform that connects to our ERP, budget under 100k a year, and our CTO decides. Send me a plan before next Wednesday.",
					"好的，下周三前发您 ERP 对接方案。", "Will do: the ERP integration plan reaches you before next Wednesday."),
			}},
		},
	},
	{
		id:          "tutoring",
		name:        localized{"教育辅导", "Tutoring"},
		description: localized{"记住学员水平、目标、薄弱点和学习进度。", "Remember each learner's level, goals, weak points, and progress."},
		instructions: localized{
			"这是一个辅导数字人。对每位学员，记下水平和学习目标、考试时间、偏好的讲解方式、已掌握的内容、反复出错的地方和正在学的单元；掌握后更新薄弱点。对自己的工作，记下哪些讲法对学员更有效。不要记录学校名称、家庭情况、排名和健康信息，也不用逐题记录答题过程。",
			"This is a tutoring digital human. For each learner, note their level and goals, exam dates, preferred way of explaining, what they have mastered, where they keep making mistakes, and the unit in progress; update weak points once mastered. For its own work, note which explanations work better. Do not record school names, family details, rankings, or health information, and do not log each exercise attempt.",
		},
		samples: []memoryTemplateSampleSource{
			{name: localized{"初次辅导", "First session"}, turns: [][2]localized{
				exchange("我初二，明年 6 月中考，最怕几何证明，讲题多画图。我们班主任姓王，人很好。", "I'm in grade 8 with my exam next June, geometry proofs scare me most, and please draw diagrams. My homeroom teacher, Ms Wang, is really nice.",
					"好的，我们重点练几何证明，讲解都配图。", "Okay, we'll focus on geometry proofs and I'll draw diagrams throughout."),
			}},
		},
	},
	{
		id:          "ecommerce_aftersales",
		name:        localized{"电商售后", "E-commerce after-sales"},
		description: localized{"记住客户的购物偏好、尺码和长期的退换货与配送偏好。", "Remember purchase preferences, sizes, and lasting return and delivery preferences."},
		instructions: localized{
			"这是一个电商售后数字人。对每位客户，记下常买的品类和品牌、尺码、对退换货和配送的长期偏好。对自己的工作，记下反复出现的商品问题。不要记录订单号、完整收货地址、电话、支付或银行卡信息，也不用记录单次售后的处理过程。",
			"This is an e-commerce after-sales digital human. For each customer, note usual categories and brands, sizes, and lasting return and delivery preferences. For its own work, note recurring product problems. Do not record order numbers, full delivery addresses, phone numbers, or payment or card details, and do not log how a single case was handled.",
		},
		samples: []memoryTemplateSampleSource{
			{name: localized{"换货", "Exchange"}, turns: [][2]localized{
				exchange("订单 88231 的跑鞋小了，我平时穿 42 码，这双要换 43。地址是杭州市西湖区某某路 18 号，以后别用某某快递。", "The running shoes in order 88231 are too small; I usually wear 42 but need a 43 here. My address is 18 Example Road, Xihu District, Hangzhou, and please stop using that courier.",
					"已为您申请换成 43 码，之后会避开那家快递。", "I've requested size 43 and we'll avoid that courier from now on."),
			}},
		},
	},
	{
		id:          "internal_helpdesk",
		name:        localized{"内部助手（HR/IT）", "Internal helpdesk (HR/IT)"},
		description: localized{"记住员工的部门、常用系统和反复遇到的问题。", "Remember each employee's department, systems, and recurring issues."},
		instructions: localized{
			"这是一个内部 HR/IT 数字人。对每位员工，记下部门、岗位、办公地点、常用的系统和设备，以及还没解决、需要跟进的问题。对自己的工作，记下反复出现的问题和已经有效的解决办法。不要记录薪资、绩效、考勤、健康或家庭等隐私，绝不记录账号密码。",
			"This is an internal HR/IT digital human. For each employee, note their department, role, office, the systems and devices they use, and unresolved issues to follow up. For its own work, note recurring problems and fixes that worked. Do not record salary, performance, attendance, health, family, or other private details, and never record credentials.",
		},
		samples: []memoryTemplateSampleSource{
			{name: localized{"IT 报修", "IT request"}, turns: [][2]localized{
				exchange("我是上海财务部的，ThinkPad 的 VPN 这周第三次连不上了。顺便说下，我下个月要休产假。", "I'm in Finance in Shanghai, and my ThinkPad's VPN failed for the third time this week. Also, I'm going on maternity leave next month.",
					"先重装最新版 VPN 客户端并清缓存试试，通常能解决反复断连。", "Try reinstalling the latest VPN client and clearing its cache; that usually fixes repeated disconnects."),
			}},
		},
	},
}

// MemoryInstructionsTemplates returns the catalog in the requested locale.
// Chinese locales get Chinese text; every other locale gets English.
func MemoryInstructionsTemplates(locale string) MemoryInstructionsCatalog {
	chinese := strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "zh")
	out := MemoryInstructionsCatalog{Version: MemoryInstructionsCatalogVersion, Templates: make([]MemoryInstructionsTemplate, 0, len(memoryTemplateSources))}
	for _, source := range memoryTemplateSources {
		template := MemoryInstructionsTemplate{
			ID: source.id, Name: source.name.in(chinese), Description: source.description.in(chinese),
			Instructions: source.instructions.in(chinese), Samples: make([]MemoryInstructionSample, 0, len(source.samples)),
		}
		for _, sample := range source.samples {
			turns := make([]MemoryPreviewTurn, 0, len(sample.turns))
			for _, turn := range sample.turns {
				turns = append(turns, MemoryPreviewTurn{User: turn[0].in(chinese), Assistant: turn[1].in(chinese)})
			}
			template.Samples = append(template.Samples, MemoryInstructionSample{Name: sample.name.in(chinese), Turns: turns})
		}
		out.Templates = append(out.Templates, template)
	}
	return out
}
