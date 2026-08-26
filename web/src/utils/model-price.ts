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
}

interface ModelPriceDisplayLabels {
  unspecifiedResolution?: string;
  withVideoInput?: string;
  withoutVideoInput?: string;
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
}

/**
 * @util Determine whether a model should use image generation pricing display.
 */
export function isImageGenerationModel(useCases?: ModelUseCase[] | null): boolean {
  return Boolean(useCases?.includes('image-gen'));
}

export function isVideoGenerationModel(useCases?: ModelUseCase[] | null): boolean {
  return Boolean(useCases?.includes('video-gen'));
}

export function isInputOnlyPriceModel(useCases?: ModelUseCase[] | null): boolean {
  const cases = useCases ?? [];
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
}: GetModelPriceDisplayParams): ModelPriceDisplayItem[] {
  if (isVideoGenerationModel(useCases)) {
    const videoRows = getVideoPriceDisplayRows(pricing, currency, labels);
    if (videoRows.length > 0) {
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
  const normalizedCurrency = sourceCurrency?.trim().toUpperCase();
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
  if (!videoPricing) return [];

  const billingUnit = videoPricing.billing_unit ?? 'million_video_tokens';
  const unit = videoPriceUnit(billingUnit);
  const currency = videoPricing.currency ?? pricing?.currency ?? fallbackCurrency;
  const rates = flattenVideoRates(videoPricing.rates, videoPricing.resolution_rates);

  return rates
    .map(rate => {
      const price = videoRatePrice(rate, billingUnit);
      if (price === null) return null;
      return {
        detail: videoRateDetail(rate, labels),
        price,
        unit,
        currency,
      };
    })
    .filter((row): row is VideoPriceDisplayRow => row !== null);
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
    flattened.push(...rates);
  }
  if (Array.isArray(resolutionRates)) {
    for (const resolutionRate of resolutionRates) {
      if (!Array.isArray(resolutionRate?.rates)) continue;
      for (const rate of resolutionRate.rates) {
        flattened.push({ ...rate, resolution: rate.resolution ?? resolutionRate.resolution });
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
