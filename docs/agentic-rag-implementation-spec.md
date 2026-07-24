# Agentic RAG 实施规格

> 版本：2026-07-24 v1  
> 状态：Implementation Ready  
> 上游方案：`docs/agentic-rag-optimization.md`  
> 适用分支：`yzh/feat/agentic-rag-optimization`

## 一、文档目的

本文将 `docs/agentic-rag-optimization.md` 中的架构方案收敛为可编码、可测试、可灰度和可回滚的实施合同。

本文负责明确：

- 本次改造的准确范围与非目标。
- 当前代码到目标实现的映射。
- Contextual Retrieval 的数据、模型、Prompt、状态机和索引合同。
- 多知识库并行检索与条件式统一重排算法。
- Agent 工具、证据包和多轮检索行为。
- 错误、超时、降级、可观测性和安全要求。
- 数据迁移、重新索引、灰度、回滚和验收门槛。
- 分阶段任务、修改文件和完成条件。

如上游架构文档中的示意字段或流程与本文冲突，以本文的代码级合同为准；架构目标不变。

## 二、宪章与仓库约束

### 1. 宪章检查

| 检查项 | 结论 |
|---|---|
| Spec Kit 过程文档使用简体中文 | 通过 |
| 新增业务源码、标识符、注释、日志和错误码使用英文 | 实施阶段强制 |
| 用户可见文案通过现有国际化机制提供 | 实施阶段强制 |
| 不硬编码模型 Provider | 强制 |
| LLM 调用通过现有 Gateway、模型解析、配额和计费链路 | 强制 |

### 2. 仓库约束

- 后端改动遵守 `api/AGENTS.md`。
- Handler 只负责输入解析、授权、调用 Service 和映射响应。
- 业务逻辑放在 `api/internal/modules/dataset`、`api/internal/modules/tools` 和 `api/internal/modules/skills` 的现有边界中。
- 使用依赖注入，不增加包级可变全局状态。
- 请求范围内的数据库、模型、向量库和并发任务必须使用传入的 `context.Context`。
- 数据库变更通过 `api/internal/migrations` 的追加式 Migration 完成，不修改历史 Migration。
- 不提交运行数据、模型输出、索引快照或本地评测结果。

## 三、范围

### 1. 本次必须实现

1. 内部知识库 `hierarchical_model` 的 Anthropic Contextual Retrieval。
2. 每个子块生成 50～100 Token 的 chunk-specific context。
3. Contextualized Chunk 同时用于 Embedding、BM25 和 Reranker。
4. 原始子块与生成上下文分开保存。
5. Contextual Index Generation、重新索引、切换和回滚。
6. Agent 绑定的多个知识库使用有上限的并行检索。
7. 相同 Reranker 指纹时直接比较分数。
8. Reranker 指纹不同、无法确认或发生降级时，使用统一 Reranker 二次重排。
9. 在子块候选层完成跨知识库聚合，最后再转换父块。
10. 同一父块最终分数继续取最高子块 Reranker 分数。
11. Agent 工具固定使用向量与 BM25 Hybrid 检索，不暴露图谱检索。
12. Agent 使用查询改写、子问题拆分、证据检查和有限重试。
13. 模型可见证据与前端 Retriever Resources 分离，消除重复内容。
14. Token 级证据预算和 UTF-8 安全截断。
15. 单元、集成、Migration、并发和评测覆盖。

### 2. 兼容但不改造

- `text_model`、`qa_model` 和 `table_model` 保持现有索引行为。
- `retrieve_knowledge` 保持现有对外能力和请求参数。
- Workflow Knowledge Retrieval 保持现有行为。
- External Knowledge Base 保持现有召回实现。
- 非 Agent 的知识库命中测试继续使用原服务入口。

### 3. 明确不做

- Metadata Filter。
- 图谱检索优化。
- “精确、平衡、高召回”三档检索策略。
- 父块平均分、多子块奖励、支持密度等复杂计分。
- 新建独立 Agent 框架或后端自主循环 Agent。
- HyDE、Late Chunking 和额外文档摘要索引。
- Contextualizer Provider 硬编码。
- Prompt Caching 的 Provider 专用实现。
- 前端管理页面；本次只提供后端状态和控制接口。

## 四、当前代码基线

| 能力 | 当前代码 | 当前行为 | 本次目标 |
|---|---|---|---|
| Parent-Child Transform | `api/internal/modules/dataset/indexing/parent_child_index_processor.go` | 生成父块和原始子块 | 保持切块规则，增加独立 Contextualizer 步骤 |
| Segment 持久化 | `api/internal/modules/dataset/indexing/indexing_runner.go` | 保存原始父块和子块 | 原始子块保持不变，生成上下文按 Index Generation 独立保存 |
| Context 存储 | 当前不存在 | 无法同时保留多个索引代次的上下文 | 新增 Generation-scoped Context Model 与 Repository |
| 向量/BM25 入库 | `api/internal/modules/dataset/indexing/parent_child_index_processor.go` | `text=child.Content` | `text=child.ContextualizedContent` |
| 单知识库召回 | `api/internal/modules/dataset/service/retrieval_service.go` | 召回、重排、父块转换、裁剪在一个方法内 | 拆为候选召回、候选重排、父块最终化 |
| 多知识库检索 | `api/internal/modules/dataset/service/knowledge_retrieval_service.go` | 逐知识库串行，合并最终父块分数 | 有界并行，在子块候选层聚合 |
| Reranker | `api/internal/modules/dataset/retrieval` | Provider/Model 解析信息没有完整传递到聚合层 | 输出稳定 RerankerFingerprint |
| Agent 工具 | `api/internal/modules/tools/builtin/knowledge/provider.go` | 输出 Context、Resources 和 Retriever Resource 消息 | 模型只接收 Evidence Groups，UI 保留 Resources |
| Agent Skill | `api/internal/modules/skills/catalog/agent-knowledge/SKILL.md` | 最多重试一次，仍暴露 graph | 固定 Hybrid，最多 3 个判断轮次、6 次调用 |

## 五、总体流程

### 1. 入库流程

```text
Extract full document
  -> Transform parent and child chunks
  -> Contextualize every child using full document
  -> Validate contextual output
  -> Persist parent, original child and contextual fields
  -> Build contextualized text
  -> Embed contextualized text
  -> Store contextualized text in searchable text property
  -> Mark document and generation progress
```

### 2. Agent 检索流程

```text
Resolve Agent-bound datasets
  -> Authorize and resolve all dataset retrieval plans
  -> Resolve reranker fingerprints before fan-out
  -> Retrieve dataset candidates with bounded concurrency
  -> Detect fingerprint consistency and rerank fallbacks
  -> Same fingerprint: compare local rerank scores
  -> Different/unknown/fallback: run one global rerank stage
  -> Group child candidates by parent
  -> parent_score = max(child_rerank_score)
  -> Apply final threshold and global top_k
  -> Pack non-duplicated evidence
  -> Return evidence to Agent
```

### 3. Agent 决策流程

```text
Decide whether knowledge is needed
  -> Rewrite the conversational question as a standalone query
  -> Retrieve once
  -> Check whether every material part is supported
  -> Retrieve only missing subquestions or one refined query
  -> Stop at sufficient evidence, 3 decision rounds, or 6 tool calls
  -> Answer with citations or state that evidence is insufficient
```

## 六、配置合同

在 `api/config/types.go` 的 `KnowledgeConfig` 增加以下字段，并在 `api/config/env_keys.go`、`api/config/load.go` 和配置测试中完整接入。

| 环境变量 | Config 字段 | 默认值 | 合法范围 | 用途 |
|---|---|---:|---:|---|
| `KNOWLEDGE_CONTEXTUAL_RETRIEVAL_ENABLED` | `ContextualRetrievalEnabled` | `false` | Boolean | 控制创建和激活 Contextual Generation；不覆盖已有 Active Generation 的读取 |
| `KNOWLEDGE_CONTEXTUALIZATION_MAX_CONCURRENCY` | `ContextualizationMaxConcurrency` | `4` | 1～16 | 单文档子块上下文化并发 |
| `KNOWLEDGE_CONTEXTUALIZATION_TIMEOUT_SECONDS` | `ContextualizationTimeoutSeconds` | `20` | 5～120 | 单子块模型调用超时 |
| `KNOWLEDGE_AGENT_PARALLEL_RETRIEVAL_ENABLED` | `AgentParallelRetrievalEnabled` | `false` | Boolean | 控制多知识库并行和条件式统一重排 |
| `KNOWLEDGE_AGENT_RETRIEVAL_MAX_CONCURRENCY` | `AgentRetrievalMaxConcurrency` | `4` | 1～20 | 多知识库并行上限 |
| `KNOWLEDGE_AGENT_DATASET_TIMEOUT_SECONDS` | `AgentDatasetTimeoutSeconds` | `18` | 5～60 | 单知识库召回与本地重排超时 |
| `KNOWLEDGE_AGENT_GLOBAL_RERANK_TIMEOUT_SECONDS` | `AgentGlobalRerankTimeoutSeconds` | `8` | 3～30 | 统一二次重排超时 |
| `KNOWLEDGE_AGENT_GLOBAL_RERANK_MAX_CANDIDATES` | `AgentGlobalRerankMaxCandidates` | `100` | 20～500 | 单次统一重排最大候选数 |
| `KNOWLEDGE_AGENT_EVIDENCE_TOKEN_BUDGET` | `AgentEvidenceTokenBudget` | `6000` | 1000～20000 | Agent 模型可见证据总预算 |

