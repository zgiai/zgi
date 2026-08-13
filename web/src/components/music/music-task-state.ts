import type { MusicTaskStatus } from '@/services/types/music';

export const MUSIC_TIMELINE_SEGMENTS = 4;

export function shouldPollMusicTask(status: MusicTaskStatus | undefined): boolean {
  return (
    status === 'queued' ||
    status === 'generating_lyrics' ||
    status === 'generating' ||
    status === 'compensation_pending'
  );
}

export function formatMusicDuration(durationMS: number | undefined): string {
  const totalSeconds = Math.max(0, Math.round((durationMS ?? 0) / 1000));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${seconds.toString().padStart(2, '0')}`;
}

export function resolveMusicDurationSeconds(
  mediaDurationSeconds: number,
  fallbackDurationMS: number
): number {
  if (Number.isFinite(mediaDurationSeconds) && mediaDurationSeconds > 0) {
    return mediaDurationSeconds;
  }
  const fallbackDurationSeconds = fallbackDurationMS / 1000;
  return Number.isFinite(fallbackDurationSeconds) && fallbackDurationSeconds > 0
    ? fallbackDurationSeconds
    : 0;
}

export function clampMusicSeekTime(
  targetSeconds: number,
  mediaDurationSeconds: number,
  fallbackDurationMS: number
): number {
  const durationSeconds = resolveMusicDurationSeconds(mediaDurationSeconds, fallbackDurationMS);
  const normalizedTarget = Number.isFinite(targetSeconds) ? Math.max(0, targetSeconds) : 0;
  return durationSeconds > 0 ? Math.min(durationSeconds, normalizedTarget) : normalizedTarget;
}

export function shouldPrepareMusicTask(
  status: MusicTaskStatus | undefined,
  rawURL: string | undefined
): boolean {
  return status === 'succeeded' && Boolean(rawURL?.trim());
}

export function resolveMusicSourcePlaybackTransition(
  previousTaskId: string | null,
  nextTaskId: string | null,
  wasPlaying: boolean,
  progress: number
): { progress: number; shouldResume: boolean } {
  const sameTask = previousTaskId !== null && previousTaskId === nextTaskId;
  return {
    progress: sameTask ? progress : 0,
    shouldResume: sameTask && wasPlaying,
  };
}

export function toMusicAssetURL(rawURL: string, apiBaseURL: string): string | null {
  const value = rawURL.trim();
  const apiBase = apiBaseURL.trim().replace(/\/+$/, '');
  if (!value) return null;
  if (value.startsWith('/') && !value.startsWith('//')) {
    return apiBase ? `${apiBase}${value}` : value;
  }
  if (!/^https?:\/\//i.test(value)) return null;

  try {
    const parsed = new URL(value);
    if (apiBase && parsed.pathname.startsWith('/console/api/files/')) {
      return `${apiBase}${parsed.pathname}${parsed.search}${parsed.hash}`;
    }
    return parsed.toString();
  } catch {
    return null;
  }
}

export function toMusicDownloadURL(url: string): string {
  if (/[?&]download=1(?:&|$)/.test(url)) return url;
  const fragmentIndex = url.indexOf('#');
  const base = fragmentIndex >= 0 ? url.slice(0, fragmentIndex) : url;
  const fragment = fragmentIndex >= 0 ? url.slice(fragmentIndex) : '';
  return `${base}${base.includes('?') ? '&' : '?'}download=1${fragment}`;
}
