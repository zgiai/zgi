'use client';

import React from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useRouter } from 'next/navigation';
import { AlertTriangle, ExternalLink, RefreshCw } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
  DialogFooter,
} from '@/components/ui/dialog';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Separator } from '@/components/ui/separator';
import { MODEL_META_KEYS } from '@/hooks/query-keys';
import { useT } from '@/i18n';
import {
  isModelMetaForbiddenError,
  useModelMetaModelUpdateProviders,
  isModelMetaReadOnlyError,
  useModelMetaProviderDiff,
  useModelMetaStatus,
  useSyncProviderModelsAction,
  useSyncProviderFull,
} from '@/hooks/provider/use-sync-provider-models';
import { cn } from '@/lib/utils';
import { useIsSuperAdmin } from '@/store/auth-store';
import type {
  ModelMetaModelUpdateProviderItem,
  ModelMetaProviderDiffItem,
  ModelMetaSyncResult,
} from '@/services/types/provider';
import { formatDate } from '@/utils/format';
import { IS_CLOUD } from '@/lib/config';

interface ProviderSyncDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onStatusRefresh?: () => void | Promise<unknown>;
}

type BlockedState = 'readonly' | 'forbidden' | null;

const STATUS_ORDER: Record<ModelMetaProviderDiffItem['status'], number> = {
  new: 0,
  updated: 1,
  local_only: 2,
  unchanged: 3,
};

/**
 * @component ProviderSyncDialog
 * @category Feature
 * @status Stable
 * @description Catalog sync dialog for reviewing ModelMeta status and provider-level changes.
 * @usage Use from provider list/sidebar sync entry points.
 */
