'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import { useQueries, useQuery } from '@tanstack/react-query';
import Link from 'next/link';
import { toast } from 'sonner';
import {
  Bot,
  CheckCircle2,
  ChevronDown,
  Ellipsis,
  Eye,
  KeyRound,
  Pencil,
  Play,
  Plus,
  Power,
  RefreshCw,
  ShieldCheck,
  Trash2,
  Unlink,
  UserRound,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Skeleton } from '@/components/ui/skeleton';
import {
  integrationCatalogItems,
  integrationConnectionItems,
  useAllIntegrationConnections,
  useAllMyIntegrationConnections,
  useCreateIntegrationConnection,
  useCreateMyIntegrationConnection,
  useDeleteIntegrationConnection,
  useDeleteMyIntegrationConnection,
  useIntegrationCatalog,
  useIntegrationOAuthFlow,
  useTestIntegrationConnection,
  useTestMyIntegrationConnection,
  useUpdateIntegrationConnection,
  useUpdateMyIntegrationConnection,
} from '@/hooks';
import { AICHAT_KEYS, INTEGRATION_KEYS } from '@/hooks/query-keys';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { integrationService } from '@/services/integration.service';
import type {
  IntegrationCatalogItem,
  IntegrationConnection,
  IntegrationConnectionDeleteImpact,
  IntegrationConnectionGrant,
  StartIntegrationOAuthFlowRequest,
} from '@/services/types/integration';
import { actionIDsForAuthMethod, actionSupportsAuthMethod } from './action-auth-compatibility';
import { IntegrationConnectionDetailDialog } from './connection-detail-dialog';
import { IntegrationConnectionDialog } from './connection-dialog';
import { safeIntegrationDisplayText, safeOptionalIntegrationDisplayText } from './display-utils';
import { IntegrationConnectionHealthBadge } from './health-badge';
import {
  integrationAuthCredentialSource,
  integrationCatalogID,
  resolveConnectionHealthState,
  resolveIntegrationAuthDefinitions,
} from './integration-utils';
import { useIntegrationMetadata } from './metadata-i18n';
import { IntegrationOAuthFlowDialog } from './oauth-flow-dialog';
import { resolveOAuthScopeUpgradeActionIDs } from './oauth-scope-upgrade';
import { IntegrationProviderCapabilitiesInline } from './provider-capabilities-inline';
import { IntegrationProviderIcon } from './provider-icon';
import {
  IntegrationProviderOAuthClientConfigsInline,
  resolveProviderOAuthClientConfigGroups,
} from './provider-oauth-client-configs-inline';

const ORGANIZATION_CREDENTIAL_SOURCES = ['organization'] as const;

function isOAuthConnection(connection: IntegrationConnection | null): boolean {
  return Boolean(connection && ['oauth', 'oauth2'].includes(connection.auth_type));
}

interface IntegrationConnectionsPanelProps {
  integrationId?: string;
  query?: string;
  category?: string;
  canManageShared?: boolean;
  onBrowseAvailable?: () => void;
}

interface ProviderConnectionGroup {
  provider: IntegrationCatalogItem;
  integrationId: string;
  connections: IntegrationConnection[];
}

type ConnectionUsageRuleState =
  | 'personal'
  | 'configured'
  | 'not_configured'
  | 'available'
  | 'loading'
  | 'error';

type ProviderDetailView = 'connections' | 'capabilities' | 'oauth_config';

interface ConnectionUsageRuleSummary {
  state: ConnectionUsageRuleState;
  ruleCount: number;
  actionCount: number;
  issueCount?: number;
}

function summarizeUsageRules(
  rules: IntegrationConnectionGrant[],
  validActionIDs: Set<string>
): ConnectionUsageRuleSummary {
  if (rules.length === 0) {
    return { state: 'not_configured', ruleCount: 0, actionCount: 0 };
  }

  const validRules = rules.filter(
    rule =>
      rule.principal_state !== 'missing' &&
      rule.allowed_action_ids.some(actionID => actionID !== '*' && validActionIDs.has(actionID))
  );
  const actions = new Set(
    validRules
      .flatMap(rule => rule.allowed_action_ids)
      .filter(actionID => actionID !== '*' && validActionIDs.has(actionID))
  );
  const issueCount = rules.length - validRules.length;

  if (validRules.length === 0 || actions.size === 0) {
    return {
      state: 'not_configured',
      ruleCount: 0,
      actionCount: 0,
      issueCount: rules.length,
    };
  }

  return {
    state: 'configured',
    ruleCount: validRules.length,
    actionCount: actions.size,
    issueCount,
  };
}

