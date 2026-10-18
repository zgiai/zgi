# Build an Agent landing page with the ZGI API

This guide is for a frontend developer who has no knowledge of the ZGI codebase. It builds a small Next.js landing page against a published Agent. The browser never receives the Agent API Key.

## 1. Prerequisites

In ZGI, publish an Agent, enable API access, and create an API Key for that Agent. For the default Docker deployment the public API base is `http://localhost:2679/api/v1`.

Add server-only environment variables to the Next.js application:

```dotenv
ZGI_BASE_URL=http://localhost:2679/api/v1
ZGI_AGENT_API_KEY=replace-with-your-agent-api-key
```

Do not prefix the key with `NEXT_PUBLIC_`. The recommended topology is:

```text
Browser -> your Next.js route handler -> ZGI /api/v1/*
```

## 2. Add a server-side proxy

Create `app/api/zgi/config/route.ts`:

```ts
const baseURL = process.env.ZGI_BASE_URL?.replace(/\/$/, '');
const apiKey = process.env.ZGI_AGENT_API_KEY;

export async function GET() {
  if (!baseURL || !apiKey)
    return Response.json({ message: 'ZGI server configuration is missing' }, { status: 500 });

  const upstream = await fetch(`${baseURL}/agents/config`, {
    headers: { Authorization: `Bearer ${apiKey}` },
    cache: 'no-store',
  });
  return new Response(upstream.body, {
    status: upstream.status,
    headers: { 'Content-Type': upstream.headers.get('Content-Type') ?? 'application/json' },
  });
}
```

Create `app/api/zgi/chat/route.ts`. It forwards the SSE body without buffering:

```ts
const baseURL = process.env.ZGI_BASE_URL?.replace(/\/$/, '');
const apiKey = process.env.ZGI_AGENT_API_KEY;

export const dynamic = 'force-dynamic';

// This sample assumes a trusted auth proxy strips any client-supplied value
// and injects X-Authenticated-User-ID. Replace this with your server-side
// session/JWT lookup when the application handles authentication itself.
function authenticatedExternalUserID(request: Request): string | undefined {
  return request.headers.get('X-Authenticated-User-ID')?.trim() || undefined;
}

export async function POST(request: Request) {
  if (!baseURL || !apiKey)
    return Response.json({ message: 'ZGI server configuration is missing' }, { status: 500 });

  const externalUserID = authenticatedExternalUserID(request);
  if (!externalUserID)
    return Response.json({ message: 'Authentication is required' }, { status: 401 });
  const browserPayload = await request.json();
  delete browserPayload.user;

  const upstream = await fetch(`${baseURL}/agents/chat`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${apiKey}`,
      'X-External-User-ID': externalUserID,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(browserPayload),
    cache: 'no-store',
    signal: request.signal,
  });

  const headers = new Headers({
    'Content-Type': upstream.headers.get('Content-Type') ?? 'application/json',
    'Cache-Control': 'no-cache, no-transform',
  });
  for (const name of ['X-ZGI-Conversation-ID', 'X-ZGI-Message-ID']) {
    const value = upstream.headers.get(name);
    if (value) headers.set(name, value);
  }
  return new Response(upstream.body, { status: upstream.status, headers });
}
```

Every stateful Agent proxy route must derive the same stable, opaque external user id from your authenticated session and send it as `X-External-User-ID`. Do not use an email address or phone number. Production BFF routes should also authorize access to this Agent, rate-limit requests, and enforce request-size limits.

## 3. Add an SSE parser

Create `lib/read-zgi-sse.ts`:

```ts
export interface ZGIEvent<T = Record<string, unknown>> {
  id?: string;
  event: string;
  data: T;
}

