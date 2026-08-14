'use client';

import {
  AlertCircle,
  ArrowRight,
  Download,
  FileText,
  Loader2,
  MoreHorizontal,
  Pause,
  Play,
  RotateCcw,
  Search,
  Trash2,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { MusicTask, MusicTaskStatus } from '@/services/types/music';
import { getErrorMessage } from '@/utils/error-notifications';
import { formatMusicDuration } from './music-task-state';
import { MusicWaveform } from './music-waveform';
import type { MusicWaveformData } from './music-waveform-data';

const TRACK_ACCENTS = [
  'bg-gradient-to-br from-[#ffd9dc] via-[#f8dce3] to-[#f7e8df] text-[#171717] hover:saturate-125 dark:from-rose-900 dark:via-pink-900 dark:to-orange-950 dark:text-rose-50',
  'bg-gradient-to-br from-[#ddd6fe] via-[#e9d5ff] to-[#f5d0fe] text-[#171717] hover:saturate-125 dark:from-violet-900 dark:via-purple-900 dark:to-fuchsia-950 dark:text-violet-50',
  'bg-gradient-to-br from-[#fef3c7] via-[#fde7c7] to-[#fed7aa] text-[#171717] hover:saturate-125 dark:from-amber-900 dark:via-orange-900 dark:to-orange-950 dark:text-amber-50',
  'bg-gradient-to-br from-[#cffafe] via-[#dbeafe] to-[#ddd6fe] text-[#171717] hover:saturate-125 dark:from-cyan-900 dark:via-sky-900 dark:to-indigo-950 dark:text-cyan-50',
] as const;

function statusLabel(status: MusicTaskStatus, t: ReturnType<typeof useT<'music'>>): string {
  switch (status) {
    case 'queued':
      return t('status.queued');
    case 'generating_lyrics':
      return t('status.generatingLyrics');
    case 'generating':
      return t('status.generating');
    case 'succeeded':
      return t('status.succeeded');
    case 'failed':
      return t('status.failed');
    case 'compensation_pending':
      return t('status.compensationPending');
  }
}

interface MusicTrackListProps {
  tasks: MusicTask[];
  isLoading: boolean;
  error: unknown;
  onRetry: () => void;
  selectedTaskId: string | null;
  playerTaskId: string | null;
  isPlaying: boolean;
  playbackProgress: number;
  onSelect: (task: MusicTask) => void;
  onPlay: (task: MusicTask) => void;
  onSeek: (task: MusicTask, progress: number) => void;
  onShowLyrics: (task: MusicTask) => void;
  onReuse: (task: MusicTask) => void;
  onDownload: (task: MusicTask) => void;
  onDelete: (task: MusicTask) => void;
  downloadingTaskId: string | null;
  deletingTaskId: string | null;
  searchInput: string;
  onSearchChange: (value: string) => void;
  page: number;
  hasMore: boolean;
  onPageChange: (page: number) => void;
  waveformDataByTaskId: Record<string, MusicWaveformData>;
}

export function MusicTrackList({
  tasks,
  isLoading,
  error,
  onRetry,
  selectedTaskId,
  playerTaskId,
  isPlaying,
  playbackProgress,
  onSelect,
  onPlay,
  onSeek,
  onShowLyrics,
  onReuse,
  onDownload,
  onDelete,
  downloadingTaskId,
  deletingTaskId,
  searchInput,
  onSearchChange,
  page,
  hasMore,
  onPageChange,
  waveformDataByTaskId,
}: MusicTrackListProps) {
  const t = useT('music');

  return (
    <section className="flex min-h-[520px] min-w-0 flex-col bg-background lg:min-h-0">
      <div data-ui="music-results-toolbar" className="shrink-0 pb-4">
        <div className="relative min-w-0">
          <Search className="pointer-events-none absolute left-4 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={searchInput}
            onChange={event => onSearchChange(event.target.value)}
            className="h-12 rounded-xl border-border bg-background pl-11 shadow-none"
            placeholder={t('searchPlaceholder')}
          />
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto pr-1">
        {isLoading ? (
          <div className="space-y-3">
            {Array.from({ length: 5 }).map((_, index) => (
              <Skeleton key={index} className="h-[92px] w-full rounded-2xl" />
            ))}
          </div>
        ) : error ? (
          <div className="flex min-h-64 flex-col items-center justify-center text-center">
            <AlertCircle className="size-6 text-destructive" />
            <p className="mt-3 text-sm text-muted-foreground">
              {getErrorMessage(error) || t('historyLoadFailed')}
            </p>
            <Button className="mt-4" size="sm" variant="outline" onClick={onRetry}>
              {t('retry')}
            </Button>
          </div>
        ) : tasks.length ? (
          <div className="space-y-3">
            {tasks.map((task, taskIndex) => {
              const active =
                task.status === 'queued' ||
                task.status === 'generating_lyrics' ||
                task.status === 'generating' ||
                task.status === 'compensation_pending';
              const playable = task.status === 'succeeded';
              const rowPlaying = playerTaskId === task.id && isPlaying;
              const waveform = waveformDataByTaskId[task.id];
              const accent = TRACK_ACCENTS[taskIndex % TRACK_ACCENTS.length];
              const selected = selectedTaskId === task.id;
              return (
                <article
                  key={task.id}
                  data-ui="music-track-card"
                  className={cn(
                    'group rounded-2xl border px-3.5 py-3 transition-[border-color,box-shadow,background-color] sm:px-4',
                    selected
                      ? 'border-[rgb(231,231,229)] bg-[rgb(250,250,249)] shadow-sm dark:border-border dark:bg-muted/25'
                      : 'border-border bg-background hover:border-foreground/20 hover:bg-muted/10'
                  )}
                >
                  <div
                    data-ui="music-track-meta"
                    className="flex min-w-0 flex-wrap items-start gap-3"
                  >
                    <button
                      type="button"
                      className="min-w-0 flex-1 text-left"
                      onClick={() => onSelect(task)}
                    >
                      <span className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
                        <h3 className="max-w-full truncate text-sm font-semibold sm:max-w-[50%]">
                          {task.title || task.prompt}
                        </h3>
                        <span className="shrink-0 rounded-lg bg-foreground/[0.055] px-2 py-0.5 text-[10px] font-medium text-muted-foreground">
                          {task.model}
                        </span>
                        {task.style_tags?.slice(0, 2).map(tag => (
                          <span key={tag} className="truncate text-[11px] text-muted-foreground">
                            · {tag}
                          </span>
                        ))}
                        {task.status !== 'succeeded' ? (
                          <span className="shrink-0 rounded-full bg-foreground/[0.055] px-2 py-0.5 text-[10px] text-muted-foreground">
                            {statusLabel(task.status, t)}
                          </span>
                        ) : null}
                      </span>
                    </button>
                    {task.mode !== 'instrumental' ? (
                      <Button
                        size="sm"
                        variant="ghost"
                        className="h-8 shrink-0 gap-1 rounded-lg px-2 text-xs"
                        aria-label={t('viewLyrics')}
                        title={t('viewLyrics')}
                        onClick={() => onShowLyrics(task)}
                      >
                        <FileText className="size-3.5" />
                        <span className="hidden xl:inline">{t('viewLyrics')}</span>
                        <ArrowRight className="hidden size-3.5 xl:block" />
                      </Button>
                    ) : null}
                    {active ? (
                      <p
                        data-ui="music-generation-wait-hint"
                        className="basis-full text-[11px] leading-5 text-muted-foreground"
                      >
                        {t('generationWaitHint')}
                      </p>
                    ) : null}
                  </div>

                  <div
                    data-ui="music-track-controls"
                    className="mt-2.5 grid min-w-0 grid-cols-[2.75rem_minmax(0,1fr)_2.75rem_auto_auto] items-center gap-2 sm:gap-3"
                  >
                    <button
                      type="button"
                      aria-label={rowPlaying ? t('pause') : t('play')}
                      disabled={!playable}
                      onClick={() => onPlay(task)}
                      className={cn(
                        'flex size-11 shrink-0 items-center justify-center rounded-full shadow-sm ring-1 ring-black/[0.025] transition-[filter,transform] active:scale-95',
                        playable ? accent : 'bg-muted text-muted-foreground shadow-none'
                      )}
                    >
                      {active ? (
                        <Loader2 className="size-4 animate-spin" />
                      ) : rowPlaying ? (
                        <Pause className="size-4 fill-current" />
                      ) : (
                        <Play className="ml-0.5 size-4 fill-current" />
                      )}
                    </button>
                    <div data-ui="music-track-waveform" className="min-w-0 overflow-hidden">
                      <MusicWaveform
                        className="w-full overflow-hidden"
                        compact
                        peaks={waveform?.peaks ?? task.waveform_peaks ?? []}
                        progress={playerTaskId === task.id ? playbackProgress : 0}
                        onSeek={playable ? progress => onSeek(task, progress) : undefined}
                        label={t('seekTrack')}
                      />
                    </div>
                    <span
                      data-ui="music-track-duration"
                      className="block w-11 shrink-0 text-right text-[11px] tabular-nums text-muted-foreground"
                    >
                      {task.duration_ms || waveform?.durationMS
                        ? formatMusicDuration(task.duration_ms || waveform?.durationMS)
                        : ''}
                    </span>
                    {playable ? (
                      <Button
                        size="sm"
                        variant="ghost"
                        className="size-9 shrink-0 rounded-xl"
                        data-ui="music-track-download"
                        disabled={downloadingTaskId === task.id}
                        aria-label={t('download')}
                        title={t('download')}
                        onClick={() => onDownload(task)}
                      >
                        {downloadingTaskId === task.id ? (
                          <Loader2 className="size-4 animate-spin" />
                        ) : (
                          <Download className="size-4" />
                        )}
                      </Button>
                    ) : null}
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button
                          size="sm"
                          variant="ghost"
                          className="size-9 shrink-0 rounded-xl"
                          aria-label={t('moreActions')}
                          title={t('moreActions')}
                        >
                          <MoreHorizontal className="size-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="w-40 rounded-xl p-1.5">
                        <DropdownMenuItem
                          className="rounded-lg py-2"
                          onSelect={() => onReuse(task)}
                        >
                          <RotateCcw />
                          {t('reuse')}
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          className="rounded-lg py-2 text-destructive focus:text-destructive"
                          disabled={active || deletingTaskId === task.id}
                          onSelect={() => onDelete(task)}
                        >
                          {deletingTaskId === task.id ? (
                            <Loader2 className="animate-spin" />
                          ) : (
                            <Trash2 />
                          )}
                          {t('delete')}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                </article>
              );
            })}
          </div>
        ) : (
          <div className="flex min-h-64 items-center justify-center text-sm text-muted-foreground">
            {t('emptyHistory')}
          </div>
        )}
      </div>

      {page > 1 || hasMore ? (
        <div className="mt-3 flex items-center justify-between border-t border-border pt-3">
          <Button
            size="sm"
            variant="ghost"
            disabled={page <= 1}
            onClick={() => onPageChange(Math.max(1, page - 1))}
          >
            {t('previousPage')}
          </Button>
          <span className="text-xs text-muted-foreground">{t('page', { page })}</span>
          <Button
            size="sm"
            variant="ghost"
            disabled={!hasMore}
            onClick={() => onPageChange(page + 1)}
          >
            {t('nextPage')}
          </Button>
        </div>
      ) : null}
    </section>
  );
}
