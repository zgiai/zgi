# 事故记录：知识库「知识图谱」构建卡在 0%（2026-09-15）

- 服务器：`ai-young` / `101.132.23.206`（`https://zgi.lingyoungai.com/`）
- 部署目录：`/data/www/zgi-pre`（分支 `main`，commit `38bd93ec9`）
- 运行镜像：`zgi-pre-api:aiyoung-20260609-161247`（构建于 2026-09-06）
- 现象：知识库 `官庄小学成长档案库`（`7821015f-5da1-479f-9c19-92fea3ddb0c5`）的知识图谱总体进度 0%，四个阶段全部「等待中」
- 排查方式：只读（日志 + 只读 SQL；唯一一次写操作在显式 `BEGIN … ROLLBACK` 事务内，已回滚）

## 结论（一句话）

新文档已经同步完成、可抽取的分段也已就绪，但**新 run `04861b35` 从未被提升为 `processing`**（没有生成 outbox 事件），而每次点击「重建图谱」都会被**旧的、文档已被删除的失败 run `759f5d87` 抢先"续跑"并再次失败**，于是永远停在 0%。

## 关键数据

数据集 `datasets`：

| 字段 | 值 |
| --- | --- |
| graph_status | `queued` |
| graph_progress | 0 |
| graph_current_run_id | NULL |
| graph_revision | 2 |
| graph_available_revision | **NULL** |
| graph_updated_at | 2026-09-15 15:30:20 |

run `graphflow_runs`（该数据集仅 2 条）：

| id | revision | mode | status | 说明 |
| --- | --- | --- | --- | --- |
| `759f5d87-…` | 1 | rebuild | **failed** | 11:35 手工重建；其文档 `cefb6a59` 已被删除 |
| `04861b35-…` | 2 | build | **pending** | 15:26:26 新文档同步批次；文档 `42c1876e` 正常 |

outbox `graph_outbox_events`：**不存在 run `04861b35` 的事件**（7 条历史事件全部 `confirmed`）。
唯一的新事件是 15:26:54 才为**旧** run 创建的：

```
id 3ca14fcc payload {"mode":"rebuild","run_id":"759f5d87-…","graph_revision":1}
```

文档/分段 `documents` + `document_segments`：

- 现有文档 `42c1876e-4fcd-41fe-8304-050b029f03c8`（`官庄小学.txt`，1006 字）→ 1 个分段，`completed`
- 已删除文档 `cefb6a59-5bcc-41c4-aaa7-45be519dc032` → **13 个孤儿分段**仍在库中（`documents` 行已不存在）

asset ref `data_library_knowledge_base_asset_refs`：

```
7529ffc5-… sync_status=synced, graph_sync_status=waiting,
graph_run_id=04861b35-…, sync_batch_id=51df0347-…
```

## 时间线

| 时间 | 事件 |
| --- | --- |
| 11:35:41 | `POST /graph/rebuild` → 创建 rebuild run `759f5d87`（revision 1） |
| 11:35:~ | 该 run 的 extraction 反复失败；文档 `cefb6a59` 被删除 |
| 15:26:24 | 新文档同步任务 `LOCAL:dataset_ref:sync`（`e1649b05`）开始 |
| 15:26:25.253 | 创建批次 run `04861b35`（revision 2，`pending`） |
| 15:26:25.272 | ref 被标记 `synced` / `graph_sync_status=waiting` |
| 15:26:25.29+ | 云端向量写入 + 删除旧文档向量；`copyAssetToDataset` 接着删除旧文档 |
| **15:26:25** | **`TryStartSyncBatch` 静默失败：run 未被提升，未生成 outbox 事件** |
| 15:26:54.913 | 用户点击「重建图谱」`POST /graph/rebuild`（HTTP 202） |
| 15:26:54.871 | `RebuildDataset` 找到失败 run（`revision > 0`）→ 调用 `Retry` → 旧 run 被提升并生成事件 |
| 15:26:56.828 | outbox 消费：`failed to enqueue task: task ID conflicts with another task` |
| 15:27–15:30 | 旧 extraction 任务重试 3 次 |
| 15:30:20.415 | `no segments found for document cefb6a59-… after 3 retries` → 旧 run 再次 failed |
| 15:30:20.480 | 数据集回到 `queued`，`graph_current_run_id=NULL` |

