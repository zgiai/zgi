import type { ConnectorRuntimeRequiresConfirmationResult } from '@/services/types/connector';
import type { DigitalHumanRun } from '@/services/types/digital-human';

/**
 * Context the DigitalHuman run owns. The projection is only trusted when it
 * matches the durable run that is waiting for approval.
 */
export interface DigitalHumanRunConnectorConfirmationContext {
  digitalHumanId: string;
  run: DigitalHumanRun | null | undefined;
}

/**
 * Every key the server may project for a pending Connector confirmation. Extra
 * keys are rejected so provider output, credentials, or frozen operation input
 * can never be mistaken for consent UI.
 */
const CONFIRMATION_KEYS: readonly string[] = [
  'route_receipt',
  'connector_id',
  'connector_name',
  'connector_name_i18n',
  'operation_id',
  'action_name',
  'action_name_i18n',
  'target',
  'consequence',
  'risk_classification',
];

const TARGET_KEYS: readonly string[] = ['type', 'id', 'scope', 'version_id', 'binding_instance_id'];

const CONSEQUENCE_KEYS: readonly string[] = [
  'external_effect',
  'description',
  'description_i18n',
  'data_egress',
  'external_destination',
];

const CANONICAL_UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const NIL_UUID = '00000000-0000-0000-0000-000000000000';

function recordOf(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

function hasOnlyKeys(record: Record<string, unknown>, allowed: readonly string[]): boolean {
  return Object.keys(record).every(key => allowed.includes(key));
}

function hasControlCharacter(value: string): boolean {
  return Array.from(value).some(character => {
    const code = character.charCodeAt(0);
    return code < 32 || code === 127;
  });
}

function safeDisplayString(value: unknown, maxLength = 256): value is string {
  return (
    typeof value === 'string' &&
    value.length > 0 &&
    value.length <= maxLength &&
    value === value.trim() &&
    !hasControlCharacter(value)
  );
}

function optionalSafeDisplayString(value: unknown, maxLength = 512): value is string | undefined {
  return value === undefined || safeDisplayString(value, maxLength);
}

function optionalSafeLocalizedText(value: unknown): value is Record<string, string> | undefined {
  if (value === undefined) return true;
  const record = recordOf(value);
  if (!record || Object.keys(record).length > 16) return false;
  const seen = new Set<string>();
  return Object.entries(record).every(([locale, text]) => {
    const normalized = locale.replaceAll('_', '-').toLocaleLowerCase();
    const equivalent =
      normalized === 'en' || normalized === 'en-us'
        ? 'en-us'
        : normalized === 'zh' || normalized === 'zh-cn' || normalized === 'zh-hans'
          ? 'zh-hans'
          : normalized;
    if (
      !/^[A-Za-z]{2,8}(?:-[A-Za-z0-9]{1,8})*$/.test(normalized) ||
      seen.has(equivalent) ||
      typeof text !== 'string' ||
      !safeDisplayString(text.trim(), 1024)
    ) {
      return false;
    }
    seen.add(equivalent);
    return true;
  });
}

function trimLocalizedText(values: Record<string, string>): Record<string, string> {
  return Object.fromEntries(
    Object.entries(values).map(([locale, value]) => [locale, value.trim()])
  );
}

function validRouteReceipt(value: unknown): value is string {
  return (
    typeof value === 'string' &&
    value.length > 0 &&
    value.length <= 128 &&
    value === value.trim() &&
    !value.includes('\u0000') &&
    !value.includes('\r') &&
    !value.includes('\n')
  );
}

/** The backend validates a canonical, non-nil UUID for a DigitalHuman binding instance. */
function validBindingInstanceID(value: unknown): value is string {
  return typeof value === 'string' && CANONICAL_UUID.test(value) && value !== NIL_UUID;
}

/**
 * Strictly parses the server-authored `pending_connector_confirmation`
 * projection for one durable DigitalHuman run.
 *
 * It returns a card-ready confirmation only when the run is durably
 * `waiting`/`waiting_approval` with a receipt and the projection matches that
 * run's receipt, DigitalHuman, published version, and a valid binding instance
 * at a consent-worthy risk. Anything else fails closed.
 */
export function parseDigitalHumanRunConnectorConfirmation(
  value: unknown,
  context: DigitalHumanRunConnectorConfirmationContext
): ConnectorRuntimeRequiresConfirmationResult | null {
  const { run } = context;
  if (!run || run.status !== 'waiting' || run.waiting_reason !== 'waiting_approval') {
    return null;
  }

  const routeReceipt = run.waiting_receipt;
  const digitalHumanId = context.digitalHumanId;
  const versionId = run.digital_human_version_id;
  if (
    !validRouteReceipt(routeReceipt) ||
    !safeDisplayString(digitalHumanId) ||
    !safeDisplayString(versionId)
  ) {
    return null;
  }

  const record = recordOf(value);
  if (
    !record ||
    !hasOnlyKeys(record, CONFIRMATION_KEYS) ||
    !validRouteReceipt(record.route_receipt) ||
    record.route_receipt !== routeReceipt ||
    !safeDisplayString(record.connector_id) ||
    !safeDisplayString(record.connector_name) ||
    !optionalSafeLocalizedText(record.connector_name_i18n) ||
    !safeDisplayString(record.operation_id) ||
    !safeDisplayString(record.action_name) ||
    !optionalSafeLocalizedText(record.action_name_i18n)
  ) {
    return null;
  }

  const target = recordOf(record.target);
  if (
    !target ||
    !hasOnlyKeys(target, TARGET_KEYS) ||
    target.type !== 'digital_human' ||
    !safeDisplayString(target.id) ||
    target.id !== digitalHumanId ||
    target.scope !== 'published' ||
    !safeDisplayString(target.version_id) ||
    target.version_id !== versionId ||
    !validBindingInstanceID(target.binding_instance_id)
  ) {
    return null;
  }

  const consequence = recordOf(record.consequence);
  if (
    !consequence ||
    !hasOnlyKeys(consequence, CONSEQUENCE_KEYS) ||
    !safeDisplayString(consequence.external_effect) ||
    !optionalSafeDisplayString(consequence.description, 1024) ||
    !optionalSafeLocalizedText(consequence.description_i18n) ||
    typeof consequence.data_egress !== 'boolean' ||
    !optionalSafeDisplayString(consequence.external_destination) ||
    (record.risk_classification !== 'high' && record.risk_classification !== 'unclassified')
  ) {
    return null;
  }

  return {
    state: 'requires_confirmation',
    confirmation: {
      route_receipt: record.route_receipt,
      connector_id: record.connector_id,
      connector_name: record.connector_name,
      ...(record.connector_name_i18n
        ? { connector_name_i18n: trimLocalizedText(record.connector_name_i18n) }
        : {}),
      operation_id: record.operation_id,
      action_name: record.action_name,
      ...(record.action_name_i18n
        ? { action_name_i18n: trimLocalizedText(record.action_name_i18n) }
        : {}),
      target: {
        type: 'digital_human',
        id: target.id,
        scope: 'published',
        version_id: target.version_id,
        binding_instance_id: target.binding_instance_id,
      },
      consequence: {
        external_effect: consequence.external_effect,
        ...(consequence.description ? { description: consequence.description } : {}),
        ...(consequence.description_i18n
          ? { description_i18n: trimLocalizedText(consequence.description_i18n) }
          : {}),
        data_egress: consequence.data_egress,
        ...(consequence.external_destination
          ? { external_destination: consequence.external_destination }
          : {}),
      },
      risk_classification: record.risk_classification,
    },
  };
}
