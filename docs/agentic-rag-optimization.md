# Agent 场景 RAG 优化方案

> 代码级实施合同见：`docs/agentic-rag-implementation-spec.md`

本文聚焦知识库接入 Agent 后的 RAG 链路优化，方案基于当前系统实现，并按照以下原则收敛：

- Agent 负责改写查询、拆分问题、判断证据是否充分以及决定是否继续检索。
- 后端使用固定、稳定的 Hybrid 检索链路，不在第一阶段引入“精确、平衡、高召回”等额外策略。
- 不引入 Metadata Filter。租户、权限和知识范围继续在知识库绑定层完成隔离。
- 暂不考虑图谱检索。
- 多知识库并行检索，按照 Reranker 是否一致决定直接聚合还是统一二次重排。
- 子块上下文严格采用 Anthropic Contextual Retrieval 的入库期上下文化方案，不在查询时自行拼接父块摘要。
- 同一父块命中多个子块时，继续直接使用最高子块分数，不引入复杂的父块聚合公式。
- 检索证据只进入模型上下文一次，Agent 的最终答案必须由证据支撑。

## 一、当前系统

### 1. Agent 输入侧

当前 Agent 可以调用知识库检索工具，并传入：

- 查询词 `query`
- 返回数量 `top_k`
- 检索模式

Agent 也会在结果较弱时尝试改写一次查询，但还缺少以下完整能力：

- 将多轮对话中的问题改写成独立、完整的检索问题。
- 判断问题是否需要访问知识库。
- 将复杂问题拆分成多个子问题。
- 针对不同子问题生成检索词。
- 判断当前证据是否覆盖所有子问题。
- 发现相互冲突的证据。
- 在证据不足时继续进行有边界的补充检索。

本次优化在现有 Agent 工具调用机制上扩展这些能力，不新建另一套 Agent 框架。

### 2. 多知识库检索

当一个 Agent 绑定多个知识库时，当前系统会：

1. 取得 Agent 绑定的知识库 ID。
2. 按顺序遍历每个知识库。
3. 使用同一个查询和 `top_k` 分别检索每个知识库。
4. 在每个知识库内部完成向量、BM25、融合和 Reranker。
5. 合并所有知识库返回的结果。
6. 按 `Record.Score` 排序。
7. 截取全局 `top_k`。

因此，目前确实是每个知识库都检索一遍，但知识库之间是串行执行的。

### 3. 父子切块与 Reranker

当前父子切块链路是：

1. 使用子块进行向量或 BM25 召回。
2. Reranker 仅根据子块内容打分。
3. 重排后将子块转换回父块。
4. 同一父块命中多个子块时，父块取最高子块分数。

问题不在“取最高分”本身，而在于子块被打分时缺少它在完整文档中的上下文。一个局部措辞与问题相似、但实际属于无关章节或无关适用范围的子块，可能获得过高分数。

### 4. 证据组装

当前检索内容可能同时通过完整 `context`、`retriever_resources` 和工具消息进入模型上下文，造成：

- 同一证据重复出现。
- 浪费上下文窗口。
- 模型重复关注同一内容。
- 缺少稳定证据编号和子问题覆盖信息。
- 难以判断答案中的结论具体由哪条证据支持。

## 二、本次不做的能力

### 1. 不引入 Metadata Filter

当前租户、权限和知识范围已经在知识库绑定层隔离。让 Agent 自动推断 Metadata 条件可能错误排除本应召回的文档，因此本次不增加：

- Agent 自动生成 Metadata 条件。
- 向量和 BM25 的 Metadata 预过滤。
- Metadata Filter 的工具参数。

文档标题、章节等信息仍可用于展示和证据定位，但不用于缩小检索范围。

如果将来出现同一知识库内需要按明确版本、日期或文档类型过滤的业务需求，应单独设计，并要求过滤条件来自用户明确表达，不能由 Agent 随意推断。

### 2. 不考虑图谱检索

本次只处理：

```text
向量召回 + BM25 召回 → 融合 → Reranker
```

即使代码中存在图谱相关入口，也不把图谱作为当前 Agent 可选择的检索能力。

### 3. 不引入三档检索策略

