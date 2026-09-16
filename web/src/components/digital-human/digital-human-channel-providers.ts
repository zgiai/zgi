import {
  FeishuCredentialDialog,
  FeishuInstallationSetupGuide,
  FeishuInstallationVerificationStatus,
} from '@/components/digital-human/channel-providers/feishu';
import { DingTalkCredentialDialog } from '@/components/digital-human/channel-providers/dingtalk';
import type { DigitalHumanChannelProviderBase } from '@/components/digital-human/channel-providers/types';

export const DIGITAL_HUMAN_CHANNEL_PROVIDERS = [
  {
    id: 'feishu',
    nameKey: 'channels.providers.feishu.name',
    accentClassName: 'bg-blue-500/10 text-blue-600',
    connectionTestMode: 'message_round_trip',
    credentialUI: {
      Dialog: FeishuCredentialDialog,
    },
    installationUI: {
      SetupGuide: FeishuInstallationSetupGuide,
      VerificationStatus: FeishuInstallationVerificationStatus,
    },
  },
  {
    id: 'dingtalk',
    nameKey: 'channels.providers.dingtalk.name',
    accentClassName: 'bg-sky-500/10 text-sky-600',
    connectionTestMode: 'basic',
    credentialUI: {
      Dialog: DingTalkCredentialDialog,
    },
    installationUI: undefined,
  },
] as const satisfies readonly DigitalHumanChannelProviderBase[];

export type DigitalHumanChannelProviderDefinition =
  (typeof DIGITAL_HUMAN_CHANNEL_PROVIDERS)[number];

export function digitalHumanChannelProvider(
  providerID: string
): DigitalHumanChannelProviderDefinition | undefined {
  return DIGITAL_HUMAN_CHANNEL_PROVIDERS.find(provider => provider.id === providerID);
}
