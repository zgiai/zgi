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

export const DEFAULT_BILLING_DISPLAY: BillingDisplaySettings = {
  currency: 'USD',
  usdToCnyRate: 7,
};

export const NORMALIZED_AI_CREDITS_PER_USD = 1_000;
const BILLING_DECIMAL_PLACES = 12;

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

export function billingDisplayInputValueFromUSD(
  amountUSD: number | null | undefined,
  configured: boolean | null | undefined,
  settings: BillingDisplaySettings
): string {
  if (!configured) return '';
  if (amountUSD === undefined || amountUSD === null || !Number.isFinite(amountUSD)) return '0';
  const displayAmount = new Decimal(amountUSD).times(
    settings.currency === 'CNY' ? settings.usdToCnyRate : 1
  );
  return trimDecimal(displayAmount);
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