第一阶段不增加“精确检索、平衡检索、高召回检索”等参数预设。后端继续使用统一的 Hybrid 检索配置，避免在没有评测依据时增加新的参数组合。

### 4. 不设计复杂父块分数

不增加父块整体相关性、多子块密度、加权平均等聚合公式。完成 Contextual Retrieval 后，同一父块仍然直接使用最高子块 Reranker 分数。

## 三、目标架构

```text
用户问题
  ↓
Agent 判断是否需要检索
  ↓
生成独立查询，必要时拆分子问题
  ↓
并行检索 Agent 绑定的全部知识库
  ↓
每个知识库执行 Contextual Embedding + Contextual BM25
  ↓
每个知识库内部融合并重排
  ↓
判断所有知识库是否使用同一个 Reranker
  ├─ 是：直接合并并按 Reranker 分数排序
  └─ 否：合并候选后使用统一 Reranker 再重排一次
  ↓
子块映射回父块，同一父块取最高子块分数
  ↓
去重并组装证据
  ↓
Agent 判断覆盖度、充分性和冲突
  ↓
证据不足且预算允许时继续检索
  ↓
基于证据回答，证据不足时明确说明
```

## 四、受约束的 Agentic RAG

### 1. Agent 负责的决策

Agent 可以决定：

- 当前问题是否需要检索。
- 如何将上下文问题改写为独立查询。
- 是否需要拆分子问题。
- 每个子问题使用什么检索词。
- 当前证据是否足以支撑回答。
- 是否存在仍未覆盖的子问题。
- 是否存在相互冲突的证据。
- 是否需要继续一轮补充检索。

Agent 不直接决定以下底层参数：

- 向量和 BM25 的权重。
- 单路召回候选数。
- Reranker 阈值。
- 每个知识库的并发数。
- 单次 Reranker 最大候选数。
- 总 Token、总耗时和总模型调用次数。

这些参数由后端统一配置并通过评测调整。

### 2. 多轮检索边界

建议默认限制：

- 最大检索轮数为 3。
- 一次回答最多处理 4 个不同子问题。
- 最多调用知识库工具 6 次。
- 检测重复或高度相似的查询。
- 限制总候选数、Reranker 调用次数、Token 和请求耗时。
- 相互独立的子问题允许并行检索。
- 有依赖关系的子问题按顺序检索。
- 达到证据充分条件后立即停止。

检索服务只返回执行状态 `success`、`no_results` 或 `no_config`，以及当前证据组。`sufficient`、`partial`、`insufficient`、子问题覆盖度、冲突和下一轮查询方向由 Agent 根据原始证据判断，不在后端增加额外 LLM Judge。

### 3. 证据充分性判断

第一层使用确定性条件：

- 是否存在达到要求的候选。
- 每个子问题是否至少有一条直接支持证据。
- 是否存在明显冲突。
- 是否只有重复证据，没有新增覆盖。
- 当前证据是否能够直接回答问题。

只有在复杂、多跳、冲突或边界场景中，才使用额外模型判断：

- 证据能否支撑最终结论。
- 哪些子问题仍未覆盖。
- 下一轮应该检索什么。

不为每一次普通检索固定增加一次 LLM 判断。

## 五、多知识库并行检索与结果聚合

### 1. 并行执行

Agent 绑定多个知识库时，所有知识库使用同一个查询并行检索：

```text
                 ┌→ 知识库 A 检索 ─┐
Query ───────────┼→ 知识库 B 检索 ─┼→ 聚合
                 └→ 知识库 C 检索 ─┘
```

实现时使用有上限的并发，不能为所有知识库无限创建任务：

- 默认最大并发数建议为 4。
- 实际并发数为 `min(知识库数量, 最大并发数)`。
- 使用请求 Context 传递取消和超时。
- 第一阶段保持当前整体语义：任一知识库检索失败，请求返回错误并取消其他未完成任务。
- 后续如需支持部分成功，应显式返回失败的知识库和 Warning，不能静默忽略。

单个知识库内部仍然可以并行执行向量与 BM25 召回。

### 2. Reranker 一致性判断

不能只比较配置中的模型名称，应比较已经解析完成的 Reranker 指纹。

建议指纹至少包括：

```text
provider
model
resolved_model_version
rerank_mode
input_template_version
```

