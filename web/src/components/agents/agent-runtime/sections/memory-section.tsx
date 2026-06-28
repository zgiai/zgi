'use client';

import { useState } from 'react';
import { Plus, Sparkles, Trash2 } from 'lucide-react';
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
import { useT } from '@/i18n';
import type { AgentMemorySlotConfig } from '@/services/types/agent';
import {
  addAgentMemoryTemplateSlot,
  applyAgentMemoryTemplate,
  createAgentMemoryTemplates,
  type AgentMemoryTemplate,
} from '../memory-templates';
import { RuntimeSection } from '../runtime-section';
import type { AgentConfigSection } from '../types';
import type { AgentMemorySlotValidationError } from '../utils';

const MAX_AGENT_MEMORY_SLOTS = 5;

interface AgentRuntimeMemorySectionProps {
  open: boolean;
  agentMemoryEnabled: boolean;
  agentMemorySlots: AgentMemorySlotConfig[];
  agentMemorySlotValidationErrors: AgentMemorySlotValidationError[];
  readOnly?: boolean;
  onToggleSection: (section: AgentConfigSection) => void;
  onChangeAgentMemoryEnabled: (value: boolean) => void;
  onChangeAgentMemorySlots: (value: AgentMemorySlotConfig[]) => void;
}

export function AgentRuntimeMemorySection({
  open,
  agentMemoryEnabled,
  agentMemorySlots,
  agentMemorySlotValidationErrors,
  readOnly = false,
  onToggleSection,
  onChangeAgentMemoryEnabled,
  onChangeAgentMemorySlots,
}: AgentRuntimeMemorySectionProps) {
  const t = useT('agents.agentRuntime');
  const [pendingRemoveMemoryIndex, setPendingRemoveMemoryIndex] = useState<number | null>(null);
  const [pendingReplaceTemplate, setPendingReplaceTemplate] = useState<AgentMemoryTemplate | null>(
    null
  );
  const [memoryItemDialogOpen, setMemoryItemDialogOpen] = useState(false);
  const [memoryTemplateDialogOpen, setMemoryTemplateDialogOpen] = useState(false);
  const [newMemoryKey, setNewMemoryKey] = useState('');
  const [newMemoryDescription, setNewMemoryDescription] = useState('');
  const memoryTemplates = createAgentMemoryTemplates(key => t(key as Parameters<typeof t>[0]));

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
    setMemoryTemplateDialogOpen(false);
    setPendingReplaceTemplate(null);
    toast.success(
      mode === 'replace' ? t('memory.templateReplaceApplied') : t('memory.templateApplied')
    );
  };
  const addTemplateSlot = (template: AgentMemoryTemplate, slotKey: string) => {
    if (readOnly) return;
    const slot = template.slots.find(item => item.key === slotKey);
    if (!slot) return;
    const result = addAgentMemoryTemplateSlot(agentMemorySlots, slot);
    if (!result.ok) {
      toast.error(t('memory.templateTooMany'));
      return;
    }
    onChangeAgentMemoryEnabled(true);
    onChangeAgentMemorySlots(result.slots);
    toast.success(t('memory.templateSlotAdded'));
  };
  const missingTemplateSlotCount = (template: AgentMemoryTemplate) =>
    template.slots.filter(slot => !usedAgentMemorySlotKeys.has(slot.key)).length;

  return (
    <>
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
              <Input
                value={newMemoryKey}
                maxLength={64}
                placeholder={nextAgentMemorySlotKey}
                error={Boolean(newMemoryKeyError)}
                errorText={newMemoryKeyError ? t(`memory.validation.${newMemoryKeyError}`) : null}
                onChange={event => setNewMemoryKey(event.target.value.toLowerCase().slice(0, 64))}
                disabled={readOnly}
              />
            </div>
            <div className="space-y-1.5">
              <Textarea
                value={newMemoryDescription}
                maxLength={200}
                showCharacterCount
                className="min-h-24"
                placeholder={t('memory.slotDescriptionPlaceholder')}
                disabled={readOnly}
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
                      {t('memory.templateSlotCount', { count: template.slots.length })}
                    </Badge>
                  </div>
                  <div className="space-y-2">
                    {template.slots.map(slot => {
                      const alreadyAdded = usedAgentMemorySlotKeys.has(slot.key);
                      const addDisabled =
                        alreadyAdded || agentMemorySlots.length >= MAX_AGENT_MEMORY_SLOTS;
                      return (
                        <div
                          key={`${template.id}-${slot.key}`}
                          className="flex items-center justify-between gap-3 rounded-md border bg-muted/20 px-3 py-2"
                        >
                          <div className="min-w-0">
                            <div className="font-mono text-xs font-semibold">{slot.key}</div>
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
                            {alreadyAdded ? t('memory.templateSlotExists') : t('memory.templateAddSlot')}
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
                            <div className="truncate text-sm font-semibold">{slot.key}</div>
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
                            {t('memory.descriptionLabel')}
                          </div>
                          <Textarea
                            value={slot.description}
                            maxLength={200}
                            showCharacterCount
                            className="min-h-20"
                            placeholder={t('memory.slotDescriptionPlaceholder')}
                            disabled={readOnly}
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
              </div>
            )}
          </div>
        </div>
      </RuntimeSection>
    </>
  );
}
