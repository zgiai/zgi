import type {
  ConnectorRuntimeConfirmation,
  ConnectorRuntimeConfirmationCommand,
  ConnectorRuntimeConfirmationDecision,
  ConnectorRuntimeDecisionResult,
  ConnectorRuntimeExecutionResult,
} from '@/services/types/connector';

export type ConnectorRuntimeExecutionState = ConnectorRuntimeExecutionResult['state'];
export type {
  ConnectorRuntimeBlockedResult,
  ConnectorRuntimeCancelledResult,
  ConnectorRuntimeCompletedResult,
  ConnectorRuntimeConfirmation,
  ConnectorRuntimeConfirmationCommand,
  ConnectorRuntimeConfirmationDecision,
  ConnectorRuntimeConfirmationRisk,
  ConnectorRuntimeConfirmationResponse,
  ConnectorRuntimeConsequence,
  ConnectorRuntimeDecisionOutcomeState,
  ConnectorRuntimeDecisionResult,
  ConnectorRuntimeExecutionResult,
  ConnectorRuntimeFailedResult,
  ConnectorRuntimeRequiresConfirmationResult,
  ConnectorRuntimeRiskClassification,
  ConnectorRuntimeUnknownResult,
} from '@/services/types/connector';

/** Host callback for a single receipt decision, optionally remembered in-session. */
export type ConnectorRuntimeDecisionHandler = (
  command: ConnectorRuntimeConfirmationCommand,
  scope?: string,
  rememberForSession?: boolean
) => Promise<ConnectorRuntimeDecisionResult>;

export type ConnectorRuntimeDecisionState =
  | { status: 'idle' }
  | {
      status: 'submitting';
      routeReceipt: string;
      decision: ConnectorRuntimeConfirmationDecision;
    }
  | {
      status: 'resolved';
      routeReceipt: string;
      decision: ConnectorRuntimeConfirmationDecision;
      outcome: ConnectorRuntimeDecisionResult['state'];
    }
  | {
      status: 'unknown';
      routeReceipt: string;
      decision: ConnectorRuntimeConfirmationDecision;
    };

export interface ConnectorRuntimeDecisionAttempt {
  state: Extract<ConnectorRuntimeDecisionState, { status: 'submitting' }>;
  command: ConnectorRuntimeConfirmationCommand;
}

export function createConnectorRuntimeDecisionState(): ConnectorRuntimeDecisionState {
  return { status: 'idle' };
}

export function validConnectorRuntimeRouteReceipt(value: string): boolean {
  return (
    value.length > 0 &&
    value.length <= 128 &&
    value === value.trim() &&
    !value.includes('\u0000') &&
    !value.includes('\r') &&
    !value.includes('\n')
  );
}

function validDecision(value: string): value is ConnectorRuntimeConfirmationDecision {
  return value === 'confirmed' || value === 'cancelled';
}

/**
 * Starts one decision for one server-issued receipt. Terminal execution states,
 * duplicate clicks, and already-resolved receipts cannot create a command.
 */
export function beginConnectorRuntimeDecision(
  current: ConnectorRuntimeDecisionState,
  result: ConnectorRuntimeExecutionResult,
  decision: ConnectorRuntimeConfirmationDecision
): ConnectorRuntimeDecisionAttempt | null {
  if (
    result.state !== 'requires_confirmation' ||
    !validDecision(decision) ||
    !validConnectorRuntimeRouteReceipt(result.confirmation.route_receipt)
  ) {
    return null;
  }

  const routeReceipt = result.confirmation.route_receipt;
  if (
    (current.status === 'submitting' ||
      current.status === 'resolved' ||
      current.status === 'unknown') &&
    current.routeReceipt === routeReceipt
  ) {
    return null;
  }

  return {
    state: { status: 'submitting', routeReceipt, decision },
    command: { route_receipt: routeReceipt, decision },
  };
}

export function resolveConnectorRuntimeDecision(
  current: ConnectorRuntimeDecisionState,
  command: ConnectorRuntimeConfirmationCommand,
  result: ConnectorRuntimeDecisionResult
): ConnectorRuntimeDecisionState {
  if (
    current.status !== 'submitting' ||
    current.routeReceipt !== command.route_receipt ||
    current.decision !== command.decision ||
    result.route_receipt !== command.route_receipt
  ) {
    return current;
  }
  return {
    status: 'resolved',
    routeReceipt: command.route_receipt,
    decision: command.decision,
    outcome: result.state,
  };
}

export function markConnectorRuntimeDecisionUnknown(
  current: ConnectorRuntimeDecisionState,
  command: ConnectorRuntimeConfirmationCommand
): ConnectorRuntimeDecisionState {
  if (
    current.status !== 'submitting' ||
    current.routeReceipt !== command.route_receipt ||
    current.decision !== command.decision
  ) {
    return current;
  }
  return {
    status: 'unknown',
    routeReceipt: command.route_receipt,
    decision: command.decision,
  };
}

