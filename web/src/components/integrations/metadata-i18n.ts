'use client';

import { useLocale } from 'next-intl';
import { useT } from '@/i18n';
import type {
  IntegrationActionDefinition,
  IntegrationAuthDefinition,
  IntegrationCatalogItem,
  IntegrationCredentialField,
  IntegrationLocalizedLabelMap,
  IntegrationProviderCredentialField,
} from '@/services/types/integration';
import { integrationErrorTranslationKey } from '@/services/integration-error-i18n';
import { formatDate } from '@/utils/format';
import { safeIntegrationDisplayText, safeOptionalIntegrationDisplayText } from './display-utils';

type LocalizedTextMap = Record<string, string>;
interface LocalizedMetadata {
  name_i18n?: LocalizedTextMap;
  label_i18n?: LocalizedTextMap;
  description_i18n?: LocalizedTextMap;
  placeholder_i18n?: LocalizedTextMap;
  documentation_url_i18n?: LocalizedTextMap;
}

type ProviderLike = Pick<
  IntegrationCatalogItem,
  'id' | 'integration_id' | 'driver_id' | 'name' | 'description' | 'documentation_url' | 'docs_url'
> &
  LocalizedMetadata;
type ActionLike = Pick<IntegrationActionDefinition, 'id' | 'name' | 'description'> &
  LocalizedMetadata;
interface LabelSource {
  category_labels_i18n?: IntegrationLocalizedLabelMap;
  tag_labels_i18n?: IntegrationLocalizedLabelMap;
  scope_labels_i18n?: IntegrationLocalizedLabelMap;
  actions?: IntegrationActionDefinition[];
}
type AuthMethodLike = Pick<
  IntegrationAuthDefinition,
  'id' | 'type' | 'credential_source' | 'label' | 'description'
> &
  LocalizedMetadata;
type CredentialFieldLike = (
  | Pick<IntegrationCredentialField, 'name' | 'label' | 'description' | 'placeholder'>
  | (Pick<IntegrationProviderCredentialField, 'key' | 'label' | 'description' | 'placeholder'> & {
      name?: string;
    })
) &
  LocalizedMetadata;

const PROVIDER_TRANSLATIONS = {
  github: {
    name: 'metadata.providers.github.name',
    description: 'metadata.providers.github.description',
    healthProbe: 'metadata.providers.github.healthProbe',
  },
  gmail: {
    name: 'metadata.providers.gmail.name',
    description: 'metadata.providers.gmail.description',
    healthProbe: 'metadata.providers.gmail.healthProbe',
  },
  feishu: {
    name: 'metadata.providers.feishu.name',
    description: 'metadata.providers.feishu.description',
    healthProbe: 'metadata.providers.feishu.healthProbe',
  },
  x: {
    name: 'metadata.providers.x.name',
    description: 'metadata.providers.x.description',
    healthProbe: 'metadata.providers.x.healthProbe',
  },
  'web-search': {
    name: 'metadata.providers.webSearch.name',
    description: 'metadata.providers.webSearch.description',
    healthProbe: 'metadata.providers.webSearch.healthProbe',
  },
} as const;

