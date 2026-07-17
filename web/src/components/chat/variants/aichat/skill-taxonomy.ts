export const SKILL_CAPABILITY_CATEGORIES = [
  'general_tools',
  'office_productivity',
  'document_processing',
  'data_analysis',
  'knowledge_retrieval',
  'content_creation',
  'planning_decision',
  'workflow_automation',
  'security_compliance',
  'other',
] as const;

export type SkillCapabilityCategory = (typeof SKILL_CAPABILITY_CATEGORIES)[number];

export const SKILL_SCENARIOS = [
  'general',
  'office_collaboration',
  'document_handling',
  'content_creation',
  'data_insights',
  'knowledge_research',
  'planning_decision',
  'business_operations',
  'customer_service',
  'hr_recruiting',
  'legal_compliance',
  'technical_development',
  'other',
] as const;

export type SkillScenario = (typeof SKILL_SCENARIOS)[number];

interface LocalizedTaxonomyLabel {
  'zh-Hans': string;
  'en-US': string;
}

interface SkillTaxonomyMetadata {
  category?: string;
  scenarios?: string[];
}

const CAPABILITY_LABELS: Record<SkillCapabilityCategory, LocalizedTaxonomyLabel> = {
  general_tools: { 'zh-Hans': '通用工具', 'en-US': 'General tools' },
  office_productivity: { 'zh-Hans': '办公效率', 'en-US': 'Office productivity' },
  document_processing: { 'zh-Hans': '文档处理', 'en-US': 'Document processing' },
  data_analysis: { 'zh-Hans': '数据分析', 'en-US': 'Data analysis' },
  knowledge_retrieval: { 'zh-Hans': '知识检索', 'en-US': 'Knowledge retrieval' },
  content_creation: { 'zh-Hans': '内容生成', 'en-US': 'Content creation' },
  planning_decision: { 'zh-Hans': '计划决策', 'en-US': 'Planning and decisions' },
  workflow_automation: { 'zh-Hans': '流程自动化', 'en-US': 'Workflow automation' },
  security_compliance: { 'zh-Hans': '安全合规', 'en-US': 'Security and compliance' },
  other: { 'zh-Hans': '其他', 'en-US': 'Other' },
};

const SCENARIO_LABELS: Record<SkillScenario, LocalizedTaxonomyLabel> = {
  general: { 'zh-Hans': '通用', 'en-US': 'General' },
  office_collaboration: { 'zh-Hans': '办公协作', 'en-US': 'Office collaboration' },
  document_handling: { 'zh-Hans': '文档与资料', 'en-US': 'Documents and files' },
  content_creation: { 'zh-Hans': '内容创作', 'en-US': 'Content creation' },
  data_insights: { 'zh-Hans': '数据洞察', 'en-US': 'Data insights' },
  knowledge_research: { 'zh-Hans': '知识研究', 'en-US': 'Knowledge and research' },
  planning_decision: { 'zh-Hans': '规划与决策', 'en-US': 'Planning and decisions' },
  business_operations: { 'zh-Hans': '业务运营', 'en-US': 'Business operations' },
  customer_service: { 'zh-Hans': '客户服务', 'en-US': 'Customer service' },
  hr_recruiting: { 'zh-Hans': '人才招聘', 'en-US': 'Recruiting' },
  legal_compliance: { 'zh-Hans': '法务合规', 'en-US': 'Legal and compliance' },
  technical_development: { 'zh-Hans': '技术研发', 'en-US': 'Technical development' },
  other: { 'zh-Hans': '其他', 'en-US': 'Other' },
};

const LEGACY_CATEGORY_ALIASES: Record<string, SkillCapabilityCategory> = {
  productivity: 'office_productivity',
  visualization: 'content_creation',
  database: 'data_analysis',
  knowledge: 'knowledge_retrieval',
  creative: 'content_creation',
  orchestration: 'workflow_automation',
  system: 'general_tools',
  security: 'security_compliance',
  general: 'general_tools',
};

const DEFAULT_SCENARIO_BY_CATEGORY: Record<SkillCapabilityCategory, SkillScenario> = {
  general_tools: 'general',
  office_productivity: 'office_collaboration',
  document_processing: 'document_handling',
  data_analysis: 'data_insights',
  knowledge_retrieval: 'knowledge_research',
  content_creation: 'content_creation',
  planning_decision: 'planning_decision',
  workflow_automation: 'business_operations',
  security_compliance: 'legal_compliance',
  other: 'other',
};

const CAPABILITY_SET = new Set<string>(SKILL_CAPABILITY_CATEGORIES);
const SCENARIO_SET = new Set<string>(SKILL_SCENARIOS);

function normalizeTaxonomyId(value: string | undefined): string {
  return value?.trim().toLowerCase() ?? '';
}

function displayLocale(locale: string): keyof LocalizedTaxonomyLabel {
  return locale.toLowerCase().startsWith('zh') ? 'zh-Hans' : 'en-US';
}

export function normalizeSkillCapabilityCategory(
  value: string | undefined
): SkillCapabilityCategory {
  const normalized = normalizeTaxonomyId(value);
  if (CAPABILITY_SET.has(normalized)) return normalized as SkillCapabilityCategory;
  return LEGACY_CATEGORY_ALIASES[normalized] ?? 'other';
}

export function resolveSkillScenarios(display: SkillTaxonomyMetadata | undefined): SkillScenario[] {
  const resolved: SkillScenario[] = [];
  const seen = new Set<SkillScenario>();

  for (const value of display?.scenarios ?? []) {
    const normalized = normalizeTaxonomyId(value);
    if (!normalized) continue;
    const scenario = SCENARIO_SET.has(normalized) ? (normalized as SkillScenario) : 'other';
    if (seen.has(scenario)) continue;
    seen.add(scenario);
    resolved.push(scenario);
  }

  if (resolved.length > 0) return resolved;
  return [DEFAULT_SCENARIO_BY_CATEGORY[normalizeSkillCapabilityCategory(display?.category)]];
}

export function getSkillCapabilityLabel(category: string | undefined, locale: string): string {
  return CAPABILITY_LABELS[normalizeSkillCapabilityCategory(category)][displayLocale(locale)];
}

export function getSkillScenarioLabel(scenario: string | undefined, locale: string): string {
  const normalized = normalizeTaxonomyId(scenario);
  const resolved = SCENARIO_SET.has(normalized) ? (normalized as SkillScenario) : 'other';
  return SCENARIO_LABELS[resolved][displayLocale(locale)];
}
