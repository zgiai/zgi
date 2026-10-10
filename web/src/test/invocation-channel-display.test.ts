import assert from 'node:assert/strict';
import test from 'node:test';

// @ts-ignore Node's direct TypeScript runner requires an explicit extension.
import { resolveChannelDisplay } from '../utils/invocation-channel-display.ts';

test('platform invocations hide upstream channel and supplier', () => {
  const display = resolveChannelDisplay(
    {
      channel_name: 'real-upstream-channel',
      provider_name: 'real-provider',
      actual_channel_name: 'actual-upstream-channel',
      supplier_name: 'real-supplier',
      pricing_details: { billing_lane: 'platform' },
    },
    'ZGI Cloud'
  );
  assert.deepEqual(display, {
    isPlatform: true,
    providerName: 'ZGI Cloud',
    channelName: 'ZGI Cloud',
    actualChannelName: 'ZGI Cloud',
    supplierName: 'ZGI Cloud',
  });
});

test('private invocations preserve real attribution', () => {
  const display = resolveChannelDisplay(
    {
      channel_name: 'custom-channel',
      provider_name: 'custom-provider',
      actual_channel_name: 'actual-channel',
      supplier_name: 'custom-supplier',
      pricing_details: { billing_lane: 'private' },
    },
    'ZGI Cloud'
  );
  assert.deepEqual(display, {
    isPlatform: false,
    providerName: 'custom-provider',
    channelName: 'custom-channel',
    actualChannelName: 'actual-channel',
    supplierName: 'custom-supplier',
  });
});

test('legacy system-provider attribution remains cloud when lane is absent', () => {
  const display = resolveChannelDisplay(
    {
      provider_name: 'legacy-provider',
      channel_name: 'legacy-channel',
      actual_channel_name: 'legacy-actual-channel',
      supplier_name: 'legacy-supplier',
      pricing_details: undefined,
      use_system_provider: true,
    },
    'ZGI Cloud'
  );
  assert.equal(display.channelName, 'ZGI Cloud');
  assert.equal(display.supplierName, 'ZGI Cloud');
});
