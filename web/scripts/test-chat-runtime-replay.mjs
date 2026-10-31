import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { loadTsModule } from './lib/load-ts-module.mjs';

// ADR 0050 steps 3 and 4: recorded streams replayed through the reducers the
// client ships. A call is one record however its reports arrive, and a turn
// shows the same rows live as after a reload.

const reducers = await loadTsModule('components/chat/controllers/aichat/reducers/index.ts');
const shared = await loadTsModule('components/chat/controllers/aichat/reducers/shared.ts');
const status = await loadTsModule('components/chat/controllers/aichat/status.ts');
const store = await loadTsModule('components/chat/controllers/aichat/store.ts');
const selectors = await loadTsModule('components/chat/controllers/aichat/selectors.ts');
const timelineView = await loadTsModule('components/chat/variants/aichat/agentic-timeline.tsx');
const eventAppliers = await loadTsModule(
  'components/chat/runtime/controller/use-chat-runtime-event-appliers.ts'
);

const fixture = name => JSON.parse(readFileSync(new URL(`./fixtures/${name}`, import.meta.url)));
const streams = fixture('chat-runtime-streams.json').streams;
const storedMessages = fixture('chat-runtime-stored-messages.json').messages;

const clientAction = phase => (state, payload, eventId) => {
  const progress = eventAppliers.clientActionProgressPayload(payload, phase);
  return progress ? reducers.applyAgentProgressState(state, progress, eventId) : state;
};
const APPLIERS = {
  message_start: (state, payload, eventId) =>
    reducers.applyMessageStartState(state, payload, {}, eventId),
  agent_progress: reducers.applyAgentProgressState,
  plan_updated: reducers.applyPlanUpdatedState,
  agent_intermediate_answer: reducers.applyIntermediateAnswerState,
  user_input_requested: reducers.applyUserInputRequestedState,
  skill_load_start: reducers.applySkillLoadStartState,
  skill_load_end: reducers.applySkillLoadEndState,
  skill_reference_read: reducers.applySkillReferenceReadState,
  skill_call_start: reducers.applySkillCallStartState,
  skill_call_end: reducers.applySkillCallEndState,
  skill_call_error: reducers.applySkillCallErrorState,
  skill_artifact_created: reducers.applySkillArtifactCreatedState,
  tool_governance_decision: reducers.applyToolGovernanceDecisionState,
  client_action_required: clientAction('client_action'),
  client_action_result: clientAction('client_action_result'),
  message: reducers.applyMessageChunkState,
  message_retract: reducers.applyMessageRetractState,
  message_end: reducers.applyMessageEndState,
  token_usage_updated: reducers.applyTokenUsageUpdatedState,
};

function messageOf(state, start) {
  return state.messagesByConversation[start.conversation_id]?.find(
    message => message.id === start.message_id
  );
}

function shownRows(message, runtimeTimeline) {
  return timelineView.visibleTimelineItems(
    selectors.messageDisplayTimeline(message, runtimeTimeline),
    message.status,
    true
  );
}

function rowSummary(item) {
  if (item.type === 'skill_event') {
    const invocation = item.invocation;
    return `${invocation.kind}:${invocation.tool_name ?? invocation.skill_id}:${status.invocationOutcome(invocation.status)}`;
  }
  if (item.type === 'tool_governance_decision') {
    return `governance:${item.event.tool_name}:${item.event.status ?? item.event.decision}`;
  }
  return item.type;
}

function assertOneRecordPerCall(invocations, label) {
  for (const key of ['invocation_id', 'runtime_id']) {
    const seen = new Set();
    for (const invocation of invocations) {
      const id = invocation[key];
      if (!id) continue;
      assert.ok(!seen.has(id), `${label}: two records for ${key} ${id}`);
      seen.add(id);
    }
  }
}

function assertOneRowPerCall(rows, label) {
  const seen = new Set();
  for (const row of rows) {
    if (row.type !== 'skill_event') continue;
    const id = row.invocation.invocation_id || row.invocation.runtime_id;
    if (!id) continue;
    assert.ok(!seen.has(id), `${label}: two rows for call ${id}`);
    seen.add(id);
  }
}

function replay(events, onStep) {
  let state = store.createAIChatInitialState();
  const start = events[0].data;
  for (const event of events) {
    const apply = APPLIERS[event.event];
    assert.ok(apply, `no applier for ${event.event}`);
    state = apply(state, event.data, event.id);
    onStep?.(state, start, event);
  }
  return { state, start };
}

