'use client';

import { Check, Settings2 } from 'lucide-react';
import { getAIChatSkillDisplayInfo } from '@/components/chat/variants/aichat/skill-display';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import type { AIChatSkillMetadata } from '@/services/types/aichat';

interface AIChatSkillPreferenceDialogProps {
  open: boolean;
  locale: string;
  skills: AIChatSkillMetadata[];
  selectedSkillIds: string[];
  isLoading: boolean;
  isSaving: boolean;
  onOpenChange: (open: boolean) => void;
  onToggleSkill: (skillId: string, checked: boolean) => void;
  onSave: () => void;
}

export function AIChatSkillPreferenceDialog({
  open,
  locale,
  skills,
  selectedSkillIds,
  isLoading,
  isSaving,
  onOpenChange,
  onToggleSkill,
  onSave,
}: AIChatSkillPreferenceDialogProps) {
  const t = useT('webapp');

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg">
        <DialogHeader>
          <DialogTitle>{t('consoleChat.skillPreferences.title')}</DialogTitle>
          <DialogDescription>{t('consoleChat.skillPreferences.description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="max-h-[520px]">
          {isLoading ? (
            <div className="space-y-3">
              <Skeleton className="h-20 w-full" />
              <Skeleton className="h-20 w-full" />
            </div>
          ) : skills.length === 0 ? (
            <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
              {t('consoleChat.skillPreferences.empty')}
            </div>
          ) : (
            <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
              {skills.map(skill => {
                const display = getAIChatSkillDisplayInfo(skill, locale);
                const checked = selectedSkillIds.includes(skill.skill_id);
                return (
                  <button
                    key={skill.skill_id}
                    type="button"
                    className={cn(
                      'flex min-h-24 cursor-pointer items-start gap-3 rounded-lg border bg-background p-4 text-left transition-colors hover:border-primary/50 hover:bg-muted/30',
                      checked ? 'border-primary bg-primary/5' : ''
                    )}
                    onClick={() => onToggleSkill(skill.skill_id, !checked)}
                  >
                    <span className="flex size-9 shrink-0 items-center justify-center rounded-md border bg-muted">
                      <Settings2 className="size-4 text-muted-foreground" />
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-semibold">{display.label}</span>
                      <span className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
                        {display.description || skill.description || skill.skill_id}
                      </span>
                    </span>
                    <span
                      className={cn(
                        'flex size-5 shrink-0 items-center justify-center rounded-full border',
                        checked
                          ? 'border-primary bg-primary text-primary-foreground'
                          : 'bg-background'
                      )}
                    >
                      {checked ? <Check className="size-3.5" /> : null}
                    </span>
                  </button>
                );
              })}
            </div>
          )}
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('consoleChat.skillPreferences.cancel')}
          </Button>
          <Button onClick={onSave} disabled={isSaving}>
            {t('consoleChat.skillPreferences.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
