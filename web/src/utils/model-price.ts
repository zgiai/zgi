import type { ModelUseCase } from '@/services/types/model';
import Decimal from 'decimal.js-light';
import {
  type BillingDisplaySettings,
  DEFAULT_BILLING_DISPLAY,
  formatBillingDisplayAmountFromUSD,
} from '@/utils/billing-display';

export type ModelPriceLabel = 'input' | 'output' | 'image' | 'video';
export type ModelPriceUnit =
  | 'perMillionTokens'
  | 'perImage'
  | 'perMillionVideoTokens'
  | 'perSecond'
  | 'perTask';

export interface ModelPriceDisplayItem {
  label: ModelPriceLabel;
  formattedValue: string;
  unit: ModelPriceUnit;
  isConfigured: boolean;
  isFree: boolean;
  detail?: string;
  displayUnit?: string;
}

interface ModelPriceDisplayLabels {
  unspecifiedResolution?: string;
  withVideoInput?: string;
  withoutVideoInput?: string;
  image?: string;
  input?: string;
  output?: string;
  speechGeneration?: string;
  transcription?: string;
  musicGeneration?: string;
  lyricsGeneration?: string;
  meteredPrice?: string;
  perImage?: string;
  perMillionTokens?: string;
  perTenThousandCharacters?: string;
  perHour?: string;
  perTrack?: string;
  perRequest?: string;
  perQuantity?: (quantity: number, unit: string) => string;
  perUnit?: (unit: string) => string;
}

interface VideoGenerationPricingRate {
  model_tier?: string | null;
  mode?: string | null;
  resolution?: string | null;
  duration_seconds?: number | string | null;
  input_video?: boolean | null;
  audio?: boolean | null;
  price_per_second?: number | string | null;
  price_per_task?: number | string | null;
  price_per_million_tokens?: number | string | null;
}

interface VideoGenerationResolutionRate {
  resolution?: string | null;
  rates?: VideoGenerationPricingRate[] | null;
}

interface StructuredModelPricing {
  currency?: string | null;
  price_per_image?: number | null;
  token_tiers?: Array<{
    min_input_tokens?: number | null;
    input_price_per_million?: number | null;
    output_price_per_million?: number | null;
  }> | null;
  metered?: Array<{
    operation?: string | null;
    meter?: string | null;
    base_unit?: string | null;
    price?: {
      amount?: number | string | null;
      currency?: string | null;
      per_quantity?: number | null;
    } | null;
  }> | null;
  video_generation?: {
    currency?: string | null;
    billing_unit?: 'second' | 'task' | 'million_video_tokens' | string | null;
    rates?: VideoGenerationPricingRate[] | null;
    resolution_rates?: VideoGenerationResolutionRate[] | null;
  } | null;
}

interface VideoPriceDisplayRow {
  detail: string;
  price: string;
  unit: ModelPriceUnit;
  currency: string | null | undefined;
}

interface GetModelPriceDisplayParams {
  inputPrice?: number | null;
  outputPrice?: number | null;
  inputPriceConfigured?: boolean | null;
  outputPriceConfigured?: boolean | null;
  pricing?: StructuredModelPricing | null;
  currency: string | null | undefined;
  useCases?: ModelUseCase[] | null;
  billingDisplay?: BillingDisplaySettings;
  labels?: ModelPriceDisplayLabels;
  videoDisplayMode?: 'detailed' | 'summary';
}

/**
 * @util Determine whether a model should use image generation pricing display.
 */
export function isImageGenerationModel(useCases?: ModelUseCase[] | null): boolean {
  return Array.isArray(useCases) && useCases.includes('image-gen');
}

export function isVideoGenerationModel(useCases?: ModelUseCase[] | null): boolean {
  return Array.isArray(useCases) && useCases.includes('video-gen');
}

export function isInputOnlyPriceModel(useCases?: ModelUseCase[] | null): boolean {
  const cases = Array.isArray(useCases) ? useCases : [];
  return (
    cases.some(useCase => useCase === 'embedding' || useCase === 'rerank') &&
    !cases.some(useCase =>
      ['text-chat', 'vision', 'reasoning', 'function-calling', 'image-gen', 'video-gen'].includes(
        useCase
      )
    )
  );
}

/**
 * @util Build price display lines for model management tables.
 */