export function IntegrationConnectionsPanel({
  integrationId,
  query = '',
  category = 'all',
  canManageShared = false,
  onBrowseAvailable,
}: IntegrationConnectionsPanelProps = {}) {
  const t = useT('integrations');
  const metadata = useIntegrationMetadata();
  const catalogQuery = useIntegrationCatalog(true, canManageShared ? 'organization' : 'account');
  const managedConnectionsQuery = useAllIntegrationConnections(undefined, canManageShared);
  const availableConnectionsQuery = useQuery({
    queryKey: INTEGRATION_KEYS.availableConnections({ all: true }),
    queryFn: () => integrationService.getAllAvailableConnections(),
    enabled: true,
    staleTime: 30_000,
    retry: false,
  });
  const preferencesQuery = useQuery({
    queryKey: AICHAT_KEYS.integrationPreferences(),
    queryFn: () => integrationService.getAIChatPreferences(),
    staleTime: 15_000,
    retry: false,
  });
  const myConnectionsQuery = useAllMyIntegrationConnections(undefined, canManageShared);
  const createMutation = useCreateIntegrationConnection();
  const createMyMutation = useCreateMyIntegrationConnection();
  const updateMutation = useUpdateIntegrationConnection();
  const updateMyMutation = useUpdateMyIntegrationConnection();
  const testMutation = useTestIntegrationConnection();
  const testMyMutation = useTestMyIntegrationConnection();
  const deleteMutation = useDeleteIntegrationConnection();
  const deleteMyMutation = useDeleteMyIntegrationConnection();
  const [createProvider, setCreateProvider] = useState<IntegrationCatalogItem | null>(null);
  const [connectionDialogMode, setConnectionDialogMode] = useState<'personal' | 'shared' | null>(
    null
  );
  const [editingConnection, setEditingConnection] = useState<IntegrationConnection | null>(null);
  const [detailConnection, setDetailConnection] = useState<IntegrationConnection | null>(null);
  const [testConnection, setTestConnection] = useState<IntegrationConnection | null>(null);
  const [deleteConnection, setDeleteConnection] = useState<IntegrationConnection | null>(null);
  const [deleteImpact, setDeleteImpact] = useState<IntegrationConnectionDeleteImpact | null>(null);
  const [lastOAuthRequest, setLastOAuthRequest] = useState<StartIntegrationOAuthFlowRequest | null>(
    null
  );
  const [oauthProviderName, setOAuthProviderName] = useState('');
  const [oauthConnectionName, setOAuthConnectionName] = useState('');
  const [isImpactLoading, setIsImpactLoading] = useState(false);
  const [expandedProviders, setExpandedProviders] = useState<Set<string>>(new Set());
  const [providerDetailViews, setProviderDetailViews] = useState<
    Record<string, ProviderDetailView>
  >({});
  const initializedExpandedProvider = useRef(false);
  const oauthFlow = useIntegrationOAuthFlow();

  const catalog = integrationCatalogItems(catalogQuery.data?.data);
  const catalogProviders = useMemo(
    () => new Map(catalog.map(item => [integrationCatalogID(item), item] as const)),
    [catalog]
  );
  const connections = useMemo(() => {
    const source = canManageShared
      ? [
          ...integrationConnectionItems(managedConnectionsQuery.data?.data),
          ...integrationConnectionItems(myConnectionsQuery.data?.data),
        ]
      : integrationConnectionItems(availableConnectionsQuery.data?.data);
    return [
      ...new Map(
        source
          .filter(connection => connection.credential_source !== 'platform')
          .map(connection => [connection.id, connection])
      ).values(),
    ];
  }, [
    availableConnectionsQuery.data?.data,
    canManageShared,
    managedConnectionsQuery.data?.data,
    myConnectionsQuery.data?.data,
  ]);
  const availableConnectionIDs = useMemo(
    () =>
      new Set(
        integrationConnectionItems(availableConnectionsQuery.data?.data)
          .filter(connection => connection.credential_source !== 'platform')
          .map(connection => connection.id)
      ),
    [availableConnectionsQuery.data?.data]
  );
  const selectedConnectionIDs = useMemo(
    () =>
      new Set(
        (preferencesQuery.data?.data.items ?? []).flatMap(
          preference => preference.selected_connection_ids ?? []
        )
      ),
    [preferencesQuery.data?.data.items]
  );
  const sharedConnections = useMemo(
    () =>
      canManageShared
        ? connections.filter(connection => connection.credential_source === 'organization')
        : [],
    [canManageShared, connections]
  );
  const sharedUsageRuleQueries = useQueries({
    queries: sharedConnections.map(connection => ({
      queryKey: INTEGRATION_KEYS.grants(connection.id),
      queryFn: () => integrationService.getConnectionGrants(connection.id),
      staleTime: 15_000,
      retry: false,
    })),
  });
  const sharedUsageRuleSummaries = useMemo(() => {
    const summaries = new Map<string, ConnectionUsageRuleSummary>();
    sharedConnections.forEach((connection, index) => {
      const query = sharedUsageRuleQueries[index];
      if (query.isLoading) {
        summaries.set(connection.id, { state: 'loading', ruleCount: 0, actionCount: 0 });
        return;
      }
      if (query.isError) {
        summaries.set(connection.id, { state: 'error', ruleCount: 0, actionCount: 0 });
        return;
      }
      const rules = query.data?.data.items ?? [];
      const provider = catalogProviders.get(connection.integration_id);
      const validActionIDs = new Set(
        actionIDsForAuthMethod(provider?.actions ?? [], connection.auth_method_id)
      );
      summaries.set(connection.id, summarizeUsageRules(rules, validActionIDs));
    });
    return summaries;
  }, [catalogProviders, sharedConnections, sharedUsageRuleQueries]);
  const normalizedQuery = query.trim().toLocaleLowerCase(metadata.locale);

  const groups = useMemo<ProviderConnectionGroup[]>(() => {
    const grouped = new Map<string, IntegrationConnection[]>();
    for (const connection of connections) {
      const provider = catalogProviders.get(connection.integration_id);
      if (!provider) continue;
      const categories = provider.categories ?? (provider.category ? [provider.category] : []);
      if (integrationId && connection.integration_id !== integrationId) continue;
      if (category !== 'all' && !categories.includes(category)) continue;
      if (
        normalizedQuery &&
        ![
          metadata.providerName(provider),
          metadata.providerDescription(provider),
          connection.name,
          connection.display_name,
          ...categories.map(value => metadata.category(value, provider)),
        ]
          .filter(Boolean)
          .join(' ')
          .toLocaleLowerCase(metadata.locale)
          .includes(normalizedQuery)
      ) {
        continue;
      }
      const current = grouped.get(connection.integration_id) ?? [];
      current.push(connection);
      grouped.set(connection.integration_id, current);
    }

    return [...grouped.entries()]
      .map(([id, items]) => ({
        integrationId: id,
        provider: catalogProviders.get(id) as IntegrationCatalogItem,
        connections: [...items].sort((left, right) =>
          left.name.localeCompare(right.name, metadata.locale)
        ),
      }))
      .sort((left, right) =>
        metadata
          .providerName(left.provider)
          .localeCompare(metadata.providerName(right.provider), metadata.locale)
      );
  }, [catalogProviders, category, connections, integrationId, metadata, normalizedQuery]);

  useEffect(() => {
    if (initializedExpandedProvider.current || groups.length === 0) return;
    initializedExpandedProvider.current = true;
    setExpandedProviders(new Set([groups[0].integrationId]));
  }, [groups]);

  useEffect(() => {
    if (!detailConnection) return;
    const refreshed = connections.find(connection => connection.id === detailConnection.id);
    if (refreshed && refreshed !== detailConnection) {
      setDetailConnection(refreshed);
    }
  }, [connections, detailConnection]);

  const connectionMutationPending =
    createMutation.isPending ||
    createMyMutation.isPending ||
    updateMutation.isPending ||
    updateMyMutation.isPending;
  const testMutationPending = testMutation.isPending || testMyMutation.isPending;
  const deleteMutationPending = deleteMutation.isPending || deleteMyMutation.isPending;

  const canManageConnection = (connection: IntegrationConnection) =>
    connection.credential_source === 'account' || canManageShared;

  const openEdit = (connection: IntegrationConnection) => {
    setEditingConnection(connection);
    setCreateProvider(catalogProviders.get(connection.integration_id) ?? null);
    setConnectionDialogMode(connection.credential_source === 'account' ? 'personal' : 'shared');
  };

  const reconnectOAuth = (connection: IntegrationConnection) => {
    const provider = catalogProviders.get(connection.integration_id);
    const requestedActionIDs = resolveOAuthScopeUpgradeActionIDs(connection, provider);
    if (requestedActionIDs === null) {
      toast.error(t('oauth.flow.scopeUpgradeUnavailable'));
      return;
    }
    const request: StartIntegrationOAuthFlowRequest = {
      integration_id: connection.integration_id,
      auth_method_id: connection.auth_method_id,
      credential_source:
        connection.credential_source === 'organization' ? 'organization' : 'account',
      intent: connection.attention_code === 'scope_update_required' ? 'scope_upgrade' : 'reconnect',
      requested_action_ids: requestedActionIDs,
      connection_id: connection.id,
      return_path: '/console/integrations?view=connected',
    };
    setLastOAuthRequest(request);
    setOAuthProviderName(
      provider ? metadata.providerName(provider) : metadata.providerName(connection.integration_id)
    );
    setOAuthConnectionName(
      safeIntegrationDisplayText(connection.name, t('common.unnamedConnection'))
    );
    void oauthFlow.begin(request);
  };

  const upgradeOAuthAction = (connection: IntegrationConnection, actionID: string) => {
    const provider = catalogProviders.get(connection.integration_id);
    const action = provider?.actions.find(candidate => candidate.id === actionID);
    if (
      !provider ||
      !action ||
      !actionSupportsAuthMethod(action, connection.auth_method_id) ||
      connection.auth_type !== 'oauth2'
    ) {
      toast.error(t('oauth.flow.scopeUpgradeUnavailable'));
      return;
    }
    const request: StartIntegrationOAuthFlowRequest = {
      integration_id: connection.integration_id,
      auth_method_id: connection.auth_method_id,
      credential_source:
        connection.credential_source === 'organization' ? 'organization' : 'account',
      intent: 'scope_upgrade',
      requested_action_ids: [actionID],
      connection_id: connection.id,
      return_path: '/console/integrations?view=connected',
    };
    setLastOAuthRequest(request);
    setOAuthProviderName(metadata.providerName(provider));
    setOAuthConnectionName(
      safeIntegrationDisplayText(connection.name, t('common.unnamedConnection'))
    );
    // Keep the connection details visible behind the OAuth progress dialog.
    // When the flow succeeds, cache invalidation refreshes the permission
    // summary in place; when it fails, the administrator can inspect the same
    // connection and retry without reopening the details.
    void oauthFlow.begin(request);
  };

  const openDelete = async (connection: IntegrationConnection) => {
    setDeleteConnection(connection);
    setDeleteImpact(null);
    setIsImpactLoading(false);
    if (connection.credential_source === 'account') return;
    setIsImpactLoading(true);
    try {
      const response = await integrationService.getDeleteImpact(connection.id);
      setDeleteImpact(response.data);
    } catch {
      setDeleteImpact(null);
    } finally {
      setIsImpactLoading(false);
    }
  };

  const deleteIsOAuth = isOAuthConnection(deleteConnection);
  const deleteIsShared = deleteConnection?.credential_source === 'organization';
  const deleteImpactUnavailable =
    Boolean(deleteIsShared) && !isImpactLoading && deleteImpact === null;
  const deleteBlocked =
    Boolean(deleteIsShared) &&
    (isImpactLoading || deleteImpactUnavailable || deleteImpact?.can_delete === false);
  const deleteDescription = deleteConnection ? (
    <span className="space-y-3">
      <span className="block">
        {deleteIsOAuth
          ? t('disconnect.description', {
              account: safeIntegrationDisplayText(
                deleteConnection.name,
                t('common.unnamedConnection')
              ),
              provider: metadata.providerName(deleteConnection.integration_id),
            })
          : t('delete.description')}
      </span>
      {deleteIsOAuth ? (
        <span className="block rounded-lg border bg-muted/30 p-3 text-xs font-normal leading-5">
          {t(
            deleteConnection.credential_source === 'account'
              ? 'disconnect.personalEffect'
              : 'disconnect.sharedEffect'
          )}
        </span>
      ) : null}
      {isImpactLoading ? (
        <span className="block">{t('delete.loadingImpact')}</span>
      ) : deleteImpactUnavailable ? (
        <span className="block font-medium text-destructive">{t('delete.impactUnavailable')}</span>
      ) : null}
      {deleteImpact?.bound_agent_count ? (
        <span className="block font-medium text-destructive">
          {t('delete.impact', { count: deleteImpact.bound_agent_count })}
        </span>
      ) : null}
      {deleteConnection.is_default ? (
        <span className="block">{t('delete.defaultImpact')}</span>
      ) : null}
      {deleteImpact?.can_delete === false ? (
        <span className="block text-xs font-normal leading-5 text-muted-foreground">
          {t('disconnect.resolveDependencies')}
        </span>
      ) : null}
    </span>
  ) : null;

  const connectionsLoading = canManageShared
    ? managedConnectionsQuery.isLoading ||
      myConnectionsQuery.isLoading ||
      availableConnectionsQuery.isLoading
    : availableConnectionsQuery.isLoading;
  const connectionsError = canManageShared
    ? managedConnectionsQuery.isError ||
      myConnectionsQuery.isError ||
      availableConnectionsQuery.isError
    : availableConnectionsQuery.isError;

  if (catalogQuery.isLoading || connectionsLoading) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-6 w-40" />
        <Skeleton className="h-72 rounded-xl" />
        <Skeleton className="h-16 rounded-xl" />
      </div>
    );
  }

  if (catalogQuery.isError || connectionsError) {
    return (
      <div className="rounded-xl border border-destructive/30 bg-destructive/5 p-10 text-center">
        <p className="text-sm text-destructive">{t('connections.loadFailed')}</p>
        <Button
          className="mt-4"
          variant="outline"
          onClick={() => {
            void catalogQuery.refetch();
            void availableConnectionsQuery.refetch();
            if (canManageShared) {
              void managedConnectionsQuery.refetch();
              void myConnectionsQuery.refetch();
            }
          }}
        >
          <RefreshCw className="size-4" />
          {t('connections.retry')}
        </Button>
      </div>
    );
  }

  return (
    <div className="@container/connections space-y-5">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h2 className="text-base font-semibold">{t('connectionCenter.connected.title')}</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {t('connectionCenter.connected.description')}
          </p>
        </div>
        <span className="text-xs text-muted-foreground">
          {t('connectionCenter.tabs.connectedCount', { count: groups.length })}
        </span>
      </div>

      {groups.length === 0 ? (
        <div className="rounded-xl border border-dashed bg-background p-12 text-center">
          <KeyRound className="mx-auto size-9 text-muted-foreground" />
          <h3 className="mt-4 font-semibold">{t('connectionCenter.connected.emptyTitle')}</h3>
          <p className="mx-auto mt-2 max-w-lg text-sm leading-6 text-muted-foreground">
            {t('connectionCenter.connected.emptyDescription')}
          </p>
          {onBrowseAvailable ? (
            <Button className="mt-5" onClick={onBrowseAvailable}>
              <Plus className="size-4" />
              {t('connectionCenter.connected.emptyAction')}
            </Button>
          ) : null}
        </div>
      ) : (
        <div className="space-y-3">
          {groups.map(group => {
            const open = expandedProviders.has(group.integrationId);
            const oauthClientConfigGroups = resolveProviderOAuthClientConfigGroups(group.provider);
            const detailViews: ProviderDetailView[] =
              canManageShared && oauthClientConfigGroups.length > 0
                ? ['connections', 'capabilities', 'oauth_config']
                : ['connections', 'capabilities'];
            const requestedDetailView = providerDetailViews[group.integrationId] ?? 'connections';
            const detailView = detailViews.includes(requestedDetailView)
              ? requestedDetailView
              : 'connections';
            const healthyConnections = group.connections.filter(
              connection => resolveConnectionHealthState(connection) === 'ready'
            ).length;
            const connected = group.connections.length > 0;
            const healthyAvailableConnections = group.connections.filter(
              connection =>
                availableConnectionIDs.has(connection.id) &&
                resolveConnectionHealthState(connection) === 'ready'
            ).length;
            const selectedConnections = group.connections.filter(
              connection =>
                selectedConnectionIDs.has(connection.id) &&
                availableConnectionIDs.has(connection.id) &&
                resolveConnectionHealthState(connection) === 'ready'
            ).length;
            const usageConfiguredConnections = group.connections.filter(connection => {
              if (connection.credential_source === 'account') return true;
              if (!canManageShared) return availableConnectionIDs.has(connection.id);
              return sharedUsageRuleSummaries.get(connection.id)?.state === 'configured';
            }).length;
            const usageRulesLoading =
              canManageShared &&
              group.connections.some(
                connection =>
                  connection.credential_source === 'organization' &&
                  sharedUsageRuleSummaries.get(connection.id)?.state === 'loading'
              );
            const usageRulesError =
              canManageShared &&
              group.connections.some(
                connection =>
                  connection.credential_source === 'organization' &&
                  sharedUsageRuleSummaries.get(connection.id)?.state === 'error'
              );
            const allUsageRulesConfigured =
              !usageRulesLoading &&
              !usageRulesError &&
              usageConfiguredConnections === group.connections.length;
            const readyForAIChat = selectedConnections > 0;
            const canUseInAIChat = healthyAvailableConnections > 0;
            const canAddPersonalConnection = resolveIntegrationAuthDefinitions(group.provider).some(
              auth => auth.available && integrationAuthCredentialSource(auth) === 'account'
            );
            const canAddSharedConnection =
              canManageShared &&
              resolveIntegrationAuthDefinitions(group.provider).some(
                auth => auth.available && integrationAuthCredentialSource(auth) === 'organization'
              );

            return (
              <Collapsible
                key={group.integrationId}
                open={open}
                onOpenChange={nextOpen => {
                  setExpandedProviders(current => {
                    const next = new Set(current);
                    if (nextOpen) next.add(group.integrationId);
                    else next.delete(group.integrationId);
                    return next;
                  });
                }}
                className="overflow-hidden rounded-xl border bg-background shadow-sm"
              >
                <div className="flex min-h-16 flex-col gap-3 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                  <div className="flex min-w-0 flex-1 items-center gap-3">
                    <div className="flex size-10 shrink-0 items-center justify-center rounded-xl border bg-muted/30 text-primary">
                      <IntegrationProviderIcon
                        integrationId={group.integrationId}
                        driverId={group.provider.driver_id}
                        className="size-5"
                      />
                    </div>
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <h3 className="font-semibold">{metadata.providerName(group.provider)}</h3>
                        <span className="text-xs text-muted-foreground">
                          {t('connectionCenter.connected.connectionCount', {
                            count: group.connections.length,
                          })}
                        </span>
                      </div>
                      <p className="mt-0.5 line-clamp-1 text-xs text-muted-foreground">
                        {metadata.providerDescription(group.provider)}
                      </p>
                    </div>
                  </div>

                  <div className="flex w-full flex-wrap items-center justify-end gap-2 sm:w-auto sm:flex-nowrap">
                    {healthyConnections > 0 ? (
                      <Badge variant="subtle" className="gap-1 text-success">
                        <CheckCircle2 className="size-3" />
                        {t('connectionCenter.connected.healthySummary', {
                          healthy: healthyConnections,
                          total: group.connections.length,
                        })}
                      </Badge>
                    ) : (
                      <Badge variant="outline">{t('health.provider.degraded')}</Badge>
                    )}
                    {canUseInAIChat ? (
                      <Button asChild size="sm" variant={readyForAIChat ? 'secondary' : 'outline'}>
                        <Link href="/console/work/chat">
                          <Bot className="size-4" />
                          {t(
                            readyForAIChat
                              ? 'connectionCenter.connected.openAIChat'
                              : 'connectionCenter.connected.enableAIChat'
                          )}
                        </Link>
                      </Button>
                    ) : (
                      <span
                        role="status"
                        className="inline-flex min-h-8 items-center gap-1.5 rounded-md border border-dashed px-2.5 text-xs text-muted-foreground"
                        aria-label={t('connectionCenter.connected.aiChatUnavailableHint')}
                      >
                        <Bot className="size-4" />
                        {t('connectionCenter.connected.aiChatUnavailable')}
                      </span>
                    )}
                    <AddConnectionButton
                      canAddPersonal={canAddPersonalConnection}
                      canAddShared={canAddSharedConnection}
                      onAddPersonal={() => {
                        setEditingConnection(null);
                        setCreateProvider(group.provider);
                        setConnectionDialogMode('personal');
                      }}
                      onAddShared={() => {
                        setEditingConnection(null);
                        setCreateProvider(group.provider);
                        setConnectionDialogMode('shared');
                      }}
                    />
                    <CollapsibleTrigger asChild>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        isIcon
                        className="order-last ml-auto size-9 shrink-0 sm:ml-0"
                        aria-label={t(
                          open
                            ? 'connectionCenter.connected.collapseProvider'
                            : 'connectionCenter.connected.expandProvider',
                          { provider: metadata.providerName(group.provider) }
                        )}
                      >
                        <ChevronDown
                          className={`size-4 text-muted-foreground transition-transform motion-reduce:transition-none ${
                            open ? 'rotate-180' : ''
                          }`}
                        />
                      </Button>
                    </CollapsibleTrigger>
                  </div>
                </div>

                <CollapsibleContent>
                  <div className="border-t bg-muted/[0.08] p-3 sm:p-4">
                    <div
                      role="tablist"
                      aria-label={t('connectionCenter.connected.views.label')}
                      className="mb-3 flex min-w-0 items-end gap-5 border-b"
                    >
                      {detailViews.map(view => {
                        const count =
                          view === 'connections'
                            ? group.connections.length
                            : view === 'capabilities'
                              ? (group.provider.actions?.length ?? 0)
                              : oauthClientConfigGroups.length;
                        const panelID = `provider-${group.integrationId}-${view}-panel`;
                        return (
                          <button
                            key={view}
                            type="button"
                            role="tab"
                            aria-selected={detailView === view}
                            aria-controls={panelID}
                            className={cn(
                              'relative flex min-h-9 items-center gap-1.5 whitespace-nowrap px-1 pb-2 text-xs font-medium transition-colors',
                              detailView === view
                                ? 'text-primary after:absolute after:inset-x-0 after:bottom-[-1px] after:h-0.5 after:bg-primary'
                                : 'text-muted-foreground hover:text-foreground'
                            )}
                            onClick={() =>
                              setProviderDetailViews(current => ({
                                ...current,
                                [group.integrationId]: view,
                              }))
                            }
                          >
                            {t(`connectionCenter.connected.views.${view}`)}
                            <span
                              className={cn(
                                'rounded-full px-1.5 py-0.5 text-[10px] tabular-nums',
                                detailView === view
                                  ? 'bg-primary/8 text-primary'
                                  : 'bg-muted text-muted-foreground'
                              )}
                            >
                              {count}
                            </span>
                          </button>
                        );
                      })}
                    </div>

                    {detailView === 'connections' ? (
                      <div id={`provider-${group.integrationId}-connections-panel`} role="tabpanel">
                        <ConnectionJourney
                          steps={[
                            {
                              icon: CheckCircle2,
                              state: connected ? 'completed' : 'current',
                              title: t('connectionCenter.connected.journey.connected.title'),
                              description: t(
                                'connectionCenter.connected.journey.connected.summary',
                                {
                                  healthy: healthyConnections,
                                  total: group.connections.length,
                                }
                              ),
                            },
                            {
                              icon: ShieldCheck,
                              state: allUsageRulesConfigured
                                ? 'completed'
                                : connected
                                  ? 'current'
                                  : 'pending',
                              title: t('connectionCenter.connected.journey.authorized.title'),
                              description: usageRulesLoading
                                ? t('connectionCenter.connected.journey.authorized.loading')
                                : usageRulesError
                                  ? t('connectionCenter.connected.usageRules.loadFailed')
                                  : t('connectionCenter.connected.journey.authorized.summary', {
                                      count: usageConfiguredConnections,
                                      total: group.connections.length,
                                    }),
                            },
                            {
                              icon: Bot,
                              state: readyForAIChat
                                ? 'completed'
                                : canUseInAIChat
                                  ? 'current'
                                  : 'pending',
                              title: t('connectionCenter.connected.journey.ready.title'),
                              description: preferencesQuery.isLoading
                                ? t('connectionCenter.connected.journey.ready.loading')
                                : preferencesQuery.isError
                                  ? t('connectionCenter.connected.journey.ready.loadFailed')
                                  : t('connectionCenter.connected.journey.ready.summary', {
                                      count: selectedConnections,
                                    }),
                            },
                          ]}
                        />

                        <div className="overflow-hidden rounded-lg border bg-background">
                          <div className="hidden grid-cols-[minmax(0,1.05fr)_minmax(0,.75fr)_minmax(0,1fr)_minmax(0,.8fr)_minmax(176px,max-content)] gap-4 border-b bg-muted/25 px-4 py-2 text-[11px] font-medium text-muted-foreground @[1040px]/connections:grid">
                            <span>{t('connectionCenter.connected.columns.account')}</span>
                            <span>{t('connectionCenter.connected.columns.health')}</span>
                            <span>{t('connectionCenter.connected.columns.usageRules')}</span>
                            <span>{t('connectionCenter.connected.columns.aiChat')}</span>
                            <span className="text-right">
                              {t('connectionCenter.connected.columns.actions')}
                            </span>
                          </div>
                          {group.connections.map((connection, index) => {
                            const healthState = resolveConnectionHealthState(connection);
                            const lastChecked =
                              connection.last_health_checked_at ?? connection.last_tested_at;
                            const displayName = safeIntegrationDisplayText(
                              connection.name,
                              t('common.unnamedConnection')
                            );
                            const externalIdentity =
                              safeOptionalIntegrationDisplayText(connection.display_name) ??
                              t('connectionDetail.notReported');
                            const currentUserCanUse = availableConnectionIDs.has(connection.id);
                            const selectedInAIChat = selectedConnectionIDs.has(connection.id);
                            const usageRuleSummary: ConnectionUsageRuleSummary =
                              connection.credential_source === 'account'
                                ? { state: 'personal', ruleCount: 0, actionCount: 0 }
                                : canManageShared
                                  ? (sharedUsageRuleSummaries.get(connection.id) ?? {
                                      state: 'loading',
                                      ruleCount: 0,
                                      actionCount: 0,
                                    })
                                  : {
                                      state: currentUserCanUse ? 'available' : 'not_configured',
                                      ruleCount: 0,
                                      actionCount: 0,
                                    };

                            return (
                              <article
                                key={connection.id}
                                className={`grid gap-4 p-4 @[1040px]/connections:grid-cols-[minmax(0,1.05fr)_minmax(0,.75fr)_minmax(0,1fr)_minmax(0,.8fr)_minmax(176px,max-content)] @[1040px]/connections:items-center ${
                                  index > 0 ? 'border-t' : ''
                                }`}
                              >
                                <div className="flex min-w-0 items-center gap-3">
                                  <div className="flex size-9 shrink-0 items-center justify-center rounded-full bg-primary/8 text-primary">
                                    <UserRound className="size-4" />
                                  </div>
                                  <div className="min-w-0">
                                    <div className="flex flex-wrap items-center gap-1.5">
                                      <h4 className="truncate text-sm font-medium">
                                        {displayName}
                                      </h4>
                                    </div>
                                    <p className="mt-0.5 truncate text-xs text-muted-foreground">
                                      {externalIdentity}
                                    </p>
                                  </div>
                                </div>

                                <div className="min-w-0">
                                  <p className="mb-1 text-[11px] font-medium text-muted-foreground @[1040px]/connections:hidden">
                                    {t('connectionCenter.connected.columns.health')}
                                  </p>
                                  <IntegrationConnectionHealthBadge connection={connection} />
                                  <p className="mt-1 text-xs text-muted-foreground">
                                    {lastChecked
                                      ? t('connectionCenter.connected.lastChecked', {
                                          date: metadata.date(
                                            lastChecked,
                                            t('connectionCenter.connected.neverChecked')
                                          ),
                                        })
                                      : t('connectionCenter.connected.neverChecked')}
                                  </p>
                                </div>

                                <ConnectionUsageRulesStatus
                                  summary={usageRuleSummary}
                                  currentUserCanUse={currentUserCanUse}
                                />

                                <ConnectionAIChatStatus
                                  selected={selectedInAIChat}
                                  available={currentUserCanUse && healthState === 'ready'}
                                  loading={preferencesQuery.isLoading}
                                  error={preferencesQuery.isError}
                                />

                                <div className="flex items-center justify-end gap-2">
                                  {canManageShared &&
                                  connection.credential_source === 'organization' ? (
                                    <ConnectionUsageRulesAction
                                      summary={usageRuleSummary}
                                      connectionName={displayName}
                                      onManage={() => setDetailConnection(connection)}
                                    />
                                  ) : null}
                                  {canManageConnection(connection) ? (
                                    <ConnectionActionsMenu
                                      connection={connection}
                                      oauthConnection={isOAuthConnection(connection)}
                                      updatePending={connectionMutationPending}
                                      testPending={testMutationPending}
                                      onView={
                                        canManageConnection(connection)
                                          ? () => setDetailConnection(connection)
                                          : undefined
                                      }
                                      onEdit={() => openEdit(connection)}
                                      onTest={() => setTestConnection(connection)}
                                      onReconnect={
                                        ['oauth', 'oauth2'].includes(connection.auth_type)
                                          ? () => reconnectOAuth(connection)
                                          : undefined
                                      }
                                      reconnectLabel={
                                        connection.attention_code === 'scope_update_required'
                                          ? t('connections.actions.upgradeScopes')
                                          : t('connections.actions.reconnect')
                                      }
                                      onToggle={() => {
                                        const data = {
                                          revision: connection.revision ?? 0,
                                          disabled: connection.status !== 'disabled',
                                        };
                                        if (connection.credential_source === 'account') {
                                          updateMyMutation.mutate({ id: connection.id, data });
                                        } else {
                                          updateMutation.mutate({ id: connection.id, data });
                                        }
                                      }}
                                      onDelete={() => void openDelete(connection)}
                                      accessibleName={t('connections.table.actionsFor', {
                                        name: displayName,
                                      })}
                                    />
                                  ) : null}
                                </div>
                              </article>
                            );
                          })}
                        </div>
                      </div>
                    ) : detailView === 'capabilities' ? (
                      <div
                        id={`provider-${group.integrationId}-capabilities-panel`}
                        role="tabpanel"
                      >
                        <IntegrationProviderCapabilitiesInline
                          provider={group.provider}
                          canManageShared={canManageShared}
                        />
                      </div>
                    ) : (
                      <div
                        id={`provider-${group.integrationId}-oauth_config-panel`}
                        role="tabpanel"
                      >
                        <IntegrationProviderOAuthClientConfigsInline provider={group.provider} />
                      </div>
                    )}
                  </div>
                </CollapsibleContent>
              </Collapsible>
            );
          })}
        </div>
      )}

      <IntegrationConnectionDialog
        context={connectionDialogMode === 'shared' ? 'shared' : 'personal'}
        open={Boolean(createProvider && connectionDialogMode)}
        catalog={createProvider ? [createProvider] : []}
        connection={editingConnection}
        isSubmitting={connectionMutationPending || testMutationPending}
        allowedCredentialSources={
          connectionDialogMode === 'shared' ? [...ORGANIZATION_CREDENTIAL_SOURCES] : ['account']
        }
        availableAuthMethodsOnly
        onOpenChange={open => {
          if (!open) {
            setCreateProvider(null);
            setEditingConnection(null);
            setConnectionDialogMode(null);
          }
        }}
        onCreate={async data => {
          const response =
            connectionDialogMode === 'shared'
              ? await createMutation.mutateAsync(data)
              : await createMyMutation.mutateAsync(data);
          const saved = response.data;
          try {
            if (saved.credential_source === 'account') {
              await testMyMutation.mutateAsync(saved.id);
            } else {
              await testMutation.mutateAsync(saved.id);
            }
          } catch {
            // Saving and testing are separate outcomes. The test mutation
            // reports the failure and refreshes the persisted health state.
          }
        }}
        onUpdate={async (id, data) => {
          if (editingConnection?.credential_source === 'account') {
            await updateMyMutation.mutateAsync({ id, data });
          } else {
            await updateMutation.mutateAsync({ id, data });
          }
          if (!data.credentials || Object.keys(data.credentials).length === 0) return;
          try {
            if (editingConnection?.credential_source === 'account') {
              await testMyMutation.mutateAsync(id);
            } else {
              await testMutation.mutateAsync(id);
            }
          } catch {
            // Keep the saved credential and expose the failed health result.
          }
        }}
      />

      <IntegrationOAuthFlowDialog
        state={oauthFlow.state}
        providerName={oauthProviderName}
        connectionName={oauthConnectionName}
        onCancel={oauthFlow.cancel}
        onDone={oauthFlow.reset}
        onRefresh={() => void oauthFlow.refresh()}
        onOpenFullPage={oauthFlow.openFullPage}
        onReopenPopup={oauthFlow.reopenPopup}
        onRetry={() => {
          if (lastOAuthRequest) void oauthFlow.begin(lastOAuthRequest);
        }}
      />

      {detailConnection && canManageConnection(detailConnection) ? (
        <IntegrationConnectionDetailDialog
          open={Boolean(detailConnection)}
          connection={detailConnection}
          provider={catalogProviders.get(detailConnection?.integration_id ?? '')}
          isTesting={testMutationPending}
          isUpgrading={oauthFlow.state.status !== 'idle'}
          canManage
          onOpenChange={open => {
            if (!open) setDetailConnection(null);
          }}
          onEdit={connection => {
            setDetailConnection(null);
            openEdit(connection);
          }}
          onTest={connection => {
            setTestConnection(connection);
          }}
          onUpgradeAction={upgradeOAuthAction}
        />
      ) : null}

      <ConfirmDialog
        variant="warning"
        open={Boolean(testConnection)}
        onOpenChange={open => {
          if (!open) setTestConnection(null);
        }}
        title={t('test.title')}
        description={t('test.description')}
        cancelText={t('test.cancel')}
        confirmText={t('test.confirm')}
        loading={testMutationPending}
        onConfirm={() => {
          if (!testConnection) return;
          if (testConnection.credential_source === 'account') {
            testMyMutation.mutate(testConnection.id);
          } else {
            testMutation.mutate(testConnection.id);
          }
        }}
      />

      <ConfirmDialog
        variant="danger"
        open={Boolean(deleteConnection)}
        onOpenChange={open => {
          if (!open) {
            setDeleteConnection(null);
            setDeleteImpact(null);
          }
        }}
        title={t(deleteIsOAuth ? 'disconnect.title' : 'delete.title')}
        description={deleteDescription}
        cancelText={t('delete.cancel')}
        confirmText={t(deleteIsOAuth ? 'disconnect.confirm' : 'delete.confirm')}
        loading={deleteMutationPending || isImpactLoading}
        confirmDisabled={deleteBlocked}
        closeOnConfirm={false}
        onConfirm={() => {
          if (!deleteConnection) return;
          const mutation =
            deleteConnection.credential_source === 'account'
              ? deleteMyMutation.mutateAsync(deleteConnection.id)
              : deleteMutation.mutateAsync(deleteConnection.id);
          void mutation
            .then(() => {
              setDeleteConnection(null);
              setDeleteImpact(null);
            })
            .catch(() => undefined);
        }}
      />
    </div>
  );
}

