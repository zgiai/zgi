'use client';

import { useCallback, useMemo } from 'react';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import type { MarketListing, MarketPrice, MarketText } from '@/services/types/zgi-market';
import {
  highestPrice,
  knownMarketMeter,
  lowestPrice,
  marketAmount,
  marketDisplayPrice,
  marketPriceCap,
  marketText,
} from './market-format';

/** The words the market pages share: meters, prices, and texts in the reader's language. */
export function useMarketLabels() {
  const t = useT();
  const { locale } = useLocale();
  const numbers = useMemo(
    () => new Intl.NumberFormat(locale.toLowerCase().startsWith('zh') ? 'zh-CN' : 'en-US'),
    [locale]
  );

  const text = useCallback((value: MarketText | undefined) => marketText(value, locale), [locale]);

  /** A meter in words; a meter this page does not know is shown by its identifier. */
  const meter = useCallback(
    (value: string) => {
      const known = knownMarketMeter(value);
      return known ? t(`market.catalog.meters.${known}`) : value;
    },
    [t]
  );

  /** The amount of a price alone: free, or a number of points such as 2.5. */
  const amount = useCallback(
    (price: MarketPrice) => {
      if (price.unit_price_micros === 0) {
        return t('market.catalog.free');
      }
      const shown = marketAmount(marketDisplayPrice(price));
      return t(`market.catalog.detail.pricing.${shown.unit}`, { amount: shown.amount });
    },
    [t]
  );

  /** What an amount is charged for, such as per 1,000,000 input tokens. */
  const per = useCallback(
    (price: MarketPrice) =>
      t('market.catalog.detail.pricing.perUnit', {
        quantity: numbers.format(marketDisplayPrice(price).per_quantity),
        // The unit meter is called what the offering calls it.
        meter: price.meter === 'unit' && price.unit_name ? price.unit_name : meter(price.meter),
      }),
    [meter, numbers, t]
  );

  /** "At most 82 points a call" for an offering whose calls are bounded; empty otherwise. */
  const cap = useCallback(
    (listing: Pick<MarketListing, 'price_caps'>, offeringSlug?: string) => {
      const micros = marketPriceCap(listing, offeringSlug);
      return micros === undefined
        ? ''
        : t('market.catalog.detail.pricing.cap', {
            amount: marketAmount({ unit_price_micros: micros, currency: 'PTS' }).amount,
          });
    },
    [t]
  );

  /** The cheapest price of a listing as a card shows it; empty when it publishes none. */
  const from = useCallback(
    (listing: MarketListing) => {
      const price = lowestPrice(listing);
      if (!price) {
        return '';
      }
      if (price.unit_price_micros === 0) {
        // Free only when nothing in the listing is priced, such as every tool of an MCP server.
        return (highestPrice(listing)?.unit_price_micros ?? 0) === 0
          ? t('market.catalog.free')
          : t('market.catalog.partlyFree');
      }
      const shown = marketAmount(price);
      return t(`market.catalog.from.${shown.unit}`, { amount: shown.amount });
    },
    [t]
  );

  /** A file size in the reader's number format: bytes below 1 KB, then KB and MB. */
  const bytes = useCallback(
    (size: number) => {
      if (size < 1024) {
        return `${numbers.format(size)} B`;
      }
      const units = ['KB', 'MB', 'GB'];
      let value = size / 1024;
      let unit = 0;
      while (value >= 1024 && unit < units.length - 1) {
        value /= 1024;
        unit += 1;
      }
      return `${numbers.format(Math.round(value * 10) / 10)} ${units[unit]}`;
    },
    [numbers]
  );

  return { t, locale, text, meter, amount, per, cap, from, bytes };
}
