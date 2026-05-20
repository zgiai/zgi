import type { WorkflowVariable, PromptConfig, JSONSchema } from '../../store/type';
import type { ValidationResult, ValidationError } from '../common/validation';
import type { PromptSource } from '@/services/types/prompt';

// Vision configuration types for LLM node
export interface LLMVisionConfig {
  detail: 'high' | 'low';
  variable_selector: string[]; // [sourceId, key, ...]
}

export interface LLMVision {
  enabled: boolean;
  configs?: LLMVisionConfig;
}

export interface LLMManagedPromptReference {
  prompt_id: string;
  prompt_name?: string;
  version?: number;
  label?: string;
  locale?: string;
  source?: PromptSource;
}

export interface LLMNodeData {
  type: 'llm';
  title: string;
  desc: string;
  variables: WorkflowVariable[];
  model: {
    provider: string;
    name: string;
    mode: 'chat' | 'completion';
    completion_params: Record<string, string | number | boolean>;
  };
  prompt_template: Array<{
    role: 'system' | 'user' | 'assistant';
    text: string;
    id?: string;
  }>;
  prompt_source?: 'inline' | 'managed';
  prompt_reference?: LLMManagedPromptReference;
  prompt_config: PromptConfig;
  vision: LLMVision;
  structured_output_enabled: boolean;
  structured_output?: { schema: JSONSchema };
  reasoning_format?: 'tagged' | 'plain';
  isInLoop: boolean;
  isInIteration: boolean;
}

export const DEFAULT_LLM_NODE_DATA: LLMNodeData = {
  type: 'llm',
  title: 'LLM',
  desc: '',
  variables: [],
  model: {
    provider: '',
    name: '',
    mode: 'chat',
    completion_params: {},
  },
  prompt_template: [
    {
      role: 'system' as const,
      text: '',
    },
  ],
  prompt_source: 'inline',
  prompt_config: {
    jinja2_variables: [],
  },
  vision: {
    enabled: false,
  },
  structured_output_enabled: false,
  reasoning_format: 'tagged',
  isInLoop: false,
  isInIteration: false,
};

export const checkValid = (nodeData: LLMNodeData): ValidationResult => {
  const errors: ValidationError[] = [];
  const warnings: ValidationError[] = [];

  if (!nodeData.model.provider || !nodeData.model.name) {
    errors.push({ code: 'llm.validation.modelRequired' });
  }
  if (nodeData.prompt_source === 'managed') {
    if (!nodeData.prompt_reference?.prompt_id) {
      errors.push({ code: 'llm.validation.promptRequired' });
    }
  } else if (nodeData.prompt_template.length === 0 || !nodeData.prompt_template[0].text) {
    // Treat missing prompt as an error to prevent publish
    errors.push({ code: 'llm.validation.promptRequired' });
  }

  // Vision validation: when enabled, variable_selector is required
  if (nodeData.vision?.enabled) {
    const selector = nodeData.vision.configs?.variable_selector;
    if (!selector || selector.length < 2 || !selector[0] || !selector[1]) {
      errors.push({ code: 'llm.validation.visionVariableRequired' });
    }
  }

  return { isValid: errors.length === 0, errors, warnings };
};
