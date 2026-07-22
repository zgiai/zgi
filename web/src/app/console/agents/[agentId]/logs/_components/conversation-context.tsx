'use client';

import MarkdownViewer from '@/components/common/markdown-viewer';
import { Skeleton } from '@/components/ui/skeleton';
import { useT } from '@/i18n/translations';
import type { WorkflowChatMessageItem } from '@/services/types/workflow';

interface ConversationContextProps {
  messages: WorkflowChatMessageItem[];
  activeRunId: string | null;
  isLoading: boolean;
  error?: string | null;
}

export function ConversationContext({
  messages,
  activeRunId,
  isLoading,
  error,
}: ConversationContextProps) {
  const t = useT('webapp');
  const activeMessage = activeRunId
    ? (messages.find(message => message.workflow_run_id === activeRunId) ?? null)
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
      <div className="space-y-3">
        <div className="flex justify-end">
          <Skeleton className="h-16 w-2/3 max-w-xl rounded-xl" />
        </div>
        <Skeleton className="h-40 rounded-xl" />
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
    <div className="space-y-3">
      <div className="flex justify-end">
        <section className="min-w-48 max-w-[min(82%,48rem)] rounded-xl border bg-muted/30 px-4 py-3">
          <div className="mb-1 text-xs font-medium text-muted-foreground">
            {t('appLogs.query')}
          </div>
          <div className="whitespace-pre-wrap break-words text-sm leading-6">
            {activeMessage.query || t('appLogs.noQuery')}
          </div>
        </section>
      </div>

      <section className="min-w-0 rounded-xl border bg-background px-4 py-3">
        <div className="mb-2 text-xs font-medium text-muted-foreground">
          {t('appLogs.answer')}
        </div>
        <div className="text-sm leading-6">
          {activeMessage.answer ? (
            <MarkdownViewer preserveSoftBreaks content={activeMessage.answer} />
          ) : (
            <div className="text-muted-foreground">{t('appLogs.noAnswerYet')}</div>
          )}
        </div>
      </section>
    </div>
  );
}
