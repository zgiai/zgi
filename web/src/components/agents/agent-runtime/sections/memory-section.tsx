'use client';

import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { FlaskConical, Plus, Sparkles, Trash2 } from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
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
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { AGENT_KEYS } from '@/hooks/query-keys';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import agentService from '@/services/agent.service';
import type { AgentMemorySlotConfig, AgentMemoryTemplate } from '@/services/types/agent';
import { AgentMemoryPreviewDialog } from '../memory-preview-dialog';
import { addAgentMemoryTemplateBlock, applyAgentMemoryTemplate } from '../memory-templates';
import { RuntimeSection } from '../runtime-section';
import type { AgentConfigSection } from '../types';
import {
  AGENT_MEMORY_LIMITS,
  clampAgentMemoryMaxChars,
  type AgentMemorySlotValidationError,
} from '../utils';

const MAX_AGENT_MEMORY_SLOTS = AGENT_MEMORY_LIMITS.maxBlocks;

interface AgentRuntimeMemorySectionProps {
  agentId: string;
  open: boolean;
  agentMemoryEnabled: boolean;
  agentMemoryAutoExtractionEnabled: boolean;
  agentMemorySlots: AgentMemorySlotConfig[];
  agentMemorySlotValidationErrors: AgentMemorySlotValidationError[];
  agentMemoryInstructions: string;
  readOnly?: boolean;
  onToggleSection: (section: AgentConfigSection) => void;
  onChangeAgentMemoryEnabled: (value: boolean) => void;
  onChangeAgentMemoryAutoExtractionEnabled: (value: boolean) => void;
  onChangeAgentMemorySlots: (value: AgentMemorySlotConfig[]) => void;
  onChangeAgentMemoryInstructions: (value: string) => void;
}