interface ConnectionJourneyStepData {
  icon: typeof CheckCircle2;
  title: string;
  description: string;
  state: 'completed' | 'current' | 'pending';
}

function ConnectionJourney({ steps }: { steps: ConnectionJourneyStepData[] }) {
  const t = useT('integrations');
  return (
    <ol className="mb-3 grid gap-0 rounded-lg border bg-background px-4 py-4 md:grid-cols-3">
      {steps.map((step, index) => {
        const Icon = step.icon;
        const nextStep = steps[index + 1];
        return (
          <li
            key={step.title}
            className="relative flex min-w-0 items-start gap-3 pb-5 last:pb-0 md:block md:px-3 md:pb-0 md:text-center"
          >
            {nextStep ? (
              <span
                aria-hidden
                className={cn(
                  'absolute left-3.5 top-7 h-[calc(100%-1.75rem)] w-px md:left-[calc(50%+0.875rem)] md:right-[calc(-50%+0.875rem)] md:top-3.5 md:h-px md:w-auto',
                  nextStep.state === 'pending' ? 'bg-border' : 'bg-success/55'
                )}
              />
            ) : null}
            <div
              className={`relative z-10 flex size-7 shrink-0 items-center justify-center rounded-full border md:mx-auto ${
                step.state === 'completed'
                  ? 'border-success/30 bg-success/12 text-success'
                  : step.state === 'current'
                    ? 'border-primary/30 bg-primary/10 text-primary'
                    : 'border-border bg-background text-muted-foreground'
              }`}
            >
              <Icon className="size-4" />
            </div>
            <div className="min-w-0 md:mt-2">
              <p className="text-xs font-semibold">
                {step.title}
                <span className="sr-only">
                  {' '}
                  {t(`connectionCenter.connected.journey.state.${step.state}`)}
                </span>
              </p>
              <p className="mt-0.5 text-[11px] leading-4 text-muted-foreground">
                {step.description}
              </p>
            </div>
          </li>
        );
      })}
    </ol>
  );
}

