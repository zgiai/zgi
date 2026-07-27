'use client';

import Link from 'next/link';
import { useEffect, useMemo, useRef, useState } from 'react';
import { usePathname } from 'next/navigation';
import { AppWindow, ArrowRightToLine, PanelLeft, X } from 'lucide-react';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { useInfiniteRunnableWebApps, useRunnableWebApps } from '@/hooks/agent/use-runnable-webapps';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useInfiniteObserver } from '@/hooks/use-infinite-observer';
import { useT } from '@/i18n/translations';
import { getSidebarCollapsed, saveSidebarCollapsed } from '@/utils/ui-local';
import type { RunnableWebAppResolvedItem } from '@/hooks/agent/use-runnable-webapps';
import { Logo } from '@/components/logo';
import { ICON_BG } from '@/lib/config';

const SIDEBAR_PAGE_SIZE = 20;
const COLLAPSED_APP_LIMIT = 6;

interface SidebarNavItem {
  id: string;
  title: string;
  preview: ReturnType<typeof toPreviewData>;
}

function toPreviewData(item: RunnableWebAppResolvedItem) {
  let iconType: 'image' | 'text' = item.icon_type === 'image' ? 'image' : 'text';
  let src = '';
  let textIcon = (item.meta_data.title || 'A').slice(0, 2).toUpperCase();
  let iconBackground = ICON_BG;
  const icon = item.meta_data.icon;

  if (item.icon_type === 'image') {
    src = item.meta_data.icon_url || icon;
  } else if (item.icon_type === 'text') {
    try {
      const parsed = JSON.parse(icon || '{}') as { icon?: string; icon_background?: string };
      textIcon = parsed.icon || textIcon;
      iconBackground = parsed.icon_background || iconBackground;
    } catch {
      iconType = 'text';
    }
  }

  textIcon = Array.from(textIcon.trim()).slice(0, 2).join('') || 'A';

  return {
    iconType,
    src,
    textIcon,
    iconBackground,
  };
}

function toSidebarNavItem(item: RunnableWebAppResolvedItem): SidebarNavItem {
  return {
    id: item.web_app_id,
    title: item.meta_data.title,
    preview: toPreviewData(item),
  };
}

