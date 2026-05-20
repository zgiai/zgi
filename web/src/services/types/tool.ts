// Strict types for builtin tools API reflecting the updated structure

// Localized string with optional keys as API may omit some locales
export interface LocalizedStringPartial {
  en_US?: string;
  zh_Hans?: string;
  pt_BR?: string;
  ja_JP?: string;
  [key: string]: string | undefined;
}

// Tool description with human-readable and LLM-oriented text
export interface ToolDescription {
  human?: LocalizedStringPartial;
  llm?: string;
}

// Tool parameter supported types
export type ToolParameterType =
  | 'string'
  | 'number'
  | 'boolean'
  | 'select'
  | 'secret-input'
  | 'file';

// Option item for select parameter
export interface ToolParameterOption {
  value: string;
  label?: LocalizedStringPartial;
}

// Tool parameter definition
export interface ToolParameter {
  name: string;
  type: ToolParameterType;
  required: boolean;
  // Localized label for display
  label?: LocalizedStringPartial;
  // Human-readable description
  human_description?: LocalizedStringPartial;
  // Legacy description field (some APIs still use this)
  description?: string | null;
  // Form type: 'form' for user input, 'llm' for LLM-provided
  form?: 'form' | 'llm';
  default?: string | number | boolean;
  options?: ToolParameterOption[];
  // Support variable binding in form
  support_variable?: boolean;
  // Help tooltip or text
  help?: LocalizedStringPartial;
}

// Tool configuration parameters (e.g. for credentials)
export interface ToolConfigParameters {
  enable: boolean;
  parameters: ToolParameter[];
}

// Single tool item under a provider entry
export interface BuiltinToolItem {
  // Author of the tool
  author?: string;
  // Tool unique name within provider
  name: string;
  // Localized label for display
  label?: LocalizedStringPartial;
  // Tool description with human and llm variants
  description?: ToolDescription;
  // Parameter schema
  parameters: ToolParameter[];
  // Configuration parameters (e.g. for credentials)
  config_parameters?: ToolConfigParameters;
  // Tool tags
  tags?: string[];
}

// Provider type alias
export type ProviderType = 'builtin' | string;

// Top-level provider entry in builtin tools response (updated schema)
export interface BuiltinToolProvider {
  // Unique id/path for provider, e.g. "zgi/dingtalk/.../dingtalk"
  id: string;
  // Author(s) of the provider
  author?: string;
  // Provider name (often same as id)
  name: string;
  // Localized provider description
  description?: LocalizedStringPartial;
  // Provider icon URL
  icon?: string;
  // Localized provider label
  label?: LocalizedStringPartial;
  // Provider type (e.g. 'builtin')
  type?: ProviderType;
  // Provider tags
  tags?: string[];
  // Authorization and deletion flags
  is_team_authorization?: boolean;
  allow_delete?: boolean;
  // Plugin identifiers
  plugin_id: string | null;
  plugin_unique_identifier: string;
  // Tool list
  tools: BuiltinToolItem[];
  // Additional labels/tags from backend
  labels?: unknown[];
}

// Response payload data type
export type BuiltinToolsResponse = BuiltinToolProvider[];

// Unified value binding type for tool parameters
export type ToolParameterBindingType = 'constant' | 'variable' | 'mixed';

// Tool parameter binding value
export interface ToolParameterBinding {
  type: ToolParameterBindingType;
  // Unified value field for both constant and variable/mixed types.
  // For 'constant', it's a primitive. For 'variable', it's a path array string[]. For 'mixed', it's a string.
  value?: string | number | boolean | string[];
}

// Legacy ValueBinding type for backward compatibility (deprecated, use ToolParameterBinding)
export type BindingMode = 'constant' | 'variable';

export interface ValueBinding<T extends string | number | boolean> {
  mode: BindingMode;
  value?: T;
  variableId?: string[];
}

// Simple normalized form field type for UI mapping (non-UI specific)
export type FormFieldType = 'text' | 'number' | 'checkbox' | 'select' | 'secret' | 'file';

export interface ToolFormField {
  name: string;
  label: string;
  type: FormFieldType;
  required: boolean;
  description?: string;
  default?: string | number | boolean;
  options?: Array<{ value: string; label: string }>;
}
