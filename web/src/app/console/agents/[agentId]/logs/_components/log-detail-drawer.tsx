'use client';

import { ChevronLeft, MessageSquareText } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import HistoryContent from '@/components/workflow/ui/workflow-run-panel/components/history-content';
import type {
  HistoryResult,
  WorkflowFinishedData,
} from '@/components/workflow/ui/workflow-run-panel/types';
import type { WorkflowRunNodeListItem } from '@/components/workflow/ui/workflow-run-nodes-list';
import { useT } from '@/i18n/translations';
import type { WorkflowChatMessageItem } from '@/services/types/workflow';
import { formatDate, formatWorkflowElapsedMs } from '@/utils/format';
import { ConversationContext } from './conversation-context';
import { RunStatusBadge } from '@/components/workflow/ui/run-status-badge';

export type HistoryTab = 'details' | 'execution' | 'results' | 'inputs';

interface LogDetailDrawerProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  selectedLogId: string | null;
  effectiveRunId: string | null;
  summary: WorkflowFinishedData | null;
  activeTab: HistoryTab;
  setActiveTab: (tab: HistoryTab) => void;
  detailLoading: boolean;
  executionItems: WorkflowRunNodeListItem[];
  result: HistoryResult;
  detailError?: string | null;
  nodeExecutionsError?: string | null;
  isConversationWorkflow: boolean;
  sortedMessages: WorkflowChatMessageItem[];
  isMessagesLoading: boolean;
  messagesError?: string | null;
  selectedMessageRunId: string | null;
  onInspectMessage: (message: WorkflowChatMessageItem) => void;
  onBackToSelectedRun: () => void;
  showDeepLinkedHint?: boolean;
}

export function LogDetailDrawer({
  open,
  onOpenChange,
  selectedLogId: _selectedLogId,
  effectiveRunId,
  summary,
  activeTab,
  setActiveTab,
  detailLoading,
  executionItems,
  result,
  detailError,
  nodeExecutionsError,
  isConversationWorkflow,
  sortedMessages,
  isMessagesLoading,
  messagesError,
  selectedMessageRunId,
  onInspectMessage,
  onBackToSelectedRun,
  showDeepLinkedHint = false,
}: LogDetailDrawerProps) {
  const t = useT('webapp');
  const tAgents = useT('agents');
  const tCommon = useT('common');

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        showClose={false}
        className="flex h-full w-screen max-w-none flex-col gap-0 p-0 md:w-[80vw] sm:max-w-none"
      >
        <SheetHeader className="shrink-0 border-b px-5 py-4 text-left">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <SheetTitle className="text-base">{t('appLogs.dialogTitle')}</SheetTitle>
              {summary?.status ? <RunStatusBadge status={summary.status} /> : null}
              <SheetClose asChild>
                <Button
                  type="button"
                  variant="outline"
                  size="xs"
                  className="shrink-0"
                  onClick={() => onOpenChange(false)}
                >
                  {tCommon('close')}
                </Button>
              </SheetClose>
            </div>
            <SheetDescription className="mt-1 truncate" title={effectiveRunId ?? ''}>
              {effectiveRunId
                ? t('appLogs.dialogDescription', { id: effectiveRunId })
                : t('appLogs.selectRunDescription')}
            </SheetDescription>
          </div>
        </SheetHeader>

        {summary ? (
          <div className="grid shrink-0 grid-cols-2 gap-3 border-b px-5 py-4 text-xs text-muted-foreground md:grid-cols-4">
            <div className="min-w-0">
              <div className="font-medium text-foreground">{t('appLogs.columns.createdAt')}</div>
              <div className="truncate">
                {summary.created_at ? formatDate(summary.created_at) : '-'}
              </div>
            </div>
            <div className="min-w-0">
              <div className="font-medium text-foreground">{tAgents('workflow.elapsed')}</div>
              <div className="truncate">
                {typeof summary.elapsed_time === 'number'
                  ? formatWorkflowElapsedMs(summary.elapsed_time)
                  : '-'}
              </div>
            </div>
            <div className="min-w-0">
              <div className="font-medium text-foreground">{tAgents('workflow.steps')}</div>
              <div className="truncate">
                {typeof summary.total_steps === 'number' ? summary.total_steps : '-'}
              </div>
            </div>
            <div className="min-w-0">
              <div className="font-medium text-foreground">{t('appLogs.columns.conversation')}</div>
              <div className="truncate" title={summary.conversation_id ?? '-'}>
                {summary.conversation_id ?? '-'}
              </div>
            </div>
          </div>
        ) : null}

        {selectedMessageRunId ? (
          <div className="shrink-0 border-b px-5 py-3">
            <Button type="button" variant="ghost" size="xs" onClick={onBackToSelectedRun}>
              <ChevronLeft className="size-4" />
              {t('appLogs.backToSelectedRun')}
            </Button>
          </div>
        ) : null}

        {showDeepLinkedHint ? (
          <div className="shrink-0 border-b px-5 py-3 text-sm text-muted-foreground">
            {t('appLogs.deepLinkedRunHint')}
          </div>
        ) : null}

        {!effectiveRunId ? (
          <div className="flex min-h-0 flex-1 items-center justify-center px-6 text-center text-sm text-muted-foreground">
            {t('appLogs.selectRunDescription')}
          </div>
        ) : (
          <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
            {isConversationWorkflow ? (
              <div className="max-h-[360px] shrink-0 overflow-y-auto border-b p-5">
                <div className="mb-3 flex items-center gap-2">
                  <MessageSquareText className="size-4 text-primary" />
                  <div className="text-sm font-medium">{t('appLogs.conversationContext')}</div>
                </div>
                <ConversationContext
                  messages={sortedMessages}
                  activeRunId={effectiveRunId}
                  isLoading={isMessagesLoading}
                  error={messagesError}
                  onInspect={onInspectMessage}
                />
              </div>
            ) : null}

            {detailError || nodeExecutionsError ? (
              <div className="mx-5 mt-4 shrink-0 rounded-xl border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive">
                {detailError || nodeExecutionsError}
              </div>
            ) : null}

            <div className="min-h-0 flex-1 overflow-hidden">
              <HistoryContent
                key={effectiveRunId}
                activeTab={activeTab}
                setActiveTab={setActiveTab}
                loading={detailLoading}
                summary={summary}
                items={executionItems}
                result={result}
              />
            </div>
          </div>
        )}
      </SheetContent>
    </Sheet>
  );
}
