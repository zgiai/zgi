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
const AGENT_DATABASE_SKILL_ID = 'agent-database';
const AGENT_WORKFLOW_SKILL_ID = 'agent-workflow';

function normalizeSkillId(skillId: string): string {
  return skillId.trim().toLowerCase();
}

export function isHiddenSystemSkill(skillId: string): boolean {
  const normalized = normalizeSkillId(skillId);
  return (
    normalized === USER_MEMORY_SKILL_ID ||
    normalized === AGENT_MEMORY_SKILL_ID ||
    normalized === AGENT_KNOWLEDGE_SKILL_ID ||
    normalized === AGENT_DATABASE_SKILL_ID ||
    normalized === AGENT_WORKFLOW_SKILL_ID
  );
}

type LocalizedText = Record<string, string>;
type LocalizedTags = Record<string, string[]>;

const SYSTEM_SKILL_DISPLAY: Record<string, {
  label: LocalizedText;
  description: LocalizedText;
  whenToUse: LocalizedText;
  tags: LocalizedTags;
  category: string;
  icon: string;
}> = {
  time: {
    label: {
      en_US: 'Time',
      zh_Hans: '时间',
    },
    description: {
      en_US: 'Looks up current time and performs timezone-aware date calculations.',
      zh_Hans: '查询当前时间，并进行带时区的日期计算。',
    },
    whenToUse: {
      en_US: 'Use when answers depend on current time, dates, deadlines, or date differences.',
      zh_Hans: '当问题依赖当前时间、日期、截止日期或日期差时启用。',
    },
    tags: {
      en_US: ['Time', 'Date'],
      zh_Hans: ['时间', '日期'],
    },
    category: 'productivity',
    icon: 'clock',
  },
  calculator: {
    label: {
      en_US: 'Calculator',
      zh_Hans: '计算器',
    },
    description: {
      en_US: 'Handles exact arithmetic, percentages, discounts, and numeric comparisons.',
      zh_Hans: '用于精确计算、百分比、折扣、变化率和数值比较。',
    },
    whenToUse: {
      en_US: 'Use when answers require exact calculation instead of mental math.',
      zh_Hans: '当问题需要精确计算，而不是让模型心算时启用。',
    },
    tags: {
      en_US: ['Math', 'Arithmetic'],
      zh_Hans: ['数学', '精确计算'],
    },
    category: 'productivity',
    icon: 'calculator',
  },
  'file-generator': {
    label: {
      en_US: 'File Generator',
      zh_Hans: '文件生成器',
    },
    description: {
      en_US: 'Creates downloadable TXT, Markdown, HTML, JSON, CSV, DOCX, XLSX, PDF, and PPTX files.',
      zh_Hans: '创建可下载的 TXT、Markdown、HTML、JSON、CSV、DOCX、XLSX、PDF 和 PPTX 文件。',
    },
    whenToUse: {
      en_US: 'Use when the answer should be delivered as a generated file.',
      zh_Hans: '当回答需要以生成文件交付时启用。',
    },
    tags: {
      en_US: ['File', 'Export'],
      zh_Hans: ['文件', '导出'],
    },
    category: 'productivity',
    icon: 'file-plus',
  },
  'work-report-generator': {
    label: {
      en_US: 'Work Report Generator',
      zh_Hans: '周报月报生成',
    },
    description: {
      en_US: 'Turns work notes, progress, metrics, risks, and plans into structured weekly or monthly reports.',
      zh_Hans: '将工作记录、项目进展、关键数据、风险问题和计划整理成结构化周报或月报。',
    },
    whenToUse: {
      en_US: 'Use when the user needs a weekly report, monthly report, work summary, project update, or management report.',
      zh_Hans: '当用户需要生成周报、月报、工作总结、项目进展汇报或管理汇报时使用。',
    },
    tags: {
      en_US: ['Report', 'Productivity', 'Summary'],
      zh_Hans: ['周报', '月报', '工作总结'],
    },
    category: 'productivity',
    icon: 'clipboard-list',
  },
  'schedule-planner': {
    label: {
      en_US: 'Schedule Planner',
      zh_Hans: '日程规划',
    },
    description: {
      en_US: 'Turns goals, tasks, deadlines, and availability into practical schedules and agendas.',
      zh_Hans: '将目标、任务、截止时间和可用时间整理成可执行的日程计划。',
    },
    whenToUse: {
      en_US: 'Use for planning days, weeks, task schedules, meeting agendas, study plans, or workload arrangements.',
      zh_Hans: '用于规划每日安排、每周计划、任务排期、会议议程、学习计划或工作负载。',
    },
    tags: {
      en_US: ['Schedule', 'Planning', 'Productivity'],
      zh_Hans: ['日程', '计划', '效率'],
    },
    category: 'productivity',
    icon: 'calendar-days',
  },
  'chart-generator': {
    label: {
      en_US: 'Chart Generator',
      zh_Hans: '图表生成器',
    },
    description: {
      en_US: 'Generates SVG charts from structured data.',
      zh_Hans: '根据结构化数据生成 SVG 图表。',
    },
    whenToUse: {
      en_US: 'Use when the answer should include a generated chart artifact.',
      zh_Hans: '当回答需要生成图表文件时使用。',
    },
    tags: {
      en_US: ['Chart', 'Visualization', 'Data'],
      zh_Hans: ['图表', '可视化', '数据'],
    },
    category: 'visualization',
    icon: 'chart-no-axes-combined',
  },
  'internal-knowledge': {
    label: {
      en_US: 'Internal Knowledge',
      zh_Hans: '内部知识库',
    },
    description: {
      en_US: 'Finds knowledge bases accessible to the current AIChat user and retrieves relevant context.',
      zh_Hans: '查找当前 AIChat 用户可访问的知识库，并检索相关上下文。',
    },
    whenToUse: {
      en_US: 'Use when an AIChat answer needs facts or source context from accessible knowledge bases.',
      zh_Hans: '当 AIChat 回复需要引用可访问知识库中的事实或来源上下文时使用。',
    },
    tags: {
      en_US: ['Knowledge', 'Retrieval'],
      zh_Hans: ['知识库', '检索'],
    },
    category: 'knowledge',
    icon: 'library',
  },
  [AGENT_KNOWLEDGE_SKILL_ID]: {
    label: {
      en_US: 'Agent Knowledge',
      zh_Hans: '智能体知识库',
    },
    description: {
      en_US: 'Retrieves only from knowledge bases bound to the current Agent configuration.',
      zh_Hans: '仅从当前智能体配置绑定的知识库中检索上下文。',
    },
    whenToUse: {
      en_US: 'Use for Agent answers that need configured knowledge base retrieval.',
      zh_Hans: '当智能体回复需要检索已绑定知识库时使用。',
    },
    tags: {
      en_US: ['Knowledge', 'Agent'],
      zh_Hans: ['知识库', '智能体'],
    },
    category: 'knowledge',
    icon: 'library',
  },
  'internal-database': {
    label: {
      en_US: 'Internal Database',
      zh_Hans: '内部数据库',
    },
    description: {
      en_US: 'Finds accessible databases, inspects tables, and performs structured record operations.',
      zh_Hans: '查找可访问的数据库、查看表结构，并执行结构化记录操作。',
    },
    whenToUse: {
      en_US: 'Use when AIChat needs facts or changes from workspace database tables.',
      zh_Hans: '当 AIChat 需要从工作区数据库表读取事实或写入变更时使用。',
    },
    tags: {
      en_US: ['Database', 'Records'],
      zh_Hans: ['数据库', '记录'],
    },
    category: 'database',
    icon: 'database',
  },
  [AGENT_DATABASE_SKILL_ID]: {
    label: {
      en_US: 'Agent Database',
      zh_Hans: '智能体数据库',
    },
    description: {
      en_US: 'Uses only database tables bound to the current Agent configuration.',
      zh_Hans: '仅使用当前智能体配置中绑定的数据库表。',
    },
    whenToUse: {
      en_US: 'Use for Agent answers or actions that need configured database records.',
      zh_Hans: '当智能体回答或操作需要使用已配置的数据库记录时使用。',
    },
    tags: {
      en_US: ['Database', 'Agent'],
      zh_Hans: ['数据库', '智能体'],
    },
    category: 'database',
    icon: 'database',
  },
  [AGENT_WORKFLOW_SKILL_ID]: {
    label: {
      en_US: 'Agent Workflow',
      zh_Hans: 'Agent 工作流',
    },
    description: {
      en_US: 'Call workflows bound to this Agent.',
      zh_Hans: '调用绑定到当前 Agent 的工作流。',
    },
    whenToUse: {
      en_US: 'Use for configured approval or process workflows.',
      zh_Hans: '用于已配置的审批或流程工作流。',
    },
    tags: {
      en_US: ['Workflow', 'Agent'],
      zh_Hans: ['工作流', '智能体'],
    },
    category: 'system',
    icon: 'workflow',
  },
  [AGENT_MEMORY_SKILL_ID]: {
    label: {
      en_US: 'Agent Memory',
      zh_Hans: '智能体记忆',
    },
    description: {
      en_US: 'Agent-scoped memory values configured for this runtime.',
      zh_Hans: '当前运行中配置的智能体级记忆值。',
    },
    whenToUse: {
      en_US: 'Use when the Agent needs configured memory slots for personalization or context.',
      zh_Hans: '当智能体需要使用已配置的记忆字段进行个性化或上下文补充时使用。',
    },
    tags: {
      en_US: ['System', 'Memory'],
      zh_Hans: ['系统', '记忆'],
    },
    category: 'system',
    icon: 'brain',
  },
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
  time: {
    current_time: {
      en_US: 'Current time',
      zh_Hans: '查询当前时间',
    },
    date_calculate: {
      en_US: 'Date calculation',
      zh_Hans: '日期计算',
    },
  },
  calculator: {
    evaluate_expression: {
      en_US: 'Evaluate expression',
      zh_Hans: '计算表达式',
    },
    calculate: {
      en_US: 'Calculate',
      zh_Hans: '基础计算',
    },
    percentage: {
      en_US: 'Percentage calculation',
      zh_Hans: '百分比计算',
    },
  },
  'file-generator': {
    generate_file: {
      en_US: 'Generate file',
      zh_Hans: '生成文件',
    },
    generate_docx: {
      en_US: 'Generate Word document',
      zh_Hans: '生成 Word 文档',
    },
    generate_pdf: {
      en_US: 'Generate PDF',
      zh_Hans: '生成 PDF',
    },
    generate_pptx: {
      en_US: 'Generate PowerPoint',
      zh_Hans: '生成 PowerPoint',
    },
  },
  'work-report-generator': {
    generate_file: {
      en_US: 'Generate report file',
      zh_Hans: '生成报告文件',
    },
  },
  'chart-generator': {
    generate_chart: {
      en_US: 'Generate chart',
      zh_Hans: '生成图表',
    },
  },
  'internal-knowledge': {
    list_accessible_knowledge_bases: {
      en_US: 'List accessible knowledge bases',
      zh_Hans: '列出可访问知识库',
    },
    retrieve_knowledge: {
      en_US: 'Retrieve knowledge',
      zh_Hans: '检索知识库',
    },
  },
  [AGENT_KNOWLEDGE_SKILL_ID]: {
    retrieve_agent_knowledge: {
      en_US: 'Retrieve agent knowledge',
      zh_Hans: '检索智能体知识库',
    },
  },
  'internal-database': {
    list_accessible_databases: {
      en_US: 'List accessible databases',
      zh_Hans: '列出可访问数据库',
    },
    list_database_tables: {
      en_US: 'List database tables',
      zh_Hans: '列出数据库表',
    },
    describe_database_table: {
      en_US: 'Describe database table',
      zh_Hans: '查看表结构',
    },
    query_table_records: {
      en_US: 'Query table records',
      zh_Hans: '查询表记录',
    },
    insert_table_records: {
      en_US: 'Insert table records',
      zh_Hans: '插入表记录',
    },
    update_table_records: {
      en_US: 'Update table records',
      zh_Hans: '更新表记录',
    },
    delete_table_records: {
      en_US: 'Delete table records',
      zh_Hans: '删除表记录',
    },
  },
  [AGENT_DATABASE_SKILL_ID]: {
    list_accessible_databases: {
      en_US: 'List agent databases',
      zh_Hans: '列出智能体数据库',
    },
    list_database_tables: {
      en_US: 'List database tables',
      zh_Hans: '列出数据库表',
    },
    describe_database_table: {
      en_US: 'Describe database table',
      zh_Hans: '查看表结构',
    },
    query_table_records: {
      en_US: 'Query table records',
      zh_Hans: '查询表记录',
    },
    insert_table_records: {
      en_US: 'Insert table records',
      zh_Hans: '插入表记录',
    },
    update_table_records: {
      en_US: 'Update table records',
      zh_Hans: '更新表记录',
    },
    delete_table_records: {
      en_US: 'Delete table records',
      zh_Hans: '删除表记录',
    },
  },
  [AGENT_WORKFLOW_SKILL_ID]: {
    list_agent_workflows: {
      en_US: 'List agent workflows',
      zh_Hans: '列出智能体工作流',
    },
    run_agent_workflow: {
      en_US: 'Run agent workflow',
      zh_Hans: '运行智能体工作流',
    },
    get_workflow_run_status: {
      en_US: 'Get workflow run status',
      zh_Hans: '查询工作流运行状态',
    },
  },
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
  const systemDisplay = SYSTEM_SKILL_DISPLAY[normalizeSkillId(skill.skill_id)];
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
  const normalizedSkillId = normalizeSkillId(skillId);
  const display = SYSTEM_SKILL_DISPLAY[normalizedSkillId];
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
    skillId: normalizedSkillId,
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

  const labels = SYSTEM_SKILL_TOOL_LABELS[normalizeSkillId(skillId)]?.[name];
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

function compactMemoryContent(content: string, maxLength = 120): string {
  const normalized = content.replace(/\s+/g, ' ').trim();
  if (normalized.length <= maxLength) return normalized;
  return `${normalized.slice(0, Math.max(0, maxLength - 3))}...`;
}

export function getAIChatUserMemoryMutationTitle(
  action: string | undefined,
  locale: Locale | string,
  options: { content?: string; entryId?: string } = {}
): string {
  const content = compactMemoryContent(options.content ?? '', 80);
  const entryId = options.entryId ?? '';
  switch (action) {
    case 'create':
      return content
        ? formatMemoryToolResult('add_user_memory', locale, { content })
        : pickLocalizedText(USER_MEMORY_TOOL_RESULT_TEXT.add_user_memory, locale, 'Saved memory');
    case 'update':
      return content
        ? formatMemoryToolResult('update_user_memory', locale, { content })
        : formatMemoryToolResult('update_user_memory_without_content', locale, { entryId }).trim();
    case 'delete':
      return formatMemoryToolResult('delete_user_memory', locale, { entryId }).trim();
    case 'clear':
      return pickLocalizedText(
        {
          en_US: 'Cleared memory',
          zh_Hans: '已清空记忆',
        },
        locale,
        'Cleared memory'
      );
    default:
      return pickLocalizedText(
        {
          en_US: 'Updated memory',
          zh_Hans: '已更新记忆',
        },
        locale,
        'Updated memory'
      );
  }
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

  if (normalizeSkillId(invocation.skill_id) !== USER_MEMORY_SKILL_ID) {
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
