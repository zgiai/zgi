export interface CodeNodeData {
  type: 'code';
  title: string;
  desc: string;
  code: string;
  code_language: 'python3' | 'javascript' | 'json';
  variables: Array<{
    variable: string;
    value_selector: string[];
    value_type:
      | 'string'
      | 'number'
      | 'boolean'
      | 'object'
      | 'file'
      | 'array'
      | 'array[string]'
      | 'array[number]'
      | 'array[boolean]'
      | 'array[object]'
      | 'array[file]';
  }>;
  outputs: Record<
    string,
    {
      type:
        | 'string'
        | 'number'
        | 'boolean'
        | 'object'
        | 'file'
        | 'array'
        | 'array[string]'
        | 'array[number]'
        | 'array[boolean]'
        | 'array[object]'
        | 'array[file]'
        | 'array[array]';
      children: null;
    }
  >;
  outputKeyOrders: string[];
  isInLoop: boolean;
  isInIteration: boolean;
}

export const DEFAULT_CODE_NODE_DATA: CodeNodeData = {
  type: 'code',
  title: 'Code',
  desc: '',
  code: '',
  code_language: 'python3',
  variables: [],
  outputs: {},
  outputKeyOrders: [],
  isInLoop: false,
  isInIteration: false,
};

import type { ValidationResult, ValidationError } from '../common/validation';
import type { WorkflowNode } from '../../store/type';
// Validate Code node data based on business rules
interface ValidationCtx {
  nodes?: WorkflowNode[];
}

export const checkValid = (nodeData: CodeNodeData, ctx?: ValidationCtx): ValidationResult => {
  const errors: ValidationError[] = [];
  const warnings: ValidationError[] = [];

  // 1) code must not be empty
  if (!nodeData.code || nodeData.code.trim() === '') {
    errors.push({ code: 'code.errors.emptyCode' });
  }

  // 2) variables: variable name must not be empty
  for (const v of nodeData.variables || []) {
    if (!v.variable || v.variable.trim() === '') {
      errors.push({ code: 'code.errors.emptyVariableName' });
      break;
    }
  }

  // 3) variables: value_selector must not be empty
  for (const v of nodeData.variables || []) {
    if (!Array.isArray(v.value_selector) || v.value_selector.length === 0) {
      errors.push({ code: 'code.errors.emptyValueSelector' });
      break;
    }
    if (Array.isArray(v.value_selector) && v.value_selector.length >= 2) {
      const [sourceId] = v.value_selector;
      const allowed = new Set<string>(['sys', 'conversation', 'environment']);
      const hasNode = Array.isArray(ctx?.nodes) ? ctx!.nodes!.some(n => n.id === sourceId) : true;
      if (!allowed.has(sourceId) && !hasNode) {
        warnings.push({ code: 'validation.invalidUpstream' });
      }
    }
  }

  return {
    isValid: errors.length === 0,
    errors,
    warnings,
  };
};
