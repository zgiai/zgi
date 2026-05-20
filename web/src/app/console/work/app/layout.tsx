'use client';

import Link from 'next/link';
import { useEffect, useMemo, useState } from 'react';
import { usePathname } from 'next/navigation';
import { AppWindow, ArrowRightToLine, PanelLeft, X } from 'lucide-react';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import { Skeleton } from '@/components/ui/skeleton';
import { Button } from '@/components/ui/button';
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet';
import { cn } from '@/lib/utils';
import { useRunnableWebApps } from '@/hooks/agent/use-runnable-webapps';
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
  const { items, isLoading } = useRunnableWebApps();
  const [isCollapsed, setIsCollapsed] = useState<boolean>(() => getSidebarCollapsed('app', false));
  const [mobileDrawerOpen, setMobileDrawerOpen] = useState(false);

  useEffect(() => {
    saveSidebarCollapsed('app', isCollapsed);
  }, [isCollapsed]);

  const navItems = useMemo(
    () =>
      items.map(item => {
        const title = item.meta_data.title;
        const preview = toPreviewData(item);
        return {
          id: item.web_app_id,
          title,
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

  const navList = (
    <div className="flex h-0 grow flex-col">
      <div
        className={cn('border-b px-3 py-3', isCollapsed && 'flex items-center justify-center px-1')}
      >
        {isCollapsed ? (
          <AppWindow className="h-4 w-4 text-muted-foreground" />
        ) : (
          <div className="min-w-0">
            <div className="text-sm font-semibold leading-5">{t('appCenter.title')}</div>
            <div className="mt-0.5 text-xs leading-4 text-muted-foreground">
              {t('appCenter.sidebarSubtitle')}
            </div>
          </div>
        )}
      </div>
      <div className="w-full flex-1 overflow-y-auto">
        <div className={cn('w-full space-y-0.5 p-1.5', isCollapsed && 'px-1')}>
          <Link
            href="/console/work/app"
            onClick={() => setMobileDrawerOpen(false)}
            className={cn(
              'flex w-full items-center rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors',
              isCollapsed && 'px-1 justify-center',
              pathname === '/console/work/app'
                ? 'bg-background text-foreground shadow-sm ring-1 ring-border/70'
                : 'text-muted-foreground hover:bg-background/70 hover:text-foreground'
            )}
            title={t('appCenter.allApps')}
          >
            <AppWindow className="size-4 shrink-0" />
            <span
              className={cn(
                'truncate transition-all duration-300 overflow-hidden line-clamp-1 text-ellipsis',
                isCollapsed ? 'opacity-0 w-0 ml-0' : 'ml-2'
              )}
            >
              {t('appCenter.allApps')}
            </span>
          </Link>
          {isLoading
            ? Array.from({ length: 6 }).map((_, index) => (
                <div
                  key={`app-skeleton-${index}`}
                  className={cn('rounded-md px-2.5 py-1.5', isCollapsed && 'px-1')}
                >
                  <div className={cn('flex items-center gap-2', isCollapsed && 'justify-center')}>
                    <Skeleton className="size-6 rounded-md" />
                    {!isCollapsed ? <Skeleton className="h-4 w-32" /> : null}
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
                    className={cn(
                      'flex w-full items-center rounded-md px-2.5 py-1.5 text-xs transition-colors',
                      isCollapsed && 'px-1 justify-center',
                      isActive
                        ? 'bg-background text-foreground shadow-sm ring-1 ring-border/70'
                        : 'text-muted-foreground hover:bg-background/70 hover:text-foreground'
                    )}
                    title={item.title}
                  >
                    <IconPreview
                      iconType={item.preview.iconType}
                      src={item.preview.src}
                      icon={item.preview.textIcon}
                      iconBackground={item.preview.iconBackground}
                      editable={false}
                      size="xs"
                    />
                    <span
                      className={cn(
                        'truncate transition-all text-[11px] leading-4 duration-300 line-clamp-2 text-ellipsis break-before-all',
                        isCollapsed ? 'opacity-0 w-0 overflow-hidden' : 'ml-2'
                      )}
                    >
                      {item.title}
                    </span>
                  </Link>
                );
              })}
        </div>
      </div>
    </div>
  );

  return (
    <div className="h-full w-full flex min-w-0 min-h-0">
      <aside
        className={cn(
          'hidden shrink-0 flex-col border-r bg-muted/10 transition-all duration-300 md:flex',
          isCollapsed ? 'w-10' : 'w-56'
        )}
      >
        {navList}
        <div className="border-t p-1">
          <Button
            onClick={() => setIsCollapsed(prev => !prev)}
            variant="ghost"
            size="xs"
            aria-label={isCollapsed ? tNav('expand') : tNav('collapse')}
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
          aria-description="app list"
          side="left"
          className="w-full sm:max-w-sm p-0 h-full flex flex-col gap-0"
          showClose={false}
        >
          <SheetHeader className="px-4 py-2 border-b h-14 flex flex-row items-center space-y-0 justify-between w-full">
            <SheetTitle className="sr-only" />
            <Logo routerToHome={false} showName={false} />
            <Button variant="ghost" size="sm" onClick={() => setMobileDrawerOpen(false)}>
              <X className="h-4 w-4" />
            </Button>
          </SheetHeader>
          <div className="h-0 grow overflow-y-auto">
            <div className="p-2 space-y-1">
              <Link
                href="/console/work/app"
                onClick={() => setMobileDrawerOpen(false)}
                className={cn(
                  'flex items-center rounded-md px-3 py-2 text-sm transition-colors w-full',
                  pathname === '/console/work/app'
                    ? 'bg-primary/10 text-primary'
                    : 'hover:bg-accent text-foreground'
                )}
              >
                <AppWindow className="size-4 shrink-0" />
                <span className="ml-2 truncate">{t('appCenter.allApps')}</span>
              </Link>
              {isLoading
                ? Array.from({ length: 6 }).map((_, index) => (
                    <div key={`app-mobile-skeleton-${index}`} className="rounded-md px-3 py-2">
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
                        className={cn(
                          'flex items-center rounded-md px-3 py-2 w-full text-sm transition-colors',
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
                          editable={false}
                          size="xs"
                        />
                        <span className="ml-2 truncate">{item.title}</span>
                      </Link>
                    );
                  })}
            </div>
          </div>
        </SheetContent>
      </Sheet>
    </div>
  );
}
