# Transcript Entry 日志化改造

状态：已实施

## 目标

将会话上下文统一为一条可追加、可重放的 `transcript_entry` 日志。用户消息、模型消息、工具调用/结果和摘要都作为日志 entry 保存；模型实际看到的上下文由日志投影得到，不再在 `message` 行上维护独立的上下文快照。

## 数据模型调整

### `transcript_entry`

保留：

- `id`
- `message_id`
- `conversation_id`
- `kind`：`user`、`assistant`、`tool`、`summary`
- `payload`：不可变的原始协议消息
- `preview`：可重新计算的模型可见版本

新增：

- `parent_entry_id`：日志链上的上一条 entry
- `covers_through_entry_id`：仅 `kind=summary` 使用，表示摘要覆盖到哪条 entry
- 幂等写入所需的 `event_id` 或等价操作标识

移除：

- `cseq`
- `seq`
- 依赖 cseq 的摘要边界字段和查询接口

### `message`

新增：

- `transcript_head_entry_id`：该 message 当前已经持久化的日志头

移除：

- `summary`
- `summary_through_seq`

`message.parent_id` 继续表示消息分支关系；`transcript_head_entry_id` 只表示该分支当前的日志恢复点。

## 上下文加载

从目标 message 的 `transcript_head_entry_id` 开始沿 `parent_entry_id` 反向读取：

1. 收集当前 head 到最近一个 `kind=summary` 的 entry。
2. 找到摘要后，读取其 `covers_through_entry_id`。
3. 继续沿 parent 链读取，直到到达覆盖边界。
4. 反转收集结果，渲染为“摘要 + 覆盖边界之后的全部原文”。

没有摘要时，一直读取到链根。摘要 entry 只参与上下文投影，不作为普通 user/assistant 消息重复注入。

如果摘要边界不在当前 head 的 parent 链上，或链出现环，应视为数据损坏并拒绝静默恢复。

## 摘要写入

摘要本身也是追加 entry。写入时记录：

- 摘要 entry 的 `parent_entry_id`：生成摘要时的当前 head；
- `covers_through_entry_id`：摘要实际覆盖的最末 entry；
- 生成摘要时的 expected head，用于并发 CAS。

摘要不会修改历史 entry 的 `payload`，也不会覆盖旧摘要。新摘要覆盖旧摘要后，旧摘要自然落在新摘要的覆盖范围内。

## 分支

分支采用复制，不在多个分支之间共享 entry：

- 创建新的 conversation 和 message 链；
- 复制分支点之前可达的全部 entry；
- 重新生成 entry、message ID；
- 同步重写 `parent_entry_id`、`covers_through_entry_id`、`message_id` 和 `conversation_id`；
- 新分支拥有独立的 `transcript_head_entry_id`。

这样删除任一分支只会删除该分支自己的 message 和 entry，不影响其他分支。

## 一致性要求

- entry 只追加，原始 `payload` 永不修改；
- append entry 与更新 message head 在同一事务中完成；
- head 更新使用 expected-head CAS，避免并发运行互相覆盖；
- 同一 `event_id` 重试必须幂等；
- `parent_entry_id`、摘要边界和 head 必须属于同一条可达链；
- 工具调用和结果仍需保留崩溃恢复状态，避免恢复时重复执行有副作用的工具。

## 实施范围

1. 调整迁移和模型，建立 entry parent 链及 message head。
2. 重写 append、恢复、摘要和投影查询，移除 cseq/seq 路径。
3. 改造分支复制逻辑，支持 ID 和 parent 引用重映射。
4. 增加断点续跑、摘要边界、并发追加、分支删除隔离和链损坏检测测试。
5. 不迁移当前分支测试数据；`main` 旧消息继续从 `metadata.agent_transcript` 读取，并在首次续跑时懒接入 parent 链。
