'use client';

import { mailDisplayGroup } from '../mail-display-model';
import { useDeferredValue } from 'react';
import { usePendingSetups } from '@/hooks/connectors/use-pending-setups';
import Link from 'next/link';
import { useLocale } from 'next-intl';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { SlidersHorizontal } from 'lucide-react';
import { localizeConnectorCatalogItem } from '@/components/connectors/connector-metadata';
import { isConnectorListed } from '@/components/connectors/connector-visibility';
import {
  ConnectorPageLoading,
  ConnectorPageMessage,
} from '@/components/connectors/connector-page-state';
import { Button } from '@/components/ui/button';
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb';
import { SearchInput } from '@/components/ui/input';
import { connectorCatalogItems, useConnectorCatalog } from '@/hooks/connectors/use-connectors';
import { useT } from '@/i18n';
import type { ConnectorCatalogStatus } from '@/services/types/connector';
import { filterConnectorCatalog, partitionConnectorCatalog } from './catalog-model';
import { ConnectorCatalogSection } from './connector-catalog-section';

function catalogStatus(value: string | null): ConnectorCatalogStatus {
  return value === 'connected' || value === 'needs_attention' ? value : 'all';
}

interface ConnectorCenterProps {
  enabled: boolean;
  featureLoading?: boolean;
}

export function ConnectorCenter({ enabled, featureLoading = false }: ConnectorCenterProps) {
  const t = useT('connectors');
  const locale = useLocale();
  const tNavigation = useT('navigation');
  const pathname = usePathname();
  const router = useRouter();
  const searchParams = useSearchParams();
  const query = searchParams.get('q') ?? '';
  const deferredQuery = useDeferredValue(query);
  const status = catalogStatus(searchParams.get('status'));
  const catalogQuery = useConnectorCatalog(undefined, enabled);
  const pendingQuery = usePendingSetups('feishu', enabled);
  const items = connectorCatalogItems(catalogQuery.data?.data)
    .filter(item => isConnectorListed(item.id))
    .map(item =>
      localizeConnectorCatalogItem(item, locale, {
        description: t('metadata.descriptionUnavailable'),
        capabilityDescription: t('metadata.capabilityDescriptionUnavailable'),
      })
    );
  const filteredItems = filterConnectorCatalog(
    items,
    { query: deferredQuery, status },
    new Set((pendingQuery.data?.data.items ?? []).map(item => item.connector_id))
  );
  const partitioned = partitionConnectorCatalog(filteredItems);

  const updateParams = (values: { q?: string; status?: ConnectorCatalogStatus }) => {
    const params = new URLSearchParams(searchParams.toString());
    if (values.q !== undefined) {
      if (values.q) params.set('q', values.q);
      else params.delete('q');
    }
    if (values.status !== undefined) {
      if (values.status === 'all') params.delete('status');
      else params.set('status', values.status);
    }
    const next = params.toString();
    router.replace(next ? `${pathname}?${next}` : pathname, { scroll: false });
  };

  const resetFilters = () => {
    const params = new URLSearchParams(searchParams.toString());
    params.delete('q');
    params.delete('status');
    const next = params.toString();
    router.replace(next ? `${pathname}?${next}` : pathname, { scroll: false });
  };

  if (!enabled) {
    return (
      <div className="h-full overflow-y-auto bg-bg-canvas/50 p-4 lg:p-6">
        <div className="mx-auto w-full max-w-[1240px]">
          <ConnectorPageMessage
            kind="disabled"
            title={t('title')}
            description={featureLoading ? t('description') : t('disabledDescription')}
          />
        </div>
      </div>
    );
  }

  return (
    <div className="h-full overflow-y-auto bg-bg-canvas/50">
      <main className="mx-auto flex min-h-full w-full max-w-[1240px] flex-col px-4 py-6 sm:px-6 lg:px-8 lg:py-8">
        <Breadcrumb className="mb-5">
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink asChild>
                <Link href="/console/connectors">{tNavigation('tools')}</Link>
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage>{t('title')}</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>

        <header className="flex flex-col gap-5">
          <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
            <div className="min-w-0">
              <h1 className="text-2xl font-semibold tracking-tight">{t('title')}</h1>
            </div>
            <Button asChild variant="outline" className="w-full sm:w-auto">
              <Link href="/console/connectors/connections">
                <SlidersHorizontal aria-hidden="true" className="size-4" />
                {t('manageConnections')}
              </Link>
            </Button>
          </div>

          <SearchInput
            value={query}
            onChange={event => updateParams({ q: event.target.value })}
            placeholder={t('searchPlaceholder')}
            aria-label={t('searchAria')}
            className="h-10 w-full bg-background"
          />

          <div
            className="flex gap-2 overflow-x-auto pb-1"
            role="group"
            aria-label={t('filters.label')}
          >
            {(['all', 'connected', 'needs_attention'] as const).map(value => (
              <Button
                key={value}
                size="sm"
                variant={status === value ? 'default' : 'outline'}
                className="rounded-full"
                aria-pressed={status === value}
                onClick={() => updateParams({ status: value })}
              >
                {value === 'all'
                  ? t('filters.all')
                  : value === 'connected'
                    ? t('filters.connected')
                    : t('filters.needsAttention')}
              </Button>
            ))}
          </div>
        </header>

        <div className="mt-7 flex-1">
          {catalogQuery.isLoading ? <ConnectorPageLoading label={t('catalog.loading')} /> : null}
          {catalogQuery.isError ? (
            <ConnectorPageMessage
              kind="error"
              title={t('catalog.loadFailedTitle')}
              description={t('catalog.loadFailedDescription')}
              actionLabel={t('actions.retry')}
              onAction={() => void catalogQuery.refetch()}
            />
          ) : null}
          {!catalogQuery.isLoading && !catalogQuery.isError && filteredItems.length === 0 ? (
            <ConnectorPageMessage
              kind="empty"
              title={t('catalog.emptyTitle')}
              description={t('catalog.emptyDescription')}
              actionLabel={t('catalog.resetFilters')}
              onAction={resetFilters}
            />
          ) : null}
          {!catalogQuery.isLoading && !catalogQuery.isError && filteredItems.length > 0 ? (
            <div className="space-y-8">
              <ConnectorCatalogSection
                classification="connector"
                items={partitioned.connectors}
                mailGroup={mailDisplayGroup(items)}
              />
              <ConnectorCatalogSection
                classification="data_source"
                items={partitioned.dataSources}
              />
            </div>
          ) : null}
        </div>
      </main>
    </div>
  );
}
