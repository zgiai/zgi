import type {
  ConnectorCatalogItem,
  ConnectorTargetBinding,
  PersonalConnection,
} from '@/services/types/connector';
import type {
  SocialCapability,
  SocialCapabilityMode,
  SocialOperationsConfiguration,
  SocialOperationsSettings,
  SocialPlatform,
} from '@/services/types/social-operations';

export const SOCIAL_CAPABILITIES: SocialCapability[] = [
  'account',
  'post',
  'readback',
  'comment',
  'inbox',
  'metrics',
  'images',
];

export interface SocialOperationsScopeKey {
  organizationId: string;
  workspaceId: string;
  accountId: string;
  employeeId: string;
  versionId: string;
}

export interface SocialPlatformChoice {
  platform: SocialPlatform;
  connectorId: string;
  connectionId: string;
  accountId: string;
  label: string;
}

export interface SocialOperationsPlatformDraft extends SocialPlatformChoice {
  maxDailyPublications: number;
  capabilities: Record<SocialCapability, SocialCapabilityMode>;
}

export interface SocialOperationsResearchChoice {
  connectorId: string;
  connectionId: string;
  label: string;
}

export interface SocialOperationsDraft {
  productName: string;
  productDescription: string;
  audience: string;
  publicationLanguage: string;
  internalReviewLanguage: string;
  sourceUrls: string[];
  sourceArtifactIds: string[];
  brandArtifactRefs: string[];
  research: {
    connectorId: string;
    connectionId: string;
  };
  platforms: SocialOperationsPlatformDraft[];
  timezone: string;
  cadence: {
    dailyContentCron: string;
    inboxCron: string;
    sevenDaySampleCron: string;
    weeklyReviewCron: string;
  };
  handoff: {
    connectorId: string;
    connectionId: string;
    operationId: string;
    destination: string;
  };
}

export type SocialOperationsDraftError =
  | 'product'
  | 'audience'
  | 'languages'
  | 'sources'
  | 'brandArtifacts'
  | 'research'
  | 'platforms'
  | 'account'
  | 'dailyLimit'
  | 'timezone'
  | 'cadence'
  | 'handoff';

export function socialOperationsQueryKey(scope: SocialOperationsScopeKey) {
  return [
    'digital-humans',
    'detail',
    scope.employeeId,
    'social-operations',
    scope.organizationId,
    scope.workspaceId,
    scope.accountId,
    scope.versionId,
  ] as const;
}

export function createSocialOperationsDraft(timezone = 'UTC'): SocialOperationsDraft {
  return {
    productName: '',
    productDescription: '',
    audience: '',
    publicationLanguage: 'en-US',
    internalReviewLanguage: 'zh-Hans',
    sourceUrls: [''],
    sourceArtifactIds: [],
    brandArtifactRefs: [],
    research: { connectorId: '', connectionId: '' },
    platforms: [],
    timezone,
    cadence: {
      dailyContentCron: '0 9 * * *',
      inboxCron: '*/20 * * * *',
      sevenDaySampleCron: '15 10 * * *',
      weeklyReviewCron: '0 16 * * 5',
    },
    handoff: { connectorId: '', connectionId: '', operationId: '', destination: '' },
  };
}

function platformForConnector(connectorId: string): SocialPlatform | undefined {
  if (connectorId === 'x-social') return 'x';
  if (connectorId === 'linkedin-social') return 'linkedin';
  if (connectorId === 'facebook-social') return 'facebook';
  return undefined;
}

export function socialPlatformChoices(
  employeeId: string,
  versionId: string,
  bindings: ConnectorTargetBinding[],
  connections: PersonalConnection[]
): SocialPlatformChoice[] {
  return bindings.flatMap(binding => {
    const platform = platformForConnector(binding.connector_id);
    const connection = connections.find(item => item.id === binding.connection_id);
    if (
      !platform ||
      !connection ||
      connection.state !== 'ready' ||
      connection.connector_id !== binding.connector_id ||
      binding.status !== 'active' ||
      binding.target.type !== 'digital_human' ||
      binding.target.id !== employeeId ||
      binding.target.scope !== 'published' ||
      binding.target.version_id !== versionId
    ) {
      return [];
    }
    return [
      {
        platform,
        connectorId: binding.connector_id,
        connectionId: connection.id,
        accountId: connection.external_account_id ?? '',
        label: connection.external_display_name || connection.name,
      },
    ];
  });
}

