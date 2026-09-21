export type ConnectorDisplayStatus =
  | 'successful'
  | 'partial'
  | 'failed'
  | 'unknown'
  | 'cancelled'
  | 'unavailable'
  | 'pending';

export interface ConnectorOperationPresentation {
  /** Derived from the ordinary invocation evidence; never a wire authority. */
  execution_status?: 'not_submitted' | 'in_progress';
  schema_version: 1;
  source_kind: 'native' | 'history';
  invocation_id: string;
  display_status: ConnectorDisplayStatus;
  title?: string;
  summary?: string;
  redacted: boolean;
  truncated: boolean;
}

function record(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

export function isConnectorOperationInvocation(value: unknown): boolean {
  const invocation = record(value);
  return (
    invocation?.runtime_source === 'connector' || invocation?.runtime_source === 'connector_history'
  );
}

function safeText(value: unknown, limit: number): value is string {
  return (
    typeof value === 'string' &&
    value.length > 0 &&
    value.length <= limit &&
    Array.from(value).every(character => {
      const code = character.charCodeAt(0);
      return code === 9 || code === 10 || (code >= 32 && code !== 127);
    })
  );
}

function validUUID(value: unknown): boolean {
  return (
    typeof value === 'string' &&
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/u.test(value) &&
    value !== '00000000-0000-0000-0000-000000000000'
  );
}

function validTarget(value: unknown): boolean {
  const target = record(value);
  if (
    !target ||
    !['chat', 'agent', 'workflow'].includes(String(target.type)) ||
    !safeText(target.id, 256)
  ) {
    return false;
  }
  const scope = target.scope || (target.type === 'chat' ? 'live' : 'draft');
  if (target.type === 'chat') {
    if (scope !== 'live' || target.version_id) return false;
  } else if (scope === 'published') {
    if (!validUUID(target.version_id)) return false;
  } else if (scope !== 'draft' || target.version_id) return false;
  return target.type === 'workflow'
    ? safeText(target.binding_instance_id, 256)
    : !target.binding_instance_id;
}

/** Reads only a host-projected, versioned display object. It never creates a
 * confirmation command, interprets provider output, or falls back to raw JSON. */
export function parseConnectorOperationPresentation(
  value: unknown
): ConnectorOperationPresentation | null {
  if (!isConnectorOperationInvocation(value)) return null;
  const invocation = record(value);
  if (!invocation || invocation.kind !== 'tool_call') return null;
  const result = record(invocation.result);
  const presentation = record(result?.connector_presentation);
  if (!presentation) return null;
  const source = invocation.runtime_source === 'connector' ? 'native' : 'history';
  const allowed = new Set([
    'schema_version',
    'source_kind',
    'invocation_id',
    'connector_id',
    'operation_id',
    'display_status',
    'title',
    'summary',
    'redacted',
    'truncated',
  ]);
  if (
    Object.keys(presentation).some(key => !allowed.has(key)) ||
    presentation.schema_version !== 1 ||
    presentation.source_kind !== source ||
    !safeText(presentation.invocation_id, 256) ||
    !['successful', 'partial', 'failed', 'unknown', 'cancelled', 'unavailable', 'pending'].includes(
      String(presentation.display_status)
    ) ||
    typeof presentation.redacted !== 'boolean' ||
    typeof presentation.truncated !== 'boolean' ||
    (presentation.title !== undefined && !safeText(presentation.title, 120)) ||
    (presentation.summary !== undefined && !safeText(presentation.summary, 800))
  ) {
    return null;
  }
  if (source === 'native') {
    const operation = record(result?.operation);
    if (
      !operation ||
      !validUUID(operation.binding_id) ||
      !validUUID(operation.connection_id) ||
      !validTarget(operation.target) ||
      !/^[a-z0-9][a-z0-9._-]{0,127}$/u.test(String(operation.connector_id ?? '')) ||
      !/^[a-z0-9][a-z0-9._-]{0,127}$/u.test(String(operation.operation_id ?? '')) ||
      operation.logical_operation_id !== presentation.invocation_id ||
      operation.connector_id !== presentation.connector_id ||
      operation.operation_id !== presentation.operation_id
    ) {
      return null;
    }
    if (result?.state === 'rejected' || result?.state === 'unresolved') {
      const evidence = record(result.execution_evidence);
      if (
        !evidence ||
        evidence.version !== 1 ||
        Object.keys(evidence).some(key => !['version', 'disposition'].includes(key)) ||
        (result.outcome_id != null &&
          result.outcome_id !== '00000000-0000-0000-0000-000000000000') ||
        (result.outcome_status != null && result.outcome_status !== '') ||
        result.confirmation != null ||
        (result.state === 'rejected'
          ? evidence.disposition !== 'not_submitted' ||
            presentation.display_status !== 'unavailable'
          : !['in_progress', 'unknown'].includes(String(evidence.disposition)) ||
            presentation.display_status !== 'unknown')
      ) {
        return null;
      }
    }
    // The durable outcome, rather than a generic tool-call status, proves success.
    if (
      presentation.display_status === 'successful' &&
      (result?.state !== 'completed' ||
        result.outcome_status !== 'successful' ||
        !validUUID(result.outcome_id))
    ) {
      return null;
    }
  } else if (
    presentation.display_status === 'pending' ||
    presentation.invocation_id !== (invocation.invocation_id ?? invocation.runtime_id)
  ) {
    return null;
  }
  return {
    schema_version: 1,
    source_kind: source,
    invocation_id: presentation.invocation_id,
    display_status: presentation.display_status as ConnectorDisplayStatus,
    ...(source === 'native' && result?.state === 'rejected'
      ? { execution_status: 'not_submitted' as const }
      : source === 'native' &&
          result?.state === 'unresolved' &&
          record(result.execution_evidence)?.disposition === 'in_progress'
        ? { execution_status: 'in_progress' as const }
        : {}),
    ...(presentation.title !== undefined ? { title: presentation.title as string } : {}),
    ...(presentation.summary !== undefined ? { summary: presentation.summary as string } : {}),
    redacted: presentation.redacted,
    truncated: presentation.truncated,
  };
}

export function connectorOperationTone(
  value: unknown
): 'success' | 'warning' | 'error' | 'running' | null {
  if (!isConnectorOperationInvocation(value)) return null;
  const presentation = parseConnectorOperationPresentation(value);
  if (presentation?.execution_status === 'in_progress') return 'running';
  if (presentation?.execution_status === 'not_submitted') return 'error';
  switch (presentation?.display_status) {
    case 'successful':
      return 'success';
    case 'pending':
      return 'running';
    case 'failed':
      return 'error';
    default:
      return 'warning';
  }
}
