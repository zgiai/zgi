import type {
  ConnectorRuntimeConfirmationCommand,
  ConnectorRuntimeConfirmationResponse,
  ConnectorRuntimeDecisionResult,
} from '@/services/types/connector';

export type ConnectorRuntimeDecisionSender = (
  command: ConnectorRuntimeConfirmationCommand,
  scope?: string
) => Promise<ConnectorRuntimeConfirmationResponse>;

interface PendingDecision {
  decision: ConnectorRuntimeConfirmationCommand['decision'];
  promise: Promise<ConnectorRuntimeDecisionResult>;
}

function unknownResult(routeReceipt: string): ConnectorRuntimeDecisionResult {
  return { route_receipt: routeReceipt, state: 'transport_unconfirmed' };
}

function validRouteReceipt(value: string): boolean {
  return (
    value.length > 0 &&
    value.length <= 128 &&
    value === value.trim() &&
    !value.includes('\u0000') &&
    !value.includes('\r') &&
    !value.includes('\n')
  );
}

function safeErrorCode(value: unknown): value is string | undefined {
  return (
    value === undefined ||
    (typeof value === 'string' &&
      value.length > 0 &&
      value.length <= 128 &&
      value === value.trim() &&
      !value.includes('\u0000') &&
      !value.includes('\r') &&
      !value.includes('\n'))
  );
}

function normalizeDecisionResult(
  command: ConnectorRuntimeConfirmationCommand,
  value: unknown
): ConnectorRuntimeDecisionResult {
  const result =
    value && typeof value === 'object' && !Array.isArray(value)
      ? (value as Record<string, unknown>)
      : null;
  if (!result) {
    return unknownResult(command.route_receipt);
  }
  const state = result.state;
  const decisionID = result.decision_id;
  const errorCode = result.error_code;
  const rememberForSession = result.remember_for_session;
  if (
    Object.keys(result).some(
      key => !['state', 'decision_id', 'error_code', 'remember_for_session'].includes(key)
    ) ||
    !safeErrorCode(errorCode) ||
    !safeErrorCode(decisionID) ||
    (rememberForSession !== undefined && typeof rememberForSession !== 'boolean')
  ) {
    return unknownResult(command.route_receipt);
  }
  const common = {
    route_receipt: command.route_receipt,
    ...(decisionID ? { decision_id: decisionID } : {}),
    ...(rememberForSession ? { remember_for_session: true } : {}),
  };
  switch (state) {
    case 'cancelled':
      if (command.decision !== 'cancelled' || errorCode !== undefined) {
        return unknownResult(command.route_receipt);
      }
      return { ...common, state: 'cancelled' };
    case 'completed':
      if (command.decision !== 'confirmed' || errorCode !== undefined) {
        return unknownResult(command.route_receipt);
      }
      return { ...common, state: 'completed' };
    case 'failed':
      if (command.decision !== 'confirmed') {
        return unknownResult(command.route_receipt);
      }
      return { ...common, state: 'failed', ...(errorCode ? { error_code: errorCode } : {}) };
    case 'unknown':
      if (command.decision !== 'confirmed') {
        return unknownResult(command.route_receipt);
      }
      return { ...common, state: 'unknown', ...(errorCode ? { error_code: errorCode } : {}) };
    default:
      return unknownResult(command.route_receipt);
  }
}

/**
 * Process-local one-shot guard shared by Chat, Agent, and Workflow. Concurrent
 * duplicate calls share one Promise; settled and unknown receipts are consumed
 * and can never be submitted again by this client session.
 */
