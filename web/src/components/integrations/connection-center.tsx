'use client';

import { useEffect, useMemo, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { ArrowLeft, FileClock, LockKeyhole, PlugZap, Settings2 } from 'lucide-react';
import { IntegrationConnectionsPanel } from '@/components/integrations/connections-panel';
import { IntegrationExecutionsPanel } from '@/components/integrations/executions-panel';
import { IntegrationOAuthRecoveryPanel } from '@/components/integrations/oauth-recovery-panel';
import { IntegrationProviderCatalog } from '@/components/integrations/provider-catalog';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { SearchInput } from '@/components/ui/input';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  integrationCatalogItems,
  integrationConnectionItems,
  useAllIntegrationConnections,
  useAllMyIntegrationConnections,
  useIntegrationCatalog,
  useIntegrationOAuthRecovery,
} from '@/hooks';
import { INTEGRATION_KEYS } from '@/hooks/query-keys';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { useT } from '@/i18n';
import { integrationService } from '@/services/integration.service';
import { useIntegrationMetadata } from './metadata-i18n';

export type IntegrationConnectionCenterView = 'available' | 'connected' | 'policies' | 'executions';

function isConnectionCenterView(value: string | null): value is IntegrationConnectionCenterView {
  return ['available', 'connected', 'policies', 'executions'].includes(value ?? '');
}

interface IntegrationConnectionCenterProps {
  enabled: boolean;
  featureLoading?: boolean;
  initialView?: IntegrationConnectionCenterView;
  initialIntegrationId?: string;
}