export function AgentRuntimeMemorySection({
  agentId,
  open,
  agentMemoryEnabled,
  agentMemoryAutoExtractionEnabled,
  agentMemorySlots,
  agentMemorySlotValidationErrors,
  agentMemoryInstructions,
  readOnly = false,
  onToggleSection,
  onChangeAgentMemoryEnabled,
  onChangeAgentMemoryAutoExtractionEnabled,
  onChangeAgentMemorySlots,
  onChangeAgentMemoryInstructions,
}: AgentRuntimeMemorySectionProps) {
  const t = useT('agents.agentRuntime');
  const { locale } = useLocale();
  const [previewOpen, setPreviewOpen] = useState(false);
  const [pendingRemoveMemoryIndex, setPendingRemoveMemoryIndex] = useState<number | null>(null);
  const [pendingReplaceTemplate, setPendingReplaceTemplate] = useState<AgentMemoryTemplate | null>(
    null
  );
  const [memoryItemDialogOpen, setMemoryItemDialogOpen] = useState(false);
  const [memoryTemplateDialogOpen, setMemoryTemplateDialogOpen] = useState(false);
  const [newMemoryName, setNewMemoryName] = useState('');
  const [newMemoryKey, setNewMemoryKey] = useState('');
  const [newMemoryDescription, setNewMemoryDescription] = useState('');
  const templatesQuery = useQuery({
    queryKey: AGENT_KEYS.memoryTemplates(agentId, locale),
    queryFn: () => agentService.getAgentMemoryTemplates(agentId, locale),
    enabled: Boolean(agentId) && open,
    staleTime: 10 * 60 * 1000,
  });
  const memoryTemplates = templatesQuery.data?.data?.catalog.templates ?? [];

  const usedAgentMemorySlotKeys = new Set(
    agentMemorySlots.map(slot => slot.key.trim().toLowerCase()).filter(Boolean)
  );
  const nextAgentMemorySlotKey = (() => {
    for (let index = agentMemorySlots.length + 1; index <= MAX_AGENT_MEMORY_SLOTS; index += 1) {
      const candidate = `memory_${index}`;
      if (!usedAgentMemorySlotKeys.has(candidate)) return candidate;
    }
    return `memory_${Date.now().toString(36).slice(-6)}`;
  })();
  const normalizedNewMemoryKey = newMemoryKey.trim().toLowerCase();
  const newMemoryKeyError = (() => {
    if (!memoryItemDialogOpen) return null;
    if (!normalizedNewMemoryKey) return 'required';
    if (!/^[a-z][a-z0-9_]*$/.test(normalizedNewMemoryKey)) return 'pattern';
    if (usedAgentMemorySlotKeys.has(normalizedNewMemoryKey)) return 'duplicate';
    return null;
  })() as AgentMemorySlotValidationError;

  const addAgentMemorySlot = () => {
    if (readOnly) return;
    if (agentMemorySlots.length >= MAX_AGENT_MEMORY_SLOTS) return;
    const key = newMemoryKey.trim().toLowerCase();
    if (!key) return;
    onChangeAgentMemoryEnabled(true);
    onChangeAgentMemorySlots([
      ...agentMemorySlots,
      {
        key,
        name: newMemoryName.trim().slice(0, 80),
        description: newMemoryDescription.trim().slice(0, AGENT_MEMORY_LIMITS.maxDescriptionChars),
        max_chars: AGENT_MEMORY_LIMITS.defaultChars,
        read_only: false,
        auto_write: true,
        enabled: true,
        sort_order: agentMemorySlots.length,
      },
    ]);
    setNewMemoryName('');
    setNewMemoryKey('');
    setNewMemoryDescription('');
    setMemoryItemDialogOpen(false);
  };
  const updateAgentMemorySlot = (index: number, patch: Partial<AgentMemorySlotConfig>) => {
    if (readOnly) return;
    onChangeAgentMemorySlots(
      agentMemorySlots.map((slot, currentIndex) =>
        currentIndex === index ? { ...slot, ...patch } : slot
      )
    );
  };
  const removeAgentMemorySlot = (index: number) => {
    if (readOnly) return;
    onChangeAgentMemorySlots(agentMemorySlots.filter((_, currentIndex) => currentIndex !== index));
  };
  const getAgentMemorySlotErrorText = (error: AgentMemorySlotValidationError) => {
    if (!error) return '';
    return t(`memory.validation.${error}`);
  };
  const openCustomMemoryDialog = () => {
    if (readOnly) return;
    setNewMemoryName('');
    setNewMemoryKey(nextAgentMemorySlotKey);
    setNewMemoryDescription('');
    setMemoryItemDialogOpen(true);
  };
  const applyTemplate = (template: AgentMemoryTemplate, mode: 'merge' | 'replace') => {
    if (readOnly) return;
    const result = applyAgentMemoryTemplate(agentMemorySlots, template, mode);
    if (!result.ok) {
      toast.error(t('memory.templateTooMany'));
      return;
    }
    onChangeAgentMemoryEnabled(true);
    onChangeAgentMemorySlots(result.slots);
    if (mode === 'replace' || !agentMemoryInstructions.trim()) {
      onChangeAgentMemoryInstructions(template.instructions);
    }
    setMemoryTemplateDialogOpen(false);
    setPendingReplaceTemplate(null);
    toast.success(
      mode === 'replace' ? t('memory.templateReplaceApplied') : t('memory.templateApplied')
    );
  };
  const addTemplateSlot = (template: AgentMemoryTemplate, slotKey: string) => {
    if (readOnly) return;
    const block = template.blocks.find(item => item.key === slotKey);
    if (!block) return;
    const result = addAgentMemoryTemplateBlock(agentMemorySlots, block);
    if (!result.ok) {
      toast.error(t('memory.templateTooMany'));
      return;
    }
    onChangeAgentMemoryEnabled(true);
    onChangeAgentMemorySlots(result.slots);
    toast.success(t('memory.templateSlotAdded'));
  };
  const missingTemplateSlotCount = (template: AgentMemoryTemplate) =>
    template.blocks.filter(slot => !usedAgentMemorySlotKeys.has(slot.key)).length;

  return (
    <>
      {previewOpen && (
        <AgentMemoryPreviewDialog
          open={previewOpen}
          onOpenChange={setPreviewOpen}
          agentId={agentId}
          slots={agentMemorySlots}
          instructions={agentMemoryInstructions}
          templates={memoryTemplates}
        />
      )}
      <ConfirmDialog
        open={pendingRemoveMemoryIndex !== null}
        onOpenChange={dialogOpen => {
          if (!dialogOpen) setPendingRemoveMemoryIndex(null);
        }}
        title={t('memory.deleteConfirmTitle')}
        description={t('memory.deleteConfirmDescription')}
        confirmText={t('memory.deleteConfirmAction')}
        cancelText={t('memory.deleteConfirmCancel')}
        variant="danger"
        onConfirm={() => {
          if (readOnly) return;
          if (pendingRemoveMemoryIndex !== null) {
            removeAgentMemorySlot(pendingRemoveMemoryIndex);
          }
          setPendingRemoveMemoryIndex(null);
        }}
      />
      <ConfirmDialog
        open={Boolean(pendingReplaceTemplate)}
        onOpenChange={dialogOpen => {
          if (!dialogOpen) setPendingReplaceTemplate(null);
        }}
        title={t('memory.templateReplaceConfirmTitle')}
        description={t('memory.templateReplaceConfirmDescription')}
        confirmText={t('memory.templateReplaceConfirmAction')}
        cancelText={t('memory.templateReplaceConfirmCancel')}
        variant="warning"
        onConfirm={() => {
          if (readOnly) return;
          if (pendingReplaceTemplate) {
            applyTemplate(pendingReplaceTemplate, 'replace');
          }
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
              <label htmlFor="agent-memory-display-name" className="text-sm font-medium">
                {t('memory.nameLabel')}
              </label>
              <Input
                id="agent-memory-display-name"
                value={newMemoryName}
                maxLength={80}
                placeholder={t('memory.slotNamePlaceholder')}
                onChange={event => setNewMemoryName(event.target.value.slice(0, 80))}
                disabled={readOnly}
              />
              <div className="text-xs leading-5 text-muted-foreground">{t('memory.nameHelp')}</div>
            </div>
            <div className="space-y-1.5">
              <label htmlFor="agent-memory-id" className="text-sm font-medium">
                {t('memory.keyLabel')}
              </label>
              <Input
                id="agent-memory-id"
                value={newMemoryKey}
                maxLength={64}
                placeholder={t('memory.slotKeyPlaceholder')}
                error={Boolean(newMemoryKeyError)}
                errorText={newMemoryKeyError ? t(`memory.validation.${newMemoryKeyError}`) : null}
                onChange={event => setNewMemoryKey(event.target.value.toLowerCase().slice(0, 64))}
                disabled={readOnly}
              />
              <div className="text-xs leading-5 text-muted-foreground">{t('memory.keyHelp')}</div>
            </div>
            <div className="space-y-1.5">
              <label htmlFor="agent-memory-description" className="text-sm font-medium">
                {t('memory.descriptionLabel')}
              </label>
              <Textarea
                id="agent-memory-description"
                value={newMemoryDescription}
                maxLength={AGENT_MEMORY_LIMITS.maxDescriptionChars}
                showCharacterCount
                className="min-h-24"
                placeholder={t('memory.slotDescriptionPlaceholder')}
                disabled={readOnly}
                onChange={event =>
                  setNewMemoryDescription(
                    event.target.value.slice(0, AGENT_MEMORY_LIMITS.maxDescriptionChars)
                  )
                }
              />
            </div>
          </DialogBody>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setMemoryItemDialogOpen(false)}>
              {t('memory.addDialogCancel')}
            </Button>
            <Button
              onClick={addAgentMemorySlot}
              disabled={
                readOnly ||
                Boolean(newMemoryKeyError) ||
                agentMemorySlots.length >= MAX_AGENT_MEMORY_SLOTS
              }
            >
              {t('memory.addDialogConfirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={memoryTemplateDialogOpen} onOpenChange={setMemoryTemplateDialogOpen}>
        <DialogContent size="lg">
          <DialogHeader>
            <DialogTitle>{t('memory.templateDialogTitle')}</DialogTitle>
            <DialogDescription>{t('memory.templateDialogDescription')}</DialogDescription>
          </DialogHeader>
          <DialogBody className="space-y-3">
            {templatesQuery.isLoading && (
              <div className="text-xs text-muted-foreground">{t('memory.templatesLoading')}</div>
            )}
            {memoryTemplates.map(template => {
              const missingCount = missingTemplateSlotCount(template);
              const mergeWouldExceed =
                agentMemorySlots.length + missingCount > MAX_AGENT_MEMORY_SLOTS;
              return (
                <div key={template.id} className="space-y-3 rounded-md border p-3">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <div className="flex items-center gap-2 text-sm font-semibold">
                        <Sparkles className="size-4 text-primary" />
                        <span>{template.name}</span>
                      </div>
                      <div className="mt-1 text-xs text-muted-foreground">
                        {template.description}
                      </div>
                    </div>
                    <Badge variant="secondary">
                      {t('memory.templateSlotCount', { count: template.blocks.length })}
                    </Badge>
                  </div>
                  <div className="rounded-md bg-muted/30 px-3 py-2 text-xs text-muted-foreground">
                    {template.instructions}
                  </div>
                  <div className="space-y-2">
                    {template.blocks.map(slot => {
                      const alreadyAdded = usedAgentMemorySlotKeys.has(slot.key);
                      const addDisabled =
                        alreadyAdded || agentMemorySlots.length >= MAX_AGENT_MEMORY_SLOTS;
                      return (
                        <div
                          key={`${template.id}-${slot.key}`}
                          className="flex items-center justify-between gap-3 rounded-md border bg-muted/20 px-3 py-2"
                        >
                          <div className="min-w-0">
                            <div className="truncate text-sm font-semibold">{slot.name}</div>
                            <div className="mt-0.5 truncate font-mono text-[11px] text-muted-foreground">
                              {t('memory.idValue', { id: slot.key })}
                            </div>
                            <div className="mt-0.5 line-clamp-2 text-xs text-muted-foreground">
                              {slot.description}
                            </div>
                          </div>
                          <Button
                            type="button"
                            variant="ghost"
                            size="xs"
                            disabled={readOnly || addDisabled}
                            onClick={() => addTemplateSlot(template, slot.key)}
                          >
                            <Plus className="size-3" />
                            {alreadyAdded
                              ? t('memory.templateSlotExists')
                              : t('memory.templateAddSlot')}
                          </Button>
                        </div>
                      );
                    })}
                  </div>
                  {mergeWouldExceed && (
                    <div className="text-xs text-destructive">{t('memory.templateTooMany')}</div>
                  )}
                  <div className="flex flex-wrap justify-end gap-2">
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      disabled={readOnly}
                      onClick={() => setPendingReplaceTemplate(template)}
                    >
                      {t('memory.templateReplace')}
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      disabled={readOnly || mergeWouldExceed}
                      onClick={() => applyTemplate(template, 'merge')}
                    >
                      <Sparkles className="size-4" />
                      {t('memory.templateMerge')}
                    </Button>
                  </div>
                </div>
              );
            })}
          </DialogBody>
        </DialogContent>
      </Dialog>

      <RuntimeSection
        title={t('sections.memory')}
        section="memory"
        open={open}
        onToggle={onToggleSection}
      >
        <div className="space-y-3">
          <div className="space-y-3 rounded-md border p-3">
            <div className="flex items-center justify-between gap-3">
              <div>
                <div className="text-sm font-medium">{t('memory.agentTitle')}</div>
                <div className="text-xs text-muted-foreground">{t('memory.agentDescription')}</div>
              </div>
              <Switch
                checked={agentMemoryEnabled}
                disabled={readOnly}
                onCheckedChange={onChangeAgentMemoryEnabled}
              />
            </div>
            {agentMemoryEnabled && (
              <div className="space-y-2">
                <div className="flex items-center justify-between gap-3 rounded-md bg-muted/30 px-3 py-2">
                  <div>
                    <div className="text-xs font-medium">{t('memory.autoExtractionTitle')}</div>
                    <div className="text-[11px] text-muted-foreground">
                      {t('memory.autoExtractionDescription')}
                    </div>
                  </div>
                  <Switch
                    checked={agentMemoryAutoExtractionEnabled}
                    disabled={readOnly}
                    onCheckedChange={onChangeAgentMemoryAutoExtractionEnabled}
                  />
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => setMemoryTemplateDialogOpen(true)}
                    disabled={readOnly}
                  >
                    <Sparkles className="size-4" />
                    {t('memory.applyTemplate')}
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={openCustomMemoryDialog}
                    disabled={readOnly || agentMemorySlots.length >= MAX_AGENT_MEMORY_SLOTS}
                  >
                    <Plus className="size-4" />
                    {agentMemorySlots.length >= MAX_AGENT_MEMORY_SLOTS
                      ? t('memory.maxItemsReached')
                      : t('memory.addCustomSlot')}
                  </Button>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={() => setPreviewOpen(true)}
                    disabled={!agentMemorySlots.some(slot => slot.enabled)}
                  >
                    <FlaskConical className="size-4" />
                    {t('memory.preview.open')}
                  </Button>
                </div>
                <div className="space-y-1">
                  <div className="text-xs font-medium text-muted-foreground">
                    {t('memory.instructionsLabel')}
                  </div>
                  <Textarea
                    value={agentMemoryInstructions}
                    maxLength={AGENT_MEMORY_LIMITS.maxInstructionsChars}
                    showCharacterCount
                    className="min-h-20"
                    placeholder={t('memory.instructionsPlaceholder')}
                    disabled={readOnly}
                    onChange={event =>
                      onChangeAgentMemoryInstructions(
                        event.target.value.slice(0, AGENT_MEMORY_LIMITS.maxInstructionsChars)
                      )
                    }
                  />
                  <div className="text-[11px] text-muted-foreground">
                    {t('memory.instructionsHelp')}
                  </div>
                </div>
                {agentMemorySlots.length === 0 ? (
                  <div className="space-y-3 rounded-md border border-dashed p-3">
                    <div className="text-xs text-muted-foreground">{t('memory.emptySlots')}</div>
                    <Button
                      type="button"
                      variant="secondary"
                      size="sm"
                      onClick={() => setMemoryTemplateDialogOpen(true)}
                      disabled={readOnly}
                    >
                      <Sparkles className="size-4" />
                      {t('memory.applyTemplate')}
                    </Button>
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
                            <div className="truncate text-sm font-semibold">
                              {slot.name?.trim() || slot.key}
                            </div>
                            <div className="mt-0.5 truncate font-mono text-[11px] text-muted-foreground">
                              {t('memory.idValue', { id: slot.key })}
                            </div>
                            {keyErrorText && (
                              <div className="mt-1 text-xs text-destructive">{keyErrorText}</div>
                            )}
                          </div>
                          <div className="flex shrink-0 items-center gap-2">
                            <Switch
                              checked={slot.enabled}
                              disabled={readOnly}
                              onCheckedChange={checked =>
                                updateAgentMemorySlot(index, { enabled: checked })
                              }
                            />
                            <Button
                              type="button"
                              variant="ghost"
                              isIcon
                              aria-label={t('memory.removeSlot')}
                              disabled={readOnly}
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
                            {t('memory.nameLabel')}
                          </div>
                          <Input
                            value={slot.name ?? ''}
                            maxLength={80}
                            placeholder={t('memory.slotNamePlaceholder')}
                            disabled={readOnly}
                            onChange={event =>
                              updateAgentMemorySlot(index, {
                                name: event.target.value.slice(0, 80),
                              })
                            }
                          />
                          <div className="text-[11px] text-muted-foreground">
                            {t('memory.nameHelp')}
                          </div>
                        </div>
                        <div className="space-y-1">
                          <div className="text-xs font-medium text-muted-foreground">
                            {t('memory.descriptionLabel')}
                          </div>
                          <Textarea
                            value={slot.description}
                            maxLength={AGENT_MEMORY_LIMITS.maxDescriptionChars}
                            showCharacterCount
                            className="min-h-20"
                            placeholder={t('memory.slotDescriptionPlaceholder')}
                            disabled={readOnly}
                            onChange={event =>
                              updateAgentMemorySlot(index, {
                                description: event.target.value.slice(
                                  0,
                                  AGENT_MEMORY_LIMITS.maxDescriptionChars
                                ),
                              })
                            }
                          />
                          <div className="text-[11px] text-muted-foreground">
                            {t('memory.descriptionHelp')}
                          </div>
                        </div>
                        <div className="space-y-2">
                          <div className="flex items-center justify-between gap-3">
                            <div className="text-xs font-medium text-muted-foreground">
                              {t('memory.limitLabel')}
                            </div>
                            <Input
                              className="w-28"
                              type="number"
                              min={AGENT_MEMORY_LIMITS.minChars}
                              max={AGENT_MEMORY_LIMITS.maxChars}
                              step={100}
                              value={slot.max_chars || AGENT_MEMORY_LIMITS.defaultChars}
                              disabled={readOnly}
                              onChange={event =>
                                updateAgentMemorySlot(index, {
                                  max_chars: Number(event.target.value),
                                })
                              }
                              onBlur={() =>
                                updateAgentMemorySlot(index, {
                                  max_chars: clampAgentMemoryMaxChars(slot.max_chars),
                                })
                              }
                            />
                          </div>
                          <div className="flex items-center justify-between gap-2 rounded-md bg-muted/30 px-3 py-2">
                            <div>
                              <div className="text-xs font-medium">{t('memory.readOnlyLabel')}</div>
                              <div className="text-[11px] text-muted-foreground">
                                {t('memory.readOnlyHelp')}
                              </div>
                            </div>
                            <Switch
                              checked={slot.read_only ?? false}
                              disabled={readOnly}
                              onCheckedChange={checked =>
                                updateAgentMemorySlot(index, { read_only: checked })
                              }
                            />
                          </div>
                          {agentMemoryAutoExtractionEnabled ? (
                            <div className="flex items-center justify-between gap-2 rounded-md bg-muted/30 px-3 py-2">
                              <div>
                                <div className="text-xs font-medium">
                                  {t('memory.autoWriteLabel')}
                                </div>
                                <div className="text-[11px] text-muted-foreground">
                                  {t('memory.autoWriteHelp')}
                                </div>
                              </div>
                              <Switch
                                checked={!slot.read_only && (slot.auto_write ?? true)}
                                disabled={readOnly || slot.read_only}
                                onCheckedChange={checked =>
                                  updateAgentMemorySlot(index, { auto_write: checked })
                                }
                              />
                            </div>
                          ) : null}
                        </div>
                      </div>
                    );
                  })
                )}
              </div>
            )}
          </div>
        </div>
      </RuntimeSection>
    </>
  );
}
