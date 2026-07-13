'use client';

import { Check, Search } from 'lucide-react';
import { getAIChatSkillDisplayInfo } from '@/components/chat/variants/aichat/skill-display';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { AIChatSkillMetadata } from '@/services/types/aichat';

interface AgentRuntimeSkillDialogProps {
  open: boolean;
  locale: string;
  selectableSkillsCount: number;
  dialogSkills: AIChatSkillMetadata[];
  normalizedSelectedSkillIds: string[];
  skillSearch: string;
  showSelectedSkillsOnly: boolean;
  onOpenChange: (open: boolean) => void;
  onChangeSkillSearch: (value: string) => void;
  onChangeShowSelectedSkillsOnly: (value: boolean) => void;
  onToggleSkill: (skillId: string, checked: boolean) => void;
}

export function AgentRuntimeSkillDialog({
  open,
  locale,
  selectableSkillsCount,
  dialogSkills,
  normalizedSelectedSkillIds,
  skillSearch,
  showSelectedSkillsOnly,
  onOpenChange,
  onChangeSkillSearch,
  onChangeShowSelectedSkillsOnly,
  onToggleSkill,
}: AgentRuntimeSkillDialogProps) {
  const t = useT('agents.agentRuntime');
  const systemSkills = dialogSkills.filter(skill => skill.source !== 'custom');
  const customSkills = dialogSkills.filter(skill => skill.source === 'custom');

  const renderSkillCards = (skills: AIChatSkillMetadata[]) => {
    if (skills.length === 0) {
      return (
        <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
          {t('skills.noMatch')}
        </div>
      );
    }

    return (
      <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
        {skills.map(skill => {
          const display = getAIChatSkillDisplayInfo(skill, locale);
          const checked = normalizedSelectedSkillIds.includes(skill.skill_id);
          return (
            <button
              key={skill.skill_id}
              type="button"
              aria-pressed={checked}
              className={cn(
                'flex min-h-32 cursor-pointer flex-col rounded-lg border bg-background p-4 text-left transition-colors disabled:cursor-not-allowed disabled:opacity-60',
                checked
                  ? 'border-primary bg-primary/5 hover:border-primary hover:bg-primary/10'
                  : 'border-border hover:border-primary/50 hover:bg-muted/30'
              )}
              onClick={() => onToggleSkill(skill.skill_id, !checked)}
            >
              <span className="flex items-start gap-3">
                <span className="flex size-10 shrink-0 items-center justify-center rounded-lg border bg-muted text-sm font-semibold">
                  {display.label.slice(0, 2).toUpperCase()}
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-semibold">{display.label}</span>
                  <span className="mt-1 inline-flex rounded border bg-muted/40 px-1.5 py-0.5 text-[11px] text-muted-foreground">
                    {skill.runtime_type}
                  </span>
                </span>
                <span
                  className={cn(
                    'flex size-5 shrink-0 items-center justify-center rounded-full border',
                    checked ? 'border-primary bg-primary text-primary-foreground' : 'bg-background'
                  )}
                >
                  {checked ? <Check className="size-3.5" /> : null}
                </span>
              </span>
              <span className="mt-3 line-clamp-2 text-xs leading-5 text-muted-foreground">
                {display.description || skill.description || skill.skill_id}
              </span>
              <span className="mt-auto pt-3 text-[11px] text-muted-foreground/70">
                {t('skills.idLabel', { id: skill.skill_id })}
              </span>
            </button>
          );
        })}
      </div>
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg">
        <DialogHeader>
          <DialogTitle>{t('skills.dialogTitle')}</DialogTitle>
          <DialogDescription>{t('skills.dialogDescription')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="max-h-[520px]">
          {selectableSkillsCount === 0 ? (
            <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
              {t('skills.enablePrompt')}
            </div>
          ) : (
            <div className="space-y-3">
              <div className="flex items-center gap-2">
                <div className="relative min-w-0 flex-1">
                  <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    value={skillSearch}
                    onChange={event => onChangeSkillSearch(event.target.value)}
                    placeholder={t('skills.searchPlaceholder')}
                    className="pl-8"
                  />
                </div>
                <label className="flex shrink-0 items-center gap-2 rounded-md border px-3 py-2 text-sm">
                  <Checkbox
                    checked={showSelectedSkillsOnly}
                    onCheckedChange={value => onChangeShowSelectedSkillsOnly(value === true)}
                  />
                  {t('skills.selectedOnly')}
                </label>
              </div>
              <Tabs defaultValue="system" className="space-y-3">
                <TabsList className="w-full justify-start">
                  <TabsTrigger value="system">
                    {t('skills.systemTab', { count: systemSkills.length })}
                  </TabsTrigger>
                  <TabsTrigger value="custom">
                    {t('skills.customTab', { count: customSkills.length })}
                  </TabsTrigger>
                </TabsList>
                <TabsContent value="system" className="mt-0">
                  {renderSkillCards(systemSkills)}
                </TabsContent>
                <TabsContent value="custom" className="mt-0">
                  {renderSkillCards(customSkills)}
                </TabsContent>
              </Tabs>
            </div>
          )}
        </DialogBody>
        <DialogFooter>
          <Button onClick={() => onOpenChange(false)}>{t('skills.done')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
