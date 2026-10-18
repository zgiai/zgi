import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import Module from 'node:module';
import path from 'node:path';
import vm from 'node:vm';
import ts from 'typescript';

// Exercise the actual generator's serializer without writing generated files.
const filename = path.resolve('scripts/generate-sensitive-word-filter.mjs');
const source = readFileSync(filename, 'utf8');
const parsed = ts.createSourceFile(
  filename,
  source,
  ts.ScriptTarget.Latest,
  true,
  ts.ScriptKind.JS
);
const writer = parsed.statements.find(
  statement => ts.isFunctionDeclaration(statement) && statement.name?.text === 'writeAutomaton'
);
assert.ok(writer, 'the generator must expose its serialization function');
const data = {
  transitions: [{ "a'b\\c\n\r\u2028\u2029": 1 }, {}],
  failures: [0, 0],
  outputs: [0, 1],
  wordCount: 1,
  maxWordLength: 9,
};
let generated;
vm.runInNewContext(`${writer.getText(parsed)}\nwriteAutomaton(data);`, {
  data,
  outputFile: 'automaton.ts',
  dirname: path.dirname,
  mkdirSync: () => {},
  writeFileSync: (_filename, content) => {
    generated = content;
  },
});
assert.match(generated, /const automatonJson = '/);
const output = ts.transpileModule(generated, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
}).outputText;
const loaded = new Module('automaton.ts');
loaded._compile(output, 'automaton.ts');
assert.deepEqual(loaded.exports.sensitiveWordAutomatonData, data);
console.log('Sensitive-word generator escaping and round-trip checks passed.');
