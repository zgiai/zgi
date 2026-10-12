'use client';

import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import type { MarketListing } from '@/services/types/zgi-market';
import { useMarketLabels } from './use-market-labels';

/** Every price the listing publishes: what is charged, for how much usage, of which offering. */
export function ZgiMarketPricingTable({ listing }: { listing: MarketListing }) {
  const { t, meter, amount, per } = useMarketLabels();
  const prices = listing.prices ?? [];
  if (prices.length === 0) {
    return (
      <p className="rounded-lg border border-dashed px-4 py-6 text-center text-sm text-muted-foreground">
        {t('market.catalog.detail.pricing.empty')}
      </p>
    );
  }
  const offerings = listing.offerings.length > 0;
  return (
    <div className="flex flex-col gap-3">
      <p className="text-sm text-muted-foreground">{t('market.catalog.detail.pricing.note')}</p>
      <div className="overflow-x-auto rounded-lg border">
        <Table>
          <TableHeader>
            <TableRow>
              {offerings ? (
                <TableHead scope="col">{t('market.catalog.detail.pricing.offering')}</TableHead>
              ) : null}
              <TableHead scope="col">{t('market.catalog.detail.pricing.meter')}</TableHead>
              <TableHead scope="col" className="text-right">
                {t('market.catalog.detail.pricing.price')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {prices.map(price => (
              <TableRow key={`${price.offering_slug ?? ''}:${price.meter}`}>
                {offerings ? (
                  <TableCell className="break-all font-mono text-xs">
                    {price.offering_slug ?? t('market.catalog.detail.pricing.wholeListing')}
                  </TableCell>
                ) : null}
                <TableCell>
                  <span className="block text-sm">{meter(price.meter)}</span>
                  <span className="block text-xs text-muted-foreground">{per(price)}</span>
                </TableCell>
                <TableCell className="whitespace-nowrap text-right text-sm font-medium tabular-nums">
                  {amount(price)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}
