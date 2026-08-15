# 长 Agent 任务上下文管理与压缩技术方案

状态：Proposal｜日期：2026-08-14｜范围：ZGI Agent 单次长任务中的模型上下文构建、工具结果管理、压缩、恢复与观测

## 1. 结论

ZGI 的压缩触发点应从“SSE 开始前”移动到“每一次真实模型调用前”。SSE 只是输出传输通道，不应拥有上下文生命周期，也不应决定何时摘要。

一个长 Agent 任务需要维护一条独立于 SSE 的活动执行 transcript：

```text
当前用户任务
  -> 模型响应（可能包含文本和一个或多个 tool_call）
  -> 所有工具执行结果
  -> 下一次模型响应
  -> 所有工具执行结果
  -> ...
  -> 最终回答
```

每次调用模型前，都基于这条 transcript 和本次实际携带的系统提示词、工具 Schema、运行时状态构造最终请求，计算完整 Token；如果接近模型窗口，先清理旧工具结果，再压缩较早的完整 API round，保留近期原文，然后才发送模型请求。

这不是对现有“SSE 开始前按数据库历史轮次生成语义摘要”方案的补丁。现有方案的数据模型、触发位置和失败语义都不适合长 Agent 任务，应删除并由 run-scoped 的上下文管理器替代。

参考实现依据：Claude Code 在每次进入模型查询循环时先处理工具结果、microcompact 和 autocompact，再调用模型；完成一轮后将本轮 assistant 消息与全部工具结果追加到下一轮状态。见 [CC-01]、[CC-02]。

## 2. 目标与非目标

### 2.1 目标

1. 单条用户指令即使触发数十或数百次模型调用，也能在同一个逻辑任务中持续运行。
2. 每次工具调用及其结果都能进入后续模型上下文，不遗漏知识图谱、文件读取、命令执行等结果。
3. 每次真实模型调用前使用最终请求计算 Token，动态工具 Schema、补充上下文和本轮工具结果都必须计费。
4. 压缩边界按 API round，而不是按用户消息轮次；压缩不能拆开 `tool_call` / `tool_result` 配对。
5. 审批、用户补充输入、客户端动作、SSE 断线重连不会丢失当前 Agent 的上下文状态。
6. 压缩失败时尽量降级和恢复，只在当前请求确实无法装入模型窗口时终止当前 run，不能把整段会话永久阻断。
7. 调试文件能够还原每一次最终实际请求及其准确的 Token 构成和压缩决策。
8. 模型物理窗口只代表协议容量；Agent 使用独立的工作窗口控制日常上下文规模，默认 256K Token。

依据：Claude Code 明确将消息按 API round 分组，以支持“只有一个用户 prompt、但内部有很多 Agent round”的会话；见 [CC-04]。工具结果限额、替换记录持久化和恢复见 [CC-05]、[CC-12]。

### 2.2 非目标

- 不以改善 SSE 展示文案为目的设计摘要。
- 不把压缩状态建模成前端必须参与的 SSE 业务流程。
- 不保存或回放模型内部推理内容；只保存模型可见的 assistant 正文、工具调用和工具结果。
- 不用摘要替代完整审计记录。原始执行记录仍可持久化，但不必全部放入模型 prompt。
- 不要求不同模型使用同一个固定百分比阈值。

## 3. 核心概念与生命周期

### 3.1 三种不同的数据

| 数据 | 生命周期 | 作用 | 是否直接进入每次模型请求 |
| --- | --- | --- | --- |
| Conversation History | 多个已结束用户轮次 | 新 Agent run 启动时提供背景 | 仅启动时选取一次，之后成为 run 初始上下文 |
| Agent Execution Transcript | 一个逻辑 `agent_run_id` | 保存本次长任务的每个 API round、工具调用和结果 | 是，是压缩的主要对象 |
| SSE Events | 一次网络连接 | 把进度、文字和状态传给客户端 | 否；只有同时属于模型 transcript 的内容才进入上下文 |

Conversation History 不能替代 Agent Execution Transcript。当前 ZGI 数据库历史主要还原用户 `Query` 和最终 `Answer`，无法完整还原一个长任务内部的工具轨迹，见 [ZGI-02]。

### 3.2 标识符

- `agent_run_id`：逻辑 Agent 任务 ID。审批、用户输入、客户端动作或 SSE 重连后仍保持不变。
- `transport_id`：一次 SSE 连接或请求尝试，仅用于传输观测。
- `api_round_seq`：本 run 内实际模型调用序号，从 1 单调递增。
- `model_request_id`：一次上游模型请求 ID，用于关联请求、响应和 usage。
- `tool_call_id`：模型工具调用与工具结果的配对 ID。

上下文 checkpoint 必须以 `agent_run_id` 为主键，而不是以 SSE 或当前 `message_id` 为生命周期边界。

依据：Claude Code 的查询循环把 `messagesForQuery + assistantMessages + toolResults` 写入下一轮 `state.messages`，同一状态跨模型调用继续流转，见 [CC-02]；内容替换决策也会写 transcript 并在 resume 时重建，见 [CC-12]。

### 3.3 API round

一个完整 API round 定义为：

```text
一次模型响应
+ 该响应产生的全部 tool_call
+ 每个 tool_call 对应的最终 tool_result（成功、失败、拒绝或取消）
```

如果一次模型响应包含多个工具调用，可以串行或并行执行，但在再次调用模型前，必须为所有工具调用补齐结果。任何压缩和裁剪都只能发生在完整 round 的边界，不能留下孤立的 `tool_call` 或 `tool_result`。

依据：Claude Code 的 API-round 分组以 assistant response ID 变化为边界，并说明同一响应的多个工具调用及交错结果属于同一组，见 [CC-04]；保留近期消息时还会反向调整边界，确保工具配对和同一 assistant response 的分片不被拆开，见 [CC-08]。

## 4. 当前 ZGI 设计的问题

### 4.1 压缩只在 SSE/run 启动前执行一次

`prepareLLMRequestForRun` 在进入 Agent loop 之前构建上下文、计算历史压力并调用 `compactContextForRun`，见 [ZGI-01]。进入 loop 后虽然每轮都会重新构造 `planningReq`，但不会再次做语义压缩。

这导致一次用户请求内部新增的几十轮工具结果永远不会触发现有摘要。它只能处理“开始新请求时数据库历史太长”，不能解决“当前 Agent 正在持续工作时上下文增长”。

### 4.2 控制指标只衡量数据库历史

当前 `HistoryPressure` 的计算是：

```text
HistoryTokens = CandidateRequestTokens - FixedRequestTokens
HistoryBudgetTokens = PromptBudget - FixedRequestTokens
HistoryPressure = HistoryTokens / HistoryBudgetTokens
```

见 [ZGI-03]。这个指标适合观察“旧对话占剩余空间的比例”，但不适合作为模型请求是否安全的唯一控制量：

- 当前 run 新增的工具结果不一定属于这里的 History。
- “固定请求”会随已加载 skill、动态工具 Schema、补充状态而变化，并不真正固定。
- 即使历史压力很低，一个巨大的最新知识图谱结果也可能使最终请求超限。
- 即使历史压力很高，只要最终请求仍有充分 headroom，也不需要立刻语义摘要。

### 4.3 摘要输入丢失 Agent 执行轨迹

当前历史构建仅产生历史用户消息和最终 assistant answer，见 [ZGI-02]；摘要输入也只序列化 `Status/User/Assistant`，见 [ZGI-04]。工具名、参数、执行结果、错误、文件引用、知识图谱检索结果和中间工作状态均不在语义摘要输入中。

