import type { ComponentType } from 'react';
import type { DigitalHumanChannelInstallation } from '@/services/types/digital-human';
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
  credentialUI: ChannelCredentialUI;
  connectionFilter?: (connection: IntegrationConnection) => boolean;
}
