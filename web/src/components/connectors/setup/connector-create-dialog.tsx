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
}

export function ConnectorCreateDialog({
  connector,
  open,
  onOpenChange,
}: ConnectorCreateDialogProps) {
  const t = useT('connectors');
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="md">
        <DialogHeader>
          <DialogTitle>{t('setup.addTitle', { connector: connector.name })}</DialogTitle>
          <DialogDescription>
            {t(supportsAppSetup(connector) ? 'app.notice' : 'setup.addDescription')}
          </DialogDescription>
        </DialogHeader>
        <DialogBody>
          <ConnectorKeySetupPanel connector={connector} embedded submitLabel={t('setup.submit')} />
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
