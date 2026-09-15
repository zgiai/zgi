/**
 * Contract tests for the digital human template creation flow.
 *
 * These run without rendering anything: they prove what a template selection
 * creates, which real skills it binds, that every template key resolves in both
 * locales, and that the UI layer keeps its guarantees (single submit, auto
 * publish, no fake capability claims).
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

assert.equal(DIGITAL_HUMAN_TEMPLATES.length, 1, 'the first release ships one template example');
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
  assert.ok(template.skills.length >= 1, `${template.id} must bind at least one real skill`);
  for (const skill of template.skills) {
    assert.ok(
      REAL_SKILL_IDS.includes(skill.id),
      `${template.id} must only reference skills the workspace really offers (${skill.id})`
    );
  }
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

// Creation order inside submit: create -> bind real skills -> publish.
const submitStart = flowSource.indexOf('const submit = async () => {');
const submitEnd = flowSource.indexOf('const draftBase =', submitStart);
assert.ok(submitStart > 0 && submitEnd > submitStart, 'the flow must keep a submit handler');
const submitBody = flowSource.slice(submitStart, submitEnd);
const createCallIndex = submitBody.indexOf('mutations.create.mutateAsync');
const bindCallIndex = submitBody.indexOf('bindSelectedSkills(');
const publishCallIndex = submitBody.indexOf('.publish(');
assert.ok(
  createCallIndex > 0 && bindCallIndex > createCallIndex && publishCallIndex > bindCallIndex,
  'the flow must create, then bind skills, then publish'
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

// Preview and dialog report a published digital human.
assert.match(previewSource, /createFlow\.preview\.published/);
assert.doesNotMatch(previewSource, /createFlow\.preview\.draft/);
assert.match(successSource, /createFlow\.success\.boundSkills/);
assert.match(
  successSource,
  /boundSkillNames\.length > 0/,
  'the dialog must list the skills that were really bound'
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
const forbiddenClaims = [
  /已连接/,
  /连接成功/,
  /在线/,
  /运行中/,
  /已部署/,
  /部署成功/,
  /已启用/,
  /connected/i,
  /online/i,
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
