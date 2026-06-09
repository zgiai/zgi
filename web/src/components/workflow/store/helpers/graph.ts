// Graph helpers: traversal and upstream variable exports
// Strictly typed, no any usage. Designed for performance and reusability.

import type {
  WorkflowEdge,
  WorkflowNode,
  WorkflowNodeData,
  WorkflowVariable,
  StartNodeData,
  LLMNodeData,
  LoopNodeData,
  ApprovalNodeData,
  QuestionAnswerNodeData,
} from '../type';
import { isContainerNode, isContainerStartNode } from '../type';
import type { NodeChange } from '@xyflow/react';
import type { CodeNodeData } from '../type';
// re-use PrimitiveType alias; avoid importing WorkflowVariable interface twice
import type { InputVar, StructuredTypeField } from '../../types/input-var';
import {
  InputVarType,
  getSystemVariablesForAgent,
  getStructuredTypeFields,
} from '../../types/input-var';
import type { AgentType } from '@/services/types/agent';
import { type NodesSuffix } from '@/i18n';
import { NODE_THEMES } from '../../nodes/custom/config';

// Primitive type alias used for upstream variable declarations
export type PrimitiveType = WorkflowVariable['type'];

// Upstream export item type used by consumers (UI selectors, etc.)
export interface UpstreamExportItem {
  nodeId: string;
  nodeType: WorkflowNodeData['type'];
  nodeTitle?: string;
  variables: Array<{
    key: string;
    type: PrimitiveType;
    writable?: boolean;
    description?: string;
    descriptionKey?: NodesSuffix;
    /** True if this is a variable-aggregator group output */
    isAggregatorGroup?: boolean;
    /** Nested children for recursive structures (e.g., json-parser nested objects) */
    children?: StructuredTypeField[];
  }>;
}

function mapArrayToBaseType(t: PrimitiveType): PrimitiveType {
  switch (t) {
    case 'array[string]':
      return 'string';
    case 'array[number]':
      return 'number';
    case 'array[boolean]':
      return 'boolean';
    case 'array[object]':
      return 'object';
    case 'array[file]':
      return 'file';
    case 'array':
      return 'object';
    default:
      return t;
  }
}

export function isArrayType(t: PrimitiveType): boolean {
  return (
    t === 'array' ||
    t === 'array[string]' ||
    t === 'array[number]' ||
    t === 'array[boolean]' ||
    t === 'array[object]' ||
    t === 'array[file]'
  );
}

function collectLoopVariableExports(
  variables: LoopNodeData['loop_variables'] | undefined,
  options: { writable: boolean }
): UpstreamExportItem['variables'] {
  const seen = new Set<string>();

  return (variables || [])
    .filter(variable => {
      const key = typeof variable.label === 'string' ? variable.label.trim() : '';
      if (!key || seen.has(key)) return false;
      seen.add(key);
      return true;
    })
    .map(variable => ({
      key: variable.label.trim(),
      type: variable.var_type as PrimitiveType,
      writable: options.writable,
      description: 'Loop variable',
    }));
}

/**
 * Return all incoming source node IDs that connect into the given node.
 */
export function getIncomingSources(
  edges: WorkflowEdge[],
  nodeId: string,
  _ctx?: { incomingEdgesMap?: Map<string, WorkflowEdge[]> }
): string[] {
  if (_ctx?.incomingEdgesMap) {
    return (_ctx.incomingEdgesMap.get(nodeId) || []).map(e => e.source);
  }
  return edges.filter(e => e.target === nodeId).map(e => e.source);
}

/**
 * Calculate the absolute position of a node by traversing parent chain.
 * Sub-nodes inside containers have relative positions; this computes the true canvas position.
 */
export function getNodeAbsolutePosition(
  nodeId: string,
  nodes: WorkflowNode[]
): { x: number; y: number } {
  const nodeMap = new Map(nodes.map(n => [n.id, n]));
  const node = nodeMap.get(nodeId);
  if (!node) return { x: 0, y: 0 };

  const pos = { x: node.position.x, y: node.position.y };
  let curr = node;

  // Recursively add parent positions
  while ((curr as unknown as { parentId?: string }).parentId) {
    const parentId = (curr as unknown as { parentId?: string }).parentId;
    if (!parentId) break;
    const parent = nodeMap.get(parentId);
    if (!parent || parent === curr) break;
    pos.x += parent.position.x;
    pos.y += parent.position.y;
    curr = parent;
  }

  return pos;
}

/**
 * Breadth-first traversal to collect all ancestors (upstream nodes) of the given node.
 * Now follows both incoming edges AND parent-child (parentId) hierarchy.
 */
