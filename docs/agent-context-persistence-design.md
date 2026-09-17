# Agent 上下文持久化设计（v3）

状态：设计定稿，待评审
取代：此前被回滚的「上下文条目表」方案（`chat_runtime_context_entries` + 会话级链尾指针）

---

## 0. 一句话

**不存「当前上下文」。** 只存两类东西：① 不可变的事实（这一轮真实发生过的协议消息）；② 压缩产物（摘要 + 精确边界）。模型看到的上下文是**纯投影**，随时可重算 —— 所以不存在「两份事实源互相对不上」的问题。

被回滚的那版之所以踩了一长串同步问题（删除要回退链尾、老会话要原子导入、重新生成撞唯一键、摘要边界要靠位置反查），根因是**它维护了第二份"上下文"**。本设计不做这件事：上下文不落库，只落"事实"和"算不出来的那部分（LLM 摘要）"。

---

## 1. 分层与职责

| 层 | 载体 | 写入方式 | 丢失后果 |
|---|---|---|---|
| **事实层** | `messages.query` / `answer`（展示与检索投影）、`transcript_entries.payload` | 只追加，永不修改 | 数据丢失，靠备份 |
| **行内派生缓存** | `transcript_entries.preview`（当前生效形态） | 可更新，但必须能从 `payload` + 策略重算 | 重算，无损失 |
| **压缩事实** | `messages.summary` + `messages.summary_through_seq` | 每次摘要写一次 | 退回未压缩（上下文变大） |
| **渲染层（不落库）** | system prompt / 工具集 / 技能 / 记忆注入 / 时间上下文 | 每次请求重新渲染 | — |

> **会话表不加 `context` 列**：上下文是投影，不需要物化。这也顺带消掉了「单行热文档」带来的一整类问题（丢更新、每步重写 MB 级 jsonb、会话列表拖大字段）。

---

## 2. 表结构

### 2.1 `chat_runtime_messages`（既有表，新增 3 列）

```sql
ALTER TABLE chat_runtime_messages
  ADD COLUMN summary                 text   NULL,
  ADD COLUMN summary_through_seq     bigint NULL,
  ADD COLUMN transcript_format_version smallint NOT NULL DEFAULT 1;
```

- `summary_through_seq` = 边界：**最后一条被压缩进这份摘要的 entry 的 `cseq`**（不是「内存里最新那一条」）。
- 语义：`cseq <= summary_through_seq` 的内容全部由 `summary` 代表；`cseq >` 的全部用原文。
- `query` / `answer` 保持原样，**只作展示与检索**；上下文一律从 entry 拼（见 §3）。

### 2.2 `chat_runtime_message_transcript_entries`（新表）

```sql
id               uuid PRIMARY KEY,          -- 应用侧生成（先有 id，才能把 ref 拼进 preview）
message_id       uuid NOT NULL REFERENCES chat_runtime_messages(id) ON DELETE CASCADE,
conversation_id  uuid NOT NULL,             -- 冗余：按会话批量取 + 权限校验
organization_id  uuid NOT NULL,             -- 冗余：取回原文时直接校验归属，不用三段 join
cseq             bigint NOT NULL,           -- 会话内单调递增；摘要边界与投影切片都用它
seq              int    NOT NULL,           -- 轮内序号（人读、调试）
kind             varchar(32) NOT NULL,      -- user / assistant / tool
preview          text   NOT NULL,           -- 当前生效形态（模型看的就是它）；有体积上界
payload          jsonb  NOT NULL,           -- 协议消息原文（含 tool_calls / tool_call_id）
byte_size        int    NOT NULL,           -- 预算计算用，避免为了算长度去读 payload
created_at       timestamptz NOT NULL DEFAULT now()

UNIQUE (message_id, seq)
UNIQUE (conversation_id, cseq)
INDEX  (conversation_id, message_id, cseq)
INDEX  (organization_id, conversation_id)
```

**不变量**

1. `payload` **永不修改**（原始事实）。
2. `preview` 可更新，但**必须能从 `payload` + 裁剪策略重算**（单向：`payload → preview`）。压缩只改这里，不改 `payload`。
3. **只有完整的协议消息才写成 entry**：失败/半成品不写；流式过程中的半截回答只留在 `answer` 列，不进 entry。
4. 用户 query 也写一条 entry（`kind=user`）。理由：边界可能落在某一轮内部，重建时必须能判断「这一轮的 query 是否已被摘要覆盖」；统一进 entry 后，一条 `cseq > 边界` 的切片就能解决，不需要额外判断。
5. 每条 entry 都有 `cseq`；**摘要边界只能落在 entry 上**。
6. 追加、`preview` 更新、摘要写入，都必须在该会话的运行租约之内（见 §7）。
7. 大内容在**写入那一刻**就归档成「entry 自己」：`ref` = entry id，`preview` 里带 ref + 原始长度 + 前 N 字；小内容直接内联。**凡是有可能被预算裁剪的内容，一律带 ref**（否则「裁了找不回」）。

