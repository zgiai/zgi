import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import {
  beginConnectorRuntimeDecision,
  createConnectorRuntimeDecisionState,
  isConnectorRuntimeDecisionAvailable,
  markConnectorRuntimeDecisionUnknown,
  parseConnectorRuntimeExecutionResult,
  resolveConnectorRuntimeDecision,
} from '../src/components/connectors/runtime/connector-runtime-model.ts';
import {
  ConnectorRuntimeDecisionTransport,
  isConnectorRuntimeDecisionRejected,
} from '../src/components/connectors/runtime/connector-runtime-transport.ts';
import {
  connectorConfirmationIsOneShot,
  parseConnectorConfirmationPresentation,
} from '../src/components/connectors/runtime/connector-confirmation-presentation.ts';

const confirmationResult = {
  state: 'requires_confirmation',
  decision_id: 'decision-1',
  confirmation: {
    route_receipt: 'connector-route-receipt-1',
    connector_id: 'github',
    connector_name: 'GitHub',
    connector_name_i18n: { 'zh-Hans': 'GitHub', 'de-DE-u-ca-gregory': 'GitHub' },
    operation_id: 'github.repository.delete',
    action_name: 'Delete repository',
    action_name_i18n: { 'zh-Hans': '删除仓库' },
    target: { type: 'workflow', id: 'release-workflow', binding_instance_id: 'node-7' },
    consequence: {
      external_effect: 'delete',
      description: 'Deletes the selected repository from GitHub.',
      description_i18n: { 'zh-Hans': '从 GitHub 删除所选仓库。' },
      data_egress: true,
      external_destination: 'api.github.com',
    },
    risk_classification: 'high',
  },
};