因此即使触发摘要，也无法可靠总结一个真实的长 Agent 任务。

### 4.4 Agent loop 已保存工具结果，但预算策略太晚且不可持续

当前 loop 会把 assistant tool calls 和每个工具结果追加到 `messages`，见 [ZGI-05]，这一点方向正确。但 `applyFinalPlanningRequestBudget` 只有在最终估算已经超过 `PromptBudget` 时才依次替换旧内容，见 [ZGI-06]，并且替换结果只作用于当次请求副本，没有成为清晰、可恢复的 canonical context 状态。

旧工具结果压缩还无条件保护“最新一个”结果，见 [ZGI-07]。保护最新结果通常合理，但如果最新知识图谱结果本身极大，仍可能直接失败，因此还需要单条工具结果上限和外部存储引用。

### 4.5 中间 assistant 正文被一刀切清空

当响应含工具调用时，当前代码把 `planningMessage.Content` 和 `ReasoningContent` 都清空，再追加到历史，见 [ZGI-11]。内部推理不应持久化是正确的，但 assistant 的普通正文可能包含后续执行所需的计划、判断和约束，不应因为同一响应含 tool call 就全部丢弃。

应区分：

- 模型返回、且后续模型可见的 assistant 正文：保留。
- 运行时生成的 SSE 进度文案：不进入 transcript。
- 模型内部 reasoning：不保存、不回放。

依据：Claude Code 下一轮状态包含本轮全部 `assistantMessages`，见 [CC-02]；摘要格式化时明确移除仅用于起草的 analysis scratchpad，见 [CC-09]。

### 4.6 失败语义过重

现有压缩最多连续尝试 3 次，失败后写入 `failed_blocked` 并阻断请求，同时把压缩建模成 SSE progress，见 [ZGI-10]。网络波动或摘要模型暂时失败不应使整段会话无法继续；如果当前请求仍在硬限制内，应发送未压缩或仅轻量压缩后的请求。

Claude Code 使用连续失败熔断来停止反复浪费压缩调用，但不会因为一次主动压缩失败就立即把会话全局阻断，见 [CC-07]。

### 4.7 调试文件的 Messages 是真的，顶部指标可能是旧的

当前 callback 确实会在每次 Agent 模型请求前写 dump，见 [ZGI-08]；但 dump 顶部从最初的 `prepared.contextBudget` 读取窗口、历史 Token 和压力，而正文的 `request.Messages`、`request.Tools` 来自当前实际请求。因此后续 round 新增工具结果时，完整 Messages 会变化，顶部指标却可能仍是 run 启动时的值。

## 5. 目标架构

### 5.1 组件边界

新增一个 run-scoped `ContextManager`，由 Agent runner 持有并在每次模型调用前调用。建议放在独立包，避免 `service` 与 `skillloop` 循环依赖：

```text
api/internal/capabilities/chatruntime/contextmgr/
  manager.go              # PrepareBeforeModelCall / ObserveModelResponse
  state.go                # AgentContextState、ApiRound、Checkpoint
  budget.go               # 完整请求估算、阈值和压力
  tool_projection.go      # 单条工具结果限额、外部引用、旧结果清理
  compactor.go            # 语义压缩接口和边界选择
  compact_prompt.go       # 直接移植 Claude Code compact prompt 与输出包装模板
  checkpoint.go           # 持久化接口
  diagnostics.go          # 指标与 dump 数据
```

`service` 负责组装依赖：模型规格、摘要模型调用、checkpoint repository、调试输出。`skillloop.Runner` 只通过接口使用上下文管理器。

```go
type Manager interface {
    PrepareBeforeModelCall(
        ctx context.Context,
        state *AgentContextState,
        req *adapter.ChatRequest,
    ) (*adapter.ChatRequest, ContextDecision, error)

    ObserveModelResponse(
        state *AgentContextState,
        response adapter.Message,
        usage *adapter.Usage,
    )

    AppendToolResults(
        state *AgentContextState,
        results []adapter.Message,
    ) error

    Checkpoint(ctx context.Context, state *AgentContextState) error
}
```

依据：Claude Code 把所有上下文治理放在 query loop 的模型调用入口，而不是 UI/transport 层，见 [CC-01]。

### 5.2 Canonical state

```go
type AgentContextState struct {
    AgentRunID string
    NextRound  int

    // 新 run 启动时从 Conversation History 选取，之后不再按 SSE 重建。
    Bootstrap []adapter.Message

    // 历史压缩后的单一累积摘要；可能为空。
    Summary *ContextSummary

    // 尚未被摘要覆盖的完整 API rounds。
    ActiveRounds []ApiRound

    // 当前用户任务、当前尚未完成的模型响应/工具批次。
    Pending *PendingRound

    // 计划、审批、已加载 skill、文件引用等需要在压缩后恢复的结构化状态。
    RuntimeState RuntimeStateRefs

    // 已把大工具结果替换为外部引用的稳定决策。
    ContentReplacements map[string]ContentReplacement

    LastUsage       *adapter.Usage
    CompactionState CompactionTracking
}
```

`AgentContextState` 是模型上下文的事实来源。原始 audit transcript 可以更完整，但模型请求必须由 state 确定性投影得到。同一个 `agent_run_id` 在恢复后必须得到相同的替换文本和 round 顺序。

每个逻辑用户轮次结束时，还要把该轮产生的模型可见 assistant/tool 消息作为版本化的 `metadata.agent_transcript` 写入对应的 `chat_runtime_messages` 行。它不包含 system prompt、bootstrap 历史、当前 `query`、最终 `answer`、reasoning 或仅供 SSE 展示的进度事件。新一轮从数据库恢复会话历史时，按以下顺序拼接：

```text
query
-> metadata.agent_transcript（完整且配对的 assistant tool_calls / tool results）
-> answer
```

数据库不需要新增工具调用表；`query / metadata / answer` 共同构成一个逻辑 turn 的持久化记录。运行中未完成的尾部工具批次不得进入后续模型请求；异常结束恢复时，只保留已经补齐全部 tool result 的完整 API round。现有 `skill_invocations`、`runtime_timeline` 和默认精简的 `model_invocations` 继续用于展示、审计与诊断，不能作为模型协议 transcript 反向还原的数据源。

`metadata.agent_transcript`、版本号、完整 `model_invocations` 请求/响应以及压缩 snapshot 都是后端私有数据。数据库持久化必须保留原值，但任何 HTTP/SSE 出口都只能发送复制后的公开 metadata：删除 transcript 和完整模型调用载荷，只保留模型调用次数、压缩状态及 `skill_invocations`、工作流、文件、审批等前端展示字段。脱敏不得原地修改持久化 metadata。

依据：Claude Code 对工具结果替换使用稳定 state，先前替换过的结果在后续请求中复用相同替换文本，并把记录写入 transcript 供 resume 重建，见 [CC-05]、[CC-12]。

### 5.3 每次模型调用前的唯一入口

```text
Runner 准备本轮动态 Tools / ToolChoice / MaxTokens
  -> ContextManager 从 canonical state 投影完整 Messages
  -> 对新增工具结果执行单条结果限额
  -> 组装最终 ChatRequest（含 system、tools、runtime state）
  -> 估算最终完整请求 Token
  -> 未到 soft limit：直接发送
  -> 到 soft limit：清理旧工具结果，重新估算
  -> 仍到 soft limit：压缩较早的完整 API rounds，重建并重新估算
  -> 超 hard limit：执行一次紧急压缩/恢复策略
  -> 写“最终实际请求”dump
  -> 调用模型
  -> 记录真实 usage
  -> 追加 assistant 响应
  -> 执行并追加该响应的全部工具结果
  -> 回到下一次模型调用前入口
```

