'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Brain, Download, Loader2, MoreHorizontal, Plus, Trash2 } from 'lucide-react';
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
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Bumped when a reply changes memory, so the entry can show a new-memory dot. */
  changeTick?: number;
}

type MemoryTab = 'memory' | 'history' | 'archive';

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

// When the user last looked at this assistant's memory, in Unix seconds. The
// entry shows a dot for anything the assistant remembered after that.
function seenKey(webAppId: string) {
  return `zgi:agent-memory-seen:${webAppId}`;
}

function readSeen(webAppId: string): number | null {
  try {
    const value = Number(window.localStorage.getItem(seenKey(webAppId)));
    return Number.isFinite(value) && value > 0 ? value : null;
  } catch {
    return null;
  }
}

function writeSeen(webAppId: string, seconds: number) {
  try {
    window.localStorage.setItem(seenKey(webAppId), String(seconds));
  } catch {
    // A missing dot is harmless.
  }
}

export function AgentMemoryManager({
  webAppId,
  memoryEnabled,
  open,
  onOpenChange,
  changeTick = 0,
}: AgentMemoryManagerProps) {
  const t = useT('webapp.agentChat.memory');
  const tItems = useT('common.memoryItems');
  const tHistory = useT('common.memoryHistory');
  const tArchive = useT('common.memoryArchive');
  const [tab, setTab] = useState<MemoryTab>('memory');
  const [values, setValues] = useState<WebAppAgentMemoryValue[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [loading, setLoading] = useState(false);
  const [busyKey, setBusyKey] = useState('');
  const [deleteAllOpen, setDeleteAllOpen] = useState(false);
  const [addOpen, setAddOpen] = useState(false);
  const [seenAt, setSeenAt] = useState<number | null>(null);
  const isLoadingRef = useRef(false);
  const tRef = useRef(t);

  useEffect(() => {
    tRef.current = t;
  }, [t]);

  const load = useCallback(
    async (quiet = false) => {
      if (isLoadingRef.current) return;
      isLoadingRef.current = true;
      if (!quiet) setLoading(true);
      try {
        const response = await WebAppService.getAgentMemory(webAppId);
        setValues(response.data.values ?? []);
        setLoaded(true);
      } catch (error) {
        if (!quiet) toast.error(getErrorMessage(error) || tRef.current('loadFailed'));
      } finally {
        isLoadingRef.current = false;
        if (!quiet) setLoading(false);
      }
    },
    [webAppId]
  );

  // The first visit marks everything as seen; later visits compare against it.
  useEffect(() => {
    const seen = readSeen(webAppId);
    if (seen) {
      setSeenAt(seen);
      return;
    }
    const now = Math.floor(Date.now() / 1000);
    writeSeen(webAppId, now);
    setSeenAt(now);
  }, [webAppId]);

  // Load quietly up front and after each reply that changed memory, so the
  // entry knows whether there is something new.
  useEffect(() => {
    if (memoryEnabled) void load(true);
  }, [load, memoryEnabled, changeTick]);

  useEffect(() => {
    if (!open) return;
    setTab('memory');
    void load(loaded);
    const now = Math.floor(Date.now() / 1000);
    writeSeen(webAppId, now);
    setSeenAt(now);
    // Opening is what matters; reloading on every refresh would loop.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, webAppId]);

  const valuesByKey = useMemo(() => new Map(values.map(value => [value.key, value])), [values]);
  const savedValues = useMemo(() => values.filter(value => value.content.trim()), [values]);
  const hasNew =
    !open &&
    seenAt !== null &&
    savedValues.some(
      value =>
        value.source_kind !== 'manager' &&
        typeof value.updated_at === 'number' &&
        value.updated_at > seenAt
    );

  // The board reports success with its own toast; failures are shown here
  // and rethrown so the board keeps the edit open.
  const save = async (key: string, content: string) => {
    const value = valuesByKey.get(key);
    setBusyKey(key);
    try {
      await WebAppService.updateAgentMemory(webAppId, key, content, value?.revision ?? 0);
      await load(true);
    } catch (error) {
      toast.error(getErrorMessage(error) || t('saveFailed'));
      await load(true);
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
      await load(true);
      const operationId = response.data?.last_operation_id;
      const until = response.data?.undoable_until;
      if (operationId && until && until > Date.now() / 1000) {
        return { undo: () => undoOperation(operationId) };
      }
      return undefined;
    } catch (error) {
      toast.error(getErrorMessage(error) || t('deleteFailed'));
      await load(true);
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
      await load(true);
    }
  };

  const removeAll = async () => {
    setBusyKey('__all__');
    try {
      await WebAppService.deleteAllAgentMemory(webAppId);
      toast.success(t('allDeleted'));
      setDeleteAllOpen(false);
      await load(true);
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
        className="relative h-8 gap-1.5 px-2.5 text-muted-foreground hover:text-foreground"
        aria-label={hasNew ? t('entryNew') : t('entry')}
        onClick={() => onOpenChange(true)}
      >
        <Brain className="size-4" />
        <span className="text-xs">{t('entry')}</span>
        {hasNew ? (
          <span
            aria-hidden="true"
            className="absolute right-1 top-1 size-2 rounded-full bg-destructive ring-2 ring-background"
          />
        ) : null}
      </Button>

      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent size="lg" className="max-h-[min(760px,calc(100vh-2rem))]">
          <DialogHeader>
            <DialogTitle>{t('title')}</DialogTitle>
            <DialogDescription>{t('description')}</DialogDescription>
          </DialogHeader>
          <DialogBody>
            <Tabs value={tab} onValueChange={value => setTab(value as MemoryTab)}>
              <div className="flex flex-wrap items-center justify-between gap-2">
                <TabsList>
                  <TabsTrigger value="memory">{t('tabMemory')}</TabsTrigger>
                  <TabsTrigger value="history">{tHistory('title')}</TabsTrigger>
                  <TabsTrigger value="archive">{tArchive('title')}</TabsTrigger>
                </TabsList>
                {tab === 'memory' ? (
                  <div className="flex items-center gap-2">
                    {memoryEnabled && savedValues.length > 0 ? (
                      <Button size="sm" variant="outline" onClick={() => setAddOpen(true)}>
                        <Plus className="mr-1 size-4" />
                        {tItems('add')}
                      </Button>
                    ) : null}
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button
                          variant="outline"
                          size="sm"
                          aria-label={t('moreActions')}
                          disabled={Boolean(busyKey)}
                        >
                          <MoreHorizontal className="mr-1 size-4" />
                          {t('more')}
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
                          disabled={savedValues.length === 0}
                          onSelect={() => setDeleteAllOpen(true)}
                        >
                          <Trash2 />
                          {t('deleteAll')}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                ) : null}
              </div>
              <TabsContent value="memory" className="mt-4">
                {loading && !loaded ? (
                  <div className="flex min-h-40 items-center justify-center">
                    <Loader2 className="size-5 animate-spin text-muted-foreground" />
                  </div>
                ) : (
                  <MemoryBoard
                    canEdit={memoryEnabled}
                    empty={t('empty')}
                    topics={savedValues.map(value => ({
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
                    addOpen={addOpen}
                    onAddOpenChange={setAddOpen}
                  />
                )}
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
                    labelFor={key => {
                      const value = valuesByKey.get(key);
                      return agentMemoryTitle({ key, name: value?.name }, tItems);
                    }}
                    canRevert={memoryEnabled}
                    refreshKey={values}
                    onReverted={() => load(true)}
                  />
                ) : null}
              </TabsContent>
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
