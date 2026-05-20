import type { ValidationResult, ValidationError } from '../common/validation';
import type { WorkflowNode } from '../../store/type';

export type ParameterType =
  | 'string'
  | 'number'
  | 'boolean'
  | 'array[string]'
  | 'array[number]'
  | 'array[boolean]'
  | 'array[object]';

export interface ParameterSchemaItem {
  name: string;
  type: ParameterType;
  required?: boolean;
  description?: string;
  options?: string[];
}

export interface ParameterExtractorNodeData {
  type: 'parameter-extractor';
  title: string;
  desc: string;
  query: string[];
  model: {
    provider: string;
    name: string;
    mode: 'chat' | 'completion';
    completion_params: Record<string, string | number | boolean>;
  };
  reasoning_mode: 'prompt';
  vision: {
    enabled: boolean;
    configs: {
      detail: 'low' | 'high';
      variable_selector: string[];
    };
  };
  instruction: string;
  parameters: ParameterSchemaItem[];
  isInLoop: boolean;
  isInIteration: boolean;
}

export const DEFAULT_PARAMETER_EXTRACTOR_NODE: ParameterExtractorNodeData = {
  type: 'parameter-extractor',
  title: 'parameter-extractor',
  desc: '',
  query: [],
  model: {
    provider: '',
    name: '',
    mode: 'chat',
    completion_params: {},
  },
  reasoning_mode: 'prompt',
  vision: {
    enabled: false,
    configs: {
      detail: 'high',
      variable_selector: [],
    },
  },
  instruction: '',
  parameters: [],
  isInLoop: false,
  isInIteration: false,
};

function isValidName(name: string): boolean {
  if (typeof name !== 'string') return false;
  if (name.length === 0) return false;
  return /^[A-Za-z][A-Za-z0-9_]*$/.test(name);
}

interface ValidationCtx {
  nodes?: WorkflowNode[];
}

export const checkValid = (
  nodeData: ParameterExtractorNodeData,
  ctx?: ValidationCtx
): ValidationResult => {
  const errors: ValidationError[] = [];
  const warnings: ValidationError[] = [];

  if (
    !Array.isArray(nodeData.query) ||
    nodeData.query.length < 2 ||
    typeof nodeData.query[0] !== 'string' ||
    typeof nodeData.query[1] !== 'string'
  ) {
    errors.push({ code: 'parameterExtractor.validation.queryRequired' });
  }
  if (Array.isArray(nodeData.query) && nodeData.query.length >= 2) {
    const [sourceId] = nodeData.query;
    const allowed = new Set<string>(['sys', 'conversation', 'environment']);
    const hasNode = Array.isArray(ctx?.nodes) ? ctx!.nodes!.some(n => n.id === sourceId) : true;
    if (!allowed.has(sourceId) && !hasNode) {
      warnings.push({ code: 'validation.invalidUpstream' });
    }
  }
  if (!nodeData.instruction || nodeData.instruction.trim().length === 0) {
    errors.push({ code: 'parameterExtractor.validation.instructionRequired' });
  }

  const params = Array.isArray(nodeData.parameters) ? nodeData.parameters : [];
  if (params.length === 0) {
    warnings.push({ code: 'parameterExtractor.validation.noParametersDefined' });
  }

  // vision variable_selector upstream check
  if (nodeData.vision?.enabled && Array.isArray(nodeData.vision.configs?.variable_selector)) {
    const vs = nodeData.vision.configs.variable_selector;
    if (vs.length >= 2) {
      const [sourceId] = vs;
      const allowed = new Set<string>(['sys', 'conversation', 'environment']);
      const hasNode = Array.isArray(ctx?.nodes) ? ctx!.nodes!.some(n => n.id === sourceId) : true;
      if (!allowed.has(sourceId) && !hasNode) {
        warnings.push({ code: 'validation.invalidUpstream' });
      }
    }
  }
  const seen = new Set<string>();
  for (const p of params) {
    if (!isValidName(p.name)) {
      errors.push({
        code: 'parameterExtractor.validation.invalidParamName',
        params: { name: p.name || '' },
      });
      continue;
    }
    if (seen.has(p.name)) {
      errors.push({
        code: 'parameterExtractor.validation.duplicateParamName',
        params: { name: p.name },
      });
      continue;
    }
    seen.add(p.name);
    const validTypes: Array<ParameterSchemaItem['type']> = [
      'string',
      'number',
      'boolean',
      'array[string]',
      'array[number]',
      'array[boolean]',
      'array[object]',
    ];
    if (!validTypes.includes(p.type)) {
      errors.push({
        code: 'parameterExtractor.validation.invalidParamType',
        params: { name: p.name },
      });
    }
  }

  return { isValid: errors.length === 0, errors, warnings };
};
