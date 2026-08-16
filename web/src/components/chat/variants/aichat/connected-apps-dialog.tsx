'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import Link from 'next/link';
import { AlertTriangle, Info, PlugZap, Star, UserRound } from 'lucide-react';
import { toast } from 'sonner';
import { IntegrationConnectionHealthBadge } from '@/components/integrations/health-badge';
import { IntegrationProviderIcon } from '@/components/integrations/provider-icon';
import {
  safeIntegrationDisplayText,
  safeOptionalIntegrationDisplayText,
} from '@/components/integrations/display-utils';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { SearchInput } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { AICHAT_KEYS, INTEGRATION_KEYS } from '@/hooks/query-keys';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import { integrationService } from '@/services/integration.service';
import type {
  AIChatIntegrationPreference,
  IntegrationCatalogItem,
  IntegrationConnection,
  IntegrationLocalizedText,
  ReplaceAIChatIntegrationPreferencesRequest,
} from '@/services/types/integration';
import { getErrorMessage } from '@/utils/error-notifications';
import {
  getAIChatExternalAppDescription,
  getAIChatExternalAppDisplayName,
} from '@/components/chat/variants/aichat/external-app-display';
import { actionSupportsAuthMethod } from '@/components/integrations/action-auth-compatibility';
import { localizeAIChatRuntimeMessage } from '@/components/chat/variants/aichat/timeline-display-i18n';

export interface AIChatConnectedAppsSummary {
  selectedConnectionCount: number;
  hasAttentionRequired: boolean;
}

interface AIChatConnectedAppsDialogProps {
  open: boolean;
  enabled: boolean;
  scopeKey: string;
  onOpenChange: (open: boolean) => void;
  onSummaryChange?: (summary: AIChatConnectedAppsSummary) => void;
}

interface DraftProviderSelection {
  selectedConnectionIds: string[];
  preferredConnectionId: string;
}

type DraftSelections = Record<string, DraftProviderSelection>;

interface ProviderGroup {
  id: string;
  driverId: string;
  name: string;
  nameI18n?: IntegrationLocalizedText;
  description: string;
  descriptionI18n?: IntegrationLocalizedText;
  connections: IntegrationConnection[];
}

function normalizeIdentifier(value: string | null | undefined): string {
  return value?.trim().toLowerCase() ?? '';
}

function catalogIntegrationId(item: IntegrationCatalogItem): string {
  return normalizeIdentifier(item.integration_id || item.id);
}

function preferenceItems(
  value: { items?: AIChatIntegrationPreference[] } | null | undefined
): AIChatIntegrationPreference[] {
  return value?.items ?? [];
}

function connectionItems(
  value: { items?: IntegrationConnection[]; data?: IntegrationConnection[] } | null | undefined
): IntegrationConnection[] {
  return value?.items ?? value?.data ?? [];
}

function catalogItems(
  value: { items?: IntegrationCatalogItem[]; data?: IntegrationCatalogItem[] } | null | undefined
): IntegrationCatalogItem[] {
  return value?.items ?? value?.data ?? [];
}

function preferencesToDraft(preferences: AIChatIntegrationPreference[]): DraftSelections {
  return preferences.reduce<DraftSelections>((result, preference) => {
    const integrationId = normalizeIdentifier(preference.integration_id);
    const selectedConnectionIds = Array.from(
      new Set(preference.selected_connection_ids.map(id => id.trim()).filter(Boolean))
    ).sort();
    const preferredConnectionId = preference.preferred_connection_id?.trim() ?? '';
    if (
      integrationId &&
      selectedConnectionIds.length > 0 &&
      selectedConnectionIds.includes(preferredConnectionId)
    ) {
      result[integrationId] = { selectedConnectionIds, preferredConnectionId };
    }
    return result;
  }, {});
}

function normalizeDraft(draft: DraftSelections): DraftSelections {
  return Object.entries(draft)
    .sort(([left], [right]) => left.localeCompare(right))
    .reduce<DraftSelections>((result, [integrationId, selection]) => {
      const selectedConnectionIds = Array.from(
        new Set(selection.selectedConnectionIds.map(id => id.trim()).filter(Boolean))
      ).sort();
      if (selectedConnectionIds.length === 0) return result;
      result[integrationId] = {
        selectedConnectionIds,
        preferredConnectionId: selectedConnectionIds.includes(selection.preferredConnectionId)
          ? selection.preferredConnectionId
          : selectedConnectionIds[0],
      };
      return result;
    }, {});
}

