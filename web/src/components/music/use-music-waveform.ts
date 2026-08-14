'use client';

import * as React from 'react';
import { loadMusicWaveform } from './music-waveform-data';

interface MusicWaveformState {
  durationMS: number;
  peaks: number[];
}

export function useMusicWaveform(
  source: string | null,
  storedPeaks?: number[],
  storedDurationMS?: number,
  enabled = false
): MusicWaveformState {
  const [waveform, setWaveform] = React.useState<MusicWaveformState>({
    durationMS: storedDurationMS ?? 0,
    peaks: storedPeaks ?? [],
  });

  React.useEffect(() => {
    const initial = {
      durationMS: storedDurationMS ?? 0,
      peaks: storedPeaks ?? [],
    };
    setWaveform(initial);
    if (!enabled || !source || initial.peaks.length > 0) return;

    const controller = new AbortController();
    void loadMusicWaveform(source, controller.signal)
      .then(result => {
        if (!controller.signal.aborted) setWaveform(result);
      })
      .catch(() => undefined);
    return () => controller.abort();
  }, [enabled, source, storedDurationMS, storedPeaks]);

  return waveform;
}
