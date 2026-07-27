import type {
  IntegrationAuthDefinition,
  IntegrationCredentialField,
  IntegrationProviderCredentialField,
} from '@/services/types/integration';

const DEFAULT_OAUTH_CLIENT_FIELDS: IntegrationCredentialField[] = [
  {
    name: 'client_id',
    type: 'text',
    required: true,
    storage: 'credentials',
  },
  {
    name: 'client_secret',
    type: 'password',
    required: true,
    secret: true,
    storage: 'credentials',
  },
];

function normalizeProviderClientField(
  field: IntegrationProviderCredentialField
): IntegrationCredentialField {
  const secret = field.secret ?? field.input === 'password';
  return {
    name: field.key.trim(),
    label: field.label,
    label_i18n: field.label_i18n,
    description: field.description,
    description_i18n: field.description_i18n,
    placeholder: field.placeholder,
    placeholder_i18n: field.placeholder_i18n,
    type:
      field.input === 'url' || field.input === 'text'
        ? 'text'
        : (field.input ?? (secret ? 'password' : 'text')),
    required: field.required,
    secret,
    options: field.options,
    storage: field.key === 'client_id' || field.key === 'client_secret' ? 'credentials' : 'config',
  };
}

function normalizeClientField(
  field: IntegrationCredentialField | IntegrationProviderCredentialField
): IntegrationCredentialField {
  if ('key' in field) return normalizeProviderClientField(field);
  const secret = field.secret ?? field.type === 'password';
  return {
    ...field,
    name: field.name.trim(),
    type: field.type ?? (secret ? 'password' : 'text'),
    secret,
    storage:
      field.storage ??
      (field.name === 'client_id' || field.name === 'client_secret' ? 'credentials' : 'config'),
  };
}

export function resolveOAuthClientFields(
  auth: IntegrationAuthDefinition | null | undefined
): IntegrationCredentialField[] {
  const configured = auth?.oauth?.client_fields;
  const fields = configured?.length ? configured : DEFAULT_OAUTH_CLIENT_FIELDS;
  const seen = new Set<string>();
  return fields.map(normalizeClientField).filter(field => {
    if (!field.name || seen.has(field.name)) return false;
    seen.add(field.name);
    return true;
  });
}
