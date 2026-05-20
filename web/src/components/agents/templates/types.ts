export const TEMPLATE_CATEGORY_IDS = [
  'recommended',
  'starter',
  'standard',
  'advanced',
  'enterprise',
  'document-intake',
  'knowledge-service',
  'data-systems',
  'integration-automation',
  'governance',
] as const;

export type AgentTemplateCategoryId = (typeof TEMPLATE_CATEGORY_IDS)[number];

export const TEMPLATE_COMPLEXITY_IDS = ['starter', 'standard', 'advanced', 'enterprise'] as const;

export type AgentTemplateComplexity = (typeof TEMPLATE_COMPLEXITY_IDS)[number];

export type AgentTemplateKind = 'chatflow' | 'workflow' | 'agent';

export type AgentTemplateKindFilter = 'all' | AgentTemplateKind;

export type AgentTemplateLocale = 'en-US' | 'zh-Hans';

export type AgentTemplateRuntimeStatus = 'ready' | 'requires-setup';

export type AgentTemplateRequirementId =
  | 'default-model'
  | 'file-input'
  | 'knowledge-base'
  | 'database'
  | 'http-endpoint'
  | 'approval-review'
  | 'scheduled-notification'
  | 'sms-channel'
  | 'code-sandbox'
  | 'image-model';

export interface AgentTemplateCategory {
  id: AgentTemplateCategoryId;
}

export interface AgentTemplateCategorySection {
  id: 'primary' | 'complexity' | 'capability';
  labelKey?: string;
  categories: AgentTemplateCategory[];
}

export interface AgentTemplatePromptReference {
  promptIdsByLocale: Partial<Record<AgentTemplateLocale, string>>;
  fallbackTitle: string;
}

export interface AgentTemplatePromptBinding extends AgentTemplatePromptReference {
  nodeIds: string[];
}

export interface AgentTemplate {
  id: string;
  copyKey: string;
  fallbackTitle: string;
  fallbackDescription: string;
  kind: AgentTemplateKind;
  complexity: AgentTemplateComplexity;
  runtimeStatus: AgentTemplateRuntimeStatus;
  requirements: AgentTemplateRequirementId[];
  yamlPath: string;
  localizedYamlPaths?: Partial<Record<AgentTemplateLocale, string>>;
  recommended?: boolean;
  categories: Array<Exclude<AgentTemplateCategoryId, 'recommended'>>;
  tags: string[];
  iconLabel: string;
  recommendedPrompts?: AgentTemplatePromptReference[];
  defaultPromptBindings?: AgentTemplatePromptBinding[];
}
