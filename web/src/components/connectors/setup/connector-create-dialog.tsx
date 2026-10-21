'use client';

import { ArrowLeft } from 'lucide-react';
import type { ReactNode } from 'react';
import { DialogDescription } from '@/components/ui/dialog';
import type { ConnectorKeySetupPanelProps } from './connector-key-setup-panel';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { useT } from '@/i18n';
import type { ConnectorCatalogItem } from '@/services/types/connector';
import { ConnectorKeySetupPanel } from './connector-key-setup-panel';
import { supportsMailSetup } from './mail-setup-state';
import { ConnectorMailProviderPanel } from './connector-mail-provider-panel';
import { connectorCatalogItems, useConnectorCatalog } from '@/hooks/connectors/use-connectors';
import { mailDisplayGroup } from '../mail-display-model';

interface ConnectorCreateDialogProps {
  connector: ConnectorCatalogItem;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  pendingSetupID?: string;
}

export function ConnectorCreateDialog({
  connector,
  open,
  onOpenChange,
  pendingSetupID,
}: ConnectorCreateDialogProps) {
  const t = useT('connectors');
  const mail = supportsMailSetup(connector);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="md" aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle>
            {pendingSetupID
              ? t('pending.continue')
              : t('setup.addTitle', { connector: mail ? t('mail.title') : connector.name })}
          </DialogTitle>
        </DialogHeader>
        <ConnectorCreateContent
          connector={connector}
          active={open}
          allowProviderSelection
          pendingSetupID={pendingSetupID}
          embedded
          submitLabel={t('setup.submit')}
        />

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            {t('reconnect.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** Shared existing setup body. Provider choice is opt-in for the directory only. */
export function ConnectorCreateContent({
  connector,
  active = true,
  allowProviderSelection = false,
  onBack,
  description,
  onReady,
  ...props
}: Omit<ConnectorKeySetupPanelProps, 'description'> & {
  active?: boolean;
  allowProviderSelection?: boolean;
  onBack?: () => void;
  description?: ReactNode;
}) {
  const t = useT('connectors');
  const mail = supportsMailSetup(connector);
  const catalog = useConnectorCatalog(undefined, active && mail && allowProviderSelection);
  const providers = allowProviderSelection
    ? (mailDisplayGroup(connectorCatalogItems(catalog.data?.data))?.providers ?? [connector])
    : [connector];
  const ready: ConnectorKeySetupPanelProps['onReady'] = onReady
    ? (connection, result) => {
        if (
          connection.connector_id !== connector.id &&
          (!allowProviderSelection || !providers.some(item => item.id === connection.connector_id))
        ) {
          return;
        }
        onReady(connection, result);
      }
    : undefined;
  return (
    <>
      {onBack ? (
        <DialogHeader className="border-b pb-4 pr-12">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="mb-2 w-fit px-2"
            onClick={onBack}
          >
            <ArrowLeft aria-hidden="true" className="size-4" />
            {t('picker.backToChoices')}
          </Button>
          <DialogTitle>{t('picker.setupTitle', { connector: connector.name })}</DialogTitle>
          {description ? <DialogDescription>{description}</DialogDescription> : null}
        </DialogHeader>
      ) : null}
      <DialogBody className={onBack ? 'p-5' : undefined}>
        {active &&
          (mail && allowProviderSelection ? (
            <ConnectorMailProviderPanel
              {...props}
              onReady={ready}
              providers={providers}
              preferredProvider={connector.id}
            />
          ) : (
            <ConnectorKeySetupPanel {...props} connector={connector} onReady={ready} />
          ))}
      </DialogBody>
    </>
  );
}
