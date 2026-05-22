'use client';

import { useState, useCallback, useEffect, useRef } from 'react';
import { Search } from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
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
    sort: 'downloads',
  });

  const lastProcessedPage = useRef(0);

  // Reset data when search or type changes
  useEffect(() => {
    setAllPlugins([]);
    setCurrentPage(1);
    lastProcessedPage.current = 0;
  }, [debouncedSearchKeyword, selectedType, locale]);

  useEffect(() => {
    if (pagePlugins.length > 0 && currentPage !== lastProcessedPage.current) {
      lastProcessedPage.current = currentPage;

      setAllPlugins(prev => {
        if (currentPage === 1) {
          return pagePlugins;
        }

        const newArray = [...prev, ...pagePlugins];
        return newArray;
      });

      setTotalCount(total);
    }
  }, [pagePlugins, total, currentPage, isLoading, isFetching]);

  const loadNextPage = useCallback(async () => {
    if (!isFetching && allPlugins.length < totalCount) {
      setCurrentPage(prev => prev + 1);
    }
  }, [isFetching, allPlugins.length, totalCount]);

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
    <div className="p-4 sm:p-6 lg:p-8 space-y-6 flex flex-col h-full overflow-y-auto bg-background">
      {/* Header */}
      <div className="space-y-2">
        <h1 className="text-2xl sm:text-3xl font-bold tracking-tight">
          {t('market.plugins.title')}
        </h1>
        <p className="text-sm text-muted-foreground">{t('market.plugins.description')}</p>
      </div>

      {/* Category Tabs */}
      <Tabs value={selectedType} onValueChange={value => setSelectedType(value as PluginCategory)}>
        <TabsList className="w-full justify-start overflow-x-auto flex-wrap h-auto">
          {PLUGIN_CATEGORIES.map(category => (
            <TabsTrigger key={category.value || 'all'} value={category.value} className="gap-2">
              {t(
                `market.plugins.categories.${category.label as 'all' | 'tool' | 'extension' | 'integration'}`
              )}
            </TabsTrigger>
          ))}
        </TabsList>
      </Tabs>

      {/* Search Bar */}
      <div className="flex items-center justify-end">
        <div className="relative w-full sm:max-w-md">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder={t('market.plugins.searchPlaceholder')}
            value={searchKeyword}
            onChange={e => setSearchKeyword(e.target.value)}
            className="pl-9 bg-background rounded-lg text-sm w-full"
          />
        </div>
      </div>

      {/* Error State */}
      {error && (
        <div className="flex flex-col items-center justify-center py-12 text-center">
          <p className="text-destructive mb-4">{error}</p>
        </div>
      )}

      {/* Loading State - Only show skeleton on initial load */}
      {isLoading && allPlugins.length === 0 && (
        <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-4 sm:gap-6">
          {Array.from({ length: 12 }).map((_, idx) => (
            <Skeleton key={idx} className="h-40 w-full" />
          ))}
        </div>
      )}

      {/* Empty State */}
      {!isLoading && allPlugins.length === 0 && (
        <div className="flex flex-col items-center justify-center py-12 text-center">
          <Search className="h-12 w-12 text-muted-foreground mb-4" />
          <h3 className="text-lg font-medium mb-2">{t('market.plugins.noResults')}</h3>
          <p className="text-muted-foreground mb-4 max-w-sm">
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
        <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-4 sm:gap-6">
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
        <div ref={sentinelRef} className="flex justify-center py-8">
          {isFetching && (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <div className="w-4 h-4 border-2 border-primary border-t-transparent rounded-full animate-spin" />
              {t('market.plugins.loading')}
            </div>
          )}
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
