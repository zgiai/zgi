'use client';

import { useState } from 'react';
import { Loader2, Plus, Sparkles, Trash2 } from 'lucide-react';
import { getAIChatSkillDisplayInfo } from '@/components/chat/variants/aichat/skill-display';
import {
  ModelSelectorParameter,
  type ModelSelectorParameterValue,
} from '@/components/common/model-selector';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
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
import { ScrollArea } from '@/components/ui/scroll-area';
import { Separator } from '@/components/ui/separator';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { AIChatSkillMetadata } from '@/services/types/aichat';
import type { AgentMemorySlotConfig } from '@/services/types/agent';
import type { Dataset } from '@/services/types/dataset';
import { AGENT_HOME_TITLE_MAX_LENGTH, AGENT_INPUT_PLACEHOLDER_MAX_LENGTH } from './constants';
import { RuntimeSection } from './runtime-section';
import type { AgentConfigSection } from './types';
import type { AgentMemorySlotValidationError } from './utils';

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
  isDatasetsLoading: boolean;
  availableDatasets: Dataset[];
  selectedKnowledgeDatasetIds: string[];
  suggestedQuestions: string[];
  isGeneratingSuggestions: boolean;
  systemPrompt: string;
  fileUploadEnabled: boolean;
  agentMemoryEnabled: boolean;
  agentMemorySlots: AgentMemorySlotConfig[];
  agentMemorySlotValidationErrors: AgentMemorySlotValidationError[];
  defaultHomeTitle: string;
  defaultInputPlaceholder: string;
  className?: string;
  scrollAreaClassName?: string;
  scrollViewportClassName?: string;
  onToggleSection: (section: AgentConfigSection) => void;
  onChangeModelValue: (value: ModelSelectorParameterValue) => void;
  onChangeHomeTitle: (value: string) => void;
  onChangeInputPlaceholder: (value: string) => void;
  onOpenSkillDialog: () => void;
  onToggleSkill: (skillId: string, checked: boolean) => void;
  onToggleKnowledgeDataset: (datasetId: string, checked: boolean) => void;
  onGenerateSuggestedQuestions: () => void;
  onChangeSuggestedQuestions: (value: string[]) => void;
  onChangeFileUploadEnabled: (value: boolean) => void;
  onChangeAgentMemoryEnabled: (value: boolean) => void;
  onChangeAgentMemorySlots: (value: AgentMemorySlotConfig[]) => void;
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
  isDatasetsLoading,
  availableDatasets,
  selectedKnowledgeDatasetIds,
  suggestedQuestions,
  isGeneratingSuggestions,
  systemPrompt,
  fileUploadEnabled,
  agentMemoryEnabled,
  agentMemorySlots,
  agentMemorySlotValidationErrors,
  defaultHomeTitle,
  defaultInputPlaceholder,
  className,
  scrollAreaClassName,
  scrollViewportClassName,
  onToggleSection,
  onChangeModelValue,
  onChangeHomeTitle,
  onChangeInputPlaceholder,
  onOpenSkillDialog,
  onToggleSkill,
  onToggleKnowledgeDataset,
  onGenerateSuggestedQuestions,
  onChangeSuggestedQuestions,
  onChangeFileUploadEnabled,
  onChangeAgentMemoryEnabled,
  onChangeAgentMemorySlots,
}: AgentRuntimeOrchestrationPanelProps) {
  const t = useT('agents.agentRuntime');
  const [pendingRemoveMemoryIndex, setPendingRemoveMemoryIndex] = useState<number | null>(null);
  const [memoryItemDialogOpen, setMemoryItemDialogOpen] = useState(false);
  const [newMemoryKey, setNewMemoryKey] = useState('');
  const [newMemoryDescription, setNewMemoryDescription] = useState('');
  const usedAgentMemorySlotKeys = new Set(
    agentMemorySlots.map(slot => slot.key.trim().toLowerCase()).filter(Boolean)
  );
  const nextAgentMemorySlotKey = (() => {
    for (let index = agentMemorySlots.length + 1; index <= 5; index += 1) {
      const candidate = `memory_${index}`;
      if (!usedAgentMemorySlotKeys.has(candidate)) return candidate;
    }
    return `memory_${Date.now().toString(36).slice(-6)}`;
  })();
  const addAgentMemorySlot = () => {
    if (agentMemorySlots.length >= 5) return;
    const key = newMemoryKey.trim().toLowerCase();
    if (!key) return;
    onChangeAgentMemorySlots([
      ...agentMemorySlots,
      {
        key,
        description: newMemoryDescription.trim().slice(0, 200),
        max_chars: 2000,
        enabled: true,
        sort_order: agentMemorySlots.length,
      },
    ]);
    setNewMemoryKey('');
    setNewMemoryDescription('');
    setMemoryItemDialogOpen(false);
  };
  const updateAgentMemorySlot = (index: number, patch: Partial<AgentMemorySlotConfig>) => {
    onChangeAgentMemorySlots(
      agentMemorySlots.map((slot, currentIndex) =>
        currentIndex === index ? { ...slot, ...patch } : slot
      )
    );
  };
  const removeAgentMemorySlot = (index: number) => {
    onChangeAgentMemorySlots(agentMemorySlots.filter((_, currentIndex) => currentIndex !== index));
  };
  const getAgentMemorySlotErrorText = (error: AgentMemorySlotValidationError) => {
    if (!error) return '';
    return t(`memory.validation.${error}`);
  };
  const normalizedNewMemoryKey = newMemoryKey.trim().toLowerCase();
  const newMemoryKeyError = (() => {
    if (!memoryItemDialogOpen) return null;
    if (!normalizedNewMemoryKey) return 'required';
    if (!/^[a-z][a-z0-9_]*$/.test(normalizedNewMemoryKey)) return 'pattern';
    if (usedAgentMemorySlotKeys.has(normalizedNewMemoryKey)) return 'duplicate';
    return null;
  })() as AgentMemorySlotValidationError;

  return (
    <section className={cn('flex min-w-0 flex-col overflow-hidden', className)}>
      <ConfirmDialog
        open={pendingRemoveMemoryIndex !== null}
        onOpenChange={open => {
          if (!open) setPendingRemoveMemoryIndex(null);
        }}
        title={t('memory.deleteConfirmTitle')}
        description={t('memory.deleteConfirmDescription')}
        confirmText={t('memory.deleteConfirmAction')}
        cancelText={t('memory.deleteConfirmCancel')}
        variant="warning"
        onConfirm={() => {
          if (pendingRemoveMemoryIndex !== null) {
            removeAgentMemorySlot(pendingRemoveMemoryIndex);
          }
          setPendingRemoveMemoryIndex(null);
        }}
      />
      <Dialog open={memoryItemDialogOpen} onOpenChange={setMemoryItemDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('memory.addDialogTitle')}</DialogTitle>
            <DialogDescription>{t('memory.addDialogDescription')}</DialogDescription>
          </DialogHeader>
          <DialogBody className="space-y-4">
            <div className="space-y-1.5">
              <Input
                value={newMemoryKey}
                maxLength={64}
                placeholder={nextAgentMemorySlotKey}
                error={Boolean(newMemoryKeyError)}
                errorText={newMemoryKeyError ? t(`memory.validation.${newMemoryKeyError}`) : null}
                onChange={event => setNewMemoryKey(event.target.value.toLowerCase().slice(0, 64))}
              />
            </div>
            <div className="space-y-1.5">
              <Textarea
                value={newMemoryDescription}
                maxLength={200}
                showCharacterCount
                className="min-h-24"
                placeholder={t('memory.slotDescriptionPlaceholder')}
                onChange={event => setNewMemoryDescription(event.target.value.slice(0, 200))}
              />
            </div>
          </DialogBody>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setMemoryItemDialogOpen(false)}>
              {t('memory.addDialogCancel')}
            </Button>
            <Button
              onClick={addAgentMemorySlot}
              disabled={Boolean(newMemoryKeyError) || agentMemorySlots.length >= 5}
            >
              {t('memory.addDialogConfirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <div className="flex h-12 shrink-0 items-center justify-between px-5">
        <div>
          <h2 className="text-sm font-semibold">{t('orchestration.title')}</h2>
          <p className="text-xs text-muted-foreground">{t('orchestration.description')}</p>
        </div>
      </div>
      <ScrollArea
        className={cn('min-h-0 flex-1', scrollAreaClassName)}
        viewportProps={scrollViewportClassName ? { className: scrollViewportClassName } : undefined}
      >
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
            title={t('sections.knowledge')}
            section="knowledge"
            open={openSections.knowledge}
            onToggle={onToggleSection}
            action={<Badge variant="subtle">{selectedKnowledgeDatasetIds.length}</Badge>}
          >
            {isDatasetsLoading ? (
              <div className="space-y-2">
                <Skeleton className="h-12 w-full" />
                <Skeleton className="h-12 w-full" />
              </div>
            ) : availableDatasets.length === 0 ? (
              <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
                {t('knowledge.empty')}
              </div>
            ) : (
              <div className="space-y-2">
                {availableDatasets.map(dataset => {
                  const checked = selectedKnowledgeDatasetIds.includes(dataset.id);
                  return (
                    <label
                      key={dataset.id}
                      className="flex cursor-pointer items-start gap-3 rounded-md border bg-background p-3"
                    >
                      <Checkbox
                        checked={checked}
                        onCheckedChange={value =>
                          onToggleKnowledgeDataset(dataset.id, value === true)
                        }
                      />
                      <span className="min-w-0 flex-1">
                        <span className="block truncate text-sm font-medium">{dataset.name}</span>
                        <span className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
                          {dataset.description || t('knowledge.noDescription')}
                        </span>
                      </span>
                    </label>
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
            <div className="space-y-3">
              <div className="space-y-3 rounded-md border p-3">
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <div className="text-sm font-medium">{t('memory.agentTitle')}</div>
                    <div className="text-xs text-muted-foreground">
                      {t('memory.agentDescription')}
                    </div>
                  </div>
                  <Switch
                    checked={agentMemoryEnabled}
                    onCheckedChange={onChangeAgentMemoryEnabled}
                  />
                </div>
                {agentMemoryEnabled && (
                  <div className="space-y-2">
                    {agentMemorySlots.length === 0 ? (
                      <div className="rounded-md border border-dashed p-3 text-xs text-muted-foreground">
                        {t('memory.emptySlots')}
                      </div>
                    ) : (
                      agentMemorySlots.map((slot, index) => {
                        const keyError = agentMemorySlotValidationErrors[index] ?? null;
                        const keyErrorText = getAgentMemorySlotErrorText(keyError);
                        return (
                          <div
                            key={`${slot.id ?? 'slot'}-${index}`}
                            className="space-y-3 rounded-md border p-3"
                          >
                            <div className="flex items-center justify-between gap-3">
                              <div className="min-w-0 flex-1">
                                <div className="truncate text-sm font-semibold">{slot.key}</div>
                                {keyErrorText && (
                                  <div className="mt-1 text-xs text-destructive">{keyErrorText}</div>
                                )}
                              </div>
                              <div className="flex shrink-0 items-center gap-2">
                                <Switch
                                  checked={slot.enabled}
                                  onCheckedChange={checked =>
                                    updateAgentMemorySlot(index, { enabled: checked })
                                  }
                                />
                                <Button
                                  type="button"
                                  variant="ghost"
                                  isIcon
                                  aria-label={t('memory.removeSlot')}
                                  onClick={() => {
                                    if (slot.id) {
                                      setPendingRemoveMemoryIndex(index);
                                      return;
                                    }
                                    removeAgentMemorySlot(index);
                                  }}
                                >
                                  <Trash2 className="size-4" />
                                </Button>
                              </div>
                            </div>
                            <div className="space-y-1">
                              <div className="text-xs font-medium text-muted-foreground">
                                {t('memory.descriptionLabel')}
                              </div>
                              <Textarea
                                value={slot.description}
                                maxLength={200}
                                showCharacterCount
                                className="min-h-20"
                                placeholder={t('memory.slotDescriptionPlaceholder')}
                                onChange={event =>
                                  updateAgentMemorySlot(index, {
                                    description: event.target.value.slice(0, 200),
                                  })
                                }
                              />
                              <div className="text-[11px] text-muted-foreground">
                                {t('memory.descriptionHelp')}
                              </div>
                            </div>
                          </div>
                        );
                      })
                    )}
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        setNewMemoryKey(nextAgentMemorySlotKey);
                        setNewMemoryDescription('');
                        setMemoryItemDialogOpen(true);
                      }}
                      disabled={agentMemorySlots.length >= 5}
                    >
                      <Plus className="size-4" />
                      {agentMemorySlots.length >= 5
                        ? t('memory.maxItemsReached')
                        : t('memory.addSlot')}
                    </Button>
                  </div>
                )}
              </div>
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
                  showCharacterCount
                  placeholder={defaultHomeTitle}
                  onChange={event =>
                    onChangeHomeTitle(
                      Array.from(event.target.value).slice(0, AGENT_HOME_TITLE_MAX_LENGTH).join('')
                    )
                  }
                />
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
                  showCharacterCount
                  placeholder={defaultInputPlaceholder}
                  onChange={event =>
                    onChangeInputPlaceholder(
                      Array.from(event.target.value)
                        .slice(0, AGENT_INPUT_PLACEHOLDER_MAX_LENGTH)
                        .join('')
                    )
                  }
                />
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
