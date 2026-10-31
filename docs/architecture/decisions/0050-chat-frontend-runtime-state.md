# ADR 0050: One Source of Truth for Chat Runtime State in the Web Client

- Status: Accepted
- Date: 2026-10-31

## Context

The web client shows one chat runtime on five surfaces: the console sidebar assistant, the console chat page, the Agent preview, the published Agent webapp and DigitalHuman chats. They share a transport interface, one stream event dispatcher, the reducers in `components/chat/controllers/aichat` and the components in `components/chat/variants/aichat`. A review of that code on 2026-10-31 found that the shared core is sound, but the rules on top of it are spread out and disagree.

**The same fact is stored several times.** A tool call lives in `message.metadata.skill_invocations`, in the streaming timeline and in the runtime timeline items. Each copy has its own identity rule:

- `upsertSkillInvocation`: invocation id, then a semantic key, runtime id, answer id, and finally the newest running item of the same kind, skill and tool;
- `skillInvocationsMatch`: invocation id, runtime id, semantic key, then a pending fallback on kind, skill, tool and path;
- `skillInvocationIdentity`: runtime id, semantic key, then a key that includes the array index;
- `timelineItemAliasIdentities`: a fourth set used when merging timelines.

`message_end` folds the timeline back into metadata, so the number of rows can change when a turn completes.

**The live view and the reloaded view are built by different code.** While streaming, event reducers build the timeline item by item. After a reload, `timelineFromAIChatMessage` rebuilds it from metadata. The message bubble shows one or the other, never both. Whether an invocation is shown, hidden, merged or shown as a governance row is decided in about seven places:

- three copies of the visibility rule (`selectors.ts`, `reducers/skill.ts`, `reducers/shared.ts`), which already differ;
- supersede and dedupe rules in the selectors and again in `agentic-timeline.tsx`, each with its own correlation lookup (about 15 fields in one, 5 in the other);
- a dedupe key for governance rows made of skill, tool, effect and asset names;
- presentation ordering that falls back to event types when an item has no reference.

**Statuses have no closed vocabulary.** `success`, `succeeded`, `completed`, `allowed` and `approved` mean "finished" in some files and not in others. Unknown values fall into default branches that mean something else: an unknown governance status becomes `needs_approval`, and an unknown tone is shown as success. The status rank used to merge invocations ranks `completed`, `failed` and `needs_resolution` below `running`, and `error` above `success`.

**Stream wiring is copied by hand.** The send, regenerate, continuation and recovery paths each build the same map of about 33 callbacks. Callbacks are optional, so a missing one compiles. `token_budget_requested` was handled on none of the four paths until #2920.

**"What the person must resolve now" has three sources.** A module-level store fed by the timeline component, a fallback derived from persisted metadata, and a copy of the store held in the composer's state. They use different ids and lifetimes, and the module-level sets are shared across conversations and mounts. The queue, regenerate and edit paths each decide on their own whether a turn is waiting.