配置加载规则：

- 非法数字不启动服务并返回英文配置错误。
- 数字超出范围时不静默截断。
- `ContextualRetrievalEnabled=false` 时禁止创建或激活新 Contextual Generation。
- 已有 Active Generation 不受开关变化影响，仍按 Active Generation 读取和维护；禁止用 Feature Flag 静默切到可能已经过期的 Legacy Class。
- 切回 Legacy 必须调用 Rollback API，并通过内容修订号校验。
- `AgentParallelRetrievalEnabled=false` 时保留当前串行检索和聚合行为。
- 并发和超时只用于 Agent 知识检索，不改变普通命中测试和 Workflow 检索。

固定常量：

```text
default_top_k = 5
max_top_k = 20
max_hybrid_candidates_per_dataset = 50
candidate_k = min(50, max(20, top_k * 4))
contextual_prompt_version = "contextual-v1"
```

## 七、数据模型与 Migration

### 1. `child_chunk_contexts` 新表

| 字段 | PostgreSQL 类型 | Null | 默认值 | 说明 |
|---|---|---|---|---|
| `id` | `uuid` | No | - | 主键 |
| `organization_id` | `uuid` | No | - | 租户边界 |
| `dataset_id` | `uuid` | No | - | 知识库 |
| `document_id` | `uuid` | No | - | 文档 |
| `child_chunk_id` | `uuid` | No | - | 原始子块 |
| `index_generation_id` | `uuid` | No | - | 所属 Contextual Generation |
| `contextual_context` | `text` | Yes | `NULL` | 模型生成的块级定位上下文 |
| `contextualized_content` | `text` | Yes | `NULL` | 实际用于 Embedding、BM25 和 Reranker 的稳定文本 |
| `status` | `varchar(32)` | No | `'pending'` | 上下文化状态 |
| `contextualizer_provider` | `varchar(255)` | Yes | `NULL` | 解析后的 Provider |
| `contextualizer_model` | `varchar(255)` | Yes | `NULL` | 解析后的 Model |
| `contextualizer_prompt_version` | `varchar(64)` | Yes | `NULL` | Prompt 版本 |
| `contextual_input_hash` | `varchar(64)` | Yes | `NULL` | SHA-256 幂等 Hash |
| `contextualized_at` | `timestamptz` | Yes | `NULL` | 成功时间 |
| `error` | `text` | Yes | `NULL` | 最近失败原因 |

合法状态：

```text
pending
running
completed
failed
```

约束：

- `completed` 时 `contextual_context`、`contextualized_content`、Provider、Model、Prompt Version、Input Hash 和时间都必须非空。
- Error 字段只保存英文操作错误，不保存完整 Prompt 或文档内容。
- 新 Hash 使用 `crypto/sha256`，不得复用当前 `simpleHash`。
- `UNIQUE(index_generation_id, child_chunk_id)`。
- `INDEX(dataset_id, document_id)` 和 `INDEX(contextual_input_hash, status)`。
- Generation、Child、Document、Dataset 和 Organization 必须属于同一作用域。
- 非 `hierarchical_model` 和 Legacy 索引不创建 Context Row。
- Context Row 必须按 Generation 保存；禁止把生成上下文字段加到 `child_chunks`，否则 Building Generation 会覆盖 Active Generation 的上下文。

### 2. `dataset_index_generations` 新表

| 字段 | PostgreSQL 类型 | Null | 说明 |
|---|---|---|---|
| `id` | `uuid` | No | 主键 |
| `organization_id` | `uuid` | No | 租户边界 |
| `dataset_id` | `uuid` | No | 知识库 |
| `generation` | `bigint` | No | 知识库内递增版本 |
| `mode` | `varchar(32)` | No | `legacy` 或 `contextual_v1` |
| `vector_class_name` | `varchar(255)` | No | 对应 Weaviate Class |
| `status` | `varchar(32)` | No | Generation 状态 |
| `contextualizer_provider` | `varchar(255)` | Yes | Contextual 模型 Provider |
| `contextualizer_model` | `varchar(255)` | Yes | Contextual 模型 |
| `prompt_version` | `varchar(64)` | Yes | Prompt 版本 |
| `content_revision` | `bigint` | No | 构建或最后一次成功同步时对应的 Dataset 内容修订号 |
| `total_documents` | `integer` | No | 文档总数 |
| `completed_documents` | `integer` | No | 成功文档数 |
| `failed_documents` | `integer` | No | 失败文档数 |
| `created_by` | `uuid` | No | 操作者 |
| `created_at` | `timestamptz` | No | 创建时间 |
| `activated_at` | `timestamptz` | Yes | 激活时间 |
| `failed_at` | `timestamptz` | Yes | 失败时间 |
| `retired_at` | `timestamptz` | Yes | Class 和 Context Row 清理完成时间 |
| `error` | `text` | Yes | 最近错误 |

合法状态：

```text
building
ready
active
superseded
failed
retiring
retired
```

允许的状态迁移：

```text
building   -> ready | failed
ready      -> active | retiring
active     -> superseded
superseded -> active | retiring
failed     -> retiring
retiring   -> retired
```

除当前 `active` 的幂等激活外，任何未列出的状态迁移都返回 `knowledge_index_generation_invalid_state`。

约束和索引：

- `UNIQUE(dataset_id, generation)`。
- `UNIQUE(vector_class_name)`。
- `INDEX(dataset_id, status)`。
- `organization_id`、`dataset_id` 外键遵循现有租户和删除规则。
- 使用 Partial Unique Index 保证一个知识库最多存在一个 `active` Generation。
- 使用另一个 Partial Unique Index 保证一个知识库最多存在一个 `building` Generation。

### 3. `datasets` 新增字段

| 字段 | 类型 | Null | 说明 |
|---|---|---|---|
| `active_index_generation_id` | `uuid` | Yes | 当前 Contextual Generation；`NULL` 表示使用 Legacy Class |
| `index_content_revision` | `bigint` | No | 当前知识库可索引内容修订号，默认 `0` |
| `legacy_index_content_revision` | `bigint` | No | Legacy Class 已同步到的内容修订号，默认 `0` |

读取规则：

```text
active_index_generation_id is null
  -> use model.GenCollectionNameByID(dataset.ID)

active_index_generation_id is not null
  -> load active generation
  -> use generation.vector_class_name
```

修订号规则：

- 新增、更新或删除会改变检索内容的 Document、Segment 或 Child 时，`index_content_revision` 在同一业务事务中加一。
- 写入当前生效索引成功后，Legacy 路径更新 `legacy_index_content_revision`，Contextual 路径更新 Active Generation 的 `content_revision`。
- Building 和 Superseded Generation 的 `content_revision` 不随当前写操作变化。
- 回滚到某个 Generation 的前提是其 `content_revision == datasets.index_content_revision`。
- 回滚到 Legacy 的前提是 `legacy_index_content_revision == index_content_revision`。
- 修订号不一致时返回 `knowledge_index_generation_stale`，要求重新构建；禁止切换到内容已经过期的 Class。

### 4. Migration 规则

- 使用 `go run ./cmd/migrate make add_contextual_retrieval_support` 生成真实 ID。
- 一个 Migration 完成 Dataset 字段、Generation 表、Context Row 表、索引和外键。
- 大规模 Context 生成和向量回填不放在 Migration 中。
- Down Migration 只允许在 `dataset_index_generations` 和 `child_chunk_contexts` 均无数据、且所有 Dataset Active ID 均为空时执行；否则返回英文错误。
- 增加 Migration 源码检查和 PostgreSQL 链路验证。
- 不修改 `api/internal/migrations/baseline`。

## 八、Contextual Retrieval 合同

