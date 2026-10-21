import assert from 'node:assert/strict';

// @ts-ignore Node's direct TypeScript runner requires an explicit extension.
import { getModelPriceDisplay, isModelPricingConfigured } from '../utils/model-price.ts';

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
    endpoints: { embeddings: true },
    input_price: 0.02,
    input_price_configured: false,
    output_price_configured: false,
  }),
  true,
  'embedding endpoint models use a positive input price without requiring output price'
);

assert.equal(
  isModelPricingConfigured({
    use_cases: ['embedding'],
    pricing: {
      rules: [{ operation: 'embedding', meter: 'input_token', price: '0.02' }],
    },
    input_price_configured: false,
    output_price_configured: false,
  }),
  true,
  'embedding input pricing rules count as configured'
);

const embeddingPriceDisplay = getModelPriceDisplay({
  pricing: {
    rules: [{ operation: 'embedding', meter: 'input_token', price: '0.02' }],
  },
  useCases: ['embedding'],
  currency: 'USD',
});
assert.deepEqual(
  embeddingPriceDisplay.map(item => item.label),
  ['input'],
  'embedding pricing rules display input price only'
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
