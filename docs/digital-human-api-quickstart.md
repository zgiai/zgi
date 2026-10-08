# Build a production-quality DigitalHuman demo with the ZGI API

This guide is for a frontend developer who does not know the ZGI codebase. It builds a branded, multi-turn, recoverable Next.js chat page against one published DigitalHuman. The browser never receives the DigitalHuman API key and never chooses its own ZGI `user` identity.

The example uses the Next.js App Router and TypeScript, but the same boundary applies to any web or mobile client:

```text
Browser or mobile UI
        |
        | your authenticated application session
        v
Your BFF / API route  -- server-only API key -->  ZGI /api/v1/digital-humans/*
```

## 1. Prerequisites

In ZGI:

1. Publish a DigitalHuman.
2. Open **External access**, enable API access, and create an API key.
3. Keep the plaintext key shown at creation time. It cannot be displayed again.

For the default Docker deployment, the public API base is `http://localhost:2679/api/v1`. Add server-only variables to your Next.js application:

```dotenv
ZGI_BASE_URL=http://localhost:2679/api/v1
ZGI_DIGITAL_HUMAN_API_KEY=replace-with-your-digital-human-api-key
```

Never prefix the key with `NEXT_PUBLIC_`. Do not send it to browser code, mobile binaries, logs, analytics, URLs, or error trackers.

Install no special SSE package: modern browsers and Node runtimes can consume the streamed `fetch` response directly.

## 2. Establish the trusted identity boundary

Every stateful DigitalHuman endpoint requires `user`. It is the data owner for runs, conversations, files, and relationship memory—not the API key.

Use a stable, opaque, case-sensitive identifier of at most 128 characters. Do not use an email address or phone number. A good pattern is a random internal UUID or a keyed, irreversible mapping from your account id.

Create `lib/zgi-server.ts`:

```ts
import 'server-only';

export const zgiBaseURL = process.env.ZGI_BASE_URL?.replace(/\/$/, '');
export const zgiAPIKey = process.env.ZGI_DIGITAL_HUMAN_API_KEY;
export const zgiConfigured = Boolean(zgiBaseURL && zgiAPIKey);

export function requireZGIConfiguration(): void {
  if (!zgiBaseURL || !zgiAPIKey) throw new Error('ZGI server configuration is missing');
}

// This sample assumes a trusted reverse proxy removes any public copy of this
// header and injects the authenticated account id. Replace this function with
// your own server-side session/JWT lookup when Next.js owns authentication.
export function authenticatedZGIUser(request: Request): string | undefined {
  const value = request.headers.get('X-Authenticated-User-ID')?.trim();
  return value && value.length <= 128 ? value : undefined;
}

export function zgiHeaders(extra?: HeadersInit): Headers {
  requireZGIConfiguration();
  const headers = new Headers(extra);
  headers.set('Authorization', `Bearer ${zgiAPIKey}`);
  return headers;
}

export function authenticationRequired(): Response {
  return Response.json(
    { code: 'host.authentication.required', message: 'Authentication is required', retryable: false },
    { status: 401 },
  );
}

export function configurationError(): Response {
  return Response.json(
    { code: 'host.configuration.missing', message: 'ZGI server configuration is missing', retryable: false },
    { status: 500 },
  );
}
```

One DigitalHuman key may serve many end users. Rotating the key does not move or split data because ZGI derives ownership from `(workspace, DigitalHuman, user)`. Conversely, the same `user` string used with another DigitalHuman is intentionally isolated.

## 3. Add server-side proxy routes

The BFF must derive the same `user` on chat, recovery, stop, runtime input, conversation, message, search, rename, delete, and upload routes. Delete any browser-supplied `user` before forwarding.

### Published configuration

Create `app/api/zgi/config/route.ts`:

