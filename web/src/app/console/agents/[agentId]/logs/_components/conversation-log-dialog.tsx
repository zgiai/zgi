'use client';

import { MessageSquareText } from 'lucide-react';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { useT } from '@/i18n/translations';
import type { WorkflowChatMessageItem } from '@/services/types/workflow';
import { formatDate } from '@/utils/format';

interface ConversationLogDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  messages: WorkflowChatMessageItem[];
  activeRunId: string | null;
  isLoading: boolean;
  error?: string | null;
  onInspect: (message: WorkflowChatMessageItem) => void;
}

export function ConversationLogDialog({
  open,
  onOpenChange,
  messages,
  activeRunId,
  isLoading,
  error,
  onInspect,
}: ConversationLogDialogProps) {
  const t = useT('webapp');

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="xl" className="h-[min(760px,calc(100vh-2rem))] overflow-hidden p-0">
        <DialogHeader className="border-b pr-14">
          <div className="flex items-center gap-2">
            <MessageSquareText className="size-5 text-primary" />
            <DialogTitle>{t('appLogs.conversationDialogTitle')}</DialogTitle>
          </div>
          <DialogDescription>{t('appLogs.conversationDialogDescription')}</DialogDescription>
        </DialogHeader>

        <DialogBody className="bg-muted/10 px-6 py-5">
          {isLoading ? (
            <div className="space-y-5">
              {Array.from({ length: 3 }).map((_, index) => (
                <Skeleton key={index} className="h-40 rounded-2xl" />
              ))}
            </div>
          ) : error ? (
            <div className="rounded-xl border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive">
              {error}
            </div>
          ) : messages.length === 0 ? (
            <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
              {t('appLogs.noMessagesDescription')}
            </div>
          ) : (
            <div className="mx-auto max-w-3xl space-y-5">
              {messages.map(message => {
                const isCurrent = Boolean(
                  activeRunId && message.workflow_run_id === activeRunId
                );

                return (
                  <div key={message.id} className="space-y-2">
                    <div className="flex justify-end">
                      <div className="max-w-[85%] rounded-2xl bg-primary px-4 py-3 text-sm text-primary-foreground shadow-sm">
                        <div className="mb-1 text-[11px] text-primary-foreground/70">
                          {formatDate(message.created_at)}
                        </div>
                        <div className="whitespace-pre-wrap break-words">
                          {message.query || t('appLogs.noQuery')}
                        </div>
                      </div>
                    </div>

                    <div className="flex justify-start">
                      <div className="max-w-[92%] rounded-2xl border bg-background px-4 py-3 text-sm shadow-sm">
                        {message.answer ? (
                          <div className="prose prose-sm max-w-none dark:prose-invert">
                            <MarkdownViewer preserveSoftBreaks content={message.answer} />
                          </div>
                        ) : (
                          <div className="text-muted-foreground">{t('appLogs.noAnswerYet')}</div>
                        )}

                        <div className="mt-3 flex items-center justify-end border-t pt-3">
                          {isCurrent ? (
                            <Badge variant="subtle">{t('appLogs.currentTurn')}</Badge>
                          ) : message.workflow_run_id ? (
                            <Button
                              type="button"
                              size="sm"
                              variant="outline"
                              onClick={() => {
                                onInspect(message);
                                onOpenChange(false);
                              }}
                            >
                              {t('appLogs.switchToTurn')}
                            </Button>
                          ) : (
                            <span className="text-xs text-muted-foreground">
                              {t('appLogs.noRunForMessage')}
                            </span>
                          )}
                        </div>
                      </div>
                    </div>
                  </div>
                );
              })}
            </div>
          )}
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}
