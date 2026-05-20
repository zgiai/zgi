type PendingEditFlush = () => void;

const pendingEditFlushers = new Set<PendingEditFlush>();

/**
 * Register a synchronous flush callback for local workflow edits that may not
 * have reached the global store yet.
 */
export function registerWorkflowPendingEditFlush(flush: PendingEditFlush): () => void {
  pendingEditFlushers.add(flush);

  return () => {
    pendingEditFlushers.delete(flush);
  };
}

/**
 * Flush all registered local workflow edits before reading the graph for save.
 */
export function flushWorkflowPendingEdits(): void {
  Array.from(pendingEditFlushers).forEach(flush => {
    flush();
  });
}
