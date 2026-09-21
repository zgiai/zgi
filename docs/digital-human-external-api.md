# DigitalHuman public API and landing page

This guide describes the public runtime for a published ZGI DigitalHuman. The
machine-readable contract is available from a running API service at
`/api/digital-human-openapi.yaml`.

For a copy-ready Next.js BFF, robust SSE parser, recovery state machine, and
production checklist, use the [DigitalHuman API quickstart](digital-human-api-quickstart.md).

## English

### Enable and publish

An administrator must publish a DigitalHuman before either public surface can
be used. Open the DigitalHuman's standalone **External access** page and enable
the surfaces independently:

- **Public landing page** exposes `/digital-human/{web_app_id}`.
- **API access** enables API keys bound to this DigitalHuman.

No deployment-wide feature flag is required. Access is controlled per published
DigitalHuman from the standalone **External access** page. Disabling a surface
rejects new calls immediately without deleting conversations, runs, or API keys.

### Authentication and end-user identity

Use the API key as a Bearer token:

```http
Authorization: Bearer zgi_...
```

The plaintext key is returned only once when it is created. Store it in a
secret manager and never place it in browser code.

Every stateful API request also supplies `user`. It is a stable, opaque,
case-sensitive end-user identifier with a maximum length of 128 characters.
Leading and trailing whitespace is removed. Do not use an email address, phone
number, or other directly identifying value.

The API key identifies the calling integration; `user` owns conversations,
files, and relationship memory. Consequently:

- one key may serve many end users;
- different `user` values cannot access each other's data;
- rotating the API key does not change an end user's data identity;
- the same `user` value used with another DigitalHuman is a different subject.

The raw `user` value is not returned and is not stored in logs, metrics, or
audit events.

### Start or continue a chat

```bash
curl --no-buffer \
  --request POST "https://zgi.example.com/api/v1/digital-humans/chat" \
  --header "Authorization: Bearer $ZGI_DIGITAL_HUMAN_API_KEY" \
  --header "Content-Type: application/json" \
  --data '{
    "request_id": "20a2952d-776d-41c9-a1e2-88739013cd43",
    "user": "customer_123",
    "query": "Summarize our next step",
    "response_mode": "streaming"
  }'
```

To continue a conversation, also send `conversation_id`; send
`parent_message_id` when the client maintains an explicit message branch.
Uploaded `file_ids` can be attached to the request.

`request_id` is idempotent within one DigitalHuman and end-user subject. A
retry with the same payload reuses the run. Reusing the ID with different
content returns HTTP `409`.

The response is `text/event-stream` and includes `X-ZGI-Run-ID`. Important
events are:

```text
run_accepted
message_start
message / message_retract / agent_progress / agent_intermediate_answer
file_parse_* / skill_* / tool_governance_decision / client_action_required
memory_* / runtime_input_* / workflow and node events
message_end
error
run_end
```

For DigitalHuman task mutations, the generic `tool_governance_decision` audit
event is kept internally for recovery and audit but is not sent to the public
DigitalHuman SSE stream. Handle the task's `approval_*` / workflow events and
the resulting task projection instead. Ordinary skill and tool approvals still
use `tool_governance_decision`.

`message_start` supplies the durable `conversation_id` and `message_id`.
`run_end` is emitted once and is the terminal event for the run. An `error`
event contains a safe public code, message, and retryability flag; internal
stack traces, provider responses, prompts, and secrets are never exposed.
The exact payload for every event name is published in `/api/digital-human-openapi.yaml`
under `x-zgi-sse-event-data-schemas`, using the same process-event families as
the Agent API. Clients must preserve unknown fields, ignore unknown events,
and never execute `client_action_required` without an explicit host allow-list.

### Resume an interrupted stream

Persist the last SSE `id` and the `X-ZGI-Run-ID` value. Reconnect with:

```bash
curl --no-buffer \
  "https://zgi.example.com/api/v1/digital-humans/runs/$RUN_ID/events?user=customer_123" \
  --header "Authorization: Bearer $ZGI_DIGITAL_HUMAN_API_KEY" \
  --header "Last-Event-ID: $LAST_EVENT_ID"
```

