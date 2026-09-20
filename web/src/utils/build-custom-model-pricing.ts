import type { ModelPricing } from '@/services/types/model';

export type CustomModelMeteredOperation =
  | 'transcription'
  | 'speech_generation'
  | 'music_generation';

export type RealtimeAudioBillingMode = 'text' | 'audio';

interface CustomModelPricingInput {
  useCases: string[];
  currency: string;
  pricePerImage?: string;
  imageBillingMode?: 'per_image' | 'token';
  imageTextInputPricePerMillion?: string;
  imageImageInputPricePerMillion?: string;
  imageOutputPricePerMillion?: string;
  videoRates?: Array<{ resolution?: string; pricePerSecond?: string }>;
  speechToTextPricePerHour?: string;
  textToSpeechPricePer10kChars?: string;
  musicPricePerTrack?: string;
  realtimeAudioBillingMode?: RealtimeAudioBillingMode;
  realtimeAudioInputPricePerMillion?: string;
  realtimeAudioOutputPricePerMillion?: string;
}

/**
 * Canonical metered billing contracts. Keep these in sync with the backend
 * `meteredPricingContracts` map so the editor and the pricing validator agree.
 */
const METERED_CONTRACTS: Record<
  CustomModelMeteredOperation,
  { meter: string; baseUnit: string; perQuantity: number }
> = {
  transcription: {
    meter: 'input_audio_duration',
    baseUnit: 'millisecond',
    perQuantity: 3_600_000,
  },
  speech_generation: {
    meter: 'input_text',
    baseUnit: 'billed_character',
    perQuantity: 10_000,
  },
  music_generation: {
    meter: 'output_track',
    baseUnit: 'track',
    perQuantity: 1,
  },
};

const MEDIA_USE_CASES = [
  'image-gen',
  'video-gen',
  'speech-to-text',
  'text-to-speech',
  'music-gen',
] as const;

const REALTIME_AUDIO_OPERATION = 'realtime_audio';

export function buildCustomModelPricing(input: CustomModelPricingInput): ModelPricing | undefined {
  const tracksImage = input.useCases.includes('image-gen');
  const tracksVideo = input.useCases.includes('video-gen');
  const tracksRealtimeAudio = input.useCases.includes('realtime-audio')
    && input.realtimeAudioBillingMode === 'audio';
  if (!MEDIA_USE_CASES.some(useCase => input.useCases.includes(useCase)) && !tracksRealtimeAudio) {
    return undefined;
  }

  // Editor amounts reach this function already converted to canonical USD, so
  // projections must be labelled USD. Labelling them with the organization's
  // display currency would make the backend convert the already-USD amount a
  // second time for CNY organizations.
  const currency = 'USD';
  const pricing: ModelPricing = {
    deployment_scope: 'global',
    catalog_origin: 'pre_custom',
  };

  if (tracksImage) {
    // An explicit null tells the backend the field was cleared. Omitting the key
    // instead reads as "this editor did not run", so the stored image price
    // would keep billing. Per-image and token billing are mutually exclusive.
    if (input.imageBillingMode === 'token') {
      const tokenPricing: Record<string, unknown> = { currency };
      if (input.imageTextInputPricePerMillion?.trim()) {
        tokenPricing.text_input_price_per_million = Number(input.imageTextInputPricePerMillion);
      }
      if (input.imageImageInputPricePerMillion?.trim()) {
        tokenPricing.image_input_price_per_million = Number(input.imageImageInputPricePerMillion);
      }
      if (input.imageOutputPricePerMillion?.trim()) {
        tokenPricing.output_price_per_million = Number(input.imageOutputPricePerMillion);
      }
      const hasTokenPrice = Object.keys(tokenPricing).length > 1;
      pricing.image_token_pricing = hasTokenPrice ? tokenPricing : null;
      pricing.price_per_image = null;
    } else {
      pricing.price_per_image = input.pricePerImage?.trim() ? Number(input.pricePerImage) : null;
    }
  }

  if (tracksVideo) {
    const rates = (input.videoRates ?? [])
      .map(rate => {
        const price = rate.pricePerSecond?.trim();
        if (!price) return null;
        return {
          resolution: rate.resolution?.trim() || '720p',
          price_per_second: Number(price),
        };
      })
      .filter((rate): rate is { resolution: string; price_per_second: number } => rate !== null);
    pricing.video_generation = rates.length > 0
      ? { currency, billing_unit: 'second', rates }
      : null;
  }

  if (tracksRealtimeAudio) {
    // Audio-token billing stores authoritative realtime_audio rules. Text-token
    // billing leaves the chat scalar columns in charge instead.
    const rules: Array<Record<string, unknown>> = [];
    const inputPrice = input.realtimeAudioInputPricePerMillion?.trim();
    const outputPrice = input.realtimeAudioOutputPricePerMillion?.trim();
    if (inputPrice) {
      rules.push({ operation: REALTIME_AUDIO_OPERATION, meter: 'input_token', unit: 'per_1m_tokens', price: inputPrice, configured: true });
    }
    if (outputPrice) {
      rules.push({ operation: REALTIME_AUDIO_OPERATION, meter: 'output_token', unit: 'per_1m_tokens', price: outputPrice, configured: true });
    }
    if (rules.length > 0) {
      pricing.rules = rules;
    }
  }

  const metered = buildMeteredPricing(input, currency);
  if (metered.length > 0) {
    pricing.metered = metered;
  }

  return pricing;
}

