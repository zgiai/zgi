'use client';

import {
  Brain,
  Calculator,
  CalendarDays,
  ChartNoAxesCombined,
  Clock,
  ClipboardList,
  Database,
  FolderCog,
  FilePlus,
  FileText,
  Library,
  Route,
  Sparkles,
  Wrench,
  Workflow,
  type LucideIcon,
  type LucideProps,
} from 'lucide-react';

const SKILL_ICON_BY_KEY: Record<string, LucideIcon> = {
  brain: Brain,
  calculator: Calculator,
  'calendar-days': CalendarDays,
  'chart-no-axes-combined': ChartNoAxesCombined,
  clock: Clock,
  'clipboard-list': ClipboardList,
  database: Database,
  file: FileText,
  'file-generator': FileText,
  'file-manager': FolderCog,
  'file-plus': FilePlus,
  'file-text': FileText,
  'folder-cog': FolderCog,
  library: Library,
  route: Route,
  time: Clock,
  tools: Wrench,
  wrench: Wrench,
  workflow: Workflow,
  sparkles: Sparkles,
};

interface AIChatSkillIconProps extends LucideProps {
  icon?: string;
}

/**
 * @component AIChatSkillIcon
 * @category Feature
 * @status Stable
 * @description Renders Skill V2 display icon keys with a safe fallback.
 * @usage Use in AIChat skill selection and trace UI.
 * @example
 * <AIChatSkillIcon icon="calculator" className="size-4" />
 */
export function AIChatSkillIcon({ icon, ...props }: AIChatSkillIconProps) {
  const Icon = icon ? (SKILL_ICON_BY_KEY[icon] ?? Sparkles) : Sparkles;
  return <Icon {...props} />;
}
