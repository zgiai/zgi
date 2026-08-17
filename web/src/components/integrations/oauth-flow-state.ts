import type {
  IntegrationOAuthFlow,
  IntegrationOAuthFlowStatus,
} from '@/services/types/integration';

export type IntegrationOAuthUIStatus =
  | 'idle'
  | 'starting'
  | 'waiting'
  | 'exchanging'
  | 'succeeded'
  | 'failed'
  | 'expired'
  | 'cancelled'
  | 'popup_blocked'
  | 'popup_closed'
  | 'timed_out';

export interface IntegrationOAuthUIState {
  status: IntegrationOAuthUIStatus;
  flow: IntegrationOAuthFlow | null;
  popupBlocked: boolean;
  popupClosed: boolean;
}

export type IntegrationOAuthUIEvent =
  | { type: 'start' }
  | { type: 'flow_created'; flow: IntegrationOAuthFlow; popupBlocked: boolean }
  | { type: 'flow_updated'; flow: IntegrationOAuthFlow }
  | { type: 'cancelled' }
  | { type: 'popup_closed' }
  | { type: 'popup_reopened' }
  | { type: 'timeout' }
  | { type: 'reset' }
  | { type: 'request_failed' };

export const initialIntegrationOAuthUIState: IntegrationOAuthUIState = {
  status: 'idle',
  flow: null,
  popupBlocked: false,
  popupClosed: false,
};

const terminalFlowStatuses = new Set<IntegrationOAuthFlowStatus>([
  'succeeded',
  'failed',
  'expired',
  'cancelled',
]);

export function isIntegrationOAuthFlowTerminal(
  status: IntegrationOAuthFlowStatus | undefined
): boolean {
  return Boolean(status && terminalFlowStatuses.has(status));
}

export function oauthFlowStatusToUIStatus(
  status: IntegrationOAuthFlowStatus
): IntegrationOAuthUIStatus {
  switch (status) {
    case 'pending':
    case 'authorizing':
      return 'waiting';
    case 'exchanging':
      return 'exchanging';
    case 'succeeded':
    case 'failed':
    case 'expired':
    case 'cancelled':
      return status;
  }
}

export function integrationOAuthUIReducer(
  state: IntegrationOAuthUIState,
  event: IntegrationOAuthUIEvent
): IntegrationOAuthUIState {
  switch (event.type) {
    case 'start':
      return {
        status: 'starting',
        flow: null,
        popupBlocked: false,
        popupClosed: false,
      };
    case 'flow_created':
      return {
        status: event.popupBlocked ? 'popup_blocked' : oauthFlowStatusToUIStatus(event.flow.status),
        flow: event.flow,
        popupBlocked: event.popupBlocked,
        popupClosed: false,
      };
    case 'flow_updated':
      return {
        ...state,
        status: oauthFlowStatusToUIStatus(event.flow.status),
        flow: event.flow,
        ...(isIntegrationOAuthFlowTerminal(event.flow.status)
          ? { popupBlocked: false, popupClosed: false }
          : {}),
      };
    case 'cancelled':
      return {
        ...state,
        status: 'cancelled',
        flow: state.flow ? { ...state.flow, status: 'cancelled' } : null,
      };
    case 'popup_closed':
      if (state.flow && isIntegrationOAuthFlowTerminal(state.flow.status)) return state;
      return {
        ...state,
        status: 'popup_closed',
        popupClosed: true,
      };
    case 'popup_reopened':
      return {
        ...state,
        status: state.flow ? oauthFlowStatusToUIStatus(state.flow.status) : 'waiting',
        popupBlocked: false,
        popupClosed: false,
      };
    case 'timeout':
      if (state.flow && isIntegrationOAuthFlowTerminal(state.flow.status)) return state;
      return { ...state, status: 'timed_out' };
    case 'request_failed':
      return { ...state, status: 'failed' };
    case 'reset':
      return initialIntegrationOAuthUIState;
  }
}

export function normalizeOAuthPollInterval(value: number | undefined): number {
  if (!Number.isFinite(value)) return 1_500;
  return Math.min(5_000, Math.max(750, Math.round(value ?? 1_500)));
}

export function oauthFlowExpiryDelay(expiresAt: string, now = Date.now()): number {
  const expiry = Date.parse(expiresAt);
  if (!Number.isFinite(expiry)) return 5 * 60_000;
  return Math.max(0, Math.min(expiry - now, 10 * 60_000));
}
