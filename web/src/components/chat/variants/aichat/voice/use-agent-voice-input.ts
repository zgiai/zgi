'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { captureError } from '@/lib/observability';
import { BrowserPCMRecorder } from './browser-pcm-recorder';
import {
  applyVoiceTranscription,
  VOICE_RECORDING_LIMIT_SECONDS,
  type AIChatVoiceTranscriber,
} from './pcm-audio';
import { getVoiceInputErrorKey } from './voice-input-errors';

export type AIChatVoiceInputPhase = 'idle' | 'requesting' | 'recording' | 'transcribing';

export interface AIChatVoiceInputController {
  phase: AIChatVoiceInputPhase;
  elapsedSeconds: number;
  disabled: boolean;
  start: () => Promise<void>;
  finish: () => Promise<void>;
  cancel: () => Promise<void>;
}

interface UseAgentVoiceInputOptions {
  transcriber?: AIChatVoiceTranscriber;
  input: string;
  onInputChange: (value: string) => void;
  onError: (error: unknown) => void;
  disabled: boolean;
}

export function useAgentVoiceInput({
  transcriber,
  input,
  onInputChange,
  onError,
  disabled,
}: UseAgentVoiceInputOptions): AIChatVoiceInputController {
  const [phase, setPhaseState] = useState<AIChatVoiceInputPhase>('idle');
  const [elapsedSeconds, setElapsedSeconds] = useState(0);
  const phaseRef = useRef<AIChatVoiceInputPhase>('idle');
  const inputRef = useRef(input);
  const recorderRef = useRef<BrowserPCMRecorder | null>(null);
  const requestRef = useRef<AbortController | null>(null);
  const mountedRef = useRef(true);
  inputRef.current = input;

  const setPhase = useCallback((nextPhase: AIChatVoiceInputPhase) => {
    phaseRef.current = nextPhase;
    if (mountedRef.current) setPhaseState(nextPhase);
  }, []);
  const getPhase = useCallback((): AIChatVoiceInputPhase => phaseRef.current, []);
  const reportVoiceInputError = useCallback(
    (error: unknown) => {
      const key = getVoiceInputErrorKey(error);
      if (key === 'cancelled') return;
      if (key === 'failed') captureError(error, 'agent.voice.input_failed');
      onError(error);
    },
    [onError]
  );

  const finish = useCallback(async () => {
    const recorder = recorderRef.current;
    if (!transcriber || !recorder || getPhase() !== 'recording') return;

    recorderRef.current = null;
    const request = new AbortController();
    requestRef.current = request;
    setPhase('transcribing');
    try {
      const pcm = await recorder.stop();
      if (request.signal.aborted) return;
      await applyVoiceTranscription({
        audio: pcm,
        signal: request.signal,
        transcribe: transcriber,
        getDraft: () => inputRef.current,
        onDraftChange: onInputChange,
      });
    } catch (error) {
      if (!request.signal.aborted) reportVoiceInputError(error);
    } finally {
      if (requestRef.current === request) requestRef.current = null;
      if (getPhase() === 'transcribing') setPhase('idle');
    }
  }, [getPhase, onInputChange, reportVoiceInputError, setPhase, transcriber]);

  const start = useCallback(async () => {
    if (!transcriber || disabled || getPhase() !== 'idle') return;

    const recorder = new BrowserPCMRecorder();
    recorderRef.current = recorder;
    setPhase('requesting');
    try {
      await recorder.start(() => {
        void finish();
      });
      if (recorderRef.current !== recorder || getPhase() !== 'requesting') return;
      setPhase('recording');
    } catch (error) {
      const isCurrentRecorder = recorderRef.current === recorder;
      if (isCurrentRecorder) recorderRef.current = null;
      if (!isCurrentRecorder || getPhase() !== 'requesting') return;
      reportVoiceInputError(error);
      setPhase('idle');
    }
  }, [disabled, finish, getPhase, reportVoiceInputError, setPhase, transcriber]);

  const cancel = useCallback(async () => {
    const recorder = recorderRef.current;
    const request = requestRef.current;
    recorderRef.current = null;
    requestRef.current = null;
    request?.abort();
    setPhase('idle');
    if (!recorder) return;

    try {
      await recorder.cancel();
    } catch (error) {
      captureError(error, 'agent.voice.recorder_cleanup_failed');
      onError(error);
    }
  }, [onError, setPhase]);

  useEffect(() => {
    if (disabled && phaseRef.current !== 'idle') void cancel();
  }, [cancel, disabled]);

  useEffect(() => {
    if (phase !== 'recording') {
      setElapsedSeconds(0);
      return;
    }

    const startedAt = Date.now();
    const timer = window.setInterval(() => {
      setElapsedSeconds(
        Math.min(VOICE_RECORDING_LIMIT_SECONDS, Math.floor((Date.now() - startedAt) / 1_000))
      );
    }, 1_000);
    return () => window.clearInterval(timer);
  }, [phase]);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      requestRef.current?.abort();
      const recorder = recorderRef.current;
      recorderRef.current = null;
      if (recorder) {
        void recorder.cancel().catch(error => {
          captureError(error, 'agent.voice.recorder_cleanup_failed');
        });
      }
    };
  }, []);

  return { phase, elapsedSeconds, disabled: disabled || !transcriber, start, finish, cancel };
}
