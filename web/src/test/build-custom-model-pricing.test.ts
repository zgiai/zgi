import assert from 'node:assert/strict';

// @ts-ignore Node's direct TypeScript runner requires an explicit extension.
import { buildCustomModelPricing, getRealtimeAudioBillingMode, readCustomModelMeteredPrice, readCustomModelPricePerImage, readCustomModelVideoPricePerSecond, readRealtimeAudioPrice } from '../utils/build-custom-model-pricing.ts';

assert.deepEqual(
  buildCustomModelPricing({ useCases: ['image-gen'], currency: 'USD', pricePerImage: '' }),
  { deployment_scope: 'global', catalog_origin: 'pre_custom', price_per_image: null },
  'clearing the image price must be submitted as an explicit null'
);

assert.deepEqual(
  buildCustomModelPricing({ useCases: ['image-gen'], currency: 'USD', pricePerImage: '0.25' }),
  { deployment_scope: 'global', catalog_origin: 'pre_custom', price_per_image: 0.25 },
  'a filled image price is submitted as a number'
);

assert.deepEqual(
  buildCustomModelPricing({ useCases: ['video-gen'], currency: 'USD' }),
  { deployment_scope: 'global', catalog_origin: 'pre_custom', video_generation: null },
  'clearing the video price must be submitted as an explicit null'
);

assert.deepEqual(
  buildCustomModelPricing({
    useCases: ['image-gen', 'video-gen'],
    currency: 'USD',
    pricePerImage: '0.25',
    videoRates: [{ resolution: '720p', pricePerSecond: '0.8' }],
  }),
  {
    deployment_scope: 'global',
    catalog_origin: 'pre_custom',
    price_per_image: 0.25,
    video_generation: {
      currency: 'USD',
      billing_unit: 'second',
      rates: [{ resolution: '720p', price_per_second: 0.8 }],
    },
  },
  'configured image and video prices are submitted together'
);

assert.equal(
  buildCustomModelPricing({ useCases: ['text-chat'], currency: 'USD' }),
  undefined,
  'models without media capability keep token pricing only'
);

assert.deepEqual(
  buildCustomModelPricing({
    useCases: ['speech-to-text'],
    currency: 'USD',
    speechToTextPricePerHour: '0.36',
  }),
  {
    deployment_scope: 'global',
    catalog_origin: 'pre_custom',
    metered: [
      {
        operation: 'transcription',
        meter: 'input_audio_duration',
        base_unit: 'millisecond',
        price: { amount: '0.36', currency: 'USD', per_quantity: 3_600_000 },
      },
    ],
  },
  'speech-to-text is billed per audio hour'
);

assert.deepEqual(
  buildCustomModelPricing({
    useCases: ['text-to-speech'],
    currency: 'USD',
    textToSpeechPricePer10kChars: '0.15',
  }),
  {
    deployment_scope: 'global',
    catalog_origin: 'pre_custom',
    metered: [
      {
        operation: 'speech_generation',
        meter: 'input_text',
        base_unit: 'billed_character',
        price: { amount: '0.15', currency: 'USD', per_quantity: 10_000 },
      },
    ],
  },
  'text-to-speech is billed per 10k characters'
);

assert.deepEqual(
  buildCustomModelPricing({
    useCases: ['music-gen'],
    currency: 'USD',
    musicPricePerTrack: '0.5',
  }),
  {
    deployment_scope: 'global',
    catalog_origin: 'pre_custom',
    metered: [
      {
        operation: 'music_generation',
        meter: 'output_track',
        base_unit: 'track',
        price: { amount: '0.5', currency: 'USD', per_quantity: 1 },
      },
    ],
  },
  'music is billed per output track'
);

assert.deepEqual(
  buildCustomModelPricing({
    useCases: ['speech-to-text', 'text-to-speech', 'music-gen'],
    currency: 'USD',
    speechToTextPricePerHour: '0.36',
    textToSpeechPricePer10kChars: '0.15',
    musicPricePerTrack: '0.5',
  }),
  {
    deployment_scope: 'global',
    catalog_origin: 'pre_custom',
    metered: [
      {
        operation: 'transcription',
        meter: 'input_audio_duration',
        base_unit: 'millisecond',
        price: { amount: '0.36', currency: 'USD', per_quantity: 3_600_000 },
      },
      {
        operation: 'speech_generation',
        meter: 'input_text',
        base_unit: 'billed_character',
        price: { amount: '0.15', currency: 'USD', per_quantity: 10_000 },
      },
      {
        operation: 'music_generation',
        meter: 'output_track',
        base_unit: 'track',
        price: { amount: '0.5', currency: 'USD', per_quantity: 1 },
      },
    ],
  },
  'multi-capability audio models submit every priced meter'
);