export function getAncestors(
  nodes: WorkflowNode[],
  edges: WorkflowEdge[],
  nodeId: string,
  _ctx?: {
    idToNode?: Map<string, WorkflowNode>;
    incomingEdgesMap?: Map<string, WorkflowEdge[]>;
  }
): string[] {
  const result: string[] = [];
  const visited = new Set<string>();
  const idToNode = _ctx?.idToNode || new Map<string, WorkflowNode>(nodes.map(n => [n.id, n]));

  // Queue stores pairs of [currentId, source] to track where we came from if needed
  const queue: string[] = [];

  // 1) Seed queue with initial parents from edges
  getIncomingSources(edges, nodeId, _ctx).forEach(id => queue.push(id));

  // 2) Seed queue with parentId if exists
  const self = idToNode.get(nodeId);
  const selfPid = (self as unknown as { parentId?: string })?.parentId;
  if (selfPid) queue.push(selfPid);

  let head = 0;
  while (head < queue.length) {
    const current = queue[head++];
    if (!current) break;
    if (visited.has(current)) continue;
    visited.add(current);
    result.push(current);

    // Follow edges upstream
    const parents = getIncomingSources(edges, current);
    for (const p of parents) {
      if (!visited.has(p)) queue.push(p);
    }

    // Follow parentId hierarchy upstream
    const node = idToNode.get(current);
    if (node) {
      const pid = (node as unknown as { parentId?: string })?.parentId;
      if (pid && !visited.has(pid)) {
        queue.push(pid);
      }
    }
  }
  return result;
}

/**
 * Infer primitive type for Start node custom input variable.
 */
function inferStartVarType(v: InputVar): PrimitiveType {
  switch (v.type) {
    case InputVarType.TEXT_INPUT:
    case InputVarType.PARAGRAPH:
    case InputVarType.SELECT:
      return 'string';
    case InputVarType.NUMBER:
      return 'number';
    case InputVarType.CHECKBOX:
      return 'boolean';
    case InputVarType.FILE_LIST:
      return 'array[file]';
    case InputVarType.FILE:
      return 'file';
    default:
      return 'string';
  }
}

/**
 * Collect standardized downstream export variables for a node, given the agent type.
 */
