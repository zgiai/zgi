import type { ValidationResult, ValidationError } from '../common/validation';
import type { WorkflowNode } from '../../store/type';
import { type LoopVariable } from '../loop/config';
import { isNumber } from '@/utils/validation';
import type { InputVar } from '../../types/input-var';

export type WriteMode =
  | 'over-write'
  | 'clear'
  | 'append'
  | 'extend'
  | 'set'
  | '+='
  | '-='
  | '*='
  | '/='
  | 'remove-first'
  | 'remove-last';
export type AssignerNodeInputType = 'variable' | 'constant';
export interface AssignerNodeOperation {
  variable_selector: string[];
  input_type: AssignerNodeInputType;
  operation: WriteMode;
  value?: unknown;
  write_mode?: string;
}
export interface AssignerNodeData {
  type: 'assigner';
  title: string;
  desc: string;
  version?: '1' | '2';
  items: AssignerNodeOperation[];
  isInLoop: boolean;
  isInIteration: boolean;
}

export const DEFAULT_ASSIGNER_NODE_DATA: AssignerNodeData = {
  type: 'assigner',
  title: '变量赋值',
  desc: '',
  version: '2',
  items: [],
  isInLoop: false,
  isInIteration: false,
};

function needsValue(op: AssignerNodeData['items'][number]['operation']): boolean {
  return (
    op === 'set' ||
    op === '+=' ||
    op === '-=' ||
    op === '*=' ||
    op === '/=' ||
    op === 'over-write' ||
    op === 'append' ||
    op === 'extend'
  );
}

interface ValidationCtx {
  nodes?: WorkflowNode[];
}

export const checkValid = (nodeData: AssignerNodeData, ctx?: ValidationCtx): ValidationResult => {
  const errors: ValidationError[] = [];
  const warnings: ValidationError[] = [];

  const items = Array.isArray(nodeData.items) ? nodeData.items : [];
  for (let i = 0; i < items.length; i += 1) {
    const op = items[i];
    // target selector
    if (!Array.isArray(op.variable_selector) || op.variable_selector.length < 2) {
      errors.push({
        code: 'assigner.validation.targetRequired',
        params: { index: i + 1 },
      });
      continue;
    }
    if (Array.isArray(op.variable_selector) && op.variable_selector.length >= 2) {
      const [sourceId] = op.variable_selector;
      const allowed = new Set<string>(['sys', 'conversation', 'environment']);
      const hasNode = Array.isArray(ctx?.nodes) ? ctx!.nodes!.some(n => n.id === sourceId) : true;
      if (!allowed.has(sourceId) && !hasNode) {
        warnings.push({ code: 'validation.invalidUpstream' });
      }
    }
    // value requirement
    if (needsValue(op.operation)) {
      if (op.input_type === 'variable') {
        const val = op.value as unknown;
        if (!Array.isArray(val) || val.length < 2) {
          errors.push({
            code: 'assigner.validation.valueVariableRequired',
            params: { index: i + 1 },
          });
        }
        if (Array.isArray(val) && val.length >= 2) {
          const [sourceId] = val;
          const allowed = new Set<string>(['sys', 'conversation', 'environment']);
          const hasNode = Array.isArray(ctx?.nodes)
            ? ctx!.nodes!.some(n => n.id === sourceId)
            : true;
          if (!allowed.has(sourceId) && !hasNode) {
            warnings.push({ code: 'validation.invalidUpstream' });
          }
        }
      } else {
        if (
          op.value === undefined ||
          op.value === null ||
          (typeof op.value === 'string' && op.value === '')
        ) {
          errors.push({
            code: 'assigner.validation.valueConstantRequired',
            params: { index: i + 1 },
          });
        }
      }
    }
    // math op hint
    if (
      op.operation === '+=' ||
      op.operation === '-=' ||
      op.operation === '*=' ||
      op.operation === '/='
    ) {
      // Validate math operations
      if (op.input_type === 'constant') {
        // For constants, we can strictly check if it's a number
        const isEmpty =
          op.value === undefined ||
          op.value === null ||
          (typeof op.value === 'string' && op.value === '');

        if (!isEmpty && !isNumber(op.value)) {
          errors.push({
            code: 'assigner.validation.mathTypeHint',
            params: { index: i + 1 },
          });
        }
      } else if (op.input_type === 'variable' && Array.isArray(op.value) && ctx?.nodes) {
        // For variables, try to check type if possible (best effort)
        // If we can resolve the variable and confirm it's NOT a number, show error/warning.
        // If we can resolve and confirm it IS a number, show nothing.
        // If we can't resolve, show nothing (assume valid to avoid noise).

        const [sourceId, varName] = op.value as string[];
        const node = ctx.nodes.find(n => n.id === sourceId);
        if (node) {
          const data = node.data;
          let varType: string | undefined;

          // Check Loop variables
          if (data.type === 'loop' && Array.isArray(data.loop_variables)) {
            const v = data.loop_variables.find((lv: LoopVariable) => lv.label === varName);
            if (v) varType = v.var_type;
          }
          // Check Start variables
          else if (data.type === 'start' && Array.isArray(data.variables)) {
            const v = data.variables.find((InputVar: InputVar) => InputVar.variable === varName);
            // Map InputVar type to primitive
            if (v) {
              if (v.type === 'number') varType = 'number';
              else if (
                v.type === 'text-input' ||
                v.type === 'paragraph' ||
                v.type === 'select' ||
                v.type === 'datetime'
              ) {
                varType = 'string';
              }
            }
          }
          // Check Iteration index
          else if (data.type === 'iteration') {
            if (varName === 'index') varType = 'number';
          }

          if (varType && varType !== 'number' && varType !== 'array[number]') {
            // We know for sure it's not a number
            errors.push({
              code: 'assigner.validation.mathTypeHint',
              params: { index: i + 1 },
            });
          }
        }
      }
    }
  }

  return { isValid: errors.length === 0, errors, warnings };
};
