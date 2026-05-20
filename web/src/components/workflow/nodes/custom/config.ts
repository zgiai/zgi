import type { WorkflowNodeData, WorkflowNodePreviewConfig } from '../../store/type';
import type React from 'react';
import {
  Play,
  Search,
  Brain,
  Square,
  GitBranch,
  Repeat,
  RotateCw,
  Wrench,
  MessageSquareCode,
  FileText,
  Variable,
  Merge,
  CircleArrowOutUpRight,
  Braces,
  StickyNote,
  Image as ImageIcon,
  ClipboardCheck,
  MessageCircleQuestion,
  Smartphone,
} from 'lucide-react';
import { MessageCircle } from 'lucide-react';
import { Pencil } from 'lucide-react';
import { Code2 } from 'lucide-react';
import { Globe, Database, Clock3 } from 'lucide-react';

// Shared Icon component type for Lucide icons
export type IconComponent = React.ComponentType<{
  className?: string;
  size?: number | string;
  strokeWidth?: number | string;
}>;

export interface NodeTheme {
  icon: IconComponent;
  badgeText: string;
  width?: number;
  height?: number;
  classNames: {
    wrapper?: string;
    card?: string;
    title?: string;
    iconBg?: string;
    desc?: string;
    content?: string;
    badge?: string;
  };
  // Used by minimap to render node color consistently with theme
  miniMapColor: string;
  handles: 'none' | 'source' | 'target' | 'both';
  // Whether the node supports user resize via bottom-right handle
  resizable?: boolean;
  // Whether height should be strictly auto-determined by content
  autoHeight?: boolean;
  preview?: WorkflowNodePreviewConfig;
}

const COMMON_LOGIC_CLASSES = {
  wrapper: 'rounded-xl',
  card: '',
  title: '',
  desc: '',
  content: 'px-3.5 pb-3.5 space-y-2',
  badge: '',
};

const NODE_ICON_TONES = {
  agent:
    'bg-[#EAF1FF] text-[#111827] shadow-[inset_0_0_0_1px_rgba(42,111,219,0.06),0_1px_2px_rgba(15,23,42,0.04)] [&_svg]:text-[#111827]',
  flow:
    'bg-[#CFF8E9] text-[#111827] shadow-[inset_0_0_0_1px_rgba(22,134,92,0.08),0_1px_2px_rgba(15,23,42,0.05)] [&_svg]:text-[#111827]',
  logic:
    'bg-[#FFF6C7] text-[#111827] shadow-[inset_0_0_0_1px_rgba(184,148,0,0.06),0_1px_2px_rgba(15,23,42,0.035)] [&_svg]:text-[#111827]',
  data:
    'bg-[#F0E5FF] text-[#111827] shadow-[inset_0_0_0_1px_rgba(106,69,194,0.07),0_1px_2px_rgba(15,23,42,0.04)] [&_svg]:text-[#111827]',
  tool:
    'bg-[#FFE97A] text-[#111827] shadow-[inset_0_0_0_1px_rgba(138,106,0,0.09),0_1px_2px_rgba(15,23,42,0.05)] [&_svg]:text-[#111827]',
  extension:
    'bg-[#E8F3FF] text-[#111827] shadow-[inset_0_0_0_1px_rgba(42,111,219,0.07),0_1px_2px_rgba(15,23,42,0.04)] [&_svg]:text-[#111827]',
  terminal:
    'bg-[#EAEAEA] text-[#111827] shadow-[inset_0_0_0_1px_rgba(15,23,42,0.06),0_1px_2px_rgba(15,23,42,0.04)] [&_svg]:text-[#111827]',
  danger:
    'bg-[#FFE1E1] text-[#111827] shadow-[inset_0_0_0_1px_rgba(194,65,65,0.07),0_1px_2px_rgba(15,23,42,0.04)] [&_svg]:text-[#111827]',
};