```ts
import { configurationError, zgiBaseURL, zgiConfigured, zgiHeaders } from '@/lib/zgi-server';

export const dynamic = 'force-dynamic';

export async function GET() {
  if (!zgiConfigured) return configurationError();
  const upstream = await fetch(`${zgiBaseURL}/digital-humans/config`, {
    headers: zgiHeaders(),
    cache: 'no-store',
  });
  return new Response(upstream.body, {
    status: upstream.status,
    headers: { 'Content-Type': upstream.headers.get('Content-Type') ?? 'application/json' },
  });
}
```

The configuration route is not end-user stateful and therefore does not send `user`.

### Start or continue a chat

Create `app/api/zgi/chat/route.ts`:

```ts
import {
  authenticatedZGIUser,
  authenticationRequired,
  configurationError,
  zgiBaseURL,
  zgiConfigured,
  zgiHeaders,
} from '@/lib/zgi-server';

export const dynamic = 'force-dynamic';

export async function POST(request: Request) {
  if (!zgiConfigured) return configurationError();
  const user = authenticatedZGIUser(request);
  if (!user) return authenticationRequired();

  const browserPayload = (await request.json()) as Record<string, unknown>;
  delete browserPayload.user;

  const upstream = await fetch(`${zgiBaseURL}/digital-humans/chat`, {
    method: 'POST',
    headers: zgiHeaders({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ ...browserPayload, user }),
    cache: 'no-store',
    signal: request.signal,
  });

  const headers = new Headers({
    'Content-Type': upstream.headers.get('Content-Type') ?? 'application/json',
    'Cache-Control': 'no-cache, no-transform',
    'X-Accel-Buffering': 'no',
  });
  const runID = upstream.headers.get('X-ZGI-Run-ID');
  if (runID) headers.set('X-ZGI-Run-ID', runID);

  return new Response(upstream.body, { status: upstream.status, headers });
}
```

The BFF forwards the body without buffering. The browser saves `X-ZGI-Run-ID` before reading the first stream frame, so it can recover even if the connection drops before `run_accepted` arrives.

### Recover an interrupted run

Create `app/api/zgi/runs/[runId]/events/route.ts`:

```ts
import {
  authenticatedZGIUser,
  authenticationRequired,
  configurationError,
  zgiBaseURL,
  zgiConfigured,
  zgiHeaders,
} from '@/lib/zgi-server';

export const dynamic = 'force-dynamic';

export async function GET(
  request: Request,
  { params }: { params: Promise<{ runId: string }> },
) {
  if (!zgiConfigured) return configurationError();
  const user = authenticatedZGIUser(request);
  if (!user) return authenticationRequired();
  const { runId } = await params;
  const cursor = request.headers.get('Last-Event-ID');

  const headers = zgiHeaders();
  if (cursor) headers.set('Last-Event-ID', cursor);
  const upstream = await fetch(
    `${zgiBaseURL}/digital-humans/runs/${encodeURIComponent(runId)}/events?user=${encodeURIComponent(user)}`,
    { headers, cache: 'no-store', signal: request.signal },
  );

  const responseHeaders = new Headers({
    'Content-Type': upstream.headers.get('Content-Type') ?? 'application/json',
    'Cache-Control': 'no-cache, no-transform',
    'X-Accel-Buffering': 'no',
  });
  const returnedRunID = upstream.headers.get('X-ZGI-Run-ID');
  if (returnedRunID) responseHeaders.set('X-ZGI-Run-ID', returnedRunID);
  return new Response(upstream.body, { status: upstream.status, headers: responseHeaders });
}
```

SSE ids are opaque. Forward `Last-Event-ID` exactly; do not parse it as a timestamp or integer.

### Stop the current run

Create `app/api/zgi/runs/[runId]/stop/route.ts`:

