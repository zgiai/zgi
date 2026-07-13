'use client';

import { useMemo, useState } from 'react';
import { getAIChatSkillDisplayInfo } from '@/components/chat/variants/aichat/skill-display';
import { AIChatSkillIcon } from '@/components/chat/variants/aichat/skill-icon';
import { Badge } from '@/components/ui/badge';
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
import { SearchInput } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
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
  hasChanges: boolean;
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
  hasChanges,
  onOpenChange,
  onToggleSkill,
  onSave,
}: AIChatSkillPreferenceDialogProps) {
  const t = useT('webapp');
  const [closeConfirmOpen, setCloseConfirmOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const hasSearchQuery = searchQuery.trim().length > 0;
  const selectedSet = useMemo(() => new Set(selectedSkillIds), [selectedSkillIds]);
  const visibleSkills = useMemo(() => {
    const query = searchQuery.trim().toLowerCase();
    if (!query) return skills;
    return skills.filter(skill => {
      const display = getAIChatSkillDisplayInfo(skill, locale);
      return [
        skill.skill_id,
        skill.name,
        skill.description,
        skill.when_to_use,
        display.label,
        display.description,
        display.whenToUse,
        ...display.tags,
      ]
        .filter(Boolean)
        .join(' ')
        .toLowerCase()
        .includes(query);
    });
  }, [locale, searchQuery, skills]);

  const requestClose = () => {
    if (isSaving) return;
    if (hasChanges) {
      setCloseConfirmOpen(true);
      return;
    }
    onOpenChange(false);
  };

  const closeWithoutConfirm = () => {
    if (isSaving) return;
    setCloseConfirmOpen(false);
    onOpenChange(false);
  };

  const handleOpenChange = (nextOpen: boolean) => {
    if (nextOpen) {
      onOpenChange(true);
      return;
    }
    requestClose();
  };

  const handleSaveAndClose = () => {
    if (isSaving) return;
    setCloseConfirmOpen(false);
    onSave();
  };

  const handleCancelCloseConfirm = () => {
    if (isSaving) return;
    setCloseConfirmOpen(false);
  };

  const handleDirectClose = () => {
    if (isSaving) return;
    setCloseConfirmOpen(false);
    onOpenChange(false);
  };

  return (
    <>
      <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogContent size="xl">
          <DialogHeader>
            <DialogTitle>{t('consoleChat.skillPreferences.title')}</DialogTitle>
            <DialogDescription>{t('consoleChat.skillPreferences.description')}</DialogDescription>
          </DialogHeader>
          <DialogBody className="max-h-[min(680px,calc(100vh-13rem))] space-y-4">
            <div className="flex flex-col gap-3 rounded-md border bg-muted/20 p-3 sm:flex-row sm:items-center sm:justify-between">
              <SearchInput
                value={searchQuery}
                onChange={event => setSearchQuery(event.target.value)}
                placeholder={t('consoleChat.skillPreferences.searchPlaceholder')}
                className="h-9 rounded-md bg-background sm:max-w-sm"
                disabled={isSaving}
              />
              {hasSearchQuery ? (
                <Badge variant="outline" className="h-8 rounded-md font-normal">
                  {t('consoleChat.skillPreferences.matchingCount', {
                    count: visibleSkills.length,
                  })}
                </Badge>
              ) : null}
            </div>
            {isLoading ? (
              <div className="space-y-3">
                <Skeleton className="h-10 w-full" />
                <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
                  {Array.from({ length: 6 }).map((_, index) => (
                    <Skeleton key={index} className="h-36 rounded-md" />
                  ))}
                </div>
              </div>
            ) : skills.length === 0 ? (
              <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
                {t('consoleChat.skillPreferences.empty')}
              </div>
            ) : visibleSkills.length === 0 ? (
              <div className="rounded-md border border-dashed p-6 text-sm text-muted-foreground">
                {t('consoleChat.skillPreferences.noResults')}
              </div>
            ) : (
              <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
                {visibleSkills.map(skill => {
                  const display = getAIChatSkillDisplayInfo(skill, locale);
                  const checked = selectedSet.has(skill.skill_id);
                  return (
                    <div
                      key={skill.skill_id}
                      role="switch"
                      aria-checked={checked}
                      aria-label={display.label}
                      aria-disabled={isSaving}
                      tabIndex={isSaving ? -1 : 0}
                      onClick={() => {
                        if (!isSaving) onToggleSkill(skill.skill_id, !checked);
                      }}
                      onKeyDown={event => {
                        if (isSaving || (event.key !== 'Enter' && event.key !== ' ')) return;
                        event.preventDefault();
                        onToggleSkill(skill.skill_id, !checked);
                      }}
                      className={cn(
                        'flex min-h-36 cursor-pointer flex-col rounded-md border p-3.5 text-left shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30',
                        checked
                          ? cn(
                              'border-primary bg-primary/5 shadow-primary/10',
                              !isSaving && 'hover:border-primary hover:bg-primary/10'
                            )
                          : cn(
                              'border-border bg-card',
                              !isSaving && 'hover:border-primary/30 hover:bg-muted/20'
                            ),
                        isSaving ? 'cursor-not-allowed opacity-70' : ''
                      )}
                    >
                      <div className="flex items-start gap-3">
                        <span className="flex size-8 shrink-0 items-center justify-center rounded-md border bg-background text-muted-foreground">
                          <AIChatSkillIcon icon={display.icon} className="size-4" />
                        </span>
                        <div className="min-w-0 flex-1">
                          <div className="flex items-start justify-between gap-3">
                            <div className="min-w-0">
                              <h3 className="truncate text-sm font-semibold text-foreground">
                                {display.label}
                              </h3>
                              <p className="mt-0.5 line-clamp-1 text-xs text-muted-foreground">
                                {display.category || skill.source || 'Skill'}
                              </p>
                            </div>
                            <Switch
                              checked={checked}
                              disabled
                              aria-hidden="true"
                              tabIndex={-1}
                              className="pointer-events-none disabled:cursor-default disabled:opacity-100"
                            />
                          </div>
                        </div>
                      </div>

                      <div className="mt-3 flex flex-wrap gap-1.5">
                        <Badge
                          variant={checked ? 'success' : 'subtle'}
                          className="rounded-md font-normal"
                        >
                          {checked
                            ? t('consoleChat.skillPreferences.enabled')
                            : t('consoleChat.skillPreferences.disabled')}
                        </Badge>
                        {display.tags.slice(0, 2).map(tag => (
                          <Badge key={tag} variant="outline" className="rounded-md font-normal">
                            {tag}
                          </Badge>
                        ))}
                      </div>

                      <p className="mt-2.5 line-clamp-3 text-sm leading-5 text-muted-foreground">
                        {display.description || skill.description}
                      </p>
                    </div>
                  );
                })}
              </div>
            )}
          </DialogBody>
          <DialogFooter className="items-center justify-between gap-3">
            <div className="mr-auto text-xs text-muted-foreground">
              {t('consoleChat.skillPreferences.selectedCount', {
                count: selectedSkillIds.length,
              })}
            </div>
            <Button variant="outline" onClick={closeWithoutConfirm} disabled={isSaving}>
              {t('consoleChat.skillPreferences.cancel')}
            </Button>
            <Button onClick={onSave} disabled={isSaving || !hasChanges}>
              {isSaving
                ? t('consoleChat.skillPreferences.saving')
                : t('consoleChat.skillPreferences.save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={closeConfirmOpen} onOpenChange={setCloseConfirmOpen}>
        <DialogContent size="sm" className="p-0">
          <DialogHeader>
            <DialogTitle>{t('consoleChat.skillPreferences.closeConfirm.title')}</DialogTitle>
            <DialogDescription>
              {t('consoleChat.skillPreferences.closeConfirm.description')}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="flex-col gap-2 border-t bg-muted/40 sm:flex-row sm:justify-end">
            <Button variant="outline" onClick={handleDirectClose} disabled={isSaving}>
              {t('consoleChat.skillPreferences.closeConfirm.directClose')}
            </Button>
            <Button variant="ghost" onClick={handleCancelCloseConfirm} disabled={isSaving}>
              {t('consoleChat.skillPreferences.closeConfirm.cancel')}
            </Button>
            <Button onClick={handleSaveAndClose} disabled={isSaving}>
              {isSaving
                ? t('consoleChat.skillPreferences.saving')
                : t('consoleChat.skillPreferences.closeConfirm.saveAndClose')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
