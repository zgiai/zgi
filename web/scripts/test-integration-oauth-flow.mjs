import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';

const require = createRequire(import.meta.url);
const ts = require('typescript');
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const statePath = path.join(root, 'src', 'components', 'integrations', 'oauth-flow-state.ts');
const hookPath = path.join(root, 'src', 'hooks', 'integrations', 'use-oauth-flow.ts');
const resultPath = path.join(root, 'src', 'components', 'integrations', 'oauth-result-client.tsx');
const clientFieldsPath = path.join(
  root,
  'src',
  'components',
  'integrations',
  'oauth-client-fields.ts'
);
const oauthConfigContinuationPath = path.join(
  root,
  'src',
  'components',
  'integrations',
  'oauth-config-continuation.ts'
);
const providerCatalogPath = path.join(
  root,
  'src',
  'components',
  'integrations',
  'provider-catalog.tsx'
);
const scopeUpgradePath = path.join(
  root,
  'src',
  'components',
  'integrations',
  'oauth-scope-upgrade.ts'
);
const actionAuthCompatibilityPath = path.join(
  root,
  'src',
  'components',
  'integrations',
  'action-auth-compatibility.ts'
);
const integrationServicePath = path.join(root, 'src', 'services', 'integration.service.ts');
const connectionsPanelPath = path.join(
  root,
  'src',
  'components',
  'integrations',
  'connections-panel.tsx'
);

const stateSource = fs.readFileSync(statePath, 'utf8');
const compiled = ts.transpileModule(stateSource, {
  compilerOptions: {
    module: ts.ModuleKind.CommonJS,
    target: ts.ScriptTarget.ES2022,
  },
}).outputText;
const module = { exports: {} };
vm.runInNewContext(compiled, {
  module,
  exports: module.exports,
  require,
  Set,
  Number,
  Date,
  Math,
});

const {
  initialIntegrationOAuthUIState,
  integrationOAuthUIReducer,
  isIntegrationOAuthFlowTerminal,
  normalizeOAuthPollInterval,
  oauthFlowExpiryDelay,
} = module.exports;

const pendingFlow = {
  flow_ref: 'opaque-flow-reference',
  status: 'pending',
  expires_at: '2026-07-23T12:05:00.000Z',
};
let state = integrationOAuthUIReducer(initialIntegrationOAuthUIState, { type: 'start' });
assert.equal(state.status, 'starting');
state = integrationOAuthUIReducer(state, {
  type: 'flow_created',
  flow: pendingFlow,
  popupBlocked: true,
});
assert.equal(state.status, 'popup_blocked');
assert.equal(state.popupBlocked, true);
state = integrationOAuthUIReducer(state, { type: 'popup_closed' });
assert.equal(state.status, 'popup_closed');
assert.equal(state.popupClosed, true);
state = integrationOAuthUIReducer(state, { type: 'popup_reopened' });
assert.equal(state.status, 'waiting');
assert.equal(state.popupBlocked, false);
assert.equal(state.popupClosed, false);
state = integrationOAuthUIReducer(state, { type: 'popup_closed' });
state = integrationOAuthUIReducer(state, {
  type: 'flow_updated',
  flow: { ...pendingFlow, status: 'exchanging' },
});
assert.equal(state.status, 'exchanging');
state = integrationOAuthUIReducer(state, {
  type: 'flow_updated',
  flow: { ...pendingFlow, status: 'succeeded' },
});
assert.equal(state.status, 'succeeded');
assert.equal(state.popupBlocked, false);
assert.equal(state.popupClosed, false);
assert.equal(isIntegrationOAuthFlowTerminal('succeeded'), true);
assert.equal(isIntegrationOAuthFlowTerminal('authorizing'), false);
assert.equal(normalizeOAuthPollInterval(5), 750);
assert.equal(normalizeOAuthPollInterval(100_000), 5_000);
assert.equal(
  oauthFlowExpiryDelay('2026-07-23T12:05:00.000Z', Date.parse('2026-07-23T12:00:00.000Z')),
  300_000
);