export function IntegrationConnectionCenter({
  enabled,
  featureLoading = false,
  initialView = 'available',
  initialIntegrationId,
}: IntegrationConnectionCenterProps) {
  const t = useT('integrations');
  const metadata = useIntegrationMetadata();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const requestedView = searchParams.get('view');
  const requestedCategory = searchParams.get('category');
  const [view, setView] = useState<IntegrationConnectionCenterView>(
    isConnectionCenterView(requestedView) ? requestedView : initialView
  );
  const [query, setQuery] = useState('');
  const [category, setCategory] = useState(requestedCategory || 'all');
  const { currentOrganization } = useOrganizations(true);
  const canManageShared = ['owner', 'admin'].includes(currentOrganization?.organization_role ?? '');
  const catalogQuery = useIntegrationCatalog(enabled, canManageShared ? 'organization' : 'account');
  const managedConnectionsQuery = useAllIntegrationConnections(
    undefined,
    enabled && canManageShared
  );
  const availableConnectionsQuery = useQuery({
    queryKey: INTEGRATION_KEYS.availableConnections({ all: true }),
    queryFn: () => integrationService.getAllAvailableConnections(),
    enabled: enabled && !canManageShared,
    staleTime: 30_000,
    retry: false,
  });
  const myConnectionsQuery = useAllMyIntegrationConnections(undefined, enabled && canManageShared);
  const oauthRecoveryQuery = useIntegrationOAuthRecovery(enabled && canManageShared);
  const oauthRecovery = oauthRecoveryQuery.data?.data;
  const catalog = integrationCatalogItems(catalogQuery.data?.data);
  const managedConnections = useMemo(() => {
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

  useEffect(() => {
    setView(isConnectionCenterView(requestedView) ? requestedView : initialView);
  }, [initialView, requestedView]);
  useEffect(() => {
    setCategory(requestedCategory || 'all');
  }, [requestedCategory]);

  const categories = useMemo(() => {
    const values = new Map<string, { value: string; label: string }>();
    for (const provider of catalog) {
      for (const value of provider.categories ?? (provider.category ? [provider.category] : [])) {
        if (!value || values.has(value)) continue;
        values.set(value, { value, label: metadata.category(value, provider) });
      }
    }
    return [...values.values()];
  }, [catalog, metadata]);

  const connectedProviderCount = useMemo(
    () =>
      new Set(
        managedConnections
          .filter(connection => connection.status !== 'disabled')
          .map(connection => connection.integration_id)
      ).size,
    [managedConnections]
  );

  const visibleView: IntegrationConnectionCenterView =
    view === 'policies' || (!canManageShared && view === 'executions') ? 'connected' : view;
  const showDiscoveryControls = visibleView === 'available' || visibleView === 'connected';
  const advancedView = visibleView === 'executions';

  const updateRouteState = (nextView: IntegrationConnectionCenterView, nextCategory = category) => {
    setView(nextView);
    const params = new URLSearchParams(searchParams.toString());
    params.delete('tab');
    params.set('view', nextView);
    if (nextCategory === 'all') params.delete('category');
    else params.set('category', nextCategory);
    const nextQuery = params.toString();
    router.replace(nextQuery ? `${pathname}?${nextQuery}` : pathname, { scroll: false });
  };

  const updateCategory = (nextCategory: string) => {
    setCategory(nextCategory);
    const params = new URLSearchParams(searchParams.toString());
    params.set('view', visibleView);
    if (nextCategory === 'all') params.delete('category');
    else params.set('category', nextCategory);
    const nextQuery = params.toString();
    router.replace(nextQuery ? `${pathname}?${nextQuery}` : pathname, { scroll: false });
  };

  const clearIntegrationFilter = () => {
    const params = new URLSearchParams(searchParams.toString());
    params.delete('integration_id');
    const nextQuery = params.toString();
    router.replace(nextQuery ? `${pathname}?${nextQuery}` : pathname, { scroll: false });
  };

  if (!enabled) {
    return (
      <div className="h-full overflow-y-auto bg-bg-canvas/50 p-4 lg:p-6">
        <div className="mx-auto w-full max-w-[1240px]">
          <div className="rounded-xl border bg-background p-10 text-center shadow-sm">
            <PlugZap className="mx-auto size-9 text-muted-foreground" />
            <h1 className="mt-4 text-xl font-semibold">{t('connectionCenter.title')}</h1>
            <p className="mx-auto mt-2 max-w-xl text-sm leading-6 text-muted-foreground">
              {featureLoading ? t('connectionCenter.description') : t('disabledDescription')}
            </p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="h-full overflow-y-auto bg-bg-canvas/50">
      <main className="mx-auto flex min-h-full w-full max-w-[1240px] flex-col px-4 py-6 sm:px-6 lg:px-8 lg:py-8">
        <header className="flex flex-col gap-5">
          <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
            <div className="min-w-0">
              <h1 className="text-2xl font-semibold tracking-tight">
                {t('connectionCenter.title')}
              </h1>
              <p className="mt-1.5 max-w-3xl text-sm leading-6 text-muted-foreground">
                {t('connectionCenter.description')}
              </p>
            </div>

            {canManageShared ? (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="outline" className="w-full sm:w-auto">
                    <Settings2 className="size-4" />
                    {t('connectionCenter.advanced.label')}
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end" className="w-52">
                  <DropdownMenuItem onClick={() => updateRouteState('executions')}>
                    <FileClock className="mr-2 size-4" />
                    {t('connectionCenter.advanced.executions')}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            ) : null}
          </div>

          {showDiscoveryControls ? (
            <>
              <SearchInput
                value={query}
                onChange={event => setQuery(event.target.value)}
                placeholder={t('connectionCenter.searchPlaceholder')}
                aria-label={t('connectionCenter.searchAria')}
                className="h-10 w-full bg-background"
              />

              <div className="flex flex-col gap-4 border-b sm:flex-row sm:items-end sm:justify-between">
                <Tabs
                  value={visibleView}
                  onValueChange={value =>
                    updateRouteState(value as IntegrationConnectionCenterView)
                  }
                >
                  <TabsList className="h-11 gap-5 rounded-none border-0 bg-transparent p-0 shadow-none">
                    <TabsTrigger
                      value="available"
                      className="relative h-11 gap-2 rounded-none border-0 bg-transparent px-1 text-muted-foreground shadow-none after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 after:scale-x-0 after:rounded-full after:bg-primary after:transition-transform data-[state=active]:border-0 data-[state=active]:bg-transparent data-[state=active]:text-primary data-[state=active]:shadow-none data-[state=active]:after:scale-x-100"
                    >
                      {t('connectionCenter.tabs.available')}
                      <span className="rounded-full bg-muted px-2 py-0.5 text-xs tabular-nums">
                        {catalog.length}
                      </span>
                    </TabsTrigger>
                    <TabsTrigger
                      value="connected"
                      className="relative h-11 gap-2 rounded-none border-0 bg-transparent px-1 text-muted-foreground shadow-none after:absolute after:inset-x-0 after:bottom-0 after:h-0.5 after:scale-x-0 after:rounded-full after:bg-primary after:transition-transform data-[state=active]:border-0 data-[state=active]:bg-transparent data-[state=active]:text-primary data-[state=active]:shadow-none data-[state=active]:after:scale-x-100"
                    >
                      {t('connectionCenter.tabs.connected')}
                      <span className="rounded-full bg-muted px-2 py-0.5 text-xs tabular-nums">
                        {connectedProviderCount}
                      </span>
                    </TabsTrigger>
                  </TabsList>
                </Tabs>
              </div>

              {categories.length > 0 ? (
                <div className="flex gap-2 overflow-x-auto pb-1" aria-label={t('catalog.title')}>
                  <Button
                    size="sm"
                    variant={category === 'all' ? 'default' : 'outline'}
                    onClick={() => updateCategory('all')}
                    className="rounded-full"
                  >
                    {t('connectionCenter.categories.all')}
                  </Button>
                  {categories.map(item => (
                    <Button
                      key={item.value}
                      size="sm"
                      variant={category === item.value ? 'default' : 'outline'}
                      onClick={() => updateCategory(item.value)}
                      className="rounded-full"
                    >
                      {item.label}
                    </Button>
                  ))}
                </div>
              ) : null}
            </>
          ) : null}
        </header>

        <section className="mt-6 flex-1">
          {canManageShared && oauthRecovery && oauthRecovery.unresolved_dead_letters > 0 ? (
            <IntegrationOAuthRecoveryPanel recovery={oauthRecovery} />
          ) : null}

          {visibleView === 'available' ? (
            <IntegrationProviderCatalog
              query={query}
              category={category}
              canManageShared={canManageShared}
              integrationId={initialIntegrationId}
              onClearIntegrationId={clearIntegrationFilter}
            />
          ) : null}
          {visibleView === 'connected' ? (
            <IntegrationConnectionsPanel
              query={query}
              category={category}
              integrationId={initialIntegrationId}
              canManageShared={canManageShared}
              onBrowseAvailable={() => updateRouteState('available')}
            />
          ) : null}
          {advancedView ? (
            <div className="space-y-5">
              <div className="flex items-center justify-between gap-3">
                <Button variant="ghost" onClick={() => updateRouteState('connected')}>
                  <ArrowLeft className="size-4" />
                  {t('connectionCenter.tabs.connected')}
                </Button>
                <div className="flex items-center gap-2 text-xs text-muted-foreground">
                  <LockKeyhole className="size-3.5" />
                  {t('connectionCenter.connected.securityNote')}
                </div>
              </div>
              {visibleView === 'executions' ? <IntegrationExecutionsPanel /> : null}
            </div>
          ) : null}
        </section>

        {showDiscoveryControls ? (
          <footer className="mt-8 flex items-center gap-2 border-t py-5 text-xs leading-5 text-muted-foreground">
            <LockKeyhole className="size-3.5 shrink-0" />
            <span>{t('connectionCenter.connected.securityNote')}</span>
          </footer>
        ) : null}
      </main>
    </div>
  );
}
