import type { LocalizedText, ParameterRuleItem } from '@/services/types/model';

const DEFAULT_VOICE_PARAMETER = 'default_voice';

export interface DefaultVoiceOption {
  value: string;
  label: string;
}

function resolveOptionLabel(
  value: string,
  localized: LocalizedText | undefined,
  locale: string
): string {
  const preferred = locale.toLowerCase().startsWith('zh') ? localized?.zh_Hans : localized?.en_US;
  return preferred?.trim() || localized?.zh_Hans?.trim() || localized?.en_US?.trim() || value;
}

export function getDefaultVoiceOptions(
  rules: ReadonlyArray<Pick<ParameterRuleItem, 'name' | 'options' | 'option_labels'>>,
  locale: string
): DefaultVoiceOption[] {
  const rule = rules.find(item => item.name === DEFAULT_VOICE_PARAMETER);
  if (!Array.isArray(rule?.options)) return [];

  return [...new Set(rule.options.map(option => option.trim()).filter(Boolean))].map(value => ({
    value,
    label: resolveOptionLabel(value, rule.option_labels?.[value], locale),
  }));
}
