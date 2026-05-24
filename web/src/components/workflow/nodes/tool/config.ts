import type {
  ToolParameterBinding,
  ToolParameterBindingType,
  ProviderType,
} from '@/services/types/tool';
import type { ValidationResult, ValidationError } from '../common/validation';
import type { WorkflowNode } from '../../store/type';

/**
 * Tool node data definition
 * - Represents invoking a builtin tool from a provider
 * - Parameters are bound via ToolParameterBinding (constant or variable)
 */
export interface ToolNodeData {
  type: 'tools';
  title: string;
  desc: string;
  // Provider type, e.g. 'builtin'
  provider_type: ProviderType;
  // Provider id, e.g. 'time', 'github'
  provider_id: string;
  // Tool name within the provider, e.g. 'current_time', 'getIssue'
  tool_name: string;
  // Tool parameters with bindings keyed by parameter name
  tool_parameters: Record<string, ToolParameterBinding>;
  // Tool configuration parameters (e.g. credentials)
  tool_configurations?: Record<string, unknown>;
  isInLoop: boolean;
  isInIteration: boolean;
}

// Re-export for convenience
export type { ToolParameterBinding, ToolParameterBindingType, ProviderType };

/**
 * Validate Tool node configuration
 * - provider_id and tool_name must be set
 * - This validation is basic; full parameter validation requires API schema
 */
interface ValidationCtx {
  nodes?: WorkflowNode[];
  // Required fields from API schema (optional, for full validation)
  requiredParams?: ToolRequiredField[];
  requiredConfigurations?: ToolRequiredField[];
}

export interface ToolRequiredField {
  name: string;
  label?: string;
  type?: 'string' | 'number' | 'boolean' | 'select' | 'secret-input' | 'file';
}

/** Default tool node data */
export const DEFAULT_TOOL_NODE_DATA: ToolNodeData = {
  type: 'tools',
  title: 'Tool',
  desc: '',
  provider_type: 'builtin',
  provider_id: '',
  tool_name: '',
  tool_parameters: {},
  isInLoop: false,
  isInIteration: false,
};

const getFieldLabel = (field: ToolRequiredField) => field.label || field.name;

const isEmptyConstant = (value: unknown) =>
  value === undefined || value === null || (typeof value === 'string' && value.trim() === '');

const isInvalidVariable = (value: unknown) => {
  if (!value) return true;
  if (Array.isArray(value)) return value.length < 2 || !value[0] || !value[1];
  return true;
};

export const checkValid = (data: ToolNodeData, ctx?: ValidationCtx): ValidationResult => {
  const errors: ValidationError[] = [];
  const warnings: ValidationError[] = [];

  if (!data.provider_id || data.provider_id.trim() === '') {
    errors.push({ code: 'tool.validation.providerRequired' });
  }
  if (!data.tool_name || data.tool_name.trim() === '') {
    errors.push({ code: 'tool.validation.toolRequired' });
  }

  for (const field of ctx?.requiredParams || []) {
    const binding = data.tool_parameters?.[field.name];
    const label = getFieldLabel(field);
    if (!binding) {
      errors.push({
        code: 'tool.validation.paramBindingMissing',
        params: { name: label },
      });
      continue;
    }

    if (binding.type === 'constant' || binding.type === 'mixed') {
      if (isEmptyConstant(binding.value)) {
        errors.push({ code: 'tool.validation.paramValueRequired', params: { name: label } });
      }
    }

    if (binding.type === 'variable') {
      const variable = binding.value;
      if (field.type === 'string' && isInvalidVariable(variable)) {
        errors.push({ code: 'tool.validation.paramValueRequired', params: { name: label } });
      } else if (isInvalidVariable(variable)) {
        errors.push({
          code: 'tool.validation.paramVariableRequired',
          params: { name: label },
        });
      }
      // Upstream missing warning when tuple provided
      if (Array.isArray(variable) && variable.length >= 2) {
        const [sourceId] = variable as string[];
        const allowed = new Set<string>(['sys', 'conversation', 'environment']);
        const hasNode = Array.isArray(ctx?.nodes) ? ctx?.nodes.some(n => n.id === sourceId) : true;
        if (!allowed.has(sourceId) && !hasNode) {
          warnings.push({ code: 'validation.invalidUpstream' });
        }
      }
    }
  }

  for (const field of ctx?.requiredConfigurations || []) {
    const value = data.tool_configurations?.[field.name];
    if (isEmptyConstant(value)) {
      errors.push({
        code: 'tool.validation.paramValueRequired',
        params: { name: getFieldLabel(field) },
      });
    }
  }

  return { isValid: errors.length === 0, errors, warnings };
};