如果影响分数的截断长度或模型参数可以配置，也应加入指纹。

只有满足以下条件时，多个知识库的分数才直接比较：

- 使用同一 Provider。
- 使用同一 Reranker 模型和解析后的版本。
- 使用同一种 Reranker 模式。
- 使用同一种 Contextual Chunk 输入格式。
- 使用相同的截断规则。
- 没有知识库因 Reranker 失败而降级到原始召回分数。

### 3. 使用同一个 Reranker

如果所有知识库的 Reranker 指纹一致：

1. 每个知识库并行完成召回、融合和 Reranker。
2. 每个知识库返回自己的候选结果。
3. 合并所有候选。
4. 直接按 Reranker 分数从高到低排序。
5. 子块映射回父块。
6. 同一父块取最高子块分数。
7. 截取全局 `top_k`。

这种情况下不再执行第二次 Reranker，避免重复成本和延迟。

### 4. 使用不同的 Reranker

如果任一知识库的 Reranker 指纹不同，或者有知识库发生 Reranker 降级：

1. 每个知识库并行完成本地检索。
2. 每个知识库保留一定数量的候选，不立即按不同模型的阈值进行全局淘汰。
3. 合并所有知识库候选并去重。
4. 使用 Agent 级统一 Reranker 对合并候选再重排一次。
5. 使用统一 Reranker 的分数进行最终排序。
6. 子块映射回父块并取最高子块分数。
7. 截取全局 `top_k`。

统一 Reranker 的选择顺序建议为：

1. Agent 明确配置的 Reranker。
2. 工作空间默认 Reranker。
3. 平台默认 Reranker。

如果无法解析出统一 Reranker，应返回明确错误，不能直接比较来自不同模型或不同降级路径的分数。

### 5. 候选数量

假设 Agent 绑定 `N` 个知识库、最终需要 `top_k` 条结果：

- 每个知识库至少保留 `top_k` 条候选。
- 合并后的最大候选数约为 `N × top_k`。
- 如果候选数超过统一 Reranker 的单次限制，按固定批次执行 Reranker。
- 所有批次必须使用相同模型、相同输入格式和相同参数。

不在每个知识库内过早只保留 1～2 条结果，否则不同 Reranker 场景下，统一二次重排无法找回已被丢弃的候选。

## 六、严格采用 Anthropic Contextual Retrieval

