package sensitiveredaction

import "regexp"

type redactionRule struct {
	EntityType string
	Risk       string
	Pattern    *regexp.Regexp
	ValueGroup int
}

var redactionRules = []redactionRule{
	{
		EntityType: "private_key",
		Risk:       "critical",
		Pattern:    regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
	},
	{
		EntityType: "token",
		Risk:       "critical",
		Pattern:    regexp.MustCompile(`(?i)\bBearer\s+([A-Za-z0-9._~+/=-]{12,})`),
		ValueGroup: 1,
	},
	{
		EntityType: "password",
		Risk:       "critical",
		Pattern:    regexp.MustCompile(`(?i)\b(password|passwd|pwd)\b\s*[:=]\s*["']?([^"',\s;]{4,})["']?`),
		ValueGroup: 2,
	},
	{
		EntityType: "secret",
		Risk:       "critical",
		Pattern:    regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|refresh[_-]?token|token|secret|client[_-]?secret)\b\s*[:=]\s*["']?([A-Za-z0-9._~+/=-]{8,})["']?`),
		ValueGroup: 2,
	},
	{
		EntityType: "email",
		Risk:       "high",
		Pattern:    regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`),
	},
	{
		EntityType: "id_card",
		Risk:       "high",
		Pattern:    regexp.MustCompile(`\b[1-9]\d{5}(?:18|19|20)\d{2}(?:0[1-9]|1[0-2])(?:0[1-9]|[12]\d|3[01])\d{3}[\dXx]\b`),
	},
	{
		EntityType: "phone",
		Risk:       "high",
		Pattern:    regexp.MustCompile(`(?:^|[^\dA-Za-z])((?:\+?86[\s-]*)?1[3-9](?:[\s-]?\d){9})(?:$|[^\dA-Za-z])`),
		ValueGroup: 1,
	},
	{
		EntityType: "bank_card",
		Risk:       "high",
		Pattern:    regexp.MustCompile(`\b(?:\d[ -]?){13,19}\b`),
	},
	{
		EntityType: "ip",
		Risk:       "medium",
		Pattern:    regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4]\d|1?\d?\d)\.){3}(?:25[0-5]|2[0-4]\d|1?\d?\d)\b`),
	},
	{
		EntityType: "contract_id",
		Risk:       "medium",
		Pattern:    regexp.MustCompile(`(?i)\b(?:contract|合同|合同号|协议号)\s*[:：#-]?\s*([A-Za-z0-9][A-Za-z0-9_-]{3,})`),
		ValueGroup: 1,
	},
	{
		EntityType: "order_id",
		Risk:       "medium",
		Pattern:    regexp.MustCompile(`(?i)\b(?:order|order_id|order no\.?|订单|订单号|工单|工单号|ticket)\s*[:：#-]?\s*([A-Za-z0-9][A-Za-z0-9_-]{3,})`),
		ValueGroup: 1,
	},
	{
		EntityType: "customer_name",
		Risk:       "medium",
		Pattern:    regexp.MustCompile(`(?:客户|联系人|候选人)\s*[:：]\s*([\p{Han}]{2,4}|[A-Za-z][A-Za-z .]{1,30})`),
		ValueGroup: 1,
	},
	{
		EntityType: "name",
		Risk:       "medium",
		Pattern:    regexp.MustCompile(`(?:姓名|名字|Name)\s*[:：]\s*([\p{Han}]{2,4}|[A-Za-z][A-Za-z .]{1,30})`),
		ValueGroup: 1,
	},
	{
		EntityType: "company",
		Risk:       "medium",
		Pattern:    regexp.MustCompile(`[\p{Han}A-Za-z0-9（）()·\s]{2,40}(?:有限公司|股份有限公司|集团|公司|科技有限公司|Inc\.|LLC|Ltd\.)`),
	},
	{
		EntityType: "address",
		Risk:       "medium",
		Pattern:    regexp.MustCompile(`[\p{Han}]{2,}(?:省|市|区|县|镇|街道|路|街|号楼|单元|室)[\p{Han}A-Za-z0-9（）()#\-]{2,60}`),
	},
}

var urlPattern = regexp.MustCompile(`https?://[^\s<>"']+`)
