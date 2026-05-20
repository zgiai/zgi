'use client';

import React, { useMemo, useState, useCallback } from 'react';
import { usePathname, useParams } from 'next/navigation';
import { useT } from '@/i18n';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useProviders } from '@/hooks';
import type { ProviderItem } from '@/services/types/provider';
import { ProviderSidebarItem } from '@/components/providers/provider-sidebar-item';
import { ProviderSidebarSyncButton } from '@/components/providers/provider-sidebar-sync-button';
import { IS_CLOUD } from '@/lib/config';
import { useProviderI18n } from '@/hooks/provider/use-provider-i18n';
import { useProviderAvailableCounts } from '@/hooks/provider/use-provider-available-counts';

export default function ProviderLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const params = useParams();
  const t = useT('aiProviders');
  const getProviderName = useProviderI18n();

  const [search, setSearch] = useState('');
  const debouncedSearch = useDebouncedValue(search, 300);

  const { items, isLoading } = useProviders({ limit: 50, initialPage: 1 });

  const filtered = useMemo(() => {
    const q = debouncedSearch.trim().toLowerCase();
    if (!q) return items;
    return items.filter(it =>
      [getProviderName(it.provider, it.provider_name), it.provider_name, it.provider].some(s =>
        s?.toLowerCase().includes(q)
      )
    );
  }, [debouncedSearch, getProviderName, items]);

  const enabled = useMemo(() => filtered.filter(it => it.is_enabled), [filtered]);
  const disabled = useMemo(() => filtered.filter(it => !it.is_enabled), [filtered]);
  const { counts: availableCounts } = useProviderAvailableCounts(filtered);

  // Determine current provider from route params or pathname segments for exact match
  const currentProvider = useMemo(() => {
    const raw = params?.providerId;
    let fromParams: string | undefined;
    if (typeof raw === 'string') {
      fromParams = raw;
    } else if (Array.isArray(raw) && raw.length > 0) {
      fromParams = raw[0];
    }

    if (fromParams) return fromParams;

    const segments = pathname?.split('/').filter(Boolean) ?? [];
    const idx = segments.indexOf('provider');
    const candidate = idx !== -1 && segments[idx + 1] ? segments[idx + 1] : undefined;
    return candidate ? decodeURIComponent(candidate) : undefined;
  }, [params, pathname]);

  const isActive = useCallback(
    (it: ProviderItem) => currentProvider === it.provider,
    [currentProvider]
  );

  return (
    <div className="flex h-full w-full">
      <aside className="flex-col flex w-72 shrink-0 border-r bg-background h-full">
        <div className="p-3 border-b space-y-1.5">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold">{t('sidebar.title')}</h2>
            {!IS_CLOUD && <ProviderSidebarSyncButton />}
          </div>
          <Input
            className="h-8 text-xs"
            placeholder={t('sidebar.searchPlaceholder')}
            value={search}
            onChange={e => setSearch(e.target.value)}
          />
        </div>

        <div className="space-y-3 h-0 grow overflow-auto px-3 pt-3 pb-3 bg-muted/30">
          {isLoading ? (
            Array.from({ length: 8 }).map((_, i) => <Skeleton key={i} className="h-10 w-full" />)
          ) : (
            <>
              {/* Enabled Section */}
              <section className="space-y-1.5">
                <div className="flex items-center justify-between px-1">
                  <h3 className="text-[11px] font-semibold text-foreground uppercase tracking-wide">
                    {t('sidebar.enabled')}
                  </h3>
                  {enabled.length > 0 && (
                    <span className="text-[11px] text-muted-foreground font-medium">
                      {enabled.length}
                    </span>
                  )}
                </div>
                <div className="space-y-1.5">
                  {enabled.length === 0 ? (
                    <div className="flex flex-col items-center justify-center py-6 px-3 text-center">
                      <div className="text-muted-foreground/60 mb-1">
                        <svg
                          className="w-8 h-8 mx-auto"
                          fill="none"
                          stroke="currentColor"
                          viewBox="0 0 24 24"
                        >
                          <path
                            strokeLinecap="round"
                            strokeLinejoin="round"
                            strokeWidth={1.5}
                            d="M20 13V6a2 2 0 00-2-2H6a2 2 0 00-2 2v7m16 0v5a2 2 0 01-2 2H6a2 2 0 01-2-2v-5m16 0h-2.586a1 1 0 00-.707.293l-2.414 2.414a1 1 0 01-.707.293h-3.172a1 1 0 01-.707-.293l-2.414-2.414A1 1 0 006.586 13H4"
                          />
                        </svg>
                      </div>
                      <p className="text-[11px] text-muted-foreground">{t('sidebar.empty')}</p>
                    </div>
                  ) : (
                    enabled.map(it => (
                      <ProviderSidebarItem
                        key={it.id}
                        provider={it}
                        availableModelCount={availableCounts[it.provider] ?? 0}
                        isActive={isActive(it)}
                      />
                    ))
                  )}
                </div>
              </section>

              {/* Divider */}
              <div className="relative">
                <div className="absolute inset-0 flex items-center">
                  <div className="w-full border-t border-border/50" />
                </div>
              </div>

              {/* Disabled Section */}
              <section className="space-y-1.5 pt-1.5">
                <div className="flex items-center justify-between px-1">
                  <h3 className="text-[11px] font-semibold text-muted-foreground uppercase tracking-wide">
                    {t('sidebar.disabled')}
                  </h3>
                  {disabled.length > 0 && (
                    <span className="text-[11px] text-muted-foreground/60 font-medium">
                      {disabled.length}
                    </span>
                  )}
                </div>
                <div className="space-y-1.5">
                  {disabled.length === 0 ? (
                    <div className="flex flex-col items-center justify-center py-6 px-3 text-center">
                      <div className="text-muted-foreground/60 mb-1">
                        <svg
                          className="w-8 h-8 mx-auto"
                          fill="none"
                          stroke="currentColor"
                          viewBox="0 0 24 24"
                        >
                          <path
                            strokeLinecap="round"
                            strokeLinejoin="round"
                            strokeWidth={1.5}
                            d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"
                          />
                        </svg>
                      </div>
                      <p className="text-[11px] text-muted-foreground">{t('sidebar.empty')}</p>
                    </div>
                  ) : (
                    disabled.map(it => (
                      <ProviderSidebarItem
                        key={it.id}
                        provider={it}
                        availableModelCount={availableCounts[it.provider] ?? 0}
                        isActive={isActive(it)}
                      />
                    ))
                  )}
                </div>
              </section>
            </>
          )}
        </div>
      </aside>
      <main className="flex-1 h-full min-w-0 p-6 overflow-auto">{children}</main>
    </div>
  );
}
