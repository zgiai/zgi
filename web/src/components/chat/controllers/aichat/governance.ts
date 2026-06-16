import type { AIChatSkillInvocation } from '@/services/types/aichat';

export function governanceStatusFromInvocation(invocation: AIChatSkillInvocation): string {
  return String(invocation.governance?.status ?? invocation.status ?? '').toLowerCase();
}

export function governanceCorrelationIdFromInvocation(
  invocation: AIChatSkillInvocation
): string | undefined {
  const candidates = [
    invocation.governance?.correlation_id,
    invocation.governance?.approval_event?.correlation_id,
  ];
  for (const correlationId of candidates) {
    if (typeof correlationId === 'string' && correlationId.trim()) {
      return correlationId.trim();
    }
  }
  return undefined;
}

export function isPendingToolGovernanceInvocation(invocation: AIChatSkillInvocation): boolean {
  return (
    Boolean(invocation.tool_name) &&
    (governanceStatusFromInvocation(invocation) === 'needs_approval' ||
      invocation.governance?.requires_approval === true)
  );
}
