'use client';

import * as React from 'react';
import { FileMusic, Music2 } from 'lucide-react';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { useT } from '@/i18n';
import type { MusicTask } from '@/services/types/music';

interface MusicLyricsDialogProps {
  task: MusicTask | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

function StructuredLyrics({ lyrics }: { lyrics: string }) {
  return (
    <div className="space-y-1 font-mono text-sm leading-7 text-foreground/85">
      {lyrics.split('\n').map((line, index) => {
        const value = line.trim();
        if (!value) return <div key={index} className="h-3" aria-hidden="true" />;
        if (/^\[[^\]]+\]$/.test(value)) {
          return (
            <h3
              key={index}
              className="pt-4 font-sans text-xs font-semibold uppercase tracking-[0.16em] text-muted-foreground first:pt-0"
            >
              {value.slice(1, -1)}
            </h3>
          );
        }
        return <p key={index}>{line}</p>;
      })}
    </div>
  );
}

export function MusicLyricsDialog({ task, open, onOpenChange }: MusicLyricsDialogProps) {
  const t = useT('music');
  const hasLyrics = Boolean(task?.lyrics?.trim());

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg" className="max-h-[min(760px,calc(100vh-2rem))] overflow-hidden p-0">
        <DialogHeader className="border-b border-border px-7 pb-5 pt-7">
          <div className="flex items-center gap-3 pr-8">
            <div className="flex size-10 items-center justify-center rounded-xl bg-foreground text-background">
              <FileMusic className="size-5" />
            </div>
            <div className="min-w-0">
              <DialogTitle className="truncate">{task?.title || t('lyricsTitle')}</DialogTitle>
              <DialogDescription className="mt-1">{t('generatedLyrics')}</DialogDescription>
            </div>
          </div>
          {task?.style_tags?.length ? (
            <div className="flex flex-wrap gap-1.5 pt-4">
              {task.style_tags.map(tag => (
                <span
                  key={tag}
                  className="rounded-full border border-border px-2.5 py-1 text-[11px] text-muted-foreground"
                >
                  {tag}
                </span>
              ))}
            </div>
          ) : null}
        </DialogHeader>
        <DialogBody className="px-7 py-6">
          {hasLyrics && task ? (
            <StructuredLyrics lyrics={task.lyrics ?? ''} />
          ) : (
            <div className="flex min-h-60 flex-col items-center justify-center text-center">
              <div className="flex size-12 items-center justify-center rounded-full bg-muted">
                <Music2 className="size-5 text-muted-foreground" />
              </div>
              <p className="mt-4 max-w-sm text-sm leading-6 text-muted-foreground">
                {task?.mode === 'instrumental'
                  ? t('noLyricsInstrumental')
                  : t('noLyricsHistorical')}
              </p>
            </div>
          )}
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
