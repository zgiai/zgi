'use client';

import Link from 'next/link';
import { Badge } from '@/components/ui/badge';
import { useT } from '@/i18n';
import type { MarketListing } from '@/services/types/zgi-market';
import { lowestPrice, marketAmount, marketText } from './market-format';

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
  return (
    <Link
      href={`/dashboard/market/catalog/${listing.kind}/${listing.slug}`}
      className="group flex h-full flex-col gap-3 rounded-lg border bg-background p-4 transition-colors hover:border-primary/40 hover:bg-accent/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
    >
      <div className="flex items-start justify-between gap-2">
        <h3 className="line-clamp-2 min-w-0 break-words text-sm font-semibold">{title}</h3>
        <Badge variant="secondary" className="shrink-0">
          {t(`market.catalog.kinds.${listing.kind}`)}
        </Badge>
      </div>
      <p className="line-clamp-3 min-h-[3.75rem] text-xs leading-5 text-muted-foreground">
        {marketText(listing.card.description, locale) || marketText(listing.card.subtitle, locale)}
      </p>
      <div className="mt-auto flex items-center justify-between gap-2 text-xs text-muted-foreground">
        <span className="min-w-0 truncate">
          {providers.length > 0
            ? providers.join(' · ')
            : marketText(listing.primary_category?.name, locale)}
        </span>
        <span className="shrink-0 font-medium text-foreground">
          {activated
            ? t('market.catalog.activated')
            : !price
              ? ''
              : price.unit_price_micros === 0
                ? t('market.catalog.free')
                : t(`market.catalog.from.${marketAmount(price).unit}`, {
                    amount: marketAmount(price).amount,
                  })}
        </span>
      </div>
    </Link>
  );
}
