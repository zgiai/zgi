import assert from 'node:assert/strict';

// @ts-ignore Node's direct TypeScript runner requires an explicit extension.
import { aggregatePlatformModels, modelDisplayVendor } from '../components/common/model-selector/model-selector/utils.ts';
// @ts-ignore Node's direct TypeScript runner requires an explicit extension.
import type { ModelItem } from '../services/types/model.ts';

function model(overrides: Partial<ModelItem>): ModelItem {
  return {
    id: overrides.id ?? 'id',
    provider: overrides.provider ?? 'provider',
    model: overrides.model ?? 'model',
    model_name: overrides.model_name ?? overrides.model ?? 'model',
    family: '',
    status: 'active',
    tagline: '',
    is_flagship: false,
    is_recommended: false,
    is_featured: false,
    is_new: false,
    access_type: 'closed',
    currency: 'USD',
    input_price: 0,
    output_price: 0,
    context_window: 0,
    max_output_tokens: 0,
    endpoints: {},
    features: {},
    tools: {},
    use_cases: [],
    input_modalities: [],
    output_modalities: [],
    is_enabled: true,
    is_available: true,
    is_configured: true,
    callable: true,
    tier: '',
    created_at: 0,
    updated_at: 0,
    ...overrides,
  } as ModelItem;
}

const aggregated = aggregatePlatformModels([
  model({
    id: 'reseller-gpt',
    provider: 'reseller-a',
    vendor: 'openai',
    model: 'gpt-5.6',
    features: { reasoning: true },
  }),
  model({
    id: 'openai-gpt',
    provider: 'openai',
    vendor: 'openai',
    model: 'gpt-5.6',
    features: { function_calling: true },
  }),
  model({ id: 'claude', provider: 'reseller-b', vendor: 'anthropic', model: 'claude-4.6' }),
  model({ id: 'custom-1', provider: 'workspace-channel', model: 'custom-model', is_custom: true }),
]);

assert.equal(aggregated.length, 3, 'same vendor + model should be displayed once');
const gpt = aggregated.find(item => item.model === 'gpt-5.6');
assert.ok(gpt);
assert.equal(modelDisplayVendor(gpt), 'openai');
assert.equal(gpt.provider, 'openai', 'deterministic primary record should prefer the vendor provider');
assert.equal(gpt.features.reasoning, true);
assert.equal(gpt.features.function_calling, true);

const claude = aggregated.find(item => item.model === 'claude-4.6');
assert.ok(claude);
assert.equal(modelDisplayVendor(claude), 'anthropic');

const custom = aggregated.find(item => item.is_custom);
assert.ok(custom);
assert.equal(modelDisplayVendor(custom), 'workspace-channel');
assert.equal(custom.provider, 'workspace-channel');

const supplierFallback = aggregatePlatformModels([
  model({
    id: 'siliconflow-deepseek',
    provider: 'siliconflow',
    vendor: 'deepseek',
    vendor_name: '深度求索',
    model: 'deepseek-v4-pro',
  }),
  model({ id: 'deepseek-direct', provider: 'deepseek', vendor: 'deepseek', model: 'deepseek-v4-pro' }),
]);
assert.equal(supplierFallback.length, 1, 'a logical platform model should not duplicate by supplier');
const supplierFallbackModel = supplierFallback[0];
assert.ok(supplierFallbackModel);
assert.equal(supplierFallbackModel.provider, 'deepseek', 'model maker should win over supplier fallback');
assert.equal(modelDisplayVendor(supplierFallbackModel), 'deepseek');
assert.equal(supplierFallbackModel.vendor_name, '深度求索');

const missingCatalogVendor = aggregatePlatformModels([
  model({
    id: 'siliconflow-only-deepseek',
    provider: 'siliconflow',
    vendor_name: 'SiliconFlow',
    model: 'deepseek-v4-pro',
  }),
]);
assert.equal(missingCatalogVendor.length, 0, 'platform models without catalog vendor mapping must not be grouped under suppliers');

const conflictingDirectoryMappings = aggregatePlatformModels([
  model({ id: 'vendor-a-model', provider: 'reseller-a', vendor: 'vendor-a', model: 'shared-name' }),
  model({ id: 'vendor-b-model', provider: 'reseller-b', vendor: 'vendor-b', model: 'shared-name' }),
]);
assert.equal(conflictingDirectoryMappings.length, 2, 'different model vendors must remain separate even when model ids match');

console.log('model selector platform aggregation tests passed');
