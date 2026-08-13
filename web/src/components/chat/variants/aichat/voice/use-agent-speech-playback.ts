'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { toast } from 'sonner';
import { captureError } from '@/lib/observability';
import type { AIChatMessage } from '@/services/types/aichat';
import { isPersistedAIChatConversationPromotion } from '@/components/chat/utils/aichat-message';
import { createBrowserSpeechAudioSession } from './browser-speech-audio';
import {
  AgentSpeechPlaybackController,
  markCompletedSpeechMessagesSeen,
  takeLatestUnseenCompletedSpeechMessage,
  type AIChatSpeechPlaybackState,
  type AIChatSpeechSynthesizer,
} from './speech-playback';
import { toAgentSpeechText } from './speech-text';
import { getSpeechPlaybackErrorKey, type SpeechPlaybackErrorKey } from './speech-playback-errors';

interface UseAgentSpeechPlaybackOptions {
  synthesizer?: AIChatSpeechSynthesizer;
  messages: AIChatMessage[];
  conversationId: string | null;
  isLoadingMessages: boolean;
  playbackErrorMessages: Record<SpeechPlaybackErrorKey, string>;
}

export interface AIChatSpeechPlaybackController {
  state: AIChatSpeechPlaybackState;
  autoPlay: boolean;
  setAutoPlay: (enabled: boolean) => void;
  toggle: (messageId: string, answer: string) => void;
  stop: () => void;
}

const INITIAL_STATE: AIChatSpeechPlaybackState = { messageId: null, phase: 'idle' };

export function useAgentSpeechPlayback({
  synthesizer,
  messages,
  conversationId,
  isLoadingMessages,
  playbackErrorMessages,
}: UseAgentSpeechPlaybackOptions): AIChatSpeechPlaybackController | undefined {
  const [state, setState] = useState<AIChatSpeechPlaybackState>(INITIAL_STATE);
  const [autoPlay, setAutoPlayState] = useState(false);
  const baselineRef = useRef<{
    conversationId: string | null;
    initialized: boolean;
    seen: Set<string>;
  }>({ conversationId, initialized: false, seen: new Set() });
  const playbackErrorMessagesRef = useRef(playbackErrorMessages);
  playbackErrorMessagesRef.current = playbackErrorMessages;

  const controller = useMemo(() => {
    if (!synthesizer) return undefined;
    return new AgentSpeechPlaybackController({
      synthesize: synthesizer,
      createSession: createBrowserSpeechAudioSession,
      onChange: setState,
      onError: error => {
        captureError(error, 'agent.voice.playback_failed');
        toast.error(playbackErrorMessagesRef.current[getSpeechPlaybackErrorKey(error)]);
      },
    });
  }, [synthesizer]);

  useEffect(() => () => controller?.stop(), [controller]);

  useEffect(() => {
    const baseline = baselineRef.current;
    if (baseline.conversationId !== conversationId) {
      if (isPersistedAIChatConversationPromotion(baseline.conversationId, conversationId)) {
        baseline.conversationId = conversationId;
        return;
      }
      controller?.stop();
      baselineRef.current = { conversationId, initialized: false, seen: new Set() };
    }
  }, [controller, conversationId]);

  useEffect(() => {
    if (!controller || isLoadingMessages) return;
    const baseline = baselineRef.current;
    if (baseline.conversationId && messages.length === 0) return;
    if (!baseline.initialized) {
      markCompletedSpeechMessagesSeen(messages, baseline.seen);
      baseline.initialized = true;
      return;
    }

    const latest = takeLatestUnseenCompletedSpeechMessage(messages, baseline.seen);
    if (!autoPlay || !latest) return;
    const text = toAgentSpeechText(latest.answer);
    if (text) void controller.play(latest.id, text);
  }, [autoPlay, controller, isLoadingMessages, messages]);

  const setAutoPlay = useCallback(
    (enabled: boolean) => {
      setAutoPlayState(enabled);
      if (!enabled) controller?.stop();
    },
    [controller]
  );

  const toggle = useCallback(
    (messageId: string, answer: string) => {
      const text = toAgentSpeechText(answer);
      if (text) void controller?.toggle(messageId, text);
    },
    [controller]
  );

  const stop = useCallback(() => controller?.stop(), [controller]);

  if (!controller) return undefined;
  return { state, autoPlay, setAutoPlay, toggle, stop };
}