压缩后的 Messages 必须写回 `AgentContextState`，下一轮继续使用压缩后的 canonical state，不能只修改某次 `planningReq` 副本。

依据：Claude Code 的调用顺序是工具结果预算、microcompact、autocompact、模型调用；压缩后更新下一轮 state，见 [CC-01]、[CC-02]。

上下文管理与执行额度必须解耦。`ContextManager` 不决定总 round 数、任务时长或业务工具调用上限；这些仍由 Agent execution policy 控制。要真正支持数小时任务，还需把当前固定 planning-round 上限改成可配置的 run budget，但不能用“上下文已长”作为停止任务的理由。当前 ZGI loop 的 round 上限入口见 [ZGI-05]，Claude Code 也把 `maxTurns` 检查作为独立于压缩的终止条件，见 `src/query.ts:L1704-L1711`。

## 6. Token 与阈值设计

### 6.1 模型物理窗口与 Agent 工作窗口

模型声明的 1M、200K 或 128K 是物理上下文窗口，表示供应商协议允许的输入和输出总容量。它不是 Agent 应当长期使用的正常工作区。为了避免上下文接近物理极限时模型效果下降，ZGI 必须增加独立的 Agent 工作窗口：

```text
ModelContextWindow          = 当前模型的物理上下文窗口
ConfiguredAgentWindowK      = CHAT_RUNTIME_AGENT_CONTEXT_WINDOW_K
ConfiguredAgentWindowTokens = ConfiguredAgentWindowK * 1_000
AgentContextWindow          = min(ModelContextWindow,
                                  ConfiguredAgentWindowTokens)
```

环境变量定义：

```dotenv
# Agent working context window size in thousands of tokens (k). Default: 256.
# 1k means 1,000 tokens. The effective value is capped by the model's physical context window.
CHAT_RUNTIME_AGENT_CONTEXT_WINDOW_K=256
```

默认值为 `256`，即 256,000 Token。单位固定为十进制 `k`，`1k = 1,000 Token`，不是 1,024。

代码校验规则：

1. 未配置时使用 `256`。
2. 配置值必须是正整数；空字符串以外的非整数、零、负数或乘以 1,000 后溢出，配置加载失败，服务不得带错误配置启动。
3. 模型规格必须提供大于零的 `ModelContextWindow`；无法确定物理窗口时，本次请求失败，不能猜测一个窗口继续执行。
4. 每次解析当前模型规格后计算 `AgentContextWindow = min(ConfiguredAgentWindowTokens, ModelContextWindow)`。
5. 如果模型物理窗口小于配置值，有效工作窗口自动等于模型物理窗口，不报错。例如配置 256K、模型只有 128K，则有效工作窗口为 128K。
6. 代码中任何最终有效值都不得大于模型物理窗口，并记录 `agent_context_window_clamped=true`，便于发现配置与模型能力不匹配。
7. 计算本次输出和压缩预留后，必须校验 `PromptBudget > 0` 且 `CompactInputLimit > EmergencyBuffer`；工作窗口小到无法容纳基本请求时，本次请求失败并报告配置/模型规格错误。

示例：

| 环境变量 | 模型物理窗口 | 有效 Agent 工作窗口 |
| ---: | ---: | ---: |
| 未配置 | 1,000K | 256K |
| 256 | 200K | 200K |
| 256 | 128K | 128K |
| 512 | 1,000K | 512K |
| 64 | 200K | 64K |

后续所有预算、压力、压缩阈值、保留尾部大小和验收判断，都必须基于 `AgentContextWindow`，不能再直接基于 `ModelContextWindow`。

依据：Claude Code 默认把模型上下文按 200K 处理，1M 需要显式模型能力、后缀、beta 或实验开启；同时允许用本地上限覆盖 1M endpoint 的上下文决策，以及用 auto-compact window 限制压缩计算窗口，见 [CC-14]。ZGI 将这种“有效工作窗口”从内部覆盖项提升为正式的运行时配置。

### 6.2 控制量必须是最终请求总 Token

定义：

```text
ModelContextWindow = 模型物理上下文窗口，仅用于约束 AgentContextWindow
AgentContextWindow = min(ModelContextWindow, ConfiguredAgentWindowTokens)
MainOutputReserve  = 本次主模型 MaxTokens
PromptBudget       = min(MaxInputTokens,
                         AgentContextWindow - MainOutputReserve)
FinalPromptTokens = EstimateChatRequest(finalRequest)
ContextPressure   = FinalPromptTokens / PromptBudget
```

`EstimateChatRequest` 必须计算：

- 完整 system prompt；
- 历史摘要和保留原文；
- 当前任务及全部 assistant/tool 消息；
- 补充运行时状态；
- 本轮完整工具定义 Schema、tool choice 等协议开销；
- 图片、文档等多模态内容。

工具结果不能从 Token 计算中排除。知识图谱结果只要实际出现在 `req.Messages` 中，就必须进入 `FinalPromptTokens`。

依据：Claude Code 的 canonical token 函数使用最近一次 API usage 加上之后新增消息的估算，并专门处理同一响应的并行工具调用和交错工具结果，避免漏算，见 [CC-03]。

例如模型物理窗口为 1M、Agent 工作窗口使用默认 256K、主模型输出预留 16K：

```text
ModelContextWindow = 1,000K
AgentContextWindow = 256K
MainOutputReserve  = 16K
PromptBudget       = 240K
```

剩余的 744K 物理容量不参与日常预算，只作为供应商物理余量，不能被普通 Agent round 消耗。

### 6.3 soft limit、hard limit 与目标值

直接删除 `HistoryPressure` 指标及当前硬编码的“历史压力 10% 触发、80% 安全、50% 目标”。不在 metadata、日志、指标或 prompt dump 中保留兼容字段。压缩决策只基于最终完整请求的 Token 数与 soft/hard limit。

建议按 headroom 配置：

```text
SummaryOutputReserve = min(model.max_output_tokens, 20_000)
EmergencyBuffer      = 模型/供应商配置，初始可取 8_000~16_000
CompactInputLimit    = AgentContextWindow - SummaryOutputReserve
SoftLimit            = min(PromptBudget - EmergencyBuffer,
                           CompactInputLimit - EmergencyBuffer)
HardLimit            = PromptBudget
TargetTokens          = min(SoftLimit - Hysteresis,
                            floor(PromptBudget * TargetRatio))
```

初始建议 `TargetRatio=0.60`，`Hysteresis` 至少 8K Token；最终值应通过真实 usage 校准。核心原则不是固定百分比，而是：触发时仍有空间完成摘要调用，压缩后也不会下一轮立刻再次触发。

依据：Claude Code 为摘要输出最多预留 20K Token，并在有效窗口前再留 13K buffer，测试时才允许百分比覆盖，见 [CC-07]；它还显式计算压缩后上下文是否会在下一轮再次触发，见 [CC-10]。

`HardLimit` 是 Agent 工作窗口推导出来的产品上限，不是模型物理上限。即使 1M 模型还有大量物理空间，普通主模型请求超过 `HardLimit` 后也必须先治理或压缩，不能继续向 1M 堆积。

以 1M 物理窗口、默认 256K 工作窗口、16K 主输出预留、20K 摘要输出预留和 13K EmergencyBuffer 为例：

