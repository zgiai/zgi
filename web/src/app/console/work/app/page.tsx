'use client';

import Link from 'next/link';
import { useEffect, useMemo, useRef, useState } from 'react';
import { ArrowRight, AppWindow, SearchX } from 'lucide-react';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import { PermissionDeniedState } from '@/components/common/permission-gate-state';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { SearchInput } from '@/components/ui/input';
import { Pagination } from '@/components/ui/pagination';
import { Skeleton } from '@/components/ui/skeleton';
import {
  type RunnableWebAppResolvedItem,
  useRunnableWebApps,
} from '@/hooks/agent/use-runnable-webapps';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useT } from '@/i18n/translations';
import { ICON_BG } from '@/lib/config';

const RECENT_WEBAPP_STORAGE_KEY = 'zgi:webapp:recent';
const APP_PAGE_SIZE = 12;

interface AppCardItem {
  id: string;
  title: string;
  desc: string | null;
  iconType: 'image' | 'text';
  icon: string;
  iconBackground: string;
  src: string;
}

function toAppCard(item: RunnableWebAppResolvedItem): AppCardItem {
  const iconType = item.icon_type;
  const iconRaw = item.meta_data.icon || '';
  let icon = item.meta_data.title.slice(0, 2).toUpperCase();
  let iconBackground = ICON_BG;
  let src = '';

  if (iconType === 'image') {
    src = item.meta_data.icon_url || iconRaw;
  } else {
    try {
      const parsed = JSON.parse(iconRaw) as { icon?: string; icon_background?: string };
      icon = parsed.icon || icon;
      iconBackground = parsed.icon_background || iconBackground;
    } catch {
      // Keep the generated text icon.
    }
  }

  return {
    id: item.web_app_id,
    title: item.meta_data.title,
    desc: item.meta_data.desc,
    iconType,
    icon,
    iconBackground,
    src,
  };
}