## 因果链

1. `LifecycleService.RebuildDataset`（`api/internal/modules/dataset/graphflow/lifecycle_service.go:653`）在开启新重建前，会先"续跑"任意失败 run：

   ```go
   Where("… status = ? AND graph_revision > ?", Failed, availableRevision)
   ```

   当 `graph_available_revision` 为 NULL 时，代码用 `0` 兜底（:671-674），于是 revision 1 的旧失败 run 永远命中。

2. `promoteNextRunTx`（:375）同一比较也用 `0` 兜底（:1419-1422 的 `aggregateDatasetGraphState` 亦然），所以运行期语义是"NULL ≈ 0"，旧 run 始终是可续跑的失败前驱。

3. 旧 run 的文档早已被删除。`extraction_handler.go:174` 用 `document_id` 查分段，得到 0 条；13 条孤儿分段挂在旧 document_id 上，所以在 `:194` 判定 `no segments found` 并落 failed。

4. 同一数据集的 run 严格串行（`idx_graphflow_runs_one_processing_per_dataset`，且 `promoteNextRunTx` 只提升 revision 最小的 pending）。旧 run 每次被「重建图谱」复活都会占住 processing 槽位，新 run `04861b35` 永远轮不到。

5. 新 run 之所以一直 `pending`，是因为 **`TryStartSyncBatch` 这一步没有成功**：run 行存在、run item 存在、ref 已 `synced`，但既没有 outbox 事件，也没有 `graph_sync_status=queued`（提升路径 `:472` 的副作用），数据集 `graph_current_run_id` 也为 NULL —— 提升事务整体未生效。

6. 失败被吞掉的位置：

   - `dataset_ref_sync_runner.go:153` — `defer r.tryStartGraphSyncBatch(...)`
   - `dataset_ref_sync_runner.go:397` — `_ = batchLifecycle.TryStartSyncBatch(...)`

   即使 `promoteNextRunTx` 返回 error（例如与 `RegisterSyncBatchDocument` 的并发写在同一事务快照下判定 `runDispatchReadyTx=false`，见 `lifecycle_service.go:490` 的 `totalRefs == 0 → false`），也**既不重试也不记日志**。这正是"没有任何报错的静默卡死"。

## 修复建议

### A. 立即解封（数据修复，需写入）

按顺序二选一或组合：

1. **让批次 run 走正常提升路径**：为 run `04861b35` 补一条 `graph_outbox_events`（`event_type='graph_run'`、`aggregate_key='run:04861b35-…'`、`run_id` 指向该 run、`status='pending'`、`available_at=now()`）。
   outbox 协调器（`OutboxReconciler`，每 ~2s）会接管：创建 extraction/alignment/graph_sync/vector_sync 任务，处理**仍然存在**的文档 `42c1876e`（其分段 `completed`，具备抽取条件）。
2. **或**把数据集 `graph_available_revision` 置为 `1`，使旧失败 run 不再成为"可续跑失败前驱"；随后新的 `POST /graph/rebuild` 会走 `StartRebuild`（mode=rebuild）路径。
   （已在 `BEGIN … ROLLBACK` 事务中验证：置 1 后 `resume_candidates=0`、`failed_pred_gate=0`。）

建议随后清理：

- 删除 13 条孤儿分段（`document_segments where document_id='cefb6a59-…'`），它们属于已删除文档，只会污染统计（当前数据集分段数显示 14，实际有效 1）。
- 处理旧 failed run `759f5d87`，避免其被反复复活。

> 数据修复前建议先备份 `datasets` / `graphflow_runs` / `graph_outbox_events` / `document_segments` 的相关行。

