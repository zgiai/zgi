/**
 * Contract tests for the DigitalHuman durable run waiting state and the narrow
 * RUN approval surface.
 *
 * The backend parks a run in `waiting` with a `waiting_reason` and, for an
 * approval wait, a `waiting_receipt`. When the runtime-log projection returns a
 * matching `pending_connector_confirmation`, the console may offer one approval
 * card; everything else stays a data-only waiting state.
 *
 * This suite proves:
 *   - the waiting state is active/nonterminal and labeled per locale,
 *   - the strict DigitalHuman parser accepts only a fully matching projection,
 *   - a missing preview cannot consent, and
 *   - waiting_question or stale runs never offer approval.
 *
 * Run with: pnpm test:digital-human-run-status
 */

import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import ts from 'typescript';

const root = process.cwd();
const read = relativePath => readFileSync(path.join(root, relativePath), 'utf8');

const transpile = relativePath =>
  ts.transpileModule(read(relativePath), {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  }).outputText;
const asDataModule = source =>
  `data:text/javascript;base64,${Buffer.from(source).toString('base64')}`;

const {
  isActiveDigitalHumanRunStatus,
  digitalHumanRunStatusLabelKey,
  digitalHumanRunApprovalReceipt,
} = await import(
  asDataModule(transpile('src/components/digital-human/digital-human-run-presentation.ts'))
);

const { parseDigitalHumanRunConnectorConfirmation } = await import(
  asDataModule(
    transpile('src/components/digital-human/digital-human-run-confirmation-presentation.ts')
  )
);

// ---------------------------------------------------------------------------
// Active / nonterminal classification
// ---------------------------------------------------------------------------

// A waiting run is parked, not finished, so every polling surface must keep it
// live. The runtime-log projection returns message statuses, so those waiting
// values are part of the same active set.
for (const status of [
  'queued',
  'running',
  'waiting',
  'canceling',
  'pending_approval',
  'waiting_approval',
  'pending_question',
  'waiting_question',
  'waiting_client_action',
]) {
  assert.equal(
    isActiveDigitalHumanRunStatus(status),
    true,
    `${status} must count as an active, nonterminal run state`
  );
}
for (const status of [
  'succeeded',
  'failed',
  'canceled',
  'stopped',
  'unknown',
  '',
  null,
  undefined,
]) {
  assert.equal(
    isActiveDigitalHumanRunStatus(status),
    false,
    `${status} must not count as an active run state`
  );
}
assert.equal(
  isActiveDigitalHumanRunStatus(' WAITING '),
  true,
  'status classification must tolerate casing and whitespace from the runtime projection'
);

// ---------------------------------------------------------------------------
// Reason-aware labels
// ---------------------------------------------------------------------------

assert.equal(
  digitalHumanRunStatusLabelKey('waiting', 'waiting_approval'),
  'status.waitingApproval',
  'an approval wait must say it is waiting for approval'
);
assert.equal(
  digitalHumanRunStatusLabelKey('waiting', 'waiting_question'),
  'status.waitingQuestion',
  'a question wait must say it is waiting for the operator'
);
assert.equal(
  digitalHumanRunStatusLabelKey('waiting', 'waiting_client_action'),
  'status.waitingClientAction',
  'a client-action wait must say it is waiting for a page action'
);
assert.equal(
  digitalHumanRunStatusLabelKey('waiting'),
  'status.waiting',
  'a waiting run without a known reason must fall back to the generic waiting label'
);
for (const status of ['queued', 'running', 'canceling', 'succeeded', 'failed', 'canceled']) {
  assert.equal(
    digitalHumanRunStatusLabelKey(status),
    `status.${status}`,
    `a non-waiting run must keep its plain status label (${status})`
  );
}

// ---------------------------------------------------------------------------
// Approval receipt accessor
// ---------------------------------------------------------------------------

