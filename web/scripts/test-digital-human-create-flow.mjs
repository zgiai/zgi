/**
 * Contract tests for the digital human template creation flow.
 *
 * These run without rendering anything: they prove what a template selection
 * creates, which real skills it declares and resolves from backend candidates,
 * that every template key resolves in both locales, and that the UI layer keeps
 * its guarantees (single submit, auto publish, no fake capability claims).
 *
 * Creation no longer reads, selects, or binds the old integration connections:
 * the operations template declares no skill and binds nothing by default. Phase
 * one reaches servers through the DigitalHuman-only serverops_ssh connector the
 * user verifies in Connector Center and binds in the generic create form, so the
 * copy describes only approved per-invocation read-only actions.
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
import {
  CUSTOM_TEMPLATE_ID,
  DIGITAL_HUMAN_TEMPLATES,
  SHARED_RULE_KEYS,
  findDigitalHumanTemplate,
  isCustomTemplateId,
} from '../src/components/digital-human/create/digital-human-templates.ts';
import {
  bindingDisplayNames,
  buildPersona,
  createDigitalHumanPayload,
  selectCandidateBindings,
  selectTemplateBindings,
  serializeDigitalHumanIcon,
  templateDefaultSkillIds,
} from '../src/components/digital-human/create/create-digital-human-payload.ts';

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
const { connectorOperationIds } = await import(connectorModelModuleURL);
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

/** Skills that really exist in the workspace, per the binding candidates API. */
const REAL_SKILL_IDS = ['file-generator'];

function resolvePath(dictionary, key) {
  return key
    .split('.')
    .reduce(
      (node, part) => (node === undefined || node === null ? undefined : node[part]),
      dictionary
    );
}