assert.deepEqual(parseConnectorRuntimeExecutionResult(confirmationResult), confirmationResult);
// A DigitalHuman chat confirmation names its published version and binding
// instance; the Chat host must accept it, and must refuse partial targets.
const digitalHumanConfirmationResult = {
  state: 'requires_confirmation',
  confirmation: {
    ...confirmationResult.confirmation,
    target: {
      type: 'digital_human',
      id: 'c37fb786-4671-4f69-b85e-31bada025c2c',
      scope: 'published',
      version_id: '25860bdd-ace2-40cb-9e3a-ea5c399bd5bc',
      binding_instance_id: '7f2e831c-221e-4e30-a6e2-ee298d4c5f9c',
    },
  },
};
assert.deepEqual(
  parseConnectorRuntimeExecutionResult(digitalHumanConfirmationResult),
  digitalHumanConfirmationResult
);
for (const target of [
  { ...digitalHumanConfirmationResult.confirmation.target, scope: 'draft' },
  { ...digitalHumanConfirmationResult.confirmation.target, version_id: undefined },
  { ...digitalHumanConfirmationResult.confirmation.target, binding_instance_id: undefined },
  { ...confirmationResult.confirmation.target, scope: 'published' },
]) {
  assert.equal(
    parseConnectorRuntimeExecutionResult({
      ...digitalHumanConfirmationResult,
      confirmation: { ...digitalHumanConfirmationResult.confirmation, target },
    }),
    null
  );
}
const terminalPresentationResult = {
  state: 'completed',
  connector_presentation: {
    schema_version: 1,
    source_kind: 'native',
    invocation_id: 'mail-send-1',
    connector_id: 'qqmail',
    operation_id: 'qqmail.message.send',
    display_status: 'successful',
    title: '发送邮件',
    summary: '邮件已成功发送。',
    redacted: true,
    truncated: false,
  },
};
const parsedTerminalPresentation = parseConnectorRuntimeExecutionResult(terminalPresentationResult);
assert.ok(parsedTerminalPresentation);
assert.equal(parsedTerminalPresentation.connector_presentation.title, '发送邮件');
assert.equal(parsedTerminalPresentation.connector_presentation.display_status, 'successful');
const failedTerminalPresentation = parseConnectorRuntimeExecutionResult({
  state: 'failed',
  error_code: 'connector_mail_submission_rejected',
  connector_presentation: {
    ...terminalPresentationResult.connector_presentation,
    display_status: 'failed',
    error_code: 'connector_mail_submission_rejected',
    summary: '邮件服务器拒绝了本次提交，请检查邮件服务状态后重试。',
  },
});
assert.ok(failedTerminalPresentation);
assert.equal(
  failedTerminalPresentation.connector_presentation.error_code,
  'connector_mail_submission_rejected'
);
const spacedTranslations = {
  ...confirmationResult,
  confirmation: {
    ...confirmationResult.confirmation,
    connector_name_i18n: { 'zh-Hans': '  GitHub  ' },
    action_name_i18n: { 'zh-Hans': '  删除仓库  ' },
    consequence: {
      ...confirmationResult.confirmation.consequence,
      description_i18n: { 'zh-Hans': '  从 GitHub 删除所选仓库。  ' },
    },
  },
};
const parsedSpacedTranslations = parseConnectorRuntimeExecutionResult(spacedTranslations);
assert.ok(parsedSpacedTranslations, 'non-empty optional translations may have surrounding spaces');
assert.equal(parsedSpacedTranslations.confirmation.connector_name_i18n['zh-Hans'], 'GitHub');
assert.equal(parsedSpacedTranslations.confirmation.action_name_i18n['zh-Hans'], '删除仓库');
assert.equal(
  parsedSpacedTranslations.confirmation.consequence.description_i18n['zh-Hans'],
  '从 GitHub 删除所选仓库。'
);
assert.equal(spacedTranslations.confirmation.action_name_i18n['zh-Hans'], '  删除仓库  ');
assert.equal(
  parsedSpacedTranslations.confirmation.route_receipt,
  confirmationResult.confirmation.route_receipt
);
assert.equal(
  parsedSpacedTranslations.confirmation.operation_id,
  confirmationResult.confirmation.operation_id
);
for (const invalidMap of [
  [],
  { 'zh-Hans': { api_key: 'must-not-render' } },
  { 'invalid locale': 'must-not-render' },
  { 'en-US': 'One', 'EN-us': 'Two' },
  { 'zh-Hans': 'One', zh_Hans: 'Two' },
  { en: 'One', 'en-US': 'Two' },
  { zh: 'One', 'zh-CN': 'Two' },
  { 'zh-CN': 'One', 'zh-Hans': 'Two' },
  { 'zh-Hans': '  ' },
  ...['\t译文', '译文\n', '译文\u0000', '\u0085译文'].map(value => ({ 'zh-Hans': value })),
]) {
  assert.equal(
    parseConnectorRuntimeExecutionResult({
      ...confirmationResult,
      confirmation: { ...confirmationResult.confirmation, action_name_i18n: invalidMap },
    }),
    null,
    'localized labels must remain bounded, flat locale-to-text maps'
  );
}
assert.equal(
  parseConnectorRuntimeExecutionResult({
    ...confirmationResult,
    confirmation: {
      ...confirmationResult.confirmation,
      operation_id: ' github.repository.delete ',
    },
  }),
  null,
  'optional translation trimming must not relax operation identity validation'
);
assert.equal(
  parseConnectorRuntimeExecutionResult({ ...confirmationResult, credential: 'must-not-render' }),
  null,
  'unknown top-level fields must not enter confirmation UI'
);
assert.equal(
  parseConnectorRuntimeExecutionResult({
    ...confirmationResult,
    confirmation: { ...confirmationResult.confirmation, api_key: 'must-not-render' },
  }),
  null,
  'unknown confirmation fields must not enter confirmation UI'
);
assert.equal(
  parseConnectorRuntimeExecutionResult({
    ...confirmationResult,
    confirmation: { ...confirmationResult.confirmation, risk_classification: 'trusted_low' },
  }),
  null,
  'trusted low-risk operations run directly and never render a confirmation'
);
assert.equal(
  parseConnectorRuntimeExecutionResult({
    ...confirmationResult,
    error_code: 'connector.confirmation.invalid',
  }),
  null,
  'a pending confirmation cannot carry a terminal error code'
);