assert.equal(
  digitalHumanRunApprovalReceipt({
    status: 'waiting',
    waiting_reason: 'waiting_approval',
    waiting_receipt: 'receipt-1',
  }),
  'receipt-1',
  'the approval receipt must be available to the confirmation action'
);
assert.equal(
  digitalHumanRunApprovalReceipt({
    status: 'waiting',
    waiting_reason: 'waiting_approval',
  }),
  null,
  'an approval wait without a receipt must not pretend one exists'
);
assert.equal(
  digitalHumanRunApprovalReceipt({
    status: 'waiting',
    waiting_reason: 'waiting_question',
    waiting_receipt: 'receipt-1',
  }),
  null,
  'a non-approval wait must never expose an approval receipt'
);
assert.equal(
  digitalHumanRunApprovalReceipt({ status: 'running', waiting_receipt: 'receipt-1' }),
  null,
  'a running run must never expose an approval receipt'
);

// ---------------------------------------------------------------------------
// Strict projection parser
// ---------------------------------------------------------------------------

const digitalHumanId = '11111111-1111-4111-8111-111111111111';
const versionId = '22222222-2222-4222-8222-222222222222';
const bindingInstanceId = '33333333-3333-4333-8333-333333333333';
const routeReceipt = 'connector-route-receipt-1';

const waitingRun = {
  id: 'run-1',
  digital_human_id: digitalHumanId,
  digital_human_version_id: versionId,
  status: 'waiting',
  waiting_reason: 'waiting_approval',
  waiting_receipt: routeReceipt,
};

const validConfirmation = {
  route_receipt: routeReceipt,
  connector_id: 'github',
  connector_name: 'GitHub',
  connector_name_i18n: { 'zh-Hans': 'GitHub' },
  operation_id: 'github.issue.create',
  action_name: 'Create issue',
  action_name_i18n: { 'zh-Hans': '创建议题' },
  target: {
    type: 'digital_human',
    id: digitalHumanId,
    scope: 'published',
    version_id: versionId,
    binding_instance_id: bindingInstanceId,
  },
  consequence: {
    external_effect: 'create_issue',
    description: 'Creates an issue in the bound repository.',
    data_egress: true,
    external_destination: 'api.github.com',
  },
  risk_classification: 'high',
};

const parse = (value, { run = waitingRun, id = digitalHumanId } = {}) =>
  parseDigitalHumanRunConnectorConfirmation(value, { digitalHumanId: id, run });

// Positive: a fully matching projection becomes card-ready consent.
const parsed = parse(validConfirmation);
assert.ok(parsed, 'a fully matching projection must parse');
assert.equal(parsed.state, 'requires_confirmation');
assert.equal(parsed.confirmation.route_receipt, routeReceipt);
assert.equal(parsed.confirmation.connector_id, 'github');
assert.equal(parsed.confirmation.action_name, 'Create issue');
assert.deepEqual(parsed.confirmation.target, validConfirmation.target);
assert.equal(parsed.confirmation.consequence.data_egress, true);
assert.equal(parsed.confirmation.risk_classification, 'high');
const unclassified = parse({ ...validConfirmation, risk_classification: 'unclassified' });
assert.ok(unclassified, 'unclassified risk still requires explicit consent');
assert.equal(unclassified.confirmation.risk_classification, 'unclassified');

// Negative: extra keys on any level are rejected so secrets or frozen input can
// never be smuggled into consent UI.
assert.equal(
  parse({ ...validConfirmation, secret: 'sk-live-must-not-render' }),
  null,
  'an extra confirmation field such as a credential must fail closed'
);
assert.equal(
  parse({
    ...validConfirmation,
    accepted_operation: { credential_ref: 'must-not-render' },
  }),
  null,
  'the in-process accepted operation must never be accepted from the projection'
);
assert.equal(
  parse({
    ...validConfirmation,
    target: { ...validConfirmation.target, credential_ref: 'must-not-render' },
  }),
  null,
  'an extra target field must fail closed'
);
assert.equal(
  parse({
    ...validConfirmation,
    consequence: { ...validConfirmation.consequence, raw_input: 'must-not-render' },
  }),
  null,
  'an extra consequence field must fail closed'
);