export function collectForNode(node: WorkflowNode, agentType: AgentType): UpstreamExportItem {
  const nodeType = (node.data as WorkflowNodeData).type;
  const base: UpstreamExportItem = {
    nodeId: node.id,
    nodeType,
    nodeTitle: node.data.title, // read-only for display
    variables: [],
  };
  switch (nodeType) {
    case 'start': {
      const data = node.data as StartNodeData;
      const customVars = (data.variables || []).map(v => {
        const varType = inferStartVarType(v);
        const children = getStructuredTypeFields(varType);
        return {
          key: v.variable,
          type: varType,
          ...(children ? { children } : {}),
        };
      });
      const sysVars = getSystemVariablesForAgent(agentType).map(sv => {
        const children = getStructuredTypeFields(sv.type);
        return {
          key: sv.key,
          type: sv.type as PrimitiveType,
          descriptionKey: sv.description,
          ...(children ? { children } : {}),
        };
      });
      // De-duplicate by key, prefer custom variable definition
      const combined = [...customVars, ...sysVars];
      const seen = new Set<string>();
      base.variables = combined.filter(v => {
        if (!v.key) return false;
        if (seen.has(v.key)) return false;
        seen.add(v.key);
        return true;
      });
      break;
    }
    case 'llm': {
      const data = node.data as LLMNodeData;
      const standard: Array<{ key: string; type: PrimitiveType; descriptionKey?: NodesSuffix }> = [
        { key: 'text', type: 'string', descriptionKey: 'outputDescriptions.llm.text' },
        { key: 'usage', type: 'object', descriptionKey: 'outputDescriptions.llm.usage' },
      ];
      const declared = (data.variables || []).map(v => ({ key: v.name, type: v.type }));
      base.variables = [...standard, ...declared];
      break;
    }
    case 'knowledge-retrieval': {
      const standard: Array<{ key: string; type: PrimitiveType; descriptionKey?: NodesSuffix }> = [
        {
          key: 'result',
          type: 'array[object]',
          descriptionKey: 'outputDescriptions.knowledge-retrieval.result',
        },
      ];
      base.variables = standard;
      break;
    }
    case 'http-request': {
      const standard: Array<{ key: string; type: PrimitiveType; descriptionKey?: NodesSuffix }> = [
        { key: 'body', type: 'string', descriptionKey: 'outputDescriptions.http-request.body' },
        {
          key: 'status_code',
          type: 'number',
          descriptionKey: 'outputDescriptions.http-request.status_code',
        },
        {
          key: 'headers',
          type: 'object',
          descriptionKey: 'outputDescriptions.http-request.headers',
        },
        {
          key: 'files',
          type: 'array[file]',
          descriptionKey: 'outputDescriptions.http-request.files',
        },
        {
          key: 'error_message',
          type: 'string',
          descriptionKey: 'outputDescriptions.http-request.error_message',
        },
        {
          key: 'error_type',
          type: 'string',
          descriptionKey: 'outputDescriptions.http-request.error_type',
        },
      ];
      base.variables = standard;
      break;
    }
    case 'code': {
      const data = node.data as CodeNodeData;
      const keys =
        data.outputKeyOrders && data.outputKeyOrders.length > 0
          ? data.outputKeyOrders
          : Object.keys(data.outputs || {});
      base.variables = keys.map(k => ({
        key: k,
        type: (data.outputs?.[k]?.type || 'string') as PrimitiveType,
      }));
      break;
    }
    case 'iteration': {
      const data = node.data as unknown as {
        output_type?: PrimitiveType;
        iterator_input_type?: PrimitiveType;
        title?: string;
      };
      const resultType = (data.output_type as PrimitiveType) || ('array[string]' as PrimitiveType);
      base.variables = [
        { key: 'output', type: resultType, descriptionKey: 'outputDescriptions.iteration.result' },
      ];
      break;
    }
    case 'loop': {
      const data = node.data as LoopNodeData;
      base.variables = collectLoopVariableExports(data.loop_variables, { writable: false });
      break;
    }
    case 'tools': {
      // Default downstream exposed variables for tool nodes
      // - text: tool-generated content
      // - files: tool-generated files
      // - json: tool-generated JSON objects
      const standard: Array<{ key: string; type: PrimitiveType; descriptionKey?: NodesSuffix }> = [
        { key: 'text', type: 'string', descriptionKey: 'outputDescriptions.tool.text' },
        { key: 'files', type: 'array[file]', descriptionKey: 'outputDescriptions.tool.files' },
        { key: 'json', type: 'array[object]', descriptionKey: 'outputDescriptions.tool.json' },
      ];
      base.variables = standard;
      break;
    }
    case 'create-scheduled-task': {
      const standard: Array<{ key: string; type: PrimitiveType; descriptionKey?: NodesSuffix }> = [
        {
          key: 'task_id',
          type: 'string',
          descriptionKey: 'outputDescriptions.create-scheduled-task.task_id',
        },
        {
          key: 'status',
          type: 'string',
          descriptionKey: 'outputDescriptions.create-scheduled-task.status',
        },
        {
          key: 'schedule_type',
          type: 'string',
          descriptionKey: 'outputDescriptions.create-scheduled-task.schedule_type',
        },
        {
          key: 'timezone',
          type: 'string',
          descriptionKey: 'outputDescriptions.create-scheduled-task.timezone',
        },
        {
          key: 'next_run_at',
          type: 'string',
          descriptionKey: 'outputDescriptions.create-scheduled-task.next_run_at',
        },
      ];
      base.variables = standard;
      break;
    }
    case 'call-database': {
      // Expose only the three variables specified by the doc:
      // - rows: array of objects (query result rows)
      // - row_count: number of rows returned
      // - duration_ms: execution duration in milliseconds
      const standard: Array<{ key: string; type: PrimitiveType; descriptionKey?: NodesSuffix }> = [
        {
          key: 'rows',
          type: 'array[object]',
          descriptionKey: 'outputDescriptions.call-database.rows',
        },
        {
          key: 'row_count',
          type: 'number',
          descriptionKey: 'outputDescriptions.call-database.row_count',
        },
        {
          key: 'duration_ms',
          type: 'number',
          descriptionKey: 'outputDescriptions.call-database.duration_ms',
        },
      ];
      base.variables = standard;
      break;
    }
    case 'sql-generator': {
      // Expose the generated SQL query as a string variable
      const standard: Array<{ key: string; type: PrimitiveType; descriptionKey?: NodesSuffix }> = [
        { key: 'sql', type: 'string', descriptionKey: 'outputDescriptions.sql-generator.sql' },
      ];
      base.variables = standard;
      break;
    }
    case 'document-extractor': {
      // Expose extracted text as a string variable
      const standard: Array<{ key: string; type: PrimitiveType; descriptionKey?: NodesSuffix }> = [
        {
          key: 'text',
          type: 'string',
          descriptionKey: 'outputDescriptions.document-extractor.text',
        },
      ];
      base.variables = standard;
      break;
    }
    case 'parameter-extractor': {
      const data = node.data as {
        parameters?: Array<{ name?: string; type?: PrimitiveType; description?: string }>;
      };
      const params = (data.parameters || []).filter(
        p => typeof p?.name === 'string' && (p.name as string).trim().length > 0 && p?.type
      ) as Array<{ name: string; type: PrimitiveType; description: string }>;
      base.variables = params.map(p => ({
        key: p.name,
        type: p.type as PrimitiveType,
        description: p?.description || '',
      }));
      break;
    }
    case 'variable-aggregator': {
      const data = node.data as unknown as {
        output_type?: PrimitiveType;
        variables?: Array<[string, string]>;
        advanced_settings?: {
          group_enabled?: boolean;
          groups?: Array<{ group_name: string; output_type?: PrimitiveType }>;
        };
      };
      const groupEnabled = Boolean(data.advanced_settings?.group_enabled);
      if (groupEnabled) {
        const groups = (data.advanced_settings?.groups || []).filter(
          g => typeof g.group_name === 'string' && g.group_name.trim().length > 0
        );
        // Group mode: each group outputs an object variable containing 'output' field
        // Description includes the inner output type for clarity
        base.variables = groups.map(g => {
          const innerType = g.output_type || 'string';
          const children = getStructuredTypeFields('object', true, innerType);
          return {
            key: g.group_name,
            type: 'object' as PrimitiveType,
            descriptionKey: 'outputDescriptions.variable-aggregator.groupOutput',
            description: innerType,
            isAggregatorGroup: true,
            ...(children ? { children } : {}),
          };
        });
      } else {
        const t: PrimitiveType = (data.output_type as PrimitiveType) || ('string' as PrimitiveType);
        base.variables = [{ key: 'output', type: t }];
      }
      break;
    }
    case 'json-parser': {
      // Expose output variables based on the configured outputs schema
      // Schema type definition for json-parser outputs
      interface JsonParserSchema {
        type: PrimitiveType;
        children?: Record<string, JsonParserSchema>;
      }
      const data = node.data as unknown as {
        outputs?: Record<string, JsonParserSchema>;
        is_flatten_output?: boolean;
      };
      const outputs = data.outputs || {};

      // Helper to map json-parser type to StructuredFieldType
      const mapToFieldType = (t: PrimitiveType): StructuredTypeField['type'] => {
        if (t === 'string') return 'string';
        if (t === 'number') return 'number';
        if (t === 'boolean') return 'boolean';
        if (t === 'file') return 'file';
        if (t === 'object') return 'object';
        // Preserve specific array types for proper type filtering
        if (t === 'array[string]') return 'array[string]';
        if (t === 'array[number]') return 'array[number]';
        if (t === 'array[boolean]') return 'array[boolean]';
        if (t === 'array[object]') return 'array[object]';
        if (t === 'array[file]') return 'array[file]';
        if (t.startsWith('array')) return 'array';
        return 'string';
      };

      // Recursively convert schema children to StructuredTypeField[]
      const schemaToFields = (schema: JsonParserSchema): StructuredTypeField[] | undefined => {
        if (!schema.children || Object.keys(schema.children).length === 0) {
          return undefined;
        }
        return Object.entries(schema.children).map(([childKey, childSchema]) => ({
          key: childKey,
          type: mapToFieldType(childSchema.type),
          children: schemaToFields(childSchema),
        }));
      };

      base.variables = Object.entries(outputs).map(([key, schema]) => ({
        key,
        type: (schema?.type || 'object') as PrimitiveType,
        descriptionKey: 'outputDescriptions.json-parser.result' as NodesSuffix,
        children: schemaToFields(schema),
      }));
      break;
    }
    case 'image-gen': {
      const standard: Array<{ key: string; type: PrimitiveType; descriptionKey?: NodesSuffix }> = [
        {
          key: 'files',
          type: 'array[file]',
          descriptionKey: 'outputDescriptions.image-gen.files',
        },
        {
          key: 'urls',
          type: 'array[string]',
          descriptionKey: 'outputDescriptions.image-gen.urls',
        },
      ];
      base.variables = standard;
      break;
    }
    case 'approval': {
      const data = node.data as ApprovalNodeData;
      const fieldVars = (data.approval?.fields || [])
        .filter(field => field.key && !field.key.startsWith('__'))
        .map(field => ({
          key: field.key,
          type: 'string' as PrimitiveType,
          description: field.label || '',
        }));
      base.variables = [
        ...fieldVars,
        {
          key: 'approval_action_id',
          type: 'string',
          descriptionKey: 'outputDescriptions.approval.approval_action_id',
        },
        {
          key: 'approval_action_label',
          type: 'string',
          descriptionKey: 'outputDescriptions.approval.approval_action_label',
        },
        {
          key: 'approval_rendered_content',
          type: 'string',
          descriptionKey: 'outputDescriptions.approval.approval_rendered_content',
        },
      ];
      break;
    }
    case 'announcement': {
      base.variables = [
        {
          key: 'title',
          type: 'string',
          descriptionKey: 'outputDescriptions.announcement.title',
        },
        {
          key: 'content',
          type: 'string',
          descriptionKey: 'outputDescriptions.announcement.content',
        },
        {
          key: 'expiration_time',
          type: 'string',
          descriptionKey: 'outputDescriptions.announcement.expiration_time',
        },
        {
          key: 'token',
          type: 'string',
          descriptionKey: 'outputDescriptions.announcement.token',
        },
        {
          key: 'access_token',
          type: 'string',
          descriptionKey: 'outputDescriptions.announcement.access_token',
        },
        {
          key: 'url',
          type: 'string',
          descriptionKey: 'outputDescriptions.announcement.url',
        },
      ];
      break;
    }
    case 'question-answer': {
      const data = node.data as QuestionAnswerNodeData;
      base.variables = [
        {
          key: 'question',
          type: 'string',
          descriptionKey: 'outputDescriptions.questionAnswer.question',
        },
        {
          key: 'answer',
          type: 'string',
          descriptionKey: 'outputDescriptions.questionAnswer.answer',
        },
        {
          key: 'answers',
          type: 'array[object]',
          descriptionKey: 'outputDescriptions.questionAnswer.answers',
        },
        {
          key: 'round',
          type: 'number',
          descriptionKey: 'outputDescriptions.questionAnswer.round',
        },
        {
          key: 'complete',
          type: 'boolean',
          descriptionKey: 'outputDescriptions.questionAnswer.complete',
        },
        ...(data.answer_type === 'choice'
          ? [
              {
                key: 'choices',
                type: 'array[object]' as PrimitiveType,
                descriptionKey: 'outputDescriptions.questionAnswer.choices' as NodesSuffix,
              },
              {
                key: 'choice_id',
                type: 'string' as PrimitiveType,
                descriptionKey: 'outputDescriptions.questionAnswer.choice_id' as NodesSuffix,
              },
              {
                key: 'choice_label',
                type: 'string' as PrimitiveType,
                descriptionKey: 'outputDescriptions.questionAnswer.choice_label' as NodesSuffix,
              },
              {
                key: 'choice_value',
                type: 'string' as PrimitiveType,
                descriptionKey: 'outputDescriptions.questionAnswer.choice_value' as NodesSuffix,
              },
            ]
          : data.extract_from_answer
            ? [
                {
                  key: 'extracted_fields',
                  type: 'object' as PrimitiveType,
                  descriptionKey:
                    'outputDescriptions.questionAnswer.extracted_fields' as NodesSuffix,
                },
                ...(data.extraction_fields || [])
                  .filter(field => field.name?.trim())
                  .map(field => ({
                    key: field.name.trim(),
                    type: field.type as PrimitiveType,
                    description: field.description,
                  })),
              ]
            : []),
      ];
      break;
    }

    default:
      break;
  }

  // iteration-start is handled specially in getUpstreamVariables where parent is accessible

  // Other node types can extend here in the future
  return base;
}