export default function ConsoleWorkAppHomePage() {
  const t = useT('webapp');
  const [search, setSearch] = useState('');
  const [queryKeyword, setQueryKeyword] = useState('');
  const [currentPage, setCurrentPage] = useState(1);
  const [recentIds, setRecentIds] = useState<string[]>([]);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const debouncedSearch = useDebouncedValue(search.trim(), 300);
  const { items, isLoading, isFetching, canUseResourceList, total, pageSize } = useRunnableWebApps({
    workspaceId: null,
    keyword: queryKeyword || undefined,
    page: currentPage,
    pageSize: APP_PAGE_SIZE,
  });
  const recentWebAppId = recentIds[0] ?? null;
  const shouldLoadRecent = currentPage === 1 && !queryKeyword && Boolean(recentWebAppId);
  const { items: recentItems } = useRunnableWebApps({
    workspaceId: null,
    webAppId: recentWebAppId,
    page: 1,
    pageSize: 1,
    enabled: shouldLoadRecent,
  });
  const totalPages = Math.max(1, Math.ceil(total / (pageSize || APP_PAGE_SIZE)));
  const isSearchPending = search.trim() !== queryKeyword;
  const showLoading = isLoading || isFetching || isSearchPending;

  useEffect(() => {
    if (typeof window === 'undefined') return;
    try {
      const raw = window.localStorage.getItem(RECENT_WEBAPP_STORAGE_KEY);
      setRecentIds(raw ? (JSON.parse(raw) as string[]) : []);
    } catch {
      setRecentIds([]);
    }
  }, []);

  useEffect(() => {
    setQueryKeyword(debouncedSearch);
    setCurrentPage(1);
  }, [debouncedSearch]);

  useEffect(() => {
    if (!showLoading && total > 0 && currentPage > totalPages) {
      setCurrentPage(totalPages);
    }
  }, [currentPage, showLoading, total, totalPages]);

  const cards = useMemo(() => items.map(toAppCard), [items]);

  const normalizedSearch = queryKeyword.toLowerCase();

  const recentCard = useMemo(() => {
    if (!shouldLoadRecent) return null;
    const recentItem = recentItems.find(item => item.web_app_id === recentWebAppId);
    return recentItem ? toAppCard(recentItem) : null;
  }, [recentItems, recentWebAppId, shouldLoadRecent]);

  const gridCards = useMemo(() => {
    if (!recentCard) return cards;
    return cards.filter(card => card.id !== recentCard.id);
  }, [cards, recentCard]);

  const clearSearch = () => {
    setSearch('');
    setQueryKeyword('');
    setCurrentPage(1);
  };

  const changePage = (page: number) => {
    setCurrentPage(page);
    scrollContainerRef.current?.scrollTo({ top: 0, behavior: 'smooth' });
  };

  if (!isLoading && !canUseResourceList) {
    return <PermissionDeniedState />;
  }

  return (
    <div ref={scrollContainerRef} className="h-full w-full overflow-auto bg-background">
      <div className="mx-auto w-full max-w-6xl px-6 py-6">
        <div className="mb-8 flex flex-col gap-5 lg:flex-row lg:items-end lg:justify-between">
          <div className="min-w-0">
            <h1 className="text-2xl font-semibold tracking-tight">{t('appCenter.title')}</h1>
            <p className="mt-1 text-sm leading-6 text-muted-foreground">
              {t('appCenter.subtitle')}
            </p>
          </div>
          <div className="w-full lg:max-w-sm">
            <SearchInput
              value={search}
              onChange={event => setSearch(event.target.value)}
              placeholder={t('appCenter.searchPlaceholder')}
              aria-label={t('appCenter.searchPlaceholder')}
              className="h-9 bg-background"
            />
          </div>
        </div>
        {showLoading ? (
          <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
            {Array.from({ length: 6 }).map((_, index) => (
              <Card key={`card-skeleton-${index}`} className="border-border">
                <CardHeader className="p-4">
                  <div className="flex items-center gap-3">
                    <Skeleton className="size-10 rounded-md" />
                    <div className="space-y-2">
                      <Skeleton className="h-4 w-40" />
                      <Skeleton className="h-3 w-28" />
                    </div>
                  </div>
                </CardHeader>
              </Card>
            ))}
          </div>
        ) : total === 0 && !normalizedSearch ? (
          <Card className="border-dashed bg-muted/20" role="status" aria-live="polite">
            <CardHeader className="items-center py-12 text-center">
              <div className="mb-2 flex h-12 w-12 items-center justify-center rounded-full bg-muted">
                <AppWindow className="h-6 w-6 text-muted-foreground" />
              </div>
              <CardTitle>{t('appCenter.noAppsTitle')}</CardTitle>
              <CardDescription className="max-w-md">
                {t('appCenter.noAppsDescription')}
              </CardDescription>
            </CardHeader>
          </Card>
        ) : total === 0 ? (
          <Card className="border-dashed bg-muted/20" role="status" aria-live="polite">
            <CardHeader className="items-center py-12 text-center">
              <div className="mb-2 flex h-12 w-12 items-center justify-center rounded-full bg-muted">
                <SearchX className="h-6 w-6 text-muted-foreground" />
              </div>
              <CardTitle>{t('appCenter.noSearchResults')}</CardTitle>
              <CardDescription className="max-w-md">
                {t('appCenter.noSearchResultsDescription')}
              </CardDescription>
              <div className="mt-4">
                <Button variant="outline" onClick={clearSearch}>
                  {t('appCenter.clearSearch')}
                </Button>
              </div>
            </CardHeader>
          </Card>
        ) : (
          <div className="space-y-6">
            {recentCard ? (
              <Card className="border-border/80 shadow-sm">
                <CardHeader className="pb-4">
                  <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
                    <div className="min-w-0">
                      <Badge variant="success" className="mb-3 rounded-full px-2">
                        {t('appCenter.recentlyUsed')}
                      </Badge>
                      <div className="flex items-start gap-3">
                        <IconPreview
                          iconType={recentCard.iconType}
                          src={recentCard.src}
                          icon={recentCard.icon}
                          iconBackground={recentCard.iconBackground}
                          alt={recentCard.title}
                          editable={false}
                          size="sm"
                        />
                        <div className="min-w-0">
                          <CardTitle className="line-clamp-1 text-xl">{recentCard.title}</CardTitle>
                          <CardDescription className="mt-2 line-clamp-2 max-w-2xl text-sm leading-6">
                            {recentCard.desc || t('appCenter.recentFallbackDescription')}
                          </CardDescription>
                        </div>
                      </div>
                    </div>
                    <div className="flex shrink-0 gap-2">
                      <Button asChild>
                        <Link href={`/console/work/app/${recentCard.id}`}>
                          {t('appCenter.openRecent')}
                          <ArrowRight className="h-3.5 w-3.5" />
                        </Link>
                      </Button>
                    </div>
                  </div>
                </CardHeader>
              </Card>
            ) : null}

            <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
              {gridCards.map(card => (
                <Link
                  key={card.id}
                  href={`/console/work/app/${card.id}`}
                  className="group block h-full rounded-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                >
                  <Card className="flex h-full min-h-40 flex-col border-border/80 transition-all group-hover:border-primary/40 group-hover:shadow-sm">
                    <CardHeader className="flex-1 p-4 pb-3">
                      <div className="flex w-full items-start gap-3">
                        <IconPreview
                          iconType={card.iconType}
                          src={card.src}
                          icon={card.icon}
                          iconBackground={card.iconBackground}
                          alt={card.title}
                          editable={false}
                          size="sm"
                        />
                        <div className="w-0 grow pt-0.5">
                          <CardTitle className="line-clamp-1 w-full break-words text-base">
                            {card.title}
                          </CardTitle>
                          <CardDescription className="mt-1 min-h-10 line-clamp-2 w-full break-words text-xs leading-5 text-muted-foreground">
                            {card.desc || null}
                          </CardDescription>
                        </div>
                      </div>
                    </CardHeader>
                    <CardContent className="mt-auto flex items-center justify-end px-4 pb-4 pt-0">
                      <span className="inline-flex h-8 items-center gap-1 px-2 text-sm font-medium text-primary opacity-80 transition-opacity group-hover:opacity-100">
                        {t('appCenter.openApp')}
                        <ArrowRight className="h-3.5 w-3.5" />
                      </span>
                    </CardContent>
                  </Card>
                </Link>
              ))}
            </div>
            <Pagination
              currentPage={currentPage}
              totalPages={totalPages}
              total={total}
              pageSize={pageSize || APP_PAGE_SIZE}
              onPageChange={changePage}
              showJump={false}
              renderInfo={(start, end, count) =>
                t('appCenter.paginationInfo', { start, end, total: count })
              }
              className="border-t pt-5"
            />
          </div>
        )}
      </div>
    </div>
  );
}
