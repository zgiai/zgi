import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import Module from 'node:module';

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
  buildModelSelectionKey,
  getModelSelectionKey,
  isModelSelectable,
} = require('../src/components/common/model-multi-selector/model-selection.ts');

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

const catalogModelKeys = new Set(
  [deepseekChat, deepseekReasoner, deepseekV4Pro, qwenSameModelName].map(item =>
    getModelSelectionKey(item)
  )
);

assert.equal(
  buildModelSelectionKey(' DeepSeek ', ' deepseek-chat '),
  'deepseek\tdeepseek-chat'
);

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
assert.equal(
  isModelSelectable(deepseekChat, 'catalog', catalogModelKeys, null, null),
  true
);

console.log('model selector restriction tests passed');