function buildMeteredPricing(
  input: CustomModelPricingInput,
  currency: string
): Array<Record<string, unknown>> {
  const entries: Array<Record<string, unknown>> = [];
  const append = (operation: CustomModelMeteredOperation, raw: string | undefined) => {
    const amount = raw?.trim();
    if (!amount) return;
    const contract = METERED_CONTRACTS[operation];
    entries.push({
      operation,
      meter: contract.meter,
      base_unit: contract.baseUnit,
      // The backend validator requires the amount to be a decimal string.
      price: { amount, currency, per_quantity: contract.perQuantity },
    });
  };

  if (input.useCases.includes('speech-to-text')) {
    append('transcription', input.speechToTextPricePerHour);
  }
  if (input.useCases.includes('text-to-speech')) {
    append('speech_generation', input.textToSpeechPricePer10kChars);
  }
  if (input.useCases.includes('music-gen')) {
    append('music_generation', input.musicPricePerTrack);
  }
  return entries;
}

export function getImageBillingMode(pricing?: ModelPricing | null): 'per_image' | 'token' {
  return pricing?.image_token_pricing && typeof pricing.image_token_pricing === 'object' ? 'token' : 'per_image';
}

export function readImageTokenPrice(
  pricing: ModelPricing | null | undefined,
  key: 'text_input_price_per_million' | 'image_input_price_per_million' | 'output_price_per_million'
): string {
  const tokenPricing = pricing?.image_token_pricing;
  if (!tokenPricing || typeof tokenPricing !== 'object') return '';
  const value = (tokenPricing as Record<string, unknown>)[key];
  if (value === undefined || value === null) return '';
  return String(value);
}

export function readCustomModelVideoRates(
  pricing?: ModelPricing | null
): Array<{ resolution: string; pricePerSecond: string }> {
  const videoGeneration = pricing?.video_generation;
  if (!videoGeneration || typeof videoGeneration !== 'object') return [];
  const record = videoGeneration as { rates?: Array<{ resolution?: unknown; price_per_second?: unknown }> };
  if (!Array.isArray(record.rates)) return [];
  return record.rates.map(rate => ({
    resolution: rate?.resolution === undefined || rate?.resolution === null ? '' : String(rate.resolution),
    pricePerSecond:
      rate?.price_per_second === undefined || rate?.price_per_second === null
        ? ''
        : String(rate.price_per_second),
  }));
}

/** Audio mode is encoded by a priced realtime_audio input rule. */
export function getRealtimeAudioBillingMode(pricing?: ModelPricing | null): RealtimeAudioBillingMode {
  const rules = pricing?.rules;
  if (!Array.isArray(rules)) return 'text';
  const hasAudioRule = rules.some(rule => {
    if (!rule || typeof rule !== 'object') return false;
    const record = rule as Record<string, unknown>;
    return record.operation === 'realtime_audio' && record.meter === 'input_token';
  });
  return hasAudioRule ? 'audio' : 'text';
}

export function readRealtimeAudioPrice(
  pricing: ModelPricing | null | undefined,
  meter: 'input_token' | 'output_token'
): string {
  const rules = pricing?.rules;
  if (!Array.isArray(rules)) return '';
  for (const rule of rules) {
    if (!rule || typeof rule !== 'object') continue;
    const record = rule as Record<string, unknown>;
    if (record.operation !== 'realtime_audio' || record.meter !== meter) continue;
    const price = record.price;
    if (price === undefined || price === null) continue;
    return String(price);
  }
  return '';
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

export function readCustomModelMeteredPrice(
  pricing: ModelPricing | null | undefined,
  operation: CustomModelMeteredOperation
): string {
  const metered = pricing?.metered;
  if (!Array.isArray(metered)) return '';
  const contract = METERED_CONTRACTS[operation];
  for (const entry of metered) {
    if (!entry || typeof entry !== 'object') continue;
    const record = entry as Record<string, unknown>;
    if (record.operation !== operation) continue;
    if (record.meter !== contract.meter || record.base_unit !== contract.baseUnit) continue;
    const price = record.price as Record<string, unknown> | undefined;
    if (Number(price?.per_quantity) !== contract.perQuantity) continue;
    const amount = price?.amount;
    if (amount === undefined || amount === null) continue;
    return String(amount);
  }
  return '';
}
