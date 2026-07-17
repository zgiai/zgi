import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import Module from 'node:module';
import { URL } from 'node:url';

const require = createRequire(import.meta.url);
const fs = require('node:fs');
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
  buildModelSelectionKey,
  getModelSelectionKey,
  isModelSelectable,
} = require('../src/components/common/model-multi-selector/model-selection.ts');
const {
  prioritizeModelsByUseCase,
} = require('../src/components/common/model-selector/model-selector/utils.ts');

const deepseekChat = {
  provider: 'deepseek',
  model: 'deepseek-chat',
  callable: true,
  is_available: true,
};
const deepseekReasoner = {
  provider: 'deepseek',
  model: 'deepseek-reasoner',
  callable: true,
  is_available: true,
};
const deepseekV4Pro = {
  provider: 'deepseek',
  model: 'deepseek-v4-pro',
  callable: true,
  is_available: true,
};
const qwenSameModelName = {
  provider: 'qwen',
  model: 'deepseek-chat',
  callable: true,
  is_available: true,
};
const openaiUnconfiguredModel = {
  provider: 'openai',
  model: 'gpt-4.1',
  status: 'active',
  callable: false,
  is_available: false,
};

const orderedModels = prioritizeModelsByUseCase(
  [
    { ...deepseekChat, use_cases: ['text-chat'] },
    { ...deepseekV4Pro, use_cases: ['text-chat', 'agent'] },
    { ...deepseekReasoner, use_cases: ['text-chat'] },
  ],
  'agent'
);
assert.deepEqual(
  orderedModels.map(item => item.model),
  ['deepseek-v4-pro', 'deepseek-chat', 'deepseek-reasoner']
);

const catalogModelKeys = new Set(
  [deepseekChat, deepseekReasoner, deepseekV4Pro, qwenSameModelName, openaiUnconfiguredModel].map(
    item => getModelSelectionKey(item)
  )
);

assert.equal(buildModelSelectionKey(' DeepSeek ', ' deepseek-chat '), 'deepseek\tdeepseek-chat');

const directProviderKeys = new Set([buildModelSelectionKey('deepseek', 'deepseek-chat')]);
assert.equal(
  isModelSelectable(deepseekChat, 'catalog', catalogModelKeys, directProviderKeys, null),
  true
);
assert.equal(
  isModelSelectable(deepseekReasoner, 'catalog', catalogModelKeys, directProviderKeys, null),
  false
);

const compatibleModelNames = new Set(['deepseek-chat']);
const emptyModelKeys = new Set();
assert.equal(
  isModelSelectable(
    deepseekChat,
    'catalog',
    catalogModelKeys,
    emptyModelKeys,
    compatibleModelNames
  ),
  true
);
assert.equal(
  isModelSelectable(
    qwenSameModelName,
    'catalog',
    catalogModelKeys,
    emptyModelKeys,
    compatibleModelNames
  ),
  true
);
assert.equal(
  isModelSelectable(
    deepseekV4Pro,
    'catalog',
    catalogModelKeys,
    emptyModelKeys,
    compatibleModelNames
  ),
  false
);

assert.equal(
  isModelSelectable(deepseekChat, 'catalog', catalogModelKeys, emptyModelKeys, null),
  false
);
assert.equal(isModelSelectable(deepseekChat, 'catalog', catalogModelKeys, null, null), true);
assert.equal(
  isModelSelectable(openaiUnconfiguredModel, 'available', catalogModelKeys, null, null),
  false
);
assert.equal(
  isModelSelectable(openaiUnconfiguredModel, 'catalog', catalogModelKeys, null, null),
  true
);

const channelDialogSource = fs.readFileSync(
  new URL('../src/components/channel/channel-dialog.tsx', import.meta.url),
  'utf8'
);
assert.match(channelDialogSource, /selectionPolicy="catalog"/);
assert.doesNotMatch(
  channelDialogSource,
  /selectionPolicy=\{mode === 'create' \? 'catalog' : 'available'\}/
);

const consoleChatSource = fs.readFileSync(
  new URL('../src/app/console/work/chat/page.tsx', import.meta.url),
  'utf8'
);
assert.match(consoleChatSource, /useCase: 'text-chat'/);
assert.match(consoleChatSource, /preferredUseCase: 'agent'/);
assert.match(consoleChatSource, /scope: 'workChat'/);
assert.match(consoleChatSource, /legacyScope: 'consoleChat'/);
assert.match(consoleChatSource, /modelUseCase="text-chat"/);
assert.match(consoleChatSource, /preferredModelUseCase="agent"/);

