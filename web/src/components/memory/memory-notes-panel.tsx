'use client';

import { useEffect, useRef, useState } from 'react';
import { Loader2 } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { useT } from '@/i18n';
import type { MemoryNote } from '@/services/types/memory-history';

interface MemoryNotesPanelProps {
  load: () => Promise<MemoryNote[]>;
  /** Changes when the memory was edited elsewhere, so the notes reload. */
  refreshKey?: unknown;
}

// Shows a DigitalHuman's notes on the user (ADR 0014 /user/notes/), such as
// commitments and open loops. The DigitalHuman keeps them, so they are
// read-only here.
export function MemoryNotesPanel({ load, refreshKey }: MemoryNotesPanelProps) {
  const t = useT('common.memoryNotes');
  const [notes, setNotes] = useState<MemoryNote[]>([]);
  const [loading, setLoading] = useState(false);
  const [failed, setFailed] = useState(false);
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
  }, [refreshKey]);

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
            </div>
            <div className="whitespace-pre-wrap break-words">{note.content}</div>
          </section>
        ))
      )}
    </div>
  );
}