export class ConnectorRuntimeDecisionTransport {
  private readonly listeners = new Set<() => void>();
  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };
  private emit() {
    this.listeners.forEach(listener => listener());
  }
  getResult(receipt: string, scope = '') {
    return this.settledByReceipt.get(JSON.stringify([scope, receipt]))?.result;
  }
  getPhase(receipt: string, scope = '') {
    const key = JSON.stringify([scope, receipt]);
    return this.pendingByReceipt.has(key)
      ? 'submitting'
      : this.settledByReceipt.get(key)?.result.state;
  }
  acceptStatus(receipt: string, value: unknown, scope = '') {
    if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
    const { resolution, ...result } = value as Record<string, unknown>;
    if (resolution !== 'resolved') return false;
    const command: ConnectorRuntimeConfirmationCommand = {
      route_receipt: receipt,
      decision: result.state === 'cancelled' ? 'cancelled' : 'confirmed',
    };
    const normalized = normalizeDecisionResult(command, result);
    if (normalized.state === 'transport_unconfirmed') return false;
    const key = JSON.stringify([scope, receipt]);
    // A late status read cannot downgrade a known terminal or an in-flight submit.
    const previous = this.settledByReceipt.get(key);
    if (
      this.pendingByReceipt.has(key) ||
      (previous && previous.result.state !== 'transport_unconfirmed')
    ) {
      return true;
    }
    this.settledByReceipt.set(key, { decision: command.decision, result: normalized });
    this.emit();
    return true;
  }
  private readonly sender: ConnectorRuntimeDecisionSender;
  private readonly pendingByReceipt = new Map<string, PendingDecision>();
  private readonly settledByReceipt = new Map<
    string,
    {
      decision: ConnectorRuntimeConfirmationCommand['decision'];
      result: ConnectorRuntimeDecisionResult;
    }
  >();

  constructor(sender: ConnectorRuntimeDecisionSender) {
    this.sender = sender;
  }

  submit(
    command: ConnectorRuntimeConfirmationCommand,
    scope = ''
  ): Promise<ConnectorRuntimeDecisionResult> {
    if (
      !validRouteReceipt(command.route_receipt) ||
      (command.decision !== 'confirmed' && command.decision !== 'cancelled')
    ) {
      return Promise.resolve(unknownResult(command.route_receipt));
    }

    const key = JSON.stringify([scope, command.route_receipt]);
    const settled = this.settledByReceipt.get(key);
    if (settled) {
      return settled.decision === command.decision
        ? Promise.resolve(settled.result)
        : Promise.resolve(unknownResult(command.route_receipt));
    }

    const pending = this.pendingByReceipt.get(key);
    if (pending) {
      return pending.decision === command.decision
        ? pending.promise
        : Promise.resolve(unknownResult(command.route_receipt));
    }

    const promise = this.sender(command, scope)
      .then(result => normalizeDecisionResult(command, result))
      .catch(() => unknownResult(command.route_receipt))
      .then(result => {
        this.pendingByReceipt.delete(key);
        this.settledByReceipt.set(key, {
          decision: command.decision,
          result,
        });
        this.emit();
        return result;
      });
    this.pendingByReceipt.set(key, {
      decision: command.decision,
      promise,
    });
    this.emit();
    return promise;
  }
}

export const connectorRuntimeDecisionTransport = new ConnectorRuntimeDecisionTransport(
  async (command, scope) => {
    const { connectorService } = await import('@/services/connector.service');
    const { useAuthStore } = await import('@/store/auth-store');
    const { useOrganizationStore } = await import('@/store/organization-store');
    const currentScope = JSON.stringify([
      useOrganizationStore.getState().currentOrganization?.id ?? '',
      useAuthStore.getState().user?.id ?? '',
    ]);
    if (scope !== currentScope) throw new Error('Connector confirmation scope changed');
    return (await connectorService.decideRuntimeConfirmation(command)).data;
  }
);

export async function submitConnectorRuntimeDecision(
  command: ConnectorRuntimeConfirmationCommand,
  expectedScope?: string
): Promise<ConnectorRuntimeDecisionResult> {
  const { useAuthStore } = await import('@/store/auth-store');
  const { useOrganizationStore } = await import('@/store/organization-store');
  const scope = JSON.stringify([
    useOrganizationStore.getState().currentOrganization?.id ?? '',
    useAuthStore.getState().user?.id ?? '',
  ]);
  if (expectedScope !== undefined && expectedScope !== scope) {
    return unknownResult(command.route_receipt);
  }
  return connectorRuntimeDecisionTransport.submit(command, scope);
}
