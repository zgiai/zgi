'use client';

import { useState, useCallback, useEffect, useRef } from 'react';
import { Search } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useT } from '@/i18n';
import { useMarketplacePlugins } from '@/hooks/use-plugins';
import PluginCard from '@/components/market/plugin-card';
import PluginDetailModal from '@/components/market/plugin-detail-modal';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import type { MarketplacePlugin, MarketplacePluginCategory } from '@/services/types/plugin';
import { useInfiniteObserver } from '@/hooks/use-infinite-observer';
import { Button } from '@/components/ui/button';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useLocale } from '@/hooks/use-locale';

type PluginCategory = '' | MarketplacePluginCategory;
type PluginSort = 'downloads' | 'newest' | 'rating';

const PLUGIN_CATEGORIES: Array<{ value: PluginCategory; label: string }> = [
  { value: '', label: 'all' },
  { value: 'tool', label: 'tool' },
  { value: 'extension', label: 'extension' },
  { value: 'integration', label: 'integration' },
];

export default function PluginsPage() {
  const t = useT();
  const { locale } = useLocale();
  const [searchKeyword, setSearchKeyword] = useState('');
  const [selectedPlugin, setSelectedPlugin] = useState<MarketplacePlugin | null>(null);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [currentPage, setCurrentPage] = useState(1);
  const [allPlugins, setAllPlugins] = useState<MarketplacePlugin[]>([]);
  const [totalCount, setTotalCount] = useState(0);
  const [selectedType, setSelectedType] = useState<PluginCategory>('');
  const [selectedSort, setSelectedSort] = useState<PluginSort>('downloads');
  const debouncedSearchKeyword = useDebouncedValue(searchKeyword, 500);

  const {
    plugins: pagePlugins,
    total,
    isLoading,
    isFetching,
    error,
  } = useMarketplacePlugins({
    page: currentPage,
    page_size: 20,
    category: selectedType || undefined,
    search: debouncedSearchKeyword || undefined,
    locale,
    sort: selectedSort,
  });

  const lastProcessedPage = useRef(0);

  // Reset data when search or type changes
  useEffect(() => {
    setAllPlugins([]);
    setCurrentPage(1);
    lastProcessedPage.current = 0;
  }, [debouncedSearchKeyword, selectedType, selectedSort, locale]);

  useEffect(() => {
    if (currentPage === lastProcessedPage.current || isLoading) {
      return;
    }

    lastProcessedPage.current = currentPage;
    setTotalCount(total);

    setAllPlugins(prev => {
      if (currentPage === 1) {
        return pagePlugins;
      }

      const existingIds = new Set(prev.map(plugin => plugin.id));
      const nextPlugins = pagePlugins.filter(plugin => !existingIds.has(plugin.id));
      return [...prev, ...nextPlugins];
    });
  }, [pagePlugins, total, currentPage, isLoading]);

  useEffect(() => {
    if (currentPage !== 1 || isFetching || isLoading) {
      return;
    }
    setTotalCount(total);
  }, [total, currentPage, isFetching, isLoading]);

  const loadNextPage = useCallback(async () => {
    if (!isFetching && !isLoading && allPlugins.length < totalCount) {
      setCurrentPage(prev => prev + 1);
    }
  }, [isFetching, isLoading, allPlugins.length, totalCount]);

  const hasNextPage = allPlugins.length < totalCount && totalCount > 0;

  const sentinelRef = useInfiniteObserver({
    hasNextPage,
    isFetchingNextPage: isFetching,
    fetchNextPage: loadNextPage,
    rootMargin: '200px',
  });

  const handlePluginCardClick = useCallback((plugin: MarketplacePlugin) => {
    setSelectedPlugin(plugin);
    setIsModalOpen(true);
  }, []);

  return (
    <div className="h-full overflow-y-auto bg-background">
      <div className="mx-auto flex w-full max-w-[1680px] flex-col gap-6 px-4 py-6 sm:px-6 lg:px-8">
        <div className="grid gap-4 lg:grid-cols-[minmax(180px,1fr)_minmax(320px,520px)_minmax(180px,1fr)] lg:items-center">
          <div className="space-y-1">
            <h1 className="text-2xl font-semibold tracking-tight sm:text-3xl">
              {t('market.plugins.title')}
            </h1>
            <p className="text-sm text-muted-foreground">{t('market.plugins.description')}</p>
          </div>

          <div className="relative w-full">
            <Search className="absolute left-4 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder={t('market.plugins.searchPlaceholder')}
              value={searchKeyword}
              onChange={e => setSearchKeyword(e.target.value)}
              className="h-11 w-full rounded-xl bg-background pl-11 text-sm shadow-sm"
            />
          </div>

          <div className="hidden lg:block" />
        </div>

        <Tabs
          value={selectedType}
          onValueChange={value => setSelectedType(value as PluginCategory)}
        >
          <TabsList className="h-auto w-full justify-start gap-2 overflow-x-auto border-0 bg-transparent p-0">
            {PLUGIN_CATEGORIES.map(category => (
              <TabsTrigger
                key={category.value || 'all'}
                value={category.value}
                className="rounded-lg border border-transparent px-4 py-2 data-[state=active]:border-primary/20 data-[state=active]:bg-primary/10 data-[state=active]:text-primary data-[state=active]:shadow-none"
              >
                {t(
                  `market.plugins.categories.${category.label as 'all' | 'tool' | 'extension' | 'integration'}`
                )}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>

        <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="text-sm text-muted-foreground">
            {t('market.plugins.resultCount', { count: totalCount })}
          </div>
          <Select
            value={selectedSort}
            onValueChange={value => setSelectedSort(value as PluginSort)}
          >
            <SelectTrigger className="h-9 w-full rounded-lg sm:w-[160px]">
              <SelectValue placeholder={t('market.plugins.sort.label')} />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="downloads">{t('market.plugins.sort.downloads')}</SelectItem>
              <SelectItem value="newest">{t('market.plugins.sort.newest')}</SelectItem>
              <SelectItem value="rating">{t('market.plugins.sort.rating')}</SelectItem>
            </SelectContent>
          </Select>
        </div>

        {/* Error State */}
        {error && (
          <div className="flex flex-col items-center justify-center rounded-xl border border-destructive/30 py-12 text-center">
            <p className="mb-4 text-destructive">{error}</p>
          </div>
        )}

        {/* Loading State - Only show skeleton on initial load */}
        {isLoading && allPlugins.length === 0 && (
          <div className="grid grid-cols-1 gap-4 xl:grid-cols-2 2xl:grid-cols-3">
            {Array.from({ length: 12 }).map((_, idx) => (
              <Skeleton key={idx} className="h-44 w-full rounded-xl" />
            ))}
          </div>
        )}

        {/* Empty State */}
        {!isLoading && allPlugins.length === 0 && (
          <div className="flex flex-col items-center justify-center rounded-xl border py-16 text-center">
            <Search className="mb-4 h-12 w-12 text-muted-foreground" />
            <h3 className="mb-2 text-lg font-medium">{t('market.plugins.noResults')}</h3>
            <p className="mb-4 max-w-sm text-muted-foreground">
              {debouncedSearchKeyword
                ? t('market.plugins.noResultsDescription', { keyword: debouncedSearchKeyword })
                : t('market.plugins.noPluginsDescription')}
            </p>
            {debouncedSearchKeyword && (
              <Button variant="outline" onClick={() => setSearchKeyword('')}>
                {t('market.plugins.clearSearch')}
              </Button>
            )}
          </div>
        )}

        {allPlugins.length > 0 && (
          <div className="grid grid-cols-1 gap-4 xl:grid-cols-2 2xl:grid-cols-3">
            {allPlugins.map(plugin => (
              <PluginCard
                key={plugin.id}
                plugin={plugin}
                onClick={() => handlePluginCardClick(plugin)}
              />
            ))}
          </div>
        )}

        {/* Infinite Scroll Sentinel */}
        {allPlugins.length > 0 && hasNextPage && (
          <div ref={sentinelRef} className="flex min-h-16 justify-center py-8">
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              {isFetching ? (
                <>
                  <div className="h-4 w-4 animate-spin rounded-full border-2 border-primary border-t-transparent" />
                  {t('market.plugins.loading')}
                </>
              ) : (
                t('market.plugins.scrollHint')
              )}
            </div>
          </div>
        )}

        {/* No More Data Indicator */}
        {allPlugins.length > 0 && !hasNextPage && (
          <div className="flex justify-center py-8">
            <div className="text-sm text-muted-foreground">
              {t('market.plugins.noMoreData') || 'No more plugins'}
            </div>
          </div>
        )}
      </div>

      {isModalOpen && (
        <PluginDetailModal
          open={isModalOpen}
          onOpenChange={setIsModalOpen}
          pluginId={selectedPlugin?.id || null}
        />
      )}
    </div>
  );
}
