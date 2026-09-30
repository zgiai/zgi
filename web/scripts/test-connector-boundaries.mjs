import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import ts from 'typescript';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const source = path.join(root, 'src');
const failures = [];
const forbidden = /(?:components|hooks|i18n\/modules)\/integrations(?:\/|$)|(?:services\/(?:types\/)?integration(?:[.-]|$))|integration-oauth-proxy|legacy-(?:connected-apps|integration)-dialog|external-app-display|external-action-operation-status/;
for (const retired of ['external-app-display.ts', 'external-action-operation-status.ts']) {
  assert.equal(fs.existsSync(path.join(source, 'components/chat/variants/aichat', retired)), false, `retired display parser: ${retired}`);
}
for (const retired of ['components/integrations', 'hooks/integrations', 'i18n/modules/integrations', 'app/console/integrations', 'app/dashboard/organization/integrations', 'app/console/api/integrations']) {
  const directory = path.join(source, retired);
  if (fs.existsSync(directory) && fs.readdirSync(directory, { recursive: true, withFileTypes: true }).some(entry => !entry.isDirectory())) failures.push(`retired source: ${retired}`);
}
function inspect(directory) {
  for (const entry of fs.readdirSync(directory, { withFileTypes: true })) {
    const file = path.join(directory, entry.name);
    if (entry.isDirectory()) { inspect(file); continue; }
    if (!/\.(?:ts|tsx|js|jsx)$/.test(file)) continue;
    const tree = ts.createSourceFile(file, fs.readFileSync(file, 'utf8'), ts.ScriptTarget.Latest, true);
    function visit(node) {
      let specifier;
      if ((ts.isImportDeclaration(node) || ts.isExportDeclaration(node)) && node.moduleSpecifier) specifier = node.moduleSpecifier;
      if (ts.isCallExpression(node) && (node.expression.kind === ts.SyntaxKind.ImportKeyword || (ts.isIdentifier(node.expression) && node.expression.text === 'require'))) specifier = node.arguments[0];
      if (specifier && ts.isStringLiteralLike(specifier)) {
        const resolved = specifier.text.startsWith('.') ? path.relative(source, path.resolve(path.dirname(file), specifier.text)).replaceAll('\\', '/') : specifier.text;
        if (forbidden.test(resolved)) failures.push(`${path.relative(root,file)}: ${specifier.text}`);
      }
      if (ts.isStringLiteralLike(node) && /^\/(?:console\/(?:api\/)?|dashboard\/organization\/)?integrations(?:\/|$)/.test(node.text)) failures.push(`${path.relative(root,file)}: retired API ${node.text}`);
      ts.forEachChild(node, visit);
    }
    visit(tree);
  }
}
inspect(source);
assert.deepEqual(failures, [], `Connector boundary violations:\n${failures.join('\n')}`);
console.log('Connector frontend dependency boundary passed.');