实现依据：[Anthropic Contextual Retrieval](https://www.anthropic.com/engineering/contextual-retrieval)。

符合性边界：

- 在入库预处理期，以完整文档和单个原始 Chunk 为输入生成简短、Chunk-specific Context。
- 将生成 Context 前置到原始 Chunk，并将同一 Contextualized Content 同时用于 Embedding 和 BM25。
- 查询期不再生成父块摘要；Reranker 也使用已经生成的 Contextualized Content。
- 不用通用文档摘要、Metadata 模板或父块拼接替代 Chunk-specific Context。
- 50～100 Token 是输出目标和本项目校验范围，不把 Anthropic 实验中的候选数量或特定模型作为必须照搬的系统参数。
- 本项目仅做 Provider-neutral Gateway 接入和确定性校验；不改变上述方法本身。

### 1. 适用条件

只有同时满足以下条件才执行：

- 数据集存在 Active `contextual_v1` Generation，或在 `ContextualRetrievalEnabled=true` 时构建新的 Contextual Generation。
- 文档 `DocForm == "hierarchical_model"`。
- 文档为内部 Provider。
- 原始完整文档和子块均非空。

`ContextualRetrievalEnabled` 是新 Generation 的发布门，不是已有 Active Generation 的读取覆盖开关。已有 Active Generation 的文档和子块写操作即使在开关关闭后也必须继续 Contextualize；无法完成时写操作失败，不能把 Active Index 留在半同步状态。

其他文档使用原始内容构建同一个 Generation，以保证一个 Dataset Class 包含该知识库的全部可检索文档。

### 2. 服务边界

新增：

```text
api/internal/modules/dataset/indexing/contextualizer.go
api/internal/modules/dataset/indexing/contextualizer_prompt.go
api/internal/modules/dataset/indexing/document_text_loader.go
api/internal/modules/dataset/indexing/contextualizer_test.go
api/internal/modules/dataset/indexing/contextualizer_prompt_test.go
```

核心接口：

```go
type ChunkContextualizer interface {
    ContextualizeDocument(
        ctx context.Context,
        input ContextualizeDocumentInput,
    ) ([]ContextualizedChild, error)
}
```

接口实现必须：

- 通过 `llmruntime.ModelResolver` 解析组织默认 Text Chat 模型。
- 通过 `llmclient.AppChat` 调用 Gateway。
- 使用 Dataset 作为 App 计费上下文。
- 保留解析后的 Provider、Model 和 Context Window。
- 不直接调用 Provider Adapter。
- 不在代码中写死 Claude、OpenAI 或其他 Provider。

`DocumentTextLoader` 合同：

```go
type DocumentTextLoader interface {
    LoadWholeDocument(
        ctx context.Context,
        document *model.Document,
    ) (string, error)
}
```

- 正常入库直接复用当前 `ExtractOutputText`，不重复解析文件。
- 手工 Child 新增、修改和 Reindex 通过 `DocumentTextLoader` 重新执行现有 Extract 流程。
- Loader 返回提取后的完整文档，不允许通过拼接 Parent Segment 代替。
- Loader 不能写 Segment、Child 或向量。
- 源文件不存在、不可访问或重新提取失败时，操作失败并保持原数据。

### 3. 输入

`ContextualizeDocumentInput` 必须包含：

```text
OrganizationID
WorkspaceID
DatasetID
DocumentID
AccountID
WholeDocument
Children[]
PromptVersion
```

每个 Child 必须包含：

```text
StableKey
OriginalContent
ParentPosition
ChildPosition
ExistingInputHash
ExistingStatus
ExistingContext
ExistingContextualizedContent
```

`StableKey` 只用于结果关联，不进入 Prompt。

### 4. Prompt

Prompt 使用固定版本 `contextual-v1`。业务源码中的 Prompt 使用英文。

System Prompt 语义：

```text
You generate concise retrieval context for one chunk from a larger document.
Return only factual context supported by the document.
Do not answer questions, add outside knowledge, or include commentary.
```

User Prompt 结构：

```text
<document>
{{WHOLE_DOCUMENT}}
</document>

<chunk>
{{CHUNK_CONTENT}}
</chunk>

Write a concise 50-100 token context that identifies what this chunk refers to,
where it belongs in the document, and any subject, time, scope, or prerequisite
needed to retrieve it correctly. Return only the context.
```

固定模型参数：

```text
temperature = 0
max_tokens = 120
stream = false
n = 1
```

### 5. 长度与输出校验

- 使用 `pkg/tokenization.TokenizationService` 对 Prompt 输入和输出做统一近似计数。
- 调用前验证：

```text
whole_document_tokens
+ child_tokens
+ prompt_overhead_tokens
+ 120
+ context_window_safety_margin
<= resolved_model.context_window
```

- `prompt_overhead_tokens` 固定按模板实际计数，不使用魔法常量。
- `context_window_safety_margin = max(512, ceil(context_window * 0.05))`，用于吸收近似 Tokenizer 与 Provider Tokenizer 的差异。
- Context 为空：失败。
- Context 估算 Token `<=100`：成功。
- Context `>100`：使用压缩 Prompt 重试一次。
- 第二次仍 `>100`：该文档上下文化失败。
- 不强制输出至少 50 Token；能准确定位的短 Context 可以接受。
- 不允许按字节截断模型输出。
- 去除 Markdown Fence、前后空白和重复的 `<context>` 标签。
- 不允许 Context 与 Original Content 完全相同。

### 6. 超长文档

如果完整文档超过模型 Context Window：

- 返回稳定错误 `knowledge_contextual_document_too_large`。
- 整个文档 Contextualization 失败。
- 不使用父块代替完整文档。
- 不静默退回 Legacy Chunk。
- 不自动更换 Provider。
- 操作者必须配置支持更长上下文的默认模型，或将源文件拆分成可独立理解的逻辑文档后重新入库。

### 7. 并发、超时和重试

- 单文档最多同时 Contextualize `ContextualizationMaxConcurrency` 个子块。
- 每个子块调用使用独立的 `ContextualizationTimeoutSeconds` 子 Context。
- 网络、限流和 5xx 错误最多重试一次。
- 重试间隔使用现有项目退避工具；没有可复用工具时使用 500ms 固定退避。
- 非法输出、Context Window 不足、权限和模型配置错误不重试。
- 任一子块最终失败时，整个文档失败，不进入 Segment 替换和索引阶段。

### 8. 幂等 Hash

`contextual_input_hash`：

```text
sha256(
  whole_document_sha256
  + "\n" + original_child_content
  + "\n" + prompt_version
  + "\n" + resolved_provider
  + "\n" + resolved_model
)
```

复用条件：

- `ChildChunkContextRepository` 在同一 Organization、Dataset、Document 和 Child 下找到 Status 为 `completed` 的历史 Context Row。
- Existing Input Hash 完全一致。
- Existing Context 和 Contextualized Content 非空。

满足条件时将生成结果复制为目标 Generation 的新 Context Row，不跨 Generation 共用同一 Row；否则重新生成。

### 9. Contextualized Content

拼接函数只有一个实现：

```text
strings.TrimSpace(contextualContext)
+ "\n\n"
+ strings.TrimSpace(originalContent)
```

不得在不同调用点自行增加 Document、Metadata、Section 或 Parent Summary。

### 10. Transformed DTO

`dto.TransformedChildChunk` 增加可选字段：

```text
ContextualContext
ContextualizedContent
ContextualizerProvider
ContextualizerModel
ContextualizerPromptVersion
ContextualInputHash
ContextualizationStatus
```

`ParentChildIndexProcessor.Transform` 只负责结构化切块，不直接调用模型。

`IndexingRunner` 在 Transform 完成、`loadSegments` 执行前调用 Contextualizer。这样模型失败不会提前删除现有 Segment。

### 11. 手工子块

`CreateChildChunk` 和 `UpdateChildChunk` 在 Active Contextual Generation 中必须：

1. 取得完整原始文档。
2. 对新增或修改后的子块生成 Context。
3. 取得 Dataset Index Write Lock，再开始数据库和向量变更；Contextualizer 调用不得占用该锁。
4. 在同一数据库事务中保存原始 Child、Active Generation 的 Context Row，并更新 Dataset 与 Active Generation 的 `content_revision`。
5. 使用 Contextualized Content 计算 Embedding 和 Weaviate `text`。
6. 延续当前 Segment Service 的向量补偿模式：向量写失败则回滚数据库事务；数据库提交失败则删除新向量或恢复旧向量。
7. Contextualizer 失败发生在任何数据库或向量写入前，必须保持原状态。

不得只使用父块代替完整文档。

PostgreSQL 与 Weaviate 不存在分布式事务，代码和测试不得宣称天然原子性。补偿失败时返回 `knowledge_index_sync_failed`、记录 Critical Structured Log，并阻止该请求返回成功。

## 九、向量与 BM25 索引合同

### 1. Versioned Class

新增 `DatasetIndexResolver`：

```text
api/internal/modules/dataset/indexing/index_generation.go
api/internal/modules/dataset/repository/index_generation_repository.go
```

职责：

- 为 Legacy Dataset 返回现有 `model.GenCollectionNameByID`。
- 为 Active Contextual Generation 返回版本化 Class。
- 为 Building Generation 生成确定性 Class 名称。
- 校验 Class 与 Dataset、Organization 的归属。

Class 名称：

```text
model.GenCollectionNameByID(datasetID) + "_V" + generation
```

名称生成必须经过 Weaviate 合法性测试。

### 2. Weaviate Properties

版本化 Class 至少包含：

| Property | 类型 | Searchable | 内容 |
|---|---|---|---|
| `text` | `text` | Yes | Contextualized Content 或非层级文档原文 |
| `original_text` | `text` | No | 原始子块或普通 Chunk 原文 |
| `contextual_context` | `text` | No | 生成上下文；非层级文档为空 |
| `content_mode` | `text` | No | `contextual_v1` 或 `original` |
| `contextual_input_hash` | `text` | No | 幂等 Hash |
| `doc_id` | `text` | No | 现有 Index Node ID |
| `document_id` | `text` | No | 文档 ID |
| `dataset_id` | `text` | No | 知识库 ID |

### 3. 索引输入

对于 `hierarchical_model` 子块：

```text
embedding input = contextualized_content
BM25 text = contextualized_content
reranker input = contextualized_content
```

对于其他文档：

```text
embedding input = original content
BM25 text = original content
reranker input = original content
```

原始内容不得被生成 Context 覆盖。

### 4. 查询 Class 解析

以下入口不得再自行调用 `GenCollectionNameByID`：

- Embedding Search。
- BM25 Search。
- Hybrid Search。
- Segment/Child Vector Create、Update、Delete。
- Index Processor Load 和 Clean。
- Contextual Reindex。

它们统一通过 `DatasetIndexResolver` 获得 Class。

## 十、重新索引与切换

### 1. 后端接口

增加需要 `knowledge_base.index.manage` 权限的接口：

```text
POST /datasets/:dataset_id/contextual-index-generations
GET  /datasets/:dataset_id/contextual-index-generations
POST /datasets/:dataset_id/contextual-index-generations/:generation_id/activate
POST /datasets/:dataset_id/contextual-index-generations/rollback
```

第一版不增加前端页面。

创建接口：

- 请求 Body 为空。
- 使用组织默认 Text Chat 模型，不接受 Provider 或 Model 参数。
- 已存在 `building` Generation 时返回 HTTP 409。
- 成功创建并入队后返回 HTTP 202。

成功响应：

```json
{
  "generation": {
    "id": "uuid",
    "generation": 1,
    "mode": "contextual_v1",
    "status": "building",
    "total_documents": 0,
    "completed_documents": 0,
    "failed_documents": 0,
    "created_at": "RFC3339",
    "activated_at": null,
    "error": null
  }
}
```

Generation Row 的创建、知识库内递增版本分配和“最多一个 Building”校验必须在一个数据库事务中完成；唯一约束冲突统一映射为 HTTP 409。

查询接口返回：

```json
{
  "items": [
    {
      "id": "uuid",
      "generation": 1,
      "mode": "contextual_v1",
      "status": "building",
      "total_documents": 10,
      "completed_documents": 4,
      "failed_documents": 0,
      "created_at": "RFC3339",
      "activated_at": null,
      "error": null
    }
  ]
}
```

响应不暴露 `vector_class_name`。

激活接口：

- 只接受 `ready` Generation。
- 成功返回 HTTP 200 和激活后的 Generation。
- 非法状态返回 HTTP 409。

回滚接口 Body：

```json
{
  "target_generation_id": "uuid or null"
}
```

- `target_generation_id=null` 表示回滚到 Legacy。
- 指定目标必须属于同一 Dataset，且状态为 `superseded` 或当前 `active`；`retiring`、`retired`、`failed`、`building` 和 `ready` 不可回滚。
- 目标 Generation 或 Legacy 的内容修订号必须等于 Dataset 当前内容修订号。
- 目标索引已经过期时返回 HTTP 409 和 `knowledge_index_generation_stale`。
- 成功返回 HTTP 200。

### 2. Reindex Task

新增 Asynq Task：

```text
dataset:contextual-index-generation
```

Payload：

```json
{
  "dataset_id": "uuid",
  "generation_id": "uuid",
  "actor_id": "uuid"
}
```

任务要求：

- `TaskID` 使用 Generation ID，防止重复入队。
- 最大自动重试 3 次。
- 每次从数据库状态恢复，不依赖内存进度。
- 任务错误、日志和状态使用英文。

### 3. 构建步骤

1. 获取 Dataset 级分布式写锁。
2. 加载 API 已创建的 `building` Generation，记录当前 `index_content_revision`，创建新 Weaviate Class。
3. 固定本次文档 ID 快照和内容修订号。
4. 对每个文档执行：
   - 非 `hierarchical_model`：使用原始 Chunk 重建到新 Class。
   - `hierarchical_model`：重新提取完整文档，读取现有子块，生成 Generation-scoped Context Row 并写新 Class。
5. 更新 Generation 进度。
6. 任一文档失败：Generation 标记 `failed`，不激活。
7. 全部成功：Generation 标记 `ready`。
8. 激活接口在事务中：
   - 校验状态为 `ready`。
   - 原 Active Generation 标记 `superseded`。
   - 新 Generation 标记 `active`。
   - 更新 `datasets.active_index_generation_id`。
9. 释放写锁。

锁合同：

- Redis Key：`dataset:index-write:{dataset_id}`。
- 初始 TTL 为 60 秒。
- Task 每 20 秒续租。
- 锁值使用 Generation ID，只有持有者可以续租和释放。
- 续租失败立即取消构建并将 Generation 标记为 `failed`。
- 普通 Document、Segment 和 Child 的索引写入使用同一 Key 和短 TTL；无法取得锁时返回 `knowledge_contextual_reindex_in_progress`。

### 4. 构建期间行为

- 读请求继续使用原 Active 或 Legacy Class。
- Dataset 的新增、更新、删除文档和子块返回 `knowledge_contextual_reindex_in_progress`。
- 不允许在构建过程中写入两个索引。
- 锁丢失或任务取消时 Generation 失败，旧索引继续服务。

### 5. 激活和回滚

- 激活前检查新 Class 对象数与数据库可索引节点数一致。
- 激活前执行一组固定 Smoke Queries。
- 激活前检查 Generation `content_revision` 等于 Dataset 当前 `index_content_revision`。
- 激活或回滚到另一个 Generation 时，在同一事务内将原 `active` 改为 `superseded`、目标改为 `active` 并更新 Dataset 指针。
- 回滚只切换状态和 Active Generation 指针，不重新生成向量。
- 回滚到 Legacy 时，在同一事务内将原 `active` 改为 `superseded`，并将 `active_index_generation_id` 设为 `NULL`。
- 回滚前必须执行同样的内容修订号检查；过期索引不得重新激活。
- Superseded Class 默认保留 7 天，由后台清理；清理前不得物理删除。
- 清理操作先用条件更新将 `superseded` 改为 `retiring`，再删除 Weaviate Class 和该 Generation 的 Context Row，最后标记为 `retired`；任一步失败时保持 `retiring` 并重试，防止清理到一半的 Generation 被回滚。
- 当前 Active Class 永远不得被清理。

## 十一、检索候选内部合同

### 1. 新增内部类型

新增文件：

```text
api/internal/modules/dataset/service/retrieval_candidate.go
```

`RetrievalCandidate`：

```text
DatasetID
DatasetName
DocumentID
DocumentName
ParentSegmentID
ChildChunkID
IndexNodeID
OriginalContent
ContextualContext
RerankContent
ContentMode
VectorScore
BM25Score
FusionScore
RerankScore
RetrievalSources
MatchType
RerankerFingerprint
LocalRerankApplied
```

要求：

- 内部 ID 不直接进入 Agent 模型可见消息。
- `RerankContent` 对 Contextual Child 等于 Contextualized Content。
- Score 使用指针区分“0 分”和“未计算”。
- 不再通过 `map[string]interface{}` 传递跨层关键字段。

`DatasetCandidateBatch`：

```text
DatasetID
DatasetName
Candidates
RerankerFingerprint
LocalRerankApplied
LocalRerankFallback
Warnings
Duration
```

### 2. RetrievalService 拆分

将当前单一 `Retrieve` 拆为：

```text
RetrieveCandidates
RerankCandidates
FinalizeParentRecords
Retrieve
```

职责：

- `RetrieveCandidates`：向量与 BM25 召回、融合、去重、可选本地重排，返回子块候选。
- `RerankCandidates`：使用指定已解析 Reranker 对候选打分。
- `FinalizeParentRecords`：子块映射父块、取最高分、阈值、排序、Top K、命中计数。
- `Retrieve`：保持现有兼容入口，依次调用上述方法。

### 3. 候选数量

每个知识库：

```text
candidate_k = min(50, max(20, top_k * 4))
```

规则：

- Hybrid Vector 和 BM25 各自最多召回 50 条。
- 融合去重后执行本地 Reranker。
- 本地 Reranker 只裁剪到 `candidate_k`。
- 在跨知识库判断完成前不应用 Dataset Score Threshold。
- Agent 路径只使用 Agent Retrieval Config 显式配置的 Score Threshold；未配置时不应用阈值。
- Dataset 自身不同的 Score Threshold 不参与跨知识库聚合，避免各库在合并前采用不同标准截断候选。
- 最终父块生成后才应用 Agent 统一阈值和全局 `top_k`。

## 十二、RerankerFingerprint

### 1. 类型

新增：

```go
type RerankerFingerprint struct {
    Provider        string
    Model           string
    Mode            string
    MaxTokensPerDoc int
    ConfigHash      string
}
```

当前 `ResolvedModel` 没有独立 Model Revision，因此 v1 使用解析后的 Provider 和 Model 作为模型身份，不虚构 `resolved_model_version`。

### 2. ConfigHash

使用规范化 JSON 的 SHA-256，内容包括所有影响分数的参数：

```text
mode
max_tokens_per_doc
score-affecting model params
weighted score config
```

Map Key 必须排序后再编码。

### 3. 可直接比较条件

多个 Dataset Batch 只有全部满足以下条件才直接比较：

- Provider 完全相同。
- Model 完全相同。
- Mode 为 `reranking_model`。
- Max Tokens Per Document 相同。
- ConfigHash 相同。
- 所有非空 Batch 都成功执行本地 Reranker。
- 没有 Batch 降级到 Fusion Score。
- 没有 External Provider Batch。

否则必须执行统一二次重排。

### 4. 指纹解析

- 在多知识库并发开始前解析 Dataset 的有效 Retrieval Options 和 Reranker。
- Agent Retrieval Config 仍覆盖 Dataset Retrieval Config。
- Provider 和 Model 来自 `llmruntime.ModelResolver` 的最终结果。
- 解析失败时不启动该 Dataset Retrieval，并返回错误。
- 执行 Reranker 时必须使用同一份已解析结果，禁止再次解析后得到不同模型。

## 十三、多知识库并行检索

### 1. 预处理

`KnowledgeRetrievalService.Retrieve` 在启动并发前：

1. 规范化 Dataset IDs，保持首次出现顺序。
2. 逐个执行访问校验。
3. 加载 Dataset。
4. 合并 Dataset 和 Agent Retrieval Config。
5. 解析 Retrieval Options。
6. Agent 路径固定设置 `RetrievalMode = "vector"`、`SearchMethod = "hybrid_search"`，明确关闭 Graph。
7. 解析 Reranker 和 Fingerprint。
8. 生成 `DatasetRetrievalPlan`。

权限或配置错误在并发前失败。

### 2. 并发执行

- 使用 `errgroup.WithContext`。
- 使用 `SetLimit(AgentRetrievalMaxConcurrency)`。
- 每个 Plan 使用独立的 `AgentDatasetTimeoutSeconds` 子 Context。
- 结果写入按 Plan Index 预分配的 Slice，不并发 Append。
- 任一 Dataset 返回错误时取消其他任务并整体失败。
- 空结果不是错误。
- 不使用 `context.Background()` 获取父块、文档或子块。

### 3. 相同 Reranker

如果所有非空 Batch 指纹可直接比较：

1. 合并所有 Child Candidates。
2. 按 Local Rerank Score 排序。
3. 映射父块。
4. 同一父块取最高 Child Rerank Score。
5. 应用统一 Score Threshold。
6. 取全局 `top_k`。

不执行第二次 Reranker。

### 4. 不同 Reranker

如果指纹不同、未知、存在 External Batch 或发生本地降级：

1. 合并每个 Batch 的 `candidate_k` 个 Child/External Candidates。
2. 按 Dataset ID、Index Node ID 去重。
3. 解析统一 Reranker。
4. 使用统一 Reranker 对 `RerankContent` 执行一次逻辑上的全局重排。
5. 如果合并后的候选超过 `AgentGlobalRerankMaxCandidates`，按 Dataset 本地排名执行公平轮询截断，避免候选较多的知识库独占名额。
6. 将截断后的所有候选放入同一次 Reranker 请求；禁止把候选拆成多个批次后直接比较各批次分数，因为 Provider 可能按批次归一化。
7. 使用统一 Rerank Score 覆盖候选最终分数。
8. 映射父块。
9. 同一父块取最高 Child Rerank Score。
10. 应用统一阈值和全局 `top_k`。

如果统一 Reranker 无法在一次请求中处理 `AgentGlobalRerankMaxCandidates` 条候选，必须在配置或模型能力校验阶段拒绝该配置；运行时仍发生此问题时返回 `knowledge_global_rerank_failed`，不得降级为跨批次分数比较。

### 5. 统一 Reranker 选择

顺序：

1. Agent Retrieval Config 中的有效 Reranker。
2. Organization 默认 Reranker。

不使用“第一个知识库的 Reranker”作为隐式默认。

无法解析统一 Reranker时返回：

```text
knowledge_global_reranker_unavailable
```

统一 Reranker 失败时返回：

```text
knowledge_global_rerank_failed
```

不得回退为直接比较不同分数。

### 6. External Knowledge Base

- External Dataset 仍然并行调用现有 `ExternalRetrieve`。
- 单独只有一个 External Dataset 时保持现有结果。
- External 与其他 Dataset 共同检索时强制统一二次重排。
- External Record 作为 Standalone Candidate，不执行父块转换。
- External Provider 返回的内部 Score 不进入最终跨库排序。

### 7. 稳定排序

所有最终排序使用以下顺序：

```text
score descending
dataset_id ascending
parent_segment_id ascending
child_chunk_id ascending
```

External Candidate 缺失 Parent/Child ID 时使用稳定的 Record ID。

## 十四、父块最终化

### 1. 计分

```text
parent_score = max(child_final_rerank_score)
```

不增加其他分数。

### 2. 数据加载

- 批量加载 Parent Segment、Document 和 Child Chunk。
- 禁止每个候选分别查询数据库。
- 所有 Repository 调用使用请求 Context。
- 同一 Parent 只生成一个最终 Record。
- 保留最高分 Child 作为 Primary Match。
- 其他命中 Child 只进入 UI Retriever Resource 调试信息，不改变分数。

### 3. 命中计数

- 只为最终返回的 Parent 更新 Hit Count。
- 同一 Parent 在一次请求中只更新一次。
- 全局重排失败时不得更新 Hit Count。

## 十五、Agent 工具合同

### 1. 请求参数

`retrieve_agent_knowledge` 只向模型暴露：

```text
query
top_k
```

规则：

- `query` 必填。
- `top_k` 默认 5，最大 20。
- 移除 Agent Tool 的 `retrieval_mode` 参数。
- Provider 内部固定设置不启用 Graph 的 Hybrid Vector + BM25 路径。
- Agent 不得传 Dataset IDs。

`retrieve_knowledge` 的现有参数不在本次修改范围内。

### 2. 检索状态

后端状态只表示检索执行结果：

```text
success
no_results
no_config
```

`sufficient`、`partial` 和 `insufficient` 是 Agent 对证据的判断，不由 Retrieval Service 伪造，也不增加额外后端 LLM Judge。

### 3. 模型可见响应

```json
{
  "query": "standalone query",
  "status": "success",
  "result_count": 2,
  "evidence_groups": [
    {
      "evidence_id": "E1",
      "source": "Knowledge Base / Document",
      "score": 0.91,
      "contextual_context": "Generated retrieval context. Not source text.",
      "content": "Original parent segment content."
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

模型可见响应不得包含：

- Dataset ID。
- Document ID。
- Segment ID。
- Child Chunk ID。
- 完整 `retriever_resources`。
- 重复的拼接 `context`。
- Graph Execution。

### 4. UI Retriever Resources

Retriever Resource 消息继续包含前端定位需要的内部 ID、Document、Parent Content 和 Primary Child 信息。

Skill Runtime 构造模型 Tool Result 时必须忽略 `ToolInvokeMessageTypeRetrieverResources`，避免 UI 消息再次进入模型上下文。

`retrieve_knowledge` 和其他 Retriever Tool 需要回归测试，确认过滤 UI Resource 消息不会丢失其模型可见主消息。

## 十六、Evidence Packing

### 1. Evidence Group

每个最终 Parent 对应一个 Evidence Group：

```text
EvidenceID
Source
Score
ContextualContext
Content
```

含义：

- `ContextualContext`：生成的检索定位信息，非原文，不得作为引用内容。
- `Content`：原始 Parent Segment，是真实证据。
- `EvidenceID`：当前工具响应内从 `E1` 递增。

### 2. Token 预算

- 总预算为 `AgentEvidenceTokenBudget`。
- 单 Evidence 最大 1500 个近似 Token。
- 按最终 Score 从高到低处理。
- Token 预算计算包含 Source、Contextual Context、Content 和 JSON 结构开销。
- 先保留 `min(result_count, floor(total_budget / 200))` 个最高分 Evidence；被预算淘汰的低分 Evidence 不进入模型 JSON，并增加 `evidence_budget_exhausted` Warning。
- 为每个保留的 Evidence 分配最多 200 Token 基础预算，再将剩余预算按分数顺序增加，单条不超过 1500 Token。
- 使用 `pkg/tokenization.TokenizationService` 估算。
- 使用 Rune 边界截断，不使用字节 Slice。
- Content 需要截断时优先保留包含 Primary Child 的连续父块窗口；无法定位 Primary Child 时保留父块开头。
- 截断标记使用非引文字符 `…`，不得把两个不连续片段拼成看似连续的原文。
- 截断后添加布尔字段 `truncated=true`；该字段对模型可见。

### 3. 去重

- Parent Segment ID 相同：保留最高分。
- Parent Content 规范化 Hash 相同：保留最高分。
- 不基于短文本模糊相似度去重。
- Contextual Context 不参与 Evidence Content Hash。

### 4. 引用规则

- Agent 最终答案引用 `[E1]`。
- 引用只能指向 `Content` 中存在的事实。
- Contextual Context 可以帮助理解来源，但不能被逐字引用或单独支撑结论。
- 证据冲突时保留双方，不做静默覆盖。

## 十七、Agentic RAG Skill 合同

### 1. 控制位置

- 多轮检索继续由 `agent-knowledge` Skill 和现有 Skill Loop 控制。
- 后端 Retrieval Service 不实现新的 Agent 循环。
- 不新增第二个 Planner 模型。

### 2. 硬限制

修改 Skill Frontmatter：

```text
max_calls_per_turn: 6
timeout_seconds: 30
```

Skill 行为最多：

- 3 个证据判断轮次。
- 6 次 `retrieve_agent_knowledge` 调用。
- 4 个不同子问题。

Skill Loop 的 `max_calls_per_turn` 是硬限制；Prompt 中的轮次和子问题数是行为合同。

### 3. 工作流

1. 判断用户问题是否依赖 Agent 知识库。
2. 将多轮对话问题改写成独立查询。
3. 第一次只使用最完整、最可能命中的查询。
4. 检查 Evidence Content 是否覆盖所有关键部分。
5. 如问题可拆分，只检索仍缺失的子问题。
6. 不重复语义相同的查询。
7. 达到充分证据后立即停止。
8. 达到轮次或调用上限后停止。
9. 没有充分证据时明确说明未找到足够依据。
10. 关键事实使用 Evidence ID 引用。

### 4. 禁止行为

- 不猜测或传递 Dataset ID。
- 不传 Graph Mode。
- 不使用 Metadata Filter。
- 不将模型记忆作为内部知识库事实。
- 不引用 Contextual Context 中独有、而原文不存在的事实。
- 不把内部 ID 暴露给用户。
- 不输出或持久化 Chain of Thought。

## 十八、错误、降级和超时

### 1. 稳定错误码

| 错误码 | 场景 | 是否重试 |
|---|---|---|
| `knowledge_contextualization_unavailable` | 缺少模型、Gateway 或配置 | No |
| `knowledge_contextual_document_too_large` | 完整文档超过模型窗口 | No |
| `knowledge_contextualization_failed` | 模型调用或输出校验最终失败 | Transient only |
| `knowledge_contextual_reindex_in_progress` | Reindex 锁期间发生写操作 | Yes |
| `knowledge_index_generation_invalid_state` | Generation 状态不允许当前操作 | No |
| `knowledge_index_generation_stale` | 目标索引内容修订号落后，不能激活或回滚 | No |
| `knowledge_index_sync_failed` | PostgreSQL 与 Weaviate 写入补偿失败 | Operator action |
| `knowledge_dataset_retrieval_failed` | 任一 Dataset 检索失败 | Transient only |
| `knowledge_global_reranker_unavailable` | 异构分数但无统一模型 | No |
| `knowledge_global_rerank_failed` | 统一二次重排失败 | Transient only |

业务源码中的错误和日志为英文；Handler 映射到现有 Response 与国际化文案。

### 2. 降级矩阵

| 场景 | 行为 |
|---|---|
| Contextual Feature 关闭且无 Active Generation | Legacy 检索 |
| Contextual Build 失败 | 继续读旧 Active 或 Legacy |
| 单 Dataset 本地 Reranker 失败且只有一个 Dataset | 保持当前 Fusion Fallback，并返回 Warning |
| 多 Dataset 任一本地 Reranker 失败 | 强制统一二次重排 |
| 多 Dataset 统一二次重排失败 | 整体错误，不混合分数 |
| 单 Dataset 无结果 | `no_results` |
| 多 Dataset 部分为空、其余成功 | 使用非空结果 |
| 多 Dataset 任一调用错误 | 整体错误并取消其他任务 |

### 3. 时间预算

```text
Agent Tool overall timeout = 30s
Per-dataset retrieval timeout = 18s
Global rerank timeout = 8s
Evidence packing and serialization reserve >= 4s
```

如果父 Context 先结束，所有子任务必须立即取消。

## 十九、安全、权限和计费

- Agent 只检索已绑定知识库；并行不改变授权边界。
- Dataset 在并发前逐个完成访问检查。
- Contextualizer 只接收同一个 Document 的完整内容和 Child。
- 不跨 Dataset、Workspace 或 Organization 共享 Prompt Cache。
- Contextualizer 使用 Dataset App Context 计费。
- 全局二次 Reranker 使用 Agent App Context 计费；调用日志记录 Agent ID，但不进入模型可见输出。
- 日志不记录完整 Document、Child、Contextualized Content 或 Prompt。
- Trace 默认记录 Hash、长度、Provider、Model、版本、候选数和耗时。
- 重新索引接口要求 `knowledge_base.index.manage`。
- Generation 和 Dataset 必须验证 Organization、Workspace 和 Dataset 归属。

## 二十、可观测性

### 1. Metrics

新增低基数 Metrics：

```text
knowledge_contextualization_total{status}
knowledge_contextualization_duration_seconds
knowledge_contextualization_output_tokens
knowledge_contextual_generation_documents_total{status}
knowledge_agent_dataset_retrieval_duration_seconds
knowledge_agent_dataset_retrieval_active
knowledge_agent_global_rerank_total{reason,status}
knowledge_agent_global_rerank_duration_seconds
knowledge_agent_evidence_tokens
knowledge_agent_retrieval_total{status}
```

禁止以 Dataset ID、Document ID、Agent ID 作为 Metric Label。

### 2. Structured Logs / Trace

每次 Agent 检索记录：

```text
request_id
agent_id
dataset_count
parallelism
candidate_count_per_dataset
reranker_fingerprint_hash_per_dataset
reranker_consistent
global_rerank_reason
global_rerank_applied
final_parent_count
evidence_token_count
duration_ms_by_stage
error_code
```

生产日志中的 Agent ID 按现有隐私规范处理；不得记录 Query 和原始证据全文。

## 二十一、接口兼容性

### 1. 保持兼容

- `KnowledgeRetrieveRequest` 现有字段保留。
- `KnowledgeRetrieveResponse.Context`、`Resources` 和 `Records` 保留给现有非 Agent 调用者。
- `retrieve_knowledge` 不变。
- `RetrievalService.Retrieve` 保持现有签名，通过新内部方法实现。

### 2. Agent 专用变化

- `retrieve_agent_knowledge` 不再暴露 `retrieval_mode`。
- Agent Tool JSON 主消息改为 Evidence Groups。
- UI Retriever Resource 消息继续返回。
- Agent Skill Runtime 不把 UI Resource 消息序列化到模型。

### 3. Feature Flag 兼容性

- `KNOWLEDGE_AGENT_PARALLEL_RETRIEVAL_ENABLED=false` 时保留当前串行多知识库检索路径。
- Agent 多知识库并行功能可以独立启用，不依赖 Contextual Generation。
- `KNOWLEDGE_CONTEXTUAL_RETRIEVAL_ENABLED=false` 时禁止创建或激活新 Generation，但继续读取和维护已有 Active Generation。
- Contextual Feature 开启但没有 Active Contextual Generation 的知识库使用 Legacy Class。
- 不改变现有文档、向量和检索 API。

## 二十二、测试规格

### 1. Config

修改或新增 Config Tests：

- 默认值正确。
- 每个环境变量正确解析。
- 小于最小值和大于最大值均失败。
- Feature 关闭时不创建 Contextual Generation。
- Contextual Feature 关闭时仍解析已有 Active Generation，防止静默切换到过期 Legacy Class。
- Parallel Feature 关闭时继续执行当前串行多知识库路径。
- `AgentGlobalRerankMaxCandidates` 的边界值 20 和 500 可用，越界值启动失败。

### 2. Migration

- Migration ID 和文件名通过 `go run ./cmd/migrate check`。
- PostgreSQL Up 后字段、表、索引、外键和约束存在。
- Down 在任意 Generation 或 Context Row 存在时失败。
- Down 在无 Generation、Context Row 和 Active ID 时成功。
- Migration 不执行数据回填。

### 3. Contextualizer Unit Tests

- Prompt 同时包含 Whole Document 和 Child。
- Prompt 不包含其他 Dataset 内容。
- Temperature、Max Tokens 和 Stream 固定。
- 解析后的 Provider/Model 被保存。
- 输出为空失败。
- 输出超过 100 Token 时压缩重试一次。
- 第二次仍超长时失败。
- Context Window 不足时不调用模型。
- Transient Error 只重试一次。
- 一个 Child 失败导致整个 Document 失败。
- 并发峰值不超过配置。
- Input Hash 一致时复用结果。
- Input Hash 变化时重新生成。

### 4. Indexing Tests

- Contextualized Content 等于唯一拼接函数结果。
- Embedding 输入是 Contextualized Content。
- Weaviate `text` 是 Contextualized Content。
- `original_text` 保持原始 Child。
- BM25 可以命中只出现在 Context 中的 Subject 或 Time。
- 非 `hierarchical_model` 内容不生成 Context。
- Contextualizer 失败时不删除原 Segment。
- 手工新增、修改 Child 使用完整 Document Contextualize。
- Child Context Row 只写入当前 Active Generation，不覆盖 Superseded Generation。
- Contextualizer 失败发生在数据库和向量写入前。
- 向量写失败时数据库事务回滚；数据库提交失败时新向量删除或旧向量恢复。
- 删除 Child 清理 Active Class 中对应对象。

### 5. Generation Tests

- Legacy Dataset 在 Active ID 为空时解析旧 Class。
- Building Generation 不影响读流量。
- 全部文档成功后才能 Ready。
- 任一文档失败时不能激活。
- 激活事务只产生一个 Active Generation。
- Rollback 恢复前一个 Generation 或 Legacy。
- 目标索引修订号过期时 Activate/Rollback 均失败。
- 新文档或子块写入成功后 Dataset 与 Active Generation 修订号同步增加。
- Active Class 不被 Cleanup。
- Reindex 锁期间写操作返回稳定错误。
- 普通索引写入与 Reindex 使用同一个 Dataset Lock Key。
- 重复 Task ID 不重复构建。

### 6. Retrieval Candidate Tests

- `RetrieveCandidates` 返回 Child，而不是提前转换 Parent。
- Local Reranker 输入为 Contextualized Content。
- `candidate_k` 公式覆盖 `top_k=1,5,20`。
- Finalize 后 Parent Score 等于最高 Child Score。
- Threshold 只在 Parent 最终化后应用。
- Parent、Document 和 Child 使用批量加载。
- Repository 使用请求 Context。

### 7. 多知识库并行测试

- 最大活跃 Dataset Task 不超过配置。
- 并行耗时接近最慢 Dataset，而不是耗时总和。
- 任一 Dataset Error 取消其他任务。
- 空 Dataset 不导致整体失败。
- 输出顺序在相同输入下稳定。
- Agent 路径不应用 Dataset 自身的不同 Score Threshold。
- Agent Retrieval Config 未配置 Score Threshold 时不提前过滤候选。

### 8. Reranker 分支测试

| 场景 | 预期 |
|---|---|
| 三个 Dataset 指纹相同且全部成功 | Global Rerank 调用 0 次 |
| Provider 不同 | Global Rerank 1 个逻辑 Stage |
| Model 不同 | Global Rerank 1 个逻辑 Stage |
| 一个本地 Reranker Fallback | Global Rerank 1 个逻辑 Stage |
| 包含 External Dataset | Global Rerank 1 个逻辑 Stage |
| 合并候选超过全局上限 | 按 Dataset 本地排名公平轮询截断，并且只调用一次 Global Rerank |
| Global Rerank 失败 | 整体返回稳定错误 |
| 只有一个 Dataset | 不执行 Global Rerank |

### 9. Tool 与 Evidence Tests

- Agent Tool 参数只有 Query 和 Top K。
- Agent Tool 内部不启用 Graph。
- 模型 JSON 只包含 Evidence Groups 和 Diagnostics。
- UI Resource 仍包含定位字段。
- UI Resource 不进入模型 Tool Result。
- Evidence ID 连续稳定。
- Contextual Context 标记为非原文。
- Content 来自原 Parent Segment。
- 同一 Parent 不重复。
- Token 预算不足时只保留预算可容纳的最高分 Evidence，并返回 Warning。
- Rune 截断正确且优先保留 Primary Child 所在连续窗口。
- 内部 ID 不进入模型可见 JSON。

### 10. Skill Tests

- `max_calls_per_turn=6`。
- `timeout_seconds=30`。
- Skill 不再建议 Graph。
- Skill 先使用独立查询。
- Skill 只检索缺失子问题。
- 没有证据时不生成知识库事实。
- 引用只使用 Evidence ID。

### 11. 回归命令

从 `api/` 运行：

```bash
go test ./internal/modules/dataset/indexing/...
go test ./internal/modules/dataset/service/...
go test ./internal/modules/dataset/retrieval/...
go test ./internal/modules/tools/builtin/knowledge/...
go test ./internal/modules/skills/...
go test ./internal/migrations/...
go test ./internal/...
go build ./...
```

实现过程中先运行窄测试，阶段完成后运行完整 `go test ./internal/...` 和 `go build ./...`。

## 二十三、质量验收门槛

### 1. 功能门槛

- 所有本规格中的确定性测试通过。
- 两个 Feature 均关闭且 Dataset 无 Active Generation 时，现有 Legacy 行为和测试无回归。
- 不存在异构 Reranker 分数直接混排。
- Parent Score 严格等于最高 Child Final Rerank Score。
- Contextualized Content 同时进入 Embedding、BM25 和 Reranker。
- 生成 Context 不覆盖原始证据。
- Agent 模型上下文中每个 Evidence 只出现一次。

### 2. 检索质量

使用固定评测集比较 Legacy 与 Contextual：

- 通用问题 Parent Recall@5 不得下降超过 2 个百分点。
- “子块缺少主体、时间、范围”专项集 Parent Recall@5 至少提升 10 个百分点。
- 子块局部相似但父语境不适用的专项集，正确 Parent 进入 Top 5 的比例至少提升 10 个百分点。
- 无答案问题的错误高分结果数量不得增加。

### 3. 性能和成本

- 3 个等延迟知识库场景，Agent 检索 P95 不高于串行基线的 70%。
- 相同 Reranker 场景不得产生 Global Rerank 成本。
- 不同 Reranker 场景最多产生一个逻辑 Global Rerank Stage。
- Contextualization 只在入库、更新和重建时发生，查询时不得调用 Contextualizer。
- Evidence Token 不超过配置预算。

## 二十四、灰度与回滚

### 1. 发布顺序

1. 发布 Migration、Config、Model 和 Repository，Feature 保持关闭。
2. 发布 Contextualizer 和 Generation 构建能力。
3. 对测试 Dataset 构建 Contextual Generation，不激活。
4. 执行索引对象数、Smoke Query 和离线评测。
5. 激活测试 Dataset。
6. 发布多知识库并行和 Reranker 分支，先只记录决策，不改变结果。
7. Shadow 结果验证后启用并行排序。
8. 发布 Agent Evidence 和 Skill 更新。
9. 分批为生产 Dataset 构建和激活 Generation。

### 2. 回滚

- Agent 并行出现问题：关闭并行功能，恢复串行 Legacy 聚合。
- Contextual 检索出现问题：将 Dataset Active Generation 回滚到前一版本或 Legacy。
- Tool Payload 出现问题：恢复旧 Agent JSON 主消息，但保持 UI Resource。
- Skill 行为出现问题：恢复旧 Skill 文件和调用上限。
- 数据库字段不立即删除；回滚代码必须能够忽略新字段。

## 二十五、逐文件改动清单

### Config

```text
api/config/types.go
api/config/env_keys.go
api/config/load.go
api/config/*_test.go
```

### Migration / Model / Repository

```text
api/internal/migrations/<generated>_add_contextual_retrieval_support.go
api/internal/migrations/<generated>_add_contextual_retrieval_support_test.go
api/internal/modules/dataset/model/dataset.go
api/internal/modules/dataset/model/child_chunk_context.go
api/internal/modules/dataset/model/index_generation.go
api/internal/modules/dataset/repository/dataset_repository.go
api/internal/modules/dataset/repository/document_repository.go
api/internal/modules/dataset/repository/child_chunk_context_repository.go
api/internal/modules/dataset/repository/index_generation_repository.go
```

### Indexing

```text
api/internal/dto/transformed_chunk_dto.go
api/internal/modules/dataset/indexing/contextualizer.go
api/internal/modules/dataset/indexing/contextualizer_prompt.go
api/internal/modules/dataset/indexing/document_text_loader.go
api/internal/modules/dataset/indexing/index_generation.go
api/internal/modules/dataset/indexing/indexing_runner.go
api/internal/modules/dataset/indexing/parent_child_index_processor.go
api/internal/modules/dataset/indexing/batch_indexing.go
api/internal/modules/dataset/service/index_write_coordinator.go
api/internal/modules/dataset/service/chunk_service.go
api/internal/modules/dataset/service/document_service.go
api/internal/modules/dataset/service/document_indexing_service.go
api/internal/modules/dataset/service/segment_service.go
api/internal/modules/dataset/task/document_indexing.go
```

### Retrieval

```text
api/internal/modules/dataset/service/retrieval_candidate.go
api/internal/modules/dataset/service/retrieval_service.go
api/internal/modules/dataset/service/knowledge_retrieval_service.go
api/internal/modules/dataset/service/hit_testing_service.go
api/internal/modules/dataset/retrieval/rerank_service.go
api/internal/modules/dataset/retrieval/rerank_runner.go
api/internal/modules/dataset/retrieval/gateway_rerank_service.go
```

### Handler / Task

```text
api/internal/modules/dataset/handler/dataset_handler.go
api/internal/modules/dataset/task/contextual_index_generation.go
api/internal/modules/dataset/task/contextual_index_generation_handler.go
```

### Agent Tool / Skill

```text
api/internal/modules/tools/builtin/knowledge/provider.go
api/internal/modules/skills/catalog/agent-knowledge/SKILL.md
api/internal/modules/skills/catalog_helpers.go
api/internal/modules/skills/runtime.go
```

### Tests

```text
api/internal/modules/dataset/indexing/contextualizer_test.go
api/internal/modules/dataset/indexing/contextualizer_prompt_test.go
api/internal/modules/dataset/indexing/index_generation_test.go
api/internal/modules/dataset/indexing/parent_child_index_processor_test.go
api/internal/modules/dataset/service/retrieval_service_test.go
api/internal/modules/dataset/service/knowledge_retrieval_service_test.go
api/internal/modules/dataset/service/segment_vector_test.go
api/internal/modules/dataset/retrieval/rerank_runner_test.go
api/internal/modules/tools/builtin/knowledge/provider_test.go
api/internal/modules/skills/runtime_catalog_test.go
```

文件清单是允许范围。新增文件必须位于相应模块，不得将 Dataset 业务逻辑放入共享基础设施。

## 二十六、实施任务与依赖

### T01：冻结基线

依赖：无。

内容：

- 为当前 Parent-Child 召回、父块 Max Score、多知识库串行和 Agent Tool Payload 增加基线测试。
- 固定评测集和 Legacy 指标。

完成条件：

- 不修改生产行为。
- 基线测试稳定通过。

### T02：配置、错误和数据结构

依赖：T01。

内容：

- 增加 Config。
- 生成 Migration。
- 增加 Generation-scoped Child Context、Index Generation、Dataset 内容修订号的 Model/Repository。
- 增加稳定错误码。

完成条件：

- Migration 和 Model 测试通过。
- Feature 默认关闭。

### T03：Contextualizer

依赖：T02。

内容：

- 实现 Prompt、模型解析、AppChat、校验、Hash、并发、超时和重试。
- 只接受完整文档和原始 Child。

完成条件：

- Contextualizer 单元测试全部通过。
- 不接入生产索引。

### T04：Contextual 索引写入管线

依赖：T03。

内容：

- 为显式传入的 Building Generation 在 `loadSegments` 前 Contextualize。
- 按目标 Building Generation 持久化 Context Row。
- Embedding 与 Weaviate `text` 使用 Contextualized Content。

完成条件：

- 未传入 Contextual Generation 时 Legacy 行为不变。
- Building Generation 中 Embedding、BM25 和 Reranker 三路输入一致。

### T05：Index Generation 与 Reindex

依赖：T04。

内容：

- Versioned Class Resolver。
- Generation API、Task、写锁、进度、激活、回滚和清理。
- 非 Parent-Child 文档复制到新 Generation。
- 新文档和 Create/Update/Delete Child 接入 Active Generation。
- 手工 Child 使用完整文档生成 Context。
- 原始内容变更时更新 Dataset 与 Active Index 的内容修订号，并覆盖向量补偿测试。

完成条件：

- 构建失败不影响旧读流量。
- 激活和回滚测试通过。

### T06：候选级 Retrieval 重构

依赖：T01。

内容：

- 增加 `RetrievalCandidate`。
- 拆分 Recall、Rerank 和 Finalize。
- 保持 `Retrieve` 兼容。
- 批量父块加载。

完成条件：

- 现有 Retrieval 回归测试通过。
- Parent Score 仍为 Child Max Score。

### T07：多知识库并行与 RerankerFingerprint

依赖：T06。

内容：

- 预解析 Dataset Plan。
- 有界并发。
- 指纹一致直接排序。
- 异构、External、Fallback 使用统一二次重排。

完成条件：

- 并发、取消、相同/不同指纹测试通过。
- 不存在异构分数直接比较。

### T08：Evidence Payload

依赖：T06、T07。

内容：

- Evidence Groups。
- Token 预算和去重。
- 模型消息与 UI Resource 分离。
- 内部 ID 隐藏。

完成条件：

- 模型上下文无重复证据。
- UI Resource 功能不回归。

### T09：Agent Skill

依赖：T08。

内容：

- 移除 Graph 参数。
- 固定 Hybrid。
- 更新查询改写、子问题、停止条件、证据引用和拒答行为。
- 将最大调用数改为 6。

完成条件：

- Skill Catalog、Governance 和 Runtime Tests 通过。

### T10：灰度和评测

依赖：T05、T07、T08、T09。

内容：

- Shadow Decision 日志。
- 测试 Dataset Generation。
- 离线质量、并发性能和成本评测。
- 按验收门槛决定是否启用。

完成条件：

- 所有功能、质量和性能门槛通过。
- 回滚演练通过。

## 二十七、禁止模型自行决定的事项

实现时不得自行：

- 扩大到 `text_model`、`qa_model` 或 `table_model` 的 Contextualization。
- 添加 Metadata Filter 或 Graph Retrieval。
- 更换父块 Max Score 规则。
- 在查询期调用 Contextualizer。
- 将父块摘要替代完整文档输入。
- 将生成 Context 当作原始证据。
- 在不同 Reranker 分数之间直接排序。
- 在 Global Rerank 失败后混用 Fusion Score 和 Rerank Score。
- 硬编码 Provider 或绕过 Gateway。
- 新建另一个 Agent Planner 服务。
- 修改前端或无关模块。
- 删除现有兼容字段或测试。
- 使用 `context.Background()` 替代请求 Context。
- 在业务源码中新增中文日志、错误或注释。

发现本规格无法覆盖的实现问题时，应暂停对应任务，更新本规格并经评审后再继续，而不是自行扩展方案。

## 二十八、Definition of Done

- [ ] 本规格范围内的所有任务完成。
- [ ] Migration 可执行、可检查、可安全回滚。
- [ ] Contextualizer 使用完整文档，输出满足合同。
- [ ] Contextualized Content 同时用于 Embedding、BM25 和 Reranker。
- [ ] 原始 Child 与生成 Context 分离保存。
- [ ] Versioned Index 可构建、激活和回滚。
- [ ] 多知识库检索为有界并行。
- [ ] 相同 Reranker 不产生额外 Global Rerank。
- [ ] 异构或降级分数必须经过统一 Reranker。
- [ ] Parent Score 等于最高 Child Final Score。
- [ ] Agent Tool 不暴露 Graph、Dataset ID 或重复证据。
- [ ] Agent 最多 3 个判断轮次、6 次工具调用。
- [ ] 两个 Feature 均关闭且 Dataset 无 Active Generation 时 Legacy 行为无回归。
- [ ] 窄测试、`go test ./internal/...` 和 `go build ./...` 通过。
- [ ] 离线质量、性能和成本门槛通过。
- [ ] 灰度和回滚演练完成。
- [ ] 新增业务源码语言符合项目宪章。
