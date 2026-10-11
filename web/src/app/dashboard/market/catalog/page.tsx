'use client';

import { useMemo, useState } from 'react';
import { Search } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import MarketEmptyState from '@/components/market/market-empty-state';
import ZgiMarketCard from '@/components/zgi-market/zgi-market-card';
import { marketText } from '@/components/zgi-market/market-format';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useLocale } from '@/hooks/use-locale';
import {
  useZgiMarketCategories,
  useZgiMarketListings,
  useZgiMarketSubscriptions,
} from '@/hooks/use-zgi-market';
import { useT } from '@/i18n';
import {
  marketListingKinds,
  type MarketCategory,
  type MarketListingKind,
} from '@/services/types/zgi-market';

const pageSize = 24;
const allCategories = 'all';

function flatten(categories: MarketCategory[]): MarketCategory[] {
  return categories.flatMap(category => [category, ...flatten(category.children ?? [])]);
}

export default function ZgiMarketCatalogPage() {
  const t = useT();
  const { locale } = useLocale();
  const [kind, setKind] = useState<MarketListingKind | ''>('');
  const [category, setCategory] = useState(allCategories);
  const [keyword, setKeyword] = useState('');
  const [page, setPage] = useState(1);
  const search = useDebouncedValue(keyword.trim(), 300);

  const categories = useZgiMarketCategories(kind || undefined);
  const listings = useZgiMarketListings({
    page,
    per_page: pageSize,
    ...(kind ? { kind } : {}),
    ...(category !== allCategories ? { category } : {}),
    ...(search ? { q: search } : {}),
  });
  const subscriptions = useZgiMarketSubscriptions();
  const activated = useMemo(
    () =>
      new Set(
        (subscriptions.data ?? [])
          .filter(subscription => subscription.status === 'active')
          .map(subscription => subscription.listing_id)
      ),
    [subscriptions.data]
  );
  const options = useMemo(() => flatten(categories.data ?? []), [categories.data]);
  const items = listings.data?.data ?? [];
  const meta = listings.data?.meta;
  const reset = () => setPage(1);

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <header className="flex flex-col gap-4 border-b px-6 py-5">
        <div>
          <h1 className="text-xl font-semibold">{t('market.catalog.title')}</h1>
          <p className="mt-1 text-sm text-muted-foreground">{t('market.catalog.description')}</p>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <Tabs
            value={kind || 'all'}
            onValueChange={value => {
              setKind(value === 'all' ? '' : (value as MarketListingKind));
              setCategory(allCategories);
              reset();
            }}
          >
            <TabsList>
              <TabsTrigger value="all">{t('market.catalog.allKinds')}</TabsTrigger>
              {marketListingKinds.map(value => (
                <TabsTrigger key={value} value={value}>
                  {t(`market.catalog.kinds.${value}`)}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
          <Select
            value={category}
            onValueChange={value => {
              setCategory(value);
              reset();
            }}
          >
            <SelectTrigger className="w-44" aria-label={t('market.catalog.category')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={allCategories}>{t('market.catalog.allCategories')}</SelectItem>
              {options.map(option => (
                <SelectItem key={option.slug} value={option.slug}>
                  {marketText(option.name, locale)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <div className="relative ml-auto w-full max-w-xs">
            <Search
              className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
              aria-hidden="true"
            />
            <Input
              className="pl-9"
              value={keyword}
              maxLength={100}
              placeholder={t('market.catalog.searchPlaceholder')}
              aria-label={t('market.catalog.searchPlaceholder')}
              onChange={event => {
                setKeyword(event.target.value);
                reset();
              }}
            />
          </div>
        </div>
      </header>

      <div className="flex-1 overflow-y-auto px-6 py-5">
        {listings.isPending ? (
          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
            {Array.from({ length: 8 }, (_, index) => (
              <Skeleton key={index} className="h-40 rounded-lg" />
            ))}
          </div>
        ) : listings.isError ? (
          <MarketEmptyState
            title={t('market.catalog.loadFailed')}
            actionLabel={t('market.catalog.retry')}
            onAction={() => void listings.refetch()}
          />
        ) : items.length === 0 ? (
          <MarketEmptyState title={t('market.catalog.empty')} />
        ) : (
          <>
            <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4">
              {items.map(listing => (
                <ZgiMarketCard
                  key={listing.id}
                  listing={listing}
                  locale={locale}
                  activated={activated.has(listing.id)}
                />
              ))}
            </div>
            {meta ? (
              <div className="mt-6 flex items-center justify-between gap-3 text-sm text-muted-foreground">
                <span>{t('market.catalog.total', { count: meta.total })}</span>
                {meta.last_page > 1 ? (
                  <div className="flex items-center gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={meta.current_page <= 1}
                      onClick={() => setPage(meta.current_page - 1)}
                    >
                      {t('market.catalog.previous')}
                    </Button>
                    <span className="tabular-nums">
                      {meta.current_page} / {meta.last_page}
                    </span>
                    <Button
                      variant="outline"
                      size="sm"
                      disabled={meta.current_page >= meta.last_page}
                      onClick={() => setPage(meta.current_page + 1)}
                    >
                      {t('market.catalog.next')}
                    </Button>
                  </div>
                ) : null}
              </div>
            ) : null}
          </>
        )}
      </div>
    </div>
  );
}
