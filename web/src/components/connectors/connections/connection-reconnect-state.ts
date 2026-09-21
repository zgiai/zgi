import type {
  ConnectorKeyField,
  KeySetupResult,
  PersonalConnection,
} from '@/services/types/connector';

export type ConnectionReconnectStatus =
  | 'idle'
  | 'confirmation_required'
  | 'verifying'
  | 'ready'
  | 'new_connection'
  | 'failed'
  | 'unknown';

export interface ConnectionReconnectState {
  status: ConnectionReconnectStatus;
  values: Record<string, string>;
  attemptId?: string;
  connection?: PersonalConnection;
  errorCode?: string;
  retryable?: boolean;
  confirmUnstableIdentity?: true;
}

export function createConnectionReconnectState(): ConnectionReconnectState {
  return { status: 'idle', values: {} };
}

function safeErrorCode(value: string | undefined): string {
  const normalized = value?.trim().toLowerCase() ?? '';
  return /^[a-z0-9][a-z0-9._-]{0,63}$/.test(normalized) ? normalized : 'request_failed';
}

export function updateConnectionReconnectValue(
  state: ConnectionReconnectState,
  name: string,
  value: string
): ConnectionReconnectState {
  if (
    state.status === 'verifying' ||
    state.status === 'ready' ||
    state.status === 'new_connection'
  ) {
    return state;
  }
  return {
    status: state.status === 'confirmation_required' ? 'confirmation_required' : 'idle',
    values: { ...state.values, [name]: value },
    confirmUnstableIdentity: state.confirmUnstableIdentity,
  };
}

export function canSubmitConnectionReconnect(
  state: ConnectionReconnectState,
  fields: readonly ConnectorKeyField[]
): boolean {
  if (state.status !== 'idle') return false;
  return fields.every(field => !field.required || Boolean(state.values[field.name]?.trim()));
}

export function beginConnectionReconnect(
  state: ConnectionReconnectState
): ConnectionReconnectState {
  if (state.status === 'verifying') return state;
  return {
    status: 'verifying',
    values: {},
    confirmUnstableIdentity: state.confirmUnstableIdentity,
  };
}

export function requireConnectionReconnectConfirmation(): ConnectionReconnectState {
  return { status: 'confirmation_required', values: {} };
}

export function confirmConnectionReconnectIdentity(): ConnectionReconnectState {
  return { status: 'idle', values: {}, confirmUnstableIdentity: true };
}

export function applyConnectionReconnectResult(
  state: ConnectionReconnectState,
  originalConnectionID: string,
  result: KeySetupResult
): ConnectionReconnectState {
  if (result.state === 'cancelled') return createConnectionReconnectState();
  if (result.state === 'verifying') {
    return { status: 'verifying', values: {}, attemptId: result.attempt_id };
  }
  if (result.state === 'failed') {
    return {
      status: 'failed',
      values: {},
      attemptId: result.attempt_id,
      errorCode: safeErrorCode(result.error_code),
      retryable: result.retryable,
    };
  }
  return {
    status: result.connection.id === originalConnectionID ? 'ready' : 'new_connection',
    values: {},
    attemptId: result.attempt_id,
    connection: result.connection,
  };
}

export function failConnectionReconnect(
  state: ConnectionReconnectState,
  errorCode: string | undefined,
  retryable: boolean,
  outcomeUnknown = false
): ConnectionReconnectState {
  return {
    status: outcomeUnknown ? 'unknown' : 'failed',
    values: {},
    attemptId: state.attemptId,
    errorCode: safeErrorCode(errorCode),
    retryable,
  };
}

export function resetConnectionReconnect(): ConnectionReconnectState {
  return createConnectionReconnectState();
}
