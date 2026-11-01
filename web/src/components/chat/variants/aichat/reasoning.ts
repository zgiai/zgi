/**
 * ZGI-13 unified thinking switch for the AIChat workbench.
 *
 * The composer sends the OpenAI-standard `reasoning_effort` model parameter;
 * the console translates it into the selected channel's own spelling. Leaving
 * the switch untouched sends nothing, so the channel default applies.
 */

/** undefined = follow the model default; true = on; false = off. */
export type AIChatReasoningPreference = boolean | undefined;

export interface AIChatReasoningToggle {
  value: AIChatReasoningPreference;
  onChange: (enabled: boolean) => void;
}

export const AICHAT_REASONING_EFFORT_PARAM = 'reasoning_effort';
export const AICHAT_REASONING_EFFORT_ON = 'medium';
export const AICHAT_REASONING_EFFORT_OFF = 'none';

interface ReasoningCapableModel {
  is_custom?: boolean;
  features?: Record<string, boolean> | null;
}

/**
 * The switch is offered only for platform models that can reason. Custom
 * models bypass the console, so their providers would not get the
 * translation.
 */
export function modelSupportsReasoningToggle(
  model: ReasoningCapableModel | null | undefined
): boolean {
  if (!model || model.is_custom) {
    return false;
  }
  const features = model.features ?? {};
  return features.reasoning === true || features.reasoning_effort === true;
}

export function reasoningEffortForPreference(
  preference: AIChatReasoningPreference
): string | undefined {
  if (preference === undefined) {
    return undefined;
  }
  return preference ? AICHAT_REASONING_EFFORT_ON : AICHAT_REASONING_EFFORT_OFF;
}

/**
 * Returns model params carrying the preference. With no preference the
 * parameter is removed, so the request matches the model default.
 */
export function withReasoningEffortParam<T>(
  params: Record<string, T> | undefined,
  preference: AIChatReasoningPreference
): Record<string, T | string> | undefined {
  const effort = reasoningEffortForPreference(preference);
  if (effort === undefined) {
    if (!params || !(AICHAT_REASONING_EFFORT_PARAM in params)) {
      return params;
    }
    const rest: Record<string, T> = { ...params };
    delete rest[AICHAT_REASONING_EFFORT_PARAM];
    return Object.keys(rest).length > 0 ? rest : undefined;
  }
  return { ...(params ?? {}), [AICHAT_REASONING_EFFORT_PARAM]: effort };
}

/** Whole seconds for the "thought for N seconds" label, at least 1. */
export function reasoningDurationSeconds(durationMs: number | undefined): number | undefined {
  if (typeof durationMs !== 'number' || !Number.isFinite(durationMs) || durationMs < 0) {
    return undefined;
  }
  return Math.max(1, Math.round(durationMs / 1000));
}

/**
 * Preferences are remembered per conversation. A new conversation starts as
 * a draft and gets its real id on the first send; the draft's choice moves
 * to it.
 */
export function carryReasoningPreference<V>(
  preferences: Record<string, V>,
  fromKey: string | null | undefined,
  toKey: string | null | undefined
): Record<string, V> {
  if (!fromKey || !toKey || fromKey === toKey) {
    return preferences;
  }
  if (!(fromKey in preferences) || toKey in preferences) {
    return preferences;
  }
  return { ...preferences, [toKey]: preferences[fromKey] };
}

/** A conversation's explicit choice, valid only for the model it was made for. */
export interface AIChatReasoningChoice {
  enabled: boolean;
  modelKey: string;
}

export function reasoningModelKey(provider?: string | null, model?: string | null): string {
  return `${provider ?? ''}/${model ?? ''}`;
}

/** Switching the model resets the choice to the model default. */
export function reasoningPreferenceForModel(
  choice: AIChatReasoningChoice | undefined,
  modelKey: string
): AIChatReasoningPreference {
  return choice && choice.modelKey === modelKey ? choice.enabled : undefined;
}
