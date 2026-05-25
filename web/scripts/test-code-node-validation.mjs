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

const { checkValid, CODE_OUTPUT_TYPES } = require('../src/components/workflow/nodes/code/config.ts');

function nodeDataWithOutput(type) {
  return {
    type: 'code',
    title: 'Code',
    desc: '',
    code: 'def main():\n    return {"result": "ok"}',
    code_language: 'python3',
    variables: [],
    outputs: {
      result: { type, children: null },
    },
    outputKeyOrders: ['result'],
    isInLoop: false,
    isInIteration: false,
  };
}

assert.equal(CODE_OUTPUT_TYPES.includes('file'), false);
assert.equal(CODE_OUTPUT_TYPES.includes('array[file]'), false);

for (const type of ['file', 'array[file]']) {
  const result = checkValid(nodeDataWithOutput(type));
  assert.equal(result.isValid, false);
  assert.equal(
    result.errors.some(error => error.code === 'code.errors.unsupportedOutputType'),
    true
  );
}

for (const type of ['string', 'object', 'array[object]']) {
  const result = checkValid(nodeDataWithOutput(type));
  assert.equal(result.isValid, true);
}

console.log('code node validation tests passed');
