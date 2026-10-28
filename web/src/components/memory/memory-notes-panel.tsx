'use client';

import { useEffect, useRef, useState } from 'react';
import { Loader2, Trash2 } from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { useT } from '@/i18n';
import type { MemoryNote } from '@/services/types/memory-history';
import { getErrorMessage } from '@/utils/error-notifications';

interface MemoryNotesPanelProps {
  load: () => Promise<MemoryNote[]>;
  /** Deletes a note; the person may delete notes but not edit them. */
  remove?: (note: MemoryNote) => Promise<unknown>;
  /** Changes when the memory was edited elsewhere, so the notes reload. */
  refreshKey?: unknown;
}

// Shows a DigitalHuman's notes on the user (ADR 0015 /user/notes/), such as
// commitments and open loops. The DigitalHuman keeps them, so the user can
// read and delete them but not edit them.
export function MemoryNotesPanel({ load, remove, refreshKey }: MemoryNotesPanelProps) {
  const t = useT('common.memoryNotes');
  const [notes, setNotes] = useState<MemoryNote[]>([]);
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState(false);
  const [pending, setPending] = useState<MemoryNote | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [reloadKey, setReloadKey] = useState(0);
  const loadRef = useRef(load);

  useEffect(() => {
    loadRef.current = load;
  }, [load]);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setFailed(false);
    loadRef
      .current()
      .then(next => {
        if (!cancelled) setNotes(next ?? []);
      })
      .catch(() => {
        if (!cancelled) setFailed(true);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [refreshKey, reloadKey]);

  const confirmDelete = async () => {
    if (!pending || !remove) return;
    setDeleting(true);
    try {
      await remove(pending);
      toast.success(t('deleted'));
      setPending(null);
      setReloadKey(value => value + 1);
    } catch (error) {
      toast.error(getErrorMessage(error) || t('deleteFailed'));
    } finally {
      setDeleting(false);
    }
  };

  if (failed) {
    return (
      <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
        {t('unavailable')}
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <div className="text-xs text-muted-foreground">{t('description')}</div>
      {loading && notes.length === 0 ? (
        <div className="flex min-h-24 items-center justify-center">
          <Loader2 className="size-5 animate-spin text-muted-foreground" />
        </div>
      ) : notes.length === 0 ? (
        <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
          {t('empty')}
        </div>
      ) : (
        notes.map(note => (
          <section key={note.path} className="space-y-2 rounded-lg border p-3 text-sm">
            <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
              <code className="truncate">{note.path}</code>
              {note.tags ? <Badge variant="secondary">{note.tags}</Badge> : null}
              <span>{new Date(note.updated_at).toLocaleString()}</span>
              {remove ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="xs"
                  className="ml-auto"
                  aria-label={t('delete')}
                  onClick={() => setPending(note)}
                >
                  <Trash2 className="size-3" />
                </Button>
              ) : null}
            </div>
            <div className="whitespace-pre-wrap break-words">{note.content}</div>
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