/**
 * Collect standardized scoped variables for a container node (e.g., iteration's item/index).
 * These are only available to nodes INSIDE the container.
 */
export function collectScopedVariablesForNode(
  node: WorkflowNode,
  agentType: AgentType,
  context?: {
    nodes: WorkflowNode[];
    edges: WorkflowEdge[];
  }
): UpstreamExportItem {
  const nodeType = (node.data as WorkflowNodeData).type;
  const base: UpstreamExportItem = {
    nodeId: node.id,
    nodeType,
    nodeTitle: node.data.title,
    variables: [],
  };

  switch (nodeType) {
    case 'iteration': {
      const inputArrayType =
        (node.data as unknown as { iterator_input_type?: PrimitiveType })?.iterator_input_type ||
        ('array[string]' as PrimitiveType);
      const baseType = mapArrayToBaseType(inputArrayType);
      base.variables = [
        {
          key: 'item',
          type: baseType,
          writable: true,
          descriptionKey: 'outputDescriptions.iteration.item',
        },
        {
          key: 'index',
          type: 'number',
          writable: true,
          descriptionKey: 'outputDescriptions.iteration.index',
        },
      ];

      // Try to resolve the input variable structure (children) and attach it to 'item'
      // This allows the iterator's 'item' to inherit the nested structure of the input array elements
      if (context && node.data.iterator_selector) {
        const selector = node.data.iterator_selector as string[];
        if (Array.isArray(selector) && selector.length >= 2) {
          const [sourceNodeId, varKey, ...subKeys] = selector;
          const sourceNode = context.nodes.find(n => n.id === sourceNodeId);
          if (sourceNode) {
            // Get source node's output variables
            const upstream = collectForNode(sourceNode, agentType);
            let targetVar = upstream.variables.find(v => v.key === varKey);

            // Traverse down if selector has subkeys (e.g. selecting a field of an object)
            for (const subKey of subKeys) {
              if (targetVar && targetVar.children) {
                targetVar = targetVar.children.find(c => c.key === subKey);
              } else {
                targetVar = undefined;
                break;
              }
            }

            // If we found the variable and it has structure (children), assign it to 'item'
            if (targetVar && targetVar.children) {
              base.variables[0].children = targetVar.children;
            }
          }
        }
      }
      if (baseType === 'file' && !base.variables[0].children) {
        const fileChildren = getStructuredTypeFields('file');
        if (fileChildren) {
          base.variables[0].children = fileChildren;
        }
      }
      break;
    }
    case 'loop': {
      const data = node.data as LoopNodeData;
      base.variables = collectLoopVariableExports(data.loop_variables, { writable: true });
      break;
    }
    default:
      break;
  }

  return base;
}