```text
PromptBudget      = 256K - 16K = 240K
CompactInputLimit = 256K - 20K = 236K
SoftLimit         = min(240K - 13K, 236K - 13K) = 223K
HardLimit         = 240K
```

也就是说，约 223K 开始主动压缩，240K 是 Agent 请求红线，而不是等到接近模型的 1M 物理窗口才压缩。

### 6.4 估算与真实 usage 校准

每次响应后记录供应商返回的 input/output/cache usage。下一次调用可采用：

```text
EstimatedContext = LastActualInputTokens
                 + Estimate(messages added after last request)
                 + Estimate(changed tools/system/runtime state)
```

同时保留对完整最终请求的本地估算并比较误差，按 `provider + model + tokenizer` 维护保守校准系数。若没有可靠 usage，直接使用完整请求估算并增加安全折扣。

依据：Claude Code 使用最近一次真实 API usage 作为基线，再估算之后新增的消息，避免对不断增长的上下文做累计重复计算，见 [CC-03]。

## 7. 工具结果管理

### 7.1 工具结果必须进入活动 transcript

所有工具结果——成功、业务失败、权限拒绝、超时——都必须用对应的 `tool_call_id` 追加到当前 API round。下一次模型调用必须看到这一批全部结果，否则模型无法知道动作是否执行、拿到了什么事实、是否需要重试。

原始工具结果进入 transcript，不代表永久全文驻留 prompt。进入 transcript 是语义正确性；外部化、清理和摘要是容量治理，两者不能混为一谈。

依据：Claude Code 下一轮状态显式追加全部 `toolResults`，见 [CC-02]；API-round 分组假设工具调用在下一个 assistant round 前都已解决，见 [CC-04]。

### 7.2 两级工具结果治理

#### 第一级：新增结果的单条/单批上限

工具返回后立即处理，而不是等整个 prompt 超限：

1. 原始结果写入可审计的外部存储。
2. 根据工具类型生成有界 preview。
3. transcript 保留工具名、`tool_call_id`、状态、关键结果、原始大小、引用地址和截断说明。
4. 同一个结果在后续请求中必须使用完全相同的替换文本，保证可恢复性和 prompt cache 稳定。

依据：Claude Code 在 microcompact 前先执行 per-message 工具结果预算，将大结果持久化并用替换内容进入请求；替换记录用于 resume，见 [CC-01]、[CC-05]、[CC-12]。

#### 第二级：旧结果 microcompaction

到达 soft limit 后，优先把较老、可重建的工具结果正文替换为短 receipt，但保留工具调用和结果壳：

```json
{
  "status": "compacted",
  "summary": "读取了 1 个文件，内容已用于后续修改",
  "artifact_ref": "tool-result://...",
  "original_tokens": 18420
}
```

microcompact 的保护集合按工具批次计算，而不是简单截取最后 N 条工具消息：

1. 如果 transcript 末尾是一个尚未被后续 assistant/user 消息消费的工具调用批次，保护该批次的全部工具结果；同一次模型响应产生的并行调用不可拆开清理。
2. 如果最新批次不足 4 个结果，再从旧到新的反方向补充最近结果，直到至少保护 4 个。
3. 因此保护数量等价于 `max(4, latest_pending_batch_result_count)`；已经被后续 assistant/user 消费的历史批次不再享受整批保护，只参与“最近 4 个”保底。
4. “保护”只表示不移除已经生成的 preview；单条超大原文仍必须先经过第一级 projection，不能把几十万 Token 原文直接留在 prompt。
5. 如果整批保护后仍超过 `HardLimit`，把该批次每个结果都外部化，并对所有 preview 应用同一个逐步缩小的上限，保留能够整体装入请求的最大公平 preview；不能按消息顺序清空前几个结果。

依据：Claude Code 的 microcompact 会把旧结果正文替换为占位内容，同时至少保留最近一个可压缩结果，见 [CC-06]。

#### 按需恢复外部工具结果

`projected` 和 `compacted` receipt 都保留稳定的 `artifact_ref`、`content_hash`、`tool_call_id` 和原始 Token 数。Agent Runtime 暴露内置只读工具 `read_context_artifact`，允许模型在确实缺少证据时恢复同一 run 的完整原始工具结果：

```json
{
  "artifact_ref": "agent-context://tool-results/<agent_run_id>/<content_hash>"
}
```

- 第一版不分页；一次读取并返回该 Artifact 的完整原文。
- 运行时只接受固定 `agent-context://tool-results/...` 引用，并验证引用属于当前 `AgentRunID`、内容 hash 与落盘原文一致。
- 工具结果以小型 JSON 页头加完整原始文本返回，避免把 JSON 原文再次整体转义后放大 Token；展开结果照常进入 transcript、计入下一次调用前的完整预算，并使用与普通工具结果相同的近期批次保护和 microcompact 选择规则。
- `read_context_artifact` 的展开结果禁止再次执行单条大结果 projection，避免形成 Artifact 的递归外部化。它被后续 microcompact 选中时只移除展开正文，复用原 `artifact_ref`、`content_hash` 和来源工具调用信息，不向 Artifact Store 写入副本。
- 如果完整原文在压缩其他可压缩内容后仍无法装入 Agent `HardLimit`，本轮明确以 context exhausted 失败；不得把刚展开、模型尚未读取的结果重新投影成 preview。
- 该工具是上下文运行时控制工具，不计入业务 Skill 工具调用额度，也不能读取其他 Agent run 的 artifact。

### 7.3 工具类型策略

| 工具结果 | prompt 中保留 | 外部化内容 |
| --- | --- | --- |
| 文件读取 | 文件路径、范围、相关片段、hash | 完整文件内容 |
| Shell/命令 | 命令、退出码、尾部错误、关键输出 | 完整 stdout/stderr |
| 搜索/网页 | 查询、命中标题、关键片段、来源引用 | 完整抓取内容 |
| 知识图谱检索 | 查询、top-k 实体/关系、关键证据、来源 ID、总命中数 | 全量节点、边和冗余 metadata |
| 文件生成/修改 | 文件路径、变更摘要、校验结果、hash | 大块生成内容或 patch 详情 |
| 结构化业务工具 | 状态、关键字段、业务 ID、错误码 | 大型原始响应 |

知识图谱结果必须先计入原始工具结果 Token，再记录 projection 前后 Token；不能因为最终只发送 preview 就把原始成本和压缩收益都隐藏掉。

### 7.4 不可破坏的约束

- 不拆开 tool call/result 配对。
- 不改写 `tool_call_id`。
- 当前未完成 round 不参与语义压缩。
- side-effect 工具的成功/失败结论和关键业务 ID 不得清掉。
- 只清正文，不删除“执行过这个工具”的事实。
- 外部引用必须有权限边界、生命周期和不可变内容 hash。

依据：Claude Code 在选取保留尾部时主动回退索引以包含匹配的 `tool_use`，并保留同一 assistant response ID 的全部分片，见 [CC-08]。

## 8. 语义压缩设计

### 8.1 压缩对象

只压缩：

- 已完成的 API rounds；
- 旧摘要加上尚未被覆盖的较早 rounds；
- 必要的初始 Conversation History。

不压缩：

- 当前未完成的工具批次；
- 近期保留尾部；
- system prompt 和 tools Schema，它们每次请求重新构造；
- 当前结构化 runtime state，它应在压缩后重新注入。

依据：Claude Code 在 compact boundary 之后重建消息，并把 summary、保留消息和恢复附件按固定顺序放回上下文，见 [CC-10]。

