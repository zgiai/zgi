import type { MarketListing, MarketPrice, MarketText } from '@/services/types/zgi-market';

/** The text of the market in the reader's language, or the other one when it is empty. */
export function marketText(text: MarketText | undefined, locale: string): string {
  if (!text) {
    return '';
  }
  const chinese = locale.toLowerCase().startsWith('zh');
  return (chinese ? text['zh-CN'] || text['en-US'] : text['en-US'] || text['zh-CN']) || '';
}

/** Micros hold six decimals of a yuan; milli-points hold three of a point. Both are exact integers. */
function decimal(amount: number, scale: number, minimum: number): string {
  const whole = Math.trunc(amount / scale);
  const digits = String(scale).length - 1;
  const fraction = String(Math.abs(amount % scale))
    .padStart(digits, '0')
    .replace(/0+$/, '');
  return fraction.length > 0 || minimum > 0
    ? `${whole}.${fraction.padEnd(minimum, '0')}`
    : String(whole);
}

/** One unit price as text and its unit: yuan for CNY, points for PTS. */
export function marketAmount(price: Pick<MarketPrice, 'unit_price_micros' | 'currency'>): {
  amount: string;
  unit: 'points' | 'yuan';
} {
  return price.currency === 'PTS'
    ? { amount: decimal(price.unit_price_micros, 1000, 0), unit: 'points' }
    : { amount: decimal(price.unit_price_micros, 1_000_000, 2), unit: 'yuan' };
}

/** The cheapest price of a listing, or undefined when it shows none. Zero means free. */
export function lowestPrice(listing: MarketListing): MarketPrice | undefined {
  return (listing.prices ?? []).reduce<MarketPrice | undefined>(
    (lowest, price) =>
      !lowest ||
      price.unit_price_micros * lowest.per_quantity < lowest.unit_price_micros * price.per_quantity
        ? price
        : lowest,
    undefined
  );
}
