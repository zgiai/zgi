import assert from 'node:assert/strict';
import { load, jsx, ui, harness, flush, read } from './connector-fixture.mjs';
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
  /case 'connector_confirmation_resolved': \{([\s\S]*?)\n      break;/
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

// Real compact confirmation component: old base, single-shot callbacks, read-only recovery.
let scope = 'org-owner',
  response,
  phase,
  checks = 0,
  posts = 0,
  finish,
  failRead = false;
const check = async () => {
  checks++;
  if (failRead) throw Error('offline');
};
const mount = () =>
  harness(react => {
    const module = load(
      'src/components/connectors/runtime/connector-runtime-confirmation-card.tsx',
      {
        react,
        'react/jsx-runtime': jsx,
        'next-intl': { useLocale: () => 'zh-Hans' },
        'lucide-react': ui,
        './use-runtime-receipt': {
          useConnectorRuntimeReceipt: () => ({ scope, phase, result: response, check }),
        },
        '@/components/aichat/operation-cards': { OperationConfirmationCard: 'OldConfirmationCard' },
        '@/components/connectors/connector-metadata': load(
          'src/components/connectors/connector-metadata.ts'
        ),
        '@/i18n/translations': { useT: () => key => key },
        './connector-runtime-model': model,
      }
    );
    return () =>
      module.ConnectorRuntimeConfirmationCard({
        result: pending,
        onDecision: (_command, expectedScope) => {
          posts++;
          assert.equal(expectedScope, scope);
          return new Promise(resolve => (finish = resolve));
        },
      });
  });
let h = mount();
h.render();
assert.equal(
  h.value.props.confirmAction.disabled,
  true,
  'restored receipts checked before offering approval'
);
await flush();
h.render();
assert.equal(h.value.type, 'OldConfirmationCard');
assert.equal(h.value.props.items.length, 1);
assert.equal(h.value.props.confirmAction.disabled, false);
const click = h.value.props.confirmAction.onClick;
click();
click();
assert.equal(posts, 1);
finish({ state: 'transport_unconfirmed', route_receipt: 'receipt-1' });
await flush();
h.render();
assert.equal(h.value.props.confirmAction, undefined);
assert.equal(h.value.props.cancelAction, undefined);
assert.equal(h.value.props.actions[0].id, 'check-connector-status');
h.value.props.actions[0].onClick();
await flush();
assert.equal(posts, 1);
assert.ok(checks >= 2);
response = { state: 'unknown', route_receipt: 'receipt-1' };
phase = 'unknown';
h.render();
assert.equal(h.value.props.statusLabel, 'runtime.confirmation.status.unknown');
assert.equal(h.value.props.actions, undefined);
h.unmount();
response = undefined;
phase = undefined;
failRead = true;
h = mount();
h.render();
await flush();
h.render();
assert.equal(
  h.value.props.confirmAction,
  undefined,
  'failed status recovery never offers blind resubmission'
);
h.unmount();
failRead = false;
h = mount();
h.render();
await flush();
h.render();
h.value.props.confirmAction.onClick();
scope = 'different-owner';
h.render();
finish({ state: 'completed', route_receipt: 'receipt-1' });
await flush();
h.render();
assert.notEqual(
  h.value.props.statusLabel,
  'runtime.confirmation.status.completed',
  'late callback cannot paint a different owner'
);
h.unmount();
console.log(
  'Connector trusted-source, ordinary approval isolation, restoration and compact one-shot UI passed'
);

const {ConnectorRuntimeDecisionTransport} = load('src/components/connectors/runtime/connector-runtime-transport.ts');
let networkCalls = 0, activeOwner='owner-a', queryResult={resolution:'resolved',state:'completed',secret:'must-not-accept'};
const shared = new ConnectorRuntimeDecisionTransport(async()=>{networkCalls++;return {state:'completed'};});
const receiptHook=harness(react=> {
  const {useConnectorRuntimeReceipt}=load('src/components/connectors/runtime/use-runtime-receipt.ts',{
    react:{...react,useSyncExternalStore:(_subscribe,snapshot)=>snapshot()},
    '@/store/auth-store':{useAuthStore:{use:{user:()=>({id:activeOwner})},getState:()=>({user:{id:activeOwner}})}},
    '@/store/organization-store':{useOrganizationStore:{use:{currentOrganization:()=>({id:'org'})},getState:()=>({currentOrganization:{id:'org'}})}},
    '@/services/connector.service':{connectorService:{getRuntimeConfirmationStatus:async()=>({data:queryResult})}},
    './connector-runtime-transport':{connectorRuntimeDecisionTransport:shared},
  });
  return()=>useConnectorRuntimeReceipt('receipt-checked');
});
receiptHook.render();await assert.rejects(receiptHook.value.check());receiptHook.render();
assert.equal(receiptHook.value.phase,'transport_unconfirmed','malformed status informs all views, not waiting approval');
queryResult={resolution:'unresolved'};await receiptHook.value.check();receiptHook.render();assert.equal(receiptHook.value.phase,'transport_unconfirmed','pending read does not authorize replay');
queryResult={resolution:'resolved',state:'unknown'};await receiptHook.value.check();receiptHook.render();assert.equal(receiptHook.value.phase,'unknown');
activeOwner='owner-b';receiptHook.render();assert.equal(receiptHook.value.phase,undefined);assert.equal(networkCalls,0);receiptHook.unmount();
console.log('Read-only receipt verification rejects malformed data and isolates shared states by owner');
