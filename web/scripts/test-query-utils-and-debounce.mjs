import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import Module from 'node:module';
import path from 'node:path';
import { setTimeout as wait } from 'node:timers/promises';
import ts from 'typescript';

function loadModule(relativePath) {
  const filename = path.resolve(relativePath);
  const output = ts.transpileModule(readFileSync(filename, 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
    fileName: filename,
  }).outputText;
  const loaded = new Module(filename);
  loaded.filename = filename;
  loaded.paths = Module._nodeModulePaths(path.dirname(filename));
  loaded._compile(output, filename);
  return loaded.exports;
}

const { workspaceInvalidatePredicate, reloadInfiniteQuery, infiniteQueryUtils } = loadModule(
  'src/hooks/query-utils.ts'
);
const matches = workspaceInvalidatePredicate('agents', 'workspace-a');
for (const [queryKey, expected] of [
  [['agents', { workspace_id: 'workspace-a' }], true],
  [['agents', { workspaceId: 'workspace-a' }], true],
  [['agents', { workspace_id: 'workspace-b' }], false],
  [['agents', 'detail', { workspace_id: 'workspace-a' }], false],
  [['datasets', { workspace_id: 'workspace-a' }], false],
  [['agents', null], true],
  [['agents', {}], true],
]) {
  assert.equal(matches({ queryKey }), expected, JSON.stringify(queryKey));
}
const key = ['agents'];
const calls = [];
await reloadInfiniteQuery(
  {
    removeQueries: options => calls.push(['remove', options]),
    refetchQueries: async options => calls.push(['refetch', options]),
  },
  key
);
assert.deepEqual(calls, [
  ['remove', { queryKey: key }],
  ['refetch', { queryKey: key }],
]);
for (const startIndex of [1, -1]) {
  const cached = { pages: [{ id: 1 }, { id: 2 }], pageParams: [null, 'cursor'] };
  let stored;
  let fetches = 0;
  await infiniteQueryUtils.refetchFromPage(
    {
      getQueryData: actual => {
        assert.equal(actual, key);
        return cached;
      },
      setQueryData: (actual, data) => {
        assert.equal(actual, key);
        stored = data;
      },
    },
    key,
    startIndex,
    async () => {
      fetches++;
    }
  );
  const count = Math.max(0, startIndex);
  assert.deepEqual(stored, {
    pages: cached.pages.slice(0, count),
    pageParams: cached.pageParams.slice(0, count),
  });
  assert.equal(cached.pages.length, 2, 'trimming must not mutate the previous cache');
  assert.equal(fetches, 1);
}
let missingCacheFetches = 0;
await infiniteQueryUtils.refetchFromPage(
  {
    getQueryData: () => undefined,
    setQueryData: () => assert.fail('missing cache must not be overwritten'),
  },
  key,
  0,
  async () => {
    missingCacheFetches++;
  }
);
assert.equal(missingCacheFetches, 1);

const { debounce } = loadModule('src/lib/utils.ts');
const received = [];
const context = { name: 'latest' };
const debounced = debounce(function (...args) {
  received.push([this, args]);
}, 5);
assert.equal(debounced.call({ name: 'earlier' }, 1), undefined);
debounced.call(context, 2, 'last');
await wait(30);
assert.deepEqual(received, [[context, [2, 'last']]]);
debounced.call(context, 3);
await wait(30);
assert.deepEqual(received, [
  [context, [2, 'last']],
  [context, [3]],
]);
console.log('Query utilities and debounce regression checks passed.');
