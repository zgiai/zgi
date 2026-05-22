/**
 * Input variable types for workflow nodes
 */

// Input variable type enumeration
export enum InputVarType {
  TEXT_INPUT = 'text-input',
  PARAGRAPH = 'paragraph',
  SELECT = 'select',
  NUMBER = 'number',
  CHECKBOX = 'checkbox',
  FILE = 'file',
  FILE_LIST = 'file-list',
}

export const ALLOWED_FILE_TYPES = ['image', 'audio', 'video', 'document'] as const;
export type WorkflowFileType = (typeof ALLOWED_FILE_TYPES)[number];

// Value selector for variable references
export interface ValueSelector {
  variable: string;
  value_selector: string[];
}

// Primitive type union for structured type fields
export type StructuredFieldType =
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

// Sub-field definition for structured types (file, aggregator group output, json-parser nested fields)
export interface StructuredTypeField {
  key: string;
  type: StructuredFieldType;
  descriptionKey?: NodesSuffix;
  isEnum?: boolean;
  options?: string[];
  /** Nested sub-fields for recursive structures (e.g., json-parser nested objects) */
  children?: StructuredTypeField[];
}

// File type fixed sub-fields (type, size, name, url, extension, mime_type)
export const FILE_TYPE_FIELDS: StructuredTypeField[] = [
  {
    key: 'type',
    type: 'string',
    descriptionKey: 'structuredFields.file.type',
    isEnum: true,
    options: [...ALLOWED_FILE_TYPES],
  },
  { key: 'size', type: 'number', descriptionKey: 'structuredFields.file.size' },
  { key: 'name', type: 'string', descriptionKey: 'structuredFields.file.name' },
  { key: 'url', type: 'string', descriptionKey: 'structuredFields.file.url' },
  { key: 'extension', type: 'string', descriptionKey: 'structuredFields.file.extension' },
  { key: 'mime_type', type: 'string', descriptionKey: 'structuredFields.file.mime_type' },
];

// Variable aggregator group output fixed sub-field
// The actual type of 'output' is dynamic (inherited from group's output_type)
export const AGGREGATOR_GROUP_FIELDS: StructuredTypeField[] = [
  { key: 'output', type: 'string', descriptionKey: 'structuredFields.aggregator.output' },
];

/**
 * Get sub-fields for a structured variable type.
 * Returns null for types that don't have expandable sub-fields.
 *
 * @param type - The variable type (e.g., 'file', 'object')
 * @param isAggregatorGroup - True if this is a variable-aggregator group output
 * @param innerType - For aggregator groups, the inner output type
 */
export function getStructuredTypeFields(
  type: string,
  isAggregatorGroup?: boolean,
  innerType?: string
): StructuredTypeField[] | null {
  // File type has fixed sub-fields
  if (type === 'file') {
    return FILE_TYPE_FIELDS;
  }

  // Variable aggregator group mode outputs an object with 'output' field
  if (type === 'object' && isAggregatorGroup) {
    // Return with the actual inner type
    return AGGREGATOR_GROUP_FIELDS.map(f => ({
      ...f,
      type: (innerType as StructuredFieldType) || 'string',
    }));
  }

  // Other types don't have expandable sub-fields
  return null;
}

// Upload file settings
export interface UploadFileSetting {
  allowed_file_upload_methods?: string[];
  allowed_file_types?: string[];
  allowed_file_extensions?: string[];
  max_file_size?: number;
}

// Input variable definition
export interface InputVar {
  type: InputVarType;
  variable: string;
  label: string;
  description?: string;
  max_length?: number;
  default?: string | boolean;
  required: boolean;
  options?: string[];
  value_selector?: ValueSelector;
  getVarValueFromDependent?: boolean;
  hide?: boolean;
  isFileItem?: boolean;
  allowed_file_upload_methods?: string[];
  allowed_file_types?: string[];
  allowed_file_extensions?: string[];
}

// System built-in variables
export interface SystemVariable {
  key: string;
  label: NodesSuffix;
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
  description: NodesSuffix;
  chatModeOnly?: boolean;
}