本方案按照 [Anthropic Contextual Retrieval](https://www.anthropic.com/engineering/contextual-retrieval) 的核心流程实现，不将其简化为“查询时给子块拼接父块摘要”。

### 1. 核心定义

Contextual Retrieval 是入库期预处理技术，由两部分组成：

- Contextual Embeddings。
- Contextual BM25。

对每一个原始子块，使用完整文档和当前子块生成一段专属于该子块的简短上下文。该上下文通常为 50～100 Tokens，并永久前置到原始子块：

```text
contextualized_chunk = contextual_context + original_chunk
```

随后：

- 使用 `contextualized_chunk` 生成向量。
- 使用 `contextualized_chunk` 建立 BM25 索引。
- 检索后使用 `contextualized_chunk` 进行 Reranker。

因此，上下文同时改善召回和重排，而不是只在 Reranker 前临时补充信息。

### 2. 入库流程

```text
完整文档
  ↓
按现有父子切块规则生成父块和子块
  ↓
针对每个子块，将“完整文档 + 当前子块”交给 Contextualizer
  ↓
生成 50～100 Token 的 chunk-specific context
  ↓
contextualized_chunk = context + child chunk
  ↓
使用 contextualized_chunk 生成 Embedding
  ↓
使用 contextualized_chunk 建立 BM25 索引
  ↓
保存原始子块、上下文和索引版本
```

Contextualizer 的提示词保持 Anthropic 的结构：

```text
<document>
{{WHOLE_DOCUMENT}}
</document>

<chunk>
{{CHUNK_CONTENT}}
</chunk>

结合完整文档，为该子块生成一段简短、明确、仅用于改善检索定位的上下文。
只输出上下文本身，不输出解释或其他内容。
```

这里使用的是完整文档，不是只使用父块，也不是直接拼接文档标题、部门、版本等人工字段。

### 3. Context 长度

Anthropic 的方案中，生成的 chunk-specific context 通常控制在 50～100 Tokens。

本系统第一版采用：

- 目标长度：50～100 Tokens。
- 最大长度：100 Tokens。
- 少于 50 Tokens 但已经能明确定位子块时，不强制补足。
- 超出 100 Tokens 时使用 Contextualizer 对内容重新压缩，不直接做字节截断。

原始子块继续使用现有切块大小。写入 Embedding、BM25 和 Reranker 前，应校验：

```text
context tokens + child tokens + model special tokens
    ≤ 对应模型的最大输入长度
```

如果超过限制，应调整入库切块，而不是在查询时随意截掉上下文或原始子块。

### 4. 存储结构

原始子块继续保存在现有结构中；模型生成内容按 Contextual Index Generation 独立保存，至少包含：

```json
{
  "original_content": "原始子块",
  "contextual_context": "模型生成的 50～100 Token 上下文",
  "contextualized_content": "上下文 + 原始子块",
  "contextualizer_model": "实际使用的模型",
  "contextualizer_prompt_version": "v1",
  "index_generation_id": "uuid",
  "contextualized_at": "时间"
}
```

用途分别为：

- `original_content`：最终证据展示和原文引用。
- `contextual_context`：解释子块在完整文档中的位置和语义。
- `contextualized_content`：Embedding、BM25 和 Reranker 输入。
- 版本字段：支持重建索引、灰度和回滚。

不要只保存拼接后的文本，也不要把不同代次的 Context 覆盖写到原始子块上，否则后续无法区分模型生成内容、原始证据和索引版本，也无法安全回滚。

### 5. 查询流程

查询时不再调用模型生成父块摘要：

1. 对 Query 生成向量。
2. 在 Contextual Embedding 索引中召回。
3. 在 Contextual BM25 索引中召回。
4. 融合并去重两路候选。
5. 使用 `contextualized_content` 进行 Reranker。
6. 将子块映射回父块。
7. 同一父块使用最高子块 Reranker 分数。
8. 最终证据中区分 `contextual_context` 和 `original_content`。

Reranker 不再使用自定义的：

```text
Document + Metadata + Section + Matched Child + Parent Context
```

而是统一使用：

```text
Contextual Context + Original Child
```

这样所有知识库使用一致的输入格式，也便于比较 Reranker 分数。

### 6. 重新索引

Contextual Retrieval 会改变 Embedding 和 BM25 的索引内容，因此不能只修改查询代码。

需要：

- 为新文档默认执行 Contextual Retrieval 入库流程。
- 为已有知识库提供后台重新索引任务。
- 使用版本化索引，避免重建期间影响线上查询。
- 新索引完整构建并验证后，再切换读取版本。
- 重建失败时继续使用旧索引。
- 记录每个文档和子块的上下文化状态。
- 支持失败重试和断点续建。

如果完整文档超过 Contextualizer 的上下文窗口，不应静默改成“只输入父块”并仍标记为 Anthropic Contextual Retrieval。应选择支持该文档长度的模型，或在入库阶段明确将文档拆分为可独立理解的逻辑文档后再进行 Contextual Retrieval。

## 七、父块聚合

完成 Contextual Retrieval 后，同一父块命中多个子块时：

```text
parent_score = max(child_contextual_rerank_score)
```

具体流程：

1. 子块使用 `contextualized_content` 参与召回和 Reranker。
2. 根据子块的 `segment_id` 找到父块。
3. 同一父块的多个命中子块去重。
4. 父块直接使用最高子块分数。
5. 保留获得最高分的子块作为主要命中证据。
6. 其他命中子块可以作为补充证据返回，但不参与复杂加权。

不增加父块平均分、支持密度或多子块奖励。

## 八、证据组装与答案生成

### 1. 证据只进入模型上下文一次

统一生成模型可见的 `evidence_groups`，避免同时重复传递完整 Context、Retriever Resources 和工具消息。

面向前端展示的资源信息可以保留，但不再次以完整文本进入模型上下文。

每条证据包含：

```json
{
  "evidence_id": "E1",
  "source": "人事制度库 / 员工管理制度",
  "score": 0.91,
  "contextual_context": "该块说明2026年正式员工的年假适用范围。",
  "content": "员工入职满一年后可申请年假。"
}
```

`contextual_context` 是模型生成的检索定位信息，`content` 才是可以直接引用的原始证据。最终答案不能把 Contextualizer 生成的内容当作原文引用。

### 2. 证据选择

证据组装时：

- 按子问题分配上下文预算。
- 优先保留直接支持结论的原文。
- 对同一父块和近似内容去重。
- 保证不同子问题的覆盖。
- 保留相互冲突的证据。
- 将最强证据放在上下文前部。
- 按 Token 控制长度。
- 使用 UTF-8 安全的截断方式。

### 3. 答案生成约束

Agent 的知识库回答应遵守：

- 与内部知识库有关的事实只能来自给定证据。
- 每个关键事实使用 `[E1]` 形式引用证据。
- 引用和精确措辞只能来自 `content`。
- 明确区分证据事实和基于事实作出的推断。
- 证据不足时说明缺少什么信息。
- 证据冲突时展示冲突及各自来源。
- 不使用模型记忆补全知识库中没有出现的内部事实。

## 九、工具返回结构

现有请求参数保持兼容，不增加 Metadata Filter 和图谱检索参数。

检索工具响应可以扩展为：

```json
{
  "query": "standalone query",
  "status": "success",
  "result_count": 1,
  "evidence_groups": [
    {
      "evidence_id": "E1",
      "source": "人事制度库 / 员工手册",
      "score": 0.91,
      "contextual_context": "该块说明正式员工年假的适用条件。",
      "content": "员工入职满一年后可申请年假。"
    }
  ],
  "diagnostics": {
    "knowledge_base_count": 3,
    "reranker_consistent": true,
    "global_rerank_applied": false,
    "warnings": []
  }
}
```

检索服务不返回 Coverage、充分性结论或 Suggested Query；这些属于 Agent 的判断过程。

## 十、实施顺序

### 第一阶段：建立基线并修正证据链路

- 建立固定评测集。
- 补充检索链路追踪。
- 去除进入模型上下文的重复证据。
- 改为 Token 级、UTF-8 安全的上下文限制。
- 明确分开保存原始子块和按 Index Generation 隔离的模型生成上下文。

### 第二阶段：实现 Contextual Retrieval

- 增加 Contextualizer 入库步骤。
- 使用完整文档和当前子块生成 50～100 Token 上下文。
- 按 Index Generation 保存 `contextual_context` 和 `contextualized_content`。
- 使用 `contextualized_content` 构建 Embedding。
- 使用 `contextualized_content` 构建 BM25。
- Reranker 统一使用 `contextualized_content`。
- 新增版本化索引和旧数据重新索引任务。
- 父块继续使用最高子块分数。

### 第三阶段：多知识库并行检索

- 将知识库串行循环改成有上限的并行执行。
- 保持权限校验和请求取消语义。
- 收集每个知识库实际解析后的 Reranker 指纹。
- 相同指纹时直接比较分数。
- 不同指纹或发生降级时，使用统一 Reranker 二次重排。
- 增加相同和不同 Reranker 的聚合测试。

### 第四阶段：受约束的 Agentic RAG

- 支持独立问题改写。
- 支持子问题拆分。
- 增加覆盖度、充分性和冲突状态。
- 增加多轮检索预算和停止条件。
- 支持独立子问题的有限并行检索。

### 第五阶段：证据包和答案生成

- 统一输出 `evidence_groups`。
- 分离检索定位上下文和原始证据。
- 分离模型可见证据和前端资源信息。
- 增加引用、冲突说明和证据不足时拒答的提示词。

## 十一、评测方案

### 1. 测试集

至少覆盖：

- 简单事实问题。
- 口语化、缩写和短查询。
- 多轮对话中的指代问题。
- 需要拆分的多跳问题。
- 子块缺少主语、时间、对象或章节语境的问题。
- 子块局部相似但完整文档语境无关的问题。
- 跨多个知识库的问题。
- 多知识库使用相同 Reranker 的问题。
- 多知识库使用不同 Reranker 的问题。
- Reranker 失败并降级的问题。
- 不同版本文档相互冲突的问题。
- 知识库中不存在答案的问题。
- 表格和结构化文档问题。

### 2. 指标

#### Agent 决策

- 不必要检索率。
- 子问题覆盖率。
- 平均检索轮数。
- 重复查询和循环率。

#### Contextual Retrieval

- Child Recall@K。
- Parent Recall@K。
- BM25 Recall@K。
- 向量 Recall@K。
- 融合后的 MRR 和 nDCG。
- 上下文化前后的召回失败率变化。
- Contextualizer 失败率。
- 平均 Context Token 数。
- 入库耗时和模型成本。

#### 多知识库聚合

- 串行与并行的 P50、P95 延迟。
- 不同知识库数量下的吞吐量。
- 相同 Reranker 时的全局排序准确率。
- 不同 Reranker 经统一二次重排后的排序准确率。
- Reranker 一致性判断准确率。
- 全局二次重排触发率。
- 单次请求 Reranker 调用次数和成本。

#### 重排和父块

- Parent nDCG。
- 子块语境缺失场景中的排序纠正率。
- 父块最高子块分数的稳定性。
- 重复父块比例。

#### 最终答案

- Answer Correctness。
- Faithfulness。
- Citation Precision。
- Citation Recall。
- 无答案场景的拒答准确率。

具体目标值在完成现有链路基线后确定。

## 十二、可观测性

每次 Agent RAG 请求建议记录：

- 请求 ID。
- 原始问题和独立查询。
- 子问题。
- 绑定知识库数量。
- 实际并发数。
- 每个知识库的开始、结束、耗时和错误。
- 每个知识库的向量、BM25 候选数。
- 每个知识库解析后的 Reranker 指纹。
- 是否满足 Reranker 一致性条件。
- 是否触发统一二次重排及触发原因。
- 子块使用的 Contextual Index 版本。
- Reranker 输入使用的 Prompt/Input Template 版本。
- 子块到父块的映射。
- 父块最终采用的最高子块分数。
- 已覆盖和未覆盖的子问题。
- 继续或停止检索的原因。
- 被选中和被丢弃的证据及原因。
- 最终答案引用的证据编号。
- 各阶段耗时、Token 和模型成本。

日志默认只记录 ID、版本、摘要和统计信息，避免保存完整敏感文档内容。

## 十三、风险与控制

### 1. 多知识库并发造成资源压力

使用固定并发上限、请求级超时和取消机制。并发上限通过压测调整，不随知识库数量无限增长。

### 2. Reranker 名称相同但实际版本不同

比较解析后的 Reranker 指纹，而不是只比较用户配置中的模型名称。

### 3. 不同 Reranker 的候选被过早淘汰

不同指纹场景为每个知识库保留足够候选，再使用统一 Reranker 做最终排序。

### 4. Contextualizer 生成错误信息

始终分别保存生成上下文和原始子块。生成上下文用于检索定位，最终引用必须来自原始子块。通过抽样评测、版本管理和失败重试控制质量。

### 5. Contextual Retrieval 入库成本增加

上下文化是一次性入库成本。使用批处理、文档级缓存能力、后台任务和断点续建降低成本，不在每次查询时重复生成。

### 6. 重新索引影响线上查询

使用版本化索引。旧索引继续提供服务，新索引完成并验证后再切换。

### 7. Agent 过度检索

通过最大轮数、最大子问题数、重复查询检测、总 Token 和总耗时限制控制。

## 十四、最终优先级

建议按以下顺序推进：

1. 建立评测基线，去除重复证据并完善追踪。
2. 严格实现 Contextual Embeddings 和 Contextual BM25。
3. 使用 Contextualized Chunk 进行 Reranker，父块继续取最高子块分数。
4. 将多知识库检索改为有上限的并行执行。
5. 增加 Reranker 指纹比较和条件式统一二次重排。
6. 增加 Agent 查询改写、子问题拆分、充分性判断和有限多轮检索。
7. 完善证据组装、引用、冲突说明和证据不足时的回答策略。

本次最核心的两个改造是：

- 将查询期临时补父块语境改为 Anthropic Contextual Retrieval 的入库期上下文化索引。
- 将多知识库串行检索改为并行检索，并依据 Reranker 一致性选择直接聚合或统一二次重排。