export function ProviderSyncDialog({
  open,
  onOpenChange,
  onStatusRefresh,
}: ProviderSyncDialogProps): React.JSX.Element | null {
  const queryClient = useQueryClient();
  const router = useRouter();
  const t = useT('aiProviders');
  const tCommon = useT('common');
  const isSuperAdmin = useIsSuperAdmin();
  const canUseModelMetaSync = !IS_CLOUD && isSuperAdmin;

  const statusQuery = useModelMetaStatus({ enabled: open && canUseModelMetaSync });
  const providerDiffQuery = useModelMetaProviderDiff({ enabled: open && canUseModelMetaSync });
  const modelUpdateProvidersQuery = useModelMetaModelUpdateProviders({
    enabled: open && canUseModelMetaSync,
  });
  const { mutateAsync: syncProvider } = useSyncProviderFull();
  const { mutateAsync: syncProviderModels } = useSyncProviderModelsAction();

  const [syncingProviders, setSyncingProviders] = React.useState<Set<string>>(() => new Set());
  const [blockedState, setBlockedState] = React.useState<BlockedState>(null);
  const [syncResultsByProvider, setSyncResultsByProvider] = React.useState<
    Record<string, ModelMetaSyncResult>
  >({});
  const [locallyUpdatedProviders, setLocallyUpdatedProviders] = React.useState<Set<string>>(
    () => new Set()
  );
  const [needsStatusRefreshOnClose, setNeedsStatusRefreshOnClose] = React.useState(false);

  const status = statusQuery.data?.data;
  const providerDiff = providerDiffQuery.data?.data;
  const modelUpdateProviders = modelUpdateProvidersQuery.data;
  const queryError =
    statusQuery.error ?? providerDiffQuery.error ?? modelUpdateProvidersQuery.error;
  const effectiveBlockedState: BlockedState =
    blockedState ??
    (isModelMetaReadOnlyError(queryError)
      ? 'readonly'
      : isModelMetaForbiddenError(queryError)
        ? 'forbidden'
        : null);

  const changedProviders = React.useMemo(
    () =>
      [...(providerDiff?.items ?? [])]
        .filter(item => item.status !== 'unchanged')
        .sort((left, right) => {
          const byStatus = STATUS_ORDER[left.status] - STATUS_ORDER[right.status];
          if (byStatus !== 0) return byStatus;
          return left.name.localeCompare(right.name);
        }),
    [providerDiff?.items]
  );
  const changedProviderNames = React.useMemo(
    () => new Set(changedProviders.map(item => item.provider)),
    [changedProviders]
  );
  const modelOnlyProviders = React.useMemo(
    () =>
      (modelUpdateProviders?.items ?? []).filter(item => !changedProviderNames.has(item.provider)),
    [changedProviderNames, modelUpdateProviders?.items]
  );

  const isLoading =
    statusQuery.isLoading || providerDiffQuery.isLoading || modelUpdateProvidersQuery.isLoading;
  const isRefreshing =
    statusQuery.isFetching || providerDiffQuery.isFetching || modelUpdateProvidersQuery.isFetching;
  const hasModelOnlyUpdates =
    modelOnlyProviders.length > 0 ||
    (status?.models.new ?? 0) > 0 ||
    (status?.models.updated ?? 0) > 0;

  const markProviderUpdated = React.useCallback((provider: string) => {
    setLocallyUpdatedProviders(previous => {
      const next = new Set(previous);
      next.add(provider);
      return next;
    });
    setNeedsStatusRefreshOnClose(true);
  }, []);

  const setProviderSyncing = React.useCallback((provider: string, syncing: boolean) => {
    setSyncingProviders(previous => {
      const next = new Set(previous);
      if (syncing) {
        next.add(provider);
      } else {
        next.delete(provider);
      }
      return next;
    });
  }, []);

  const setProviderSyncResult = React.useCallback(
    (provider: string, result: ModelMetaSyncResult | null) => {
      setSyncResultsByProvider(previous => {
        if (result === null) {
          if (!(provider in previous)) {
            return previous;
          }

          const next = { ...previous };
          delete next[provider];
          return next;
        }

        return {
          ...previous,
          [provider]: result,
        };
      });
    },
    []
  );

  const closeDialog = React.useCallback(() => {
    const shouldRefreshStatus = needsStatusRefreshOnClose;

    setSyncingProviders(new Set());
    setBlockedState(null);
    setSyncResultsByProvider({});
    setLocallyUpdatedProviders(new Set());
    setNeedsStatusRefreshOnClose(false);
    onOpenChange(false);

    queueMicrotask(() => {
      queryClient.removeQueries({ queryKey: MODEL_META_KEYS.providerDiff() });
      queryClient.removeQueries({ queryKey: MODEL_META_KEYS.modelUpdateProviders() });

      if (shouldRefreshStatus) {
        void onStatusRefresh?.();
      }
    });
  }, [needsStatusRefreshOnClose, onOpenChange, onStatusRefresh, queryClient]);

  const handleDialogOpenChange = React.useCallback(
    (nextOpen: boolean) => {
      if (nextOpen) {
        setBlockedState(null);
        setSyncResultsByProvider({});
        onOpenChange(true);
        return;
      }

      closeDialog();
    },
    [closeDialog, onOpenChange]
  );

  const handleRefresh = React.useCallback(async () => {
    setSyncResultsByProvider({});
    setLocallyUpdatedProviders(new Set());
    setNeedsStatusRefreshOnClose(false);
    await Promise.all([
      statusQuery.refetch(),
      providerDiffQuery.refetch(),
      modelUpdateProvidersQuery.refetch(),
    ]);
  }, [modelUpdateProvidersQuery, providerDiffQuery, statusQuery]);

  const handleOpenProvider = React.useCallback(
    (provider: string) => {
      closeDialog();
      router.push(`/dashboard/provider/${encodeURIComponent(provider)}`);
    },
    [closeDialog, router]
  );

  const handleSyncProvider = React.useCallback(
    async (provider: string) => {
      setProviderSyncing(provider, true);
      setProviderSyncResult(provider, null);

      try {
        const response = await syncProvider(provider);
        if (response.data.status === 'success') {
          markProviderUpdated(provider);
          setProviderSyncResult(provider, null);
        } else {
          setProviderSyncResult(provider, response.data);
          if (response.data.status === 'partial' && response.data.success_models > 0) {
            setNeedsStatusRefreshOnClose(true);
          }
        }
        setBlockedState(null);
      } catch (error) {
        if (isModelMetaReadOnlyError(error)) {
          setBlockedState('readonly');
        } else if (isModelMetaForbiddenError(error)) {
          setBlockedState('forbidden');
        }
      } finally {
        setProviderSyncing(provider, false);
      }
    },
    [markProviderUpdated, setProviderSyncResult, setProviderSyncing, syncProvider]
  );

  const handleSyncProviderModels = React.useCallback(
    async (provider: string) => {
      setProviderSyncing(provider, true);
      setProviderSyncResult(provider, null);

      try {
        const response = await syncProviderModels({ provider });
        if (response.data.status === 'success') {
          markProviderUpdated(provider);
          setProviderSyncResult(provider, null);
        } else {
          setProviderSyncResult(provider, response.data);
          if (response.data.status === 'partial' && response.data.success_models > 0) {
            setNeedsStatusRefreshOnClose(true);
          }
        }
        setBlockedState(null);
      } catch (error) {
        if (isModelMetaReadOnlyError(error)) {
          setBlockedState('readonly');
        } else if (isModelMetaForbiddenError(error)) {
          setBlockedState('forbidden');
        }
      } finally {
        setProviderSyncing(provider, false);
      }
    },
    [markProviderUpdated, setProviderSyncResult, setProviderSyncing, syncProviderModels]
  );

  if (!canUseModelMetaSync) {
    return null;
  }

  return (
    <Dialog open={open} onOpenChange={handleDialogOpenChange}>
      <DialogContent size="xl" className="max-w-5xl p-0 overflow-hidden">
        <DialogHeader className="px-6 pt-6 pb-4">
          <DialogTitle>{t('syncStatus.title')}</DialogTitle>
          <DialogDescription>{t('syncStatus.description')}</DialogDescription>
        </DialogHeader>

        <DialogBody className="px-6 pb-6 pt-0 space-y-4">
            {effectiveBlockedState === 'readonly' ? (
              <Alert className="border-warning/40 bg-warning/5">
                <AlertTriangle className="size-4 text-warning" />
                <AlertTitle>{t('syncStatus.readonlyTitle')}</AlertTitle>
                <AlertDescription>{t('syncStatus.readonlyDescription')}</AlertDescription>
              </Alert>
            ) : null}

            {effectiveBlockedState === 'forbidden' ? (
              <Alert variant="destructive">
                <AlertTriangle className="size-4" />
                <AlertTitle>{t('syncStatus.forbiddenTitle')}</AlertTitle>
                <AlertDescription>{t('syncStatus.forbiddenDescription')}</AlertDescription>
              </Alert>
            ) : null}

            {queryError && effectiveBlockedState === null ? (
              <Alert variant="destructive">
                <AlertTriangle className="size-4" />
                <AlertTitle>{t('syncStatus.loadErrorTitle')}</AlertTitle>
                <AlertDescription>
                  {(queryError as Error).message || t('sidebar.syncError')}
                </AlertDescription>
              </Alert>
            ) : null}

            {status?.degraded ? (
              <Alert className="border-warning/40 bg-warning/5">
                <AlertTriangle className="size-4 text-warning" />
                <AlertTitle>{t('syncStatus.providerErrorsTitle')}</AlertTitle>
                <AlertDescription className="space-y-2">
                  <p>{t('syncStatus.providerErrorsDescription')}</p>
                  {status.provider_errors && status.provider_errors.length > 0 ? (
                    <div className="space-y-1">
                      {status.provider_errors.map(item => (
                        <div
                          key={`${item.provider}-${item.error}`}
                          className="rounded-md border border-warning/20 bg-background/60 px-3 py-2 text-xs"
                        >
                          <span className="font-medium">{item.provider}</span>
                          <span className="mx-1 text-muted-foreground">:</span>
                          <span>{item.error}</span>
                        </div>
                      ))}
                    </div>
                  ) : null}
                </AlertDescription>
              </Alert>
            ) : null}

            {modelUpdateProviders?.provider_errors && modelUpdateProviders.provider_errors.length > 0 ? (
              <Alert className="border-warning/40 bg-warning/5">
                <AlertTriangle className="size-4 text-warning" />
                <AlertTitle>{t('syncStatus.modelErrorsTitle')}</AlertTitle>
                <AlertDescription>
                  {t('syncStatus.modelErrorsDescription')}
                </AlertDescription>
              </Alert>
            ) : null}

            <div className="grid gap-4 md:grid-cols-2">
              <div className="rounded-xl border bg-muted/10 p-4">
                <div className="flex items-center justify-between">
                  <div>
                    <div className="text-sm font-medium">{t('syncStatus.providersTitle')}</div>
                    <div className="text-xs text-muted-foreground">
                      {status?.checked_at
                        ? t('syncStatus.checkedAt', {
                            time: formatDate(status.checked_at, 'YYYY-MM-DD HH:mm:ss'),
                          })
                        : '-'}
                    </div>
                  </div>
                  <Badge
                    variant={status?.has_updates ? 'warning' : 'success'}
                    className="capitalize"
                  >
                    {status?.has_updates
                      ? t('syncStatus.stateUpdates')
                      : t('syncStatus.stateUpToDate')}
                  </Badge>
                </div>

                <div className="mt-4 grid grid-cols-3 gap-3 text-sm">
                  <SummaryItem label={t('diff.new')} value={status?.providers.new ?? 0} />
                  <SummaryItem label={t('diff.updated')} value={status?.providers.updated ?? 0} />
                  <SummaryItem
                    label={t('syncStatus.localOnly')}
                    value={status?.providers.local_only ?? 0}
                  />
                </div>
              </div>

              <div className="rounded-xl border bg-muted/10 p-4">
                <div className="flex items-center justify-between">
                  <div>
                    <div className="text-sm font-medium">{t('syncStatus.modelsTitle')}</div>
                    <div className="text-xs text-muted-foreground truncate">
                      {status?.upstream_source
                        ? t('syncStatus.source', { source: status.upstream_source })
                        : '-'}
                    </div>
                  </div>
                  {status?.degraded ? <Badge variant="warning">{t('syncStatus.stateDegraded')}</Badge> : null}
                </div>

                <div className="mt-4 grid grid-cols-3 gap-3 text-sm">
                  <SummaryItem label={t('diff.new')} value={status?.models.new ?? 0} />
                  <SummaryItem label={t('diff.updated')} value={status?.models.updated ?? 0} />
                  <SummaryItem
                    label={t('syncStatus.localOnly')}
                    value={status?.models.local_only ?? 0}
                  />
                </div>
              </div>
            </div>

            <Separator />

            <div className="flex items-start justify-between gap-4">
              <div>
                <div className="text-sm font-medium">{t('syncStatus.providerChangesTitle')}</div>
                <div className="text-sm text-muted-foreground">
                  {t('syncStatus.providerChangesDescription')}
                </div>
              </div>
              <Badge variant="secondary">
                {changedProviders.length} {t('syncStatus.changedProviders')}
              </Badge>
            </div>

            <div className="space-y-3 rounded-xl border p-4">
              {isLoading ? (
                Array.from({ length: 4 }).map((_, index) => (
                  <div key={index} className="rounded-lg border p-4 space-y-3">
                    <div className="h-4 w-32 rounded bg-muted" />
                    <div className="h-3 w-48 rounded bg-muted" />
                  </div>
                ))
              ) : changedProviders.length > 0 ? (
                changedProviders.map(item => {
                  const isLocallyUpdated = locallyUpdatedProviders.has(item.provider);
                  const syncResult = syncResultsByProvider[item.provider];
                  const canSync = item.status === 'new' || item.status === 'updated';
                  const canOpenProvider = item.status !== 'new' || isLocallyUpdated;
                  const isRowSyncing = syncingProviders.has(item.provider);

                  return (
                    <div key={item.provider} className="rounded-xl border bg-background p-4">
                      <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
                        <div className="min-w-0 space-y-2">
                          <div className="flex items-center gap-2">
                            <div className="text-sm font-medium truncate">{item.name}</div>
                            <Badge variant={statusVariant(item.status)}>
                              {t(`syncStatus.status.${item.status}`)}
                            </Badge>
                          </div>
                          <div className="text-xs text-muted-foreground">{item.provider}</div>
                          {item.changed_fields && item.changed_fields.length > 0 ? (
                            <div className="flex flex-wrap gap-2">
                              {item.changed_fields.map(field => (
                                <Badge key={field} variant="outline" className="font-mono">
                                  {field}
                                </Badge>
                              ))}
                            </div>
                          ) : item.status === 'local_only' ? (
                            <div className="text-xs text-muted-foreground">
                              {t('syncStatus.localOnlyDescription')}
                            </div>
                          ) : null}
                          {syncResult ? (
                            <ProviderSyncFeedback result={syncResult} />
                          ) : null}
                        </div>

                        <div className="flex items-center gap-2">
                          {canOpenProvider ? (
                            <Button
                              variant="outline"
                              size="sm"
                              onClick={() => handleOpenProvider(item.provider)}
                            >
                              <ExternalLink className="mr-1 size-4" />
                              {t('syncStatus.openProvider')}
                            </Button>
                          ) : null}
                          {canSync ? (
                            <Button
                              variant={isLocallyUpdated ? 'outline' : 'default'}
                              size="sm"
                              onClick={() => handleSyncProvider(item.provider)}
                              disabled={
                                effectiveBlockedState !== null || isRowSyncing || isLocallyUpdated
                              }
                              className={cn(
                                isLocallyUpdated
                                  ? 'border-success/30 text-success hover:bg-success/5 hover:text-success'
                                  : undefined
                              )}
                            >
                              <RefreshCw
                                className={`mr-1 size-4 ${isRowSyncing ? 'animate-spin' : ''}`}
                              />
                              {isRowSyncing
                                ? t('syncStatus.syncingProvider')
                                : isLocallyUpdated
                                  ? t('syncStatus.updatedAction')
                                  : t('syncStatus.syncProvider')}
                            </Button>
                          ) : null}
                        </div>
                      </div>
                    </div>
                  );
                })
              ) : (
                <div className="flex min-h-52 flex-col items-center justify-center gap-3 rounded-xl border border-dashed bg-muted/10 px-6 text-center">
                  <Badge variant={status?.has_updates ? 'warning' : 'success'}>
                    {status?.has_updates
                      ? t('syncStatus.stateUpdates')
                      : t('syncStatus.stateUpToDate')}
                  </Badge>
                  <div className="text-sm font-medium">{t('syncStatus.noProviderChanges')}</div>
                  {hasModelOnlyUpdates ? (
                    <div className="max-w-xl text-sm text-muted-foreground">
                      {t('syncStatus.modelOnlyHint')}
                    </div>
                  ) : null}
                </div>
              )}
            </div>

            <Separator />

            <div className="flex items-start justify-between gap-4">
              <div>
                <div className="text-sm font-medium">{t('syncStatus.modelChangesTitle')}</div>
                <div className="text-sm text-muted-foreground">
                  {t('syncStatus.modelChangesDescription')}
                </div>
              </div>
              <Badge variant="secondary">
                {modelOnlyProviders.length} {t('syncStatus.changedProviders')}
              </Badge>
            </div>

            <div className="space-y-3 rounded-xl border p-4">
              {isLoading ? (
                Array.from({ length: 4 }).map((_, index) => (
                  <div key={index} className="rounded-lg border p-4 space-y-3">
                    <div className="h-4 w-32 rounded bg-muted" />
                    <div className="h-3 w-48 rounded bg-muted" />
                  </div>
                ))
              ) : modelOnlyProviders.length > 0 ? (
                modelOnlyProviders.map(item => {
                  const isRowSyncing = syncingProviders.has(item.provider);

                  return (
                    <ModelOnlyProviderCard
                      key={item.provider}
                      item={item}
                      feedback={syncResultsByProvider[item.provider]}
                      isUpdated={locallyUpdatedProviders.has(item.provider)}
                      isRowSyncing={isRowSyncing}
                      isBlocked={effectiveBlockedState !== null}
                      onOpenProvider={handleOpenProvider}
                      onSyncProvider={() => handleSyncProviderModels(item.provider)}
                    />
                  );
                })
              ) : (
                <div className="flex min-h-52 flex-col items-center justify-center gap-3 rounded-xl border border-dashed bg-muted/10 px-6 text-center">
                  <Badge variant={status?.has_updates ? 'warning' : 'success'}>
                    {status?.has_updates
                      ? t('syncStatus.stateUpdates')
                      : t('syncStatus.stateUpToDate')}
                  </Badge>
                  <div className="text-sm font-medium">{t('syncStatus.noModelChanges')}</div>
                  <div className="max-w-xl text-sm text-muted-foreground">
                    {t('syncStatus.modelOnlyHint')}
                  </div>
                </div>
              )}
            </div>
        </DialogBody>

        <DialogFooter className="border-t px-6 py-4">
          <Button
            variant="outline"
            onClick={() => {
              closeDialog();
            }}
          >
            {tCommon('close')}
          </Button>
          <Button variant="secondary" onClick={() => void handleRefresh()} disabled={isRefreshing}>
            <RefreshCw className={`mr-1 size-4 ${isRefreshing ? 'animate-spin' : ''}`} />
            {tCommon('refresh')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function SummaryItem({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-lg border bg-background px-3 py-2">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 text-lg font-semibold">{value}</div>
    </div>
  );
}

function statusVariant(status: ModelMetaProviderDiffItem['status']) {
  switch (status) {
    case 'new':
      return 'success';
    case 'updated':
      return 'warning';
    case 'local_only':
      return 'outline';
    default:
      return 'secondary';
  }
}

function ModelOnlyProviderCard({
  item,
  feedback,
  isUpdated,
  isRowSyncing,
  isBlocked,
  onOpenProvider,
  onSyncProvider,
}: {
  item: ModelMetaModelUpdateProviderItem;
  feedback?: ModelMetaSyncResult;
  isUpdated: boolean;
  isRowSyncing: boolean;
  isBlocked: boolean;
  onOpenProvider: (provider: string) => void;
  onSyncProvider: () => void;
}) {
  const t = useT('aiProviders');

  return (
    <div className="rounded-xl border bg-background p-4">
      <div className="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
        <div className="min-w-0 space-y-2">
          <div className="flex items-center gap-2">
            <div className="text-sm font-medium truncate">{item.name}</div>
            <Badge variant="warning">{t('syncStatus.stateUpdates')}</Badge>
          </div>
          <div className="text-xs text-muted-foreground">{item.provider}</div>
          <div className="flex flex-wrap gap-2">
            <Badge variant="outline">
              {t('diff.new')}: {item.new_models}
            </Badge>
            <Badge variant="outline">
              {t('diff.updated')}: {item.updated_models}
            </Badge>
            <Badge variant="outline">
              {t('syncStatus.remoteModels')}: {item.total_remote}
            </Badge>
            <Badge variant="outline">
              {t('syncStatus.localModels')}: {item.total_local}
            </Badge>
          </div>
          {feedback ? <ProviderSyncFeedback result={feedback} /> : null}
        </div>

        <div className="flex items-center gap-2">
          <Button variant="outline" size="sm" onClick={() => onOpenProvider(item.provider)}>
            <ExternalLink className="mr-1 size-4" />
            {t('syncStatus.openProvider')}
          </Button>
          <Button
            variant={isUpdated ? 'outline' : 'default'}
            size="sm"
            onClick={onSyncProvider}
            disabled={isBlocked || isRowSyncing || isUpdated}
            className={cn(
              isUpdated
                ? 'border-success/30 text-success hover:bg-success/5 hover:text-success'
                : undefined
            )}
          >
            <RefreshCw className={`mr-1 size-4 ${isRowSyncing ? 'animate-spin' : ''}`} />
            {isRowSyncing
              ? t('syncStatus.syncingModels')
              : isUpdated
                ? t('syncStatus.updatedAction')
                : t('syncStatus.syncModels')}
          </Button>
        </div>
      </div>
    </div>
  );
}

function ProviderSyncFeedback({ result }: { result: ModelMetaSyncResult }) {
  const t = useT('aiProviders');

  if (result.status === 'success') {
    return null;
  }

  return (
    <div
      className={cn(
        'rounded-lg border px-3 py-2 text-xs',
        result.status === 'partial'
          ? 'border-warning/30 bg-warning/5 text-warning'
          : 'border-destructive/30 bg-destructive/5 text-destructive'
      )}
    >
      <div className="font-medium">
        {result.status === 'partial'
          ? t('syncResult.partialTitle', {
              success: result.success_models,
              failed: result.failed_models,
            })
          : t('syncResult.failedTitle', {
              failed: result.failed_models,
            })}
      </div>
      {result.errors && result.errors.length > 0 ? (
        <div className="mt-1 space-y-1 text-muted-foreground">
          {result.errors.slice(0, 2).map(error => (
            <div key={error}>{error}</div>
          ))}
        </div>
      ) : null}
    </div>
  );
}
