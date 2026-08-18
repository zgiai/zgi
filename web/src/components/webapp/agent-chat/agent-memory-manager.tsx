'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Brain, Download, Loader2, RotateCcw, Save, Trash2 } from 'lucide-react';
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
import { Textarea } from '@/components/ui/textarea';
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
  const [open, setOpen] = useState(false);
  const [values, setValues] = useState<WebAppAgentMemoryValue[]>([]);
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(false);
  const [busyKey, setBusyKey] = useState('');
  const [deleteKey, setDeleteKey] = useState<string | null>(null);
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
      setDrafts(Object.fromEntries(nextValues.map(value => [value.key, value.content])));
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

  const save = async (value: WebAppAgentMemoryValue) => {
    const content = (drafts[value.key] ?? '').trim();
    if (!content || content === value.content) return;
    setBusyKey(value.key);
    try {
      await WebAppService.updateAgentMemory(webAppId, value.key, content, value.revision);
      toast.success(t('saved'));
      await load();
    } catch (error) {
      toast.error(getErrorMessage(error) || t('saveFailed'));
      await load();
    } finally {
      setBusyKey('');
    }
  };

  const remove = async () => {
    if (!deleteKey) return;
    const value = valuesByKey.get(deleteKey);
    if (!value) return;
    setBusyKey(deleteKey);
    try {
      await WebAppService.deleteAgentMemory(webAppId, deleteKey, value.revision);
      toast.success(t('deleted'));
      setDeleteKey(null);
      await load();
    } catch (error) {
      toast.error(getErrorMessage(error) || t('deleteFailed'));
      await load();
    } finally {
      setBusyKey('');
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

  const undo = async (value: WebAppAgentMemoryValue) => {
    if (!value.last_operation_id) return;
    setBusyKey(value.key);
    try {
      await WebAppService.undoAgentMemoryOperation(webAppId, value.last_operation_id);
      toast.success(t('undone'));
      await load();
    } catch (error) {
      toast.error(getErrorMessage(error) || t('undoFailed'));
      await load();
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
            {loading ? (
              <div className="flex min-h-40 items-center justify-center">
                <Loader2 className="size-5 animate-spin text-muted-foreground" />
              </div>
            ) : values.length === 0 ? (
              <div className="rounded-lg border border-dashed p-8 text-center text-sm text-muted-foreground">
                {t('empty')}
              </div>
            ) : (
              values.map(value => {
                const draft = drafts[value.key] ?? '';
                const isBusy = busyKey === value.key;
                const undoable =
                  value.source_kind === 'automatic' &&
                  Boolean(value.last_operation_id) &&
                  Boolean(value.undoable_until && value.undoable_until > Date.now() / 1000);
                return (
                  <section key={value.key} className="space-y-3 rounded-xl border p-4">
                    <div className="flex flex-wrap items-start justify-between gap-2">
                      <div>
                        <div className="font-medium">{value.name || value.key}</div>
                        <div className="mt-1 text-xs text-muted-foreground">
                          {t('metadata', {
                            source: value.source_kind
                              ? t(`sources.${value.source_kind}`)
                              : t('notSaved'),
                            revision: value.revision,
                            time: value.updated_at
                              ? new Date(value.updated_at * 1000).toLocaleString()
                              : t('notSaved'),
                          })}
                        </div>
                      </div>
                      <Badge variant="outline">{value.key}</Badge>
                    </div>
                    <Textarea
                      value={draft}
                      maxLength={value.max_chars}
                      rows={4}
                      disabled={isBusy || !memoryEnabled}
                      onChange={event =>
                        setDrafts(current => ({ ...current, [value.key]: event.target.value }))
                      }
                    />
                    <div className="flex flex-wrap justify-end gap-2">
                      {undoable ? (
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          disabled={isBusy}
                          onClick={() => void undo(value)}
                        >
                          <RotateCcw className="mr-1.5 size-3.5" />
                          {t('undo')}
                        </Button>
                      ) : null}
                      {value.content ? (
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          disabled={isBusy}
                          onClick={() => setDeleteKey(value.key)}
                        >
                          <Trash2 className="mr-1.5 size-3.5" />
                          {t('delete')}
                        </Button>
                      ) : null}
                      <Button
                        type="button"
                        size="sm"
                        disabled={
                          isBusy || !memoryEnabled || !draft.trim() || draft === value.content
                        }
                        onClick={() => void save(value)}
                      >
                        {isBusy ? (
                          <Loader2 className="mr-1.5 size-3.5 animate-spin" />
                        ) : (
                          <Save className="mr-1.5 size-3.5" />
                        )}
                        {t('save')}
                      </Button>
                    </div>
                  </section>
                );
              })
            )}
          </DialogBody>
          <DialogFooter className="justify-between">
            <Button
              type="button"
              variant="destructive"
              disabled={!hasSavedValues || Boolean(busyKey)}
              onClick={() => setDeleteAllOpen(true)}
            >
              <Trash2 className="mr-1.5 size-4" />
              {t('deleteAll')}
            </Button>
            <Button
              type="button"
              variant="outline"
              disabled={Boolean(busyKey)}
              onClick={() => void exportValues()}
            >
              <Download className="mr-1.5 size-4" />
              {t('export')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={Boolean(deleteKey)}
        onOpenChange={next => !next && setDeleteKey(null)}
        title={t('deleteConfirmTitle')}
        description={t('deleteConfirmDescription')}
        confirmText={t('delete')}
        cancelText={t('cancel')}
        variant="danger"
        loading={Boolean(deleteKey && busyKey === deleteKey)}
        onConfirm={() => void remove()}
      />
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
