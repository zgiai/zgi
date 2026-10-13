'use client';

import Link from 'next/link';
import { Check } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { useT } from '@/i18n';
import type { MarketListing } from '@/services/types/zgi-market';
import { highestPrice, lowestPrice, marketAmount, marketText } from './market-format';
import { MarketKindIcon } from './market-kind-icon';

interface ZgiMarketCardProps {
  listing: MarketListing;
  locale: string;
  activated: boolean;
}

/** One listing of the market as a link to its page. */
export default function ZgiMarketCard({ listing, locale, activated }: ZgiMarketCardProps) {
  const t = useT();
  const price = lowestPrice(listing);
  const title = marketText(listing.card.title, locale) || listing.slug;
  const providers = [
    ...new Set(listing.offerings.map(offering => offering.provider).filter(Boolean)),
  ];
  const source =
    providers.length > 0
      ? providers.join(' · ')
      : listing.plugin?.manifest?.author || marketText(listing.primary_category?.name, locale);
  const facts = [source, listing.current_version?.label].filter(Boolean);
  return (
    <Link
      href={`/dashboard/market/catalog/${listing.kind}/${listing.slug}`}
      className="group flex h-full flex-col gap-3 rounded-lg border bg-background p-4 transition-colors hover:border-primary/40 hover:bg-accent/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <div className="flex items-start gap-3">
        <MarketKindIcon kind={listing.kind} />
        <div className="min-w-0 flex-1">
          <h3 className="line-clamp-2 break-words text-sm font-semibold leading-5">{title}</h3>
          <p className="mt-0.5 truncate text-xs text-muted-foreground">{facts.join(' · ')}</p>
        </div>
        <Badge variant="secondary" className="shrink-0">
          {t(`market.catalog.kinds.${listing.kind}`)}
        </Badge>
      </div>
      <p className="line-clamp-3 min-h-[3.75rem] text-xs leading-5 text-muted-foreground">
        {marketText(listing.card.description, locale) || marketText(listing.card.subtitle, locale)}
      </p>
      <div className="mt-auto flex items-center justify-between gap-2 border-t pt-3 text-xs">
        <span className="font-medium tabular-nums text-foreground">
          {!price
            ? ''
            : price.unit_price_micros === 0
              ? // Free only when nothing in the listing is priced, such as every tool of a server.
                (highestPrice(listing)?.unit_price_micros ?? 0) === 0
                ? t('market.catalog.free')
                : t('market.catalog.partlyFree')
              : t(`market.catalog.from.${marketAmount(price).unit}`, {
                  amount: marketAmount(price).amount,
                })}
        </span>
        {activated ? (
          <span className="inline-flex shrink-0 items-center gap-1 font-medium text-success">
            <Check className="size-3.5" aria-hidden="true" />
            {t('market.catalog.activated')}
          </span>
        ) : null}
      </div>
    </Link>
  );
}
