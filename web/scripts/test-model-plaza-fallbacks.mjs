import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import Module from 'node:module';
import { resolve } from 'node:path';
import ts from 'typescript';

import { normalizeModelList } from '../src/utils/model-normalize.ts';

const [model] = normalizeModelList([
  null,
  'invalid model',
  {
    provider: 'doubao',
    model: 'broken-video',
    use_cases: 'video-gen',
    input_modalities: { value: 'video' },
    output_modalities: null,
    endpoints: [],
    features: 'invalid',
    tools: null,
    parameters: [],
  },
]);

assert.ok(model, 'object-shaped model entries should remain renderable');
assert.deepEqual(model.use_cases, [], 'invalid use_cases must fall back to an empty array');
assert.deepEqual(model.input_modalities, [], 'invalid input modalities must fall back to an array');
assert.deepEqual(
  model.output_modalities,
  [],
  'missing output modalities must fall back to an array'
);
assert.deepEqual(model.endpoints, {}, 'invalid endpoint metadata must fall back to an object');
assert.deepEqual(model.features, {}, 'invalid feature metadata must fall back to an object');
assert.deepEqual(model.tools, {}, 'missing tool metadata must fall back to an object');
assert.equal(model.parameters?.max_stop_sequences, 0);

assert.deepEqual(normalizeModelList(null), [], 'a missing model list must not throw');
assert.deepEqual(normalizeModelList({ items: [] }), [], 'an invalid model list must not throw');

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
const { getModelExperienceHref } = loadTypeScriptModule(
  'src/features/model-plaza/experience-route.ts'
);
const videoWorkbenchSource = readFileSync(
  resolve('src/components/chat/variants/img/video-workbench.tsx'),
  'utf8'
);

const experienceModel = (useCases, overrides = {}) => ({
  provider: 'zgi-cloud',
  model: 'catalog/model name',
  use_cases: useCases,
  ...overrides,
});

assert.equal(
  getModelExperienceHref(experienceModel(['video-gen'])),
  '/console/work/video?provider=zgi-cloud&model=catalog%2Fmodel+name'
);
assert.equal(
  getModelExperienceHref(experienceModel(['music-gen'])),
  '/console/work/music?provider=zgi-cloud&model=catalog%2Fmodel+name'
);
assert.equal(
  getModelExperienceHref(experienceModel(['image-gen'])),
  '/console/work/image?provider=zgi-cloud&model=catalog%2Fmodel+name'
);
assert.equal(
  getModelExperienceHref(experienceModel(['speech-to-text'])),
  '/console/work/chat?provider=zgi-cloud&model=catalog%2Fmodel+name'
);
assert.equal(
  getModelExperienceHref(experienceModel(['text-to-speech'])),
  '/console/work/chat?provider=zgi-cloud&model=catalog%2Fmodel+name'
);
assert.match(
  videoWorkbenchSource,
  /React\.useEffect\(\(\) => \{\s*if \(isLoading\) return;[\s\S]*stillAvailable/,
  'video workbench must preserve the requested model while available models load'
);

assert.doesNotThrow(() =>
  getModelPriceDisplay({
    useCases: ['video-gen'],
    currency: { invalid: true },
    pricing: {
      video_generation: {
        resolution_rates: [null, 'invalid', { rates: [null, 'invalid', {}] }],
      },
    },
    videoDisplayMode: 'summary',
  })
);

assert.doesNotThrow(() =>
  getModelPriceDisplay({
    useCases: ['text-to-speech'],
    currency: 123,
    pricing: {
      token_tiers: { invalid: true },
      metered: 'invalid',
    },
  })
);

const tokenPriceItems = getModelPriceDisplay({
  inputPrice: 5,
  outputPrice: 25,
  cacheReadPrice: 0.5,
  inputPriceConfigured: true,
  outputPriceConfigured: true,
  cacheReadPriceConfigured: true,
  useCases: ['text-chat', 'vision'],
  currency: 'USD',
});
assert.deepEqual(
  tokenPriceItems.map(item => item.label),
  ['input', 'output', 'cacheRead'],
  'cache read pricing must not be labelled as video pricing'
);

const cacheWriteOnlyPriceItems = getModelPriceDisplay({
  inputPrice: 5,
  outputPrice: 25,
  cacheWritePrice: 6.25,
  inputPriceConfigured: true,
  outputPriceConfigured: true,
  cacheWritePriceConfigured: true,
  useCases: ['text-chat'],
  currency: 'USD',
});
assert.deepEqual(
  cacheWriteOnlyPriceItems.map(item => item.label),
  ['input', 'output', 'cacheWrite'],
  'cache write pricing must display independently when cache read is absent'
);

const structuredTokenPriceItems = getModelPriceDisplay({
  useCases: ['text-chat', 'vision'],
  currency: 'USD',
  pricing: {
    token_tiers: [
      {
        min_input_tokens: 0,
        input_price_per_million: 5,
        output_price_per_million: 25,
        cached_input_price_per_million: 0.5,
        cache_write_price_per_million: 6.25,
        cache_write_5m_price_per_million: 7.5,
        cache_creation_1h_price_per_million: 10,
      },
    ],
  },
});
assert.deepEqual(
  structuredTokenPriceItems.map(item => item.label),
  ['input', 'output', 'cacheRead', 'cacheWrite', 'cacheWrite5m', 'cacheWrite1h'],
  'structured token cache prices must use cache labels'
);

console.log('Model plaza fallback checks passed.');
