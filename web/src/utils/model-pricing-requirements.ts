import type { ModelUseCase } from '@/services/types/model';

export type RealtimeAudioBillingMode = 'text' | 'audio';

export interface ModelPricingFormValues {
  useCases: string[];
  realtimeAudioBillingMode?: RealtimeAudioBillingMode;
  inputPrice?: string | null;
  outputPrice?: string | null;
  pricePerImage?: string | null;
  imageBillingMode?: 'per_image' | 'token';
  imageTokenTextPrice?: string | null;
  imageTokenImagePrice?: string | null;
  imageTokenOutputPrice?: string | null;
  videoPricePerSecond?: string | null;
  videoRates?: Array<{ resolution?: string; pricePerSecond?: string | null }>;
  speechToTextPrice?: string | null;
  textToSpeechPrice?: string | null;
  musicPricePerTrack?: string | null;
  realtimeAudioInputPrice?: string | null;
  realtimeAudioOutputPrice?: string | null;
}

export interface MissingModelPricing {
  useCase: string;
  field: keyof ModelPricingFormValues;
}

/** A price is configured when it parses to a finite, non-negative number. 0 is free. */
export function isPricingValueConfigured(raw: unknown): boolean {
  if (typeof raw === 'number') return Number.isFinite(raw) && raw >= 0;
  if (typeof raw === 'string') {
    const trimmed = raw.trim();
    return trimmed !== '' && Number.isFinite(Number(trimmed)) && Number(trimmed) >= 0;
  }
  return false;
}

const TOKEN_USE_CASES = [
  'text-chat',
  'function-calling',
  'agent',
  'vision',
  'reasoning',
] as const;

const FIELD_BY_USE_CASE: Partial<Record<ModelUseCase, keyof ModelPricingFormValues>> = {
  embedding: 'inputPrice',
  rerank: 'inputPrice',
  'speech-to-text': 'speechToTextPrice',
  'text-to-speech': 'textToSpeechPrice',
  'music-gen': 'musicPricePerTrack',
};

/**
 * Returns the required price fields still missing for the selected
 * capabilities. Mirrors the backend MissingPricingForUseCases semantics:
 * realtime audio may bill by text tokens or by audio in/out tokens.
 */
export function missingModelPricing(values: ModelPricingFormValues): MissingModelPricing[] {
  const missing: MissingModelPricing[] = [];
  const useCases = new Set(values.useCases ?? []);

  for (const useCase of TOKEN_USE_CASES) {
    if (!useCases.has(useCase)) continue;
    if (!isPricingValueConfigured(values.inputPrice)) missing.push({ useCase, field: 'inputPrice' });
    if (!isPricingValueConfigured(values.outputPrice)) missing.push({ useCase, field: 'outputPrice' });
  }

  if (useCases.has('realtime-audio')) {
    if (values.realtimeAudioBillingMode === 'audio') {
      if (!isPricingValueConfigured(values.realtimeAudioInputPrice)) {
        missing.push({ useCase: 'realtime-audio', field: 'realtimeAudioInputPrice' });
      }
      if (!isPricingValueConfigured(values.realtimeAudioOutputPrice)) {
        missing.push({ useCase: 'realtime-audio', field: 'realtimeAudioOutputPrice' });
      }
    } else {
      // Text-token billing reuses the chat input/output columns.
      if (!isPricingValueConfigured(values.inputPrice)) missing.push({ useCase: 'realtime-audio', field: 'inputPrice' });
      if (!isPricingValueConfigured(values.outputPrice)) missing.push({ useCase: 'realtime-audio', field: 'outputPrice' });
    }
  }

  if (useCases.has('image-gen')) {
    if (values.imageBillingMode === 'token') {
      // Token billing needs at least one of the three upstream token prices.
      const hasTokenPrice =
        isPricingValueConfigured(values.imageTokenTextPrice) ||
        isPricingValueConfigured(values.imageTokenImagePrice) ||
        isPricingValueConfigured(values.imageTokenOutputPrice);
      if (!hasTokenPrice) missing.push({ useCase: 'image-gen', field: 'imageTokenTextPrice' });
    } else if (!isPricingValueConfigured(values.pricePerImage)) {
      missing.push({ useCase: 'image-gen', field: 'pricePerImage' });
    }
  }

  if (useCases.has('video-gen')) {
    const hasPricedRate = (values.videoRates ?? []).some(rate =>
      isPricingValueConfigured(rate?.pricePerSecond)
    );
    if (!hasPricedRate) missing.push({ useCase: 'video-gen', field: 'videoPricePerSecond' });
  }

  for (const [useCase, field] of Object.entries(FIELD_BY_USE_CASE) as Array<
    [ModelUseCase, keyof ModelPricingFormValues]
  >) {
    if (!useCases.has(useCase)) continue;
    if (!isPricingValueConfigured(values[field])) missing.push({ useCase, field });
  }

  return missing;
}

export const MISSING_PRICING_FIELD_LABELS: Record<string, string> = {
  inputPrice: 'input_price',
  outputPrice: 'output_price',
  pricePerImage: 'price_per_image',
  imageTokenTextPrice: 'image_token_text_price',
  imageTokenImagePrice: 'image_token_image_price',
  imageTokenOutputPrice: 'image_token_output_price',
  videoPricePerSecond: 'video_price_per_second',
  speechToTextPrice: 'speech_to_text_price',
  textToSpeechPrice: 'text_to_speech_price',
  musicPricePerTrack: 'music_price_per_track',
  realtimeAudioInputPrice: 'realtime_audio_input_price',
  realtimeAudioOutputPrice: 'realtime_audio_output_price',
};
