import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import vm from 'node:vm';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';

const require = createRequire(import.meta.url);
const ts = require('typescript');
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const selectionPath = path.join(
  root,
  'src',
  'components',
  'integrations',
  'auth-method-selection.ts'
);
const pickerPath = path.join(
  root,
  'src',
  'components',
  'integrations',
  'auth-method-picker-dialog.tsx'
);
const catalogPath = path.join(root, 'src', 'components', 'integrations', 'provider-catalog.tsx');
const dialogPath = path.join(root, 'src', 'components', 'integrations', 'connection-dialog.tsx');
const oauthClientDialogPath = path.join(
  root,
  'src',
  'components',
  'integrations',
  'oauth-client-config-dialog.tsx'
);

const selectionSource = fs.readFileSync(selectionPath, 'utf8');
const compiled = ts.transpileModule(selectionSource, {
  compilerOptions: {
    module: ts.ModuleKind.CommonJS,
    target: ts.ScriptTarget.ES2022,
  },
}).outputText;
const module = { exports: {} };
vm.runInNewContext(compiled, {
  module,
  exports: module.exports,
  require: specifier => {
    if (specifier === './integration-utils') {
      return {
        integrationAuthCredentialSource: auth =>
          auth.credential_source ?? (auth.type === 'platform' ? 'platform' : 'organization'),
      };
    }
    return require(specifier);
  },
});

const {
  authMethodCanStart,
  authMethodsSharingOAuthClient,
  oauthClientConfigID,
  resolveAuthMethodPresentation,
  selectPrimaryAuthMethod,
} = module.exports;

const personalOAuth = {
  id: 'user-oauth',
  type: 'oauth2',
  credential_source: 'account',
  label: 'User OAuth',
  available: true,
  oauth: {
    connect_enabled: true,
    client_configured: true,
  },
};
const organizationForm = {
  id: 'tenant-app',
  type: 'service_account',
  credential_source: 'organization',
  label: 'Tenant app',
  available: true,
};
assert.equal(
  selectPrimaryAuthMethod([organizationForm, personalOAuth], false)?.id,
  'user-oauth',
  'browser authorization should be the provider-neutral primary experience'
);
const organizationOAuth = {
  ...personalOAuth,
  id: 'organization-oauth',
  credential_source: 'organization',
  oauth: {
    ...personalOAuth.oauth,
    client_config_id: 'shared-provider-app',
  },
};
const accountOAuth = {
  ...personalOAuth,
  oauth: {
    ...personalOAuth.oauth,
    client_config_id: 'shared-provider-app',
  },
};
assert.equal(oauthClientConfigID(accountOAuth), 'shared-provider-app');
assert.deepEqual(
  authMethodsSharingOAuthClient([accountOAuth, organizationOAuth, organizationForm], accountOAuth)
    .map(method => method.id)
    .sort(),
  ['organization-oauth', 'user-oauth'],
  'personal and organization OAuth connections may intentionally share one provider app'
);
assert.equal(
  authMethodCanStart(
    {
      ...personalOAuth,
      oauth: { connect_enabled: true, client_configured: false },
    },
    false
  ),
  false,
  'members must not start OAuth without a configured client'
);
assert.equal(
  authMethodCanStart(
    {
      ...personalOAuth,
      oauth: { connect_enabled: true, client_configured: false },
    },
    true
  ),
  true,
  'administrators may select an OAuth method that needs client configuration'
);
assert.deepEqual(
  { ...resolveAuthMethodPresentation(organizationForm) },
  {
    credentialSource: 'organization',
    identityKind: 'service',
    acquisitionStrategy: 'manual_form',
    lifecycleStrategy: 'exchange_on_demand',
    requestAuthStrategy: 'provider_custom',
  },
  'legacy catalogs should receive a conservative provider-neutral presentation'
);
assert.equal(
  resolveAuthMethodPresentation({
    id: 'personal-token',
    type: 'api_key',
    credential_source: 'account',
    label: 'Personal token',
    available: true,
  }).identityKind,
  'application',
  'credential ownership must not be mistaken for the external identity kind'
);
assert.deepEqual(
  {
    ...resolveAuthMethodPresentation({
      ...organizationForm,
      identity_kind: 'channel',
      acquisition_strategy: 'manual_form',
      lifecycle_strategy: 'static',
      request_auth_strategy: 'webhook_url',
    }),
  },
  {
    credentialSource: 'organization',
    identityKind: 'channel',
    acquisitionStrategy: 'manual_form',
    lifecycleStrategy: 'static',
    requestAuthStrategy: 'webhook_url',
  },
  'explicit provider catalog strategies must drive the UI'
);
assert.equal(
  resolveAuthMethodPresentation({
    id: 'anonymous',
    type: 'no_auth',
    credential_source: 'organization',
    label: 'Anonymous',
    available: true,
  }).requestAuthStrategy,
  'none',
  'no-auth methods must not claim a request credential strategy'
);

const pickerSource = fs.readFileSync(pickerPath, 'utf8');
const catalogSource = fs.readFileSync(catalogPath, 'utf8');
const dialogSource = fs.readFileSync(dialogPath, 'utf8');
const oauthClientDialogSource = fs.readFileSync(oauthClientDialogPath, 'utf8');
assert.doesNotMatch(
  pickerSource,
  /integrationId\s*===|providerName\s*===|gmail|feishu|twitter|['"]x['"]/i,
  'the generic picker must not branch on provider identity'
);
assert.match(pickerSource, /resolveAuthMethodPresentation\(method\)/);
assert.match(pickerSource, /authMethodPicker\.identity/);
assert.match(pickerSource, /authMethodPicker\.credentialSource/);
assert.match(pickerSource, /authMethodPicker\.sharedOAuth\.title/);
assert.match(pickerSource, /authMethodPicker\.result\./);
assert.match(pickerSource, /actionsForAuthMethod\(actions,\s*method\.id\)/);
assert.match(catalogSource, /selectPrimaryAuthMethod\(authDefinitions,\s*canManageShared\)/);
assert.match(catalogSource, /lockedAuthMethodId=\{selectedAuthMethod\?\.id\}/);
assert.match(catalogSource, /actions=\{authPickerProvider\?\.actions \?\? \[\]\}/);
assert.match(catalogSource, /authMethodsSharingOAuthClient/);
assert.match(catalogSource, /onConfigured=/);
assert.match(dialogSource, /lockedAuthMethodId/);
assert.match(oauthClientDialogSource, /oauth\.clientConfig\.sharedTitle/);
assert.match(oauthClientDialogSource, /useIntegrationOAuthClientConfigImpact/);
assert.match(oauthClientDialogSource, /oauth\.clientConfig\.rotateSecret/);
assert.match(oauthClientDialogSource, /oauth\.clientConfig\.dangerTitle/);
assert.match(oauthClientDialogSource, /oauth\.clientConfig\.saveAndContinue/);
assert.match(oauthClientDialogSource, /await onConfigured\?\.\(\)/);

console.log('integration authentication method picker checks passed.');
