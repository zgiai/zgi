'use client';

import React from 'react';
import { Button } from '@/components/ui/button';
import { RefreshCw } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';
import { ProviderSyncDialog } from '@/components/providers/provider-sync-dialog';
import { useModelMetaStatus } from '@/hooks/provider/use-sync-provider-models';
import { useIsSuperAdmin } from '@/store/auth-store';
import { IS_CLOUD } from '@/lib/config';

export function ProviderSyncButton() {
  const t = useT('aiProviders');
  const isSuperAdmin = useIsSuperAdmin();
  const canUseModelMetaSync = !IS_CLOUD && isSuperAdmin;
  const statusQuery = useModelMetaStatus({ enabled: canUseModelMetaSync });
  const { data: statusResponse } = statusQuery;
  const [open, setOpen] = React.useState(false);
  const hasUpdates = Boolean(statusResponse?.data.has_updates);
  const isDegraded = Boolean(statusResponse?.data.degraded);

  if (!canUseModelMetaSync) {
    return null;
  }

  return (
    <>
      <Button
        variant={hasUpdates ? 'default' : 'outline'}
        size="sm"
        onClick={() => setOpen(true)}
        className="gap-2"
      >
        <span className="relative">
          <RefreshCw className={cn('h-4 w-4')} />
          {hasUpdates || isDegraded ? (
            <span className="absolute -right-1 -top-1 size-2 rounded-full bg-warning" />
          ) : null}
        </span>
        {t('syncStatus.trigger')}
      </Button>
      <ProviderSyncDialog
        open={open}
        onOpenChange={setOpen}
        onStatusRefresh={() => statusQuery.refetch()}
      />
    </>
  );
}