function draftKey(draft: DraftSelections): string {
  return JSON.stringify(normalizeDraft(draft));
}

function isExpired(value: string | null | undefined): boolean {
  if (!value) return false;
  const timestamp = Date.parse(value);
  return Number.isFinite(timestamp) && timestamp <= Date.now();
}

function isConnectionSelectable(connection: IntegrationConnection): boolean {
  if (connection.status !== 'active') return false;
  if (connection.auth_status === 'reconnect_required' || connection.auth_status === 'expired') {
    return false;
  }
  return !isExpired(connection.expires_at) && !isExpired(connection.refresh_token_expires_at);
}

function connectionNeedsAttention(connection: IntegrationConnection): boolean {
  return (
    !isConnectionSelectable(connection) ||
    connection.health_status === 'degraded' ||
    connection.health_status === 'unhealthy' ||
    connection.scope_status === 'drifted'
  );
}

function selectedConnectionCount(draft: DraftSelections): number {
  return Object.values(draft).reduce(
    (total, selection) => total + selection.selectedConnectionIds.length,
    0
  );
}

function buildProviderGroups(
  catalog: IntegrationCatalogItem[],
  connections: IntegrationConnection[],
  preferences: AIChatIntegrationPreference[]
): ProviderGroup[] {
  const catalogById = new Map(
    catalog
      .map(item => [catalogIntegrationId(item), item] as const)
      .filter(([integrationId]) => Boolean(integrationId))
  );
  const connectionGroups = new Map<string, IntegrationConnection[]>();
  for (const connection of connections) {
    const integrationId = normalizeIdentifier(connection.integration_id);
    if (!integrationId) continue;
    const existing = connectionGroups.get(integrationId) ?? [];
    existing.push(connection);
    connectionGroups.set(integrationId, existing);
  }

  const ids = new Set<string>();
  for (const item of catalog) {
    const integrationId = catalogIntegrationId(item);
    if (integrationId && item.enabled) ids.add(integrationId);
  }
  for (const integrationId of connectionGroups.keys()) ids.add(integrationId);
  for (const preference of preferences) {
    const integrationId = normalizeIdentifier(preference.integration_id);
    if (integrationId) ids.add(integrationId);
  }

  return Array.from(ids)
    .map<ProviderGroup>(integrationId => {
      const provider = catalogById.get(integrationId);
      const compatibleConnections = (connectionGroups.get(integrationId) ?? []).filter(
        connection =>
          !provider ||
          provider.actions.some(action =>
            actionSupportsAuthMethod(action, connection.auth_method_id)
          )
      );
      return {
        id: integrationId,
        driverId: provider?.driver_id ?? compatibleConnections[0]?.driver_id ?? '',
        name: provider?.name || integrationId,
        nameI18n: provider?.name_i18n,
        description: provider?.description ?? '',
        descriptionI18n: provider?.description_i18n,
        connections: compatibleConnections.sort((left, right) => {
          const availability =
            Number(isConnectionSelectable(right)) - Number(isConnectionSelectable(left));
          return availability || left.name.localeCompare(right.name);
        }),
      };
    })
    .sort((left, right) => {
      const availability =
        Number(right.connections.length > 0) - Number(left.connections.length > 0);
      return availability || left.name.localeCompare(right.name);
    });
}

function toRequest(draft: DraftSelections): ReplaceAIChatIntegrationPreferencesRequest {
  return {
    items: Object.entries(normalizeDraft(draft)).map(([integrationId, selection]) => ({
      integration_id: integrationId,
      selected_connection_ids: selection.selectedConnectionIds,
      preferred_connection_id: selection.preferredConnectionId,
    })),
  };
}

