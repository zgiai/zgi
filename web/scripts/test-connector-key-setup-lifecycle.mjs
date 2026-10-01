import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import { connectorRequestErrorDetails } from '../src/hooks/connectors/connector-request-error.ts';
import {
  applyKeySetupResult,
  beginKeySetup,
  canSubmitKeySetup,
  createKeySetupState,
  failKeySetup,
  resetKeySetup,
  retryKeySetup,
  updateKeySetupValue,
} from '../src/components/connectors/setup/key-setup-state.ts';

const fields = [{ name: 'token', label: 'Personal access token', required: true, secret: true }];
const secretSentinel = 'zgi-secret-must-disappear';

let state = createKeySetupState();
assert.equal(state.status, 'idle');
assert.equal(canSubmitKeySetup(state, fields), false);

state = updateKeySetupValue(state, 'token', secretSentinel);
assert.equal(canSubmitKeySetup(state, fields), true);

state = beginKeySetup(state);
assert.equal(state.status, 'verifying');
assert.deepEqual(state.values, {}, 'submission must immediately clear client Key state');
assert.equal(JSON.stringify(state).includes(secretSentinel), false);
assert.equal(
  canSubmitKeySetup(state, fields),
  false,
  'verifying must prevent duplicate submission'
);

state = applyKeySetupResult(state, {
  attempt_id: '33333333-3333-4333-8333-333333333333',
  state: 'verifying',
});
assert.equal(state.status, 'verifying');
assert.equal(state.attemptId, '33333333-3333-4333-8333-333333333333');

state = applyKeySetupResult(state, {
  attempt_id: '33333333-3333-4333-8333-333333333333',
  state: 'ready',
  binding_warning: 'connector_target_forbidden',
  attachment: {
    id: '66666666-6666-4666-8666-666666666666',
    connection_id: '44444444-4444-4444-8444-444444444444',
    connector_id: 'github',
    target: { type: 'chat', id: '77777777-7777-4777-8777-777777777777' },
    status: 'active',
    compatibility: 'native',
  },
  connection: {
    id: '44444444-4444-4444-8444-444444444444',
    connector_id: 'github',
    name: '@octocat',
    state: 'ready',
    revision: 1,
    credential_configured: true,
    created_at: '2026-09-01T00:00:00Z',
    updated_at: '2026-09-01T00:00:00Z',
  },
});
assert.equal(state.status, 'ready');
assert.equal(state.connection?.name, '@octocat');
assert.equal(state.bindingWarning, 'connector_target_forbidden');
assert.equal(state.attachment?.target.type, 'chat');
assert.deepEqual(state.values, {});

state = resetKeySetup();
state = updateKeySetupValue(state, 'token', secretSentinel);
state = beginKeySetup(state);
state = failKeySetup(state, 'provider_unavailable', true);
assert.equal(state.status, 'failed');
assert.equal(state.errorCode, 'provider_unavailable');
assert.equal(state.retryable, true);
assert.equal(JSON.stringify(state).includes(secretSentinel), false);

state = retryKeySetup(state);
assert.equal(state.status, 'idle');
assert.deepEqual(state.values, {}, 'retry must require the Key to be entered again');

state = updateKeySetupValue(state, 'token', secretSentinel);
state = resetKeySetup();
assert.equal(state.status, 'idle');
assert.deepEqual(state.values, {}, 'close and unmount cleanup must clear client Key state');
assert.equal(JSON.stringify(state).includes(secretSentinel), false);

state = applyKeySetupResult(beginKeySetup(updateKeySetupValue(state, 'token', secretSentinel)), {
  attempt_id: '55555555-5555-4555-8555-555555555555',
  state: 'failed',
  error_code: 'invalid_key',
  retryable: true,
});
assert.equal(state.status, 'failed');
assert.equal(state.errorCode, 'invalid_key');
assert.deepEqual(state.values, {});

for (const requestError of [
  { code: 'ERR_NETWORK', message: 'Network Error' },
  { code: 'ECONNABORTED', message: 'timeout exceeded' },
  { response: { status: 0 } },
]) {
  const details = connectorRequestErrorDetails(requestError);
  assert.equal(details.outcomeUnknown, true);
  assert.equal(details.code, 'transport_unconfirmed');
  state = beginKeySetup(updateKeySetupValue(createKeySetupState(), 'token', secretSentinel));
  state = failKeySetup(state, details.code, details.retryable, details.outcomeUnknown);
  assert.equal(state.status, 'unknown', 'missing responses cannot establish verification failure');
  assert.equal(state.attemptId, undefined, 'a lost POST response cannot invent an attempt ID');
  assert.deepEqual(state.values, {});
  assert.equal(
    canSubmitKeySetup(state, fields),
    false,
    'unknown results must not resubmit the Key'
  );
  assert.equal(JSON.stringify(state).includes(secretSentinel), false);
  state = retryKeySetup(state);
  assert.equal(state.status, 'idle');
  assert.equal(
    canSubmitKeySetup(state, fields),
    false,
    'manual retry must require fresh Key input'
  );
}

