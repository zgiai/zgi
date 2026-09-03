import assert from 'node:assert/strict';
import { load, jsx } from './connector-fixture.mjs';

const model = load('src/components/connectors/runtime/connector-operation-presentation.ts');
const component = load('src/components/connectors/runtime/connector-operation-summary.tsx', {
  'react/jsx-runtime': jsx,
  '@/i18n/translations': { useT: () => key => key },
  '@/lib/utils': { cn: (...values) => values.join(' ') },
  './connector-operation-presentation': model,
});
const fixture = (status, source = 'native') => ({
  kind: 'tool_call',
  runtime_source: source === 'native' ? 'connector' : 'connector_history',
  invocation_id: 'action-1',
  status: 'success',
  result: {
    state: 'completed',
    outcome_id: 'd013d536-c5a4-4145-a04c-4047b6d31d2b',
    outcome_status: status,
    operation: {
      connector_id: 'github',
      operation_id: 'github.issue.create',
      logical_operation_id: 'action-1',
      binding_id: '416d5ec9-cf14-45b2-be55-06c31cb6a8d1',
      connection_id: '467eb74b-7fd7-42d4-9632-ebf0a6e96b13',
      target: { type: 'chat', id: 'conversation-1', scope: 'live' },
    },
    connector_presentation: {
      schema_version: 1,
      source_kind: source,
      invocation_id: 'action-1',
      connector_id: 'github',
      operation_id: 'github.issue.create',
      display_status: status,
      redacted: true,
      truncated: true,
    },
    arguments: { api_key: 'SECRET-DO-NOT-DISPLAY' },
    confirmation: { route_receipt: 'FORGED-RECEIPT' },
  },
});
for (const source of ['native', 'history']) {
  for (const status of ['successful', 'partial', 'failed', 'unknown', 'cancelled', 'unavailable']) {
    const invocation = fixture(status, source);
    const parsed = model.parseConnectorOperationPresentation(invocation);
    assert.equal(parsed.display_status, status);
    const expectedTone =
      status === 'successful' ? 'success' : status === 'failed' ? 'error' : 'warning';
    assert.equal(model.connectorOperationTone(invocation), expectedTone);
    const tree = JSON.stringify(component.ConnectorOperationSummary({ invocation }));
    assert.ok(tree.includes(`runtime.presentation.status.${status}`));
    for (const secret of [
      'SECRET-DO-NOT-DISPLAY',
      'FORGED-RECEIPT',
      'route_receipt',
      'onDecision',
      'button',
    ])
      assert.ok(!tree.includes(secret));
  }
}
for (const mutate of [
  value => delete value.runtime_source,
  value => {
    value.result.connector_presentation.schema_version = 2;
  },
  value => {
    value.result.connector_presentation.confirmation = { route_receipt: 'forged' };
  },
  value => {
    value.result.connector_presentation.summary = 'x'.repeat(801);
  },
  value => {
    value.result.operation.logical_operation_id = 'another-action';
  },
  value => {
    delete value.result.outcome_id;
  },
  value => {
    value.result.outcome_status = 'unknown';
  },
  value => {
    value.result.operation.binding_id = 'invalid';
  },
  value => {
    value.result.operation.target.scope = 'published';
  },
  value => {
    value.result.outcome_id = '00000000-0000-0000-0000-000000000000';
  },
]) {
  const invocation = fixture('successful');
  mutate(invocation);
  assert.equal(model.parseConnectorOperationPresentation(invocation), null);
  if (model.isConnectorOperationInvocation(invocation)) {
    const tree = JSON.stringify(component.ConnectorOperationSummary({ invocation }));
    assert.ok(tree.includes('runtime.presentation.unavailable'));
    assert.ok(!tree.includes('SECRET-DO-NOT-DISPLAY'));
  }
}
assert.equal(model.parseConnectorOperationPresentation(fixture('pending', 'history')), null);
const unrelated = { ...fixture('successful'), runtime_source: 'builtin' };
assert.equal(model.connectorOperationTone(unrelated), null);
console.log('Connector read-only operation presentation tests passed.');