// Negative: the projection must match the durable run.
assert.equal(
  parse({ ...validConfirmation, route_receipt: 'other-receipt' }),
  null,
  'a confirmation receipt that does not match the run waiting receipt must fail closed'
);
assert.equal(
  parse(validConfirmation, { run: { ...waitingRun, waiting_receipt: 'other-receipt' } }),
  null,
  'a stale run receipt must invalidate the projection'
);
assert.equal(
  parse({
    ...validConfirmation,
    target: { ...validConfirmation.target, id: '99999999-9999-4999-8999-999999999999' },
  }),
  null,
  'a confirmation for another digital human must fail closed'
);
assert.equal(
  parse(validConfirmation, { id: '99999999-9999-4999-8999-999999999999' }),
  null,
  'a projection loaded for another digital human id must fail closed'
);
assert.equal(
  parse({
    ...validConfirmation,
    target: { ...validConfirmation.target, version_id: '99999999-9999-4999-8999-999999999999' },
  }),
  null,
  'a confirmation for another published version must fail closed'
);
assert.equal(
  parse({
    ...validConfirmation,
    target: { ...validConfirmation.target, type: 'chat' },
  }),
  null,
  'a non digital-human target must fail closed'
);
assert.equal(
  parse({
    ...validConfirmation,
    target: { ...validConfirmation.target, scope: 'draft' },
  }),
  null,
  'a draft-scoped target must fail closed'
);
for (const target of [
  { ...validConfirmation.target, binding_instance_id: 'not-a-uuid' },
  { ...validConfirmation.target, binding_instance_id: '00000000-0000-0000-0000-000000000000' },
  { ...validConfirmation.target, binding_instance_id: '' },
  (() => {
    const { binding_instance_id: _omitted, ...rest } = validConfirmation.target;
    return rest;
  })(),
]) {
  assert.equal(
    parse({ ...validConfirmation, target }),
    null,
    'a missing or invalid binding instance must fail closed'
  );
}

// Negative: bounded, safe display fields only.
for (const value of [
  { ...validConfirmation, connector_name: '  GitHub' },
  { ...validConfirmation, connector_name: '' },
  { ...validConfirmation, connector_id: '' },
  { ...validConfirmation, action_name: '' },
  { ...validConfirmation, risk_classification: 'trusted_low' },
  (() => {
    const { risk_classification: _omitted, ...rest } = validConfirmation;
    return rest;
  })(),
  {
    ...validConfirmation,
    consequence: { ...validConfirmation.consequence, external_effect: '' },
  },
  {
    ...validConfirmation,
    consequence: { ...validConfirmation.consequence, data_egress: 'yes' },
  },
  { ...validConfirmation, route_receipt: 'line\nbreak' },
  null,
  'not-an-object',
  [],
]) {
  assert.equal(parse(value), null, 'malformed or unsafe projections must fail closed');
}
assert.equal(
  parse({
    ...validConfirmation,
    action_name_i18n: { 'zh-Hans': '创建议题', 'ZH-hans': '重复' },
  }),
  null,
  'duplicate localized labels must fail closed'
);
assert.equal(
  parse({
    ...validConfirmation,
    action_name_i18n: { 'not a locale': '创建议题' },
  }),
  null,
  'invalid localized-label keys must fail closed'
);

// Negative: only an approval wait can ever offer approval.
for (const run of [
  { ...waitingRun, status: 'succeeded' },
  { ...waitingRun, status: 'running' },
  { ...waitingRun, waiting_reason: 'waiting_question' },
  { ...waitingRun, waiting_reason: 'waiting_client_action' },
  { ...waitingRun, waiting_receipt: null },
  { ...waitingRun, waiting_receipt: '  ' },
  null,
]) {
  assert.equal(
    parse(validConfirmation, { run }),
    null,
    'a non-approval or stale run must never expose an approval action'
  );
}
assert.equal(
  parse(validConfirmation, { run: { ...waitingRun, digital_human_version_id: '' } }),
  null,
  'a run without a published version must not expose an approval action'
);

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

