import assert from 'node:assert/strict';
import fs from 'node:fs';
import { createRequire } from 'node:module';
import Module from 'node:module';
import path from 'node:path';

const require = createRequire(import.meta.url);
const ts = require('typescript');

Module._extensions['.ts'] = (mod, filename) => {
  const source = require('node:fs').readFileSync(filename, 'utf8');
  const output = ts.transpileModule(source, {
    compilerOptions: {
      esModuleInterop: true,
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2020,
    },
    fileName: filename,
  }).outputText;
  mod._compile(output, filename);
};

const {
  SKILL_CAPABILITY_CATEGORIES,
  SKILL_SCENARIOS,
  getSkillCapabilityLabel,
  getSkillScenarioLabel,
  normalizeSkillCapabilityCategory,
  resolveSkillScenarios,
} = require('../src/components/chat/variants/aichat/skill-taxonomy.ts');
const {
  buildAIChatSkillDisplayMap,
  getAIChatSkillDisplayInfo,
} = require('../src/components/chat/variants/aichat/skill-display.ts');
const {
  AI_CHAT_SKILL_ICON_BY_KEY,
} = require('../src/components/chat/variants/aichat/skill-icon-registry.ts');

assert.equal(SKILL_CAPABILITY_CATEGORIES.length, 10);
assert.equal(SKILL_SCENARIOS.length, 13);

assert.equal(normalizeSkillCapabilityCategory(' productivity '), 'office_productivity');
assert.equal(normalizeSkillCapabilityCategory('visualization'), 'content_creation');
assert.equal(normalizeSkillCapabilityCategory('document_processing'), 'document_processing');
assert.equal(normalizeSkillCapabilityCategory('unknown-provider-category'), 'other');

assert.deepEqual(
  resolveSkillScenarios({
    category: 'document_processing',
    scenarios: [' Document_Handling ', '', 'document_handling', 'LEGAL_COMPLIANCE'],
  }),
  ['document_handling', 'legal_compliance']
);
assert.deepEqual(resolveSkillScenarios({ category: 'database' }), ['data_insights']);
assert.deepEqual(resolveSkillScenarios({ category: 'unknown-provider-category' }), ['other']);

assert.equal(getSkillCapabilityLabel('document_processing', 'zh-Hans'), '文档处理');
assert.equal(getSkillCapabilityLabel('document_processing', 'en-US'), 'Document processing');
assert.equal(getSkillScenarioLabel('customer_service', 'zh-Hans'), '客户服务');
assert.equal(getSkillScenarioLabel('customer_service', 'en-US'), 'Customer service');

const systemSkillWithApiTaxonomy = {
  skill_id: 'time',
  name: 'Time',
  description: 'API description',
  when_to_use: 'API use case',
  runtime_type: 'tool',
  enabled: true,
  has_tools: true,
  has_references: false,
  has_scripts: false,
  scripts_supported: false,
  max_calls_per_turn: 1,
  timeout_seconds: 10,
  display: {
    category: 'data_analysis',
    scenarios: ['data_insights'],
    label: { zh_Hans: '接口时间能力' },
  },
};
const resolvedDisplay = getAIChatSkillDisplayInfo(systemSkillWithApiTaxonomy, 'zh-Hans');
assert.equal(resolvedDisplay.label, '接口时间能力');
assert.equal(resolvedDisplay.category, 'data_analysis');
assert.equal(resolvedDisplay.categoryLabel, '数据分析');
assert.deepEqual(resolvedDisplay.scenarios, ['data_insights']);

const displayMap = buildAIChatSkillDisplayMap([systemSkillWithApiTaxonomy], 'zh-Hans');
assert.equal(displayMap.time.category, 'data_analysis');
assert.deepEqual(displayMap.time.scenarios, ['data_insights']);

const catalogRoot = path.resolve('../api/internal/modules/skills/catalog');
const catalogIcons = fs
  .readdirSync(catalogRoot, { withFileTypes: true })
  .filter(entry => entry.isDirectory())
  .map(entry => {
    const markdown = fs.readFileSync(path.join(catalogRoot, entry.name, 'SKILL.md'), 'utf8');
    const icon = markdown.match(/^ {2}icon: ([^\r\n]+)$/m)?.[1]?.trim();
    assert.ok(icon, `${entry.name} must declare display.icon`);
    assert.ok(AI_CHAT_SKILL_ICON_BY_KEY[icon], `${entry.name} uses unsupported icon ${icon}`);
    return icon;
  });

assert.equal(catalogIcons.length, 30);
assert.equal(new Set(catalogIcons).size, 30, 'built-in Skills should use distinct icons');

console.log('AIChat Skill taxonomy checks passed.');
