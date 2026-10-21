# 实施计划：数字人长期记忆按需召回

**分支**：`yzh/feat/digital-human-next` | **日期**：2026-09-22 | **规格**：[spec.md](./spec.md)

## 摘要

将数字人长期记忆从运行前固定注入最近 50 条，改为 Agent 可自主调用的专用检索 Skill。数据库中的长期记忆保持事实源，系统内部知识库只作为当前可见记忆的向量/混合检索投影。内部知识库 ID 不进入 `digital_human_resource_bindings`，不出现在前端绑定和候选列表；运行时以后端身份、scope 和内部配置为准调用现有知识库检索服务。

## 技术背景

**语言/版本**：Go 1.26.2、TypeScript

**主要依赖**：Gin、GORM、PostgreSQL、现有 ChatRuntime SkillLoop、现有 Dataset RetrievalService、Weaviate/vector store

**存储**：PostgreSQL 事实源和内部关联配置；现有知识库索引保存向量投影

**测试**：Go `testing`、GORM 仓储测试、现有 runtime/route 测试、前端静态脚本测试

**目标平台**：Linux 服务端

**项目类型**：Go Web 服务 monorepo

**性能目标**：单次召回复用现有知识库检索预算，默认 top-k 5，Agent 传入 top-k 最大 20；不得额外加载最近 50 条全文上下文

**约束**：不修改长期记忆沉淀和事实表；内部知识库不对用户暴露；权限必须在检索前由后端确定；不做最近 50 条隐式兜底

**规模/范围**：每个数字人一个内部知识库投影；本期只实现召回链路和投影同步接口，不改变普通知识库和 Agent 知识库语义

## 宪章检查

- [x] 本计划及生成的过程文档使用简体中文。
- [x] 新增业务源码使用英文标识符、日志和注释；用户可见文案不新增硬编码。
- [x] 未新增前端用户可见文案。
- [x] 任务包含源码、契约和过程文档语言合规检查。

## 结构决策

- `api/internal/modules/digitalhuman/` 保留数字人所有权、运行时工具和内部配置解析。
- `api/internal/modules/dataset/service/` 继续作为知识库检索执行层，只增加数字人专用后端调用入口。
- `digital_human_resource_bindings` 只保存用户可配置资源，不承载内部记忆知识库。
- `web/` 不新增长期记忆知识库 UI；当前绑定 UI 不需要知道内部投影。

## 实施阶段

### 阶段 1：运行时召回

1. 删除 `DefaultMemoryRuntimeProvider.Prepare` 的最近 50 条查询和系统提示词渲染。
2. 增加数字人专用检索工具，Agent 只提供 query/top_k/retrieval_mode。
3. 后端根据当前 run 补齐数字人、租户、工作区、授权 scope 和内部知识库配置。
4. 调用现有知识库检索执行器并输出统一的 status/context/source summary/context blocks。

### 阶段 2：内部配置与投影

1. 在内部记忆配置中保存内部知识库 ID和投影状态，不通过资源绑定表暴露。
2. 在召回前按需建立/修复幂等投影，使用 memory ID、revision、content digest 更新向量文档；这样本期不改动沉淀写路径。
3. active 以外的状态删除或禁用投影；每次召回都以数据库当前 active 视图做最终校验。
4. 将投影修复设计成可重复执行的同步边界，后续可由后台 worker 复用同一入口扩展异步重试。

### 阶段 3：验证

1. 验证无固定 50 条注入、Agent 能按 query 调用、top-k 有界。
2. 验证资源绑定列表、候选列表和知识库列表不包含内部投影。
3. 验证跨数字人隔离、scope 隔离和陈旧 revision 过滤。

## 复杂度跟踪

| 事项 | 必要原因 | 更简单方案为何不可用 |
| --- | --- | --- |
| 内部知识库关联与用户资源绑定分离 | 当前绑定列表和候选接口会直接暴露记录 | 给普通绑定增加 hidden 标记会要求覆盖多个公开数据面，容易漏过滤 |
| 投影同步任务 | 向量索引不能成为事实源，且记忆更新后需要清理旧 revision | 每次召回临时从数据库生成向量会增加延迟且无法复用现有知识库索引 |
