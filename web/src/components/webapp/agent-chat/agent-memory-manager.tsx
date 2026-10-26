'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Brain, Download, Loader2, MoreHorizontal, Trash2 } from 'lucide-react';
import { toast } from 'sonner';
import { MemoryArchivePanel } from '@/components/memory/memory-archive-panel';
import { MemoryHistoryPanel } from '@/components/memory/memory-history-panel';
import {
  agentMemoryAddTargets,
  agentMemoryMeta,
  agentMemoryTitle,
  agentMemoryUndoable,
} from '@/components/memory/agent-memory-topics';
import { MemoryBoard } from '@/components/memory/memory-board';
import { joinFacts, splitFacts } from '@/components/memory/memory-topics';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useT } from '@/i18n';
import WebAppService from '@/services/webapp.service';
import type { WebAppAgentMemoryValue } from '@/services/types/webapp';
import { getErrorMessage } from '@/utils/error-notifications';

interface AgentMemoryManagerProps {
  webAppId: string;
  memoryEnabled: boolean;
}

function downloadJSON(filename: string, value: unknown) {
  const url = URL.createObjectURL(
    new Blob([JSON.stringify(value, null, 2)], { type: 'application/json' })
  );
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

export function AgentMemoryManager({ webAppId, memoryEnabled }: AgentMemoryManagerProps) {
  const t = useT('webapp.agentChat.memory');
  const tItems = useT('common.memoryItems');
  const tHistory = useT('common.memoryHistory');
  const tArchive = useT('common.memoryArchive');
  const [open, setOpen] = useState(false);
  const [tab, setTab] = useState<'memory' | 'history' | 'archive'>('memory');
  const [values, setValues] = useState<WebAppAgentMemoryValue[]>([]);
  const [loading, setLoading] = useState(false);
  const [busyKey, setBusyKey] = useState('');
  const [deleteAllOpen, setDeleteAllOpen] = useState(false);
  const isLoadingRef = useRef(false);
  const loadedWebAppIdRef = useRef('');
  const tRef = useRef(t);

  useEffect(() => {
    tRef.current = t;
  }, [t]);

  const load = useCallback(async () => {
    if (isLoadingRef.current) return;
    isLoadingRef.current = true;
    setLoading(true);
    try {
      const response = await WebAppService.getAgentMemory(webAppId);
      const nextValues = response.data.values ?? [];
      setValues(nextValues);
    } catch (error) {
      toast.error(getErrorMessage(error) || tRef.current('loadFailed'));
    } finally {
      isLoadingRef.current = false;
      setLoading(false);
    }
  }, [webAppId]);

  useEffect(() => {
    if (!open) {
      loadedWebAppIdRef.current = '';
      return;
    }
    if (loadedWebAppIdRef.current === webAppId) return;
    loadedWebAppIdRef.current = webAppId;
    void load();
  }, [load, open, webAppId]);

  const valuesByKey = useMemo(() => new Map(values.map(value => [value.key, value])), [values]);
  const hasSavedValues = values.some(value => Boolean(value.content.trim()));

  // The board reports success with its own toast; failures are shown here
  // and rethrown so the board keeps the edit open.
  const save = async (key: string, content: string) => {
    const value = valuesByKey.get(key);
    setBusyKey(key);
    try {
      await WebAppService.updateAgentMemory(webAppId, key, content, value?.revision ?? 0);
      await load();
    } catch (error) {
      toast.error(getErrorMessage(error) || t('saveFailed'));
      await load();
      throw error;
    } finally {
      setBusyKey('');
    }
  };

  const remove = async (key: string) => {
    const value = valuesByKey.get(key);
    if (!value) return;
    setBusyKey(key);
    try {
      const response = await WebAppService.deleteAgentMemory(webAppId, key, value.revision);
      await load();
      const operationId = response.data?.last_operation_id;
      const until = response.data?.undoable_until;
      if (operationId && until && until > Date.now() / 1000) {
        return { undo: () => undoOperation(operationId) };
      }
      return undefined;
    } catch (error) {
      toast.error(getErrorMessage(error) || t('deleteFailed'));
      await load();
      throw error;
    } finally {
      setBusyKey('');
    }
  };

  const undoOperation = async (operationId: string) => {
    try {
      await WebAppService.undoAgentMemoryOperation(webAppId, operationId);
      toast.success(t('undone'));
    } catch (error) {
      toast.error(getErrorMessage(error) || t('undoFailed'));
      throw error;
    } finally {
      await load();
    }
  };

  const removeAll = async () => {
    setBusyKey('__all__');
    try {
      await WebAppService.deleteAllAgentMemory(webAppId);
      toast.success(t('allDeleted'));
      setDeleteAllOpen(false);
      await load();
    } catch (error) {
      toast.error(getErrorMessage(error) || t('deleteFailed'));
    } finally {
      setBusyKey('');
    }
  };

  const exportValues = async () => {
    setBusyKey('__export__');
    try {
      const response = await WebAppService.exportAgentMemory(webAppId);
      downloadJSON('agent-memory.json', response.data);
    } catch (error) {
      toast.error(getErrorMessage(error) || t('exportFailed'));
    } finally {
      setBusyKey('');
    }
  };

  return (
    <>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="size-8 p-0"
        title={t('title')}
        aria-label={t('title')}
        onClick={() => setOpen(true)}
      >
        <Brain className="size-4" />
      </Button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent size="lg" className="max-h-[min(760px,calc(100vh-2rem))]">
          <DialogHeader>
            <DialogTitle>{t('title')}</DialogTitle>
            <DialogDescription>{t('description')}</DialogDescription>
          </DialogHeader>
          <DialogBody className="space-y-4">
            <Tabs
              value={tab}
              onValueChange={value => setTab(value as 'memory' | 'history' | 'archive')}
            >
              <TabsList>
                <TabsTrigger value="memory">{t('title')}</TabsTrigger>
                <TabsTrigger value="history">{tHistory('title')}</TabsTrigger>
                <TabsTrigger value="archive">{tArchive('title')}</TabsTrigger>
              </TabsList>
              <TabsContent value="archive" className="mt-4">
                {tab === 'archive' ? (
                  <MemoryArchivePanel
                    load={async () =>
                      (await WebAppService.getAgentMemoryArchive(webAppId)).data.entries
                    }
                    remove={entryId =>
                      WebAppService.deleteAgentMemoryArchiveEntry(webAppId, entryId)
                    }
                    refreshKey={values}
                  />
                ) : null}
              </TabsContent>
              <TabsContent value="history" className="mt-4">
                {tab === 'history' ? (
                  <MemoryHistoryPanel
                    load={async before =>
                      (await WebAppService.getAgentMemoryHistory(webAppId, { before })).data
                    }
                    revert={versionId =>
                      WebAppService.revertAgentMemoryVersion(webAppId, versionId)
                    }
                    labelFor={key => valuesByKey.get(key)?.name || key}
                    canRevert={memoryEnabled}
                    refreshKey={values}
                    onReverted={load}
                  />
                ) : null}
              </TabsContent>
              <TabsContent value="memory" className="mt-4 space-y-4">
                {loading ? (
                  <div className="flex min-h-40 items-center justify-center">
                    <Loader2 className="size-5 animate-spin text-muted-foreground" />
                  </div>
                ) : (
                  <MemoryBoard
                    canEdit={memoryEnabled}
                    empty={t('empty')}
                    topics={values
                      .filter(value => value.content.trim())
                      .map(value => ({
                        key: value.key,
                        title: agentMemoryTitle(value, tItems),
                        content: value.content,
                        meta: agentMemoryMeta(value, tItems),
                        undoAutomatic:
                          agentMemoryUndoable(value) && value.last_operation_id
                            ? () => undoOperation(value.last_operation_id as string)
                            : undefined,
                      }))}
                    onSave={save}
                    onDeleteTopic={remove}
                    addTargets={agentMemoryAddTargets(values, tItems)}
                    onAdd={(key, fact) =>
                      save(
                        key,
                        joinFacts([...splitFacts(valuesByKey.get(key)?.content ?? ''), fact])
                      )
                    }
                    menu={
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button
                            variant="outline"
                            size="sm"
                            isIcon
                            aria-label={t('moreActions')}
                            disabled={Boolean(busyKey)}
                          >
                            <MoreHorizontal className="size-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem onSelect={() => void exportValues()}>
                            <Download />
                            {t('export')}
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            variant="destructive"
                            disabled={!hasSavedValues}
                            onSelect={() => setDeleteAllOpen(true)}
                          >
                            <Trash2 />
                            {t('deleteAll')}
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    }
                  />
                )}
              </TabsContent>
            </Tabs>
          </DialogBody>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={deleteAllOpen}
        onOpenChange={setDeleteAllOpen}
        title={t('deleteAllConfirmTitle')}
        description={t('deleteAllConfirmDescription')}
        confirmText={t('deleteAll')}
        cancelText={t('cancel')}
        variant="danger"
        loading={busyKey === '__all__'}
        onConfirm={() => void removeAll()}
      />
    </>
  );
}
