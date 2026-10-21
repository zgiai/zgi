'use client';

import { useCallback, useMemo, useState } from 'react';
import { AlertCircle, ArrowLeft } from 'lucide-react';
import { ConnectionAppDialog } from '@/components/connectors/connections/connection-app-dialog';
import { ConnectionReconnectDialog } from '@/components/connectors/connections/connection-reconnect-dialog';
import {
  ConnectorPickerDialog,
  type ConnectorPickerActionRequest,
} from '@/components/connectors/picker';
import { ConnectorKeySetupPanel } from '@/components/connectors/setup/connector-key-setup-panel';
import {
  supportsAppSetup,
  supportsMessagingContract,
} from '@/components/connectors/setup/app-setup-state';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { DIGITAL_HUMAN_CHANNEL_PROVIDERS } from '@/components/digital-human/digital-human-channel-providers';
import { useDigitalHumanChannelMutations } from '@/hooks/digital-human/use-digital-human-channels';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import { localizeConnectorCatalogItem } from '@/components/connectors/connector-metadata';
import type { ConnectorCatalogItem, PersonalConnection } from '@/services/types/connector';
import { getErrorMessage } from '@/utils/error-notifications';

const ENTERPRISE_TAG = 'enterprise' as const;
const MESSAGING_TAG = 'messaging' as const;

function isChannelConnector(
  connector: ConnectorCatalogItem,
  channelProviderIDs: ReadonlySet<string>
) {
  const tags = connector.tags ?? [];
  return (
    channelProviderIDs.has(connector.id) &&
    tags.includes(ENTERPRISE_TAG) &&
    (tags.includes(MESSAGING_TAG) || connector.category === 'communication')
  );
}

export function ChannelInstallationSetupDialog({
  open,
  onOpenChange,
  id,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  id: string;
}) {
  const t = useT('digitalHumans');
  const connectorT = useT('connectors');
  const { locale } = useLocale();
  const mutations = useDigitalHumanChannelMutations(id);
  const [setupConnector, setSetupConnector] = useState<ConnectorCatalogItem>();
  const [repair, setRepair] = useState<{
    connector: ConnectorCatalogItem;
    connection: PersonalConnection;
  }>();
  const [actionError, setActionError] = useState<string>();
  const channelProviderIDs = useMemo(
    () => new Set(DIGITAL_HUMAN_CHANNEL_PROVIDERS.map(provider => provider.id)),
    []
  );
  const catalogFilter = useCallback(
    (connector: ConnectorCatalogItem) => isChannelConnector(connector, channelProviderIDs),
    [channelProviderIDs]
  );

  const close = () => {
    setSetupConnector(undefined);
    setRepair(undefined);
    setActionError(undefined);
    onOpenChange(false);
  };

  const createInstallation = async (
    connector: ConnectorCatalogItem,
    connection: PersonalConnection
  ) => {
    setActionError(undefined);
    try {
      await mutations.createInstallation.mutateAsync({
        provider: connector.id,
        connector_id: connector.id,
        connection_id: connection.id,
        name: connection.name?.trim() || localizeConnectorCatalogItem(connector, locale).name,
      });
      close();
    } catch (error) {
      setActionError(getErrorMessage(error));
    }
  };

  const handleSetupReady = async (connection: PersonalConnection) => {
    if (!setupConnector) return;
    try {
      await createInstallation(setupConnector, connection);
    } catch {
      // The dialog keeps the setup state and presents the safe error banner.
    }
  };

  const handlePickerAdd = async (
    request: ConnectorPickerActionRequest & { connection: PersonalConnection }
  ) => {
    await createInstallation(request.connector, request.connection);
  };

  const handleConnectAndAdd = (request: ConnectorPickerActionRequest) => {
    setActionError(undefined);
    setSetupConnector(request.connector);
  };

  const handleUpdateKey = (
    request: ConnectorPickerActionRequest & { connection: PersonalConnection }
  ) => {
    setActionError(undefined);
    setRepair({ connector: request.connector, connection: request.connection });
  };

  const pickerOpen = open && !setupConnector && !repair;
  const repairProvider = repair
    ? DIGITAL_HUMAN_CHANNEL_PROVIDERS.find(provider => provider.id === repair.connector.id)
    : undefined;

  return (
    <>
      <ConnectorPickerDialog
        open={pickerOpen}
        target={{ type: 'digital_human', id, scope: 'draft' }}
        targetName={t('channels.addDialog.target')}
        title={t('channels.addDialog.title')}
        description={t('channels.addDialog.description')}
        catalogParams={{ tag: ENTERPRISE_TAG }}
        catalogFilter={catalogFilter}
        bindings={[]}
        draft
        groupByConnector
        showUnavailableReferences={false}
        onOpenChange={nextOpen => {
          if (!nextOpen) close();
        }}
        onRequestAdd={handlePickerAdd}
        onRequestSwitch={async request => {
          if (request.connection) await createInstallation(request.connector, request.connection);
        }}
        onRequestConnectAndAdd={handleConnectAndAdd}
        onRequestUpdateKey={handleUpdateKey}
        onRequestViewReason={() => setActionError(t('channels.errors.installation'))}
        onRequestDetach={async () => undefined}
      >
        {actionError ? (
          <Alert variant="destructive" className="mt-3">
            <AlertCircle className="size-4" aria-hidden="true" />
            <AlertTitle>{t('channels.installations.credential.createError')}</AlertTitle>
            <AlertDescription>{actionError}</AlertDescription>
          </Alert>
        ) : null}
      </ConnectorPickerDialog>

      {setupConnector ? (
        <Dialog
          open
          onOpenChange={nextOpen => {
            if (!nextOpen) setSetupConnector(undefined);
          }}
        >
          <DialogContent size="md">
            <DialogHeader>
              <Button
                variant="ghost"
                className="mb-2 w-fit"
                onClick={() => setSetupConnector(undefined)}
              >
                <ArrowLeft className="size-4" />
                {connectorT('picker.backToChoices')}
              </Button>
              <DialogTitle>
                {connectorT('picker.setupTitle', {
                  connector: localizeConnectorCatalogItem(setupConnector, locale).name,
                })}
              </DialogTitle>
              <DialogDescription>{t('channels.addDialog.setupDescription')}</DialogDescription>
            </DialogHeader>
            <DialogBody>
              {actionError ? (
                <Alert variant="destructive" className="mb-4">
                  <AlertCircle className="size-4" aria-hidden="true" />
                  <AlertTitle>{t('channels.installations.credential.createError')}</AlertTitle>
                  <AlertDescription>{actionError}</AlertDescription>
                </Alert>
              ) : null}
              <ConnectorKeySetupPanel
                connector={setupConnector}
                embedded
                onReady={connection => void handleSetupReady(connection)}
              />
            </DialogBody>
            <DialogFooter>
              <Button variant="outline" onClick={close}>
                {t('common.cancel')}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      ) : null}

      {repair && repairProvider ? (
        supportsAppSetup(repair.connector) || supportsMessagingContract(repair.connector) ? (
          <ConnectionAppDialog
            open
            connector={repair.connector}
            connection={repair.connection}
            onOpenChange={nextOpen => {
              if (!nextOpen) setRepair(undefined);
            }}
          />
        ) : (
          <ConnectionReconnectDialog
            open
            connector={repair.connector}
            connection={repair.connection}
            connectionId={repair.connection.id}
            onOpenChange={nextOpen => {
              if (!nextOpen) setRepair(undefined);
            }}
          />
        )
      ) : null}
    </>
  );
}
