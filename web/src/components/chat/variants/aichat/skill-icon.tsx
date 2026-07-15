'use client';

import { Sparkles, type LucideProps } from 'lucide-react';
import { AI_CHAT_SKILL_ICON_BY_KEY } from './skill-icon-registry';

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
  const Icon = icon ? (AI_CHAT_SKILL_ICON_BY_KEY[icon] ?? Sparkles) : Sparkles;
  return <Icon {...props} />;
}
