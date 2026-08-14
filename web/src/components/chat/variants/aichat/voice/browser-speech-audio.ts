import type {
  AIChatSpeechAudioSession,
  AIChatSpeechAudioSessionFactory,
  AIChatSpeechAudioSessionHandlers,
} from './speech-playback';

const SPEECH_MIME_TYPE = 'audio/mpeg';

class BrowserSpeechAudioSession implements AIChatSpeechAudioSession {
  private readonly audio = new Audio();
  private readonly mediaSource = new MediaSource();
  private readonly objectURL: string;
  private readonly handlers: AIChatSpeechAudioSessionHandlers;
  private reader: ReadableStreamDefaultReader<Uint8Array> | null = null;
  private closed = false;

  constructor(handlers: AIChatSpeechAudioSessionHandlers) {
    this.handlers = handlers;
    this.objectURL = URL.createObjectURL(this.mediaSource);
    this.audio.preload = 'auto';
    this.audio.src = this.objectURL;
    this.audio.addEventListener('playing', this.handlePlaying);
    this.audio.addEventListener('pause', this.handlePause);
    this.audio.addEventListener('ended', this.handleEnded);
    this.audio.addEventListener('error', this.handleError);
  }

  async attach(stream: ReadableStream<Uint8Array>, signal: AbortSignal): Promise<void> {
    if (this.closed || signal.aborted) throw abortError();
    const sourceBuffer = await this.openSourceBuffer(signal);
    this.reader = stream.getReader();
    const abort = () => {
      void this.reader?.cancel().catch(() => undefined);
    };
    signal.addEventListener('abort', abort, { once: true });

    try {
      while (!this.closed && !signal.aborted) {
        const { done, value } = await this.reader.read();
        if (done) break;
        if (!value || value.byteLength === 0) continue;
        await appendAudioChunk(sourceBuffer, value, signal);
      }
      if (this.closed || signal.aborted) throw abortError();
      await waitForSourceBuffer(sourceBuffer, signal);
      if (this.mediaSource.readyState === 'open') this.mediaSource.endOfStream();
    } finally {
      signal.removeEventListener('abort', abort);
      this.reader.releaseLock();
      this.reader = null;
    }
  }

  play(): Promise<void> {
    if (this.closed) return Promise.reject(new Error('Speech audio session is closed.'));
    return this.audio.play();
  }

  pause(): void {
    if (!this.closed) this.audio.pause();
  }

  close(): void {
    if (this.closed) return;
    this.closed = true;
    void this.reader?.cancel().catch(() => undefined);
    this.audio.pause();
    this.audio.removeEventListener('playing', this.handlePlaying);
    this.audio.removeEventListener('pause', this.handlePause);
    this.audio.removeEventListener('ended', this.handleEnded);
    this.audio.removeEventListener('error', this.handleError);
    this.audio.removeAttribute('src');
    this.audio.load();
    URL.revokeObjectURL(this.objectURL);
  }

  private openSourceBuffer(signal: AbortSignal): Promise<SourceBuffer> {
    if (this.mediaSource.readyState === 'open') {
      return Promise.resolve(this.mediaSource.addSourceBuffer(SPEECH_MIME_TYPE));
    }
    return new Promise((resolve, reject) => {
      const cleanup = () => {
        this.mediaSource.removeEventListener('sourceopen', onOpen);
        signal.removeEventListener('abort', onAbort);
      };
      const onOpen = () => {
        cleanup();
        if (this.closed || signal.aborted) {
          reject(abortError());
          return;
        }
        try {
          resolve(this.mediaSource.addSourceBuffer(SPEECH_MIME_TYPE));
        } catch (error) {
          reject(error);
        }
      };
      const onAbort = () => {
        cleanup();
        reject(abortError());
      };
      this.mediaSource.addEventListener('sourceopen', onOpen, { once: true });
      signal.addEventListener('abort', onAbort, { once: true });
    });
  }

  private handlePlaying = () => this.handlers.onPlaying();
  private handlePause = () => {
    if (!this.closed && !this.audio.ended) this.handlers.onPause();
  };
  private handleEnded = () => this.handlers.onEnded();
  private handleError = () =>
    this.handlers.onError(new Error('The browser could not play speech audio.'));
}

async function appendAudioChunk(
  sourceBuffer: SourceBuffer,
  chunk: Uint8Array,
  signal: AbortSignal
): Promise<void> {
  await waitForSourceBuffer(sourceBuffer, signal);
  if (signal.aborted) throw abortError();
  const audio = chunk.slice().buffer;
  sourceBuffer.appendBuffer(audio);
  await waitForSourceBuffer(sourceBuffer, signal);
}

function waitForSourceBuffer(sourceBuffer: SourceBuffer, signal: AbortSignal): Promise<void> {
  if (!sourceBuffer.updating) return Promise.resolve();
  return new Promise((resolve, reject) => {
    const cleanup = () => {
      sourceBuffer.removeEventListener('updateend', onUpdateEnd);
      sourceBuffer.removeEventListener('error', onError);
      signal.removeEventListener('abort', onAbort);
    };
    const onUpdateEnd = () => {
      cleanup();
      resolve();
    };
    const onError = () => {
      cleanup();
      reject(new Error('The browser rejected streamed speech audio.'));
    };
    const onAbort = () => {
      cleanup();
      reject(abortError());
    };
    sourceBuffer.addEventListener('updateend', onUpdateEnd, { once: true });
    sourceBuffer.addEventListener('error', onError, { once: true });
    signal.addEventListener('abort', onAbort, { once: true });
  });
}

function abortError(): DOMException {
  return new DOMException('Speech playback was cancelled.', 'AbortError');
}

export const createBrowserSpeechAudioSession: AIChatSpeechAudioSessionFactory = handlers => {
  if (typeof Audio === 'undefined' || typeof MediaSource === 'undefined') {
    throw new Error('Streaming speech playback is not supported by this browser.');
  }
  if (!MediaSource.isTypeSupported(SPEECH_MIME_TYPE)) {
    throw new Error('MP3 streaming playback is not supported by this browser.');
  }
  return new BrowserSpeechAudioSession(handlers);
};
