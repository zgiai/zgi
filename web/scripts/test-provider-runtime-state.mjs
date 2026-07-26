import assert from 'node:assert/strict';

import {
  getProviderSidebarRuntimeState,
  shouldPromptProviderChannelSetup,
} from '../src/utils/provider-runtime-state.ts';

const enabledCatalogProvider = {
  is_enabled: true,
  model_count: 3,
  channel_count: 0,
};

assert.equal(
  getProviderSidebarRuntimeState(enabledCatalogProvider, 2),
  'available_models',
  'available models must override a stale zero channel count for aliased providers'
);
assert.equal(
  shouldPromptProviderChannelSetup(enabledCatalogProvider, true),
  false,
  'the onboarding banner must stay hidden when aliased providers already expose available models'
);

assert.equal(
  getProviderSidebarRuntimeState(enabledCatalogProvider, 0),
  'pending_channels',
  'a provider with catalog models and no channel or available models still needs channel setup'
);
assert.equal(
  shouldPromptProviderChannelSetup(enabledCatalogProvider, false),
  true,
  'the onboarding banner must remain visible when no channel or available model exists'
);

const configuredProvider = {
  ...enabledCatalogProvider,
  channel_count: 1,
};

assert.equal(
  getProviderSidebarRuntimeState(configuredProvider, undefined),
  'unknown',
  'configured providers must remain unknown while availability is loading'
);
assert.equal(
  getProviderSidebarRuntimeState(configuredProvider, 0),
  'configured_no_models',
  'configured providers without available models need a distinct diagnostic state'
);
assert.equal(
  shouldPromptProviderChannelSetup(configuredProvider, false),
  false,
  'an existing channel must suppress first-time channel onboarding'
);

console.log('Provider runtime state checks passed.');