const ACTION_TRANSLATIONS = {
  'connection.test': {
    name: 'metadata.actions.connectionTest.name',
    description: 'metadata.actions.connectionTest.description',
  },
  'github.user.get': {
    name: 'metadata.actions.githubUserGet.name',
    description: 'metadata.actions.githubUserGet.description',
  },
  'github.repository.list': {
    name: 'metadata.actions.githubRepositoryList.name',
    description: 'metadata.actions.githubRepositoryList.description',
  },
  'github.repository.search': {
    name: 'metadata.actions.githubRepositorySearch.name',
    description: 'metadata.actions.githubRepositorySearch.description',
  },
  'github.issue.list': {
    name: 'metadata.actions.githubIssueList.name',
    description: 'metadata.actions.githubIssueList.description',
  },
  'github.issue.get': {
    name: 'metadata.actions.githubIssueGet.name',
    description: 'metadata.actions.githubIssueGet.description',
  },
  'github.issue.comment.list': {
    name: 'metadata.actions.githubIssueCommentList.name',
    description: 'metadata.actions.githubIssueCommentList.description',
  },
  'github.issue.create': {
    name: 'metadata.actions.githubIssueCreate.name',
    description: 'metadata.actions.githubIssueCreate.description',
  },
  'github.issue.comment.create': {
    name: 'metadata.actions.githubIssueCommentCreate.name',
    description: 'metadata.actions.githubIssueCommentCreate.description',
  },
  'gmail.account.get': {
    name: 'metadata.actions.gmailAccountGet.name',
    description: 'metadata.actions.gmailAccountGet.description',
  },
  'gmail.mail.send': {
    name: 'metadata.actions.gmailMailSend.name',
    description: 'metadata.actions.gmailMailSend.description',
  },
  'gmail.mail.search': {
    name: 'metadata.actions.gmailMailSearch.name',
    description: 'metadata.actions.gmailMailSearch.description',
  },
  'gmail.mail.get': {
    name: 'metadata.actions.gmailMailGet.name',
    description: 'metadata.actions.gmailMailGet.description',
  },
  'gmail.mail.reply': {
    name: 'metadata.actions.gmailMailReply.name',
    description: 'metadata.actions.gmailMailReply.description',
  },
  'gmail.draft.create': {
    name: 'metadata.actions.gmailDraftCreate.name',
    description: 'metadata.actions.gmailDraftCreate.description',
  },
  'feishu.account.get': {
    name: 'metadata.actions.feishuAccountGet.name',
    description: 'metadata.actions.feishuAccountGet.description',
  },
  'feishu.drive.list': {
    name: 'metadata.actions.feishuDriveList.name',
    description: 'metadata.actions.feishuDriveList.description',
  },
  'feishu.document.read': {
    name: 'metadata.actions.feishuDocumentRead.name',
    description: 'metadata.actions.feishuDocumentRead.description',
  },
  'feishu.message.send_user': {
    name: 'metadata.actions.feishuMessageSendUser.name',
    description: 'metadata.actions.feishuMessageSendUser.description',
  },
  'feishu.message.send_bot': {
    name: 'metadata.actions.feishuMessageSendBot.name',
    description: 'metadata.actions.feishuMessageSendBot.description',
  },
  'feishu.message.list': {
    name: 'metadata.actions.feishuMessageList.name',
    description: 'metadata.actions.feishuMessageList.description',
  },
  'feishu.calendar.event.list': {
    name: 'metadata.actions.feishuCalendarEventList.name',
    description: 'metadata.actions.feishuCalendarEventList.description',
  },
  'feishu.calendar.event.create': {
    name: 'metadata.actions.feishuCalendarEventCreate.name',
    description: 'metadata.actions.feishuCalendarEventCreate.description',
  },
  'x.account.get': {
    name: 'metadata.actions.xAccountGet.name',
    description: 'metadata.actions.xAccountGet.description',
  },
  'x.post.list_own': {
    name: 'metadata.actions.xPostListOwn.name',
    description: 'metadata.actions.xPostListOwn.description',
  },
  'x.post.search_recent': {
    name: 'metadata.actions.xPostSearchRecent.name',
    description: 'metadata.actions.xPostSearchRecent.description',
  },
  'x.post.create': {
    name: 'metadata.actions.xPostCreate.name',
    description: 'metadata.actions.xPostCreate.description',
  },
  'x.user.get_by_username': {
    name: 'metadata.actions.xUserGetByUsername.name',
    description: 'metadata.actions.xUserGetByUsername.description',
  },
  'x.post.list_by_user': {
    name: 'metadata.actions.xPostListByUser.name',
    description: 'metadata.actions.xPostListByUser.description',
  },
  'web.search': {
    name: 'metadata.actions.webSearch.name',
    description: 'metadata.actions.webSearch.description',
  },
  'web.fetch': {
    name: 'metadata.actions.webFetch.name',
    description: 'metadata.actions.webFetch.description',
  },
} as const;

