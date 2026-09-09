# DigitalHuman public API and landing page

This guide describes the public runtime for a published ZGI DigitalHuman. The
machine-readable contract is available from a running API service at
`/api/openapi.yaml`.

## English

### Enable and publish

An administrator must publish a DigitalHuman before either public surface can
be used. In **Access and publishing**, enable the surfaces independently:

- **Public landing page** exposes `/digital-human/{web_app_id}`.
- **API access** enables API keys bound to this DigitalHuman.

The deployment must also enable the corresponding global gates:

```env
ZGI_DIGITAL_HUMAN_PUBLIC_WEBAPP_ENABLED=true
ZGI_DIGITAL_HUMAN_EXTERNAL_API_ENABLED=true
```

Both gates default to `false`. Disabling a surface rejects new calls
immediately without deleting conversations, runs, or API keys.

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
message / tool / runtime_input events
message_end
error
run_end
```

`message_start` supplies the durable `conversation_id` and `message_id`.
`run_end` is emitted once and is the terminal event for the run. An `error`
event contains a safe public code, message, and retryability flag; internal
stack traces, provider responses, prompts, and secrets are never exposed.

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

- When relationship memory is disabled, an unauthenticated visitor receives a
  signed anonymous session and can use text chat.
- When relationship memory is enabled, chatting requires a ZGI login. The
  public configuration remains readable so the page can explain the login
  requirement.
- File upload always requires login.
- Anonymous sessions own conversations only and never create or read user
  relationship memory.
- Logged-in sessions use the account subject, so relationship memory can
  continue across conversations according to the published policy.

The landing page never receives or stores a DigitalHuman API key.

## 中文

### 启用与发布

数字人必须先发布，外部入口才能使用。在数字人的“访问与发布”区域中，可以
独立开启：

- **公开落地页**：访问路径为 `/digital-human/{web_app_id}`；
- **API 访问**：允许使用绑定到该数字人的 API Key。

部署环境还需开启对应的全局开关：

```env
ZGI_DIGITAL_HUMAN_PUBLIC_WEBAPP_ENABLED=true
ZGI_DIGITAL_HUMAN_EXTERNAL_API_ENABLED=true
```

两个开关默认均为 `false`。关闭入口会立即拒绝新调用，但不会删除已有会话、
运行记录或 API Key。

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
message / tool / runtime_input 等过程事件
message_end
error
run_end
```

`message_start` 会返回持久化后的 `conversation_id` 和 `message_id`；
`run_end` 只发送一次，是运行的最终事件。`error` 只包含安全错误码、消息和是否
可重试，不会暴露堆栈、模型响应、提示词或密钥。

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

- 关系记忆关闭时，未登录访客会获得服务端签名的匿名会话，可进行纯文本对话；
- 关系记忆开启时，公开配置仍可读取，但开始聊天必须登录 ZGI；
- 文件上传始终要求登录；
- 匿名身份只拥有会话，永远不会读取或创建用户关系记忆；
- 登录用户使用账号主体，可按发布策略在不同会话间延续关系记忆。

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
| POST | `/api/v1/digital-humans/files/upload` | Upload a chat file / 上传聊天文件 |