let steps = 0;
let compared = 0;
for (const stream of streams) {
  const { state, start } = replay(stream.events, (current, startEvent, event) => {
    const message = messageOf(current, startEvent);
    if (!message) return;
    steps += 1;
    const label = `${stream.name} after ${event.event} ${event.id ?? ''}`;
    assertOneRecordPerCall(message.metadata?.skill_invocations ?? [], label);
    assertOneRowPerCall(
      shownRows(message, current.streamingByMessageId[startEvent.message_id]?.timeline),
      label
    );
  });

  const live = messageOf(state, start);
  const stored = storedMessages.find(message => message.id === start.message_id);
  // A stream that stopped for a question is continued by a later stream of
  // the same message; only the stored copy of the finished turn compares.
  if (!stored || stored.status !== live.status) continue;
  compared += 1;
  const reloaded = { ...live, metadata: stored.metadata, status: stored.status };
  assert.deepEqual(
    shownRows(live, state.streamingByMessageId[start.message_id]?.timeline).map(rowSummary),
    shownRows(reloaded, []).map(rowSummary),
    `${stream.name}: the rows shown live and after a reload differ`
  );

  // Folding the stored copy into the live message, as a refresh does, keeps
  // one record per call and changes nothing when done twice.
  const once = shared.mergeMessageMetadata(live.metadata, stored.metadata);
  assertOneRecordPerCall(once.skill_invocations ?? [], `${stream.name} refreshed`);
  assert.deepEqual(
    shared.mergeMessageMetadata(once, stored.metadata).skill_invocations,
    once.skill_invocations,
    `${stream.name}: refreshing twice changes the records`
  );
}
assert.ok(steps > 150 && compared >= 5, `replayed ${steps} steps, compared ${compared} turns`);

// The stored approval turn holds the request and the decision as two rows of
// one runtime id; the client reads them as one decision, approved.
const approval = storedMessages.find(message =>
  (message.metadata.skill_invocations ?? []).some(
    invocation => invocation.kind === 'tool_governance' && invocation.status === 'needs_approval'
  )
);
const decisions = shared
  .mergeMessageMetadata(undefined, approval.metadata)
  .skill_invocations.filter(invocation => invocation.kind === 'tool_governance');
assert.deepEqual(
  decisions.map(invocation => invocation.status),
  ['approved'],
  'the request and the decision of one approval are one record'
);

// Two calls of the same tool at once stay two calls.
{
  const tools = streams.find(stream => stream.name === 'tools-1').events;
  const [first, second] = tools.filter(event => event.event === 'skill_call_start');
  const parallelStart = {
    ...first,
    id: null,
    data: {
      ...first.data,
      invocation_id: `${first.data.invocation_id}-parallel`,
      runtime_id: `${first.data.runtime_id}#parallel`,
    },
  };
  const events = [];
  for (const event of tools) {
    events.push(event.id === first.id ? { ...event, id: null } : { ...event, id: null });
    if (event === first) events.push(parallelStart);
  }
  const { state, start } = replay(events);
  const message = messageOf(state, start);
  const calls = message.metadata.skill_invocations.filter(
    invocation => invocation.tool_name === first.data.tool_name
  );
  assert.equal(calls.length, 2, 'a parallel call of the same tool keeps its own record');
  assert.equal(second.data.tool_name !== first.data.tool_name, true);
}

// The transition table: an older report never undoes a newer one.
const move = status.canInvocationMoveTo;
assert.equal(move('running', 'success'), true);
assert.equal(move('needs_approval', 'approved'), true);
assert.equal(move('needs_approval', 'rejected'), true, 'a rejection ends a waiting call');
assert.equal(move('needs_approval', 'running'), false, 'an older running report stays older');
assert.equal(move('success', 'running'), false);
assert.equal(move('success', 'error'), true, 'a later failure report wins');
assert.equal(move('error', 'success'), false);
assert.equal(move('running', undefined), false, 'a report without a status keeps the status');
assert.equal(move(undefined, 'running'), true);
assert.equal(
  shared.mergeSkillInvocationByStatus(
    { kind: 'tool_call', status: 'needs_approval', runtime_id: 'r' },
    { kind: 'tool_call', status: 'rejected', runtime_id: 'r' }
  ).status,
  'rejected'
);

console.log(`chat runtime replay: ${steps} steps, ${compared} turns equal live and reloaded`);
