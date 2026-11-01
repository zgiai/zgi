import type { ConnectorConfirmationPreview } from '@/services/types/connector';

const object = (value: unknown): Record<string, unknown> | null =>
  value !== null && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
const text = (value: unknown, limit: number): value is string =>
  typeof value === 'string' &&
  Array.from(value).length <= limit &&
  Array.from(value).every(char => {
    const code = char.charCodeAt(0);
    return !((code < 32 && code !== 9 && code !== 10) || code === 127 || /\p{Cf}/u.test(char));
  });
const only = (value: Record<string, unknown>, keys: string[]) =>
  Object.keys(value).every(key => keys.includes(key));

/** Authenticated query-only presentation. It is never parsed from model output. */
export function parseConnectorConfirmationPresentation(
  value: unknown
): ConnectorConfirmationPreview | null {
  const envelope = object(value);
  if (
    !envelope ||
    !only(envelope, ['version', 'preview', 'one_shot']) ||
    envelope.version !== 1 ||
    (envelope.one_shot !== undefined && typeof envelope.one_shot !== 'boolean')
  ) {
    return null;
  }
  const preview = object(envelope.preview);
  if (
    !preview ||
    !only(preview, ['account_name', 'account_identity', 'kind', 'fields', 'complete']) ||
    !text(preview.account_name, 128) ||
    !text(preview.account_identity, 512) ||
    !['mail', 'message', 'resource'].includes(String(preview.kind)) ||
    typeof preview.complete !== 'boolean' ||
    !Array.isArray(preview.fields) ||
    preview.fields.length > 32
  ) {
    return null;
  }
  const fields: ConnectorConfirmationPreview['fields'] = [];
  for (const value of preview.fields) {
    const field = object(value);
    const labels = object(field?.label_i18n);
    if (
      !field ||
      !only(field, ['label', 'label_i18n', 'value', 'truncated', 'format']) ||
      (field.format !== undefined && field.format !== 'command') ||
      !text(field.label, 128) ||
      !text(field.value, 8192) ||
      typeof field.truncated !== 'boolean' ||
      !labels ||
      !only(labels, ['en-US', 'zh-Hans']) ||
      !Object.values(labels).every(value => text(value, 128))
    ) {
      return null;
    }
    fields.push({
      label: field.label,
      label_i18n: labels as Record<string, string>,
      value: field.value,
      truncated: field.truncated,
      ...(field.format === 'command' ? { format: 'command' as const } : {}),
    });
  }
  return {
    account_name: preview.account_name,
    account_identity: preview.account_identity,
    kind: preview.kind as ConnectorConfirmationPreview['kind'],
    fields,
    complete: preview.complete && Boolean(preview.account_identity) && fields.length > 0,
  };
}

/**
 * Whether the server marked the confirmation one-shot: its surface
 * (DigitalHuman) confirms each operation once and rejects a session grant.
 */
export function connectorConfirmationIsOneShot(value: unknown): boolean {
  return object(value)?.one_shot === true;
}
