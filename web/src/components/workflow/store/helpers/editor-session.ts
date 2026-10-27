/**
 * Tracks the workflow editor on screen. The workflow store is a single global
 * store, so a save that finishes after the user left must not read or write
 * it: the store may already hold another editor's graph, or a freshly reopened
 * editor of the same workflow.
 *
 * Each mount gets its own session token, so reopening the same workflow does
 * not hand the store back to saves started by the previous mount.
 */
let activeSession: { agentId: string; token: number } | null = null;
let lastToken = 0;

/** Marks a new editor session for agentId. Returns a release function for cleanup. */
export function claimWorkflowEditor(agentId: string): () => void {
  lastToken += 1;
  const token = lastToken;
  activeSession = { agentId, token };
  return () => {
    if (activeSession?.token === token) activeSession = null;
  };
}

/** The open editor session for agentId, or null when agentId's editor is not open. */
export function currentWorkflowEditorSession(agentId: string): number | null {
  return activeSession?.agentId === agentId ? activeSession.token : null;
}

/** Whether the editor session that started a save is still the one on screen. */
export function isWorkflowEditorSessionActive(token: number | null | undefined): boolean {
  return token != null && activeSession?.token === token;
}
