# 多模态定价运维说明（本次改造）

本文记录本次「模型多模态价格改造」在 Pre 侧引入、需要部署与运维知晓的行为。契约本身
（`pricing.rules[]`、operation/meter/unit 取值）以 Console 侧
`api/docs/PRICING_RULES_V2.md` 为准。

## 1. 报价兜底模板（P-2）

- 代码里不再保存任何兜底价格。`DefaultPricingFallbackRules()` 只返回骨架
  （id / operation / meter / provider / enabled）。
- 价格来自部署配置环境变量 `PRICING_FALLBACK_DEFAULT_PRICES`，取值形如：

  ```json
  {
    "token.chat.input.default": "1",
    "token.chat.output.default": "2",
    "token.embedding.input.default": "1",
    "token.rerank.input.default": "1",
    "image.qwen.default": 160,
    "image.doubao.default": 100,
    "image.openai.default": 200,
    "image.gcp.default": 180,
    "image.midjourney.default": 300,
    "image.generic.default": 200
  }
  ```

  未配置价格（或 JSON 非法）时，对应骨架不带金额，**不会**产生
  `code_default_fallback` 报价；缺价请求按既有语义 fail-closed。
- `GET /console/api/llm/pricing/fallback/template` 返回「骨架 + 本环境配置值」。
  设置页「缺失价格计费策略」面板新增「载入模板」按钮：把模板写入当前组织的
  `override_rules`（保存后来源标记为 `admin_fallback`），模板与手工规则不可区分
  （已知限制）。
- 该面板默认折叠，展开后功能不变；`fallbackEnabled` 初始值改为跟随接口加载结果，
  不再乐观置为 `true`。

## 2. 缓存价格回落倍率（跨仓，Console 侧实现）

- 该行为由 **Console 侧**结算器实现（`zgi-console/api/internal/modules/billing/cache_multiplier.go`），
  不在 Pre：未显式配置 cache 价时回落基准 = 模型 input 价 × 厂商倍率（未知 provider 恒为 1）。
  当前已知厂商：`anthropic` / `claude`（read 0.1、write 1.25）。
- 倍率会写入共享的结算快照 `cache_read_price_multiplier` / `cache_write_price_multiplier`
  以及 `provider`，便于对账解释。
- Pre 侧仍按同步到的显式缓存价或「未配置」处理；不做 provider 倍率注册表、不做模型级倍率覆盖
  （按需求 §0.1 C2）。

## 3. `*_price_configured` 布尔列下线（C′/C3）

- 迁移顺序（ID 升序执行）：
  1. `20260916150000` 给 `llm_custom_models` 加 `pricing` 列；
  2. `20260916150500` **回填**：把 6 个布尔列表达的「显式配置」投影为
     `pricing.rules[].configured=true` 并派生 `token_tiers`；幂等，重跑跳过；
  3. `20260916151000` 删除 `llm_models` / `llm_custom_models` 的 6 个布尔列。
- 「已配置」判定改为只读 `pricing`（`ResolveConfigured`），并叠加组织 override。
  因此 **不要**跳过 150500 直接执行删列，否则存量自定义模型会被判定为未配置。
- 回滚顺序相反：删列的 down 会把布尔列加回（值为 false，原值不可恢复），
  再回滚回填迁移不会销毁已生成的 pricing。

## 4. 视频预扣与结算的 lane 语义（P-1 / P-4）

- private lane：预扣与结算都读本地模型价；缺价返回 `ErrPricingNotConfigured`。
- platform lane：本地报价恒为 0，预扣仍创建 billing attempt 并携带
  `X-ZGI-Billing-*` / `X-ZGI-Request-ID` / `X-ZGI-Attempt-ID`；结算金额以 Console
  权威结算（`resp.Settlement` 或按 attempt 查询）为准，**不会**因为本地报价为 0 而失败。
- 成功结算不再出现「缺价静默 0 扣费」；失败退款不因缺价被阻断（仍会回滚）。

## 5. 统计维度（X-4）

- `llm_usage_bills.operation` 为 `varchar(64) NOT NULL DEFAULT ''`，索引
  `(organization_id, operation, request_created_at DESC)`。
- 默认 operation 为 `chat`；embedding/rerank/image/video/music/audio 在其路径上显式覆盖。
- 模型用量统计按 operation 分组返回；调用日志支持 `attempt_id` 过滤，
  输出中的 `settlement_id` 在无远端扣费 ID 时即 attempt_id。