预览文本格式（示例）：

```
[工具结果已归档] ref=tr_01J8Q7K… 原始长度=912,340 字符
预览（前 200 字）：订单 12345：已支付 199 元，2026-09-15 14:22…
```

「怎么取回」的说明放在系统提示词/工具描述里，**不要写进存储的文本**（否则工具改名后历史预览全是废说明）。

### 2.3 明确不建的东西

- ❌ 上下文条目表（`compression_stage` / `compacted_payload` / `parent_entry_id` 链 / 会话级 head 指针 / `operation_key`）。
- ❌ 会话表的 `context` / `context_revision` / `context_run_id`。
- ❌ 单独的归档表与 TTL（原文就在 entry 里，见 §9）。

---

## 3. 上下文投影（模型看到的消息列表）

```
project(conversation, at = 当前 tail):
    msgs = []
    msgs += renderPrologue()                       # system prompt / 工具 / 记忆注入 / 时间：渲染层，不落库
    S = 最近的、summary_through_seq 存在的摘要（在 tail 之前或之上）
    if S != NULL:
        msgs += summaryAsUserMessage(S.summary)    # 与当时发送形态一致：role = user
        msgs += entries(WHERE cseq > S.summary_through_seq ORDER BY cseq ASC)
    else:
        msgs += entries(ORDER BY cseq ASC)
    return msgs
```

规则：

- **只读 `preview`**，绝不读 `payload`（否则每次投影都要把几 MB 原文捞出来）。
- 摘要块用 **user 角色**，放在保留消息之前（复现当时的发送形态）。
- 一步算一次，可在进程内缓存；失效条件：新增 entry 或产生新摘要。
- 协议合法性：窗口起点不能落在一条 `tool` 结果上（它的 assistant tool_call 在上文），也不能切断 assistant 的 tool_call 批次；不满足就向前吸附到最近的合法边界。

---

## 4. 压缩阶梯（每次发给模型之前，按顺序尝试）

| 步骤 | 手段 | 成本 | 有损？ | 落库位置 |
|---|---|---|---|---|
| ① | **工具结果压缩**：大结果 → `ref` + 摘录 | 便宜、确定性 | 无损（原文在 `payload`） | 该 entry 的 `preview` |
| ② | 还超 → **摘要**（LLM） | 贵 | 有损 | `messages.summary` + `summary_through_seq` |
| ③ | 还超 → 摘要时**少保留几条**（边界往后推） | 便宜 | 有损 | 同上 |

**关键点**

1. **先裁剪、后摘要**：裁剪是确定性、可逆的；摘要是 LLM、有损的，必须当最后手段。多数轮次在第 ① 步就解决，根本不会产生摘要。
2. **已经压缩过的工具结果不重复压缩**：策略不变时，`preview` 里已经是压缩后的形态（体积已小），检查时直接跳过。因此本设计**不需要** `preview_policy_version` 之类的字段；将来真要改策略，用一次性回填或读时懒重算处理。
3. **摘要输入** = `[上一份摘要]` + `[cseq 在 (上一边界, 新边界] 的 entry]`，**排除最近的保留窗口**（目标 8 条，实际条数**动态调整**）。

   **保留窗口必须协议合法，不能被截断**：
   - 窗口起点不能是一条 `tool` 结果（它的 `tool_call` 落在窗口外）；
   - 不能把 assistant 的 `tool_call` 批次切成两半（有 call 没 result，或有 result 没 call）；
   - 不满足就**向前吸附**到最近的合法边界 —— 宁可多带，实际可能变成 9、10… 条；
   - **对齐只在「选边界」时做一次，结果记进 `summary_through_seq`**；因此重建时天然合法，不需要再对齐一遍。
4. **保留窗口在边界之后**，所以：
   - 摘要与原文**天然不重叠**；
   - 保留多少条对重建**没有任何影响**（它们本来就属于「边界之后的全部」）；
   - 保留窗口的大小可以按预算动态决定，重建只认边界。
