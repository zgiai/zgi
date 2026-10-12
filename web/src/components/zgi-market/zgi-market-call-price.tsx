'use client';

import { Badge } from '@/components/ui/badge';
import { lowestPrice, marketAmount } from '@/components/zgi-market/market-format';
import { useZgiMarketListing } from '@/hooks/use-zgi-market';
import { useT } from '@/i18n';
import type { MarketToolCandidate } from '@/services/types/zgi-market';

/**
 * What one call of a candidate costs the organization: the price of the API offering, or of the
 * MCP server's listing. Nothing is shown until the listing is read or when it publishes no price.
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
  if (price.unit_price_micros === 0) {
    return <Badge variant="subtle">{t('market.tools.free')}</Badge>;
  }
  return (
    <Badge variant="subtle">
      {t('market.tools.perCall', { amount: marketAmount(price).amount })}
    </Badge>
  );
}