```ts
import {
  authenticatedZGIUser,
  authenticationRequired,
  configurationError,
  zgiBaseURL,
  zgiConfigured,
  zgiHeaders,
} from '@/lib/zgi-server';

export async function POST(
  request: Request,
  { params }: { params: Promise<{ runId: string }> },
) {
  if (!zgiConfigured) return configurationError();
  const user = authenticatedZGIUser(request);
  if (!user) return authenticationRequired();
  const { runId } = await params;
  const upstream = await fetch(
    `${zgiBaseURL}/digital-humans/runs/${encodeURIComponent(runId)}/stop`,
    {
      method: 'POST',
      headers: zgiHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ user }),
      cache: 'no-store',
    },
  );
  return new Response(upstream.body, {
    status: upstream.status,
    headers: { 'Content-Type': upstream.headers.get('Content-Type') ?? 'application/json' },
  });
}
```

Use the same pattern for the remaining endpoints listed in section 7. In production, add host-application authorization, per-user rate limits, request-size limits, and request correlation ids at this BFF boundary.

## 4. Parse the SSE stream correctly

Create `lib/read-zgi-sse.ts`:

```ts
export type ZGIEvent<T = Record<string, unknown>> = {
  id?: string;
  event: string;
  data: T;
};

export class ZGIRequestError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code: string,
    readonly retryable: boolean,
  ) {
    super(message);
  }
}

export async function readZGISSE(
  response: Response,
  onEvent: (event: ZGIEvent) => void,
): Promise<void> {
  if (!response.ok) {
    const error = await response.json().catch(() => ({
      code: `http.${response.status}`,
      message: response.statusText,
      retryable: response.status === 429 || response.status >= 500,
    }));
    throw new ZGIRequestError(
      error.message ?? 'ZGI request failed',
      response.status,
      error.code ?? `http.${response.status}`,
      Boolean(error.retryable),
    );
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

ZGI sends both a native SSE `event:` and a JSON envelope with the same event name. Dispatch on the JSON value when present. Keep the latest non-empty `id`; ignore unknown events and unknown payload fields for forward compatibility.

The minimum client state machine uses these six DigitalHuman lifecycle events:

| Event | Client action |
| --- | --- |
| `run_accepted` | Save `run_id`, `request_id`, `status`, and `created` |
| `message_start` | Save `conversation_id` and `message_id`; clear existing text when `replace=true` |
| `message` | Append `answer` only when its `message_id` matches the active message |
| `message_end` | End the current message phase and inspect its status |
| `error` | Show the safe message, log only the stable code, and inspect `retryable` |
| `run_end` | Stop reconnecting and record the final durable run status |

ZGI also emits the same typed process-event families used by the Agent API. They are fully described by `x-zgi-sse-event-data-schemas` in `/api/digital-human-openapi.yaml`:

| Family | Events to handle in a polished client |
| --- | --- |
| Progress | `agent_progress`, `agent_intermediate_answer`, `message_retract` |
| User interaction | `user_input_requested`, `runtime_input_*`, `client_action_required` |
| Files and tools | `file_parse_*`, `skill_*`, `tool_governance_decision` (ordinary tools) |
| Relationship memory | `memory_create`, `memory_update`, `memory_delete`, `memory_clear` |
| Workflow | `workflow_*`, `node_*`, `approval_*`, `question_answer_*`, `iteration_*`, `loop_*`, `text_chunk`, `text_replace` |

A basic client can ignore these after saving the event id. A richer client should render only the documented public summaries, treat host actions as allow-listed data, and never display hidden reasoning or arbitrary HTML.

DigitalHuman task mutations are the exception to the generic tool-governance
row: their internal `tool_governance_decision` event is retained for audit and
replay but is not emitted on the public DigitalHuman SSE stream. Clients should
render the corresponding `approval_*` / workflow events and then refresh the
task projection. Ordinary tool approvals continue to use
`tool_governance_decision`.

## 5. Build the client state machine

Create `lib/digital-human-state.ts`:

```ts
import type { ZGIEvent } from './read-zgi-sse';

export type ChatPhase =
  | 'idle'
  | 'accepting'
  | 'streaming'
  | 'finalizing'
  | 'reconnecting'
  | 'completed'
  | 'stopped'
  | 'error';

