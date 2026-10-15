'use client';

import Link from 'next/link';
import { ArrowUpRight, Store } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Skeleton } from '@/components/ui/skeleton';
import { marketText } from '@/components/zgi-market/market-format';
import { ZgiMarketCallPrice } from '@/components/zgi-market/zgi-market-call-price';
import {
  candidateToolChips,
  marketItemHref,
  usableMarketConnections,
} from '@/hooks/use-zgi-market-subject-tools';
import { useLocale } from '@/hooks/use-locale';
import { useZgiMarketToolCandidates } from '@/hooks/use-zgi-market';
import { useT } from '@/i18n';
import type { MarketConnection, MarketToolCandidate } from '@/services/types/zgi-market';

function ServerRow({
  listing,
  connections,
}: {
  listing: MarketToolCandidate;
  connections: MarketConnection[];
}) {
  const t = useT();
  const { locale } = useLocale();
  const title = marketText(listing.title, locale) || listing.slug;
  const name =
    listing.kind === 'api' && listing.offering_slug ? `${title} · ${listing.offering_slug}` : title;
  const href = marketItemHref(listing);
  const skipped = (listing.operations ?? []).filter(operation => !operation.bindable).length;
  const needsConnection =
    listing.required_connectors.length > 0 &&
    usableMarketConnections(listing, connections).length === 0;
  const connectorName = listing.required_connectors
    .map(connector => marketText(connector.title, locale) || connector.slug)
    .join(' / ');
  return (
    <li
      data-market-kind={listing.kind}
      className="flex h-full flex-col rounded-xl border bg-card p-4 shadow-sm"
    >
      <div className="flex items-start gap-3">
        <span className="flex size-10 shrink-0 items-center justify-center rounded-lg border bg-muted text-primary">
          <Store className="size-5" aria-hidden="true" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="min-w-0 truncate text-sm font-semibold">{name}</h3>
            <Badge variant="outline" className="text-[10px]">
              {t(`market.catalog.kinds.${listing.kind}`)}
            </Badge>
            <Badge variant={needsConnection ? 'warning' : 'success'} className="text-[10px]">
              {needsConnection
                ? t('market.tools.needsConnection', { name: connectorName })
                : t('market.tools.activated')}
            </Badge>
          </div>
          <p className="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
            <span>{t('market.tools.toolCount', { count: listing.tools.length })}</span>
            <ZgiMarketCallPrice listing={listing} />
          </p>
          {skipped > 0 ? (
            <p className="mt-1 text-xs text-muted-foreground">
              {t('market.tools.operationsSkipped', { count: skipped })}
            </p>
          ) : null}
        </div>
      </div>
      <div className="mt-3 flex flex-wrap gap-1">
        {candidateToolChips(listing, locale)
          .slice(0, 8)
          .map(tool => (
            <Badge
              key={tool.key}
              variant={tool.risk === 'read' ? 'subtle' : 'warning'}
              className="font-mono text-[11px]"
              title={tool.hint}
            >
              {tool.label} · {t(`market.catalog.risk.${tool.risk}`)}
            </Badge>
          ))}
        {listing.tools.length > 8 ? (
          <Badge variant="subtle" className="text-[11px]">
            +{listing.tools.length - 8}
          </Badge>
        ) : null}
      </div>
      <div className="mt-auto pt-3">
        <Link
          href={href}
          className="inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline"
        >
          {needsConnection ? t('market.tools.connect') : t('market.tools.viewListing')}
          <ArrowUpRight className="size-3.5" aria-hidden="true" />
        </Link>
      </div>
    </li>
  );
}

/**
 * The organization's activated ZGI Market servers on the connector page: they are bound where
 * connectors are bound, so they are listed where connectors are listed.
 */
export function ZgiMarketConnectorCenterSection() {
  const t = useT();
  const candidates = useZgiMarketToolCandidates();
  const listings = candidates.data?.listings ?? [];
  // An instance without ZGI Market answers with an error: the page then has no market group.
  if (candidates.isError) return null;
  return (
    <section
      aria-labelledby="connector-section-zgi-market"
      data-testid="zgi-market-connector-center"
    >
      <div className="mb-3 flex items-end justify-between gap-4">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <h2 id="connector-section-zgi-market" className="text-lg font-semibold">
              {t('market.tools.group')}
            </h2>
            <Badge variant="subtle">{listings.length}</Badge>
          </div>
          <p className="mt-1 text-sm text-muted-foreground">
            {t('market.tools.centerDescription')}
          </p>
        </div>
        <Link
          href="/console/market/catalog"
          className="shrink-0 text-sm font-medium text-primary hover:underline"
        >
          {t('market.tools.browse')}
        </Link>
      </div>
      {candidates.isPending ? (
        <div className="grid gap-4 lg:grid-cols-2">
          <Skeleton className="h-32 rounded-xl" />
          <Skeleton className="h-32 rounded-xl" />
        </div>
      ) : listings.length === 0 ? (
        <div className="rounded-xl border border-dashed bg-background/60 px-5 py-8 text-center text-sm text-muted-foreground">
          {t('market.tools.centerEmpty')}
        </div>
      ) : (
        <ul className="grid auto-rows-fr gap-4 lg:grid-cols-2">
          {listings.map(listing => (
            <ServerRow
              key={listing.subscription_id}
              listing={listing}
              connections={candidates.data?.connections ?? []}
            />
          ))}
        </ul>
      )}
    </section>
  );
}