/** Dictionary-backed resolver; a typo in a template key fails this suite. */
function createResolver(dictionary, locale) {
  const missing = [];
  const resolve = key => {
    const value = resolvePath(dictionary, key);
    if (typeof value !== 'string' || !value.trim()) {
      missing.push(`${locale}:${key}`);
      return '';
    }
    return value;
  };
  resolve.missing = missing;
  return resolve;
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
// Template catalogue
// ---------------------------------------------------------------------------

assert.equal(DIGITAL_HUMAN_TEMPLATES.length, 2, 'the release ships the file and operations templates');
assert.equal(
  new Set(DIGITAL_HUMAN_TEMPLATES.map(template => template.id)).size,
  DIGITAL_HUMAN_TEMPLATES.length,
  'template ids must be unique'
);
assert.ok(
  DIGITAL_HUMAN_TEMPLATES.every(template => !isCustomTemplateId(template.id)),
  'the custom entry must never be registered as a template'
);
assert.equal(CUSTOM_TEMPLATE_ID, 'custom', 'the custom id is part of the create-page URL contract');

for (const template of DIGITAL_HUMAN_TEMPLATES) {
  assert.ok(template.tagKeys.length >= 3, `${template.id} needs at least three tags`);
  assert.ok(template.goalKeys.length >= 3, `${template.id} needs at least three goals`);
  assert.ok(
    template.skills.length >= 1 || template.id === 'opsSpecialist',
    `${template.id} must declare at least one real skill, or be the opsSpecialist design target`
  );
  for (const skill of template.skills) {
    assert.ok(
      REAL_SKILL_IDS.includes(skill.id),
      `${template.id} must only reference skills the workspace really offers (${skill.id})`
    );
  }
  // The create-time integration connection field is gone: no template may keep a
  // `connections` list, and a template that cannot bind a skill yet must declare
  // no skill at all rather than smuggle a capability in.
  assert.ok(
    !Object.prototype.hasOwnProperty.call(template, 'connections'),
    `${template.id} must not declare a connection field`
  );
  for (const sharedRuleKey of SHARED_RULE_KEYS) {
    assert.ok(
      template.ruleKeys.includes(sharedRuleKey),
      `${template.id} must inherit the shared safety rule ${sharedRuleKey}`
    );
  }
  assert.ok(template.ruleKeys.length >= 4, `${template.id} must write at least four rules`);
  assert.equal(
    new Set(template.ruleKeys).size,
    template.ruleKeys.length,
    `${template.id} must not repeat a rule`
  );
  assert.equal(template.defaults.autonomyLevel, 'supervised');
  assert.equal(template.defaults.visibility, 'workspace');
  assert.equal(template.defaults.memoryEnabled, false);
  assert.equal(template.defaults.manualTrigger, true);
  assert.match(
    template.iconBackground,
    /^#[0-9a-f]{6}$/i,
    `${template.id} needs a text-avatar colour`
  );
}

const fileGeneration = findDigitalHumanTemplate('fileGeneration');
assert.equal(fileGeneration?.id, 'fileGeneration');
assert.equal(
  DIGITAL_HUMAN_TEMPLATES[1]?.id,
  'opsSpecialist',
  'the operations template must be the second registered template'
);
const opsSpecialist = findDigitalHumanTemplate('opsSpecialist');
assert.equal(opsSpecialist?.id, 'opsSpecialist', 'the operations template must be registered');
assert.deepEqual(
  fileGeneration?.skills.map(skill => skill.id),
  ['file-generator'],
  'file generation must keep binding its real skill'
);
assert.deepEqual(
  opsSpecialist?.skills,
  [],
  'the operations template must not fake a skill that does not exist'
);
assert.ok(
  opsSpecialist !== null && !Object.prototype.hasOwnProperty.call(opsSpecialist, 'connections'),
  'the operations template must not declare a connection field'
);
assert.deepEqual(
  DIGITAL_HUMAN_TEMPLATES.filter(template => template.skills.length === 0).map(
    template => template.id
  ),
  ['opsSpecialist'],
  'only the operations template ships without a bindable skill'
);
assert.equal(findDigitalHumanTemplate('webResearch'), null, 'the removed search template must be gone');
assert.equal(findDigitalHumanTemplate('not-a-template'), null);
assert.equal(findDigitalHumanTemplate(null), null);
assert.equal(findDigitalHumanTemplate(undefined), null);

// ---------------------------------------------------------------------------
// Locale coverage
// ---------------------------------------------------------------------------

for (const [locale, dictionary] of [
  ['en-US', enUS],
  ['zh-Hans', zhHans],
]) {
  const texts = createResolver(dictionary, locale);
  for (const template of DIGITAL_HUMAN_TEMPLATES) {
    for (const key of [
      template.titleKey,
      template.shortDescriptionKey,
      template.responsibilityKey,
      template.personaKey,
    ]) {
      assert.ok(texts(key).length > 0, `${locale} is missing ${key}`);
    }
    for (const key of [
      ...template.tagKeys,
      ...template.goalKeys,
      ...template.ruleKeys,
      ...template.skills.map(skill => skill.labelKey),
    ]) {
      assert.ok(texts(key).length > 0, `${locale} is missing ${key}`);
    }
  }
  assert.deepEqual(texts.missing, [], `${locale} is missing create-flow copy`);
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

const zhTexts = createResolver(zhHans, 'zh-Hans');
const duty = '负责把每周进度整理成可交付文件。';
const persona = buildPersona(fileGeneration, duty, zhTexts);

assert.ok(
  persona.startsWith(zhHans.createFlow.templates.fileGeneration.persona),
  'the persona must start with the template persona'
);
assert.ok(persona.includes('## 职责范围'), 'the persona must label the responsibility section');
assert.ok(
  persona.includes(`- 用户确认的职责描述：${duty}`),
  'the persona must carry the user-confirmed responsibility verbatim'
);
assert.ok(
  persona.includes(`- 角色定位：${zhHans.createFlow.templates.fileGeneration.title}`),
  'the persona must carry the template title'
);

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
  icon: { type: 'text', icon: 'XY', iconBackground: '#0f766e' },
  model: 'deepseek-chat',
  template: fileGeneration,
  texts: zhTexts,
});

assert.deepEqual(
  Object.keys(templatePayload).sort(),
  EXPECTED_PAYLOAD_KEYS,
  'the create request must only carry the agreed fields, never credentials or capability claims'
);
assert.equal(templatePayload.name, '小研');
assert.equal(templatePayload.identity_name, '小研');
assert.equal(templatePayload.description, duty);
assert.equal(templatePayload.default_model, 'deepseek-chat');
assert.equal(templatePayload.autonomy_level, 'supervised');
assert.equal(templatePayload.visibility, 'workspace');
assert.deepEqual(templatePayload.memory_policy, { enabled: false, ownership: 'digital_human' });
assert.deepEqual(templatePayload.trigger_policy, { manual: true });
assert.ok(templatePayload.persona.includes(duty));
assert.deepEqual(
  templatePayload.goals,
  fileGeneration.goalKeys.map(key => zhTexts(key)),
  'goals must come from the template'
);
assert.deepEqual(
  templatePayload.rules,
  fileGeneration.ruleKeys.map(key => zhTexts(key)),
  'rules must be the shared safety rules plus the template rule'
);
assert.ok(
  templatePayload.rules.includes(zhHans.createFlow.sharedRules.credentials),
  'every digital human must carry the credential rule'
);
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

