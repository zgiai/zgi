export interface OutputVariable {
  variable: string;
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
    | 'array[file]';
  value_selector: string[];
}
export interface EndNodeData {
  type: 'end';
  title: string;
  desc: string;
  outputs: OutputVariable[];
  isInLoop: boolean;
  isInIteration: boolean;
}
import type { ValidationResult, ValidationError } from '../common/validation';
import type { WorkflowNode } from '../../store/type';

interface ValidationCtx {
  nodes?: WorkflowNode[];
}

export const DEFAULT_END_NODE_DATA: EndNodeData = {
  type: 'end',
  title: 'End',
  desc: '',
  outputs: [],
  isInLoop: false,
  isInIteration: false,
};

export const checkValid = (data: EndNodeData, ctx?: ValidationCtx): ValidationResult => {
  const errors: ValidationError[] = [];
  const warnings: ValidationError[] = [];

  if (!Array.isArray(data.outputs) || data.outputs.length === 0) {
    errors.push({ code: 'end.validation.outputsRequired' });
  } else {
    data.outputs.forEach((o, idx) => {
      if (!o.variable || o.variable.trim() === '') {
        errors.push({
          code: 'end.validation.outputVariableRequired',
          params: { index: idx + 1 },
        });
      }
      if (!Array.isArray(o.value_selector) || o.value_selector.length < 2) {
        errors.push({
          code: 'end.validation.outputSelectorRequired',
          params: { index: idx + 1 },
        });
      }
      // upstream missing warning
      if (Array.isArray(o.value_selector) && o.value_selector.length >= 2) {
        const [sourceId] = o.value_selector;
        const allowed = new Set<string>(['sys', 'conversation', 'environment']);
        const hasNode = Array.isArray(ctx?.nodes) ? ctx!.nodes!.some(n => n.id === sourceId) : true;
        if (!allowed.has(sourceId) && !hasNode) {
          warnings.push({ code: 'validation.invalidUpstream' });
        }
      }
    });
  }

  return { isValid: errors.length === 0, errors, warnings };
};
