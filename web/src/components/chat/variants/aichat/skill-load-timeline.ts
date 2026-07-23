import type { AIChatSkillInvocation } from '@/services/types/aichat';

function invocationRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

function invocationString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function skillLoadAttemptOutcome(invocation: AIChatSkillInvocation): string {
  const argumentsRecord = invocationRecord(invocation.arguments);
  const resultRecord = invocationRecord(invocation.result);
  return (
    invocationString(argumentsRecord.outcome) || invocationString(resultRecord.outcome)
  ).toLowerCase();
}

function skillLoadAttemptAccessStatus(invocation: AIChatSkillInvocation): string {
  const argumentsRecord = invocationRecord(invocation.arguments);
  const resultRecord = invocationRecord(invocation.result);
  return (
    invocationString(argumentsRecord.access_status) ||
    invocationString(resultRecord.access_status)
  ).toLowerCase();
}

export function isUserRelevantSkillLoadFailure(invocation: AIChatSkillInvocation): boolean {
  const status = String(invocation.status ?? '')
    .trim()
    .toLowerCase();
  if (status !== 'error' && status !== 'blocked' && status !== 'denied') return false;
  if (invocation.kind === 'skill_load') return true;
  if (invocation.kind !== 'skill_load_attempt') return false;

  const outcome = skillLoadAttemptOutcome(invocation);
  if (
    outcome === 'not_exposed_current_surface' ||
    outcome === 'page_mismatch' ||
    outcome === 'version_changed' ||
    outcome === 'restore_budget_exceeded'
  ) {
    return false;
  }

  // Diagnostics written before surface mismatch had a dedicated outcome used
  // policy_denied with an unavailable access status. A real authorization
  // denial records denied or verification_failed instead.
  if (outcome === 'policy_denied') {
    const accessStatus = skillLoadAttemptAccessStatus(invocation);
    if (accessStatus === 'unavailable' || accessStatus === 'not_applicable') return false;
  }
  return true;
}

export function isRoutineSkillLoadInvocation(invocation: AIChatSkillInvocation): boolean {
  if (invocation.kind !== 'skill_load' && invocation.kind !== 'skill_load_attempt') return false;
  return !isUserRelevantSkillLoadFailure(invocation);
}
