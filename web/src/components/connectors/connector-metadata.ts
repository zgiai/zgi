import { connectorPurpose } from './connector-presentation';
import type {
  ConnectorCategory,
  ConnectorCatalogItem,
  ConnectorLocalizedText,
  ConnectorRuntimeConfirmation,
  ConnectorTag,
} from '../../services/types/connector.ts';

interface ConnectorMetadataFallbacks {
  description?: string;
  capabilityDescription?: string;
}

function normalizedLocale(value: string): string {
  return value.trim().replaceAll('_', '-').toLocaleLowerCase();
}

function localeCandidates(locale: string): string[] {
  const normalized = normalizedLocale(locale);
  const language = normalized.split('-')[0] ?? normalized;
  const candidates = [normalized];
  if (language === 'zh') candidates.push('zh-hans', 'zh-cn', 'zh');
  else if (language === 'en') candidates.push('en-us', 'en');
  else if (language) candidates.push(language);
  return Array.from(new Set(candidates));
}

function localizedEntries<T>(values: Record<string, T> | undefined): Map<string, T> {
  const entries = new Map<string, T>();
  if (!values || typeof values !== 'object' || Array.isArray(values)) return entries;
  Object.entries(values).forEach(([locale, value]) => {
    entries.set(normalizedLocale(locale), value);
  });
  return entries;
}

export function resolveConnectorLocalizedText(
  values: ConnectorLocalizedText | undefined,
  locale: string,
  fallback = ''
): string {
  const entries = localizedEntries(values);
  for (const candidate of localeCandidates(locale)) {
    const value = entries.get(candidate);
    if (typeof value === 'string' && value.trim()) return value.trim();
  }
  return fallback.trim();
}

export const CONNECTOR_CATEGORIES = [
  'communication',
  'productivity',
  'information',
  'developer',
  'other',
] as const satisfies readonly ConnectorCategory[];

export const CONNECTOR_TAGS = [
  'social',
  'messaging',
  'email',
  'enterprise',
  'collaboration',
  'documents',
  'tables',
  'workflow',
  'search',
  'web',
  'research',
  'code',
  'repository',
  'issues',
  'integration',
] as const satisfies readonly ConnectorTag[];

const CATEGORY_SEARCH_ALIASES: Record<ConnectorCategory, string[]> = {
  communication: ['communication', 'social', '通讯', '社交', '沟通'],
  productivity: ['productivity', 'tools', '办公', '工具', '协作'],
  information: ['information', 'retrieval', '信息', '检索'],
  developer: ['developer', 'development', '开发', '代码'],
  other: ['other', '其他'],
};

const TAG_SEARCH_ALIASES: Record<ConnectorTag, string[]> = {
  social: ['social', '社交'],
  messaging: ['messaging', 'message', '消息'],
  email: ['email', 'mail', '邮件'],
  enterprise: ['enterprise', '企业'],
  collaboration: ['collaboration', '协作'],
  documents: ['documents', 'document', '文档'],
  tables: ['tables', 'table', '表格'],
  workflow: ['workflow', '流程'],
  search: ['search', '搜索'],
  web: ['web', '网页'],
  research: ['research', '研究'],
  code: ['code', '代码'],
  repository: ['repository', '仓库', '代码仓库'],
  issues: ['issues', 'issue', '问题'],
  integration: ['integration', '集成'],
};

export function connectorCategory(item: Pick<ConnectorCatalogItem, 'category'>): ConnectorCategory {
  return item.category && CONNECTOR_CATEGORIES.includes(item.category) ? item.category : 'other';
}

export function connectorTags(item: Pick<ConnectorCatalogItem, 'tags'>): ConnectorTag[] {
  const seen = new Set<ConnectorTag>();
  return (item.tags ?? []).filter(tag => {
    if (!CONNECTOR_TAGS.includes(tag) || seen.has(tag)) return false;
    seen.add(tag);
    return true;
  });
}

