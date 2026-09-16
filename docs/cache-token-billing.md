# 缓存 Token 计费与模型价格同步

LLM 调用按四个互斥分项统计和计费：普通输入、缓存读取、缓存写入、输出。价格单位统一为 USD/百万 Token。

## Token 口径

```text
总 Token = 普通输入 Token + 缓存读取 Token + 缓存写入 Token + 输出 Token
```

- OpenAI 兼容协议的 `prompt_tokens` 通常包含缓存 Token，网关会根据 `prompt_tokens_details.cached_tokens` 和 `cache_creation_tokens` 拆分普通输入。
- Anthropic 的 `input_tokens`、`cache_read_input_tokens` 和 `cache_creation_input_tokens` 按相加语义处理。
- Gemini 的 `cachedContentTokenCount` 作为缓存读取 Token。
- 上游没有返回缓存明细时，保持原有输入/输出计费口径。

## 价格优先级

```text
组织模型覆盖价 > 同步模型基础价
```

输入、输出、缓存读取和缓存写入均支持组织覆盖价。留空表示清除覆盖并回退到同步价；明确填写 `0` 表示免费。

模型同步兼容顶层价格字段和 `pricing.token_tiers` 中的每百万 Token 字段。缓存读取价兼容 `cache_read_price_per_million` 与 `cached_input_price_per_million`；缓存写入价读取 `cache_write_price_per_million`。上游未提供缓存价格时保持“未配置”，不会静默回退到普通输入价。

价格字段使用 `numeric(24,12)`，模型同步和站点币种换算均保留最多 12 位小数。小于 `0.0001` 的非零价格或费用直接展示实际数值，不再显示为“小于 0.0001”。

## 数据库与历史记录

迁移 `20260823100000_add_llm_cache_usage_billing` 增加缓存 Token、缓存覆盖价格和价格配置状态字段，并扩展总 Token 约束。历史账单的缓存字段默认是 `0`，因此原有总 Token 关系保持成立。

迁移 `20260823110000_increase_llm_price_precision` 将同步模型价格、项目覆盖价格和自定义模型价格统一提升到 12 位小数。迁移只能保留执行时数据库中仍存在的精度；此前已被六位或四位字段舍入的价格，需要在迁移后重新同步模型或重新保存配置。

旧表无法区分“未提供缓存价”和“明确配置为 0”。迁移只把已有非零缓存价标记为已配置；升级后应执行一次模型目录同步，让上游明确提供的免费价格重新以“已配置 0”落库。

每次结算的 `pricing_snapshot` 保存四类 Token、四类实际价格、四类费用以及缓存价格来源。后续模型同步或组织配置变化不会重新解释已结算账单。

## 缓存价格回落倍率（2026-09）

未显式配置缓存价时的回落倍率由 **Console 侧**结算器
（`zgi-console/api/internal/modules/billing/cache_multiplier.go`）按 provider 决定
（未知 provider 恒为 1），并写入共享的 `pricing_snapshot`
（`cache_read_price_multiplier` / `cache_write_price_multiplier` / `provider`）。
Pre 侧仍按同步到的显式缓存价或“未配置”处理，行为不变。

## `*_price_configured` 列下线（2026-09）

`llm_models` / `llm_custom_models` 的 6 个 `*_price_configured` 布尔列由迁移
`20260916151000_drop_price_configured_booleans` 删除，「已配置」判定改由 `pricing`
派生。删列之前必须先执行 `20260916150500_backfill_price_configured_pricing`，把布尔列
表达的显式配置投影到 `pricing.rules[].configured`；详见
[multimodal-pricing-operations.md](./multimodal-pricing-operations.md)。