const rejectedKey = connectorRequestErrorDetails({
  response: { status: 422, data: { error_code: 'connector_key_invalid' } },
});
assert.equal(rejectedKey.outcomeUnknown, false);
assert.equal(rejectedKey.code, 'connector_key_invalid');
assert.equal(rejectedKey.retryable, false);

const source = relativePath => readFileSync(path.join(process.cwd(), relativePath), 'utf8');
for (const hookPath of [
  'src/hooks/connectors/use-connector-setup.ts',
  'src/hooks/connectors/use-connector-reconnect.ts',
]) {
  const hook = source(hookPath);
  assert.match(hook, /queryFn: \(\) => connectorService\.getKeySetup\(state\.attemptId/);
  assert.match(hook, /query\.state\.status !== 'error'/, 'a failed poll waits for manual recovery');
  assert.match(hook, /retry: false/);
  assert.match(hook, /destroyValues\(requestValues\)/);
  assert.match(hook, /outcomeUnknown/);

  const retryBody = hook.match(
    /const retryStatus = useCallback\(\(\) => \{([\s\S]*?)\n {2}\},/
  )?.[1];
  assert.ok(retryBody, 'status retry must be explicitly separate from Key submission');
  assert.doesNotMatch(
    retryBody,
    /submit|startKeySetup|reconnectConnection|reset|randomUUID|setState/
  );
  // Execute the actual callback body with a read-only query spy, without React or network I/O.
  const retryStatus = new Function(
    'enabled',
    'state',
    'statusRefreshing',
    'refetchStatus',
    retryBody
  );
  const verifying = applyKeySetupResult(beginKeySetup(createKeySetupState()), {
    attempt_id: '77777777-7777-4777-8777-777777777777',
    state: 'verifying',
  });
  const before = globalThis.structuredClone(verifying);
  let statusReads = 0;
  const refetch = () => {
    statusReads += 1;
  };
  retryStatus(true, verifying, false, refetch);
  assert.equal(statusReads, 1);
  assert.deepEqual(
    verifying,
    before,
    'status recovery must preserve the same attempt and empty Key state'
  );
  retryStatus(false, verifying, false, refetch);
  retryStatus(true, verifying, true, refetch);
  retryStatus(true, { status: 'unknown', values: {} }, false, refetch);
  retryStatus(true, { status: 'verifying', values: {} }, false, refetch);
  assert.equal(
    statusReads,
    1,
    'disabled, inflight and missing-attempt retries must not issue any request'
  );
}

const serviceSource = source('src/services/connector.service.ts');
for (const method of ['startKeySetup', 'reconnectConnection']) {
  const methodSource = serviceSource.split(`  ${method}(`)[1]?.split('\n  }')[0];
  assert.ok(methodSource);
  assert.match(
    methodSource,
    /retryAttemptsOverride: 0/,
    'Key commands must never opt into automatic retries'
  );
}
const panelSource = source('src/components/connectors/setup/connector-key-setup-panel.tsx');
const setupEnabledExpression = panelSource.match(/enabled: ([^\n]+),/)?.[1];
assert.ok(setupEnabledExpression);
const canStartNewKeyVerification = new Function('connector', `return (${setupEnabledExpression});`);
assert.equal(
  canStartNewKeyVerification({ key_fields: fields, capabilities: [{ usable: false }] }),
  true,
  'an expired or under-permissioned saved Key cannot disable the new-Key form'
);
assert.equal(
  canStartNewKeyVerification({ key_fields: [], capabilities: [{ usable: true }] }),
  false
);
assert.equal(canStartNewKeyVerification({ key_fields: fields, capabilities: [] }), false);
const reconnectSource = source(
  'src/components/connectors/connections/connection-reconnect-dialog.tsx'
);
const statusSource = source('src/components/connectors/setup/connector-setup-status.tsx');
assert.match(panelSource, /onRetryStatus=\{setup\.retryStatus\}/);
assert.match(reconnectSource, /onRetry=\{reconnect\.retryStatus\}/);
assert.match(statusSource, /statusError && state\.attemptId && onRetryStatus/);
for (const component of [panelSource, reconnectSource]) {
  assert.match(component, /setup\.errors\.resultUnknown/);
  assert.match(component, /refreshConnections/);
  assert.match(component, /href="\/console\/connectors\/connections"/);
}

console.log('Connector Key setup lifecycle and secret-clearing checks passed.');
