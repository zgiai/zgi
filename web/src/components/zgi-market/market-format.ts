import type { MarketListing, MarketPrice, MarketText } from '@/services/types/zgi-market';

/** The meters the market names today. A meter added later is shown by its identifier. */
export const marketKnownMeters = [
  'request',
  'token_input',
  'token_output',
  'page',
  'image',
  'second',
  'character',
  'tool_call',
  'install',
] as const;
export type MarketKnownMeter = (typeof marketKnownMeters)[number];

export function knownMarketMeter(meter: string): MarketKnownMeter | undefined {
  return marketKnownMeters.find(known => known === meter);
}

// Customers read a price per a round quantity, not per base unit: a million tokens, a thousand
// characters. Every other meter is quoted for the quantity the price row names.
const displayQuantity: Partial<Record<string, number>> = {
  token_input: 1_000_000,
  token_output: 1_000_000,
  character: 1_000,
};

/**
 * A price as a customer reads it: for the display quantity of its meter, rounded half up to a
 * whole milli-point or micro. The arithmetic is BigInt, so no float ever holds money.
 */
export function marketDisplayPrice(price: MarketPrice): MarketPrice {
  const quantity = displayQuantity[price.meter];
  if (!quantity || price.per_quantity === quantity) {
    return price;
  }
  const per = BigInt(price.per_quantity);
  const two = BigInt(2);
  const scaled = (BigInt(price.unit_price_micros) * BigInt(quantity) * two + per) / (per * two);
  return { ...price, unit_price_micros: Number(scaled), per_quantity: quantity };
}

/** A date of the market as a calendar day in the reader's language; empty when it is not a date. */
export function marketDate(value: string | null | undefined, locale: string): string {
  if (!value) {
    return '';
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return '';
  }
  return new Intl.DateTimeFormat(locale.toLowerCase().startsWith('zh') ? 'zh-CN' : 'en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  }).format(date);
}

/** An address the page may link to: an absolute http(s) URL and nothing else. */
export function marketExternalUrl(value: string | undefined): string | undefined {
  if (!value) {
    return undefined;
  }
  try {
    const url = new URL(value);
    return url.protocol === 'https:' || url.protocol === 'http:' ? url.toString() : undefined;
  } catch {
    return undefined;
  }
}

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

/** The dearest price of a listing, or undefined when it shows none. */
export function highestPrice(listing: MarketListing): MarketPrice | undefined {
  return (listing.prices ?? []).reduce<MarketPrice | undefined>(
    (highest, price) =>
      !highest ||
      price.unit_price_micros * highest.per_quantity >
        highest.unit_price_micros * price.per_quantity
        ? price
        : highest,
    undefined
  );
}

/**
 * Whether a page the market names after an authorization belongs to this site. The callback page
 * opens nothing else: a callback the market could not place would otherwise leave for its website.
 */
export function marketSameOrigin(target: string, origin: string): boolean {
  try {
    return new URL(target, origin).origin === new URL(origin).origin;
  } catch {
    return false;
  }
}

/** The market's error code of a failed request, or an empty string when it carries none. */
export function marketErrorCode(error: unknown): string {
  const data = (error as { response?: { data?: { error_code?: unknown } } } | null)?.response?.data;
  return typeof data?.error_code === 'string' ? data.error_code : '';
}

interface SkillPackageFile {
  path: string;
  size: number;
}

const scriptPattern = /\.(py|js|mjs|cjs|ts|sh|bash|rb|pl|php)$/i;

/**
 * The files of a skill package by top-level directory, the root first, and the ones that are
 * scripts: a reader should know whether a skill only instructs or also brings code to run.
 */
export function skillPackageSummary<T extends SkillPackageFile>(
  files: T[]
): { groups: Array<{ directory: string; files: T[] }>; scripts: T[] } {
  const groups = new Map<string, T[]>();
  for (const file of [...files].sort((a, b) => a.path.localeCompare(b.path))) {
    const cut = file.path.indexOf('/');
    const directory = cut < 0 ? '' : file.path.slice(0, cut);
    groups.set(directory, [...(groups.get(directory) ?? []), file]);
  }
  return {
    groups: [...groups.entries()]
      .sort(([a], [b]) => (a === '' ? -1 : b === '' ? 1 : a.localeCompare(b)))
      .map(([directory, grouped]) => ({ directory, files: grouped })),
    scripts: files.filter(file => scriptPattern.test(file.path)),
  };
}
