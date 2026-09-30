'use client';

import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { ConnectorAppSetupPanel } from '@/components/connectors/setup/connector-app-setup-panel';
import { ConnectorMessagingSetupPanel } from '@/components/connectors/setup/connector-messaging-setup-panel';
import { supportsMessagingContract } from '@/components/connectors/setup/messaging-setup-state';
import { useT } from '@/i18n';
import type { ConnectorCatalogItem, PersonalConnection } from '@/services/types/connector';

export function ConnectionAppDialog({
  open,
  onOpenChange,
  connector,
  connection,
  mode = 'manual',
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  connector: ConnectorCatalogItem;
  connection: PersonalConnection;
  mode?: 'manual' | 'qr';
}) {
  const t = useT('connectors');
  const messaging = supportsMessagingContract(connector);
  const SetupPanel = messaging ? ConnectorMessagingSetupPanel : ConnectorAppSetupPanel;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="md">
        <DialogHeader>
          <DialogTitle>
            {t(
              messaging
                ? 'messaging.reauthorizeTitle'
                : mode === 'qr'
                  ? 'app.repairTitle'
                  : 'app.updateTitle'
            )}
          </DialogTitle>
        </DialogHeader>
        <DialogBody>
          {open && (
            <SetupPanel
              connector={connector}
              updateConnection={connection}
              initialMode={mode}
              embedded
            />
          )}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('reconnect.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