function ConnectionUsageRulesStatus({
  summary,
  currentUserCanUse,
}: {
  summary: ConnectionUsageRuleSummary;
  currentUserCanUse: boolean;
}) {
  const t = useT('integrations');
  const label = t('connectionCenter.connected.columns.usageRules');

  if (summary.state === 'loading') {
    return (
      <div className="min-w-0">
        <p className="mb-1 text-[11px] font-medium text-muted-foreground @[1040px]/connections:hidden">
          {label}
        </p>
        <Skeleton className="h-5 w-28" />
        <Skeleton className="mt-2 h-3 w-40" />
      </div>
    );
  }

  const stateContent = {
    personal: {
      variant: 'info' as const,
      title: t('connectionCenter.connected.usageRules.personal'),
      description: t('connectionCenter.connected.usageRules.personalDescription'),
    },
    configured: {
      variant: summary.issueCount ? ('warning' as const) : ('success' as const),
      title: summary.issueCount
        ? t('connectionCenter.connected.usageRules.configuredWithIssues')
        : t('connectionCenter.connected.usageRules.configured'),
      description: summary.issueCount
        ? t('connectionCenter.connected.usageRules.configuredWithIssuesSummary', {
            rules: summary.ruleCount,
            actions: summary.actionCount,
            issues: summary.issueCount,
          })
        : t('connectionCenter.connected.usageRules.configuredSummary', {
            rules: summary.ruleCount,
            actions: summary.actionCount,
          }),
    },
    not_configured: {
      variant: 'warning' as const,
      title: t('connectionCenter.connected.usageRules.notConfigured'),
      description: t('connectionCenter.connected.usageRules.notConfiguredDescription'),
    },
    available: {
      variant: 'success' as const,
      title: t('connectionCenter.connected.usageRules.availableToMe'),
      description: t('connectionCenter.connected.usageRules.managedByOrganization'),
    },
    error: {
      variant: 'warning' as const,
      title: t('connectionCenter.connected.usageRules.unknown'),
      description: t('connectionCenter.connected.usageRules.loadFailed'),
    },
    loading: {
      variant: 'subtle' as const,
      title: '',
      description: '',
    },
  }[summary.state];

  return (
    <div className="min-w-0">
      <p className="mb-1 text-[11px] font-medium text-muted-foreground @[1040px]/connections:hidden">
        {label}
      </p>
      <div className="flex flex-wrap items-center gap-1.5">
        <Badge variant={stateContent.variant}>{stateContent.title}</Badge>
        {summary.state === 'configured' && currentUserCanUse ? (
          <Badge variant="outline">
            {t('connectionCenter.connected.usageRules.currentUserAvailable')}
          </Badge>
        ) : null}
      </div>
      <p className="mt-1 text-xs leading-5 text-muted-foreground">{stateContent.description}</p>
    </div>
  );
}