/**
 * Compute upstream variable export schema for all ancestors of the given node.
 */
export function getUpstreamVariables(
  nodes: WorkflowNode[],
  edges: WorkflowEdge[],
  nodeId: string,
  agentType: AgentType,
  _ctx?: {
    idToNode?: Map<string, WorkflowNode>;
    incomingEdgesMap?: Map<string, WorkflowEdge[]>;
  }
): UpstreamExportItem[] {
  const ancestorIds = getAncestors(nodes, edges, nodeId, _ctx);
  const idToNode = _ctx?.idToNode || new Map<string, WorkflowNode>(nodes.map(n => [n.id, n]));
  const result: UpstreamExportItem[] = [];
  const seen = new Set<string>();

  // Identify the scope hierarchy (parent containers of the target node)
  const scopeHierarchy = new Set<string>();
  let currentScopeNode = idToNode.get(nodeId);
  while (currentScopeNode) {
    const pid = currentScopeNode.parentId;
    if (!pid) break;
    scopeHierarchy.add(pid);
    currentScopeNode = idToNode.get(pid);
  }

  for (const aid of ancestorIds) {
    const node = idToNode.get(aid);
    if (!node) continue;
    const nodeType = (node.data as WorkflowNodeData).type;

    if (isContainerStartNode(nodeType)) {
      // Treat container-start downstream variables as belonging to its parent container node.
      const parentId = (node as unknown as { parentId?: string })?.parentId;
      if (!parentId || seen.has(parentId)) continue;

      const parent = idToNode.get(parentId);
      const pType = (parent?.data as WorkflowNodeData)?.type;

      // Only provide internal variables if the container is an actual scope parent of the target node.
      if (parent && isContainerNode(pType) && scopeHierarchy.has(parentId)) {
        const item = collectScopedVariablesForNode(parent, agentType, { nodes, edges });
        if ((item.variables?.length || 0) > 0) {
          seen.add(parentId);
          result.push(item);
          continue;
        }
      }
    }

    if (seen.has(node.id)) continue;

    // If we encounter a container node directly (via BFS/parentId chain):
    // 1) If it's in our scope hierarchy, we export its internal scoped variables.
    // 2) If it's NOT in our scope hierarchy, it's just a normal upstream node, so we export its standard output.
    if (isContainerNode(nodeType) && scopeHierarchy.has(node.id)) {
      const item = collectScopedVariablesForNode(node, agentType, { nodes, edges });
      if ((item.variables?.length || 0) > 0) {
        seen.add(node.id);
        result.push(item);
        continue;
      }
    }

    const item = collectForNode(node, agentType);
    // Normalize legacy types for backward compatibility
    item.variables = (item.variables || []).map(v => {
      let t = v.type as PrimitiveType;
      if (t === 'array') {
        if (v.key === 'files' || v.key === 'sys.files') t = 'array[file]';
        else t = 'array[object]';
      }
      return { ...v, type: t };
    });

    seen.add(node.id);
    result.push(item);
  }

  return result;
}

