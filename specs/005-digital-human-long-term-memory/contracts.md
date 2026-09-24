# 契约：数字人长期记忆检索 Skill

## 工具标识

```text
Skill: digital-human-memory
Tool: retrieve_digital_human_memory
```

## 输入

```json
{
  "query": "string, required",
  "top_k": "integer, optional, 1..20",
  "retrieval_mode": "vector | hybrid, optional"
}
```

不接受 `digital_human_id`、`dataset_id`、组织/工作区 ID、scope、embedding model、reranker 或权限绕过参数。

## 输出

```json
{
  "query": "当前检索 query",
  "status": "success | no_results | no_config | projection_unavailable | error",
  "context": "供模型参考的上下文",
  "result_count": 0,
  "source_summary": [],
  "context_blocks": [],
  "warnings": []
}
```

结果中的来源 ID 和 revision 用于运行时审计与一致性校验；最终用户是否看到内部 ID 由 Agent 输出策略决定，Skill 不得泄露内部知识库 ID。

## 可见性契约

- 内部知识库不出现在数字人资源绑定响应中。
- 内部知识库不出现在知识库候选、知识库列表和普通 Agent 配置响应中。
- 只有数字人运行时服务可以解析内部知识库 ID。
- 所有召回结果必须经过数字人身份、租户、工作区、授权 scope 和当前 active revision 校验。
