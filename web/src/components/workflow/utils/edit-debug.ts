const WORKFLOW_EDIT_DEBUG_STORAGE_KEY = 'workflow-edit-debug';

/**
 * @util Check whether workflow edit diagnostics are enabled for the current browser session.
 */
export function isWorkflowEditDebugEnabled(): boolean {
  if (process.env.NODE_ENV === 'production' || typeof window === 'undefined') return false;

  try {
    return window.localStorage.getItem(WORKFLOW_EDIT_DEBUG_STORAGE_KEY) === '1';
  } catch {
    return false;
  }
}

/**
 * @util Write workflow edit diagnostics behind an explicit localStorage flag.
 */
export function logWorkflowEditDebug(
  label: string | undefined,
  message: string,
  data?: Record<string, unknown>
): void {
  if (!label || !isWorkflowEditDebugEnabled()) return;
  // eslint-disable-next-line no-console
  console.debug(`[workflow-edit:${label}] ${message}`, data ?? {});
}
