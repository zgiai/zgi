# Agent 运行时上下文管理

本文说明当前代码中的上下文管理边界，替代早期的设计提案。旧提案可在 Git 历史中查看；其中的本地 checkpoint、跨进程恢复及计划目录不能视为已经实现的接口。

## 职责与目录

上下文管理服务于一次 Agent 运行中的模型请求，不由 SSE 连接决定生命周期。

| 位置 | 职责 |
| --- | --- |
| [contextmgr/manager.go](../api/internal/capabilities/chatruntime/contextmgr/manager.go) | 请求准备、按上下文阈值压缩历史工具结果、语义摘要、工具配对校验和运行内状态 |
| [contextmgr/budget.go](../api/internal/capabilities/chatruntime/contextmgr/budget.go) | 模型物理窗口、80% 触发阈值与摘要输出上限 |
| [contextmgr/types.go](../api/internal/capabilities/chatruntime/contextmgr/types.go) | 配置、预算、决策及摘要调用契约 |
| [service/agent_context_manager.go](../api/internal/capabilities/chatruntime/service/agent_context_manager.go) | 模型规格、摘要调用、存储和诊断依赖的组装 |
| [skillloop/runner.go](../api/internal/capabilities/chatruntime/skillloop/runner.go) | 在模型调用前准备上下文，响应后记录状态与用量 |
| [service/agent_transcript.go](../api/internal/capabilities/chatruntime/service/agent_transcript.go) | 逻辑轮次的模型可见工具轨迹整理 |

没有配置 ContextManager 的 Runner 仍有原来的最终请求预算检查。不能仅凭新组件存在就认为所有入口都已使用同一条路径。

## 请求处理

Runner 的 `prepareContextRequest` 先整理系统消息及供应商兼容的工具定义，再调用 `PrepareBeforeModelCall`：

1. 根据模型物理上下文窗口计算 80% 的请求阈值；窗口缺失时使用 512K。
2. 只有完整请求达到 80% 上下文阈值后，才压缩保护区之外超过 32K token 的历史工具结果；压缩结果限制在 10K token 内，保留开头和结尾，中间内容永久省略。
3. 估算完整请求；达到 80% 时保留最近 4 个完整模型轮次，并把更早上下文交给同一模型压缩，摘要最多 20K Token。
4. 校验工具调用与结果配对，避免发送孤立的工具结果。
5. 将摘要、固定上下文和保留轮次重新组装并估算；仍超过 80% 时减少保留轮次。
6. 仍超限则返回 `ErrContextExhausted`，不发送该超限请求。

模型响应通过 `ObserveModelResponse` 进入运行内状态。本轮工具调用回合的 `ReasoningContent` 保留在运行内请求中，发送前只对需要回传推理的模型（DeepSeek、Kimi）保留，其他模型清除；持久化的轮次轨迹和 checkpoint 不保存推理内容（ADR 0033）。普通 assistant 内容与内部推理内容不可混为一谈。

工具结果压缩按保护轮次 4→3→2 逐步执行；如果仍超过阈值，再进入语义摘要，并从保留最新 4 轮开始按 4→3→2→1 递减。一轮是一次模型响应及其紧随其后的全部工具结果，不能随意截断并行工具调用与结果。当前用户输入始终保留。工具结果压缩后直接回写为新的持久化内容，不再提供读取原始结果的上下文工具。

## 工作窗口

```text
模型物理窗口 = 模型规格中的 context_window，缺失时为 512,000
上下文触发阈值 = 模型物理窗口 × 80%
摘要最大输出 = 20,000 Token
```

模型提供正数 `MaxInputTokens` 时，请求仍受该供应商输入上限限制。Token 估算用于请求准入，不等同于供应商最终报告的计费用量，也不是企业费用上限。

## 状态、原文与恢复边界

- ContextManager 维护当前进程内的消息、摘要、工具内容替换记录和模型轮次状态。
- `agent-context://tool-results/...` 是原文引用，不是公开下载地址；读取仍需经过应用的授权路径。
- `agent_transcript` 逻辑轮次记录与完整运行 checkpoint 是不同概念。已有轨迹整理代码不证明进程退出后可以恢复任意在途工具或审批。
- 跨进程恢复、暂停点恢复、SSE 断连后的执行语义必须分别做集成验收，不能根据文件存在或内存测试推断通过。
- 运行文件、调试输出和工具原文不得提交到仓库。

上下文压缩不授予工具权限，不替代审批，也不决定运行时长、执行轮数或成员额度。恢复功能仍需遵守这些独立边界。

## 摘要调用与观测

服务层通过同一计费应用上下文调用摘要模型，记录请求类型、运行 ID、API round、窗口、估算和压缩决策。摘要请求也会消耗模型用量，不能在费用核对时遗漏。

摘要响应无候选、为空、被长度或内容过滤终止，或返回工具调用时，服务层将其视为失败；不会执行摘要响应中的工具调用。`max_tokens` 只限制摘要输出上限，不保证模型一定生成完整的 20K。

诊断应按每次实际请求核对窗口、最终估算、工具结果压缩次数、最终摘要动作和失败原因。完整 prompt 与工具原文可能包含业务数据；调试输出的启用、访问、脱敏和保留策略应单独审查，不能把调试文件当作公开审计接口。

## 验证

从 `api/` 执行组件测试：

```bash
go test ./internal/capabilities/chatruntime/contextmgr -count=1
```

现有测试覆盖工作窗口裁剪、工具结果投影、并行工具配对、硬限制与恢复、重复压缩和长工具循环。服务与 Runner 的集成测试还应验证实际请求路径、摘要计费、轨迹持久化与公开响应脱敏。

修改 Agent 循环、提示结构或换模型后，用 `api/evals/runtime` 对真实部署跑场景回归，并用 `--baseline` 与改动前的结果对比通过率、模型调用次数、工具错误和缓存命中比例。

这些测试不代替真实供应商的长任务回归、进程故障恢复、账单核对或生产磁盘保留策略验收。修改上下文逻辑时应同时核查这几类边界，而不是只确认摘要文本生成成功。
