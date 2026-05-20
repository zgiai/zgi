import type { Locale } from '@/i18n';
import type { ModelUseCase } from '@/services/types/model';

const CNY_PER_USD = 7;

export type ModelPriceLabel = 'input' | 'output';
export type ModelPriceUnit = 'perMillionTokens' | 'perImage';

export interface ModelPriceDisplayItem {
  label: ModelPriceLabel;
  formattedValue: string;
  unit: ModelPriceUnit;
}

interface GetModelPriceDisplayParams {
  inputPrice?: number | null;
  outputPrice?: number | null;
  useCases?: ModelUseCase[] | null;
  locale: Locale | string;
}

/**
 * @util Determine whether a model should use image generation pricing display.
 */
export function isImageGenerationModel(useCases?: ModelUseCase[] | null): boolean {
  return Boolean(useCases?.includes('image-gen'));
}

/**
 * @util Build localized price display lines for model management tables.
 */
export function getModelPriceDisplay({
  inputPrice,
  outputPrice,
  useCases,
  locale,
}: GetModelPriceDisplayParams): ModelPriceDisplayItem[] {
  if (isImageGenerationModel(useCases)) {
    return [
      {
        label: 'output',
        formattedValue: formatModelPriceValue(inputPrice || outputPrice, locale),
        unit: 'perImage',
      },
    ];
  }

  return [
    {
      label: 'input',
      formattedValue: formatModelPriceValue(inputPrice, locale),
      unit: 'perMillionTokens',
    },
    {
      label: 'output',
      formattedValue: formatModelPriceValue(outputPrice, locale),
      unit: 'perMillionTokens',
    },
  ];
}

function formatModelPriceValue(price?: number | null, locale?: Locale | string): string {
  if (price === undefined || price === null || Number.isNaN(price)) {
    return '-';
  }

  const isChinese = locale === 'zh-Hans';
  const symbol = isChinese ? '￥' : '$';
  const localizedPrice = isChinese ? price * CNY_PER_USD : price;

  return `${symbol}${localizedPrice.toFixed(2)}`;
}
