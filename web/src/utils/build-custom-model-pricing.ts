import type { ModelPricing } from '@/services/types/model';

type CustomModelPricingInput = {
  useCases: string[];
  currency: string;
  pricePerImage?: string;
  videoPricePerSecond?: string;
};

export function buildCustomModelPricing(input: CustomModelPricingInput): ModelPricing | undefined {
  const pricing: ModelPricing = {
    deployment_scope: 'global',
    catalog_origin: 'pre_custom',
  };

  if (input.useCases.includes('image-gen') && input.pricePerImage?.trim()) {
    pricing.price_per_image = Number(input.pricePerImage);
  }

  if (input.useCases.includes('video-gen') && input.videoPricePerSecond?.trim()) {
    pricing.video_generation = {
      currency: input.currency || 'USD',
      billing_unit: 'second',
      rates: [{ resolution: '720p', price_per_second: Number(input.videoPricePerSecond) }],
    };
  }

  if (pricing.price_per_image === undefined && !pricing.video_generation) {
    return undefined;
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
