import type { ApiKeyItem, CreateApiKeyResponse } from '@/services/types/apikey';
import type {
  AdjustChannelWalletResponse,
  ChannelDetail,
  ChannelItem,
} from '@/services/types/channel';
import type {
  AiCreditProduct,
  AiCredits,
  MonthlyStats,
  Transaction,
  TransactionDetail,
  TransactionsResponse,
} from '@/services/types/pay';
import type { WorkspaceQuota, WorkspaceQuotaList } from '@/services/types/workspace-quota';
import type {
  ModelUsageByAppTypeItem,
  ModelUsageByModelItem,
  ModelUsageDailyItem,
  ModelUsageData,
  ModelUsageSummary,
} from '@/services/types/statistics';

export const AI_CREDITS_SCALE = 1000;
export const DEFAULT_AI_CREDIT_EDIT_BACKEND_MAX = 99_999_999_000;
export const DEFAULT_AI_CREDIT_EDIT_MAX = shrinkAiCreditEditMax(DEFAULT_AI_CREDIT_EDIT_BACKEND_MAX);
export const CHANNEL_INITIAL_CREDIT_BACKEND_MAX = DEFAULT_AI_CREDIT_EDIT_BACKEND_MAX;
export const CHANNEL_INITIAL_CREDIT_MAX = shrinkAiCreditEditMax(CHANNEL_INITIAL_CREDIT_BACKEND_MAX);
export const MODEL_USAGE_AI_CREDITS_INTERNAL_PRECISION = 3;
export const MODEL_USAGE_AI_CREDITS_DISPLAY_PRECISION = 2;
export const CHANNEL_POINTS_PER_USD = 1_000;
export const USD_TO_CNY_ESTIMATE_RATE = 7;

const AI_CREDITS_PRECISION = 1;

interface AiCreditTransformOptions {
  preserveUnlimitedSentinel?: boolean;
  precision?: number;
}

interface FormatAiCreditValueOptions {
  locale?: Intl.LocalesArgument;
  maximumFractionDigits?: number;
  minimumFractionDigits?: number;
}

interface FormatChannelCreditOptions {
  locale?: Intl.LocalesArgument;
}

interface FormatAiCreditFiatEstimateOptions extends FormatChannelCreditOptions {
  symbol?: string;
}

/**
 * @util Normalize backend AI credits into UI display units.
 */
export function normalizeAiCreditValue(
  value?: number | null,
  options: AiCreditTransformOptions = {}
): number | null | undefined {
  if (value === undefined || value === null) return value;
  if (!Number.isFinite(value)) return value;
  if (options.preserveUnlimitedSentinel !== false && value === -1) return value;

  return roundAiCreditNumber(value / AI_CREDITS_SCALE, options.precision);
}

/**
 * @util Convert UI AI credits back into backend storage units.
 */
export function denormalizeAiCreditValue(
  value?: number | null,
  options: AiCreditTransformOptions = {}
): number | null | undefined {
  if (value === undefined || value === null) return value;
  if (!Number.isFinite(value)) return value;
  if (options.preserveUnlimitedSentinel !== false && value === -1) return value;

  return roundAiCreditNumber(value * AI_CREDITS_SCALE, options.precision);
}

/**
 * @util Normalize numeric AI credit strings returned by backend.
 */
export function normalizeAiCreditString(
  value?: string | null,
  options: AiCreditTransformOptions = {}
): string | null | undefined {
  if (value === undefined || value === null || value === '') return value;

  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return value;

  const normalized = normalizeAiCreditValue(parsed, options);
  return normalized === undefined || normalized === null ? value : formatAiCreditNumber(normalized);
}

/**
 * @util Normalize workflow billing metrics that may arrive as string or number.
 */
export function normalizeAiCreditMetricValue(
  value?: number | string,
  options: AiCreditTransformOptions = {}
): number | string | undefined {
  if (value === undefined) return value;

  if (typeof value === 'number') {
    return normalizeAiCreditValue(value, options) ?? value;
  }

  if (typeof value === 'string') {
    return normalizeAiCreditString(value, options) ?? value;
  }

  return value;
}

/**
 * @util Shrink backend AI credit edit limits into UI units.
 */
export function shrinkAiCreditEditMax(value: number): number {
  if (!Number.isFinite(value) || value <= 0) return 0;

  return Math.trunc(value / AI_CREDITS_SCALE);
}

/**
 * @util Sanitize integer AI credit inputs and clamp them to a maximum UI value.
 */
