import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import fs from 'node:fs';
import Module from 'node:module';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const require = createRequire(import.meta.url);
const ts = require('typescript');
const webRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const srcRoot = path.join(webRoot, 'src');
const originalResolveFilename = Module._resolveFilename;

Module._resolveFilename = function resolveFilename(request, parent, isMain, options) {
  if (request.startsWith('@/')) {
    return originalResolveFilename.call(
      this,
      path.join(srcRoot, request.slice(2)),
      parent,
      isMain,
      options
    );
  }
  return originalResolveFilename.call(this, request, parent, isMain, options);
};

Module._extensions['.ts'] = (mod, filename) => {
  const source = require('node:fs').readFileSync(filename, 'utf8');
  const output = ts.transpileModule(source, {
    compilerOptions: {
      esModuleInterop: true,
      jsx: ts.JsxEmit.ReactJSX,
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2020,
    },
    fileName: filename,
  }).outputText;
  mod._compile(output, filename);
};
Module._extensions['.tsx'] = Module._extensions['.ts'];

const {
  collectWorkflowVariableReferences,
  findNewlyInvalidWorkflowVariableReferences,
  resolveWorkflowVariableReferenceHealth,
  simulateWorkflowNodeDeletion,
} = require('../src/components/workflow/common/variable-reference-health.ts');
const { AgentType } = require('../src/services/types/agent.ts');

const sourceId = '1783473778253_1_hjhor';
const consumerId = '1783473778253_2_answer';
const middleId = '1783473778253_3_middle';

function makeNode(id, data) {
  return { id, type: 'custom', position: { x: 0, y: 0 }, data };
}

const sourceNode = makeNode(sourceId, {
  type: 'llm',
  title: '生成内容',
  variables: [],
});
const middleNode = makeNode(middleId, {
  type: 'code',
  title: '处理中转',
  outputs: { result: { type: 'string' } },
  outputKeyOrders: ['result'],
});
const consumerNode = makeNode(consumerId, {
  type: 'answer',
  title: '直接回复',
  answer: `内容：{{#${sourceId}.text#}}`,
});
const nodes = [sourceNode, middleNode, consumerNode];
const directEdge = { id: 'direct', source: sourceId, target: consumerId };
const throughSource = { id: 'source-middle', source: sourceId, target: middleId };
const throughConsumer = { id: 'middle-consumer', source: middleId, target: consumerId };

const active = resolveWorkflowVariableReferenceHealth({
  nodes,
  edges: [directEdge],
  consumerNodeId: consumerId,
  selector: [sourceId, 'text'],
  agentType: AgentType.CONVERSATIONAL_AGENT,
});
assert.equal(active?.status, 'active');

const disconnected = findNewlyInvalidWorkflowVariableReferences({
  beforeNodes: nodes,
  beforeEdges: [directEdge],
  afterNodes: nodes,
  afterEdges: [],
  agentType: AgentType.CONVERSATIONAL_AGENT,
});
assert.equal(disconnected.length, 1);
assert.equal(disconnected[0].status, 'source_unreachable');
assert.equal(disconnected[0].consumerTitle, '直接回复');

const alternativePath = findNewlyInvalidWorkflowVariableReferences({
  beforeNodes: nodes,
  beforeEdges: [directEdge, throughSource, throughConsumer],
  afterNodes: nodes,
  afterEdges: [throughSource, throughConsumer],
  agentType: AgentType.CONVERSATIONAL_AGENT,
});
assert.equal(alternativePath.length, 0);

const deletedGraph = simulateWorkflowNodeDeletion(nodes, [directEdge], [sourceId]);
const deleted = findNewlyInvalidWorkflowVariableReferences({
  beforeNodes: nodes,
  beforeEdges: [directEdge],
  afterNodes: deletedGraph.nodes,
  afterEdges: deletedGraph.edges,
  agentType: AgentType.CONVERSATIONAL_AGENT,
});
assert.equal(deleted.length, 1);
assert.equal(deleted[0].status, 'source_deleted');
assert.deepEqual(deleted[0].selector, [sourceId, 'text']);

const outputRemoved = resolveWorkflowVariableReferenceHealth({
  nodes: [
    makeNode(sourceId, {
      type: 'code',
      title: '生成内容',
      outputs: { another: { type: 'string' } },
      outputKeyOrders: ['another'],
    }),
    consumerNode,
  ],
  edges: [directEdge],
  consumerNodeId: consumerId,
  selector: [sourceId, 'text'],
  agentType: AgentType.CONVERSATIONAL_AGENT,
});
assert.equal(outputRemoved?.status, 'output_removed');

