import type { AIChatSkillInvocation } from '@/services/types/aichat';

function governanceString(value: unknown): string | undefined {
  if (typeof value === 'string' && value.trim()) return value.trim();
  if (typeof value === 'number' && Number.isFinite(value)) return String(value);
  return undefined;
}

function governanceRecord(value: unknown): Record<string, unknown> | undefined {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
  return value as Record<string, unknown>;
}

function governanceRecordString(value: unknown, key: string): string | undefined {
  return governanceString(governanceRecord(value)?.[key]);
}

export function governanceStatusFromInvocation(invocation: AIChatSkillInvocation): string {
  return String(invocation.governance?.status ?? invocation.status ?? '').toLowerCase();
}

function governanceApprovalStatusFromInvocation(invocation: AIChatSkillInvocation): string {
  return String(
    invocation.approval_status ??
      invocation.governance?.approval_status ??
      invocation.governance?.approval_result?.approval_status ??
      ''
  ).toLowerCase();
}

export function governanceCorrelationIdFromInvocation(
  invocation: AIChatSkillInvocation
): string | undefined {
  const modelFeedback = governanceRecord(invocation.governance?.model_feedback);
  const matchedGrant = governanceRecord(invocation.governance?.matched_grant);
  const auditMatchedGrant = governanceRecord(invocation.asset_operation_audit?.matched_grant);
  const governanceAuditMatchedGrant = governanceRecord(
    invocation.governance?.asset_operation_audit?.matched_grant
  );
  const candidates = [
    governanceRecordString(invocation, 'correlation_id'),
    governanceRecordString(invocation, 'approved_by_correlation_id'),
    invocation.governance?.correlation_id,
    invocation.governance?.approved_by_correlation_id,
    invocation.governance?.approval_event?.correlation_id,
    invocation.governance?.approval_event?.approved_by_correlation_id,
    invocation.asset_operation_audit?.correlation_id,
    invocation.asset_operation_audit?.approved_by_correlation_id,
    invocation.governance?.asset_operation_audit?.correlation_id,
    invocation.governance?.asset_operation_audit?.approved_by_correlation_id,
    governanceRecordString(matchedGrant, 'approval_correlation_id'),
    governanceRecordString(auditMatchedGrant, 'approval_correlation_id'),
    governanceRecordString(governanceAuditMatchedGrant, 'approval_correlation_id'),
    governanceRecordString(modelFeedback?.asset_operation_audit, 'correlation_id'),
    governanceRecordString(modelFeedback?.asset_operation_audit, 'approved_by_correlation_id'),
  ];
  for (const correlationId of candidates) {
    const normalized = governanceString(correlationId);
    if (normalized) return normalized;
  }
  return undefined;
}

export function isPendingToolGovernanceInvocation(invocation: AIChatSkillInvocation): boolean {
  const approvalStatus = governanceApprovalStatusFromInvocation(invocation);
  if (approvalStatus === 'approved' || approvalStatus === 'rejected') return false;
  if (invocation.governance?.requires_approval === false) return false;
  return (
    Boolean(invocation.tool_name) &&
    (governanceStatusFromInvocation(invocation) === 'needs_approval' ||
      invocation.governance?.requires_approval === true)
  );
}