export function getModelPriceDisplay({
  inputPrice,
  outputPrice,
  inputPriceConfigured,
  outputPriceConfigured,
  pricing,
  currency,
  useCases,
  billingDisplay = DEFAULT_BILLING_DISPLAY,
  labels,
  videoDisplayMode = 'detailed',
}: GetModelPriceDisplayParams): ModelPriceDisplayItem[] {
  if (isVideoGenerationModel(useCases)) {
    const videoRows = getVideoPriceDisplayRows(pricing, currency, labels);
    if (videoRows.length > 0) {
      if (videoDisplayMode === 'summary') return summarizeVideoPriceRows(videoRows);
      return videoRows.map(row =>
        buildSourceCurrencyModelPriceDisplayItem(
          'video',
          row.price,
          row.currency ?? currency,
          row.unit,
          billingDisplay,
          row.detail
        )
      );
    }
    return [
      buildModelPriceDisplayItem('video', null, false, 'perMillionVideoTokens', billingDisplay),
    ];
  }

  const structuredItems = getStructuredPriceDisplay(pricing, currency, labels);
  if (structuredItems.length > 0) return structuredItems;

  if (isImageGenerationModel(useCases)) {
    if (outputPriceConfigured) {
      return [buildModelPriceDisplayItem('image', outputPrice, true, 'perImage', billingDisplay)];
    }
    return [
      buildModelPriceDisplayItem(
        'image',
        inputPrice,
        Boolean(inputPriceConfigured),
        'perImage',
        billingDisplay
      ),
    ];
  }

  if (isInputOnlyPriceModel(useCases)) {
    return [
      buildModelPriceDisplayItem(
        'input',
        inputPrice,
        Boolean(inputPriceConfigured),
        'perMillionTokens',
        billingDisplay
      ),
    ];
  }

  return [
    buildModelPriceDisplayItem(
      'input',
      inputPrice,
      Boolean(inputPriceConfigured),
      'perMillionTokens',
      billingDisplay
    ),
    buildModelPriceDisplayItem(
      'output',
      outputPrice,
      Boolean(outputPriceConfigured),
      'perMillionTokens',
      billingDisplay
    ),
  ];
}

function getStructuredPriceDisplay(
  pricing: StructuredModelPricing | null | undefined,
  fallbackCurrency: string | null | undefined,
  labels?: ModelPriceDisplayLabels
): ModelPriceDisplayItem[] {
  if (!isRecord(pricing)) return [];

  if (typeof pricing.price_per_image === 'number' && Number.isFinite(pricing.price_per_image)) {
    return [
      buildStructuredPriceItem(
        'image',
        labels?.image ?? '图像',
        pricing.price_per_image,
        safeCurrency(fallbackCurrency, pricing.currency, 'CNY'),
        labels?.perImage ?? '/ 张'
      ),
    ];
  }

  const tokenTier = (Array.isArray(pricing.token_tiers) ? pricing.token_tiers : [])
    .filter(isRecord)
    .sort(
      (left, right) =>
        finiteNumber(left.min_input_tokens) - finiteNumber(right.min_input_tokens)
    )[0];
  if (tokenTier) {
    return [
      buildOptionalStructuredPriceItem(
        'input',
        labels?.input ?? '输入',
        finiteNumberOrUndefined(tokenTier.input_price_per_million),
        safeCurrency(fallbackCurrency, pricing.currency, 'USD'),
        labels?.perMillionTokens ?? '/ 百万 tokens'
      ),
      buildOptionalStructuredPriceItem(
        'output',
        labels?.output ?? '输出',
        finiteNumberOrUndefined(tokenTier.output_price_per_million),
        safeCurrency(fallbackCurrency, pricing.currency, 'USD'),
        labels?.perMillionTokens ?? '/ 百万 tokens'
      ),
    ].filter((item): item is ModelPriceDisplayItem => item !== null);
  }

  const metered = Array.isArray(pricing.metered) ? pricing.metered : [];
  return metered.flatMap(rawItem => {
    if (!isRecord(rawItem)) return [];
    const price = isRecord(rawItem.price) ? rawItem.price : undefined;
    const amount = Number(price?.amount);
    if (!Number.isFinite(amount)) return [];
    return [
      buildStructuredPriceItem(
        'output',
        structuredOperationLabel(
          nonEmptyString(rawItem.operation) || nonEmptyString(rawItem.meter) || 'metered',
          labels
        ),
        amount,
        safeCurrency(price?.currency, fallbackCurrency, pricing.currency, 'CNY'),
        structuredUnitLabel(
          nonEmptyString(rawItem.base_unit),
          finiteNumberOrUndefined(price?.per_quantity),
          labels
        )
      ),
    ];
  });
}

function buildOptionalStructuredPriceItem(
  label: ModelPriceLabel,
  detail: string,
  amount: number | null | undefined,
  currency: string,
  displayUnit: string
): ModelPriceDisplayItem | null {
  if (typeof amount !== 'number' || !Number.isFinite(amount)) return null;
  return buildStructuredPriceItem(label, detail, amount, currency, displayUnit);
}

