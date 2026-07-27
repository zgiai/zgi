'use client';

import { useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import {
  ArrowRight,
  CheckCircle2,
  ChevronDown,
  Eye,
  KeyRound,
  Plus,
  SearchX,
  Settings2,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardFooter, CardHeader } from '@/components/ui/card';
import { Skeleton } from '@/components/ui/skeleton';
import {
  integrationCatalogItems,
  integrationConnectionItems,
  useAllIntegrationConnections,
  useAllMyIntegrationConnections,
  useCreateIntegrationConnection,
  useCreateMyIntegrationConnection,
  useIntegrationCatalog,
} from '@/hooks';
import { INTEGRATION_KEYS } from '@/hooks/query-keys';
import { useT } from '@/i18n';
import { integrationService } from '@/services/integration.service';
import type {
  IntegrationAuthDefinition,
  IntegrationCatalogItem,
} from '@/services/types/integration';
import { IntegrationAuthMethodPickerDialog } from './auth-method-picker-dialog';
import {
  authMethodsSharingOAuthClient,
  isOAuthAuthMethod,
  selectPrimaryAuthMethod,
} from './auth-method-selection';
import { IntegrationConnectionDialog } from './connection-dialog';
import { IntegrationProviderHealthBadge } from './health-badge';
import {
  integrationAuthCredentialSource,
  integrationCatalogID,
  resolveIntegrationAuthDefinitions,
  resolveProviderHealthState,
} from './integration-utils';
import { useIntegrationMetadata } from './metadata-i18n';
import { IntegrationOAuthClientConfigDialog } from './oauth-client-config-dialog';
import { IntegrationProviderIcon } from './provider-icon';
import { IntegrationProviderCapabilitiesSheet } from './provider-capabilities-sheet';

interface IntegrationProviderCatalogProps {
  query?: string;
  category?: string;
  canManageShared?: boolean;
  integrationId?: string;
  onClearIntegrationId?: () => void;
}

export function IntegrationProviderCatalog({
  query = '',
  category = 'all',
  canManageShared = false,
  integrationId,
  onClearIntegrationId,
}: IntegrationProviderCatalogProps) {
  const t = useT('integrations');
  const metadata = useIntegrationMetadata();
  const catalogQuery = useIntegrationCatalog(true, canManageShared ? 'organization' : 'account');
  const managedConnectionsQuery = useAllIntegrationConnections(undefined, canManageShared);
  const availableConnectionsQuery = useQuery({
    queryKey: INTEGRATION_KEYS.availableConnections({ all: true }),
    queryFn: () => integrationService.getAllAvailableConnections(),
    enabled: !canManageShared,
    staleTime: 30_000,
    retry: false,
  });
  const myConnectionsQuery = useAllMyIntegrationConnections(undefined, canManageShared);
  const createMutation = useCreateIntegrationConnection();
  const createMyMutation = useCreateMyIntegrationConnection();
  const [selectedProvider, setSelectedProvider] = useState<IntegrationCatalogItem | null>(null);
  const [selectedAuthMethod, setSelectedAuthMethod] = useState<IntegrationAuthDefinition | null>(
    null
  );
  const [dialogMode, setDialogMode] = useState<'personal' | 'shared' | null>(null);
  const [authPickerProvider, setAuthPickerProvider] = useState<IntegrationCatalogItem | null>(null);
  const [authPickerMethods, setAuthPickerMethods] = useState<IntegrationAuthDefinition[]>([]);
  const [authPickerRecommendedId, setAuthPickerRecommendedId] = useState('');
  const [capabilityProvider, setCapabilityProvider] = useState<IntegrationCatalogItem | null>(null);
  const [oauthClientAuth, setOAuthClientAuth] = useState<IntegrationAuthDefinition | null>(null);
  const [oauthClientProvider, setOAuthClientProvider] = useState<IntegrationCatalogItem | null>(
    null
  );
  const [continueAfterOAuthConfig, setContinueAfterOAuthConfig] = useState(false);
  const catalog = integrationCatalogItems(catalogQuery.data?.data).filter(item => item.enabled);
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
  const normalizedQuery = query.trim().toLocaleLowerCase(metadata.locale);

  const filtered = useMemo(
    () =>
      catalog.filter(item => {
        if (integrationId && integrationCatalogID(item) !== integrationId) return false;
        const categories = item.categories ?? (item.category ? [item.category] : []);
        if (category !== 'all' && !categories.includes(category)) return false;
        if (!normalizedQuery) return true;
        return [
          integrationCatalogID(item),
          item.driver_id,
          item.name,
          item.description,
          metadata.providerName(item),
          metadata.providerDescription(item),
          ...categories.map(value => metadata.category(value, item)),
          ...(item.tags ?? []).map(tag => metadata.tag(tag, item)),
        ]
          .filter(Boolean)
          .join(' ')
          .toLocaleLowerCase(metadata.locale)
          .includes(normalizedQuery);
      }),
    [catalog, category, integrationId, metadata, normalizedQuery]
  );
  const focusedProvider = integrationId
    ? catalog.find(item => integrationCatalogID(item) === integrationId)
    : undefined;
  const connectionsLoading = canManageShared
    ? managedConnectionsQuery.isLoading || myConnectionsQuery.isLoading
    : availableConnectionsQuery.isLoading;
  const connectionsError = canManageShared
    ? managedConnectionsQuery.isError || myConnectionsQuery.isError
    : availableConnectionsQuery.isError;

  const beginConnection = (provider: IntegrationCatalogItem, auth: IntegrationAuthDefinition) => {
    const source = integrationAuthCredentialSource(auth);
    setSelectedProvider(provider);
    setSelectedAuthMethod(auth);
    setDialogMode(source === 'organization' ? 'shared' : 'personal');
  };

  const openAuthMethod = (provider: IntegrationCatalogItem, auth: IntegrationAuthDefinition) => {
    if (isOAuthAuthMethod(auth) && !auth.oauth?.client_configured) {
      if (canManageShared) {
        setContinueAfterOAuthConfig(true);
        setOAuthClientProvider(provider);
        setOAuthClientAuth(auth);
      }
      return;
    }
    beginConnection(provider, auth);
  };

  if (catalogQuery.isLoading || connectionsLoading) {
    return (
      <div className="@container/provider-catalog space-y-4">
        <Skeleton className="h-6 w-36" />
        <div className="grid gap-4 @md/provider-catalog:grid-cols-2 @4xl/provider-catalog:grid-cols-3 @6xl/provider-catalog:grid-cols-4">
          {Array.from({ length: 4 }).map((_, index) => (
            <Skeleton key={index} className="h-48 rounded-xl" />
          ))}
        </div>
      </div>
    );
  }

  if (catalogQuery.isError || connectionsError) {
    return (
      <div className="rounded-xl border border-destructive/30 bg-destructive/5 p-10 text-center">
        <p className="text-sm text-destructive">{t('catalog.loadFailed')}</p>
        <Button
          className="mt-4"
          variant="outline"
          onClick={() => {
            void catalogQuery.refetch();
            if (canManageShared) {
              void managedConnectionsQuery.refetch();
              void myConnectionsQuery.refetch();
            } else {
              void availableConnectionsQuery.refetch();
            }
          }}
        >
          {t('catalog.retry')}
        </Button>
      </div>
    );
  }

  return (
    <div className="@container/provider-catalog space-y-5">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <h2 className="text-base font-semibold">{t('connectionCenter.quickConnect.title')}</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {t('connectionCenter.quickConnect.description')}
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-xs text-muted-foreground">
            {t('connectionCenter.tabs.availableCount', { count: filtered.length })}
          </span>
          {canManageShared ? (
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                setSelectedProvider(null);
                setSelectedAuthMethod(null);
                setDialogMode('shared');
              }}
            >
              <Plus className="size-4" />
              {t('connectionCenter.quickConnect.addShared')}
            </Button>
          ) : null}
        </div>
      </div>

      {integrationId ? (
        <div className="flex flex-wrap items-center justify-between gap-2 rounded-lg border bg-muted/20 px-3 py-2 text-sm">
          <span>
            {t('connections.filteredBy', {
              provider: focusedProvider
                ? metadata.providerName(focusedProvider)
                : metadata.providerName(integrationId),
            })}
          </span>
          {onClearIntegrationId ? (
            <Button variant="link" size="sm" onClick={onClearIntegrationId}>
              {t('connections.showAll')}
            </Button>
          ) : null}
        </div>
      ) : null}

      {filtered.length === 0 ? (
        <div className="rounded-xl border border-dashed bg-background p-12 text-center text-muted-foreground">
          <SearchX className="mx-auto size-8" />
          <p className="mt-3 text-sm">{t('connectionCenter.quickConnect.empty')}</p>
        </div>
      ) : (
        <div className="grid gap-4 @md/provider-catalog:grid-cols-2 @4xl/provider-catalog:grid-cols-3 @6xl/provider-catalog:grid-cols-4">
          {filtered.map(provider => {
            const integrationId = integrationCatalogID(provider);
            const scopedConnections = connections.filter(
              connection => connection.integration_id === integrationId
            );
            const activeConnections = scopedConnections.filter(
              connection => connection.status === 'active'
            ).length;
            const authDefinitions = resolveIntegrationAuthDefinitions(provider).filter(auth => {
              const source = integrationAuthCredentialSource(auth);
              return (
                auth.available &&
                (source === 'account' || (canManageShared && source === 'organization'))
              );
            });
            const primaryAuth = selectPrimaryAuthMethod(authDefinitions, canManageShared);
            const oauthClientMissing =
              Boolean(primaryAuth && isOAuthAuthMethod(primaryAuth)) &&
              !primaryAuth?.oauth?.client_configured;
            const oauthSettingsAuth = canManageShared
              ? authDefinitions.find(
                  method => isOAuthAuthMethod(method) && method.oauth?.client_configured
                )
              : undefined;
            const health = resolveProviderHealthState(
              {
                ...provider,
                connection_summary: undefined,
                health_state: undefined,
              },
              connections
            );
            return (
              <Card
                key={integrationId}
                hoverLift
                className="@container/provider-card group flex min-h-40 min-w-0 flex-col bg-background"
              >
                <CardHeader className="flex-row items-start gap-3 space-y-0 p-4 pb-2">
                  <div className="flex size-11 shrink-0 items-center justify-center rounded-xl border bg-muted/30 text-primary transition-colors group-hover:border-primary/30 group-hover:bg-primary/5">
                    <IntegrationProviderIcon
                      integrationId={integrationId}
                      driverId={provider.driver_id}
                      className="size-6"
                    />
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-start justify-between gap-2">
                      <h3 className="truncate font-semibold">{metadata.providerName(provider)}</h3>
                      {activeConnections > 0 ? (
                        <Badge variant="subtle" className="shrink-0 gap-1 text-[11px]">
                          <CheckCircle2 className="size-3 text-success" />
                          {t('connectionCenter.tabs.connectedCount', {
                            count: activeConnections,
                          })}
                        </Badge>
                      ) : (
                        <IntegrationProviderHealthBadge state={health} />
                      )}
                    </div>
                    <p className="mt-1 line-clamp-2 text-sm leading-5 text-muted-foreground">
                      {metadata.providerDescription(provider)}
                    </p>
                  </div>
                </CardHeader>

                <CardFooter className="mt-auto flex flex-col gap-1.5 border-t bg-muted/10 p-3">
                  <div className="flex min-w-0 flex-1">
                    <Button
                      size="sm"
                      className={
                        authDefinitions.length > 1 ? 'min-w-0 flex-1 rounded-r-none' : 'w-full'
                      }
                      variant={oauthClientMissing ? 'outline' : 'default'}
                      onClick={() => {
                        if (primaryAuth) openAuthMethod(provider, primaryAuth);
                      }}
                      disabled={!primaryAuth || (oauthClientMissing && !canManageShared)}
                    >
                      {oauthClientMissing ? (
                        <KeyRound className="size-4" />
                      ) : activeConnections > 0 ? (
                        <Plus className="size-4" />
                      ) : (
                        <ArrowRight className="size-4" />
                      )}
                      <span className="whitespace-nowrap">
                        {oauthClientMissing
                          ? t(
                              canManageShared
                                ? 'oauth.clientConfig.configureAction'
                                : 'oauth.clientConfig.adminSetupRequired'
                            )
                          : t(
                              activeConnections > 0
                                ? 'connectionCenter.quickConnect.addAnother'
                                : 'connectionCenter.quickConnect.connect'
                            )}
                      </span>
                    </Button>
                    {authDefinitions.length > 1 ? (
                      <Button
                        type="button"
                        size="sm"
                        isIcon
                        variant={oauthClientMissing ? 'outline' : 'default'}
                        className="shrink-0 rounded-l-none border-l border-l-background/30"
                        aria-label={t('authMethodPicker.openFor', {
                          provider: metadata.providerName(provider),
                        })}
                        onClick={() => {
                          setAuthPickerProvider(provider);
                          setAuthPickerMethods(authDefinitions);
                          setAuthPickerRecommendedId(primaryAuth?.id ?? '');
                        }}
                      >
                        <ChevronDown className="size-4" />
                      </Button>
                    ) : null}
                  </div>
                  <div
                    className={
                      oauthSettingsAuth
                        ? 'grid w-full grid-cols-[minmax(0,3fr)_minmax(0,2fr)] gap-1'
                        : 'grid w-full grid-cols-1'
                    }
                  >
                    {oauthSettingsAuth ? (
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        className="min-w-0 justify-center px-2 text-primary hover:bg-primary/5 hover:text-primary"
                        onClick={() => {
                          setContinueAfterOAuthConfig(false);
                          setOAuthClientProvider(provider);
                          setOAuthClientAuth(oauthSettingsAuth);
                        }}
                      >
                        <Settings2 className="size-4 shrink-0" />
                        <span className="whitespace-nowrap">
                          {t('oauth.clientConfig.manageAction')}
                        </span>
                      </Button>
                    ) : null}
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      className="min-w-0 justify-center px-2 text-primary hover:bg-primary/5 hover:text-primary"
                      aria-label={t('capabilities.viewFor', {
                        provider: metadata.providerName(provider),
                      })}
                      onClick={() => setCapabilityProvider(provider)}
                    >
                      <Eye className="size-4 shrink-0" />
                      <span className="whitespace-nowrap">{t('capabilities.view')}</span>
                    </Button>
                  </div>
                </CardFooter>
              </Card>
            );
          })}
        </div>
      )}

      <IntegrationConnectionDialog
        open={Boolean(dialogMode)}
        catalog={selectedProvider ? [selectedProvider] : catalog}
        context={dialogMode === 'shared' ? 'shared' : 'personal'}
        isSubmitting={
          dialogMode === 'shared' ? createMutation.isPending : createMyMutation.isPending
        }
        allowedCredentialSources={dialogMode === 'shared' ? ['organization'] : ['account']}
        availableAuthMethodsOnly
        lockedAuthMethodId={selectedAuthMethod?.id}
        onOpenChange={open => {
          if (!open) {
            setSelectedProvider(null);
            setSelectedAuthMethod(null);
            setDialogMode(null);
          }
        }}
        onCreate={async data => {
          if (dialogMode === 'shared') await createMutation.mutateAsync(data);
          else await createMyMutation.mutateAsync(data);
        }}
        onUpdate={async () => undefined}
      />

      <IntegrationAuthMethodPickerDialog
        open={Boolean(authPickerProvider)}
        integrationId={authPickerProvider ? integrationCatalogID(authPickerProvider) : ''}
        providerName={authPickerProvider ? metadata.providerName(authPickerProvider) : ''}
        methods={authPickerMethods}
        actions={authPickerProvider?.actions ?? []}
        recommendedAuthMethodId={authPickerRecommendedId}
        canConfigureOAuthClient={canManageShared}
        onOpenChange={open => {
          if (!open) {
            setAuthPickerProvider(null);
            setAuthPickerMethods([]);
            setAuthPickerRecommendedId('');
          }
        }}
        onSelect={auth => {
          if (!authPickerProvider) return;
          openAuthMethod(authPickerProvider, auth);
          setAuthPickerProvider(null);
          setAuthPickerMethods([]);
          setAuthPickerRecommendedId('');
        }}
      />

      <IntegrationOAuthClientConfigDialog
        open={Boolean(oauthClientAuth && oauthClientProvider)}
        integrationId={oauthClientProvider ? integrationCatalogID(oauthClientProvider) : ''}
        providerName={oauthClientProvider ? metadata.providerName(oauthClientProvider) : ''}
        auth={oauthClientAuth}
        relatedAuthMethods={
          oauthClientProvider
            ? authMethodsSharingOAuthClient(
                resolveIntegrationAuthDefinitions(oauthClientProvider),
                oauthClientAuth
              )
            : []
        }
        onConfigured={
          continueAfterOAuthConfig && oauthClientProvider && oauthClientAuth
            ? () => {
                beginConnection(oauthClientProvider, {
                  ...oauthClientAuth,
                  oauth: oauthClientAuth.oauth
                    ? {
                        ...oauthClientAuth.oauth,
                        client_configured: true,
                        client_config_source: 'organization',
                      }
                    : oauthClientAuth.oauth,
                });
              }
            : undefined
        }
        onOpenChange={nextOpen => {
          if (!nextOpen) {
            setContinueAfterOAuthConfig(false);
            setOAuthClientAuth(null);
            setOAuthClientProvider(null);
          }
        }}
      />

      <IntegrationProviderCapabilitiesSheet
        provider={capabilityProvider}
        open={Boolean(capabilityProvider)}
        onOpenChange={nextOpen => {
          if (!nextOpen) setCapabilityProvider(null);
        }}
        onConnect={provider => {
          const methods = resolveIntegrationAuthDefinitions(provider).filter(auth => {
            const source = integrationAuthCredentialSource(auth);
            return (
              auth.available &&
              (source === 'account' || (canManageShared && source === 'organization'))
            );
          });
          const auth = selectPrimaryAuthMethod(methods, canManageShared);
          if (auth) openAuthMethod(provider, auth);
        }}
      />
    </div>
  );
}
