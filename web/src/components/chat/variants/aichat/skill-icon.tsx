'use client';

import {
  Calculator,
  Clock,
  FileText,
  Sparkles,
  Wrench,
  type LucideIcon,
  type LucideProps,
} from 'lucide-react';

const SKILL_ICON_BY_KEY: Record<string, LucideIcon> = {
  calculator: Calculator,
  clock: Clock,
  file: FileText,
  'file-generator': FileText,
  'file-text': FileText,
  time: Clock,
  tools: Wrench,
  wrench: Wrench,
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
