# 新会话改造任务书：Pre 调用统计补齐真实 token + cloud 模式渠道展示

> 交付对象：新会话的编码 agent。本文件是自包含任务书，直接按此执行。
> 仓库：`/Users/du/wwwdata/zgi-pre`（运行时 Pre）、`/Users/du/wwwdata/zgi-console`（控制面/计费 Console）。

## 0. 现象

Pre 使用上游能力时，部分能力的 token 统计拿不到：价格/点数是对的，但 Pre 的「调用概览/调用日志」里 prompt/completion/cache token 全为 0；同样的调用在 Console 里能看到真实 token。用户会因此对账单产生疑问。

另有一个展示问题：Pre 统计页在 **cloud（平台）模式** 下不应展示具体供应商/渠道，只显示「ZGI Cloud」；使用 **自定义渠道/供应商（private 模式）** 时正常显示真实名称。

## 1. 架构与两条 lane（核对结论）

Pre 运行时按 `billing_lane` 分两条结算路径（见 `api/internal/modules/llm/gateway/billing_lane.go`）：
- `platform`（cloud 模式）：Pre 把请求代理给 Console API，**由 Console 真正调用上游并做权威计量/扣费**；Pre 只拿结算结果并镜像到本地。
- `private`（自定义渠道）：Pre 自己调上游并在本地结算。

Pre 统计读的是本地表 `llm_usage_bills`（模型 `api/internal/modules/llm/gateway/usage_bill.go:20`，表名 `llm_usage_bills`），由 `statistics/repository/invocation_log_repository.go` 聚合。
Console 自己的用量表是 `llm_tenant_usage_logs`（`zgi-console/api/internal/modules/billing/model.go:20`，含完整 token 字段），由 `SettlementHandler.SettleUsage` 写入。

## 2. 根因（已核对，按 lane 分类）

### 2.1 platform/cloud lane：Console 没有把 token 传给 Pre（主因）

- Console 的结算查询权威快照 `SettlementSnapshot`（`zgi-console/api/internal/modules/billing/service.go`）与底层 `BillingDeduction`（`.../billing/model.go:134`）**没有 prompt/completion/cache token 字段**，只有 `ActualQuantity/EstimatedQuantity/Operation/Meter/BaseUnit/PricingSnapshot` 与点数。
- Console 的查询响应 `SettlementQueryHandler`（`zgi-console/api/internal/modules/billing/settlement_http.go`）因此也不返回 token。
- Pre 的查询响应结构 `settlementQueryResponse`（`api/internal/modules/llm/gateway/settlement_query.go:14`）同样没有 token 字段；`settlementFromQueryResult`（`api/internal/modules/llm/gateway/billing_attribution.go:212`）只映射点位/成本/渠道。
- 内联结算结构 `adapter.SettlementResult`（`api/internal/modules/llm/protocol/adapters/adapter.go:468`）也没有 token 字段。
- Pre `FinalizePlatformProxySettlement`（`api/internal/modules/llm/gateway/billing_proxy_finalizer.go:16`）与 `MirrorPlatformMediaSettlement`（同文件 :106）只设置 `bc.ActualCredits` 和 `applyPlatformSettlementCostSnapshot`（同文件 :135，只拷价格/成本/渠道），**从不设置 `bc.PromptTokens/CompletionTokens/TotalTokens/Cache*`**。
- 最终 `buildUsageBill` → `usageBillTokens`（`api/internal/modules/llm/gateway/billing_usage_bill.go:105/:220`）从 `BillingContext` 取 token，于是写入 `llm_usage_bills` 的全是 0。

> 结论：platform lane 的 token 真相在 Console（`llm_tenant_usage_logs`），但 Console 的 settlement 响应没暴露，Pre 也没解析。需两端一起改。

### 2.2 private lane：媒体能力没记录 token（次因）

- chat/embedding/rerank：已在 `api/internal/modules/llm/gateway/request.go:579-660` 从 `adapter.Usage` 写入 `bc`，正常。
- image：`adapter.ImageResponse`（`adapter.go:658`）没有 `Usage` 字段，`image_service.go` 从不设置 `bc.PromptTokens/CompletionTokens` → **按 token 计费的图片调用 token 记为 0**（即使 private lane）。
- video：`video_service.go:907-911` 用 `resp.Usage.PromptTokens/CompletionTokens/TotalTokens`，但视频用量是 `video_tokens`（raw 解析见 `protocol/adapters/provider/raw_protocol_helpers.go` 的 `video_tokens/total_video_tokens`），prompt/completion 可能为 0，且未走归一化 bucket。
- audio/music（metered）：本就按单位计费，token 为 0 属预期，但 UI 不应展示 token 公式（现状已按 `metered_unit` 处理）。

## 3. 改造点

### 3.1 Console → 暴露 token

1. `zgi-console/api/internal/modules/billing/service.go`：`SettlementSnapshot` 增加 token 字段（prompt_tokens、completion_tokens、total_tokens、cache_read_tokens、cache_write_tokens、cache_write_5m_tokens、cache_write_1h_tokens、reasoning_output_tokens；若支持 realtime 再加 audio_input/audio_output_tokens）。
2. `GetSettlementByAttempt`：按 `organization_id + request_id/attempt_id` 关联 `llm_tenant_usage_logs`（该表有完整 token）填充上述字段。若一次 attempt 可能多条 usage log，明确定义聚合口径（建议按 request_id 聚合/取最新）。
3. `SettlementQueryHandler`（`billing/settlement_http.go`）：响应 JSON 增加这些字段。
4. 如果官方流的内联结算也走同一份数据，给内联 settlement payload（Console 侧对应响应结构）也补 token 字段，避免 Pre 只能靠 query 兜底。
5. 补 Console 侧契约测试：settlement query 返回 token。