function ConnectionUsageRulesAction({
  summary,
  connectionName,
  onManage,
}: {
  summary: ConnectionUsageRuleSummary;
  connectionName: string;
  onManage: () => void;
}) {
  const t = useT('integrations');
  const needsConfiguration = summary.state === 'not_configured';
  const loading = summary.state === 'loading';

  return (
    <Button
      type="button"
      variant={needsConfiguration ? 'default' : 'secondary'}
      size="sm"
      className="min-w-0 flex-1 gap-1.5 px-3 text-xs font-medium @[1040px]/connections:flex-none"
      onClick={onManage}
      disabled={loading}
      aria-label={t('connectionCenter.connected.usageRules.manageFor', {
        name: connectionName,
      })}
    >
      <ShieldCheck className="size-3.5 shrink-0" />
      <span className="truncate">
        {t(
          needsConfiguration
            ? 'connectionCenter.connected.usageRules.configure'
            : 'connectionCenter.connected.usageRules.manage'
        )}
      </span>
    </Button>
  );
}

function ConnectionAIChatStatus({
  selected,
  available,
  loading,
  error,
}: {
  selected: boolean;
  available: boolean;
  loading: boolean;
  error: boolean;
}) {
  const t = useT('integrations');
  const enabled = selected && available;
  const selectedButUnavailable = selected && !available;

  if (loading) {
    return (
      <div className="min-w-0">
        <p className="mb-1 text-[11px] font-medium text-muted-foreground @[1040px]/connections:hidden">
          {t('connectionCenter.connected.columns.aiChat')}
        </p>
        <Skeleton className="h-5 w-28" />
        <Skeleton className="mt-2 h-3 w-36" />
      </div>
    );
  }

  return (
    <div className="min-w-0">
      <p className="mb-1 text-[11px] font-medium text-muted-foreground @[1040px]/connections:hidden">
        {t('connectionCenter.connected.columns.aiChat')}
      </p>
      <Badge
        variant={
          error
            ? 'warning'
            : enabled
              ? 'success'
              : selectedButUnavailable
                ? 'warning'
                : available
                  ? 'info'
                  : 'subtle'
        }
      >
        {t(
          error
            ? 'connectionCenter.connected.aiChat.unknown'
            : enabled
              ? 'connectionCenter.connected.aiChat.selected'
              : selectedButUnavailable
                ? 'connectionCenter.connected.aiChat.selectedButUnavailable'
                : available
                  ? 'connectionCenter.connected.aiChat.available'
                  : 'connectionCenter.connected.aiChat.unavailable'
        )}
      </Badge>
      <p className="mt-1 text-xs leading-5 text-muted-foreground">
        {t(
          error
            ? 'connectionCenter.connected.aiChat.loadFailed'
            : enabled
              ? 'connectionCenter.connected.aiChat.selectedDescription'
              : selectedButUnavailable
                ? 'connectionCenter.connected.aiChat.selectedButUnavailableDescription'
                : available
                  ? 'connectionCenter.connected.aiChat.availableDescription'
                  : 'connectionCenter.connected.aiChat.unavailableDescription'
        )}
      </p>
    </div>
  );
}