export function researchConnectionChoices(
  employeeId: string,
  versionId: string,
  bindings: ConnectorTargetBinding[],
  connections: PersonalConnection[],
  catalog: ConnectorCatalogItem[]
): SocialOperationsResearchChoice[] {
  return bindings.flatMap(binding => {
    const connection = connections.find(item => item.id === binding.connection_id);
    const connector = catalog.find(item => item.id === binding.connector_id);
    const definedOperations = new Set(
      connector?.capabilities.filter(operation => operation.usable).map(operation => operation.id)
    );
    const usableOperations = new Set(connection?.usable_operation_ids ?? []);
    if (
      !connection ||
      !connector ||
      connection.state !== 'ready' ||
      connection.connector_id !== binding.connector_id ||
      binding.status !== 'active' ||
      binding.target.type !== 'digital_human' ||
      binding.target.id !== employeeId ||
      binding.target.scope !== 'published' ||
      binding.target.version_id !== versionId ||
      !definedOperations.has('web.search') ||
      !definedOperations.has('web.fetch') ||
      !usableOperations.has('web.search') ||
      !usableOperations.has('web.fetch')
    ) {
      return [];
    }
    return [{
      connectorId: connector.id,
      connectionId: connection.id,
      label: connection.external_display_name || connection.name,
    }];
  });
}

export function platformDraft(choice: SocialPlatformChoice): SocialOperationsPlatformDraft {
  const capabilities = Object.fromEntries(
    SOCIAL_CAPABILITIES.map(capability => [
      capability,
      choice.platform === 'linkedin' && capability === 'inbox' ? 'optional' : 'required',
    ])
  ) as Record<SocialCapability, SocialCapabilityMode>;
  return { ...choice, maxDailyPublications: 1, capabilities };
}

export function initialVerifiedAccountId(
  platform: SocialPlatform,
  accounts: Array<{ id: string }>
) {
  return platform !== 'linkedin' && accounts.length === 1 ? accounts[0].id : '';
}

const SOCIAL_OPERATIONS_HANDOFF_OPERATIONS = new Set([
  'feishu.message.send',
  'dingtalk.messages.send_direct',
  'dingtalk.messages.send_group',
  'gmail.message.send',
  'qqmail.message.send',
  'netease163.message.send',
]);

export function isSupportedSocialOperationsHandoff(
  connectorId: string,
  operationId: string
) {
  return SOCIAL_OPERATIONS_HANDOFF_OPERATIONS.has(operationId) && (
    operationId.startsWith(`${connectorId}.`) ||
    (connectorId === 'dingtalk' && operationId.startsWith('dingtalk.'))
  );
}

export function draftFromSocialOperationsConfiguration(
  configuration: SocialOperationsConfiguration
): SocialOperationsDraft {
  return {
    productName: configuration.product.name,
    productDescription: configuration.product.description,
    audience: configuration.audience,
    publicationLanguage: configuration.publication_language,
    internalReviewLanguage: configuration.internal_review_language,
    sourceUrls: configuration.sources.flatMap(source => (source.url ? [source.url] : [])),
    sourceArtifactIds: configuration.sources.flatMap(source =>
      source.artifact_id ? [source.artifact_id] : []
    ),
    brandArtifactRefs: [...(configuration.brand_artifact_refs ?? [])],
    research: {
      connectorId: configuration.research.connector_id,
      connectionId: configuration.research.connection_id,
    },
    platforms: configuration.platforms.map(platform => ({
      platform: platform.platform,
      connectorId: platform.connector_id,
      connectionId: platform.connection_id,
      accountId: platform.account_id,
      label: platform.account_id,
      maxDailyPublications: platform.max_daily_publications,
      capabilities: Object.fromEntries(
        platform.capabilities.map(item => [item.capability, item.mode])
      ) as Record<SocialCapability, SocialCapabilityMode>,
    })),
    timezone: configuration.timezone,
    cadence: {
      dailyContentCron: configuration.cadence.daily_content_cron,
      inboxCron: configuration.cadence.inbox_cron,
      sevenDaySampleCron: configuration.cadence.seven_day_sample_cron,
      weeklyReviewCron: configuration.cadence.weekly_review_cron,
    },
    handoff: {
      connectorId: configuration.team_handoff.connector_id,
      connectionId: configuration.team_handoff.connection_id,
      operationId: configuration.team_handoff.operation_id,
      destination: configuration.team_handoff.destination,
    },
  };
}

function trimmed(value: string, maximum: number) {
  const result = value.trim();
  return result && result.length <= maximum ? result : undefined;
}

function uuid(value: string) {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value);
}

