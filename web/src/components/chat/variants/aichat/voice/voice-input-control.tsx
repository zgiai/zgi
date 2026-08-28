'use client';

import { LoaderCircle, Mic, Square, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useT } from '@/i18n/translations';
import { formatVoiceRecordingDuration } from './pcm-audio';
import type { AIChatVoiceInputController } from './use-agent-voice-input';

export function AIChatVoiceInputControl({
  phase,
  elapsedSeconds,
  disabled,
  start,
  finish,
  cancel,
}: AIChatVoiceInputController) {
  const t = useT('webapp');

  if (phase === 'idle') {
    const label = t('consoleChat.voice.start');
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            type="button"
            isIcon
            variant="ghost"
            className="size-8 rounded-full"
            disabled={disabled}
            onClick={() => void start()}
            aria-label={label}
          >
            <Mic className="size-4" />
          </Button>
        </TooltipTrigger>
        <TooltipContent side="top">{label}</TooltipContent>
      </Tooltip>
    );
  }

  const statusLabel = t(
    phase === 'requesting'
      ? 'consoleChat.voice.requesting'
      : phase === 'recording'
        ? 'consoleChat.voice.recording'
        : 'consoleChat.voice.transcribing'
  );
  const recordingDuration = formatVoiceRecordingDuration(elapsedSeconds);

  return (
    <div
      className="flex items-center gap-1"
      role="group"
      aria-label={t('consoleChat.voice.controls')}
    >
      {phase === 'recording' ? (
        <Button
          type="button"
          variant="outline"
          className="h-8 gap-1.5 rounded-full border-destructive/40 px-2.5 text-destructive hover:bg-destructive/10 hover:text-destructive"
          onClick={() => void finish()}
          aria-label={t('consoleChat.voice.finish')}
        >
          <Square className="size-3 fill-current" />
          <span className="hidden text-xs sm:inline">{statusLabel}</span>
          <span className="text-xs tabular-nums">{recordingDuration}</span>
        </Button>
      ) : (
        <Button
          type="button"
          variant="ghost"
          className="h-8 gap-1.5 rounded-full px-2.5 text-muted-foreground"
          onClick={() => void cancel()}
          aria-label={t(
            phase === 'transcribing'
              ? 'consoleChat.voice.cancelTranscription'
              : 'consoleChat.voice.cancelRecording'
          )}
        >
          <LoaderCircle className="size-4 animate-spin" />
          <span className="hidden text-xs sm:inline">{statusLabel}</span>
        </Button>
      )}
      {phase === 'recording' ? (
        <Button
          type="button"
          isIcon
          variant="ghost"
          className="size-8 rounded-full text-muted-foreground"
          onClick={() => void cancel()}
          aria-label={t('consoleChat.voice.cancelRecording')}
        >
          <X className="size-4" />
        </Button>
      ) : null}
      <span className="sr-only" aria-live="polite">
        {statusLabel}
      </span>
    </div>
  );
}
