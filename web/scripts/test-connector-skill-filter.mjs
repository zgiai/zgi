import assert from 'node:assert/strict';
import fs from 'node:fs';
import vm from 'node:vm';
import ts from 'typescript';

const read = file => fs.readFileSync(`src/components/dashboard/organization/${file}`, 'utf8');
const filters = ts.createSourceFile('filters.tsx', read('aichat-skill-catalog-filters.tsx'), ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
let connectorOption;
function visit(node) {
  if (ts.isJsxElement(node) && node.openingElement.tagName.getText(filters) === 'SelectItem' && node.getText(filters).includes('dependency.external')) {
    connectorOption = node.openingElement.attributes.properties.find(attribute => attribute.name?.getText(filters) === 'value')?.initializer?.text;
  }
  ts.forEachChild(node, visit);
}
visit(filters);
const settings = ts.createSourceFile('settings.tsx', read('aichat-skill-settings-section.tsx'), ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
const filter = settings.statements.find(node => ts.isFunctionDeclaration(node) && node.name?.text === 'filterSkills');
assert.ok(filter, 'use the production filtering function');
const context = { isInvalidSkill: () => false };
vm.createContext(context);
vm.runInContext(ts.transpileModule(filter.getText(settings), { compilerOptions: { target: ts.ScriptTarget.ES2022 } }).outputText, context);
const skills = [{ skill_id: 'native-search' }, { skill_id: 'ordinary' }];
const displays = { 'native-search': { dependencyKind: 'connector' }, ordinary: { dependencyKind: 'standalone' } };
const apply = dependency => Array.from(context.filterSkills(skills, displays, [], '', 'all', 'all', 'all', 'all', dependency), skill => skill.skill_id);
assert.deepEqual(apply(connectorOption), ['native-search'], 'selecting the Connector option keeps native Connector skills');
assert.deepEqual(apply('standalone'), ['ordinary']);
assert.deepEqual(apply('all'), ['native-search', 'ordinary']);
console.log('Connector skill dependency filter passed.');