### 8.2 保留尾部

从 transcript 尾部反向累计，初始建议：

```text
TailMinTokens = clamp(PromptBudget * 0.05, 4_000, 10_000)
TailMaxTokens = clamp(PromptBudget * 0.20, 16_000, 40_000)
TailMinTextRounds = 3
```

达到 `TailMinTokens` 和最少文本 round 后停止扩展，但不能超过 `TailMaxTokens`；最后再把边界调整到完整 API round。数值需要配置化并通过长任务评测校准。

依据：Claude Code 的 session-memory compaction 默认保留至少 10K Token、至少 5 个文本消息、最多 40K Token，并在选取后修正工具配对边界，见 [CC-08]。ZGI 按模型 PromptBudget 比例化，是对不同窗口模型的适配，不是照抄常量。

### 8.3 摘要输入和输出

首版摘要 Prompt 和相关输出模板不重新设计，直接使用 Claude Code `src/services/compact/prompt.ts` 中的实现，包括：

- `NO_TOOLS_PREAMBLE` 和 `NO_TOOLS_TRAILER`；
- `BASE_COMPACT_PROMPT`；
- `PARTIAL_COMPACT_UP_TO_PROMPT`；
- `getCompactPrompt`；
- `getPartialCompactPrompt`；
- `formatCompactSummary`；
- `getCompactUserSummaryMessage`。

模板以原始英文内容移植到 `contextmgr/compact_prompt.go`，保留原有 section 顺序、`<analysis>/<summary>` 输出约定、禁止工具调用规则、analysis 清理和“直接续接任务”的包装语义。首版不翻译、不删减、不另写一套 ZGI 摘要 Prompt，避免两套摘要协议产生行为差异。

使用规则：

1. ZGI 的正常语义压缩是“总结较早前缀、保留近期原文”，因此使用 `getPartialCompactPrompt(..., "up_to")` 对应的 `PARTIAL_COMPACT_UP_TO_PROMPT`。
2. 只有不保留原文尾部的完整压缩路径才使用 `getCompactPrompt` 和 `BASE_COMPACT_PROMPT`。
3. 模型输出先经过 `formatCompactSummary`，移除 `<analysis>` 起草内容，只保留格式化后的 summary。
4. 写回活动上下文时使用 `getCompactUserSummaryMessage` 的续接包装，并设置“近期消息已原文保留”的语义。
5. Claude Code 模板中的 transcript path 不能直接写入 ZGI 本地文件路径；ZGI 有可授权的 audit artifact ref 时替换为该引用，否则省略这个可选字段。除此之外不改变模板正文。
6. 摘要请求不携带工具 Schema；如果模型仍返回 tool call，视为摘要失败，不能把工具调用写入 summary。

摘要输入使用模型实际看到过的投影 transcript，而不是数据库 `Query/Answer`。输入消息按以下顺序提供给上述模板：

```text
previous_summary（如果存在）
+ 要压缩的完整 API rounds
+ Claude Code compact prompt
```

`previous_summary` 按模型上一轮实际看到的 compact summary message 传入，完整 API rounds 使用经过稳定工具结果投影后的模型可见版本。ZGI 不再把这些内容转换成旧的 `Status/User/Assistant` JSON。

Claude Code 模板已经要求摘要覆盖用户目标、后来修正、文件和代码、错误修复、已完成工作、全部用户消息、未完成任务、当前工作和后续继续所需上下文，因此这些内容不再由 ZGI 维护第二份 headings 列表。

最终摘要作为“不可信历史数据”进入上下文，system 和当前用户指令始终优先。模型生成的 `<analysis>` 仅用于本次摘要起草，经过 `formatCompactSummary` 后不得写入 checkpoint、dump 的最终 Messages 或后续模型上下文。

依据：Claude Code 的完整、部分和 prefix compact prompt、no-tools guard、格式化及续接包装均位于 [CC-09]。

### 8.4 压缩提交

只有当以下条件全部满足时才原子提交：

1. 摘要非空且通过结构校验；
2. 覆盖边界是完整 `api_round_seq`；
3. 重建后所有工具配对有效；
4. `FinalPromptTokens <= TargetTokens`，或至少低于 `SoftLimit` 且比压缩前显著下降；
5. checkpoint 持久化成功。

提交后：

```text
Summary = newSummary
ActiveRounds = preservedTail
CompactedThroughRound = boundaryRound
RuntimeState = 原样保留并在请求中重新注入
```

原始 audit transcript 不删除；只是从模型活动上下文投影中移除已覆盖 rounds。

依据：Claude Code 用 compact boundary、summary、messagesToKeep、attachments、hook results 的固定顺序重建上下文，并携带 preserved segment 元数据，见 [CC-10]。

### 8.5 防止压缩递归

摘要请求也要做完整 Token 估算和单条工具结果投影，但必须带 `request_kind=compact` 并禁用再次语义压缩，否则“为了压缩而发起的请求”可能递归触发新的摘要。摘要请求装不下时只走第 10.3 节的 API-round 头部裁剪策略。

依据：Claude Code 通过独立的 compact summary 调用处理摘要，并在该调用自身 prompt-too-long 时进入专门的 round-group 裁剪重试，而不是重新进入主 autocompact，见 [CC-13]。

## 9. 暂停、续接与持久化

### 9.1 checkpoint 时机

以下时机必须保存 `AgentContextState`：

- 每次语义压缩成功后；
- 等待审批、用户输入、客户端动作或治理结果前；
- 进程准备迁移或优雅退出前；
- 可选：每个完成的 API round 后异步保存增量日志。

SSE 断开本身不创建新的上下文，也不触发摘要。客户端重连只重新订阅同一个 `agent_run_id` 的事件；如果运行被挂起，则从最近 checkpoint 恢复。

依据：Claude Code 把工具结果替换决策写入 transcript，并能在 resume 时重建完全一致的替换 state，见 [CC-12]。

### 9.2 checkpoint 内容

```text
agent_run_id
schema_version
model/provider/tokenizer
model_context_window_tokens
configured_agent_window_tokens
effective_agent_window_tokens
summary + compacted_through_api_round
active_rounds + pending_round
content_replacement_records
last_actual_usage + estimator_scale
active plan / approval / user-input / client-action state refs
loaded skills and deferred tool refs
file/artifact refs and hashes
consecutive_compaction_failures
created_at / updated_at
```

恢复 checkpoint 时重新读取当前配置和模型规格，并重新计算有效工作窗口。若新的工作窗口小于 checkpoint 记录值，必须在下一次主模型调用前先执行工具结果治理和压缩；不得沿用旧的较大 PromptBudget。

压缩会吃掉早期消息中携带的运行状态，因此计划、文件、已加载 skill、异步任务等必须是结构化 state，并在压缩后重新注入，不能指望摘要模型完整复述。

依据：Claude Code 压缩后重新注入文件、异步 Agent、计划、plan mode、已调用 skill、延迟工具和 MCP 指令等状态，见 [CC-10]。

## 10. 失败与恢复策略

### 10.1 主动压缩失败

- 请求仍低于 `HardLimit`：记录失败，发送经过工具结果治理后的原请求，不阻断 run。
- 连续失败计数达到 3：本 run 暂停主动语义压缩，避免每轮重复浪费模型调用；轻量工具治理继续执行。
- 最终请求超过本地 `HardLimit` 时，无论主动压缩熔断器是否打开，都必须强制执行一次 final recovery compact。该次恢复可以只保护一个最近 API round，以获得比普通主动压缩更大的回收空间。
- final recovery 成功后清零失败计数并继续 run；仍失败才以 `context_exhausted` 结束当前 `agent_run_id`，同时保留 checkpoint。
- 一次成功压缩后清零计数。

