import type { Organization } from '@/services/types/organization';
import Decimal from 'decimal.js-light';

export type BillingDisplayCurrency = 'USD' | 'CNY';

export interface BillingDisplaySettings {
  currency: BillingDisplayCurrency;
  usdToCnyRate: number;
}

interface UsageAmountFormatOptions {
  locale?: Intl.LocalesArgument;
}

interface RecordedCurrencyFormatOptions extends UsageAmountFormatOptions {
  showSign?: boolean;
}

export const DEFAULT_BILLING_DISPLAY: BillingDisplaySettings = {
  currency: 'USD',
  usdToCnyRate: 7,
};

export const NORMALIZED_AI_CREDITS_PER_USD = 1_000;
const BILLING_DECIMAL_PLACES = 12;
const CANONICAL_PRICE_QUANTIZATION = new Decimal('0.000000000001');
const DISPLAY_TAIL_RELATIVE_TOLERANCE = new Decimal('0.000000001');
const TOKENS_PER_MILLION = 1_000_000;

/** Rebuilds a historical subtotal using only values recorded on that invocation. */
export function calculateRecordedTokenCostUSD(
  tokens: number,
  recordedPriceUSDPer1MTokens: string | undefined
): string | undefined {
  if (!Number.isFinite(tokens) || tokens < 0 || recordedPriceUSDPer1MTokens === undefined) {
    return undefined;
  }
  try {
    return new Decimal(tokens)
      .times(recordedPriceUSDPer1MTokens)
      .div(TOKENS_PER_MILLION)
      .toString();
  } catch {
    return undefined;
  }
}

export function getBillingDisplaySettings(
  organization?: Pick<Organization, 'billing_display_currency' | 'usd_to_cny_rate'> | null
): BillingDisplaySettings {
  const currency = organization?.billing_display_currency === 'CNY' ? 'CNY' : 'USD';
  const rawRate = organization?.usd_to_cny_rate;
  const parsedRate =
    typeof rawRate === 'string' ? Number(rawRate) : typeof rawRate === 'number' ? rawRate : NaN;

  return {
    currency,
    usdToCnyRate:
      Number.isFinite(parsedRate) && parsedRate > 0
        ? parsedRate
        : DEFAULT_BILLING_DISPLAY.usdToCnyRate,
  };
}

export function getBillingCurrencySymbol(settings: BillingDisplaySettings): string {
  return settings.currency === 'CNY' ? '¥' : '$';
}

/**
 * Converts frontend-normalized AI credits to their canonical USD amount.
 * The API value has already been divided by AI_CREDITS_SCALE before it reaches this boundary.
 */
export function normalizedAiCreditsToUSD(
  normalizedCredits: number | null | undefined
): number | null {
  if (
    normalizedCredits === undefined ||
    normalizedCredits === null ||
    !Number.isFinite(normalizedCredits) ||
    normalizedCredits < 0
  ) {
    return null;
  }

  return new Decimal(normalizedCredits).div(NORMALIZED_AI_CREDITS_PER_USD).toNumber();
}

/**
 * Formats settled usage from frontend-normalized credits in the organization's display currency.
 */
export function formatBillingDisplayAmountFromNormalizedCredits(
  normalizedCredits: number | null | undefined,
  settings: BillingDisplaySettings,
  { locale }: UsageAmountFormatOptions = {}
): string {
  const amountUSD = normalizedAiCreditsToUSD(normalizedCredits);
  if (amountUSD === null) return '-';
  if (
    settings.currency === 'CNY' &&
    (!Number.isFinite(settings.usdToCnyRate) || settings.usdToCnyRate <= 0)
  ) {
    return '-';
  }

  const displayAmount = new Decimal(amountUSD).times(
    settings.currency === 'CNY' ? settings.usdToCnyRate : 1
  );

  const symbol = getBillingCurrencySymbol(settings);
  const estimatePrefix = settings.currency === 'CNY' ? '≈' : '';

  const displayNumber = displayAmount.toNumber();
  if (!Number.isFinite(displayNumber)) return '-';
  const maximumFractionDigits = displayAmount.abs().lt(1) ? BILLING_DECIMAL_PLACES : 2;
  const formatted = displayNumber.toLocaleString(locale, {
    minimumFractionDigits: 2,
    maximumFractionDigits,
  });
  return `${estimatePrefix}${symbol}${formatted}`;
}

