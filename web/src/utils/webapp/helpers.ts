import type { WebAppWorkflowConfig } from '@/services/types/webapp';

/**
 * Detect webapp mode from config to decide routing between /chat and /run
 * - WORKFLOW type → 'run'
 * - CONVERSATIONAL_WORKFLOW type → 'chat'
 * - Default fallback → 'chat' (conservative)
 */
export function detectWebappMode(config: WebAppWorkflowConfig | null | undefined): 'chat' | 'run' {
  if (!config) return 'chat';

  // Decide by workflow config type
  const t = config.config?.type?.toUpperCase?.() ?? '';
  if (t === 'WORKFLOW') return 'run';
  if (t === 'CONVERSATIONAL_WORKFLOW') return 'chat';

  // Conservative default: chat
  return 'chat';
}
