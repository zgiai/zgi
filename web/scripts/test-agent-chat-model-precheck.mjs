import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import ts from 'typescript';
import vm from 'node:vm';
import { createRequire } from 'node:module';

const root = process.cwd();
const helperPath = path.join(root, 'src/components/agents/agent-runtime/model-precheck.ts');
const helperSource = fs.readFileSync(helperPath, 'utf8');
const transpiled = ts.transpileModule(helperSource, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  fileName: helperPath,
}).outputText;
const helperModule = { exports: {} };
vm.runInNewContext(transpiled, {
  module: helperModule,
  exports: helperModule.exports,
  require: createRequire(helperPath),
});

const { allowSendAfterAgentModelPrecheck, visibleAgentModelPrecheckWarnings } =
  helperModule.exports;
const warning = {
  kind: 'private_channel_upstream_unavailable',
  reason: 'auth_invalid',
  scope: 'all',
};

assert.deepEqual(
  structuredClone(visibleAgentModelPrecheckWarnings({ status: 'warning', warnings: [warning] })),
  [warning],
  'warning status should expose warnings'
);
assert.deepEqual(
  structuredClone(visibleAgentModelPrecheckWarnings({ status: 'ok', warnings: [warning] })),
  [],
  'healthy model should not expose stale warnings'
);
assert.deepEqual(
  structuredClone(visibleAgentModelPrecheckWarnings({ status: 'unknown', warnings: [warning] })),
  [],
  'unknown status should not guess a warning'
);

const events = [];
const allowed = await allowSendAfterAgentModelPrecheck(async () => {
  events.push('precheck');
});
assert.equal(allowed, true, 'a warning precheck must never block sending');
if (allowed) events.push('send');
assert.deepEqual(events, ['precheck', 'send']);

assert.equal(
  await allowSendAfterAgentModelPrecheck(async () => {
    throw new Error('precheck unavailable');
  }),
  true,
  'a failed precheck must never block sending'
);

const files = {
  preview: fs.readFileSync(
    path.join(root, 'src/components/agents/agent-runtime/preview-panel.tsx'),
    'utf8'
  ),
  webapp: fs.readFileSync(path.join(root, 'src/components/webapp/agent-chat/index.tsx'), 'utf8'),
  agentService: fs.readFileSync(path.join(root, 'src/services/agent.service.ts'), 'utf8'),
  webappService: fs.readFileSync(path.join(root, 'src/services/webapp.service.ts'), 'utf8'),
};

assert.match(files.preview, /useAgentDraftModelPrecheck/, 'draft preview must query model risk');
assert.match(files.preview, /inputTopNotice=/, 'draft preview must render the warning above input');
assert.match(files.preview, /beforeSend=/, 'draft preview must refresh before sending');
assert.match(
  files.webapp,
  /usePublishedAgentModelPrecheck/,
  'published agent must query model risk'
);
assert.match(
  files.webapp,
  /inputTopNotice=/,
  'published agent must render the warning above input'
);
assert.match(files.webapp, /beforeSend=/, 'published agent must refresh before sending');
assert.match(
  files.agentService,
  /\/agents\/\$\{agentId\}\/runtime\/model-precheck/,
  'draft agent service must use the dedicated precheck endpoint'
);
assert.match(
  files.webappService,
  /\/webapps\/\$\{webAppId\}\/runtime\/model-precheck/,
  'published agent service must use the dedicated precheck endpoint'
);

console.log('agent chat model precheck regression checks passed');
