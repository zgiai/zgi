'use client';

import * as React from 'react';
import {
  Download,
  FileText,
  Pause,
  Play,
  RotateCcw,
  RotateCw,
  Volume2,
  VolumeX,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Slider } from '@/components/ui/slider';
import { useT } from '@/i18n';
import type { MusicTask } from '@/services/types/music';
import {
  clampMusicSeekTime,
  formatMusicDuration,
  MUSIC_TIMELINE_SEGMENTS,
  resolveMusicDurationSeconds,
  toMusicDownloadURL,
} from './music-task-state';
import { MusicSegmentedProgress } from './music-segmented-progress';
import type { MusicWaveformData } from './music-waveform-data';
import { useMusicWaveform } from './use-music-waveform';

interface MusicPlayerProps {
  task: MusicTask | null;
  source: string | null;
  playRequestToken: number;
  playRequestAction: 'play' | 'toggle';
  seekRequestToken: number;
  seekRequestProgress: number;
  onPlayingChange: (playing: boolean) => void;
  onProgressChange: (progress: number) => void;
  onShowLyrics: (task: MusicTask) => void;
  onWaveformChange: (taskId: string, waveform: MusicWaveformData) => void;
}

const seekSeconds = 10;

function SeekStepIcon({ direction }: { direction: 'backward' | 'forward' }) {
  const Icon = direction === 'backward' ? RotateCcw : RotateCw;
  return (
    <span className="relative flex size-5 items-center justify-center" aria-hidden="true">
      <Icon className="size-5" />
      <span className="absolute text-[7px] font-bold leading-none">{seekSeconds}</span>
    </span>
  );
}