export async function readZGISSE(
  response: Response,
  onEvent: (event: ZGIEvent) => void,
): Promise<void> {
  if (!response.ok) {
    const error = await response.json().catch(() => ({ message: response.statusText }));
    throw new Error(error.message ?? `ZGI request failed (${response.status})`);
  }
  if (!response.body) throw new Error('ZGI returned an empty stream');

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';

  while (true) {
    const { value, done } = await reader.read();
    buffer += decoder.decode(value, { stream: !done }).replace(/\r\n/g, '\n');
    const frames = buffer.split('\n\n');
    buffer = frames.pop() ?? '';

    for (const frame of frames) {
      let id: string | undefined;
      let nativeEvent = 'message';
      const dataLines: string[] = [];
      for (const line of frame.split('\n')) {
        if (line.startsWith('id:')) id = line.slice(3).trim();
        else if (line.startsWith('event:')) nativeEvent = line.slice(6).trim();
        else if (line.startsWith('data:')) dataLines.push(line.slice(5).trimStart());
      }
      if (dataLines.length === 0) continue;
      const envelope = JSON.parse(dataLines.join('\n')) as {
        event?: string;
        data?: Record<string, unknown>;
      };
      onEvent({ id, event: envelope.event ?? nativeEvent, data: envelope.data ?? {} });
    }
    if (done) break;
  }
}
```

ZGI includes a native SSE `event:` and a JSON envelope with the same event name. The parser accepts both. Keep the latest non-empty `id` so a disconnected stream can be recovered.

The complete field-level event contract is machine-readable at `/api/openapi.yaml` under `x-zgi-sse-event-data-schemas`. At minimum, model these public events in the frontend:

```ts
type MessageEnd = {
  conversation_id: string;
  message_id: string;
  status:
    | 'completed' | 'stopped' | 'error' | 'failed'
    | 'waiting_approval' | 'waiting_question';
  metadata: Record<string, unknown>;
  answer?: string;
};

type ToolGovernanceDecision = {
  conversation_id: string;
  message_id: string;
  correlation_id: string;
  decision: string;
  requires_approval: false;
  reason?: string;
  risk_level?: string;
  effect?: string;
  asset_type?: string;
};

type UserInputRequested = {
  conversation_id: string;
  message_id: string;
  request_id: string;
  message?: string;
  questions: Array<{
    id: string;
    question: string;
    options?: Array<string | { label: string; description?: string }>;
  }>;
};
```

Unknown event names and payload fields must be ignored for forward compatibility. Do not display `agent_progress.activity === 'reasoning'` as model reasoning content; it is only a state label and carries no reasoning text.

## 4. Render the landing page

Create `app/agent/page.tsx`:

```tsx
'use client';

import { FormEvent, useEffect, useState } from 'react';
import { readZGISSE } from '@/lib/read-zgi-sse';

type Config = {
  name: string;
  description: string;
  icon_url: string;
  home_title: string;
  opening_statement: string;
  input_placeholder: string;
  theme_color: string;
  suggested_questions: string[];
  file_upload_enabled: boolean;
  memory_requires_user: boolean;
  external_user_required: boolean;
};

