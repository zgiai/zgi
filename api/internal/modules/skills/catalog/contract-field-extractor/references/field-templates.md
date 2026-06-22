# Field Templates

Use this reference only when the user explicitly chooses a built-in field template or asks for a common/default contract field list.

Do not invent additional fields beyond the chosen template unless the user asks to customize the field list.

## basic contract info

```json
{
  "fields": [
    {
      "key": "contract_name",
      "label": "合同名称",
      "type": "string",
      "required": true,
      "description": "合同标题或正文中明确出现的合同名称",
      "extraction_rule": "优先从首页标题、合同抬头、文件标题中提取",
      "default_value": "",
      "empty_value": "未提取到"
    },
    {
      "key": "contract_number",
      "label": "合同编号",
      "type": "string",
      "required": false,
      "description": "合同编号、协议编号或订单编号",
      "extraction_rule": "提取合同编号、协议编号、订单编号等标签后的明确值",
      "default_value": "",
      "empty_value": "未提取到"
    },
    {
      "key": "signing_date",
      "label": "签署日期",
      "type": "date",
      "required": false,
      "description": "合同签署或落款日期",
      "extraction_rule": "优先从签署页、落款、签订日期中提取",
      "default_value": "",
      "empty_value": "未提取到"
    },
    {
      "key": "effective_date",
      "label": "生效日期",
      "type": "date",
      "required": false,
      "description": "合同明确约定的生效日期",
      "extraction_rule": "提取生效日期、生效条件或有效期起始日期",
      "default_value": "",
      "empty_value": "未提取到"
    }
  ]
}
```

## parties and contacts

```json
{
  "fields": [
    {
      "key": "party_a",
      "label": "甲方",
      "type": "party",
      "required": true,
      "description": "合同甲方的完整名称和识别信息",
      "extraction_rule": "提取甲方、委托方、买方等对应主体，保留统一社会信用代码或地址等信息",
      "default_value": "",
      "empty_value": "未提取到"
    },
    {
      "key": "party_b",
      "label": "乙方",
      "type": "party",
      "required": true,
      "description": "合同乙方的完整名称和识别信息",
      "extraction_rule": "提取乙方、受托方、卖方等对应主体，保留统一社会信用代码或地址等信息",
      "default_value": "",
      "empty_value": "未提取到"
    },
    {
      "key": "contact_person",
      "label": "联系人",
      "type": "array",
      "required": false,
      "description": "合同中明确列出的联系人",
      "extraction_rule": "提取联系人姓名、电话、邮箱等明确联系信息",
      "default_value": "",
      "empty_value": "未提取到"
    }
  ]
}
```

## amount and payment

```json
{
  "fields": [
    {
      "key": "contract_amount",
      "label": "合同金额",
      "type": "money",
      "required": true,
      "description": "合同总金额，保留币种和大小写金额",
      "extraction_rule": "优先提取含总价、合同金额、价款、费用总额的条款",
      "default_value": "",
      "empty_value": "未提取到"
    },
    {
      "key": "currency",
      "label": "币种",
      "type": "string",
      "required": false,
      "description": "合同金额对应的币种",
      "extraction_rule": "从金额上下文提取人民币、CNY、美元、USD等币种",
      "default_value": "",
      "empty_value": "未提取到"
    },
    {
      "key": "payment_terms",
      "label": "付款条款",
      "type": "clause",
      "required": false,
      "description": "付款节点、比例、条件和账户要求",
      "extraction_rule": "提取付款、结算、开票、账户相关条款",
      "default_value": "",
      "empty_value": "未提取到"
    }
  ]
}
```

## term and performance

```json
{
  "fields": [
    {
      "key": "contract_term",
      "label": "合同期限",
      "type": "date_range",
      "required": false,
      "description": "合同有效期或履行期限",
      "extraction_rule": "提取合同期限、服务期限、履行期限、有效期起止时间",
      "default_value": "",
      "empty_value": "未提取到"
    },
    {
      "key": "delivery_or_performance",
      "label": "交付或履行内容",
      "type": "clause",
      "required": false,
      "description": "主要交付物、服务范围或履行义务",
      "extraction_rule": "提取交付、服务内容、工作范围、履行义务相关条款",
      "default_value": "",
      "empty_value": "未提取到"
    },
    {
      "key": "acceptance_terms",
      "label": "验收条款",
      "type": "clause",
      "required": false,
      "description": "验收标准、验收流程和验收时限",
      "extraction_rule": "提取验收、确认、测试、交付确认相关条款",
      "default_value": "",
      "empty_value": "未提取到"
    }
  ]
}
```

## risk and clauses

```json
{
  "fields": [
    {
      "key": "breach_clause",
      "label": "违约责任",
      "type": "clause",
      "required": false,
      "description": "违约责任、违约金和赔偿规则",
      "extraction_rule": "提取违约、赔偿、违约金、损失承担相关条款",
      "default_value": "",
      "empty_value": "未提取到"
    },
    {
      "key": "termination_clause",
      "label": "解除或终止条款",
      "type": "clause",
      "required": false,
      "description": "合同解除、终止、提前终止条件",
      "extraction_rule": "提取解除、终止、提前终止、单方解除相关条款",
      "default_value": "",
      "empty_value": "未提取到"
    },
    {
      "key": "renewal_clause",
      "label": "续约条款",
      "type": "clause",
      "required": false,
      "description": "自动续约或续签条件",
      "extraction_rule": "提取续约、续签、自动延续相关条款",
      "default_value": "",
      "empty_value": "未提取到"
    },
    {
      "key": "jurisdiction",
      "label": "争议解决",
      "type": "clause",
      "required": false,
      "description": "争议解决方式、管辖法院或仲裁机构",
      "extraction_rule": "提取争议解决、管辖、仲裁、诉讼相关条款",
      "default_value": "",
      "empty_value": "未提取到"
    }
  ]
}
```
