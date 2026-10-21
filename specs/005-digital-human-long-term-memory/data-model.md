# 数据模型：数字人长期记忆召回

## 事实源：`digital_human_memories`

本功能不修改该表。它继续保存记忆内容、revision、status、visibility、memory class/type、business scope、memory epoch 和 content digest。

## 内部召回配置

`digital_human_memory_states` 增加一个内部知识库 ID，并与数字人一对一；知识库自身的 `retrieval_config` 保存沿用的检索参数：

| 字段 | 说明 |
| --- | --- |
| `digital_human_id` | 数字人主体 ID |
| `retrieval_config` | 内部 dataset 的持久化检索配置，不允许 Agent 任意覆盖治理参数 |
| `memory_dataset_id` | nullable；首次召回时惰性创建，创建失败不写入事实表 |
| `updated_at` | 配置更新时间 |

该关联属于召回基础设施，不改变长期记忆事实表和沉淀流程。

## 知识库投影文档 metadata

每个可检索文档或 chunk 必须包含：

- `memory_id`
- `memory_revision`
- `content_digest`
- `document_id` / `dataset_id`
- active/status/scope 由数据库事实源在召回后再次校验，不信任向量 metadata

投影只允许当前 active 版本进入正常检索；更新使用稳定的 `memory_id + revision/content_digest` 幂等替换；失效、归档和 supersede 删除或禁用旧投影。

## 运行时检索请求

后端自动补齐：

- `digital_human_id`
- `organization_id`
- `workspace_id`
- 授权 memory scopes
- 内部 `memory_dataset_id`
- 持久化检索配置

Agent 只提供：

- `query`（必填）
- `top_k`（可选，后端限制）
- `retrieval_mode`（可选，限制为 vector/hybrid）
