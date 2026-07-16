import type { Organization } from '@/services/types/organization';

export type BillingDisplayCurrency = 'USD' | 'CNY';

export interface BillingDisplaySettings {
  currency: BillingDisplayCurrency;
  usdToCnyRate: number;
}

export const DEFAULT_BILLING_DISPLAY: BillingDisplaySettings = {
  currency: 'USD',
  usdToCnyRate: 7,
};

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

export function formatBillingDisplayAmountFromUSD(
  amountUSD: number | null | undefined,
  settings: BillingDisplaySettings
): string {
  if (amountUSD === undefined || amountUSD === null || !Number.isFinite(amountUSD)) {
    return '-';
  }
  const displayAmount =
    settings.currency === 'CNY' ? amountUSD * settings.usdToCnyRate : amountUSD;
  return `${getBillingCurrencySymbol(settings)}${displayAmount.toFixed(2)}`;
}

export function billingDisplayInputValueFromUSD(
  amountUSD: number | null | undefined,
  configured: boolean | null | undefined,
  settings: BillingDisplaySettings
): string {
  if (!configured) return '';
  if (amountUSD === undefined || amountUSD === null || !Number.isFinite(amountUSD)) return '0';
  const displayAmount =
    settings.currency === 'CNY' ? amountUSD * settings.usdToCnyRate : amountUSD;
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
