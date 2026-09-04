'use client';

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
import { useT } from '@/i18n';
import type { ConnectorCatalogItem } from '@/services/types/connector';
import { ConnectorKeySetupPanel } from './connector-key-setup-panel';
import { supportsAppSetup } from './app-setup-state';

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
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="md">
        <DialogHeader>
          <DialogTitle>
            {pendingSetupID
              ? t('pending.continue')
              : t('setup.addTitle', { connector: connector.name })}
          </DialogTitle>
          <DialogDescription>
            {t(supportsAppSetup(connector) ? 'app.notice' : 'setup.addDescription')}
          </DialogDescription>
        </DialogHeader>
        <DialogBody>
          {open && (
            <ConnectorKeySetupPanel
              connector={connector}
              pendingSetupID={pendingSetupID}
              embedded
              submitLabel={t('setup.submit')}
            />
          )}
        </DialogBody>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            {t('reconnect.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
