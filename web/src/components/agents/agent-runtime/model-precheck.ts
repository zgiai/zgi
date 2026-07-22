import type {
  AIChatModelPrecheckResult,
  AIChatModelPrecheckWarning,
} from '@/services/types/aichat';

export function visibleAgentModelPrecheckWarnings(
  result?: AIChatModelPrecheckResult
): AIChatModelPrecheckWarning[] {
  return result?.status === 'warning' ? result.warnings : [];
}

export async function allowSendAfterAgentModelPrecheck(
  refresh: () => Promise<unknown>
): Promise<true> {
  try {
    await refresh();
  } catch {
    // Precheck is advisory. Its availability must never decide whether a message can be sent.
  }
  return true;
}
