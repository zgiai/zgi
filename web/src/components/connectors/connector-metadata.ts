import { connectorPurpose } from './connector-presentation';
import type {
  ConnectorCatalogItem,
  ConnectorLocalizedText,
  ConnectorRuntimeConfirmation,
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
  return [
    item.id,
    item.name,
    item.description,
    ...stableAliases,
    ...(['gmail', 'qqmail', 'netease163'].includes(item.id) ? ['邮箱', 'email', 'mail'] : []),
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