let state = createConnectorRuntimeDecisionState();
assert.equal(isConnectorRuntimeDecisionAvailable(state, confirmationResult), true);
const confirmAttempt = beginConnectorRuntimeDecision(state, confirmationResult, 'confirmed');
assert.ok(confirmAttempt);
assert.deepEqual(confirmAttempt.command, {
  route_receipt: 'connector-route-receipt-1',
  decision: 'confirmed',
});
assert.deepEqual(
  Object.keys(confirmAttempt.command).sort(),
  ['decision', 'route_receipt'],
  'a continuation must contain only the opaque route receipt and one decision'
);
assert.equal(
  JSON.stringify(confirmAttempt.command).includes(confirmationResult.confirmation.action_name),
  false,
  'the continuation must not copy display metadata or operation input'
);
state = confirmAttempt.state;
assert.equal(isConnectorRuntimeDecisionAvailable(state, confirmationResult), false);
assert.equal(beginConnectorRuntimeDecision(state, confirmationResult, 'confirmed'), null);
state = resolveConnectorRuntimeDecision(state, confirmAttempt.command, {
  route_receipt: confirmAttempt.command.route_receipt,
  state: 'completed',
});
assert.equal(state.status, 'resolved');
assert.equal(beginConnectorRuntimeDecision(state, confirmationResult, 'cancelled'), null);

const nextResult = {
  ...confirmationResult,
  confirmation: { ...confirmationResult.confirmation, route_receipt: 'connector-route-receipt-2' },
};
const nextAttempt = beginConnectorRuntimeDecision(state, nextResult, 'cancelled');
assert.ok(nextAttempt, 'a distinct server-issued receipt represents a distinct operation');
state = markConnectorRuntimeDecisionUnknown(nextAttempt.state, nextAttempt.command);
assert.equal(state.status, 'unknown');
assert.equal(isConnectorRuntimeDecisionAvailable(state, nextResult), false);
assert.equal(
  beginConnectorRuntimeDecision(state, nextResult, 'cancelled'),
  null,
  'an unknown outcome consumes the receipt and must never be retried automatically'
);

for (const terminalResult of [
  { state: 'blocked' },
  { state: 'cancelled' },
  { state: 'completed' },
  { state: 'failed', error_code: 'connector.provider.failed' },
  { state: 'unknown', error_code: 'connector.outcome.unknown' },
]) {
  assert.deepEqual(parseConnectorRuntimeExecutionResult(terminalResult), terminalResult);
  assert.equal(
    isConnectorRuntimeDecisionAvailable(createConnectorRuntimeDecisionState(), terminalResult),
    false
  );
  assert.equal(
    beginConnectorRuntimeDecision(
      createConnectorRuntimeDecisionState(),
      terminalResult,
      'confirmed'
    ),
    null
  );
}
for (const stateWithForbiddenError of ['blocked', 'cancelled', 'completed']) {
  assert.equal(
    parseConnectorRuntimeExecutionResult({
      state: stateWithForbiddenError,
      error_code: 'connector.result.invalid',
    }),
    null,
    `${stateWithForbiddenError} cannot carry an error code`
  );
}
for (const invalidReceipt of ['', ' padded', 'line\nbreak', 'x'.repeat(129)]) {
  const malformed = {
    ...confirmationResult,
    confirmation: { ...confirmationResult.confirmation, route_receipt: invalidReceipt },
  };
  assert.equal(parseConnectorRuntimeExecutionResult(malformed), null);
  assert.equal(
    beginConnectorRuntimeDecision(createConnectorRuntimeDecisionState(), malformed, 'confirmed'),
    null
  );
}

let releasePending;
let sendCount = 0;
const pendingTransport = new ConnectorRuntimeDecisionTransport(() => {
  sendCount += 1;
  return new Promise(resolve => {
    releasePending = () => resolve({ state: 'completed', decision_id: 'decision-1' });
  });
});
const pendingCommand = { route_receipt: 'shared-pending-receipt', decision: 'confirmed' };
const firstPending = pendingTransport.submit(pendingCommand);
const concurrentPending = pendingTransport.submit(pendingCommand);
assert.equal(
  firstPending,
  concurrentPending,
  'concurrent duplicate decisions must share one Promise'
);
assert.equal(sendCount, 1, 'concurrent duplicate decisions must use one network request');
releasePending();
assert.deepEqual(await firstPending, {
  route_receipt: pendingCommand.route_receipt,
  state: 'completed',
  decision_id: 'decision-1',
});
await pendingTransport.submit(pendingCommand);
assert.equal(sendCount, 1, 'a settled receipt must never be sent again');
assert.equal(
  (await pendingTransport.submit({ ...pendingCommand, decision: 'cancelled' })).state,
  'transport_unconfirmed'
);

