'use client';

import Link from 'next/link';
import { ExternalLink, Loader2, Plus, Sparkles, Trash2 } from 'lucide-react';
import { getAIChatSkillDisplayInfo } from '@/components/chat/variants/aichat/skill-display';
import { ModelSelectorParameter, type ModelSelectorParameterValue } from '@/components/common/model-selector';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Separator } from '@/components/ui/separator';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import { useT } from '@/i18n';
import type { AIChatSkillMetadata } from '@/services/types/aichat';
import {
  AGENT_HOME_TITLE_MAX_LENGTH,
  AGENT_INPUT_PLACEHOLDER_MAX_LENGTH,
} from './constants';
import { RuntimeSection } from './runtime-section';
import type { AgentConfigSection } from './types';

interface AgentRuntimeOrchestrationPanelProps {
  locale: string;
  openSections: Record<AgentConfigSection, boolean>;
  modelValue: ModelSelectorParameterValue;
  homeTitle: string;
  inputPlaceholder: string;
  selectedSkills: AIChatSkillMetadata[];
  normalizedSelectedSkillIds: string[];
  selectableSkillsCount: number;
  isSkillsLoading: boolean;
  isSkillConfigLoading: boolean;
  suggestedQuestions: string[];
  isGeneratingSuggestions: boolean;
  systemPrompt: string;
  fileUploadEnabled: boolean;
  useMemory: boolean;
  defaultHomeTitle: string;
  defaultInputPlaceholder: string;
  onToggleSection: (section: AgentConfigSection) => void;
  onChangeModelValue: (value: ModelSelectorParameterValue) => void;
  onChangeHomeTitle: (value: string) => void;
  onChangeInputPlaceholder: (value: string) => void;
  onOpenSkillDialog: () => void;
  onToggleSkill: (skillId: string, checked: boolean) => void;
  onGenerateSuggestedQuestions: () => void;
  onChangeSuggestedQuestions: (value: string[]) => void;
  onChangeFileUploadEnabled: (value: boolean) => void;
  onChangeUseMemory: (value: boolean) => void;
}

