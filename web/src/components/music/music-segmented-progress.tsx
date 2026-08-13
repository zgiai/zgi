'use client';

import * as React from 'react';
import { cn } from '@/lib/utils';

interface MusicSegmentedProgressProps {
  segments: number;
  progress: number;
  onSeek?: (progress: number) => void;
  label: string;
  className?: string;
}

export function MusicSegmentedProgress({
  segments,
  progress,
  onSeek,
  label,
  className,
}: MusicSegmentedProgressProps) {
  const segmentCount = Math.max(1, Math.min(12, Math.round(segments)));
  const normalizedProgress = Math.min(1, Math.max(0, progress));

  function seekFromPointer(event: React.MouseEvent<HTMLButtonElement>) {
    if (!onSeek || event.detail === 0) return;
    const bounds = event.currentTarget.getBoundingClientRect();
    if (bounds.width > 0) onSeek((event.clientX - bounds.left) / bounds.width);
  }

  return (
    <button
      type="button"
      data-ui="music-player-segmented-progress"
      aria-label={label}
      disabled={!onSeek}
      className={cn(
        'grid h-5 min-w-0 flex-1 cursor-pointer items-center gap-1.5 disabled:cursor-default',
        className
      )}
      style={{ gridTemplateColumns: `repeat(${segmentCount}, minmax(0, 1fr))` }}
      onClick={seekFromPointer}
      onKeyDown={event => {
        if (!onSeek) return;
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
      {Array.from({ length: segmentCount }).map((_, index) => {
        const segmentProgress = Math.min(
          1,
          Math.max(0, normalizedProgress * segmentCount - index)
        );
        return (
          <span
            key={index}
            data-ui="music-player-progress-track-segment"
            className="relative h-1.5 overflow-hidden rounded-full bg-[rgb(209,211,216)] dark:bg-white/20"
            aria-hidden="true"
          >
            <span
              className="absolute inset-y-0 left-0 rounded-full bg-foreground transition-[width] duration-100"
              style={{ width: `${segmentProgress * 100}%` }}
            />
          </span>
        );
      })}
    </button>
  );
}