export function formatBillingDisplayAmountFromUSD(
  amountUSD: number | string | null | undefined,
  settings: BillingDisplaySettings
): string {
  if (amountUSD === undefined || amountUSD === null) {
    return '-';
  }
  if (
    settings.currency === 'CNY' &&
    (!Number.isFinite(settings.usdToCnyRate) || settings.usdToCnyRate <= 0)
  ) {
    return '-';
  }
  let displayAmount: Decimal;
  try {
    displayAmount = new Decimal(amountUSD).times(
      settings.currency === 'CNY' ? settings.usdToCnyRate : 1
    );
  } catch {
    return '-';
  }
  const displayNumber = displayAmount.toNumber();
  if (!Number.isFinite(displayNumber)) return '-';
  const maximumFractionDigits = displayAmount.abs().lt(1) ? BILLING_DECIMAL_PLACES : 2;
  const formatted = displayNumber.toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits,
  });
  return `${getBillingCurrencySymbol(settings)}${formatted}`;
}

export function formatRecordedBillingAmount(
  amount: number | string | null | undefined,
  currency: BillingDisplayCurrency,
  { locale }: UsageAmountFormatOptions = {}
): string {
  if (amount === undefined || amount === null) return '-';
  let recordedAmount: Decimal;
  try {
    recordedAmount = new Decimal(amount);
  } catch {
    return '-';
  }
  const displayNumber = recordedAmount.toNumber();
  if (!Number.isFinite(displayNumber)) return '-';
  const maximumFractionDigits = recordedAmount.abs().lt(1) ? BILLING_DECIMAL_PLACES : 2;
  const formatted = displayNumber.toLocaleString(locale, {
    minimumFractionDigits: 2,
    maximumFractionDigits,
  });
  return `${currency === 'CNY' ? '¥' : '$'}${formatted}`;
}

export function formatRecordedBillingAmountFromUSD(
  amountUSD: number | string | null | undefined,
  displayCurrency: BillingDisplayCurrency,
  recordedUSDToCNYRate: number | string | null | undefined,
  options: UsageAmountFormatOptions = {}
): string {
  if (displayCurrency === 'USD') {
    return formatRecordedBillingAmount(amountUSD, 'USD', options);
  }
  if (amountUSD === undefined || amountUSD === null || recordedUSDToCNYRate == null) {
    return formatRecordedBillingAmount(amountUSD, 'USD', options);
  }
  try {
    const rate = new Decimal(recordedUSDToCNYRate);
    if (!rate.isPositive()) return formatRecordedBillingAmount(amountUSD, 'USD', options);
    return formatRecordedBillingAmount(
      new Decimal(amountUSD).times(rate).toString(),
      'CNY',
      options
    );
  } catch {
    return formatRecordedBillingAmount(amountUSD, 'USD', options);
  }
}