export function AIChatConnectedAppsDialog({
  open,
  enabled,
  scopeKey,
  onOpenChange,
  onSummaryChange,
}: AIChatConnectedAppsDialogProps) {
  const t = useT('webapp');
  const { locale } = useLocale();
  const queryClient = useQueryClient();
  const initializedForOpenRef = useRef(false);
  const [draft, setDraft] = useState<DraftSelections>({});
  const [searchQuery, setSearchQuery] = useState('');
  const [closeConfirmOpen, setCloseConfirmOpen] = useState(false);

  const catalogQuery = useQuery({
    queryKey: INTEGRATION_KEYS.catalog(),
    queryFn: () => integrationService.getCatalog(),
    enabled,
    staleTime: 60_000,
    retry: false,
  });
  const connectionsQuery = useQuery({
    queryKey: INTEGRATION_KEYS.availableConnections(scopeKey),
    queryFn: () => integrationService.getAllAvailableConnections(),
    enabled,
    staleTime: 15_000,
    retry: false,
  });
  const preferencesQuery = useQuery({
    queryKey: AICHAT_KEYS.integrationPreferences(scopeKey),
    queryFn: () => integrationService.getAIChatPreferences(),
    enabled,
    staleTime: 15_000,
    retry: false,
  });

  const catalog = useMemo(() => catalogItems(catalogQuery.data?.data), [catalogQuery.data?.data]);
  const connections = useMemo(
    () => connectionItems(connectionsQuery.data?.data),
    [connectionsQuery.data?.data]
  );
  const preferences = useMemo(
    () => preferenceItems(preferencesQuery.data?.data),
    [preferencesQuery.data?.data]
  );
  const savedDraft = useMemo(() => preferencesToDraft(preferences), [preferences]);
  const groups = useMemo(
    () => buildProviderGroups(catalog, connections, preferences),
    [catalog, connections, preferences]
  );
  const visibleGroups = useMemo(() => {
    const query = searchQuery.trim().toLowerCase();
    if (!query) return groups;
    return groups.filter(group =>
      [
        group.id,
        group.name,
        ...Object.values(group.nameI18n ?? {}),
        group.description,
        ...Object.values(group.descriptionI18n ?? {}),
        ...group.connections.flatMap(connection => [
          connection.name,
          connection.display_name ?? '',
        ]),
      ]
        .join(' ')
        .toLowerCase()
        .includes(query)
    );
  }, [groups, searchQuery]);
  const isLoading =
    catalogQuery.isLoading || connectionsQuery.isLoading || preferencesQuery.isLoading;
  const hasLoadError = catalogQuery.isError || connectionsQuery.isError || preferencesQuery.isError;
  const hasChanges = !isLoading && !hasLoadError && draftKey(draft) !== draftKey(savedDraft);

  const connectionById = useMemo(
    () => new Map(connections.map(connection => [connection.id, connection])),
    [connections]
  );
  const savedSummary = useMemo<AIChatConnectedAppsSummary>(() => {
    const selectedIds = Object.values(savedDraft).flatMap(item => item.selectedConnectionIds);
    return {
      selectedConnectionCount: selectedIds.length,
      hasAttentionRequired: selectedIds.some(id => {
        const connection = connectionById.get(id);
        return !connection || connectionNeedsAttention(connection);
      }),
    };
  }, [connectionById, savedDraft]);

  useEffect(() => {
    onSummaryChange?.(savedSummary);
  }, [onSummaryChange, savedSummary]);

  useEffect(() => {
    if (!open) {
      initializedForOpenRef.current = false;
      setCloseConfirmOpen(false);
      return;
    }
    if (!isLoading && !hasLoadError && !initializedForOpenRef.current) {
      // Preferences are a replace-style resource. Never infer deletion from a
      // connection missing in a separately loaded catalog snapshot; preserving
      // the server-sanitized preference is the fail-closed behavior under races.
      setDraft(savedDraft);
      initializedForOpenRef.current = true;
    }
  }, [hasLoadError, isLoading, open, savedDraft]);

  const saveMutation = useMutation({
    mutationFn: (payload: ReplaceAIChatIntegrationPreferencesRequest) =>
      integrationService.replaceAIChatPreferences(payload),
    onSuccess: response => {
      queryClient.setQueryData(AICHAT_KEYS.integrationPreferences(scopeKey), response);
      void queryClient.invalidateQueries({
        queryKey: [...AICHAT_KEYS.all, 'integration-preferences'],
      });
      toast.success(t('consoleChat.connectedApps.saved'));
      initializedForOpenRef.current = false;
      setCloseConfirmOpen(false);
      onOpenChange(false);
    },
    onError: error => {
      toast.error(
        localizeAIChatRuntimeMessage(
          getErrorMessage(error),
          t,
          t('consoleChat.connectedApps.saveFailed')
        ) ?? t('consoleChat.connectedApps.saveFailed')
      );
    },
  });

  useEffect(() => {
    initializedForOpenRef.current = false;
    setDraft({});
    setCloseConfirmOpen(false);
  }, [scopeKey]);

  const toggleConnection = (integrationId: string, connectionId: string, checked: boolean) => {
    setDraft(current => {
      const existing = current[integrationId] ?? {
        selectedConnectionIds: [],
        preferredConnectionId: '',
      };
      const selectedConnectionIds = checked
        ? Array.from(new Set([...existing.selectedConnectionIds, connectionId])).sort()
        : existing.selectedConnectionIds.filter(id => id !== connectionId);
      if (selectedConnectionIds.length === 0) {
        const next = { ...current };
        delete next[integrationId];
        return next;
      }
      return {
        ...current,
        [integrationId]: {
          selectedConnectionIds,
          preferredConnectionId: selectedConnectionIds.includes(existing.preferredConnectionId)
            ? existing.preferredConnectionId
            : selectedConnectionIds[0],
        },
      };
    });
  };

  const setPreferredConnection = (integrationId: string, connectionId: string) => {
    setDraft(current => {
      const selection = current[integrationId];
      if (!selection?.selectedConnectionIds.includes(connectionId)) return current;
      return {
        ...current,
        [integrationId]: { ...selection, preferredConnectionId: connectionId },
      };
    });
  };

  const requestClose = () => {
    if (saveMutation.isPending) return;
    if (hasChanges) {
      setCloseConfirmOpen(true);
      return;
    }
    onOpenChange(false);
  };

  const missingSelectionCount = useMemo(
    () =>
      Object.values(savedDraft)
        .flatMap(selection => selection.selectedConnectionIds)
        .filter(id => !connectionById.has(id)).length,
    [connectionById, savedDraft]
  );

  return (
    <>
      <Dialog
        open={open}
        onOpenChange={nextOpen => {
          if (nextOpen) onOpenChange(true);
          else requestClose();
        }}
      >
        <DialogContent size="xl">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <PlugZap className="size-5 text-primary" />
              {t('consoleChat.connectedApps.title')}
            </DialogTitle>
            <DialogDescription>{t('consoleChat.connectedApps.description')}</DialogDescription>
          </DialogHeader>
          <DialogBody className="max-h-[min(680px,calc(100vh-13rem))] space-y-4">
            <div className="flex gap-2 rounded-md border border-primary/20 bg-primary/5 p-3 text-sm text-muted-foreground">
              <Info className="mt-0.5 size-4 shrink-0 text-primary" />
              <span>{t('consoleChat.connectedApps.statusExplanation')}</span>
            </div>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <SearchInput
                value={searchQuery}
                onChange={event => setSearchQuery(event.target.value)}
                placeholder={t('consoleChat.connectedApps.searchPlaceholder')}
                className="h-9 rounded-md bg-background sm:max-w-sm"
                disabled={saveMutation.isPending}
              />
              <Button asChild type="button" variant="outline" size="sm">
                <Link href="/console/integrations?view=connected">
                  <UserRound className="size-4" />
                  {t('consoleChat.connectedApps.managePersonal')}
                </Link>
              </Button>
            </div>

            {missingSelectionCount > 0 ? (
              <div className="flex gap-2 rounded-md border border-warning/30 bg-warning/10 p-3 text-sm text-warning">
                <AlertTriangle className="mt-0.5 size-4 shrink-0" />
                <span>
                  {t('consoleChat.connectedApps.missingSelections', {
                    count: missingSelectionCount,
                  })}
                </span>
              </div>
            ) : null}

            {isLoading ? (
              <div className="space-y-3">
                {Array.from({ length: 3 }).map((_, index) => (
                  <Skeleton key={index} className="h-40 rounded-md" />
                ))}
              </div>
            ) : hasLoadError ? (
              <div className="rounded-md border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive">
                <p>{t('consoleChat.connectedApps.loadFailed')}</p>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="mt-3"
                  onClick={() => {
                    void catalogQuery.refetch();
                    void connectionsQuery.refetch();
                    void preferencesQuery.refetch();
                  }}
                >
                  {t('consoleChat.connectedApps.retry')}
                </Button>
              </div>
            ) : groups.length === 0 ? (
              <div className="rounded-md border border-dashed p-6 text-sm text-muted-foreground">
                {t('consoleChat.connectedApps.empty')}
              </div>
            ) : visibleGroups.length === 0 ? (
              <div className="rounded-md border border-dashed p-6 text-sm text-muted-foreground">
                {t('consoleChat.connectedApps.noResults')}
              </div>
            ) : (
              <div className="space-y-3">
                {visibleGroups.map(group => {
                  const selection = draft[group.id];
                  const groupName = safeIntegrationDisplayText(
                    getAIChatExternalAppDisplayName(group.id, group.name, t, {
                      locale,
                      nameI18n: group.nameI18n,
                    }),
                    t('consoleChat.connectedApps.unknownExternalApp')
                  );
                  const groupDescription = safeOptionalIntegrationDisplayText(
                    getAIChatExternalAppDescription(group.id, group.description, locale, t, {
                      descriptionI18n: group.descriptionI18n,
                    })
                  );
                  return (
                    <section key={group.id} className="rounded-lg border bg-card p-4 shadow-sm">
                      <div className="flex items-start gap-3">
                        <span className="flex size-9 shrink-0 items-center justify-center rounded-md border bg-background text-muted-foreground">
                          <IntegrationProviderIcon
                            integrationId={group.id}
                            driverId={group.driverId}
                            className="size-4"
                          />
                        </span>
                        <div className="min-w-0 flex-1">
                          <div className="flex flex-wrap items-center justify-between gap-2">
                            <h3 className="truncate text-sm font-semibold">{groupName}</h3>
                            <Badge variant={selection ? 'success' : 'subtle'}>
                              {selection
                                ? t('consoleChat.connectedApps.selectedCount', {
                                    count: selection.selectedConnectionIds.length,
                                  })
                                : t('consoleChat.connectedApps.notSelected')}
                            </Badge>
                          </div>
                          {groupDescription ? (
                            <p className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
                              {groupDescription}
                            </p>
                          ) : null}
                        </div>
                      </div>

                      {group.connections.length === 0 ? (
                        <div className="mt-3 flex flex-wrap items-center justify-between gap-2 rounded-md border border-dashed bg-muted/20 p-3 text-xs text-muted-foreground">
                          <span>{t('consoleChat.connectedApps.noAuthorizedConnections')}</span>
                          <Button asChild type="button" variant="link" size="xs">
                            <Link
                              href={`/console/integrations?view=available&integration_id=${encodeURIComponent(group.id)}`}
                            >
                              {t('consoleChat.connectedApps.addPersonal')}
                            </Link>
                          </Button>
                        </div>
                      ) : (
                        <div className="mt-3 space-y-2">
                          {group.connections.map(connection => {
                            const selectable = isConnectionSelectable(connection);
                            const selected =
                              selection?.selectedConnectionIds.includes(connection.id) ?? false;
                            const preferred =
                              selected && selection?.preferredConnectionId === connection.id;
                            const connectionName = safeIntegrationDisplayText(
                              connection.name,
                              t('consoleChat.connectedApps.unnamedConnection')
                            );
                            const connectionDisplayName =
                              safeOptionalIntegrationDisplayText(connection.display_name) ||
                              t(
                                `consoleChat.connectedApps.credentialSource.${connection.credential_source}`
                              );
                            return (
                              <div
                                key={connection.id}
                                className={cn(
                                  'flex items-start gap-3 rounded-md border px-3 py-2.5 transition-colors',
                                  selected ? 'border-primary/40 bg-primary/5' : 'bg-background',
                                  !selectable && 'bg-muted/30 opacity-75'
                                )}
                              >
                                <Checkbox
                                  checked={selected}
                                  disabled={(!selectable && !selected) || saveMutation.isPending}
                                  onCheckedChange={checked =>
                                    toggleConnection(group.id, connection.id, checked === true)
                                  }
                                  aria-label={t('consoleChat.connectedApps.selectConnection', {
                                    name: connectionName,
                                  })}
                                  className="mt-0.5"
                                />
                                <div className="min-w-0 flex-1">
                                  <div className="flex flex-wrap items-center gap-2">
                                    <span className="truncate text-sm font-medium">
                                      {connectionName}
                                    </span>
                                    <IntegrationConnectionHealthBadge connection={connection} />
                                    {connection.auth_status === 'reconnect_required' ? (
                                      <Badge variant="destructive">
                                        {t('consoleChat.connectedApps.reconnectRequired')}
                                      </Badge>
                                    ) : null}
                                    {connection.auth_status === 'expired' ||
                                    isExpired(connection.expires_at) ||
                                    isExpired(connection.refresh_token_expires_at) ? (
                                      <Badge variant="destructive">
                                        {t('consoleChat.connectedApps.expired')}
                                      </Badge>
                                    ) : null}
                                    {connection.scope_status === 'drifted' ? (
                                      <Badge variant="warning">
                                        {t('consoleChat.connectedApps.scopeChanged')}
                                      </Badge>
                                    ) : null}
                                  </div>
                                  <p className="mt-1 text-xs text-muted-foreground">
                                    {connectionDisplayName}
                                  </p>
                                </div>
                                <Button
                                  type="button"
                                  variant="ghost"
                                  size="sm"
                                  className={cn(
                                    'h-8 shrink-0 gap-1 px-2 text-xs',
                                    preferred && 'text-primary'
                                  )}
                                  disabled={!selected || !selectable || saveMutation.isPending}
                                  onClick={() => setPreferredConnection(group.id, connection.id)}
                                  aria-pressed={preferred}
                                  aria-label={t(
                                    preferred
                                      ? 'consoleChat.connectedApps.preferredConnectionLabel'
                                      : 'consoleChat.connectedApps.makePreferredConnectionLabel',
                                    { name: connectionName }
                                  )}
                                  title={t('consoleChat.connectedApps.preferredDescription')}
                                >
                                  <Star className={cn('size-3.5', preferred && 'fill-current')} />
                                  <span className="hidden sm:inline">
                                    {preferred
                                      ? t('consoleChat.connectedApps.preferred')
                                      : t('consoleChat.connectedApps.makePreferred')}
                                  </span>
                                </Button>
                              </div>
                            );
                          })}
                        </div>
                      )}
                    </section>
                  );
                })}
              </div>
            )}
          </DialogBody>
          <DialogFooter className="items-center justify-between gap-3">
            <span className="mr-auto text-xs text-muted-foreground">
              {t('consoleChat.connectedApps.totalSelected', {
                count: selectedConnectionCount(draft),
              })}
            </span>
            <Button variant="outline" onClick={requestClose} disabled={saveMutation.isPending}>
              {t('consoleChat.connectedApps.cancel')}
            </Button>
            <Button
              onClick={() => saveMutation.mutate(toRequest(draft))}
              disabled={saveMutation.isPending || isLoading || hasLoadError || !hasChanges}
            >
              {saveMutation.isPending
                ? t('consoleChat.connectedApps.saving')
                : t('consoleChat.connectedApps.save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={closeConfirmOpen} onOpenChange={setCloseConfirmOpen}>
        <DialogContent size="sm">
          <DialogHeader>
            <DialogTitle>{t('consoleChat.connectedApps.closeConfirm.title')}</DialogTitle>
            <DialogDescription>
              {t('consoleChat.connectedApps.closeConfirm.description')}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setCloseConfirmOpen(false);
                onOpenChange(false);
              }}
            >
              {t('consoleChat.connectedApps.closeConfirm.discard')}
            </Button>
            <Button variant="ghost" onClick={() => setCloseConfirmOpen(false)}>
              {t('consoleChat.connectedApps.closeConfirm.keepEditing')}
            </Button>
            <Button onClick={() => saveMutation.mutate(toRequest(draft))}>
              {t('consoleChat.connectedApps.closeConfirm.saveAndClose')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
