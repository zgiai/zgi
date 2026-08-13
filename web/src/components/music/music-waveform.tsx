'use client';

import * as React from 'react';
import { cn } from '@/lib/utils';

interface MusicWaveformProps {
  peaks: number[];
  progress?: number;
  onSeek?: (progress: number) => void;
  compact?: boolean;
  className?: string;
  label?: string;
}

export function MusicWaveform({
  peaks,
  progress = 0,
  onSeek,
  compact = false,
  className,
  label = 'Music waveform',
}: MusicWaveformProps) {
  const normalizedProgress = Math.min(1, Math.max(0, progress));
  const normalizedPeaks = React.useMemo(
    () => peaks.map(peak => Math.min(100, Math.max(0, Number.isFinite(peak) ? peak : 0))),
    [peaks]
  );

  const bars = normalizedPeaks.length ? (
    <span
      className={cn(
        'flex w-full min-w-0 items-center gap-px overflow-hidden',
        compact ? 'h-9' : 'h-12'
      )}
    >
      {normalizedPeaks.map((peak, index) => {
        const played = (index + 1) / normalizedPeaks.length <= normalizedProgress;
        return (
          <span
            key={index}
            className={cn(
              'min-w-px flex-1 rounded-full transition-colors',
              played ? 'bg-foreground' : 'bg-foreground/20'
            )}
            style={{ height: `${Math.max(compact ? 8 : 10, peak)}%` }}
          />
        );
      })}
    </span>
  ) : (
    <span className={cn('flex w-full items-center', compact ? 'h-9' : 'h-12')} aria-hidden="true">
      <span className="h-px w-full bg-border" />
    </span>
  );

  if (!onSeek) return <div className={cn('min-w-0 overflow-hidden', className)}>{bars}</div>;

  return (
    <button
      type="button"
      aria-label={label}
      className={cn('block min-w-0 cursor-pointer overflow-hidden', className)}
      onClick={event => {
        if (event.detail === 0) return;
        const bounds = event.currentTarget.getBoundingClientRect();
        if (bounds.width > 0) onSeek((event.clientX - bounds.left) / bounds.width);
      }}
      onKeyDown={event => {
        if (event.key === 'ArrowLeft') {
          event.preventDefault();
          onSeek(normalizedProgress - 0.05);
        }
        if (event.key === 'ArrowRight') {
          event.preventDefault();
          onSeek(normalizedProgress + 0.05);
        }
      }}
    >
      {bars}
    </button>
  );
}
