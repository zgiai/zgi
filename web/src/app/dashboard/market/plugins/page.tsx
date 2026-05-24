'use client';

import { useState, useCallback, useEffect, useMemo, useRef, type FormEvent } from 'react';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import { toast } from 'sonner';
import { Check, ChevronsUpDown, MessageSquareText, Search, Upload } from 'lucide-react';
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
import {
  useMarketplaceBranding,
  useMarketplaceCategories,
  useMarketplacePlugins,
} from '@/hooks/use-plugins';
import PluginCard from '@/components/market/plugin-card';
import MarketEmptyState from '@/components/market/market-empty-state';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import type { MarketplacePlugin, MarketplacePluginCategory } from '@/services/types/plugin';
import { useInfiniteObserver } from '@/hooks/use-infinite-observer';
import { Button } from '@/components/ui/button';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useLocale } from '@/hooks/use-locale';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { pluginService } from '@/services/plugin.service';
import type { MarketplacePluginFeedbackRequestType } from '@/services/types/plugin';
import { useAuthStore, useOrganizationStore } from '@/store';

type PluginCategory = '' | MarketplacePluginCategory;
type PluginSort = 'downloads' | 'newest';
type PluginSource = 'official' | 'third_party';
const DEFAULT_PLUGIN_CATEGORY = 'recommend';