let unknownSendCount = 0;
const unknownTransport = new ConnectorRuntimeDecisionTransport(async () => {
  unknownSendCount += 1;
  throw new Error('network outcome is unknown');
});
const unknownCommand = { route_receipt: 'unknown-outcome-receipt', decision: 'confirmed' };
assert.equal((await unknownTransport.submit(unknownCommand)).state, 'transport_unconfirmed');
assert.equal((await unknownTransport.submit(unknownCommand)).state, 'transport_unconfirmed');
assert.equal(unknownSendCount, 1, 'unknown outcomes must never be retried');

let malformedSendCount = 0;
const malformedTransport = new ConnectorRuntimeDecisionTransport(async () => {
  malformedSendCount += 1;
  return { state: 'completed', error_code: 'connector.result.invalid' };
});
const malformedCommand = { route_receipt: 'malformed-terminal-receipt', decision: 'confirmed' };
assert.equal((await malformedTransport.submit(malformedCommand)).state, 'transport_unconfirmed');
assert.equal((await malformedTransport.submit(malformedCommand)).state, 'transport_unconfirmed');
assert.equal(malformedSendCount, 1, 'a malformed terminal response still consumes the receipt');

// Only read-only reconciliation can resolve an unconfirmed transport result.
unknownTransport.acceptStatus(unknownCommand.route_receipt, { resolution: 'unresolved' });
assert.equal(unknownTransport.getPhase(unknownCommand.route_receipt), 'transport_unconfirmed');
unknownTransport.acceptStatus(unknownCommand.route_receipt, {
  resolution: 'resolved',
  state: 'completed',
  secret: 'forbidden',
});
assert.equal(unknownTransport.getPhase(unknownCommand.route_receipt), 'transport_unconfirmed');
unknownTransport.acceptStatus(unknownCommand.route_receipt, {
  resolution: 'resolved',
  state: 'completed',
  decision_id: 'checked',
});
assert.equal(unknownTransport.getPhase(unknownCommand.route_receipt), 'completed');
unknownTransport.acceptStatus(unknownCommand.route_receipt, {
  resolution: 'resolved',
  state: 'unknown',
});
assert.equal(
  unknownTransport.getPhase(unknownCommand.route_receipt),
  'completed',
  'late reads cannot downgrade durable evidence'
);
assert.equal(unknownSendCount, 1);
assert.equal(unknownTransport.getPhase(unknownCommand.route_receipt, 'other-owner'), undefined);
const knownUnknown = new ConnectorRuntimeDecisionTransport(async () => ({ state: 'unknown' }));
assert.equal(
  (await knownUnknown.submit({ route_receipt: 'known-unknown', decision: 'confirmed' })).state,
  'unknown',
  'a server unknown is not a transport failure'
);
const remembered = new ConnectorRuntimeDecisionTransport(async () => ({
  state: 'completed',
  remember_for_session: true,
}));
const rememberedResult = await remembered.submit({
  route_receipt: 'remembered-receipt',
  decision: 'confirmed',
  remember_for_session: true,
});
assert.equal(rememberedResult.state, 'completed');
assert.equal(rememberedResult.remember_for_session, true);

// A one-shot surface refuses a session grant before anything runs. That
// refusal leaves the receipt open for a corrected decision; any other failure
// is still an unknown outcome that consumes it.
const httpError = (status, code) =>
  Object.assign(new Error('Request failed'), { response: { status, data: { code } } });
let rejectedSendCount = 0;
const rejectedTransport = new ConnectorRuntimeDecisionTransport(async command => {
  rejectedSendCount += 1;
  if (command.remember_for_session) {
    throw httpError(403, 'connector_session_grant_unsupported');
  }
  return { state: 'completed', decision_id: 'one-shot' };
});
const rejectedCommand = {
  route_receipt: 'one-shot-receipt',
  decision: 'confirmed',
  remember_for_session: true,
};
const firstRejected = rejectedTransport.submit(rejectedCommand);
assert.equal(
  rejectedTransport.submit(rejectedCommand),
  firstRejected,
  'a duplicate click while refused shares the one request'
);
await assert.rejects(firstRejected, error => isConnectorRuntimeDecisionRejected(error));
assert.equal(rejectedTransport.getPhase(rejectedCommand.route_receipt), undefined);
assert.equal(rejectedTransport.getResult(rejectedCommand.route_receipt), undefined);
const retried = await rejectedTransport.submit({
  route_receipt: rejectedCommand.route_receipt,
  decision: 'confirmed',
});
assert.equal(retried.state, 'completed', 'a refused receipt may be confirmed once more');
await rejectedTransport.submit({
  route_receipt: rejectedCommand.route_receipt,
  decision: 'confirmed',
});
assert.equal(rejectedSendCount, 2, 'the corrected decision still consumes the receipt');

