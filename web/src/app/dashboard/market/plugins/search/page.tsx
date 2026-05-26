'use client';

import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from 'react';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { ChevronDown, Search, X } from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import MarketEmptyState from '@/components/market/market-empty-state';
import PluginCard from '@/components/market/plugin-card';
import { useInfiniteObserver } from '@/hooks/use-infinite-observer';
import { useLocale } from '@/hooks/use-locale';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import {
  useMarketplaceBranding,
  useMarketplaceCategories,
  useMarketplacePlugins,
} from '@/hooks/use-plugins';
import { useT } from '@/i18n';
import type { MarketplacePlugin, MarketplacePluginCategory } from '@/services/types/plugin';

type PluginSort = 'downloads' | 'newest';
type PluginSource = 'all' | 'official' | 'third_party';

function parseCategories(value: string | null) {
  return (value || '')
    .split(',')
    .map(item => item.trim())
    .filter(Boolean);
}

export default function PluginSearchPage() {
  const t = useT();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const { locale } = useLocale();
  const branding = useMarketplaceBranding();
  const { categories: marketplaceCategories } = useMarketplaceCategories(locale);

  const queryFromUrl = searchParams.get('q') || '';
  const sourceFromUrl = (searchParams.get('source') as PluginSource | null) || 'all';
  const sortFromUrl = (searchParams.get('sort') as PluginSort | null) || 'downloads';
  const categoriesFromUrl = useMemo(
    () => parseCategories(searchParams.get('categories')),
    [searchParams]
  );

  const [searchKeyword, setSearchKeyword] = useState(queryFromUrl);
  const [selectedSource, setSelectedSource] = useState<PluginSource>(
    sourceFromUrl === 'official' || sourceFromUrl === 'third_party' ? sourceFromUrl : 'all'
  );
  const [selectedSort, setSelectedSort] = useState<PluginSort>(
    sortFromUrl === 'newest' ? 'newest' : 'downloads'
  );
  const [selectedCategories, setSelectedCategories] = useState<string[]>(categoriesFromUrl);
  const [currentPage, setCurrentPage] = useState(1);
  const [allPlugins, setAllPlugins] = useState<MarketplacePlugin[]>([]);
  const [totalCount, setTotalCount] = useState(0);
  const debouncedSearchKeyword = useDebouncedValue(searchKeyword, 2000);
  const searchInputRef = useRef<HTMLInputElement>(null);

  const pluginCategories = useMemo(
    () =>
      marketplaceCategories.map(category => ({
        value: category.code,
        label: category.name || category.name_zh_hans || category.name_en_us || category.code,
      })),
    [marketplaceCategories]
  );

  const categoryLookup = useMemo(
    () => new Map(pluginCategories.map(category => [category.value, category.label])),
    [pluginCategories]
  );

  const sourceOfficial =
    selectedSource === 'official' ? true : selectedSource === 'third_party' ? false : undefined;
  const categoryParam =
    selectedCategories.length > 0
      ? (selectedCategories.join(',') as MarketplacePluginCategory)
      : undefined;

  const {
    plugins: pagePlugins,
    total,
    isLoading,
    isFetching,
    error,
  } = useMarketplacePlugins({
    page: currentPage,
    page_size: 20,
    category: categoryParam,
    search: queryFromUrl || undefined,
    locale,
    sort: selectedSort,
    is_official: sourceOfficial,
  });

  const lastProcessedPage = useRef(0);

  const replaceSearchParams = useCallback(
    (updates: Record<string, string | undefined>) => {
      const nextSearchParams = new URLSearchParams(searchParams.toString());
      Object.entries(updates).forEach(([key, value]) => {
        if (value) {
          nextSearchParams.set(key, value);
        } else {
          nextSearchParams.delete(key);
        }
      });
      const nextQuery = nextSearchParams.toString();
      router.replace(nextQuery ? `${pathname}?${nextQuery}` : pathname, { scroll: false });
    },
    [pathname, router, searchParams]
  );

  useEffect(() => {
    setSearchKeyword(queryFromUrl);
  }, [queryFromUrl]);

  useEffect(() => {
    const nextQuery = debouncedSearchKeyword.trim();
    if (nextQuery !== searchKeyword.trim()) {
      return;
    }
    if (nextQuery === queryFromUrl) {
      return;
    }
    replaceSearchParams({ q: nextQuery || undefined });
  }, [debouncedSearchKeyword, queryFromUrl, replaceSearchParams, searchKeyword]);

  useEffect(() => {
    setSelectedSource(
      sourceFromUrl === 'official' || sourceFromUrl === 'third_party' ? sourceFromUrl : 'all'
    );
  }, [sourceFromUrl]);

  useEffect(() => {
    setSelectedSort(sortFromUrl === 'newest' ? 'newest' : 'downloads');
  }, [sortFromUrl]);

  useEffect(() => {
    setSelectedCategories(categoriesFromUrl);
  }, [categoriesFromUrl]);

  useEffect(() => {
    setAllPlugins([]);
    setCurrentPage(1);
    lastProcessedPage.current = 0;
  }, [queryFromUrl, selectedSource, selectedSort, selectedCategories.join(','), locale]);

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

  const handleSearchSubmit = useCallback(
    (event: FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      const nextQuery = (searchInputRef.current?.value ?? searchKeyword).trim();
      setSearchKeyword(nextQuery);
      replaceSearchParams({ q: nextQuery || undefined });
    },
    [replaceSearchParams, searchKeyword]
  );

  const handleSourceChange = useCallback(
    (value: string) => {
      const nextSource = value as PluginSource;
      setSelectedSource(nextSource);
      replaceSearchParams({ source: nextSource === 'all' ? undefined : nextSource });
    },
    [replaceSearchParams]
  );

  const handleSortChange = useCallback(
    (value: string) => {
      const nextSort = value as PluginSort;
      setSelectedSort(nextSort);
      replaceSearchParams({ sort: nextSort === 'downloads' ? undefined : nextSort });
    },
    [replaceSearchParams]
  );

  const handleCategoryToggle = useCallback(
    (category: string) => {
      const nextCategories = selectedCategories.includes(category)
        ? selectedCategories.filter(item => item !== category)
        : [...selectedCategories, category];
      setSelectedCategories(nextCategories);
      replaceSearchParams({
        categories: nextCategories.length > 0 ? nextCategories.join(',') : undefined,
      });
    },
    [replaceSearchParams, selectedCategories]
  );

  const categoryLabel = useMemo(() => {
    if (selectedCategories.length === 0) {
      return t('market.plugins.searchPage.allCategories');
    }
    if (selectedCategories.length === 1) {
      return categoryLookup.get(selectedCategories[0]) || selectedCategories[0];
    }
    return t('market.plugins.searchPage.selectedCategories', {
      count: selectedCategories.length,
    });
  }, [categoryLookup, selectedCategories, t]);

  const handlePluginCardClick = useCallback(
    (plugin: MarketplacePlugin) => {
      router.push(`/dashboard/market/plugins/${plugin.id}`);
    },
    [router]
  );

  return (
    <div className="h-full overflow-y-auto bg-background">
      <div className="mx-auto flex w-full max-w-[1680px] flex-col gap-6 px-4 py-6 sm:px-6 lg:px-8">
        <div className="space-y-8">
          <div className="relative grid grid-cols-[auto_minmax(0,1fr)] gap-3 sm:items-center min-[920px]:min-h-10">
            <Button
              variant="ghost"
              className="h-10 w-10 rounded-lg sm:col-start-1 sm:row-start-1"
              onClick={() => router.push('/dashboard/market/plugins')}
            >
              <X className="h-4 w-4" />
              <span className="sr-only">{t('market.plugins.searchPage.exit')}</span>
            </Button>

            <form
              onSubmit={handleSearchSubmit}
              className="relative min-w-0 sm:col-start-2 sm:row-start-1 min-[920px]:absolute min-[920px]:left-1/2 min-[920px]:top-0 min-[920px]:col-span-1 min-[920px]:row-start-1 min-[920px]:w-[min(48vw,560px)] min-[920px]:-translate-x-1/2"
            >
              <Search className="absolute left-4 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                ref={searchInputRef}
                placeholder={t('market.plugins.searchPlaceholder')}
                value={searchKeyword}
                onChange={event => setSearchKeyword(event.target.value)}
                className="h-10 w-full rounded-lg bg-background pl-10 text-sm shadow-sm"
              />
            </form>
          </div>

          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
              <Select value={selectedSource} onValueChange={handleSourceChange}>
                <SelectTrigger className="h-9 w-full rounded-lg bg-background text-muted-foreground shadow-sm sm:w-[156px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">{t('market.plugins.sourceType.all')}</SelectItem>
                  <SelectItem value="official">
                    {t('market.plugins.sourceType.official')}
                  </SelectItem>
                  <SelectItem value="third_party">
                    {t('market.plugins.sourceType.thirdParty')}
                  </SelectItem>
                </SelectContent>
              </Select>

              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    variant="outline"
                    className="h-9 w-full justify-between rounded-lg bg-background px-3 font-normal text-muted-foreground shadow-sm sm:w-[180px]"
                  >
                    <span className="truncate">{categoryLabel}</span>
                    <ChevronDown className="ml-2 h-4 w-4 shrink-0" />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start" className="w-[220px]">
                  {pluginCategories.map(category => (
                    <DropdownMenuCheckboxItem
                      key={category.value}
                      checked={selectedCategories.includes(category.value)}
                      onCheckedChange={() => handleCategoryToggle(category.value)}
                      onSelect={event => event.preventDefault()}
                    >
                      {category.label}
                    </DropdownMenuCheckboxItem>
                  ))}
                  {selectedCategories.length > 0 && (
                    <Button
                      variant="ghost"
                      className="mt-1 h-8 w-full justify-start px-2 text-xs"
                      onClick={() => {
                        setSelectedCategories([]);
                        replaceSearchParams({ categories: undefined });
                      }}
                    >
                      {t('market.plugins.searchPage.clearCategories')}
                    </Button>
                  )}
                </DropdownMenuContent>
              </DropdownMenu>
            </div>

            <Select value={selectedSort} onValueChange={handleSortChange}>
              <SelectTrigger className="h-9 w-full rounded-lg bg-background shadow-sm sm:w-[160px]">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="downloads">{t('market.plugins.sort.downloads')}</SelectItem>
                <SelectItem value="newest">{t('market.plugins.sort.newest')}</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>

        {error && (
          <div className="flex flex-col items-center justify-center rounded-xl border border-destructive/30 py-12 text-center">
            <p className="mb-4 text-destructive">{error}</p>
          </div>
        )}

        {isLoading && allPlugins.length === 0 && (
          <div className="grid grid-cols-1 gap-4 min-[760px]:grid-cols-2 min-[1040px]:grid-cols-3 min-[1440px]:grid-cols-4">
            {Array.from({ length: 12 }).map((_, idx) => (
              <Skeleton key={idx} className="h-44 w-full rounded-xl" />
            ))}
          </div>
        )}

        {!isLoading && allPlugins.length === 0 && (
          <MarketEmptyState
            title={t('market.plugins.searchPage.noResults')}
            actionLabel={
              branding.upload_application_enabled !== false
                ? t('market.plugins.empty.action')
                : undefined
            }
            onAction={
              branding.upload_application_enabled !== false
                ? () => toast.info(t('common.comingSoon'))
                : undefined
            }
          />
        )}

        {allPlugins.length > 0 && (
          <div className="grid grid-cols-1 gap-4 min-[760px]:grid-cols-2 min-[1040px]:grid-cols-3 min-[1440px]:grid-cols-4">
            {allPlugins.map(plugin => (
              <PluginCard
                key={plugin.id}
                plugin={plugin}
                branding={branding}
                onClick={() => handlePluginCardClick(plugin)}
              />
            ))}
          </div>
        )}

        {allPlugins.length > 0 && hasNextPage && (
          <div ref={sentinelRef} className="flex min-h-16 justify-center py-8">
            <div className="flex items-center justify-center">
              {isFetching ? (
                <div className="h-4 w-4 animate-spin rounded-full border-2 border-primary border-t-transparent" />
              ) : (
                t('market.plugins.scrollHint')
              )}
            </div>
          </div>
        )}

        {allPlugins.length > 0 && !hasNextPage && (
          <div className="flex justify-center py-8">
            <div className="flex w-24 items-center justify-center gap-1.5">
              <span className="h-px flex-1 bg-border" />
              <span className="h-1 w-1 rounded-full bg-muted-foreground/40" />
              <span className="h-px flex-1 bg-border" />
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