依据：Claude Code 使用 3 次连续失败的 circuit breaker，成功后重置，见 [CC-07]。

### 10.2 本地 HardLimit 或上游返回 prompt-too-long

本地估算已经超过 `HardLimit` 时必须在发送前执行 final recovery compact，不能直接终止而让恢复路径不可达。估算低于 `HardLimit` 的请求仍可能因 tokenizer 误差或供应商隐藏开销被上游判定超限，此时：

1. 标记该次请求未成功，不创建新的 API round。
2. 对相同 canonical state 执行一次 reactive compaction。
3. 重建最终请求并重试一次。
4. 仍超限则终止当前 `agent_run_id`，返回 `context_exhausted`，保留 checkpoint 供人工处理。

不能无限重试，也不能把 conversation 标成永久不可用。

依据：Claude Code 对被暂存的 prompt-too-long 错误执行 reactive compact，构建压缩后消息并回到循环重试；再次失败则让错误显现，见 [CC-11]。

### 10.3 摘要请求自身超限

摘要请求也可能超限。按完整 API round 从最旧端逐步移除摘要输入，每次至少保留一个可总结 round，最多重试 3 次；被移除内容仍保留在 audit transcript，并在错误指标中明确记为 lossy recovery。

依据：Claude Code 的摘要请求遇到 prompt-too-long 时按 API-round group 从头裁剪并重试，见 [CC-13]。

## 11. 调试输出与可观测性

### 11.1 dump 时机

每一次实际调用主模型或摘要模型，都在“所有压缩完成、最终请求已经确定、发送前”生成一个顺序编号文件。只有真正准备发送的 request 才能叫“最终实际请求”。

当前中文分段格式可以保留，但顶部指标必须从本次 `request` 和本次 `ContextDecision` 重新计算，不能复用 run 启动时的 `prepared.contextBudget`。

依据：Claude Code 在每次 query loop 入口都基于当前消息重新计算 threshold；见 [CC-01]、[CC-07]。当前 ZGI 已有每次 Agent 模型请求 callback，可作为新 dump 的接入点，见 [ZGI-08]。

### 11.2 建议头部

```text
AgentRunID:
API Round:
请求类型: main | compact | reactive_compact
模型:
时间:

模型物理上下文窗口:
配置 Agent 工作窗口(k):
有效 Agent 工作窗口 Token:
工作窗口是否按模型物理窗口裁剪:
PromptBudget:
固定请求 Token:
可压缩区 Token:
最终 Prompt Token:
总上下文压力:
SoftLimit:
HardLimit:
TargetTokens:

本次决策: none | tool_projection | microcompact | semantic_compact | reactive_compact
压缩前 Token:
压缩后 Token:
压缩覆盖至 API Round:
保留原文 API Rounds:
工具结果原始 Token:
工具结果投影后 Token:
估算器/校准系数:
```

正文继续打印：系统提示词、历史摘要、保留原文 transcript、运行时补充上下文、当前用户任务、工具完整 Schema、最终实际 `req.Messages`。再增加“最终完整 ChatRequest”JSON；仅打印 Messages 无法确认 tools、tool choice、MaxTokens 和供应商扩展字段。

### 11.3 安全要求

- dump 默认关闭，仅在本地 debug 配置启用。
- 本地调试时设置 `ZGI_AICHAT_CONTEXT_PROMPT_DUMP=true`；关闭或不配置时不创建文件。
- 文件必须位于 gitignore 的 runtime storage 下。
- 对密钥、认证头、cookie 和工具敏感字段做脱敏。
- 生产环境优先保存 Token 指标和 hash，不保存完整 prompt。
- 设置总大小和保留期，避免长任务 dump 占满磁盘。

## 12. 对现有代码的删除与保留

### 12.1 直接删除

1. 删除 `prepareLLMRequestForRun` 中 `contextRequiresCompaction -> compactContextForRun` 的一次性压缩分支；该函数只负责新 run 的初始上下文装配。[ZGI-01]
2. 删除以 `MessageID/CoveredThroughMessageID/SourceTurnCount` 表示压缩边界的旧 snapshot 模型。新边界使用 `agent_run_id + api_round_seq`。[ZGI-04]、[CC-04]
3. 删除 `contextCompactionPreferredTurns` 和按数据库用户轮次逐步扩大 coverage 的算法；它无法处理单 prompt 长 Agent。[ZGI-04]、[CC-04]
4. 删除 `contextCompactionTriggerPressure=10%`、`SafePressure=80%`、`TargetPressure=50%`、`contextRequiresCompaction` 以及 `HistoryPressure` 相关 metadata、日志、指标和 dump 字段，改为完整最终请求的 soft/hard/target Token。[ZGI-03]、[CC-07]
5. 删除只接收 `previous_summary + Status/User/Assistant` 的摘要输入，改为完整、已投影的 API rounds。[ZGI-04]
6. 删除压缩 `running/completed` 作为 SSE 业务阶段的设计，以及 `failed_blocked` 会话级状态和专用 `aichat.context.compaction_unavailable` 阻断语义。压缩可作为普通观测事件，但不能由 SSE 生命周期驱动。[ZGI-10]
7. 删除同一压缩动作连续立即调用模型 3 次的固定 retry 流程，改为失败熔断与 prompt-too-long 的单次 reactive recovery。[CC-07]、[CC-11]
8. 删除“有 tool call 就清空全部 assistant Content”的逻辑；仅过滤 reasoning 和明确标记的 presentation-only 内容。[ZGI-11]、[CC-02]
9. 新 manager 接管后，删除 `request_budget.go` 中仅在超硬限制后才运行的重复旧工具结果压缩 stages，避免两套规则修改同一 transcript。[ZGI-06]、[ZGI-07]
10. 新 checkpoint 能恢复完整 run 后，从 Agent continuation 路径删除依赖关键词猜测的 `recentExecutionContext` 合成文本。它是当前缺少执行 transcript 的补丁，不应与新的 canonical state 并存。[ZGI-09]

### 12.2 保留并迁移

1. 保留模型规格中的物理 `ContextWindow`、`MaxInputTokens` 和输出预留计算；增加有效 `AgentContextWindow`，后续所有预算只能从工作窗口派生。[ZGI-03]、[CC-07]、[CC-14]
2. 保留完整 `ChatRequest` Token estimator，并接入每次调用前的统一 manager。[ZGI-06]
3. 保留 `projectMaterializedFileContent` 这类明确的无损/可引用投影，迁移到 `tool_projection.go` 并记录替换 state。[ZGI-05]
4. 保留 prompt dump 的顺序编号和中文可读分段，但所有指标改为 per-request 计算，并增加完整 ChatRequest。[ZGI-08]
5. 保留原始 Conversation History 作为新 run 的 bootstrap；它不再承担 run 内工具轨迹恢复职责。[ZGI-02]、[CC-02]

## 13. 接入位置

### 13.1 配置加载与工作窗口解析

配置接入位置：

```text
api/config/env_keys.go
  envChatRuntimeAgentContextWindowK = "CHAT_RUNTIME_AGENT_CONTEXT_WINDOW_K"

api/config/types.go
  ChatRuntimeConfig.AgentContextWindowK int

api/config/load.go
  默认值 256
  校验正整数和乘法溢出

api/.env.example
api/.env.docker.example
  注释明确单位为 k，默认值为 256
```