export function isConnectorRuntimeDecisionAvailable(
  current: ConnectorRuntimeDecisionState,
  result: ConnectorRuntimeExecutionResult
): boolean {
  if (
    result.state !== 'requires_confirmation' ||
    !validConnectorRuntimeRouteReceipt(result.confirmation.route_receipt)
  ) {
    return false;
  }
  return !(
    (current.status === 'submitting' ||
      current.status === 'resolved' ||
      current.status === 'unknown') &&
    current.routeReceipt === result.confirmation.route_receipt
  );
}

function recordOf(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

function hasOnlyKeys(record: Record<string, unknown>, allowed: readonly string[]): boolean {
  const allowedKeys = new Set(allowed);
  return Object.keys(record).every(key => allowedKeys.has(key));
}

function safeDisplayString(value: unknown, maxLength = 256): value is string {
  return (
    typeof value === 'string' &&
    value.length > 0 &&
    value.length <= maxLength &&
    value === value.trim() &&
    !value.includes('\u0000') &&
    !value.includes('\r')
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
    const containsControlCharacter =
      typeof text === 'string' &&
      Array.from(text).some(character => {
        const code = character.charCodeAt(0);
        return code < 32 || (code >= 127 && code <= 159);
      });
    if (
      normalized.length > 35 ||
      !/^[A-Za-z]{2,8}(?:-[A-Za-z0-9]{1,8})*$/.test(normalized) ||
      seen.has(equivalent) ||
      typeof text !== 'string' ||
      containsControlCharacter ||
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

function parseRuntimeConfirmation(value: unknown): ConnectorRuntimeConfirmation | null {
  const record = recordOf(value);
  if (
    !record ||
    !hasOnlyKeys(record, [
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
    ]) ||
    !safeDisplayString(record.route_receipt, 128) ||
    !validConnectorRuntimeRouteReceipt(record.route_receipt) ||
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
  const consequence = recordOf(record.consequence);
  if (
    !target ||
    !hasOnlyKeys(target, ['type', 'id', 'binding_instance_id']) ||
    !['chat', 'agent', 'workflow'].includes(String(target.type)) ||
    !safeDisplayString(target.id) ||
    !optionalSafeDisplayString(target.binding_instance_id) ||
    !consequence ||
    !hasOnlyKeys(consequence, [
      'external_effect',
      'description',
      'description_i18n',
      'data_egress',
      'external_destination',
    ]) ||
    !safeDisplayString(consequence.external_effect) ||
    !optionalSafeDisplayString(consequence.description, 1024) ||
    !optionalSafeLocalizedText(consequence.description_i18n) ||
    typeof consequence.data_egress !== 'boolean' ||
    !optionalSafeDisplayString(consequence.external_destination) ||
    !['high', 'unclassified'].includes(String(record.risk_classification))
  ) {
    return null;
  }

  return {
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
      type: target.type as 'chat' | 'agent' | 'workflow',
      id: target.id,
      ...(target.binding_instance_id ? { binding_instance_id: target.binding_instance_id } : {}),
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
    risk_classification: record.risk_classification as 'high' | 'unclassified',
  };
}

/**
 * Parses only the credential-free Connector runtime envelope. Extra fields are
 * rejected so arbitrary provider output can never be mistaken for consent UI.
 */
export function parseConnectorRuntimeExecutionResult(
  value: unknown
): ConnectorRuntimeExecutionResult | null {
  const record = recordOf(value);
  if (!record || !hasOnlyKeys(record, ['state', 'decision_id', 'error_code', 'confirmation'])) {
    return null;
  }
  if (!optionalSafeDisplayString(record.decision_id)) return null;
  if (!optionalSafeDisplayString(record.error_code, 128)) return null;

  if (record.state === 'requires_confirmation') {
    if (record.error_code !== undefined) return null;
    const confirmation = parseRuntimeConfirmation(record.confirmation);
    if (!confirmation) return null;
    return {
      state: 'requires_confirmation',
      ...(record.decision_id ? { decision_id: record.decision_id } : {}),
      confirmation,
    };
  }

  if (
    (record.state === 'blocked' || record.state === 'cancelled' || record.state === 'completed') &&
    record.confirmation === undefined &&
    record.error_code === undefined
  ) {
    return {
      state: record.state,
      ...(record.decision_id ? { decision_id: record.decision_id } : {}),
    };
  }

  if (
    (record.state === 'failed' || record.state === 'unknown') &&
    record.confirmation === undefined
  ) {
    return {
      state: record.state,
      ...(record.decision_id ? { decision_id: record.decision_id } : {}),
      ...(record.error_code ? { error_code: record.error_code } : {}),
    };
  }
  return null;
}
