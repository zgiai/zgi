export const WORKFLOW_SAFE_NUMBER_MIN = Number.MIN_SAFE_INTEGER;
export const WORKFLOW_SAFE_NUMBER_MAX = Number.MAX_SAFE_INTEGER;

export function clampWorkflowSafeNumber(value: number): number {
  if (!Number.isFinite(value)) return 0;
  return Math.min(WORKFLOW_SAFE_NUMBER_MAX, Math.max(WORKFLOW_SAFE_NUMBER_MIN, value));
}
