'use client';

import { forwardRef, useCallback, useEffect, useImperativeHandle, useRef } from 'react';
import { toast } from 'sonner';
import { useT } from '@/i18n/translations';
import { transcribeWorkspaceVoice } from '@/services/voice-transcription.service';
import { AIChatVoiceInputControl } from './voice-input-control';
import { getVoiceInputErrorKey } from './voice-input-errors';
import { useAgentVoiceInput } from './use-agent-voice-input';

interface WorkspaceVoiceInputControlProps {
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  onActiveChange?: (active: boolean) => void;
}

export interface WorkspaceVoiceInputControlHandle {
  finish: () => Promise<void>;
}

export const WorkspaceVoiceInputControl = forwardRef<
  WorkspaceVoiceInputControlHandle,
  WorkspaceVoiceInputControlProps
>(function WorkspaceVoiceInputControl(
  { value, onChange, disabled = false, onActiveChange },
  ref
) {
  const t = useT('webapp');
  const ownsActivityRef = useRef(false);
  const handleError = useCallback(
    (error: unknown) => {
      const key = getVoiceInputErrorKey(error);
      if (key === 'cancelled') return;
      const message = {
        permissionDenied: t('consoleChat.voice.errors.permissionDenied'),
        unsupported: t('consoleChat.voice.errors.unsupported'),
        noSpeech: t('consoleChat.voice.errors.noSpeech'),
        timeout: t('consoleChat.voice.errors.timeout'),
        balance: t('consoleChat.voice.errors.balance'),
        quota: t('consoleChat.voice.errors.quota'),
        unavailable: t('consoleChat.voice.errors.unavailable'),
        failed: t('consoleChat.voice.errors.failed'),
      }[key];
      toast.error(message);
    },
    [t]
  );
  const voiceInput = useAgentVoiceInput({
    transcriber: transcribeWorkspaceVoice,
    input: value,
    onInputChange: onChange,
    onError: handleError,
    disabled,
  });
  const start = useCallback(async () => {
    ownsActivityRef.current = true;
    onActiveChange?.(true);
    await voiceInput.start();
  }, [onActiveChange, voiceInput.start]);

  useImperativeHandle(ref, () => ({ finish: voiceInput.finish }), [voiceInput.finish]);

  useEffect(() => {
    if (voiceInput.phase !== 'idle' || !ownsActivityRef.current) return;
    ownsActivityRef.current = false;
    onActiveChange?.(false);
  }, [onActiveChange, voiceInput.phase]);

  useEffect(
    () => () => {
      if (ownsActivityRef.current) onActiveChange?.(false);
    },
    [onActiveChange]
  );

  return <AIChatVoiceInputControl {...voiceInput} start={start} />;
});
