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

const USER_MEMORY_SKILL_ID = 'user-memory';

const SYSTEM_SKILL_DISPLAY: Record<string, {
  label: Record<string, string>;
  description: Record<string, string>;
  whenToUse: Record<string, string>;
  tags: Record<string, string[]>;
  category: string;
  icon: string;
}> = {
  [USER_MEMORY_SKILL_ID]: {
    label: {
      en_US: 'User Memory',
      zh_Hans: '用户记忆',
    },
    description: {
      en_US: 'Private account-level memory.',
      zh_Hans: '账号级私有记忆。',
    },
    whenToUse: {
      en_US: 'Remember or update user preferences, facts, instructions, and temporary context.',
      zh_Hans: '读取或维护用户偏好、事实、指令和临时上下文。',
    },
    tags: {
      en_US: ['system', 'memory'],
      zh_Hans: ['系统', '记忆'],
    },
    category: 'system',
    icon: 'brain',
  },
};

const SYSTEM_SKILL_TOOL_LABELS: Record<string, Record<string, Record<string, string>>> = {
  [USER_MEMORY_SKILL_ID]: {
    read_user_memory: {
      en_US: 'Read memory',
      zh_Hans: '读取记忆',
    },
    add_user_memory: {
      en_US: 'Add memory',
      zh_Hans: '新增记忆',
    },
    update_user_memory: {
      en_US: 'Update memory',
      zh_Hans: '更新记忆',
    },
    delete_user_memory: {
      en_US: 'Delete memory',
      zh_Hans: '删除记忆',
    },
    list_temporary_memories: {
      en_US: 'List temporary memories',
      zh_Hans: '查看临时记忆',
    },
  },
};

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
  const systemDisplay = SYSTEM_SKILL_DISPLAY[skill.skill_id];
  if (systemDisplay) {
    return getSystemAIChatSkillDisplayInfo(skill.skill_id, locale);
  }

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

function getSystemAIChatSkillDisplayInfo(
  skillId: string,
  locale: Locale | string
): AIChatSkillDisplayInfo {
  const display = SYSTEM_SKILL_DISPLAY[skillId];
  if (!display) {
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

  return {
    skillId,
    label: pickLocalizedText(display.label, locale, skillId),
    description: pickLocalizedText(display.description, locale, ''),
    whenToUse: pickLocalizedText(display.whenToUse, locale, ''),
    tags: pickLocalizedTags(display.tags, locale),
    category: display.category,
    icon: display.icon,
  };
}

export function buildAIChatSkillDisplayMap(
  skills: AIChatSkillMetadata[],
  locale: Locale | string
): AIChatSkillDisplayMap {
  const map = skills.reduce<AIChatSkillDisplayMap>((acc, skill) => {
    acc[skill.skill_id] = getAIChatSkillDisplayInfo(skill, locale);
    return acc;
  }, {});
  for (const skillId of Object.keys(SYSTEM_SKILL_DISPLAY)) {
    map[skillId] = getSystemAIChatSkillDisplayInfo(skillId, locale);
  }
  return map;
}

export function getFallbackAIChatSkillDisplayInfo(
  skillId: string,
  locale: Locale | string = 'zh-Hans'
): AIChatSkillDisplayInfo {
  return getSystemAIChatSkillDisplayInfo(skillId, locale);
}

export function getAIChatSkillToolDisplayName(
  skillId: string,
  toolName: string | undefined,
  locale: Locale | string
): string {
  const name = toolName?.trim();
  if (!name) return '';

  const labels = SYSTEM_SKILL_TOOL_LABELS[skillId]?.[name];
  if (!labels) return name;
  return pickLocalizedText(labels, locale, name);
}
