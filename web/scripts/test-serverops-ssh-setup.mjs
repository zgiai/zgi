import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';
import path from 'node:path';
import { harness, jsx, load, nodes, ui } from './connector-fixture.mjs';
import {
  CONNECTOR_CENTER_ROUTE_ROOT,
  SERVEROPS_SSH_CONNECTOR_ID,
  SERVEROPS_SSH_HOST_FIELD,
  SERVEROPS_SSH_HOST_KEY_FIELD,
  SERVEROPS_SSH_PASSWORD_FIELD,
  SERVEROPS_SSH_PASSPHRASE_FIELD,
  SERVEROPS_SSH_PORT_FIELD,
  SERVEROPS_SSH_PRIVATE_KEY_FIELD,
  SERVEROPS_SSH_SETUP_METHOD,
  SERVEROPS_SSH_USERNAME_FIELD,
  buildServerOpsSSHCredentialPayload,
  buildServerOpsSSHProbePayload,
  clearServerOpsSSHSecretValues,
  createServerOpsSSHFormValues,
  destroyServerOpsSSHCredentialPayload,
  destroyServerOpsSSHSecretValues,
  isConnectorCenterRoute,
  isServerOpsSSHConnector,
  isServerOpsSSHHostKeyFingerprintValid,
  isServerOpsSSHPortValid,
  serverOpsSSHFieldError,
  serverOpsSSHObservationSignature,
  serverOpsSSHObservationTargetSignature,
  serverOpsSSHProbeFieldError,
  supportsServerOpsSSHSetup,
} from '../src/components/connectors/setup/serverops-ssh-setup-state.ts';

const readSource = relativePath => readFileSync(path.join(process.cwd(), relativePath), 'utf8');

// ---------------------------------------------------------------------------
// Routing: only the exact catalog identity. Standalone setup is limited to
// Connector Center, while embedded Agent/Digital Human hosts can opt in.
// ---------------------------------------------------------------------------
const ssh = { id: SERVEROPS_SSH_CONNECTOR_ID, setup_method: SERVEROPS_SSH_SETUP_METHOD };
assert.equal(isServerOpsSSHConnector(ssh), true);
assert.equal(isServerOpsSSHConnector({ ...ssh, id: 'serverops_ssh_other' }), false);
assert.equal(isServerOpsSSHConnector({ ...ssh, setup_method: 'key' }), false);
assert.equal(isServerOpsSSHConnector({ ...ssh, setup_method: undefined }), false);
assert.equal(isServerOpsSSHConnector({ id: SERVEROPS_SSH_CONNECTOR_ID }), false);

for (const pathname of [
  CONNECTOR_CENTER_ROUTE_ROOT,
  `${CONNECTOR_CENTER_ROUTE_ROOT}/`,
  `${CONNECTOR_CENTER_ROUTE_ROOT}/connections`,
  `${CONNECTOR_CENTER_ROUTE_ROOT}/connections/11111111-1111-4111-8111-111111111111`,
  `${CONNECTOR_CENTER_ROUTE_ROOT}?q=ssh`,
]) {
  assert.equal(isConnectorCenterRoute(pathname), true, `${pathname} is a Connector Center route`);
  assert.equal(supportsServerOpsSSHSetup(ssh, pathname), true);
}
for (const pathname of [
  '/console/digital-humans/create',
  '/console/digital-humans/abc/configuration',
  '/console/agents/abc',
  '/console/workflows/abc',
  '/console/connectors-other',
  '/console/chat',
  '',
  null,
  undefined,
]) {
  assert.equal(isConnectorCenterRoute(pathname), false, `${pathname} is not Connector Center`);
  assert.equal(supportsServerOpsSSHSetup(ssh, pathname), false, 'route-only setup stays disabled');
  assert.equal(
    supportsServerOpsSSHSetup(ssh, pathname, true),
    true,
    'embedded Agent/DigitalHuman setup may render the dedicated SSH panel'
  );
}
assert.equal(
  supportsServerOpsSSHSetup({ id: 'github', setup_method: 'key' }, CONNECTOR_CENTER_ROUTE_ROOT),
  false
);

// ---------------------------------------------------------------------------
// Catalog affordance: the frozen SSH identity must admit its Connector Center
// panel, while a mismatched or disabled descriptor stays unavailable.
// ---------------------------------------------------------------------------
const appSetup = load('src/components/connectors/setup/app-setup-state.ts');
const catalogModel = load('src/components/connectors/catalog/catalog-model.ts', {
  '@/components/connectors/connector-metadata': load(
    'src/components/connectors/connector-metadata.ts'
  ),
  '../setup/app-setup-state': appSetup,
});

