'use client';

import Link from 'next/link';
import { Check, Plus } from 'lucide-react';
import { toast } from 'sonner';
import { ConnectorIcon } from '@/components/connectors/connector-icon';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { marketText } from '@/components/zgi-market/market-format';
import {
  usableMarketConnections,
  useZgiMarketSubjectSelection,
} from '@/hooks/use-zgi-market-subject-tools';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { MarketToolCandidate, MarketToolSubjectType } from '@/services/types/zgi-market';
import { getErrorMessage } from '@/utils/error-notifications';

interface ZgiMarketConnectorPickerGroupProps {
  subjectType: MarketToolSubjectType;
  subjectId: string;
  readOnly?: boolean;
}

/**
 * The organization's activated ZGI Market servers as a group of a connector picker, in the rows
 * the picker gives its own connectors. Adding or removing one replaces the subject's market
 * binding at once.
 */
export function ZgiMarketConnectorPickerGroup({
  subjectType,
  subjectId,
  readOnly = false,
}: ZgiMarketConnectorPickerGroupProps) {
  const t = useT();
  const { locale } = useLocale();
  const selection = useZgiMarketSubjectSelection(subjectType, subjectId);
  const headingId = `zgi-market-picker-${subjectId}`;

  const toggle = async (listing: MarketToolCandidate, name: string, selected: boolean) => {
    try {
      if (selected) {
        await selection.remove(listing.listing_id);
        toast.success(t('market.tools.removedToast', { name }));
        return;
      }
      const usable = usableMarketConnections(listing, selection.connections);
      await selection.add(listing, usable[0]?.id);
      toast.success(t('market.tools.addedToast', { name }));
    } catch (error) {
      toast.error(t('market.tools.saveFailed', { message: getErrorMessage(error) }));
    }
  };

  return (
    <section className="mt-6" aria-labelledby={headingId} data-testid="zgi-market-picker-group">
      <div className="mb-3">
        <div className="flex items-center gap-2">
          <h3 id={headingId} className="text-sm font-semibold">
            {t('market.tools.group')}
          </h3>
          <Badge variant="subtle">{selection.listings.length}</Badge>
        </div>
        <p className="mt-1 text-xs leading-5 text-muted-foreground">
          {t('market.tools.groupHint')}
        </p>
      </div>

      {!selection.gatewayConfigured ? (
        <p
          role="status"
          className="mb-3 rounded-lg border border-warning/30 px-3 py-2 text-xs leading-5 text-muted-foreground"
        >
          {t('market.tools.gatewayMissing')}
        </p>
      ) : null}

      {selection.isLoading ? (
        <div className="space-y-3">
          <Skeleton className="h-[82px] rounded-xl" />
          <Skeleton className="h-[82px] rounded-xl" />
        </div>
      ) : selection.isError ? (
        <p className="text-xs text-destructive" role="alert">
          {t('market.tools.loadFailed')}
        </p>
      ) : selection.listings.length === 0 ? (
        <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-dashed bg-background/60 px-5 py-6 text-sm text-muted-foreground">
          <span>{t('market.tools.empty')}</span>
          <Link
            href="/dashboard/market/catalog?kind=mcp"
            className="font-medium text-primary hover:underline"
          >
            {t('market.tools.browse')}
          </Link>
        </div>
      ) : (
        <div className="space-y-3">
          {selection.listings.map(listing => {
            const name = marketText(listing.title, locale) || listing.slug;
            const selected = selection.items.some(item => item.listing_id === listing.listing_id);
            const needsConnection =
              listing.required_connectors.length > 0 &&
              usableMarketConnections(listing, selection.connections).length === 0;
            const connectorName = listing.required_connectors
              .map(connector => marketText(connector.title, locale) || connector.slug)
              .join(' / ');
            return (
              <article
                key={listing.listing_id}
                className="min-w-0 rounded-xl border bg-background p-3.5 shadow-xs"
              >
                <div className="grid min-w-0 grid-cols-[40px_minmax(0,1fr)] items-center gap-3 sm:grid-cols-[40px_minmax(0,1fr)_minmax(220px,320px)] sm:gap-4">
                  <ConnectorIcon
                    connectorId={`zgi-market:${listing.slug}`}
                    name={name}
                    classification="connector"
                  />
                  <div className="min-w-0 flex-1">
                    <h3 className="truncate text-sm font-semibold" title={name}>
                      {name}
                    </h3>
                    <p className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
                      {needsConnection && !selected ? (
                        <>
                          {t('market.tools.needsConnection', { name: connectorName })}
                          {' · '}
                          <Link
                            href={`/dashboard/market/catalog/mcp/${listing.slug}`}
                            className="font-medium text-primary hover:underline"
                          >
                            {t('market.tools.connect')}
                          </Link>
                        </>
                      ) : (
                        t('market.tools.rowDescription', {
                          kind: t('market.catalog.kinds.mcp'),
                          count: listing.tools.length,
                        })
                      )}
                    </p>
                    <div className="mt-2 flex min-w-0 flex-wrap gap-1">
                      {listing.tools.slice(0, 4).map(tool => (
                        <Badge
                          key={tool.name}
                          variant={tool.risk_level === 'read' ? 'subtle' : 'warning'}
                          className="max-w-full font-normal"
                          title={marketText(tool.description, locale) || tool.name}
                        >
                          <span className="truncate font-mono">{tool.name}</span>
                          <span className="shrink-0">
                            {' · '}
                            {t(`market.catalog.risk.${tool.risk_level}`)}
                          </span>
                        </Badge>
                      ))}
                      {listing.tools.length > 4 ? (
                        <Badge variant="subtle" className="font-normal">
                          +{listing.tools.length - 4}
                        </Badge>
                      ) : null}
                    </div>
                  </div>
                  <Button
                    type="button"
                    variant="outline"
                    className={cn(
                      'col-span-2 min-h-[54px] w-full min-w-0 shrink-0 justify-between rounded-lg px-3 sm:col-span-1',
                      selected && 'border-primary/25 bg-primary/[0.03]'
                    )}
                    disabled={readOnly || selection.isSaving || (needsConnection && !selected)}
                    aria-pressed={selected}
                    aria-label={
                      selected
                        ? t('market.tools.remove', { name })
                        : t('market.tools.add', { name })
                    }
                    onClick={() => void toggle(listing, name, selected)}
                  >
                    <span className="min-w-0 text-left">
                      <span className="block truncate text-sm font-medium">
                        {selected ? t('market.tools.added') : t('market.tools.addAction')}
                      </span>
                      <span className="mt-0.5 block truncate text-[11px] font-normal text-muted-foreground">
                        {selected
                          ? t('market.tools.clickToRemove')
                          : t('market.tools.billedPerCall')}
                      </span>
                    </span>
                    {selected ? (
                      <Check aria-hidden="true" className="ml-2 size-4 shrink-0 text-primary" />
                    ) : (
                      <Plus aria-hidden="true" className="ml-2 size-4 shrink-0" />
                    )}
                  </Button>
                </div>
              </article>
            );
          })}
        </div>
      )}
    </section>
  );
}
