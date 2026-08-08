import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { partitionProviders } from '../src/utils/provider-list.ts';

const providers = [
  { id: 'global-enabled', provider_type: 'global', is_enabled: true },
  { id: 'custom-enabled', provider_type: 'custom', is_enabled: true },
  { id: 'custom-disabled', provider_type: 'custom', is_enabled: false },
  { id: 'global-disabled', provider_type: 'global', is_enabled: false },
];

const { officialProviders, customProviders } = partitionProviders(providers);

assert.deepEqual(
  officialProviders.map(provider => provider.id),
  ['global-enabled', 'global-disabled'],
  'global providers must stay in the official provider group'
);
assert.deepEqual(
  customProviders.map(provider => provider.id),
  ['custom-enabled', 'custom-disabled'],
  'custom providers must stay in the custom provider group even when disabled'
);
assert.equal(
  officialProviders.length + customProviders.length,
  providers.length,
  'partitioning must neither duplicate nor drop providers'
);
assert.deepEqual(
  providers.map(provider => provider.id),
  ['global-enabled', 'custom-enabled', 'custom-disabled', 'global-disabled'],
  'partitioning must not mutate the aggregate provider list'
);
assert.throws(
  () => partitionProviders([{ id: 'unknown', provider_type: 'partner', is_enabled: true }]),
  /Unsupported provider type: partner/,
  'unknown provider types must fail instead of disappearing silently'
);

const scriptDirectory = path.dirname(fileURLToPath(import.meta.url));
const providerPageSource = readFileSync(
  path.resolve(scriptDirectory, '../src/app/dashboard/provider/page.tsx'),
  'utf8'
);
const modelDetailsSource = readFileSync(
  path.resolve(scriptDirectory, '../src/components/usage/model-details-section.tsx'),
  'utf8'
);

assert.match(
  providerPageSource,
  /const \{ items: allProviders, isLoading \} = useProviders\(\);/,
  'the provider page must use the aggregate provider list as its single source'
);
assert.match(
  providerPageSource,
  /partitionProviders\(allProviders\)/,
  'the provider page must partition the aggregate list by provider type'
);
assert.doesNotMatch(
  providerPageSource,
  /useCustomProviders/,
  'the provider page must not merge the legacy custom-provider list into the aggregate list'
);
assert.match(
  modelDetailsSource,
  /const \{ items: providers \} = useProviders\(/,
  'model usage details must resolve names from the aggregate provider list'
);
assert.doesNotMatch(
  modelDetailsSource,
  /useCustomProviders/,
  'model usage details must not duplicate providers from the legacy custom-provider list'
);

console.log('Provider list segmentation checks passed.');
