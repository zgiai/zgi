import assert from 'node:assert/strict';

// @ts-ignore Node's direct TypeScript runner requires an explicit extension.
import { isModelPricingConfigured } from '../utils/model-price.ts';

assert.equal(
  isModelPricingConfigured({
    input_price_configured: false,
    output_price_configured: false,
  }),
  false,
  'chat models without column prices are unconfigured'
);

assert.equal(
  isModelPricingConfigured({
    input_price_configured: true,
    output_price_configured: true,
  }),
  true,
  'chat models with both column prices are configured'
);

assert.equal(
  isModelPricingConfigured({
    pricing: {
      token_pricing: { chat: { input_price_per_million: '2', output_price_per_million: '8' } },
    },
  }),
  true,
  'legacy token_pricing counts as configured'
);

assert.equal(
  isModelPricingConfigured({
    pricing: {
      token_tiers: [{ min_input_tokens: 0, input_price_per_million: 2, output_price_per_million: 8 }],
    },
  }),
  true,
  'token_tiers counts as configured'
);

assert.equal(
  isModelPricingConfigured({
    use_cases: ['embedding'],
    input_price_configured: true,
    output_price_configured: false,
  }),
  true,
  'embedding models only need input price'
);

assert.equal(
  isModelPricingConfigured({
    use_cases: ['image-gen'],
    output_price_configured: true,
  }),
  true,
  'image models can use a single configured column'
);

assert.equal(
  isModelPricingConfigured({
    use_cases: ['video-gen'],
    input_price_configured: true,
    output_price_configured: true,
  }),
  false,
  'video models still need structured video pricing'
);

console.log('model pricing configured checks passed');
