import assert from 'node:assert/strict';
import { loadTsModule } from './lib/load-ts-module.mjs';

const { applyAgentProgressState: apply } = await loadTsModule(
  'components/chat/controllers/aichat/reducers/skill.ts'
);
const pending = receipt => ({
  state: 'requires_confirmation',
  confirmation: {
    route_receipt: receipt,
    connector_id: 'qqmail',
    connector_name: 'QQ Mail',
    operation_id: 'send',
    action_name: 'Send',
    target: { type: 'chat', id: 'chat' },
    risk_classification: 'high',
    consequence: { external_effect: 'external_send', data_egress: true },
  },
});
const event = (receipt, state) => ({
  conversation_id: 'chat',
  message_id: 'message',
  content: '',
  progress_id: receipt,
  phase:
    state === 'pending' ? 'connector_confirmation_required' : 'connector_confirmation_resolved',
  result: state === 'pending' ? pending(receipt) : { state },
  created_at: 100,
});
const empty = () => ({ messagesByConversation: {}, streamingByMessageId: {} });
const rows = state => state.streamingByMessageId.message.timeline;
for (const outcome of ['completed', 'failed', 'cancelled', 'unknown']) {
  let state = apply(empty(), event('r1', 'pending'), '100-1');
  assert.equal(rows(state).length, 1);
  state.streamingByMessageId.message.timeline.push({
    id: 'pending-skill',
    type: 'skill_event',
    invocation: {
      kind: 'tool_call',
      runtime_source: 'connector',
      result: { state: 'requires_confirmation', confirmation: { route_receipt: 'r1' } },
    },
  });
  const id = rows(state)[0].id;
  state = apply(state, event('r1', outcome), '100-2');
  assert.equal(rows(state).length, 1);
  assert.equal(rows(state)[0].id, id, 'updates original timeline position');
  assert.equal(rows(state)[0].result.state, outcome);
  assert.equal(state.streamingByMessageId.message.last_event_id, '100-2');
  assert.equal(apply(state, event('r1', outcome), '100-2'), state, 'event replay');
  state = apply(state, event('r1', 'pending'), '100-3');
  assert.equal(rows(state)[0].result.state, outcome, 'late required cannot downgrade terminal');
}
let state = apply(empty(), event('r1', 'pending'), '200-1');
state = apply(state, event('r2', 'pending'), '200-2');
state = apply(state, event('r1', 'completed'), '200-3');
assert.deepEqual(
  rows(state).map(row => row.result.state),
  ['completed', 'requires_confirmation']
);
state = apply(state, { ...event('r3', 'pending'), conversation_id: 'other-chat' }, '200-4');
assert.equal(rows(state).length, 2, 'message identity cannot cross conversation');
state = apply(
  state,
  { conversation_id: 'chat', message_id: 'message', content: 'Working' },
  '200-5'
);
assert.equal(rows(state).length, 3, 'ordinary progress retains confirmation evidence');
state = apply(
  state,
  { conversation_id: 'chat', message_id: 'message', content: 'Working' },
  '200-6'
);
assert.equal(rows(state).length, 3, 'ordinary text still deduplicates');
state = apply(
  state,
  {
    conversation_id: 'chat',
    message_id: 'message',
    content: '',
    phase: 'context_compaction',
    progress_id: 'c',
    status: 'running',
  },
  '200-7'
);
state = apply(
  state,
  {
    conversation_id: 'chat',
    message_id: 'message',
    content: '',
    phase: 'context_compaction',
    progress_id: 'c',
    status: 'completed',
  },
  '200-8'
);
assert.equal(rows(state).filter(row => row.progress_id === 'c').length, 1);
assert.equal(rows(state).at(-1).status, 'completed');
console.log(
  'Real progress reducer: confirmation lifecycle, replay, scope, ordinary progress and compaction passed'
);