The event ID is opaque. Do not parse or increment it. Clients should reconnect
only while the run is non-terminal and use bounded exponential backoff.

Run state is also available without opening a stream:

```bash
curl \
  "https://zgi.example.com/api/v1/digital-humans/runs/$RUN_ID?user=customer_123" \
  --header "Authorization: Bearer $ZGI_DIGITAL_HUMAN_API_KEY"
```

### Stop a run or submit runtime input

```bash
curl --request POST \
  "https://zgi.example.com/api/v1/digital-humans/runs/$RUN_ID/stop" \
  --header "Authorization: Bearer $ZGI_DIGITAL_HUMAN_API_KEY" \
  --header "Content-Type: application/json" \
  --data '{"user":"customer_123"}'
```

```bash
curl --request POST \
  "https://zgi.example.com/api/v1/digital-humans/runs/$RUN_ID/runtime-input" \
  --header "Authorization: Bearer $ZGI_DIGITAL_HUMAN_API_KEY" \
  --header "Content-Type: application/json" \
  --data '{
    "user": "customer_123",
    "client_input_id": "868e8000-7c82-4542-a093-870a982e553a",
    "content": "Proceed with option B"
  }'
```

Use a new UUID for each logical runtime input. Repeating the same
`client_input_id` is safe.

### Conversations and files

The API supports listing, searching, reading, renaming, and deleting
conversations, plus reading message history. GET and DELETE endpoints receive
`user` as a query parameter; PATCH endpoints receive it in the JSON body.

Upload a file with multipart form data:

```bash
curl --request POST \
  "https://zgi.example.com/api/v1/digital-humans/files/upload" \
  --header "Authorization: Bearer $ZGI_DIGITAL_HUMAN_API_KEY" \
  --form "user=customer_123" \
  --form "file=@./brief.pdf"
```

A file is bound to the DigitalHuman workspace and derived end-user subject.
It cannot be reused by another `user` or another DigitalHuman.

### Public landing page

The canonical URL is `/digital-human/{web_app_id}`. The legacy
`/digital-human/{web_app_id}/chat` URL redirects to it.

- An unauthenticated visitor receives a signed anonymous session and can use
  text chat without relationship memory.
- File upload always requires login.
- Anonymous sessions own conversations only and never create or read user
  relationship memory.
- Logged-in sessions use the account subject, so relationship memory is enabled
  by default across conversations. Memory and automatic capture remain under
  the user's relationship settings; DigitalHuman configuration cannot override
  them.

The landing page never receives or stores a DigitalHuman API key.

## 中文

### 启用与发布

数字人必须先发布，外部入口才能使用。在数字人的独立“外接”页面中，可以独立开启：

- **公开落地页**：访问路径为 `/digital-human/{web_app_id}`；
- **API 访问**：允许使用绑定到该数字人的 API Key。

不需要额外开启部署级全局开关。访问权限由数字人独立“外接”页面按数字人
逐个控制。关闭入口会立即拒绝新调用，但不会删除已有会话、运行记录或 API Key。

### 认证与终端用户身份

API Key 通过 Bearer 方式传递：

```http
Authorization: Bearer zgi_...
```

Key 明文只在创建时返回一次，应保存在密钥管理系统中，不能放入浏览器代码。

所有有状态接口还必须传入 `user`。它应是稳定、不透明、大小写敏感的业务用户
标识，最长 128 字符，服务端会去除首尾空格。请勿使用邮箱、手机号等直接身份
信息。

API Key 代表调用方和审计主体，`user` 才是会话、文件及关系记忆的数据主体。
因此：

- 一个 Key 可以服务多个终端用户；
- 不同 `user` 之间的数据完全隔离；
- 轮换 Key 不会改变同一 `user` 的数据身份；
- 同一 `user` 在不同数字人下默认是不同主体。

原始 `user` 不会被返回，也不会写入日志、指标或审计事件。

### 创建或继续对话

```bash
curl --no-buffer \
  --request POST "https://zgi.example.com/api/v1/digital-humans/chat" \
  --header "Authorization: Bearer $ZGI_DIGITAL_HUMAN_API_KEY" \
  --header "Content-Type: application/json" \
  --data '{
    "request_id": "20a2952d-776d-41c9-a1e2-88739013cd43",
    "user": "customer_123",
    "query": "总结下一步工作",
    "response_mode": "streaming"
  }'
```

