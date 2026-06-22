# Business Identifiers Reference

## 使用场景

Use for business-sensitive identifiers, including company names, customer names, supplier names, order IDs, contract IDs, ticket IDs, project codes, case numbers, and internal business references.

Trigger examples: 公司名脱敏, 客户名脱敏, 订单号脱敏, 合同号脱敏, 工单脱敏, 业务数据脱敏, customer data redaction, contract number redaction.

## Payload 示例

```json
{
  "text": "客户：李四，合同号：HT-2026-001，订单号：ORD-88991234，供应商：星河科技有限公司",
  "level": "medium",
  "strategy": "partial",
  "preserve_rules": {
    "keep_last_digits": 4
  }
}
```

## 数据规则

- Use `high` when business records are sent outside the organization or used in model training.
- Use `medium` for internal sharing when suffix traceability is needed.
- Keep only limited suffixes for order or contract IDs.
- Company and customer names can be ambiguous; include a warning when context-dependent names may remain.
- Do not invent or normalize business identifiers.

## 质询规则

Call `request_user_input` if the user asks to preserve order, contract, or customer traceability but does not specify the retention rule.
