import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import ts from 'typescript';

const root = process.cwd();
const read = relativePath => readFileSync(path.join(root, relativePath), 'utf8');
const transpile = relativePath =>
  ts.transpileModule(read(relativePath), {
    compilerOptions: {
      module: ts.ModuleKind.ESNext,
      target: ts.ScriptTarget.ES2022,
    },
  }).outputText;

const databaseBindingModuleURL = `data:text/javascript;base64,${Buffer.from(
  transpile('src/components/agents/agent-runtime/database-binding-draft.ts')
).toString('base64')}`;
const rebaseOutput = transpile(
  'src/components/agents/agent-runtime/binding-rebase-merge.ts'
).replace("from './database-binding-draft';", `from '${databaseBindingModuleURL}';`);
const rebaseModuleURL = `data:text/javascript;base64,${Buffer.from(rebaseOutput).toString(
  'base64'
)}`;
const {
  getAgentConnectorBindingConflicts,
  mergeSupersededAgentRuntimePayload,
  normalizeAgentConnectorBindings,
} = await import(rebaseModuleURL);

const connector = (connectorId, connectionId) => ({
  connector_id: connectorId,
  connection_id: connectionId,
});
const basePayload = {
  system_prompt: '',
  model_provider: 'provider',
  model: 'model',
  model_parameters: {},
  enabled_skill_ids: [],
  use_memory: false,
  file_upload_enabled: false,
  home_title: 'Agent',
  opening_statement: '',
  input_placeholder: '',
  theme_color: 'default',
  suggested_questions: [],
  knowledge_dataset_ids: [],
  database_bindings: [],
  workflow_bindings: [],
  binding_revision: 'revision-submitted',
};

{
  const submittedCommand = { reference_id: 'unavailable', expected_revision: 2, action: 'detach' };
  const replacement = { ...submittedCommand, action: 'replace', connection_id: 'new-choice' };
  const another = { reference_id: 'another', expected_revision: 1, action: 'detach' };
  const submitted = { ...basePayload, connector_reference_commands: [submittedCommand] };
  const merged = mergeSupersededAgentRuntimePayload(
    submitted,
    { ...submitted, connector_reference_commands: [replacement, another] },
    basePayload
  );
  assert.deepEqual(
    merged.connector_reference_commands,
    [replacement, another],
    'save completion preserves repair commands changed in flight'
  );
  assert.deepEqual(
    mergeSupersededAgentRuntimePayload(submitted, submitted, basePayload)
      .connector_reference_commands,
    [],
    'acknowledged commands are not replayed'
  );
}

assert.deepEqual(
  normalizeAgentConnectorBindings([
    {
      connector_id: ' GitHub ',
      connection_id: ' CONNECTION-A ',
      allowed_action_ids: ['must-not-survive'],
      access_mode: 'write',
    },
    connector('github', 'connection-b'),
    connector('', 'missing-connector'),
  ]),
  [connector('github', 'connection-b')],
  'new Agent Connector bindings contain only connector_id + connection_id and dedupe by Connector'
);

assert.deepEqual(
  normalizeAgentConnectorBindings([
    { ...connector('github', 'connection-direct'), authorization_mode: 'direct', revision: 3 },
  ]),
  [
    {
      ...connector('github', 'connection-direct'),
      authorization_mode: 'direct',
      revision: 3,
    },
  ],
  'Agent authorization mode survives normalization and remains bound to the selected connection'
);

assert.deepEqual(
  normalizeAgentConnectorBindings([
    connector('github', 'connection-current'),
    connector('github', 'connection-confirmed-replacement'),
  ]),
  [connector('github', 'connection-confirmed-replacement')],
  'an Agent has one selected connection per provider and an explicit replacement wins'
);

{
  const submitted = {
    ...basePayload,
    connector_bindings: [connector('github', 'connection-a')],
  };
  const current = {
    ...submitted,
    connector_bindings: [
      connector('github', 'connection-local'),
      connector('exa', 'connection-exa'),
    ],
  };
  const saved = {
    ...submitted,
    connector_bindings: [connector('github', 'connection-server')],
    binding_revision: 'revision-saved',
  };

  const merged = mergeSupersededAgentRuntimePayload(submitted, current, saved);
  assert.deepEqual(merged.connector_bindings, [
    connector('exa', 'connection-exa'),
    connector('github', 'connection-server'),
  ]);
  assert.equal(merged.binding_revision, 'revision-saved');
}

