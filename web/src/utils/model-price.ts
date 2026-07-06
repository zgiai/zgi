import type { ModelUseCase } from '@/services/types/model';
import {
  type BillingDisplaySettings,
  DEFAULT_BILLING_DISPLAY,
  formatBillingDisplayAmountFromUSD,
} from '@/utils/billing-display';

export type ModelPriceLabel = 'input' | 'output' | 'image';
export type ModelPriceUnit = 'perMillionTokens' | 'perImage';

export interface ModelPriceDisplayItem {
  label: ModelPriceLabel;
  formattedValue: string;
  unit: ModelPriceUnit;
  isConfigured: boolean;
  isFree: boolean;
}

interface GetModelPriceDisplayParams {
  inputPrice?: number | null;
  outputPrice?: number | null;
  inputPriceConfigured?: boolean | null;
  outputPriceConfigured?: boolean | null;
  useCases?: ModelUseCase[] | null;
  billingDisplay?: BillingDisplaySettings;
}

/**
 * @util Determine whether a model should use image generation pricing display.
 */
export function isImageGenerationModel(useCases?: ModelUseCase[] | null): boolean {
  return Boolean(useCases?.includes('image-gen'));
}

export function isInputOnlyPriceModel(useCases?: ModelUseCase[] | null): boolean {
  const cases = useCases ?? [];
  return (
    cases.some(useCase => useCase === 'embedding' || useCase === 'rerank') &&
    !cases.some(useCase =>
      ['text-chat', 'vision', 'reasoning', 'function-calling', 'image-gen'].includes(useCase)
    )
  );
}

/**
 * @util Build USD price display lines for model management tables.
 */
export function getModelPriceDisplay({
  inputPrice,
  outputPrice,
  inputPriceConfigured,
  outputPriceConfigured,
  useCases,
  billingDisplay = DEFAULT_BILLING_DISPLAY,
}: GetModelPriceDisplayParams): ModelPriceDisplayItem[] {
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