配置加载阶段只校验语法和值域；模型物理窗口在具体请求解析模型规格后才确定，因此每次请求还要执行一次：

```go
configuredTokens := checkedMultiply(config.AgentContextWindowK, 1_000)
agentContextWindow := min(configuredTokens, modelSpec.ContextWindow)
```

必须把 `agentContextWindow` 作为明确字段传给 `ContextManager`，不能让不同模块各自重新读取 env 或各自裁剪。这样主请求预算、摘要请求预算、dump 和指标使用同一个最终值。

### 13.2 `service.prepareLLMRequestForRun`

调整为：

```text
解析模型规格和附件
-> 选取 Conversation History bootstrap
-> 创建或恢复 AgentContextState
-> 创建初始 LLMRequest 模板
```

这里不做语义压缩，不根据 SSE 启动状态计算一次性摘要。

### 13.3 `skillloop.Runner`

在每轮动态 `Tools/ToolChoice/MaxTokens` 已设置后、`runModelToolRound` 调用上游之前执行：

```go
planningReq, decision, err = contextManager.PrepareBeforeModelCall(
    ctx,
    contextState,
    planningReq,
)
```

具体位置应替换当前 `runModelToolRound` 中单独调用 `applyFinalPlanningRequestBudget` 的位置，保证 streaming 和非 streaming 两条上游调用路径共用同一个最终请求。

模型返回后先 `ObserveModelResponse`，工具执行完成后一次性 `AppendToolResults`；全部结果配对完成，才进入下一 round。

依据：当前 ZGI 的最终请求预算入口就在上游调用前，见 [ZGI-06]；Claude Code 也在同一位置执行整套上下文治理，见 [CC-01]。

### 13.4 暂停型工具

遇到 approval、question、governance、client action 或 user input pending 时：

```text
保存 pending round 和未完成 tool calls
-> checkpoint
-> 结束或挂起 transport
-> 收到结果后恢复同一 agent_run_id
-> 为未完成 tool calls 补齐 tool_result
-> 下一次模型调用前重新走 ContextManager
```

不得把恢复后的状态简化为“上一轮最终 answer + 合成 recent execution context”。

## 14. 实施顺序

### 阶段 1：建立正确的观测基线

- 接入 `CHAT_RUNTIME_AGENT_CONTEXT_WINDOW_K`，默认 256，完成配置加载校验和按模型物理窗口裁剪。
- 为每个逻辑 run 引入 `agent_run_id` 和 `api_round_seq`。
- 把 dump 改为从每次最终 ChatRequest 计算总 Token、工具 Token 和压力。
- 建立长任务回放测试，确认当前所有工具结果实际进入下一轮请求。

完成标准：`4.txt` 一类文件中，知识图谱结果出现在 Messages 时，顶部 `FinalPromptTokens` 和工具结果 Token 同步增长。

### 阶段 2：统一 canonical transcript 和工具结果治理

- 引入 `AgentContextState`。
- 将模型响应和全部工具结果按 API round 写入 state。
- 实现单条工具结果外部化和稳定 replacement record。
- 把已有文件内容 projection 迁入统一规则。

完成标准：100 个工具 round 不丢配对；恢复后生成的最终请求与恢复前相同。

### 阶段 3：每次调用前语义压缩

- 实现 soft/hard/target 预算。
- 实现 API-round 边界选择、近期尾部保留和摘要生成。
- 直接移植 Claude Code compact prompt、prefix prompt、格式化和续接包装模板，并用快照测试锁定模板内容。
- 实现 runtime state 重新注入、原子 checkpoint 和失败熔断。
- 实现 prompt-too-long reactive compact。

完成标准：压缩能在同一用户任务执行期间触发，且模型继续完成任务，不需要新 SSE/新用户消息。

### 阶段 4：删除旧设计

- 删除旧 `context_compaction.go`、旧 snapshot metadata、旧错误码和相关测试。
- 删除旧 history-pressure 触发和重复 request-budget 压缩 stages。
- 删除被 checkpoint 替代的 continuation 合成上下文。
- 更新测试、指标和调试文档。

不做双写长期兼容。阶段 3 验证通过后尽快删除旧路径，避免两套压缩逻辑同时生效。

## 15. 测试与验收

### 15.1 单元测试

- 未配置工作窗口时默认值为 256K；配置单位按 `1k=1,000 Token` 转换。
- 非整数、零、负数和乘法溢出的工作窗口配置会使配置加载失败。
- 配置 256K、模型物理窗口 1M 时，有效工作窗口为 256K。
- 配置 256K、模型物理窗口 128K 时，有效工作窗口被裁剪为 128K，并产生 clamped 诊断。
- 工作窗口不足以覆盖主模型输出预留或摘要安全余量时，本次请求明确失败。
- 恢复 checkpoint 时工作窗口从 512K 降为 256K，会在下一次主模型调用前重新预算并压缩。
- PromptBudget、soft/hard limit、TargetTokens 和近期尾部大小全部从有效工作窗口派生。
- prefix 压缩使用 Claude Code `PARTIAL_COMPACT_UP_TO_PROMPT`，完整压缩使用 `BASE_COMPACT_PROMPT`。
- compact prompt 的 no-tools 前后缀、section 顺序、输出格式化和续接包装有完整快照测试，防止无意漂移。
- 摘要输出中的 `<analysis>` 被移除，后续上下文只包含格式化后的 summary。
- 摘要模型返回 tool call 时压缩失败，不执行该工具，也不提交 checkpoint。
- 最终 Token 估算包含 system、tools、全部 messages、知识图谱结果和多模态内容。
- 一个 assistant 响应包含 3 个 tool calls，3 个结果全部归入同一 API round。
- 任意裁剪边界不会产生孤立 tool call/result。
- 最新超大工具结果会触发单条结果 projection，而不是直接越过预算。
- microcompact 保留结果壳、业务 ID 和外部引用。
- 摘要成功后保留尾部 Token 在配置区间内。
- assistant 普通正文保留，reasoning 和 presentation-only 进度不进入 transcript。
- 连续压缩失败 3 次后熔断；一次成功后计数清零。

### 15.2 集成测试

1. 单条用户请求运行至少 100 次模型调用，每轮包含工具调用；运行中至少触发两次压缩并最终正常完成。
2. 大型知识图谱结果进入下一次模型请求，Token 先计入、再被有界投影，dump 显示压缩前后差值。
3. 多工具并行/串行混合执行后，下一次模型请求包含全部结果。
4. 在审批、用户输入和客户端动作处暂停并恢复，`agent_run_id`、summary、tail、工具替换记录保持一致。
5. SSE 中途断线但 Agent 仍运行时，不触发压缩状态重建；重连只恢复事件。
6. 主模型返回 prompt-too-long 后 reactive compact 一次并成功恢复。
7. 摘要请求自身过长时按最旧 API rounds 降级，不产生非法消息序列。
8. 连续主动压缩失败 3 次后，超过本地 HardLimit 的请求仍会执行一次 final recovery；成功则继续，失败则保存 checkpoint 并终止当前 run。
9. HTTP/SSE 返回的 metadata 不包含 `agent_transcript`、完整 `model_invocations` 或压缩 snapshot，且脱敏不修改数据库原值。

### 15.3 必须观测的指标