const types = read('src/services/types/agent.ts');
const pageModel = read(
  'src/components/agents/agent-runtime/hooks/use-agent-runtime-page-model.tsx'
);
const dialog = read('src/components/agents/agent-runtime/connector-dialog.tsx');
const dialogs = read('src/components/agents/agent-runtime/dialogs.tsx');
const bindingHealth = read('src/components/agents/agent-runtime/binding-health.tsx');
const signature = read('src/components/agents/agent-runtime/utils.ts');
const resourceSection = read('src/components/agents/agent-runtime/resource-section.tsx');
const integrationSection = read(
  'src/components/agents/agent-runtime/sections/connector-section.tsx'
);
const orchestration = read('src/components/agents/agent-runtime/orchestration-panel.tsx');
const sourceAST = source =>
  ts.createSourceFile('component.tsx', source, ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX);
const findNode = (node, predicate) => {
  if (predicate(node)) return node;
  return ts.forEachChild(node, child => findNode(child, predicate));
};
const compileExpression = (expression, dependencies) => {
  const code = ts.transpileModule(`const callback = ${expression};`, {
    compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.ESNext },
  }).outputText;
  return new Function(...Object.keys(dependencies), `${code}\nreturn callback;`)(
    ...Object.values(dependencies)
  );
};
const resourceAST = sourceAST(resourceSection);
const clickAttribute = findNode(
  resourceAST,
  node =>
    ts.isJsxAttribute(node) &&
    node.name.getText(resourceAST) === 'onClick' &&
    node.getText(resourceAST).includes('onAdd(')
);
assert.ok(
  clickAttribute?.initializer?.expression,
  'the real add button must expose its click callback'
);
const anchor = { tagName: 'BUTTON', isConnected: true };
let clickedAnchor;
let propagationStopped = false;
const click = compileExpression(clickAttribute.initializer.expression.getText(resourceAST), {
  disabled: false,
  onAdd: value => {
    clickedAnchor = value;
  },
});
click({
  currentTarget: anchor,
  stopPropagation: () => {
    propagationStopped = true;
  },
});
assert.equal(clickedAnchor, anchor, 'the true add button element must reach the Agent menu host');
assert.equal(propagationStopped, true);
const disabledClick = compileExpression(
  clickAttribute.initializer.expression.getText(resourceAST),
  {
    disabled: true,
    onAdd: () => {
      throw new Error('read-only add must not open a menu');
    },
  }
);
disabledClick({ currentTarget: anchor, stopPropagation: () => {} });

const pageAST = sourceAST(pageModel);
const openProperty = findNode(
  pageAST,
  node => ts.isPropertyAssignment(node) && node.name.getText(pageAST) === 'onOpenConnectorDialog'
);
assert.ok(openProperty);
const state = { anchor: null, open: false };
const openDependencies = {
  isRuntimeConfigReadOnly: false,
  setConnectorDialogAnchorElement: value => {
    state.anchor = value;
  },
  setConnectorDialogOpen: value => {
    state.open = typeof value === 'function' ? value(state.open) : value;
  },
};
const openMenu = compileExpression(openProperty.initializer.getText(pageAST), openDependencies);
openMenu(anchor);
assert.equal(state.anchor, anchor);
assert.equal(state.open, true);
openMenu(anchor);
assert.equal(state.open, false, 'clicking the actual trigger again closes the menu');
assert.equal(state.anchor, anchor, 'closing the menu preserves its real trigger');
openMenu(anchor);
assert.equal(state.open, true);
state.open = false;
const readOnlyOpen = compileExpression(openProperty.initializer.getText(pageAST), {
  ...openDependencies,
  isRuntimeConfigReadOnly: true,
});
readOnlyOpen({ tagName: 'BUTTON', isConnected: true });
assert.equal(state.anchor, anchor, 'read-only attempts cannot replace the stored trigger');
assert.equal(state.open, false);
openMenu({ tagName: 'BUTTON', isConnected: false });
assert.equal(state.anchor, anchor, 'a detached trigger must not replace the live menu anchor');
assert.equal(state.open, false);

