/**
 * Tracks which workflow the editor on screen is showing. The workflow store is
 * a single global store, so a save that finishes after the user left (or opened
 * another workflow) must not read or write it: the store already holds another
 * editor's graph.
 */
let activeEditorAgentId: string | null = null;

/** Marks agentId as the open editor. Returns a release function for cleanup. */
export function claimWorkflowEditor(agentId: string): () => void {
  activeEditorAgentId = agentId;
  return () => {
    if (activeEditorAgentId === agentId) activeEditorAgentId = null;
  };
}

/** Whether the global workflow store still belongs to agentId's editor. */
export function isActiveWorkflowEditor(agentId: string): boolean {
  return activeEditorAgentId === agentId;
}