export function sanitizeAiCreditIntegerInput(value: string, max: number): string {
  const digits = value.replace(/\D/g, '').replace(/^0+(?=\d)/, '');
  if (!digits) return '';

  const numeric = Number(digits);
  if (!Number.isFinite(numeric)) return String(max);

  return String(Math.min(numeric, max));
}

/**
 * @util Check whether the transaction currency type represents AI credits.
 */
export function isAiCreditsCurrencyType(currencyType?: string): boolean {
  return currencyType?.trim().toLowerCase() !== 'cash';
}

/**
 * @util Normalize AI credits payload returned by payment hooks.
 */
export function normalizeAiCredits(credits: AiCredits): AiCredits {
  return {
    ...credits,
    subscription_credits: normalizeAiCreditValue(credits.subscription_credits) ?? 0,
    purchased_credits: normalizeAiCreditValue(credits.purchased_credits) ?? 0,
    total_earned: normalizeAiCreditValue(credits.total_earned) ?? 0,
    total_spent: normalizeAiCreditValue(credits.total_spent) ?? 0,
    official_ai_credits: {
      ...credits.official_ai_credits,
      balance: normalizeAiCreditValue(credits.official_ai_credits.balance) ?? 0,
    },
    private_channel_funds: {
      ...credits.private_channel_funds,
      total: normalizeAiCreditValue(credits.private_channel_funds.total) ?? 0,
      channels: credits.private_channel_funds.channels.map(channel => ({
        ...channel,
        balance: normalizeAiCreditValue(channel.balance) ?? 0,
      })),
    },
  };
}

/**
 * @util Normalize AI credit product payload for UI display.
 */
export function normalizeAiCreditProduct(product: AiCreditProduct): AiCreditProduct {
  return {
    ...product,
    credit_amount: normalizeAiCreditValue(product.credit_amount) ?? 0,
  };
}

/**
 * @util Normalize monthly AI credit consumption statistics.
 */
export function normalizeMonthlyStats(stats: MonthlyStats): MonthlyStats {
  return {
    ...stats,
    credits: {
      ...stats.credits,
      total_credits_consumed: normalizeAiCreditValue(stats.credits.total_credits_consumed) ?? 0,
      subscription_credits_consumed:
        normalizeAiCreditValue(stats.credits.subscription_credits_consumed) ?? 0,
      purchased_credits_consumed:
        normalizeAiCreditValue(stats.credits.purchased_credits_consumed) ?? 0,
    },
  };
}

/**
 * @util Normalize model usage summary credits for UI display.
 */
export function normalizeModelUsageSummary(summary: ModelUsageSummary): ModelUsageSummary {
  return {
    ...summary,
    official_points:
      normalizeAiCreditValue(summary.official_points, {
        precision: MODEL_USAGE_AI_CREDITS_INTERNAL_PRECISION,
      }) ?? 0,
    private_points:
      normalizeAiCreditValue(summary.private_points, {
        precision: MODEL_USAGE_AI_CREDITS_INTERNAL_PRECISION,
      }) ?? 0,
    total_points:
      normalizeAiCreditValue(summary.total_points, {
        precision: MODEL_USAGE_AI_CREDITS_INTERNAL_PRECISION,
      }) ?? 0,
  };
}

/**
 * @util Normalize model usage item credits for UI display.
 */
export function normalizeModelUsageByModelItem(item: ModelUsageByModelItem): ModelUsageByModelItem {
  return {
    ...item,
    official_points:
      normalizeAiCreditValue(item.official_points, {
        precision: MODEL_USAGE_AI_CREDITS_INTERNAL_PRECISION,
      }) ?? 0,
    private_points:
      normalizeAiCreditValue(item.private_points, {
        precision: MODEL_USAGE_AI_CREDITS_INTERNAL_PRECISION,
      }) ?? 0,
    total_points:
      normalizeAiCreditValue(item.total_points, {
        precision: MODEL_USAGE_AI_CREDITS_INTERNAL_PRECISION,
      }) ?? 0,
  };
}

/**
 * @util Normalize model usage app-type item credits for UI display.
 */
export function normalizeModelUsageByAppTypeItem(
  item: ModelUsageByAppTypeItem
): ModelUsageByAppTypeItem {
  return {
    ...item,
    official_points:
      normalizeAiCreditValue(item.official_points, {
        precision: MODEL_USAGE_AI_CREDITS_INTERNAL_PRECISION,
      }) ?? 0,
    private_points:
      normalizeAiCreditValue(item.private_points, {
        precision: MODEL_USAGE_AI_CREDITS_INTERNAL_PRECISION,
      }) ?? 0,
    total_points:
      normalizeAiCreditValue(item.total_points, {
        precision: MODEL_USAGE_AI_CREDITS_INTERNAL_PRECISION,
      }) ?? 0,
  };
}

