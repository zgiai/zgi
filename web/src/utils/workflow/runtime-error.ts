export type WorkflowRuntimeErrorKind =
  | 'model_service_timeout'
  | 'model_service_unavailable'
  | 'model_invocation_failed'
  | 'planning_output_truncated'
  | 'agent_output_truncated'
  | 'server_unavailable';

export function classifyWorkflowRuntimeError(
  message: string | null | undefined
): WorkflowRuntimeErrorKind | undefined {
  const normalized = message?.trim().toLowerCase();
  if (!normalized) return undefined;

  if (
    normalized.includes('agent_output_truncated') ||
    normalized.includes('agent output truncated')
  ) {
    return 'agent_output_truncated';
  }

  if (
    normalized.includes('planning_output_truncated') ||
    normalized.includes('finish_reason=length') ||
    normalized.includes('finish_reason=max_tokens')
  ) {
    return 'planning_output_truncated';
  }

  if (
    normalized.includes('internal server error') ||
    normalized.includes('unknown error') ||
    normalized.includes('status code 500')
  ) {
    return 'server_unavailable';
  }

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
    normalized.includes('unexpected eof') ||
    normalized.includes('all providers failed') ||
    normalized.includes('upstream service error') ||
    normalized.includes('channel unavailable')
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
