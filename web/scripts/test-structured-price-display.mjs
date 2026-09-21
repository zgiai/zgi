import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import Module from 'node:module';
import { resolve } from 'node:path';
import ts from 'typescript';

function loadTypeScriptModule(path, mocks = {}) {
  const filename = resolve(path);
  const source = readFileSync(filename, 'utf8');
  const compiled = ts.transpileModule(source, {
    compilerOptions: {
      esModuleInterop: true,
      module: ts.ModuleKind.CommonJS,
      target: ts.ScriptTarget.ES2022,
    },
    fileName: filename,
  }).outputText;
  const loadedModule = new Module(filename);
  loadedModule.filename = filename;
  loadedModule.paths = Module._nodeModulePaths(resolve(filename, '..'));
  const nativeRequire = loadedModule.require.bind(loadedModule);
  loadedModule.require = id => (Object.hasOwn(mocks, id) ? mocks[id] : nativeRequire(id));
  loadedModule._compile(compiled, filename);
  return loadedModule.exports;
}

const billingDisplay = loadTypeScriptModule('src/utils/billing-display.ts');
const { getModelPriceDisplay } = loadTypeScriptModule('src/utils/model-price.ts', {
  '@/utils/billing-display': billingDisplay,
});

// USD catalog price shown to a CNY organization: convert with the current rate,
// mark the estimate, and never leak the USD symbol.
const cnyImage = getModelPriceDisplay({
  useCases: ['image-gen'],
  currency: 'USD',
  pricing: { price_per_image: 1 },
  billingDisplay: { currency: 'CNY', usdToCnyRate: 7 },
});
assert.deepEqual(
  cnyImage.map(item => [item.label, item.formattedValue]),
  [['image', '≈¥7.00']],
  'structured USD prices must convert to the CNY display currency and never show the USD symbol'
);

const cnyTokenTier = getModelPriceDisplay({
  useCases: ['text-chat', 'vision'],
  currency: 'USD',
  pricing: { token_tiers: [{ min_input_tokens: 0, input_price_per_million: 5 }] },
  billingDisplay: { currency: 'CNY', usdToCnyRate: 6 },
});
assert.deepEqual(
  cnyTokenTier.map(item => [item.label, item.formattedValue]),
  [['input', '≈¥30.00']],
  'structured token tiers must apply the current rate with the estimate marker'
);

const cnyMetered = getModelPriceDisplay({
  useCases: ['text-to-speech'],
  currency: 'USD',
  pricing: {
    metered: [
      {
        operation: 'speech_generation',
        meter: 'input_text',
        base_unit: 'billed_character',
        price: { amount: 0.5, currency: 'USD', per_quantity: 10000 },
      },
    ],
  },
  billingDisplay: { currency: 'CNY', usdToCnyRate: 7 },
});
assert.deepEqual(
  cnyMetered.map(item => [item.label, item.formattedValue]),
  [['output', '≈¥3.50']],
  'structured metered prices must convert to the CNY display currency'
);

// A same-currency USD price must stay a plain dollar value, not an estimate.
const usdImage = getModelPriceDisplay({
  useCases: ['image-gen'],
  currency: 'USD',
  pricing: { price_per_image: 0.14 },
  billingDisplay: { currency: 'USD', usdToCnyRate: 7 },
});
assert.deepEqual(
  usdImage.map(item => [item.label, item.formattedValue]),
  [['image', '$0.14']],
  'same-currency structured prices must keep the plain currency symbol'
);

// A CNY-sourced price shown to a USD organization converts once and is marked.
const usdFromCny = getModelPriceDisplay({
  useCases: ['image-gen'],
  currency: 'CNY',
  pricing: { currency: 'CNY', price_per_image: 70 },
  billingDisplay: { currency: 'USD', usdToCnyRate: 7 },
});
assert.deepEqual(
  usdFromCny.map(item => [item.label, item.formattedValue]),
  [['image', '≈$10.00']],
  'structured CNY prices must convert to USD once and keep the estimate marker'
);

console.log('Structured price display checks passed.');