5. **边界写在边界所在的那一行**（可能不是当前行 —— 例如执行到第 90 轮时压缩到第 90 轮第 43 条，就写在第 90 轮那行的 `message` 上）。写入必须在运行租约内。
6. 摘要质量护栏：摘要必须小于被压缩内容；只记录事实与已决定的结论，不写指令式语句。

---

## 5. 重建（从历史轮次开分支 / 崩溃续跑 / 排障）

1. 找**最近的、`summary_through_seq <= 目标点`** 的摘要；
2. 拼 `[摘要(role=user)] + [cseq > 边界的全部 entry]`；
3. 没有摘要 → 直接拼全部 entry（老数据兜底）。

要点：

- **不需要知道保留窗口大小、不需要补拼、不会重叠、不会缺口** —— 边界精确到 entry（`cseq`）。
- **不调用大模型**。
- 老会话没有 entry（transcript 曾存在 `metadata.agent_transcript`）→ 需要一次性回填；回填后从 metadata 移除该键，避免又出现两个来源。完全没有过程数据的老会话，降级为按 `query`/`answer` 重建。

---

## 6. 分支（从某一轮开新会话）

- 复制范围：边界摘要所在行及其后到分支点的 `messages` 行 + 这些消息下的 `entries`；
- 复制时**重铸 entry 主键与 `cseq`**，并**同步重写 `preview` 文本里嵌的 `ref`**（这是 preview 带 ref 的连带代价）；
- 重置运行作用域标识（run lease / active message / 运行状态）；
- 可选：给复制过来的行打 `inherited` 标记，便于展示与排查。

---

## 7. 并发与一致性

- **事实层**：只 INSERT，天然无冲突；幂等靠 `UNIQUE(message_id, seq)` 与 `UNIQUE(conversation_id, cseq)`（崩溃续跑从 `max + 1` 继续）。
- **`preview` 更新 / 摘要写入 / message 状态更新**：全部要在运行租约栅栏内执行；并加**部分唯一索引**保证「一个会话只有一个活跃 run」，让过期执行写不进来。
- **冲突策略分级**：副作用**之前**冲突 → fail-closed 中止（不执行工具）；副作用**之后**冲突 → 记录并继续/标错，**不允许静默丢弃**。

---

## 8. 安全

- `ref` 归属校验链：`entry → message → conversation → organization`，必须属于调用者自己的会话；
- 取回要**截断/分页 + 计入 token 预算**（防模型循环取回把上下文重新撑爆）；
- 可见性：`preview` 可对客户端可见；`payload` 需要展开权限。沿用现有脱敏边界。

---

## 9. 保留策略（取代 TTL）

**不做 TTL**：原文只存一份（就在 entry 里），它是事实，不应该被隐式地悄悄删掉。要界住体积，必须是一个**显式的产品策略**。

粒度必须是**会话级**，不能是 entry 级：

- 当前上下文 = `摘要 + 边界之后的全部 entry`。按 entry 删很难判断「某条老 entry 是否还被某个会话/某个分支需要」，**删错一条就丢记忆，而且很难发现**；
- 整个会话删掉是天然安全的：分支创建时已经把需要的行**复制走**了（§6），所以删父会话不影响任何现存会话 —— **零降级**。

可选做法：

| 做法 | 说明 | 代价 |
|---|---|---|
| **① 整会话删除** ✅ **已定（第一阶段只做这个）** | 触发方式：**用户主动删除会话** → 整会话级联删除（`ON DELETE CASCADE` 一路删 entries） | 零降级 |
| **② 冷归档**（暂不做） | 老会话的 `payload` 搬到对象存储（TOS/S3），entry 行只留 `preview` + 对象 key | 取原文 / 从该会话分支时需先把 payload 拉回来（慢一次，不丢数据） |
| **③ 按 entry 删除**（不做） | 省得最多 | 必须保护「最后一个摘要之后的全部 entry + 所有 summary」，还要处理分支依赖；风险高、收益低 |

**已定：第一阶段只做 ①，并且只由用户主动删除触发；不做按时间/配额的自动清理。**
含义与代价：存储会随会话总量持续增长，没有自动上限。将来要加自动清理时，位置已经留好（一个后台任务 + 配置项），不需要改表结构。

---

## 10. 明确不做（防止又走回去）

- ❌ 不建上下文条目表（链 / head 指针 / operation key / 压缩阶段）。
- ❌ 会话表不加 `context` 列。
- ❌ 压缩不改 `payload`（只改 `preview` 和 `summary`）。
- ❌ 不做内容哈希去重（每个会话存自己那份，换来更严的权限校验）。
- ❌ 不允许删中间轮（只能从某轮开新分支）。

