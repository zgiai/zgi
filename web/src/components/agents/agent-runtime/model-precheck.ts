import type {
  AIChatModelPrecheckResult,
  AIChatModelPrecheckWarning,
} from '@/services/types/aichat';

export function visibleAgentModelPrecheckWarnings(
  result?: AIChatModelPrecheckResult
): AIChatModelPrecheckWarning[] {
  return result?.status === 'warning' ? result.warnings : [];
}

export function allowSendAfterAgentModelPrecheck(refresh: () => Promise<unknown>): true {
  try {
    void refresh().catch(() => {
      // Precheck is advisory. Its availability must never decide whether a message can be sent.
    });
  } catch {
    // Precheck is advisory. Its availability must never decide whether a message can be sent.
  }
  return true;
}