export function buildSocialOperationsSettings(
  draft: SocialOperationsDraft
): { settings: SocialOperationsSettings } | { error: SocialOperationsDraftError } {
  const productName = trimmed(draft.productName, 256);
  const productDescription = trimmed(draft.productDescription, 4000);
  if (!productName || !productDescription) return { error: 'product' };
  const audience = trimmed(draft.audience, 2000);
  if (!audience) return { error: 'audience' };
  const publicationLanguage = trimmed(draft.publicationLanguage, 64);
  const internalReviewLanguage = trimmed(draft.internalReviewLanguage, 64);
  if (!publicationLanguage || !internalReviewLanguage) return { error: 'languages' };

  const sourceUrls = draft.sourceUrls.map(value => value.trim()).filter(Boolean);
  const sourceArtifactIds = draft.sourceArtifactIds.map(value => value.trim()).filter(Boolean);
  const uniqueSources = new Set([...sourceUrls, ...sourceArtifactIds.map(id => `artifact:${id}`)]);
  if (
    uniqueSources.size === 0 ||
    uniqueSources.size !== sourceUrls.length + sourceArtifactIds.length ||
    sourceUrls.some(value => {
      try {
        const parsed = new URL(value);
        return (
          parsed.protocol !== 'https:' ||
          !parsed.hostname ||
          Boolean(parsed.username || parsed.password)
        );
      } catch {
        return true;
      }
    }) ||
    sourceArtifactIds.some(id => !uuid(id))
  ) {
    return { error: 'sources' };
  }

  const brandArtifactRefs = draft.brandArtifactRefs.map(value => value.trim()).filter(Boolean);
  if (
    new Set(brandArtifactRefs).size !== brandArtifactRefs.length ||
    brandArtifactRefs.some(id => !uuid(id))
  ) {
    return { error: 'brandArtifacts' };
  }
  const research = {
    connectorId: trimmed(draft.research.connectorId, 128),
    connectionId: trimmed(draft.research.connectionId, 128),
  };
  if (!research.connectorId || !research.connectionId || !uuid(research.connectionId)) {
    return { error: 'research' };
  }
  if (draft.platforms.length === 0 || draft.platforms.length > 10) return { error: 'platforms' };
  if (draft.platforms.some(item => !trimmed(item.accountId, 256))) return { error: 'account' };
  if (
    draft.platforms.some(
      item =>
        !Number.isInteger(item.maxDailyPublications) ||
        item.maxDailyPublications < 1 ||
        item.maxDailyPublications > 100
    )
  ) {
    return { error: 'dailyLimit' };
  }
  const timezone = trimmed(draft.timezone, 128);
  if (!timezone) return { error: 'timezone' };
  const dailyContentCron = trimmed(draft.cadence.dailyContentCron, 256);
  const inboxCron = trimmed(draft.cadence.inboxCron, 256);
  const sevenDaySampleCron = trimmed(draft.cadence.sevenDaySampleCron, 256);
  const weeklyReviewCron = trimmed(draft.cadence.weeklyReviewCron, 256);
  if (!dailyContentCron || !inboxCron || !sevenDaySampleCron || !weeklyReviewCron) {
    return { error: 'cadence' };
  }
  const handoff = {
    connectorId: trimmed(draft.handoff.connectorId, 128),
    connectionId: trimmed(draft.handoff.connectionId, 128),
    operationId: trimmed(draft.handoff.operationId, 128),
    destination: trimmed(draft.handoff.destination, 512),
  };
  if (
    !handoff.connectorId ||
    !handoff.connectionId ||
    !handoff.operationId ||
    !handoff.destination ||
    !handoff.operationId.startsWith(`${handoff.connectorId}.`)
  ) {
    return { error: 'handoff' };
  }

  return {
    settings: {
      product: { name: productName, description: productDescription },
      audience,
      publication_language: publicationLanguage,
      internal_review_language: internalReviewLanguage,
      sources: [
        ...sourceUrls.map(url => ({ url })),
        ...sourceArtifactIds.map(artifact_id => ({ artifact_id })),
      ],
      brand_artifact_refs: brandArtifactRefs,
      research: {
        connector_id: research.connectorId,
        connection_id: research.connectionId,
      },
      platforms: draft.platforms.map(item => ({
        platform: item.platform,
        connector_id: item.connectorId,
        connection_id: item.connectionId,
        account_id: item.accountId.trim(),
        max_daily_publications: item.maxDailyPublications,
        capabilities: SOCIAL_CAPABILITIES.map(capability => ({
          capability,
          mode:
            item.platform === 'linkedin' && capability === 'inbox'
              ? 'optional'
              : item.capabilities[capability],
        })),
      })),
      timezone,
      cadence: {
        daily_content_cron: dailyContentCron,
        inbox_cron: inboxCron,
        seven_day_sample_cron: sevenDaySampleCron,
        weekly_review_cron: weeklyReviewCron,
      },
      team_handoff: {
        connector_id: handoff.connectorId,
        connection_id: handoff.connectionId,
        operation_id: handoff.operationId,
        destination: handoff.destination,
      },
    },
  };
}
