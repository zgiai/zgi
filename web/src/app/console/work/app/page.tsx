'use client';

import Link from 'next/link';
import { useEffect, useMemo, useState } from 'react';
import { ArrowRight, AppWindow, SearchX } from 'lucide-react';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { SearchInput } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { useRunnableWebApps } from '@/hooks/agent/use-runnable-webapps';
import { useT } from '@/i18n/translations';
import { ICON_BG } from '@/lib/config';

const RECENT_WEBAPP_STORAGE_KEY = 'zgi:webapp:recent';

export default function ConsoleWorkAppHomePage() {
  const t = useT('webapp');
  const { items, isLoading } = useRunnableWebApps();
  const [search, setSearch] = useState('');
  const [recentIds, setRecentIds] = useState<string[]>([]);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    try {
      const raw = window.localStorage.getItem(RECENT_WEBAPP_STORAGE_KEY);
      setRecentIds(raw ? (JSON.parse(raw) as string[]) : []);
    } catch {
      setRecentIds([]);
    }
  }, []);

  const cards = useMemo(
    () =>
      items.map(item => {
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
            // Ignore
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
      }),
    [items]
  );

  const normalizedSearch = search.trim().toLowerCase();
  const filteredCards = useMemo(() => {
    if (!normalizedSearch) return cards;
    return cards.filter(card => {
      const haystack = `${card.title} ${card.desc ?? ''}`.toLowerCase();
      return haystack.includes(normalizedSearch);
    });
  }, [cards, normalizedSearch]);

  const recentCard = useMemo(() => {
    if (normalizedSearch) return null;
    const recentIdSet = new Set(recentIds);
    return cards.find(card => recentIdSet.has(card.id)) ?? null;
  }, [cards, normalizedSearch, recentIds]);

  const spotlightCard = recentCard ?? filteredCards[0] ?? null;
  const spotlightMode: 'recent' | 'quick-start' | null = spotlightCard
    ? recentCard
      ? 'recent'
      : 'quick-start'
    : null;

  return (
    <div className="h-full w-full overflow-auto bg-background">
      <div className="mx-auto w-full max-w-6xl px-6 py-6">
        <div className="mb-6 flex flex-col gap-4 border-b pb-5 lg:flex-row lg:items-end lg:justify-between">
          <div className="min-w-0">
            <div className="mb-2 flex items-center gap-2">
              <div className="flex h-8 w-8 items-center justify-center rounded-lg border bg-muted/30">
                <AppWindow className="h-4 w-4 text-muted-foreground" />
              </div>
              <Badge variant="secondary" className="rounded-full px-2 text-xs">
                {t('appCenter.totalApps', { count: cards.length })}
              </Badge>
            </div>
            <h1 className="text-2xl font-semibold tracking-tight">{t('appCenter.title')}</h1>
            <p className="mt-1 text-sm leading-6 text-muted-foreground">
              {t('appCenter.subtitle')}
            </p>
            <div className="mt-3 flex flex-wrap items-center gap-2 text-xs font-medium">
              <Badge variant="secondary" className="rounded-full px-2">
                {t('appCenter.totalApps', { count: cards.length })}
              </Badge>
              <Badge variant="subtle" className="rounded-full px-2">
                {t('appCenter.filteredApps', { count: filteredCards.length })}
              </Badge>
              {normalizedSearch ? (
                <Badge variant="outline" className="rounded-full px-2">
                  {t('appCenter.searchKeyword', { keyword: search.trim() })}
                </Badge>
              ) : null}
            </div>
          </div>
          <div className="w-full space-y-2 lg:max-w-sm">
            <SearchInput
              value={search}
              onChange={event => setSearch(event.target.value)}
              placeholder={t('appCenter.searchPlaceholder')}
              className="h-9 bg-background"
            />
            <div className="flex items-center justify-between text-xs text-muted-foreground">
              <span>
                {normalizedSearch
                  ? t('appCenter.searchSummary', {
                      shown: filteredCards.length,
                      total: cards.length,
                    })
                  : t('appCenter.searchHint')}
              </span>
              {normalizedSearch ? (
                <Button variant="ghost" size="xs" onClick={() => setSearch('')}>
                  {t('appCenter.clearSearch')}
                </Button>
              ) : null}
            </div>
          </div>
        </div>
        {isLoading ? (
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
        ) : cards.length === 0 ? (
          <Card className="border-dashed bg-muted/20">
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
        ) : filteredCards.length === 0 ? (
          <Card className="border-dashed bg-muted/20">
            <CardHeader className="items-center py-12 text-center">
              <div className="mb-2 flex h-12 w-12 items-center justify-center rounded-full bg-muted">
                <SearchX className="h-6 w-6 text-muted-foreground" />
              </div>
              <CardTitle>{t('appCenter.noSearchResults')}</CardTitle>
              <CardDescription className="max-w-md">
                {t('appCenter.noSearchResultsDescription')}
              </CardDescription>
              <div className="mt-4">
                <Button variant="outline" onClick={() => setSearch('')}>
                  {t('appCenter.clearSearch')}
                </Button>
              </div>
            </CardHeader>
          </Card>
        ) : (
          <div className="space-y-5">
            {spotlightCard && spotlightMode ? (
              <Card className="border-border/80 shadow-sm">
                <CardHeader className="pb-4">
                  <div className="flex flex-col gap-4 md:flex-row md:items-start md:justify-between">
                    <div className="min-w-0">
                      <Badge variant={spotlightMode === 'recent' ? 'success' : 'subtle'} className="mb-3 rounded-full px-2">
                        {spotlightMode === 'recent'
                          ? t('appCenter.recentlyUsed')
                          : t('appCenter.quickStart')}
                      </Badge>
                      <div className="flex items-start gap-3">
                        <IconPreview
                          iconType={spotlightCard.iconType}
                          src={spotlightCard.src}
                          icon={spotlightCard.icon}
                          iconBackground={spotlightCard.iconBackground}
                          editable={false}
                          size="sm"
                        />
                        <div className="min-w-0">
                          <CardTitle className="line-clamp-1 text-xl">{spotlightCard.title}</CardTitle>
                          <CardDescription className="mt-2 max-w-2xl text-sm leading-6">
                            {spotlightCard.desc ||
                              (spotlightMode === 'recent'
                                ? t('appCenter.recentFallbackDescription')
                                : t('appCenter.quickStartFallbackDescription'))}
                          </CardDescription>
                          <div className="mt-3 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                            <Badge variant="subtle" className="rounded-full px-2">
                              {t('appCenter.ready')}
                            </Badge>
                            <span>{t('appCenter.appId')}: {spotlightCard.id.slice(0, 8)}</span>
                          </div>
                        </div>
                      </div>
                    </div>
                    <div className="flex shrink-0 gap-2">
                      <Button asChild>
                        <Link href={`/console/work/app/${spotlightCard.id}`}>
                          {spotlightMode === 'recent'
                            ? t('appCenter.openRecent')
                            : t('appCenter.openQuickStart')}
                          <ArrowRight className="h-3.5 w-3.5" />
                        </Link>
                      </Button>
                      <Button asChild variant="outline">
                        <Link href="/console/work/app">{t('appCenter.browseApps')}</Link>
                      </Button>
                    </div>
                  </div>
                </CardHeader>
              </Card>
            ) : null}

            <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
              {filteredCards.map(card => (
                <Link key={card.id} href={`/console/work/app/${card.id}`}>
                  <Card className="group h-full border-border/80 transition-all hover:border-primary/40 hover:shadow-sm">
                    <CardHeader className="p-4 pb-3">
                      <div className="flex w-full items-start gap-3">
                        <IconPreview
                          iconType={card.iconType}
                          src={card.src}
                          icon={card.icon}
                          iconBackground={card.iconBackground}
                          editable={false}
                          size="sm"
                        />
                        <div className="w-0 grow pt-0.5">
                          <CardTitle className="line-clamp-1 w-full break-all text-base">
                            {card.title}
                          </CardTitle>
                          <CardDescription className="mt-1 line-clamp-2 w-full break-all text-xs leading-5 text-muted-foreground">
                            {card.desc || '--'}
                          </CardDescription>
                        </div>
                      </div>
                    </CardHeader>
                    <CardContent className="flex items-center justify-between px-4 pb-4 pt-0">
                      <div className="space-y-1">
                        <span className="block text-xs text-muted-foreground">
                          {t('appCenter.ready')}
                        </span>
                        <span className="block text-[11px] text-muted-foreground/80">
                          {t('appCenter.appId')}: {card.id.slice(0, 8)}
                        </span>
                      </div>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-8 gap-1 px-2 text-primary opacity-80 group-hover:opacity-100"
                        tabIndex={-1}
                      >
                        {t('appCenter.openApp')}
                        <ArrowRight className="h-3.5 w-3.5" />
                      </Button>
                    </CardContent>
                  </Card>
                </Link>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