const hookSource = fs.readFileSync(hookPath, 'utf8');
const resultSource = fs.readFileSync(resultPath, 'utf8');
const integrationServiceSource = fs.readFileSync(integrationServicePath, 'utf8');
const connectionsPanelSource = fs.readFileSync(connectionsPanelPath, 'utf8');
assert.match(hookSource, /operationRef\.current !== operation/);
assert.match(hookSource, /cancelOAuthFlowSilently\(createdFlowId\)/);
assert.match(hookSource, /monitorPopup\(\);\s*dispatch\(\{ type: 'popup_reopened' \}\)/);
assert.match(
  hookSource,
  /type:\s*'zgi:integration-oauth-status';\s*\n\s*status:/,
  'popup status messages should contain only their type and status'
);
const popupMessageContractStart = hookSource.search(
  /export\s+(?:type|interface)\s+IntegrationOAuthPopupStatusMessage/
);
assert.ok(popupMessageContractStart >= 0, 'popup status message contract must exist');
assert.doesNotMatch(
  hookSource.slice(popupMessageContractStart),
  /code|token|secret|connection_id|flow_id|flow_ref/,
  'popup status message contract must not expose provider secrets or internal references'
);
assert.match(
  resultSource,
  /window\.opener\.postMessage\(message,\s*window\.location\.origin\)/,
  'popup result must use an exact same-origin target'
);
assert.doesNotMatch(
  resultSource,
  />\s*\{flowId\}\s*<|value=\{flowId\}|children=\{flowId\}|connection_id|access_token|refresh_token|authorization_code/,
  'OAuth result UI must not render flow or credential references'
);
assert.match(resultSource, /SAFE_ERROR_CODE/);
assert.match(resultSource, /metadata\.error\(errorCode\)/);
assert.match(resultSource, /pollingRef\.current/);
assert.match(resultSource, /role="status"/);
assert.match(
  integrationServiceSource,
  /startOAuthFlow[\s\S]*?['"]\/integrations\/oauth\/flows['"][\s\S]*?withCredentials:\s*true/,
  'OAuth flow start must accept the HttpOnly browser-binding cookie across Web/API origins'
);
const scopeUpgradeHandler = connectionsPanelSource.slice(
  connectionsPanelSource.indexOf('const upgradeOAuthAction'),
  connectionsPanelSource.indexOf('const openDelete')
);
assert.ok(scopeUpgradeHandler.length > 0, 'connection scope-upgrade handler must exist');
assert.doesNotMatch(
  scopeUpgradeHandler,
  /setDetailConnection\(null\)/,
  'starting or failing a scope upgrade must not close the connection details'
);
const detailDialogWiring = connectionsPanelSource.slice(
  connectionsPanelSource.indexOf('{detailConnection && canManageConnection'),
  connectionsPanelSource.indexOf(
    '<ConfirmDialog',
    connectionsPanelSource.indexOf('{detailConnection')
  )
);
assert.ok(detailDialogWiring.length > 0, 'connection detail dialog wiring must exist');
assert.doesNotMatch(
  detailDialogWiring,
  /onTest=\{connection => \{\s*setDetailConnection\(null\)/,
  'testing a connection must keep its detail dialog mounted for refreshed results'
);

const clientFieldsSource = fs.readFileSync(clientFieldsPath, 'utf8');
const clientFieldsCompiled = ts.transpileModule(clientFieldsSource, {
  compilerOptions: {
    module: ts.ModuleKind.CommonJS,
    target: ts.ScriptTarget.ES2022,
  },
}).outputText;
const clientFieldsModule = { exports: {} };
vm.runInNewContext(clientFieldsCompiled, {
  module: clientFieldsModule,
  exports: clientFieldsModule.exports,
  require,
  Set,
});
const { resolveOAuthClientFields } = clientFieldsModule.exports;
const normalizedFields = resolveOAuthClientFields({
  id: 'google_oauth',
  type: 'oauth2',
  label: 'Google OAuth',
  available: true,
  oauth: {
    connect_enabled: true,
    client_configured: false,
    client_fields: [
      { key: 'client_id', input: 'text', required: true },
      { key: 'client_secret', input: 'password', required: true, secret: true },
    ],
  },
});
assert.deepEqual(
  normalizedFields.map(field => [field.name, field.type, field.storage]),
  [
    ['client_id', 'text', 'credentials'],
    ['client_secret', 'password', 'credentials'],
  ],
  'provider client_fields must normalize their key/input contract before rendering'
);

const oauthConfigContinuationSource = fs.readFileSync(oauthConfigContinuationPath, 'utf8');
const oauthConfigContinuationCompiled = ts.transpileModule(oauthConfigContinuationSource, {
  compilerOptions: {
    module: ts.ModuleKind.CommonJS,
    target: ts.ScriptTarget.ES2022,
  },
}).outputText;
const oauthConfigContinuationModule = { exports: {} };
vm.runInNewContext(oauthConfigContinuationCompiled, {
  module: oauthConfigContinuationModule,
  exports: oauthConfigContinuationModule.exports,
  require: specifier => {
    if (specifier === './integration-utils') {
      return {
        integrationCatalogID: item => item.integration_id ?? item.id,
        resolveIntegrationAuthDefinitions: item => item.auth ?? [],
      };
    }
    if (specifier === './auth-method-selection') {
      return {
        isOAuthAuthMethod: auth =>
          auth.acquisition_strategy === 'browser_redirect' ||
          auth.type === 'oauth' ||
          auth.type === 'oauth2',
      };
    }
    return require(specifier);
  },
});
const { resolveConfiguredOAuthContinuation } = oauthConfigContinuationModule.exports;
for (const integrationId of ['feishu', 'gmail', 'x']) {
  const authMethodId = `${integrationId}_account_oauth`;
  const provider = {
    id: integrationId,
    integration_id: integrationId,
    enabled: true,
    auth: [
      {
        id: authMethodId,
        type: 'oauth2',
        available: true,
        acquisition_strategy: 'browser_redirect',
        oauth: {
          connect_enabled: true,
          client_configured: true,
        },
      },
    ],
  };
  const resolved = resolveConfiguredOAuthContinuation([provider], integrationId, authMethodId);
  assert.equal(
    resolved?.provider,
    provider,
    `${integrationId} must continue with the freshly fetched provider`
  );
  assert.equal(
    resolved?.auth,
    provider.auth[0],
    `${integrationId} must continue with the freshly fetched OAuth method`
  );
  assert.equal(
    resolveConfiguredOAuthContinuation(
      [
        {
          ...provider,
          auth: [
            {
              ...provider.auth[0],
              oauth: { ...provider.auth[0].oauth, client_configured: false },
            },
          ],
        },
      ],
      integrationId,
      authMethodId
    ),
    null,
    `${integrationId} must not continue from stale unconfigured catalog data`
  );
}

const providerCatalogSource = fs.readFileSync(providerCatalogPath, 'utf8');
const configuredContinuationStart = providerCatalogSource.indexOf('onConfigured={');
const configuredContinuationEnd = providerCatalogSource.indexOf(
  'onOpenChange={nextOpen',
  configuredContinuationStart
);
const configuredContinuationHandler = providerCatalogSource.slice(
  configuredContinuationStart,
  configuredContinuationEnd
);
assert.ok(configuredContinuationStart >= 0, 'catalog OAuth save continuation must exist');
assert.match(
  configuredContinuationHandler,
  /await catalogQuery\.refetch\(\)/,
  'OAuth save continuation must wait for a fresh catalog response'
);
assert.match(
  configuredContinuationHandler,
  /resolveConfiguredOAuthContinuation/,
  'OAuth save continuation must resolve the provider and auth method from the fresh catalog'
);
assert.doesNotMatch(
  configuredContinuationHandler,
  /client_configured:\s*true/,
  'OAuth save continuation must not patch the stale auth snapshot locally'
);

const scopeUpgradeSource = fs.readFileSync(scopeUpgradePath, 'utf8');
const actionAuthCompatibilitySource = fs.readFileSync(actionAuthCompatibilityPath, 'utf8');
const actionAuthCompatibilityCompiled = ts.transpileModule(actionAuthCompatibilitySource, {
  compilerOptions: {
    module: ts.ModuleKind.CommonJS,
    target: ts.ScriptTarget.ES2022,
  },
}).outputText;
const actionAuthCompatibilityModule = { exports: {} };
vm.runInNewContext(actionAuthCompatibilityCompiled, {
  module: actionAuthCompatibilityModule,
  exports: actionAuthCompatibilityModule.exports,
  require,
  Set,
});
const scopeUpgradeCompiled = ts.transpileModule(scopeUpgradeSource, {
  compilerOptions: {
    module: ts.ModuleKind.CommonJS,
    target: ts.ScriptTarget.ES2022,
  },
}).outputText;
const scopeUpgradeModule = { exports: {} };
vm.runInNewContext(scopeUpgradeCompiled, {
  module: scopeUpgradeModule,
  exports: scopeUpgradeModule.exports,
  require: specifier =>
    specifier === './action-auth-compatibility'
      ? actionAuthCompatibilityModule.exports
      : require(specifier),
  Set,
});
const { actionIDsForAuthMethod, actionSupportsAuthMethod } = actionAuthCompatibilityModule.exports;
const { resolveOAuthScopeUpgradeActionIDs } = scopeUpgradeModule.exports;
const scopeProvider = {
  actions: [
    {
      id: 'gmail.account.get',
      required_scopes: ['gmail.identity.read'],
      supported_auth_method_ids: ['google_account_oauth'],
    },
    {
      id: 'gmail.mail.send',
      required_scopes: ['gmail.send'],
      supported_auth_method_ids: ['google_account_oauth'],
    },
    {
      id: 'gmail.domain.audit',
      required_scopes: ['gmail.send'],
      supported_auth_method_ids: ['google_service_account'],
    },
  ],
};
assert.deepEqual(
  [
    ...resolveOAuthScopeUpgradeActionIDs(
      {
        attention_code: 'scope_update_required',
        missing_required_scopes: ['GMAIL.SEND'],
        auth_method_id: 'google_account_oauth',
      },
      scopeProvider
    ),
  ],
  ['gmail.mail.send'],
  'scope upgrades must request only actions mapped to the missing scope and current auth method'
);
assert.equal(
  resolveOAuthScopeUpgradeActionIDs(
    {
      attention_code: 'scope_update_required',
      missing_required_scopes: ['unknown.scope'],
      auth_method_id: 'google_account_oauth',
    },
    scopeProvider
  ),
  null,
  'unmapped scope gaps must fail closed instead of requesting every action'
);
assert.equal(
  resolveOAuthScopeUpgradeActionIDs(
    { attention_code: null, auth_method_id: 'google_account_oauth' },
    scopeProvider
  ),
  undefined,
  'ordinary reconnects should preserve provider defaults and existing scopes'
);
assert.equal(
  actionSupportsAuthMethod(
    { supported_auth_method_ids: [' Google_Account_OAuth '] },
    'google_account_oauth'
  ),
  true,
  'auth method compatibility should normalize identifiers'
);
assert.equal(
  actionSupportsAuthMethod({ supported_auth_method_ids: [] }, 'any_method'),
  true,
  'an empty compatibility list should mean every auth method'
);
assert.deepEqual(
  [
    ...actionIDsForAuthMethod(scopeProvider.actions, 'google_account_oauth', [
      'gmail.mail.send',
      'gmail.domain.audit',
    ]),
  ],
  ['gmail.mail.send'],
  'OAuth defaults must not request actions reserved for another auth method'
);

console.log('integration OAuth flow state and popup privacy checks passed.');
