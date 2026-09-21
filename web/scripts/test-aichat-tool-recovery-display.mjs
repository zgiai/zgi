import assert from 'node:assert/strict';
import { load } from './connector-fixture.mjs';
const model = load('src/components/connectors/runtime/connector-operation-presentation.ts');
const {
  formatTimelineDebugValue,
  recoveredInvalidArgumentTimelineItemIds,
  sanitizeTimelineDisplayPayload,
  sanitizeTimelineDisplayString,
  summarizeTimelineArgumentsForDisplay,
} = load('src/components/chat/variants/aichat/timeline-display-safety.ts', {
  '@/components/connectors/runtime/connector-operation-presentation': model,
});
const uuid = '123e4567-e89b-12d3-a456-426614174000';
const compactUuid = '123e4567e89b12d3a456426614174000';
const safePayload = sanitizeTimelineDisplayPayload({
  connection_id: uuid,
  workspace_id: 'platform-workspace',
  issue_id: '142',
  repository_id: 'zgi',
  chat_id: 'oc_public_chat',
  message_id: 'om_public_message',
  resource_id: 'github-resource-17',
  file_id: 'external-file-9',
  nested: {
    requestUuid: uuid,
    compact_reference: compactUuid,
    connection_name: 'Work GitHub',
    note: `completed ${uuid}`,
  },
});
assert.deepEqual(safePayload, {
  issue_id: '142',
  repository_id: 'zgi',
  chat_id: 'oc_public_chat',
  message_id: 'om_public_message',
  resource_id: 'github-resource-17',
  file_id: 'external-file-9',
  nested: {
    requestUuid: '[hidden]',
    compact_reference: '[hidden]',
    connection_name: 'Work GitHub',
    note: 'completed [hidden]',
  },
});
assert.equal(
  sanitizeTimelineDisplayString(`connection_id ${uuid} is invalid`),
  'connection_id [hidden] is invalid'
);
assert.equal(
  sanitizeTimelineDisplayString('skill tool ordinary-skill/perform_task has invalid arguments'),
  'skill tool ordinary-skill/perform_task has invalid arguments'
);
assert.equal(sanitizeTimelineDisplayString(uuid, '已隐藏'), '已隐藏');
assert.equal(sanitizeTimelineDisplayString('__zgi_hidden_reference__', '已隐藏'), '已隐藏');
assert.equal(
  sanitizeTimelineDisplayString('connection __zgi_hidden_reference__ is unavailable', '已隐藏'),
  'connection 已隐藏 is unavailable'
);
assert.deepEqual(sanitizeTimelineDisplayPayload({ request_id: uuid }, '已隐藏'), {
  request_id: '已隐藏',
});
assert.equal(
  sanitizeTimelineDisplayString('__zgi_redacted__', 'Redacted', 'Content truncated'),
  'Redacted'
);
assert.equal(
  sanitizeTimelineDisplayString('[REDACTED]', 'Redacted', 'Content truncated'),
  'Redacted'
);
assert.equal(
  sanitizeTimelineDisplayString('__zgi_truncated__', 'Redacted', 'Content truncated'),
  'Content truncated'
);
assert.equal(
  sanitizeTimelineDisplayString('[TRUNCATED]', 'Redacted', 'Content truncated'),
  'Content truncated'
);
assert.equal(
  formatTimelineDebugValue(
    { public: '__zgi_truncated__', private: '__zgi_redacted__' },
    'Redacted',
    'Content truncated'
  ),
  JSON.stringify({ public: 'Content truncated', private: 'Redacted' })
);
assert.doesNotMatch(JSON.stringify(safePayload), /123e4567|connection_id|workspace_id/i);
assert.match(JSON.stringify(safePayload), /issue_id|repository_id|chat_id|message_id|resource_id/);
assert.match(JSON.stringify(safePayload), /requestUuid|\[hidden\]/);

const argumentSummary = summarizeTimelineArgumentsForDisplay({
  connection_id: uuid,
  arguments: {
    query: 'private repository query',
    limit: 5,
    resource_id: uuid,
    issue_id: '142',
  },
});
assert.deepEqual(argumentSummary, {
  arguments: {
    query: { type: 'string', length: 24 },
    limit: { type: 'number' },
    resource_id: { type: 'string', length: 8 },
    issue_id: { type: 'string', length: 3 },
  },
});
assert.doesNotMatch(JSON.stringify(argumentSummary), /private repository|123e4567/i);
assert.match(JSON.stringify(argumentSummary), /resource_id|issue_id/);
assert.deepEqual(
  summarizeTimelineArgumentsForDisplay({
    action_id: uuid,
    arguments: { type: 'object', keys: 0 },
  }),
  {
    action_id: { type: 'string', length: 8 },
    arguments: { type: 'object', keys: 0 },
  }
);

const ordinaryToolRecovered = recoveredInvalidArgumentTimelineItemIds([
  {
    id: 'ordinary-failed',
    type: 'skill_event',
    invocation: {
      status: 'error',
      skill_id: 'calculator',
      tool_name: 'calculate',
      error: 'argument validation failed',
    },
  },
  {
    id: 'ordinary-succeeded',
    type: 'skill_event',
    invocation: {
      status: 'success',
      skill_id: 'calculator',
      tool_name: 'calculate',
    },
  },
]);
assert.deepEqual([...ordinaryToolRecovered], ['ordinary-failed']);

for (const source of ['connector', 'connector_history']) {
  const failed = {
    id: 'failed',
    type: 'skill_event',
    invocation: {
      kind: 'tool_call',
      runtime_source: source,
      skill_id: 'connector-runtime',
      tool_name: 'same_name',
      status: 'error',
      error: 'invalid arguments',
      result: { operation: { connection_id: 'first', logical_operation_id: 'action-1' } },
    },
  };
  const successful = {
    id: 'successful',
    type: 'skill_event',
    invocation: {
      kind: 'tool_call',
      runtime_source: source,
      skill_id: 'connector-runtime',
      tool_name: 'same_name',
      status: 'success',
      result: { operation: { connection_id: 'second', logical_operation_id: 'action-2' } },
    },
  };
  assert.equal(recoveredInvalidArgumentTimelineItemIds([failed, successful]).size, 0);
}
console.log('Ordinary recovery and native operation display safety passed.');
