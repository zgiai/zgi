import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import path from 'node:path';
import YAML from 'yaml';

const root = process.cwd();
const contractPath = path.resolve(root, '..', 'docs', 'connectors-api.yaml');
const contract = YAML.parse(readFileSync(contractPath, 'utf8'));

assert.equal(contract.openapi, '3.1.0');
assert.equal(contract.servers?.[0]?.url, '/console/api');

// Keep documented mutation headers aligned with the CORS policy used by the
// actual API bootstrap, not merely with route-handler tests without CORS.
const corsSource = readFileSync(
  path.resolve(root, '..', 'api', 'internal', 'bootstrap', 'fxapp', 'infra.go'),
  'utf8'
);
const corsHeaderDeclaration = corsSource.match(/var corsAllowHeaders = \[\]string\{([\s\S]*?)\n\}/);
assert.ok(corsHeaderDeclaration, 'API bootstrap must declare an explicit CORS header allowlist');
const allowedHeaders = new Set(
  [...corsHeaderDeclaration[1].matchAll(/"([^"\n]+)"/g)].map(match => match[1].toLowerCase())
);
const contractHeaders = new Set();
function collectRequestHeaders(value) {
  if (!value || typeof value !== 'object') return;
  if (value.in === 'header' && typeof value.name === 'string') {
    contractHeaders.add(value.name.toLowerCase());
  }
  for (const child of Object.values(value)) collectRequestHeaders(child);
}
collectRequestHeaders(contract.paths);
collectRequestHeaders(contract.components.parameters);
assert.ok(contractHeaders.has('idempotency-key'));
assert.ok(contractHeaders.has('x-impact-token'));
for (const header of contractHeaders) {
  assert.ok(allowedHeaders.has(header), `API CORS must allow Connector request header ${header}`);
}
assert.equal(allowedHeaders.has('*'), false, 'CORS must not allow arbitrary headers');

const expectedPaths = [
  '/connectors/{connectorId}/app-credential-setups',
  '/connectors/{connectorId}/onboarding-flows',
  '/connector-connections/{connectionId}/onboarding-flows',
  '/connectors/onboarding-flows/{flowId}',
  '/connector-preferences/chat/me',
  '/connector-preferences/chat/me/{connectorId}',
  '/connectors/catalog',
  '/connectors/catalog/{connectorId}',
  '/connectors/{connectorId}/key-setups',
  '/connectors/key-setups/{attemptId}',
  '/connector-connections',
  '/connector-connections/{connectionId}',
  '/connector-connections/{connectionId}/reconnect',
  '/connector-connections/{connectionId}/diagnostics',
  '/connector-connections/{connectionId}/removal-impact',
  '/connector-target-bindings',
  '/connector-target-bindings/{bindingId}',
  '/connectors/runtime/confirmations/{receipt}',
  '/connectors/runtime/confirmations',
  '/connector-unavailable-references',
  '/connector-unavailable-references/{referenceId}/resolve',
];
assert.deepEqual(Object.keys(contract.paths), expectedPaths);
assert.equal(
  Object.keys(contract.paths).some(route => route.includes('integrations')),
  false,
  'Connector routes must not expose the legacy integrations boundary'
);