const supersededProperty = findNode(
  pageAST,
  node => ts.isPropertyAssignment(node) && node.name.getText(pageAST) === 'onSaveSuperseded'
);
assert.ok(supersededProperty);
const warnings = [];
const onSuperseded = compileExpression(supersededProperty.initializer.getText(pageAST), {
  lastServerPayloadRef: { current: basePayload },
  mergeSupersededAgentRuntimePayload,
  getAgentConnectorBindingConflicts,
  applyServerBindingPayload: () => {},
  setConnectorDialogOpen: openDependencies.setConnectorDialogOpen,
  toast: { warning: message => warnings.push(message) },
  t: key => key,
});
onSuperseded(
  { savedPayload: { ...basePayload, connector_bindings: [connector('github', 'server')] } },
  { ...basePayload, connector_bindings: [connector('github', 'original')] },
  { ...basePayload, connector_bindings: [connector('github', 'pending')] }
);
assert.equal(state.open, true, 'a CAS conflict still requests the picker to reopen');
assert.equal(state.anchor, anchor, 'CAS reopening reuses the actual trigger after closing');
assert.deepEqual(warnings, ['toasts.connectorBindingRevisionChanged']);

assert.match(resourceSection, /onAdd\(event\.currentTarget\)/);
assert.match(integrationSection, /onOpenDialog: \(anchorElement: HTMLElement\) => void/);
assert.match(integrationSection, /onAdd=\{onOpenDialog\}/);
assert.match(orchestration, /onOpenConnectorDialog: \(anchorElement: HTMLElement\) => void/);
assert.match(orchestration, /onOpenDialog=\{onOpenConnectorDialog\}/);
assert.match(pageModel, /connectorMenuOpen: connectorDialogOpen/);
assert.match(orchestration, /menuOpen=\{connectorMenuOpen\}/);
assert.match(integrationSection, /addMenuOpen=\{menuOpen\}/);
assert.match(resourceSection, /menuOpen=\{addMenuOpen\}/);
for (const menuOpen of [undefined, false, true]) {
  const popup = findNode(
    resourceAST,
    node => ts.isJsxAttribute(node) && node.name.getText(resourceAST) === 'aria-haspopup'
  );
  const expanded = findNode(
    resourceAST,
    node => ts.isJsxAttribute(node) && node.name.getText(resourceAST) === 'aria-expanded'
  );
  assert.ok(popup?.initializer?.expression);
  assert.ok(expanded?.initializer?.expression);
  assert.equal(
    compileExpression(popup.initializer.expression.getText(resourceAST), { menuOpen }),
    menuOpen === undefined ? undefined : 'dialog'
  );
  assert.equal(
    compileExpression(expanded.initializer.expression.getText(resourceAST), { menuOpen }),
    menuOpen
  );
}
assert.match(pageModel, /anchorElement: connectorDialogAnchorElement/);
assert.match(dialog, /anchorElement: HTMLElement \| null/);
assert.match(dialog, /anchorElement=\{anchorElement\}/);
assert.doesNotMatch(`${resourceSection}\n${pageModel}\n${dialog}`, /document\.activeElement/);

