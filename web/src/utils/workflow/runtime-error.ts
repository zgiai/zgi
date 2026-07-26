export type WorkflowRuntimeErrorKind =
  | 'model_service_timeout'
  | 'model_service_unavailable'
  | 'model_invocation_failed';

export function classifyWorkflowRuntimeError(
  message: string | null | undefined
): WorkflowRuntimeErrorKind | undefined {
  const normalized = message?.trim().toLowerCase();
  if (!normalized) return undefined;

  if (
    normalized.includes('tls handshake timeout') ||
    normalized.includes('timeout awaiting response headers') ||
    normalized.includes('context deadline exceeded') ||
    normalized.includes('client.timeout exceeded') ||
    normalized.includes('i/o timeout')
  ) {
    return 'model_service_timeout';
  }

  if (
    normalized.includes('connection reset by peer') ||
    normalized.includes('connection refused') ||
    normalized.includes('network is unreachable') ||
    normalized.includes('no such host') ||
    normalized.includes('unexpected eof')
  ) {
    return 'model_service_unavailable';
  }

  if (
    normalized.includes('failed to invoke llm') ||
    normalized.includes('provider stream call failed')
  ) {
    return 'model_invocation_failed';
  }

  return undefined;
}
