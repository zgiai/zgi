# 多模态定价运维说明（本次改造）

本文记录本次「模型多模态价格改造」在 Pre 侧引入、需要部署与运维知晓的行为。契约本身
（`pricing.rules[]`、operation/meter/unit 取值）以 Console 侧
`api/docs/PRICING_RULES_V2.md` 为准。

## 1. 报价兜底模板（P-2）

- 兜底价随代码内置（`builtInPricingFallbackDefaultPrices()`），**无需任何环境变量**即可
  生效，金额与改造前的硬编码值一致：
  - token：聊天输入 `1`、聊天输出 `2`、向量化输入 `1`、重排序输入 `1`（`usd_per_1m_tokens`）；
  - 图片：qwen `160`、doubao `100`、openai `200`、gcp `180`、midjourney `300`、
    未匹配服务商 `200`（`credits_per_image`）。
- 环境变量 `PRICING_FALLBACK_DEFAULT_PRICES` 降级为**可选覆盖**：只覆盖其中列出的条目，
  未列出的沿用内置金额；JSON 非法时整体回退到内置金额并记录日志。取值形如：

  ```json
  {
    "token.chat.input.default": "1.5",
    "image.openai.default": 250
  }
  ```

  > ⚠️ 该变量由 `os.Getenv` 读取，**写进 `api/.env` 文件里不生效**（Pre 其余配置走 viper 读
  > `.env`，两条路径不互通，`.env` 的内容不会进入进程环境）。必须作为**进程环境变量**注入：
  > docker-compose 的 `environment:`、k8s 的 `env:`、systemd 的 `Environment=`，或启动前 `export`。

- 代码内置价仍受组织总开关约束：只有该组织在设置页打开「缺失价格计费策略」后，
  `code_default_fallback` 才会产生报价。
- `GET /console/api/llm/pricing/fallback/template` 返回「规则 + 内置金额 + 本环境覆盖值」，
  仍可供接口调用；但设置页「模型缺价计费策略」面板**已降级为只读展示**：不再提供
  「载入模板 / 保存」，也不能在表格里改价，只显示生效规则与系统默认金额，并提示到
  「模型接入」按模型配置价格。调价统一收敛到模型模块，避免兜底覆盖价与模型价两处打架。
- 后端 `override_rules` 能力保留：`PUT /pricing/fallback` 仍可写入组织覆盖，且 `effective`
  合并仍以「组织覆盖 > 部署默认」为序，已有覆盖会在只读面板里如实展示。
- 该面板默认折叠；「启用缺价计费策略」开关是面板上唯一的可交互项，**切换即保存**
  （回传当前已存的 `override_rules`，不会清空历史覆盖）。`fallbackEnabled` 初始值跟随
  接口加载结果，不再乐观置为 `true`。

## 2. 缓存价格回落倍率（跨仓，Console 侧实现）

- 该行为由 **Console 侧**结算器实现（`zgi-console/api/internal/modules/billing/cache_multiplier.go`），
  不在 Pre：未显式配置 cache 价时回落基准 = 模型 input 价 × 厂商倍率（未知 provider 恒为 1）。
  当前已知厂商：`anthropic` / `claude`（read 0.1、write 1.25）。
- 倍率会写入共享的结算快照 `cache_read_price_multiplier` / `cache_write_price_multiplier`
  以及 `provider`，便于对账解释。
- Pre 侧仍按同步到的显式缓存价或「未配置」处理；不做 provider 倍率注册表、不做模型级倍率覆盖
  （按需求 §0.1 C2）。

## 3. `*_price_configured` 布尔列：保留但不再使用（C′/C3）

- 迁移顺序（ID 升序执行）：
  1. `20260916150000` 给 `llm_custom_models` 加 `pricing` 列；
  2. `20260916150500` **回填**：把 6 个布尔列表达的「显式配置」投影为
     `pricing.rules[].configured=true` 并派生 `token_tiers`；幂等，重跑跳过。
- 6 个 `*_price_configured` 列**不做 DROP COLUMN**。项目迁移规约
  （`api/internal/migrations/README.md` 规则 10）要求破坏性 schema 操作必须显式
  `AllowDestructive`，普通 up 迁移不得使用；保留这些空列不影响运行，也避免
  本地/生产库出现未受控的破坏性变更。原 `20260916151000_drop_price_configured_booleans`
  迁移已移除。
- 运行时只读 `pricing`（`ResolveConfigured`）并叠加组织 override；模型结构体上的
  `*_price_configured` 是 `gorm:"-"` 派生 JSON 字段，不读写数据库列。
- **150500 回填仍必须执行**：否则存量自定义模型（`pricing` 为默认 `'{}'`）没有
  rules，会被判定为「未配置」，缺价时 fail-closed 或按 fallback 计费。
- 升级顺序：先执行迁移（含 150500）再发新代码；列是否保留与判定无关。

## 4. 视频预扣与结算的 lane 语义（P-1 / P-4）

- private lane：预扣与结算都读本地模型价；缺价返回 `ErrPricingNotConfigured`。
- platform lane：本地报价恒为 0，预扣仍创建 billing attempt 并携带
  `X-ZGI-Billing-*` / `X-ZGI-Request-ID` / `X-ZGI-Attempt-ID`；结算金额以 Console
  权威结算（`resp.Settlement` 或按 attempt 查询）为准，**不会**因为本地报价为 0 而失败。
- 成功结算不再出现「缺价静默 0 扣费」；失败退款不因缺价被阻断（仍会回滚）。

## 5. 统计维度（X-4）

- `llm_usage_bills.operation`：由 `20260916120000` 追加列（`varchar(64)`，可空），
  再由追加迁移 `20260916153000_harden_usage_bills_operation` 收紧为
  `NOT NULL DEFAULT ''`（历史 NULL 行统一回填为 `''`，历史 operation 无法推断）。
  索引仍沿用 `20260916120000` 创建的
  `(organization_id, operation, request_created_at)`；PostgreSQL 可反向扫描该索引
  满足倒序查询，因此未再建 DESC 索引。
- 默认 operation 为 `chat`；embedding/rerank/image/video/music/audio 在其路径上显式覆盖。
- 模型用量统计按 operation 分组返回；调用日志支持 `attempt_id` 过滤，
  输出中的 `settlement_id` 在无远端扣费 ID 时即 attempt_id。
- 已应用的迁移不做原地修改（Pre README 规则 40：schema 变更一律追加迁移）。

## 6. 点单位与倍率下发（单位统一）

- **唯一账本单位 = 点**（`numeric(20,3)`）。内部单位（= 点 × 1000）退役；`llm_usage_bills`、
  `channel_wallets`、`billing_attempt_entries`、开发者配额等列均为点。
- **扣减倍率 `points_per_usd` 的权威在 Console**（`billing_configs`，全环境单值，当前 1000），
  随 catalog 发布 payload 下发：
  `CatalogService.GetPublishedCatalog → catalogsync.Synchronizer → public.llm_billing_settings`。
  Pre 本地报价/本地扣点**只读本地副本**（`llm/billingsettings`）。
- **离线行为**：Console 不可达时沿用上次同步值；首次无值（或行值非法）用编译默认
  `1000` 并打告警日志；catalog 不带该字段（旧 Console）时忽略，不清零本地值。
- **快照**：`llm_usage_bills.pricing_snapshot.charged_credits` 为点；`points_per_usd` 随结算快照留痕，
  便于对账复核。
- **计量预扣衔接（P2/W15）**：`PreDeductMeteredUsage` 的字段名不变，值语义为点；
  验收数字按点（$0.10 图片 = `estimated = reserved = 100`）。