const containerSourceId = '1783473778253_4_container_source';
const iterationId = '1783473778253_5_iteration';
const iterationStartId = `${iterationId}start`;
const iterationChildId = '1783473778253_6_iteration_child';
const loopId = '1783473778253_7_loop';
const loopStartId = `${loopId}start`;
const loopChildId = '1783473778253_8_loop_child';

const containerSource = makeNode(containerSourceId, {
  type: 'code',
  title: 'Container source',
  outputs: {
    items: { type: 'array[string]' },
    initial: { type: 'number' },
  },
  outputKeyOrders: ['items', 'initial'],
});
const iterationNode = makeNode(iterationId, {
  type: 'iteration',
  title: 'Iteration',
  iterator_selector: [containerSourceId, 'items'],
  iterator_input_type: 'array[string]',
  output_selector: [iterationChildId, 'text'],
  output_type: 'array[string]',
  _children: [iterationStartId, iterationChildId],
});
const iterationStartNode = {
  ...makeNode(iterationStartId, { type: 'iteration-start', title: '' }),
  parentId: iterationId,
};
const iterationChild = {
  ...makeNode(iterationChildId, {
    type: 'llm',
    title: 'Iteration child',
    prompt_template: `Current item: {{#${iterationId}.item#}}`,
    variables: [],
  }),
  parentId: iterationId,
};
const loopNode = makeNode(loopId, {
  type: 'loop',
  title: 'Loop',
  loop_variables: [
    {
      label: 'num',
      var_type: 'number',
      value_type: 'variable',
      value: [containerSourceId, 'initial'],
    },
  ],
  break_conditions: [
    {
      id: 'condition',
      varType: 'number',
      variable_selector: [loopId, 'num'],
      comparison_operator: '=',
      value: '3',
    },
  ],
  _children: [loopStartId, loopChildId],
});
const loopStartNode = {
  ...makeNode(loopStartId, { type: 'loop-start', title: '' }),
  parentId: loopId,
};
const loopChild = {
  ...makeNode(loopChildId, {
    type: 'code',
    title: 'Loop child',
    outputs: { result: { type: 'string' } },
    outputKeyOrders: ['result'],
  }),
  parentId: loopId,
};
const containerNodes = [
  containerSource,
  iterationNode,
  iterationStartNode,
  iterationChild,
  loopNode,
  loopStartNode,
  loopChild,
];
const containerEdges = [
  { id: 'source-iteration', source: containerSourceId, target: iterationId },
  { id: 'iteration-start-child', source: iterationStartId, target: iterationChildId },
  { id: 'source-loop', source: containerSourceId, target: loopId },
  { id: 'loop-start-child', source: loopStartId, target: loopChildId },
];

const containerReferences = collectWorkflowVariableReferences(containerNodes);
assert.deepEqual(
  containerReferences.map(reference => [reference.consumerNodeId, ...reference.selector]),
  [
    [iterationId, containerSourceId, 'items'],
    [iterationId, iterationChildId, 'text'],
    [iterationChildId, iterationId, 'item'],
    [loopId, containerSourceId, 'initial'],
    [loopId, loopId, 'num'],
  ]
);

for (const reference of containerReferences) {
  const health = resolveWorkflowVariableReferenceHealth({
    nodes: containerNodes,
    edges: containerEdges,
    consumerNodeId: reference.consumerNodeId,
    selector: reference.selector,
    agentType: AgentType.CONVERSATIONAL_AGENT,
  });
  assert.equal(
    health?.status,
    'active',
    `${reference.consumerNodeId}:${reference.selector.join('.')} should be active`
  );
}

const disconnectedContainerInput = resolveWorkflowVariableReferenceHealth({
  nodes: containerNodes,
  edges: containerEdges.filter(edge => edge.id !== 'source-iteration'),
  consumerNodeId: iterationId,
  selector: [containerSourceId, 'items'],
  agentType: AgentType.CONVERSATIONAL_AGENT,
});
assert.equal(disconnectedContainerInput?.status, 'source_unreachable');

const impactProviderSource = fs.readFileSync(
  path.join(srcRoot, 'components/workflow/ui/variable-reference-impact-provider.tsx'),
  'utf8'
);
assert.doesNotMatch(
  impactProviderSource,
  /applyEdgeChanges/,
  'disconnecting an edge should not open the variable impact confirmation'
);
assert.match(
  impactProviderSource,
  /doNotWarnAgainThisEdit/,
  'node deletion confirmation should offer an edit-session suppression option'
);

const assignerNodeSource = fs.readFileSync(
  path.join(srcRoot, 'components/workflow/nodes/assigner/index.tsx'),
  'utf8'
);
assert.match(
  assignerNodeSource,
  /<ValueBadge[\s\S]*selector=\{valueSelector\}/,
  'assigner source variables should use the shared variable badge'
);

console.log('Workflow variable reference health regression checks passed.');