const AUTH_METHOD_TRANSLATIONS = {
  'github:personal_access_token': {
    label: 'metadata.authMethods.githubPersonal.label',
    description: 'metadata.authMethods.githubPersonal.description',
  },
  'github:organization_personal_access_token': {
    label: 'metadata.authMethods.githubOrganization.label',
    description: 'metadata.authMethods.githubOrganization.description',
  },
  'gmail:google_oauth': {
    label: 'metadata.authMethods.gmailOAuth.label',
    description: 'metadata.authMethods.gmailOAuth.description',
  },
  'gmail:organization_google_oauth': {
    label: 'metadata.authMethods.gmailOrganizationOAuth.label',
    description: 'metadata.authMethods.gmailOrganizationOAuth.description',
  },
  'feishu:feishu_user_oauth': {
    label: 'metadata.authMethods.feishuUserOAuth.label',
    description: 'metadata.authMethods.feishuUserOAuth.description',
  },
  'feishu:organization_feishu_user_oauth': {
    label: 'metadata.authMethods.feishuOrganizationOAuth.label',
    description: 'metadata.authMethods.feishuOrganizationOAuth.description',
  },
  'feishu:feishu_tenant_app': {
    label: 'metadata.authMethods.feishuTenantApp.label',
    description: 'metadata.authMethods.feishuTenantApp.description',
  },
  'x:x_oauth': {
    label: 'metadata.authMethods.xOAuth.label',
    description: 'metadata.authMethods.xOAuth.description',
  },
  'x:organization_x_oauth': {
    label: 'metadata.authMethods.xOrganizationOAuth.label',
    description: 'metadata.authMethods.xOrganizationOAuth.description',
  },
  'web-search:platform': {
    label: 'metadata.authMethods.webSearchPlatform.label',
    description: 'metadata.authMethods.webSearchPlatform.description',
  },
  'web-search:api_key': {
    label: 'metadata.authMethods.webSearchOrganization.label',
    description: 'metadata.authMethods.webSearchOrganization.description',
  },
} as const;

const CREDENTIAL_FIELD_TRANSLATIONS = {
  'github:token': {
    label: 'metadata.credentialFields.githubToken.label',
    description: 'metadata.credentialFields.githubToken.description',
    placeholder: 'metadata.credentialFields.githubToken.placeholder',
  },
  'gmail:client_id': {
    label: 'metadata.credentialFields.oauthClientID.label',
    description: 'metadata.credentialFields.oauthClientID.description',
    placeholder: 'metadata.credentialFields.oauthClientID.placeholder',
  },
  'gmail:client_secret': {
    label: 'metadata.credentialFields.oauthClientSecret.label',
    description: 'metadata.credentialFields.oauthClientSecret.description',
    placeholder: 'metadata.credentialFields.oauthClientSecret.placeholder',
  },
  'feishu:client_id': {
    label: 'metadata.credentialFields.oauthClientID.label',
    description: 'metadata.credentialFields.oauthClientID.description',
    placeholder: 'metadata.credentialFields.oauthClientID.placeholder',
  },
  'feishu:client_secret': {
    label: 'metadata.credentialFields.oauthClientSecret.label',
    description: 'metadata.credentialFields.oauthClientSecret.description',
    placeholder: 'metadata.credentialFields.oauthClientSecret.placeholder',
  },
  'x:client_id': {
    label: 'metadata.credentialFields.oauthClientID.label',
    description: 'metadata.credentialFields.oauthClientID.description',
    placeholder: 'metadata.credentialFields.oauthClientID.placeholder',
  },
  'x:client_secret': {
    label: 'metadata.credentialFields.oauthClientSecret.label',
    description: 'metadata.credentialFields.oauthClientSecret.description',
    placeholder: 'metadata.credentialFields.oauthClientSecret.placeholder',
  },
  'web-search:api_key': {
    label: 'metadata.credentialFields.exaApiKey.label',
    description: 'metadata.credentialFields.exaApiKey.description',
    placeholder: 'metadata.credentialFields.exaApiKey.placeholder',
  },
} as const;

const CATEGORY_TRANSLATIONS = {
  developer_tools: 'metadata.categories.developer_tools',
  knowledge_retrieval: 'metadata.categories.knowledge_retrieval',
  external: 'metadata.categories.external',
} as const;

const TAG_TRANSLATIONS = {
  code: 'metadata.tags.code',
  repositories: 'metadata.tags.repositories',
  issues: 'metadata.tags.issues',
  external: 'metadata.tags.external',
  web: 'metadata.tags.web',
  search: 'metadata.tags.search',
} as const;

const SCOPE_TRANSLATIONS = {
  'metadata:read': 'metadata.scopes.metadataRead',
  'issues:read': 'metadata.scopes.issuesRead',
  'web:search': 'metadata.scopes.webSearch',
  'web:read': 'metadata.scopes.webRead',
} as const;

