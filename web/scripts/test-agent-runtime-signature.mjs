import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import Module from 'node:module';
import path from 'node:path';
import ts from 'typescript';

const root = process.cwd();
const utilsPath = path.join(root, 'src/components/agents/agent-runtime/utils.ts');
const output = ts.transpileModule(readFileSync(utilsPath, 'utf8'), {
  compilerOptions: {
    module: ts.ModuleKind.CommonJS,
    target: ts.ScriptTarget.ES2022,
    esModuleInterop: true,
  },
  fileName: utilsPath,
}).outputText;
const testModule = new Module(utilsPath);
testModule.filename = utilsPath;
testModule.paths = Module._nodeModulePaths(path.dirname(utilsPath));
const originalRequire = testModule.require.bind(testModule);
testModule.require = moduleID => {
  if (moduleID === '@/lib/config') return { ICON_BG: '#000000' };
  return originalRequire(moduleID);
};
testModule._compile(output, utilsPath);

const { buildAgentRuntimeSignature } = testModule.exports;

const basePayload = {
  system_prompt: 'prompt',
  model_provider: 'provider',
  model: 'model',
  model_parameters: {},
  enabled_skill_ids: [],
  use_memory: false,
  agent_memory_enabled: true,
  agent_memory_auto_extraction_enabled: false,
  agent_memory_slots: [
    {
      key: 'profile',
      name: 'User profile',
      description: 'Stable user profile facts',
      max_chars: 2000,
      enabled: true,
      sort_order: 0,
    },
  ],
  file_upload_enabled: false,
  home_title: 'Agent',
  opening_statement: '',
  input_placeholder: '',
  theme_color: 'default',
  suggested_questions: [],
  knowledge_dataset_ids: [],
  knowledge_retrieval_config: {},
  database_bindings: [],
  workflow_bindings: [],
  integration_bindings: [],
};

assert.equal(
  buildAgentRuntimeSignature(basePayload),
  buildAgentRuntimeSignature({
    ...basePayload,
    agent_memory_config_revision: 'server-revision-after-save',
    agent_memory_slots: [
      {
        ...basePayload.agent_memory_slots[0],
        id: 'server-slot-id',
        updated_at: 1787043600,
      },
    ],
  }),
  'Server memory revision and persistence metadata must not keep the editor dirty after save.'
);

assert.notEqual(
  buildAgentRuntimeSignature(basePayload),
  buildAgentRuntimeSignature({
    ...basePayload,
    agent_memory_slots: [
      {
        ...basePayload.agent_memory_slots[0],
        name: 'Preferred user profile',
      },
    ],
  }),
  'Changing a memory display name must mark the editor dirty.'
);

console.log('Agent runtime signature checks passed.');