export default function ConsoleWorkAppLayout({ children }: { children: React.ReactNode }) {
  const t = useT('webapp');
  const tNav = useT('navigation');
  const pathname = usePathname();
  const [isCollapsed, setIsCollapsed] = useState<boolean>(() => getSidebarCollapsed('app', false));
  const [mobileDrawerOpen, setMobileDrawerOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [queryKeyword, setQueryKeyword] = useState('');
  const debouncedSearchQuery = useDebouncedValue(searchQuery.trim(), 300);
  const currentWebappId = useMemo(() => {
    const match = pathname.match(/^\/console\/work\/app\/([^/?#]+)/);
    return match?.[1] ?? null;
  }, [pathname]);
  const { items, isLoading, isFetching, fetchNextPage, hasMore, isFetchingNextPage } =
    useInfiniteRunnableWebApps({
      workspaceId: null,
      keyword: queryKeyword || undefined,
      pageSize: SIDEBAR_PAGE_SIZE,
      enabled: !isCollapsed || mobileDrawerOpen,
    });
  const { items: currentItems } = useRunnableWebApps({
    workspaceId: null,
    webAppId: currentWebappId,
    page: 1,
    pageSize: 1,
    enabled: Boolean(currentWebappId),
  });
  const { items: collapsedItems, isLoading: isCollapsedAppsLoading } = useRunnableWebApps({
    workspaceId: null,
    page: 1,
    pageSize: COLLAPSED_APP_LIMIT,
    enabled: isCollapsed,
  });
  const isSearchPending = searchQuery.trim() !== queryKeyword;
  const showNavLoading = isLoading || isSearchPending || (isFetching && items.length === 0);
  const desktopScrollRef = useRef<HTMLDivElement>(null);
  const mobileScrollRef = useRef<HTMLDivElement>(null);
  const desktopLoadMoreRef = useInfiniteObserver({
    hasNextPage: hasMore && !isSearchPending,
    isFetchingNextPage,
    fetchNextPage,
    rootRef: desktopScrollRef,
    enabled: !isCollapsed,
  });
  const mobileLoadMoreRef = useInfiniteObserver({
    hasNextPage: hasMore && !isSearchPending,
    isFetchingNextPage,
    fetchNextPage,
    rootRef: mobileScrollRef,
    enabled: mobileDrawerOpen,
  });

  useEffect(() => {
    saveSidebarCollapsed('app', isCollapsed);
  }, [isCollapsed]);

  useEffect(() => {
    setQueryKeyword(debouncedSearchQuery);
  }, [debouncedSearchQuery]);

  const currentApp = useMemo(
    () =>
      currentItems[0] ??
      (currentWebappId ? items.find(item => item.web_app_id === currentWebappId) : null),
    [currentItems, currentWebappId, items]
  );

  const navItems = useMemo(() => items.map(toSidebarNavItem), [items]);
  const collapsedNavItems = useMemo(() => collapsedItems.map(toSidebarNavItem), [collapsedItems]);

  const currentAppPreview = useMemo(
    () => (currentApp ? toPreviewData(currentApp) : null),
    [currentApp]
  );

  const clearSearch = () => {
    setSearchQuery('');
    setQueryKeyword('');
  };

  const renderDesktopAppLink = (item: SidebarNavItem) => {
    const isActive = pathname === `/console/work/app/${item.id}`;
    const appLink = (
      <Link
        key={item.id}
        href={`/console/work/app/${item.id}`}
        onClick={() => setMobileDrawerOpen(false)}
        aria-current={isActive ? 'page' : undefined}
        aria-label={item.title}
        className={cn(
          'relative flex h-11 w-full shrink-0 items-center rounded-md px-3 py-2 text-[13px] transition-colors',
          isCollapsed && 'justify-center px-2',
          isActive
            ? 'bg-primary/10 text-primary'
            : 'text-muted-foreground hover:bg-background/70 hover:text-foreground'
        )}
        title={isCollapsed ? undefined : item.title}
      >
        {isActive ? (
          <span
            aria-hidden="true"
            className="absolute inset-y-2 left-0 w-0.5 rounded-r-full bg-primary"
          />
        ) : null}
        <IconPreview
          iconType={item.preview.iconType}
          src={item.preview.src}
          icon={item.preview.textIcon}
          iconBackground={item.preview.iconBackground}
          alt={item.title}
          editable={false}
          size="xs"
        />
        {!isCollapsed ? (
          <span className="ml-2 min-w-0 truncate text-ellipsis text-[13px] leading-5">
            {item.title}
          </span>
        ) : null}
      </Link>
    );

    if (!isCollapsed) return appLink;

    return (
      <Tooltip key={item.id}>
        <TooltipTrigger asChild>{appLink}</TooltipTrigger>
        <TooltipContent side="right" sideOffset={8} className="max-w-64 px-3 py-2">
          <div className="text-sm font-medium leading-5">{item.title}</div>
        </TooltipContent>
      </Tooltip>
    );
  };

  const renderDesktopAllAppsLink = () => {
    const isActive = pathname === '/console/work/app';
    const allAppsLink = (
      <Link
        href="/console/work/app"
        onClick={() => setMobileDrawerOpen(false)}
        aria-current={isActive ? 'page' : undefined}
        aria-label={t('appCenter.allApps')}
        className={cn(
          'group relative flex h-11 w-full shrink-0 items-center rounded-md border px-3 py-2 text-[13px] font-medium shadow-xs transition-colors',
          isCollapsed && 'justify-center px-2',
          isActive
            ? 'border-primary/25 bg-primary/10 text-primary'
            : 'border-border/70 bg-background/70 text-foreground hover:border-primary/30 hover:bg-primary/5 hover:text-primary'
        )}
      >
        {isActive ? (
          <span
            aria-hidden="true"
            className="absolute inset-y-2 left-0 w-0.5 rounded-r-full bg-primary"
          />
        ) : null}
        <span
          className={cn(
            'flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground transition-colors',
            isActive
              ? 'bg-primary text-primary-foreground'
              : 'group-hover:bg-primary/15 group-hover:text-primary'
          )}
        >
          <AppWindow className="size-4" />
        </span>
        {!isCollapsed ? (
          <span className="ml-2 min-w-0 truncate text-ellipsis text-[13px] leading-5">
            {t('appCenter.allApps')}
          </span>
        ) : null}
      </Link>
    );

    if (!isCollapsed) return allAppsLink;

    return (
      <Tooltip>
        <TooltipTrigger asChild>{allAppsLink}</TooltipTrigger>
        <TooltipContent side="right" sideOffset={8} className="px-3 py-2">
          <div className="text-sm font-medium leading-5">{t('appCenter.allApps')}</div>
        </TooltipContent>
      </Tooltip>
    );
  };

  const appSearch = (
    <div className="relative">
      <SearchInput
        type="text"
        role="searchbox"
        value={searchQuery}
        onChange={event => setSearchQuery(event.target.value)}
        placeholder={t('appCenter.searchPlaceholder')}
        aria-label={t('appCenter.searchPlaceholder')}
        className="h-9 rounded-md pr-9 text-sm"
      />
      {searchQuery ? (
        <button
          type="button"
          onClick={clearSearch}
          aria-label={t('appCenter.clearSearch')}
          className="focus-ring absolute right-1.5 top-1/2 flex size-6 -translate-y-1/2 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
        >
          <X className="size-3.5" />
        </button>
      ) : null}
    </div>
  );

  const navList = (
    <div className="flex h-0 grow flex-col">
      {!isCollapsed ? <div className="border-b px-3 py-3">{appSearch}</div> : null}
      <div
        ref={desktopScrollRef}
        className={cn(
          'w-full flex-1',
          isCollapsed
            ? 'overflow-hidden'
            : 'overflow-y-auto scrollbar-thin [scrollbar-gutter:stable]'
        )}
      >
        <div className={cn('flex w-full flex-col gap-0.5 p-2', isCollapsed && 'px-2')}>
          <div className="mb-1 border-b border-border/70 pb-2">{renderDesktopAllAppsLink()}</div>
          {!isCollapsed && showNavLoading
            ? Array.from({ length: 6 }).map((_, index) => (
                <div key={`app-skeleton-${index}`} className="h-11 shrink-0 rounded-md px-3 py-2">
                  <div className="flex items-center gap-2">
                    <Skeleton className="size-6 rounded-md" />
                    <Skeleton className="h-4 w-32" />
                  </div>
                </div>
              ))
            : (isCollapsed ? collapsedNavItems : navItems).map(item => renderDesktopAppLink(item))}
          {isCollapsed && isCollapsedAppsLoading ? (
            <div className="space-y-0.5">
              {Array.from({ length: 3 }).map((_, index) => (
                <div
                  key={`app-collapsed-skeleton-${index}`}
                  className="flex h-11 items-center justify-center rounded-md"
                >
                  <Skeleton className="size-8 rounded-md" />
                </div>
              ))}
            </div>
          ) : null}
          {!showNavLoading && queryKeyword && navItems.length === 0 && !isCollapsed ? (
            <div className="px-3 py-6 text-center" role="status" aria-live="polite">
              <p className="text-xs text-muted-foreground">{t('appCenter.noSearchResults')}</p>
              <Button type="button" variant="link" size="xs" onClick={clearSearch} className="mt-1">
                {t('appCenter.clearSearch')}
              </Button>
            </div>
          ) : null}
          {!isCollapsed && !showNavLoading && isFetchingNextPage ? (
            <div className="space-y-1 py-1" role="status">
              <span className="sr-only">{t('appCenter.loadingMoreApps')}</span>
              <Skeleton className="h-10 w-full rounded-md" />
              <Skeleton className="h-10 w-full rounded-md" />
            </div>
          ) : null}
          {!isCollapsed && !showNavLoading && hasMore ? (
            <div ref={desktopLoadMoreRef} className="h-1 shrink-0" aria-hidden="true" />
          ) : null}
          {!isCollapsed && !showNavLoading && !hasMore && navItems.length > 0 ? (
            <div className="px-3 py-3 text-center text-xs text-muted-foreground">
              {t('appCenter.noMoreApps')}
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );

  return (
    <div className="h-full w-full flex min-w-0 min-h-0">
      <aside
        className={cn(
          'hidden shrink-0 flex-col border-r bg-muted/10 transition-all duration-300 md:flex',
          isCollapsed ? 'w-16' : 'w-64'
        )}
      >
        {navList}
        <div className="border-t p-1">
          <Button
            onClick={() => {
              if (!isCollapsed) clearSearch();
              setIsCollapsed(prev => !prev);
            }}
            variant="ghost"
            size="xs"
            aria-label={isCollapsed ? tNav('expand') : tNav('collapse')}
            aria-expanded={!isCollapsed}
            className={cn(
              'w-full flex h-7 items-center rounded-md py-0 text-xs transition-colors',
              isCollapsed ? 'justify-center px-0' : 'justify-start px-2.5',
              'hover:bg-primary/5 hover:text-primary'
            )}
          >
            <ArrowRightToLine
              className={cn(
                'h-4 w-4 shrink-0 transition-transform duration-300',
                !isCollapsed && 'rotate-180'
              )}
            />
            <span
              className={cn(
                'truncate transition-all duration-300 ml-2',
                isCollapsed && 'opacity-0 w-0 overflow-hidden ml-0'
              )}
            >
              {isCollapsed ? tNav('expand') : tNav('collapse')}
            </span>
          </Button>
        </div>
      </aside>
      <div className="w-0 grow h-full min-w-0 min-h-0 flex flex-col">
        <div className="md:hidden flex items-center justify-between py-1 px-2 border-b">
          <div className="flex items-center gap-1 min-w-0">
            <Button variant="ghost" size="sm" onClick={() => setMobileDrawerOpen(true)}>
              <PanelLeft className="h-4 w-4" />
              {t('appCenter.appList')}
            </Button>
          </div>
          {currentApp && currentAppPreview ? (
            <div className="min-w-0 max-w-[58%] flex items-center gap-2">
              <IconPreview
                iconType={currentAppPreview.iconType}
                src={currentAppPreview.src}
                icon={currentAppPreview.textIcon}
                iconBackground={currentAppPreview.iconBackground}
                alt={currentApp.meta_data.title}
                editable={false}
                size="xs"
              />
            </div>
          ) : null}
        </div>
        <div className="w-full h-0 grow min-h-0">{children}</div>
      </div>
      <Sheet open={mobileDrawerOpen} onOpenChange={setMobileDrawerOpen}>
        <SheetContent
          side="left"
          className="w-full sm:max-w-sm p-0 h-full flex flex-col gap-0"
          showClose={false}
        >
          <SheetHeader className="px-4 py-2 border-b h-14 flex flex-row items-center space-y-0 justify-between w-full">
            <SheetTitle className="sr-only">{t('appCenter.appList')}</SheetTitle>
            <Logo routerToHome={false} showName={false} />
            <Button
              variant="ghost"
              size="sm"
              aria-label={t('appCenter.closeAppList')}
              onClick={() => setMobileDrawerOpen(false)}
            >
              <X className="h-4 w-4" />
            </Button>
          </SheetHeader>
          <div className="border-b px-3 py-3">{appSearch}</div>
          <div ref={mobileScrollRef} className="h-0 grow overflow-y-auto">
            <div className="p-2 space-y-1">
              <Link
                href="/console/work/app"
                onClick={() => setMobileDrawerOpen(false)}
                aria-current={pathname === '/console/work/app' ? 'page' : undefined}
                className={cn(
                  'group mb-1 flex min-h-10 w-full items-center rounded-md border px-3 py-2 text-sm font-medium shadow-xs transition-colors',
                  pathname === '/console/work/app'
                    ? 'border-primary/25 bg-primary/10 text-primary'
                    : 'border-border/70 bg-background/70 text-foreground hover:border-primary/30 hover:bg-primary/5 hover:text-primary'
                )}
              >
                <span
                  className={cn(
                    'flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground transition-colors',
                    pathname === '/console/work/app'
                      ? 'bg-primary text-primary-foreground'
                      : 'group-hover:bg-primary/15 group-hover:text-primary'
                  )}
                >
                  <AppWindow className="size-4" />
                </span>
                <span className="ml-2 truncate">{t('appCenter.allApps')}</span>
              </Link>
              {showNavLoading
                ? Array.from({ length: 6 }).map((_, index) => (
                    <div
                      key={`app-mobile-skeleton-${index}`}
                      className="min-h-10 rounded-md px-3 py-2"
                    >
                      <div className="flex items-center gap-2">
                        <Skeleton className="size-6 rounded-md" />
                        <Skeleton className="h-4 w-32" />
                      </div>
                    </div>
                  ))
                : navItems.map(item => {
                    const isActive = pathname === `/console/work/app/${item.id}`;
                    return (
                      <Link
                        key={item.id}
                        href={`/console/work/app/${item.id}`}
                        onClick={() => setMobileDrawerOpen(false)}
                        aria-current={isActive ? 'page' : undefined}
                        className={cn(
                          'flex min-h-10 w-full items-center rounded-md px-3 py-2 text-sm transition-colors',
                          isActive
                            ? 'bg-primary/10 text-primary'
                            : 'hover:bg-accent text-foreground'
                        )}
                        title={item.title}
                      >
                        <IconPreview
                          iconType={item.preview.iconType}
                          src={item.preview.src}
                          icon={item.preview.textIcon}
                          iconBackground={item.preview.iconBackground}
                          alt={item.title}
                          editable={false}
                          size="xs"
                        />
                        <span className="ml-2 truncate">{item.title}</span>
                      </Link>
                    );
                  })}
              {!showNavLoading && queryKeyword && navItems.length === 0 ? (
                <div className="px-3 py-8 text-center" role="status" aria-live="polite">
                  <p className="text-sm text-muted-foreground">{t('appCenter.noSearchResults')}</p>
                  <Button
                    type="button"
                    variant="link"
                    size="sm"
                    onClick={clearSearch}
                    className="mt-1"
                  >
                    {t('appCenter.clearSearch')}
                  </Button>
                </div>
              ) : null}
              {!showNavLoading && isFetchingNextPage ? (
                <div className="space-y-1 py-1" role="status">
                  <span className="sr-only">{t('appCenter.loadingMoreApps')}</span>
                  <Skeleton className="h-10 w-full rounded-md" />
                  <Skeleton className="h-10 w-full rounded-md" />
                </div>
              ) : null}
              {!showNavLoading && hasMore ? (
                <div ref={mobileLoadMoreRef} className="h-1 shrink-0" aria-hidden="true" />
              ) : null}
              {!showNavLoading && !hasMore && navItems.length > 0 ? (
                <div className="px-3 py-3 text-center text-xs text-muted-foreground">
                  {t('appCenter.noMoreApps')}
                </div>
              ) : null}
            </div>
          </div>
        </SheetContent>
      </Sheet>
    </div>
  );
}