继续已有会话时增加 `conversation_id`；客户端维护显式消息分支时再传
`parent_message_id`。上传后的 `file_ids` 也可随请求发送。

`request_id` 在“数字人＋终端用户主体”范围内幂等：相同载荷重试会复用原
运行；同一个 ID 对应不同载荷时返回 HTTP `409`。

响应类型为 `text/event-stream`，响应头包含 `X-ZGI-Run-ID`。主要事件为：

```text
run_accepted
message_start
message / message_retract / agent_progress / agent_intermediate_answer
file_parse_* / skill_* / tool_governance_decision / client_action_required
memory_* / runtime_input_* / workflow 与 node 等过程事件
message_end
error
run_end
```

数字人任务变更的通用 `tool_governance_decision` 事件仍会在服务端保留，用于审计和断线恢复，
但不会发送到数字人公开 SSE。客户端应处理任务对应的 `approval_*` / workflow 事件以及最终任务结果。
普通技能和工具审批仍使用 `tool_governance_decision`。

`message_start` 会返回持久化后的 `conversation_id` 和 `message_id`；
`run_end` 只发送一次，是运行的最终事件。`error` 只包含安全错误码、消息和是否
可重试，不会暴露堆栈、模型响应、提示词或密钥。
`/api/digital-human-openapi.yaml` 的 `x-zgi-sse-event-data-schemas` 给出了每个事件名的精确
载荷，并与 Agent API 使用相同的过程事件分组。客户端需要保留未知字段、忽略
未知事件，且只能通过明确的宿主白名单执行 `client_action_required`。

### 断线恢复

客户端应保存最近一个 SSE `id` 和 `X-ZGI-Run-ID`。重连示例：

```bash
curl --no-buffer \
  "https://zgi.example.com/api/v1/digital-humans/runs/$RUN_ID/events?user=customer_123" \
  --header "Authorization: Bearer $ZGI_DIGITAL_HUMAN_API_KEY" \
  --header "Last-Event-ID: $LAST_EVENT_ID"
```

事件 ID 是不透明值，客户端不能解析或自行递增。仅在运行未终止时重连，并采用
有上限的指数退避。也可以通过
`GET /api/v1/digital-humans/runs/{run_id}?user=...` 查询当前状态。

停止运行和提交运行时输入分别使用：

```text
POST /api/v1/digital-humans/runs/{run_id}/stop
POST /api/v1/digital-humans/runs/{run_id}/runtime-input
```

运行时输入需提供新的 UUID `client_input_id`；同一逻辑输入重复提交相同 ID 是
安全的。

### 会话与文件

API 支持会话列表、搜索、查询、改名、删除和消息历史。GET/DELETE 接口通过
查询参数传 `user`，PATCH 接口通过 JSON 正文传 `user`。

文件使用 multipart 上传：

```bash
curl --request POST \
  "https://zgi.example.com/api/v1/digital-humans/files/upload" \
  --header "Authorization: Bearer $ZGI_DIGITAL_HUMAN_API_KEY" \
  --form "user=customer_123" \
  --form "file=@./brief.pdf"
```

文件归属当前数字人的工作空间及派生用户主体，不能跨 `user` 或跨数字人复用。

### 公开落地页

规范地址为 `/digital-human/{web_app_id}`；旧地址
`/digital-human/{web_app_id}/chat` 会重定向到规范地址。

- 未登录访客会获得服务端签名的匿名会话，可进行不使用关系记忆的纯文本对话；
- 文件上传始终要求登录；
- 匿名身份只拥有会话，永远不会读取或创建用户关系记忆；
- 登录用户使用账号主体，默认启用跨会话关系记忆；是否启用记忆和自动提取完全由用户自己的关系设置控制，数字人配置无权覆盖。

公开落地页不会接收或保存数字人 API Key。

## Endpoint reference / 接口索引