const schemas = contract.components.schemas;
assert.deepEqual(Object.keys(contract.paths['/connectors/runtime/confirmations/{receipt}']), ['get']);
assert.equal(schemas.ConnectorRuntimeStatusResponse.oneOf[0].properties.resolution.enum[0], 'unresolved');
assert.equal(schemas.ConnectorRuntimeStatusResponse.oneOf[1].additionalProperties, false);
// Validate every local reference, including the versioned extension's request/response schemas.
function validateRefs(value) {
  if (!value || typeof value !== 'object') return;
  if (value.$ref) {
    assert.ok(value.$ref.startsWith('#/'), 'API contract uses local references');
    assert.ok(
      value.$ref
        .slice(2)
        .split('/')
        .reduce((node, key) => node?.[key], contract),
      `unresolved ${value.$ref}`
    );
  }
  for (const child of Object.values(value)) validateRefs(child);
}
validateRefs(contract);
assert.equal(schemas.AppCredentialValues.properties.app_secret.writeOnly, true);
assert.equal(schemas.AppCredentialValues.additionalProperties, false);
assert.deepEqual(schemas.AppCredentialValues.required, ['region', 'app_id', 'app_secret']);
assert.equal(schemas.ConnectorCatalogItem.required.includes('extension_schema_version'), false);
assert.equal(schemas.ConnectorCatalogItem.properties.extension_schema_version.const, '1.1.0');
assert.equal(schemas.AppProviderEvidence.properties.events_enabled.const, false);
assert.equal(schemas.AppProviderEvidence.properties.user_authorization_enabled.const, false);
assert.equal(schemas.ConnectorOnboardingFlow.additionalProperties, false);
assert.equal(schemas.ConnectorOnboardingFlow.properties.app_secret, undefined);
assert.equal(schemas.ConnectorOnboardingFlow.properties.device_code, undefined);
const addons = schemas.ConnectorPermissionTemplate.properties.addons.properties;
const compiledTemplate = JSON.parse(
  readFileSync(
    path.resolve(
      root,
      '..',
      'api/internal/modules/connectors/catalog/definitions/feishu-permissions.json'
    ),
    'utf8'
  )
);
assert.equal(
  schemas.ConnectorPermissionTemplate.properties.profile_id.const,
  compiledTemplate.profile_id
);
assert.equal(
  schemas.ConnectorPermissionTemplate.properties.profile_version.const,
  compiledTemplate.profile_version
);
assert.equal(addons.scopes.properties.tenant.maxItems, 40);
// Additive read operations use the same application setup, without new permissions or routes.
const feishuDefinition = JSON.parse(
  readFileSync(path.resolve(root, '..', 'api/internal/modules/connectors/catalog/definitions/feishu.json'), 'utf8')
);
assert.equal(feishuDefinition.plugin_version, '1.2.0');
assert.equal(feishuDefinition.operations.length, 38);
for (const id of ['feishu.tenant.get', 'feishu.chat.members.list']) {
  const operation = feishuDefinition.operations.find(item => item.id === id);
  assert.ok(operation, `missing native read ${id}`);
  assert.equal(operation.effect, 'read');
  assert.equal(operation.credential_subject, 'app');
  assert.ok(operation.name_i18n['zh-Hans']);
  assert.ok(operation.description_i18n['en-US']);
  assert.equal(operation.input_schema.additionalProperties, false);
  assert.equal(operation.input_schema.properties.connection_id, undefined);
  assert.equal(operation.input_schema.properties.app_secret, undefined);
  assert.ok(contract.info.description.includes(id), `documented catalog omits ${id}`);
}
const membersOperation = feishuDefinition.operations.find(item => item.id === 'feishu.chat.members.list');
assert.equal(membersOperation.input_schema.properties.member_id_type, undefined);
assert.equal(membersOperation.input_schema.properties.page_size.default, 20);
assert.equal(membersOperation.input_schema.properties.page_size.maximum, 100);
assert.equal(membersOperation.required_scopes.length, 1, 'member permissions are alternatives, not four AND requirements');
assert.ok(membersOperation.required_scopes[0].includes('im:chat.members:read'));
assert.equal(
  addons.scopes.properties.tenant.maxItems,
  compiledTemplate.addons.scopes.tenant.length
);
assert.equal(addons.scopes.properties.user.maxItems, 1);
assert.equal(addons.events.properties.items.properties.tenant.maxItems, 6);
assert.equal(addons.events.properties.items.properties.user.maxItems, 3);
assert.equal(addons.callbacks.properties.items.maxItems, 1);
const onboardingResponse = contract.components.responses.ConnectorOnboardingSnapshot;
assert.equal(onboardingResponse.headers['Cache-Control'].schema.const, 'no-store');
assert.equal(schemas.ChatConnectorPreferenceUpdate.additionalProperties, false);
assert.deepEqual(schemas.ChatConnectorPreferenceUpdate.required, ['mode', 'expected_revision']);
assert.deepEqual(schemas.ChatConnectorPreferenceUpdate.properties.mode.enum, ['selected', 'none']);
assert.equal(schemas.ChatConnectorPreferenceUpdate.properties.expected_revision.minimum, 0);
assert.equal(schemas.ChatConnectorPreferenceSnapshot.properties.defaults.maxProperties, 64);
assert.equal(
  schemas.InitialChatConnectorSelection.properties.initial_connector_defaults_revision.minimum,
  0
);
assert.equal(
  schemas.InitialChatConnectorSelection.properties.initial_connector_bindings.maxItems,
  64
);
assert.equal(schemas.ChatConnectorDefault.properties.connection.properties.credentials, undefined);
const preferenceUpdate = contract.paths['/connector-preferences/chat/me/{connectorId}'].put;
assert.ok(
  preferenceUpdate.parameters.some(value => value.$ref === '#/components/parameters/IdempotencyKey')
);
assert.ok(preferenceUpdate.responses['409']);
assert.deepEqual(schemas.TargetRef.properties.scope.enum, ['live', 'draft', 'published']);
assert.equal(schemas.TargetBinding.properties.compatibility, undefined);
assert.equal(schemas.ConnectorReferenceCommand.properties.expected_revision.minimum, 1);
assert.equal(schemas.ConnectorReferenceCommand.additionalProperties, false);
assert.equal(schemas.LocalizedText.type, 'object');
assert.equal(schemas.LocalizedText.maxProperties, 16);
assert.equal(schemas.LocalizedText.additionalProperties.type, 'string');
assert.equal(schemas.LocalizedText.propertyNames.maxLength, 35);
const localePattern = new RegExp(schemas.LocalizedText.propertyNames.pattern);
for (const locale of ['en-US', 'zh-Hans', 'fr-FR']) assert.ok(localePattern.test(locale));
assert.equal(localePattern.test('zh_Hans'), false, 'public metadata uses BCP 47 locale tags');
for (const [schemaName, fields] of [
  ['ConnectorCatalogItem', ['name_i18n', 'description_i18n']],
  ['CapabilitySummary', ['name_i18n', 'description_i18n']],
  ['KeyField', ['label_i18n', 'placeholder_i18n']],
  ['ConnectorRuntimeConfirmation', ['connector_name_i18n', 'action_name_i18n']],
  ['ConnectorRuntimeConsequence', ['description_i18n']],
]) {
  for (const field of fields) {
    assert.equal(schemas[schemaName].properties[field].$ref, '#/components/schemas/LocalizedText');
    assert.equal(
      schemas[schemaName].required.includes(field),
      false,
      `${field} must remain optional for older plugin versions`
    );
  }
}
assert.deepEqual(schemas.ConnectorClassification.enum, ['connector', 'data_source']);
assert.deepEqual(schemas.ConnectorReadiness.enum, ['ready', 'reconnect_required', 'unavailable']);
assert.deepEqual(
  schemas.ConnectorCatalogItem.properties.connection_summary.properties.needs_attention_count,
  { type: 'integer', minimum: 0 }
);
assert.equal(
  schemas.ConnectorCatalogItem.properties.connection_summary.required.includes(
    'needs_attention_count'
  ),
  false,
  'attention counts are an optional rolling-upgrade field, not a replacement for primary readiness'
);