const dialogAST = sourceAST(dialog);
const closeAutoFocusAttribute = findNode(
  dialogAST,
  node => ts.isJsxAttribute(node) && node.name.getText(dialogAST) === 'onCloseAutoFocus'
);
assert.ok(
  closeAutoFocusAttribute?.initializer?.expression,
  'the triggerless setup dialog must handle final-close focus explicitly'
);
assert.match(dialog, /pickerFocusRef\.current = \{ open, anchorElement \}/);
for (const closeReason of ['dismiss', 'back', 'ready', 'missing-anchor', 'detached-anchor']) {
  const focusCalls = [];
  const trigger =
    closeReason === 'missing-anchor'
      ? null
      : {
          isConnected: closeReason !== 'detached-anchor',
          focus: options => focusCalls.push(options),
        };
  const pickerFocusRef = { current: { open: true, anchorElement: trigger } };
  const closeAutoFocus = compileExpression(
    closeAutoFocusAttribute.initializer.expression.getText(dialogAST),
    { pickerFocusRef, open: true, anchorElement: trigger }
  );
  // Radix retains the closing dialog callback, so the latest host state must win
  // over the old open=true render captured when the setup dialog was mounted.
  pickerFocusRef.current = {
    open: closeReason === 'back' || closeReason === 'ready',
    anchorElement: trigger,
  };
  let prevented = false;
  closeAutoFocus({
    preventDefault: () => {
      prevented = true;
    },
  });
  assert.equal(prevented, true, 'Radix must not focus its absent DialogTrigger');
  assert.equal(focusCalls.length, closeReason === 'dismiss' ? 1 : 0, closeReason);
  if (closeReason === 'dismiss') assert.deepEqual(focusCalls[0], { preventScroll: true });
}
const manageAttribute = findNode(
  dialogAST,
  node => ts.isJsxAttribute(node) && node.name.getText(dialogAST) === 'onRequestManageConnector'
);
assert.equal(manageAttribute, undefined, 'Agent picker only manages its own draft, not other targets');

const connectorTypeStart = types.indexOf('export interface AgentConnectorBinding');
const connectorTypeEnd = types.indexOf('\n}', connectorTypeStart);
assert.notEqual(connectorTypeStart, -1);
const connectorType = types.slice(connectorTypeStart, connectorTypeEnd);
assert.match(connectorType, /connector_id: string/);
assert.match(connectorType, /connection_id: string/);
assert.doesNotMatch(connectorType, /action|grant|policy|access_mode/);
assert.match(types, /connector_bindings\?: AgentConnectorBinding\[\]/);

assert.doesNotMatch(pageModel, /enable_connectors/);
assert.equal(
  [...pageModel.matchAll(/connectorsEnabled: true/g)].length,
  3,
  'Agent model, orchestration and dialog must always enable the Connector surface'
);
assert.match(
  pageModel,
  /onOpenConnectorDialog: \(anchorElement: HTMLElement\) => \{\s*if \(isRuntimeConfigReadOnly/
);
assert.match(pageModel, /open: connectorDialogOpen,/);
assert.match(pageModel, /connector_bindings: connectorBindings/);
assert.match(
  pageModel,
  /setConnectorBindings\(normalizeAgentConnectorBindings\(payload\.connector_bindings \?\? \[\]\)\)/
);
assert.match(pageModel, /mergeSupersededAgentRuntimePayload/);
assert.match(signature, /connector_bindings:/);

assert.match(dialog, /ConnectorPickerDialog/);
assert.match(dialog, /groupByConnector/);
assert.doesNotMatch(dialog, /ConnectorPickerMenu/);
assert.match(dialog, /onConfirmConnectorBindings/);
assert.match(dialog, /onRequestSwitch/);
assert.match(dialog, /ConnectorKeySetupPanel/);
assert.match(dialog, /normalizeAgentConnectorBindings/);
assert.match(dialog, /connectionReadyNotAdded/);
assert.match(dialog, /useEffect\(\(\) => \{[\s\S]*setSetup\(undefined\);[\s\S]*setAuthorization\(undefined\)/);
assert.doesNotMatch(dialog, /connectorService|attachTargetBinding|createTargetBinding/);
assert.match(dialog, /authorizationMode|addToAgentTitle/);
assert.match(dialog, /picker\.authorization\.confirm/);
assert.match(dialogs, /<AgentRuntimeConnectorDialog \{\.\.\.model\.dialogs\.connector\} \/>/);
assert.doesNotMatch(dialogs, /connectorsEnabled|externalIntegrationsEnabled/);
assert.match(bindingHealth, /bindingHealth\.types\.connector_connection/);

console.log('Agent Connector picker draft, revision, and native snapshot checks passed.');