export function MusicPlayer({
  task,
  source,
  playRequestToken,
  playRequestAction,
  seekRequestToken,
  seekRequestProgress,
  onPlayingChange,
  onProgressChange,
  onShowLyrics,
  onWaveformChange,
}: MusicPlayerProps) {
  const t = useT('music');
  const audioRef = React.useRef<HTMLAudioElement | null>(null);
  const handledPlayRequestTokenRef = React.useRef(0);
  const [playing, setPlaying] = React.useState(false);
  const [muted, setMuted] = React.useState(false);
  const [volume, setVolume] = React.useState(80);
  const [currentTimeMS, setCurrentTimeMS] = React.useState(0);
  const [mediaDurationMS, setMediaDurationMS] = React.useState(0);
  const [waveformLoadSource, setWaveformLoadSource] = React.useState<string | null>(null);
  const waveform = useMusicWaveform(
    source,
    task?.waveform_peaks,
    task?.duration_ms,
    Boolean(source && waveformLoadSource === source)
  );

  const durationMS = mediaDurationMS || waveform.durationMS;
  const progress = durationMS > 0 ? currentTimeMS / durationMS : 0;

  const updatePlaying = React.useCallback(
    (next: boolean) => {
      setPlaying(next);
      onPlayingChange(next);
    },
    [onPlayingChange]
  );

  React.useEffect(() => {
    setCurrentTimeMS(0);
    setMediaDurationMS(task?.duration_ms ?? 0);
    setWaveformLoadSource(null);
    updatePlaying(false);
  }, [source, task?.duration_ms, updatePlaying]);

  React.useEffect(() => {
    if (audioRef.current) audioRef.current.volume = volume / 100;
  }, [source, volume]);

  React.useEffect(() => {
    if (task && waveform.peaks.length > 0) onWaveformChange(task.id, waveform);
  }, [onWaveformChange, task, waveform]);

  React.useEffect(() => {
    const audio = audioRef.current;
    if (!playRequestToken || !source || !audio) return;
    if (handledPlayRequestTokenRef.current === playRequestToken) return;
    handledPlayRequestTokenRef.current = playRequestToken;
    if (playRequestAction === 'toggle' && !audio.paused) {
      audio.pause();
      return;
    }
    void audio.play().catch(() => updatePlaying(false));
  }, [playRequestAction, playRequestToken, source, updatePlaying]);

  const seekTo = React.useCallback(
    (nextProgress: number) => {
      const audio = audioRef.current;
      if (!audio) return;
      const durationSeconds = resolveMusicDurationSeconds(audio.duration, durationMS);
      if (durationSeconds <= 0) return;
      const nextTime = clampMusicSeekTime(
        Math.min(1, Math.max(0, nextProgress)) * durationSeconds,
        audio.duration,
        durationMS
      );
      audio.currentTime = nextTime;
      setCurrentTimeMS(nextTime * 1000);
    },
    [durationMS]
  );

  React.useEffect(() => {
    onProgressChange(progress);
  }, [onProgressChange, progress]);

  React.useEffect(() => {
    const audio = audioRef.current;
    if (!seekRequestToken || !source || !audio) return;

    const applySeek = () => seekTo(seekRequestProgress);
    if (audio.readyState >= HTMLMediaElement.HAVE_METADATA) {
      applySeek();
      return;
    }
    audio.addEventListener('loadedmetadata', applySeek, { once: true });
    return () => audio.removeEventListener('loadedmetadata', applySeek);
  }, [seekRequestProgress, seekRequestToken, seekTo, source]);

  function seekBy(seconds: number) {
    const audio = audioRef.current;
    if (!audio) return;
    const nextTime = clampMusicSeekTime(audio.currentTime + seconds, audio.duration, durationMS);
    audio.currentTime = nextTime;
    setCurrentTimeMS(nextTime * 1000);
  }

  function togglePlayback() {
    const audio = audioRef.current;
    if (!audio || !source) return;
    if (audio.paused) void audio.play().catch(() => updatePlaying(false));
    else audio.pause();
  }

  function changeVolume(values: number[]) {
    const nextVolume = values[0] ?? 0;
    setVolume(nextVolume);
    if (nextVolume > 0) setMuted(false);
  }

  return (
    <footer className="shrink-0 border-t border-border bg-background/95 px-4 py-2 backdrop-blur lg:px-8">
      <audio
        ref={audioRef}
        src={source ?? undefined}
        preload="metadata"
        muted={muted}
        onPlay={() => updatePlaying(true)}
        onPlaying={() => {
          updatePlaying(true);
          setWaveformLoadSource(source);
        }}
        onPause={() => updatePlaying(false)}
        onEnded={() => updatePlaying(false)}
        onTimeUpdate={event => setCurrentTimeMS(event.currentTarget.currentTime * 1000)}
        onLoadedMetadata={event => {
          const seconds = event.currentTarget.duration;
          if (Number.isFinite(seconds)) setMediaDurationMS(seconds * 1000);
        }}
      />

      <div className="mx-auto grid max-w-[1600px] items-center gap-4 lg:grid-cols-[280px_minmax(360px,1fr)_220px]">
        <div className="min-w-0">
          <p className="text-[10px] font-medium text-muted-foreground">
            {task ? t('nowPlaying') : t('selectedTrack')}
          </p>
          <p className="mt-0.5 truncate text-sm font-semibold">
            {task?.title || task?.prompt || t('nothingPlaying')}
          </p>
          {task ? (
            <p className="truncate text-[11px] text-muted-foreground">
              {task.style_tags?.join(' · ') || task.model}
            </p>
          ) : null}
        </div>

        <div className="min-w-0">
          <div data-ui="music-player-controls" className="flex items-center justify-center gap-1">
            <Button
              size="sm"
              variant="ghost"
              className="size-8 rounded-full"
              disabled={!source}
              aria-label={t('rewind')}
              onClick={() => seekBy(-seekSeconds)}
            >
              <SeekStepIcon direction="backward" />
            </Button>
            <Button
              size="sm"
              className="size-10 rounded-full bg-foreground text-background hover:bg-foreground/85"
              disabled={!source}
              aria-label={playing ? t('pause') : t('play')}
              onClick={togglePlayback}
            >
              {playing ? (
                <Pause className="size-4 fill-current" />
              ) : (
                <Play className="ml-0.5 size-4 fill-current" />
              )}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              className="size-8 rounded-full"
              disabled={!source}
              aria-label={t('forward')}
              onClick={() => seekBy(seekSeconds)}
            >
              <SeekStepIcon direction="forward" />
            </Button>
          </div>
          <div className="mt-1 flex min-w-0 items-center gap-3">
            <span className="w-9 shrink-0 text-right text-[11px] tabular-nums text-muted-foreground">
              {formatMusicDuration(currentTimeMS)}
            </span>
            <MusicSegmentedProgress
              segments={MUSIC_TIMELINE_SEGMENTS}
              progress={progress}
              onSeek={source ? seekTo : undefined}
              label={t('seekTrack')}
            />
            <span className="w-9 shrink-0 text-[11px] tabular-nums text-muted-foreground">
              {formatMusicDuration(durationMS)}
            </span>
          </div>
        </div>

        <div className="flex items-center justify-end gap-1">
          {task?.mode !== 'instrumental' ? (
            <Button
              size="sm"
              variant="ghost"
              className="size-9 rounded-full"
              disabled={!task}
              aria-label={t('viewLyrics')}
              onClick={() => task && onShowLyrics(task)}
            >
              <FileText className="size-4" />
            </Button>
          ) : null}
          {source ? (
            <Button asChild size="sm" variant="ghost" className="size-9 rounded-full">
              <a href={toMusicDownloadURL(source)} aria-label={t('download')}>
                <Download className="size-4" />
              </a>
            </Button>
          ) : null}
          <Button
            size="sm"
            variant="ghost"
            className="size-9 rounded-full"
            disabled={!source}
            aria-label={muted ? t('unmute') : t('mute')}
            onClick={() => setMuted(value => !value)}
          >
            {muted ? <VolumeX className="size-4" /> : <Volume2 className="size-4" />}
          </Button>
          <Slider
            value={[muted ? 0 : volume]}
            min={0}
            max={100}
            step={1}
            disabled={!source}
            aria-label={t('volume')}
            className="hidden w-20 sm:flex [&_[role=slider]]:bg-foreground [&>span:first-child>span]:bg-foreground"
            onValueChange={changeVolume}
          />
        </div>
      </div>
    </footer>
  );
}
