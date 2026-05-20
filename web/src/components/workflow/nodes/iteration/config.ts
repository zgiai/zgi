import type { WorkflowVariable } from '../../store/type';
export type IterationPrimitiveType = WorkflowVariable['type'];
export type IterationValueSelector = string[];
export type ErrorHandleMode = 'terminated' | 'continue' | 'drop-error-output';
export interface IterationNodeData {
  type: 'iteration';
  title: string;
  desc: string;
  startNodeType?: string;
  start_node_id: string;
  iteration_id?: string;
  iterator_selector: IterationValueSelector;
  iterator_input_type: IterationPrimitiveType;
  output_selector: IterationValueSelector;
  output_type: IterationPrimitiveType;
  is_parallel: boolean;
  parallel_nums: number;
  error_handle_mode: ErrorHandleMode;
  isInLoop?: boolean;
  isInIteration?: boolean;
  _children: string[];
}
import type { ValidationResult, ValidationError } from '../common/validation';
import type { WorkflowNode } from '../../store/type';

export const DEFAULT_ITERATION_NODE_DATA: IterationNodeData = {
  type: 'iteration',
  title: '迭代',
  desc: '',
  start_node_id: '',
  iterator_selector: [],
  iterator_input_type: 'array[string]' as WorkflowVariable['type'],
  output_selector: [],
  output_type: 'array[string]' as WorkflowVariable['type'],
  is_parallel: false,
  parallel_nums: 10,
  error_handle_mode: 'terminated',
  isInLoop: false,
  isInIteration: false,
  _children: [],
};

interface ValidationCtx {
  nodes?: WorkflowNode[];
}

export const checkValid = (nodeData: IterationNodeData, ctx?: ValidationCtx): ValidationResult => {
  const errors: ValidationError[] = [];
  const warnings: ValidationError[] = [];
  if (!Array.isArray(nodeData.iterator_selector) || nodeData.iterator_selector.length === 0) {
    errors.push({ code: 'iteration.validation.inputRequired' });
  }
  if (!Array.isArray(nodeData.output_selector) || nodeData.output_selector.length === 0) {
    errors.push({ code: 'iteration.validation.outputRequired' });
  }
  const allowed = new Set<string>(['sys', 'conversation', 'environment']);
  if (Array.isArray(nodeData.iterator_selector) && nodeData.iterator_selector.length >= 2) {
    const [sourceId] = nodeData.iterator_selector;
    const hasNode = Array.isArray(ctx?.nodes) ? ctx!.nodes!.some(n => n.id === sourceId) : true;
    if (!allowed.has(sourceId) && !hasNode) {
      warnings.push({ code: 'validation.invalidUpstream' });
    }
  }
  if (Array.isArray(nodeData.output_selector) && nodeData.output_selector.length >= 2) {
    const [sourceId] = nodeData.output_selector;
    const hasNode = Array.isArray(ctx?.nodes) ? ctx!.nodes!.some(n => n.id === sourceId) : true;
    if (!allowed.has(sourceId) && !hasNode) {
      warnings.push({ code: 'validation.invalidUpstream' });
    }
  }
  return { isValid: errors.length === 0, errors, warnings };
};

export function mapToArrayType(base: WorkflowVariable['type']): WorkflowVariable['type'] {
  switch (base) {
    case 'string':
      return 'array[string]';
    case 'number':
      return 'array[number]';
    case 'boolean':
      return 'array[boolean]';
    case 'object':
      return 'array[object]';
    case 'file':
      return 'array[file]';
    case 'array':
    case 'array[string]':
    case 'array[number]':
    case 'array[boolean]':
    case 'array[object]':
    case 'array[file]':
      return base;
    default:
      return 'array[string]';
  }
}