/**
 * Filter upstream variables to only include writable ones
 */
export function getUpstreamWritableVariables(
  nodes: WorkflowNode[],
  edges: WorkflowEdge[],
  nodeId: string,
  agentType: AgentType,
  _ctx?: {
    idToNode?: Map<string, WorkflowNode>;
    incomingEdgesMap?: Map<string, WorkflowEdge[]>;
  }
): UpstreamExportItem[] {
  const groups = getUpstreamVariables(nodes, edges, nodeId, agentType, _ctx);
  const filtered = groups
    .map(g => ({
      ...g,
      variables: (g.variables || []).filter(v => v.writable === true),
    }))
    .filter(g => (g.variables?.length || 0) > 0);
  return filtered;
}

export function dedupeEdges(edges: WorkflowEdge[]): WorkflowEdge[] {
  const seen = new Set<string>();
  const result: WorkflowEdge[] = [];
  for (const e of edges) {
    const key = `${e.source}|${e.sourceHandle ?? 'source'}|${e.target}|${e.targetHandle ?? 'target'}`;
    if (seen.has(key)) continue;
    seen.add(key);
    result.push(e);
  }
  return result;
}

export function getAdjacency(edges: WorkflowEdge[]): Map<string, string[]> {
  const adj = new Map<string, string[]>();
  for (const e of edges) {
    const list = adj.get(e.source) || [];
    list.push(e.target);
    adj.set(e.source, list);
  }
  return adj;
}

