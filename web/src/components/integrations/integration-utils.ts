import type {
  IntegrationAuthDefinition,
  IntegrationCatalogItem,
  IntegrationConnection,
  IntegrationConnectionHealthState,
  IntegrationCredentialField,
  IntegrationCredentialSchema,
  IntegrationProviderHealthState,
  IntegrationCredentialSource,
} from '@/services/types/integration';

export function integrationCatalogID(item: Pick<IntegrationCatalogItem, 'id' | 'integration_id'>) {
  return item.integration_id?.trim() || item.id.trim();
}

export function resolveIntegrationAuthDefinitions(
  item: IntegrationCatalogItem | undefined
): IntegrationAuthDefinition[] {
  if (!item) return [];
  if (item.auth?.length) {
    return item.auth.filter(
      definition =>
        definition.type !== 'platform' && integrationAuthCredentialSource(definition) !== 'platform'
    );
  }

  const sources = new Set((item.credential_sources ?? []).filter(source => source !== 'platform'));
  const supportedAuthTypes = (item.auth_types ?? []).filter(type => type !== 'platform');
  const authTypes = supportedAuthTypes.length
    ? supportedAuthTypes
    : sources.has('organization')
      ? ['api_key']
      : [];
  const definitions: IntegrationAuthDefinition[] = [];

  for (const authType of authTypes) {
    const candidateSources = sources.has('account')
      ? (['organization', 'account'] as const).filter(source => sources.has(source))
      : (['organization'] as const);
    for (const source of candidateSources) {
      definitions.push({
        id: `${source}-${authType}`,
        type: authType,
        label: authType,
        available: true,
        credential_source: source,
        credential_schema:
          authType === 'api_key'
            ? (item.credential_schema ?? legacyAPIKeySchema())
            : item.credential_schema,
      });
    }
  }

  return definitions;
}

export function integrationAuthCredentialSource(
  definition: IntegrationAuthDefinition
): IntegrationCredentialSource {
  return (
    definition.credential_source ?? (definition.type === 'platform' ? 'platform' : 'organization')
  );
}

function legacyAPIKeySchema(): IntegrationCredentialSchema {
  return {
    type: 'object',
    fields: [
      {
        name: 'api_key',
        type: 'password',
        required: true,
        secret: true,
        storage: 'credentials',
      },
    ],
  };
}

export function resolveCredentialFields(
  item: IntegrationCatalogItem | undefined,
  auth: IntegrationAuthDefinition | undefined
): IntegrationCredentialField[] {
  if (auth?.fields?.length) {
    return auth.fields.map(field =>
      normalizeCredentialField({
        ...field,
        name: field.key,
        label: field.label,
        description: field.description,
        placeholder: field.placeholder,
        type:
          field.input === 'url' || field.input === 'text'
            ? 'text'
            : (field.input ?? (field.secret ? 'password' : 'text')),
        required: field.required,
        secret: field.secret,
        options: field.options,
        storage: 'credentials',
      })
    );
  }
  const schema = auth?.credential_schema ?? item?.credential_schema;
  if (!schema && auth?.type === 'api_key') return legacyAPIKeySchema().fields ?? [];
  if (!schema) return [];
  if (schema.fields?.length) return schema.fields.map(field => normalizeCredentialField(field));

  const required = new Set(schema.required ?? []);
  return Object.entries(schema.properties ?? {}).map(([name, property]) => {
    const secret = Boolean(
      property.writeOnly || property['x-secret'] || property.format === 'password'
    );
    const enumValues = property.enum ?? [];
    const type =
      enumValues.length > 0
        ? 'select'
        : property.type === 'boolean'
          ? 'boolean'
          : secret
            ? 'password'
            : 'text';
    return normalizeCredentialField({
      ...property,
      name,
      label: property.title,
      label_i18n: property.title_i18n,
      description: property.description,
      description_i18n: property.description_i18n,
      placeholder: property['x-placeholder'],
      placeholder_i18n: property['x-placeholder-i18n'],
      type,
      required: required.has(name),
      secret,
      default_value:
        typeof property.default === 'boolean' ? property.default : String(property.default ?? ''),
      options: enumValues.map((value, index) => ({
        value: String(value),
        label: property.enumNames?.[index] ?? String(value),
      })),
      storage: property['x-storage'] ?? 'credentials',
    });
  });
}

function normalizeCredentialField(field: IntegrationCredentialField): IntegrationCredentialField {
  const secret = field.secret ?? field.type === 'password';
  return {
    ...field,
    name: field.name.trim(),
    type: field.type ?? (secret ? 'password' : 'text'),
    secret,
    storage: field.storage ?? 'credentials',
    options: field.options ?? [],
  };
}

export function resolveConnectionHealthState(
  connection: IntegrationConnection,
  now = Date.now()
): IntegrationConnectionHealthState {
  if (connection.status === 'disabled') return 'disabled';
  if (connection.expires_at) {
    const expiresAt = Date.parse(connection.expires_at);
    if (Number.isFinite(expiresAt) && expiresAt <= now) return 'expired';
  }
  if (connection.refresh_token_expires_at) {
    const refreshTokenExpiresAt = Date.parse(connection.refresh_token_expires_at);
    if (Number.isFinite(refreshTokenExpiresAt) && refreshTokenExpiresAt <= now) return 'expired';
  }
  if (connection.auth_status === 'expired') return 'expired';
  if (connection.auth_status === 'reconnect_required') return 'revoked';
  if (connection.health_status === 'unhealthy') return 'error';
  if (
    connection.health_status === 'degraded' ||
    connection.scope_status === 'drifted' ||
    Boolean(connection.attention_code)
  ) {
    return 'degraded';
  }
  if (connection.health_status === 'healthy') return 'ready';
  if (connection.status === 'pending') return 'testing';
  if (connection.status === 'invalid') return 'error';
  if (connection.last_error_code) return 'degraded';
  return 'unknown';
}

export function resolveProviderHealthState(
  item: IntegrationCatalogItem,
  connections: IntegrationConnection[]
): IntegrationProviderHealthState {
  if (item.health_state) return item.health_state;
  if (!item.enabled) return 'unavailable';

  const scoped = connections.filter(
    connection => connection.integration_id === integrationCatalogID(item)
  );
  if (scoped.some(connection => resolveConnectionHealthState(connection) === 'ready')) {
    return 'ready';
  }
  if (
    scoped.some(connection =>
      ['degraded', 'expired', 'revoked', 'error'].includes(resolveConnectionHealthState(connection))
    )
  ) {
    return 'degraded';
  }
  if (scoped.length > 0 || (item.connection_summary?.active ?? 0) > 0) {
    // Credentials being present is not evidence that the upstream provider
    // accepted them. Only an explicit healthy observation is "ready".
    return 'configured';
  }
  return 'setup_required';
}
