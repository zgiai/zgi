import type { ScopedTranslations } from '@/i18n/translations';
import {
  integrationErrorTranslationKey,
  looksLikeIntegrationErrorCode,
} from '@/services/integration-error-i18n';
import {
  formatTimelineDebugValue,
  sanitizeTimelineDisplayString,
  summarizeTimelineArgumentsForDisplay,
} from '@/components/chat/variants/aichat/timeline-display-safety';

type WebappTranslator = ScopedTranslations<'webapp'>;

const ARGUMENT_FIELD_LABEL_KEYS = {
  integration_id: 'consoleChat.skills.trace.argumentFields.integration',
  action_id: 'consoleChat.skills.trace.argumentFields.action',
  connection_selector: 'consoleChat.skills.trace.argumentFields.connectionSelection',
  arguments: 'consoleChat.skills.trace.argumentFields.actionArguments',
  query: 'consoleChat.skills.trace.argumentFields.query',
  num_results: 'consoleChat.skills.trace.argumentFields.resultCount',
  search_type: 'consoleChat.skills.trace.argumentFields.searchType',
  include_domains: 'consoleChat.skills.trace.argumentFields.includeDomains',
  exclude_domains: 'consoleChat.skills.trace.argumentFields.excludeDomains',
  start_published_date: 'consoleChat.skills.trace.argumentFields.startPublishedDate',
  end_published_date: 'consoleChat.skills.trace.argumentFields.endPublishedDate',
  urls: 'consoleChat.skills.trace.argumentFields.urls',
  content_mode: 'consoleChat.skills.trace.argumentFields.contentMode',
  highlight_query: 'consoleChat.skills.trace.argumentFields.highlightQuery',
  max_characters: 'consoleChat.skills.trace.argumentFields.maxCharacters',
  max_age_hours: 'consoleChat.skills.trace.argumentFields.maxAgeHours',
  owner: 'consoleChat.skills.trace.argumentFields.repositoryOwner',
  repo: 'consoleChat.skills.trace.argumentFields.repository',
  state: 'consoleChat.skills.trace.argumentFields.state',
  labels: 'consoleChat.skills.trace.argumentFields.labels',
  page: 'consoleChat.skills.trace.argumentFields.page',
  per_page: 'consoleChat.skills.trace.argumentFields.perPage',
  visibility: 'consoleChat.skills.trace.argumentFields.visibility',
  affiliation: 'consoleChat.skills.trace.argumentFields.affiliation',
  sort: 'consoleChat.skills.trace.argumentFields.sort',
  direction: 'consoleChat.skills.trace.argumentFields.direction',
} as const;

interface StructuralSummary {
  type: string;
  length?: number;
  keys?: number;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === 'object' && !Array.isArray(value));
}

function structuralSummary(value: unknown): StructuralSummary | null {
  if (!isRecord(value) || typeof value.type !== 'string') return null;
  const keys = Object.keys(value);
  if (!keys.every(key => ['type', 'length', 'keys'].includes(key))) return null;
  return {
    type: value.type.toLowerCase(),
    length: typeof value.length === 'number' ? value.length : undefined,
    keys: typeof value.keys === 'number' ? value.keys : undefined,
  };
}

function argumentTypeLabel(summary: StructuralSummary, t: WebappTranslator): string {
  switch (summary.type) {
    case 'string':
      return t('consoleChat.skills.trace.argumentTypes.string', { count: summary.length ?? 0 });
    case 'array':
      return t('consoleChat.skills.trace.argumentTypes.array', { count: summary.length ?? 0 });
    case 'object':
      return t('consoleChat.skills.trace.argumentTypes.object', { count: summary.keys ?? 0 });
    case 'number':
      return t('consoleChat.skills.trace.argumentTypes.number');
    case 'boolean':
      return t('consoleChat.skills.trace.argumentTypes.boolean');
    default:
      return t('consoleChat.skills.trace.argumentTypes.unknown');
  }
}

function argumentFieldLabel(key: string, t: WebappTranslator): string {
  const normalized = key.trim().toLowerCase();
  const labelKey = ARGUMENT_FIELD_LABEL_KEYS[normalized as keyof typeof ARGUMENT_FIELD_LABEL_KEYS];
  return labelKey ? t(labelKey) : key;
}

function formatArgumentSummaryNode(value: unknown, t: WebappTranslator, depth = 0): string | null {
  const summary = structuralSummary(value);
  if (summary) return argumentTypeLabel(summary, t);
  if (!isRecord(value)) {
    return formatTimelineDebugValue(
      value,
      t('consoleChat.skills.trace.values.hidden'),
      t('consoleChat.skills.trace.values.truncated')
    );
  }
  if (depth >= 4) return t('consoleChat.skills.trace.values.truncated');

  const rows = Object.entries(value).flatMap(([key, nested]) => {
    const formatted = formatArgumentSummaryNode(nested, t, depth + 1);
    return formatted ? [`${argumentFieldLabel(key, t)}: ${formatted}`] : [];
  });
  return rows.length > 0
    ? rows.join('\n')
    : t('consoleChat.skills.trace.argumentTypes.object', { count: 0 });
}

