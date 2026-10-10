import assert from 'node:assert/strict';

// @ts-ignore Node's direct TypeScript runner requires an explicit extension.
import { isPricingValueConfigured, missingModelPricing } from '../utils/model-pricing-requirements.ts';

assert.equal(isPricingValueConfigured('0'), true, 'zero is a valid free price');
assert.equal(isPricingValueConfigured(''), false, 'blank is not configured');
assert.equal(isPricingValueConfigured('abc'), false, 'non-numeric is not configured');

assert.deepEqual(
  missingModelPricing({ useCases: ['text-chat'] }).map(item => item.field).sort(),
  ['inputPrice', 'outputPrice'],
  'text chat requires input and output'
);

assert.deepEqual(
  missingModelPricing({ useCases: ['embedding'] }).map(item => item.field),
  ['inputPrice'],
  'embedding requires input only'
);

assert.deepEqual(
  missingModelPricing({ useCases: ['image-gen', 'video-gen', 'music-gen'] }).map(item => item.field),
  ['pricePerImage', 'videoPricePerSecond', 'musicPricePerTrack'],
  'media capabilities require their own price field'
);

assert.deepEqual(
  missingModelPricing({ useCases: ['speech-to-text', 'text-to-speech'] }).map(item => item.field),
  ['speechToTextPrice', 'textToSpeechPrice'],
  'audio capabilities require their metered price'
);

assert.deepEqual(
  missingModelPricing({ useCases: ['realtime-audio'], realtimeAudioBillingMode: 'text' }).map(item => item.field),
  ['inputPrice', 'outputPrice'],
  'realtime audio text mode uses the chat columns'
);

assert.deepEqual(
  missingModelPricing({
    useCases: ['realtime-audio'],
    realtimeAudioBillingMode: 'audio',
    inputPrice: '1',
    outputPrice: '2',
  }).map(item => item.field),
  ['realtimeAudioInputPrice', 'realtimeAudioOutputPrice'],
  'chat prices must not satisfy realtime audio audio mode'
);

assert.deepEqual(
  missingModelPricing({
    useCases: ['realtime-audio'],
    realtimeAudioBillingMode: 'audio',
    realtimeAudioInputPrice: '32',
    realtimeAudioOutputPrice: '64',
  }),
  [],
  'complete audio token prices pass'
);

assert.deepEqual(
  missingModelPricing({ useCases: ['text-chat'], inputPrice: '1', outputPrice: '0' }),
  [],
  'zero output price counts as configured'
);

assert.deepEqual(
  missingModelPricing({ useCases: ['image-gen'], imageBillingMode: 'token', imageTokenTextPrice: '5' }),
  [],
  'image token mode is satisfied by any one upstream token price'
);

assert.deepEqual(
  missingModelPricing({ useCases: ['image-gen'], imageBillingMode: 'token' }).map(item => item.field),
  ['imageTokenTextPrice'],
  'image token mode requires at least one token price'
);

assert.deepEqual(
  missingModelPricing({
    useCases: ['video-gen'],
    videoRates: [
      { resolution: '720p', pricePerSecond: '0.8' },
      { resolution: '1080p', pricePerSecond: '1.2' },
    ],
  }),
  [],
  'video is satisfied by a priced resolution rate'
);

assert.deepEqual(
  missingModelPricing({ useCases: ['video-gen'], videoRates: [{ resolution: '720p', pricePerSecond: '' }] }).map(item => item.field),
  ['videoPricePerSecond'],
  'an unpriced video rate fails validation'
);

assert.deepEqual(
  missingModelPricing({ useCases: ['reasoning'] }).map(item => item.field).sort(),
  ['inputPrice', 'outputPrice'],
  'reasoning is billed through chat tokens (reasoning tokens are output tokens)'
);

assert.deepEqual(
  missingModelPricing({ useCases: ['reasoning'], inputPrice: '1', outputPrice: '2' }),
  [],
  'reasoning accepts chat input/output prices'
);

console.log('model-pricing-requirements tests passed');
