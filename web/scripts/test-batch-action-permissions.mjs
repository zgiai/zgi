import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import test from 'node:test';
import ts from 'typescript';

const path = '../src/components/workflow-test/batch-test-overview.tsx';
const source = ts.createSourceFile(path, readFileSync(new URL(path, import.meta.url), 'utf8'), ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
function initializer(name) {
  const matches = [];
  const visit = node => {
    if (ts.isVariableDeclaration(node) && ts.isIdentifier(node.name) && node.name.text === name) matches.push(node.initializer);
    ts.forEachChild(node, visit);
  };
  visit(source);
  assert.equal(matches.length, 1, `Expected one production declaration: ${name}`);
  assert.ok(matches[0]);
  return matches[0].getText(source);
}
const names = ['canUpdateTestAssets', 'canDebugTest', 'canStopTestRun', 'canViewBatchResults', 'canCreateAndRunBatch', 'canRetestBatch'];
const declarations = names.map(name => `const ${name} = ${initializer(name)};`).join('\n');
function access(permissions) {
  return vm.runInNewContext(`${declarations}\n({${names.join(',')}})`, { permissions });
}
test('missing batch permissions fail closed', () => {
  assert.deepEqual(Object.values(access(undefined)), names.map(() => false));
  assert.deepEqual(Object.values(access({})), names.map(() => false));
});
for (let mask = 0; mask < 16; mask++) {
  test(`batch action permission combination ${mask}`, () => {
    const permissions = { canUpdate: !!(mask & 1), canDebug: !!(mask & 2), canStop: !!(mask & 4), canViewLogs: !!(mask & 8) };
    const result = access(permissions);
    assert.equal(result.canCreateAndRunBatch, permissions.canUpdate && permissions.canDebug && permissions.canViewLogs);
    assert.equal(result.canRetestBatch, permissions.canDebug && permissions.canViewLogs);
    assert.equal(result.canStopTestRun, permissions.canStop);
    assert.equal(result.canViewBatchResults, permissions.canViewLogs);
  });
}
for (const canRetestBatch of [false, true]) {
  test(`retest confirmation rechecks current permission: ${canRetestBatch}`, () => {
    let calls = 0;
    vm.runInNewContext(`const confirmRetestBatch = ${initializer('confirmRetestBatch')}; confirmRetestBatch();`, {
      canRetestBatch, retestingBatch: { id: 'fixture', name: 'Fixture' },
      buildRetestName: name => name, setRetestingBatch() {},
      retestBatch: { mutate() { calls++; } },
    });
    assert.equal(calls, Number(canRetestBatch));
  });
}
