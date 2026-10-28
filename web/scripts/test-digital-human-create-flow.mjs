/**
 * Contract tests for the digital human template creation flow.
 *
 * These run without rendering anything: they prove what a template selection
 * creates, which real skills it declares and resolves from backend candidates,
 * what a template still needs before creation may proceed, and that the UI
 * layer keeps its guarantees (single submit, auto publish, no fake capability
 * claims).
 *
 * Templates are plain data served by the API (ADR 0022), so this suite builds
 * small fixture templates instead of reading a hard-coded client catalog. The
 * built-in template copy itself is owned and tested by the API.
 *
 * Creation no longer reads, selects, or binds the old integration connections:
 * a template that needs a server connection declares it in `connectors` and
 * binds no skill by default. Phase one reaches servers through the
 * DigitalHuman-only serverops_ssh connector the user verifies in Connector
 * Center and binds in the generic create form.
 *
 * Run with: pnpm test:digital-human-create-flow
 */

import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { readFile, readdir } from 'node:fs/promises';
import path from 'node:path';
import { URL } from 'node:url';

import ts from 'typescript';

import enUS from '../src/i18n/modules/digitalHumans/en-US.ts';
import zhHans from '../src/i18n/modules/digitalHumans/zh-Hans.ts';
import * as templateModule from '../src/components/digital-human/create/digital-human-templates.ts';
import {
  bindingDisplayNames,
  buildPersona,
  createDigitalHumanPayload,
  selectCandidateBindings,
  selectTemplateBindings,
  serializeDigitalHumanIcon,
  templateDefaultSkillIds,
} from '../src/components/digital-human/create/create-digital-human-payload.ts';

const {
  CUSTOM_TEMPLATE_ID,
  findDigitalHumanTemplate,
  isCustomTemplateId,
  templateRequirementGap,
  templateStartingName,
} = templateModule;

// Node cannot resolve the extensionless TypeScript specifiers the create-time
// bindings helper uses to share the Connector model, so that pure module is
// transpiled and its local imports inlined as data URLs, like the other pure
// model suites do.
const transpileSource = relativePath =>
  ts.transpileModule(readFileSync(path.join(process.cwd(), relativePath), 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  }).outputText;
const asDataModule = source =>
  `data:text/javascript;base64,${Buffer.from(source).toString('base64')}`;

