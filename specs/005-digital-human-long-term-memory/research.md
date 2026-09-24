# 研究记录：数字人长期记忆召回

## 决策 1：不复用 `digital_human_resource_bindings`

当前 `/digital-humans/:id/draft/bindings` 会返回数字人的全部资源绑定，前端会把所有 `knowledge_dataset` 记录渲染到知识库区域。候选接口也会查询工作区的全部 `datasets`。因此将内部知识库写入该绑定表会直接暴露给用户。

采用数字人长期记忆内部配置保存投影知识库 ID，由运行时后端读取；不进入资源绑定快照和普通知识库候选链路。

## 决策 2：数据库是事实源，知识库是只读投影

`digital_human_memories` 已经包含 active/invalidated/archived、revision、memory epoch、scope 和 visibility 等权威字段。向量文档只保存用于检索和一致性判断的来源 metadata，不改变事实表。

投影同步采用最终一致语义。本期在召回边界惰性修复；同一入口可由后续后台任务复用。召回阶段仍需根据数据库当前 active/status/scope 做防陈旧校验。

## 决策 3：复用现有知识库检索执行器

现有 `KnowledgeRetrievalService` 已支持 vector、hybrid、BM25、rerank、top-k、阈值和来源摘要。长期记忆只增加一个后端专用入口，用内部知识库 ID 调用同一检索服务，不让 Agent 传 dataset ID。

## 决策 4：专用 Skill，不改变普通 Agent 知识库语义

现有 `agent-knowledge` Skill 默认面向普通 Agent，并从 Agent 配置解析知识库。数字人长期记忆具有不同的所有权、scope 和不可见性，采用 `digital-human-memory` 专用运行时 Skill/Tool 更安全。

## 当前实现事实（设计前基线与落地结果）

- 基线的 [`memory_runtime.go`](../../api/internal/modules/digitalhuman/memory_runtime.go) 在 `Prepare` 阶段固定读取 50 条记忆；落地后已移除该查询，改为运行时 Skill 工具。
- [`memory_repository.go`](../../api/internal/modules/digitalhuman/memory_repository.go) 使用 `updated_at DESC, id DESC LIMIT ?`，不使用当前问题。
- [`agent-knowledge/SKILL.md`](../../api/internal/modules/skills/catalog/agent-knowledge/SKILL.md) 已规定 query、top_k 和 retrieval_mode 的工具使用方式。
- [`knowledge_retrieval_service.go`](../../api/internal/modules/dataset/service/knowledge_retrieval_service.go) 提供知识库召回和来源结果格式；数字人运行时通过内部授权开关调用，普通调用不能读取内部 provider。
- [`digital-human-resource-bindings.tsx`](../../web/src/components/digital-human/digital-human-resource-bindings.tsx) 将所有 knowledge dataset 绑定展示给用户。

## 未采用的方案

### 直接复用资源绑定

会被前端展示，并且用户可以修改，不满足“用户无感”。

### 把内部知识库标记为普通 dataset 后只在一个接口过滤

知识库列表、文件关联、候选、权限和详情接口较多，容易遗漏。除非后续增加全局 system-owned dataset 过滤，否则不作为首选。

### 保留最近 50 条作为隐式兜底

会重新引入与当前问题无关的上下文污染，并且使向量召回效果无法评估。
