import type { ParameterRuleItem, ParameterScalarValue } from '@/services/types/model';

const TEXT_CHAT_PRESET_TARGET_KEYS = [
  'temperature',
  'top_p',
  'presence_penalty',
  'frequency_penalty',
] as const;

type TextChatPresetTargetKey = (typeof TEXT_CHAT_PRESET_TARGET_KEYS)[number];

export type TextChatParameterPresetId = 'precise' | 'balanced' | 'creative';

interface TextChatParameterPresetDefinition {
  id: TextChatParameterPresetId;
  values: Partial<Record<TextChatPresetTargetKey, ParameterScalarValue>>;
}

interface ApplyTextChatParameterPresetOptions {
  presetId: TextChatParameterPresetId;
  rules: ParameterRuleItem[];
  enabledMap: Record<string, boolean>;
  localValues: Record<string, number | string | boolean>;
}

interface ApplyTextChatParameterPresetResult {
  appliedCount: number;
  enabledMap: Record<string, boolean>;
  localValues: Record<string, number | string | boolean>;
}

const TEXT_CHAT_PRESET_TARGET_KEY_SET = new Set<string>(TEXT_CHAT_PRESET_TARGET_KEYS);

export const TEXT_CHAT_PARAMETER_PRESETS: readonly TextChatParameterPresetDefinition[] = [
  {
    id: 'precise',
    values: {
      temperature: 0.15,
      top_p: 1,
      presence_penalty: 0,
      frequency_penalty: 0,
    },
  },
  {
    id: 'balanced',
    values: {
      temperature: 0.55,
      top_p: 1,
      presence_penalty: 0.05,
      frequency_penalty: 0.1,
    },
  },
  {
    id: 'creative',
    values: {
      temperature: 0.95,
      top_p: 1,
      presence_penalty: 0.45,
      frequency_penalty: 0.2,
    },
  },
] as const;

const TEXT_CHAT_PARAMETER_PRESET_ID_SET = new Set<TextChatParameterPresetId>(
  TEXT_CHAT_PARAMETER_PRESETS.map(item => item.id)
);

export function isTextChatParameterPresetId(value: string): value is TextChatParameterPresetId {
  return TEXT_CHAT_PARAMETER_PRESET_ID_SET.has(value as TextChatParameterPresetId);
}

function roundToPrecision(value: number, precision: number): number {
  if (precision <= 0) {
    return Math.round(value);
  }

  const factor = Math.pow(10, precision);
  return Math.round(value * factor) / factor;
}

function getRulePresetLookupKey(rule: Pick<ParameterRuleItem, 'name' | 'template_key'>): string {
  const templateKey = String(rule.template_key || '').trim();

  if (templateKey && TEXT_CHAT_PRESET_TARGET_KEY_SET.has(templateKey)) {
    return templateKey;
  }

  return String(rule.name).trim();
}

function normalizePresetValueForRule(
  rule: ParameterRuleItem,
  value: ParameterScalarValue
): number | string | boolean | undefined {
  if (value === null || value === undefined) {
    return undefined;
  }

  if (rule.type === 'boolean') {
    return Boolean(value);
  }

  if (rule.type === 'string' || rule.type === 'text') {
    return String(value);
  }

  let numericValue =
    typeof value === 'number' && Number.isFinite(value) ? value : Number.parseFloat(String(value));

  if (!Number.isFinite(numericValue)) {
    return undefined;
  }

  if (typeof rule.min === 'number') {
    numericValue = Math.max(numericValue, rule.min);
  }

  if (typeof rule.max === 'number') {
    numericValue = Math.min(numericValue, rule.max);
  }

  if (rule.type === 'int') {
    return Math.round(numericValue);
  }

  if (typeof rule.precision === 'number' && Number.isFinite(rule.precision)) {
    return roundToPrecision(numericValue, rule.precision);
  }

  return numericValue;
}

/**
 * @util Check whether the current parameter rules can benefit from a text-chat preset.
 */
export function hasTextChatParameterPresetTargets(rules: ParameterRuleItem[]): boolean {
  return rules.some(rule => TEXT_CHAT_PRESET_TARGET_KEY_SET.has(getRulePresetLookupKey(rule)));
}

/**
 * @util Apply a text-chat parameter preset while respecting the current schema and value bounds.
 */
export function applyTextChatParameterPreset({
  presetId,
  rules,
  enabledMap,
  localValues,
}: ApplyTextChatParameterPresetOptions): ApplyTextChatParameterPresetResult {
  const preset = TEXT_CHAT_PARAMETER_PRESETS.find(item => item.id === presetId);

  if (!preset) {
    return {
      appliedCount: 0,
      enabledMap,
      localValues,
    };
  }

  const nextEnabledMap = { ...enabledMap };
  const nextLocalValues = { ...localValues };
  let appliedCount = 0;

  rules.forEach(rule => {
    const lookupKey = getRulePresetLookupKey(rule) as TextChatPresetTargetKey;
    const presetValue = preset.values[lookupKey];

    if (presetValue === undefined) {
      return;
    }

    const normalizedValue = normalizePresetValueForRule(rule, presetValue);

    if (normalizedValue === undefined) {
      return;
    }

    nextEnabledMap[rule.name] = true;
    nextLocalValues[rule.name] = normalizedValue;
    appliedCount += 1;
  });

  return {
    appliedCount,
    enabledMap: nextEnabledMap,
    localValues: nextLocalValues,
  };
}