for (const [label, failure] of [
  ['timeout', Object.assign(new Error('timeout'), { code: 'ECONNABORTED' })],
  ['server error', httpError(500, 'connector_internal')],
  ['other access denial', httpError(403, 'connector_access_denied')],
  ['code on another status', httpError(409, 'connector_session_grant_unsupported')],
]) {
  let sends = 0;
  const transport = new ConnectorRuntimeDecisionTransport(async () => {
    sends += 1;
    throw failure;
  });
  const command = { route_receipt: `locked-${label}`, decision: 'confirmed' };
  assert.equal((await transport.submit(command)).state, 'transport_unconfirmed', label);
  assert.equal((await transport.submit(command)).state, 'transport_unconfirmed', label);
  assert.equal(sends, 1, `${label} must never be replayed`);
}

assert.equal(connectorConfirmationIsOneShot({ version: 1, preview: null, one_shot: true }), true);
assert.equal(connectorConfirmationIsOneShot({ version: 1, preview: null }), false);
const oneShotPreview = {
  account_name: 'ops',
  account_identity: 'ops@example.com',
  kind: 'resource',
  complete: true,
  fields: [{ label: 'Command', label_i18n: {}, value: 'uptime', truncated: false }],
};
assert.ok(
  parseConnectorConfirmationPresentation({ version: 1, preview: oneShotPreview, one_shot: true }),
  'a one-shot presentation keeps its preview'
);
assert.equal(
  parseConnectorConfirmationPresentation({ version: 1, preview: oneShotPreview, one_shot: 'yes' }),
  null
);

const root = process.cwd();
const source = relativePath => readFileSync(path.join(root, relativePath), 'utf8');
const confirmationSource = source(
  'src/components/connectors/runtime/connector-runtime-confirmation-card.tsx'
);
const dialogBridgeSource = source(
  'src/components/connectors/runtime/connector-runtime-surface.tsx'
);
const previewSummarySource = source(
  'src/components/connectors/runtime/connector-runtime-preview-summary.tsx'
);
const blockedSource = source(
  'src/components/connectors/runtime/connector-runtime-blocked-state.tsx'
);
const surfaceSource = source('src/components/connectors/runtime/connector-runtime-surface.tsx');
const resultSource = source('src/components/aichat/operation-cards/operation-result-card.tsx');
const modelSource = source('src/components/connectors/runtime/connector-runtime-model.ts');
const transportSource = source('src/components/connectors/runtime/connector-runtime-transport.ts');
const receiptHookSource = source('src/components/connectors/runtime/use-runtime-receipt.ts');
const serviceSource = source('src/services/connector.service.ts');
const contractSource = source('src/services/types/connector.ts');
const chatSource = source('src/components/chat/variants/aichat/agentic-timeline.tsx');
const chatHostSource = source('src/components/chat/variants/aichat/aichat-chat.tsx');
const agentSource = source('src/components/agents/agent-runtime/preview-panel.tsx');
const workflowResultSource = source(
  'src/components/workflow/ui/workflow-run-panel/components/results.tsx'
);
const workflowNodeSource = source(
  'src/components/workflow/ui/workflow-run-nodes-list/runtime-structured-view.tsx'
);
const streamSource = source('src/components/chat/transports/aichat-transport.ts');
const connectorRuntimeSource = [
  confirmationSource,
  dialogBridgeSource,
  previewSummarySource,
  blockedSource,
  resultSource,
  surfaceSource,
  modelSource,
  transportSource,
].join('\n');

