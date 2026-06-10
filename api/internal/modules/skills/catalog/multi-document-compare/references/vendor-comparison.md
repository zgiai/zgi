# Vendor Comparison

## Use When

Use for comparing supplier proposals, vendor solutions, bids, tenders, quotations, RFP responses, implementation plans, service commitments, and multi-provider方案.

## Language Rules

- Match the user's requested language.
- For Chinese requests, use Chinese headings and table fields only. Do not use `Vendor`, `Price`, `Delivery`, `Function`, `Risk`, or `Recommendation`.
- For English requests, use English headings and table fields only.

## Output Shape

For Chinese output:

```md
## 横向对比摘要

...

## 对比表格

| 维度 | 供应商A | 供应商B | 供应商C | 差异说明 | 原文依据 |
| --- | --- | --- | --- | --- | --- |
| 价格 | ... | ... | ... | ... | ... |
| 交付 | ... | ... | ... | ... | ... |
| 职责 | ... | ... | ... | ... | ... |
| 时间 | ... | ... | ... | ... | ... |
| 功能 | ... | ... | ... | ... | ... |
| 风险 | ... | ... | ... | ... | ... |

## 优势与不足

| 供应商 | 优势 | 不足 | 风险 |
| --- | --- | --- | --- |
| ... | ... | ... | ... |

## 待确认问题

- ...
```

## Data Rules

- Compare dimensions requested by the user first.
- Common dimensions include price, delivery timeline, scope, responsibilities, service support, technical capability, implementation risk, acceptance criteria, and exclusions.
- Do not rank suppliers unless the user asks and the source content provides enough comparable evidence.
- Do not infer missing prices, dates, or commitments. Use `未提及` or `无法判断`.
- Highlight non-comparable areas when vendors use different scope, assumptions, pricing basis, or service boundaries.

## Clarification Rules

Ask when vendor names are unclear, when the user wants a recommendation but no decision criteria are provided, or when documents use incompatible scopes that make direct comparison unreliable.
