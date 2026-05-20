import type { Locale } from '@/i18n/config';
import type { AIChatSkillMetadata } from '@/services/types/aichat';

export interface AIChatSkillDisplayInfo {
  skillId: string;
  label: string;
  description: string;
  whenToUse: string;
  tags: string[];
  category: string;
  icon: string;
}

export type AIChatSkillDisplayMap = Record<string, AIChatSkillDisplayInfo>;

function toDisplayLocale(locale: Locale | string): string {
  if (locale === 'en-US') return 'en_US';
  return 'zh_Hans';
}

function pickLocalizedText(
  values: Record<string, string> | undefined,
  locale: Locale | string,
  fallback: string
): string {
  const displayLocale = toDisplayLocale(locale);
  return values?.[displayLocale] ?? values?.zh_Hans ?? values?.en_US ?? fallback;
}

function pickLocalizedTags(
  values: Record<string, string[]> | undefined,
  locale: Locale | string
): string[] {
  const displayLocale = toDisplayLocale(locale);
  return values?.[displayLocale] ?? values?.zh_Hans ?? values?.en_US ?? [];
}

export function getAIChatSkillDisplayInfo(
  skill: AIChatSkillMetadata,
  locale: Locale | string
): AIChatSkillDisplayInfo {
  return {
    skillId: skill.skill_id,
    label: pickLocalizedText(skill.display?.label, locale, skill.name || skill.skill_id),
    description: pickLocalizedText(skill.display?.description, locale, skill.description),
    whenToUse: pickLocalizedText(skill.display?.when_to_use, locale, skill.when_to_use),
    tags: pickLocalizedTags(skill.display?.tags, locale),
    category: skill.display?.category ?? 'general',
    icon: skill.display?.icon ?? 'sparkles',
  };
}

export function buildAIChatSkillDisplayMap(
  skills: AIChatSkillMetadata[],
  locale: Locale | string
): AIChatSkillDisplayMap {
  return skills.reduce<AIChatSkillDisplayMap>((map, skill) => {
    map[skill.skill_id] = getAIChatSkillDisplayInfo(skill, locale);
    return map;
  }, {});
}

export function getFallbackAIChatSkillDisplayInfo(skillId: string): AIChatSkillDisplayInfo {
  return {
    skillId,
    label: skillId,
    description: '',
    whenToUse: '',
    tags: [],
    category: 'general',
    icon: 'sparkles',
  };
}