// Mirrors the frozen catalog contract in catalog/serverops_ssh.go.
const sshCatalogItem = {
  id: SERVEROPS_SSH_CONNECTOR_ID,
  name: 'SSH Server Operations',
  description: 'Read-only inspection of one Linux server over SSH.',
  classification: 'connector',
  key_fields: [],
  extension_schema_version: '1.3.0',
  setup_method: SERVEROPS_SSH_SETUP_METHOD,
  setup: { handler: 'serverops-ssh-credentials-v1', methods: ['manual'] },
  credential_fields: [
    { name: SERVEROPS_SSH_HOST_FIELD, required: true, secret: false },
    { name: SERVEROPS_SSH_PORT_FIELD, required: true, secret: false },
    { name: SERVEROPS_SSH_USERNAME_FIELD, required: true, secret: false },
    { name: SERVEROPS_SSH_PASSWORD_FIELD, required: false, secret: true },
    { name: SERVEROPS_SSH_PRIVATE_KEY_FIELD, required: false, secret: true },
    { name: SERVEROPS_SSH_PASSPHRASE_FIELD, required: false, secret: true },
    { name: SERVEROPS_SSH_HOST_KEY_FIELD, required: true, secret: false },
  ],
  capabilities: [
    {
      id: 'serverops.system_snapshot',
      usable: false,
      unavailable_reason: 'connector_not_verified',
    },
  ],
  connection_summary: { count: 0 },
};
assert.equal(
  appSetup.supportsServerOpsSSHConnectorSetup(sshCatalogItem),
  true,
  'the frozen SSH identity must admit setup'
);
assert.equal(
  appSetup.supportsConnectorSetup(sshCatalogItem),
  true,
  'the Connector Center catalog row must be able to launch the SSH panel'
);
assert.equal(catalogModel.getConnectorCatalogRowState(sshCatalogItem), 'not_connected');

for (const [label, candidate] of [
  ['wrong id', { ...sshCatalogItem, id: 'serverops_ssh_other' }],
  ['missing setup_method', { ...sshCatalogItem, setup_method: undefined }],
  ['wrong setup_method', { ...sshCatalogItem, setup_method: 'key' }],
  ['missing setup descriptor', { ...sshCatalogItem, setup: undefined }],
  [
    'descriptor without a manual method',
    {
      ...sshCatalogItem,
      setup: { handler: 'serverops-ssh-credentials-v1', methods: ['qr_create'] },
    },
  ],
  [
    'availability disabled',
    { ...sshCatalogItem, setup_availability: { enabled: false, methods: ['manual'] } },
  ],
  [
    'availability without a manual method',
    { ...sshCatalogItem, setup_availability: { enabled: true, methods: ['qr_create'] } },
  ],
  [
    'plugin unavailable',
    {
      ...sshCatalogItem,
      capabilities: [{ id: 'op', usable: false, unavailable_reason: 'plugin_unavailable' }],
    },
  ],
]) {
  assert.equal(
    appSetup.supportsConnectorSetup(candidate),
    false,
    `${label} must not admit SSH setup`
  );
}

// Existing contracts stay untouched: a legacy Key connector is still admitted,
// and the closed SSH id cannot be smuggled through the generic Key fallback.
const legacyKeyConnector = {
  id: 'github',
  key_fields: [{ name: 'token', label: 'Token', required: true, secret: true }],
  capabilities: [{ id: 'github.repo.read', usable: true }],
  connection_summary: { count: 0 },
};
assert.equal(appSetup.supportsConnectorSetup(legacyKeyConnector), true);
assert.equal(
  appSetup.supportsConnectorSetup({ ...legacyKeyConnector, id: SERVEROPS_SSH_CONNECTOR_ID }),
  false,
  'the SSH id alone must never open the generic Key path'
);

