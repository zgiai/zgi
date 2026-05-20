'use client';

import React, { useCallback } from 'react';
import {
  isModelMetaForbiddenError,
  isModelMetaReadOnlyError,
  useSyncModels,
  useCheckModelUpdates,
  useSyncProviderFull,
} from '@/hooks/provider/use-sync-provider-models';
import { Button } from '@/components/ui/button';
import { RefreshCw } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';
import ModelDiffDialog from '@/components/providers/model-diff-dialog';
import type { ModelMetaSyncResult } from '@/services/types/provider';
import { useIsSuperAdmin } from '@/store/auth-store';
import { IS_CLOUD } from '@/lib/config';

interface ModelUpdatesButtonProps {
  providerId: string;
  disabled?: boolean;
}

export function ModelUpdatesButton({ providerId, disabled }: ModelUpdatesButtonProps) {
  const t = useT('aiProviders');
  const isSuperAdmin = useIsSuperAdmin();
  const canUseModelMetaSync = !IS_CLOUD && isSuperAdmin;
  const { mutate: syncSelectedModels, isPending: isSyncingSelected } = useSyncModels(providerId);
  const { mutate: syncProviderFull, isPending: isSyncingProvider } = useSyncProviderFull();
  const [syncFeedback, setSyncFeedback] = React.useState<ModelMetaSyncResult | null>(null);
  const [blockedState, setBlockedState] = React.useState<'readonly' | 'forbidden' | null>(null);

  const {
    isCheckingUpdates,
    showDiffDialog,
    setShowDiffDialog,
    diffData,
    setDiffData,
    onCheckUpdates,
  } = useCheckModelUpdates(providerId);

  const handleSyncSelected = useCallback(
    (models: string[]) => {
      setSyncFeedback(null);
      syncSelectedModels(models, {
        onSuccess: response => {
          setBlockedState(null);
          if (response.data.status === 'success') {
            setShowDiffDialog(false);
            setDiffData(null);
            setSyncFeedback(null);
            return;
          }

          setSyncFeedback(response.data);
        },
        onError: error => {
          if (isModelMetaReadOnlyError(error)) {
            setBlockedState('readonly');
          } else if (isModelMetaForbiddenError(error)) {
            setBlockedState('forbidden');
          }
        },
      });
    },
    [syncSelectedModels, setShowDiffDialog, setDiffData]
  );

  const handleSyncProviderFull = useCallback(() => {
    setSyncFeedback(null);
    syncProviderFull(providerId, {
      onSuccess: response => {
        setBlockedState(null);
        if (response.data.status === 'success') {
          setShowDiffDialog(false);
          setDiffData(null);
          setSyncFeedback(null);
          return;
        }

        setSyncFeedback(response.data);
      },
      onError: error => {
        if (isModelMetaReadOnlyError(error)) {
          setBlockedState('readonly');
        } else if (isModelMetaForbiddenError(error)) {
          setBlockedState('forbidden');
        }
      },
    });
  }, [providerId, setDiffData, setShowDiffDialog, syncProviderFull]);

  React.useEffect(() => {
    if (!showDiffDialog) {
      setSyncFeedback(null);
    }
  }, [showDiffDialog]);

  if (!canUseModelMetaSync) {
    return null;
  }

  return (
    <>
      <Button
        size="sm"
        variant="outline"
        onClick={onCheckUpdates}
        disabled={disabled || isCheckingUpdates || blockedState === 'forbidden'}
        className="gap-2"
      >
        <RefreshCw className={cn('h-3.5 w-3.5', isCheckingUpdates && 'animate-spin')} />
        {isCheckingUpdates
          ? t('sidebar.checkingUpdates')
          : t('sidebar.checkUpdates')}
      </Button>

      <ModelDiffDialog
        open={showDiffDialog}
        onOpenChange={setShowDiffDialog}
        diffData={diffData}
        onSyncSelected={handleSyncSelected}
        isSyncing={isSyncingSelected}
        onSyncProviderFull={handleSyncProviderFull}
        isSyncingProvider={isSyncingProvider}
        syncFeedback={syncFeedback}
        syncBlockedState={blockedState}
      />
    </>
  );
}