export const NODE_THEMES: Record<WorkflowNodeData['type'] | 'default', NodeTheme> = {
  start: {
    icon: Play,
    badgeText: '开始',
    width: 280,
    height: 48,
    classNames: {
      ...COMMON_LOGIC_CLASSES,
      wrapper: 'rounded-xl min-w-[280px]',
      iconBg: NODE_ICON_TONES.flow,
    },
    miniMapColor: '#10b981',
    handles: 'source',
    resizable: false,
    autoHeight: true,
  },
  loop: {
    icon: RotateCw,
    badgeText: '循环',
    classNames: {
      wrapper: 'rounded-xl flex flex-col h-full shadow-lg overflow-visible',
      card: 'border-slate-200/80 dark:border-slate-700/80 grow flex flex-col transition-all overflow-visible',
      title: 'text-blue-600 dark:text-blue-400 font-bold',
      iconBg: NODE_ICON_TONES.logic,
      desc: '',
      content: 'grow flex flex-col bg-transparent',
      badge: 'bg-blue-100 dark:bg-blue-900/50 text-blue-600 dark:text-blue-400',
    },
    miniMapColor: '#3b82f6',
    handles: 'both',
    resizable: true,
    autoHeight: false,
    width: 300,
    height: 200,
    preview: { kind: 'container', width: 300, height: 200 },
  },
  'loop-start': {
    icon: Play,
    badgeText: '',
    classNames: {
      ...COMMON_LOGIC_CLASSES,
      iconBg: NODE_ICON_TONES.logic,
    },
    miniMapColor: '#3b82f6',
    handles: 'none',
    resizable: false,
    autoHeight: true,
  },
  'parameter-extractor': {
    icon: Variable,
    badgeText: 'Param',
    width: 280,
    height: 100,
    classNames: {
      ...COMMON_LOGIC_CLASSES,
      iconBg: NODE_ICON_TONES.agent,
    },
    miniMapColor: '#f43f5e',
    handles: 'both',
    resizable: false,
    autoHeight: true,
  },
  'variable-aggregator': {
    icon: Merge,
    badgeText: '聚合',
    width: 280,
    height: 88,
    classNames: {
      ...COMMON_LOGIC_CLASSES,
      iconBg: NODE_ICON_TONES.data,
    },
    miniMapColor: '#f97316',
    handles: 'both',
    resizable: false,
    autoHeight: true,
  },
  'knowledge-retrieval': {
    icon: Search,
    badgeText: '检索',
    width: 280,
    height: 76,
    classNames: {
      ...COMMON_LOGIC_CLASSES,
      iconBg: NODE_ICON_TONES.agent,
    },
    miniMapColor: '#f43f5e',
    handles: 'both',
    resizable: false,
    autoHeight: true,
  },
  llm: {
    icon: Brain,
    badgeText: 'LLM',
    width: 280,
    height: 88,
    classNames: {
      ...COMMON_LOGIC_CLASSES,
      iconBg: NODE_ICON_TONES.agent,
    },
    miniMapColor: '#2563eb',
    handles: 'both',
    resizable: false,
    autoHeight: true,
  },
  'http-request': {
    icon: Globe,
    badgeText: 'HTTP',
    width: 280,
    height: 106,
    classNames: {
      ...COMMON_LOGIC_CLASSES,
      iconBg: NODE_ICON_TONES.extension,
    },
    miniMapColor: '#64748b',
    handles: 'both',
    resizable: false,
    autoHeight: true,
  },
  'create-scheduled-task': {
    icon: Clock3,
    badgeText: 'TASK',
    width: 280,
    height: 112,
    classNames: {
      ...COMMON_LOGIC_CLASSES,
      iconBg: NODE_ICON_TONES.extension,
    },
    miniMapColor: '#64748b',
    handles: 'both',
    resizable: false,
    autoHeight: true,
  },
  'notification-sms': {
    icon: Smartphone,
    badgeText: 'SMS',
    width: 280,
    height: 106,
    classNames: {
      ...COMMON_LOGIC_CLASSES,
      iconBg: NODE_ICON_TONES.extension,
    },
    miniMapColor: '#64748b',
    handles: 'both',
    resizable: false,
    autoHeight: true,
  },
  'call-database': {
    icon: Database,
    badgeText: '数据库',
    width: 280,
    height: 124,
    classNames: {
      ...COMMON_LOGIC_CLASSES,
      iconBg: NODE_ICON_TONES.extension,
    },
    miniMapColor: '#64748b',
    handles: 'both',
    resizable: false,
    autoHeight: true,
  },
  'sql-generator': {
    icon: MessageSquareCode,
    badgeText: 'SQL',
    width: 280,
    height: 124,
    classNames: {
      ...COMMON_LOGIC_CLASSES,
      iconBg: NODE_ICON_TONES.extension,
    },
    miniMapColor: '#64748b',
    handles: 'both',
    resizable: false,
    autoHeight: true,
  },
  tools: {
    icon: Wrench,
    badgeText: 'TOOL',
    width: 280,
    height: 88,
    classNames: {
      ...COMMON_LOGIC_CLASSES,
      iconBg: NODE_ICON_TONES.tool,
    },
    miniMapColor: '#64748b',
    handles: 'both',
    resizable: false,
    autoHeight: true,
  },
  end: {
    icon: Square,
    badgeText: '结束',
    width: 280,
    height: 48,
    classNames: {
      ...COMMON_LOGIC_CLASSES,
      iconBg: NODE_ICON_TONES.danger,
    },
    miniMapColor: '#ef4444',
    handles: 'target',
    resizable: false,
    autoHeight: true,
  },
  'loop-end': {
    icon: CircleArrowOutUpRight,
    badgeText: '退出循环',
    width: 280,
    height: 48,
    classNames: {
      ...COMMON_LOGIC_CLASSES,
      iconBg: NODE_ICON_TONES.flow,
    },
    miniMapColor: '#14b8a6',
    handles: 'target',
    resizable: false,
    autoHeight: true,
  },
  answer: {
    icon: MessageCircle,
    badgeText: '直接回复',
    width: 280,
    height: 88,
    classNames: {
      ...COMMON_LOGIC_CLASSES,
      iconBg: NODE_ICON_TONES.flow,
    },
    miniMapColor: '#06b6d4',
    handles: 'both',
    resizable: false,
    autoHeight: true,
  },
  'if-else': {
    icon: GitBranch,
    badgeText: '条件',
    width: 280,
    height: 133,
    classNames: {
      ...COMMON_LOGIC_CLASSES,
      iconBg: NODE_ICON_TONES.logic,
    },
    miniMapColor: '#14b8a6',
    handles: 'source',
    resizable: false,
    autoHeight: true,
    preview: { kind: 'branch', width: 280, height: 133 },
  },
  code: {
    icon: Code2,
    badgeText: '代码',
    width: 280,
    height: 124,
    classNames: {
      ...COMMON_LOGIC_CLASSES,
      iconBg: NODE_ICON_TONES.extension,
    },
    miniMapColor: '#64748b',
    handles: 'both',
    resizable: false,
    autoHeight: true,
  },
  assigner: {
    icon: Pencil,
    badgeText: '变量赋值',
    width: 280,
    height: 88,
    classNames: {
      ...COMMON_LOGIC_CLASSES,
      iconBg: NODE_ICON_TONES.data,
    },
    miniMapColor: '#f97316',
    handles: 'both',
    resizable: false,
    autoHeight: true,
  },
  iteration: {
    icon: Repeat,
    badgeText: '迭代',
    classNames: {
      wrapper: 'rounded-xl flex flex-col h-full shadow-lg overflow-visible',
      card: 'border-slate-200/80 dark:border-slate-700/80 grow flex flex-col transition-all overflow-visible',
      title: 'text-indigo-600 dark:text-indigo-400 font-bold',
      iconBg: NODE_ICON_TONES.logic,
      desc: '',
      content: 'grow flex flex-col bg-transparent',
      badge: 'bg-indigo-100 dark:bg-indigo-900/50 text-indigo-600 dark:text-indigo-400',
    },
    miniMapColor: '#9333ea',
    handles: 'both',
    resizable: true,
    autoHeight: false,
    width: 300,
    height: 200,
    preview: { kind: 'container', width: 300, height: 200 },
  },
  'iteration-start': {
    icon: Square,
    badgeText: '',
    classNames: {
      ...COMMON_LOGIC_CLASSES,
      iconBg: NODE_ICON_TONES.logic,
    },
    miniMapColor: '#9ca3af',
    handles: 'none',
    resizable: false,
    autoHeight: true,
  },
  'document-extractor': {
    icon: FileText,
    badgeText: 'Document',
    width: 280,
    height: 76,
    classNames: {
      ...COMMON_LOGIC_CLASSES,
      iconBg: NODE_ICON_TONES.data,
    },
    miniMapColor: '#f97316',
    handles: 'both',
    resizable: false,
    autoHeight: true,
  },
  'json-parser': {
    icon: Braces,
    badgeText: 'JSON',
    width: 280,
    height: 88,
    classNames: {
      ...COMMON_LOGIC_CLASSES,
      iconBg: NODE_ICON_TONES.data,
    },
    miniMapColor: '#f97316',
    handles: 'both',
    resizable: false,
    autoHeight: true,
  },
  'image-gen': {
    icon: ImageIcon,
    badgeText: 'Image Gen',
    width: 280,
    height: 106,
    classNames: {
      ...COMMON_LOGIC_CLASSES,
      iconBg: NODE_ICON_TONES.agent,
    },
    miniMapColor: '#a855f7',
    handles: 'both',
    resizable: false,
    autoHeight: true,
  },
  approval: {
    icon: ClipboardCheck,
    badgeText: 'Approval',
    width: 280,
    height: 156,
    classNames: {
      ...COMMON_LOGIC_CLASSES,
      iconBg: NODE_ICON_TONES.logic,
    },
    miniMapColor: '#f59e0b',
    handles: 'source',
    resizable: false,
    autoHeight: true,
  },
  'question-answer': {
    icon: MessageCircleQuestion,
    badgeText: '问答',
    width: 300,
    height: 136,
    classNames: {
      ...COMMON_LOGIC_CLASSES,
      iconBg: NODE_ICON_TONES.agent,
    },
    miniMapColor: '#2563eb',
    handles: 'none',
    resizable: false,
    autoHeight: true,
  },
  note: {
    icon: StickyNote,
    badgeText: '',
    width: 240,
    height: 160,
    classNames: {
      wrapper: '',
      card: '',
      title: '',
      desc: '',
      content: '',
      badge: '',
      iconBg: '',
    },
    miniMapColor: '#facc15',
    handles: 'none',
    resizable: true,
    autoHeight: false,
  },
  default: {
    icon: Square,
    badgeText: 'Unknown',
    classNames: {
      ...COMMON_LOGIC_CLASSES,
      iconBg: NODE_ICON_TONES.terminal,
    },
    miniMapColor: '#9ca3af',
    handles: 'none',
    resizable: false,
    autoHeight: true,
    width: 280,
  },
};

// Optional per-node overrides and future extensibility point
export const NODE_CONFIG: Record<string, { handleKey?: string }> = {
  tool: { handleKey: 'tool' },
  'document-extractor': { handleKey: 'documentExtractor' },
  'parameter-extractor': { handleKey: 'parameterExtractor' },
  'json-parser': { handleKey: 'jsonParser' },
  'create-scheduled-task': { handleKey: 'createScheduledTask' },
  'notification-sms': { handleKey: 'notificationSms' },
};
