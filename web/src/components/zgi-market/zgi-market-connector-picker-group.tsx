'use client';

import { useState } from 'react';
import Link from 'next/link';
import { Check, Plus, SlidersHorizontal } from 'lucide-react';
import { toast } from 'sonner';
import { ConnectorIcon } from '@/components/connectors/connector-icon';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { marketText } from '@/components/zgi-market/market-format';
import { ZgiMarketCallPrice } from '@/components/zgi-market/zgi-market-call-price';
import { ZgiMarketOperationsDialog } from '@/components/zgi-market/zgi-market-operations-dialog';
import {
  bindableOperations,
  candidateToolChips,
  marketItemHref,
  matchesMarketRowFilter,
  usableMarketConnections,
  useZgiMarketSubjectSelection,
  type MarketRowFilter,
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
  /** The search and status filter of the picker the group is shown in. */
  filter?: MarketRowFilter;
}

/**
 * The organization's activated ZGI Market servers and API offerings as a group of a connector
 * picker, in the rows the picker gives its own connectors. Adding or removing one replaces the
 * subject's market binding at once.
 */
export function ZgiMarketConnectorPickerGroup({
  subjectType,
  subjectId,
  readOnly = false,
  filter,
}: ZgiMarketConnectorPickerGroupProps) {
  const t = useT();
  const { locale } = useLocale();
  const selection = useZgiMarketSubjectSelection(subjectType, subjectId);
  const headingId = `zgi-market-picker-${subjectId}`;
  // The API offering whose operations are being chosen, by its activation.
  const [choosing, setChoosing] = useState<number | null>(null);
  const filtering = Boolean(filter && (filter.query.trim() || filter.status !== 'all'));
  const listings = filter
    ? selection.listings.filter(listing =>
        matchesMarketRowFilter(listing, selection.connections, filter)
      )
    : selection.listings;

  // A search or a status filter that leaves no market row leaves no market group either.
  if (filtering && !selection.isLoading && !selection.isError && listings.length === 0) {
    return null;
  }

  const toggle = async (listing: MarketToolCandidate, name: string, selected: boolean) => {
    try {
      if (selected) {
        await selection.remove(listing.subscription_id);
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

  const chooseOperations = async (
    listing: MarketToolCandidate,
    name: string,
    operations: string[] | undefined
  ) => {
    try {
      const bound = selection.items.find(item => item.subscription_id === listing.subscription_id);
      if (bound) {
        await selection.setOperations(listing.subscription_id, operations);
        toast.success(t('market.tools.operations.savedToast', { name }));
      } else {
        const usable = usableMarketConnections(listing, selection.connections);
        await selection.add(listing, usable[0]?.id, operations);
        toast.success(t('market.tools.addedToast', { name }));
      }
      setChoosing(null);
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
          <Badge variant="subtle">{listings.length}</Badge>
          {/* The way to the market also when something is activated: what is missing is there. */}
          {listings.length > 0 ? (
            <Link
              href="/console/market/catalog"
              className="ml-auto text-xs font-medium text-primary hover:underline"
            >
              {t('market.tools.browse')}
            </Link>
          ) : null}
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
      ) : listings.length === 0 ? (
        <div className="flex flex-wrap items-center justify-between gap-2 rounded-xl border border-dashed bg-background/60 px-5 py-6 text-sm text-muted-foreground">
          <span>{t('market.tools.empty')}</span>
          <Link href="/console/market/catalog" className="font-medium text-primary hover:underline">
            {t('market.tools.browse')}
          </Link>
        </div>
      ) : (
        <div className="space-y-3">
          {listings.map(listing => {
            const title = marketText(listing.title, locale) || listing.slug;
            // Two offerings of one API listing share its title; the offering tells them apart.
            const name =
              listing.kind === 'api' && listing.offering_slug
                ? `${title} · ${listing.offering_slug}`
                : title;
            const selected = selection.items.some(
              item => item.subscription_id === listing.subscription_id
            );
            const unbindable = listing.tools.length === 0;
            const needsConnection =
              listing.required_connectors.length > 0 &&
              usableMarketConnections(listing, selection.connections).length === 0;
            const connectorName = listing.required_connectors
              .map(connector => marketText(connector.title, locale) || connector.slug)
              .join(' / ');
            const bound = selection.items.find(
              item => item.subscription_id === listing.subscription_id
            );
            // An offering with one bindable operation has nothing to choose between.
            const choosable = listing.kind === 'api' && bindableOperations(listing).length > 1;
            return (
              <article
                key={listing.subscription_id}
                data-market-kind={listing.kind}
                className="min-w-0 rounded-xl border bg-background p-3.5 shadow-xs"
              >
                <div className="grid min-w-0 grid-cols-[40px_minmax(0,1fr)] items-center gap-3 sm:grid-cols-[40px_minmax(0,1fr)_minmax(220px,320px)] sm:gap-4">
                  <ConnectorIcon
                    connectorId={`zgi-market:${listing.offering_slug ?? listing.slug}`}
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
                            href={marketItemHref(listing)}
                            className="font-medium text-primary hover:underline"
                          >
                            {t('market.tools.connect')}
                          </Link>
                        </>
                      ) : unbindable ? (
                        t('market.tools.apiNotBindable')
                      ) : (
                        t('market.tools.rowDescription', {
                          kind: t(`market.catalog.kinds.${listing.kind}`),
                          count: listing.tools.length,
                        })
                      )}
                    </p>
                    <div className="mt-2 flex min-w-0 flex-wrap gap-1">
                      {candidateToolChips(listing, locale)
                        .slice(0, 4)
                        .map(tool => (
                          <Badge
                            key={tool.key}
                            variant={tool.risk === 'read' ? 'subtle' : 'warning'}
                            className="max-w-full font-normal"
                            title={tool.hint}
                          >
                            <span className="truncate font-mono">{tool.label}</span>
                            <span className="shrink-0">
                              {' · '}
                              {t(`market.catalog.risk.${tool.risk}`)}
                            </span>
                          </Badge>
                        ))}
                      {listing.tools.length > 4 ? (
                        <Badge variant="subtle" className="font-normal">
                          +{listing.tools.length - 4}
                        </Badge>
                      ) : null}
                    </div>
                    <div className="mt-2 flex flex-wrap items-center gap-2">
                      <ZgiMarketCallPrice listing={listing} />
                      {choosable ? (
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          className="h-7 gap-1 px-2 text-xs"
                          disabled={
                            readOnly || selection.isSaving || (needsConnection && !selected)
                          }
                          onClick={() => setChoosing(listing.subscription_id)}
                        >
                          <SlidersHorizontal className="size-3.5" aria-hidden="true" />
                          {t('market.tools.operations.choose')}
                        </Button>
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
                    disabled={
                      readOnly ||
                      selection.isSaving ||
                      ((needsConnection || unbindable) && !selected)
                    }
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
                {choosable ? (
                  <ZgiMarketOperationsDialog
                    open={choosing === listing.subscription_id}
                    onOpenChange={next => setChoosing(next ? listing.subscription_id : null)}
                    listing={listing}
                    name={name}
                    selected={
                      bound && !bound.all_operations
                        ? (bound.operations ?? []).map(operation => operation.key)
                        : undefined
                    }
                    saving={selection.isSaving}
                    confirmLabel={
                      bound ? t('market.tools.operations.save') : t('market.tools.addAction')
                    }
                    onConfirm={operations => void chooseOperations(listing, name, operations)}
                  />
                ) : null}
              </article>
            );
          })}
        </div>
      )}
    </section>
  );
}