export default function AgentPage() {
  const [config, setConfig] = useState<Config>();
  const [query, setQuery] = useState('');
  const [answer, setAnswer] = useState('');
  const [conversationID, setConversationID] = useState<string>();
  const [messageID, setMessageID] = useState<string>();
  const [lastEventID, setLastEventID] = useState<string>();
  const [status, setStatus] = useState<'idle' | 'running' | 'waiting' | 'error'>('idle');
  useEffect(() => {
    fetch('/api/zgi/config', { cache: 'no-store' })
      .then(async (response) => {
        const body = await response.json();
        if (!response.ok) throw new Error(body.message);
        return body.data as Config;
      })
      .then(setConfig)
      .catch(() => setStatus('error'));
  }, []);

  async function send(event: FormEvent) {
    event.preventDefault();
    if (!query.trim() || status === 'running') return;
    setAnswer('');
    setStatus('running');

    try {
      const response = await fetch('/api/zgi/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          query,
          conversation_id: conversationID,
          response_mode: 'streaming',
        }),
      });
      await readZGISSE(response, ({ id, event, data }) => {
        if (id) setLastEventID(id);
        if (event === 'message_start') {
          setConversationID(String(data.conversation_id));
          setMessageID(String(data.message_id));
        } else if (event === 'message') {
          setAnswer((current) => current + String(data.answer ?? ''));
        } else if (event === 'message_end') {
          const next = String(data.status);
          setStatus(next.startsWith('waiting_') ? 'waiting' : next === 'completed' ? 'idle' : 'error');
        } else if (event === 'error') {
          setStatus('error');
        }
      });
    } catch {
      setStatus('error');
    }
  }

  if (!config) return <main>Loading Agent…</main>;
  return (
    <main style={{ maxWidth: 760, margin: '64px auto', padding: 24 }}>
      {config.icon_url && <img src={config.icon_url} alt="" width={56} height={56} />}
      <h1>{config.home_title || config.name}</h1>
      <p>{config.opening_statement || config.description}</p>
      <div>{answer}</div>
      <form onSubmit={send}>
        <textarea
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          placeholder={config.input_placeholder}
        />
        <button disabled={status === 'running'} style={{ background: config.theme_color }}>
          Send
        </button>
      </form>
      {config.suggested_questions.map((question) => (
        <button key={question} onClick={() => setQuery(question)}>{question}</button>
      ))}
      <small>{conversationID} {messageID} {lastEventID}</small>
    </main>
  );
}
```

This is intentionally unstyled. The API fields provide enough data to reproduce ZGI landing-page behavior while leaving layout and design-system choices to the host application.

## 5. Complete production behavior

Implement these routes in the same server-side proxy style:

| User action | ZGI endpoint |
| --- | --- |
| Upload a file | `POST /files/upload` with multipart `file`; send returned `id` in chat `file_ids` |
| Load conversations | `GET /agents/conversations?page=1&limit=20` |
| Load messages | `GET /agents/conversations/{conversation_id}/messages?page=1&limit=100` |
| Search history | `GET /agents/conversations/search?query=...&limit=20` |
| Rename conversation | `PATCH /agents/conversations/{conversation_id}` with `{ "title": "..." }` |
| Delete conversation | `DELETE /agents/conversations/{conversation_id}` |
| Stop generation | `POST /agents/conversations/{conversation_id}/stop` |
| Regenerate | `POST /agents/messages/{message_id}/regenerate` |
| Recover stream | `GET /agents/conversations/{conversation_id}/events?message_id=...&after_id=...` |
| Continue workflow question | `POST .../{message_id}/workflow-continuation` with `{ "type": "question_answer", "inputs": { ... } }` |
| Continue workflow approval | The same workflow endpoint with `type`, top-level event `approval_token`, an `action` advertised by `approval_form.actions`, and `inputs` |
| Answer a Skill prompt | `POST .../{message_id}/user-input/{request_id}/continue` with `{ "answers": { ... } }` |
| Confirm/reject an admitted Linux platform CLI call | `POST .../{message_id}/tool-governance/{correlation_id}/continue` with `{ "action": "approve" }` or `{ "action": "reject" }`; retain the original final-user header/query identity |
| Manage Agent Memory | `GET/PUT/DELETE /agents/memory...` |

Send `X-External-User-ID` on every row above, including list/search/get/update/delete, stream recovery, continuations, Memory, and file upload. ZGI stores only a workspace-and-Agent-scoped derived UUID. The legacy chat JSON, query, and multipart `user` values remain temporarily accepted; responses include deprecation headers, and a mismatch with the canonical header returns `400`.

The browser state machine should treat `completed`, `stopped`, `error`, and `failed` as terminal. A `waiting_*` status ends only the current SSE phase: retain the same conversation and message, render the stored event payload, and open the response stream returned by the matching continuation endpoint.

There are two public continuation paths. Select one from the trigger event and final waiting status:

| Trigger event | Matching wait | Continue with |
| --- | --- | --- |
| `approval_requested` / `question_answer_requested` | `waiting_approval` / `waiting_question` | workflow continuation; copy approval payload `approval_token` and render its nested `approval_form`, or submit question inputs |
| `user_input_requested` with `source=agent_workflow_question_answer` | `waiting_question` | workflow continuation with `type=question_answer`; this is a projection of a workflow question, not a Skill prompt |
| Any other `user_input_requested` | `waiting_question` | Skill user-input continuation; key `answers` by each question `id` |

Published Agent tool governance is non-interactive: `tool_governance_decision.requires_approval` is always `false` and only reports whether Agent bindings allowed or denied the invocation. Host-page coordination events belong to contextual sidebar experiences and are outside this API.

### Recovery state matrix

| State after loading conversation detail and messages | Recovery behavior |
| --- | --- |
| `runtime_status=running` with `active_message_id` | Replay that message from the event-stream beginning, then keep listening; use `after_id` only for later interruptions in the same page |
| Latest message is `waiting_approval` | Replay that message to rebuild `approval_requested`; use `metadata.workflow_runs[].approval.approval_form` only as an immediate fallback snapshot |
| Latest message is `waiting_question` | Replay that message and route the recovered question by event `source` as described above |
| Latest message is terminal | Render history only; do not open a recovery stream |

If the connection drops after `message_start`, do not immediately repeat the chat POST. Call the events endpoint with the saved `message_id` and `after_id`, deduplicate by SSE id, and append recovered `message` chunks. Retry network failures, `429`, and retryable `5xx` responses with bounded exponential backoff; do not retry `400/401/403/404`. Recoverable events are retained for 24 hours.

After a page reload, load message history before recovering. Do not apply a stored `after_id` to a freshly rendered page: the cursor does not prove which chunks are present in the DOM. Replay the newest running or waiting message without `after_id`; replace its historical answer with the first replayed text chunk and append subsequent chunks. Message history is newest-first, so sort it by `created_at` before rendering.

For approval UI, read the credential from `event.data.approval_token` and the display model from `event.data.approval_form`. Render `approval_form.content` safely, build inputs from `approval_form.fields`, and render only `approval_form.actions`. `approval_form.token` is a compatibility fallback, not the canonical event path. If `event.data.ui_approval_allowed` is false, credentials are redacted and the approval must be completed through its configured channel.

```ts
const data = event.data;
const form = data.approval_form ?? {};
const approval = {
  ...form,
  formId: data.approval_form_id ?? form.id,
  token: data.approval_token ?? form.token,
  uiAllowed: data.ui_approval_allowed !== false,
};
```

Submit the selected action id, exact `approval.token`, and collected `inputs` to workflow continuation. Render the card when `approval_requested` arrives, and enable it as soon as the matching `message_end.status` becomes `waiting_approval`; do not wait for the HTTP response body to close. Disable submission while the continuation request is in flight and after `approval_form.expiration_at` or `approval_expired`. If retained events and message metadata cannot reconstruct a token, request id, fields, or actions, show an unrecoverable-state prompt; never invent approval actions or continuation identifiers.

## 6. Error handling and API reference

Before an SSE stream starts, errors have a matching HTTP status and JSON body:

```json
{ "code": "199001", "message": "Invalid parameter" }
```

- `400`: invalid input, missing `X-External-User-ID`, or a mismatch with a legacy `user` value
- `401`: missing, invalid, expired, inactive, or revoked key
- `403`: permission denied
- `404`: Agent, publication, conversation, or message not found in this caller scope
- `409`: a mutable resource revision conflicted
- `413` / `415`: uploaded file is too large / unsupported
- `429`: rate or upstream limit; retry with exponential backoff
- `500`: internal error; log the response code and request id, not the API Key

Do not automatically retry `400`, `401`, `403`, or `404`. An uncertain retry of a chat POST may create another message. Once streaming, consume `error` and `message_end` instead of expecting another HTTP response.

The running ZGI server exposes the machine-readable Agent contract at `/api/openapi.yaml` and Swagger UI at `/swagger/index.html`.
