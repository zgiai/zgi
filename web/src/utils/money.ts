import Decimal from 'decimal.js';

/**
 * Canonical money helpers for the Pre web client.
 *
 * Money is stored as canonical USD and converted exactly once at the display
 * boundary. Every arithmetic step goes through decimal.js so binary floats are
 * never used for money, and values cross this module as decimal strings so the
 * precision survives the API -> editor -> API round trip.
 */

/** Canonical USD storage precision (the API stores 12 decimal places). */
export const MONEY_DECIMAL_PLACES = 12;

export type MoneyCurrency = 'USD' | 'CNY';

export type MoneyValue = string | number | Decimal;

/** Parses a money value without routing valid input through a binary float. */
export function parseMoney(value: MoneyValue | null | undefined): Decimal | null {
  if (value === null || value === undefined) return null;
  if (value instanceof Decimal) return value.isFinite() ? value : null;
  if (typeof value === 'number') {
    // Numbers are accepted only as already-parsed compatibility values coming
    // from the API; no money arithmetic is performed on the JS number itself.
    return Number.isFinite(value) ? new Decimal(value) : null;
  }
  const trimmed = value.trim();
  if (trimmed === '') return null;
  try {
    const parsed = new Decimal(trimmed);
    return parsed.isFinite() ? parsed : null;
  } catch {
    return null;
  }
}

function requireMoney(value: MoneyValue, name: string): Decimal {
  const parsed = parseMoney(value);
  if (parsed === null) throw new Error('Invalid money value: ' + name);
  return parsed;
}

/** Renders a decimal without exponent notation and without trailing zeros. */
export function toMoneyString(value: MoneyValue | null | undefined): string {
  const parsed = parseMoney(value ?? null);
  return parsed === null ? '' : parsed.toFixed();
}

export function addMoney(left: MoneyValue, right: MoneyValue): string {
  return requireMoney(left, 'left').plus(requireMoney(right, 'right')).toFixed();
}

export function subMoney(left: MoneyValue, right: MoneyValue): string {
  return requireMoney(left, 'left').minus(requireMoney(right, 'right')).toFixed();
}

export function mulMoney(left: MoneyValue, right: MoneyValue): string {
  return requireMoney(left, 'left').times(requireMoney(right, 'right')).toFixed();
}

export function divMoney(numerator: MoneyValue, denominator: MoneyValue): string {
  const divisor = requireMoney(denominator, 'denominator');
  if (divisor.isZero()) throw new Error('Money division by zero');
  return requireMoney(numerator, 'numerator').div(divisor).toFixed();
}

/** Rounds a money value to an explicit number of decimal places exactly once. */
export function quantizeMoney(
  value: MoneyValue,
  decimalPlaces: number = MONEY_DECIMAL_PLACES
): string {
  return requireMoney(value, 'value').toDecimalPlaces(decimalPlaces).toFixed();
}

export function isZeroMoney(value: MoneyValue | null | undefined): boolean {
  const parsed = parseMoney(value ?? null);
  return parsed !== null && parsed.isZero();
}

export function isPositiveMoney(value: MoneyValue | null | undefined): boolean {
  const parsed = parseMoney(value ?? null);
  return parsed !== null && parsed.isPositive();
}

export interface ConvertMoneyOptions {
  from: string;
  to: string;
  /** USD -> CNY rate, i.e. how many CNY one canonical USD buys. */
  usdToCnyRate: MoneyValue;
  decimalPlaces?: number;
}

function normalizeCurrency(currency: string): string {
  return currency.trim().toUpperCase();
}

/**
 * Converts an amount between the canonical currencies exactly once. Missing or
 * non-positive rates fail closed instead of silently using 1.
 */
export function convertMoney(amount: MoneyValue, options: ConvertMoneyOptions): string {
  const value = requireMoney(amount, 'amount');
  const from = normalizeCurrency(options.from);
  const to = normalizeCurrency(options.to);
  const decimalPlaces = options.decimalPlaces ?? MONEY_DECIMAL_PLACES;
  if (from === to) return quantizeMoney(value, decimalPlaces);

  const rate = requireMoney(options.usdToCnyRate, 'usdToCnyRate');
  // decimal.js treats +0 as positive, so compare against zero explicitly.
  if (!rate.gt(0)) throw new Error('Missing USD/CNY exchange rate');
  if (from === 'USD' && to === 'CNY') return quantizeMoney(value.times(rate), decimalPlaces);
  if (from === 'CNY' && to === 'USD') return quantizeMoney(value.div(rate), decimalPlaces);
  throw new Error('Unsupported currency conversion: ' + from + ' -> ' + to);
}

export function currencySymbol(currency: string): string {
  const normalized = normalizeCurrency(currency);
  if (normalized === 'CNY') return '¥';
  if (normalized === 'USD') return '$';
  return normalized === '' ? '' : normalized + ' ';
}

export interface FormatMoneyOptions {
  locale?: Intl.LocalesArgument;
  minimumFractionDigits?: number;
  maximumFractionDigits?: number;
  /** Marks a converted estimate, matching the catalog/estimate display rule. */
  estimate?: boolean;
}

/**
 * Formats a money value for display. Formatting is the only place that leaves
 * the decimal domain; the value itself is never mutated.
 */
export function formatMoney(
  value: MoneyValue | null | undefined,
  currency: string,
  options: FormatMoneyOptions = {}
): string {
  const parsed = parseMoney(value ?? null);
  if (parsed === null) return '-';
  const absolute = parsed.abs();
  const maximumFractionDigits =
    options.maximumFractionDigits ?? (absolute.lt(1) && !absolute.isZero() ? MONEY_DECIMAL_PLACES : 2);
  const minimumFractionDigits = options.minimumFractionDigits ?? 2;
  const formatted = parsed.toNumber().toLocaleString(options.locale, {
    minimumFractionDigits,
    maximumFractionDigits,
  });
  return (options.estimate ? '≈' : '') + currencySymbol(currency) + formatted;
}
