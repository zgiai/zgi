import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import ts from 'typescript';
import vm from 'node:vm';
import { createRequire } from 'node:module';

const root = process.cwd();
const helperPath = path.join(root, 'src/components/workflow/utils/workflow-precheck.ts');
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

const {
  loadAdvisoryWorkflowPrecheckWarnings,
  workflowPrecheckGraphNodes,
  workflowPrecheckModelFingerprint,
} = helperModule.exports;

const warning = { code: 207015, params: { reason: 'auth_invalid' } };
assert.deepEqual(
  globalThis.structuredClone(
    await loadAdvisoryWorkflowPrecheckWarnings(async () => ({
      status: 'warning',
      warnings: [warning],
    }))
  ),
  [warning],
  'warning status should expose workflow channel warnings'
);
assert.deepEqual(
  globalThis.structuredClone(
    await loadAdvisoryWorkflowPrecheckWarnings(async () => ({
      status: 'ok',
      warnings: [warning],
    }))
  ),
  [],
  'healthy workflow models should not expose stale warnings'
);
assert.deepEqual(
  globalThis.structuredClone(
    await loadAdvisoryWorkflowPrecheckWarnings(async () => {
      throw new Error('precheck unavailable');
    })
  ),
  [],
  'a failed advisory precheck must not block workflow execution'
);

const llmNode = {
  id: 'llm-1',
  data: {
    type: 'llm',
    title: 'Draft title',
    model: {
      provider: 'deepseek',
      name: 'deepseek-chat',
      completion_params: { temperature: 0.7 },
    },
  },
};
const originalFingerprint = workflowPrecheckModelFingerprint([llmNode]);
assert.deepEqual(
  globalThis.structuredClone(
    workflowPrecheckGraphNodes([
      llmNode,
      { id: 'answer-1', data: { type: 'answer', text: 'not needed by precheck' } },
    ])
  ),
  [
    {
      id: 'llm-1',
      data: {
        type: 'llm',
        model: { provider: 'deepseek', name: 'deepseek-chat' },
      },
    },
  ],
  'the precheck request should contain model routing fields only'
);
assert.equal(
  workflowPrecheckModelFingerprint([
    { ...llmNode, data: { ...llmNode.data, title: 'Renamed node' } },
  ]),
  originalFingerprint,
  'non-model edits should not trigger another precheck'
);
assert.equal(
  workflowPrecheckModelFingerprint([
    {
      ...llmNode,
      data: {
        ...llmNode.data,
        model: {
          ...llmNode.data.model,
          completion_params: { temperature: 0.2 },
        },
      },
    },
  ]),
  originalFingerprint,
  'generation parameter changes should not trigger a routing precheck'
);
assert.notEqual(
  workflowPrecheckModelFingerprint([
    {
      ...llmNode,
      data: {
        ...llmNode.data,
        model: { provider: 'deepseek', name: 'deepseek-reasoner' },
      },
    },
  ]),
  originalFingerprint,
  'changing the selected model must trigger another precheck'
);

const files = {
  runPanel: fs.readFileSync(
    path.join(root, 'src/components/workflow/ui/workflow-run-panel/index.tsx'),
    'utf8'
  ),
  chatPanel: fs.readFileSync(
    path.join(
      root,
      'src/components/workflow/ui/workflow-chat-panel/hooks/use-workflow-chat-panel-state.tsx'
    ),
    'utf8'
  ),
  service: fs.readFileSync(path.join(root, 'src/services/workflow.service.ts'), 'utf8'),
};

assert.match(
  files.runPanel,
  /workflowPrecheckModelFingerprint\(nodes\)/,
  'the debug panel must track the current canvas model selection'
);
assert.match(
  files.runPanel,
  /if \(!open \|\| isHistory[\s\S]*loadWorkflowPrecheckWarnings\(\)/,
  'opening the draft debug panel must load warnings before execution'
);
assert.match(
  files.runPanel,
  /graph:\s*\{\s*nodes:\s*currentNodes\s*\}/,
  'the precheck must send the current canvas instead of waiting for autosave'
);
assert.match(
  files.runPanel,
  /void loadWorkflowPrecheckWarnings\(payload\.inputs\)[\s\S]*start\(payload\)/,
  'ordinary workflow execution must not wait for the advisory precheck'
);
assert.match(
  files.service,
  /buildWorkflowDraftPrecheckBody/,
  'the workflow service must preserve the current graph in the precheck request'
);
assert.match(
  files.chatPanel,
  /workflowPrecheckModelFingerprint\(nodes\)/,
  'the conversation workflow debugger must track the current canvas model selection'
);
assert.match(
  files.chatPanel,
  /if \(!open \|\| !canRunDraft[\s\S]*loadWorkflowChatPrecheckWarnings\(\)/,
  'opening the conversation workflow debugger must load warnings before sending'
);
assert.match(
  files.chatPanel,
  /graph:\s*\{\s*nodes:\s*currentNodes\s*\}/,
  'conversation workflow precheck must send the current canvas'
);
assert.match(
  files.chatPanel,
  /void loadWorkflowChatPrecheckWarnings\(payload\)[\s\S]*await start\(payload\)/,
  'conversation workflow execution must not wait for the advisory precheck'
);
assert.match(
  files.service,
  /buildWorkflowChatDraftPrecheckBody/,
  'the conversation workflow service must preserve the current graph in the precheck request'
);

console.log('workflow draft precheck warning regression checks passed');