function buildStructuredPriceItem(
  label: ModelPriceLabel,
  detail: string,
  amount: number,
  currency: string,
  displayUnit: string
): ModelPriceDisplayItem {
  return {
    label,
    detail,
    formattedValue: formatStructuredAmount(amount, currency),
    unit: label === 'image' ? 'perImage' : 'perMillionTokens',
    isConfigured: true,
    isFree: amount === 0,
    displayUnit,
  };
}

function formatStructuredAmount(amount: number, currency: string): string {
  const normalizedCurrency = safeCurrency(currency, 'USD').trim().toUpperCase();
  const symbol =
    normalizedCurrency === 'CNY' ? '¥' : normalizedCurrency === 'USD' ? '$' : `${currency} `;
  return `${symbol}${amount.toLocaleString(undefined, {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  })}`;
}

function structuredOperationLabel(operation: string, labels?: ModelPriceDisplayLabels): string {
  const operationLabels: Record<string, string> = {
    speech_generation: labels?.speechGeneration ?? '语音合成',
    transcription: labels?.transcription ?? '语音识别',
    music_generation: labels?.musicGeneration ?? '音乐生成',
    lyrics_generation: labels?.lyricsGeneration ?? '歌词生成',
  };
  return operationLabels[operation] || labels?.meteredPrice || '计量价格';
}

function structuredUnitLabel(
  baseUnit?: string | null,
  quantity?: number | null,
  labels?: ModelPriceDisplayLabels
): string {
  if (baseUnit === 'billed_character' && quantity === 10_000) {
    return labels?.perTenThousandCharacters ?? '/ 万字符';
  }
  if (baseUnit === 'millisecond' && quantity === 3_600_000) return labels?.perHour ?? '/ 小时';
  if (baseUnit === 'track') return labels?.perTrack ?? '/ 首';
  if (baseUnit === 'request') return labels?.perRequest ?? '/ 次';
  if (quantity && quantity !== 1) {
    return (
      labels?.perQuantity?.(quantity, baseUnit || '单位') ?? `/ ${quantity} ${baseUnit || '单位'}`
    );
  }
  return labels?.perUnit?.(baseUnit || '次') ?? `/ ${baseUnit || '次'}`;
}

function buildModelPriceDisplayItem(
  label: ModelPriceLabel,
  price: number | null | undefined,
  isConfigured: boolean,
  unit: ModelPriceUnit,
  billingDisplay: BillingDisplaySettings
): ModelPriceDisplayItem {
  return {
    label,
    formattedValue: isConfigured
      ? formatBillingDisplayAmountFromUSD(price ?? 0, billingDisplay)
      : '-',
    unit,
    isConfigured,
    isFree: isConfigured && (price ?? 0) === 0,
  };
}

function buildSourceCurrencyModelPriceDisplayItem(
  label: ModelPriceLabel,
  price: number | string,
  sourceCurrency: string | null | undefined,
  unit: ModelPriceUnit,
  billingDisplay: BillingDisplaySettings,
  detail?: string
): ModelPriceDisplayItem {
  return {
    label,
    detail,
    formattedValue: formatSourceCurrencyAmount(price, sourceCurrency, billingDisplay),
    unit,
    isConfigured: true,
    isFree: decimalIsZero(price),
  };
}

function formatSourceCurrencyAmount(
  price: number | string,
  sourceCurrency: string | null | undefined,
  billingDisplay: BillingDisplaySettings
): string {
  const normalizedCurrency = nonEmptyString(sourceCurrency)?.toUpperCase();
  try {
    const priceUSD =
      normalizedCurrency === 'CNY'
        ? new Decimal(price).div(billingDisplay.usdToCnyRate)
        : new Decimal(price);
    return formatBillingDisplayAmountFromUSD(priceUSD.toString(), billingDisplay);
  } catch {
    return '-';
  }
}

function getVideoPriceDisplayRows(
  pricing: StructuredModelPricing | null | undefined,
  fallbackCurrency: string | null | undefined,
  labels: ModelPriceDisplayLabels | undefined
): VideoPriceDisplayRow[] {
  const videoPricing = pricing?.video_generation;
  if (!isRecord(videoPricing)) return [];

  const billingUnit = nonEmptyString(videoPricing.billing_unit) ?? 'million_video_tokens';
  const unit = videoPriceUnit(billingUnit);
  const currency = safeCurrencyOrUndefined(
    videoPricing.currency,
    pricing?.currency,
    fallbackCurrency
  );
  const rates = flattenVideoRates(videoPricing.rates, videoPricing.resolution_rates);

  return rates.flatMap(rate => {
    const price = videoRatePrice(rate, billingUnit);
    if (price === null) return [];
    return [
      {
        detail: videoRateDetail(rate, labels),
        price,
        unit,
        currency,
      } satisfies VideoPriceDisplayRow,
    ];
  });
}