function AddConnectionButton({
  canAddPersonal,
  canAddShared,
  onAddPersonal,
  onAddShared,
}: {
  canAddPersonal: boolean;
  canAddShared: boolean;
  onAddPersonal: () => void;
  onAddShared: () => void;
}) {
  const t = useT('integrations');
  if (!canAddPersonal && !canAddShared) return null;

  if (canAddPersonal && canAddShared) {
    return (
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button type="button" size="sm" variant="outline">
            <Plus className="size-4" />
            {t('connectionCenter.connected.addAccount')}
            <ChevronDown className="size-3.5" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-56">
          <DropdownMenuItem onSelect={onAddPersonal}>
            <UserRound className="mr-2 size-4" />
            {t('connectionCenter.connected.addPersonalAccount')}
          </DropdownMenuItem>
          <DropdownMenuItem onSelect={onAddShared}>
            <ShieldCheck className="mr-2 size-4" />
            {t('connectionCenter.connected.addSharedAccount')}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    );
  }

  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      onClick={canAddPersonal ? onAddPersonal : onAddShared}
    >
      <Plus className="size-4" />
      {t(
        canAddPersonal
          ? 'connectionCenter.connected.addPersonalAccount'
          : 'connectionCenter.connected.addSharedAccount'
      )}
    </Button>
  );
}

