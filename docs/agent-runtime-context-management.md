# Agent 运行时上下文管理

本文说明当前代码中的上下文管理边界，替代早期的设计提案。旧提案可在 Git 历史中查看；其中的本地 checkpoint、跨进程恢复及计划目录不能视为已经实现的接口。

## 职责与目录

上下文管理服务于一次 Agent 运行中的模型请求，不由 SSE 连接决定生命周期。

| 位置 | 职责 |
| --- | --- |
| [contextmgr/manager.go](../api/internal/capabilities/chatruntime/contextmgr/manager.go) | 请求准备、工具结果投影、压缩、工具配对校验和运行内状态 |
| [contextmgr/budget.go](../api/internal/capabilities/chatruntime/contextmgr/budget.go) | 工作窗口、输入输出预留、软限制、硬限制与压缩目标 |
| [contextmgr/types.go](../api/internal/capabilities/chatruntime/contextmgr/types.go) | 配置、预算、决策及摘要调用契约 |
| [contextmgr/file_store.go](../api/internal/capabilities/chatruntime/contextmgr/file_store.go) | 大工具结果原文的文件存储，不是运行 checkpoint |
| [contextmgr/memory_store.go](../api/internal/capabilities/chatruntime/contextmgr/memory_store.go) | 工具结果的内存存储，供隔离测试使用 |
| [service/agent_context_manager.go](../api/internal/capabilities/chatruntime/service/agent_context_manager.go) | 模型规格、摘要调用、存储和诊断依赖的组装 |
| [skillloop/runner.go](../api/internal/capabilities/chatruntime/skillloop/runner.go) | 在模型调用前准备上下文，响应后记录状态与用量 |
| [service/agent_transcript.go](../api/internal/capabilities/chatruntime/service/agent_transcript.go) | 逻辑轮次的模型可见工具轨迹整理 |

没有配置 ContextManager 的 Runner 仍有原来的最终请求预算检查。不能仅凭新组件存在就认为所有入口都已使用同一条路径。

## 请求处理

Runner 的 `prepareContextRequest` 先整理系统消息及供应商兼容的工具定义，再调用 `PrepareBeforeModelCall`：

1. 根据当前请求的输出预留和模型规格计算预算。
2. 将过大的工具结果投影为有限内容与原文引用。
3. 估算完整请求；达到软限制时先清理旧工具结果，再尝试语义压缩。
4. 校验工具调用与结果配对，避免发送孤立的工具结果。
5. 超过硬限制时尝试缩小待处理工具批次预览和最终恢复。
6. 重新估算；仍超限则返回 `ErrContextExhausted`，不发送该超限请求。

模型响应通过 `ObserveModelResponse` 进入运行内状态，同时清除 `ReasoningContent`。普通 assistant 内容与内部推理内容不可混为一谈。

压缩按完整 API round 选择边界，不能随意截断并行工具调用与结果。摘要失败不是整段会话永久不可用的标记：请求仍在硬限制内时可以继续；超限后的恢复仍失败则返回错误。上游提示输入过长时还有单次主请求重试路径，不能无限递归压缩。

## 工作窗口

`CHAT_RUNTIME_AGENT_CONTEXT_WINDOW_K` 默认值为 `256`，单位是 1,000 Token，不是 1,024。

```text
有效工作窗口 = min(配置值 × 1,000, 模型物理上下文窗口)
输入预算 = 有效工作窗口 - 本次主模型输出预留
```

模型提供正数 `MaxInputTokens` 时，输入预算还受其限制；输出预留受模型输出上限约束。模型窗口缺失或无法容纳输入、摘要及安全余量时，预算计算会失败，不能猜测一个窗口继续。

软限制用于提前治理上下文，硬限制用于拒绝仍然过大的请求，压缩目标用于避免连续触发。具体默认值和计算规则以 `budget.go` 为准。Token 估算用于请求准入，不等同于供应商最终报告的计费用量，也不是企业费用上限。

## 状态、原文与恢复边界

- ContextManager 维护当前进程内的消息、摘要、工具内容替换记录和 API round 状态。
- FileStore 只保存大工具结果原文。目录为运行时 storage 下的 `agent-context/tool-results`；写入采用临时文件和重命名，目录权限 0700、文件权限 0600。
- `agent-context://tool-results/...` 是原文引用，不是公开下载地址；读取仍需经过应用的授权路径。
- `agent_transcript` 逻辑轮次记录与完整运行 checkpoint 是不同概念。已有轨迹整理代码不证明进程退出后可以恢复任意在途工具或审批。
- 跨进程恢复、暂停点恢复、SSE 断连后的执行语义必须分别做集成验收，不能根据文件存在或内存测试推断通过。
- 运行文件、调试输出和工具原文不得提交到仓库。

上下文压缩不授予工具权限，不替代审批，也不决定运行时长、执行轮数或成员额度。恢复功能仍需遵守这些独立边界。

## 摘要调用与观测

服务层通过同一计费应用上下文调用摘要模型，记录请求类型、运行 ID、API round、窗口、估算和压缩决策。摘要请求也会消耗模型用量，不能在费用核对时遗漏。

摘要响应无候选、为空、被长度或内容过滤终止，或返回工具调用时，服务层将其视为失败；不会执行摘要响应中的工具调用。

诊断应按每次实际请求核对窗口、最终估算、工具投影和失败原因。完整 prompt 与工具原文可能包含业务数据；调试输出的启用、访问、脱敏和保留策略应单独审查，不能把调试文件当作公开审计接口。

## 验证

从 `api/` 执行组件测试：

```bash
go test ./internal/capabilities/chatruntime/contextmgr -count=1
```

现有测试覆盖工作窗口裁剪、工具结果投影、并行工具配对、硬限制与恢复、重复压缩和长工具循环。服务与 Runner 的集成测试还应验证实际请求路径、摘要计费、轨迹持久化与公开响应脱敏。

这些测试不代替真实供应商的长任务回归、进程故障恢复、账单核对或生产磁盘保留策略验收。修改上下文逻辑时应同时核查这几类边界，而不是只确认摘要文本生成成功。
