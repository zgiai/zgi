import type { AgentType } from '@/services/types/agent';
import {
  collectForNode,
  collectScopedVariablesForNode,
  getAncestors,
} from '../store/helpers/graph';
import type { WorkflowEdge, WorkflowNode } from '../store/type';
import { isContainerNode, isContainerStartNode } from '../store/type';
import {
  isSpecialVariableSource,
  normalizeVariableSelector,
  type WorkflowPrimitiveType,
  type WorkflowVariableReferenceStatus,
} from './variable-reference';

export interface WorkflowVariableReferenceDescriptor {
  consumerNodeId: string;
  selector: string[];
  sourceId: string;
  keyPath: string[];
}

export interface WorkflowVariableReferenceHealth extends WorkflowVariableReferenceDescriptor {
  status: WorkflowVariableReferenceStatus;
  sourceNode?: WorkflowNode;
  type?: WorkflowPrimitiveType;
}

export interface WorkflowVariableReferenceImpact extends WorkflowVariableReferenceHealth {
  consumerTitle?: string;
}

const VARIABLE_TOKEN_PATTERN = /\{\{#([^.#}]+)\.([^#}]+)#\}\}/g;

function outputPathType(
  sourceNode: WorkflowNode,
  keyPath: string[],
  agentType: AgentType,
  outputCache?: Map<string, ReturnType<typeof collectForNode>>,
  scopedOutputCache?: Map<string, ReturnType<typeof collectScopedVariablesForNode>>,
  useScopedVariables = false,
  graph?: { nodes: WorkflowNode[]; edges: WorkflowEdge[] }
): WorkflowPrimitiveType | undefined {
  const [rootKey, ...nestedPath] = keyPath;
  if (!rootKey) return undefined;

  const cache = useScopedVariables ? scopedOutputCache : outputCache;
  let sourceOutput = cache?.get(sourceNode.id);
  if (!sourceOutput) {
    sourceOutput = useScopedVariables
      ? collectScopedVariablesForNode(sourceNode, agentType, graph)
      : collectForNode(sourceNode, agentType);
    cache?.set(sourceNode.id, sourceOutput);
  }
  const output = sourceOutput.variables.find(item => item.key === rootKey);
  if (!output) return undefined;
  if (nestedPath.length === 0 || !output.children?.length) return output.type;

  let children = output.children;
  let nestedType: WorkflowPrimitiveType | undefined = output.type;
  for (const key of nestedPath) {
    const field = children.find(item => item.key === key);
    if (!field) return undefined;
    nestedType = field.type as WorkflowPrimitiveType;
    children = field.children || [];
  }
  return nestedType;
}

export function createWorkflowVariableReferenceHealthResolver(args: {
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
  agentType: AgentType;
}) {
  const nodeById = new Map(args.nodes.map(node => [node.id, node]));
  const ancestorsByConsumer = new Map<string, Set<string>>();
  const outputCache = new Map<string, ReturnType<typeof collectForNode>>();
  const scopedOutputCache = new Map<string, ReturnType<typeof collectScopedVariablesForNode>>();

  const isInsideContainerScope = (consumerNodeId: string, containerId: string): boolean => {
    let current = nodeById.get(consumerNodeId);
    const visited = new Set<string>();
    while (current?.parentId && !visited.has(current.parentId)) {
      if (current.parentId === containerId) return true;
      visited.add(current.parentId);
      current = nodeById.get(current.parentId);
    }
    return false;
  };

  return (consumerNodeId: string, selector: string[]): WorkflowVariableReferenceHealth | null => {
    const normalized = normalizeVariableSelector(selector);
    if (!normalized) return null;

    const [sourceId, ...keyPath] = normalized;
    const base: WorkflowVariableReferenceDescriptor = {
      consumerNodeId,
      selector: normalized,
      sourceId,
      keyPath,
    };

    if (isSpecialVariableSource(sourceId)) {
      return { ...base, status: 'active' };
    }

    const sourceNode = nodeById.get(sourceId);
    if (!sourceNode) {
      return { ...base, status: 'source_deleted' };
    }

    const sourceIsScopedContainer =
      isContainerNode(sourceNode.data.type) &&
      (sourceId === consumerNodeId || isInsideContainerScope(consumerNodeId, sourceId));
    const type = outputPathType(
      sourceNode,
      keyPath,
      args.agentType,
      outputCache,
      scopedOutputCache,
      sourceIsScopedContainer,
      args
    );
    if (!type) {
      return { ...base, sourceNode, status: 'output_removed' };
    }

    if (sourceIsScopedContainer) {
      return { ...base, sourceNode, type, status: 'active' };
    }

    const sourceIsDirectContainerChild =
      isContainerNode(nodeById.get(consumerNodeId)?.data.type) &&
      sourceNode.parentId === consumerNodeId;
    if (sourceIsDirectContainerChild) {
      return { ...base, sourceNode, type, status: 'active' };
    }

    let ancestors = ancestorsByConsumer.get(consumerNodeId);
    if (!ancestors) {
      ancestors = new Set(getAncestors(args.nodes, args.edges, consumerNodeId));
      ancestorsByConsumer.set(consumerNodeId, ancestors);
    }
    if (!ancestors.has(sourceId)) {
      return { ...base, sourceNode, type, status: 'source_unreachable' };
    }

    return { ...base, sourceNode, type, status: 'active' };
  };
}

export function resolveWorkflowVariableReferenceHealth(args: {
  nodes: WorkflowNode[];
  edges: WorkflowEdge[];
  consumerNodeId: string;
  selector: string[];
  agentType: AgentType;
}): WorkflowVariableReferenceHealth | null {
  return createWorkflowVariableReferenceHealthResolver(args)(args.consumerNodeId, args.selector);
}

function addReference(
  references: WorkflowVariableReferenceDescriptor[],
  seen: Set<string>,
  consumerNodeId: string,
  selector: string[]
) {
  const normalized = normalizeVariableSelector(selector);
  if (!normalized) return;
  const key = `${consumerNodeId}:${normalized.join('::')}`;
  if (seen.has(key)) return;
  seen.add(key);
  references.push({
    consumerNodeId,
    selector: normalized,
    sourceId: normalized[0],
    keyPath: normalized.slice(1),
  });
}

export function collectWorkflowVariableReferences(
  nodes: WorkflowNode[]
): WorkflowVariableReferenceDescriptor[] {
  const references: WorkflowVariableReferenceDescriptor[] = [];
  const seenReferences = new Set<string>();
  const selectorProperty = (key?: string): boolean =>
    Boolean(key && (key === 'selector' || key.endsWith('_selector')));

  const isVariableBackedValue = (record?: Record<string, unknown>): boolean =>
    record?.type === 'variable' ||
    record?.value_type === 'variable' ||
    record?.input_type === 'variable';

  for (const node of nodes) {
    const visited = new WeakSet<object>();
    const visit = (value: unknown, key?: string, owner?: Record<string, unknown>) => {
      if (typeof value === 'string') {
        VARIABLE_TOKEN_PATTERN.lastIndex = 0;
        let match: RegExpExecArray | null;
        while ((match = VARIABLE_TOKEN_PATTERN.exec(value))) {
          addReference(references, seenReferences, node.id, [match[1], ...match[2].split('.')]);
        }
        return;
      }

      if (!value || typeof value !== 'object') return;
      if (visited.has(value)) return;
      visited.add(value);

      if (Array.isArray(value)) {
        const isStringSelector = value.length >= 2 && value.every(item => typeof item === 'string');
        const isExplicitSelector =
          selectorProperty(key) ||
          (key === 'value' && isVariableBackedValue(owner)) ||
          (key === 'file' && owner?.type === 'file');

        if (isStringSelector && isExplicitSelector) {
          addReference(references, seenReferences, node.id, value as string[]);
          return;
        }

        if (key === 'variables') {
          value.forEach(item => {
            if (
              Array.isArray(item) &&
              item.length >= 2 &&
              item.every(part => typeof part === 'string')
            ) {
              addReference(references, seenReferences, node.id, item as string[]);
              return;
            }
            visit(item, undefined, owner);
          });
          return;
        }

        value.forEach(item => visit(item, undefined, owner));
        return;
      }

      const record = value as Record<string, unknown>;
      Object.entries(record).forEach(([childKey, child]) => {
        if (childKey === '_children') return;
        visit(child, childKey, record);
      });
    };

    visit(node.data);
  }

  return references;
}

export function findNewlyInvalidWorkflowVariableReferences(args: {
  beforeNodes: WorkflowNode[];
  beforeEdges: WorkflowEdge[];
  afterNodes: WorkflowNode[];
  afterEdges: WorkflowEdge[];
  agentType: AgentType;
}): WorkflowVariableReferenceImpact[] {
  const survivingNodeIds = new Set(args.afterNodes.map(node => node.id));
  const beforeReferences = collectWorkflowVariableReferences(args.beforeNodes);
  const resolveBefore = createWorkflowVariableReferenceHealthResolver({
    nodes: args.beforeNodes,
    edges: args.beforeEdges,
    agentType: args.agentType,
  });
  const resolveAfter = createWorkflowVariableReferenceHealthResolver({
    nodes: args.afterNodes,
    edges: args.afterEdges,
    agentType: args.agentType,
  });

  return beforeReferences.flatMap(reference => {
    if (!survivingNodeIds.has(reference.consumerNodeId)) return [];
    const before = resolveBefore(reference.consumerNodeId, reference.selector);
    if (!before || before.status !== 'active') return [];

    const after = resolveAfter(reference.consumerNodeId, reference.selector);
    if (!after || after.status === 'active') return [];

    return [
      {
        ...after,
        consumerTitle: args.afterNodes.find(node => node.id === reference.consumerNodeId)?.data
          ?.title,
      },
    ];
  });
}

export function simulateWorkflowNodeDeletion(
  nodes: WorkflowNode[],
  edges: WorkflowEdge[],
  nodeIds: string[]
): { nodes: WorkflowNode[]; edges: WorkflowEdge[]; deletedNodeIds: string[] } {
  const requestedIds = new Set(nodeIds);
  const deletedIds = new Set<string>();
  const deletedContainerIds = new Set<string>();

  for (const node of nodes) {
    if (!requestedIds.has(node.id)) continue;
    if (node.data.type === 'start' || isContainerStartNode(node.data.type)) continue;
    deletedIds.add(node.id);
    if (isContainerNode(node.data.type)) deletedContainerIds.add(node.id);
  }

  if (deletedContainerIds.size > 0) {
    for (const node of nodes) {
      if (node.parentId && deletedContainerIds.has(node.parentId)) deletedIds.add(node.id);
    }
  }

  return {
    nodes: nodes.filter(node => !deletedIds.has(node.id)),
    edges: edges.filter(edge => !deletedIds.has(edge.source) && !deletedIds.has(edge.target)),
    deletedNodeIds: [...deletedIds],
  };
}