export type ChatState = {
  phase: ChatPhase;
  runId?: string;
  conversationId?: string;
  messageId?: string;
  requestId?: string;
  lastEventId?: string;
  answer: string;
  activity?: string;
  intermediateAnswer?: string;
  pendingInput?: Record<string, unknown>;
  artifacts: Array<{ id: string; name: string; url?: string }>;
  error?: string;
};

export const initialChatState: ChatState = { phase: 'idle', answer: '', artifacts: [] };

export function applyZGIEvent(state: ChatState, event: ZGIEvent): ChatState {
  const data = event.data;
  const lastEventId = event.id || state.lastEventId;

  switch (event.event) {
    case 'client_send':
      return {
        ...state,
        phase: 'accepting',
        requestId: String(data.request_id),
        answer: '',
        error: undefined,
      };
    case 'client_reconnecting':
      return { ...state, phase: 'reconnecting', error: undefined };
    case 'run_accepted':
      return {
        ...state,
        phase: 'accepting',
        runId: String(data.run_id),
        requestId: String(data.request_id),
        lastEventId,
      };
    case 'message_start':
      return {
        ...state,
        phase: 'streaming',
        conversationId: String(data.conversation_id),
        messageId: String(data.message_id),
        answer: data.replace ? '' : state.answer,
        lastEventId,
      };
    case 'message':
      if (state.messageId && String(data.message_id) !== state.messageId) return state;
      return { ...state, answer: state.answer + String(data.answer ?? ''), lastEventId };
    case 'message_retract': {
      const candidate = String(data.answer ?? '');
      const answer = candidate && state.answer.endsWith(candidate)
        ? state.answer.slice(0, -candidate.length)
        : state.answer;
      return { ...state, answer, lastEventId };
    }
    case 'agent_progress':
      return { ...state, activity: String(data.activity ?? data.status ?? ''), lastEventId };
    case 'agent_intermediate_answer':
      return {
        ...state,
        intermediateAnswer: data.delta
          ? (state.intermediateAnswer ?? '') + String(data.content ?? '')
          : String(data.content ?? ''),
        lastEventId,
      };
    case 'user_input_requested':
      return { ...state, pendingInput: data, activity: String(data.message ?? 'Input required'), lastEventId };
    case 'runtime_input_applied':
    case 'runtime_input_follow_up_started':
      return { ...state, pendingInput: undefined, activity: String(data.status ?? 'Input applied'), lastEventId };
    case 'file_parse_start':
    case 'file_parse_end':
    case 'file_parse_error':
      return { ...state, activity: `${String(data.name ?? 'File')}: ${String(data.status ?? event.event)}`, lastEventId };
    case 'skill_call_start':
    case 'skill_call_end':
    case 'skill_call_error':
      return { ...state, activity: `${String(data.tool_name ?? 'Tool')}: ${String(data.status ?? event.event)}`, lastEventId };
    case 'skill_artifact_created': {
      const artifact = {
        id: String(data.artifact_id ?? data.file_id),
        name: String(data.filename ?? 'Generated file'),
        url: data.download_url ? String(data.download_url) : data.url ? String(data.url) : undefined,
      };
      return { ...state, artifacts: [...state.artifacts, artifact], lastEventId };
    }
    case 'workflow_started':
    case 'node_started':
    case 'node_finished':
    case 'workflow_paused':
    case 'workflow_resumed':
    case 'workflow_finished':
    case 'workflow_failed':
      return { ...state, activity: String(data.title ?? data.status ?? event.event), lastEventId };
    case 'client_action_required':
      // Never navigate or mutate assets automatically. Route only explicitly allow-listed actions.
      return { ...state, activity: String(data.label ?? data.reason ?? 'Action required'), lastEventId };
    case 'message_end': {
      const status = String(data.status);
      return {
        ...state,
        phase: status === 'completed' ? 'finalizing' : status === 'stopped' ? 'stopped' : 'error',
        error: status === 'error' || status === 'failed' ? 'The response could not be completed' : state.error,
        lastEventId,
      };
    }
    case 'error':
      return {
        ...state,
        phase: 'error',
        error: String(data.message ?? 'Request failed'),
        lastEventId,
      };
    case 'run_end': {
      const status = String(data.status);
      return {
        ...state,
        phase: status === 'succeeded' ? 'completed' : status === 'canceled' ? 'stopped' : 'error',
        lastEventId,
      };
    }
    default:
      return { ...state, lastEventId };
  }
}
```

`message_end` closes the current chat-message stream. `run_end` is the durable terminal boundary. Keep listening after a successful `message_end` until `run_end` arrives.

## 6. Render a branded, recoverable page

Create `app/digital-human/page.tsx`:

```tsx
'use client';