const EFFECT_TRANSLATIONS = {
  none: 'grants.actionEffect.none',
  read: 'grants.actionEffect.read',
  create: 'grants.actionEffect.create',
  update: 'grants.actionEffect.update',
  delete: 'grants.actionEffect.delete',
  publish: 'grants.actionEffect.publish',
  invoke: 'grants.actionEffect.invoke',
  schedule: 'grants.actionEffect.schedule',
  external_send: 'grants.actionEffect.external_send',
} as const;

const RISK_TRANSLATIONS = {
  low: 'grants.riskLevel.low',
  medium: 'grants.riskLevel.medium',
  high: 'grants.riskLevel.high',
  critical: 'grants.riskLevel.critical',
} as const;

const INVOKE_FROM_TRANSLATIONS = {
  aichat: 'enums.invokeFrom.aichat',
  agent: 'enums.invokeFrom.agent',
  workflow: 'enums.invokeFrom.workflow',
  api: 'enums.invokeFrom.api',
} as const;

const HEALTH_REASON_TRANSLATIONS = {
  runtime_success: 'connectionHealth.reason.runtimeSuccess',
  connection_test_succeeded: 'connectionHealth.reason.connectionTestSucceeded',
  scheduled_check_succeeded: 'connectionHealth.reason.scheduledCheckSucceeded',
} as const;

function normalizedLocaleCandidates(locale: string): string[] {
  const normalized = locale.trim();
  const lower = normalized.toLowerCase();
  const candidates = [normalized];
  if (lower.startsWith('zh')) candidates.push('zh-Hans', 'zh-CN', 'zh');
  if (lower.startsWith('en')) candidates.push('en-US', 'en');
  return [...new Set(candidates)];
}

function localizedMetadataValue(
  metadata: LocalizedMetadata | null | undefined,
  field: keyof Pick<
    LocalizedMetadata,
    'name_i18n' | 'label_i18n' | 'description_i18n' | 'placeholder_i18n' | 'documentation_url_i18n'
  >,
  locale: string
): string | null {
  const values = metadata?.[field];
  if (!values || typeof values !== 'object') return null;
  for (const candidate of normalizedLocaleCandidates(locale)) {
    const value = safeOptionalIntegrationDisplayText(values[candidate]);
    if (value) return value;
  }
  return null;
}

function localizedLabelValue(
  labels: IntegrationLocalizedLabelMap | null | undefined,
  value: string | null | undefined,
  locale: string
): string | null {
  if (!labels || !value) return null;
  const normalizedValue = value.trim().toLowerCase();
  const localized = labels[value] ?? labels[normalizedValue];
  if (!localized || typeof localized !== 'object') return null;
  for (const candidate of normalizedLocaleCandidates(locale)) {
    const label = safeOptionalIntegrationDisplayText(localized[candidate]);
    if (label) return label;
  }
  return null;
}

function normalizedIntegrationID(value: string | null | undefined): string {
  const normalized = value?.trim().toLowerCase() ?? '';
  if (normalized === 'exa' || normalized === 'exa-rest') return 'web-search';
  return normalized;
}