const contextualAIChatSource = fs.readFileSync(
  new URL('../src/components/aichat/contextual/contextual-ai-chat-dock.tsx', import.meta.url),
  'utf8'
);
assert.match(contextualAIChatSource, /useCase: 'agent'/);
assert.match(contextualAIChatSource, /scope: 'contextualSidebar'/);
assert.match(contextualAIChatSource, /legacyScope: 'consoleChat'/);
assert.match(contextualAIChatSource, /repairUnavailableSelection: true/);
assert.doesNotMatch(contextualAIChatSource, /useCase: 'text-chat'/);
assert.doesNotMatch(contextualAIChatSource, /consoleChat\.modelUnavailable/);
assert.doesNotMatch(contextualAIChatSource, /isSelectedModelUnavailable/);

const agentRuntimeSource = fs.readFileSync(
  new URL(
    '../src/components/agents/agent-runtime/hooks/use-agent-runtime-page-model.tsx',
    import.meta.url
  ),
  'utf8'
);
assert.match(agentRuntimeSource, /useAvailableModels\(\{ use_case: 'agent-runtime' \}\)/);
assert.doesNotMatch(agentRuntimeSource, /useAvailableModels\(\{ use_case: 'text-chat' \}\)/);
assert.doesNotMatch(agentRuntimeSource, /useAvailableModels\(\{ use_case: 'agent' \}\)/);
assert.match(agentRuntimeSource, /isAgentModelRecommended/);

const aiChatToolbarSource = fs.readFileSync(
  new URL('../src/components/chat/variants/aichat/input-toolbar.tsx', import.meta.url),
  'utf8'
);
assert.match(aiChatToolbarSource, /modelType=\{modelUseCase\}/);
assert.match(aiChatToolbarSource, /preferredUseCase=\{preferredModelUseCase\}/);

const persistedAIChatModelSource = fs.readFileSync(
  new URL('../src/hooks/model/use-persisted-ai-chat-model-selection.ts', import.meta.url),
  'utf8'
);
assert.doesNotMatch(persistedAIChatModelSource, /model\.model_name === candidate\.model/);
assert.match(
  persistedAIChatModelSource,
  /isModelAvailable\(legacySaved, availableModels\.models\)/
);

const aiChatSource = fs.readFileSync(
  new URL('../src/components/chat/variants/aichat/aichat-chat.tsx', import.meta.url),
  'utf8'
);
const regenerateStart = aiChatSource.indexOf('const handleRegenerate');
const regenerateEnd = aiChatSource.indexOf('\n  const ', regenerateStart + 1);
assert.notEqual(regenerateStart, -1);
assert.notEqual(regenerateEnd, -1);
assert.match(aiChatSource.slice(regenerateStart, regenerateEnd), /await beforeSend\(\)/);

const agentRuntimeModelSectionSource = fs.readFileSync(
  new URL('../src/components/agents/agent-runtime/sections/model-section.tsx', import.meta.url),
  'utf8'
);
assert.match(agentRuntimeModelSectionSource, /modelType="text-chat"/);
assert.match(agentRuntimeModelSectionSource, /availabilityUseCase="agent-runtime"/);
assert.match(agentRuntimeModelSectionSource, /preferredUseCase="agent"/);
assert.match(agentRuntimeModelSectionSource, /capabilityFilter=\{\{ features_tool_call: true \}\}/);
assert.doesNotMatch(agentRuntimeModelSectionSource, /TabsTrigger/);
assert.match(agentRuntimeModelSectionSource, /modelSelection\.compatibilityWarning/);

const modelSelectorSource = fs.readFileSync(
  new URL('../src/components/common/model-selector/model-selector/index.tsx', import.meta.url),
  'utf8'
);
assert.match(modelSelectorSource, /prioritizeModelsByUseCase\(items, preferredUseCase\)/);
assert.match(modelSelectorSource, /highlightedLabel/);

const modelRowSource = fs.readFileSync(
  new URL(
    '../src/components/common/model-selector/model-selector/components/model-row-item.tsx',
    import.meta.url
  ),
  'utf8'
);
assert.match(modelRowSource, /<Badge/);

const webAppAgentChatSource = fs.readFileSync(
  new URL('../src/components/webapp/agent-chat/index.tsx', import.meta.url),
  'utf8'
);
assert.doesNotMatch(webAppAgentChatSource, /compatibilityWarning/);

console.log('model selector restriction tests passed');