// System built-in variables definition
// Common variables available in all workflow types
// Chat-only variables are marked with chatModeOnly: true
export const SYSTEM_VARIABLES: SystemVariable[] = [
  // Common system variables (all workflow types)
  {
    key: 'sys.tenant_id',
    label: 'systemVariables.tenant_id.label',
    type: 'string',
    description: 'systemVariables.tenant_id.description',
  },
  {
    key: 'sys.user_id',
    label: 'systemVariables.user_id.label',
    type: 'string',
    description: 'systemVariables.user_id.description',
  },
  {
    key: 'sys.agent_id',
    label: 'systemVariables.agent_id.label',
    type: 'string',
    description: 'systemVariables.agent_id.description',
  },
  {
    key: 'sys.workflow_id',
    label: 'systemVariables.workflow_id.label',
    type: 'string',
    description: 'systemVariables.workflow_id.description',
  },
  {
    key: 'sys.workflow_run_id',
    label: 'systemVariables.workflow_run_id.label',
    type: 'string',
    description: 'systemVariables.workflow_run_id.description',
  },
  {
    key: 'sys.workflow_type',
    label: 'systemVariables.workflow_type.label',
    type: 'string',
    description: 'systemVariables.workflow_type.description',
  },
  // Chat workflow exclusive variables
  {
    key: 'sys.query',
    label: 'systemVariables.query.label',
    type: 'string',
    description: 'systemVariables.query.description',
    chatModeOnly: true,
  },
  {
    key: 'sys.files',
    label: 'systemVariables.files.label',
    type: 'array[file]',
    description: 'systemVariables.files.description',
    chatModeOnly: true,
  },
  {
    key: 'sys.conversation_id',
    label: 'systemVariables.conversation_id.label',
    type: 'string',
    description: 'systemVariables.conversation_id.description',
    chatModeOnly: true,
  },
  {
    key: 'sys.dialogue_count',
    label: 'systemVariables.dialogue_count.label',
    type: 'number',
    description: 'systemVariables.dialogue_count.description',
    chatModeOnly: true,
  },
];

import { type NodesSuffix } from '@/i18n';
import { filterLowercaseExtensions } from '@/utils/file-helpers';
import { type ValidationError } from '../nodes/common/validation';
// Helper: filter system variables by agent type (chatModeOnly awareness)
import { AgentType } from '@/services/types/agent';
export const getSystemVariablesForAgent = (agentType: AgentType) => {
  const isConversational = agentType === AgentType.CONVERSATIONAL_AGENT;
  return SYSTEM_VARIABLES.filter(v => (isConversational ? true : !v.chatModeOnly));
};

// Translator function type used outside components
type Translator = (key: string, values?: Record<string, string | number | Date>) => string;

// Helper functions for input variable validation
export const validateInputVar = (inputVar: InputVar): ValidationError[] => {
  const errors: ValidationError[] = [];

  if (!inputVar.variable || inputVar.variable.trim() === '') {
    errors.push({ code: 'start.validation.variableNameRequired' });
  }

  if (
    inputVar.type === InputVarType.SELECT &&
    (!inputVar.options || inputVar.options.length === 0)
  ) {
    errors.push({ code: 'start.validation.selectNeedsOptions' });
  }

  if (
    inputVar.type === InputVarType.NUMBER &&
    inputVar.default &&
    typeof inputVar.default === 'string' &&
    isNaN(Number(inputVar.default))
  ) {
    errors.push({ code: 'start.validation.defaultMustBeNumber' });
  }

  if (inputVar.max_length && inputVar.max_length <= 0) {
    errors.push({ code: 'start.validation.maxLengthGtZero' });
  }

  if (inputVar.type === InputVarType.FILE || inputVar.type === InputVarType.FILE_LIST) {
    const allowedTypes = inputVar.allowed_file_types ?? [];
    if (allowedTypes.length === 0) {
      errors.push({ code: 'start.validation.fileTypeRequired' });
    }

    if (
      allowedTypes.includes('custom') &&
      filterLowercaseExtensions(inputVar.allowed_file_extensions ?? []).length === 0
    ) {
      errors.push({ code: 'start.validation.customExtensionsRequired' });
    }
  }

  return errors;
};

// Helper function to get input variable type label
export const getInputVarTypeLabel = (type: InputVarType, t?: Translator): string => {
  if (t) {
    // Use i18n when translator is provided (nodes namespace already bound)
    return t(`start.types.${type}`);
  }
  // Fallback to English labels when translator is not provided
  const labels: Record<InputVarType, string> = {
    [InputVarType.TEXT_INPUT]: 'Text Input',
    [InputVarType.PARAGRAPH]: 'Paragraph',
    [InputVarType.SELECT]: 'Select',
    [InputVarType.NUMBER]: 'Number',
    [InputVarType.CHECKBOX]: 'Checkbox',
    [InputVarType.FILE]: 'File',
    [InputVarType.FILE_LIST]: 'File List',
  };
  return labels[type] || type;
};

// Helper function to create default input variable
export const createDefaultInputVar = (type: InputVarType = InputVarType.TEXT_INPUT): InputVar => {
  const baseVar = {
    type,
    variable: '',
    label: '',
    required: false,
  };

  if (type === InputVarType.CHECKBOX) {
    return {
      ...baseVar,
      default: false,
    };
  }

  return {
    ...baseVar,
  };
};
