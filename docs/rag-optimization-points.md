# RAG 优化点

## 摘要

当前 RAG 系统的优化主线是：先建立评估与可观测性体系，再优化召回质量，最后提升生成侧对证据的使用能力。

目前已确定的优化方向包括：

- 先补齐 **RAG 评估与可观测性**，作为后续所有优化的验证基础。
- 基于文档结构进行父子切块，并利用**标题路径增强 chunk 语义**。（这个其实跟Contextual Retrieval很像，但是更加的轻量，可以用这个走通流程，看看效果，然后接着做**Contextual Retrieval**）
- 当前混合检索存在向量、keyword、full-text 三路堆叠问题，分数不可比、职责重叠、多路命中信号丢失，需要收敛为向量 + BM25。
- 在检索前增加 query 改写，提升短 query、口语 query 和多轮指代问题的召回效果。
- 增强检索可解释性，记录召回来源、分数、命中词、rerank 变化和最终上下文，提升可信度并方便持续优化 RAG 链路。
- 在生成侧增加证据感知的上下文组装与回答约束，让模型基于结构化证据回答并保留来源引用。

整体目标是让 RAG 链路从“能召回、能回答”升级为“可评估、可解释、可追溯、可持续优化”。

## 竞品调研：文档问题处理方式

### 调研结论

主流 RAG 产品和方案已经不再把文档处理理解为“固定长度切块 + 向量召回”。它们普遍在解决几个问题：

- 文档结构复杂时，先做结构化解析、按文档类型选择切块策略。
- chunk 太小会丢上下文，太大会影响精确召回，所以使用父子切块或层级切块。
- 单纯向量召回不够稳定，所以使用向量 + 全文/BM25 的混合检索，并配合 rerank。
- chunk 被孤立后语义不足，所以通过 summary、标题路径、contextual chunking 等方式补充上下文。
- 检索结果必须能测试、解释和追溯，否则很难持续优化。

### Dify

Dify 的知识库支持 General 和 Parent-child 两种 chunk mode。Parent-child 模式下，child chunk 用于匹配 query，命中后返回整个 parent chunk，用来解决“小块召回精确但缺上下文，大块上下文完整但召回不准”的矛盾。

Dify 还支持 Summary Auto-Gen，会为 chunk 自动生成 summary，并把 summary 一起 embedding 和索引；当 query 命中 summary 时，可以返回对应 chunk。这和 Anthropic 的 Contextual Retrieval 思路接近，都是给孤立 chunk 补充可检索的语义上下文。

在检索侧，Dify 支持 vector search、full-text search、hybrid search，并支持 rerank model。它也提供 Retrieval Testing，用于模拟 query、调试检索效果和记录检索事件。

对当前系统的启发：

- 我们的父子切块方向是对的，但还需要把 parent/child 的职责做清楚。
- 标题路径增强可以先落地，后续再加入 contextual summary。
- 混合检索不应该是多路简单堆叠，而应该是明确的向量 + BM25/全文检索 + rerank。
- 需要提供检索测试和检索记录，支撑评估与可观测性。

资料来源：

