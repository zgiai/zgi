'use client';

import { mailDisplayGroup } from '../mail-display-model';
import { useCallback, useDeferredValue, useEffect, useMemo, useState } from 'react';
import { usePendingSetups } from '@/hooks/connectors/use-pending-setups';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  connectorCatalogItems,
  personalConnectionItems,
  useConnectorCatalog,
  usePersonalConnections,
} from '@/hooks/connectors/use-connectors';
import { useT } from '@/i18n';
import type {
  ConnectorCatalogStatus,
  ConnectorCategory,
  ConnectorTag,
} from '@/services/types/connector';
import { CONNECTOR_CATEGORIES, CONNECTOR_TAGS } from '@/components/connectors/connector-metadata';
import { filterConnectorCatalog, partitionConnectorCatalog } from './catalog-model';
import { ConnectorCatalogSection } from './connector-catalog-section';
import { ZgiMarketConnectorCenterSection } from '@/components/zgi-market/zgi-market-connector-center-section';

function catalogStatus(value: string | null): ConnectorCatalogStatus {
  return value === 'connected' || value === 'needs_attention' ? value : 'all';
}

function catalogCategory(value: string | null): ConnectorCategory | undefined {
  return value && CONNECTOR_CATEGORIES.includes(value as ConnectorCategory)
    ? (value as ConnectorCategory)
    : undefined;
}

function catalogTag(value: string | null): ConnectorTag | undefined {
  return value && CONNECTOR_TAGS.includes(value as ConnectorTag)
    ? (value as ConnectorTag)
    : undefined;
}

/** The URL carries the query for sharing; the input itself must never wait on it. */
const SEARCH_URL_SYNC_DELAY_MS = 300;

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
  const queryFromUrl = searchParams.get('q') ?? '';
  const [searchText, setSearchText] = useState(queryFromUrl);
  const debouncedSearchText = useDebouncedValue(searchText, SEARCH_URL_SYNC_DELAY_MS);
  const deferredQuery = useDeferredValue(searchText);
  const status = catalogStatus(searchParams.get('status'));
  const category = catalogCategory(searchParams.get('category'));
  const tag = catalogTag(searchParams.get('tag'));
  const catalogQuery = useConnectorCatalog(undefined, enabled);
  const pendingQuery = usePendingSetups('feishu', enabled);
  const connectionsQuery = usePersonalConnections(undefined, enabled);
  const connectionNames = useMemo(() => {
    const names = new Map<string, string[]>();
    for (const connection of personalConnectionItems(connectionsQuery.data?.data)) {
      const values = names.get(connection.connector_id) ?? [];
      if (connection.name) values.push(connection.name);
      if (connection.external_display_name) values.push(connection.external_display_name);
      names.set(connection.connector_id, values);
    }
    return names;
  }, [connectionsQuery.data]);
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
    { query: deferredQuery, status, category, tag, connectionNames },
    new Set((pendingQuery.data?.data.items ?? []).map(item => item.connector_id))
  );
  const partitioned = partitionConnectorCatalog(filteredItems);

  const updateParams = useCallback(
    (values: {
      q?: string;
      status?: ConnectorCatalogStatus;
      category?: ConnectorCategory;
      tag?: ConnectorTag;
    }) => {
      const params = new URLSearchParams(searchParams.toString());
      if (values.q !== undefined) {
        if (values.q) params.set('q', values.q);
        else params.delete('q');
      }
      if (values.status !== undefined) {
        if (values.status === 'all') params.delete('status');
        else params.set('status', values.status);
      }
      if ('category' in values) {
        if (values.category) params.set('category', values.category);
        else params.delete('category');
      }
      if ('tag' in values) {
        if (values.tag) params.set('tag', values.tag);
        else params.delete('tag');
      }
      const next = params.toString();
      router.replace(next ? `${pathname}?${next}` : pathname, { scroll: false });
    },
    [pathname, router, searchParams]
  );

  const resetFilters = useCallback(() => {
    const params = new URLSearchParams(searchParams.toString());
    params.delete('q');
    params.delete('status');
    params.delete('category');
    params.delete('tag');
    const next = params.toString();
    setSearchText('');
    router.replace(next ? `${pathname}?${next}` : pathname, { scroll: false });
  }, [pathname, router, searchParams]);

  // Adopt URL changes that did not come from the search box, such as back/forward or a shared link.
  useEffect(() => {
    setSearchText(queryFromUrl);
  }, [queryFromUrl]);

  // Persist the settled text so the URL stays shareable without driving the input value.
  useEffect(() => {
    const nextQuery = debouncedSearchText.trim();
    if (nextQuery !== searchText.trim()) return;
    if (nextQuery === queryFromUrl) return;
    updateParams({ q: nextQuery });
  }, [debouncedSearchText, queryFromUrl, searchText, updateParams]);

  if (!enabled) {
    return (
      <div className="h-full overflow-y-auto bg-bg-canvas/50 p-4 lg:p-6">
        <div className="mx-auto w-full max-w-[1240px]">
          <ConnectorPageMessage
            kind="disabled"
            title={t('title')}
            description={featureLoading ? t('catalog.loading') : t('disabledDescription')}
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
            value={searchText}
            onChange={event => setSearchText(event.target.value)}
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
          <div className="flex flex-col gap-2 sm:flex-row">
            <Select
              value={category ?? 'all'}
              onValueChange={value =>
                updateParams({
                  category: value === 'all' ? undefined : (value as ConnectorCategory),
                })
              }
            >
              <SelectTrigger className="w-full sm:w-56" aria-label={t('filters.category')}>
                <SelectValue placeholder={t('filters.category')} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t('filters.allCategories')}</SelectItem>
                {CONNECTOR_CATEGORIES.map(value => (
                  <SelectItem key={value} value={value}>
                    {t(`categories.${value}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Select
              value={tag ?? 'all'}
              onValueChange={value =>
                updateParams({ tag: value === 'all' ? undefined : (value as ConnectorTag) })
              }
            >
              <SelectTrigger className="w-full sm:w-56" aria-label={t('filters.tag')}>
                <SelectValue placeholder={t('filters.tag')} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t('filters.allTags')}</SelectItem>
                {CONNECTOR_TAGS.map(value => (
                  <SelectItem key={value} value={value}>
                    {t(`tags.${value}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
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
          <div className="mt-8">
            <ZgiMarketConnectorCenterSection />
          </div>
        </div>
      </main>
    </div>
  );
}
