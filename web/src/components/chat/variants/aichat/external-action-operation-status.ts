export type AIChatExternalActionOperationStatus =
  | 'completed'
  | 'already_completed'
  | 'succeeded'
  | 'executing'
  | 'failed_safe'
  | 'partially_succeeded'
  | 'failed'
  | 'outcome_unknown';

export type AIChatExternalActionOperationTone = 'running' | 'success' | 'warning' | 'error';

export interface AIChatExternalActionOperationState {
  status: AIChatExternalActionOperationStatus;
  tone: AIChatExternalActionOperationTone;
}

interface ExternalActionInvocationLike {
  skill_id?: string;
  tool_name?: string;
  result?: unknown;
}

const OPERATION_STATUS_TONES: Record<
  AIChatExternalActionOperationStatus,
  AIChatExternalActionOperationTone
> = {
  completed: 'success',
  already_completed: 'success',
  succeeded: 'success',
  executing: 'running',
  failed_safe: 'error',
  partially_succeeded: 'warning',
  failed: 'error',
  outcome_unknown: 'warning',
};

const SAFE_DIAGNOSTIC_CODE_PATTERN = /^[a-z0-9][a-z0-9._:-]{0,127}$/i;

function normalizedIdentifier(value: unknown): string {
  return typeof value === 'string' ? value.trim().toLowerCase() : '';
}

function unknownRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

export function isAIChatExternalExecuteActionInvocation(
  invocation: ExternalActionInvocationLike
): boolean {
  return (
    normalizedIdentifier(invocation.skill_id) === 'external-apps' &&
    normalizedIdentifier(invocation.tool_name) === 'execute_action'
  );
}

export function getAIChatExternalActionOperationState(
  invocation: ExternalActionInvocationLike
): AIChatExternalActionOperationState | null {
  if (!isAIChatExternalExecuteActionInvocation(invocation)) return null;
  const result = unknownRecord(invocation.result);
  const status = normalizedIdentifier(result?.operation_status);
  if (!Object.prototype.hasOwnProperty.call(OPERATION_STATUS_TONES, status)) return null;
  const normalizedStatus = status as AIChatExternalActionOperationStatus;
  return {
    status: normalizedStatus,
    tone: OPERATION_STATUS_TONES[normalizedStatus],
  };
}

/**
 * Diagnostic codes are display-safe identifiers only. Provider messages and
 * response bodies must never be rendered through this path.
 */
export function safeExternalActionDiagnosticCode(value: unknown): string | null {
  if (typeof value !== 'string') return null;
  const code = value.trim();
  return SAFE_DIAGNOSTIC_CODE_PATTERN.test(code) ? code : null;
}

export function isWeComNoValidRecipientCode(
  integrationId: unknown,
  providerErrorCode: unknown
): boolean {
  return (
    normalizedIdentifier(integrationId) === 'wecom' &&
    safeExternalActionDiagnosticCode(providerErrorCode) === '81013'
  );
}
