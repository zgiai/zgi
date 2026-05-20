'use client';

import { Badge } from '@/components/ui/badge';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { Skeleton } from '@/components/ui/skeleton';
import { useT } from '@/i18n/translations';
import type { WorkflowChatMessageItem } from '@/services/types/workflow';
import { formatDate } from '@/utils/format';
import { cn } from '@/lib/utils';

function MessageListItem({
  message,
  onInspect,
  activeRunId,
}: {
  message: WorkflowChatMessageItem;
  onInspect: (message: WorkflowChatMessageItem) => void;
  activeRunId?: string | null;
}) {
  const t = useT('webapp');
  const isActive = Boolean(message.workflow_run_id && message.workflow_run_id === activeRunId);

  return (
    <button
      type="button"
      className={cn(
        'w-full rounded-lg border bg-background p-3 text-left transition-colors hover:border-primary/40 hover:bg-primary/5',
        isActive && 'border-primary/50 bg-primary/5'
      )}
      onClick={() => onInspect(message)}
      disabled={!message.workflow_run_id}
    >
      <div className="flex items-center justify-between gap-2 text-xs">
        <span className="text-muted-foreground">{formatDate(message.created_at)}</span>
        {isActive ? (
          <Badge variant="secondary" className="h-5 px-2 text-[11px]">
            {t('appLogs.currentTurn')}
          </Badge>
        ) : null}
      </div>
      <div className="mt-2 line-clamp-2 text-sm leading-relaxed text-foreground">
        {message.query || t('appLogs.noQuery')}
      </div>
      {!isActive ? (
        <div className="mt-2 text-xs font-medium text-primary">{t('appLogs.switchToTurn')}</div>
      ) : null}
    </button>
  );
}

interface ConversationContextProps {
  messages: WorkflowChatMessageItem[];
  activeRunId: string | null;
  isLoading: boolean;
  error?: string | null;
  onInspect: (message: WorkflowChatMessageItem) => void;
}

export function ConversationContext({
  messages,
  activeRunId,
  isLoading,
  error,
  onInspect,
}: ConversationContextProps) {
  const t = useT('webapp');
  const activeMessage = activeRunId
    ? (messages.find(
        message => message.workflow_run_id && message.workflow_run_id === activeRunId
      ) ?? null)
    : (messages[0] ?? null);

  if (error) {
    return (
      <div className="rounded-lg border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive">
        {error}
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_280px]">
        <Skeleton className="h-48 rounded-lg" />
        <Skeleton className="h-48 rounded-lg" />
      </div>
    );
  }

  if (!activeMessage) {
    if (activeRunId && messages.length > 0) {
      return (
        <div className="rounded-lg border border-dashed px-4 py-8 text-center">
          <div className="text-sm font-medium">{t('appLogs.messageNotFoundTitle')}</div>
          <div className="mt-2 text-sm text-muted-foreground">
            {t('appLogs.messageNotFoundDescription')}
          </div>
        </div>
      );
    }

    return (
      <div className="rounded-lg border border-dashed px-4 py-8 text-center">
        <div className="text-sm font-medium">{t('appLogs.noMessagesTitle')}</div>
        <div className="mt-2 text-sm text-muted-foreground">
          {t('appLogs.noMessagesDescription')}
        </div>
      </div>
    );
  }

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_300px]">
      <div className="min-w-0 space-y-4">
        <div>
          <div className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
            {t('appLogs.query')}
          </div>
          <div className="rounded-lg bg-muted/40 p-4 text-sm leading-relaxed">
            {activeMessage.query || t('appLogs.noQuery')}
          </div>
        </div>

        <div>
          <div className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
            {t('appLogs.answer')}
          </div>
          <div className="max-h-64 overflow-y-auto rounded-lg border bg-background p-4 text-sm leading-relaxed">
            {activeMessage.answer ? (
              <MarkdownViewer content={activeMessage.answer} />
            ) : (
              <div className="text-muted-foreground">{t('appLogs.noAnswerYet')}</div>
            )}
          </div>
        </div>
      </div>

      <div className="min-w-0">
        <div className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">
          {t('appLogs.sameConversation')}
        </div>
        <div className="max-h-80 space-y-2 overflow-y-auto pr-1">
          {messages.map(message => (
            <MessageListItem
              key={message.id}
              message={message}
              activeRunId={activeRunId}
              onInspect={onInspect}
            />
          ))}
        </div>
      </div>
    </div>
  );
}
