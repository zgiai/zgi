import type { AIChatSkillInvocation } from '@/services/types/aichat';

export function governanceStatusFromInvocation(invocation: AIChatSkillInvocation): string {
  return String(invocation.governance?.status ?? invocation.status ?? '').toLowerCase();
}

export function governanceCorrelationIdFromInvocation(
  invocation: AIChatSkillInvocation
): string | undefined {
  const correlationId = invocation.governance?.correlation_id;
  if (typeof correlationId === 'string' && correlationId.trim()) {
    return correlationId.trim();
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