for (const field of ['connector_name', 'action_name', 'risk_classification']) {
  assert.match(previewSummarySource + dialogBridgeSource, new RegExp(field));
}
for (const field of ['fields', 'truncated', 'previewUnavailable']) {
  assert.match(previewSummarySource, new RegExp(field));
}
// A command line preview field is rendered as a wrapping code block.
assert.match(previewSummarySource, /field\.format === 'command'/);
assert.match(
  previewSummarySource,
  /<pre[\s\S]*whitespace-pre-wrap[\s\S]*<code>\{field\.value\}<\/code>/
);
assert.match(contractSource, /format\?: 'command';/);
assert.match(confirmationSource, /runtime\.confirmation\.title/);
assert.doesNotMatch(confirmationSource, /external_destination/);
assert.match(dialogBridgeSource, /ConnectorRuntimeDialogApprovalBridge/);
assert.match(dialogBridgeSource, /publishToolGovernancePendingApproval/);
assert.match(dialogBridgeSource, /ToolGovernancePendingApproval/);
assert.doesNotMatch(confirmationSource, /ToolGovernanceDecisionCard|rememberForSession/);
assert.match(surfaceSource, /result\.state === 'requires_confirmation'/);
assert.match(surfaceSource, /result\.state === 'blocked'/);
assert.match(surfaceSource, /result\.state === 'cancelled'/);
assert.match(surfaceSource, /result\.state === 'completed'/);
assert.match(surfaceSource, /result\.state === 'failed'/);
assert.match(surfaceSource, /result\.state === 'unknown'/);
assert.match(surfaceSource, /return null;/);
assert.match(surfaceSource, /<ConnectorRuntimeTerminalState/);
assert.match(serviceSource, /post', '\/connectors\/runtime\/confirmations'/);
assert.match(contractSource, /route_receipt: string;/);
assert.match(contractSource, /decision: ConnectorRuntimeConfirmationDecision;/);
assert.match(transportSource, /pendingByReceipt/);
assert.match(transportSource, /settledByReceipt/);
assert.match(transportSource, /return unknownResult\(command\.route_receipt\);\n {6}\}\)/);
assert.match(chatSource, /<ConnectorRuntimeSurface/);
assert.match(chatSource, /parseConnectorRuntimeExecutionResult/);
assert.match(chatHostSource, /recoverStreamingConversation/);
assert.doesNotMatch(chatHostSource, /continueClientAction/);
assert.match(agentSource, /surface="agent-draft"/);
assert.match(agentSource, /<Chat/);
assert.doesNotMatch(workflowResultSource, /ConnectorRuntime/);
assert.doesNotMatch(workflowNodeSource, /ConnectorRuntime/);
assert.match(
  source('src/components/workflow/ui/workflow-run-nodes-list/index.tsx'),
  /ConnectorWorkflowResult/
);
assert.match(streamSource, /connector_confirmation_required/);
assert.match(streamSource, /connector_confirmation_resolved/);
assert.match(streamSource, /error_code: envelope\.error_code/);
assert.match(dialogBridgeSource, /rememberForSession/);
assert.match(dialogBridgeSource, /allowRememberForSession: allowsSessionGrant/);
assert.match(dialogBridgeSource, /runtime\.confirmation\.sessionGrantUnsupported/);
assert.doesNotMatch(connectorRuntimeSource, /localStorage|sessionStorage|components\/integrations/);
assert.doesNotMatch(`${surfaceSource}\n${confirmationSource}`, /connectorService|axios|fetch\(/);
assert.doesNotMatch(blockedSource, /onClick|onDecision|confirmAction|cancelAction/);
assert.match(surfaceSource, /ConnectorRuntimeTerminalState/);

// T2-01: an initial status-read failure is a retryable read error, never a
// post-submit transport_unconfirmed outcome.
assert.doesNotMatch(
  receiptHookSource,
  /markUnconfirmed/,
  'a failed status read must not force transport_unconfirmed'
);
assert.match(receiptHookSource, /readFailed/, 'the receipt hook exposes a read-failure signal');
assert.doesNotMatch(
  transportSource,
  /markUnconfirmed/,
  'only a failed confirmation POST may record transport_unconfirmed'
);
assert.match(confirmationSource, /receipt\.readFailed/);
assert.match(confirmationSource, /runtime\.confirmation\.statusReadFailed/);
assert.match(confirmationSource, /runtime\.confirmation\.retryStatus/);
assert.doesNotMatch(
  confirmationSource,
  /decisionStateRef\.current = unconfirmed/,
  'a failed read must not fabricate an unknown decision state'
);

console.log('Connector runtime one-shot confirmation contract checks passed.');
