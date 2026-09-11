import assert from 'node:assert/strict';
import { load, harness, read } from './connector-fixture.mjs';
import ts from 'typescript';
const model = load('src/components/connectors/runtime/connector-runtime-model.ts');
const source = load('src/components/connectors/runtime/connector-runtime-source.ts', {
  './connector-runtime-model': model,
});
const pending = {
  state: 'requires_confirmation',
  decision_id: 'decision-1',
  confirmation: {
    route_receipt: 'receipt-1',
    connector_id: 'feishu',
    connector_name: 'Feishu',
    operation_id: 'feishu.send',
    action_name: 'Send message',
    target: { type: 'chat', id: 'chat' },
    risk_classification: 'high',
    consequence: {
      external_effect: 'external_send',
      description: 'Send once',
      data_egress: true,
      external_destination: 'Feishu',
    },
  },
};
assert.ok(model.parseConnectorRuntimeExecutionResult(pending));
for (const state of ['completed', 'failed', 'unknown', 'blocked']) {
  assert.equal(
    source.parseConnectorProgressResult({ phase: 'tool_result', result: { state } }),
    null
  );
  assert.equal(
    source.parseConnectorNodeResult(
      { tool_info: { provider_type: 'builtin', provider_id: 'feishu', tool_name: 'send' } },
      { state }
    ),
    null
  );
  assert.equal(
    source.parseConnectorNodeResult(undefined, { state, provider_type: 'connector' }),
    null
  );
}
assert.equal(
  source.parseConnectorProgressResult({
    phase: 'connector_confirmation_resolved',
    result: pending,
  }),
  null
);
assert.ok(
  source.parseConnectorNodeResult(
    { tool_info: { provider_type: 'connector', provider_id: 'feishu', tool_name: 'feishu.send' } },
    pending
  )
);
const required = {
  event_type: 'connector_confirmation_required',
  route_receipt: 'receipt-1',
  conversation_id: 'chat',
  message_id: 'message',
  result: pending,
};
const terminal = {
  ...required,
  event_type: 'connector_confirmation_resolved',
  result: { state: 'unknown' },
};
const progress = {
  type: 'progress_text',
  phase: required.event_type,
  progress_id: 'receipt-1',
  result: pending,
};
const other = { type: 'progress_text', phase: 'ordinary', result: { state: 'unknown' } };
const pendingSkill = {
  id: 'pending-skill',
  type: 'skill_event',
  invocation: {
    kind: 'tool_call',
    runtime_source: 'connector',
    result: {
      state: 'requires_confirmation',
      confirmation: { route_receipt: 'receipt-1' },
    },
  },
};
assert.deepEqual(
  source.parseConnectorSkillEventResult({
    ...pendingSkill,
    invocation: { ...pendingSkill.invocation, result: pending },
  }),
  pending
);
const rawRequired = { ...required, result: undefined, ...pending };
const rawTerminal = { ...terminal, result: undefined, state: 'unknown' };
assert.equal(
  source.connectorChatEvents(
    { connector_confirmation_events: [rawRequired] },
    [],
    'chat',
    'message'
  )[0].result.state,
  'requires_confirmation'
);
assert.equal(
  source.connectorChatEvents(
    { connector_confirmation_events: [rawRequired, rawTerminal] },
    [],
    'chat',
    'message'
  )[0].result.state,
  'unknown'
);
const dispatchBody = read('src/components/chat/transports/aichat-transport.ts').match(
  /case 'connector_confirmation_resolved': \{([\s\S]*?)\n {6}break;/
)[1];
const dispatchConnector = new Function(
  'data',
  'event',
  'callbacks',
  'eventId',
  ts.transpileModule(dispatchBody, {
    compilerOptions: { target: ts.ScriptTarget.ES2022 },
  }).outputText
);
for (const payload of [rawRequired, rawTerminal]) {
  let delivered;
  dispatchConnector(
    payload,
    payload.event_type,
    { onAgentProgress: event => (delivered = event) },
    'event-id'
  );
  assert.equal(source.parseConnectorProgressResult(delivered).state, payload.state);
  assert.equal(delivered.progress_id, 'receipt-1');
}
const events = source.connectorChatEvents(
  { connector_confirmation_events: [terminal, required] },
  [progress, other],
  'chat',
  'message'
);
assert.equal(events.length, 1);
assert.equal(events[0].result.state, 'unknown');
assert.equal(events[0].live, false);
assert.deepEqual(source.withoutSupersededConnectorProgress([progress, other], events), [other]);
assert.deepEqual(
  source.connectorChatEvents(
    { connector_confirmation_events: [required] },
    [],
    'other-chat',
    'message'
  ),
  []
);
assert.deepEqual(
  source.connectorChatEvents(
    { connector_confirmation_events: [{ ...required, route_receipt: 'other' }] },
    [],
    'chat',
    'message'
  ),
  []
);
assert.equal(source.connectorOwnsWaitingState({}, []), true);
for (const key of ['tool_governance_continuation', 'agent_workflow_continuation'])
  assert.equal(
    source.connectorOwnsWaitingState({ [key]: { status: 'waiting_approval' } }, []),
    false
  );
assert.equal(
  source.connectorOwnsWaitingState({}, [
    {
      type: 'tool_governance_decision',
      event: { correlation_id: 'ordinary', status: 'needs_approval' },
    },
  ]),
  false
);
assert.equal(
  source.connectorOwnsWaitingState({}, [
    {
      type: 'tool_governance_decision',
      event: { correlation_id: 'ordinary', status: 'needs_approval' },
    },
    {
      type: 'tool_governance_decision',
      event: { correlation_id: 'ordinary', decision: 'allowed' },
    },
  ]),
  true
);

const { ConnectorRuntimeDecisionTransport } = load(
  'src/components/connectors/runtime/connector-runtime-transport.ts'
);
let networkCalls = 0,
  activeOwner = 'owner-a',
  queryResult = { resolution: 'resolved', state: 'completed', secret: 'must-not-accept' };
const shared = new ConnectorRuntimeDecisionTransport(async () => {
  networkCalls++;
  return { state: 'completed' };
});
const queryClient = { fetchQuery: ({ queryFn }) => queryFn() };
const receiptHook = harness(react => {
  const { useConnectorRuntimeReceipt } = load(
    'src/components/connectors/runtime/use-runtime-receipt.ts',
    {
      react: { ...react, useSyncExternalStore: (_subscribe, snapshot) => snapshot() },
      '@/store/auth-store': {
        useAuthStore: {
          use: { user: () => ({ id: activeOwner }) },
          getState: () => ({ user: { id: activeOwner } }),
        },
      },
      '@/store/organization-store': {
        useOrganizationStore: {
          use: { currentOrganization: () => ({ id: 'org' }) },
          getState: () => ({ currentOrganization: { id: 'org' } }),
        },
      },
      '@/services/connector.service': {
        connectorService: { getRuntimeConfirmationStatus: async () => ({ data: queryResult }) },
      },
      './connector-runtime-transport': { connectorRuntimeDecisionTransport: shared },
      '@tanstack/react-query': { useQueryClient: () => queryClient },
      './connector-confirmation-presentation': load(
        'src/components/connectors/runtime/connector-confirmation-presentation.ts'
      ),
    }
  );
  return () => useConnectorRuntimeReceipt('receipt-checked');
});
receiptHook.render();
await assert.rejects(receiptHook.value.check());
receiptHook.render();
assert.equal(
  receiptHook.value.phase,
  'transport_unconfirmed',
  'malformed status informs all views, not waiting approval'
);
queryResult = { resolution: 'unresolved' };
await receiptHook.value.check();
receiptHook.render();
assert.equal(
  receiptHook.value.phase,
  'transport_unconfirmed',
  'pending read does not authorize replay'
);
queryResult = { resolution: 'resolved', state: 'unknown' };
await receiptHook.value.check();
receiptHook.render();
assert.equal(receiptHook.value.phase, 'unknown');
activeOwner = 'owner-b';
receiptHook.render();
assert.equal(receiptHook.value.phase, undefined);
assert.equal(networkCalls, 0);
receiptHook.unmount();
console.log(
  'Read-only receipt verification rejects malformed data and isolates shared states by owner'
);

const merged = source.mergeConnectorTimeline(
  [
    { id: 'before', type: 'progress_text', phase: 'ordinary', content: 'Earlier', created_at: 1 },
    { ...progress, id: 'original', created_at: 2 },
    { id: 'after', type: 'progress_text', content: 'Later', created_at: 3 },
    progress,
  ],
  events
);
assert.equal(merged.length, 3);
assert.equal(merged[1].id, 'original');
assert.equal(merged[1].result.state, 'unknown');
const mergedSkill = source.mergeConnectorTimeline([pendingSkill, progress], events);
assert.equal(
  mergedSkill.some(item => item.type === 'skill_event'),
  false,
  'terminal receipt removes the stale pending Connector skill row'
);
assert.equal(
  source.mergeConnectorTimeline([pendingSkill], []).length,
  1,
  'pending Connector skill row remains until a terminal receipt exists'
);
const pendingMergedSkill = source.mergeConnectorTimeline(
  [pendingSkill],
  source.connectorChatEvents({ connector_confirmation_events: [required] }, [], 'chat', 'message')
);
assert.equal(
  pendingMergedSkill.some(item => item.type === 'skill_event'),
  false,
  'required receipt uses the confirmation card instead of a duplicate skill row'
);
const restored = source.mergeConnectorTimeline(
  [],
  source.connectorChatEvents(
    { connector_confirmation_events: [{ ...required, created_at: 2 }, terminal] },
    [],
    'chat',
    'message'
  )
);
assert.equal(restored.length, 1);
assert.equal(restored[0].result.state, 'unknown');
console.log('History and live receipts occupy a single original timeline position');
