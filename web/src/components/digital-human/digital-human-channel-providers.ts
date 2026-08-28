import {
  FeishuCredentialDialog,
  FeishuCredentialFields,
  FeishuInstallationSetupGuide,
  FeishuInstallationVerificationStatus,
  createEmptyFeishuCredentialDraft,
  isFeishuCredentialComplete,
  serializeFeishuCredential,
} from '@/components/digital-human/channel-providers/feishu';
import {
  DingTalkCredentialDialog,
  DingTalkCredentialFields,
  createEmptyDingTalkCredentialDraft,
  isDingTalkCredentialComplete,
  serializeDingTalkCredential,
} from '@/components/digital-human/channel-providers/dingtalk';
import type { DigitalHumanChannelProviderBase } from '@/components/digital-human/channel-providers/types';

export const DIGITAL_HUMAN_CHANNEL_PROVIDERS = [
  {
    id: 'feishu',
    integrationId: 'feishu',
    driverId: 'feishu-open-api',
    authMethodId: 'feishu_tenant_app',
    credentialSource: 'organization',
    authType: 'service_account',
    region: 'cn',
    nameKey: 'channels.providers.feishu.name',
    descriptionKey: 'channels.providers.feishu.description',
    defaultBotNameKey: 'channels.providers.feishu.defaultBotName',
    enabled: true,
    accentClassName: 'bg-blue-500/10 text-blue-600',
    connectionTestMode: 'message_round_trip',
    credentialUI: {
      createEmptyDraft: createEmptyFeishuCredentialDraft,
      serialize: serializeFeishuCredential,
      isComplete: isFeishuCredentialComplete,
      Fields: FeishuCredentialFields,
      Dialog: FeishuCredentialDialog,
    },
    installationUI: {
      SetupGuide: FeishuInstallationSetupGuide,
      VerificationStatus: FeishuInstallationVerificationStatus,
    },
    connectionFilter: connection =>
      connection.integration_id === 'feishu' &&
      connection.driver_id === 'feishu-open-api' &&
      connection.credential_source === 'organization' &&
      connection.auth_method_id === 'feishu_tenant_app' &&
      connection.status === 'active',
  },
  {
    id: 'dingtalk',
    integrationId: 'dingtalk',
    driverId: 'dingtalk-open-api',
    authMethodId: 'organization_dingtalk_internal_app',
    credentialSource: 'organization',
    authType: 'service_account',
    region: 'cn',
    nameKey: 'channels.providers.dingtalk.name',
    descriptionKey: 'channels.providers.dingtalk.description',
    defaultBotNameKey: 'channels.providers.dingtalk.defaultBotName',
    enabled: true,
    accentClassName: 'bg-sky-500/10 text-sky-600',
    connectionTestMode: 'basic',
    credentialUI: {
      createEmptyDraft: createEmptyDingTalkCredentialDraft,
      serialize: serializeDingTalkCredential,
      isComplete: isDingTalkCredentialComplete,
      Fields: DingTalkCredentialFields,
      Dialog: DingTalkCredentialDialog,
    },
    installationUI: undefined,
    connectionFilter: connection =>
      connection.integration_id === 'dingtalk' &&
      connection.driver_id === 'dingtalk-open-api' &&
      connection.credential_source === 'organization' &&
      connection.auth_method_id === 'organization_dingtalk_internal_app' &&
      connection.status === 'active',
  },
] as const satisfies readonly DigitalHumanChannelProviderBase[];

export type DigitalHumanChannelProviderDefinition =
  (typeof DIGITAL_HUMAN_CHANNEL_PROVIDERS)[number];

export const ENABLED_DIGITAL_HUMAN_CHANNEL_PROVIDERS = DIGITAL_HUMAN_CHANNEL_PROVIDERS.filter(
  provider => provider.enabled
);

export function digitalHumanChannelProvider(
  providerID: string
): DigitalHumanChannelProviderDefinition | undefined {
  return DIGITAL_HUMAN_CHANNEL_PROVIDERS.find(provider => provider.id === providerID);
}