export function localizeConnectorCatalogItem(
  item: ConnectorCatalogItem,
  locale: string,
  fallbacks: ConnectorMetadataFallbacks = {}
): ConnectorCatalogItem {
  return {
    ...item,
    name: resolveConnectorLocalizedText(item.name_i18n, locale, item.name),
    description: connectorPurpose(item.id, locale) ?? '',
    key_fields: item.key_fields.map(field => ({
      ...field,
      label: resolveConnectorLocalizedText(field.label_i18n, locale, field.label),
      placeholder: resolveConnectorLocalizedText(
        field.placeholder_i18n,
        locale,
        field.placeholder ?? ''
      ),
    })),
    credential_fields: item.credential_fields?.map(field => ({
      ...field,
      label: resolveConnectorLocalizedText(field.label_i18n, locale, field.label),
      placeholder: resolveConnectorLocalizedText(
        field.placeholder_i18n,
        locale,
        field.placeholder ?? ''
      ),
    })),
    capabilities: item.capabilities.map(capability => ({
      ...capability,
      name: resolveConnectorLocalizedText(capability.name_i18n, locale, capability.name),
      description:
        resolveConnectorLocalizedText(
          capability.description_i18n,
          locale,
          capability.description
        ) ||
        fallbacks.capabilityDescription ||
        '',
    })),
  };
}

export function connectorCatalogSearchValues(item: ConnectorCatalogItem): string[] {
  const stableAliases =
    item.id.trim().toLocaleLowerCase() === 'web-search'
      ? ['Exa', 'Exa Web Search', 'Exa 网页搜索', 'websearch', 'web search', '网页搜索']
      : [];
  const category = connectorCategory(item);
  const tags = connectorTags(item);
  return [
    item.id,
    item.name,
    item.description,
    ...stableAliases,
    ...(['gmail', 'qqmail', 'netease163'].includes(item.id) ? ['邮箱', 'email', 'mail'] : []),
    category,
    ...CATEGORY_SEARCH_ALIASES[category],
    ...tags,
    ...tags.flatMap(tag => TAG_SEARCH_ALIASES[tag]),
    ...Object.values(item.name_i18n ?? {}),
    ...Object.values(item.description_i18n ?? {}),
    ...(item.examples ?? []),
    ...item.capabilities.flatMap(capability => [
      capability.name,
      capability.description,
      ...Object.values(capability.name_i18n ?? {}),
      ...Object.values(capability.description_i18n ?? {}),
    ]),
  ].filter(value => typeof value === 'string' && value.trim());
}

export function localizeConnectorRuntimeConfirmation(
  confirmation: ConnectorRuntimeConfirmation,
  locale: string,
  connector?: ConnectorCatalogItem
): ConnectorRuntimeConfirmation {
  const localizedConnector =
    connector?.id === confirmation.connector_id
      ? localizeConnectorCatalogItem(connector, locale)
      : undefined;
  const capability = localizedConnector?.capabilities.find(
    item => item.id === confirmation.operation_id
  );
  const isChinese = normalizedLocale(locale).startsWith('zh');
  const isExa = confirmation.connector_id === 'web-search';
  const builtInConnectorName = isExa ? (isChinese ? 'Exa 网页搜索' : 'Exa Web Search') : undefined;
  const builtInActionName =
    isExa && confirmation.operation_id === 'web.search'
      ? isChinese
        ? '搜索网页'
        : 'Search Web'
      : isExa && confirmation.operation_id === 'web.fetch'
        ? isChinese
          ? '读取网页'
          : 'Fetch Webpage'
        : undefined;
  return {
    ...confirmation,
    connector_name:
      resolveConnectorLocalizedText(
        confirmation.connector_name_i18n,
        locale,
        localizedConnector?.name ?? ''
      ) ||
      builtInConnectorName ||
      confirmation.connector_name,
    action_name:
      resolveConnectorLocalizedText(
        confirmation.action_name_i18n,
        locale,
        capability?.name ?? ''
      ) ||
      builtInActionName ||
      confirmation.action_name,
    consequence: {
      ...confirmation.consequence,
      description: resolveConnectorLocalizedText(
        confirmation.consequence.description_i18n,
        locale,
        confirmation.consequence.description ?? ''
      ),
    },
  };
}

export function formatConnectorDateTime(
  value: string | undefined,
  locale: string,
  fallback: string
): string {
  if (!value) return fallback;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return fallback;
  try {
    return new Intl.DateTimeFormat(locale.trim().replaceAll('_', '-'), {
      dateStyle: 'medium',
      timeStyle: 'short',
    }).format(date);
  } catch {
    return new Intl.DateTimeFormat(undefined, {
      dateStyle: 'medium',
      timeStyle: 'short',
    }).format(date);
  }
}
