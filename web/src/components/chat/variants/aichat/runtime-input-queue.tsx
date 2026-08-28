'use client';

import { Clock3, LoaderCircle, MessageSquarePlus, Send, Undo2 } from 'lucide-react';
import type { AIChatQueuedTurn } from '@/components/chat/utils/queued-turns';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import type { AIChatRuntimeInput } from '@/services/types/aichat';

interface AIChatRuntimeInputQueueProps {
  items: AIChatRuntimeInput[];
  queuedTurns?: AIChatQueuedTurn[];
  guidingTurnIDs?: ReadonlySet<string>;
  onGuideTurn?: (turn: AIChatQueuedTurn) => void;
  onEditTurn?: (turn: AIChatQueuedTurn) => void;
}

export function AIChatRuntimeInputQueue({
  items,
  queuedTurns = [],
  guidingTurnIDs,
  onGuideTurn,
  onEditTurn,
}: AIChatRuntimeInputQueueProps) {
  const t = useT('webapp');
  const itemCount = items.length + queuedTurns.length;
  if (itemCount === 0) return null;

  return (
    <section
      aria-label={t('consoleChat.runtimeInput.queueTitle')}
      aria-live="polite"
      aria-relevant="additions text"
      className="overflow-hidden rounded-xl border bg-background/95 shadow-sm backdrop-blur"
    >
      <div className="flex items-center gap-2 border-b px-3 py-2 text-xs font-medium text-foreground">
        <MessageSquarePlus className="size-3.5 text-primary" />
        <span>{t('consoleChat.runtimeInput.queueTitle')}</span>
        {itemCount > 0 ? (
          <span className="rounded-full bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">
            {itemCount}/2
          </span>
        ) : null}
      </div>
      <div className="max-h-28 divide-y overflow-y-auto">
        {queuedTurns.map(turn => {
          const isGuiding = guidingTurnIDs?.has(turn.id) ?? false;
          return (
            <div
              key={turn.id}
              className="flex min-w-0 flex-wrap items-start gap-2.5 px-3 py-2 sm:flex-nowrap"
            >
              <Clock3 className="mt-0.5 size-3.5 shrink-0 text-muted-foreground" />
              <div className="min-w-0 flex-1">
                <p className="line-clamp-2 whitespace-pre-wrap break-words text-xs text-foreground">
                  {turn.content}
                </p>
                <p className="mt-0.5 text-[11px] text-muted-foreground">
                  {t('consoleChat.runtimeInput.waitingNextTurn')}
                </p>
              </div>
              <div className="ml-6 flex w-full shrink-0 items-center justify-end gap-1 sm:ml-0 sm:w-auto">
                {onGuideTurn ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="h-7 px-2 text-[11px]"
                    disabled={isGuiding}
                    onClick={() => onGuideTurn(turn)}
                    title={t('consoleChat.runtimeInput.guideCurrent')}
                    aria-label={t('consoleChat.runtimeInput.guideCurrent')}
                  >
                    {isGuiding ? (
                      <LoaderCircle className="mr-1 size-3 animate-spin motion-reduce:animate-none" />
                    ) : (
                      <Send className="mr-1 size-3" />
                    )}
                    {t('consoleChat.runtimeInput.guideCurrent')}
                  </Button>
                ) : null}
                {onEditTurn ? (
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="h-7 px-2 text-[11px]"
                    disabled={isGuiding}
                    onClick={() => onEditTurn(turn)}
                    title={t('consoleChat.runtimeInput.withdrawEdit')}
                    aria-label={t('consoleChat.runtimeInput.withdrawEdit')}
                  >
                    <Undo2 className="mr-1 size-3" />
                    {t('consoleChat.runtimeInput.withdrawEdit')}
                  </Button>
                ) : null}
              </div>
            </div>
          );
        })}
        {items.map(item => {
          const queuedForFollowUp = item.status === 'queued_follow_up';
          const statusLabel = queuedForFollowUp
            ? t('consoleChat.runtimeInput.queuedFollowUp')
            : item.optimistic
              ? t('consoleChat.runtimeInput.submitting')
              : t('consoleChat.runtimeInput.waitingCurrentRun');
          const StatusIcon = queuedForFollowUp ? Clock3 : LoaderCircle;

          return (
            <div
              key={item.id || item.client_input_id}
              className="flex min-w-0 items-start gap-2.5 px-3 py-2"
            >
              <StatusIcon
                className={cn(
                  'mt-0.5 size-3.5 shrink-0 text-muted-foreground',
                  !queuedForFollowUp && 'animate-spin motion-reduce:animate-none'
                )}
              />
              <div className="min-w-0 flex-1">
                <p className="line-clamp-2 whitespace-pre-wrap break-words text-xs text-foreground">
                  {item.content}
                </p>
                <p className="mt-0.5 text-[11px] text-muted-foreground">{statusLabel}</p>
              </div>
            </div>
          );
        })}
      </div>
    </section>
  );
}