export function formatRecordedCurrencyAmount(
  amount: number | string | null | undefined,
  currency: string | null | undefined,
  { locale, showSign = false }: RecordedCurrencyFormatOptions = {}
): string {
  if (amount === undefined || amount === null) return '-';
  const normalizedCurrency = currency?.trim().toUpperCase();
  if (!normalizedCurrency) return String(amount);
  let value: number;
  try {
    value = new Decimal(amount).toNumber();
  } catch {
    return '-';
  }
  if (!Number.isFinite(value)) return '-';
  try {
    return new Intl.NumberFormat(locale, {
      style: 'currency',
      currency: normalizedCurrency,
      currencyDisplay: 'code',
      signDisplay: showSign ? 'exceptZero' : 'auto',
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(value);
  } catch {
    const sign = showSign && value > 0 ? '+' : '';
    return `${sign}${value.toFixed(2)} ${normalizedCurrency}`;
  }
}

/**
 * Formats a monetary amount in the organization's selected display currency.
 * The source amount remains unchanged; conversion is presentation-only.
 */
export function formatCurrencyAmountForBillingDisplay(
  amount: number | string | null | undefined,
  sourceCurrency: string | null | undefined,
  settings: BillingDisplaySettings,
  options: RecordedCurrencyFormatOptions = {}
): string {
  if (amount === undefined || amount === null) return '-';
  const source = sourceCurrency?.trim().toUpperCase();
  if (source !== 'USD' && source !== 'CNY') {
    return formatRecordedCurrencyAmount(amount, sourceCurrency, options);
  }
  if (source === settings.currency) {
    return formatRecordedCurrencyAmount(amount, source, options);
  }
  if (!Number.isFinite(settings.usdToCnyRate) || settings.usdToCnyRate <= 0) {
    return formatRecordedCurrencyAmount(amount, source, options);
  }
  try {
    const converted =
      source === 'USD'
        ? new Decimal(amount).times(settings.usdToCnyRate)
        : new Decimal(amount).div(settings.usdToCnyRate);
    return `≈${formatRecordedCurrencyAmount(converted.toString(), settings.currency, options)}`;
  } catch {
    return formatRecordedCurrencyAmount(amount, source, options);
  }
}

export function formatCatalogCurrencyAmount(
  amount: number | string | null | undefined,
  sourceCurrency: string | null | undefined,
  settings: BillingDisplaySettings,
  options: UsageAmountFormatOptions = {}
): string {
  return formatCurrencyAmountForBillingDisplay(amount, sourceCurrency, settings, options);
}

export function billingDisplayInputValueFromUSD(
  amountUSD: number | string | null | undefined,
  configured: boolean | null | undefined,
  settings: BillingDisplaySettings
): string {
  if (!configured) return '';
  if (amountUSD === undefined || amountUSD === null) return '0';
  try {
    const displayAmount = new Decimal(amountUSD).times(
      settings.currency === 'CNY' ? settings.usdToCnyRate : 1
    );
    return trimBillingDisplayInput(displayAmount, settings);
  } catch {
    return '0';
  }
}

export function billingDisplayInputToUSD(
  displayValue: string,
  settings: BillingDisplaySettings
): string {
  const trimmed = displayValue.trim();
  if (trimmed === '') return '';
  try {
    const parsed = new Decimal(trimmed);
    if (
      settings.currency === 'CNY' &&
      (!Number.isFinite(settings.usdToCnyRate) || settings.usdToCnyRate <= 0)
    ) {
      return trimmed;
    }
    const amountUSD = settings.currency === 'CNY' ? parsed.div(settings.usdToCnyRate) : parsed;
    return trimDecimal(amountUSD);
  } catch {
    return trimmed;
  }
}

function trimDecimal(value: Decimal): string {
  return value.toDecimalPlaces(BILLING_DECIMAL_PLACES).toFixed();
}

/**
 * Removes the conversion tail introduced when a CNY input was persisted in a
 * canonical USD column with 12 decimal places. The tolerance is derived from
 * that storage quantum, so meaningful small prices are not rounded to zero.
 */
function trimBillingDisplayInput(value: Decimal, settings: BillingDisplaySettings): string {
  if (settings.currency !== 'CNY') {
    return trimDecimal(value);
  }

  const tolerance = CANONICAL_PRICE_QUANTIZATION.times(settings.usdToCnyRate).div(2);
  for (let decimalPlaces = 0; decimalPlaces < BILLING_DECIMAL_PLACES; decimalPlaces += 1) {
    const candidate = value.toDecimalPlaces(decimalPlaces);
    if (!value.isZero() && candidate.isZero()) continue;
    const difference = candidate.minus(value).abs();
    const relativeDifference = value.isZero() ? new Decimal(0) : difference.div(value.abs());
    if (difference.lte(tolerance) && relativeDifference.lte(DISPLAY_TAIL_RELATIVE_TOLERANCE)) {
      return candidate.toFixed();
    }
  }
  return trimDecimal(value);
}
