export const VOICE_PCM_SAMPLE_RATE = 16_000;
export const VOICE_RECORDING_LIMIT_SECONDS = 59;

const MAX_PCM_SAMPLES = VOICE_PCM_SAMPLE_RATE * VOICE_RECORDING_LIMIT_SECONDS;

export type AIChatVoiceTranscriber = (audio: ArrayBuffer, signal: AbortSignal) => Promise<string>;

export class VoiceAudioError extends Error {
  constructor(readonly code: 'EMPTY_AUDIO' | 'INVALID_SAMPLE_RATE') {
    super(code === 'EMPTY_AUDIO' ? 'No audio was recorded.' : 'The audio sample rate is invalid.');
    this.name = 'VoiceAudioError';
  }
}

function resampleMono(samples: Float32Array, inputSampleRate: number): Float32Array {
  if (!Number.isFinite(inputSampleRate) || inputSampleRate <= 0) {
    throw new VoiceAudioError('INVALID_SAMPLE_RATE');
  }
  if (samples.length === 0) {
    throw new VoiceAudioError('EMPTY_AUDIO');
  }

  const inputLimit = Math.min(
    samples.length,
    Math.floor(inputSampleRate * VOICE_RECORDING_LIMIT_SECONDS)
  );
  if (inputSampleRate === VOICE_PCM_SAMPLE_RATE) {
    return samples.slice(0, Math.min(inputLimit, MAX_PCM_SAMPLES));
  }

  const outputLength = Math.min(
    MAX_PCM_SAMPLES,
    Math.max(1, Math.floor((inputLimit * VOICE_PCM_SAMPLE_RATE) / inputSampleRate))
  );
  const output = new Float32Array(outputLength);
  const ratio = inputSampleRate / VOICE_PCM_SAMPLE_RATE;

  for (let index = 0; index < outputLength; index += 1) {
    if (ratio > 1) {
      const windowStart = index * ratio;
      const windowEnd = Math.min((index + 1) * ratio, inputLimit);
      let weightedSum = 0;
      for (
        let sourceSample = Math.floor(windowStart);
        sourceSample < Math.ceil(windowEnd);
        sourceSample += 1
      ) {
        const overlap = Math.min(windowEnd, sourceSample + 1) - Math.max(windowStart, sourceSample);
        weightedSum += samples[sourceSample] * overlap;
      }
      output[index] = weightedSum / (windowEnd - windowStart);
      continue;
    }

    const sourceIndex = index * ratio;
    const leftIndex = Math.min(Math.floor(sourceIndex), inputLimit - 1);
    const rightIndex = Math.min(leftIndex + 1, inputLimit - 1);
    const fraction = sourceIndex - leftIndex;
    output[index] = samples[leftIndex] * (1 - fraction) + samples[rightIndex] * fraction;
  }

  return output;
}

export function encodeMonoPCM16(samples: Float32Array, inputSampleRate: number): ArrayBuffer {
  const normalized = resampleMono(samples, inputSampleRate);
  const buffer = new ArrayBuffer(normalized.length * Int16Array.BYTES_PER_ELEMENT);
  const view = new DataView(buffer);

  normalized.forEach((value, index) => {
    const clipped = Math.max(-1, Math.min(1, value));
    const pcm = clipped < 0 ? Math.round(clipped * 32_768) : Math.round(clipped * 32_767);
    view.setInt16(index * Int16Array.BYTES_PER_ELEMENT, pcm, true);
  });

  return buffer;
}

export function mergeVoiceTranscript(currentDraft: string, transcript: string): string {
  const recognized = transcript.trim();
  if (!recognized) return currentDraft;
  const current = currentDraft.trimEnd();
  return current ? `${current} ${recognized}` : recognized;
}

export function formatVoiceRecordingDuration(elapsedSeconds: number): string {
  const totalSeconds = Math.min(
    VOICE_RECORDING_LIMIT_SECONDS,
    Math.max(0, Math.floor(elapsedSeconds))
  );
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`;
}

export async function applyVoiceTranscription(options: {
  audio: ArrayBuffer;
  signal: AbortSignal;
  transcribe: AIChatVoiceTranscriber;
  getDraft: () => string;
  onDraftChange: (value: string) => void;
}): Promise<void> {
  const transcript = await options.transcribe(options.audio, options.signal);
  if (options.signal.aborted) return;
  options.onDraftChange(mergeVoiceTranscript(options.getDraft(), transcript));
}