const typesSource = read('src/services/types/digital-human.ts');
const runStatusUnion = typesSource.match(/export type DigitalHumanRunStatus =[\s\S]*?;/)?.[0] ?? '';
assert.match(
  runStatusUnion,
  /'waiting'/,
  'DigitalHumanRunStatus must model the durable waiting state'
);
assert.match(
  typesSource,
  /export type DigitalHumanRunWaitingReason =[\s\S]*?'waiting_approval'[\s\S]*?'waiting_question'[\s\S]*?'waiting_client_action'/,
  'the waiting reason union must model every backend waiting reason'
);
assert.match(
  typesSource,
  /waiting_reason\?: DigitalHumanRunWaitingReason/,
  'DigitalHumanRun must carry the optional waiting reason'
);
assert.match(
  typesSource,
  /waiting_receipt\?: string \| null/,
  'DigitalHumanRun must carry the optional approval receipt'
);
assert.match(
  typesSource,
  /pending_connector_confirmation\?: unknown/,
  'the runtime log projection must stay unknown until the strict parser accepts it'
);

// ---------------------------------------------------------------------------
// Polling wiring
// ---------------------------------------------------------------------------

const hooksSource = read('src/hooks/digital-human/use-digital-humans.ts');
assert.equal(
  (hooksSource.match(/isActiveDigitalHumanRunStatus\(/g) ?? []).length,
  3,
  'the runs list, run detail, and runtime-log pollers must all use the shared classifier'
);
assert.doesNotMatch(
  hooksSource,
  /\['queued', 'running', 'canceling'\]/,
  'no direct status array may remain in the polling hooks'
);
assert.match(
  hooksSource,
  /return active \? 1_500 : 10_000/,
  'the runs list must stay fast while active and low-frequency when idle'
);

const runsSource = read('src/components/digital-human/digital-human-runs.tsx');
assert.match(
  runsSource,
  /isActiveDigitalHumanRunStatus\(status\)/,
  'the runs list active filter must include waiting runs'
);
assert.match(
  runsSource,
  /digitalHumanRunStatusLabelKey\(run\.status, run\.waiting_reason\)/,
  'the runs list must label a waiting run by its reason'
);

const dashboardSource = read('src/components/digital-human/digital-human-dashboard.tsx');
assert.match(
  dashboardSource,
  /runItems\.find\(run => isActiveDigitalHumanRunStatus\(run\.status\)\)/,
  'the dashboard active run must include waiting runs'
);
assert.match(
  dashboardSource,
  /digitalHumanRunStatusLabelKey\(run\.status, run\.waiting_reason\)/,
  'the dashboard recent runs must label a waiting run by its reason'
);

const runDetailSource = read('src/components/digital-human/digital-human-run-detail-drawer.tsx');
assert.match(
  runDetailSource,
  /isActiveDigitalHumanRunStatus\(run\.status\)/,
  'the run detail must keep a waiting run active so it can still be canceled'
);
assert.match(
  runDetailSource,
  /digitalHumanRunApprovalReceipt\(run\)/,
  'the run detail must gate approval on the durable approval receipt'
);
assert.match(
  runDetailSource,
  /parseDigitalHumanRunConnectorConfirmation\(/,
  'the run detail must parse the projection strictly before offering approval'
);

const transportSource = read('src/components/chat/transports/digital-human-runtime-transport.ts');
assert.match(
  transportSource,
  /isActiveDigitalHumanRunStatus\(submittedRun\.status\)/,
  'the chat transport must treat a waiting run as active when reconciling an aborted turn'
);
assert.match(
  transportSource,
  /isActiveDigitalHumanRunStatus\(run\.status\)/,
  'the chat transport must not fail a run that is waiting for a decision'
);

// Surfaces outside the run detail must not consume the approval receipt yet.
for (const [name, source] of [
  ['runs list', runsSource],
  ['dashboard', dashboardSource],
  ['runtime transport', transportSource],
]) {
  assert.doesNotMatch(
    source,
    /waiting_receipt/,
    `the ${name} must not consume the approval receipt`
  );
}

// ---------------------------------------------------------------------------
// Narrow RUN approval UI wiring
// ---------------------------------------------------------------------------

assert.match(
  runDetailSource,
  /ConnectorRuntimeConfirmationCard/,
  'the run detail must reuse the existing Connector confirmation card'
);
assert.match(
  runDetailSource,
  /requireCompletePreview/,
  'the run detail must require a complete owner-scoped preview before approval'
);
assert.match(
  runDetailSource,
  /showPreview/,
  'the run detail must display the account and operation preview'
);
assert.match(
  runDetailSource,
  /submitConnectorRuntimeDecision/,
  'the run detail must submit through the existing one-shot transport'
);
for (const key of [
  'DIGITAL_HUMAN_KEYS.run(',
  'DIGITAL_HUMAN_KEYS.runRuntimeLog(',
  'DIGITAL_HUMAN_KEYS.runs(',
]) {
  assert.ok(
    runDetailSource.includes(key),
    `a decision must invalidate ${key}…) so a stale approval cannot survive`
  );
}
assert.match(
  runDetailSource,
  /unavailableTitle/,
  'a missing or malformed projection must show a localized fail-closed message'
);
assert.doesNotMatch(
  runDetailSource,
  /remember_for_session|rememberForSession/,
  'the RUN approval UI must never offer remember-for-session'
);
assert.match(
  runDetailSource,
  /runs\.canceling/,
  'the existing run cancellation action must stay available in the footer'
);

const confirmationCardSource = read(
  'src/components/connectors/runtime/connector-runtime-confirmation-card.tsx'
);
assert.match(
  confirmationCardSource,
  /requireCompletePreview = false/,
  'the complete-preview gate must be opt-in and default off'
);
assert.match(
  confirmationCardSource,
  /showPreview = false/,
  'preview rendering must be opt-in and default off'
);
assert.match(
  confirmationCardSource,
  /receipt\.preview\?\.complete === true/,
  'the card must verify the owner-scoped preview completeness'
);
assert.match(
  confirmationCardSource,
  /decision === 'confirmed' && requireCompletePreview && !previewReady/,
  'the card must refuse approval without a complete preview'
);
assert.match(
  confirmationCardSource,
  /disabled: !canConfirm/,
  'the Confirm action must be disabled until the preview requirement is met'
);
assert.match(
  confirmationCardSource,
  /receipt\.preview\.fields\.forEach/,
  'the card must render every preview field'
);
assert.match(
  confirmationCardSource,
  /preview-account-identity/,
  'the card must render the preview account identity'
);
assert.doesNotMatch(
  confirmationCardSource,
  /remember_for_session|rememberForSession/,
  'the shared card must not offer remember-for-session'
);

// The generic DigitalHuman CREATE page must not grow approval wiring.
const createFlowSource = read('src/components/digital-human/create/digital-human-create-flow.tsx');
assert.doesNotMatch(
  createFlowSource,
  /pending_connector_confirmation|ConnectorRuntimeConfirmationCard|waiting_receipt/,
  'the generic DigitalHuman CREATE page must not change'
);

// ---------------------------------------------------------------------------
// Localization
// ---------------------------------------------------------------------------

for (const [locale, source, labels] of [
  [
    'zh-Hans',
    read('src/i18n/modules/digitalHumans/zh-Hans.ts'),
    {
      waiting: '等待中',
      waitingApproval: '等待审批',
      waitingQuestion: '等待你补充信息',
      waitingClientAction: '等待页面操作',
    },
  ],
  [
    'en-US',
    read('src/i18n/modules/digitalHumans/en-US.ts'),
    {
      waiting: 'Waiting',
      waitingApproval: 'Waiting for approval',
      waitingQuestion: 'Waiting for your response',
      waitingClientAction: 'Waiting for page action',
    },
  ],
]) {
  for (const [key, label] of Object.entries(labels)) {
    assert.match(
      source,
      new RegExp(`\\b${key}:\\s*'${label.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}'`),
      `${locale} must translate status.${key}`
    );
  }
  for (const key of [
    'accountName',
    'accountIdentity',
    'previewUnavailable',
    'unavailableTitle',
    'unavailableDescription',
    'unavailableNoReceipt',
  ]) {
    assert.match(
      source,
      new RegExp(`\\b${key}:`),
      `${locale} must translate runs.connectorConfirmation.${key}`
    );
  }
}

console.log('Digital human run waiting-state and approval checks passed');