// Every template must produce a usable digital human.
for (const template of DIGITAL_HUMAN_TEMPLATES) {
  const payload = createDigitalHumanPayload({
    name: 'x',
    responsibility: 'y',
    icon: { type: 'text', icon: 'X', iconBackground: template.iconBackground },
    model: 'm',
    template,
    texts: zhTexts,
  });
  assert.ok(payload.persona.length > 0, `${template.id} persona must not be empty`);
  assert.equal(payload.goals.length, template.goalKeys.length, `${template.id} must write goals`);
  assert.equal(
    payload.rules.length,
    template.ruleKeys.length,
    `${template.id} must write rules`
  );
  assert.equal(payload.default_model, 'm');
}

// The operations template must create a usable digital human without smuggling
// a binding, concrete connection, or credential into the create request.
const opsPayload = createDigitalHumanPayload({
  name: '运维专员',
  responsibility: '整理环境问题证据。',
  icon: { type: 'text', icon: '', iconBackground: opsSpecialist.iconBackground },
  model: 'deepseek-chat',
  template: opsSpecialist,
  texts: zhTexts,
});
assert.deepEqual(Object.keys(opsPayload).sort(), EXPECTED_PAYLOAD_KEYS);
assert.equal(opsPayload.goals.length, opsSpecialist.goalKeys.length);
assert.equal(opsPayload.rules.length, opsSpecialist.ruleKeys.length);
assert.ok(
  opsPayload.persona.includes(zhHans.createFlow.templates.opsSpecialist.persona),
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

// Safety copy: the operations template must state the read-only boundary in both
// locales and must never imply an approval can be reused or a connection works
// before it is bound and verified.
const opsCopyZh = zhHans.createFlow.templates.opsSpecialist;
const opsCopyEn = enUS.createFlow.templates.opsSpecialist;
assert.ok(
  opsCopyZh.responsibility.includes('不执行部署、重启、写入、配置修改，也不运行任意命令'),
  'the zh operations responsibility must state the read-only boundary'
);
assert.ok(
  opsCopyEn.responsibility.includes(
    'It does not deploy, restart, write, change configuration, or run arbitrary commands.'
  ),
  'the en operations responsibility must state the read-only boundary'
);
assert.ok(
  opsCopyZh.persona.includes('不得部署、重启、写入、修改配置、开启交互式会话或运行任意命令') &&
    opsCopyZh.persona.includes('不得假设之前的批准仍然有效或已被记住'),
  'the zh operations persona must forbid writes and remembered approval'
);
assert.ok(
  opsCopyEn.persona.includes(
    'Never deploy, restart, write, change configuration, open an interactive session, or run arbitrary commands.'
  ) && opsCopyEn.persona.includes('never assume an earlier approval still applies or has been remembered'),
  'the en operations persona must forbid writes and remembered approval'
);
assert.ok(
  !Object.prototype.hasOwnProperty.call(opsCopyZh, 'connections') &&
    !Object.prototype.hasOwnProperty.call(opsCopyEn, 'connections'),
  'the operations copy must not keep a connection family label'
);

// Phase one reaches servers through the DigitalHuman-only serverops_ssh
// connector. Both locales must describe only the supported behaviour: the user
// creates and verifies SSH server connections in Connector Center, binds one or
// more distinct named servers in the generic create form, and the role uses only
// the approved per-invocation read-only actions. They must never claim the
// connector works before it is bound and verified, never promise deploy, restart,
// write, arbitrary-command, or configuration-change behaviour, and never claim an
// approval can be remembered.
const OPS_PHASE_ONE_COPY = {
  'en-US': {
    shortDescription: [
      /verify in Connector Center/i,
      /system snapshot, systemd service status, and a bounded journal tail/i,
      /No deploy, restart, or configuration changes/i,
    ],
    responsibility: [
      /one or more named Linux servers you create and verify in Connector Center/i,
      /more than one server is bound and a request does not name a target/i,
      /ask which server to check before acting/i,
      /per-invocation approval/i,
      /does not deploy, restart, write, change configuration, or run arbitrary commands/i,
      /usable only after it is verified and bound/i,
    ],
    persona: [
      /created and verified in Connector Center and bound to you/i,
      /not bound or not verified is not usable/i,
      /ask the user which server they mean/i,
      /one invocation at a time/i,
      /Every invocation needs its own approval/i,
      /has been remembered/i,
      /Separate observation from inference/i,
    ],
    notice: [/does not bind a Skill on its own/i, /verified and bound to this digital human/i],
  },
  'zh-Hans': {
    shortDescription: [
      /已在连接中心验证并绑定/,
      /系统快照、systemd 服务状态和有界日志尾部/,
      /不部署、不重启、不修改配置/,
    ],
    responsibility: [
      /一台或多台具名 Linux 服务器/,
      /先询问要检查哪一台/,
      /每次调用都需要单独批准/,
      /不执行部署、重启、写入、配置修改，也不运行任意命令/,
      /连接只有在通过验证并绑定后才可用/,
    ],
    persona: [
      /在连接中心创建并验证、且已绑定给你的 SSH 服务器连接/,
      /未绑定或未验证的连接不可用/,
      /先询问用户指的是哪一台/,
      /每次只发起一次调用/,
      /每次调用都需要当次批准/,
      /已被记住/,
      /把观察与推断分开/,
    ],
    notice: [/该模板本身不会绑定技能/, /通过验证并绑定到这个数字人/],
  },
};
for (const [locale, dictionary] of [
  ['en-US', enUS],
  ['zh-Hans', zhHans],
]) {
  const copy = dictionary.createFlow.templates.opsSpecialist;
  const expected = OPS_PHASE_ONE_COPY[locale];
  for (const [field, patterns] of [
    ['shortDescription', expected.shortDescription],
    ['responsibility', expected.responsibility],
    ['persona', expected.persona],
    ['notice', expected.notice],
  ]) {
    const text = field === 'notice' ? dictionary.createFlow.form.templateNotEnabled : copy[field];
    assert.equal(typeof text, 'string', `${locale} opsSpecialist.${field} must be copy`);
    assert.ok(text.trim().length > 0, `${locale} opsSpecialist.${field} must not be empty`);
    for (const pattern of patterns) {
      assert.match(
        text,
        pattern,
        `${locale} opsSpecialist.${field} must state the phase-one boundary (${pattern})`
      );
    }
  }
}

// Custom (blank) entry keeps the previous behaviour.
const customPayload = createDigitalHumanPayload({
  name: '自定义助手',
  responsibility: '整理每周进度。',
  icon: { type: 'text', icon: '', iconBackground: '' },
  model: 'deepseek-chat',
  template: null,
  texts: zhTexts,
});
assert.deepEqual(Object.keys(customPayload).sort(), EXPECTED_PAYLOAD_KEYS);
assert.equal(customPayload.persona, '整理每周进度。');
assert.deepEqual(customPayload.goals, []);
assert.deepEqual(customPayload.rules, []);
assert.equal(customPayload.visibility, 'workspace');
assert.deepEqual(customPayload.memory_policy, { enabled: false, ownership: 'digital_human' });

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
assert.match(
  connectorCreatePickerSource,
  /\/console\/connectors\/connections/,
  'the picker must link to Connector Center for creation, verification, and maintenance'
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
  assert.ok(
    !Object.prototype.hasOwnProperty.call(dictionary.createFlow.templates.opsSpecialist, 'connections'),
    `${locale} opsSpecialist must not keep a connection label`
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

// A template that seeds no skill must warn the user only while nothing is
// selected: the generic form can bind a Skill or a Connector connection, and
// once one is selected the notice must disappear instead of denying a capability
// creation can now deliver.
assert.match(
  flowSource,
  /const templateWithoutSkills = Boolean\(template && template\.skills\.length === 0\)/,
  'the flow must detect a template that declares no skill'
);
assert.match(
  flowSource,
  /const hasSelectedCapability = selectedSkillIds\.length > 0 \|\| connectorSelections\.length > 0/,
  'a selected skill or Connector connection must count as a selected capability'
);
assert.match(
  flowSource,
  /const showTemplateNotEnabled = templateWithoutSkills && !hasSelectedCapability/,
  'the not-enabled notice must require a zero-seed-skill template with nothing selected'
);
assert.match(
  flowSource,
  /showTemplateNotEnabled\s*\?\s*t\('createFlow\.form\.templateNotEnabled'\)/,
  'such a template must surface the localized not-enabled notice'
);
assert.match(
  formSource,
  /notice\?: string \| null;/,
  'the form must accept the inline notice copy'
);
assert.match(
  formSource,
  /notice \? \([\s\S]*role="status"[\s\S]*\{notice\}/,
  'the form must render the notice as a status message and never as a silent hint'
);

// Gallery: catalogue driven, keyboard reachable, and it never claims a skill is bound yet.
assert.match(gallerySource, /DIGITAL_HUMAN_TEMPLATES\.map\(/);
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
  /template\.skills\.map\(skill => texts\(skill\.labelKey\)\)/,
  'skill labels must resolve from the template skill keys'
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

// Combined with the template facts, the render guard cannot produce a skills
// section for a template that declares none, and the operations template must
// stay free of any connection or capability claim.
assert.deepEqual(
  fileGeneration.skills.map(skill => skill.id),
  ['file-generator'],
  'file generation still declares exactly its real skill'
);
assert.deepEqual(opsSpecialist.skills, [], 'the operations template still declares no skill');
assert.ok(
  opsSpecialist !== null && !Object.prototype.hasOwnProperty.call(opsSpecialist, 'connections'),
  'the operations template still declares no connection field'
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

console.log('digital human create-flow contract tests passed');
