import { encodeMonoPCM16, VOICE_PCM_SAMPLE_RATE, VOICE_RECORDING_LIMIT_SECONDS } from './pcm-audio';
import { withBasePath } from '@/lib/config';

const RECORDER_WORKLET_PATH = withBasePath('/audio/pcm-recorder-worklet.js');
const RECORDER_WORKLET_NAME = 'zgi-pcm-recorder';

export type VoiceRecordingErrorCode =
  | 'MICROPHONE_UNSUPPORTED'
  | 'RECORDING_CANCELLED'
  | 'INVALID_RECORDER_STATE'
  | 'INVALID_AUDIO_DATA';

export class VoiceRecordingError extends Error {
  constructor(readonly code: VoiceRecordingErrorCode) {
    super(code);
    this.name = 'VoiceRecordingError';
  }
}

function stopStream(stream: MediaStream | null): void {
  stream?.getTracks().forEach(track => track.stop());
}

function combineChunks(chunks: Float32Array[]): Float32Array {
  const length = chunks.reduce((total, chunk) => total + chunk.length, 0);
  const combined = new Float32Array(length);
  let offset = 0;
  chunks.forEach(chunk => {
    combined.set(chunk, offset);
    offset += chunk.length;
  });
  return combined;
}

export class BrowserPCMRecorder {
  private stream: MediaStream | null = null;
  private context: AudioContext | null = null;
  private source: MediaStreamAudioSourceNode | null = null;
  private worklet: AudioWorkletNode | null = null;
  private silentGain: GainNode | null = null;
  private limitTimer: number | null = null;
  private chunks: Float32Array[] = [];
  private capturedSampleCount = 0;
  private cancelled = false;
  private recording = false;
  private recordingError: VoiceRecordingError | null = null;

  async start(onLimitReached: () => void): Promise<void> {
    if (this.recording || this.context || this.stream) {
      throw new VoiceRecordingError('INVALID_RECORDER_STATE');
    }
    const AudioContextConstructor = window.AudioContext;
    if (
      !navigator.mediaDevices?.getUserMedia ||
      !AudioContextConstructor ||
      typeof AudioWorkletNode === 'undefined'
    ) {
      throw new VoiceRecordingError('MICROPHONE_UNSUPPORTED');
    }

    this.cancelled = false;
    this.recordingError = null;
    this.chunks = [];
    this.capturedSampleCount = 0;
    const stream = await navigator.mediaDevices.getUserMedia({
      audio: {
        channelCount: 1,
        echoCancellation: true,
        noiseSuppression: true,
      },
    });
    if (this.cancelled) {
      stopStream(stream);
      throw new VoiceRecordingError('RECORDING_CANCELLED');
    }
    this.stream = stream;

    try {
      const context = new AudioContextConstructor({ sampleRate: VOICE_PCM_SAMPLE_RATE });
      this.context = context;
      if (!context.audioWorklet || typeof context.audioWorklet.addModule !== 'function') {
        throw new VoiceRecordingError('MICROPHONE_UNSUPPORTED');
      }
      await context.audioWorklet.addModule(RECORDER_WORKLET_PATH);
      if (this.cancelled) {
        await this.releaseResources();
        throw new VoiceRecordingError('RECORDING_CANCELLED');
      }

      this.source = context.createMediaStreamSource(stream);
      this.worklet = new AudioWorkletNode(context, RECORDER_WORKLET_NAME);
      this.silentGain = context.createGain();
      this.silentGain.gain.value = 0;
      const maxCapturedSamples = Math.ceil(context.sampleRate * VOICE_RECORDING_LIMIT_SECONDS);
      this.worklet.port.onmessage = event => {
        if (!(event.data instanceof Float32Array)) {
          this.recordingError = new VoiceRecordingError('INVALID_AUDIO_DATA');
          return;
        }
        const remainingSamples = maxCapturedSamples - this.capturedSampleCount;
        if (remainingSamples <= 0) return;
        const chunk =
          event.data.length <= remainingSamples
            ? event.data
            : event.data.slice(0, remainingSamples);
        this.chunks.push(chunk);
        this.capturedSampleCount += chunk.length;
      };
      this.source.connect(this.worklet);
      this.worklet.connect(this.silentGain);
      this.silentGain.connect(context.destination);
      this.recording = true;
      this.limitTimer = window.setTimeout(onLimitReached, VOICE_RECORDING_LIMIT_SECONDS * 1_000);
    } catch (error) {
      await this.releaseResources();
      throw error;
    }
  }

  async stop(): Promise<ArrayBuffer> {
    if (!this.recording || !this.context) {
      throw new VoiceRecordingError('INVALID_RECORDER_STATE');
    }
    this.recording = false;
    const sampleRate = this.context.sampleRate;
    const chunks = this.chunks;
    this.chunks = [];
    this.capturedSampleCount = 0;
    const recordingError = this.recordingError;
    await this.releaseResources();
    if (recordingError) throw recordingError;
    return encodeMonoPCM16(combineChunks(chunks), sampleRate);
  }

  async cancel(): Promise<void> {
    this.cancelled = true;
    this.recording = false;
    this.chunks = [];
    this.capturedSampleCount = 0;
    await this.releaseResources();
  }

  private async releaseResources(): Promise<void> {
    if (this.limitTimer !== null) {
      window.clearTimeout(this.limitTimer);
      this.limitTimer = null;
    }
    if (this.worklet) {
      this.worklet.port.onmessage = null;
      this.worklet.port.close();
    }
    this.source?.disconnect();
    this.worklet?.disconnect();
    this.silentGain?.disconnect();
    stopStream(this.stream);

    const context = this.context;
    this.source = null;
    this.worklet = null;
    this.silentGain = null;
    this.stream = null;
    this.context = null;
    if (context && context.state !== 'closed') {
      await context.close();
    }
  }
}
