'use client';

import { useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import { Store } from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { marketText } from '@/components/zgi-market/market-format';
import { useLocale } from '@/hooks/use-locale';
import {
  useBindZgiMarketSubjectTools,
  useZgiMarketSubjectTools,
  useZgiMarketToolCandidates,
} from '@/hooks/use-zgi-market';
import { useT } from '@/i18n';
import type { MarketToolSubjectType } from '@/services/types/zgi-market';
import { getErrorMessage } from '@/utils/error-notifications';

interface ZgiMarketToolsSectionProps {
  subjectType: MarketToolSubjectType;
  subjectId: string;
  readOnly?: boolean;
}

/** Selected listings by listing id, each with the connection it uses or null. */
type Selection = Record<number, number | null>;

/**
 * The ZGI Market MCP servers bound to an agent or a digital human. Saving replaces the binding at
 * once; it does not wait for the agent or digital human to be published.
 */
export function ZgiMarketToolsSection({ subjectType, subjectId, readOnly = false }: ZgiMarketToolsSectionProps) {
  const t = useT();
  const { locale } = useLocale();
  const candidates = useZgiMarketToolCandidates(Boolean(subjectId));
  const binding = useZgiMarketSubjectTools(subjectType, subjectId);
  const bind = useBindZgiMarketSubjectTools(subjectType, subjectId);
  const [selection, setSelection] = useState<Selection>({});

  const saved = useMemo<Selection>(() => {
    const result: Selection = {};
    for (const item of binding.data?.items ?? []) {
      result[item.listing_id] = item.connection_id ?? null;
    }
    return result;
  }, [binding.data]);

  useEffect(() => {
    setSelection(saved);
  }, [saved]);

  const dirty = JSON.stringify(selection) !== JSON.stringify(saved);
  const listings = candidates.data?.listings ?? [];
  const connections = candidates.data?.connections ?? [];
  const count = Object.keys(selection).length;

  const toggle = (listingId: number, checked: boolean) => {
    setSelection(current => {
      const next = { ...current };
      if (checked) {
        next[listingId] = null;
      } else {
        delete next[listingId];
      }
      return next;
    });
  };

  const save = () => {
    const items = Object.entries(selection).map(([listingId, connectionId]) => ({
      listing_id: Number(listingId),
      ...(connectionId ? { connection_id: connectionId } : {}),
    }));
    bind.mutate(items, {
      onSuccess: () => toast.success(t('market.tools.saved')),
      onError: error => toast.error(t('market.tools.saveFailed', { message: getErrorMessage(error) })),
    });
  };

  return (
    <section className="space-y-3 rounded-lg border p-4" aria-labelledby={`zgi-market-tools-${subjectId}`}>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 id={`zgi-market-tools-${subjectId}`} className="flex items-center gap-2 text-sm font-semibold">
            <Store className="size-4 text-primary" aria-hidden="true" />
            {t('market.tools.title')}
            <Badge variant="subtle">{t('market.tools.selected', { count })}</Badge>
          </h3>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">{t('market.tools.description')}</p>
        </div>
        {!readOnly ? (
          <Button size="sm" disabled={!dirty || bind.isPending} onClick={save}>
            {bind.isPending ? t('market.tools.saving') : t('market.tools.save')}
          </Button>
        ) : null}
      </div>

      {binding.data && !binding.data.gateway_configured ? (
        <p className="rounded-md bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:bg-amber-950/40 dark:text-amber-200">
          {t('market.tools.gatewayMissing')}
        </p>
      ) : null}

      {candidates.isPending || binding.isPending ? (
        <div className="space-y-2">
          <Skeleton className="h-14 w-full" />
          <Skeleton className="h-14 w-full" />
        </div>
      ) : candidates.isError || binding.isError ? (
        <p className="text-xs text-destructive">{t('market.tools.loadFailed')}</p>
      ) : listings.length === 0 ? (
        <div className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-dashed p-3 text-xs text-muted-foreground">
          <span>{t('market.tools.empty')}</span>
          <Link href="/dashboard/market/catalog?kind=mcp" className="font-medium text-primary hover:underline">
            {t('market.tools.browse')}
          </Link>
        </div>
      ) : (
        <ul className="space-y-2">
          {listings.map(listing => {
            const checked = listing.listing_id in selection;
            const needed = listing.required_connectors.map(connector => connector.slug);
            const usable = connections.filter(
              connection => needed.includes(connection.connector_slug) && connection.status === 'connected'
            );
            return (
              <li key={listing.listing_id} className="rounded-md border p-3">
                <label className="flex cursor-pointer items-start gap-3">
                  <Checkbox
                    checked={checked}
                    disabled={readOnly}
                    onCheckedChange={value => toggle(listing.listing_id, value === true)}
                    aria-label={marketText(listing.title, locale) || listing.slug}
                  />
                  <span className="min-w-0 flex-1">
                    <span className="block text-sm font-medium">{marketText(listing.title, locale) || listing.slug}</span>
                    <span className="mt-1 flex flex-wrap gap-1">
                      {listing.tools.map(tool => (
                        <Badge
                          key={tool.name}
                          variant={tool.risk_level === 'read' ? 'subtle' : 'warning'}
                          className="font-mono text-[11px]"
                        >
                          {tool.name} · {t(`market.catalog.risk.${tool.risk_level}`)}
                        </Badge>
                      ))}
                    </span>
                  </span>
                </label>
                {checked && needed.length > 0 ? (
                  <div className="mt-3 flex flex-wrap items-center gap-2 pl-7 text-xs">
                    <span className="text-muted-foreground">{t('market.tools.connection')}</span>
                    {usable.length === 0 ? (
                      <Link
                        href={`/dashboard/market/catalog/mcp/${listing.slug}`}
                        className="text-primary hover:underline"
                      >
                        {t('market.tools.noConnection')}
                      </Link>
                    ) : (
                      <Select
                        disabled={readOnly}
                        value={selection[listing.listing_id] ? String(selection[listing.listing_id]) : ''}
                        onValueChange={value =>
                          setSelection(current => ({ ...current, [listing.listing_id]: Number(value) }))
                        }
                      >
                        <SelectTrigger className="h-8 w-56">
                          <SelectValue placeholder={t('market.tools.connectionPlaceholder')} />
                        </SelectTrigger>
                        <SelectContent>
                          {usable.map(connection => (
                            <SelectItem key={connection.id} value={String(connection.id)}>
                              {connection.name}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    )}
                  </div>
                ) : null}
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
