import { URL } from 'node:url';
import { readFileSync } from 'node:fs';
import ts from 'typescript';

// Shared real, side-effect-free modules; hooks and UI still require explicit mocks.
const pure = {
  'connector-presentation': 'connector-presentation.ts',
  'mail-display-model': 'mail-display-model.ts',
  'mail-connection-name': 'mail-connection-name.ts',
  'personal-connections-model': 'connections/personal-connections-model.ts',
  'mail-picker-groups': 'picker/mail-picker-groups.ts',
};
const key = name => name.split('/').at(-1).replace(/\.ts$/, '');
const read = name =>
  readFileSync(new URL('../src/components/connectors/' + pure[key(name)], import.meta.url), 'utf8');
const modules = new Map();
export function connectorPureImport(name) {
  const id = key(name);
  if (!pure[id]) return undefined;
  if (!modules.has(id)) {
    const module = { exports: {} };
    const code = ts.transpileModule(read(id), {
      compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
    }).outputText;
    new Function('require', 'exports', 'module', code)(
      dependency => {
        const value = connectorPureImport(dependency);
        if (!value) throw new Error('Unexpected pure model dependency: ' + dependency);
        return value;
      },
      module.exports,
      module
    );
    modules.set(id, module.exports);
  }
  return modules.get(id);
}
export const rewriteConnectorPureImports = source =>
  source.replace(/from '([^']+)'/g, (original, name) => {
    if (!pure[key(name)]) return original;
    const code = ts.transpileModule(read(name), {
      compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
    }).outputText;
    return `from 'data:text/javascript;base64,${Buffer.from(rewriteConnectorPureImports(code)).toString('base64')}'`;
  });