interface ConnectionActionsMenuProps {
  connection: IntegrationConnection;
  oauthConnection: boolean;
  accessibleName: string;
  updatePending: boolean;
  testPending: boolean;
  onView?: () => void;
  onEdit: () => void;
  onTest: () => void;
  onReconnect?: () => void;
  reconnectLabel?: string;
  onToggle: () => void;
  onDelete: () => void;
}

function ConnectionActionsMenu({
  connection,
  oauthConnection,
  accessibleName,
  updatePending,
  testPending,
  onView,
  onEdit,
  onTest,
  onReconnect,
  reconnectLabel,
  onToggle,
  onDelete,
}: ConnectionActionsMenuProps) {
  const t = useT('integrations');
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="sm" isIcon aria-label={accessibleName}>
          <Ellipsis className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-48">
        {onView ? (
          <DropdownMenuItem onClick={onView}>
            <Eye className="mr-2 size-4" />
            {t('connections.actions.view')}
          </DropdownMenuItem>
        ) : null}
        <DropdownMenuItem onClick={onEdit}>
          <Pencil className="mr-2 size-4" />
          {t('connections.actions.edit')}
        </DropdownMenuItem>
        <DropdownMenuItem disabled={testPending} onClick={onTest}>
          <Play className="mr-2 size-4" />
          {t('connections.actions.test')}
        </DropdownMenuItem>
        {onReconnect ? (
          <DropdownMenuItem onClick={onReconnect}>
            <RefreshCw className="mr-2 size-4" />
            {reconnectLabel ?? t('connections.actions.reconnect')}
          </DropdownMenuItem>
        ) : null}
        <DropdownMenuItem disabled={updatePending} onClick={onToggle}>
          <Power className="mr-2 size-4" />
          {t(
            connection.status === 'disabled'
              ? 'connections.actions.enable'
              : 'connections.actions.disable'
          )}
        </DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem className="text-destructive" onClick={onDelete}>
          {oauthConnection ? (
            <Unlink className="mr-2 size-4" />
          ) : (
            <Trash2 className="mr-2 size-4" />
          )}
          {t(
            oauthConnection ? 'connections.actions.disconnectAccount' : 'connections.actions.delete'
          )}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
