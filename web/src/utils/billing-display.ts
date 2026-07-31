import type { Organization } from '@/services/types/organization';

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

  return normalizedCredits / NORMALIZED_AI_CREDITS_PER_USD;
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

  const displayAmount = settings.currency === 'CNY' ? amountUSD * settings.usdToCnyRate : amountUSD;
  if (!Number.isFinite(displayAmount)) return '-';

  const symbol = getBillingCurrencySymbol(settings);
  const estimatePrefix = settings.currency === 'CNY' ? '≈' : '';

  if (displayAmount > 0 && displayAmount < 0.0001) {
    return `<${symbol}0.0001`;
  }

  const maximumFractionDigits = displayAmount > 0 && displayAmount < 1 ? 4 : 2;
  const formatted = displayAmount.toLocaleString(locale, {
    minimumFractionDigits: 2,
    maximumFractionDigits,
  });
  return `${estimatePrefix}${symbol}${formatted}`;
}

export function formatBillingDisplayAmountFromUSD(
  amountUSD: number | null | undefined,
  settings: BillingDisplaySettings
): string {
  if (amountUSD === undefined || amountUSD === null || !Number.isFinite(amountUSD)) {
    return '-';
  }
  const displayAmount = settings.currency === 'CNY' ? amountUSD * settings.usdToCnyRate : amountUSD;
  return `${getBillingCurrencySymbol(settings)}${displayAmount.toFixed(2)}`;
}

export function billingDisplayInputValueFromUSD(
  amountUSD: number | null | undefined,
  configured: boolean | null | undefined,
  settings: BillingDisplaySettings
): string {
  if (!configured) return '';
  if (amountUSD === undefined || amountUSD === null || !Number.isFinite(amountUSD)) return '0';
  const displayAmount = settings.currency === 'CNY' ? amountUSD * settings.usdToCnyRate : amountUSD;
  return trimDecimal(displayAmount);
}

export function billingDisplayInputToUSD(
  displayValue: string,
  settings: BillingDisplaySettings
): string {
  const trimmed = displayValue.trim();
  if (trimmed === '') return '';
  const parsed = Number(trimmed);
  if (!Number.isFinite(parsed)) return trimmed;
  const amountUSD = settings.currency === 'CNY' ? parsed / settings.usdToCnyRate : parsed;
  return trimDecimal(amountUSD);
}

function trimDecimal(value: number): string {
  const rounded = Math.round(value * 1_000_000) / 1_000_000;
  return String(rounded);
}