### 3.2 Pre → 解析并写入本地 bill

1. `api/internal/modules/llm/gateway/settlement_query.go`：`settlementQueryResponse` 增加 token 字段，与 Console 响应字段名一致。
2. `api/internal/modules/llm/protocol/adapters/adapter.go`：`SettlementResult` 增加 token 字段（内联路径）。
3. `settlementFromQueryResult`（`billing_attribution.go`）：把 token 映射进 `SettlementResult`。
4. `FinalizePlatformProxySettlement` / `MirrorPlatformMediaSettlement`（`billing_proxy_finalizer.go`）：在 `upsertUsageBill` 之前，用 settlement 的 token 覆盖 `bc.PromptTokens/CacheReadTokens/CacheWriteTokens/CompletionTokens/TotalTokens`（注意：只有当 settlement 提供了有效 token 时才覆盖，避免把 private lane 的已采集值清零；platform lane 本来就应为 0，可安全覆盖）。
5. 保留 `bc.TotalTokens` 的兜底：若 settlement 没给 total，则用各分项相加（现有 `usageBillTokens` 已有此逻辑）。
6. 若已有从 bill hydrate 的逻辑（`billing_grpc.go:1403` 附近 `bc.PromptTokens = bill.PromptTokens`），确认新字段也被 hydrate。

### 3.3 Pre private 媒体补 token

1. image：给 `adapter.ImageResponse` 增加 `Usage *Usage`（或等价字段），在各图片适配器里解析上游返回的 image token 用量；`image_service.go` 在 `settleImageSuccess` 前把 usage 归一化写入 `bc`（prompt/cache/completion/total）。参考 `request.go` 的 `applyUsageToBillingContext` 写法。
2. video：把 `video_tokens` 归一化记入 bill（可为 video 增加独立 token 字段，或先统一映射到 `TotalTokens` 并保证 non-zero）；不要把 video token 混入 prompt/completion 造成口径混乱。
3. 为 image token / video token 补单测，断言 `llm_usage_bills` 写入后 token 非 0。

### 3.4 前端展示：cloud 模式隐藏供应商/渠道

文件：`web/src/components/usage/invocation-log-section.tsx`
- 列表行 `:315-318`：`actual_channel_name || channel_name` 与 `supplier_name`。
- 详情 `:480-485`：provider / channel / actualChannel / supplier。
- 已有 `:577` 的 `isPlatform = item.pricing_details?.billing_lane === 'platform'`。

改造：
- 计算 lane：优先 `item.pricing_details?.billing_lane === 'platform'`（类型见 `web/src/services/types/statistics.ts:174`）；兼容 `use_system_provider`（若有）。
- platform：渠道/实际渠道/供应商统一显示 `ZGI Cloud`（或隐藏供应商行），不得泄露真实上游供应商名。
- private：保持现状，显示 `channel_name` / `actual_channel_name` / `supplier_name`。
- 抽一个纯函数（如 `resolveChannelDisplay(item)`）并加单测，避免两处渲染不一致。
- 注意 i18n：新增文案走 en-US / zh-Hans 两个 locale（现有 i18n 模块 `web/src/i18n/modules/dashboard/*`）。

## 4. 验收标准

1. platform 模式、带 token 的调用（chat/embedding/rerank）：Pre `llm_usage_bills` 的 prompt/completion/total（及 cache）token 与 Console `llm_tenant_usage_logs` 一致（或符合既定聚合口径），Pre 调用概览可见。
2. platform 模式图片/视频调用：若上游返回 token 用量，Pre 同样能看到（图片 token 计费场景）。
3. private 模式：行为不变且 token 正确；图片 token 计费场景 token 非 0。
4. 展示：platform 调用只显示 ZGI Cloud；private 调用显示真实渠道/供应商。
5. 回归：新增/更新 Console 契约测试 + Pre 结算/统计测试 + 前端展示单测；两端 `go test` / `tsc` / `eslint` 绿。

## 5. 建议验证命令

- Console：`cd zgi-console/api && go test ./internal/modules/billing/... ./internal/modules/gateway/...`
- Pre：`cd zgi-pre/api && go test ./internal/modules/llm/gateway/... ./internal/modules/llm/statistics/...`
- Pre Web：`cd zgi-pre/web && node_modules/.bin/tsc --noEmit && node_modules/.bin/eslint <changed files>`
- 端到端：用 cloud 模式跑一次 chat、一次按 token 计费的图片，对照 Console 与 Pre 的调用详情 token。

## 6. 注意事项 / 易错点

- **不要**用「Pre 自己估算的 token」去顶替 platform lane 的真实用量；platform lane 的权威来源是 Console。
- 覆盖 `bc` token 时注意私有 lane 已有值不要被空 settlement 清零。
- token 字段的 JSON 命名两端必须一致（建议直接复用 `prompt_tokens` 等 snake_case）。
- `llm_usage_bills` 通过 `attempt_id` upsert（`billing_usage_bill.go`），改完后确认 upsert 的 `usageBillUpsertColumns` 覆盖 token 列（已包含，无需改）。
- 展示层不要只按 `provider_name` 判断 cloud；以 `billing_lane` 为准。
