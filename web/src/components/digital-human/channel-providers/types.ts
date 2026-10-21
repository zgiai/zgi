import type { ComponentType } from 'react';
import type {
  DigitalHumanChannelConnectionTest,
  DigitalHumanChannelConnectionTestConversationType,
  DigitalHumanChannelInstallation,
  DigitalHumanChannelInstallationVerification,
} from '@/services/types/digital-human';

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
  nameKey: string;
  accentClassName: string;
  connectionTestMode: 'basic' | 'message_round_trip';
  credentialUI: ChannelCredentialUI;
  installationUI?: ChannelInstallationUI;
}