import { FormEvent, useEffect, useReducer, useRef, useState } from 'react';
import { applyZGIEvent, initialChatState } from '@/lib/digital-human-state';
import { readZGISSE, ZGIRequestError } from '@/lib/read-zgi-sse';

type Config = {
  name: string;
  description: string;
  icon_url: string;
  home_title: string;
  opening_statement: string;
  input_placeholder: string;
  suggested_questions: string[];
  file_upload_enabled: boolean;
};

export default function DigitalHumanPage() {
  const [config, setConfig] = useState<Config>();
  const [query, setQuery] = useState('');
  const [state, dispatch] = useReducer(applyZGIEvent, initialChatState);
  const stateRef = useRef(state);
  const abortRef = useRef<AbortController | null>(null);
  stateRef.current = state;

  useEffect(() => {
    fetch('/api/zgi/config', { cache: 'no-store' })
      .then(async response => {
        const body = await response.json();
        if (!response.ok) throw new Error(body.message ?? 'Could not load DigitalHuman');
        return body.data as Config;
      })
      .then(setConfig)
      .catch(error => dispatch({ event: 'error', data: { message: error.message } }));
    return () => abortRef.current?.abort();
  }, []);

  async function consume(response: Response) {
    await readZGISSE(response, event => dispatch(event));
  }

  async function send(event: FormEvent) {
    event.preventDefault();
    const text = query.trim();
    if (!text || ['accepting', 'streaming', 'finalizing', 'reconnecting'].includes(state.phase)) return;

    abortRef.current?.abort();
    abortRef.current = new AbortController();
    const requestId = crypto.randomUUID();
    dispatch({ event: 'client_send', data: { request_id: requestId } });

    let acceptedRunID: string | undefined;
    try {
      const response = await fetch('/api/zgi/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          request_id: requestId,
          query: text,
          conversation_id: state.conversationId,
          response_mode: 'streaming',
        }),
        signal: abortRef.current.signal,
      });
      const runId = response.headers.get('X-ZGI-Run-ID');
      acceptedRunID = runId ?? undefined;
      if (acceptedRunID) {
        dispatch({ event: 'run_accepted', data: { run_id: acceptedRunID, request_id: requestId } });
      }
      setQuery('');
      await consume(response);
    } catch (error) {
      if (abortRef.current?.signal.aborted) return;
      const current = stateRef.current;
      const recoveryRunID = acceptedRunID ?? current.runId;
      if (recoveryRunID) await recover(recoveryRunID, current.lastEventId);
      else dispatch({ event: 'error', data: { message: error instanceof Error ? error.message : 'Request failed' } });
    }
  }

  async function recover(runId: string, lastEventId?: string) {
    dispatch({ event: 'client_reconnecting', data: {} });
    try {
      const response = await fetch(`/api/zgi/runs/${encodeURIComponent(runId)}/events`, {
        headers: lastEventId ? { 'Last-Event-ID': lastEventId } : undefined,
        cache: 'no-store',
      });
      await consume(response);
    } catch (error) {
      const message = error instanceof ZGIRequestError && error.retryable
        ? 'Connection interrupted. Retry recovery.'
        : error instanceof Error ? error.message : 'Recovery failed';
      dispatch({ event: 'error', data: { message } });
    }
  }

  async function stop() {
    if (!state.runId) return;
    const response = await fetch(`/api/zgi/runs/${encodeURIComponent(state.runId)}/stop`, { method: 'POST' });
    if (!response.ok) {
      const body = await response.json().catch(() => ({}));
      dispatch({ event: 'error', data: { message: body.message ?? 'Could not stop the run' } });
    }
  }

  if (!config) return <main aria-busy="true">Loading DigitalHuman…</main>;
  const busy = ['accepting', 'streaming', 'finalizing', 'reconnecting'].includes(state.phase);

  return (
    <main style={{ maxWidth: 800, margin: '48px auto', padding: 24 }}>
      <header>
        {config.icon_url && <img src={config.icon_url} alt="" width={56} height={56} />}
        <h1>{config.home_title || config.name}</h1>
        <p>{config.opening_statement || config.description}</p>
      </header>

      <section aria-live="polite" aria-busy={busy} style={{ minHeight: 180, whiteSpace: 'pre-wrap' }}>
        {state.answer || 'Choose a suggestion or ask a question.'}
      </section>

      {state.activity && <p role="status">{state.activity}</p>}
      {state.intermediateAnswer && <aside aria-label="Intermediate result">{state.intermediateAnswer}</aside>}
      {state.pendingInput && <aside>More input is required to continue this run.</aside>}
      {state.artifacts.length > 0 && (
        <ul>
          {state.artifacts.map(file => (
            <li key={file.id}>{file.url ? <a href={file.url}>{file.name}</a> : file.name}</li>
          ))}
        </ul>
      )}

      {state.error && (
        <div role="alert">
          {state.error}
          {state.runId && <button onClick={() => recover(state.runId!, state.lastEventId)}>Reconnect</button>}
        </div>
      )}

      <form onSubmit={send}>
        <label htmlFor="question">Your question</label>
        <textarea
          id="question"
          value={query}
          onChange={event => setQuery(event.target.value)}
          placeholder={config.input_placeholder || 'Ask anything'}
          disabled={busy}
          rows={4}
          style={{ display: 'block', width: '100%' }}
        />
        <button disabled={busy || !query.trim()}>
          Send
        </button>
        {busy && <button type="button" onClick={stop}>Stop</button>}
      </form>

      <nav aria-label="Suggested questions">
        {config.suggested_questions.map(question => (
          <button key={question} type="button" disabled={busy} onClick={() => setQuery(question)}>
            {question}
          </button>
        ))}
      </nav>
      <small>Phase: {state.phase}</small>
    </main>
  );
}
```

This is deliberately light on visual styling. The published configuration supplies the title, copy, icon, accent, placeholder, and suggested questions; apply your own design system around the same state machine.

For a polished demo, add:

- A conversation sidebar with loading, empty, search, rename, and delete states.
- Markdown rendering with raw HTML disabled or sanitized.
- File chips, upload progress, type/size validation, and retry.
- Responsive layout, visible keyboard focus, `aria-live` status, and mobile composer behavior.
- Bounded reconnect backoff with a visible retry action.
- Deduplication by non-empty SSE id and active `message_id`.
- Typed progress, file, tool, artifact, runtime-input, and workflow views driven by the OpenAPI event map.
- An explicit allow-list for `client_action_required`; never navigate or perform asset operations from arbitrary payload data.
- Observability using stable error codes and host request ids only—never API keys, raw `user`, prompts, or answers.

## 7. Complete the product surface

Implement these operations through the same authenticated BFF pattern:

| User action | ZGI endpoint | Identity location |
| --- | --- | --- |
| Load published presentation | `GET /digital-humans/config` | No `user` |
| Start or continue chat | `POST /digital-humans/chat` | JSON `user` |
| Inspect run status | `GET /digital-humans/runs/{run_id}` | Query `user` |
| Recover run stream | `GET /digital-humans/runs/{run_id}/events` | Query `user`; forward `Last-Event-ID` |
| Stop generation | `POST /digital-humans/runs/{run_id}/stop` | JSON `user` |
| Add runtime input | `POST /digital-humans/runs/{run_id}/runtime-input` | JSON `user` |
| List conversations | `GET /digital-humans/conversations?page=1&limit=20` | Query `user` |
| Search history | `GET /digital-humans/conversations/search?query=...&limit=20` | Query `user` |
| Get conversation | `GET /digital-humans/conversations/{id}` | Query `user` |
| Rename conversation | `PATCH /digital-humans/conversations/{id}` | JSON `user` |
| Delete conversation | `DELETE /digital-humans/conversations/{id}` | Query `user` |
| Load messages | `GET /digital-humans/conversations/{id}/messages?page=1&limit=100` | Query `user` |
| Continue a task approval | `POST /digital-humans/conversations/{conversation_id}/messages/{message_id}/workflow-continuation` | JSON `user`, `approval_token`, and `action` |
| Answer a model-generated question | `POST /digital-humans/conversations/{conversation_id}/messages/{message_id}/user-input/{request_id}/continue` | JSON `user` and `answers`; resumes the same Agent Loop |
| Confirm/reject an admitted Linux platform CLI call | `POST /digital-humans/conversations/{conversation_id}/messages/{message_id}/tool-governance/{correlation_id}/continue` | JSON `user` and `action` (`approve` or `reject`); executes only the frozen business call, never the original Shell |
| Upload a file | `POST /digital-humans/files/upload` with multipart `file` | Multipart `user` |
| List or create private tasks | `GET/POST /digital-humans/tasks` | Query/body `user` |
| Read or update a private task | `GET/PATCH /digital-humans/tasks/{task_id}` | Query/body `user` |
| Apply a task action | `POST /digital-humans/tasks/{task_id}/actions/{action}` | JSON `user` and `expected_revision` |
| Read task runs and results | `GET /digital-humans/tasks/{task_id}/runs` | Query `user` |
| Read task activities | `GET /digital-humans/tasks/{task_id}/activities` | Query `user` |

DigitalHuman v1 intentionally does not expose regeneration or relationship-memory management endpoints. Conversation-created task proposals and updates pause for explicit approval; continue them through the workflow-continuation endpoint using the approval token from the SSE event. Task management is available only for the current user's private task namespace; shared DigitalHuman tasks remain a console-only capability.

If the stream emits `user_input_requested` and ends in `waiting_question`, answer
it through `/user-input/{request_id}/continue`. Do not send a new chat message
or use `/workflow-continuation`: the endpoint restores the same Agent Loop and
continues the pending question in place.

### Private task example

Create a task with the same stable identity used by chat:

```bash
curl --request POST "https://zgi.example.com/api/v1/digital-humans/tasks" \
  --header "Authorization: Bearer $ZGI_DIGITAL_HUMAN_API_KEY" \
  --header "Idempotency-Key: 8e5a6f5a-7b5d-4d20-8b63-cc47a40fba0e" \
  --header "Content-Type: application/json" \
  --data '{"user":"customer_123","name":"Follow up","goal":"Summarize the latest requirements","schedule":{"type":"manual"},"delivery_mode":"task_only"}'