### B. 代码修复（防止再次发生）

1. **`dataset_ref_sync_runner.go`**：不要吞掉批次提升错误 —— 至少 `logger.ErrorContext`，并且不要在 `defer` 里 fire-and-forget；把 `tryStartGraphSyncBatch` 放到成功路径显式调用、失败时让任务重试（`TryStartSyncBatch` 本身是幂等的）。
2. **`LifecycleService.RebuildDataset` / `promoteNextRunTx` / `aggregateDatasetGraphState`**：把 `graph_available_revision IS NULL` 的兜底语义显式化（统一 `COALESCE(…,0)` 或统一 NULL 语义），避免"查询命中旧失败 run、运行期又认为它不算前驱"的分歧。
3. **`RebuildDataset` 续跑失败 run 前**：校验该 run 的 `document_id` 是否仍有存活文档/分段；对"文档已删除"的失败 run 直接标记 superseded，而不是 `Retry`。
4. **`enqueueOrReactivateGraphFlowTask`（`worker/task_enqueue.go:13`）**：`removed == false` 时静默返回 `nil`（asynq 任务 ID 冲突），会让调用方以为入队成功。应返回可诊断错误或明确降级日志。
5. 增加巡检：清理/报警"数据集存在 failed 前驱 + 存在更高 revision 的 pending run 超过 N 分钟"的组合。

## 处置记录（2026-09-15 15:49）

已备份相关行到 `ai-young:/data/www/backups/graphflow-stuck-20260915-154907.txt`，随后执行了一条写入：

```sql
UPDATE datasets SET graph_available_revision = 1, graph_updated_at = now()
 WHERE id = '7821015f-5da1-479f-9c19-92fea3ddb0c5'
   AND graph_available_revision IS NULL;
```

### 为什么是这一步（而不是直接补 outbox 事件）

`RunOutboxHandler.Process`（`worker/run_outbox_handler.go:44-49`）对仍处于 `pending` 的 run **直接返回错误**：
"Only the lifecycle promoter may claim a queued run"，并且该事件会被反复重试。所以只补一条 outbox 事件不可能放行。

真正的闸门是 `promoteNextRunTx` 的 **failed-predecessor gate**（`lifecycle_service.go:419-436`）：
mode=`build` 时，只要存在 `failed` run 满足 `graph_available_revision < revision < candidate.revision`，提升就永远返回 `nil, nil`。
把 `graph_available_revision` 置为 1（= 承认 revision 1 这个失败发布已被取代）正好清除该闸门，
这也与 `TestLifecycleServiceRebuildCanRecoverPastFailedRevision` 表达的"recover past a failed revision"设计一致。

> 补充：子任务分析还确认了 `ReconcileActiveRuns`（`:771-808`，`runtime.go:137` 注册、10s ticker）会周期性提升 pending run，
> 且 `TryStartSyncBatch` 只处理**最低 revision** 的 pending run —— 所以低 revision 的失效 run 会永久挡住后续 run。

### 生效结果

| 时间 | 事件 |
| --- | --- |
| 15:49:11.866 | sweeper 将 run `04861b35` 提升为 `processing`，`graph_current_run_id` 指向它 |
| 15:49:11.872 | 生成并确认 outbox 事件 `4e777c78` |
| 15:49:16.808 | 创建 extraction 任务 `6c0cdf6f`，目标文档 `42c1876e`（**新文档，分段存在**） |
| 15:49:17.18 | 任务已进入 LLM gateway，选中 route 列表，开始为分段 `6031c469` 抽取 |
| 15:49:17.206 | 任务失败 → run 落 failed，`graph_error_code=graph_task_failed` |

**第一层问题（卡 0%、任务不派发）已修复**：管线现在正确派发到仍然存在的文档与分段。
**但暴露出第二层、独立的阻塞**：

```
llm extraction failed: failed to select provider: model not found:
current workspace has no enabled route for model "gpt-4o"
```

数据集配置的抽取模型是 `entity_model = gpt-4o` / `openai`：

