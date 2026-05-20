import type {
  AgentTemplate,
  AgentTemplateCategoryId,
  AgentTemplateComplexity,
  AgentTemplateKind,
  AgentTemplateKindFilter,
  AgentTemplateRequirementId,
  AgentTemplateRuntimeStatus,
} from './types';

export type TemplateTranslator = (key: string, values?: Record<string, unknown>) => string;

const KIND_FILTER_ORDER: AgentTemplateKindFilter[] = ['all', 'chatflow', 'workflow', 'agent'];

const KIND_LABEL_KEYS: Record<AgentTemplateKind, string> = {
  chatflow: 'agents.templates.kinds.chatflow',
  workflow: 'agents.templates.kinds.workflow',
  agent: 'agents.templates.kinds.agent',
};

const COMPLEXITY_LABEL_KEYS: Record<AgentTemplateComplexity, string> = {
  starter: 'agents.templates.complexity.starter',
  standard: 'agents.templates.complexity.standard',
  advanced: 'agents.templates.complexity.advanced',
  enterprise: 'agents.templates.complexity.enterprise',
};

const RUNTIME_STATUS_LABEL_KEYS: Record<AgentTemplateRuntimeStatus, string> = {
  ready: 'agents.templates.runtime.ready',
  'requires-setup': 'agents.templates.runtime.requiresSetup',
};

const REQUIREMENT_LABEL_KEYS: Record<AgentTemplateRequirementId, string> = {
  'default-model': 'agents.templates.requirements.defaultModel',
  'file-input': 'agents.templates.requirements.fileInput',
  'knowledge-base': 'agents.templates.requirements.knowledgeBase',
  database: 'agents.templates.requirements.database',
  'http-endpoint': 'agents.templates.requirements.httpEndpoint',
  'approval-review': 'agents.templates.requirements.approvalReview',
  'scheduled-notification': 'agents.templates.requirements.scheduledNotification',
  'sms-channel': 'agents.templates.requirements.smsChannel',
  'code-sandbox': 'agents.templates.requirements.codeSandbox',
  'image-model': 'agents.templates.requirements.imageModel',
};

const REQUIREMENT_SETUP_HINT_KEYS: Record<AgentTemplateRequirementId, string> = {
  'default-model': 'agents.templates.setupHints.defaultModel',
  'file-input': 'agents.templates.setupHints.fileInput',
  'knowledge-base': 'agents.templates.setupHints.knowledgeBase',
  database: 'agents.templates.setupHints.database',
  'http-endpoint': 'agents.templates.setupHints.httpEndpoint',
  'approval-review': 'agents.templates.setupHints.approvalReview',
  'scheduled-notification': 'agents.templates.setupHints.scheduledNotification',
  'sms-channel': 'agents.templates.setupHints.smsChannel',
  'code-sandbox': 'agents.templates.setupHints.codeSandbox',
  'image-model': 'agents.templates.setupHints.imageModel',
};

const CATEGORY_LABEL_KEYS: Record<AgentTemplateCategoryId, string> = {
  recommended: 'agents.templates.recommended',
  starter: 'agents.templates.categories.starter',
  standard: 'agents.templates.categories.standard',
  advanced: 'agents.templates.categories.advanced',
  enterprise: 'agents.templates.categories.enterprise',
  'document-intake': 'agents.templates.categories.documentIntake',
  'knowledge-service': 'agents.templates.categories.knowledgeService',
  'data-systems': 'agents.templates.categories.dataSystems',
  'integration-automation': 'agents.templates.categories.integrationAutomation',
  governance: 'agents.templates.categories.governance',
};

export function normalizeSearchValue(value: string): string {
  return value.trim().toLowerCase();
}

export function getAvailableTemplateKindFilters(
  templates: AgentTemplate[]
): AgentTemplateKindFilter[] {
  const availableKinds = new Set(templates.map(template => template.kind));
  return KIND_FILTER_ORDER.filter(kind => kind === 'all' || availableKinds.has(kind));
}

export function getKindLabel(t: TemplateTranslator, kind: AgentTemplateKind): string {
  return t(KIND_LABEL_KEYS[kind]);
}

export function getKindFilterLabel(t: TemplateTranslator, kind: AgentTemplateKindFilter): string {
  return kind === 'all' ? t('agents.templates.allTypes') : getKindLabel(t, kind);
}