export function computeRunnableSets(
  nodes: WorkflowNode[],
  edges: WorkflowEdge[]
): {
  mainRunnable: Set<string>;
  iterRunnableMap: Map<string, Set<string>>;
  commentSet: Set<string>;
} {
  const idToNode = new Map<string, WorkflowNode>();
  const mainRunnable = new Set<string>();
  const iterRunnableMap = new Map<string, Set<string>>();
  const adj = new Map<string, string[]>();
  let startNodeId = '';

  for (let i = 0; i < nodes.length; i++) {
    const n = nodes[i];
    idToNode.set(n.id, n);
    if ((n.data as WorkflowNodeData).type === 'start') startNodeId = n.id;
  }

  for (let i = 0; i < edges.length; i++) {
    const e = edges[i];
    const list = adj.get(e.source) || [];
    list.push(e.target);
    adj.set(e.source, list);
  }

  if (startNodeId) {
    const q: string[] = (adj.get(startNodeId) || []).slice();
    const visited = new Set<string>();
    let head = 0;
    while (head < q.length) {
      const cur = q[head++];
      if (visited.has(cur)) continue;
      visited.add(cur);
      mainRunnable.add(cur);
      const nexts = adj.get(cur) || [];
      for (let j = 0; j < nexts.length; j++) q.push(nexts[j]);
    }
  }

  for (let i = 0; i < nodes.length; i++) {
    const n = nodes[i];
    const type = (n.data as WorkflowNodeData).type;
    if (isContainerStartNode(type)) {
      const parentId = (n as unknown as { parentId?: string })?.parentId || '';
      const set = new Set<string>();
      const q: string[] = (adj.get(n.id) || []).slice();
      const visited = new Set<string>();
      let head = 0;
      while (head < q.length) {
        const cur = q[head++];
        if (visited.has(cur)) continue;
        visited.add(cur);
        const node = idToNode.get(cur);
        if (!node) continue;
        const pid = (node as unknown as { parentId?: string })?.parentId || '';
        if (pid !== parentId) continue;
        set.add(cur);
        const nexts = adj.get(cur) || [];
        for (let j = 0; j < nexts.length; j++) {
          const nextNode = idToNode.get(nexts[j]);
          if (nextNode?.parentId === parentId) q.push(nexts[j]);
        }
      }
      iterRunnableMap.set(n.id, set);
    }
  }

  const commentSet = new Set<string>();
  for (let i = 0; i < nodes.length; i++) {
    const id = nodes[i].id;
    const type = (nodes[i].data as WorkflowNodeData).type;
    if (type === 'start' || isContainerStartNode(type)) continue;
    if (mainRunnable.has(id)) continue;
    let inAnyIter = false;
    for (const set of iterRunnableMap.values()) {
      if (set.has(id)) {
        inAnyIter = true;
        break;
      }
    }
    if (!inAnyIter) commentSet.add(id);
  }

  const commentedIterParents = new Set<string>();
  for (let i = 0; i < nodes.length; i++) {
    const n = nodes[i];
    const t = (n.data as WorkflowNodeData).type;
    if (isContainerNode(t) && commentSet.has(n.id)) {
      commentedIterParents.add(n.id);
    }
  }
  if (commentedIterParents.size > 0) {
    for (let i = 0; i < nodes.length; i++) {
      const n = nodes[i];
      const pid = (n as unknown as { parentId?: string }).parentId || '';
      if (pid && commentedIterParents.has(pid)) {
        commentSet.add(n.id);
      }
    }
  }

  return { mainRunnable, iterRunnableMap, commentSet };
}

export function reachableHasOutput(
  nodes: WorkflowNode[],
  edges: WorkflowEdge[],
  fromId: string,
  allowedOutputs: Set<WorkflowNodeData['type']>,
  scopeParentId?: string,
  memo?: Map<string, boolean>,
  _ctx?: {
    idToNode?: Map<string, WorkflowNode>;
    adj?: Map<string, string[]>;
  }
): boolean {
  const key = `${fromId}|${Array.from(allowedOutputs).sort().join(',')}|${scopeParentId || ''}`;
  if (memo && memo.has(key)) return memo.get(key) as boolean;
  const idToNode = _ctx?.idToNode || new Map<string, WorkflowNode>(nodes.map(n => [n.id, n]));
  const adj = _ctx?.adj || getAdjacency(edges);
  const q: string[] = (adj.get(fromId) || []).slice();
  const visited = new Set<string>();
  let head = 0;
  while (head < q.length) {
    const cur = q[head++];
    if (visited.has(cur)) continue;
    visited.add(cur);
    const node = idToNode.get(cur);
    if (!node) continue;
    const type = (node.data as WorkflowNodeData).type;
    const pid = (node as unknown as { parentId?: string })?.parentId;
    if (scopeParentId && pid !== scopeParentId) continue;
    if (allowedOutputs.has(type)) {
      if (memo) memo.set(key, true);
      return true;
    }
    const nexts = adj.get(cur) || [];
    for (const nx of nexts) q.push(nx);
  }
  if (memo) memo.set(key, false);
  return false;
}

/**
 * Breadth-first traversal to collect all descendants (downstream nodes) of the given node.
 * Returns a flat list of node ids reachable via outgoing edges from the root.
 */
export function getDescendants(
  edges: WorkflowEdge[],
  nodeId: string,
  _ctx?: { adj?: Map<string, string[]> }
): string[] {
  const result: string[] = [];
  const visited = new Set<string>();
  const queue: string[] = [];

  // Seed queue with direct children of root
  if (_ctx?.adj) {
    (_ctx.adj.get(nodeId) || []).forEach(id => queue.push(id));
  } else {
    for (const e of edges) {
      if (e.source === nodeId) queue.push(e.target);
    }
  }

  let head = 0;
  while (head < queue.length) {
    const current = queue[head++];
    if (visited.has(current)) continue;
    visited.add(current);
    result.push(current);
    // Enqueue children of current
    for (const e of edges) {
      if (e.source === current && !visited.has(e.target)) {
        queue.push(e.target);
      }
    }
  }

  return result;
}

