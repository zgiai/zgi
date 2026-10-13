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
  'unit',
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

/**
 * The most one call can cost at the list prices, in milli-points: of one offering, or of the
 * dearest offering when none is named, as for an MCP server whose tools wrap several. Undefined
 * when the market publishes no cap for it.
 */
export function marketPriceCap(
  listing: Pick<MarketListing, 'price_caps'>,
  offeringSlug?: string
): number | undefined {
  const caps = (listing.price_caps ?? []).filter(
    cap => (!offeringSlug || cap.offering_slug === offeringSlug) && cap.max_charge_micros > 0
  );
  return caps.length > 0 ? Math.max(...caps.map(cap => cap.max_charge_micros)) : undefined;
}

/** Whether a listing is charged by anything but the call itself, such as a unit the upstream reports. */
export function chargedByUsage(prices: MarketPrice[]): boolean {
  return prices.some(
    price => price.unit_price_micros > 0 && price.meter !== 'request' && price.meter !== 'tool_call'
  );
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

const marketCostReasonPattern =
  /^one call can cost up to (\d+(?:\.\d{1,3})?) points, above the confirmation threshold of (\d+(?:\.\d{1,3})?) points$/;

/**
 * Reads the two amounts out of the reason the API gives when a market tool asks for approval
 * because of its cost alone: the most one call can cost and the threshold it is over, in points.
 * Undefined for any other reason, which is then shown as it is.
 */
export function marketCostApprovalAmounts(
  reason: string | null | undefined
): { max: string; threshold: string } | undefined {
  const match = marketCostReasonPattern.exec((reason ?? '').trim());
  return match ? { max: match[1] as string, threshold: match[2] as string } : undefined;
}

/**
 * What one unit of the meter `unit` is in a listing, as its offerings call it: `credits`, or
 * `credits / pages` when two offerings count different things. Empty when no price names one.
 */
export function marketUnitName(listing: Pick<MarketListing, 'prices'>): string {
  const names = (listing.prices ?? [])
    .filter(price => price.meter === 'unit' && price.unit_name)
    .map(price => price.unit_name as string);
  return [...new Set(names)].join(' / ');
}

/**
 * The sentence of a pending approval with the reason added when a market tool asks for its cost
 * alone, so the person who approves reads what one call can cost. `say` puts the two amounts and
 * the sentence into the reader's language; any other reason leaves the sentence as it is.
 */
export function withMarketCostReason(
  sentence: string,
  reason: string | null | undefined,
  say: (values: { sentence: string; max: string; threshold: string }) => string
): string {
  const cost = marketCostApprovalAmounts(reason);
  return cost ? say({ sentence, ...cost }) : sentence;
}

const marketWriteRefusal =
  "This tool writes data and needs a person's confirmation, so it does not run unattended";
const marketCostRefusalPattern =
  /^One call of this tool can cost up to (\d+(?:\.\d{1,3})?) points, above the confirmation threshold of (\d+(?:\.\d{1,3})?) points, so it does not run unattended$/;

/**
 * Reads the reason the API gives when a market tool is refused where nobody can approve it: it
 * writes, or one call can cost too much. Undefined for any other reason.
 */
export function marketUnattendedRefusal(
  reason: string | null | undefined
): { kind: 'write' } | { kind: 'cost'; max: string; threshold: string } | undefined {
  const text = (reason ?? '').trim();
  if (text === marketWriteRefusal) {
    return { kind: 'write' };
  }
  const match = marketCostRefusalPattern.exec(text);
  return match
    ? { kind: 'cost', max: match[1] as string, threshold: match[2] as string }
    : undefined;
}