export function getComplexityLabel(
  t: TemplateTranslator,
  complexity: AgentTemplateComplexity
): string {
  return t(COMPLEXITY_LABEL_KEYS[complexity]);
}

export function getRuntimeStatusLabel(
  t: TemplateTranslator,
  status: AgentTemplateRuntimeStatus
): string {
  return t(RUNTIME_STATUS_LABEL_KEYS[status]);
}

export function getRequirementLabel(
  t: TemplateTranslator,
  requirement: AgentTemplateRequirementId
): string {
  return t(REQUIREMENT_LABEL_KEYS[requirement]);
}

function getRequirementSetupHint(
  t: TemplateTranslator,
  requirement: AgentTemplateRequirementId
): string {
  return t(REQUIREMENT_SETUP_HINT_KEYS[requirement]);
}

export function getCategoryLabel(t: TemplateTranslator, categoryId: AgentTemplateCategoryId) {
  return t(CATEGORY_LABEL_KEYS[categoryId]);
}

export function getTemplateCopy(t: TemplateTranslator, template: AgentTemplate) {
  const titleKey = `agents.templates.items.${template.copyKey}.title`;
  const descriptionKey = `agents.templates.items.${template.copyKey}.description`;
  const title = t(titleKey);
  const description = t(descriptionKey);

  return {
    title: title === titleKey ? template.fallbackTitle : title,
    description: description === descriptionKey ? template.fallbackDescription : description,
  };
}

export function getRequirementSummary(t: TemplateTranslator, template: AgentTemplate) {
  if (template.requirements.length === 0) {
    return t('agents.templates.requirements.none');
  }

  return t('agents.templates.requirements.summary', {
    requirements: template.requirements
      .map(requirement => getRequirementLabel(t, requirement))
      .join(' / '),
  });
}

export function getRunHint(t: TemplateTranslator, template: AgentTemplate) {
  if (template.runtimeStatus === 'ready') {
    if (template.requirements.includes('file-input')) {
      return t('agents.templates.card.readyWithFilesHint');
    }
    if (template.requirements.includes('http-endpoint')) {
      return t('agents.templates.card.readyWithHttpHint');
    }
    return t('agents.templates.card.readyHint');
  }

  const setupRequirements = template.requirements.filter(
    requirement => requirement !== 'default-model'
  );
  const requirements = setupRequirements.length > 0 ? setupRequirements : template.requirements;

  return t('agents.templates.card.setupHint', {
    requirements: requirements
      .map(requirement => getRequirementSetupHint(t, requirement))
      .join(' / '),
  });
}

export function getTemplateSearchText(t: TemplateTranslator, template: AgentTemplate): string {
  const copy = getTemplateCopy(t, template);

  return [
    copy.title,
    copy.description,
    template.kind,
    getKindLabel(t, template.kind),
    getComplexityLabel(t, template.complexity),
    getRuntimeStatusLabel(t, template.runtimeStatus),
    ...template.requirements.map(requirement => getRequirementLabel(t, requirement)),
    template.categories.join(' '),
    ...template.categories.map(category => getCategoryLabel(t, category)),
    template.tags.join(' '),
  ]
    .join(' ')
    .toLowerCase();
}

export function getTemplateCardView(t: TemplateTranslator, template: AgentTemplate) {
  const copy = getTemplateCopy(t, template);

  return {
    title: copy.title,
    description: copy.description,
    kindLabel: getKindLabel(t, template.kind),
    complexityLabel: getComplexityLabel(t, template.complexity),
    runtimeStatusLabel: getRuntimeStatusLabel(t, template.runtimeStatus),
    runtimeStatus: template.runtimeStatus,
    requirementSummary: getRequirementSummary(t, template),
    runHint: getRunHint(t, template),
  };
}

export function getTemplatePreviewView(t: TemplateTranslator, template: AgentTemplate) {
  const cardView = getTemplateCardView(t, template);

  return {
    ...cardView,
    requirements: template.requirements.map(requirement => getRequirementLabel(t, requirement)),
    setupRequirements: template.requirements
      .filter(requirement => requirement !== 'default-model')
      .map(requirement => getRequirementSetupHint(t, requirement)),
    categories: template.categories.map(category => getCategoryLabel(t, category)),
  };
}