```sql
SELECT entity_model, entity_model_provider FROM datasets WHERE id='7821015f-…';
-- gpt-4o | openai
```

而该组织当前**已启用的**路由（`llm_routes`，`is_enabled AND deleted_at IS NULL`）里没有任何一条提供 `gpt-4o`：

| provider | route_type | 模型（models 数组） |
| --- | --- | --- |
| openai-compatible | USER_OWNED | ZDTF-VL |
| openai-compatible | USER_OWNED | InnoSpark-235B |
| doubao-speech | USER_OWNED | seed-tts-2.0, volc.seedasr.sauc.duration |
| qwen | USER_OWNED | gte-rerank-v2, qwen-vl-plus, **qwen3-max**, qwen3-rerank, qwen3-vl-plus, qwen3.6-flash, qwen3.6-plus, qwen3.7-max, qwq-plus, text-embedding-v3, text-embedding-v4 |
| openai-compatible | USER_OWNED（15:26:23 更新） | text-embedding-3-large, text-embedding-3-small, **deepseek-v4-pro**, deepseek-v4-flash, qwen3.7-plus |
| zgi-cloud | 官方 | （models 为空，且路由日志显示 "does not support model gpt-4o"） |

唯一提供过 `gpt-4o` 的两条路由 `e3559736-…`（openai-compatible）与 `ae27b075-…`（openai）
**都已在 2026-03-22 软删除**（`deleted_at` 非空），所以 `gpt-4o` 现在无处可路由。

### 第二层修复（待确认，尚未执行）

任选其一：

1. **改数据集抽取模型**（推荐，与 UI「抽取模型」选择器一致）：
   把 `entity_model` / `entity_model_provider` 改为可用模型，例如 `qwen3-max` / `qwen` 或 `deepseek-v4-pro` / `openai-compatible`。
   注意 `dataset_service.go:806-812` 会要求"更换抽取模型需确认"，即需要带 `ConfirmGraphRebuild` 走更新接口（相当于再次全量重建）。
2. **恢复一条支持 `gpt-4o` 的路由**（在控制台重新启用/新增 openai 兼容路由并列出 `gpt-4o`）。

> 影响面：同一组织下还有多个数据集同样配置 `entity_model=gpt-4o`（`浠水小学成长档案库`、`明溪第一小学成长档案库`
> 状态为 failed；`老师教学设计独家笔记`、`作文知识库`、`数学`、`心理小妙招`、`家校沟通桥梁基本规章制度`
> 状态为 waiting_content）。在路由恢复或模型改掉之前，这些库重建图谱都会以同样的 "model not found: gpt-4o" 失败。

## 第二层：新建知识库也失败（2026-09-15 16:04）

新建知识库 `57711c03-6856-4cc3-9a09-3c0d10a0620c`（同名"官庄小学成长档案库"，
抽取模型 `deepseek-v4-flash` → 后改为 `qwen3.6-flash`，嵌入 `text-embedding-v4`）后，
四个阶段中的前三个**最终都 100% 成功**，卡在最后一步：

| task | status | progress |
| --- | --- | --- |
| extraction | completed | 100 |
| alignment | completed | 100 |
| graph_sync | completed | 100 |
| vector_sync | **failed** | 0 |

run `db3252a0-c5ef-4865-bb68-70aef175b1d4`：status=failed，**progress=90**，
`error_code=graph_task_failed`，
`error_message=vector sync failed: graph embedding dimension mismatch: skip retry for the task`。

### 根因：run 快照里的嵌入维度取自"文件资产"，而不是知识库的嵌入模型

`graphflow_runs` 里这条 run 的快照是自相矛盾的：

```
embedding_model_provider = qwen
embedding_model          = text-embedding-v4
embedding_dimension      = 3072        ← 这是 openai/text-embedding-3-large 的维度
```

而该文件对应的**文件资产**确实是另一套嵌入：