---

## 11. 落地时必须一起补的护栏

| # | 事项 | 为什么 |
|---|---|---|
| 1 | **工具执行状态机**：派发**前** fail-closed 写「已开始」，结束后写结果；状态 `not_started / started / outcome_unknown / completed`；恢复时遇到「已开始但无结果」→ 合成明确的「结果未知」，**绝不重跑有副作用的工具** | 否则会重复扣款/重复删文件 |
| 2 | **悬空 tool_call 的确定性补齐**：给未闭合的调用合成结果并记录，禁止现在的「整批丢掉」 | provider 会拒；丢批是静默丢数据 |
| 3 | **窄查询白名单**：列表/投影路径永不 select `payload` | 否则每次列表都拖 MB 级数据 |
| 4 | 「禁止删中间轮」落到**接口层**：封死 `DeleteSubtreeScoped → RebaseHeadAfterMessageDelete`，并明确「移动叶子 = 遗弃」是官方替代 | 规则只写在文档里，下一个 handler 就会再犯 |
| 5 | 运行作用域 id 只保留一处（不要既有列又有 `metadata.context_run_id`） | 又是两个来源 |
| 6 | **重建/降级要可观测**：沿用 `lossy_recovery_dropped_rounds` 这类计数 | 让「变粗了」看得见 |

---

## 12. 回归测试清单

1. 摘要边界切片**不重不漏**（`cseq > 边界` 的结果与预期逐条一致）；
2. **先裁剪后摘要**的顺序：只裁剪就能过预算时，不产生摘要、不调模型；
3. 已裁剪的 entry 再次进入预算检查时**不被重复压缩**；
4. `preview` 更新后，投影直接使用新的 `preview`（不读 `payload`）；
5. 重建不调用大模型（可用假 provider 断言零调用）；
6. 工具派发后崩溃：恢复合成「结果未知」，**不重复执行**；
7. 悬空 tool_call 被补齐，且生成的请求协议合法（配对校验通过）；
8. 追加幂等：重复 `(message_id, seq)` / `(conversation_id, cseq)` 被拒；
9. **`payload` 不被任何列表/投影查询 select**（SQL 断言 + 性能基线）；
10. 老数据回填后能重建；完全没有过程数据的会话降级正确；
11. 分支复制后 `ref` 重写正确、权限校验通过；
12. 保留策略按**会话**删除后，存量分支不受影响（零降级）；将来若做冷归档，payload 取回后可正常重建；
13. 并发：两个 run 同会话写入，后者被拒（部分唯一索引生效）；
14. 保留窗口起点吸附后不产生孤儿 tool 消息。

---

## 13. 已定决策 / 剩余开放项

**已定**

- ✅ 压缩保留窗口**目标 8 条**，实际条数**动态调整**（`tool_call` / `tool_result` 不可中间截断，见 §4）。纯压缩参数，与重建无关。
- ✅ 保留策略：**只做用户主动删除会话 → 整会话级联删除**；暂不做自动清理、暂不做冷归档（见 §9）。
- ✅ entry 表**冗余 `organization_id`**（取回原文时直接校验归属，不做三段 join，见 §2.2）。
- ✅ **第一个 PR 只做持久化层**（entry 表 + 投影 + 裁剪写 `preview` + 摘要边界 + 重建/分支）；**工具执行状态机与悬空 tool_call 补齐放第二个 PR**（见 §14）。
- ✅ 老会话**不做数据回填**：没有 entry 时读时兜底，按 `query` / `answer` 降级重建（见 §5）。

**剩余开放项（不阻塞开工）**

- 取回工具的名字与描述文案；
- 摘要的长度上限与提示词；
- `transcript_format_version` 的演进规则（当前 1）；
- 分支复制时是否打 `inherited` 标记。

---

## 14. 落地顺序

| 阶段 | 内容 | 说明 |
|---|---|---|
| **①（第一个 PR）** | entry 表 + 迁移 + repository；上下文投影（`摘要 + cseq > 边界`）；工具结果裁剪写回 `preview`；摘要与 `summary_through_seq`；重建与分支复制 | 纯持久化层，不动 agent loop 的执行路径 |
| **②（第二个 PR）** | 工具执行状态机（派发前 fail-closed 写意图）+ 悬空 `tool_call` 确定性补齐 | 唯一「不做就一定出事」的一条（防重复副作用），必须紧跟 |
| **③（收尾）** | 窄查询白名单、禁止删中间轮落到接口层、lossy recovery 计数、老数据兜底打磨 | 护栏与可观测性 |
