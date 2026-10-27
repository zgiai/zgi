import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import ts from 'typescript';

function load(file) {
  const compiled = ts.transpileModule(readFileSync(path.join(process.cwd(), file), 'utf8'), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText;
  const module = { exports: {} };
  new Function('require', 'module', 'exports', compiled)(
    name => {
      throw new Error(`unexpected runtime import ${name}`);
    },
    module,
    module.exports
  );
  return module.exports;
}

const source = readFileSync(
  path.join(process.cwd(), 'src/components/workflow/hooks/use-combined-workflow-save.ts'),
  'utf8'
);
const leaveGuard = readFileSync(
  path.join(process.cwd(), 'src/components/workflow/hooks/use-workflow-leave-guard.tsx'),
  'utf8'
);

assert.match(source, /useRef<Promise<boolean> \| null>\(null\)/);
assert.match(
  source,
  /const activeSave = inFlightSaveRef\.current;[\s\S]*await activeSave[\s\S]*continue;/,
  'a publish-time save must await the exact in-flight draft save promise'
);
assert.match(source, /inFlightSaveRef\.current = savePromise;[\s\S]*await savePromise/);
assert.match(
  source,
  /if \(!stateAfterFlush\.isDirty && !stateAfterFlush\.hasLayoutChanges\)[\s\S]*return true/,
  'a clean draft must be reported as safely saved'
);
assert.match(
  source,
  /catch \(error\)[\s\S]*return false/,
  'draft persistence failures must propagate as an explicit false result'
);
assert.match(
  source,
  /const saved = await handleCombinedSave\(\{ silent: true \}\);[\s\S]*if \(!saved\)[\s\S]*return;[\s\S]*publishWorkflowMutation\.mutateAsync/,
  'publishing must stop before the publish mutation when draft saving fails'
);
assert.match(source, /current\.graphVersion !== graphVersion/);
assert.match(
  source,
  /const MAX_SAVE_PASSES = \d+;[\s\S]*if \(savePasses >= MAX_SAVE_PASSES\)[\s\S]*savePasses \+= 1;/,
  'the save loop must be bounded so reference churn cannot save forever'
);
assert.match(
  source,
  /function workflowMetadataKey[\s\S]*connector_reference_commands: \(data\?\.connector_reference_commands \?\? \[\]\)\.filter/,
  'accepted connector commands cleared after save must not count as a new edit'
);
assert.match(source, /current\.nodes !== nodes/);
assert.match(source, /currentMetadata !== submittedMetadata/);
assert.match(
  leaveGuard,
  /const saved = await onSave\(\{ silent: true \}\);[\s\S]*if \(saved === false\) return;/,
  'save-and-leave must stay in the editor when draft persistence reports failure'
);

// Leave guard must not dead-lock while a save is in flight.
const dialogFooter = leaveGuard.slice(
  leaveGuard.indexOf('<DialogFooter'),
  leaveGuard.indexOf('</DialogFooter>')
);
assert.ok(dialogFooter.length > 0, 'leave guard dialog footer must exist');
assert.equal(
  (dialogFooter.match(/disabled=\{isSaving\}/g) ?? []).length,
  1,
  'only save-and-leave may be disabled while saving'
);
assert.match(
  dialogFooter,
  /<Button disabled=\{isSaving\} onClick=\{onSaveAndLeave\}>/,
  'save-and-leave is the action disabled while saving'
);
assert.doesNotMatch(leaveGuard, /showCloseButton=\{!isSaving\}/, 'close button stays visible');
assert.match(
  leaveGuard,
  /const closeGuard = React\.useCallback\(\(\) => \{\s*setPending\(null\);\s*\}/,
  'closing the guard (Esc / overlay / close) must work while saving'
);
assert.match(leaveGuard, /'workflow\.leaveGuardWorkflow'/, 'workflow editors use workflow copy');

// Unchanged saves must keep node array identity so the save loop settles.
const { mergeSavedConnectorNodes } = load(
  'src/components/workflow/store/helpers/merge-saved-connectors.ts'
);
const plainNodes = [
  { id: 'start', position: { x: 180, y: 280 }, data: { type: 'start' } },
  { id: 'end', position: { x: 530, y: 280 }, data: { type: 'end' } },
];
assert.equal(
  mergeSavedConnectorNodes(plainNodes, plainNodes, structuredClone(plainNodes)),
  plainNodes,
  'merge must return the current array when no node changed'
);
const connectorNode = {
  id: 'tool',
  position: { x: 0, y: 0 },
  data: {
    provider_type: 'connector',
    provider_id: 'github',
    tool_name: 'get_user',
    connector: { connector_id: 'github', connection_id: 'c1' },
  },
};
const sameConnector = [connectorNode];
assert.equal(
  mergeSavedConnectorNodes(sameConnector, sameConnector, structuredClone(sameConnector)),
  sameConnector,
  'merge must return the current array when canonical connector refs are identical'
);
const canonical = [
  {
    ...connectorNode,
    data: {
      ...connectorNode.data,
      connector: { ...connectorNode.data.connector, binding_id: 'b1' },
    },
  },
];
const merged = mergeSavedConnectorNodes(sameConnector, sameConnector, canonical);
assert.notEqual(merged, sameConnector, 'server-assigned refs produce a new array');
assert.equal(merged[0].data.connector.binding_id, 'b1');
assert.deepEqual(merged[0].position, connectorNode.position, 'merge never moves nodes');

const workflowHook = readFileSync(
  path.join(process.cwd(), 'src/hooks/workflow/use-workflow.ts'),
  'utf8'
);
assert.match(
  workflowHook,
  /const nodesChanged = nodes !== currentState\.nodes;[\s\S]*\.\.\.\(nodesChanged \? \{ nodes \} : \{\}\)/,
  'draft save success must not replace nodes when the merge changed nothing'
);

console.log('Workflow save/publish barrier checks passed.');