const keySetupRequest = schemas.KeySetupRequest;
assert.equal(keySetupRequest.additionalProperties, false);
assert.deepEqual(keySetupRequest.required, ['values']);
assert.equal(keySetupRequest.properties.values.writeOnly, true);
assert.equal(keySetupRequest.properties.values.additionalProperties.minLength, 1);
assert.equal(schemas.KeyField.properties.secret.const, true);

const keySetupOperation = contract.paths['/connectors/{connectorId}/key-setups'].post;
const reconnectOperation = contract.paths['/connector-connections/{connectionId}/reconnect'].post;
for (const operation of [keySetupOperation, reconnectOperation]) {
  const idempotencyParameter = operation.parameters.find(
    parameter => parameter.$ref === '#/components/parameters/IdempotencyKey'
  );
  assert.ok(idempotencyParameter, 'Key mutations must require an idempotency key');
}
assert.equal(
  keySetupOperation.requestBody.content['application/json'].schema.$ref,
  '#/components/schemas/KeySetupRequest'
);
assert.equal(
  reconnectOperation.requestBody.content['application/json'].schema.$ref,
  '#/components/schemas/ReconnectKeyRequest'
);
assert.equal(schemas.ReconnectKeyRequest.properties.values.writeOnly, true);
assert.equal(schemas.ReconnectKeyRequest.properties.confirm_unstable_identity.type, 'boolean');

