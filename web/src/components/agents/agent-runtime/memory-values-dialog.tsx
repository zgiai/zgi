'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
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
import { MemoryItemList } from '@/components/memory/memory-item-list';
import { useT } from '@/i18n';
import agentService from '@/services/agent.service';
import type { AgentMemoryValue } from '@/services/types/agent';
import { getErrorMessage } from '@/utils/error-notifications';
import { formatDate } from '@/utils/format';

interface AgentRuntimeMemoryValuesDialogProps {
  agentId: string;
  open: boolean;
  defaultUserId?: string;
  onOpenChange: (open: boolean) => void;
}

function formatMemoryValueUpdatedAt(value: AgentMemoryValue): string {
  const source = value.updated_at_unix ?? value.updated_at_iso ?? value.updated_at;
  if (source === undefined || source === null || source === '') {
    return value.updated_at_display || '-';
  }
  const formatted = formatDate(source, 'YYYY-MM-DD HH:mm:ss');
  return formatted === 'Invalid Date' ? value.updated_at_display || '-' : formatted;
}

export function AgentRuntimeMemoryValuesDialog({
  agentId,
  open,
  defaultUserId,
  onOpenChange,
}: AgentRuntimeMemoryValuesDialogProps) {
  const t = useT('agents.agentRuntime');
  const [values, setValues] = useState<AgentMemoryValue[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [savingKey, setSavingKey] = useState('');
  const [clearingKey, setClearingKey] = useState('');
  const [pendingClearKey, setPendingClearKey] = useState('');
  const isLoadingRef = useRef(false);
  const loadedDialogKeyRef = useRef('');
  const tRef = useRef(t);

  const valuesByKey = useMemo(() => new Map(values.map(value => [value.key, value])), [values]);

  useEffect(() => {
    tRef.current = t;
  }, [t]);

  const loadValues = useCallback(async () => {
    if (!defaultUserId) {
      return;
    }
    if (isLoadingRef.current) {
      return;
    }
    isLoadingRef.current = true;
    setIsLoading(true);
    try {
      const response = await agentService.getAgentMemoryValues(agentId);
      const nextValues = response.data.values ?? [];
      setValues(nextValues);
    } catch (error) {
      toast.error(getErrorMessage(error) || tRef.current('memoryValues.loadFailed'));
    } finally {
      isLoadingRef.current = false;
      setIsLoading(false);
    }
  }, [agentId, defaultUserId]);

  useEffect(() => {
    if (!open) {
      loadedDialogKeyRef.current = '';
      return;
    }
    const dialogKey = `${agentId}:${defaultUserId ?? ''}`;
    if (loadedDialogKeyRef.current === dialogKey) return;
    loadedDialogKeyRef.current = dialogKey;
    void loadValues();
  }, [agentId, defaultUserId, loadValues, open]);

  const saveValue = async (key: string, content: string) => {
    const value = valuesByKey.get(key);
    if (!value) return;
    setSavingKey(key);
    try {
      const response = await agentService.updateAgentMemoryValue(agentId, { key, content });
      setValues(current => current.map(item => (item.key === key ? response.data : item)));
      toast.success(t('memoryValues.saveSuccess'));
    } catch (error) {
      toast.error(getErrorMessage(error) || t('memoryValues.saveFailed'));
      throw error;
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
            {isLoading && values.length === 0 ? (
              <div className="rounded-md border border-dashed p-6 text-sm text-muted-foreground">
                {t('memoryValues.loading')}
              </div>
            ) : (
              <MemoryItemList
                canEdit
                busy={Boolean(savingKey || clearingKey)}
                empty={t('memoryValues.empty')}
                items={values.map(value => ({
                  id: value.key,
                  label: value.name?.trim() || value.key,
                  badges: [value.key],
                  content: value.content ?? '',
                  maxChars: value.max_chars,
                  meta: [
                    value.description || t('memoryValues.noDescription'),
                    t('memoryValues.updatedAt', { time: formatMemoryValueUpdatedAt(value) }),
                  ].join(' · '),
                }))}
                onSave={(row, content) => saveValue(row.id, content)}
                onDelete={row => setPendingClearKey(row.id)}
              />
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