function summarizeVideoPriceRows(rows: VideoPriceDisplayRow[]): ModelPriceDisplayItem[] {
  const groups = [
    { detail: '有参考视频', rows: rows.filter(row => row.detail.includes('含视频输入')) },
    { detail: '无参考视频', rows: rows.filter(row => row.detail.includes('无视频输入')) },
  ];

  return groups.flatMap(group => {
    if (group.rows.length === 0) return [];
    const prices = group.rows.map(row => Number(row.price)).filter(Number.isFinite);
    if (prices.length === 0) return [];
    const min = Math.min(...prices);
    const max = Math.max(...prices);
    const currency = group.rows[0]?.currency || 'CNY';
    return [
      {
        label: 'video' as const,
        detail: group.detail,
        formattedValue:
          min === max
            ? formatStructuredAmount(min, currency)
            : `${formatStructuredAmount(min, currency)}–${formatStructuredAmount(max, currency)}`,
        unit: 'perMillionVideoTokens' as const,
        isConfigured: true,
        isFree: min === 0 && max === 0,
        displayUnit: '/ 百万视频 tokens',
      },
    ];
  });
}

function videoPriceUnit(billingUnit: string | null | undefined): ModelPriceUnit {
  if (billingUnit === 'second') return 'perSecond';
  if (billingUnit === 'task') return 'perTask';
  return 'perMillionVideoTokens';
}

function flattenVideoRates(
  rates: VideoGenerationPricingRate[] | null | undefined,
  resolutionRates: VideoGenerationResolutionRate[] | null | undefined
): VideoGenerationPricingRate[] {
  const flattened: VideoGenerationPricingRate[] = [];
  if (Array.isArray(rates)) {
    flattened.push(...rates.filter(isRecord));
  }
  if (Array.isArray(resolutionRates)) {
    for (const resolutionRate of resolutionRates) {
      if (!isRecord(resolutionRate)) continue;
      if (!Array.isArray(resolutionRate?.rates)) continue;
      for (const rate of resolutionRate.rates) {
        if (!isRecord(rate)) continue;
        flattened.push({
          ...rate,
          resolution: nonEmptyString(rate.resolution) ?? nonEmptyString(resolutionRate.resolution),
        });
      }
    }
  }
  return flattened;
}

function videoRatePrice(
  rate: VideoGenerationPricingRate,
  billingUnit: string | null | undefined
): string | null {
  const rawPrice =
    billingUnit === 'second'
      ? rate.price_per_second
      : billingUnit === 'task'
        ? rate.price_per_task
        : rate.price_per_million_tokens;
  if (rawPrice === undefined || rawPrice === null) return null;
  try {
    const price = new Decimal(rawPrice);
    return price.isNegative() ? null : price.toString();
  } catch {
    return null;
  }
}

function decimalIsZero(value: number | string): boolean {
  try {
    return new Decimal(value).isZero();
  } catch {
    return false;
  }
}

function videoRateDetail(
  rate: VideoGenerationPricingRate,
  labels: ModelPriceDisplayLabels | undefined
): string {
  const parts = [
    nonEmptyString(rate.resolution) ?? labels?.unspecifiedResolution,
    rate.duration_seconds !== undefined && rate.duration_seconds !== null
      ? `${rate.duration_seconds}s`
      : undefined,
    nonEmptyString(rate.model_tier),
    nonEmptyString(rate.mode),
    rate.input_video === true
      ? labels?.withVideoInput
      : rate.input_video === false
        ? labels?.withoutVideoInput
        : undefined,
  ].filter((part): part is string => Boolean(part));

  return parts.join(' / ');
}

function nonEmptyString(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined;
  const trimmed = value.trim();
  return trimmed === '' ? undefined : trimmed;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function finiteNumber(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0;
}

function finiteNumberOrUndefined(value: unknown): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined;
}

function safeCurrency(...values: unknown[]): string {
  return safeCurrencyOrUndefined(...values) ?? 'USD';
}

function safeCurrencyOrUndefined(...values: unknown[]): string | undefined {
  for (const value of values) {
    const currency = nonEmptyString(value);
    if (currency) return currency;
  }
  return undefined;
}
