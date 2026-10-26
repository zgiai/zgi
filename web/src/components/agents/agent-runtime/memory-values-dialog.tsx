'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { toast } from 'sonner';
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
import { agentMemoryAddTargets, agentMemoryTitle } from '@/components/memory/agent-memory-topics';
import { MemoryBoard } from '@/components/memory/memory-board';
import { joinFacts, splitFacts } from '@/components/memory/memory-topics';
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
  const tItems = useT('common.memoryItems');
  const [values, setValues] = useState<AgentMemoryValue[]>([]);
  const [isLoading, setIsLoading] = useState(false);
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

  // The board shows its own success toasts; failures are reported here and
  // rethrown so the board keeps the edit open.
  const saveValue = async (key: string, content: string) => {
    try {
      const response = await agentService.updateAgentMemoryValue(agentId, {
        key,
        content,
        expected_revision: valuesByKey.get(key)?.revision ?? 0,
      });
      setValues(current =>
        current.some(item => item.key === key)
          ? current.map(item => (item.key === key ? response.data : item))
          : [...current, response.data]
      );
    } catch (error) {
      toast.error(getErrorMessage(error) || t('memoryValues.saveFailed'));
      throw error;
    }
  };

  const clearValue = async (key: string) => {
    try {
      await agentService.clearAgentMemoryValue(agentId, { key });
      setValues(current => current.filter(item => item.key !== key));
    } catch (error) {
      toast.error(getErrorMessage(error) || t('memoryValues.clearFailed'));
      throw error;
    }
  };

  return (
    <>
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
              <MemoryBoard
                canEdit
                empty={t('memoryValues.empty')}
                topics={values
                  .filter(value => value.content?.trim())
                  .map(value => ({
                    key: value.key,
                    title: agentMemoryTitle(value, tItems),
                    content: value.content,
                    meta: tItems('meta', {
                      time: formatMemoryValueUpdatedAt(value),
                      source: tItems(`sources.${value.source_kind ?? 'manager'}`),
                    }),
                  }))}
                onSave={saveValue}
                onDeleteTopic={clearValue}
                addTargets={agentMemoryAddTargets(values, tItems)}
                onAdd={(key, fact) =>
                  saveValue(
                    key,
                    joinFacts([...splitFacts(valuesByKey.get(key)?.content ?? ''), fact])
                  )
                }
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
