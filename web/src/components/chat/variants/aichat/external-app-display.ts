import type { ScopedTranslations } from '@/i18n/translations';
import type { IntegrationLocalizedText } from '@/services/types/integration';

type WebappTranslator = ScopedTranslations<'webapp'>;

const EXTERNAL_APP_NAME_KEYS = {
  github: 'consoleChat.connectedApps.providers.github.name',
  'web-search': 'consoleChat.connectedApps.providers.webSearch.name',
} as const;

const EXTERNAL_APP_DESCRIPTION_KEYS = {
  github: 'consoleChat.connectedApps.providers.github.description',
  'web-search': 'consoleChat.connectedApps.providers.webSearch.description',
} as const;

const EXTERNAL_ACTION_NAME_KEYS = {
  'github.user.get': 'consoleChat.connectedApps.actions.githubUserGet',
  'github.repository.list': 'consoleChat.connectedApps.actions.githubRepositoryList',
  'github.issue.list': 'consoleChat.connectedApps.actions.githubIssueList',
  'web.search': 'consoleChat.connectedApps.actions.webSearch',
  'web.fetch': 'consoleChat.connectedApps.actions.webFetch',
} as const;

function normalizedIdentifier(value: string | null | undefined): string {
  return value?.trim().toLowerCase() ?? '';
}

function normalizedLocale(value: string | null | undefined): string {
  return value?.trim().replace(/_/g, '-').toLowerCase() ?? '';
}

function localeCandidates(locale: string): string[] {
  const normalized = normalizedLocale(locale);
  const language = normalized.split('-')[0];
  if (language === 'zh') {
    return [normalized, 'zh-hans', 'zh-cn', 'zh'];
  }
  if (language === 'en') {
    return [normalized, 'en-us', 'en'];
  }
  return [normalized, language].filter(Boolean);
}

export function getAIChatLocalizedExternalText(
  values: IntegrationLocalizedText | null | undefined,
  locale: string
): string | null {
  if (!values) return null;
  const entries = Object.entries(values).flatMap(([key, value]) => {
    const text = typeof value === 'string' ? value.trim() : '';
    return text ? ([[normalizedLocale(key), text]] as const) : [];
  });
  if (entries.length === 0) return null;
  const byLocale = new Map(entries);
  for (const candidate of localeCandidates(locale)) {
    const text = byLocale.get(candidate);
    if (text) return text;
  }
  return null;
}

interface ExternalAppDisplayMetadata {
  locale?: string;
  nameI18n?: IntegrationLocalizedText | null;
}

interface ExternalAppDescriptionMetadata {
  descriptionI18n?: IntegrationLocalizedText | null;
}

interface ExternalActionDisplayMetadata {
  locale?: string;
  fallbackName?: string | null;
  nameI18n?: IntegrationLocalizedText | null;
}

interface ExternalArgumentDisplayMetadata {
  locale: string;
  argumentLabelsI18n?: Record<string, unknown> | null;
  argumentValueLabelsI18n?: Record<string, unknown> | null;
}

interface ExternalInvocationDisplaySource {
  skill_id?: string | null;
  tool_name?: string | null;
  arguments?: Record<string, unknown> | null;
  result?: Record<string, unknown> | null;
}

export interface AIChatExternalArgumentDisplayEntry {
  key: string;
  label: string | null;
  value: unknown;
}

const OPAQUE_EXTERNAL_LABEL_PATTERN =
  /(?:__zgi_hidden_reference__|\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b|\b[0-9a-f]{32}\b)/i;

function unknownRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

function safeExternalMetadataText(value: unknown): string | null {
  if (typeof value !== 'string') return null;
  const text = value.trim();
  if (!text || text.length > 160 || OPAQUE_EXTERNAL_LABEL_PATTERN.test(text)) return null;
  return text;
}

function externalLocalizedTextMap(value: unknown): IntegrationLocalizedText | null {
  const record = unknownRecord(value);
  if (!record) return null;
  const entries = Object.entries(record).flatMap(([locale, text]) => {
    const safeText = safeExternalMetadataText(text);
    return safeText ? ([[locale, safeText]] as const) : [];
  });
  return entries.length > 0 ? Object.fromEntries(entries) : null;
}

function localizedRecord(value: unknown, locale: string): Record<string, unknown> | null {
  const record = unknownRecord(value);
  if (!record) return null;
  const byLocale = new Map(
    Object.entries(record).map(([key, nested]) => [normalizedLocale(key), nested] as const)
  );
  for (const candidate of localeCandidates(locale)) {
    const localized = unknownRecord(byLocale.get(candidate));
    if (localized) return localized;
  }
  return null;
}