```

Use the returned `revision` for optimistic updates and lifecycle actions. A
stale revision returns `409 digital_human.task.conflict`; refresh the task and
retry with the new revision. The runs endpoint returns a bounded
`result_summary`, not model output or execution traces:

For API-created tasks, results are intentionally kept in the task center. For a
task created from a connected channel, the same execution is delivered back to
the original private/group conversation and remains available in the task
details if channel delivery is unavailable.

```bash
curl "https://zgi.example.com/api/v1/digital-humans/tasks/$TASK_ID/runs?user=customer_123" \
  --header "Authorization: Bearer $ZGI_DIGITAL_HUMAN_API_KEY"
```

Authenticated landing pages use
`/api/v1/digital-human-webapps/{web_app_id}/tasks...`; the server derives the
account subject from the signed session and the browser must not send a
`user`. Anonymous landing-page sessions cannot create tasks or register task
tools. API keys are not exposed to the landing page.

When loading history, message pages are newest-first. Sort by `created_at` before rendering chronological chat. If conversation detail reports a non-terminal runtime and you retained its `run_id`, recover the run after history renders. On a full reload, replaying from the beginning is safer than applying a stale cursor to a new DOM; deduplicate by `(message_id, SSE id)`.

For file upload, construct multipart in the BFF so the server—not the browser—adds `user`. Send the returned `data.id` in a later chat request's `file_ids`. Files are scoped to the same DigitalHuman and derived user subject; do not share ids across users or DigitalHumans.

## 8.1 Edit and regenerate a historical question

Root-message replay is available for the API, console, and authenticated
landing page. Use the conversation and message ids returned by `message_start`
and history. Omitting `query` reuses the original question; providing `query`
edits it before execution:

```bash
curl --request POST \
  "https://zgi.example.com/api/v1/digital-humans/conversations/$CONVERSATION_ID/messages/$MESSAGE_ID/regenerate" \
  --header "Authorization: Bearer $ZGI_DIGITAL_HUMAN_API_KEY" \
  --header "Content-Type: application/json" \
  --data '{"user":"customer_123","query":"Compare the three options again and highlight risk"}'
