'use client';

import Link from 'next/link';
import { useEffect, useMemo, useState } from 'react';
import { usePathname } from 'next/navigation';
import { AppWindow, ArrowRightToLine, PanelLeft, X } from 'lucide-react';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import { SearchInput } from '@/components/ui/input';
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';
import { useRunnableWebApps } from '@/hooks/agent/use-runnable-webapps';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useT } from '@/i18n/translations';
import { getSidebarCollapsed, saveSidebarCollapsed } from '@/utils/ui-local';
import type { RunnableWebAppResolvedItem } from '@/hooks/agent/use-runnable-webapps';
import { Logo } from '@/components/logo';
import { ICON_BG } from '@/lib/config';

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

  return {
    iconType,
    src,
    textIcon,
    iconBackground,
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
  const { items, isLoading, isFetching } = useRunnableWebApps({
    workspaceId: null,
    keyword: queryKeyword || undefined,
  });
  const isSearchPending = searchQuery.trim() !== queryKeyword;
  const showNavLoading = isLoading || isFetching || isSearchPending;

  useEffect(() => {
    saveSidebarCollapsed('app', isCollapsed);
  }, [isCollapsed]);

  useEffect(() => {
    setQueryKeyword(debouncedSearchQuery);
  }, [debouncedSearchQuery]);

  const navItems = useMemo(
    () =>
      items.map(item => {
        const title = item.meta_data.title;
        const preview = toPreviewData(item);
        return {
          id: item.web_app_id,
          title,
          description: item.meta_data.desc?.trim() || '',
          preview,
        };
      }),
    [items]
  );

  const currentWebappId = useMemo(() => {
    const match = pathname.match(/^\/console\/work\/app\/([^/?#]+)/);
    return match?.[1] ?? null;
  }, [pathname]);

  const currentApp = useMemo(
    () => (currentWebappId ? items.find(item => item.web_app_id === currentWebappId) : null),
    [currentWebappId, items]
  );

  const currentAppPreview = useMemo(
    () => (currentApp ? toPreviewData(currentApp) : null),
    [currentApp]
  );

  const clearSearch = () => {
    setSearchQuery('');
    setQueryKeyword('');
  };

  const appSearch = (
    <div className="relative">
      <SearchInput
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
      {!isCollapsed ? (
        <div className="space-y-3 border-b px-3 py-3">
          <div className="min-w-0">
            <div className="text-sm font-semibold leading-5">{t('appCenter.title')}</div>
            <div className="mt-0.5 text-xs leading-4 text-muted-foreground">
              {t('appCenter.sidebarSubtitle')}
            </div>
          </div>
          {appSearch}
        </div>
      ) : null}
      <div className="w-full flex-1 overflow-y-auto">
        <div className={cn('flex w-full flex-col gap-0.5 p-2', isCollapsed && 'px-2')}>
          <Link
            href="/console/work/app"
            onClick={() => setMobileDrawerOpen(false)}
            aria-current={pathname === '/console/work/app' ? 'page' : undefined}
            className={cn(
              'flex h-10 w-full shrink-0 items-center rounded-md px-3 py-2 text-[13px] font-medium transition-colors',
              isCollapsed && 'justify-center px-2',
              pathname === '/console/work/app'
                ? 'bg-background text-foreground shadow-sm ring-1 ring-border/70'
                : 'text-muted-foreground hover:bg-background/70 hover:text-foreground'
            )}
            title={t('appCenter.allApps')}
          >
            <AppWindow className="size-4 shrink-0" />
            {!isCollapsed ? (
              <span className="ml-2 line-clamp-1 truncate text-ellipsis">
                {t('appCenter.allApps')}
              </span>
            ) : null}
          </Link>
          {showNavLoading
            ? Array.from({ length: 6 }).map((_, index) => (
                <div
                  key={`app-skeleton-${index}`}
                  className={cn('h-10 shrink-0 rounded-md px-3 py-2', isCollapsed && 'px-2')}
                >
                  <div className={cn('flex items-center gap-2', isCollapsed && 'justify-center')}>
                    <Skeleton className="size-6 rounded-md" />
                    {!isCollapsed ? <Skeleton className="h-4 w-32" /> : null}
                  </div>
                </div>
              ))
            : navItems.map(item => {
                const isActive = pathname === `/console/work/app/${item.id}`;
                const appLink = (
                  <Link
                    key={item.id}
                    href={`/console/work/app/${item.id}`}
                    onClick={() => setMobileDrawerOpen(false)}
                    aria-current={isActive ? 'page' : undefined}
                    aria-label={item.title}
                    className={cn(
                      'flex h-10 w-full shrink-0 items-center rounded-md px-3 py-2 text-[13px] transition-colors',
                      isCollapsed && 'justify-center px-2',
                      isActive
                        ? 'bg-background text-foreground shadow-sm ring-1 ring-border/70'
                        : 'text-muted-foreground hover:bg-background/70 hover:text-foreground'
                    )}
                    title={isCollapsed ? undefined : item.title}
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
                    <TooltipContent side="right" sideOffset={8} className="max-w-64 px-3 py-2.5">
                      <div className="text-sm font-semibold leading-5">{item.title}</div>
                      {item.description ? (
                        <div className="mt-1 line-clamp-3 whitespace-pre-line text-xs font-normal leading-5 text-muted-foreground">
                          {item.description}
                        </div>
                      ) : null}
                    </TooltipContent>
                  </Tooltip>
                );
              })}
          {!showNavLoading && queryKeyword && navItems.length === 0 && !isCollapsed ? (
            <div className="px-3 py-6 text-center" role="status" aria-live="polite">
              <p className="text-xs text-muted-foreground">{t('appCenter.noSearchResults')}</p>
              <Button type="button" variant="link" size="xs" onClick={clearSearch} className="mt-1">
                {t('appCenter.clearSearch')}
              </Button>
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
          isCollapsed ? 'w-14' : 'w-60'
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
          <div className="h-0 grow overflow-y-auto">
            <div className="p-2 space-y-1">
              <Link
                href="/console/work/app"
                onClick={() => setMobileDrawerOpen(false)}
                aria-current={pathname === '/console/work/app' ? 'page' : undefined}
                className={cn(
                  'flex min-h-10 w-full items-center rounded-md px-3 py-2 text-sm transition-colors',
                  pathname === '/console/work/app'
                    ? 'bg-primary/10 text-primary'
                    : 'hover:bg-accent text-foreground'
                )}
              >
                <AppWindow className="size-4 shrink-0" />
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
            </div>
          </div>
        </SheetContent>
      </Sheet>
    </div>
  );
}
