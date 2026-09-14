import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import vm from 'node:vm';
import test from 'node:test';
import ts from 'typescript';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';

const require = createRequire(import.meta.url);
const filename = new URL('../src/components/files/file-sidebar.tsx', import.meta.url);
const code = ts.transpileModule(readFileSync(filename, 'utf8'), {
  compilerOptions: {
    target: ts.ScriptTarget.ES2022,
    module: ts.ModuleKind.CommonJS,
    jsx: ts.JsxEmit.ReactJSX,
    esModuleInterop: true,
  },
  fileName: filename.pathname,
}).outputText;

for (const permitted of [false, true]) {
  test(`file sidebar text creation callback is permission gated (${permitted})`, () => {
    const buttons = [];
    let clicks = 0;
    const callback = () => { clicks += 1; };
    const inert = () => null;
    const mocks = {
      '@/i18n': { useT: () => key => key },
      '@/components/ui/button': {
        Button: props => {
          buttons.push(props);
          return React.createElement('button', null, props.children);
        },
      },
      '@/components/ui/progress': { Progress: inert },
      '@/components/ui/skeleton': { Skeleton: inert },
      '@/lib/utils': { cn: (...args) => args.filter(Boolean).join(' ') },
      '@/hooks/use-files': {
        useStorageUsage: () => ({ used: 0, total: 1, isLoading: false }),
        useFileFolders: () => ({ folders: [], isLoading: false }),
      },
      '@/services/file-manage.service': { fileManageService: {} },
      './folder-tree-node': { FolderTreeNode: inert },
      './file-folder-levels': { MAX_FILE_FOLDER_TREE_LEVEL: 5 },
    };
    const module = { exports: {} };
    vm.runInNewContext(code, {
      module,
      exports: module.exports,
      require: name => Object.hasOwn(mocks, name) ? mocks[name] : require(name),
      console,
    });
    const html = renderToStaticMarkup(React.createElement(module.exports.FileSidebar, {
      onCreateTextFile: permitted ? callback : undefined,
    }));
    assert.equal(html.includes('files.sidebar.newTextFile'), permitted);
    assert.equal(html.includes('files.sidebar.uploadFile'), false, 'text creation must not grant upload');
    const action = buttons.find(button => button.onClick === callback);
    assert.equal(Boolean(action), permitted);
    if (action) action.onClick();
    assert.equal(clicks, permitted ? 1 : 0);
  });
}
