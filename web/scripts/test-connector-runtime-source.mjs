import assert from 'node:assert/strict';
import { load } from './connector-fixture.mjs';

const model = load('src/components/connectors/runtime/connector-runtime-model.ts');
const source = load('src/components/connectors/runtime/connector-runtime-source.ts', {
  './connector-runtime-model': model,
});

const presentation = {
  schema_version: 1,
  source_kind: 'native',
  invocation_id: 'logical-mail-operation-1',
  connector_id: 'qqmail',
  operation_id: 'qqmail.message.send',
  display_status: 'failed',
  summary: '邮件服务器拒绝了本次提交，请检查邮件服务状态后重试。',
  error_code: 'connector_mail_submission_rejected',
  redacted: true,
  truncated: false,
};

const progress = {
  id: 'progress-1',
  type: 'progress_text',
  phase: 'connector_confirmation_resolved',
  progress_id: 'receipt-1',
  result: { state: 'failed', error_code: presentation.error_code, connector_presentation: presentation },
};
const skill = {
  id: 'skill-1',
  type: 'skill_event',
  invocation: {
    kind: 'tool_call',
    runtime_source: 'connector',
    invocation_id: presentation.invocation_id,
    skill_id: 'connector-runtime',
    tool_name: 'qqmail_message_send',
    result: {
      state: 'failed',
      error_code: presentation.error_code,
      execution_evidence: { version: 1, disposition: 'unknown' },
      operation: { logical_operation_id: presentation.invocation_id },
      connector_presentation: presentation,
    },
  },
};

assert.equal(source.connectorOperationIdentityFromTimelineItem(progress), presentation.invocation_id);
assert.equal(source.connectorOperationIdentityFromTimelineItem(skill), presentation.invocation_id);
assert.equal(source.parseConnectorSkillEventResult(skill).state, 'failed');
const legacySkill = JSON.parse(JSON.stringify(skill));
delete legacySkill.invocation.runtime_source;
assert.equal(source.parseConnectorSkillEventResult(legacySkill).state, 'failed');

const merged = source.withoutSupersededConnectorProgress([progress, skill], []);
assert.equal(merged.length, 1, 'skill and progress projections must render once');
assert.equal(merged[0].type, 'progress_text');

const restored = source.withoutSupersededConnectorProgress(
  [skill],
  [{ receipt: 'receipt-1', result: progress.result, live: false }]
);
assert.equal(restored.length, 0, 'a restored terminal event must replace a skill-only projection');

const skillOnly = source.withoutSupersededConnectorProgress([skill], []);
assert.equal(skillOnly.length, 1, 'a skill-only legacy projection must remain visible');

const restoredEvents = source.connectorChatEvents(
  {
    connector_confirmation_events: [
      {
        conversation_id: 'conversation-1',
        message_id: 'message-1',
        event_type: 'connector_confirmation_resolved',
        route_receipt: 'receipt-1',
        state: 'failed',
        error_code: presentation.error_code,
        connector_presentation: presentation,
      },
    ],
  },
  [],
  'conversation-1',
  'message-1'
);
assert.equal(restoredEvents[0].result.connector_presentation.summary, presentation.summary);

console.log('Connector runtime source identity/dedupe tests passed.');
