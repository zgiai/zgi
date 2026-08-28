import type { ComponentType } from 'react';
import type {
  DigitalHumanChannelConnectionTest,
  DigitalHumanChannelConnectionTestConversationType,
  DigitalHumanChannelInstallation,
  DigitalHumanChannelInstallationVerification,
} from '@/services/types/digital-human';
import type { IntegrationConnection } from '@/services/types/integration';

export type ChannelCredentialDraft = Record<string, string>;

export interface ChannelCredentialFieldsProps {
  value: ChannelCredentialDraft;
  onChange: (value: ChannelCredentialDraft) => void;
  editing?: boolean;
}

export interface ChannelCredentialDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  digitalHumanID: string;
  installation: DigitalHumanChannelInstallation;
}

export interface ChannelCredentialUI {
  createEmptyDraft: () => ChannelCredentialDraft;
  serialize: (value: ChannelCredentialDraft) => Record<string, string>;
  isComplete: (value: ChannelCredentialDraft) => boolean;
  Fields: ComponentType<ChannelCredentialFieldsProps>;
  Dialog: ComponentType<ChannelCredentialDialogProps>;
}

export interface ChannelInstallationSetupProps {
  installation: DigitalHumanChannelInstallation;
}

export interface ChannelInstallationVerificationProps {
  digitalHumanID: string;
  installation: DigitalHumanChannelInstallation;
  result?: DigitalHumanChannelInstallationVerification;
  connectionTests?: Partial<
    Record<DigitalHumanChannelConnectionTestConversationType, DigitalHumanChannelConnectionTest>
  >;
  onConnectionTestChange?: (test: DigitalHumanChannelConnectionTest) => void;
  onStartConnectionTest?: (
    conversationType: DigitalHumanChannelConnectionTestConversationType
  ) => Promise<void>;
  startingConnectionTest?: boolean;
}

export interface ChannelInstallationUI {
  SetupGuide?: ComponentType<ChannelInstallationSetupProps>;
  VerificationStatus?: ComponentType<ChannelInstallationVerificationProps>;
}

export interface DigitalHumanChannelProviderBase {
  id: string;
  integrationId: string;
  driverId: string;
  authMethodId: string;
  credentialSource: 'organization';
  authType: 'service_account';
  region: string;
  nameKey: string;
  descriptionKey: string;
  defaultBotNameKey: string;
  enabled: boolean;
  accentClassName: string;
  connectionTestMode: 'basic' | 'message_round_trip';
  credentialUI: ChannelCredentialUI;
  installationUI?: ChannelInstallationUI;
  connectionFilter?: (connection: IntegrationConnection) => boolean;
}
