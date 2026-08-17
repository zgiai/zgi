const defaultMusicWaveformPeakCount = 160;

interface AudioSampleBuffer {
  numberOfChannels: number;
  length: number;
  duration: number;
  getChannelData(channel: number): Float32Array;
}

export interface MusicWaveformData {
  durationMS: number;
  peaks: number[];
}

export function toMusicWaveformSource(source: string): string {
  const hashIndex = source.indexOf('#');
  const hash = hashIndex >= 0 ? source.slice(hashIndex) : '';
  const withoutHash = hashIndex >= 0 ? source.slice(0, hashIndex) : source;
  const queryIndex = withoutHash.indexOf('?');
  if (queryIndex < 0) return source;

  const pathname = withoutHash.slice(0, queryIndex);
  const query = new URLSearchParams(withoutHash.slice(queryIndex + 1));
  if (query.get('delivery') !== 'direct') return source;
  query.delete('delivery');
  const encodedQuery = query.toString();
  return `${pathname}${encodedQuery ? `?${encodedQuery}` : ''}${hash}`;
}

export function buildMusicWaveformPeaks(
  audio: AudioSampleBuffer,
  requestedPeakCount = defaultMusicWaveformPeakCount
): number[] {
  if (
    audio.numberOfChannels <= 0 ||
    audio.length <= 0 ||
    !Number.isInteger(requestedPeakCount) ||
    requestedPeakCount <= 0
  ) {
    return [];
  }

  const peakCount = Math.min(requestedPeakCount, audio.length);
  const rawPeaks = new Array<number>(peakCount).fill(0);
  for (let peakIndex = 0; peakIndex < peakCount; peakIndex += 1) {
    const start = Math.floor((peakIndex * audio.length) / peakCount);
    const end = Math.max(start + 1, Math.floor(((peakIndex + 1) * audio.length) / peakCount));
    let peak = 0;
    for (let channel = 0; channel < audio.numberOfChannels; channel += 1) {
      const samples = audio.getChannelData(channel);
      for (let sampleIndex = start; sampleIndex < end; sampleIndex += 1) {
        peak = Math.max(peak, Math.abs(samples[sampleIndex] ?? 0));
      }
    }
    rawPeaks[peakIndex] = peak;
  }

  const maxPeak = Math.max(...rawPeaks);
  if (maxPeak <= 0) return rawPeaks;
  return rawPeaks.map(peak => Math.round((peak / maxPeak) * 100));
}

export async function loadMusicWaveform(
  source: string,
  signal: AbortSignal
): Promise<MusicWaveformData> {
  const response = await fetch(toMusicWaveformSource(source), {
    credentials: 'include',
    signal,
  });
  if (!response.ok) {
    throw new Error(`music waveform request failed with status ${response.status}`);
  }

  const audioContext = new AudioContext();
  try {
    const decoded = await audioContext.decodeAudioData(await response.arrayBuffer());
    return {
      durationMS: Math.round(decoded.duration * 1000),
      peaks: buildMusicWaveformPeaks(decoded),
    };
  } finally {
    await audioContext.close().catch(() => undefined);
  }
}
