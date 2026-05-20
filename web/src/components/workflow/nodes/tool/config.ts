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
  tool_configurations?: Record<string, any>;
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
  // Required parameters from API schema (optional, for full validation)
  requiredParams?: string[];
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

export const checkValid = (data: ToolNodeData, ctx?: ValidationCtx): ValidationResult => {
  const errors: ValidationError[] = [];
  const warnings: ValidationError[] = [];

  if (!data.provider_id || data.provider_id.trim() === '') {
    errors.push({ code: 'tool.validation.providerRequired' });
  }
  if (!data.tool_name || data.tool_name.trim() === '') {
    errors.push({ code: 'tool.validation.toolRequired' });
  }

  // Validate required parameters if schema is provided
  const requiredParams = ctx?.requiredParams || [];
  for (const paramName of requiredParams) {
    const binding = data.tool_parameters?.[paramName];
    if (!binding) {
      errors.push({
        code: 'tool.validation.paramBindingMissing',
        params: { name: paramName },
      });
      continue;
    }

    if (binding.type === 'constant') {
      const v = binding.value;
      if (v === undefined || v === null || (typeof v === 'string' && v.trim() === '')) {
        errors.push({
          code: 'tool.validation.paramValueRequired',
          params: { name: paramName },
        });
      }
    }

    if (binding.type === 'variable') {
      const variable = binding.value;
      const invalid = (() => {
        if (!variable) return true;
        if (Array.isArray(variable)) {
          return variable.length < 2 || !variable[0] || !variable[1];
        }
        return true;
      })();
      if (invalid) {
        errors.push({
          code: 'tool.validation.paramVariableRequired',
          params: { name: paramName },
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

  return { isValid: errors.length === 0, errors, warnings };
};
