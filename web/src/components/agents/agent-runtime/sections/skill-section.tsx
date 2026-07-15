'use client';

import { Plus, Trash2 } from 'lucide-react';
import { AIChatSkillIcon } from '@/components/chat/variants/aichat/skill-icon';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { useT } from '@/i18n';
import { RuntimeSection } from '../runtime-section';
import type { AgentConfigSection, AgentRuntimeSelectedSkillItem } from '../types';
import type { AgentBindingHealth } from '@/services/types/agent';
import { AgentBindingHealthBadge } from '../binding-health';
import { AgentRuntimeSelectionCardIcon } from '../selection-dialog';

interface AgentRuntimeSkillSectionProps {
  open: boolean;
  selectedSkillItems: AgentRuntimeSelectedSkillItem[];
  normalizedSelectedSkillIds: string[];
  selectableSkillsCount: number;
  isSkillsLoading: boolean;
  isSkillConfigLoading: boolean;
  bindingHealth?: AgentBindingHealth;
  readOnly?: boolean;
  onToggleSection: (section: AgentConfigSection) => void;
  onOpenSkillDialog: () => void;
  onToggleSkill: (skillId: string, checked: boolean) => void;
}

export function AgentRuntimeSkillSection({
  open,
  selectedSkillItems,
  normalizedSelectedSkillIds,
  selectableSkillsCount,
  isSkillsLoading,
  isSkillConfigLoading,
  bindingHealth,
  readOnly = false,
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
            {t('selectedCount', { count: normalizedSelectedSkillIds.length })}
          </Badge>
          <Button
            isIcon
            variant="outline"
            className="size-8"
            onClick={onOpenSkillDialog}
            disabled={readOnly || isSkillsLoading || isSkillConfigLoading}
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
      ) : selectableSkillsCount === 0 && selectedSkillItems.length === 0 ? (
        <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
          {t('skills.enablePrompt')}
        </div>
      ) : selectedSkillItems.length === 0 ? (
        <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
          {t('skills.emptySelected')}
        </div>
      ) : (
        <div className="space-y-2">
          {selectedSkillItems.map(skill => {
            const removeLabel = t('skills.remove', { name: skill.label });
            const healthItem = bindingHealth?.items.find(
              item => item.binding_type === 'skill' && item.resource_id === skill.skillId
            );
            return (
              <div
                key={skill.skillId}
                className="flex items-start gap-3 rounded-md border bg-background p-3"
              >
                <AgentRuntimeSelectionCardIcon>
                  <AIChatSkillIcon icon={skill.icon} />
                </AgentRuntimeSelectionCardIcon>
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <div className="min-w-0 truncate text-sm font-medium">{skill.label}</div>
                    <AgentBindingHealthBadge item={healthItem} />
                  </div>
                  <div className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
                    {skill.description || t('skills.noDescription')}
                  </div>
                </div>
                <Button
                  isIcon
                  variant="ghost"
                  className="size-7 shrink-0 text-muted-foreground hover:text-destructive"
                  onClick={() => onToggleSkill(skill.skillId, false)}
                  disabled={readOnly}
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