```

The response is a recoverable SSE stream. It starts with `run_accepted`, then
emits a `message_start` with `replace: true`, the normal process and answer
events, `message_end`, and `run_end`. A non-root historical message is not
destructively replaced; replay it as a new branch by calling `/chat` with its
`parent_message_id`. The same owner checks apply to the conversation, message,
and `user`, and an API key rotation does not change ownership.

## 8. Retry and failure behavior

Before SSE starts, ZGI returns JSON:

```json
{ "code": "199001", "message": "Invalid parameter", "retryable": false }
```

After HTTP 200 starts streaming, failures arrive as `error` events followed by one `run_end`. Do not wait for a second HTTP response.

| Result | Recommended behavior |
| --- | --- |
| Network interruption after a `run_id` exists | Recover the same run with `Last-Event-ID` |
| `retryable=true`, `429`, or retryable `5xx` | Bounded exponential backoff with jitter |
| `400` | Fix payload or missing/invalid `user`; do not retry unchanged |
| `401` | Replace invalid, expired, inactive, or revoked key |
| `403` | Check publication and API access settings |
| `404` | Treat resource as unavailable in this user/DigitalHuman scope |
| `409` on chat | A `request_id` was reused with different content; create a new id only for a new logical request |
| `413` / `415` | Reject file size/type in UI and ask the user to choose another file |

Do not blindly repeat `POST /chat` after a connection failure. If ZGI accepted the request, recover by `run_id`. Only retry the POST with the same `request_id` when no run id was observed and the payload is byte-for-byte the same logical request.

## 9. Production checklist

- API key is server-only and has no `NEXT_PUBLIC_` prefix.
- Every stateful BFF route derives the same stable opaque `user` from trusted authentication.
- Browser-supplied `user` is removed, not trusted.
- SSE proxying is unbuffered and preserves `X-ZGI-Run-ID`.
- Client saves `run_id`, `conversation_id`, `message_id`, and the latest non-empty SSE id.
- Unknown events and fields are ignored safely.
- `message_end` and `run_end` are handled as separate boundaries.
- Recovery, stop, history, rename, delete, runtime input, and file upload are user-scoped.
- Markdown and event content are sanitized before rendering.
- Retries are bounded and respect `retryable`.
- Logs contain stable error/request ids but no secrets, raw `user`, prompts, answers, or files.
- API access can be disabled immediately without deleting keys or history.

The running ZGI server exposes the machine-readable contract at `/api/digital-human-openapi.yaml` (with `/api/openapi.yaml` retained as a compatibility alias on deployments that do not yet host the Agent specification) and Swagger UI at `/swagger/index.html`. The in-product **External access → API docs** page provides the same endpoint, event, identity, and error contract in Chinese and English.
