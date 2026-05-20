import type { ValidationResult, ValidationError } from '../common/validation';

/**
 * Supported output types for JSON Parser node
 */
export type JsonParserOutputType =
  | 'string'
  | 'number'
  | 'boolean'
  | 'object'
  | 'array[string]'
  | 'array[number]'
  | 'array[boolean]'
  | 'array[object]';

/**
 * Output schema definition with recursive children support
 */
export interface JsonParserOutputSchema {
  type: JsonParserOutputType;
  /** Nested children for object and array[object] types */
  children?: Record<string, JsonParserOutputSchema>;
}

/**
 * Error handling strategy for JSON Parser node
 * - 'none': Returns FAILED status on any error (default)
 * - 'fail-branch': Routes to the fail branch edge; status is EXCEPTION
 * - 'default-value': Uses values from default_value list; status is EXCEPTION
 */
export type JsonParserErrorStrategy = 'none' | 'fail-branch' | 'default-value';

/**
 * Default value item for error handling
 */
export interface JsonParserDefaultValueItem {
  key: string;
  value: string;
  type: JsonParserOutputType;
}

/**
 * Retry configuration for JSON Parser node
 */
export interface JsonParserRetryConfig {
  enable: boolean;
  max_times: number;
  interval: number;
}

/**
 * JSON Parser node data structure
 */
export interface JsonParserNodeData {
  type: 'json-parser';
  title: string;
  desc: string;
  /** Path to the upstream variable, e.g. ['http_request_1', 'body'] */
  input_selector: string[];
  /** When true, each top-level key in outputs is exposed as separate variable */
  is_flatten_output: boolean;
  /** Output schema definition */
  outputs: Record<string, JsonParserOutputSchema>;
  /** Error handling strategy */
  error_strategy?: JsonParserErrorStrategy;
  /** Default values for error handling (when error_strategy is 'default-value') */
  default_value?: JsonParserDefaultValueItem[];
  /** Retry configuration */
  retry_config?: JsonParserRetryConfig;
  isInLoop: boolean;
  isInIteration: boolean;
}

/**
 * Default JSON Parser node configuration
 */
export const DEFAULT_JSON_PARSER_NODE: JsonParserNodeData = {
  type: 'json-parser',
  title: 'JSON Parser',
  desc: '',
  input_selector: [],
  is_flatten_output: false,
  outputs: {
    result: {
      type: 'object',
      children: {},
    },
  },
  error_strategy: 'none',
  default_value: [],
  retry_config: {
    enable: false,
    max_times: 3,
    interval: 1000,
  },
  isInLoop: false,
  isInIteration: false,
};

/**
 * Valid output types for validation
 */
const VALID_OUTPUT_TYPES: JsonParserOutputType[] = [
  'string',
  'number',
  'boolean',
  'object',
  'array[string]',
  'array[number]',
  'array[boolean]',
  'array[object]',
];

/**
 * Validate output key name (must be valid identifier)
 */
function isValidOutputKey(key: string): boolean {
  if (typeof key !== 'string' || key.length === 0) return false;
  return /^[A-Za-z_][A-Za-z0-9_]*$/.test(key);
}

/**
 * Recursively validate output schema
 */
function validateOutputSchema(
  schema: JsonParserOutputSchema,
  path: string,
  errors: ValidationError[]
): void {
  if (!VALID_OUTPUT_TYPES.includes(schema.type)) {
    errors.push({
      code: 'jsonParser.validation.invalidOutputType',
      params: { path, type: schema.type },
    });
    return;
  }

  // Validate children for object and array[object] types
  if ((schema.type === 'object' || schema.type === 'array[object]') && schema.children) {
    const childKeys = Object.keys(schema.children);
    const seen = new Set<string>();

    for (const childKey of childKeys) {
      if (!isValidOutputKey(childKey)) {
        errors.push({
          code: 'jsonParser.validation.invalidOutputKey',
          params: { key: childKey, path },
        });
        continue;
      }

      if (seen.has(childKey)) {
        errors.push({
          code: 'jsonParser.validation.duplicateOutputKey',
          params: { key: childKey, path },
        });
        continue;
      }
      seen.add(childKey);

      const childSchema = schema.children[childKey];
      if (childSchema) {
        validateOutputSchema(childSchema, `${path}.${childKey}`, errors);
      }
    }
  }
}

/**
 * Validate JSON Parser node configuration
 */
export const checkValid = (nodeData: JsonParserNodeData): ValidationResult => {
  const errors: ValidationError[] = [];
  const warnings: ValidationError[] = [];

  // Validate input_selector is configured
  if (
    !Array.isArray(nodeData.input_selector) ||
    nodeData.input_selector.length < 2 ||
    !nodeData.input_selector[0] ||
    !nodeData.input_selector[1]
  ) {
    errors.push({ code: 'jsonParser.validation.inputRequired' });
  }

  // Validate outputs are defined
  const outputKeys = Object.keys(nodeData.outputs || {});
  if (outputKeys.length === 0) {
    errors.push({ code: 'jsonParser.validation.outputsRequired' });
  }

  // Validate each output schema
  const seenKeys = new Set<string>();
  for (const key of outputKeys) {
    if (!isValidOutputKey(key)) {
      errors.push({
        code: 'jsonParser.validation.invalidOutputKey',
        params: { key, path: 'outputs' },
      });
      continue;
    }

    if (seenKeys.has(key)) {
      errors.push({
        code: 'jsonParser.validation.duplicateOutputKey',
        params: { key, path: 'outputs' },
      });
      continue;
    }
    seenKeys.add(key);

    const schema = nodeData.outputs[key];
    if (schema) {
      validateOutputSchema(schema, key, errors);
    }
  }

  // Validate flatten mode constraints
  if (nodeData.is_flatten_output && outputKeys.length === 0) {
    warnings.push({ code: 'jsonParser.validation.flattenNoOutputs' });
  }

  // Validate error strategy
  if (
    nodeData.error_strategy &&
    !['none', 'fail-branch', 'default-value'].includes(nodeData.error_strategy)
  ) {
    errors.push({ code: 'jsonParser.validation.invalidErrorStrategy' });
  }

  // Validate default values when error_strategy is 'default-value'
  if (nodeData.error_strategy === 'default-value') {
    if (!nodeData.default_value || nodeData.default_value.length === 0) {
      warnings.push({ code: 'jsonParser.validation.defaultValueRequired' });
    }
  }

  return { isValid: errors.length === 0, errors, warnings };
};