function argumentMetadataValue(
  metadata: Record<string, unknown> | null | undefined,
  fieldPath: string,
  fieldName: string
): unknown {
  return metadata?.[fieldPath] ?? metadata?.[fieldName];
}

function localizedArgumentLabel(
  metadata: ExternalArgumentDisplayMetadata,
  fieldPath: string,
  fieldName: string
): string | null {
  return getAIChatLocalizedExternalText(
    externalLocalizedTextMap(
      argumentMetadataValue(metadata.argumentLabelsI18n, fieldPath, fieldName)
    ),
    metadata.locale
  );
}

function localizedArgumentValue(
  metadata: ExternalArgumentDisplayMetadata,
  fieldPath: string,
  fieldName: string,
  rawValue: string
): string | null {
  const fieldMetadata = argumentMetadataValue(
    metadata.argumentValueLabelsI18n,
    fieldPath,
    fieldName
  );
  // Primary contract: argument_value_labels_i18n[arg][locale][enumValue] = label.
  const primary = safeExternalMetadataText(
    localizedRecord(fieldMetadata, metadata.locale)?.[rawValue]
  );
  if (primary) return primary;

  // Compatibility for arg -> enumValue -> locale -> label.
  return getAIChatLocalizedExternalText(
    externalLocalizedTextMap(unknownRecord(fieldMetadata)?.[rawValue]),
    metadata.locale
  );
}

function argumentEnumKey(value: unknown): string | null {
  if (typeof value === 'string') return value;
  if (typeof value === 'boolean') return value ? 'true' : 'false';
  if (typeof value === 'number' && Number.isFinite(value)) return String(value);
  return null;
}

function externalMetadataString(value: unknown): string | null {
  return safeExternalMetadataText(value);
}

function invocationMetadataValue(
  invocation: ExternalInvocationDisplaySource,
  keys: string[]
): unknown {
  const argumentsValue = unknownRecord(invocation.arguments);
  const result = unknownRecord(invocation.result);
  const nestedResult = unknownRecord(result?.result);
  for (const source of [argumentsValue, result, nestedResult]) {
    for (const key of keys) {
      if (source?.[key] !== undefined) return source[key];
    }
  }
  return undefined;
}

export function isAIChatExternalAppsInvocation(
  invocation: ExternalInvocationDisplaySource
): boolean {
  return normalizedIdentifier(invocation.skill_id) === 'external-apps';
}

export function getAIChatExternalInvocationDisplayName(
  invocation: ExternalInvocationDisplaySource,
  locale: string,
  t: WebappTranslator,
  fallbackToolName: string
): string | null {
  if (!isAIChatExternalAppsInvocation(invocation)) return null;
  if (normalizedIdentifier(invocation.tool_name) !== 'execute_action') {
    return fallbackToolName || t('consoleChat.connectedApps.actions.generic');
  }

  const integrationId = externalMetadataString(
    invocationMetadataValue(invocation, ['integration_id'])
  );
  const actionId = externalMetadataString(invocationMetadataValue(invocation, ['action_id']));
  if (!integrationId || !actionId) {
    return fallbackToolName || t('consoleChat.connectedApps.actions.generic');
  }
  const integrationName = externalMetadataString(
    invocationMetadataValue(invocation, ['integration_name'])
  );
  const actionName = externalMetadataString(invocationMetadataValue(invocation, ['action_name']));
  const integration = getAIChatExternalAppDisplayName(
    integrationId,
    integrationName ?? integrationId,
    t,
    {
      locale,
      nameI18n: externalLocalizedTextMap(
        invocationMetadataValue(invocation, ['integration_name_i18n'])
      ),
    }
  );
  const action = getAIChatExternalActionDisplayName(actionId, t, {
    locale,
    fallbackName: actionName,
    nameI18n: externalLocalizedTextMap(invocationMetadataValue(invocation, ['action_name_i18n'])),
  });
  return t('consoleChat.governance.externalToolLabel', { integration, action });
}