- [Dify：Configure the Chunk Settings](https://docs.dify.ai/en/use-dify/knowledge/create-knowledge/chunking-and-cleaning-text)
- [Dify：Specify the Index Method and Retrieval Settings](https://docs.dify.ai/en/use-dify/knowledge/create-knowledge/setting-indexing-methods)
- [Dify：Summary Index](https://dify.ai/blog/dify-1.12.0-summary-index-from-fragmented-retrieval-to-full-context)
- [Dify：Test Knowledge Retrieval](https://docs.dify.ai/en/use-dify/knowledge/test-retrieval)

### RAGFlow

RAGFlow 明确强调深度文档理解。它提供多种内置 chunking template，按文件布局和类型选择不同切块方式，例如 General、Q&A、Table、Paper、Book、Laws、Presentation、Picture、One 等，用来减少语义损失和答案错配。

RAGFlow 也支持 parent-child chunking：文档先切成语义相对完整的 parent chunk，再拆成更细的 child chunk。检索时先定位 child chunk，再自动关联并召回 parent chunk，从而同时获得“精确定位”和“上下文补充”。

在检索和调试侧，RAGFlow 支持全文检索 + 向量检索的多路召回，并提供相似度阈值、向量权重、检索测试、chunk 可视化、手动修正 chunk、添加关键词提升排序、引用展示等能力。

对当前系统的启发：

- 文档类型路由是必要的，不同文档不应该共用同一套切块方式。**后面具体上手使用一下这个，看看他们是怎么做路由的。**
- 父子切块不是只存父子关系，还要在检索时明确“child 召回、parent 返回”。
- 表格、法律、书籍、论文、PPT 等文档类型应有专门解析和切块策略。
- 检索结果要能可视化、可干预、可测试，否则很难定位切块和召回问题。

资料来源：

- [RAGFlow：Configure dataset](https://ragflow.io/docs/configure_knowledge_base)
- [RAGFlow：Configure child chunking strategy](https://ragflow.io/docs/configure_child_chunking_strategy)

### Amazon Bedrock Knowledge Bases

Amazon Bedrock Knowledge Bases 提供 advanced parsing、semantic chunking、hierarchical chunking 和 query reformulation。它强调复杂 PDF、表格、嵌套结构、图片文本等文档如果只做普通切块，可能导致召回结果无法回答用户问题。

Bedrock 的 hierarchical chunking 会维护 parent chunk 和 child chunk 的层级关系：semantic search 发生在 child chunk 上，检索返回时用 parent chunk 提供更完整的上下文。它也支持 query decomposition，把复杂 query 拆解后再检索。

对当前系统的启发：

- 我们应把父子切块和 query 改写作为一组能力设计，而不是割裂优化。
- 对复杂文档，解析质量和切块质量会直接决定召回质量。
- query decomposition 对复杂问题、多条件问题、多意图问题很重要。

资料来源：

- [AWS：Advanced parsing, chunking, and query reformulation](https://aws.amazon.com/blogs/machine-learning/amazon-bedrock-knowledge-bases-now-supports-advanced-parsing-chunking-and-query-reformulation-giving-greater-control-of-accuracy-in-rag-based-applications/)

### Anthropic Contextual Retrieval

Anthropic 提出的 Contextual Retrieval 会在每个 chunk 前补充一段由 LLM 生成的简短上下文，再用增强后的文本做 embedding 和 BM25 索引。它解决的是传统 RAG 中 chunk 被切出来后丢失文档背景的问题。

Anthropic 的实验结果显示：Contextual Embeddings 将 top-20 chunk 检索失败率降低 35%；Contextual Embeddings + Contextual BM25 降低 49%；再叠加 rerank 后降低 67%。

对当前系统的启发：

- 标题路径增强是轻量版 contextual chunking，可以先落地验证。
- 后续可以为每个 child chunk 生成 1-2 句话的 contextual summary，用于 embedding 和 BM25。
- summary 只用于检索增强，最终回答和引用仍应以原文 chunk 为准。
- 该方案有额外 LLM 成本，适合异步生成、缓存、可开关配置。

资料来源：

- [Anthropic：Contextual Retrieval](https://www.anthropic.com/engineering/contextual-retrieval)

### 对当前优化方向的修正

结合竞品调研，当前优化点可以进一步收敛为：

- 切块侧：先做结构化父子切块和标题路径增强，再逐步引入 contextual summary。
- 召回侧：只保留向量 + BM25 两路主召回，明确融合和 rerank 机制。
- Query 侧：增加 query rewrite / query decomposition，适配短 query、多轮指代和复杂问题。
- 可观测侧：补齐检索测试、召回 trace、分数记录、命中来源、chunk 可视化。
- 生成侧：把命中结果组装成带来源编号、标题路径、页码和命中片段的证据包，而不是普通文本拼接。

## 优化点 0：RAG 评估与可观测性体系

### 现在的问题

当前 RAG 优化主要依赖经验判断，缺少稳定的评测集、指标和链路观测。后面提出的所有优化点：切块、混合检索、query 改写、rerank 等优化是否真的提升效果，很难被量化验证，也不容易定位问题发生在解析、切块、召回、融合、重排还是回答阶段。

### 业界先进做法

生产级 RAG 会先建立评估与可观测性体系，再持续迭代检索策略。常见做法是维护标准评测集，跟踪 recall@k、MRR、nDCG、答案命中率、无答案拒答率、延迟、成本等指标，并记录每次请求的 query 改写、召回来源、命中 chunk、融合分数、rerank 前后排名和最终上下文。

### 优化方式

建立一套贯穿全链路的评估和观测机制：目前使用医院文档数据集构造出来了一个小的评测集，后面尝试走通整个评测的流程，就可以全量生成评测集进行评测。

### 带来的效果

- 后续所有 RAG 优化都有可量化验证标准。
- 能快速定位召回差、排序差、回答差的具体环节。
- 避免凭主观感觉调参。
- 支持不同文档类型下的效果对比。
- 为持续迭代切块、混合检索、query 改写和 rerank 打基础。

## 优化点 1：结构化切块 + 标题上下文增强

### 现在的问题

当前切块虽然基于解析后的文档结构元素，但切块边界和标题信息利用不充分。很多 child chunk 切出来后丢失章节语境，单独看语义不完整，导致向量检索时容易召回不准，尤其影响合同、手册、报告、制度类文档。

### 业界先进做法

生产级 RAG 通常不会只按固定长度切文本，而是做 layout-aware / structure-aware chunking：先识别标题、段落、列表、表格、代码、图表、公式等结构元素，再按语义完整单元生成 chunk。同时，会把标题、章节路径、文档上下文写入 chunk metadata，必要时也注入 embedding 文本。

**LlamaIndex Metadata Injection**：metadata 默认会注入到发给 embedding model 和 LLM 的文本中，标题、章节等 metadata 可以成为 embedding 上下文。

### 优化方式

基于解析后的文档结构生成父子块：

- parent 按标题层级、section、表格、代码、图表等结构单元生成，保证上下文完整。
- child 在 parent 内部按段落、句子、表格行组等生成，用于精确 embedding 和召回。
- 每个 chunk 保留完整 `heading_path` metadata。
- child embedding 文本拼入精简标题路径，例如：

```text
付款条款 > 逾期付款

乙方应在收到发票后 30 日内付款。
```

- 检索后用 `heading_path` 做聚合、rerank、来源展示和 prompt 组装。

### 带来的效果

- chunk 不再丢失章节语境。
- 短文本、条款、指标类内容更容易被正确召回。
- 同样词语在不同章节下更容易区分。
- 返回来源更清晰，可解释性更好。
- 为后续按章节过滤、rerank、质量评估打基础。

## 优化点 2：向量 + BM25 混合检索

### 现在的问题

当前系统里存在向量、keyword、full-text 三路召回，但这不是清晰的混合检索，而是多路召回堆叠。`keyword_search` 是简单词频倒排，`full_text_search` 更接近 BM25，两者职责重叠；三路结果合并时分数不可比，同一 chunk 被多路命中后也没有稳定转化为排序增益，导致权重配置难以真正生效，最终排序不稳定、可解释性差。

### 业界先进做法

生产级 RAG 中常见的混合检索是 dense + sparse，也就是向量检索 + BM25。向量检索负责语义相似，BM25 负责关键词、专名、数字、精确短语等字面匹配。两路召回后通过 RRF、分数归一化加权或 reranker 做融合排序，而不是把各路原始分数直接混排。

### 优化方式

后续只维护向量 + BM25 两路混合检索：

- 向量召回作为语义召回主路。
- BM25 作为关键词/全文召回主路，替代简单 `keyword_search` 的主召回职责。
- 两路各自召回较大的 candidate pool。
- 每个候选保留 `vector_score`、`bm25_score`、`matched_terms`、`retrieval_sources`。
- 使用 RRF 或归一化加权进行融合排序。
- 同一 chunk 同时被向量和 BM25 命中时给予多路命中增益。
- 融合后的候选再进入 reranker 精排。

### 带来的效果

- 混合检索链路更清晰，减少 keyword 和 full-text 职责重叠。
- 语义召回和精确词面召回互补。
- 专名、编号、金额、日期、条款号等内容更容易被召回。
- 多路命中信号能转化为排序优势。
- 分数融合更稳定，便于后续评估和调参。

## 优化点 3：Query 改写与检索意图增强

### 现在的问题

用户原始 query 往往不是最适合检索的表达。常见问题包括 query 过短、口语化、指代不清、多轮上下文缺失、缺少文档中的关键词、多个意图混在一起等。直接用原始 query 做向量和 BM25 检索，会导致召回不全或召回偏移。

### 业界先进做法

生产级 RAG 会在检索前增加 query transformation / query rewrite 层，将用户问题改写成更适合检索的表达。常见方式包括改写为独立问题、扩展同义词和关键词、生成多 query 变体、拆解多意图问题、为 BM25 生成词面检索 query，为向量检索生成语义 query。

### 优化方式

在检索前增加轻量 Query Understanding 层：

- 保留 `original_query`，用于 rerank 和最终回答。
- 生成 `semantic_query`，用于向量召回。
- 生成 `bm25_query` 和 `expanded_terms`，用于 BM25 召回。
- 对多轮对话中的指代问题改写成独立问题。
- 对多意图 query 拆成多个子 query 分别召回。
- 改写失败时回退到原始 query。
- 记录改写结果，便于调试和评估。

### 带来的效果

- 短 query、口语 query 的召回率更高。
- BM25 更容易命中文档中的同义词、专名、编号、条款词。
- 多意图问题不容易只召回其中一部分。
- 多轮对话中的“这个”“那个”等指代能变成可检索的问题。
- 向量 + BM25 混合检索的整体稳定性更好。

## 优化点 4：检索可解释性增强

### 现在的问题

当前检索结果缺少清晰的解释信息。系统返回了 chunk 和分数，但很难判断它为什么被召回：是向量命中、BM25 命中、query 改写命中，还是 rerank 后被提到前面。出现错误结果时，也不容易定位问题发生在召回、融合、重排还是上下文组装阶段。

### 业界先进做法

生产级 RAG 通常会记录并展示检索链路的关键证据，包括召回来源、命中关键词、原始分数、融合分数、rerank 前后排名、命中的 child chunk、返回的 parent chunk、使用的 query 变体和最终进入 prompt 的上下文。这样既方便调试，也方便向用户解释答案来源。

### 优化方式

为每条检索结果补充可解释信息：

- 记录 `retrieval_sources`，标明来自向量、BM25、query 改写、rerank 或多路命中。
- 记录 `vector_score`、`bm25_score`、`fusion_score`、`rerank_score`。
- 记录 BM25 命中的 `matched_terms` 和命中字段。
- 记录命中的 child chunk、聚合后的 parent chunk、标题路径和页码。
- 记录 rerank 前后排名变化。
- 在调试接口或日志中输出完整检索 trace。
- 最终回答时保留可读的来源说明和证据片段。

### 带来的效果

- 检索结果为什么出现更容易解释。
- 出错时能快速定位是召回问题、融合问题、rerank 问题还是上下文组装问题。
- 方便评估不同优化策略的真实影响。
- 提升调试效率和用户信任。
- 为后续做检索质量看板和人工标注闭环打基础。

## 优化点 5：证据感知的上下文组装与回答约束

### 现在的问题

当前生成侧把检索命中的内容拼成一段普通文本后交给 LLM。上下文里缺少清晰的证据边界、来源编号、标题路径、页码、命中分数和 child chunk 命中信息。模型虽然拿到了内容，但不知道哪些片段更重要、来自哪里、能支撑什么结论，容易出现引用不清、证据错配和基于上下文外信息回答的问题。

### 业界先进做法

生产级 RAG 通常会在生成前增加 context packing / evidence grounding 层：把召回结果组织成结构化证据包，保留来源、标题路径、页码、命中片段、父级上下文和排序信息，并在 prompt 中要求模型只基于证据回答、关键结论必须引用来源、证据不足时明确拒答。

### 优化方式

在检索结果进入 LLM 前增加 RAG Context Packer：

- 按融合分数或 rerank 分数重新排序证据。
- 对同一文档、同一标题路径下的 chunk 做合并和去重。
- 优先保留命中的 child chunk，同时补充必要 parent 上下文。
- 每段上下文增加来源编号、文档名、标题路径、页码和分数。
- 按 token budget 截断，避免按字符硬截断破坏证据。
- 在生成 prompt 中约束模型：只基于证据回答；关键结论必须引用来源；证据不足时明确说明无法判断。

### 带来的效果

- 模型更容易判断哪些内容是可用证据。
- 回答可追溯，用户能看到结论来自哪段材料。
- 减少幻觉、误引用和上下文外回答。
- 高分证据不会因为乱序或硬截断被破坏。
- 前面做的结构化切块、标题路径、混合召回和检索解释信息能真正传递到生成侧。
