'use client';

import * as React from 'react';
import { ChevronRight, Lightbulb, Loader2 } from 'lucide-react';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import { reasoningDurationSeconds } from '@/components/chat/variants/aichat/reasoning';

interface AIChatReasoningBlockProps {
  content: string;
  durationMs?: number;
  /** The model is still producing reasoning for this message. */
  isThinking?: boolean;
}

/**
 * @component AIChatReasoningBlock
 * @category Feature
 * @status Stable
 * @description Collapsible "thinking" block showing the model's reasoning above the answer (ZGI-13).
 * @usage Render inside an assistant message when it carries reasoning content
 * @example
 * <AIChatReasoningBlock content={reasoning} durationMs={1200} />
 */
export function AIChatReasoningBlock({
  content,
  durationMs,
  isThinking = false,
}: AIChatReasoningBlockProps) {
  const t = useT('webapp');
  const [open, setOpen] = React.useState(false);
  const contentId = React.useId();
  const seconds = reasoningDurationSeconds(durationMs);
  const label = isThinking
    ? t('consoleChat.reasoning.thinking')
    : seconds !== undefined
      ? t('consoleChat.reasoning.thought', { seconds })
      : t('consoleChat.reasoning.thoughtBriefly');

  if (!content) {
    return null;
  }

  return (
    <div className="mb-3 min-w-0 max-w-full">
      <button
        type="button"
        className="inline-flex max-w-full items-center gap-1.5 rounded-md px-1.5 py-1 text-xs text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        aria-expanded={open}
        aria-controls={contentId}
        title={open ? t('consoleChat.reasoning.collapse') : t('consoleChat.reasoning.expand')}
        onClick={() => setOpen(value => !value)}
      >
        {isThinking ? (
          <Loader2 className="size-3.5 shrink-0 animate-spin" />
        ) : (
          <Lightbulb className="size-3.5 shrink-0" />
        )}
        <span className="truncate">{label}</span>
        <ChevronRight
          className={cn('size-3.5 shrink-0 transition-transform', open && 'rotate-90')}
        />
      </button>
      {open ? (
        <div
          id={contentId}
          className="mt-1 max-h-96 overflow-y-auto whitespace-pre-wrap break-words border-l-2 border-muted-foreground/20 pl-3 text-xs leading-6 text-muted-foreground"
        >
          {content}
        </div>
      ) : null}
    </div>
  );
}