// The external-route link targets a real Connector Center detail route.
const connectorDetailPage = 'src/app/console/connectors/[connectorId]/page.tsx';
assert.ok(existsSync(path.join(process.cwd(), connectorDetailPage)), connectorDetailPage);
const sshConnectorCenterRoute = `${CONNECTOR_CENTER_ROUTE_ROOT}/${SERVEROPS_SSH_CONNECTOR_ID}`;
assert.equal(sshConnectorCenterRoute, '/console/connectors/serverops_ssh');
assert.equal(/[?#]/.test(sshConnectorCenterRoute), false, 'no secret or query can ride the route');

// ---------------------------------------------------------------------------
// Host key fingerprint: canonical OpenSSH SHA256 only. Never a skip/TOFU form.
// ---------------------------------------------------------------------------
const validFingerprint = 'SHA256:47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU';
assert.equal(isServerOpsSSHHostKeyFingerprintValid(validFingerprint), true);

const rejectedFingerprints = [
  '',
  'SHA256:',
  'SHA256:AAAA',
  'SHA256:47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU=',
  'SHA256:47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFV', // non-canonical pad bits
  'sha256:47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU', // wrong prefix case
  'SHA256:47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuF U', // embedded space
  ` ${validFingerprint}`,
  `${validFingerprint}\n`,
  'SHA256:47DEQpj8HBSa_TImW+5JCeuQeRkm5NMpJWZG3hSuFU', // URL-safe alphabet
  'MD5:47:DE:Qp:j8:HB:sa',
  'skip',
  'accept-new',
];
for (const value of rejectedFingerprints) {
  assert.equal(
    isServerOpsSSHHostKeyFingerprintValid(value),
    false,
    `fingerprint must be rejected: ${JSON.stringify(value)}`
  );
}

// The SHA-256 digest of empty input renders to exactly the canonical fixture.
assert.equal(
  isServerOpsSSHHostKeyFingerprintValid(
    `SHA256:${Buffer.from([]).toString('base64').replace(/=+$/, '')}`
  ),
  false,
  'a non-SHA256-length digest is rejected'
);
const digest32 = Buffer.alloc(32, 7);
assert.equal(
  isServerOpsSSHHostKeyFingerprintValid(`SHA256:${digest32.toString('base64').replace(/=+$/, '')}`),
  true
);

// ---------------------------------------------------------------------------
// Port: canonical in-range decimal only.
// ---------------------------------------------------------------------------
for (const value of ['1', '22', '80', '1024', '65535']) {
  assert.equal(isServerOpsSSHPortValid(value), true, `port ${value} is valid`);
}
for (const value of ['', '0', '65536', '022', '+22', '-22', '22 ', ' 22', '2.2', 'abc', '123456']) {
  assert.equal(isServerOpsSSHPortValid(value), false, `port ${JSON.stringify(value)} is invalid`);
}
assert.equal(serverOpsSSHFieldError(createServerOpsSSHFormValues()), 'host');
assert.equal(
  serverOpsSSHFieldError({
    ...createServerOpsSSHFormValues(),
    [SERVEROPS_SSH_HOST_FIELD]: 'srv.internal',
    [SERVEROPS_SSH_PORT_FIELD]: '022',
  }),
  'port'
);
assert.equal(
  serverOpsSSHFieldError({
    ...createServerOpsSSHFormValues(),
    [SERVEROPS_SSH_HOST_FIELD]: 'srv.internal',
    [SERVEROPS_SSH_PORT_FIELD]: '22',
  }),
  'username'
);

// ---------------------------------------------------------------------------
// Passphrase omission and temporary payload destruction.
// ---------------------------------------------------------------------------
const completeForm = {
  [SERVEROPS_SSH_HOST_FIELD]: 'srv.internal',
  [SERVEROPS_SSH_PORT_FIELD]: '22',
  [SERVEROPS_SSH_USERNAME_FIELD]: 'deploy',
  [SERVEROPS_SSH_PRIVATE_KEY_FIELD]:
    '-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n-----END OPENSSH PRIVATE KEY-----\n',
  [SERVEROPS_SSH_PASSPHRASE_FIELD]: '',
  [SERVEROPS_SSH_HOST_KEY_FIELD]: validFingerprint,
};
assert.equal(serverOpsSSHFieldError(completeForm), undefined);

const passwordForm = {
  ...completeForm,
  [SERVEROPS_SSH_PASSWORD_FIELD]: 'server-password',
  [SERVEROPS_SSH_PRIVATE_KEY_FIELD]: '',
  [SERVEROPS_SSH_PASSPHRASE_FIELD]: '',
};
assert.equal(serverOpsSSHFieldError(passwordForm, 'password'), undefined);
const passwordPayload = buildServerOpsSSHCredentialPayload(passwordForm, 'password');
assert.equal(passwordPayload[SERVEROPS_SSH_PASSWORD_FIELD], 'server-password');
assert.equal(Object.hasOwn(passwordPayload, SERVEROPS_SSH_PRIVATE_KEY_FIELD), false);
assert.equal(Object.hasOwn(passwordPayload, SERVEROPS_SSH_PASSPHRASE_FIELD), false);

const withoutPassphrase = buildServerOpsSSHCredentialPayload(completeForm);
assert.deepEqual(
  Object.keys(withoutPassphrase).sort(),
  [
    SERVEROPS_SSH_HOST_FIELD,
    SERVEROPS_SSH_HOST_KEY_FIELD,
    SERVEROPS_SSH_PORT_FIELD,
    SERVEROPS_SSH_PRIVATE_KEY_FIELD,
    SERVEROPS_SSH_USERNAME_FIELD,
  ].sort()
);
assert.equal(
  Object.prototype.hasOwnProperty.call(withoutPassphrase, SERVEROPS_SSH_PASSPHRASE_FIELD),
  false,
  'a blank optional passphrase must be omitted entirely'
);

const explicitBlank = buildServerOpsSSHCredentialPayload({
  ...completeForm,
  [SERVEROPS_SSH_PASSPHRASE_FIELD]: '   ',
});
assert.equal(
  Object.prototype.hasOwnProperty.call(explicitBlank, SERVEROPS_SSH_PASSPHRASE_FIELD),
  false,
  'a whitespace-only passphrase must be omitted rather than sent blank'
);

const withPassphrase = buildServerOpsSSHCredentialPayload({
  ...completeForm,
  [SERVEROPS_SSH_PASSPHRASE_FIELD]: 'correct horse battery staple',
});
assert.equal(withPassphrase[SERVEROPS_SSH_PASSPHRASE_FIELD], 'correct horse battery staple');

const secretSentinel = 'serverops-ssh-secret-sentinel';
const payload = buildServerOpsSSHCredentialPayload({
  ...completeForm,
  [SERVEROPS_SSH_PRIVATE_KEY_FIELD]: secretSentinel,
  [SERVEROPS_SSH_PASSPHRASE_FIELD]: secretSentinel,
});
assert.equal(JSON.stringify(payload).includes(secretSentinel), true);
destroyServerOpsSSHCredentialPayload(payload);
assert.deepEqual(payload, {}, 'the temporary request map must be emptied after the request');
assert.equal(JSON.stringify(payload).includes(secretSentinel), false);

const clearedForm = clearServerOpsSSHSecretValues({
  ...completeForm,
  [SERVEROPS_SSH_PASSWORD_FIELD]: secretSentinel,
  [SERVEROPS_SSH_PRIVATE_KEY_FIELD]: secretSentinel,
  [SERVEROPS_SSH_PASSPHRASE_FIELD]: secretSentinel,
});
assert.equal(clearedForm[SERVEROPS_SSH_PRIVATE_KEY_FIELD], '');
assert.equal(clearedForm[SERVEROPS_SSH_PASSPHRASE_FIELD], '');
assert.equal(clearedForm[SERVEROPS_SSH_PASSWORD_FIELD], '');
assert.equal(clearedForm[SERVEROPS_SSH_HOST_FIELD], 'srv.internal');
assert.equal(JSON.stringify(clearedForm).includes(secretSentinel), false);

const unmounted = {
  ...completeForm,
  [SERVEROPS_SSH_PASSWORD_FIELD]: secretSentinel,
  [SERVEROPS_SSH_PRIVATE_KEY_FIELD]: secretSentinel,
  [SERVEROPS_SSH_PASSPHRASE_FIELD]: secretSentinel,
};
destroyServerOpsSSHSecretValues(unmounted);
assert.equal(unmounted[SERVEROPS_SSH_PASSWORD_FIELD], '');
assert.equal(unmounted[SERVEROPS_SSH_PRIVATE_KEY_FIELD], '');
assert.equal(unmounted[SERVEROPS_SSH_PASSPHRASE_FIELD], '');
assert.equal(JSON.stringify(unmounted).includes(secretSentinel), false);

// ---------------------------------------------------------------------------
// Two-stage probe: the fingerprint is optional for the read-only test while the
// final strict save still requires a canonical one.
// ---------------------------------------------------------------------------
const probeForm = {
  ...completeForm,
  [SERVEROPS_SSH_HOST_KEY_FIELD]: '',
};
assert.equal(
  serverOpsSSHFieldError(probeForm),
  'host_key_sha256',
  'the strict final save must still require a fingerprint'
);
assert.equal(
  serverOpsSSHProbeFieldError(probeForm),
  undefined,
  'the read-only probe must accept a blank fingerprint'
);

const probePayload = buildServerOpsSSHProbePayload(probeForm);
assert.deepEqual(
  Object.keys(probePayload).sort(),
  [
    SERVEROPS_SSH_HOST_FIELD,
    SERVEROPS_SSH_PORT_FIELD,
    SERVEROPS_SSH_PRIVATE_KEY_FIELD,
    SERVEROPS_SSH_USERNAME_FIELD,
  ].sort()
);
assert.equal(
  Object.prototype.hasOwnProperty.call(probePayload, SERVEROPS_SSH_HOST_KEY_FIELD),
  false,
  'a blank fingerprint must be omitted so the backend captures the host key'
);
assert.equal(
  Object.prototype.hasOwnProperty.call(probePayload, SERVEROPS_SSH_PASSPHRASE_FIELD),
  false,
  'a blank passphrase must stay omitted for the probe'
);

const probeWithPin = buildServerOpsSSHProbePayload({
  ...probeForm,
  [SERVEROPS_SSH_HOST_KEY_FIELD]: validFingerprint,
  [SERVEROPS_SSH_PASSPHRASE_FIELD]: 'correct horse battery staple',
});
assert.equal(probeWithPin[SERVEROPS_SSH_HOST_KEY_FIELD], validFingerprint);
assert.equal(probeWithPin[SERVEROPS_SSH_PASSPHRASE_FIELD], 'correct horse battery staple');

assert.equal(
  serverOpsSSHProbeFieldError({ ...probeForm, [SERVEROPS_SSH_HOST_KEY_FIELD]: validFingerprint }),
  undefined,
  'the probe must accept a canonical supplied fingerprint'
);
assert.equal(
  serverOpsSSHProbeFieldError({ ...probeForm, [SERVEROPS_SSH_HOST_KEY_FIELD]: 'SHA256:AAAA' }),
  'host_key_sha256',
  'a supplied probe fingerprint is still strictly validated'
);
assert.equal(serverOpsSSHProbeFieldError({ ...probeForm, [SERVEROPS_SSH_HOST_FIELD]: '' }), 'host');
assert.equal(
  serverOpsSSHProbeFieldError({ ...probeForm, [SERVEROPS_SSH_PORT_FIELD]: '0' }),
  'port'
);
assert.equal(
  serverOpsSSHProbeFieldError({ ...probeForm, [SERVEROPS_SSH_USERNAME_FIELD]: '' }),
  'username'
);
assert.equal(
  serverOpsSSHProbeFieldError({ ...probeForm, [SERVEROPS_SSH_PRIVATE_KEY_FIELD]: '' }),
  'private_key'
);
assert.equal(
  serverOpsSSHProbeFieldError({ ...probeForm, [SERVEROPS_SSH_PASSPHRASE_FIELD]: 'x\u0000y' }),
  'passphrase'
);

const probeSecretSentinel = 'serverops-ssh-probe-secret-sentinel';
const probeSecretPayload = buildServerOpsSSHProbePayload({
  ...probeForm,
  [SERVEROPS_SSH_PRIVATE_KEY_FIELD]: probeSecretSentinel,
  [SERVEROPS_SSH_PASSPHRASE_FIELD]: probeSecretSentinel,
});
assert.equal(JSON.stringify(probeSecretPayload).includes(probeSecretSentinel), true);
destroyServerOpsSSHCredentialPayload(probeSecretPayload);
assert.deepEqual(probeSecretPayload, {}, 'the probe request map must be emptied after the request');
assert.equal(JSON.stringify(probeSecretPayload).includes(probeSecretSentinel), false);

// The stale-confirmation signature is non-secret and tracks host/user/port plus
// the manual pin only, so it can be kept after the probe.
const signatureForm = {
  ...probeForm,
  [SERVEROPS_SSH_PRIVATE_KEY_FIELD]: probeSecretSentinel,
  [SERVEROPS_SSH_PASSPHRASE_FIELD]: probeSecretSentinel,
};
const signatureBase = serverOpsSSHObservationSignature(signatureForm);
assert.equal(
  signatureBase.includes(probeSecretSentinel),
  false,
  'the observation signature must never include a secret'
);
assert.equal(serverOpsSSHObservationSignature({ ...signatureForm }), signatureBase);
assert.notEqual(
  serverOpsSSHObservationSignature({ ...signatureForm, [SERVEROPS_SSH_PORT_FIELD]: '2222' }),
  signatureBase
);
assert.notEqual(
  serverOpsSSHObservationSignature({
    ...signatureForm,
    [SERVEROPS_SSH_HOST_KEY_FIELD]: validFingerprint,
  }),
  signatureBase
);
assert.equal(
  serverOpsSSHObservationTargetSignature({
    ...signatureForm,
    [SERVEROPS_SSH_HOST_KEY_FIELD]: validFingerprint,
  }),
  serverOpsSSHObservationTargetSignature(signatureForm),
  'a fingerprint-only edit must not change the target signature'
);

// ---------------------------------------------------------------------------
// Component wiring: standalone and embedded surfaces, write-only secrets, no
// URL/storage.
// ---------------------------------------------------------------------------
const panelSource = readSource('src/components/connectors/setup/serverops-ssh-setup-panel.tsx');
const routerSource = readSource('src/components/connectors/setup/connector-key-setup-panel.tsx');
const agentConnectorSource = readSource('src/components/agents/agent-runtime/connector-dialog.tsx');
const reconnectSource = readSource(
  'src/components/connectors/connections/connection-reconnect-dialog.tsx'
);
const serviceSource = readSource('src/services/connector.service.ts');

assert.doesNotMatch(routerSource, /usePathname\(\)/);
assert.match(
  routerSource,
  /supportsServerOpsSSHConnectorSetup\(props\.connector\)/
);
assert.match(
  agentConnectorSource,
  /<ConnectorKeySetupPanel[\s\S]*embedded[\s\S]*target=\{\{ type: 'agent'/
);
assert.doesNotMatch(reconnectSource, /usePathname\(\)/);
assert.match(reconnectSource, /supportsServerOpsSSHConnectorSetup\(props\.connector\)/);
assert.match(panelSource, /connectorService\.startCredentialSetup\(/);
assert.match(panelSource, /connectorService\.reconnectConnection\(/);
assert.match(panelSource, /updateConnection\.id/);
assert.match(panelSource, /connectorService\.getKeySetup\(attemptId\)/);
assert.match(panelSource, /invalidatePersonalConnectionQueries/);
assert.match(panelSource, /connectorRequestErrorDetails/);
assert.match(panelSource, /destroyServerOpsSSHCredentialPayload\(payload\)/);
assert.match(panelSource, /destroyServerOpsSSHSecretValues\(latestValues\.current\)/);
assert.doesNotMatch(panelSource, /localStorage|sessionStorage|document\.cookie/);
assert.doesNotMatch(panelSource, /console\.(log|debug|info)/);
assert.doesNotMatch(panelSource, /skip/i, 'host verification must never be skippable');
assert.doesNotMatch(
  panelSource,
  /\btarget\s*:/,
  'the SSH setup request must never forward a target binding'
);

// The two-stage flow probes first and only persists after an explicit confirm.
assert.match(panelSource, /connectorService\.testServerOpsSSHHostKey\(/);
assert.match(panelSource, /buildServerOpsSSHProbePayload\(values, authMethod\)/);
assert.match(panelSource, /serverOpsSSHProbeFieldError\(values, authMethod\)/);
assert.match(panelSource, /serverOpsSSHFieldError\(\s*\{/);
assert.match(panelSource, /destroyServerOpsSSHCredentialPayload\(probePayload\)/);
assert.match(panelSource, /t\('serveropsSsh\.probe'\)/);
assert.match(panelSource, /t\('serveropsSsh\.probing/);
assert.match(panelSource, /t\('serveropsSsh\.confirmationTitle'\)/);
assert.match(panelSource, /t\('serveropsSsh\.confirmationWarning/);
assert.match(panelSource, /t\('serveropsSsh\.confirm'\)/);
assert.match(panelSource, /t\('serveropsSsh\.reTest'\)/);
assert.match(panelSource, /pendingObservation\.hostname/);
assert.match(panelSource, /pendingObservation\.host_key_sha256/);

// The probe must retain the live form secrets so a confirmation can follow; the
// temporary probe payload alone is destroyed.
const probeSection = panelSource.slice(
  panelSource.indexOf('const startProbe'),
  panelSource.indexOf('const confirmObservation')
);
assert.ok(probeSection.includes('const startProbe'), 'the probe handler must exist');
assert.match(probeSection, /testServerOpsSSHHostKey/);
assert.match(probeSection, /destroyServerOpsSSHCredentialPayload\(probePayload\)/);
assert.doesNotMatch(
  probeSection,
  /clearServerOpsSSHSecretValues/,
  'the probe must not clear the form secrets that the confirmation needs'
);
assert.doesNotMatch(
  probeSection,
  /destroyServerOpsSSHSecretValues/,
  'the probe must not destroy the live form secrets'
);

// Confirmation creates a fresh Idempotency-Key, uses the strict final payload,
// and destroys it while clearing the form secrets only when the save begins.
const confirmSection = panelSource.slice(
  panelSource.indexOf('const confirmObservation'),
  panelSource.indexOf('const retry =')
);
assert.ok(confirmSection.includes('const confirmObservation'), 'the confirm handler must exist');
assert.match(confirmSection, /idempotencyKey\.current = globalThis\.crypto\.randomUUID\(\)/);
assert.match(confirmSection, /buildServerOpsSSHCredentialPayload\(/);
assert.match(confirmSection, /clearServerOpsSSHSecretValues/);
assert.match(confirmSection, /connectorService\.startCredentialSetup/);
assert.match(confirmSection, /connectorService\.reconnectConnection/);
assert.match(confirmSection, /destroyServerOpsSSHCredentialPayload\(payload\)/);

const probeMethodSource = serviceSource.split('  testServerOpsSSHHostKey(')[1]?.split('\n  }')[0];
assert.ok(probeMethodSource, 'testServerOpsSSHHostKey must exist on connectorService');
assert.match(probeMethodSource, /host-key-tests/);
assert.match(probeMethodSource, /retryAttemptsOverride: 0/);
assert.match(probeMethodSource, /skipErrorHandling: true/);
assert.doesNotMatch(
  probeMethodSource,
  /Idempotency-Key/,
  'the read-only host-key probe must not send an Idempotency-Key'
);

// Both SSH submissions carry a write-only private key, so both must opt into
// the sensitive-request path; a bare `true` fourth argument is the flag.
for (const [method, call] of [
  [
    'startCredentialSetup',
    /connectorService\.startCredentialSetup\(\s*connector\.id,\s*\{ values: payload \},\s*idempotencyKey\.current,\s*true\s*\)/,
  ],
  [
    'reconnectConnection',
    /connectorService\.reconnectConnection\(\s*updateConnection\.id,\s*\{ values: payload \},\s*idempotencyKey\.current,\s*true\s*\)/,
  ],
]) {
  assert.match(
    panelSource,
    call,
    `the SSH panel must opt ${method} into sensitive-request handling`
  );
}

for (const method of ['startCredentialSetup', 'reconnectConnection']) {
  const methodSource = serviceSource.split(`  ${method}(`)[1]?.split('\n  }')[0];
  assert.ok(methodSource, `${method} must exist on connectorService`);
  assert.match(methodSource, /Idempotency-Key/);
  assert.match(methodSource, /retryAttemptsOverride: 0/);
  assert.match(
    methodSource,
    /sensitiveRequest = false/,
    `${method} must default its sensitive-request flag to false`
  );
  assert.match(
    methodSource,
    /sensitiveRequest\s*\?\s*\{\s*skipErrorHandling:\s*true\s*\}\s*:\s*\{\s*\}/,
    `${method} must add skipErrorHandling only when the sensitive flag is set`
  );
}

// ---------------------------------------------------------------------------
// Service contract: the sensitive opt-in actually reaches the request config,
// and omitting it preserves the default error-reporting path exactly.
// ---------------------------------------------------------------------------
const serviceCalls = [];
class RecordingBaseService {
  async request(...args) {
    serviceCalls.push(args);
    return { code: '0', data: { state: 'ready' } };
  }
}
const { connectorService } = load('src/services/connector.service.ts', {
  '@/lib/http/services': { BaseService: RecordingBaseService },
});
const secretValues = {
  values: {
    [SERVEROPS_SSH_PRIVATE_KEY_FIELD]: secretSentinel,
    [SERVEROPS_SSH_PASSPHRASE_FIELD]: secretSentinel,
  },
};

await connectorService.startCredentialSetup(
  SERVEROPS_SSH_CONNECTOR_ID,
  secretValues,
  'idem-opt-in-1',
  true
);
assert.equal(
  serviceCalls.at(-1)[3].skipErrorHandling,
  true,
  'startCredentialSetup(true) must add skipErrorHandling to the request config'
);
await connectorService.reconnectConnection('connection-1', secretValues, 'idem-opt-in-2', true);
assert.equal(
  serviceCalls.at(-1)[3].skipErrorHandling,
  true,
  'reconnectConnection(true) must add skipErrorHandling to the request config'
);

await connectorService.startCredentialSetup(
  SERVEROPS_SSH_CONNECTOR_ID,
  secretValues,
  'idem-default-1'
);
assert.equal(
  Object.prototype.hasOwnProperty.call(serviceCalls.at(-1)[3], 'skipErrorHandling'),
  false,
  'startCredentialSetup must keep the default error-reporting path when the flag is omitted'
);
await connectorService.reconnectConnection('connection-1', secretValues, 'idem-default-2');
assert.equal(
  Object.prototype.hasOwnProperty.call(serviceCalls.at(-1)[3], 'skipErrorHandling'),
  false,
  'reconnectConnection must keep the default error-reporting path when the flag is omitted'
);

// The read-only probe posts to its own route with no Idempotency-Key and always
// opts out of shared error reporting.
await connectorService.testServerOpsSSHHostKey(SERVEROPS_SSH_CONNECTOR_ID, {
  values: {
    [SERVEROPS_SSH_HOST_FIELD]: 'srv.internal',
    [SERVEROPS_SSH_PORT_FIELD]: '22',
    [SERVEROPS_SSH_USERNAME_FIELD]: 'deploy',
    [SERVEROPS_SSH_PRIVATE_KEY_FIELD]: secretSentinel,
  },
});
const probeServiceCall = serviceCalls.at(-1);
assert.equal(probeServiceCall[0], 'post');
assert.equal(probeServiceCall[1], `/connectors/${SERVEROPS_SSH_CONNECTOR_ID}/host-key-tests`);
assert.equal(probeServiceCall[3].retryAttemptsOverride, 0);
assert.equal(probeServiceCall[3].skipErrorHandling, true);
assert.equal(
  Object.prototype.hasOwnProperty.call(probeServiceCall[3], 'headers'),
  false,
  'the probe request config must not carry an Idempotency-Key header'
);

// The sensitive opt-in is positional, so every non-SSH caller must still pass
// exactly three arguments; a stray fourth `true` would silently opt them in.
function topLevelArguments(source, fromIndex) {
  const open = source.indexOf('(', fromIndex);
  assert.ok(open >= 0, 'expected a call expression');
  const args = [];
  let current = '';
  let depth = 0;
  for (let i = open + 1; i < source.length; i++) {
    const char = source[i];
    if ('([{'.includes(char)) depth++;
    else if (')]}'.includes(char)) {
      if (char === ')' && depth === 0) {
        args.push(current);
        return args;
      }
      depth--;
    }
    if (char === ',' && depth === 0) {
      args.push(current);
      current = '';
      continue;
    }
    current += char;
  }
  throw new Error('unterminated call expression');
}
for (const [relativePath, callee] of [
  [
    'src/components/connectors/setup/connector-messaging-setup-panel.tsx',
    'connectorService.startCredentialSetup',
  ],
  [
    'src/components/connectors/setup/connector-messaging-setup-panel.tsx',
    'connectorService.reconnectConnection',
  ],
  [
    'src/components/connectors/setup/connector-app-setup-panel.tsx',
    'connectorService.reconnectConnection',
  ],
  ['src/hooks/connectors/use-connector-reconnect.ts', 'connectorService.reconnectConnection'],
  ['src/hooks/connectors/use-connector-mail-setup.ts', 'connectorService.reconnectConnection'],
]) {
  const source = readSource(relativePath);
  const start = source.indexOf(callee);
  assert.ok(start >= 0, `${relativePath} must still call ${callee}`);
  assert.equal(
    topLevelArguments(source, start + callee.length - 1).length,
    3,
    `${relativePath} must keep the default three-argument ${callee} call`
  );
}

// ---------------------------------------------------------------------------
// Embedded routing: Agent/Digital Human hosts render the same dedicated SSH
// form in place. A generic non-embedded picker still links to Connector Center
// instead of falling through to fields that do not understand SSH auth.
// ---------------------------------------------------------------------------
const sshState = load('src/components/connectors/setup/serverops-ssh-setup-state.ts');
let currentPathname = '/console/digital-humans/create';
const panelMocks = {
  'react/jsx-runtime': jsx,
  'next/link': { default: 'Link' },
  'next/navigation': { usePathname: () => currentPathname },
  'lucide-react': ui,
  '@/components/ui/alert': ui,
  '@/components/ui/button': ui,
  '@/components/ui/card': ui,
  '@/hooks/connectors/use-connector-setup': {
    useConnectorSetup: () => ({
      state: { status: 'idle' },
      canSubmit: false,
      updateValue() {},
      submit() {},
      retry() {},
      refreshConnections() {},
      retryStatus() {},
      statusError: undefined,
      statusRefreshing: false,
    }),
  },
  '@/i18n': { useT: () => key => key },
  './connector-key-form': { ConnectorKeyForm: 'ConnectorKeyForm' },
  './connector-setup-status': { ConnectorSetupStatus: 'ConnectorSetupStatus' },
  './connector-app-setup-panel': { ConnectorAppSetupPanel: 'ConnectorAppSetupPanel' },
  './use-creation-default': {
    useCreationDefault: () => ({ option: () => null, begin() {}, complete: async () => true }),
  },
  './app-setup-state': appSetup,
  './messaging-setup-state': { supportsMessagingContract: () => false },
  './connector-messaging-setup-panel': {
    ConnectorMessagingSetupPanel: 'ConnectorMessagingSetupPanel',
  },
  './mail-setup-state': { supportsMailSetup: () => false },
  './connector-mail-setup-panel': { ConnectorMailSetupPanel: 'ConnectorMailSetupPanel' },
  './serverops-ssh-setup-state': sshState,
  './serverops-ssh-setup-panel': { ServerOpsSSHSetupPanel: 'ServerOpsSSHSetupPanel' },
};
let panelModule;
const panelHarness = harness(react => {
  panelModule = load('src/components/connectors/setup/connector-key-setup-panel.tsx', {
    ...panelMocks,
    react,
  });
  return () =>
    panelModule.ConnectorKeySetupPanel({
      connector: sshCatalogItem,
      embedded: true,
      target: { type: 'digital_human', id: 'digital-human-1' },
    });
});

currentPathname = '/console/digital-humans/create';
let panelNodes = nodes(panelHarness.render());
assert.equal(
  panelNodes.some(node => node.type === 'ServerOpsSSHSetupPanel'),
  true,
  'embedded Agent/DigitalHuman setup must render the dedicated SSH panel'
);
assert.equal(
  panelNodes.some(node => node.type === 'ServerOpsSSHExternalSetupNotice'),
  false,
  'embedded setup must not render the external-route notice'
);
const externalTree = panelModule.ConnectorKeySetupPanel({ connector: sshCatalogItem });
const externalNodes = nodes(externalTree);
assert.equal(
  externalNodes.some(node => node.type === 'ServerOpsSSHSetupPanel'),
  true,
  'the connector-owned setup dispatch must render the dedicated SSH panel on every host'
);

for (const pathname of ['/console/connectors', '/console/connectors/serverops_ssh']) {
  currentPathname = pathname;
  panelNodes = nodes(panelHarness.render());
  assert.ok(
    panelNodes.some(node => node.type === 'ServerOpsSSHSetupPanel'),
    `${pathname} must render the dedicated SSH panel`
  );
  assert.equal(
    panelNodes.some(node => typeof node.type === 'function'),
    false,
    `${pathname} must not render the external-route notice`
  );
}

assert.doesNotMatch(routerSource, /ServerOpsSSHExternalSetupNotice/);
assert.doesNotMatch(routerSource, /SERVEROPS_SSH_CONNECTOR_ID/);

// ---------------------------------------------------------------------------
// Localized copy exists and explains optional host-key confirmation.
// ---------------------------------------------------------------------------
const en = (await import('../src/i18n/modules/connectors/en-US.ts')).default;
const zh = (await import('../src/i18n/modules/connectors/zh-Hans.ts')).default;
const keyShape = value => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return typeof value;
  return Object.fromEntries(
    Object.keys(value)
      .sort()
      .map(key => [key, keyShape(value[key])])
  );
};
assert.deepEqual(
  keyShape(zh.serveropsSsh),
  keyShape(en.serveropsSsh),
  'English and Simplified Chinese serveropsSsh keys must match'
);
for (const [locale, messages] of [
  ['en-US', en.serveropsSsh],
  ['zh-Hans', zh.serveropsSsh],
]) {
  for (const [key, value] of Object.entries(messages)) {
    if (typeof value === 'string') {
      assert.ok(value.trim(), `${locale} serveropsSsh.${key} must not be empty`);
    } else {
      for (const [nested, nestedValue] of Object.entries(value)) {
        assert.ok(nestedValue.trim(), `${locale} serveropsSsh.${key}.${nested} must not be empty`);
      }
    }
  }
  assert.match(messages.fingerprintHelp, /SHA256/);
  assert.match(messages.fingerprintHelp, /trusted|可信|verified|核实/i);
  assert.match(messages.fingerprintHelp, /never skipped|不会跳过/);
  assert.match(messages.fingerprintHelp, /optional|可选/i);
  assert.match(messages.confirmationWarning, /confirm|确认/i);
  assert.ok(messages.probe.trim(), `${locale} must name the test/probe action`);
  assert.ok(messages.probingTitle.trim(), `${locale} must describe the probing stage`);
  assert.ok(messages.confirmationTitle.trim(), `${locale} must name the confirmation`);
  assert.ok(messages.observedHostname.trim(), `${locale} must label the remote hostname`);
  assert.ok(messages.observedFingerprint.trim(), `${locale} must label the observed fingerprint`);
  assert.ok(messages.confirmationWarning.trim(), `${locale} must warn the hash is untrusted`);
  assert.ok(messages.confirm.trim(), `${locale} must offer an explicit confirm action`);
  assert.ok(messages.reTest.trim(), `${locale} must offer a re-test action`);
  assert.ok(messages.errors.staleObservation.trim(), `${locale} must report a stale observation`);
  assert.ok(messages.errors.fingerprintChanged.trim(), `${locale} must report a changed pin`);
}

console.log('ServerOps SSH setup routing, validation, and secret-handling checks passed.');
