import assert from 'node:assert/strict';
import { load } from './connector-fixture.mjs';

const model = load('src/components/connectors/runtime/connector-operation-presentation.ts');

// Secrets and host-only evidence that must never survive the read-only
// presentation parser. The Connector terminal card renders this parsed object,
// so asserting on the parser output is the security boundary.
const FORBIDDEN = [
  'SECRET-DO-NOT-DISPLAY',
  'FORGED-RECEIPT',
  'route_receipt',
  'onDecision',
  'api_key',
  'arguments',
  'confirmation',
  'binding_id',
  'connection_id',
  'outcome_id',
];
const ALLOWED_KEYS = new Set([
  'schema_version',
  'source_kind',
  'invocation_id',
  'display_status',
  'execution_status',
  'title',
  'summary',
  'error_code',
  'redacted',
  'truncated',
]);

function assertSafePresentation(parsed) {
  const serialized = JSON.stringify(parsed);
  for (const secret of FORBIDDEN) {
    assert.ok(!serialized.includes(secret), `presentation leaked ${secret}: ${serialized}`);
  }
  for (const key of Object.keys(parsed)) {
    assert.ok(ALLOWED_KEYS.has(key), `presentation carried unexpected key ${key}`);
  }
}

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
    assert.equal(parsed.source_kind, source);
    assertSafePresentation(parsed);
    const expectedTone =
      status === 'successful' ? 'success' : status === 'failed' ? 'error' : 'warning';
    assert.equal(model.connectorOperationTone(invocation), expectedTone);
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
  // A forged or malformed invocation must never project a presentation, so no
  // secret-bearing raw result can reach a terminal card.
  assert.equal(model.parseConnectorOperationPresentation(invocation), null);
}

assert.equal(model.parseConnectorOperationPresentation(fixture('pending', 'history')), null);
const unrelated = { ...fixture('successful'), runtime_source: 'builtin' };
assert.equal(model.connectorOperationTone(unrelated), null);

for (const [state, disposition, status, label, tone] of [
  ['rejected', 'not_submitted', 'unavailable', 'not_submitted', 'error'],
  ['unresolved', 'in_progress', 'unknown', 'in_progress', 'running'],
  ['unresolved', 'unknown', 'unknown', 'unknown', 'warning'],
]) {
  const invocation = fixture(status);
  Object.assign(invocation.result, {
    state,
    execution_evidence: { version: 1, disposition },
  });
  delete invocation.result.outcome_id;
  delete invocation.result.outcome_status;
  delete invocation.result.confirmation;
  Object.assign(invocation.result.connector_presentation, {
    title: '获取机器人信息',
    summary: '系统校验冲突，本次操作未提交。',
    redacted: false,
    truncated: false,
  });
  const parsed = model.parseConnectorOperationPresentation(invocation);
  assert.ok(parsed);
  assert.equal(parsed.execution_status ?? parsed.display_status, label);
  assert.equal(parsed.title, '获取机器人信息');
  assert.ok(parsed.summary.includes('系统校验冲突'));
  assertSafePresentation(parsed);
  assert.equal(model.connectorOperationTone(invocation), tone);
  for (const mutate of [
    v => delete v.result.execution_evidence,
    v => {
      v.result.execution_evidence.version = 2;
    },
    v => {
      v.result.execution_evidence.private_data = 'forged';
    },
    v => {
      v.result.execution_evidence.disposition = state === 'rejected' ? 'unknown' : 'not_submitted';
    },
    v => {
      v.result.outcome_id = 'd013d536-c5a4-4145-a04c-4047b6d31d2b';
    },
    v => {
      v.result.confirmation = { route_receipt: 'FORGED-RECEIPT' };
    },
  ]) {
    const forged = JSON.parse(JSON.stringify(invocation));
    mutate(forged);
    assert.equal(model.parseConnectorOperationPresentation(forged), null);
  }
}

console.log('Connector read-only operation presentation tests passed.');
