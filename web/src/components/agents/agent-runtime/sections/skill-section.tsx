'use client';

import { Plus, Trash2 } from 'lucide-react';
import { getAIChatSkillDisplayInfo } from '@/components/chat/variants/aichat/skill-display';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { useT } from '@/i18n';
import type { AIChatSkillMetadata } from '@/services/types/aichat';
import { RuntimeSection } from '../runtime-section';
import type { AgentConfigSection } from '../types';

interface AgentRuntimeSkillSectionProps {
  locale: string;
  open: boolean;
  selectedSkills: AIChatSkillMetadata[];
  normalizedSelectedSkillIds: string[];
  selectableSkillsCount: number;
  isSkillsLoading: boolean;
  isSkillConfigLoading: boolean;
  onToggleSection: (section: AgentConfigSection) => void;
  onOpenSkillDialog: () => void;
  onToggleSkill: (skillId: string, checked: boolean) => void;
}

export function AgentRuntimeSkillSection({
  locale,
  open,
  selectedSkills,
  normalizedSelectedSkillIds,
  selectableSkillsCount,
  isSkillsLoading,
  isSkillConfigLoading,
  onToggleSection,
  onOpenSkillDialog,
  onToggleSkill,
}: AgentRuntimeSkillSectionProps) {
  const t = useT('agents.agentRuntime');

  return (
    <RuntimeSection
      title={t('sections.skills')}
      section="skills"
      open={open}
      onToggle={onToggleSection}
      action={
        <div className="flex items-center gap-2">
          <Badge variant="subtle">
            {t('skills.selectedCount', { count: normalizedSelectedSkillIds.length })}
          </Badge>
          <Button
            isIcon
            variant="outline"
            className="size-8"
            onClick={onOpenSkillDialog}
            aria-label={t('skills.add')}
            title={t('skills.add')}
          >
            <Plus className="size-4" />
          </Button>
        </div>
      }
    >
      {isSkillsLoading || isSkillConfigLoading ? (
        <div className="space-y-2">
          <Skeleton className="h-14 w-full" />
          <Skeleton className="h-14 w-full" />
        </div>
      ) : selectableSkillsCount === 0 ? (
        <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
          {t('skills.enablePrompt')}
        </div>
      ) : selectedSkills.length === 0 ? (
        <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
          {t('skills.emptySelected')}
        </div>
      ) : (
        <div className="space-y-2">
          {selectedSkills.map(skill => {
            const display = getAIChatSkillDisplayInfo(skill, locale);
            const removeLabel = t('skills.remove', { name: display.label });
            return (
              <div
                key={skill.skill_id}
                className="flex items-start gap-3 rounded-md border bg-background p-3"
              >
                <div className="min-w-0 flex-1">
                  <div className="truncate text-sm font-medium">{display.label}</div>
                  <div className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
                    {display.description || skill.description || skill.skill_id}
                  </div>
                  <div className="mt-1 truncate text-[11px] text-muted-foreground/70">
                    {t('skills.idLabel', { id: skill.skill_id })}
                  </div>
                </div>
                <Button
                  isIcon
                  variant="ghost"
                  className="size-7 shrink-0 text-muted-foreground hover:text-destructive"
                  onClick={() => onToggleSkill(skill.skill_id, false)}
                  aria-label={removeLabel}
                  title={removeLabel}
                >
                  <Trash2 className="size-4" />
                </Button>
              </div>
            );
          })}
        </div>
      )}
    </RuntimeSection>
  );
}
