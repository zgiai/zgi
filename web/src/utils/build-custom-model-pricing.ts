import type { ModelPricing } from '@/services/types/model';

type CustomModelPricingInput = {
  useCases: string[];
  currency: string;
  pricePerImage?: string;
  videoPricePerSecond?: string;
};

export function buildCustomModelPricing(input: CustomModelPricingInput): ModelPricing | undefined {
  const tracksImage = input.useCases.includes('image-gen');
  const tracksVideo = input.useCases.includes('video-gen');
  if (!tracksImage && !tracksVideo) {
    return undefined;
  }

  const pricing: ModelPricing = {
    deployment_scope: 'global',
    catalog_origin: 'pre_custom',
  };

  if (tracksImage) {
    // An explicit null tells the backend the field was cleared. Omitting the key
    // instead reads as "this editor did not run", so the stored image price
    // would keep billing.
    pricing.price_per_image = input.pricePerImage?.trim() ? Number(input.pricePerImage) : null;
  }

  if (tracksVideo) {
    pricing.video_generation = input.videoPricePerSecond?.trim()
      ? {
          currency: input.currency || 'USD',
          billing_unit: 'second',
          rates: [{ resolution: '720p', price_per_second: Number(input.videoPricePerSecond) }],
        }
      : null;
  }

  return pricing;
}

export function readCustomModelPricePerImage(pricing?: ModelPricing | null): string {
  if (pricing?.price_per_image === undefined || pricing.price_per_image === null) return '';
  return String(pricing.price_per_image);
}

export function readCustomModelVideoPricePerSecond(pricing?: ModelPricing | null): string {
  const videoGeneration = pricing?.video_generation;
  if (!videoGeneration || typeof videoGeneration !== 'object') return '';
  const rates = (videoGeneration as { rates?: Array<{ price_per_second?: unknown }> }).rates;
  const rate = rates?.[0]?.price_per_second;
  if (rate === undefined || rate === null) return '';
  return String(rate);
}
