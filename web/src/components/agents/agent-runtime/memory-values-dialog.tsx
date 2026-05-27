'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { Loader2, Save, Trash2 } from 'lucide-react';
import { toast } from 'sonner';
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
import { Textarea } from '@/components/ui/textarea';
import { useT } from '@/i18n';
import agentService from '@/services/agent.service';
import type { AgentMemoryValue } from '@/services/types/agent';
import { getErrorMessage } from '@/utils/error-notifications';

interface AgentRuntimeMemoryValuesDialogProps {
  agentId: string;
  open: boolean;
  defaultUserId?: string;
  onOpenChange: (open: boolean) => void;
}

export function AgentRuntimeMemoryValuesDialog({
  agentId,
  open,
  defaultUserId,
  onOpenChange,
}: AgentRuntimeMemoryValuesDialogProps) {
  const t = useT('agents.agentRuntime');
  const [values, setValues] = useState<AgentMemoryValue[]>([]);
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [isLoading, setIsLoading] = useState(false);
  const [savingKey, setSavingKey] = useState('');
  const [clearingKey, setClearingKey] = useState('');
  const [pendingClearKey, setPendingClearKey] = useState('');

  const valuesByKey = useMemo(() => new Map(values.map(value => [value.key, value])), [values]);

  const loadValues = useCallback(async () => {
    if (!defaultUserId) {
      return;
    }
    setIsLoading(true);
    try {
      const response = await agentService.getAgentMemoryValues(agentId);
      const nextValues = response.data.values ?? [];
      setValues(nextValues);
      setDrafts(Object.fromEntries(nextValues.map(value => [value.key, value.content ?? ''])));
    } catch (error) {
      toast.error(getErrorMessage(error) || t('memoryValues.loadFailed'));
    } finally {
      setIsLoading(false);
    }
  }, [agentId, defaultUserId, t]);

  useEffect(() => {
    if (!open) return;
    void loadValues();
  }, [loadValues, open]);

  const saveValue = async (key: string) => {
    const value = valuesByKey.get(key);
    if (!value) return;
    setSavingKey(key);
    try {
      const response = await agentService.updateAgentMemoryValue(agentId, {
        key,
        content: drafts[key] ?? '',
      });
      setValues(current => current.map(item => (item.key === key ? response.data : item)));
      setDrafts(current => ({ ...current, [key]: response.data.content ?? '' }));
      toast.success(t('memoryValues.saveSuccess'));
    } catch (error) {
      toast.error(getErrorMessage(error) || t('memoryValues.saveFailed'));
    } finally {
      setSavingKey('');
    }
  };

  const clearValue = async (key: string) => {
    setClearingKey(key);
    try {
      const response = await agentService.clearAgentMemoryValue(agentId, {
        key,
      });
      setValues(current => current.map(item => (item.key === key ? response.data : item)));
      setDrafts(current => ({ ...current, [key]: '' }));
      toast.success(t('memoryValues.clearSuccess'));
    } catch (error) {
      toast.error(getErrorMessage(error) || t('memoryValues.clearFailed'));
    } finally {
      setClearingKey('');
      setPendingClearKey('');
    }
  };

  return (
    <>
      <ConfirmDialog
        open={Boolean(pendingClearKey)}
        onOpenChange={nextOpen => {
          if (!nextOpen) setPendingClearKey('');
        }}
        title={t('memoryValues.clearConfirmTitle')}
        description={t('memoryValues.clearConfirmDescription')}
        confirmText={t('memoryValues.clearConfirmAction')}
        cancelText={t('memoryValues.clearConfirmCancel')}
        variant="warning"
        loading={Boolean(clearingKey)}
        onConfirm={() => {
          if (pendingClearKey) void clearValue(pendingClearKey);
        }}
      />
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent size="xl" className="max-h-[min(760px,calc(100vh-2rem))]">
          <DialogHeader>
            <DialogTitle>{t('memoryValues.title')}</DialogTitle>
            <DialogDescription>{t('memoryValues.description')}</DialogDescription>
          </DialogHeader>
          <DialogBody className="space-y-4">
            {values.length === 0 ? (
              <div className="rounded-md border border-dashed p-6 text-sm text-muted-foreground">
                {isLoading ? t('memoryValues.loading') : t('memoryValues.empty')}
              </div>
            ) : (
              <div className="space-y-3">
                {values.map(value => {
                  const draft = drafts[value.key] ?? '';
                  const overLimit = Array.from(draft).length > value.max_chars;
                  return (
                    <div key={value.key} className="rounded-md border p-3">
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                          <div className="text-sm font-medium">{value.key}</div>
                          <div className="mt-1 text-xs text-muted-foreground">
                            {value.description || t('memoryValues.noDescription')}
                          </div>
                          <div className="mt-1 text-xs text-muted-foreground">
                            {t('memoryValues.updatedAt', {
                              time: value.updated_at_display || value.updated_at_iso || '-',
                            })}
                          </div>
                        </div>
                      </div>
                      <Textarea
                        className="mt-3 min-h-24"
                        value={draft}
                        maxLength={2000}
                        showCharacterCount
                        placeholder={t('memoryValues.contentPlaceholder')}
                        onChange={event =>
                          setDrafts(current => ({
                            ...current,
                            [value.key]: event.target.value.slice(0, 2000),
                          }))
                        }
                      />
                      {overLimit && (
                        <div className="mt-1 text-xs text-destructive">
                          {t('memoryValues.overLimit')}
                        </div>
                      )}
                      <div className="mt-3 flex justify-end gap-2">
                        <Button
                          variant="outline"
                          size="sm"
                          onClick={() => setPendingClearKey(value.key)}
                          disabled={clearingKey === value.key}
                        >
                          <Trash2 className="size-4" />
                          {t('memoryValues.clear')}
                        </Button>
                        <Button
                          size="sm"
                          onClick={() => void saveValue(value.key)}
                          disabled={savingKey === value.key || overLimit}
                        >
                          {savingKey === value.key ? (
                            <Loader2 className="size-4 animate-spin" />
                          ) : (
                            <Save className="size-4" />
                          )}
                          {t('memoryValues.save')}
                        </Button>
                      </div>
                    </div>
                  );
                })}
              </div>
            )}
          </DialogBody>
          <DialogFooter>
            <Button variant="ghost" onClick={() => onOpenChange(false)}>
              {t('memoryValues.close')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