```text
agent_context_model_window_tokens
agent_context_configured_window_tokens
agent_context_effective_window_tokens
agent_context_window_clamped_total
agent_context_prompt_tokens
agent_context_pressure
agent_context_fixed_tokens
agent_context_compressible_tokens
agent_context_tool_result_tokens_before/after
agent_context_compaction_total{type,result}
agent_context_compaction_tokens_before/after
agent_context_compaction_latency_ms
agent_context_compaction_consecutive_failures
agent_context_reactive_recovery_total
agent_context_checkpoint_latency_ms
agent_context_estimation_error_ratio
```

## 16. 源码依据索引

以下路径以本地参考源码快照根目录 `Claude-Code-main/` 为基准。部分实验功能的实现可能在该快照中被裁剪；本方案只引用可见的调用链和具体实现，不推断缺失代码。

### Claude Code 依据

- **[CC-01] 每次模型调用前治理上下文**：`src/query.ts:L365-L468`。依次取得 compact boundary 后消息、执行工具结果预算、snip、microcompact、context collapse、autocompact。
- **[CC-02] 模型响应和工具结果进入下一轮状态**：`src/query.ts:L1695-L1699`、`L1714-L1727`。下一轮 `messages` 由 `messagesForQuery + assistantMessages + toolResults` 组成。
- **[CC-03] 真实 usage 加新增消息估算**：`src/utils/tokens.ts:L201-L260`。文档明确将该函数作为 context threshold 的 canonical 计算，并处理同响应并行工具调用的交错结果。
- **[CC-04] 按 API round 分组**：`src/services/compact/grouping.ts:L3-L62`。明确说明替代 human-turn grouping，以支持 single-prompt agentic sessions。
- **[CC-05] 单条消息工具结果预算**：`src/query.ts:L369-L394`、`src/utils/toolResultStorage.ts:L750-L908`。大结果在 microcompact 前处理，替换决策在后续轮次稳定复用。
- **[CC-06] 清理旧工具结果**：`src/services/compact/microCompact.ts:L401-L510`。清除较旧正文、保留最近 N 个且至少保留一个。
- **[CC-07] headroom 阈值与失败熔断**：`src/services/compact/autoCompact.ts:L28-L90`、`L225-L238`、`L241-L350`。预留摘要输出和 buffer，按当前 Token 触发，连续 3 次失败后熔断。
- **[CC-08] 保留近期原文与 API 约束**：`src/services/compact/sessionMemoryCompact.ts:L44-L61`、`L188-L313`、`L316-L397`。近期尾部使用 Token/文本数量上下限，并保证 tool pair 和同 response 分片完整。
- **[CC-09] 摘要 Prompt、内容、格式和续接包装**：`src/services/compact/prompt.ts:L19-L303`、`L305-L374`。包含 no-tools guard、完整 compact prompt、prefix/partial compact prompt、摘要格式化和继续任务包装；摘要覆盖目标、文件、错误、用户消息、未完成任务及继续工作所需上下文，并移除 analysis scratchpad。
- **[CC-10] 压缩后重建和状态恢复**：`src/services/compact/compact.ts:L325-L366`、`L517-L642`。固定重建顺序，重新附加文件、异步 Agent、计划、模式、skill 和工具状态，并估算真实压缩后上下文。
- **[CC-11] prompt-too-long 反应式恢复**：`src/query.ts:L1062-L1169`。一次 collapse/compact 后构建新 state 重试，防止无限循环。
- **[CC-12] resume 恢复工具结果替换**：`src/utils/toolResultStorage.ts:L911-L935`、`L938-L987`。替换记录写 transcript，并在恢复时重建 replacement state。
- **[CC-13] 摘要请求超限降级**：`src/services/compact/compact.ts:L235-L290`、`L445-L475`。按 API-round group 丢弃最旧部分后重试摘要。
- **[CC-14] 物理窗口与本地有效窗口分离**：`src/utils/context.ts:L8-L12`、`L35-L97`，默认窗口为 200K，1M 由模型后缀、能力、beta 或实验开启，并允许本地上限覆盖 1M endpoint；`src/services/compact/autoCompact.ts:L32-L48` 允许用 auto-compact window 裁剪压缩计算窗口；1M 独立模型选项见 `src/utils/model/modelOptions.ts:L143-L162`。

### ZGI 当前实现依据

- **[ZGI-01] 仅 run 启动前压缩**：`api/internal/capabilities/chatruntime/service/stream.go:L451-L483`；调用发生在进入执行流程前，见同文件 `L50-L73`。
- **[ZGI-02] 数据库历史仅用户与最终回答**：`api/internal/capabilities/chatruntime/service/context_budget.go:L468-L500`。
- **[ZGI-03] 当前 HistoryPressure 公式**：`api/internal/capabilities/chatruntime/service/context_budget.go:L160-L180`；触发函数见 `L66-L78`。
- **[ZGI-04] 旧摘要按用户轮次且不含工具轨迹**：`api/internal/capabilities/chatruntime/service/context_compaction.go:L36-L75`、`L331-L421`；按固定保留 8 轮扩大 coverage 见 `L439-L457`。
- **[ZGI-05] 当前 loop、round 上限与 assistant/tool 消息追加**：`api/internal/capabilities/chatruntime/skillloop/runner.go:L235-L330`、`L507-L515`、`L524-L697`。
- **[ZGI-06] 最终请求预算仅超限后分阶段替换**：`api/internal/capabilities/chatruntime/skillloop/request_budget.go:L131-L203`；接入点见 `runner.go:L1690-L1703`。
- **[ZGI-07] 旧工具结果替换并保护最新结果**：`api/internal/capabilities/chatruntime/skillloop/request_budget.go:L404-L449`。
- **[ZGI-08] 每次 Agent 模型请求的 dump callback 与旧指标来源**：`api/internal/capabilities/chatruntime/service/skill_loop.go:L77-L103`；`api/internal/capabilities/chatruntime/service/context_prompt_dump.go:L122-L148`。
- **[ZGI-09] 关键词驱动的 recent execution 合成上下文**：`api/internal/capabilities/chatruntime/service/context_budget.go:L530-L662`。
- **[ZGI-10] 旧失败阻断和 SSE progress**：`api/internal/capabilities/chatruntime/service/context_compaction.go:L111-L175`、`L228-L283`、`L505-L513`；专用错误见 `service/error_catalog.go:L8-L30`。
- **[ZGI-11] tool call 响应清空 assistant 正文**：`api/internal/capabilities/chatruntime/skillloop/runner.go:L507-L515`。

## 17. 最终决策摘要

1. 压缩属于 Agent 模型调用循环，不属于 SSE。
2. 模型物理窗口与 Agent 工作窗口分离；工作窗口由 `CHAT_RUNTIME_AGENT_CONTEXT_WINDOW_K` 配置，默认 256K，并按模型物理窗口向下裁剪。
3. 后续所有预算和阈值只基于有效 Agent 工作窗口，不允许因为模型支持 1M 就把日常上下文堆到 1M。
4. 每次实际模型调用前都计算最终完整请求 Token，并决定是否治理或压缩。
5. 工具结果始终进入当前 run 的 transcript；大结果先外部化和有界投影，旧结果再 microcompact。
6. 语义压缩按完整 API round 处理，保留近期原文和工具配对。
7. 上下文状态以 `agent_run_id` 持久化，暂停和重连后继续同一状态。
8. 总上下文压力是唯一压力控制量；原“历史压力”指标及其兼容字段全部删除。
9. 旧的 SSE 开始前摘要、数据库轮次 snapshot、`failed_blocked` 和重复压缩 stages 直接删除，不做长期兼容。