const connectorModelModuleURL = asDataModule(
  transpileSource('src/components/digital-human/digital-human-connector-model.ts')
);
const createPayloadModuleURL = asDataModule(
  transpileSource('src/components/digital-human/create/create-digital-human-payload.ts')
);
const createBindingsModuleURL = asDataModule(
  transpileSource('src/components/digital-human/create/create-digital-human-bindings.ts')
    .replace(
      /from\s+['"]\.\.\/digital-human-connector-model['"];/,
      `from '${connectorModelModuleURL}';`
    )
    .replace(
      /from\s+['"]\.\/create-digital-human-payload['"];/,
      `from '${createPayloadModuleURL}';`
    )
);
const { composeCreateBindings } = await import(createBindingsModuleURL);
const connectorCreateModelModuleURL = asDataModule(
  transpileSource(
    'src/components/digital-human/create/digital-human-connector-create-model.ts'
  ).replace(
    /from\s+['"]\.\.\/digital-human-connector-model['"];/,
    `from '${connectorModelModuleURL}';`
  )
);
const {
  connectorApprovalPolicy,
  connectorOperationIds,
  connectorSupportsReadOnlyAuto,
  createConnectorDefaultOperationIds,
  updateDigitalHumanApprovalPolicy,
} = await import(
  connectorModelModuleURL
);
const {
  addCreateConnectorSelection,
  createConnectorCandidateKey,
  createConnectorCandidateSelectable,
  createConnectorSelectionBindings,
  createConnectorSelectionCandidates,
  removeCreateConnectorSelection,
  setCreateConnectorSelectionOperation,
  toggleCreateConnectorSelection,
} = await import(connectorCreateModelModuleURL);

/** The only keys the create request may ever carry. */
const EXPECTED_PAYLOAD_KEYS = [
  'autonomy_level',
  'default_model',
  'description',
  'goals',
  'icon',
  'icon_type',
  'identity_name',
  'memory_policy',
  'name',
  'persona',
  'rules',
  'trigger_policy',
  'visibility',
];

function resolvePath(dictionary, key) {
  return key
    .split('.')
    .reduce(
      (node, part) => (node === undefined || node === null ? undefined : node[part]),
      dictionary
    );
}

function collectLeafPaths(value, prefix = '') {
  if (Array.isArray(value)) return [`${prefix}[]`];
  if (value && typeof value === 'object') {
    return Object.keys(value).flatMap(key =>
      collectLeafPaths(value[key], prefix ? `${prefix}.${key}` : key)
    );
  }
  return [prefix];
}

// ---------------------------------------------------------------------------
// Fixture templates (ADR 0022: templates are plain data from the API)
// ---------------------------------------------------------------------------

/** The safety rules every digital human starts with, as the catalog serves them. */
const SHARED_RULES = ['不得索取或保存凭据。', '不确定时先询问用户。'];

const fileGeneration = {
  id: 'fileGeneration',
  source: 'builtin',
  title: '文件生成助手',
  description: '把资料整理成可交付文件。',
  responsibility: '把用户提供的资料整理成可交付的文件。',
  persona: '你是一名严谨的文件整理助手。',
  goals: ['产出结构清晰的文件', '保留来源', '按时交付'],
  rules: [...SHARED_RULES, '生成前确认文件格式。'],
  tags: ['文件', '整理', '交付'],
  skills: [{ id: 'file-generator', required: true }],
  connectors: [],
  memory_instructions: '  记住用户偏好的文件格式。  ',
  icon_background: '#0f766e',
};

const opsSpecialist = {
  id: 'opsSpecialist',
  source: 'builtin',
  title: '运维专员',
  description: '对已绑定的服务器做只读检查。',
  responsibility: '对已绑定的服务器做只读检查。',
  persona: '你是一名谨慎、证据优先的运维专员。',
  goals: ['确认目标服务器', '只读检查', '区分观察与推断'],
  rules: [...SHARED_RULES, '每次调用都需要单独批准。'],
  tags: ['运维', '只读', '证据'],
  skills: [],
  connectors: [
    {
      connector_id: 'serverops_ssh',
      required_operation_ids: ['server.system.snapshot', 'server.service.status', 'server.journal.tail'],
      optional_operation_ids: ['server.service.restart'],
      required: true,
    },
  ],
  memory_instructions: '',
  icon_background: '#1d4ed8',
};

const workspaceTemplate = {
  id: 'tpl-workspace-1',
  source: 'workspace',
  title: '周报模板',
  description: '团队保存的周报助手。',
  identity_name: '  小周  ',
  responsibility: '整理每周进度。',
  persona: '你负责整理团队周报。',
  goals: ['汇总进度'],
  rules: ['只引用真实进度。'],
  tags: ['周报'],
  skills: [{ id: 'file-generator', required: false }],
  connectors: [],
  memory_instructions: '   ',
  revision: 3,
};

const TEMPLATES = [fileGeneration, opsSpecialist, workspaceTemplate];

// ---------------------------------------------------------------------------
// Template module surface
// ---------------------------------------------------------------------------

assert.equal(CUSTOM_TEMPLATE_ID, 'custom', 'the custom id is part of the create-page URL contract');
for (const removed of ['DIGITAL_HUMAN_TEMPLATES', 'SHARED_RULE_KEYS', 'templateKey']) {
  assert.ok(
    !(removed in templateModule),
    `the client must not keep a hard-coded template catalog (${removed})`
  );
}
assert.equal(isCustomTemplateId('custom'), true);
assert.equal(isCustomTemplateId('fileGeneration'), false);
assert.equal(isCustomTemplateId(null), false);
assert.equal(isCustomTemplateId(undefined), false);

assert.equal(findDigitalHumanTemplate(TEMPLATES, 'fileGeneration'), fileGeneration);
assert.equal(findDigitalHumanTemplate(TEMPLATES, 'tpl-workspace-1'), workspaceTemplate);
assert.equal(findDigitalHumanTemplate(TEMPLATES, 'not-a-template'), null);
assert.equal(
  findDigitalHumanTemplate(TEMPLATES, CUSTOM_TEMPLATE_ID),
  null,
  'the custom entry is never a template'
);
assert.equal(findDigitalHumanTemplate(TEMPLATES, null), null);
assert.equal(findDigitalHumanTemplate(TEMPLATES, undefined), null);
assert.equal(findDigitalHumanTemplate([], 'fileGeneration'), null, 'an empty catalog finds nothing');

// A saved template starts with its own name; a built-in one with its title.
assert.equal(templateStartingName(workspaceTemplate), '小周');
assert.equal(templateStartingName(fileGeneration), '文件生成助手');
assert.equal(
  templateStartingName({ ...workspaceTemplate, identity_name: '   ' }),
  '周报模板',
  'a blank saved name falls back to the title'
);

// ---------------------------------------------------------------------------
// Template requirements
// ---------------------------------------------------------------------------

const fileCandidate = {
  type: 'skill',
  resource_id: 'file-generator',
  display_name: 'file-generator',
  resource_revision: 'sha256:bbb',
  access_modes: ['write', 'read'],
  metadata: {},
  status: 'active',
  available: true,
};

assert.equal(templateRequirementGap(null, [], [], 0), null, 'the custom entry needs nothing');
assert.deepEqual(
  templateRequirementGap(fileGeneration, [], ['file-generator'], 0),
  { missingSkillIds: ['file-generator'], connectorMissing: false },
  'a required skill the workspace does not offer blocks creation'
);
assert.deepEqual(
  templateRequirementGap(
    fileGeneration,
    [{ ...fileCandidate, available: false }],
    ['file-generator'],
    0
  ),
  { missingSkillIds: ['file-generator'], connectorMissing: false },
  'an unavailable candidate does not satisfy a required skill'
);
assert.deepEqual(
  templateRequirementGap(fileGeneration, [fileCandidate], [], 0),
  { missingSkillIds: ['file-generator'], connectorMissing: false },
  'a required skill the user deselected blocks creation'
);
assert.equal(
  templateRequirementGap(fileGeneration, [fileCandidate], ['file-generator'], 0),
  null,
  'an offered and selected required skill satisfies the template'
);
assert.equal(
  templateRequirementGap(workspaceTemplate, [], [], 0),
  null,
  'an optional skill never blocks creation'
);
assert.deepEqual(
  templateRequirementGap(opsSpecialist, [], [], 0),
  { missingSkillIds: [], connectorMissing: true },
  'a template that needs a connection blocks creation until one is selected'
);
assert.equal(
  templateRequirementGap(opsSpecialist, [], [], 1),
  null,
  'a selected connection satisfies the template'
);
assert.deepEqual(
  templateRequirementGap({ ...fileGeneration, connectors: opsSpecialist.connectors }, [], [], 0),
  { missingSkillIds: ['file-generator'], connectorMissing: true },
  'every missing requirement is reported together'
);

// ---------------------------------------------------------------------------
// Locale coverage
// ---------------------------------------------------------------------------

for (const [locale, dictionary] of [
  ['en-US', enUS],
  ['zh-Hans', zhHans],
]) {
  for (const key of [
    'createFlow.gallery.builtinTitle',
    'createFlow.gallery.workspaceTitle',
    'createFlow.gallery.loadFailed',
    'createFlow.gallery.retry',
    'createFlow.detail.requiredSkill',
    'createFlow.detail.memory',
    'createFlow.detail.connector',
    'createFlow.form.requiredSkillsMissing',
    'createFlow.form.connectorRequired',
  ]) {
    const value = resolvePath(dictionary, key);
    assert.ok(typeof value === 'string' && value.trim(), `${locale} is missing ${key}`);
  }
  assert.equal(
    dictionary.createFlow.templates,
    undefined,
    `${locale} must not keep client-side template copy (ADR 0022)`
  );
  assert.equal(
    dictionary.createFlow.sharedRules,
    undefined,
    `${locale} must not keep client-side shared rules (ADR 0022)`
  );
  assert.equal(
    dictionary.createFlow.form.templateNotEnabled,
    undefined,
    `${locale} must not expose the removed template capability warning`
  );
}

assert.deepEqual(
  collectLeafPaths(zhHans.createFlow).sort(),
  collectLeafPaths(enUS.createFlow).sort(),
  'zh-Hans and en-US createFlow keys must match'
);

for (const [locale, dictionary] of [
  ['en-US', enUS],
  ['zh-Hans', zhHans],
]) {
  const arrays = [];
  const emptyLeaves = [];
  const walk = (value, prefix) => {
    if (Array.isArray(value)) {
      arrays.push(prefix);
      return;
    }
    if (value && typeof value === 'object') {
      Object.keys(value).forEach(key => walk(value[key], prefix ? `${prefix}.${key}` : key));
      return;
    }
    if (typeof value === 'string' && !value.trim()) emptyLeaves.push(prefix);
  };
  walk(dictionary.createFlow, 'createFlow');
  assert.deepEqual(emptyLeaves, [], `${locale} has empty create-flow copy`);
  // Array values leak `number` into the shared DotNotationKeys union and break
  // `pnpm type-check`; lists of copy are addressed by named keys instead.
  assert.deepEqual(
    arrays,
    [],
    `${locale} must not use array values in the message tree; use named keys`
  );
}

// ---------------------------------------------------------------------------
// Persona composition
// ---------------------------------------------------------------------------

assert.equal(
  buildPersona(fileGeneration, fileGeneration.responsibility),
  fileGeneration.persona,
  'an unchanged responsibility leaves the template persona as is'
);
assert.equal(
  buildPersona(fileGeneration, `  ${fileGeneration.responsibility}  `),
  fileGeneration.persona,
  'whitespace around an unchanged responsibility does not count as a change'
);
assert.equal(
  buildPersona(fileGeneration, '   '),
  fileGeneration.persona,
  'an empty responsibility adds nothing'
);
const duty = '负责把每周进度整理成可交付文件。';
const persona = buildPersona(fileGeneration, duty);
assert.equal(
  persona,
  `${fileGeneration.persona}\n\n${duty}`,
  'a changed responsibility is added verbatim as a plain paragraph'
);
assert.ok(!persona.includes('##'), 'the persona must not carry fixed headings');
assert.ok(!persona.includes('职责范围'), 'the persona must not carry the old Chinese section labels');

// ---------------------------------------------------------------------------
// Icon serialization
// ---------------------------------------------------------------------------

assert.deepEqual(
  serializeDigitalHumanIcon({ type: 'text', icon: 'D', iconBackground: '#0f766e' }, 'X', '#000000'),
  { icon_type: 'text', icon: JSON.stringify({ icon: 'D', icon_background: '#0f766e' }) }
);
assert.deepEqual(
  serializeDigitalHumanIcon({ type: 'text', icon: '', iconBackground: '' }, '小项', '#0f766e'),
  { icon_type: 'text', icon: JSON.stringify({ icon: '小项', icon_background: '#0f766e' }) }
);
assert.deepEqual(
  serializeDigitalHumanIcon({ type: 'text', icon: '', iconBackground: '' }, '!', ''),
  { icon_type: 'text', icon: JSON.stringify({ icon: '!', icon_background: '#0847f7' }) }
);
assert.deepEqual(
  serializeDigitalHumanIcon(
    { type: 'image', iconUrl: 'https://example.com/a.png', imageId: 'img-1' },
    'X',
    '#000000'
  ),
  { icon_type: 'image', icon: 'img-1' }
);

// ---------------------------------------------------------------------------
// Create request
// ---------------------------------------------------------------------------

const templatePayload = createDigitalHumanPayload({
  name: '  小研  ',
  responsibility: `  ${duty}  `,
  icon: { type: 'text', icon: 'XY', iconBackground: '' },
  model: 'deepseek-chat',
  template: fileGeneration,
  sharedRules: SHARED_RULES,
});

assert.deepEqual(
  Object.keys(templatePayload).filter(key => key !== 'template_id').sort(),
  EXPECTED_PAYLOAD_KEYS,
  'the create request must only carry the agreed fields, never credentials or capability claims'
);
// A built-in template names itself so the API keeps its required items as
// setup requirements (ADR 0023); the blank entry and saved templates do not.
assert.equal(templatePayload.template_id, fileGeneration.id);
assert.equal(templatePayload.name, '小研');
assert.equal(templatePayload.identity_name, '小研');
assert.equal(templatePayload.description, duty);
assert.equal(templatePayload.default_model, 'deepseek-chat');
assert.equal(templatePayload.autonomy_level, 'supervised');
assert.equal(templatePayload.visibility, 'workspace');
assert.deepEqual(
  templatePayload.memory_policy,
  { instructions: '记住用户偏好的文件格式。' },
  'the template memory instructions guide memory; nothing else is written'
);
assert.deepEqual(templatePayload.trigger_policy, { manual: true });
assert.equal(templatePayload.persona, `${fileGeneration.persona}\n\n${duty}`);
assert.equal(
  JSON.parse(templatePayload.icon).icon_background,
  '#0f766e',
  'the template avatar colour is the text-avatar fallback'
);
assert.deepEqual(templatePayload.goals, fileGeneration.goals, 'goals must come from the template');
assert.notEqual(templatePayload.goals, fileGeneration.goals, 'goals must be copied, not shared');
assert.deepEqual(templatePayload.rules, fileGeneration.rules, 'rules must come from the template');
assert.notEqual(templatePayload.rules, fileGeneration.rules, 'rules must be copied, not shared');
assert.ok(
  !('skills' in templatePayload) && !('bindings' in templatePayload),
  'skills must be bound through the bindings API, not smuggled into the create request'
);

const credentialKeyPattern = /password|passwd|token|secret|private|credential|ssh/i;
for (const key of Object.keys(templatePayload)) {
  assert.ok(
    !credentialKeyPattern.test(key),
    `the create request must not carry a credential field (found ${key})`
  );
}

// Every template must produce a usable digital human, and memory_policy only
// ever carries instructions: memory is always on (ADR 0019).
for (const template of TEMPLATES) {
  const payload = createDigitalHumanPayload({
    name: 'x',
    responsibility: template.responsibility,
    icon: { type: 'text', icon: 'X', iconBackground: '' },
    model: 'm',
    template,
    sharedRules: SHARED_RULES,
  });
  assert.deepEqual(Object.keys(payload).filter(key => key !== 'template_id').sort(), EXPECTED_PAYLOAD_KEYS);
  assert.equal(payload.persona, template.persona, `${template.id} persona must be the template persona`);
  assert.deepEqual(payload.goals, template.goals, `${template.id} must write its goals`);
  assert.deepEqual(payload.rules, template.rules, `${template.id} must write its rules`);
  assert.equal(payload.default_model, 'm');
  assert.ok(
    !('enabled' in payload.memory_policy) && !('ownership' in payload.memory_policy),
    `${template.id} memory_policy must not carry enabled/ownership`
  );
  assert.deepEqual(
    payload.memory_policy,
    template.memory_instructions.trim() ? { instructions: template.memory_instructions.trim() } : {},
    `${template.id} memory_policy carries only its instructions`
  );
}

// The connector template must create a usable digital human without smuggling
// a binding, concrete connection, or credential into the create request.
const opsPayload = createDigitalHumanPayload({
  name: '运维专员',
  responsibility: '整理环境问题证据。',
  icon: { type: 'text', icon: '', iconBackground: '' },
  model: 'deepseek-chat',
  template: opsSpecialist,
  sharedRules: SHARED_RULES,
});
assert.deepEqual(Object.keys(opsPayload).filter(key => key !== 'template_id').sort(), EXPECTED_PAYLOAD_KEYS);
assert.deepEqual(opsPayload.memory_policy, {});
assert.ok(
  opsPayload.persona.startsWith(opsSpecialist.persona),
  'the operations create request must carry the reviewed persona'
);
for (const forbiddenField of [
  'bindings',
  'connections',
  'connection_id',
  'credentials',
  'credential_ref',
  'allowed_action_ids',
]) {
  assert.ok(
    !(forbiddenField in opsPayload),
    `the operations create request must not carry ${forbiddenField}`
  );
}

// Custom (blank) entry: the responsibility is the persona, and it starts with
// the shared safety rules.
const customPayload = createDigitalHumanPayload({
  name: '自定义助手',
  responsibility: '整理每周进度。',
  icon: { type: 'text', icon: '', iconBackground: '' },
  model: 'deepseek-chat',
  template: null,
  sharedRules: SHARED_RULES,
});
assert.deepEqual(Object.keys(customPayload).filter(key => key !== 'template_id').sort(), EXPECTED_PAYLOAD_KEYS);
assert.equal(customPayload.persona, '整理每周进度。');
assert.deepEqual(customPayload.goals, []);
assert.deepEqual(customPayload.rules, SHARED_RULES, 'the custom entry starts with the shared rules');
assert.notEqual(customPayload.rules, SHARED_RULES, 'shared rules must be copied, not shared');
assert.equal(customPayload.visibility, 'workspace');
assert.equal(customPayload.autonomy_level, 'supervised');
assert.deepEqual(customPayload.trigger_policy, { manual: true });
assert.deepEqual(customPayload.memory_policy, {}, 'the custom entry writes an empty memory policy');
assert.equal(
  JSON.parse(customPayload.icon).icon_background,
  '#0847f7',
  'the custom entry falls back to the default avatar colour'
);

// ---------------------------------------------------------------------------
// Skill selection against the workspace's real candidates
// ---------------------------------------------------------------------------

const candidates = [
  {
    type: 'skill',
    resource_id: 'file-generator',
    display_name: 'file-generator',
    resource_revision: 'sha256:bbb',
    access_modes: ['write', 'read'],
    metadata: {},
    status: 'active',
    available: false,
  },
];

const selected = selectTemplateBindings(fileGeneration, candidates);
assert.deepEqual(selected, [], 'an unavailable skill must never be bound');

const available = candidates.map(candidate => ({ ...candidate, available: true }));
const boundOnce = selectTemplateBindings(fileGeneration, available);
assert.equal(boundOnce.length, 1, 'a template must bind the skill once it is available');
assert.deepEqual(
  boundOnce,
  [
    {
      type: 'skill',
      resource_id: 'file-generator',
      parent_resource_id: undefined,
      display_name: 'file-generator',
      access_mode: 'write',
      resource_revision: 'sha256:bbb',
      metadata: {},
    },
  ],
  'the binding must carry the candidate revision and the first access mode the candidate offers'
);
assert.deepEqual(
  selectTemplateBindings(null, available),
  [],
  'the custom entry binds nothing'
);
assert.deepEqual(
  selectTemplateBindings(opsSpecialist, available),
  [],
  'a template that declares no skill must never fake a binding'
);
assert.deepEqual(
  selectTemplateBindings(fileGeneration, []),
  [],
  'a missing candidate must be skipped instead of invented'
);
assert.deepEqual(
  bindingDisplayNames([{ display_name: 'File Generator', resource_id: 'x' }, { resource_id: 'y' }]),
  ['File Generator', 'y'],
  'bound skill names fall back to the resource id'
);

// ---------------------------------------------------------------------------
// The user's own skill selection during creation
// ---------------------------------------------------------------------------

const offered = [
  { ...candidates[0], available: true, resource_id: 'ppt-slide-planner', display_name: 'ppt' },
  { ...candidates[0], available: true },
  {
    ...candidates[0],
    available: true,
    resource_id: 'content-summary',
    display_name: 'content-summary',
  },
  { ...candidates[0], resource_id: 'chart-generator', display_name: 'chart-generator' },
];

assert.deepEqual(
  templateDefaultSkillIds(fileGeneration, offered),
  ['file-generator'],
  'a template only pre-selects the skills the workspace really offers'
);
assert.deepEqual(
  templateDefaultSkillIds(null, offered),
  [],
  'the custom entry starts with nothing pre-selected'
);
assert.deepEqual(
  templateDefaultSkillIds(fileGeneration, []),
  [],
  'an unavailable workspace pre-selects nothing'
);

assert.deepEqual(
  selectCandidateBindings(['content-summary', 'file-generator'], offered).map(
    item => item.resource_id
  ),
  ['file-generator', 'content-summary'],
  'only the selected, available skills are bound, in candidate order'
);
assert.deepEqual(
  selectCandidateBindings(['chart-generator', 'not-offered'], offered),
  [],
  'a selection the workspace cannot satisfy binds nothing instead of inventing it'
);
assert.deepEqual(selectCandidateBindings([], offered), [], 'an empty selection binds nothing');
assert.equal(
  selectCandidateBindings(['file-generator'], offered)[0].access_mode,
  'write',
  'the binding must reuse the access mode the candidate offers'
);

// The creation request still cannot carry a binding, connection, or credential.
for (const forbiddenField of [
  'bindings',
  'connection_id',
  'connection',
  'integration_connection',
  'credentials',
  'credential_ref',
]) {
  assert.ok(
    !(forbiddenField in templatePayload),
    `the create request must not carry ${forbiddenField}`
  );
}

// ---------------------------------------------------------------------------
// Create-time bindings: skills plus Connector connections in one replacement
// ---------------------------------------------------------------------------

const connectorCandidates = [
  {
    type: 'connector',
    resource_id: 'connection-1',
    parent_resource_id: 'calendar',
    display_name: 'Calendar primary',
    resource_revision: 'connector:1',
    access_modes: ['execute'],
    metadata: { allowed_operation_ids: ['events.read', 'events.write', 'calendars.read'] },
    status: 'ready',
    available: true,
  },
  {
    type: 'connector',
    resource_id: 'connection-2',
    parent_resource_id: 'calendar',
    display_name: 'Calendar team',
    resource_revision: 'connector:2',
    access_modes: ['execute'],
    metadata: { allowed_operation_ids: ['events.read', 'calendars.read'] },
    status: 'ready',
    available: true,
  },
  {
    type: 'connector',
    resource_id: 'connection-3',
    parent_resource_id: 'mail',
    display_name: 'Mail shared',
    resource_revision: 'connector:3',
    access_modes: ['execute'],
    metadata: { allowed_operation_ids: ['messages.send'] },
    status: 'ready',
    available: true,
  },
];

const connectorSelection = (connectorId, connectionId, operationIds, resourceRevision) => ({
  type: 'connector',
  resource_id: connectionId,
  parent_resource_id: connectorId,
  display_name: connectionId,
  access_mode: 'execute',
  resource_revision: resourceRevision,
  metadata: { allowed_operation_ids: operationIds },
});

const selectedConnections = [
  connectorSelection('calendar', 'connection-1', ['events.read'], 'connector:1'),
  connectorSelection('calendar', 'connection-2', ['calendars.read', 'events.read'], 'connector:2'),
  connectorSelection('mail', 'connection-3', ['messages.send'], 'connector:3'),
];

const mergedBindings = composeCreateBindings(
  ['file-generator', 'not-offered'],
  offered,
  selectedConnections,
  connectorCandidates
);

// One merged replacement list, in skill-then-connector order.
assert.deepEqual(
  mergedBindings.map(item => `${item.type}:${item.parent_resource_id ?? ''}:${item.resource_id}`),
  [
    'skill::file-generator',
    'connector:calendar:connection-1',
    'connector:calendar:connection-2',
    'connector:mail:connection-3',
  ],
  'the helper must return one list with the offered skill and every selected connection'
);
assert.deepEqual(
  mergedBindings[0],
  selectCandidateBindings(['file-generator'], offered)[0],
  'skills must keep the existing candidate-resolved binding behaviour'
);
assert.ok(
  mergedBindings.every(item => item.type === 'skill' || item.type === 'connector'),
  'only skills and native Connector bindings may enter the create-time replacement list'
);

// Two distinct connections of the same connector both survive.
const sameConnector = mergedBindings.filter(item => item.parent_resource_id === 'calendar');
assert.equal(sameConnector.length, 2, 'two connections of one connector must both survive');
assert.deepEqual(
  sameConnector.map(item => item.resource_id),
  ['connection-1', 'connection-2'],
  'same-connector connections keep their own identity'
);

// Allowed operation narrowing, current revision, and credential freedom.
assert.deepEqual(
  mergedBindings.find(item => item.resource_id === 'connection-1').metadata.allowed_operation_ids,
  ['events.read'],
  'a narrowed selection must carry exactly the operations the user chose'
);
assert.deepEqual(
  mergedBindings.find(item => item.resource_id === 'connection-2').metadata.allowed_operation_ids,
  ['calendars.read', 'events.read'],
  'the selected operation order must be preserved for the chosen connection'
);
assert.equal(
  mergedBindings.find(item => item.resource_id === 'connection-1').resource_revision,
  'connector:1',
  'the output must carry the current candidate revision'
);
for (const item of mergedBindings) {
  assert.ok(
    !Object.prototype.hasOwnProperty.call(item, 'credential_ref'),
    'the merged list must never carry a credential reference'
  );
}
for (const item of mergedBindings.filter(item => item.type === 'connector')) {
  assert.ok(
    item.metadata.allowed_operation_ids.length > 0,
    'every connector binding must name at least one allowed operation'
  );
}
assert.doesNotMatch(
  JSON.stringify(mergedBindings),
  /credential|secret|password|token|ssh/i,
  'the merged list must not leak credential material or a credential handle'
);

// Connector metadata is an allowlist, not a copy: a dirty or future candidate
// carrying extra secret-like values must not leak them into the request.
const dirtyConnectorCandidates = [
  {
    ...connectorCandidates[0],
    metadata: {
      allowed_operation_ids: ['events.read', 'events.write'],
      credential_ref: 'credential-handle-must-not-travel',
      password: 'super-secret-must-not-travel',
      nested: { access_token: 'token-must-not-travel' },
    },
  },
];
const dirtyComposed = composeCreateBindings(
  [],
  [],
  [connectorSelection('calendar', 'connection-1', ['events.read'], 'connector:1')],
  dirtyConnectorCandidates
);
assert.equal(dirtyComposed.length, 1, 'a dirty connector candidate must still compose');
assert.deepEqual(
  Object.keys(dirtyComposed[0].metadata),
  ['allowed_operation_ids'],
  'connector metadata must contain exactly the allowlisted operation ids'
);
assert.deepEqual(
  dirtyComposed[0].metadata.allowed_operation_ids,
  ['events.read'],
  'the allowlisted operations must be the validated selection, not the candidate metadata'
);
assert.doesNotMatch(
  JSON.stringify(dirtyComposed),
  /credential|secret|password|token|ssh|must-not-travel/i,
  'no extra candidate metadata, secret value, or credential key may reach the binding'
);

// An exact repeated selection is deduplicated, not sent twice.
assert.deepEqual(
  composeCreateBindings(
    [],
    [],
    [
      selectedConnections[0],
      connectorSelection('calendar', 'connection-1', ['events.read'], 'connector:1'),
    ],
    connectorCandidates
  ).map(item => item.resource_id),
  ['connection-1'],
  'an exact repeated selection must collapse to one binding'
);
assert.deepEqual(
  composeCreateBindings([], [], [], connectorCandidates),
  [],
  'nothing selected composes an empty replacement list instead of a call'
);

// Every stale, missing, unavailable, unready, widening, or empty selection
// fails explicitly rather than disappearing or publishing a different scope.
assert.throws(
  () =>
    composeCreateBindings(
      [],
      [],
      [connectorSelection('calendar', 'connection-1', ['events.read'], 'connector:old')],
      connectorCandidates
    ),
  /changed since it was selected/,
  'a stale selection must fail instead of publishing an old revision'
);
assert.throws(
  () =>
    composeCreateBindings(
      [],
      [],
      [connectorSelection('calendar', 'connection-9', ['events.read'], 'connector:1')],
      connectorCandidates
    ),
  /not offered to this workspace/,
  'a selection with no exact candidate must fail instead of disappearing'
);
assert.throws(
  () =>
    composeCreateBindings(
      [],
      [],
      [connectorSelection('other-connector', 'connection-1', ['events.read'], 'connector:1')],
      connectorCandidates
    ),
  /not offered to this workspace/,
  'the candidate must match the exact connector and connection pair'
);
assert.throws(
  () =>
    composeCreateBindings(
      [],
      [],
      [selectedConnections[0]],
      connectorCandidates.map(candidate =>
        candidate.resource_id === 'connection-1' ? { ...candidate, available: false } : candidate
      )
    ),
  /is not available/,
  'an unavailable connection must fail instead of being dropped'
);
assert.throws(
  () =>
    composeCreateBindings(
      [],
      [],
      [selectedConnections[0]],
      connectorCandidates.map(candidate =>
        candidate.resource_id === 'connection-1' ? { ...candidate, available: undefined } : candidate
      )
    ),
  /is not available/,
  'a candidate that is not explicitly ready must fail'
);
assert.throws(
  () =>
    composeCreateBindings(
      [],
      [],
      [connectorSelection('calendar', 'connection-2', ['events.write'], 'connector:2')],
      connectorCandidates
    ),
  /outside its allowed set/,
  'a selection may never widen the operations the candidate allows'
);
assert.throws(
  () =>
    composeCreateBindings(
      [],
      [],
      [connectorSelection('calendar', 'connection-1', [], 'connector:1')],
      connectorCandidates
    ),
  /has no selected operations/,
  'a connection with no selected operations must fail instead of binding everything'
);
assert.throws(
  () =>
    composeCreateBindings(
      [],
      [],
      [selectedConnections[0]],
      connectorCandidates.map(candidate =>
        candidate.resource_id === 'connection-1'
          ? { ...candidate, metadata: { allowed_operation_ids: [] } }
          : candidate
      )
    ),
  /offers no operations/,
  'a candidate that allows no operation must fail instead of binding nothing usable'
);
assert.throws(
  () =>
    composeCreateBindings(
      [],
      [],
      [
        selectedConnections[0],
        connectorSelection('calendar', 'connection-1', ['events.write'], 'connector:1'),
      ],
      connectorCandidates
    ),
  /selected twice with different operations/,
  'one connection selected with two different operation sets must fail instead of merging'
);

// The composition helper stays generic: no protocol-specific id, no template-id
// branch, and no raw credential or action key.
const createBindingsSource = readFileSync(
  new URL(
    '../src/components/digital-human/create/create-digital-human-bindings.ts',
    import.meta.url
  ),
  'utf8'
);
assert.ok(
  !/server-ssh|ssh/i.test(createBindingsSource),
  'the composition helper must not hardcode a protocol-specific connector id'
);
assert.ok(
  !createBindingsSource.includes('opsSpecialist') &&
    !createBindingsSource.includes('fileGeneration'),
  'the composition helper must not branch on a template id'
);
assert.doesNotMatch(
  createBindingsSource,
  /credential_ref|allowed_action_ids/,
  'the composition helper must never name a credential handle or a raw action key'
);

// ---------------------------------------------------------------------------
// Create-time Connector selection model
// ---------------------------------------------------------------------------

// Several connections, including two of the same connector, can be selected at
// once, and each one keeps the candidate snapshot it was chosen from.
let connectorPicks = addCreateConnectorSelection([], connectorCandidates[0]);
connectorPicks = addCreateConnectorSelection(connectorPicks, connectorCandidates[1]);
connectorPicks = addCreateConnectorSelection(connectorPicks, connectorCandidates[2]);
assert.deepEqual(
  connectorPicks.map(item => item.binding.resource_id),
  ['connection-1', 'connection-2', 'connection-3'],
  'the create picker must allow several connections, including two of one connector'
);
assert.deepEqual(
  createConnectorSelectionBindings(connectorPicks).map(item => item.resource_id),
  ['connection-1', 'connection-2', 'connection-3'],
  'the composed bindings must keep the selection order'
);
assert.deepEqual(
  createConnectorSelectionCandidates(connectorPicks).map(candidate => candidate.resource_id),
  ['connection-1', 'connection-2', 'connection-3'],
  'every binding must travel with its matching candidate snapshot'
);
assert.deepEqual(
  connectorOperationIds(connectorPicks[0].binding),
  ['events.read', 'events.write', 'calendars.read'],
  'a new selection must start from every operation the candidate allows'
);
assert.equal(
  addCreateConnectorSelection(connectorPicks, connectorCandidates[1]).length,
  connectorPicks.length,
  'selecting the exact same connection twice must not duplicate it'
);

// An SSH connection starts with its read actions only; the service write
// actions stay selectable but must be ticked on purpose.
const sshReadOperations = ['server.system.snapshot', 'server.service.status', 'server.journal.tail'];
const sshWriteOperations = [
  'server.service.restart',
  'server.service.start',
  'server.service.stop',
  'server.service.reload',
];
const sshCandidate = {
  ...connectorCandidates[0],
  resource_id: 'ssh-connection-1',
  parent_resource_id: 'serverops_ssh',
  metadata: { allowed_operation_ids: [...sshReadOperations, ...sshWriteOperations] },
};
const sshPicks = addCreateConnectorSelection([], sshCandidate);
assert.deepEqual(
  connectorOperationIds(sshPicks[0].binding),
  sshReadOperations,
  'a new SSH selection must preselect only its read actions'
);
assert.deepEqual(
  connectorOperationIds(sshPicks[0].candidate),
  [...sshReadOperations, ...sshWriteOperations],
  'the write actions must stay offered by the candidate snapshot'
);
const sshWithRestart = setCreateConnectorSelectionOperation(
  sshPicks,
  createConnectorCandidateKey(sshPicks[0].candidate),
  'server.service.restart',
  true
);
assert.deepEqual(
  connectorOperationIds(sshWithRestart[0].binding),
  [...sshReadOperations, 'server.service.restart'],
  'a write action must remain selectable after the default'
);
assert.deepEqual(
  createConnectorDefaultOperationIds({ metadata: { allowed_operation_ids: sshWriteOperations } }),
  sshWriteOperations,
  'a connection offering only write actions keeps them so the binding stays usable'
);

// Operation narrowing is a strict subset of the candidate's allowed set and can
// never become empty or wider.
const firstConnectionKey = createConnectorCandidateKey(connectorPicks[0].candidate);
let narrowedPicks = setCreateConnectorSelectionOperation(
  connectorPicks,
  firstConnectionKey,
  'events.write',
  false
);
assert.deepEqual(
  connectorOperationIds(narrowedPicks[0].binding),
  ['events.read', 'calendars.read'],
  'unchecking an operation must narrow that connection only'
);
assert.deepEqual(
  connectorOperationIds(narrowedPicks[1].binding),
  ['events.read', 'calendars.read'],
  'narrowing one connection must not touch another'
);
narrowedPicks = setCreateConnectorSelectionOperation(
  narrowedPicks,
  firstConnectionKey,
  'events.read',
  false
);
narrowedPicks = setCreateConnectorSelectionOperation(
  narrowedPicks,
  firstConnectionKey,
  'calendars.read',
  false
);
assert.deepEqual(
  connectorOperationIds(narrowedPicks[0].binding),
  ['calendars.read'],
  'the last selected operation must never be removable'
);
assert.deepEqual(
  connectorOperationIds(
    setCreateConnectorSelectionOperation(narrowedPicks, firstConnectionKey, 'messages.send', true)[0]
      .binding
  ),
  ['calendars.read'],
  'the picker must never add an operation outside the candidate allowed set'
);

// Toggling removes a selected connection without touching the rest.
assert.deepEqual(
  toggleCreateConnectorSelection(connectorPicks, connectorCandidates[1]).map(
    item => item.binding.resource_id
  ),
  ['connection-1', 'connection-3'],
  'toggling a selected connection must remove it'
);
assert.deepEqual(
  removeCreateConnectorSelection(connectorPicks, firstConnectionKey).map(
    item => item.binding.resource_id
  ),
  ['connection-2', 'connection-3'],
  'removing by identity must drop exactly the named connection'
);

// Only available native Connector candidates with at least one operation are
// selectable; anything else is refused instead of stored.
assert.equal(
  createConnectorCandidateSelectable({ ...connectorCandidates[0], available: false }),
  false,
  'an unavailable connection must not be selectable'
);
assert.equal(
  createConnectorCandidateSelectable({
    ...connectorCandidates[0],
    metadata: { allowed_operation_ids: [] },
  }),
  false,
  'a connection without operations must not be selectable'
);
assert.equal(
  createConnectorCandidateSelectable({ ...connectorCandidates[0], type: 'skill' }),
  false,
  'only native Connector candidates may enter the create selection'
);
assert.equal(
  addCreateConnectorSelection([], { ...connectorCandidates[0], available: false }).length,
  0,
  'an unavailable connection must be refused, not stored'
);

// The snapshot must keep the revision and allowed operations the user saw even
// if a later server response mutates the source object.
const mutableCandidate = {
  ...connectorCandidates[0],
  metadata: { ...connectorCandidates[0].metadata },
};
const snapshotPicks = addCreateConnectorSelection([], mutableCandidate);
mutableCandidate.resource_revision = 'connector:mutated';
mutableCandidate.metadata.allowed_operation_ids = ['events.write'];
assert.equal(
  snapshotPicks[0].binding.resource_revision,
  'connector:1',
  'the stored snapshot must keep the revision the user selected'
);
assert.deepEqual(
  connectorOperationIds(snapshotPicks[0].binding),
  ['events.read', 'events.write', 'calendars.read'],
  'the stored snapshot must keep the operations the user selected'
);

// In-place mutation of the fetched candidate must not reach the stored snapshot
// or its binding: the snapshot deep-copies the operation and access-mode arrays
// instead of sharing them with the server response.
const inPlaceCandidate = {
  ...connectorCandidates[0],
  access_modes: [...connectorCandidates[0].access_modes],
  metadata: {
    allowed_operation_ids: [...connectorCandidates[0].metadata.allowed_operation_ids],
  },
};
const inPlacePicks = addCreateConnectorSelection([], inPlaceCandidate);
inPlaceCandidate.metadata.allowed_operation_ids.push('events.delete');
inPlaceCandidate.metadata.allowed_operation_ids[0] = 'events.mutated';
inPlaceCandidate.access_modes.push('write');
assert.deepEqual(
  connectorOperationIds(inPlacePicks[0].candidate),
  ['events.read', 'events.write', 'calendars.read'],
  'in-place mutation of candidate.metadata.allowed_operation_ids must not change the stored snapshot'
);
assert.deepEqual(
  connectorOperationIds(inPlacePicks[0].binding),
  ['events.read', 'events.write', 'calendars.read'],
  'in-place mutation of the fetched candidate must not change the stored binding'
);
assert.deepEqual(
  inPlacePicks[0].candidate.access_modes,
  ['execute'],
  'the snapshot must deep-copy access_modes so later mutation cannot reach it'
);

// The snapshot is an explicit allowlist. Extra metadata (credential-like or
// otherwise) and extra top-level properties must never enter the snapshot or the
// composed output, even though the candidate is otherwise selectable.
const dirtyCreateCandidate = {
  type: 'connector',
  resource_id: 'connection-dirty',
  parent_resource_id: 'calendar',
  display_name: 'Calendar dirty',
  description: 'keep this allowlisted description',
  resource_revision: 'connector:dirty',
  access_modes: ['execute'],
  metadata: {
    allowed_operation_ids: ['events.read'],
    credential_ref: 'credential-handle-must-not-travel',
    password: 'super-secret-must-not-travel',
    nested: { access_token: 'token-must-not-travel' },
  },
  status: 'ready',
  available: true,
  secret_token: 'top-level-secret-must-not-travel',
  internal_note: 'must-not-travel',
};
const dirtyCreatePicks = addCreateConnectorSelection([], dirtyCreateCandidate);
const dirtyCreateSnapshot = dirtyCreatePicks[0].candidate;
assert.deepEqual(
  Object.keys(dirtyCreateSnapshot).sort(),
  [
    'access_modes',
    'available',
    'description',
    'display_name',
    'metadata',
    'parent_resource_id',
    'resource_id',
    'resource_revision',
    'status',
    'type',
  ],
  'the selected snapshot must carry exactly the allowlisted candidate fields'
);
assert.deepEqual(
  Object.keys(dirtyCreateSnapshot.metadata),
  ['allowed_operation_ids'],
  'candidate metadata must be reduced to allowed_operation_ids only'
);
assert.equal(
  dirtyCreateSnapshot.description,
  'keep this allowlisted description',
  'the allowlisted description must survive the snapshot'
);
assert.deepEqual(
  connectorOperationIds(dirtyCreateSnapshot),
  ['events.read'],
  'the snapshot must keep the validated allowed operations'
);
assert.doesNotMatch(
  JSON.stringify(dirtyCreatePicks),
  /credential|secret|password|token|ssh|must-not-travel/i,
  'no extra candidate metadata or top-level property may enter the selected snapshot'
);

const dirtyCreateOutput = createConnectorSelectionBindings(dirtyCreatePicks);
assert.deepEqual(
  Object.keys(dirtyCreateOutput[0]).sort(),
  [
    'access_mode',
    'display_name',
    'metadata',
    'parent_resource_id',
    'resource_id',
    'resource_revision',
    'type',
  ],
  'the selected binding must carry exactly the request fields it declares'
);
assert.deepEqual(
  Object.keys(dirtyCreateOutput[0].metadata),
  ['allowed_operation_ids'],
  'the selected binding metadata must stay an operation-id allowlist'
);
assert.doesNotMatch(
  JSON.stringify(dirtyCreateOutput),
  /credential|secret|password|token|ssh|must-not-travel/i,
  'no extra candidate field may reach the composed selection binding'
);

const dirtyCreateComposed = composeCreateBindings(
  [],
  [],
  dirtyCreateOutput,
  createConnectorSelectionCandidates(dirtyCreatePicks)
);
assert.equal(dirtyCreateComposed.length, 1, 'a dirty but selectable candidate must still compose');
assert.doesNotMatch(
  JSON.stringify(dirtyCreateComposed),
  /credential|secret|password|token|ssh|must-not-travel/i,
  'no extra candidate field may reach the composed create-time output'
);

// The picker selection composes into the single create-time replacement list
// together with the offered skill.
assert.deepEqual(
  composeCreateBindings(
    ['file-generator'],
    offered,
    createConnectorSelectionBindings(connectorPicks.slice(0, 2)),
    createConnectorSelectionCandidates(connectorPicks.slice(0, 2))
  ).map(item => `${item.type}:${item.parent_resource_id ?? ''}:${item.resource_id}`),
  ['skill::file-generator', 'connector:calendar:connection-1', 'connector:calendar:connection-2'],
  'the picker selection must compose skills and connections into one replacement list'
);

// ---------------------------------------------------------------------------
// UI source contracts
// ---------------------------------------------------------------------------

const readSource = relative => readFile(new URL(relative, import.meta.url), 'utf8');

const flowSource = await readSource(
  '../src/components/digital-human/create/digital-human-create-flow.tsx'
);
const formSource = await readSource(
  '../src/components/digital-human/create/digital-human-template-form.tsx'
);
const pickerSource = await readSource(
  '../src/components/digital-human/digital-human-skill-picker.tsx'
);
const gallerySource = await readSource(
  '../src/components/digital-human/create/digital-human-template-gallery.tsx'
);
const previewSource = await readSource(
  '../src/components/digital-human/create/digital-human-template-preview.tsx'
);
const successSource = await readSource(
  '../src/components/digital-human/create/digital-human-create-success-dialog.tsx'
);
const createPageSource = await readSource('../src/app/console/digital-humans/create/page.tsx');
const connectorCreatePickerSource = await readSource(
  '../src/components/digital-human/create/digital-human-connector-create-picker.tsx'
);
const connectorCreateModelSource = await readSource(
  '../src/components/digital-human/create/digital-human-connector-create-model.ts'
);

assert.match(
  createPageSource,
  /<Suspense fallback=\{null\}>/,
  'the static create route must read its query parameter behind a Suspense boundary'
);
assert.match(
  createPageSource,
  /<DigitalHumanFeatureGate permission="digital_human\.create">/,
  'the create page must require digital_human.create'
);

const integrationServiceSource = await readSource(
  '../src/services/digital-human.service.ts'
);
const createPayloadSource = await readSource(
  '../src/components/digital-human/create/create-digital-human-payload.ts'
);

// The create-time old-integrations coupling is gone from the service too: only
// the workspace skill candidates reader and the id-scoped draft editor's binding
// candidates reader may remain. The id-scoped reader belongs to the draft editor
// and is not part of this removal.
assert.doesNotMatch(
  integrationServiceSource,
  /digital-human-integration-candidates/,
  'the create-time integration candidates endpoint must be gone from the service'
);
assert.doesNotMatch(
  integrationServiceSource,
  /listIntegrationCandidates/,
  'the create-time integration candidates reader must be gone from the service'
);
assert.match(
  integrationServiceSource,
  /listSkillCandidates\(\): Promise<ApiResponseData<DigitalHumanResourceCandidateList>> \{\s*return this\.request\('get', '\/digital-human-skill-candidates'\);\s*\}/,
  'the workspace skill candidates reader must stay a parameterless GET'
);
assert.match(
  integrationServiceSource,
  /listBindingCandidates\(/,
  'the id-scoped draft editor binding candidates reader must stay available'
);

// Neither the create payload mapping nor the create request may hardcode a
// provider id or smuggle a capability claim in.
assert.ok(
  !createPayloadSource.includes('server-ssh'),
  'the create payload module must not hardcode the server-ssh integration id'
);

const queryKeysSource = await readSource('../src/hooks/query-keys.ts');
const digitalHumanHooksSource = await readSource(
  '../src/hooks/digital-human/use-digital-humans.ts'
);

// The workspace skill candidates reader stays the only pre-creation candidate
// list: a zero-argument key factory on the digital-human root, read through a
// parameterless hook that only takes an optional enabled flag.
const skillCandidatesKeySegment = queryKeysSource.match(
  /skillCandidates:\s*\(\)\s*=>\s*\[\.\.\.DIGITAL_HUMAN_KEYS\.all,\s*'([^']+)'\]/
)?.[1];
assert.equal(
  skillCandidatesKeySegment,
  'skill-candidates',
  'skillCandidates must stay a zero-argument key factory on the digital-human root'
);
assert.doesNotMatch(
  queryKeysSource,
  /integrationCandidates/,
  'the create-time integration candidates query key must be gone'
);

assert.match(
  digitalHumanHooksSource,
  /export function useDigitalHumanSkillCandidates\(enabled = true\)/,
  'the skill candidates hook may only take an optional enabled flag'
);
const skillCandidatesHookSource =
  digitalHumanHooksSource.match(
    /export function useDigitalHumanSkillCandidates[\s\S]*?\n\}/
  )?.[0] ?? '';
assert.ok(
  skillCandidatesHookSource.includes('useQuery('),
  'the skill candidates hook must read through useQuery'
);
assert.match(
  skillCandidatesHookSource,
  /queryKey:\s*DIGITAL_HUMAN_KEYS\.skillCandidates\(\)/,
  'the hook must use the dedicated skill candidates query key'
);
assert.match(
  skillCandidatesHookSource,
  /queryFn:\s*\(\)\s*=>\s*digitalHumanService\.listSkillCandidates\(\)/,
  'the hook must call the parameterless skill candidates service method'
);
assert.match(
  skillCandidatesHookSource,
  /enabled,/,
  'the hook must pass enabled through unchanged'
);
assert.match(
  skillCandidatesHookSource,
  /staleTime:\s*30_000/,
  'the hook must cache skill candidates for 30 seconds'
);
assert.doesNotMatch(
  digitalHumanHooksSource,
  /useDigitalHumanIntegrationCandidates/,
  'the create-time integration candidates hook must be gone'
);

// The create payload carries no capability choice at all: bindings happen only
// through the bindings API, exactly once.
const createPayloadCallSource =
  flowSource.match(/createDigitalHumanPayload\(\{[\s\S]*?\n\s*\}\)/)?.[0] ?? '';
assert.ok(createPayloadCallSource.length > 0, 'the create payload call must be present');
assert.doesNotMatch(
  createPayloadCallSource,
  /connection/i,
  'the create payload must not carry a connection choice'
);
assert.equal(
  (flowSource.match(/replaceDraftBindings/g) ?? []).length,
  1,
  'the selected skills must go through exactly one bindings replacement'
);

// ---------------------------------------------------------------------------
// One binding replacement for the selected capabilities and resources
// ---------------------------------------------------------------------------

// The flow composes skills and Connector selections with the approved helper
// and never fabricates a binding for a capability the workspace does not offer.
assert.match(
  flowSource,
  /composeCreateBindings\(\s*selectedSkillIds,\s*skillCandidates,\s*createConnectorSelectionBindings\(connectorSelections\),\s*createConnectorSelectionCandidates\(connectorSelections\)\s*\)/,
  'create bindings must merge the skill selection and the Connector selection snapshots'
);
assert.doesNotMatch(
  flowSource,
  /selectIntegrationConnectionBinding|selectableIntegrationConnectionCandidates|integrationCandidateAllowedActionIds|connectionSelection|connectionFields|DigitalHumanConnectionField|DigitalHumanConnectionSelector/,
  'the flow must not keep the removed connection selection or binding helpers'
);
assert.doesNotMatch(
  flowSource,
  /allowed_action_ids|credential_ref/,
  'the flow must never read raw action metadata or credential keys'
);

// The single replacement call receives the full list.
const bindCapabilitiesSource =
  flowSource.match(/const bindSelectedCapabilities = async[\s\S]*?\n {2}\};/)?.[0] ?? '';
assert.ok(
  bindCapabilitiesSource.length > 0,
  'the flow must expose one clearly named capability binding helper'
);
assert.equal(
  (bindCapabilitiesSource.match(/replaceDraftBindings\(/g) ?? []).length,
  1,
  'the helper must call the bindings API exactly once'
);
assert.match(
  bindCapabilitiesSource,
  /replaceDraftBindings\(id, lockVersion, items\)/,
  'the replacement must receive the complete items list'
);
const emptyReturnIndex = bindCapabilitiesSource.indexOf('if (items.length === 0) return [];');
const replaceIndex = bindCapabilitiesSource.indexOf('replaceDraftBindings(');
assert.ok(
  emptyReturnIndex > 0 && replaceIndex > emptyReturnIndex,
  'an empty selection must skip the bindings API call entirely'
);
assert.match(
  bindCapabilitiesSource,
  /bindingDisplayNames\(bound\.data\?\.items \?\? items\)/,
  'the reported names must fall back to the full items list'
);

// The created state and success dialog speak about capabilities, not skills.
assert.doesNotMatch(
  flowSource,
  /boundSkillNames/,
  'the flow must not keep a skill-only bound-name field'
);
assert.match(flowSource, /boundCapabilityNames/, 'the flow must track bound capability names');
assert.doesNotMatch(
  successSource,
  /boundSkillNames/,
  'the dialog must not keep a skill-only prop'
);
assert.match(
  successSource,
  /boundCapabilityNames/,
  'the dialog must receive the bound capability names'
);

// Generic copy: the shared wording is about the selected capabilities and
// resources, while a zero-binding creation must never be reported as done.
for (const [locale, dictionary] of [
  ['en-US', enUS],
  ['zh-Hans', zhHans],
]) {
  const formCopy = dictionary.createFlow.form;
  const successCopy = dictionary.createFlow.success;
  assert.ok(
    formCopy.bindingFailed.includes('capabilities or resources') ||
      formCopy.bindingFailed.includes('能力或资源'),
    `${locale} bindingFailed must speak about capabilities or resources`
  );
  assert.ok(
    formCopy.publishFailed.includes('capabilities/resources') ||
      formCopy.publishFailed.includes('能力/资源'),
    `${locale} publishFailed must speak about capabilities/resources`
  );
  assert.equal(
    typeof successCopy.boundCapabilities,
    'string',
    `${locale} must expose the generic boundCapabilities label`
  );
  assert.ok(
    !Object.prototype.hasOwnProperty.call(successCopy, 'boundSkills'),
    `${locale} must not keep the skill-only boundSkills label`
  );
  // A creation that bound nothing must not be described as having its
  // capabilities and resources in place: the really-bound list is rendered
  // separately and only when it is not empty.
  assert.doesNotMatch(
    successCopy.written,
    /capabilit|resources|connection|能力|资源|连接/i,
    `${locale} success.written must not claim capabilities, resources, or connections are in place`
  );
  assert.doesNotMatch(
    successCopy.completedItems,
    /capabilit|resources|connection|能力|资源|连接/i,
    `${locale} completedItems must not report capabilities or resources as done`
  );
}

// The create request still never carries a connection or a credential.
assert.doesNotMatch(
  createPayloadCallSource,
  /selection|candidate|connection|credential/i,
  'the create payload must stay free of connections and credentials'
);

// Neither generic surface hardcodes a provider id or a sensitive metadata key.
for (const [name, source] of [
  ['create flow', flowSource],
  ['template form', formSource],
  ['success dialog', successSource],
  ['template gallery', gallerySource],
  ['template preview', previewSource],
]) {
  for (const forbiddenToken of ['server-ssh', 'credential_ref', 'allowed_action_ids']) {
    assert.ok(!source.includes(forbiddenToken), `${name} must not mention ${forbiddenToken}`);
  }
}

// ---------------------------------------------------------------------------
// Generic create-time Connector picker contracts
// ---------------------------------------------------------------------------

// The picker lists the owner's saved connections through the reviewed
// pre-creation reader. The shared ConnectorPicker owns filtering and the host
// keeps a full 512-candidate eligibility snapshot for safe create-time binding.
assert.match(
  connectorCreatePickerSource,
  /useDigitalHumanConnectorCandidates\(\{\s*limit:\s*512\s*\}\)/,
  'the picker must load the full pre-creation candidate page'
);
// Alias, status, connection creation, and operation scoping come from the
// shared ConnectorPicker/Setup surfaces.
assert.match(
  connectorCreatePickerSource,
  /<ConnectorPickerDialog[\s\S]*selectionMode="multiple-connections"/,
  'the create picker must reuse the shared multiple-connection adapter'
);
assert.match(
  connectorCreatePickerSource,
  /onRequestConnectAndAdd=/,
  'the create picker must open setup from the usage surface'
);
assert.match(
  connectorCreatePickerSource,
  /<ConnectorKeySetupPanel[\s\S]*embedded/,
  'the create picker must reuse the shared credential setup panel without a draft target'
);
const setupTagStart = connectorCreatePickerSource.indexOf('<ConnectorKeySetupPanel');
const setupTagEnd = connectorCreatePickerSource.indexOf('/>', setupTagStart);
const setupTag = connectorCreatePickerSource.slice(setupTagStart, setupTagEnd + 2);
assert.doesNotMatch(
  setupTag,
  /target=/,
  'pre-creation setup must not attach a credential to a non-existent draft target'
);
assert.match(
  connectorCreatePickerSource,
  /<DigitalHumanConnectorOperationDetails[\s\S]*onOperation=/,
  'the create picker must scope operations on selected connections'
);
assert.doesNotMatch(
  connectorCreatePickerSource,
  /\/console\/connectors\/connections/,
  'the template picker must not expose a redundant Connector Center link'
);

// The create page may open the shared credential/reconnect surfaces directly
// from the usage context, while the template form itself remains generic.
assert.match(
  connectorCreatePickerSource,
  /ConnectionReconnectDialog|ConnectionAppDialog|supportsAppSetup/,
  'the create Connector picker must reuse shared setup and maintenance controls'
);
for (const [name, source] of [
  ['create Connector picker', connectorCreatePickerSource],
  ['create Connector model', connectorCreateModelSource],
]) {
  for (const forbiddenToken of [
    'server-ssh',
    'credential_ref',
    'allowed_action_ids',
    'opsSpecialist',
    'fileGeneration',
  ]) {
    assert.ok(!source.includes(forbiddenToken), `${name} must not mention ${forbiddenToken}`);
  }
}
// The model copies only validated operation ids and never a raw credential key.
assert.doesNotMatch(
  connectorCreateModelSource,
  /credential_ref|password|api_key|allowed_action_ids|server-ssh/,
  'the create Connector model must stay credential-free and protocol-agnostic'
);

// The generic create form renders the section for every template and the
// custom entry, and not inside a template branch.
assert.match(
  formSource,
  /<DigitalHumanConnectorCreateSection[\s\S]*?selections=\{connectorSelections\}[\s\S]*?onChangeSelections=\{onChangeConnectorSelections\}/,
  'the generic create form must render the Connector section for every template and custom entry'
);
assert.doesNotMatch(
  formSource,
  /template\.id\s*===|isCustomTemplateId/,
  'the form must not branch the Connector section on a template id'
);
assert.doesNotMatch(
  flowSource,
  /useDigitalHumanConnectorCandidates/,
  'the flow must not fetch Connector candidates itself; the shared picker owns that'
);

// ---------------------------------------------------------------------------
// No create-time old-integrations coupling remains anywhere in the feature
// ---------------------------------------------------------------------------

// The draft editor's id-scoped integration binding is a different surface and is
// intentionally not part of this negative contract.
const templatesSource = await readSource(
  '../src/components/digital-human/create/digital-human-templates.ts'
);
const createFeatureSources = [
  ['create flow', flowSource],
  ['template form', formSource],
  ['template gallery', gallerySource],
  ['template preview', previewSource],
  ['success dialog', successSource],
  ['template catalogue', templatesSource],
  ['create payload', createPayloadSource],
  ['create bindings', createBindingsSource],
  ['create connector model', connectorCreateModelSource],
  ['create connector picker', connectorCreatePickerSource],
  ['digital human service', integrationServiceSource],
  ['query keys', queryKeysSource],
  ['digital human hooks', digitalHumanHooksSource],
];
for (const [name, source] of createFeatureSources) {
  for (const removedToken of [
    'digital-human-integration-candidates',
    'listIntegrationCandidates',
    'useDigitalHumanIntegrationCandidates',
    'integrationCandidates',
    'DigitalHumanConnectionSelector',
    'DigitalHumanConnectionField',
    'connectionFields',
    'connectionSelection',
    'template.connections',
    'connections.length',
    'server-ssh',
  ]) {
    assert.ok(
      !source.includes(removedToken),
      `${name} must not keep the removed create-time connection coupling (${removedToken})`
    );
  }
}

// The removed connection selector component must not come back.
const createDirEntries = await readdir(
  new URL('../src/components/digital-human/create/', import.meta.url)
);
assert.ok(
  !createDirEntries.includes('digital-human-connection-selector.tsx'),
  'the create page must not ship a connection selector component'
);

// The removed connection copy must be gone from both locales.
for (const [locale, dictionary] of [
  ['en-US', enUS],
  ['zh-Hans', zhHans],
]) {
  const form = dictionary.createFlow.form;
  assert.deepEqual(
    Object.keys(form).filter(key => key.startsWith('connection')),
    [],
    `${locale} must not keep the removed connection form copy`
  );
  assert.equal(
    dictionary.createFlow.templates,
    undefined,
    `${locale} must not keep template copy, connection labels included (ADR 0022)`
  );
}

// The digital-human search skill was test-only functionality; connectors own search now.
const REMOVED_SEARCH_TOKENS = ['digital-human-exa-web-search', 'ExaWebSearch', 'exaWebSearch'];
async function collectSources(dir) {
  const entries = await readdir(dir, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    if (entry.isDirectory()) {
      files.push(...(await collectSources(new URL(`${entry.name}/`, dir))));
    } else if (/\.(?:ts|tsx)$/.test(entry.name)) {
      files.push(new URL(entry.name, dir));
    }
  }
  return files;
}
for (const dir of [
  new URL('../src/components/digital-human/', import.meta.url),
  new URL('../src/hooks/digital-human/', import.meta.url),
  new URL('../src/i18n/modules/digitalHumans/', import.meta.url),
]) {
  for (const file of await collectSources(dir)) {
    const source = await readFile(file, 'utf8');
    for (const token of REMOVED_SEARCH_TOKENS) {
      assert.ok(
        !source.includes(token),
        `${file.pathname} must not reference the removed digital-human search skill (${token})`
      );
    }
  }
}
for (const relative of [
  '../src/services/digital-human.service.ts',
  '../src/services/types/digital-human.ts',
]) {
  const source = await readSource(relative);
  for (const token of REMOVED_SEARCH_TOKENS) {
    assert.ok(!source.includes(token), `${relative} must not reference ${token}`);
  }
}

assert.match(flowSource, /useAvailableModels\(\{ use_case: 'agent-runtime' \}\)/);
assert.match(flowSource, /invalidTemplate/, 'an unknown template parameter must fall back');

// Single submit, and never a second digital human for the same form.
assert.match(flowSource, /if \(submitting\) return;/, 'submit must ignore re-entrant clicks');
assert.match(
  flowSource,
  /if \(created\) \{[\s\S]*router\.push\(`\$\{LIST_PATH\}\/\$\{created\.id\}\/configuration`\)[\s\S]*return;/,
  'a form that already produced a digital human must never create a second one'
);
assert.equal(
  (flowSource.match(/setValue\(initialValue\)/g) ?? []).length,
  1,
  'only the discard path may reset the form, so a failed create keeps every field'
);
// The selection is validated before creation, and its failure surfaces a
// visible error without creating anything.
const composeGuardStart = flowSource.indexOf('items = composeSelectedBindings();');
const composeCatchEnd = flowSource.indexOf('mutations.create.mutateAsync', composeGuardStart);
assert.ok(
  composeGuardStart > 0 && composeCatchEnd > composeGuardStart,
  'the flow must compose the selection before the create request'
);
const composeCatchBody = flowSource.slice(
  flowSource.indexOf('} catch {', composeGuardStart),
  composeCatchEnd
);
assert.match(
  composeCatchBody,
  /setSubmitError\(t\('createFlow\.form\.invalidSelection'\)\)/,
  'an invalid selection must block creation with a visible error'
);
assert.doesNotMatch(
  composeCatchBody,
  /mutations\.create/,
  'an invalid selection must not create a digital human'
);

const catchEnd = flowSource.indexOf('} finally {');
const catchStart = flowSource.lastIndexOf('} catch {', catchEnd);
assert.ok(catchStart > 0 && catchEnd > catchStart, 'the create call must keep a catch/finally pair');
assert.ok(
  flowSource.slice(catchStart, catchEnd).includes('setSubmitError') &&
    !flowSource.slice(catchStart, catchEnd).includes('setValue('),
  'the failure path must keep the user input and only surface the error'
);

// Creation order inside submit: create -> bind selected capabilities -> publish.
const submitStart = flowSource.indexOf('const submit = async () => {');
const submitEnd = flowSource.indexOf('const draftBase =', submitStart);
assert.ok(submitStart > 0 && submitEnd > submitStart, 'the flow must keep a submit handler');
const submitBody = flowSource.slice(submitStart, submitEnd);
const createCallIndex = submitBody.indexOf('mutations.create.mutateAsync');
const bindCallIndex = submitBody.indexOf('bindSelectedCapabilities(');
const publishCallIndex = submitBody.indexOf('.publish(');
const composeCallIndex = submitBody.indexOf('composeSelectedBindings()');
assert.ok(
  composeCallIndex > 0 && composeCallIndex < createCallIndex,
  'the flow must validate the composed selection before the create request'
);
assert.ok(
  createCallIndex > 0 && bindCallIndex > createCallIndex && publishCallIndex > bindCallIndex,
  'the flow must create, then bind the selected capabilities, then publish'
);
assert.match(
  submitBody,
  /if \(!notice\)\s*\{[\s\S]*?\.publish\(/,
  'publishing must be skipped when binding failed, leaving the created draft'
);
assert.match(
  flowSource,
  /createFlow\.form\.invalidSelection/,
  'an invalid selection must surface a visible error before creating'
);
assert.ok(
  submitBody.indexOf('digitalHumanService.get(') === -1 ||
    submitBody.indexOf('digitalHumanService.get(') < publishCallIndex,
  'the fresh read must stay before publishing'
);
assert.match(flowSource, /replaceDraftBindings/, 'binding must go through the bindings API');
assert.match(
  flowSource,
  /composeCreateBindings\(/,
  'bindings must be resolved from the real candidates, the skill selection, and the Connector snapshots'
);
assert.match(
  flowSource,
  /createConnectorSelectionBindings\(connectorSelections\)[\s\S]*createConnectorSelectionCandidates\(connectorSelections\)/,
  'the flow must compose the selected Connector bindings with their candidate snapshots'
);
assert.match(
  flowSource,
  /useDigitalHumanSkillCandidates\(\)/,
  'the creation flow must offer the workspace skills the server reports as bindable'
);
assert.match(
  flowSource,
  /templateDefaultSkillIds\(template, skillCandidates\)/,
  'a template may only seed the selection with skills the workspace offers'
);
assert.match(
  flowSource,
  /createFlow\.form\.bindingFailed/,
  'a failed binding must be reported instead of silently dropped'
);
assert.match(
  flowSource,
  /createFlow\.form\.publishFailed/,
  'a failed publish must be reported instead of silently dropped'
);
assert.doesNotMatch(
  flowSource,
  /status:\s*'active'/,
  'the flow must publish through the publish API rather than writing status directly'
);

// Field level validation and copy that matches auto-publish.
assert.match(formSource, /nameRef\.current\?\.focus\(\)/);
assert.match(formSource, /responsibilityRef\.current\?\.focus\(\)/);
for (const field of ['name', 'responsibility']) {
  assert.match(formSource, new RegExp(`aria-invalid=\\{${field}Error\\}`));
  assert.match(formSource, new RegExp(`aria-describedby=\\{${field}Error \\?`));
}
assert.match(
  formSource,
  /const modelBlocked = modelsLoading \|\| noModelAvailable \|\| Boolean\(modelsError\)/,
  'the form must block submit while models are loading, missing, or failed'
);
assert.match(formSource, /disabled=\{submitting \|\| modelBlocked\}/);
assert.match(formSource, /aria-busy=\{submitting\}/);
assert.match(
  formSource,
  /createFlow\.form\.skillsHint/,
  'the skill area must explain that binding and publishing happen on creation'
);
// The create form and the draft editor must render the same skill picker: the
// configuration page's bound card for what is chosen, and the same dialog to add.
assert.match(
  formSource,
  /<DigitalHumanSkillBindingCard[\s\S]*onRemove=\{\(\) => removeSkill\(candidate\.resource_id\)\}/,
  'the create form must review selected skills with the shared bound-skill card'
);
assert.match(
  formSource,
  /<DigitalHumanSkillPickerDialog[\s\S]*selectedIds=\{pendingSkillIds\}[\s\S]*onSave=\{saveSkillSelection\}/,
  'the create form must add skills through the shared picker dialog'
);
assert.match(
  formSource,
  /bindings\.sections\.skills\.add[\s\S]*openSkillPicker/,
  'the section header must offer the same add action as the configuration page'
);
assert.match(
  formSource,
  /bindings\.selectedCount/,
  'the section header must show the same selected count as the configuration page'
);
assert.match(
  formSource,
  /createFlow\.form\.skillsHint/,
  'the skill area must explain that binding and publishing happen on creation'
);
assert.match(
  formSource,
  /createFlow\.form\.skillsOptional/,
  'creating without skills must stay allowed'
);
assert.match(
  pickerSource,
  /aria-pressed=\{isSelected\}[\s\S]*onClick=\{\(\) => onToggle\(candidate\)\}/,
  'the shared skill card must be a toggle'
);
assert.match(
  pickerSource,
  /getAIChatSkillDisplayInfo/,
  'the shared skill card must reuse the workspace skill display metadata'
);
assert.match(
  pickerSource,
  /<ResourceBindingCard[\s\S]*<Trash2/,
  'the shared bound-skill card must reuse the configuration card with a remove action'
);
assert.match(
  pickerSource,
  /<ResourceSelectionDialog[\s\S]*title=\{t\('bindings\.sections\.skills\.pickerTitle'\)\}[\s\S]*bindings\.skills\.save/,
  'the shared picker dialog must reuse the configuration dialog shell and copy'
);
assert.doesNotMatch(
  formSource,
  /roleCapabilities|capabilityStatus/,
  'the form must not render the removed placeholder capability list'
);

// A template that seeds no skill uses the same generic form as every other
// template. There is no template-specific capability warning or extra gate.
assert.doesNotMatch(
  flowSource,
  /templateWithoutSkills|hasSelectedCapability|showTemplateNotEnabled|templateNotEnabled/,
  'the flow must not render the removed template capability warning'
);
assert.match(
  formSource,
  /notice\?: string \| null;/,
  'the form must retain the generic result notice for create errors or drafts'
);
assert.match(
  formSource,
  /notice \? \([\s\S]*role="status"[\s\S]*\{notice\}/,
  'the form must render the generic result notice as a status message'
);

// Gallery: catalogue driven, keyboard reachable, and it never claims a skill is bound yet.
assert.match(
  gallerySource,
  /templates: readonly DigitalHumanTemplate\[\];/,
  'the gallery must render the catalog it is given, not a client-side list'
);
assert.match(
  gallerySource,
  /template\.source === 'builtin'[\s\S]*template\.source === 'workspace'/,
  'built-in templates must come before the workspace saved ones (ADR 0022)'
);
assert.match(gallerySource, /section\.items\.map\(template =>/);
assert.match(
  gallerySource,
  /createFlow\.gallery\.loadFailed[\s\S]*onClick=\{onRetry\}[\s\S]*createFlow\.gallery\.retry/,
  'a catalog that fails to load must offer a retry'
);

// Flow: the catalog comes from the API, and a template whose requirements are
// not met cannot create a digital human.
assert.match(flowSource, /useDigitalHumanTemplates\(\{ locale \}\)/);
assert.match(
  flowSource,
  /sharedRules = catalog\?\.shared_rules \?\? \[\]/,
  'the custom entry takes its shared rules from the API catalog'
);
assert.match(flowSource, /name: template \? templateStartingName\(template\) : ''/);
assert.match(
  flowSource,
  /templateRequirementGap\(\s*template,\s*skillCandidates,\s*selectedSkillIds,\s*connectorSelections\.length\s*\)/,
  'requirements must be checked against the real candidates and the current selection'
);
assert.match(
  flowSource,
  /if \(!notice && requirementGap\) \{[\s\S]*?configuration\?setup=1/,
  'with a template requirement missing, creation stops at a draft and opens the setup checklist (ADR 0023)'
);
assert.doesNotMatch(
  flowSource,
  /if \(requirementNotice\) \{\s*setSubmitError\(requirementNotice\);/,
  'a missing requirement no longer refuses creation; publishing waits instead'
);
assert.match(gallerySource, /sm:group-focus-within:opacity-100/);
assert.match(
  gallerySource,
  /<button[\s\S]*aria-label=\{t\('createFlow\.gallery\.createWithRole'/,
  'the card body must be a labelled button so Enter/Space can start creation'
);
assert.match(gallerySource, /onClick=\{\(\) => onSelectTemplate\(CUSTOM_TEMPLATE_ID\)\}/);
// The card actions take over the tag row instead of reserving their own space,
// so every card keeps one constant height.
assert.match(
  gallerySource,
  /relative mt-3 h-8[\s\S]*DigitalHumanTagList[\s\S]*absolute inset-0[\s\S]*createFlow\.gallery\.details/,
  'the card must swap the tags and the actions inside one fixed-height slot'
);
assert.match(
  gallerySource,
  /createFlow\.form\.skillsPending/,
  'the gallery must say skills are resolved against the workspace on creation'
);
assert.doesNotMatch(
  gallerySource,
  /requiresSetup/,
  'the gallery must not claim a placeholder capability status'
);

// Template detail: the skills section renders only from a template's own facts,
// in declaration order, and only when the template really declares skills.
assert.match(
  previewSource,
  /export function DigitalHumanCapabilityList\(/,
  'the shared read-only list must be generic, not skill-named'
);
assert.match(
  previewSource,
  /DigitalHumanCapabilityList\(\s*\{\s*items\s*\}/,
  'the generic list must take a display-ready items prop'
);
// Display strings may repeat, so rows must never be keyed by the string alone.
assert.ok(
  !previewSource.includes('key={item}'),
  'the generic list must not key rows by a display string that may repeat'
);
assert.ok(
  previewSource.includes('key={`${index}:${item}`}'),
  'the generic list must key rows by index and item together'
);
assert.ok(
  previewSource.includes('items.map((item, index) =>'),
  'the generic list must map with an index to build a stable unique key'
);
for (const [name, source] of [
  ['template preview', previewSource],
  ['template gallery', gallerySource],
  ['success dialog', successSource],
]) {
  assert.ok(
    !source.includes('DigitalHumanSkillList'),
    `${name} must not keep the misleading skill-only list name`
  );
}
assert.match(
  gallerySource,
  /DigitalHumanCapabilityList items=\{templateSkills\(detailTemplate\)\}/,
  'the skills section must render skill labels through the generic list'
);
assert.match(
  successSource,
  /<DigitalHumanCapabilityList items=\{boundCapabilityNames\}/,
  'the dialog must pass the bound capability names to the generic list'
);
assert.match(
  gallerySource,
  /template\.skills\.map\(skill => \{[\s\S]*skillNames\.get\(skill\.id\) \|\| skill\.id[\s\S]*skill\.required \? t\('createFlow\.detail\.requiredSkill'/,
  'skill labels must resolve from the workspace candidates by id and mark required skills'
);
assert.match(
  gallerySource,
  /detailTemplate\.memory_instructions \?[\s\S]*createFlow\.detail\.memory/,
  'the detail drawer must show the template memory instructions when it has them'
);
assert.match(
  gallerySource,
  /detailTemplate\.connectors\.some\(connector => connector\.required\) \?[\s\S]*createFlow\.detail\.connector[\s\S]*createFlow\.form\.connectorRequired/,
  'the detail drawer must say a connector template needs a verified connection'
);
assert.match(
  gallerySource,
  /detailTemplate\.skills\.length > 0/,
  'the skills section must render only when the template declares skills'
);
assert.doesNotMatch(
  gallerySource,
  /template\.connections|templateConnections|createFlow\.detail\.connections/,
  'the detail drawer must not keep a connection section'
);
assert.doesNotMatch(
  gallerySource,
  /\.sort\s*\(/,
  'the detail sections must keep the template declaration order'
);

// A template is plain data: its type declares skills by id and a connector
// need as a flag, never a connection list or an i18n key.
const templateTypesSource = await readSource('../src/services/types/digital-human.ts');
const templateTypeBlock =
  templateTypesSource.match(/export interface DigitalHumanTemplate \{[\s\S]*?\n\}/)?.[0] ?? '';
assert.ok(templateTypeBlock, 'the template type must live with the service types');
assert.match(templateTypeBlock, /connectors: DigitalHumanTemplateConnector\[\];/);
assert.match(templateTypeBlock, /memory_instructions: string;/);
assert.doesNotMatch(
  templateTypeBlock,
  /connections|Key\b|Keys\b/,
  'the template type must not carry connections or i18n keys'
);

// The detail copy for the removed connection section must be gone from both
// locales, while the skills copy the drawer still uses must stay.
for (const [locale, dictionary] of [
  ['en-US', enUS],
  ['zh-Hans', zhHans],
]) {
  const detail = dictionary.createFlow.detail;
  assert.ok(
    !Object.prototype.hasOwnProperty.call(detail, 'connections') &&
      !Object.prototype.hasOwnProperty.call(detail, 'connectionsPending'),
    `${locale} must not keep the removed connection detail copy`
  );
  assert.equal(typeof detail.skills, 'string', `${locale} detail.skills must stay`);
  assert.ok(detail.skills.trim().length > 0, `${locale} detail.skills must not be empty`);
}

// Preview and dialog report a published digital human.
assert.match(previewSource, /createFlow\.preview\.published/);
assert.doesNotMatch(previewSource, /createFlow\.preview\.draft/);
assert.match(
  successSource,
  /createFlow\.success\.boundCapabilities/,
  'the dialog must label the bound capabilities and resources generically'
);
assert.doesNotMatch(
  successSource,
  /createFlow\.success\.boundSkills/,
  'the dialog must not fall back to the skill-only label'
);
assert.match(
  successSource,
  /boundCapabilityNames\.length > 0/,
  'the dialog must list the capabilities and resources that were really bound'
);
assert.doesNotMatch(
  successSource,
  /connection/i,
  'the dialog must not claim a connection was bound or is live'
);
assert.doesNotMatch(
  successSource,
  /DigitalHumanConnectionField|DigitalHumanConnectionSelector/,
  'the dialog must not render a connection selector'
);

// Reduced motion: every animated surface must opt out of its transition.
for (const [name, source] of [
  ['template gallery', gallerySource],
  ['template form', formSource],
  ['success dialog', successSource],
]) {
  const transitions = source.match(/transition-(?:colors|opacity|transform|all)\b/g) ?? [];
  const reduced = source.match(/motion-reduce:transition-none/g) ?? [];
  assert.equal(
    reduced.length,
    transitions.length,
    `${name} must disable every transition under prefers-reduced-motion`
  );
}

// The template must not surface runtime-location or connection state.
for (const [name, source] of [
  ['template form', formSource],
  ['template gallery', gallerySource],
  ['template preview', previewSource],
]) {
  assert.doesNotMatch(
    source,
    /createFlow\.[a-zA-Z.]*(local|device|online|connected)/i,
    `${name} must not surface runtime-location or connection state`
  );
}

// Copy guard: no locale may claim a capability is already connected or running.
// The operations copy says a connection is only usable once it is verified and
// bound, so a bare /connected/ match would flag that condition; the English
// patterns therefore target positive claims only.
const forbiddenClaims = [
  /已连接/,
  /连接成功/,
  /在线/,
  /运行中/,
  /已部署/,
  /部署成功/,
  /已启用/,
  /\b(?:already|now|successfully) connected\b/i,
  /\bserver is connected to\b/i,
  /connection is (?:live|active|ready|healthy|established)/i,
  /\bonline\b/i,
  /deployed successfully/i,
];
for (const [locale, dictionary] of [
  ['en-US', enUS],
  ['zh-Hans', zhHans],
]) {
  const copy = [];
  const collect = value => {
    if (typeof value === 'string') {
      copy.push(value);
      return;
    }
    if (value && typeof value === 'object') Object.values(value).forEach(collect);
  };
  collect(dictionary.createFlow);
  const joined = copy.join('\n');
  for (const pattern of forbiddenClaims) {
    assert.doesNotMatch(
      joined,
      pattern,
      `${locale} must not claim a capability is already connected, online, or deployed`
    );
  }
}

{
  // ADR 0026: only an SSH Server Operations connection offers read_only_auto,
  // and always asking is stored as no policy at all.
  const ssh = {
    type: 'connector',
    resource_id: 'connection-1',
    parent_resource_id: 'serverops_ssh',
    access_mode: 'execute',
    metadata: { allowed_operation_ids: ['server.command.run'] },
  };
  const github = { ...ssh, resource_id: 'connection-2', parent_resource_id: 'github' };
  assert.equal(connectorSupportsReadOnlyAuto(ssh), true);
  assert.equal(connectorSupportsReadOnlyAuto(github), false);
  assert.equal(
    connectorSupportsReadOnlyAuto({ ...ssh, metadata: { credential_mode: 'per_user' } }),
    false
  );
  const [auto, untouched] = updateDigitalHumanApprovalPolicy([ssh, github], ssh, 'read_only_auto');
  assert.equal(connectorApprovalPolicy(auto), 'read_only_auto');
  assert.deepEqual(auto.metadata.allowed_operation_ids, ['server.command.run']);
  assert.equal(untouched, github);
  const [asked] = updateDigitalHumanApprovalPolicy([auto], auto, 'always_ask');
  assert.equal('approval_policy' in asked.metadata, false);
  assert.equal(connectorApprovalPolicy(asked), 'always_ask');
  assert.deepEqual(updateDigitalHumanApprovalPolicy([github], github, 'read_only_auto'), [github]);
}

{
  // ADR 0026: the create flow keeps an SSH connection's read_only_auto choice
  // and drops it for any other connector.
  const sshCandidate = {
    type: 'connector',
    resource_id: 'ssh-1',
    parent_resource_id: 'serverops_ssh',
    display_name: 'Server',
    resource_revision: 'connector:9',
    access_modes: ['execute'],
    metadata: { allowed_operation_ids: ['server.command.run'] },
    status: 'ready',
    available: true,
  };
  const sshSelection = {
    ...connectorSelection('serverops_ssh', 'ssh-1', ['server.command.run'], 'connector:9'),
    metadata: { allowed_operation_ids: ['server.command.run'], approval_policy: 'read_only_auto' },
  };
  const mailSelection = {
    ...connectorSelection('mail', 'connection-3', ['messages.send'], 'connector:3'),
    metadata: { allowed_operation_ids: ['messages.send'], approval_policy: 'read_only_auto' },
  };
  const [ssh, mail] = composeCreateBindings(
    [],
    [],
    [sshSelection, mailSelection],
    [sshCandidate, ...connectorCandidates]
  );
  assert.equal(ssh.metadata.approval_policy, 'read_only_auto');
  assert.equal('approval_policy' in mail.metadata, false);
  const [plain] = composeCreateBindings(
    [],
    [],
    [connectorSelection('serverops_ssh', 'ssh-1', ['server.command.run'], 'connector:9')],
    [sshCandidate]
  );
  assert.deepEqual(Object.keys(plain.metadata), ['allowed_operation_ids']);
}

console.log('digital human create-flow contract tests passed');