**Surface behaviour is chosen by string compares.** `surface`, `runtimeSurface` and `variant` are compared inside the 2,300-line chat shell. What the transport supports and what the shell turns on drifted apart: DigitalHuman chats had approvals turned off although their transports support them (fixed in #2920).

**Tests pin single behaviours.** About 28 script tests under `web/scripts` each cover one behaviour of the chat client; five run in the CI frontend gate. None covers surface combinations or checks that the live and reloaded timelines agree. One of them loads `selectors.ts` with its imports stripped, so moving a helper between files can break it without a type error (seen in #2919).

Recent bugs share this cause:

- a page navigation showed as a yellow pending governance row "运行此工具。" during a run and disappeared after reload (#2919);
- a governance decision carried by `skill_call_end` was dropped as a stale event (#2920);
- a queued message could be sent over a pending approval (#2920).

Each was fixed where it showed, and each fix is another rule in one of the places above.

## Decision

1. **Decode events once, into a closed set of types.**
   - The transport edge parses every stream event with a schema into a typed event, or rejects it with a logged reason. No `(data ?? {}) as T` casts and no invented defaults such as `status: 'unknown'` or an empty `message_id`.
   - Statuses are mapped once, at this edge, into closed unions per kind:
     - invocation lifecycle: `pending`, `running`, `waiting`, `succeeded`, `failed`, `cancelled`, `unknown`;
     - governance: `none`, or a decision `allowed`, `needs_approval`, `approved`, `rejected`, `denied`, `blocked`, `unknown`;
     - message phase: `streaming`, `waiting_for_person` (with the reason), `completed`, `stopped`, `error`.
   - An unknown backend value maps to `unknown`, never to a state that asks the person to act or that reads as success.
   - Governance is either absent or a decision. An empty object is absent.

2. **Store each invocation once, keyed by one id.**
   - The canonical id is the backend `invocation_id`, or `runtime_id` when there is none. A client-generated id is bound once at start for events that carry neither. Arguments, display text, page addresses and array positions never decide identity.
   - Invocations live in one normalized map per message. A governance decision is attached to its invocation by `correlation_id`, not stored as a separate row.
   - Status changes go through a transition table per kind. A late `running` cannot overwrite `succeeded`, and a retried call that succeeds reads as succeeded.
   - Metadata, timeline items, counts and name lists are derived by selectors. `message_end` replaces the normalized map with the server's copy; it does not merge two shapes.

3. **One pure function builds the timeline, live and after reload.**
   - `buildTimeline(invocations, presentation, capabilities)` is the only place that decides visibility, grouping, supersession and order. The live view and the reloaded view call it with the same inputs, so they cannot differ.
   - Visibility comes from data the backend sends (`presentation_visibility`, a lane or group id, what an item supersedes), not from skill or tool names. Tool-specific branches (console navigator, DigitalHuman tasks, connectors, shell jobs, asset observation) move to the backend's presentation fields or to a small table keyed by tool, in one file.
   - Components render the items they are given. `agentic-timeline.tsx` keeps no governance, status or dedupe rules.

4. **One exhaustive event table.**
   - A single `createRuntimeEventSink(appliers, hooks)` is built from a `Record<RuntimeEventName, Applier>`, so a missing event is a type error.
   - Send, regenerate, continuation and recovery supply only lifecycle hooks (open, terminal, close, error). The event cursor is kept per conversation stream and checked once in the sink, before any reducer runs.
   - One event is one state transition. Facts carried by an event, such as a governance decision on `skill_call_end`, apply in the same transition and never become a second event with a derived id.
   - A full replay rebuilds the projection from empty instead of appending deltas onto history.

5. **One selector answers "what must the person resolve now".**
   - `selectPendingInteraction(state, conversationId)` derives a single interaction from the leaf message's phase and the normalized invocations: a tool approval, a connector confirmation, a workflow approval, a question, a page action or a budget request.
   - The composer, the queue, regenerate, edit and the approval panel all read it. Local state only tracks a submission per server id (`submitting`, `failed`), and server state clears it.
   - The module-level pending and dismissed stores are removed.

6. **Surface capabilities come from one profile.**
   - `resolveChatSurfaceProfile(surface, transport)` returns a capability record: approvals, runtime input, workflow approvals, skill and connector pickers, permission control, diagnostics, upload routing.
   - Transport capabilities (`supportsToolGovernanceApproval`, `supportsRuntimeInput`, and so on) are inputs to the profile, never re-derived from the surface name.
   - A table-driven test covers every surface and transport combination.
   - The controller returns a stable actions object and the profile, so callers stop working around a new object on every render.

## Migration

The steps are ordered so that each one ships alone, keeps behaviour, and makes the next one smaller. Each step lands with tests in `web/scripts` or `web/src/test` that run in CI.

1. **Vocabulary and event table** (decisions 1 and 4).
   - 1a, done:
     - `controllers/aichat/status.ts` holds the closed outcome and governance vocabularies and their mappers. The invocation tone and the governance invocation status read them, so an unknown value shows as a warning and is no longer read as `needs_approval` or success.
     - `runtime/controller/runtime-event-callbacks.ts` is the event table. Send, regenerate, continuation and recovery spread it and keep only their lifecycle callbacks; every event a continuation receives marks its stream as started.
     - `AgentAdvancedChatTransport`, which nothing used, is deleted.
     - Tests (`test:chat-runtime-status`, in the CI frontend gate): the mappers, every message and governance status from the Go constants, and the table covering every non-lifecycle stream callback.
   - 1b, done:
     - `transports/stream-event-decoder.ts` checks every event once, before any handler: a payload that is not an object, or a message event without its conversation and message ids, is dropped and recorded in the timeline debug log. The dispatcher no longer casts `data ?? {}` or invents empty ids for connector confirmations.
     - The event table keeps one cursor per stream: an event whose id is not newer than the last one applied from that stream is a repeat and changes nothing. The reducers keep their own checks until step 3.
     - Tests (`test:chat-stream-decoder`, in the CI frontend gate) replay streams recorded from the local stack (`web/scripts/fixtures/chat-runtime-streams.json`), check that every event the dispatcher handles is classified, and check the cursor.
     - Field-level schemas per event are left to step 3, where the normalized invocation map defines the fields the client reads.
2. **Pending interaction and surface profile** (decisions 5 and 6).
   - 2a, done: `variants/aichat/chat-surface-profile.ts` turns the surface, layout, runtime surface, the host's permission-control switch and the transport's capabilities into one record. The chat shell reads it and no longer compares surface names. `test:chat-surface-profile` pins every surface as its page mounts it and the rules that hold for every combination.
   - 2b, done:
     - `controllers/aichat/pending-interaction.ts` returns the one interaction to resolve: a workflow approval, a tool approval or a question, in the composer's order, or the leaf message's own wait for a page action or budget. The queue, regenerate, edit and branch switching all stop while it is set.
     - Pending approvals live in a scope each chat shell creates and drops with itself; the module-wide maps are gone. Readers subscribe to the scope, so a dismissal re-renders them.
     - A workflow approval whose submission fails leaves its submitted state and can be sent again.
     - `test:chat-pending-interaction` covers the selector and the scope.
   - Left for step 3: the composer still picks the approval card from the published approval and the persisted fallback. Connector confirmations come from a receipt the bubble loads, which is not in the controller state yet; once invocations are normalized, the selector can derive the card itself and the publishing goes.
3. **Normalized invocations** (decision 2).
   - Store invocations in one map per message with the canonical id and the transition table.
   - Keep the old metadata shape as a selector, so components change in step 4 only.
   - Tests: parallel calls to the same tool, retries, late `running` events, `message_end` replacement.
   - 3a, done (identity and transitions, on the existing list):
     - One rule matches a report to its call, in `reducers/shared.ts`: call id, then runtime id, then what the call did. Live events, the message_end snapshot and a reloaded message all use it; the three copies it replaces disagreed (the reload keyed on the runtime id, the live path on what the call did), so a call could be one row live and two after a reload. Different ids never fold together. The records of 3,725 messages stored on the local stack showed that call and runtime ids never name two calls; the only cross-id fold left is a page navigation and the client action that carries it out.
     - A report that joins two records folds them into one: the call's end turns it into the governance decision that the decision event already recorded under the runtime id.
     - `canInvocationMoveTo` in `status.ts` is the transition table on the closed outcome vocabulary. It replaces the status rank: a rejection now ends a waiting call (the rank ranked `rejected` below `needs_approval`), and an older `running` report does not undo a wait.
     - Governance decisions keep the runtime id of their call.
     - One rule, `isRecordedSkillInvocation`, decides which calls a message keeps; the display drops only what its own cards show.
     - Tests: `test:chat-runtime-replay` replays the recorded streams, now with a sidebar approval turn (navigate, ask, approve, continue in the same message), through the reducers the client ships, loaded by `scripts/lib/load-ts-module.mjs`. Every step keeps one record and one row per call; a parallel call of the same tool keeps its own record; refreshing from the stored copy twice changes nothing; the stored request and decision of one approval read as one approved decision.
   - Not done: invocations are still a list in the message metadata, not a map, and `message_end` still merges its snapshot into the live list instead of replacing it, because the snapshot may leave out what the live stream showed. Field-level event schemas are not written.
4. **One timeline builder** (decision 3).
   - Move every visibility, supersede and dedupe rule into `buildTimeline`.
   - Use it for both the live and the reloaded view.
   - Delete the rule copies from the reducers, the selectors and `agentic-timeline.tsx`.
   - Tests: fixtures of real streams, recorded from the local stack, whose live and reloaded timelines must be equal.
   - 4a, done (one entry point, equality tested):
     - `messageDisplayTimeline` in `selectors.ts` is the one timeline a message shows: the live event order, then the records only the stored message has; the stored message alone when there are no live events. The message bubble and the approval card read it; the card used to merge the two timelines its own way.
     - `visibleTimelineItems` in `agentic-timeline.tsx` is the pure row filter the component uses, exported so tests run it.
     - Tests: for every recorded turn, the rows shown live equal the rows shown after a reload, and at each of the 172 steps the new entry point shows what the bubble showed before.
   - Not done: the visibility, supersede and dedupe rules still live in `selectors.ts` and `agentic-timeline.tsx`; they are reached through the two entry points above, not moved into one module.
   - Found while testing: when the model's first text is retracted before a tool call, the live view keeps it as process narration, and the stored presentation drops it, so a reload shows one row fewer. The backend's presentation persistence decides this; it is not changed here.
5. **Backend presentation fields** (decision 3).
   - Add `presentation_visibility`, a lane or group id and `supersedes` where the backend knows them.
   - Remove the matching tool-specific branches from the client.
   - 5a, done: `presentation_visibility`. An asset observation is kept for the status line and is not a row. The backend sets it where it builds the observation, passes a recorded invocation's visibility on to the emitted event (DigitalHuman task decisions now carry it live too), and adds it to stored messages on read. The client hides any record marked internal with one rule and no longer recognises asset observations by action names.
   - Not done: a lane or group id and `supersedes`. A page navigation and its client action, and a governed call and its decision, are still paired on the client by what they did; the DigitalHuman task and connector branches stay.

Steps 1 and 2 are small and remove most of the current user-facing risk. Steps 3 and 4 are the large ones and are worth doing once 1 and 2 have landed.

## Consequences

- A change to status handling, visibility or identity is made in one place, and the live view cannot drift from the reloaded one.
- New events and new surfaces fail type checks or the surface table instead of failing silently.
- The large components shrink. `agentic-timeline.tsx` becomes rendering only, and the chat shell stops interpreting surfaces.
- Decoding at the edge rejects malformed events that the client tolerates today. The decoders must accept every shape the backend sends now, including older persisted metadata, so steps 1 and 3 need fixtures from stored messages, not only from new streams.
- Older persisted messages keep loading: the normalizer reads the current metadata shapes, including invocations without `invocation_id`, which get an id bound from `runtime_id` or their stored position once, at load.
- The single-chat controller used by the workflow webapp and the image page is not part of this change. `AgentAdvancedChatTransport`, which nothing uses, is deleted in step 1.

## Alternatives considered

- **Keep fixing bugs where they show.** Each fix adds a rule to one of the places that decide the same thing, which is how the current state came about. Rejected.
- **Rewrite the chat client at once.** Too large to review or roll back, and it would mix behaviour changes with structure. Rejected in favour of the ordered steps above.
- **Move all presentation decisions to the backend now.** The backend should send presentation fields (step 5), but the client still needs one decoder and one builder for older stored messages and for events without those fields. Kept as the last step, not the first.
