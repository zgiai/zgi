import type { Locale } from '@/i18n/config';
import type { AIChatSkillInvocation, AIChatSkillMetadata } from '@/services/types/aichat';

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
const AGENT_MEMORY_SKILL_ID = 'agent-memory';
const AGENT_KNOWLEDGE_SKILL_ID = 'agent-knowledge';

export function isHiddenSystemSkill(skillId: string): boolean {
  const normalized = skillId.trim().toLowerCase();
  return (
    normalized === USER_MEMORY_SKILL_ID ||
    normalized === AGENT_MEMORY_SKILL_ID ||
    normalized === AGENT_KNOWLEDGE_SKILL_ID
  );
}

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
  [AGENT_MEMORY_SKILL_ID]: {
    label: {
      en_US: 'Agent Memory',
      zh_Hans: '智能体固定记忆',
    },
    description: {
      en_US: 'Agent-scoped fixed-slot memory.',
      zh_Hans: '当前智能体的固定槽位记忆。',
    },
    whenToUse: {
      en_US: 'Read or update configured memory slots for the current Agent.',
      zh_Hans: '读取或更新当前智能体已配置的记忆槽位。',
    },
    tags: {
      en_US: ['system', 'memory', 'agent'],
      zh_Hans: ['系统', '记忆', '智能体'],
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
  [AGENT_MEMORY_SKILL_ID]: {
    read_agent_memory: {
      en_US: 'Read agent memory',
      zh_Hans: '读取智能体记忆',
    },
    update_agent_memory: {
      en_US: 'Update agent memory',
      zh_Hans: '更新智能体记忆',
    },
    clear_agent_memory: {
      en_US: 'Clear agent memory',
      zh_Hans: '清空智能体记忆',
    },
  },
};

const USER_MEMORY_TOOL_RESULT_TEXT: Record<string, Record<string, string>> = {
  add_user_memory: {
    en_US: 'Saved memory: {content}',
    zh_Hans: '已保存记忆：{content}',
  },
  update_user_memory: {
    en_US: 'Updated memory: {content}',
    zh_Hans: '已更新记忆：{content}',
  },
  update_user_memory_without_content: {
    en_US: 'Updated memory {entryId}',
    zh_Hans: '已更新记忆 {entryId}',
  },
  delete_user_memory: {
    en_US: 'Deleted memory {entryId}',
    zh_Hans: '已删除记忆 {entryId}',
  },
  read_user_memory: {
    en_US: 'Read {count} memories',
    zh_Hans: '已读取 {count} 条记忆',
  },
  list_temporary_memories: {
    en_US: 'Listed {count} temporary memories',
    zh_Hans: '已查看 {count} 条临时记忆',
  },
};

const AGENT_MEMORY_TOOL_RESULT_TEXT: Record<string, Record<string, string>> = {
  update_agent_memory: {
    en_US: 'Updated {key}: {content}',
    zh_Hans: '已更新 {key}：{content}',
  },
  clear_agent_memory: {
    en_US: 'Cleared {key}',
    zh_Hans: '已清空 {key}',
  },
  read_agent_memory: {
    en_US: 'Read {count} agent memory slots',
    zh_Hans: '已读取 {count} 个智能体记忆槽位',
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

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}

function stringFromRecord(source: Record<string, unknown>, keys: string[]): string {
  for (const key of keys) {
    const value = source[key];
    if (typeof value === 'string' && value.trim()) return value.trim();
    if (typeof value === 'number' && Number.isFinite(value)) return String(value);
  }
  return '';
}

function numberFromRecord(source: Record<string, unknown>, keys: string[]): number | null {
  for (const key of keys) {
    const value = source[key];
    if (typeof value === 'number' && Number.isFinite(value)) return value;
    if (typeof value === 'string' && value.trim() && Number.isFinite(Number(value))) {
      return Number(value);
    }
  }
  return null;
}

function compactMemoryContent(content: string): string {
  const normalized = content.replace(/\s+/g, ' ').trim();
  if (normalized.length <= 120) return normalized;
  return `${normalized.slice(0, 117)}...`;
}

function formatMemoryToolResult(
  key: keyof typeof USER_MEMORY_TOOL_RESULT_TEXT,
  locale: Locale | string,
  replacements: Record<string, string | number>
): string {
  let text = pickLocalizedText(USER_MEMORY_TOOL_RESULT_TEXT[key], locale, key);
  for (const [name, value] of Object.entries(replacements)) {
    text = text.replace(`{${name}}`, String(value));
  }
  return text;
}

function formatAgentMemoryToolResult(
  key: keyof typeof AGENT_MEMORY_TOOL_RESULT_TEXT,
  locale: Locale | string,
  replacements: Record<string, string | number>
): string {
  let text = pickLocalizedText(AGENT_MEMORY_TOOL_RESULT_TEXT[key], locale, key);
  for (const [name, value] of Object.entries(replacements)) {
    text = text.replace(`{${name}}`, String(value));
  }
  return text;
}

export function getAIChatSkillResultDisplay(
  invocation: AIChatSkillInvocation,
  locale: Locale | string
): string | null {
  if (invocation.status !== 'success') {
    return null;
  }

  const toolName = invocation.tool_name?.trim();
  const result = isRecord(invocation.result) ? invocation.result : {};
  const args = isRecord(invocation.arguments) ? invocation.arguments : {};

  if (invocation.skill_id === AGENT_MEMORY_SKILL_ID) {
    const key = stringFromRecord(result, ['key']) || stringFromRecord(args, ['key']);
    const content = compactMemoryContent(
      stringFromRecord(result, ['content']) || stringFromRecord(args, ['content'])
    );
    switch (toolName) {
      case 'update_agent_memory':
        return content
          ? formatAgentMemoryToolResult('update_agent_memory', locale, { key, content })
          : getAIChatSkillToolDisplayName(invocation.skill_id, toolName, locale);
      case 'clear_agent_memory':
        return formatAgentMemoryToolResult('clear_agent_memory', locale, { key }).trim();
      case 'read_agent_memory': {
        const entries = Array.isArray(result.entries) ? result.entries : [];
        return formatAgentMemoryToolResult('read_agent_memory', locale, {
          count: entries.length,
        });
      }
      default:
        return null;
    }
  }

  if (invocation.skill_id !== USER_MEMORY_SKILL_ID) {
    return null;
  }

  const content = compactMemoryContent(
    stringFromRecord(result, ['content', 'memory', 'text']) ||
      stringFromRecord(args, ['content', 'memory', 'text'])
  );
  const entryId =
    stringFromRecord(result, ['entry_id', 'id']) || stringFromRecord(args, ['entry_id', 'id']);

  switch (toolName) {
    case 'add_user_memory':
      return content
        ? formatMemoryToolResult('add_user_memory', locale, { content })
        : getAIChatSkillToolDisplayName(invocation.skill_id, toolName, locale);
    case 'update_user_memory':
      if (content) {
        return formatMemoryToolResult('update_user_memory', locale, { content });
      }
      return formatMemoryToolResult('update_user_memory_without_content', locale, {
        entryId: entryId || '',
      }).trim();
    case 'delete_user_memory':
      return formatMemoryToolResult('delete_user_memory', locale, {
        entryId: entryId || '',
      }).trim();
    case 'read_user_memory': {
      const count = numberFromRecord(result, ['entries_count', 'count']) ?? 0;
      return formatMemoryToolResult('read_user_memory', locale, { count });
    }
    case 'list_temporary_memories': {
      const count = numberFromRecord(result, ['entries_count', 'count']) ?? 0;
      return formatMemoryToolResult('list_temporary_memories', locale, { count });
    }
    default:
      return null;
  }
}