```
data_library_document_assets 377e0e17-…  generation_no=1
  embedding_provider=openai  embedding_model=text-embedding-3-large  embedding_dimension=3072
```

取值来源就是这里（**bug**）：

```go
// api/internal/modules/datalibrary/worker/dataset_ref_sync_runner.go:324-337
batchLifecycle.RegisterSyncBatchDocument(ctx, graphflow.SyncBatchDocumentRequest{
    …
    EmbeddingProvider:  stringPtrValue(dataset.EmbeddingModelProvider), // ← 用知识库的模型（对）
    EmbeddingModel:     stringPtrValue(dataset.EmbeddingModel),         // ← 用知识库的模型（对）
    EmbeddingDimension: intPtrValue(asset.EmbeddingDimension),         // ← 却用文件的维度（错）
})
```

同文件里已经有正确的取值函数 `datasetRefSyncEmbeddingTarget(dataset, asset)`
（`:723-743`，优先知识库模型），但**维度**这一项漏了，仍然沿用资产的值。

### 为什么它在 vector_sync 才炸

`vector_sync_handler.go:329-341` 在真正写向量前会做一致性校验：

```go
actualDimension := embedSvc.GetDimension()   // 解析出的实际维度
if len(vecs) > 0 { actualDimension = len(vecs[0]) }
if graphRun != nil {
    if err := validateExtractionRunSnapshot(graphFlowTask, graphRun, dataset, actualDimension); err != nil {
        return err                 // ← errEmbeddingDimensionMismatch
    }
}
```

`validateExtractionRunSnapshot`（`extraction_handler.go:66-68`）里：

```go
if run.EmbeddingDimension > 0 && actualDimension > 0 && run.EmbeddingDimension != actualDimension {
    return errEmbeddingDimensionMismatch
}
```

所以：run 记录的是 3072，实际模型 `text-embedding-v4` 产出 1024 → 判定不一致 → 任务以
`SkipRetry` 终止（`vector_sync_handler.go:361`），整条 run 落到 90% failed。

> 这也解释了 UI 的文案为什么对不上：界面说"图谱同步在重试 3 次后仍未成功。请点击修复图谱"，
> 但这个错是**确定性的数据不一致**，重试不是解决手段；而"修复图谱"
> （`LifecycleService.ReconcileRun`）也只是重跑，**换不出新的 run 快照**，所以修不好。

### 为什么"新建知识库"这条路也走不通

文件资产在上传到知识库**之前**就已经用工作区默认嵌入（这里是 `openai/text-embedding-3-large`，3072）
算好了嵌入；而新建知识库默认用 `qwen/text-embedding-v4`（1024）。
两者维度不同 → 只要不修这个取值 bug，**任何**新建知识库放过这个文件，都会以同样的
`graph embedding dimension mismatch` 结束。

## 第三层修复建议

1. **代码（根因）**：`dataset_ref_sync_runner.go:336` 的 `EmbeddingDimension`
   应改为按**知识库嵌入目标**解析出的维度，而不是 `asset.EmbeddingDimension`。
   同文件 `datasetRefSyncEmbeddingTarget` 已经解析出 provider/model，
   需要一并解析该模型的维度（走嵌入服务/模型注册表）。
2. **文案/行为**：`errEmbeddingDimensionMismatch` 属确定性失败，UI 不应再引导"重试"或"修复图谱"，
   应提示"知识库嵌入模型与文件资产嵌入模型不一致"。
3. **数据快照一致性**：图 run 的 `embedding_*` 三个字段应在创建时由**同一来源**（知识库）一次性写入，
   避免 provider/model 与 dimension 来自不同来源。

## 附：本次排查使用的只读命令

```bash
ssh ai-young 'docker logs zgi-pre-api-1 --since 2h | grep -E "graphflow|extraction"'
ssh ai-young 'psql -h pgm-uf6lmuto7166433r.pg.rds.aliyuncs.com -U aiyoung -d aiyoung \
  -c "select id,graph_revision,mode,status from graphflow_runs where dataset_id=…"'
```