export default function PluginsPage() {
  const t = useT();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const { locale } = useLocale();
  const categoryFromUrl = searchParams.get('category') || '';
  const [searchKeyword, setSearchKeyword] = useState('');
  const [currentPage, setCurrentPage] = useState(1);
  const [allPlugins, setAllPlugins] = useState<MarketplacePlugin[]>([]);
  const [totalCount, setTotalCount] = useState(0);
  const [selectedType, setSelectedType] = useState<PluginCategory>(
    categoryFromUrl || DEFAULT_PLUGIN_CATEGORY
  );
  const [selectedSort, setSelectedSort] = useState<PluginSort>('downloads');
  const [selectedSource, setSelectedSource] = useState<PluginSource>('official');
  const [isFeedbackOpen, setIsFeedbackOpen] = useState(false);
  const branding = useMarketplaceBranding();
  const { categories: marketplaceCategories } = useMarketplaceCategories(locale);
  const pluginCategories = useMemo<Array<{ value: PluginCategory; label: string }>>(
    () =>
      marketplaceCategories.map(category => ({
        value: category.code,
        label: category.name || category.name_zh_hans || category.name_en_us || category.code,
      })),
    [marketplaceCategories]
  );
  const sourceOfficial = selectedSource === 'official';

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
    locale,
    sort: selectedSort,
    is_official: sourceOfficial,
  });

  const lastProcessedPage = useRef(0);

  useEffect(() => {
    setAllPlugins([]);
    setCurrentPage(1);
    lastProcessedPage.current = 0;
  }, [selectedType, selectedSort, selectedSource, locale]);

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
    router.push(`/dashboard/market/plugins/${plugin.id}`);
  }, [router]);

  const handleSearchSubmit = useCallback(
    (event: FormEvent<HTMLFormElement>) => {
      event.preventDefault();
      const query = searchKeyword.trim();
      router.push(
        query
          ? `/dashboard/market/plugins/search?q=${encodeURIComponent(query)}`
          : '/dashboard/market/plugins/search'
      );
    },
    [router, searchKeyword]
  );

  const handleCategoryChange = useCallback(
    (value: string) => {
      const nextCategory = value as PluginCategory;
      const nextSearchParams = new URLSearchParams(searchParams.toString());

      nextSearchParams.set('category', nextCategory);

      const nextQuery = nextSearchParams.toString();
      router.replace(nextQuery ? `${pathname}?${nextQuery}` : pathname, { scroll: false });
      setSelectedType(nextCategory);
    },
    [pathname, router, searchParams]
  );

  useEffect(() => {
    const nextCategory = categoryFromUrl || DEFAULT_PLUGIN_CATEGORY;
    if (nextCategory !== selectedType) {
      setSelectedType(nextCategory);
    }
  }, [categoryFromUrl, selectedType]);

  return (
    <div className="h-full overflow-y-auto bg-background">
      <div className="mx-auto flex w-full max-w-[1680px] flex-col gap-6 px-4 py-6 sm:px-6 lg:px-8">
        <div className="space-y-8">
          <div className="relative grid grid-cols-1 gap-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center min-[920px]:min-h-10">
            <h1 className="flex h-10 w-fit items-center text-xl font-semibold tracking-tight sm:col-start-1 sm:row-start-1">
              {t('market.plugins.title')}
            </h1>

            <form
              onSubmit={handleSearchSubmit}
              className="relative min-w-0 sm:col-span-2 sm:row-start-2 min-[920px]:absolute min-[920px]:left-1/2 min-[920px]:top-0 min-[920px]:col-span-1 min-[920px]:row-start-1 min-[920px]:w-[min(48vw,560px)] min-[920px]:-translate-x-1/2"
            >
              <Search className="absolute left-4 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder={t('market.plugins.searchPlaceholder')}
                value={searchKeyword}
                onChange={e => setSearchKeyword(e.target.value)}
                className="h-10 w-full rounded-lg bg-background pl-10 text-sm shadow-sm"
              />
            </form>

            <div className="flex min-w-0 items-center justify-start gap-2 sm:col-start-2 sm:row-start-1 sm:justify-end">
              {branding.feedback_enabled !== false && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="outline"
                      className="h-10 w-10 shrink-0 rounded-lg bg-background p-0 shadow-sm"
                      onClick={() => setIsFeedbackOpen(true)}
                    >
                      <MessageSquareText className="h-4 w-4" />
                      <span className="sr-only">{t('market.plugins.feedback')}</span>
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>{t('market.plugins.feedback')}</TooltipContent>
                </Tooltip>
              )}
              {branding.upload_application_enabled !== false && (
                <Button
                  variant="default"
                  className="h-10 shrink-0 rounded-lg px-4 shadow-sm"
                  onClick={() => toast.info(t('common.comingSoon'))}
                >
                  <Upload className="mr-2 h-4 w-4 shrink-0" />
                  {t('market.plugins.publishPlugin')}
                </Button>
              )}
            </div>
          </div>

          <Tabs
            value={selectedType}
            onValueChange={handleCategoryChange}
          >
            <TabsList className="h-auto w-full justify-start gap-1 overflow-x-auto border-0 bg-transparent p-0">
              {pluginCategories.map(category => (
                <TabsTrigger
                  key={category.value || 'all'}
                  value={category.value}
                  className="h-10 rounded-lg border border-transparent px-4 text-sm font-medium text-muted-foreground data-[state=active]:border-transparent data-[state=active]:bg-primary/10 data-[state=active]:text-primary data-[state=active]:shadow-none"
                >
                  {category.label}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>

          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex flex-col gap-3 sm:flex-row sm:items-center">
              <Select
                value={selectedSource}
                onValueChange={value => setSelectedSource(value as PluginSource)}
              >
                <SelectTrigger className="h-9 w-full rounded-lg bg-background text-muted-foreground shadow-sm sm:w-[156px]">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="official">
                    {t('market.plugins.sourceType.official')}
                  </SelectItem>
                  <SelectItem value="third_party">
                    {t('market.plugins.sourceType.thirdParty')}
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>

            <Select
              value={selectedSort}
              onValueChange={value => setSelectedSort(value as PluginSort)}
            >
              <SelectTrigger className="h-9 w-full rounded-lg bg-background shadow-sm sm:w-[160px]">
                <SelectValue placeholder={t('market.plugins.sort.label')} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="downloads">{t('market.plugins.sort.downloads')}</SelectItem>
                <SelectItem value="newest">{t('market.plugins.sort.newest')}</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>

        {/* Error State */}
        {error && (
          <div className="flex flex-col items-center justify-center rounded-xl border border-destructive/30 py-12 text-center">
            <p className="mb-4 text-destructive">{error}</p>
          </div>
        )}

        {/* Loading State - Only show skeleton on initial load */}
        {isLoading && allPlugins.length === 0 && (
          <div className="grid grid-cols-1 gap-4 min-[760px]:grid-cols-2 min-[1040px]:grid-cols-3 min-[1440px]:grid-cols-4">
            {Array.from({ length: 12 }).map((_, idx) => (
              <Skeleton key={idx} className="h-44 w-full rounded-xl" />
            ))}
          </div>
        )}

        {/* Empty State */}
        {!isLoading && allPlugins.length === 0 && (
          <MarketEmptyState
            title={t('market.plugins.empty.title')}
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

        {/* Infinite Scroll Sentinel */}
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

        {/* No More Data Indicator */}
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

      <PluginFeedbackDialog
        open={isFeedbackOpen}
        onOpenChange={setIsFeedbackOpen}
      />
    </div>
  );
}

function PluginFeedbackDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const t = useT();
  const { locale } = useLocale();
  const user = useAuthStore.use.user();
  const currentOrganization = useOrganizationStore.use.currentOrganization();
  const [requestType, setRequestType] =
    useState<MarketplacePluginFeedbackRequestType>('existing_official');
  const [pluginId, setPluginId] = useState('');
  const [pluginSearch, setPluginSearch] = useState('');
  const [isPluginPickerOpen, setIsPluginPickerOpen] = useState(false);
  const [content, setContent] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const needsPlugin = requestType === 'existing_official';
  const debouncedPluginSearch = useDebouncedValue(pluginSearch, 300);
  const { plugins: officialPlugins, isFetching: isFetchingOfficialPlugins } = useMarketplacePlugins(
    {
      page: 1,
      page_size: 100,
      locale,
      sort: 'downloads',
      is_official: true,
      search: debouncedPluginSearch || undefined,
    },
    {
      enabled: open && needsPlugin,
      staleTime: 30 * 1000,
    }
  );

  const selectedPluginId = useMemo(() => {
    if (!needsPlugin) return '';
    return pluginId || officialPlugins[0]?.id || '';
  }, [needsPlugin, officialPlugins, pluginId]);
  const selectedPlugin = officialPlugins.find(plugin => plugin.id === selectedPluginId);

  useEffect(() => {
    if (open && needsPlugin && !pluginId && officialPlugins[0]?.id) {
      setPluginId(officialPlugins[0].id);
    }
  }, [needsPlugin, officialPlugins, open, pluginId]);

  const handleSubmit = async () => {
    if (!content.trim() || (needsPlugin && !selectedPluginId)) {
      toast.error(t('market.plugins.feedbackDialog.required'));
      return;
    }

    setIsSubmitting(true);
    try {
      await pluginService.submitMarketplacePluginFeedback({
        request_type: requestType,
        plugin_id: needsPlugin ? selectedPluginId : undefined,
        content: content.trim(),
        submitter_id: user?.id,
        submitter_name: user?.name || user?.email,
        submitter_email: user?.email,
        submitter_organization_id: currentOrganization?.id,
        submitter_organization_name: currentOrganization?.name,
      });
      toast.success(t('market.plugins.feedbackDialog.submitted'));
      setContent('');
      setRequestType('existing_official');
      setPluginId('');
      setPluginSearch('');
      onOpenChange(false);
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('market.plugins.feedbackDialog.submitFailed')
      );
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{t('market.plugins.feedbackDialog.title')}</DialogTitle>
        </DialogHeader>
        <DialogBody className="grid gap-5">
          <div className="grid gap-2">
            <Label>
              {t('market.plugins.feedbackDialog.requestType')}
              <span className="text-destructive">*</span>
            </Label>
            <Select
              value={requestType}
              onValueChange={value => setRequestType(value as MarketplacePluginFeedbackRequestType)}
            >
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="existing_official">
                  {t('market.plugins.feedbackDialog.types.existingOfficial')}
                </SelectItem>
                <SelectItem value="missing_plugin">
                  {t('market.plugins.feedbackDialog.types.missingPlugin')}
                </SelectItem>
                <SelectItem value="other">
                  {t('market.plugins.feedbackDialog.types.other')}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>

          {needsPlugin && (
            <div className="grid gap-2">
              <Label>
                {t('market.plugins.feedbackDialog.officialPlugin')}
                <span className="text-destructive">*</span>
              </Label>
              <Popover open={isPluginPickerOpen} onOpenChange={setIsPluginPickerOpen}>
                <PopoverTrigger asChild>
                  <Button
                    type="button"
                    variant="outline"
                    className="h-10 w-full justify-between rounded-lg bg-background px-3 font-normal"
                  >
                    <span className="truncate">
                      {selectedPlugin?.name ||
                        t('market.plugins.feedbackDialog.officialPluginPlaceholder')}
                    </span>
                    <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 text-muted-foreground" />
                  </Button>
                </PopoverTrigger>
                <PopoverContent
                  align="start"
                  className="z-[60] w-[var(--radix-popover-trigger-width)] p-2"
                >
                  <div className="relative">
                    <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                    <Input
                      value={pluginSearch}
                      onChange={event => setPluginSearch(event.target.value)}
                      className="h-9 pl-9"
                      placeholder={t('market.plugins.feedbackDialog.officialPluginSearch')}
                    />
                  </div>
                  <div className="mt-2 max-h-64 overflow-y-auto">
                    {isFetchingOfficialPlugins && officialPlugins.length === 0 ? (
                      <div className="px-3 py-6 text-center text-sm text-muted-foreground">
                        {t('market.plugins.loading')}
                      </div>
                    ) : officialPlugins.length === 0 ? (
                      <div className="px-3 py-6 text-center text-sm text-muted-foreground">
                        {t('market.plugins.noResults')}
                      </div>
                    ) : (
                      <div className="grid gap-1">
                        {officialPlugins.map(plugin => (
                          <button
                            key={plugin.id}
                            type="button"
                            className="flex h-9 min-w-0 items-center gap-2 rounded-md px-2 text-left text-sm hover:bg-muted"
                            onClick={() => {
                              setPluginId(plugin.id);
                              setIsPluginPickerOpen(false);
                            }}
                          >
                            <Check
                              className={`h-4 w-4 shrink-0 ${
                                selectedPluginId === plugin.id ? 'opacity-100' : 'opacity-0'
                              }`}
                            />
                            <span className="min-w-0 truncate">{plugin.name}</span>
                          </button>
                        ))}
                      </div>
                    )}
                  </div>
                  <div className="mt-2 border-t pt-2 text-xs text-muted-foreground">
                    {t('market.plugins.feedbackDialog.officialPluginLimitHint')}
                  </div>
                </PopoverContent>
              </Popover>
            </div>
          )}

          <div className="grid gap-2">
            <Label>
              {t('market.plugins.feedbackDialog.content')}
              <span className="text-destructive">*</span>
            </Label>
            <Textarea
              value={content}
              maxLength={2000}
              rows={5}
              className="min-h-32 max-h-60"
              placeholder={t('market.plugins.feedbackDialog.contentPlaceholder')}
              onChange={event => setContent(event.target.value)}
            />
            <div className="text-right text-xs text-muted-foreground">{content.length}/2000</div>
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)} disabled={isSubmitting}>
            {t('market.plugins.feedbackDialog.cancel')}
          </Button>
          <Button onClick={handleSubmit} disabled={isSubmitting}>
            {isSubmitting
              ? t('market.plugins.feedbackDialog.submitting')
              : t('market.plugins.feedbackDialog.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
