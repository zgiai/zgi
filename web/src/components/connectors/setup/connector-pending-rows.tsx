'use client';

import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { Clock3, MoreHorizontal } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useT } from '@/i18n';
import { connectorService } from '@/services/connector.service';
import { pendingSetupKeys } from '@/hooks/connectors/use-pending-setups';
import type { ConnectorPendingSetup } from '@/services/types/connector';
import { useAuthStore } from '@/store/auth-store';
import { useOrganizationStore } from '@/store/organization-store';

export function ConnectorPendingRows({
  items,
  onContinue,
}: {
  items: ConnectorPendingSetup[];
  onContinue: (id: string) => void;
}) {
  const t = useT('connectors');
  const queryClient = useQueryClient();
  const [removing, setRemoving] = useState<ConnectorPendingSetup>();
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  const owner = useAuthStore.use.user()?.id;
  const org = useOrganizationStore.use.currentOrganization()?.id;
  const [scope, setScope] = useState(`${org}:${owner}`);
  if (scope !== `${org}:${owner}`) {
    setScope(`${org}:${owner}`);
    setRemoving(undefined);
    setBusy(false);
    setFailed(false);
  }
  const cancel = async () => {
    if (!removing || busy) return;
    setBusy(true);
    setFailed(false);
    const current = () =>
      useAuthStore.getState().user?.id === owner &&
      useOrganizationStore.getState().currentOrganization?.id === org;
    try {
      await connectorService.cancelPendingSetup(removing.id, removing.revision);
      if (current()) setRemoving(undefined);
      await queryClient.invalidateQueries({ queryKey: pendingSetupKeys });
    } catch {
      if (current()) setFailed(true);
    } finally {
      if (current()) setBusy(false);
    }
  };
  return (
    <>
      {items.length > 0 && (
        <div className="space-y-2 border-t pt-3">
          <p className="text-xs font-medium text-muted-foreground">
            {t('pending.count', { count: items.length })}
          </p>
          {items.map(item => (
            <div
              key={item.id}
              className="flex min-w-0 items-center gap-2 rounded-lg border border-warning/20 p-3"
            >
              <Clock3 className="size-4 shrink-0 text-warning" aria-hidden="true" />
              <div className="min-w-0 flex-1">
                <p className="truncate font-mono text-xs" title={item.app_id}>
                  {item.app_id}
                </p>
                <p className="mt-1 text-xs text-muted-foreground">
                  {t(
                    item.state === 'awaiting_application'
                      ? 'pending.awaitingShort'
                      : 'pending.verifyShort'
                  )}{' '}
                  · {t(item.region === 'feishu' ? 'app.feishu' : 'app.lark')}
                </p>
                {item.last_verification_at && (
                  <p className="mt-1 truncate text-xs text-muted-foreground">
                    {t('pending.lastChecked', {
                      date: new Date(item.last_verification_at).toLocaleString(),
                    })}
                  </p>
                )}
              </div>
              <Button size="sm" variant="outline" onClick={() => onContinue(item.id)}>
                {t('pending.continue')}
              </Button>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    size="sm"
                    variant="ghost"
                    isIcon
                    aria-label={t('catalog.connectionActions')}
                  >
                    <MoreHorizontal className="size-4" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onSelect={() => setRemoving(item)}>
                    {t('pending.remove')}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          ))}
        </div>
      )}
      <ConfirmDialog
        open={Boolean(removing)}
        onOpenChange={open => {
          if (!open && !busy) setRemoving(undefined);
        }}
        title={t('pending.removeTitle')}
        description={failed ? t('pending.removeFailed') : t('pending.removeHint')}
        confirmText={t('pending.remove')}
        cancelText={t('reconnect.close')}
        loading={busy}
        confirmDisabled={busy}
        closeOnConfirm={false}
        onConfirm={() => void cancel()}
      />
    </>
  );
}
