// Centralized runtime colors for workflow run status
// Keep in sync with Tailwind palette to match theme

export const RUN_STATUS_COLORS = {
  // Colors sourced from theme CSS variables for full theming control
  running: 'var(--edge-running)',
  succeeded: 'var(--edge-succeeded)',
  failed: 'var(--edge-failed)',
  stopped: 'var(--edge-stopped)',
  paused: 'var(--edge-paused)',
} as const;

export type RunVisualStatus = keyof typeof RUN_STATUS_COLORS;
