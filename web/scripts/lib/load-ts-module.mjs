import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import ts from 'typescript';

// Loads a TypeScript module from src/ together with the local modules it
// imports, so a test can run reducers and selectors as they ship. Each file is
// transpiled on its own (types are erased, nothing is type-checked) into
// node_modules/.cache, where package imports still resolve.

const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../..');
const srcRoot = path.join(webRoot, 'src');
const cacheRoot = path.join(webRoot, 'node_modules', '.cache', 'ts-test-modules');
const extensions = ['.ts', '.tsx', '/index.ts', '/index.tsx'];

function resolveLocal(specifier, fromFile) {
  let base;
  if (specifier.startsWith('@/')) base = path.join(srcRoot, specifier.slice(2));
  else if (specifier.startsWith('.')) base = path.resolve(path.dirname(fromFile), specifier);
  else return null;
  if (/\.(ts|tsx|js|mjs)$/.test(base) && existsSync(base)) return base;
  base = base.replace(/\.js$/, '');
  for (const extension of extensions) {
    if (existsSync(base + extension)) return base + extension;
  }
  throw new Error(`cannot resolve ${specifier} from ${fromFile}`);
}

function needsPackageExtension(specifier) {
  const parts = specifier.split('/');
  const packageName = specifier.startsWith('@') ? parts.slice(0, 2).join('/') : parts[0];
  if (packageName === specifier) return false;
  const packageDir = path.join(webRoot, 'node_modules', packageName);
  const manifest = path.join(packageDir, 'package.json');
  if (!existsSync(manifest) || JSON.parse(readFileSync(manifest, 'utf8')).exports) return false;
  return existsSync(path.join(webRoot, 'node_modules', `${specifier}.js`));
}

function outputPath(file) {
  const relative = path.relative(srcRoot, file).replace(/\.(ts|tsx|js)$/, '.mjs');
  return path.join(cacheRoot, relative);
}

function transpile(file, done) {
  if (done.has(file)) return;
  done.add(file);
  const source = readFileSync(file, 'utf8');
  const output = ts.transpileModule(source, {
    fileName: file,
    compilerOptions: {
      module: ts.ModuleKind.ESNext,
      target: ts.ScriptTarget.ES2022,
      jsx: ts.JsxEmit.ReactJSX,
      verbatimModuleSyntax: false,
    },
  }).outputText;
  const target = outputPath(file);
  const rewritten = output.replace(
    /(\bfrom\s*|\bimport\s*\(\s*|\bimport\s+)(['"])([^'"]+)\2/g,
    (match, prefix, quote, specifier) => {
      // Stylesheets mean nothing outside the bundler.
      if (/\.(css|scss)$/.test(specifier)) {
        return `${prefix}${quote}data:text/javascript,export default {}${quote}`;
      }
      const local = resolveLocal(specifier, file);
      if (!local) {
        // Package subpaths the bundler finds without an exports map, such as
        // next/navigation, need their file extension under node.
        return needsPackageExtension(specifier)
          ? `${prefix}${quote}${specifier}.js${quote}`
          : match;
      }
      transpile(local, done);
      let relative = path.relative(path.dirname(target), outputPath(local)).replace(/\\/g, '/');
      if (!relative.startsWith('.')) relative = `./${relative}`;
      return `${prefix}${quote}${relative}${quote}`;
    }
  );
  mkdirSync(path.dirname(target), { recursive: true });
  const previous = existsSync(target) ? readFileSync(target, 'utf8') : null;
  if (previous !== rewritten) writeFileSync(target, rewritten);
}

/** Imports src/<relativePath> with its local imports. */
export async function loadTsModule(relativePath) {
  const file = path.join(srcRoot, relativePath);
  transpile(file, new Set());
  const url = pathToFileURL(outputPath(file));
  url.search = createHash('sha1')
    .update(readFileSync(outputPath(file)))
    .digest('hex')
    .slice(0, 8);
  return import(url.href);
}
