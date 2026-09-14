import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import test from 'node:test';
import ts from 'typescript';

const filename = new URL('../src/components/files/aichat/build-context-items.ts', import.meta.url);
const code = ts.transpileModule(readFileSync(filename, 'utf8'), {
  compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS },
  fileName: filename.pathname,
}).outputText;
const mocks = {
  '@/hooks/use-files': { FILE_FOLDERS_KEY: 'folders', FILES_QUERY_KEY: 'files', STORAGE_USAGE_KEY: 'usage' },
  '@/hooks/query-keys': { FILE_KEYS: { all: ['files'] } },
};
const module = { exports: {} };
vm.runInNewContext(code, {
  module, exports: module.exports,
  require: name => {
    assert.ok(Object.hasOwn(mocks, name), `Unexpected import: ${name}`);
    return mocks[name];
  },
});

test('Files page independently wires preview permission and memo dependency', () => {
  const source = readFileSync(new URL('../src/components/files/file-management-content.tsx', import.meta.url), 'utf8');
  assert.match(source, /const canPreview = hasAnyPermission\(FILE_PERMISSION_ACTIONS\.preview\)/);
  assert.match(source, /buildFilesAIChatContextItems\(\{[\s\S]*?canPreview,/);
  assert.match(source, /\[\s*activeCategory,[\s\S]*?canPreview,[\s\S]*?selectedFiles,/);
});

for (let mask = 0; mask < 8; mask += 1) {
  test(`File context separates list, preview, upload and delete (mask=${mask})`, () => {
    const canPreview = Boolean(mask & 1);
    const canUpload = Boolean(mask & 2);
    const canManage = Boolean(mask & 4);
    const contexts = module.exports.buildFilesAIChatContextItems({
      files: [{ id: 'fixture-file', name: 'Fixture.txt', extension: 'txt' }],
      selectedFileIds: [], currentPage: 1, totalPages: 1, total: 1, pageSize: 20,
      sort: 'created_at', activeCategory: 'all', searchValue: '',
      currentWorkspace: null, isOrganizationMode: false,
      contextReady: true, queryStatus: 'ready', canPreview, canUpload, canManage,
    });
    assert.equal(contexts.length, 2);
    for (const context of contexts) {
      const capabilities = new Map(context.capabilities.map(value => [value.id, value]));
      assert.equal(capabilities.get('file.list_visible').permissions.length, 0);
      for (const [id, permission, allowed] of [
        ['file.read', 'file.preview', canPreview],
        ['file.create', 'file.upload', canUpload],
        ['file.delete', 'file.delete', canManage],
      ]) {
        assert.equal(capabilities.has(id), allowed);
        if (allowed) {
          assert.equal(capabilities.get(id).permissions.join(','), permission);
          assert.equal(capabilities.get(id).status, 'available');
          if (id !== 'file.read') assert.equal(capabilities.get(id).requiresConfirmation, true);
        }
      }
    }
    assert.equal(contexts[1].description.includes('Use read_file'), canPreview);
  });
}