/**
 * @util Normalize model usage daily credits for UI display.
 */
export function normalizeModelUsageDailyItem(item: ModelUsageDailyItem): ModelUsageDailyItem {
  return {
    ...item,
    official_points:
      normalizeAiCreditValue(item.official_points, {
        precision: MODEL_USAGE_AI_CREDITS_INTERNAL_PRECISION,
      }) ?? 0,
    private_points:
      normalizeAiCreditValue(item.private_points, {
        precision: MODEL_USAGE_AI_CREDITS_INTERNAL_PRECISION,
      }) ?? 0,
    total_points:
      normalizeAiCreditValue(item.total_points, {
        precision: MODEL_USAGE_AI_CREDITS_INTERNAL_PRECISION,
      }) ?? 0,
  };
}

/**
 * @util Format AI credits without compact notation so detailed views keep their decimal precision.
 */
export function formatAiCreditValue(
  value?: number | null,
  {
    locale,
    maximumFractionDigits = MODEL_USAGE_AI_CREDITS_DISPLAY_PRECISION,
    minimumFractionDigits = 0,
  }: FormatAiCreditValueOptions = {}
): string {
  if (value === undefined || value === null || !Number.isFinite(value)) return '-';

  const rounded = roundAiCreditNumber(value, maximumFractionDigits);
  return rounded.toLocaleString(locale, {
    maximumFractionDigits,
    minimumFractionDigits,
  });
}

/**
 * @util Format private channel quota in user-facing points.
 */
export function formatChannelCreditPoints(
  value?: number | null,
  { locale }: FormatChannelCreditOptions = {}
): string {
  if (value === undefined || value === null || !Number.isFinite(value)) return '-';

  return Math.round(value).toLocaleString(locale);
}

/**
 * @util Convert normalized private channel points to an approximate USD amount.
 */
export function channelPointsToUsd(value?: number | null): number | null {
  if (value === undefined || value === null || !Number.isFinite(value)) return null;

  return value / CHANNEL_POINTS_PER_USD;
}

/**
 * @util Convert USD input into private channel points.
 */
export function usdToChannelPoints(value?: number | null): number | undefined {
  if (value === undefined || value === null || !Number.isFinite(value)) return undefined;

  return Math.round(value * CHANNEL_POINTS_PER_USD);
}

/**
 * @util Format the approximate USD value for private channel points.
 */
export function formatChannelCreditUsd(
  value?: number | null,
  { locale }: FormatChannelCreditOptions = {}
): string {
  const usd = channelPointsToUsd(value);
  if (usd === null) return '-';

  return `$${usd.toLocaleString(locale, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`;
}

/**
 * @util Format the approximate fiat value for AI credit points.
 */
export function formatAiCreditFiatEstimate(
  value?: number | null,
  { locale, symbol }: FormatAiCreditFiatEstimateOptions = {}
): string {
  const usd = channelPointsToUsd(value);
  if (usd === null) return '-';

  const localeText = Array.isArray(locale) ? locale[0] : locale;
  const shouldEstimateCny =
    typeof localeText === 'string' && localeText.toLowerCase().startsWith('zh');
  const fiat = shouldEstimateCny ? usd * USD_TO_CNY_ESTIMATE_RATE : usd;
  const resolvedSymbol = symbol ?? (shouldEstimateCny ? '￥' : '$');

  return `${resolvedSymbol}${fiat.toLocaleString(locale, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`;
}

/**
 * @util Normalize model usage statistics credits for UI display.
 */
export function normalizeModelUsageData(data: ModelUsageData): ModelUsageData {
  return {
    ...data,
    summary: normalizeModelUsageSummary(data.summary),
    by_model: data.by_model.map(normalizeModelUsageByModelItem),
    by_app_type: data.by_app_type.map(normalizeModelUsageByAppTypeItem),
    daily_trend: data.daily_trend.map(normalizeModelUsageDailyItem),
  };
}

/**
 * @util Normalize bill/transaction payloads containing AI credit balances.
 */
export function normalizeTransactionsResponse(
  response: TransactionsResponse
): TransactionsResponse {
  return {
    ...response,
    data: response.data.map(normalizeTransaction),
  };
}

/**
 * @util Normalize channel list item balances for UI display.
 */