export function AgentRuntimeOrchestrationPanel({
  locale,
  openSections,
  modelValue,
  homeTitle,
  inputPlaceholder,
  selectedSkills,
  normalizedSelectedSkillIds,
  selectableSkillsCount,
  isSkillsLoading,
  isSkillConfigLoading,
  suggestedQuestions,
  isGeneratingSuggestions,
  systemPrompt,
  fileUploadEnabled,
  useMemory,
  defaultHomeTitle,
  defaultInputPlaceholder,
  onToggleSection,
  onChangeModelValue,
  onChangeHomeTitle,
  onChangeInputPlaceholder,
  onOpenSkillDialog,
  onToggleSkill,
  onGenerateSuggestedQuestions,
  onChangeSuggestedQuestions,
  onChangeFileUploadEnabled,
  onChangeUseMemory,
}: AgentRuntimeOrchestrationPanelProps) {
  const t = useT('agents.agentRuntime');

  return (
    <section className="flex min-w-0 flex-col overflow-hidden">
      <div className="flex h-12 shrink-0 items-center justify-between px-5">
        <div>
          <h2 className="text-sm font-semibold">{t('orchestration.title')}</h2>
          <p className="text-xs text-muted-foreground">{t('orchestration.description')}</p>
        </div>
      </div>
      <ScrollArea className="min-h-0 flex-1">
        <div className="space-y-5 px-5 pb-6">
          <RuntimeSection
            title={t('sections.model')}
            section="model"
            open={openSections.model}
            onToggle={onToggleSection}
          >
            <ModelSelectorParameter
              modelType="text-chat"
              value={modelValue}
              onChange={onChangeModelValue}
              className="w-full"
            />
          </RuntimeSection>

          <Separator className="h-px" />

          <RuntimeSection
            title={t('sections.skills')}
            section="skills"
            open={openSections.skills}
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
                <Button asChild variant="link" className="h-auto px-1 text-sm">
                  <Link href="/dashboard/organization/aichat-skills">
                    {t('skills.enableAction')}
                    <ExternalLink className="ml-1 size-3.5" />
                  </Link>
                </Button>
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

          <Separator className="h-px" />

          <RuntimeSection
            title={t('sections.files')}
            section="files"
            open={openSections.files}
            onToggle={onToggleSection}
          >
            <div className="flex items-center justify-between rounded-md border p-3">
              <div>
                <div className="text-sm font-medium">{t('files.title')}</div>
                <div className="text-xs text-muted-foreground">{t('files.description')}</div>
              </div>
              <Switch checked={fileUploadEnabled} onCheckedChange={onChangeFileUploadEnabled} />
            </div>
          </RuntimeSection>

          <Separator className="h-px" />

          <RuntimeSection
            title={t('sections.memory')}
            section="memory"
            open={openSections.memory}
            onToggle={onToggleSection}
          >
            <div className="flex items-center justify-between rounded-md border p-3">
              <div>
                <div className="text-sm font-medium">{t('memory.title')}</div>
                <div className="text-xs text-muted-foreground">{t('memory.description')}</div>
              </div>
              <Switch checked={useMemory} onCheckedChange={onChangeUseMemory} />
            </div>
          </RuntimeSection>

          <Separator className="h-px" />

          <RuntimeSection
            title={t('sections.experience')}
            section="experience"
            open={openSections.experience}
            onToggle={onToggleSection}
          >
            <div className="space-y-3">
              <div className="text-xs font-semibold text-muted-foreground">
                {t('experience.homeGroup')}
              </div>
              <div className="space-y-1.5">
                <div className="text-xs font-medium text-muted-foreground">
                  {t('appearance.homeTitle')}
                </div>
                <Input
                  value={homeTitle}
                  maxLength={AGENT_HOME_TITLE_MAX_LENGTH}
                  placeholder={defaultHomeTitle}
                  onChange={event =>
                    onChangeHomeTitle(
                      Array.from(event.target.value).slice(0, AGENT_HOME_TITLE_MAX_LENGTH).join('')
                    )
                  }
                />
                <div className="text-right text-xs text-muted-foreground">
                  {Array.from(homeTitle).length}/{AGENT_HOME_TITLE_MAX_LENGTH}
                </div>
              </div>
            </div>

            <div className="space-y-3 pt-2">
              <div className="text-xs font-semibold text-muted-foreground">
                {t('experience.inputGroup')}
              </div>
              <div className="space-y-1.5">
                <div className="text-xs font-medium text-muted-foreground">
                  {t('appearance.inputPlaceholder')}
                </div>
                <Input
                  value={inputPlaceholder}
                  maxLength={AGENT_INPUT_PLACEHOLDER_MAX_LENGTH}
                  placeholder={defaultInputPlaceholder}
                  onChange={event =>
                    onChangeInputPlaceholder(
                      Array.from(event.target.value)
                        .slice(0, AGENT_INPUT_PLACEHOLDER_MAX_LENGTH)
                        .join('')
                    )
                  }
                />
                <div className="text-right text-xs text-muted-foreground">
                  {Array.from(inputPlaceholder).length}/{AGENT_INPUT_PLACEHOLDER_MAX_LENGTH}
                </div>
              </div>
            </div>

            <div className="space-y-3 pt-2">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <div className="text-xs font-semibold text-muted-foreground">
                    {t('experience.questionsGroup')}
                  </div>
                  <div className="mt-0.5 text-xs text-muted-foreground">
                    {t('suggestions.help')}
                  </div>
                </div>
                <div className="flex shrink-0 items-center gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    className="h-8 gap-1.5 px-2 text-xs"
                    onClick={onGenerateSuggestedQuestions}
                    disabled={isGeneratingSuggestions || !systemPrompt.trim() || !modelValue.model}
                  >
                    {isGeneratingSuggestions ? (
                      <Loader2 className="size-3.5 animate-spin" />
                    ) : (
                      <Sparkles className="size-3.5" />
                    )}
                    {t('suggestions.generate')}
                  </Button>
                  <Button
                    isIcon
                    variant="outline"
                    className="size-8"
                    onClick={() =>
                      onChangeSuggestedQuestions(
                        suggestedQuestions.length >= 6
                          ? suggestedQuestions
                          : [...suggestedQuestions, '']
                      )
                    }
                    disabled={suggestedQuestions.length >= 6}
                    aria-label={t('suggestions.add')}
                    title={t('suggestions.add')}
                  >
                    <Plus className="size-4" />
                  </Button>
                </div>
              </div>
              {suggestedQuestions.length === 0 ? (
                <div className="space-y-3 rounded-md border border-dashed p-4 text-sm text-muted-foreground">
                  <div>{t('suggestions.empty')}</div>
                  <div className="flex flex-wrap gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-8 gap-1.5 px-2 text-xs"
                      onClick={onGenerateSuggestedQuestions}
                      disabled={
                        isGeneratingSuggestions || !systemPrompt.trim() || !modelValue.model
                      }
                    >
                      {isGeneratingSuggestions ? (
                        <Loader2 className="size-3.5 animate-spin" />
                      ) : (
                        <Sparkles className="size-3.5" />
                      )}
                      {t('suggestions.generate')}
                    </Button>
                    <Button
                      variant="outline"
                      size="sm"
                      className="h-8 gap-1.5 px-2 text-xs"
                      onClick={() => onChangeSuggestedQuestions([''])}
                    >
                      <Plus className="size-3.5" />
                      {t('suggestions.manualAdd')}
                    </Button>
                  </div>
                </div>
              ) : (
                suggestedQuestions.map((question, index) => (
                  <div key={index} className="flex items-center gap-2">
                    <Input
                      value={question}
                      maxLength={200}
                      placeholder={t('suggestions.placeholder')}
                      onChange={event =>
                        onChangeSuggestedQuestions(
                          suggestedQuestions.map((item, itemIndex) =>
                            itemIndex === index ? event.target.value : item
                          )
                        )
                      }
                    />
                    <Button
                      isIcon
                      variant="ghost"
                      className="size-8 shrink-0 text-muted-foreground hover:text-destructive"
                      onClick={() =>
                        onChangeSuggestedQuestions(
                          suggestedQuestions.filter((_, itemIndex) => itemIndex !== index)
                        )
                      }
                      aria-label={t('suggestions.delete')}
                      title={t('suggestions.delete')}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </div>
                ))
              )}
            </div>
          </RuntimeSection>
        </div>
      </ScrollArea>
    </section>
  );
}
