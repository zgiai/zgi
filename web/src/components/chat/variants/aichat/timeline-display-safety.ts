import { isConnectorOperationInvocation } from '@/components/connectors/runtime/connector-operation-presentation';
export interface TimelineSkillInvocationLike {
  kind?: string;
  status?: string;
  skill_id?: string;
  tool_name?: string;
  runtime_source?: string;
  arguments?: unknown;
  result?: unknown;
  message?: string;
  error?: string;
  error_code?: string;
}

export interface TimelineSkillItemLike {
  id: string;
  type: string;
  invocation?: TimelineSkillInvocationLike;
}

const INTERNAL_DISPLAY_FIELD_KEYS = new Set([
  'connection_id',
  'connection_ids',
  'credential_id',
  'credential_ids',
  'workspace_id',
  'workspace_ids',
  'organization_id',
  'organization_ids',
  'tenant_id',
  'tenant_ids',
  'account_id',
  'account_ids',
  'conversation_id',
  'conversation_ids',
  'correlation_id',
  'correlation_ids',
  'approved_by_correlation_id',
  'runtime_id',
  'runtime_ids',
  'invocation_id',
  'invocation_ids',
  'answer_id',
  'answer_ids',
]);

const UUID_DISPLAY_PATTERN =
  /\b(?:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|[0-9a-f]{32})\b/gi;
const EXACT_UUID_DISPLAY_PATTERN =
  /^(?:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|[0-9a-f]{32})$/i;
const DEFAULT_REDACTED_DISPLAY_VALUE = '[hidden]';
const HIDDEN_REFERENCE_SENTINEL = '__zgi_hidden_reference__';
const HIDDEN_REFERENCE_PATTERN = /__zgi_hidden_reference__/gi;
const REDACTED_SENTINEL_PATTERN = /(?:__zgi_redacted__|\[redacted\])/gi;
const TRUNCATED_SENTINEL_PATTERN = /(?:__zgi_truncated__|\[truncated\])/gi;

const INVALID_ARGUMENTS_PATTERN =
  /(?:\binvalid[\s_-]+arguments?\b|\barguments?[\s_-]+invalid\b|\barguments?[\s_-]+validation[\s_-]+failed\b|\bfailed[\s_-]+to[\s_-]+validate[\s_-]+arguments?\b)/i;

function normalizeDisplayFieldKey(key: string): string {
  return key
    .replace(/([a-z0-9])([A-Z])/g, '$1_$2')
    .replace(/[-\s]+/g, '_')
    .toLowerCase();
}

export function isInternalTimelineDisplayFieldKey(key: string): boolean {
  const normalized = normalizeDisplayFieldKey(key);
  return INTERNAL_DISPLAY_FIELD_KEYS.has(normalized);
}

export function looksLikeOpaqueTimelineIdentifier(value: string): boolean {
  const normalized = value.trim();
  return Boolean(normalized && EXACT_UUID_DISPLAY_PATTERN.test(normalized));
}

export function sanitizeTimelineDisplayString(
  value: string,
  redactedDisplayValue = DEFAULT_REDACTED_DISPLAY_VALUE,
  truncatedDisplayValue = redactedDisplayValue
): string | null {
  const trimmed = value.trim();
  if (!trimmed) return null;
  if (
    looksLikeOpaqueTimelineIdentifier(trimmed) ||
    trimmed.toLowerCase() === HIDDEN_REFERENCE_SENTINEL
  ) {
    return redactedDisplayValue;
  }

  const sanitized = trimmed
    .replace(HIDDEN_REFERENCE_PATTERN, redactedDisplayValue)
    .replace(REDACTED_SENTINEL_PATTERN, redactedDisplayValue)
    .replace(TRUNCATED_SENTINEL_PATTERN, truncatedDisplayValue)
    .replace(UUID_DISPLAY_PATTERN, redactedDisplayValue)
    .replace(/\s{2,}/g, ' ')
    .trim();
  if (!sanitized) return null;
  return sanitized;
}

export function sanitizeTimelineDisplayPayload(
  value: unknown,
  redactedDisplayValue = DEFAULT_REDACTED_DISPLAY_VALUE,
  truncatedDisplayValue = redactedDisplayValue
): unknown {
  if (value === undefined || value === null || value === '') return null;
  if (typeof value === 'string') {
    return sanitizeTimelineDisplayString(value, redactedDisplayValue, truncatedDisplayValue);
  }
  if (typeof value === 'number' || typeof value === 'boolean') return value;
  if (Array.isArray(value)) {
    const items = value
      .map(item =>
        sanitizeTimelineDisplayPayload(item, redactedDisplayValue, truncatedDisplayValue)
      )
      .filter(item => item !== null && item !== undefined);
    return items.length > 0 ? items : null;
  }
  if (typeof value === 'object') {
    const entries = Object.entries(value as Record<string, unknown>).flatMap(([key, rawValue]) => {
      if (isInternalTimelineDisplayFieldKey(key)) return [];
      const sanitized = sanitizeTimelineDisplayPayload(
        rawValue,
        redactedDisplayValue,
        truncatedDisplayValue
      );
      return sanitized === null || sanitized === undefined ? [] : [[key, sanitized] as const];
    });
    return entries.length > 0 ? Object.fromEntries(entries) : null;
  }
  return String(value);
}

