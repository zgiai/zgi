export type AIChatSpeechPlaybackPhase = 'idle' | 'loading' | 'playing' | 'paused';

export interface AIChatSpeechPlaybackState {
  messageId: string | null;
  phase: AIChatSpeechPlaybackPhase;
}

export type AIChatSpeechSynthesizer = (
  text: string,
  signal: AbortSignal
) => Promise<ReadableStream<Uint8Array>>;

export interface AIChatSpeechAudioSessionHandlers {
  onPlaying: () => void;
  onPause: () => void;
  onEnded: () => void;
  onError: (error: Error) => void;
}

export interface AIChatSpeechAudioSession {
  attach: (stream: ReadableStream<Uint8Array>, signal: AbortSignal) => Promise<void>;
  play: () => void | Promise<void>;
  pause: () => void;
  close: () => void;
}

export type AIChatSpeechAudioSessionFactory = (
  handlers: AIChatSpeechAudioSessionHandlers
) => AIChatSpeechAudioSession;

interface AgentSpeechPlaybackControllerOptions {
  synthesize: AIChatSpeechSynthesizer;
  createSession: AIChatSpeechAudioSessionFactory;
  onChange?: (state: AIChatSpeechPlaybackState) => void;
  onError?: (error: Error) => void;
}

interface SpeechMessageCandidate {
  id: string;
  answer: string;
  status: string;
}

const IDLE_SPEECH_STATE: AIChatSpeechPlaybackState = { messageId: null, phase: 'idle' };

function isCompletedSpeechMessage(message: SpeechMessageCandidate): boolean {
  return message.status === 'completed' && message.answer.trim().length > 0;
}

export function markCompletedSpeechMessagesSeen(
  messages: SpeechMessageCandidate[],
  seen: Set<string>
): void {
  for (const message of messages) {
    if (isCompletedSpeechMessage(message)) seen.add(message.id);
  }
}

export function takeLatestUnseenCompletedSpeechMessage(
  messages: SpeechMessageCandidate[],
  seen: Set<string>
): SpeechMessageCandidate | null {
  let latest: SpeechMessageCandidate | null = null;
  for (const message of messages) {
    if (!isCompletedSpeechMessage(message) || seen.has(message.id)) continue;
    seen.add(message.id);
    latest = message;
  }
  return latest;
}

export class AgentSpeechPlaybackController {
  private readonly synthesize: AIChatSpeechSynthesizer;
  private readonly createSession: AIChatSpeechAudioSessionFactory;
  private readonly onChange?: (state: AIChatSpeechPlaybackState) => void;
  private readonly onError?: (error: Error) => void;
  private state: AIChatSpeechPlaybackState = IDLE_SPEECH_STATE;
  private session: AIChatSpeechAudioSession | null = null;
  private request: AbortController | null = null;
  private generation = 0;

  constructor(options: AgentSpeechPlaybackControllerOptions) {
    this.synthesize = options.synthesize;
    this.createSession = options.createSession;
    this.onChange = options.onChange;
    this.onError = options.onError;
  }

  snapshot(): AIChatSpeechPlaybackState {
    return this.state;
  }

  async toggle(messageId: string, text: string): Promise<void> {
    if (this.state.messageId !== messageId) {
      await this.play(messageId, text);
      return;
    }
    if (this.state.phase === 'playing') {
      this.session?.pause();
      return;
    }
    if (this.state.phase === 'paused') {
      await this.resume();
      return;
    }
    this.stop();
  }

  async play(messageId: string, text: string): Promise<void> {
    const normalizedMessageId = messageId.trim();
    const normalizedText = text.trim();
    if (!normalizedMessageId || !normalizedText) return;

    this.stop();
    const generation = ++this.generation;
    const request = new AbortController();
    this.request = request;
    let session: AIChatSpeechAudioSession;
    try {
      session = this.createSession({
        onPlaying: () => this.handleSessionState(generation, 'playing'),
        onPause: () => this.handleSessionState(generation, 'paused'),
        onEnded: () => this.finish(generation),
        onError: error => this.fail(generation, error),
      });
    } catch (error) {
      request.abort();
      this.request = null;
      this.setState(IDLE_SPEECH_STATE);
      this.onError?.(asError(error));
      return;
    }
    this.session = session;
    this.setState({ messageId: normalizedMessageId, phase: 'loading' });

    try {
      const playResult = session.play();
      void Promise.resolve(playResult).catch(error => this.fail(generation, asError(error)));
      const stream = await this.synthesize(normalizedText, request.signal);
      if (!this.isCurrent(generation)) {
        await stream.cancel();
        return;
      }
      await session.attach(stream, request.signal);
    } catch (error) {
      if (!request.signal.aborted) this.fail(generation, asError(error));
    }
  }

  stop(): void {
    this.generation += 1;
    this.request?.abort();
    this.request = null;
    this.session?.close();
    this.session = null;
    this.setState(IDLE_SPEECH_STATE);
  }

  private async resume(): Promise<void> {
    if (!this.session) return;
    try {
      await this.session.play();
    } catch (error) {
      this.fail(this.generation, asError(error));
    }
  }

  private handleSessionState(generation: number, phase: 'playing' | 'paused'): void {
    if (!this.isCurrent(generation) || !this.state.messageId) return;
    this.setState({ messageId: this.state.messageId, phase });
  }

  private finish(generation: number): void {
    if (!this.isCurrent(generation)) return;
    this.stop();
  }

  private fail(generation: number, error: Error): void {
    if (!this.isCurrent(generation)) return;
    this.stop();
    this.onError?.(error);
  }

  private isCurrent(generation: number): boolean {
    return generation === this.generation && this.session !== null;
  }

  private setState(state: AIChatSpeechPlaybackState): void {
    if (this.state.messageId === state.messageId && this.state.phase === state.phase) return;
    this.state = state;
    this.onChange?.(state);
  }
}

function asError(error: unknown): Error {
  return error instanceof Error ? error : new Error('Speech playback failed.');
}