export function normalizeChannelItem(channel: ChannelItem): ChannelItem {
  return {
    ...channel,
    remaining_funds: normalizeAiCreditValue(channel.remaining_funds) ?? undefined,
  };
}

/**
 * @util Normalize channel detail balances for UI display.
 */
export function normalizeChannelDetail(channel: ChannelDetail): ChannelDetail {
  return {
    ...channel,
    remaining_funds: normalizeAiCreditValue(channel.remaining_funds) ?? undefined,
    balance: normalizeAiCreditString(channel.balance) ?? undefined,
  };
}

/**
 * @util Normalize channel wallet adjustment results for UI display.
 */
export function normalizeAdjustChannelWalletResponse(
  response: AdjustChannelWalletResponse
): AdjustChannelWalletResponse {
  return {
    ...response,
    amount: normalizeAiCreditValue(response.amount) ?? 0,
    balance_before: normalizeAiCreditValue(response.balance_before) ?? 0,
    balance_after: normalizeAiCreditValue(response.balance_after) ?? 0,
  };
}

/**
 * @util Normalize workspace quota values for UI display.
 */
export function normalizeWorkspaceQuota(quota: WorkspaceQuota): WorkspaceQuota {
  return {
    ...quota,
    used_quota: normalizeAiCreditValue(quota.used_quota) ?? 0,
    remain_quota: normalizeAiCreditValue(quota.remain_quota) ?? 0,
    quota_limit: normalizeAiCreditValue(quota.quota_limit, {
      preserveUnlimitedSentinel: false,
    }),
  };
}

/**
 * @util Normalize workspace quota list payloads for UI display.
 */
export function normalizeWorkspaceQuotaList(list: WorkspaceQuotaList): WorkspaceQuotaList {
  return {
    ...list,
    items: list.items.map(normalizeWorkspaceQuota),
  };
}

/**
 * @util Normalize API key quota values for UI display.
 */
export function normalizeApiKeyItem(item: ApiKeyItem): ApiKeyItem {
  return {
    ...item,
    used_quota: normalizeAiCreditValue(item.used_quota) ?? 0,
    remain_quota: normalizeAiCreditValue(item.remain_quota) ?? 0,
    quota_limit: normalizeAiCreditValue(item.quota_limit, {
      preserveUnlimitedSentinel: false,
    }),
  };
}

/**
 * @util Normalize API key creation responses for UI display.
 */
export function normalizeCreateApiKeyResponse(
  response: CreateApiKeyResponse
): CreateApiKeyResponse {
  return {
    ...response,
    keys: response.keys.map(normalizeApiKeyItem),
  };
}

function normalizeTransaction(transaction: Transaction): Transaction {
  const shouldNormalizeAiCredits = isAiCreditsCurrencyType(transaction.currency_type);

  return {
    ...transaction,
    amount: shouldNormalizeAiCredits
      ? (normalizeAiCreditValue(transaction.amount) ?? undefined)
      : transaction.amount,
    // Bill account balances are cash amounts, not AI credits.
    balance_after: transaction.balance_after,
    balance_before: transaction.balance_before,
    transaction_detail: transaction.transaction_detail
      ? normalizeTransactionDetail(transaction.transaction_detail)
      : transaction.transaction_detail,
  };
}

function normalizeTransactionDetail(detail: TransactionDetail): TransactionDetail {
  return {
    ...detail,
    expired_amount: normalizeAiCreditValue(detail.expired_amount) ?? 0,
    new_credits: normalizeAiCreditValue(detail.new_credits) ?? 0,
    purchased_balance_after: normalizeAiCreditValue(detail.purchased_balance_after) ?? 0,
    purchased_balance_before: normalizeAiCreditValue(detail.purchased_balance_before) ?? 0,
    subscription_balance_after: normalizeAiCreditValue(detail.subscription_balance_after) ?? 0,
    subscription_balance_before: normalizeAiCreditValue(detail.subscription_balance_before) ?? 0,
  };
}

function roundAiCreditNumber(value: number, precision: number = AI_CREDITS_PRECISION): number {
  const safePrecision =
    Number.isInteger(precision) && precision >= 0 ? precision : AI_CREDITS_PRECISION;
  const rounded = Number(value.toFixed(safePrecision));
  return Object.is(rounded, -0) ? 0 : rounded;
}

function formatAiCreditNumber(value: number): string {
  const rounded = roundAiCreditNumber(value);
  return Number.isInteger(rounded) ? String(rounded) : String(rounded);
}