const detachBindingOperation = contract.paths['/connector-target-bindings/{bindingId}'].delete;
assert.equal(
  detachBindingOperation.requestBody.content['application/json'].schema.$ref,
  '#/components/schemas/DetachTargetBindingRequest'
);
assert.deepEqual(schemas.DetachTargetBindingRequest.required, ['revision']);
assert.equal(schemas.DetachTargetBindingRequest.properties.revision.minimum, 1);
assert.ok(
  schemas.TargetBinding.required.includes('revision'),
  'binding responses must expose the revision required for detach CAS'
);

const confirmationOperation = contract.paths['/connectors/runtime/confirmations'].post;
assert.equal(
  confirmationOperation.requestBody.content['application/json'].schema.$ref,
  '#/components/schemas/ConnectorRuntimeConfirmationCommand'
);
assert.equal(schemas.ConnectorRuntimeConfirmationCommand.additionalProperties, false);
assert.deepEqual(schemas.ConnectorRuntimeConfirmationCommand.required, [
  'route_receipt',
  'decision',
]);
assert.deepEqual(Object.keys(schemas.ConnectorRuntimeConfirmationCommand.properties), [
  'route_receipt',
  'decision',
]);
assert.deepEqual(schemas.ConnectorRuntimeConfirmationCommand.properties.decision.enum, [
  'confirmed',
  'cancelled',
]);

const responseRoots = [
  'ConnectorCatalogItem',
  'KeySetupResult',
  'PersonalConnection',
  'PersonalConnectionDetail',
  'TargetBinding',
  'ConnectorRuntimeConfirmation',
  'ConnectorRuntimeConfirmationResponse',
  'RemovalImpact',
  'ConnectorOnboardingFlow',
];
const responsePropertyNames = new Set();

function visitSchema(schema, seenRefs = new Set()) {
  if (!schema || typeof schema !== 'object') return;
  if (schema.$ref) {
    const name = schema.$ref.split('/').at(-1);
    if (!name || seenRefs.has(name)) return;
    seenRefs.add(name);
    // The versioned, fixed configuration template alone may use the word
    // "scopes". Legacy responses and actual credentials keep their old ban.
    if (name === 'ConnectorPermissionTemplate') {
      const template = schemas[name];
      assert.equal(template.additionalProperties, false);
      assert.deepEqual(Object.keys(template.properties), [
        'profile_id',
        'profile_version',
        'addons',
        'user_event_dependencies',
      ]);
      assert.deepEqual(Object.keys(template.properties.addons.properties), [
        'preset',
        'scopes',
        'events',
        'callbacks',
      ]);
      assert.doesNotMatch(
        JSON.stringify(template),
        /app_secret|device_code|access_token|encrypted_credentials/
      );
      return;
    }
    visitSchema(schemas[name], seenRefs);
    return;
  }
  for (const name of Object.keys(schema.properties ?? {})) {
    responsePropertyNames.add(name);
  }
  for (const value of Object.values(schema.properties ?? {})) visitSchema(value, seenRefs);
  for (const value of schema.allOf ?? []) visitSchema(value, seenRefs);
  for (const value of schema.oneOf ?? []) visitSchema(value, seenRefs);
  if (schema.items) visitSchema(schema.items, seenRefs);
}

for (const name of responseRoots) visitSchema(schemas[name]);

const forbiddenResponseProperties = [
  'values',
  'key',
  'api_key',
  'encrypted_credentials',
  'grants',
  'scopes',
  'policies',
  'auth_type',
  'auth_method',
  'auth_method_id',
  'credential_source',
];
for (const property of forbiddenResponseProperties) {
  assert.equal(
    responsePropertyNames.has(property),
    false,
    `response schemas must not expose ${property}`
  );
}

const publicContractText = JSON.stringify({ paths: contract.paths, schemas });
for (const forbiddenConcept of [
  'complete-setup',
  'oauth',
  'credential_source',
  'allowed_action_ids',
]) {
  assert.equal(
    publicContractText.toLowerCase().includes(forbiddenConcept),
    false,
    `public Connector contract must not expose ${forbiddenConcept}`
  );
}

console.log('Connector API contract shape checks passed.');