/**
 * Adjust container layout constraints:
 * - Clamp only nodes whose position changed (avoid squeezing children on parent resize)
 * - If a container parent resized (dimensions change), ensure it is at least
 *   children far-edge + padding (24px in both x/y) so left-bottom resize won't intrude.
 */
export function adjustContainerLayout(
  nextNodes: WorkflowNode[],
  nodeChanges: Array<NodeChange<WorkflowNode>>
): WorkflowNode[] {
  const positionChangedIds = new Set<string>(
    nodeChanges
      .filter(
        (c): c is Extract<NodeChange<WorkflowNode>, { type: 'position'; id: string }> =>
          c.type === 'position' && 'id' in c
      )
      .map(c => c.id)
  );

  // Padding constants (match iteration content paddings and header height where needed)
  const ITER_HEADER_H = 40;
  const PAD_X = 12;
  const PAD_Y = 12;

  const parentById = new Map<string, WorkflowNode>(nextNodes.map(n => [n.id, n]));

  // 1) Clamp only nodes that moved
  const clampedNodes = nextNodes.map(n => {
    const parentId = (n as unknown as { parentId?: string })?.parentId;
    if (!parentId) return n;
    const parent = parentById.get(parentId);
    if (!parent) return n;
    const pType = (parent.data as WorkflowNodeData).type;
    if (!isContainerNode(pType)) return n;
    if (!positionChangedIds.has(n.id)) return n;
    const getDim = (node: WorkflowNode) => {
      const type = (node.data as WorkflowNodeData).type;
      const theme = NODE_THEMES[type] || NODE_THEMES.default;
      return {
        w: node.measured?.width ?? node.width ?? theme.width ?? 280,
        h: node.measured?.height ?? node.height ?? theme.height ?? 120,
      };
    };

    const nodeDim = getDim(n);
    const childW = nodeDim.w;
    const childH = nodeDim.h;

    const parentW = (parent as Partial<WorkflowNode>).width ?? 600;
    const parentH = (parent as Partial<WorkflowNode>).height ?? 420;
    const minX = PAD_X;
    const minY = ITER_HEADER_H + PAD_Y;
    const maxX = Math.max(minX, parentW - childW - PAD_X);
    const maxY = Math.max(minY, parentH - childH - PAD_Y);
    const pos = n.position || { x: 0, y: 0 };
    const nx = Math.min(Math.max(pos.x, minX), maxX);
    const ny = Math.min(Math.max(pos.y, minY), maxY);
    if (nx === pos.x && ny === pos.y) return n;
    return { ...n, position: { x: nx, y: ny } } as WorkflowNode;
  });

  // 2) Ensure iteration parent min size (12px headroom on right/bottom from children)
  const dimensionsChangedIds = new Set<string>(
    nodeChanges
      .filter(
        (c): c is Extract<NodeChange<WorkflowNode>, { type: 'dimensions'; id: string }> =>
          c.type === 'dimensions' && 'id' in c
      )
      .map(c => c.id)
  );

  const adjustedNodes = clampedNodes.map(n => {
    if (!dimensionsChangedIds.has(n.id)) return n;
    const dataType = (n.data as WorkflowNodeData)?.type;
    if (!isContainerNode(dataType)) return n;
    const parentW = (n as Partial<WorkflowNode>).width ?? 600;
    const parentH = (n as Partial<WorkflowNode>).height ?? 420;
    let maxRight = 0;
    let maxBottom = 0;
    for (const child of clampedNodes) {
      const pid = (child as unknown as { parentId?: string })?.parentId;
      if (pid !== n.id) continue;

      const type = (child.data as WorkflowNodeData).type;
      const theme = NODE_THEMES[type] || NODE_THEMES.default;
      const cw = child.measured?.width ?? child.width ?? theme.width ?? 280;
      const ch = child.measured?.height ?? child.height ?? theme.height ?? 120;

      const pos = child.position || { x: 0, y: 0 };
      const right = pos.x + cw;
      const bottom = pos.y + ch;
      if (right > maxRight) maxRight = right;
      if (bottom > maxBottom) maxBottom = bottom;
    }
    // Leave 12px headroom on the right and bottom
    const reqW = Math.max(parentW, maxRight + PAD_X);
    const reqH = Math.max(parentH, maxBottom + PAD_Y);
    if (reqW === parentW && reqH === parentH) return n;
    return { ...n, width: reqW, height: reqH } as WorkflowNode;
  });

  return adjustedNodes;
}

/**
 * Quick mapping of node IDs to their current titles.
 * Used for performance-critical variable token rendering.
 */
export function computeNodeIdToTitle(nodes: WorkflowNode[]): Map<string, string> {
  const map = new Map<string, string>();
  for (let i = 0; i < nodes.length; i++) {
    const n = nodes[i];
    const title = n.data?.title;
    if (title) map.set(n.id, title);
  }
  return map;
}