function readableEnglishMetadataText(value: string | null | undefined): string | null {
  const safe = safeOptionalIntegrationDisplayText(value);
  if (!safe) return null;
  const readable = safe
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .replace(/[._-]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
  if (!readable) return null;
  return readable.charAt(0).toUpperCase() + readable.slice(1);
}

function metadataKey<T extends Record<string, unknown>>(map: T, value: string | null | undefined) {
  const normalized = value?.trim().toLowerCase() ?? '';
  return Object.prototype.hasOwnProperty.call(map, normalized) ? (normalized as keyof T) : null;
}

export function useIntegrationMetadata() {
  const t = useT('integrations');
  const locale = useLocale();
  const isChineseLocale = locale.toLowerCase().startsWith('zh');
  const intlLocale = locale === 'zh-Hans' ? 'zh-CN' : locale;

  const providerTranslation = (integrationID: string | null | undefined) => {
    const key = metadataKey(PROVIDER_TRANSLATIONS, normalizedIntegrationID(integrationID));
    return key ? PROVIDER_TRANSLATIONS[key] : null;
  };

  const localizedText = (values: LocalizedTextMap | null | undefined, fallback: string): string => {
    if (values && typeof values === 'object') {
      for (const candidate of normalizedLocaleCandidates(locale)) {
        const value = safeOptionalIntegrationDisplayText(values[candidate]);
        if (value) return value;
      }
    }
    return safeIntegrationDisplayText(fallback, '');
  };

  const providerName = (provider: ProviderLike | string, fallback?: string): string => {
    const entity = typeof provider === 'string' ? null : provider;
    const integrationID =
      typeof provider === 'string'
        ? provider
        : provider.integration_id || provider.id || provider.driver_id;
    const localized = localizedMetadataValue(entity, 'name_i18n', locale);
    if (localized) return localized;
    const translation = providerTranslation(integrationID);
    if (translation) return t(translation.name);
    if (isChineseLocale) return t('common.unknownExternalApp');
    if (typeof provider === 'string') {
      return (
        readableEnglishMetadataText(fallback) ??
        readableEnglishMetadataText(provider) ??
        t('common.unknownExternalApp')
      );
    }
    return (
      readableEnglishMetadataText(provider.name) ??
      readableEnglishMetadataText(fallback) ??
      t('common.unknownExternalApp')
    );
  };

  const providerDescription = (provider: ProviderLike, fallback?: string): string => {
    const localized = localizedMetadataValue(provider, 'description_i18n', locale);
    if (localized) return localized;
    const translation = providerTranslation(
      provider.integration_id || provider.id || provider.driver_id
    );
    if (translation) return t(translation.description);
    if (isChineseLocale) return fallback ?? t('catalog.noDescription');
    return safeIntegrationDisplayText(provider.description, fallback ?? t('catalog.noDescription'));
  };

  const healthProbeDescription = (
    provider: ProviderLike,
    healthProbe: ({ description?: string } & LocalizedMetadata) | null | undefined,
    fallback?: string
  ): string => {
    const localized = localizedMetadataValue(healthProbe, 'description_i18n', locale);
    if (localized) return localized;
    const translation = providerTranslation(
      provider.integration_id || provider.id || provider.driver_id
    );
    if (translation) return t(translation.healthProbe);
    if (isChineseLocale) return fallback ?? t('detail.healthProbeSupported');
    return safeIntegrationDisplayText(
      healthProbe?.description,
      fallback ?? t('detail.healthProbeSupported')
    );
  };

  const actionName = (action: ActionLike, fallback?: string): string => {
    const localized = localizedMetadataValue(action, 'name_i18n', locale);
    if (localized) return localized;
    const key = metadataKey(ACTION_TRANSLATIONS, action.id);
    if (key) return t(ACTION_TRANSLATIONS[key].name);
    const safeFallback = safeOptionalIntegrationDisplayText(fallback);
    const safeName = safeOptionalIntegrationDisplayText(action.name);
    const actionIdentity = action.id.trim().toLowerCase();
    const displayFallback =
      safeFallback && safeFallback.trim().toLowerCase() !== actionIdentity ? safeFallback : null;
    const displayName =
      safeName && safeName.trim().toLowerCase() !== actionIdentity ? safeName : null;
    if (isChineseLocale) {
      return (
        [displayFallback, displayName].find(value => value && /[\u3400-\u9fff]/u.test(value)) ??
        t('common.unknownAction')
      );
    }
    return (
      readableEnglishMetadataText(displayFallback) ??
      readableEnglishMetadataText(displayName) ??
      t('common.unknownAction')
    );
  };

  const actionNameByID = (actionID: string | null | undefined, fallback?: string): string => {
    const key = metadataKey(ACTION_TRANSLATIONS, actionID);
    if (key) return t(ACTION_TRANSLATIONS[key].name);
    const safeFallback = safeOptionalIntegrationDisplayText(fallback);
    if (!safeFallback || safeFallback.trim().toLowerCase() === actionID?.trim().toLowerCase()) {
      return t('common.unknownAction');
    }
    if (isChineseLocale) {
      return /[\u3400-\u9fff]/u.test(safeFallback) ? safeFallback : t('common.unknownAction');
    }
    return readableEnglishMetadataText(safeFallback) ?? t('common.unknownAction');
  };

  const actionDescription = (action: ActionLike): string | null => {
    const localized = localizedMetadataValue(action, 'description_i18n', locale);
    if (localized) return localized;
    const key = metadataKey(ACTION_TRANSLATIONS, action.id);
    if (key) return t(ACTION_TRANSLATIONS[key].description);
    if (isChineseLocale) return t('catalog.noDescription');
    return safeOptionalIntegrationDisplayText(action.description);
  };

  const documentationURL = (provider: ProviderLike): string | null => {
    const localized = localizedMetadataValue(provider, 'documentation_url_i18n', locale);
    if (localized) return localized;
    const value = safeOptionalIntegrationDisplayText(
      provider.documentation_url || provider.docs_url
    );
    if (!value) return null;
    const integrationID = normalizedIntegrationID(
      provider.integration_id || provider.id || provider.driver_id
    );
    if (locale.toLowerCase().startsWith('zh') && integrationID === 'github') {
      return value.replace('https://docs.github.com/en/rest', 'https://docs.github.com/zh/rest');
    }
    return value;
  };

  const authMethodTranslation = (integrationID: string, auth: AuthMethodLike) => {
    const id = auth.id || auth.type;
    const key = metadataKey(
      AUTH_METHOD_TRANSLATIONS,
      `${normalizedIntegrationID(integrationID)}:${id}`
    );
    return key ? AUTH_METHOD_TRANSLATIONS[key] : null;
  };

  const authMethodLabel = (integrationID: string, auth: AuthMethodLike): string => {
    const localized = localizedMetadataValue(auth, 'label_i18n', locale);
    if (localized) return localized;
    const translation = authMethodTranslation(integrationID, auth);
    if (translation) return t(translation.label);
    if (isChineseLocale) return authType(auth.type);
    return readableEnglishMetadataText(auth.label) ?? authType(auth.type);
  };

  const authMethodDescription = (integrationID: string, auth: AuthMethodLike): string | null => {
    const localized = localizedMetadataValue(auth, 'description_i18n', locale);
    if (localized) return localized;
    const translation = authMethodTranslation(integrationID, auth);
    if (translation) return t(translation.description);
    if (isChineseLocale) return null;
    return safeOptionalIntegrationDisplayText(auth.description);
  };

  const credentialFieldTranslation = (integrationID: string, field: CredentialFieldLike) => {
    const fieldName = 'name' in field && field.name ? field.name : 'key' in field ? field.key : '';
    const key = metadataKey(
      CREDENTIAL_FIELD_TRANSLATIONS,
      `${normalizedIntegrationID(integrationID)}:${fieldName}`
    );
    return key ? CREDENTIAL_FIELD_TRANSLATIONS[key] : null;
  };

  const credentialFieldLabel = (integrationID: string, field: CredentialFieldLike): string => {
    const localized = localizedMetadataValue(field, 'label_i18n', locale);
    if (localized) return localized;
    const translation = credentialFieldTranslation(integrationID, field);
    if (translation) return t(translation.label);
    if (isChineseLocale) return t('dialog.credentialField');
    const fieldName = 'name' in field && field.name ? field.name : 'key' in field ? field.key : '';
    return readableEnglishMetadataText(field.label || fieldName) ?? t('dialog.credentialField');
  };

  const credentialFieldDescription = (
    integrationID: string,
    field: CredentialFieldLike
  ): string | null => {
    const localized = localizedMetadataValue(field, 'description_i18n', locale);
    if (localized) return localized;
    const translation = credentialFieldTranslation(integrationID, field);
    if (translation) return t(translation.description);
    if (isChineseLocale) return null;
    return safeOptionalIntegrationDisplayText(field.description);
  };

  const credentialFieldPlaceholder = (
    integrationID: string,
    field: CredentialFieldLike
  ): string | null => {
    const localized = localizedMetadataValue(field, 'placeholder_i18n', locale);
    if (localized) return localized;
    const translation = credentialFieldTranslation(integrationID, field);
    if (translation) return t(translation.placeholder);
    if (isChineseLocale) return null;
    return safeOptionalIntegrationDisplayText(field.placeholder);
  };

  const optionLabel = (option: { label: string } & LocalizedMetadata): string =>
    localizedMetadataValue(option, 'label_i18n', locale) ??
    (isChineseLocale
      ? t('dialog.selectValue')
      : (readableEnglishMetadataText(option.label) ?? t('dialog.selectValue')));

  const category = (value: string, source?: LabelSource): string => {
    const localized = localizedLabelValue(source?.category_labels_i18n, value, locale);
    if (localized) return localized;
    const key = metadataKey(CATEGORY_TRANSLATIONS, value);
    return key ? t(CATEGORY_TRANSLATIONS[key]) : t('metadata.categories.unknown');
  };

  const tag = (value: string, source?: LabelSource): string => {
    const localized = localizedLabelValue(source?.tag_labels_i18n, value, locale);
    if (localized) return localized;
    const key = metadataKey(TAG_TRANSLATIONS, value);
    return key ? t(TAG_TRANSLATIONS[key]) : t('metadata.tags.unknown');
  };

  const scope = (value: string, source?: LabelSource): string => {
    const direct = localizedLabelValue(source?.scope_labels_i18n, value, locale);
    if (direct) return direct;
    for (const action of source?.actions ?? []) {
      const localized = localizedLabelValue(action.scope_labels_i18n, value, locale);
      if (localized) return localized;
    }
    const key = metadataKey(SCOPE_TRANSLATIONS, value);
    return key
      ? t(SCOPE_TRANSLATIONS[key])
      : (safeOptionalIntegrationDisplayText(value) ?? t('metadata.scopes.unknown'));
  };

  const effect = (value: string | null | undefined): string => {
    const key = metadataKey(EFFECT_TRANSLATIONS, value);
    return key ? t(EFFECT_TRANSLATIONS[key]) : t('grants.actionEffect.unknown');
  };

  const risk = (value: string | null | undefined): string => {
    const key = metadataKey(RISK_TRANSLATIONS, value);
    return key ? t(RISK_TRANSLATIONS[key]) : t('grants.riskLevel.unknown');
  };

  function authType(value: string | null | undefined): string {
    switch (value?.trim().toLowerCase()) {
      case 'platform':
        return t('enums.authType.platform');
      case 'api_key':
        return t('enums.authType.api_key');
      case 'oauth':
        return t('enums.authType.oauth');
      case 'oauth2':
        return t('enums.authType.oauth2');
      case 'custom_credential':
        return t('enums.authType.custom_credential');
      case 'service_account':
        return t('enums.authType.service_account');
      case 'no_auth':
        return t('enums.authType.no_auth');
      default:
        return t('enums.authType.unknown');
    }
  }

  const invokeFrom = (value: string | null | undefined): string => {
    const key = metadataKey(INVOKE_FROM_TRANSLATIONS, value);
    return key ? t(INVOKE_FROM_TRANSLATIONS[key]) : t('enums.invokeFrom.unknown');
  };

  const error = (value: string | null | undefined): string => {
    const key = integrationErrorTranslationKey(value, 'integrations');
    return key ? t(key) : t('errors.unknown');
  };

  const healthReason = (value: string | null | undefined): string => {
    const reasonKey = metadataKey(HEALTH_REASON_TRANSLATIONS, value);
    if (reasonKey) return t(HEALTH_REASON_TRANSLATIONS[reasonKey]);
    const errorKey = integrationErrorTranslationKey(value, 'integrations');
    return errorKey ? t(errorKey) : t('connectionHealth.reason.unknown');
  };

  const date = (value: string | number | Date | null | undefined, fallback: string): string => {
    if (value === null || value === undefined || value === '') return fallback;
    const formatted = formatDate(value, 'YYYY-MM-DD HH:mm', { locale: intlLocale });
    return formatted === 'Invalid Date' ? fallback : formatted;
  };

  const number = (value: number): string => new Intl.NumberFormat(intlLocale).format(value);
  const duration = (milliseconds: number): string =>
    `${number(milliseconds)} ${t('units.millisecondsShort')}`;
  const usd = (value: number): string =>
    new Intl.NumberFormat(intlLocale, {
      style: 'currency',
      currency: 'USD',
      minimumFractionDigits: 0,
      maximumFractionDigits: 4,
    }).format(value);

  return {
    locale,
    localizedText,
    providerName,
    providerDescription,
    healthProbeDescription,
    actionName,
    actionNameByID,
    actionDescription,
    documentationURL,
    authMethodLabel,
    authMethodDescription,
    credentialFieldLabel,
    credentialFieldDescription,
    credentialFieldPlaceholder,
    optionLabel,
    category,
    tag,
    scope,
    effect,
    risk,
    authType,
    invokeFrom,
    error,
    healthReason,
    date,
    number,
    duration,
    usd,
  };
}
