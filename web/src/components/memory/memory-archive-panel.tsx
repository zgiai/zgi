'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { Loader2, Pencil, Trash2 } from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Textarea } from '@/components/ui/textarea';
import { useT } from '@/i18n';
import type { MemoryArchiveEntry } from '@/services/types/memory-history';
import { getErrorMessage } from '@/utils/error-notifications';

interface MemoryArchivePanelProps {
  load: () => Promise<MemoryArchiveEntry[]>;
  remove: (entryId: string) => Promise<unknown>;
  /** Saves new text for an entry; without it entries are read-only. */
  update?: (entryId: string, content: string) => Promise<unknown>;
  canManage?: boolean;
  /** Changes when the memory was edited elsewhere, so the archive reloads. */
  refreshKey?: unknown;
}

// Shows a user's memory archive (ADR 0014): older memory moved out of full
// files, which the assistant searches when needed. Users can remove entries.
export function MemoryArchivePanel({
  load,
  remove,
  update,
  canManage = true,
  refreshKey,
}: MemoryArchivePanelProps) {
  const t = useT('common.memoryArchive');
  const [entries, setEntries] = useState<MemoryArchiveEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState(false);
  const [pending, setPending] = useState<MemoryArchiveEntry | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [editing, setEditing] = useState<{ id: string; content: string } | null>(null);
  const [saving, setSaving] = useState(false);
  const loadRef = useRef(load);

  useEffect(() => {
    loadRef.current = load;
  }, [load]);

  const reload = useCallback(async () => {
    setLoading(true);
    setFailed(false);
    try {
      setEntries((await loadRef.current()) ?? []);
    } catch {
      setFailed(true);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload, refreshKey]);

  const confirmDelete = async () => {
    if (!pending) return;
    setDeleting(true);
    try {
      await remove(pending.id);
      toast.success(t('deleted'));
      setPending(null);
      await reload();
    } catch (error) {
      toast.error(getErrorMessage(error) || t('deleteFailed'));
    } finally {
      setDeleting(false);
    }
  };

  const save = async () => {
    if (!editing || !update) return;
    setSaving(true);
    try {
      await update(editing.id, editing.content);
      toast.success(t('saved'));
      setEditing(null);
      await reload();
    } catch (error) {
      toast.error(getErrorMessage(error) || t('saveFailed'));
    } finally {
      setSaving(false);
    }
  };

  if (failed && entries.length === 0) {
    return (
      <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
        {t('unavailable')}
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <div className="text-xs text-muted-foreground">{t('description')}</div>
      {loading && entries.length === 0 ? (
        <div className="flex min-h-24 items-center justify-center">
          <Loader2 className="size-5 animate-spin text-muted-foreground" />
        </div>
      ) : entries.length === 0 ? (
        <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
          {t('empty')}
        </div>
      ) : (
        entries.map(entry => (
          <section key={entry.id} className="space-y-2 rounded-lg border p-3 text-sm">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="flex min-w-0 flex-wrap items-center gap-2 text-xs text-muted-foreground">
                {entry.tags ? <Badge variant="secondary">{entry.tags}</Badge> : null}
                {entry.source_key ? (
                  <span className="truncate">{t('from', { key: entry.source_key })}</span>
                ) : null}
                <span>{new Date(entry.created_at).toLocaleString()}</span>
              </div>
              {canManage && editing?.id !== entry.id ? (
                <div className="flex items-center gap-1">
                  {update ? (
                    <Button
                      type="button"
                      variant="ghost"
                      size="xs"
                      aria-label={t('edit')}
                      onClick={() => setEditing({ id: entry.id, content: entry.content })}
                    >
                      <Pencil className="size-3" />
                    </Button>
                  ) : null}
                  <Button
                    type="button"
                    variant="ghost"
                    size="xs"
                    aria-label={t('delete')}
                    onClick={() => setPending(entry)}
                  >
                    <Trash2 className="size-3" />
                  </Button>
                </div>
              ) : null}
            </div>
            {editing?.id === entry.id ? (
              <div className="space-y-2">
                <Textarea
                  value={editing.content}
                  maxLength={2000}
                  onChange={event => setEditing({ id: entry.id, content: event.target.value })}
                />
                <div className="flex justify-end gap-2">
                  <Button type="button" variant="ghost" size="sm" onClick={() => setEditing(null)}>
                    {t('cancel')}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    disabled={saving || !editing.content.trim()}
                    onClick={() => void save()}
                  >
                    {saving ? <Loader2 className="mr-1.5 size-3.5 animate-spin" /> : null}
                    {t('save')}
                  </Button>
                </div>
              </div>
            ) : (
              <div className="whitespace-pre-wrap break-words">{entry.content}</div>
            )}
          </section>
        ))
      )}
      <ConfirmDialog
        open={Boolean(pending)}
        onOpenChange={next => !next && setPending(null)}
        title={t('deleteConfirmTitle')}
        description={t('deleteConfirmDescription')}
        confirmText={t('delete')}
        cancelText={t('cancel')}
        variant="warning"
        loading={deleting}
        onConfirm={() => void confirmDelete()}
      />
    </div>
  );
}
