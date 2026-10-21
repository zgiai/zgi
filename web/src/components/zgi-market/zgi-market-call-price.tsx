'use client';

import { Badge } from '@/components/ui/badge';
import {
  chargedByUsage,
  highestPrice,
  lowestPrice,
  marketAmount,
  marketPriceCap,
} from '@/components/zgi-market/market-format';
import { useZgiMarketListing } from '@/hooks/use-zgi-market';
import { useT } from '@/i18n';
import type { MarketToolCandidate } from '@/services/types/zgi-market';

/**
 * What one call of a candidate costs the organization: the price of the API offering, or of the
 * MCP server's listing, as a range when its tools are priced apart. Nothing is shown until the
 * listing is read or when it publishes no price.
 */
export function ZgiMarketCallPrice({
  listing: candidate,
}: {
  listing: Pick<MarketToolCandidate, 'kind' | 'slug' | 'offering_slug'>;
}) {
  const t = useT();
  const listing = useZgiMarketListing(candidate.kind, candidate.slug);
  if (!listing.data) return null;
  // An API offering has its own prices; an MCP server has the listing's.
  const prices = (listing.data.prices ?? []).filter(
    entry => candidate.kind !== 'api' || entry.offering_slug === candidate.offering_slug
  );
  const price = lowestPrice({ ...listing.data, prices });
  if (!price) return null;
  // The tools of an MCP server may be priced apart: free only when the dearest one is.
  const dearest = highestPrice({ ...listing.data, prices }) ?? price;
  if (dearest.unit_price_micros === 0) {
    return <Badge variant="subtle">{t('market.tools.free')}</Badge>;
  }
  // A call charged by what it used has no single price: say what it can cost at most.
  const cap = marketPriceCap(
    listing.data,
    candidate.kind === 'api' ? candidate.offering_slug : undefined
  );
  if (cap !== undefined && chargedByUsage(prices)) {
    return (
      <Badge variant="subtle">
        {t('market.tools.perCallAtMost', {
          amount: marketAmount({ unit_price_micros: cap, currency: 'PTS' }).amount,
        })}
      </Badge>
    );
  }
  if (price.unit_price_micros === 0) {
    return (
      <Badge variant="subtle">
        {t('market.tools.perCallUpTo', { amount: marketAmount(dearest).amount })}
      </Badge>
    );
  }
  if (
    dearest.unit_price_micros * price.per_quantity !==
    price.unit_price_micros * dearest.per_quantity
  ) {
    return (
      <Badge variant="subtle">
        {t('market.tools.perCallRange', {
          from: marketAmount(price).amount,
          to: marketAmount(dearest).amount,
        })}
      </Badge>
    );
  }
  return (
    <Badge variant="subtle">
      {t('market.tools.perCall', { amount: marketAmount(price).amount })}
    </Badge>
  );
}