| Method | Path | Purpose / 用途 |
|---|---|---|
| GET | `/api/v1/digital-humans/config` | Published configuration / 已发布配置 |
| POST | `/api/v1/digital-humans/chat` | Start or continue a streamed chat / 创建或继续流式对话 |
| GET | `/api/v1/digital-humans/runs/{run_id}` | Get run state / 查询运行状态 |
| GET | `/api/v1/digital-humans/runs/{run_id}/events` | Resume SSE / 恢复事件流 |
| POST | `/api/v1/digital-humans/runs/{run_id}/stop` | Stop a run / 停止运行 |
| POST | `/api/v1/digital-humans/runs/{run_id}/runtime-input` | Submit runtime input / 提交运行时输入 |
| GET | `/api/v1/digital-humans/conversations` | List conversations / 会话列表 |
| GET | `/api/v1/digital-humans/conversations/search` | Search conversations / 搜索会话 |
| GET/PATCH/DELETE | `/api/v1/digital-humans/conversations/{id}` | Manage a conversation / 管理会话 |
| GET | `/api/v1/digital-humans/conversations/{id}/messages` | Message history / 消息历史 |
| POST | `/api/v1/digital-humans/conversations/{conversation_id}/messages/{message_id}/workflow-continuation` | Submit a task approval and resume the paused chat / 提交任务审批并恢复暂停的对话 |
| POST | `/api/v1/digital-humans/conversations/{conversation_id}/messages/{message_id}/user-input/{request_id}/continue` | Answer a model-generated question and resume the same Agent Loop / 回答模型问题并继续原 Agent Loop |
| POST | `/api/v1/digital-humans/files/upload` | Upload a chat file / 上传聊天文件 |
| GET/POST | `/api/v1/digital-humans/tasks` | List or create user-owned tasks / 查询或创建用户私有任务 |
| GET/PATCH | `/api/v1/digital-humans/tasks/{task_id}` | Read or update one user-owned task / 查询或更新用户私有任务 |
| POST | `/api/v1/digital-humans/tasks/{task_id}/actions/{action}` | Change task lifecycle / 执行任务生命周期操作 |
| GET | `/api/v1/digital-humans/tasks/{task_id}/runs` | Read safe task runs and result summaries / 查询执行记录和安全结果摘要 |
| GET | `/api/v1/digital-humans/tasks/{task_id}/activities` | Read task activity history / 查询任务活动记录 |

### User-owned task management / 用户私有任务

Tasks created through the API belong to the supplied `user`, never to the API
key itself. The same key can safely serve many users. Every task request uses
the same `user` value as chat and conversation requests, and the value is
validated but never returned or persisted as raw text.

API-created tasks use the task center as their primary result destination. The
returned task includes `delivery_target: "task_center"` and keeps
`delivery_fallback: "task_center"` for clients that share the same response
model with channel-created tasks. Channel-created tasks instead bind a private
or group conversation as their primary destination; their results are sent back
to that conversation while the task center remains the fallback record.

Create a manual task:

```bash
curl --request POST "https://zgi.example.com/api/v1/digital-humans/tasks" \
  --header "Authorization: Bearer $ZGI_DIGITAL_HUMAN_API_KEY" \
  --header "Idempotency-Key: 8e5a6f5a-7b5d-4d20-8b63-cc47a40fba0e" \
  --header "Content-Type: application/json" \
  --data '{
    "user": "customer_123",
    "name": "Follow up on requirements",
    "goal": "Summarize the latest requirement changes and propose next steps",
    "schedule": {"type": "manual"},
    "priority": "normal",
    "delivery_mode": "task_only"
  }'
```

List and update only that user's tasks:

```bash
curl "https://zgi.example.com/api/v1/digital-humans/tasks?user=customer_123&page=1&page_size=20" \
  --header "Authorization: Bearer $ZGI_DIGITAL_HUMAN_API_KEY"

curl --request PATCH "https://zgi.example.com/api/v1/digital-humans/tasks/$TASK_ID" \
  --header "Authorization: Bearer $ZGI_DIGITAL_HUMAN_API_KEY" \
  --header "Idempotency-Key: 2d6a6e90-6fe8-4ac7-a605-08c424ce9d31" \
  --header "Content-Type: application/json" \
  --data '{
    "user": "customer_123",
    "expected_revision": 1,
    "priority": "high"
  }'
```