export function getAIChatExternalArgumentDisplayEntries(
  value: unknown,
  metadata: ExternalArgumentDisplayMetadata
): AIChatExternalArgumentDisplayEntry[] {
  const rows: AIChatExternalArgumentDisplayEntry[] = [];
  const metadataPath = (path: string[]) => path.filter(part => !/^\d+$/.test(part));

  const visit = (nested: unknown, path: string[], depth: number) => {
    if (rows.length >= 40 || depth > 6) return;
    if (Array.isArray(nested)) {
      if (nested.every(entry => argumentEnumKey(entry) !== null)) {
        const localizedPath = metadataPath(path);
        const fieldName = localizedPath[localizedPath.length - 1] ?? '';
        const fieldPath = localizedPath.join('.');
        rows.push({
          key: path.join('.') || 'value',
          label: path.length ? localizedArgumentLabel(metadata, fieldPath, fieldName) : null,
          value: nested.slice(0, 20).map(entry => {
            const enumKey = argumentEnumKey(entry);
            return enumKey === null
              ? entry
              : (localizedArgumentValue(metadata, fieldPath, fieldName, enumKey) ?? entry);
          }),
        });
        return;
      }
      nested
        .slice(0, 20)
        .forEach((entry, index) => visit(entry, [...path, String(index)], depth + 1));
      return;
    }

    const record = unknownRecord(nested);
    if (record) {
      Object.entries(record)
        .filter(
          ([fieldName]) =>
            fieldName !== 'argument_labels_i18n' && fieldName !== 'argument_value_labels_i18n'
        )
        .slice(0, 40)
        .forEach(([fieldName, entry]) => visit(entry, [...path, fieldName], depth + 1));
      return;
    }

    if (path.length === 0) return;
    const localizedPath = metadataPath(path);
    const fieldName = localizedPath[localizedPath.length - 1] ?? '';
    const fieldPath = localizedPath.join('.');
    const enumKey = argumentEnumKey(nested);
    rows.push({
      key: path.join('.'),
      label: localizedArgumentLabel(metadata, fieldPath, fieldName),
      value:
        enumKey === null
          ? nested
          : (localizedArgumentValue(metadata, fieldPath, fieldName, enumKey) ?? nested),
    });
  };

  visit(value, [], 0);
  return rows;
}

function meaningfulFallbackName(
  value: string | null | undefined,
  identifier: string
): string | null {
  const trimmed = value?.trim() ?? '';
  if (!trimmed || normalizedIdentifier(trimmed) === normalizedIdentifier(identifier)) return null;
  return trimmed;
}

export function getAIChatExternalAppDisplayName(
  integrationId: string,
  fallbackName: string,
  t: WebappTranslator,
  metadata: ExternalAppDisplayMetadata = {}
): string {
  const localized = getAIChatLocalizedExternalText(metadata.nameI18n, metadata.locale ?? 'en-US');
  if (localized) return localized;
  const normalized = normalizedIdentifier(integrationId);
  const key = EXTERNAL_APP_NAME_KEYS[normalized as keyof typeof EXTERNAL_APP_NAME_KEYS];
  if (key) return t(key);
  const fallback = meaningfulFallbackName(fallbackName, integrationId);
  const locale = metadata.locale ?? 'en-US';
  if (fallback && normalizedLocale(locale).startsWith('en')) return fallback;
  if (fallback && /[\u3400-\u9fff]/u.test(fallback)) return fallback;
  return t('consoleChat.connectedApps.unknownExternalApp');
}

export function getAIChatExternalAppDescription(
  integrationId: string,
  fallbackDescription: string,
  locale: string,
  t: WebappTranslator,
  metadata: ExternalAppDescriptionMetadata = {}
): string {
  const localized = getAIChatLocalizedExternalText(metadata.descriptionI18n, locale);
  if (localized) return localized;
  const normalized = normalizedIdentifier(integrationId);
  const key =
    EXTERNAL_APP_DESCRIPTION_KEYS[normalized as keyof typeof EXTERNAL_APP_DESCRIPTION_KEYS];
  if (key) return t(key);
  if (normalizedLocale(locale).startsWith('en') && fallbackDescription.trim()) {
    return fallbackDescription.trim();
  }
  return t('consoleChat.connectedApps.providers.genericDescription');
}

export function getAIChatExternalActionDisplayName(
  actionId: string,
  t: WebappTranslator,
  metadata: ExternalActionDisplayMetadata = {}
): string {
  const locale = metadata.locale ?? 'en-US';
  const localized = getAIChatLocalizedExternalText(metadata.nameI18n, locale);
  if (localized) return localized;
  const normalized = normalizedIdentifier(actionId);
  const key = EXTERNAL_ACTION_NAME_KEYS[normalized as keyof typeof EXTERNAL_ACTION_NAME_KEYS];
  if (key) return t(key);
  const fallbackName = meaningfulFallbackName(metadata.fallbackName, actionId);
  if (fallbackName && normalizedLocale(locale).startsWith('en')) return fallbackName;
  if (fallbackName && /[\u3400-\u9fff]/u.test(fallbackName)) return fallbackName;
  return t('consoleChat.connectedApps.actions.generic');
}