assert.deepEqual(
  buildCustomModelPricing({
    useCases: ['speech-to-text'],
    currency: 'CNY',
    speechToTextPricePerHour: '2.52',
  }),
  {
    deployment_scope: 'global',
    catalog_origin: 'pre_custom',
    metered: [
      {
        operation: 'transcription',
        meter: 'input_audio_duration',
        base_unit: 'millisecond',
        // The editor already converted the amount to USD; the label must stay
        // USD so the backend does not convert it a second time.
        price: { amount: '2.52', currency: 'USD', per_quantity: 3_600_000 },
      },
    ],
  },
  'amounts are labelled USD even when the organization displays CNY'
);

assert.equal(
  readCustomModelMeteredPrice(
    {
      metered: [
        {
          operation: 'transcription',
          meter: 'input_audio_duration',
          base_unit: 'millisecond',
          price: { amount: '0.36', currency: 'USD', per_quantity: 3_600_000 },
        },
      ],
    },
    'transcription'
  ),
  '0.36',
  'metered read helper returns the stored decimal string'
);

assert.equal(
  readCustomModelMeteredPrice({ metered: [] }, 'music_generation'),
  '',
  'metered read helper returns empty when the operation is absent'
);

assert.equal(
  readCustomModelPricePerImage({ price_per_image: 0.25 }),
  '0.25',
  'image read helper returns the stored number'
);

assert.equal(
  readCustomModelVideoPricePerSecond({
    video_generation: { rates: [{ price_per_second: 0.8 }] },
  }),
  '0.8',
  'video read helper returns the first rate'
);

assert.deepEqual(
  buildCustomModelPricing({
    useCases: ['realtime-audio'],
    currency: 'USD',
    realtimeAudioBillingMode: 'audio',
    realtimeAudioInputPricePerMillion: '32',
    realtimeAudioOutputPricePerMillion: '64',
  }),
  {
    deployment_scope: 'global',
    catalog_origin: 'pre_custom',
    rules: [
      { operation: 'realtime_audio', meter: 'input_token', unit: 'per_1m_tokens', price: '32', configured: true },
      { operation: 'realtime_audio', meter: 'output_token', unit: 'per_1m_tokens', price: '64', configured: true },
    ],
  },
  'audio-token realtime billing stores realtime_audio rules'
);

assert.equal(
  buildCustomModelPricing({ useCases: ['realtime-audio'], currency: 'USD', realtimeAudioBillingMode: 'text' }),
  undefined,
  'text-token realtime billing keeps using the chat scalar columns'
);

assert.equal(
  getRealtimeAudioBillingMode({ rules: [{ operation: 'realtime_audio', meter: 'input_token', price: '32' }] }),
  'audio',
  'a realtime_audio input rule selects audio billing'
);

assert.equal(
  readRealtimeAudioPrice({ rules: [{ operation: 'realtime_audio', meter: 'output_token', price: '64' }] }, 'output_token'),
  '64',
  'realtime audio read helper returns the stored price'
);

assert.deepEqual(
  buildCustomModelPricing({
    useCases: ['image-gen'],
    currency: 'USD',
    imageBillingMode: 'token',
    imageTextInputPricePerMillion: '5',
    imageImageInputPricePerMillion: '10',
    imageOutputPricePerMillion: '40',
  }),
  {
    deployment_scope: 'global',
    catalog_origin: 'pre_custom',
    price_per_image: null,
    image_token_pricing: {
      currency: 'USD',
      text_input_price_per_million: 5,
      image_input_price_per_million: 10,
      output_price_per_million: 40,
    },
  },
  'image token billing emits image_token_pricing and clears the per-image price'
);

assert.deepEqual(
  buildCustomModelPricing({
    useCases: ['video-gen'],
    currency: 'USD',
    videoRates: [
      { resolution: '720p', pricePerSecond: '0.8' },
      { resolution: '1080p', pricePerSecond: '1.2' },
    ],
  }),
  {
    deployment_scope: 'global',
    catalog_origin: 'pre_custom',
    video_generation: {
      currency: 'USD',
      billing_unit: 'second',
      rates: [
        { resolution: '720p', price_per_second: 0.8 },
        { resolution: '1080p', price_per_second: 1.2 },
      ],
    },
  },
  'video billing emits one rate per resolution'
);

console.log('build-custom-model-pricing tests passed');