function isStructuralArgumentSummary(value: Record<string, unknown>): boolean {
  const type = typeof value.type === 'string' ? value.type.toLowerCase() : '';
  if (!['string', 'array', 'object', 'number', 'boolean'].includes(type)) return false;
  return Object.keys(value).every(key => ['type', 'length', 'keys'].includes(key));
}

function structuralArgumentSummary(value: unknown): unknown {
  if (value === null || value === undefined) return null;
  if (typeof value === 'string') return { type: 'string', length: value.length };
  if (typeof value === 'number') return { type: 'number' };
  if (typeof value === 'boolean') return { type: 'boolean' };
  if (Array.isArray(value)) return { type: 'array', length: value.length };
  if (typeof value !== 'object') return { type: typeof value };

  const record = value as Record<string, unknown>;
  if (isStructuralArgumentSummary(record)) return record;
  const entries = Object.entries(record).flatMap(([key, nested]) => {
    if (isInternalTimelineDisplayFieldKey(key)) return [];
    const summary = structuralArgumentSummary(nested);
    return summary === null || summary === undefined ? [] : [[key, summary] as const];
  });
  return entries.length > 0 ? Object.fromEntries(entries) : null;
}

export function summarizeTimelineArgumentsForDisplay(
  value: unknown,
  redactedDisplayValue = DEFAULT_REDACTED_DISPLAY_VALUE,
  truncatedDisplayValue = redactedDisplayValue
): unknown {
  return structuralArgumentSummary(
    sanitizeTimelineDisplayPayload(value, redactedDisplayValue, truncatedDisplayValue)
  );
}

export function formatTimelineDebugValue(
  value: unknown,
  redactedDisplayValue = DEFAULT_REDACTED_DISPLAY_VALUE,
  truncatedDisplayValue = redactedDisplayValue
): string | null {
  const sanitized = sanitizeTimelineDisplayPayload(
    value,
    redactedDisplayValue,
    truncatedDisplayValue
  );
  if (sanitized === undefined || sanitized === null || sanitized === '') return null;
  if (typeof sanitized === 'string') return sanitized;
  if (typeof sanitized === 'number' || typeof sanitized === 'boolean') return String(sanitized);
  try {
    return JSON.stringify(sanitized);
  } catch {
    return String(sanitized);
  }
}

function normalizedToolIdentity(invocation: TimelineSkillInvocationLike): string | null {
  const skillId = invocation.skill_id?.trim().toLowerCase() ?? '';
  const toolName = invocation.tool_name?.trim().toLowerCase() ?? '';
  return skillId && toolName ? `${skillId}\u0000${toolName}` : null;
}

function safeDiagnosticIdentifier(value: unknown): string | null {
  if (typeof value !== 'string') return null;
  const normalized = value.trim().toLowerCase();
  if (
    !normalized ||
    normalized.length > 160 ||
    looksLikeOpaqueTimelineIdentifier(normalized) ||
    !/^[a-z0-9][a-z0-9._:/-]*$/.test(normalized)
  ) {
    return null;
  }
  return normalized;
}

function objectStringField(value: unknown, key: string): string | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null;
  return safeDiagnosticIdentifier((value as Record<string, unknown>)[key]);
}

function isSuccessfulToolInvocation(invocation: TimelineSkillInvocationLike): boolean {
  const status = invocation.status?.trim().toLowerCase() ?? '';
  return ['success', 'succeeded', 'completed', 'complete', 'ok'].includes(status);
}

function isRecoverableInvalidArgumentsInvocation(invocation: TimelineSkillInvocationLike): boolean {
  const status = invocation.status?.trim().toLowerCase() ?? '';
  if (!['error', 'failed', 'blocked'].includes(status)) return false;
  const reasonCode = objectStringField(invocation.arguments, 'reason_code');
  if (reasonCode === 'action_arguments_schema_mismatch') return true;
  const message = [invocation.message, invocation.error].filter(Boolean).join(' ');
  return INVALID_ARGUMENTS_PATTERN.test(message);
}

export function recoveredInvalidArgumentTimelineItemIds(
  timeline: readonly TimelineSkillItemLike[]
): ReadonlySet<string> {
  const recovered = new Set<string>();
  const laterSuccessfulTools = new Set<string>();
  for (let index = timeline.length - 1; index >= 0; index -= 1) {
    const item = timeline[index];
    if (item.type !== 'skill_event' || !item.invocation) continue;
    // Connector results remain visible independently, including different
    // connections or inputs sharing the same tool name.
    if (isConnectorOperationInvocation(item.invocation)) continue;
    const identity = normalizedToolIdentity(item.invocation);
    if (!identity) continue;
    if (isSuccessfulToolInvocation(item.invocation)) {
      laterSuccessfulTools.add(identity);
      continue;
    }
    if (
      laterSuccessfulTools.has(identity) &&
      isRecoverableInvalidArgumentsInvocation(item.invocation)
    )
      {recovered.add(item.id);}
  }
  return recovered;
}