Lifecycle actions use the task's current `revision` and return `409` with
`digital_human.task.conflict` when another request changed it. Supported
actions are `confirm`, `decline`, `run_now`, `pause`, `resume`, `retry`,
`cancel`, and `archive`:

For a task created from a connected channel, the runs response also includes
`result_delivery_status`. A failed or unknown channel delivery does not erase
the execution result: `result_summary` remains available in the task center and
`result_delivery_error` contains a bounded safe explanation.

```bash
curl --request POST \
  "https://zgi.example.com/api/v1/digital-humans/tasks/$TASK_ID/actions/run_now" \
  --header "Authorization: Bearer $ZGI_DIGITAL_HUMAN_API_KEY" \
  --header "Idempotency-Key: 7c5e1774-2b2b-4d6e-9e7f-9a5e0b66509c" \
  --header "Content-Type: application/json" \
  --data '{"user":"customer_123","expected_revision":2}'
```

### Conversational task approvals / 对话任务审批

Task proposals and task mutations produced by a DigitalHuman conversation are
paused until the end user approves them. The chat SSE contains an
`approval_requested` event and the message ends in `waiting_approval`. A client
must render the approval form in place of the composer, then continue the same
message; do not send a second natural-language confirmation message:

```bash
curl --request POST \
  "https://zgi.example.com/api/v1/digital-humans/conversations/$CONVERSATION_ID/messages/$MESSAGE_ID/workflow-continuation" \
  --header "Authorization: Bearer $ZGI_DIGITAL_HUMAN_API_KEY" \
  --header "Content-Type: application/json" \
  --data '{
    "type": "approval",
    "user": "customer_123",
    "approval_token": "<token-from-approval_requested>",
    "action": "approve",
    "inputs": {"name": "Optional edited task name", "goal": "Optional edited goal"}
  }'
```

The continuation returns the normal `message_start`, `approval_result`,
`message`, `message_end`, and `error` SSE events. `decline` cancels a new
proposal; declining an update leaves the existing task unchanged. The token,
conversation, message, and `user` are checked together, so a token cannot be
used to approve another user's task.

### Model-generated user questions / 模型追问

When the SSE stream ends with `waiting_question` and emits
`user_input_requested`, the client must answer the question through the
dedicated continuation endpoint below. This is not a new chat request and is
not a task/workflow approval:

```bash
curl --request POST \
  "https://zgi.example.com/api/v1/digital-humans/conversations/$CONVERSATION_ID/messages/$MESSAGE_ID/user-input/$REQUEST_ID/continue" \
  --header "Authorization: Bearer $ZGI_DIGITAL_HUMAN_API_KEY" \
  --header "Content-Type: application/json" \
  --data '{
    "user": "customer_123",
    "answers": {"water_reminder_intent": "keep_active"}
  }'
```

The server validates the conversation, message leaf, request id, and user
identity, then resumes the original Agent Loop and returns the normal SSE
events. Answers are not appended as a synthetic user message.

The runs endpoint exposes bounded execution metadata and a safe
`result_summary`; it never exposes execution traces, tool arguments, model
responses, prompts, API keys, or another user's context:

```bash
curl "https://zgi.example.com/api/v1/digital-humans/tasks/$TASK_ID/runs?user=customer_123" \
  --header "Authorization: Bearer $ZGI_DIGITAL_HUMAN_API_KEY"
```

For the hosted landing page, authenticated sessions use the equivalent
`/api/v1/digital-human-webapps/{web_app_id}/tasks...` endpoints. The browser
does not send `user`; the server derives the account subject from the signed
WebApp session. Anonymous sessions receive an authentication error and never
register task tools. The console chat page uses the same owner boundary at
`/console/api/digital-humans/{digital_human_id}/my-tasks...`; the existing
`/console/api/digital-humans/{digital_human_id}/tasks` page remains the shared
DigitalHuman task manager.

The API, console, and authenticated landing page therefore share task
ownership semantics while keeping channel-conversation tasks and DigitalHuman
shared tasks out of user-private lists. Task results are available in task
details and run history; they are not automatically injected into another
conversation.
