'use client';

import { useCallback, useMemo } from 'react';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import type { MarketListing, MarketPrice, MarketText } from '@/services/types/zgi-market';
import {
  knownMarketMeter,
  lowestPrice,
  marketAmount,
  marketDisplayPrice,
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

  /** The amount of a price alone: "免费", "2.5 点". */
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

  /** What an amount is charged for: "每 1,000,000 输入 token". */
  const per = useCallback(
    (price: MarketPrice) =>
      t('market.catalog.detail.pricing.perUnit', {
        quantity: numbers.format(marketDisplayPrice(price).per_quantity),
        meter: meter(price.meter),
      }),
    [meter, numbers, t]
  );

  /** The cheapest price of a listing as a card shows it; empty when it publishes none. */
  const from = useCallback(
    (listing: MarketListing) => {
      const price = lowestPrice(listing);
      if (!price) {
        return '';
      }
      if (price.unit_price_micros === 0) {
        return t('market.catalog.free');
      }
      const shown = marketAmount(price);
      return t(`market.catalog.from.${shown.unit}`, { amount: shown.amount });
    },
    [t]
  );

  return { t, locale, text, meter, amount, per, from };
}
