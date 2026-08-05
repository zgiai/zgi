import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';

import { INTEGRATION_KEYS } from '../src/hooks/query-keys.ts';

import {
  connectionNeedsSetup,
  CONNECTION_SETUP_TEST_REUSE_WINDOW_MS,
  CURRENT_CONNECTION_SETUP_VERSION,
  resolveAutomaticConnectionVerification,
  resolveConnectionSetupInitialization,
} from '../src/components/integrations/connection-setup-lifecycle.ts';

assert.equal(CURRENT_CONNECTION_SETUP_VERSION, 2);
assert.equal(
  connectionNeedsSetup({ setup_version: 1, setup_completed_at: '2026-08-04T00:00:00Z' }),
  true,
  'connections completed before action-policy review must resume setup'
);
assert.equal(
  connectionNeedsSetup({ setup_version: 2, setup_completed_at: '2026-08-05T00:00:00Z' }),
  false
);
assert.equal(connectionNeedsSetup({ setup_version: 2, setup_completed_at: null }), true);

assert.deepEqual(INTEGRATION_KEYS.myConnectionLists(), ['integrations', 'my-connections']);
assert.deepEqual(INTEGRATION_KEYS.myConnections({ all: true }).slice(0, 2), [
  'integrations',
  'my-connections',
]);
assert.deepEqual(INTEGRATION_KEYS.availableConnectionLists(), [
  'integrations',
  'available-connections',
]);
assert.deepEqual(INTEGRATION_KEYS.availableConnections({ all: true }).slice(0, 2), [
  'integrations',
  'available-connections',
]);

const firstOpen = resolveConnectionSetupInitialization(null, 'connection-a', true);
assert.deepEqual(firstOpen, {
  initializedConnectionID: 'connection-a',
  shouldReset: true,
});

const sameConnectionRefresh = resolveConnectionSetupInitialization(
  firstOpen.initializedConnectionID,
  'connection-a',
  true
);
assert.equal(
  sameConnectionRefresh.shouldReset,
  false,
  'refreshing the saved connection object must not reset the completed wizard'
);

const switchedConnection = resolveConnectionSetupInitialization(
  sameConnectionRefresh.initializedConnectionID,
  'connection-b',
  true
);
assert.equal(
  switchedConnection.shouldReset,
  true,
  'switching connections must initialize the wizard'
);

const closed = resolveConnectionSetupInitialization(
  switchedConnection.initializedConnectionID,
  'connection-b',
  false
);
assert.deepEqual(closed, { initializedConnectionID: null, shouldReset: false });

const reopened = resolveConnectionSetupInitialization(
  closed.initializedConnectionID,
  'connection-b',
  true
);
assert.equal(reopened.shouldReset, true, 'reopening the dialog must start a fresh setup session');

const now = Date.parse('2026-08-05T08:00:00.000Z');
assert.equal(
  resolveAutomaticConnectionVerification({ open: false, initialStep: 0, verified: false, now }),
  'skip',
  'closed setup dialogs must not contact the provider'
);
assert.equal(
  resolveAutomaticConnectionVerification({ open: true, initialStep: 3, verified: false, now }),
  'skip',
  'opening the usage-target editor must not retest the provider'
);
assert.equal(
  resolveAutomaticConnectionVerification({ open: true, initialStep: 0, verified: false, now }),
  'run',
  'starting or continuing setup must verify the connection automatically'
);
assert.equal(
  resolveAutomaticConnectionVerification({
    open: true,
    initialStep: 0,
    verified: true,
    now,
    lastTestedAt: new Date(now - CONNECTION_SETUP_TEST_REUSE_WINDOW_MS + 1).toISOString(),
  }),
  'reuse',
  'a test completed immediately before setup opens must not be duplicated'
);
assert.equal(
  resolveAutomaticConnectionVerification({
    open: true,
    initialStep: 0,
    verified: true,
    now,
    lastTestedAt: new Date(now - CONNECTION_SETUP_TEST_REUSE_WINDOW_MS - 1).toISOString(),
  }),
  'run',
  'an older health result must be refreshed when setup opens'
);
assert.equal(
  resolveAutomaticConnectionVerification({
    open: true,
    initialStep: 0,
    verified: false,
    now,
    lastTestedAt: new Date(now - 1).toISOString(),
  }),
  'run',
  'a recent failed test must never be reused as a successful verification'
);

const root = process.cwd();
const setupDialogSource = readFileSync(
  path.join(root, 'src/components/integrations/connection-setup-dialog.tsx'),
  'utf8'
);
const actionSetupSource = readFileSync(
  path.join(root, 'src/components/integrations/connection-action-setup.tsx'),
  'utf8'
);
const connectionsPanelSource = readFileSync(
  path.join(root, 'src/components/integrations/connections-panel.tsx'),
  'utf8'
);
const providerCatalogSource = readFileSync(
  path.join(root, 'src/components/integrations/provider-catalog.tsx'),
  'utf8'
);

function connectionDialogCreateHandler(source) {
  const start = source.indexOf('onCreate={async data => {');
  const end = source.indexOf('onUpdate=', start);
  assert.notEqual(start, -1, 'connection dialog create handler must exist');
  assert.notEqual(end, -1, 'connection dialog update handler must follow create');
  return source.slice(start, end);
}

assert.match(setupDialogSource, /IntegrationConnectionActionSetup/);
assert.match(
  setupDialogSource,
  /resolveAutomaticConnectionVerification/,
  'the setup dialog must automatically verify a connection before configuration continues'
);
assert.doesNotMatch(
  connectionDialogCreateHandler(connectionsPanelSource),
  /test(?:My)?Mutation\.mutateAsync/,
  'the connected-app page must let the setup dialog run the single automatic verification'
);
assert.doesNotMatch(
  connectionDialogCreateHandler(providerCatalogSource),
  /test(?:My)?Mutation\.mutateAsync/,
  'the provider catalog must not test once before opening the auto-verifying setup dialog'
);
assert.match(
  setupDialogSource,
  /item\.id !== connection\?\.id/,
  'opening setup for another connection must never test the previously opened connection'
);
assert.match(
  setupDialogSource,
  /await Promise\.all\(\[/,
  'setup completion must wait until active connection status queries have refreshed'
);
assert.match(
  setupDialogSource,
  /INTEGRATION_KEYS\.availableConnectionLists\(\)/,
  'setup completion must refresh the availability list used by the connected-app UI'
);
assert.doesNotMatch(
  setupDialogSource,
  /IntegrationProviderCapabilitiesInline/,
  'the setup wizard must not reuse the aggregate capability diagnostics editor'
);
assert.match(
  actionSetupSource,
  /connection\.permission_summary\?\.adapted_capabilities/,
  'setup readiness must use the exact connection being configured'
);
assert.doesNotMatch(
  actionSetupSource,
  /availability\s*===\s*['"]needs_connection['"]/,
  'aggregate needs_connection state must not block an already-created connection setup'
);
assert.match(
  setupDialogSource,
  /const usageRulesStep = personal \? -1 : 3/,
  'shared usage rules must remain a separate wizard step'
);

console.log('Integration connection setup lifecycle checks passed.');
