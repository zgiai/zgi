// System settings category
export interface SystemSettingsCategory {
  category: string;
  category_name: string;
}

// UI component types
export type UIComponentType = 'input' | 'select' | 'switch' | 'textarea' | 'number';

// Validation rule
export interface ValidationRule {
  pattern?: string;
  min?: number;
  max?: number;
  minLength?: number;
  maxLength?: number;
}

// UI option for select component
export interface UIOption {
  label: LocalizedString;
  value: string | number;
}

// Show when condition operators
export type ShowWhenOperator = 'eq' | 'ne' | 'gt' | 'lt' | 'gte' | 'lte' | 'in' | 'contains';

// Show when condition - determines when a config item should be visible
export interface ShowWhenCondition {
  key: string;
  operator: ShowWhenOperator;
  value: string | number | boolean | Array<string | number | boolean>;
}

export interface LocalizedString {
  zh_Hans: string;
  en_US: string;
}

// Config item
export interface SystemConfigItem {
  key: string;
  group: string;
  display_name: LocalizedString;
  description?: LocalizedString;
  value_type: 'string' | 'integer' | 'boolean' | 'float';
  default_value: string | number | boolean;
  is_required: boolean;
  is_sensitive: boolean;
  ui_component: UIComponentType;
  ui_options?: UIOption[];
  placeholder?: LocalizedString;
  help_text?: LocalizedString;
  validation_rule?: ValidationRule;
  sort_order: number;
  show_when?: ShowWhenCondition;
}

// Config group
export interface SystemConfigGroup {
  group_key: string;
  display_name: string;
  description?: string;
  sort_order: number;
}

// Settings metadata
export interface SystemSettingsMetadata {
  category: string;
  display_name: LocalizedString;
  description?: LocalizedString;
  icon?: string;
  groups: SystemConfigGroup[];
  configs: SystemConfigItem[];
}

// Settings with metadata response
export interface SystemSettingsWithMetadata {
  id: string;
  category: string;
  category_name_en_us: string;
  settings: Record<string, unknown>;
  created_at: string;
  updated_at: string;
  updated_by: string;
}

// API responses
export interface SystemSettingsCategoriesResponse {
  categories: SystemSettingsCategory[];
}
