import { connectorPureImport } from './connector-pure-fixture.mjs';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import ts from 'typescript';

export const read = file => readFileSync(file, 'utf8');
export function load(file, imports = {}, suffix = '') {
  const code = ts.transpileModule(read(file), {
    compilerOptions: {
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
      jsx: ts.JsxEmit.ReactJSX,
    },
  }).outputText;
  const module = { exports: {} };
  new Function('require', 'module', 'exports', code + suffix)(
    name => {
      if (!Object.hasOwn(imports, name) && connectorPureImport(name))
        return connectorPureImport(name);
      assert.ok(Object.hasOwn(imports, name), `${file}: unexpected import ${name}`);
      return imports[name];
    },
    module,
    module.exports
  );
  return module.exports;
}
export const element = (type, props, key) => ({ type, props: props ?? {}, key });
export const jsx = { jsx: element, jsxs: element, Fragment: 'Fragment' };
export const ui = new Proxy({}, { get: (_, name) => name });
export const flush = async () => {
  for (let i = 0; i < 20; i++) await Promise.resolve();
};
export function nodes(tree) {
  if (!tree || typeof tree !== 'object') return [];
  if (Array.isArray(tree)) return tree.flatMap(nodes);
  return [tree, ...nodes(tree.props?.children)];
}
export function harness(factory) {
  const slots = [];
  let cursor = 0,
    effects = [],
    renderValue;
  const same = (a, b) => a && b && a.length === b.length && a.every((v, i) => Object.is(v, b[i]));
  const react = {
    useRef(initial) {
      const i = cursor++;
      return (slots[i] ??= { current: initial });
    },
    useState(initial) {
      const i = cursor++;
      if (!(i in slots)) slots[i] = typeof initial === 'function' ? initial() : initial;
      return [
        slots[i],
        next => {
          slots[i] = typeof next === 'function' ? next(slots[i]) : next;
        },
      ];
    },
    useMemo(fn, deps) {
      const i = cursor++;
      if (!same(slots[i]?.deps, deps)) slots[i] = { deps, value: fn() };
      return slots[i].value;
    },
    useCallback(fn, deps) {
      return react.useMemo(() => fn, deps);
    },
    useEffect(fn, deps) {
      const i = cursor++;
      if (!same(slots[i]?.deps, deps))
        effects.push(() => {
          slots[i]?.cleanup?.();
          slots[i] = { deps, cleanup: fn() };
        });
    },
  };
  const render = factory(react);
  return {
    render(...args) {
      cursor = 0;
      effects = [];
      renderValue = render(...args);
      effects.forEach(run => run());
      return renderValue;
    },
    get value() {
      return renderValue;
    },
    unmount() {
      for (const slot of slots) slot?.cleanup?.();
    },
  };
}