export function formatAIChatTimelineArgumentSummary(
  value: unknown,
  t: WebappTranslator
): string | null {
  const summary = summarizeTimelineArgumentsForDisplay(
    value,
    t('consoleChat.skills.trace.values.hidden'),
    t('consoleChat.skills.trace.values.truncated')
  );
  return formatArgumentSummaryNode(summary, t);
}

export function formatAIChatTimelineValue(value: unknown, t: WebappTranslator): string | null {
  return formatTimelineDebugValue(
    value,
    t('consoleChat.skills.trace.values.hidden'),
    t('consoleChat.skills.trace.values.truncated')
  );
}

export function getAIChatInvocationKindLabel(value: unknown, t: WebappTranslator): string | null {
  if (typeof value !== 'string' || !value.trim()) return null;
  switch (value.trim().toLowerCase()) {
    case 'skill_load':
    case 'skill_load_attempt':
      return t('consoleChat.skills.trace.kinds.skillLoad');
    case 'reference_read':
      return t('consoleChat.skills.trace.kinds.referenceRead');
    case 'tool_call':
      return t('consoleChat.skills.trace.kinds.toolCall');
    case 'guardrail':
      return t('consoleChat.skills.trace.kinds.guardrail');
    default:
      return t('consoleChat.skills.trace.kinds.runtimeEvent');
  }
}

export function localizeAIChatRuntimeErrorCode(value: unknown, t: WebappTranslator): string | null {
  if (typeof value !== 'string') return null;
  const normalized = value.trim().toLowerCase();
  if (!normalized) return null;
  const key = integrationErrorTranslationKey(normalized, 'webapp');
  if (key) return t(key);
  return normalized.startsWith('integration_')
    ? t('consoleChat.skills.trace.errors.externalAppFailed')
    : null;
}

export function localizeAIChatRuntimeMessage(
  value: unknown,
  t: WebappTranslator,
  fallback?: string,
  errorCode?: unknown
): string | null {
  const localizedErrorCode = localizeAIChatRuntimeErrorCode(errorCode, t);
  if (localizedErrorCode) return localizedErrorCode;
  if (typeof value !== 'string' || !value.trim()) return fallback ?? null;
  const raw = value.trim();
  const normalized = raw.toLowerCase();
  if (
    /(?:invalid[\s_-]+arguments?|arguments?[\s_-]+invalid|arguments?[\s_-]+validation[\s_-]+failed|failed[\s_-]+to[\s_-]+validate[\s_-]+arguments?)/i.test(
      normalized
    )
  ) {
    return t('consoleChat.skills.trace.errors.invalidArguments');
  }
  const integrationErrorKey = integrationErrorTranslationKey(normalized, 'webapp');
  if (integrationErrorKey) return t(integrationErrorKey);
  if (looksLikeIntegrationErrorCode(normalized)) {
    return t('consoleChat.skills.trace.errors.externalAppFailed');
  }
  if (/preferred connection|connection.*selected|selected.*connection/i.test(normalized)) {
    return t('consoleChat.skills.trace.errors.preferredConnectionUnavailable');
  }
  if (/unauthori[sz]ed|forbidden|access denied/i.test(normalized)) {
    return t('consoleChat.skills.trace.errors.accessDenied');
  }
  if (/reconnect|reauthori[sz]e/i.test(normalized)) {
    return t('consoleChat.skills.trace.errors.reconnectRequired');
  }
  if (/connection.*expired|expired.*connection/i.test(normalized)) {
    return t('consoleChat.skills.trace.errors.connectionExpired');
  }
  if (/insufficient.*scope|missing.*scope|scope.*required/i.test(normalized)) {
    return t('consoleChat.skills.trace.errors.insufficientScope');
  }
  if (/connection.*not found|unknown.*connection/i.test(normalized)) {
    return t('consoleChat.skills.trace.errors.connectionNotFound');
  }
  if (/quota.*exceeded|usage limit/i.test(normalized)) {
    return t('consoleChat.skills.trace.errors.quotaExceeded');
  }
  if (/timed? out|timeout/i.test(normalized)) {
    return t('consoleChat.skills.trace.errors.timeout');
  }
  if (/rate limit|too many requests/i.test(normalized)) {
    return t('consoleChat.skills.trace.errors.rateLimited');
  }
  const isAsciiTechnicalMessage =
    /[a-z]/i.test(raw) && Array.from(raw).every(character => character.charCodeAt(0) <= 0x7f);
  if (fallback && isAsciiTechnicalMessage) {
    return fallback;
  }
  return (
    sanitizeTimelineDisplayString(
      raw,
      t('consoleChat.skills.trace.values.hidden'),
      t('consoleChat.skills.trace.values.truncated')
    ) ??
    fallback ??
    null
  );
}
