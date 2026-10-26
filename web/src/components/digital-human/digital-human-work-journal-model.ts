export interface WorkJournalScope {
  accountId: string;
  organizationId: string;
  workspaceId: string;
  employeeId: string;
}

export function workJournalQueryKey(scope: WorkJournalScope) {
  return [
    'digital-humans',
    'work-journal',
    scope.accountId,
    scope.organizationId,
    scope.workspaceId,
    scope.employeeId,
  ] as const;
}

export function sameWorkJournalScope(left: WorkJournalScope, right?: WorkJournalScope) {
  return Boolean(
    right &&
      left.accountId === right.accountId &&
      left.organizationId === right.organizationId &&
      left.workspaceId === right.workspaceId &&
      left.employeeId === right.employeeId
  );
}

export function appendWorkJournalPrompt(persona: string, prompt: string) {
  return persona ? `${persona}\n\n${prompt}` : prompt;
}
