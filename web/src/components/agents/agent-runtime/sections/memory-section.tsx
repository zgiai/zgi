'use client';

import { useState } from 'react';
import { Plus, Trash2 } from 'lucide-react';
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
import { RuntimeSection } from '../runtime-section';
import type { AgentConfigSection } from '../types';
import type { AgentMemorySlotValidationError } from '../utils';

interface AgentRuntimeMemorySectionProps {
  open: boolean;
  agentMemoryEnabled: boolean;
  agentMemorySlots: AgentMemorySlotConfig[];
  agentMemorySlotValidationErrors: AgentMemorySlotValidationError[];
  onToggleSection: (section: AgentConfigSection) => void;
  onChangeAgentMemoryEnabled: (value: boolean) => void;
  onChangeAgentMemorySlots: (value: AgentMemorySlotConfig[]) => void;
}

export function AgentRuntimeMemorySection({
  open,
  agentMemoryEnabled,
  agentMemorySlots,
  agentMemorySlotValidationErrors,
  onToggleSection,
  onChangeAgentMemoryEnabled,
  onChangeAgentMemorySlots,
}: AgentRuntimeMemorySectionProps) {
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
  const normalizedNewMemoryKey = newMemoryKey.trim().toLowerCase();
  const newMemoryKeyError = (() => {
    if (!memoryItemDialogOpen) return null;
    if (!normalizedNewMemoryKey) return 'required';
    if (!/^[a-z][a-z0-9_]*$/.test(normalizedNewMemoryKey)) return 'pattern';
    if (usedAgentMemorySlotKeys.has(normalizedNewMemoryKey)) return 'duplicate';
    return null;
  })() as AgentMemorySlotValidationError;

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
              <Switch checked={agentMemoryEnabled} onCheckedChange={onChangeAgentMemoryEnabled} />
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
                  {agentMemorySlots.length >= 5 ? t('memory.maxItemsReached') : t('memory.addSlot')}
                </Button>
              </div>
            )}
          </div>
        </div>
      </RuntimeSection>
    </>
  );
}
