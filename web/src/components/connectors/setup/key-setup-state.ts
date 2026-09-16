import type {
  ConnectorKeyField,
  ConnectorTargetBinding,
  KeySetupResult,
  PersonalConnection,
} from '@/services/types/connector';

export type KeySetupViewStatus = 'idle' | 'verifying' | 'ready' | 'failed' | 'unknown';

export interface KeySetupState {
  status: KeySetupViewStatus;
  values: Record<string, string>;
  attemptId?: string;
  connection?: PersonalConnection;
  errorCode?: string;
  retryable?: boolean;
  bindingWarning?: string;
  attachment?: ConnectorTargetBinding;
}

export function createKeySetupState(): KeySetupState {
  return { status: 'idle', values: {} };
}

function safeErrorCode(value: string | undefined): string {
  const normalized = value?.trim().toLowerCase() ?? '';
  return /^[a-z0-9][a-z0-9._-]{0,63}$/.test(normalized) ? normalized : 'request_failed';
}

export function updateKeySetupValue(
  state: KeySetupState,
  name: string,
  value: string
): KeySetupState {
  if (state.status === 'verifying' || state.status === 'ready') return state;
  return {
    ...state,
    status: 'idle',
    values: { ...state.values, [name]: value },
    errorCode: undefined,
    retryable: undefined,
  };
}

export function canSubmitKeySetup(
  state: KeySetupState,
  fields: readonly ConnectorKeyField[]
): boolean {
  if (state.status !== 'idle' && state.status !== 'failed') return false;
  return fields.every(field => !field.required || Boolean(state.values[field.name]?.trim()));
}

export function beginKeySetup(state: KeySetupState): KeySetupState {
  if (state.status === 'verifying') return state;
  return { status: 'verifying', values: {} };
}

export function applyKeySetupResult(state: KeySetupState, result: KeySetupResult): KeySetupState {
  if (result.state === 'cancelled') return createKeySetupState();
  if (result.state === 'verifying') {
    return {
      status: 'verifying',
      values: {},
      attemptId: result.attempt_id,
      bindingWarning: result.binding_warning,
    };
  }
  if (result.state === 'ready') {
    return {
      status: 'ready',
      values: {},
      attemptId: result.attempt_id,
      connection: result.connection,
      bindingWarning: result.binding_warning,
      attachment: result.attachment,
    };
  }
  return {
    status: 'failed',
    values: {},
    attemptId: result.attempt_id,
    errorCode: safeErrorCode(result.error_code),
    retryable: result.retryable,
    bindingWarning: result.binding_warning,
  };
}

export function failKeySetup(
  state: KeySetupState,
  errorCode: string | undefined,
  retryable: boolean,
  outcomeUnknown = false
): KeySetupState {
  return {
    status: outcomeUnknown ? 'unknown' : 'failed',
    values: {},
    attemptId: state.attemptId,
    errorCode: safeErrorCode(errorCode),
    retryable,
  };
}

export function retryKeySetup(state: KeySetupState): KeySetupState {
  if (state.status !== 'failed' && state.status !== 'unknown') return state;
  return createKeySetupState();
}

export function resetKeySetup(): KeySetupState {
  return createKeySetupState();
}
