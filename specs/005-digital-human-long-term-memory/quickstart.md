# 快速验证：数字人长期记忆按需召回

## 1. 静态可见性检查

```bash
rg -n "digital_human_resource_bindings|knowledge_dataset" web/src/components/digital-human api/internal/modules/digitalhuman
```

确认内部记忆知识库没有被加入资源绑定响应，也没有被加入候选列表。

## 2. 召回行为

准备一条更新时间较早但内容与当前问题高度相关的 active memory，以及多条较新的无关 memory。运行数字人并观察：

- 系统提示词不再包含固定的最近 50 条列表；
- Agent 可以调用 `retrieve_digital_human_memory`；
- 相关旧记忆出现在结果中；
- 无关新记忆不会因为“最近”自动出现。

## 3. 一致性行为

更新同一个 memory 的 content/revision 后执行投影同步，确认旧 revision 不再被返回。将 memory 标记为 invalidated 或 archived 后重试召回，确认结果为空或不包含该 memory。

## 4. 最小测试命令

```bash
cd api
gofmt -w internal/modules/digitalhuman
go test ./internal/modules/digitalhuman ./internal/modules/dataset/service ./routes/v1
```
