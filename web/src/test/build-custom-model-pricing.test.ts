import assert from 'node:assert/strict';

// @ts-ignore Node's direct TypeScript runner requires an explicit extension.
import { buildCustomModelPricing } from '../utils/build-custom-model-pricing.ts';

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
    videoPricePerSecond: '0.8'
  }),
  {
    deployment_scope: 'global',
    catalog_origin: 'pre_custom',
    price_per_image: 0.25,
    video_generation: { currency: 'USD', billing_unit: 'second', rates: [{ resolution: '720p', price_per_second: 0.8 }] }
  },
  'configured image and video prices are submitted together'
);

assert.equal(
  buildCustomModelPricing({ useCases: ['text-chat'], currency: 'USD' }),
  undefined,
  'models without image/video capability keep token pricing only'
);

console.log('build-custom-model-pricing tests passed');
