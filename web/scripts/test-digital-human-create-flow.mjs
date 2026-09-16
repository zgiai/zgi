/**
 * Contract tests for the digital human template creation flow.
 *
 * These run without rendering anything: they prove what a template selection
 * creates, which real skills it declares and resolves from backend candidates,
 * that every template key resolves in both locales, and that the UI layer keeps
 * its guarantees (single submit, auto publish, no fake capability claims).
 *
 * Creation no longer reads, selects, or binds the old integration connections:
 * the operations template is a design target that declares no skill, so it binds
 * nothing and the UI says so instead of implying a working server connection.
 *
 * Run with: pnpm test:digital-human-create-flow
 */

import assert from 'node:assert/strict';
import { readFile, readdir } from 'node:fs/promises';
import { URL } from 'node:url';

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
// locales and must never claim service-status or log capabilities it lacks.
const opsCopyZh = zhHans.createFlow.templates.opsSpecialist;
const opsCopyEn = enUS.createFlow.templates.opsSpecialist;
assert.ok(
  opsCopyZh.responsibility.includes('不执行部署、重启、配置修改或任意命令'),
  'the zh operations responsibility must state the read-only boundary'
);
assert.ok(
  opsCopyEn.responsibility.includes(
    'Deployment, restarts, configuration changes, and arbitrary commands are not performed.'
  ),
  'the en operations responsibility must state the read-only boundary'
);
assert.ok(
  opsCopyZh.persona.includes('不得声称已完成部署、重启、配置修改、服务状态检查或日志读取'),
  'the zh operations persona must not claim unverified service or log capabilities'
);
assert.ok(
  opsCopyEn.persona.includes(
    'never claim to have deployed, restarted, changed configuration, checked service status, or read logs'
  ),
  'the en operations persona must not claim unverified service or log capabilities'
);
assert.ok(
  !Object.prototype.hasOwnProperty.call(opsCopyZh, 'connections') &&
    !Object.prototype.hasOwnProperty.call(opsCopyEn, 'connections'),
  'the operations copy must not keep a connection family label'
);

// The operations template is a design target, not a wired capability. Both
// locales must say it cannot connect, inspect, or deploy yet, that target
// authorization plus read-only verification come first, and that deployment or
// configuration changes need separate approval.
const OPS_UNDELIVERED_COPY = {
  'en-US': {
    shortDescription: [/not connected to a server yet/i, /cannot connect, inspect, or deploy/i],
    responsibility: [
      /It is not connected to a server yet and has no configured, verified tools or authorization/i,
      /must not connect to a target, collect data, or claim measured findings/i,
    ],
    persona: [
      /is not connected to any server/i,
      /until its tools and authorization are configured and verified/i,
      /must not connect to a target, collect system state, or claim measured findings/i,
      /once enabled it operates only within the approved scope using verified actions/i,
      /target authorization and read-only verification first/i,
      /deployment or configuration changes need separate approval/i,
    ],
    notice: [
      /is not connected to a server yet/i,
      /Until target authorization and read-only verification are complete/i,
      /cannot connect to a target, collect state, or run operations/i,
      /deployment or configuration changes need separate approval/i,
    ],
  },
  'zh-Hans': {
    shortDescription: [/尚未接入服务器/, /不能连接、巡检或部署/],
    responsibility: [
      /当前尚未接入服务器，也还没有已配置并验证的工具与授权/,
      /不得连接目标、不得采集数据，也不得声称实测结论/,
    ],
    persona: [
      /尚未接入任何服务器/,
      /在工具与授权尚未配置并验证之前/,
      /不得连接目标、不得采集系统状态，也不得声称实测结论/,
      /启用之后，仅可在已批准范围内按已验证动作操作/,
      /目标服务器授权与只读能力验证/,
      /部署或配置变更的操作还需要单独审批/,
    ],
    notice: [
      /尚未接入服务器/,
      /在完成目标授权与只读能力验证之前/,
      /不能连接目标、采集状态或执行操作/,
      /部署或配置变更的操作还需要单独审批/,
    ],
  },
};
for (const [locale, dictionary] of [
  ['en-US', enUS],
  ['zh-Hans', zhHans],
]) {
  const copy = dictionary.createFlow.templates.opsSpecialist;
  const expected = OPS_UNDELIVERED_COPY[locale];
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
        `${locale} opsSpecialist.${field} must state the current boundary (${pattern})`
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
// One binding replacement for the selected skills
// ---------------------------------------------------------------------------

// The flow builds the skill bindings with the approved helper and never
// fabricates a binding for a capability the workspace does not offer.
assert.match(
  flowSource,
  /selectCandidateBindings\(selectedSkillIds, skillCandidates\)/,
  'skill bindings must come from the approved helper'
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
const catchStart = flowSource.indexOf('} catch {');
const catchEnd = flowSource.indexOf('} finally {', catchStart);
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
assert.ok(
  createCallIndex > 0 && bindCallIndex > createCallIndex && publishCallIndex > bindCallIndex,
  'the flow must create, then bind the selected capabilities, then publish'
);
assert.ok(
  submitBody.indexOf('digitalHumanService.get(') === -1 ||
    submitBody.indexOf('digitalHumanService.get(') < publishCallIndex,
  'the fresh read must stay before publishing'
);
assert.match(flowSource, /replaceDraftBindings/, 'binding must go through the bindings API');
assert.match(
  flowSource,
  /selectCandidateBindings\(selectedSkillIds, skillCandidates\)/,
  'bindings must be resolved from the real binding candidates and the user selection'
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

// A template that declares no capability this product can bind yet must warn the
// user in the configure form instead of letting creation imply a working server.
assert.match(
  flowSource,
  /const templateWithoutSkills = Boolean\(template && template\.skills\.length === 0\)/,
  'the flow must detect a template that declares no skill'
);
assert.match(
  flowSource,
  /templateWithoutSkills\s*\?\s*t\('createFlow\.form\.templateNotEnabled'\)/,
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
// The operations copy honestly says it is *not* connected yet and only acts once
// a server *is* connected, so a bare /connected/ match would flag a negation or a
// condition; the English patterns therefore target positive claims only.
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
