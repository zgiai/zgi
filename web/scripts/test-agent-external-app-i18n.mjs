import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';

const root = process.cwd();
const read = relativePath => readFileSync(path.join(root, relativePath), 'utf8');

function agentIntegrationMessages(source) {
  const start = source.lastIndexOf('\n    integration: {');
  const end = source.indexOf('\n    suggestions: {', start);
  assert.ok(start >= 0 && end > start, 'agent external-app translation block is missing');
  return source.slice(start, end);
}

const zhMessages = agentIntegrationMessages(read('src/i18n/modules/agents/zh-Hans.ts'));
const enMessages = agentIntegrationMessages(read('src/i18n/modules/agents/en-US.ts'));

for (const expected of [
  "helpText: '选择组织已授权且可用的连接，并指定智能体可以执行的操作。'",
  "dialogTitle: '添加外部应用'",
  "connectionStatus: '{integration} · {status}'",
  "allowedActions: '允许执行的操作'",
  "actionUnavailable: '操作授权已失效'",
]) {
  assert.ok(zhMessages.includes(expected), `missing zh-Hans copy: ${expected}`);
}
assert.doesNotMatch(
  zhMessages,
  /\b(?:Agent|Action|Integration)\b/,
  'zh-Hans external-app copy must not mix untranslated product terms'
);
assert.doesNotMatch(enMessages, /[\u3400-\u9fff]/, 'en-US external-app copy contains Chinese text');

const dialog = read('src/components/agents/agent-runtime/integration-dialog.tsx');
assert.match(dialog, /useIntegrationMetadata\(\)/);
assert.match(dialog, /t\('integration\.connectionStatus'/);
assert.match(dialog, /integrationMetadata\.providerName\(/);
assert.match(dialog, /integrationMetadata\.actionName\(/);
assert.match(dialog, /integrationMetadata\.actionDescription\(/);
assert.doesNotMatch(dialog, /\{integrationName\}\s*·/);
assert.doesNotMatch(
  dialog,
  /catalogByIntegration\.get\([^\n]+\)\s*\?\?\s*candidate\.integration_id/
);

const section = read('src/components/agents/agent-runtime/sections/integration-section.tsx');
assert.match(section, /useIntegrationMetadata\(\)/);
assert.match(section, /integrationMetadata\.providerName\(/);
assert.match(section, /integrationMetadata\.actionName\(/);
assert.doesNotMatch(section, /catalogItem\s*\?\?\s*binding\.integration_id/);

const pageModel = read(
  'src/components/agents/agent-runtime/hooks/use-agent-runtime-page-model.tsx'
);
assert.doesNotMatch(pageModel, /safeIntegrationDisplayText\([^\n]*'External app'/);
assert.match(pageModel, /integrationAction:\s*t\('bindingHealth\.types\.integration_action'\)/);
assert.match(pageModel, /getAgentBindingConflict\(error\)/);
assert.match(pageModel, /t\('toasts\.saveBindingsInvalid'\)/);
assert.doesNotMatch(
  pageModel,
  /onSaveFailed:[\s\S]{0,300}getErrorMessage\(error\)/,
  'Agent draft save must not surface untranslated backend error prose'
);

const bindingHealth = read('src/components/agents/agent-runtime/binding-health.tsx');
assert.doesNotMatch(bindingHealth, /safeIntegrationDisplayText\((?:reason|suggestion),/);
assert.match(bindingHealth, /return t\('bindingHealth\.reasons\.unknown'\)/);
assert.match(bindingHealth, /return t\('bindingHealth\.suggestions\.unknown'\)/);

console.log('Agent external-app i18n checks passed.');
