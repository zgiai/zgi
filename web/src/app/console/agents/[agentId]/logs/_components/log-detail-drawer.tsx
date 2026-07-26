'use client';

import { useEffect, useState } from 'react';
import { ChevronLeft, ListTree, MessageSquareText } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Sheet, SheetContent } from '@/components/ui/sheet';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import HistoryContent from '@/components/workflow/ui/workflow-run-panel/components/history-content';
import type {
  HistoryResult,
  WorkflowFinishedData,
} from '@/components/workflow/ui/workflow-run-panel/types';
import type { WorkflowRunNodeListItem } from '@/components/workflow/ui/workflow-run-nodes-list';
import { useT } from '@/i18n/translations';
import type { WorkflowChatMessageItem } from '@/services/types/workflow';
import { formatDate, formatWorkflowElapsedMs } from '@/utils/format';
import { ConversationLogDialog } from './conversation-log-dialog';
import { ConversationContext } from './conversation-context';
import { RuntimeLogDetailHeader } from './runtime-log-detail-header';

export type HistoryTab = 'details' | 'execution' | 'results' | 'inputs';

function shortenIdentifier(value?: string | null): string {
  if (!value) return '-';
  if (value.length <= 18) return value;
  return `${value.slice(0, 8)}…${value.slice(-6)}`;
}

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
  sourceLabel?: string | null;
  version?: string | null;
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
  sourceLabel,
  version,
}: LogDetailDrawerProps) {
  const t = useT('webapp');
  const tAgents = useT('agents');
  const tCommon = useT('common');
  const [conversationDialogOpen, setConversationDialogOpen] = useState(false);
  const [conversationDetailTab, setConversationDetailTab] = useState<
    'conversation' | 'execution'
  >('conversation');

  useEffect(() => {
    if (open && showDeepLinkedHint) setConversationDetailTab('execution');
  }, [effectiveRunId, open, showDeepLinkedHint]);

  const executionContent = (
    <>
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
          navigationVariant="compact"
          visibleTabs={isConversationWorkflow ? ['execution'] : ['execution', 'results']}
        />
      </div>
    </>
  );

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        showClose={false}
        className="flex h-full w-screen max-w-none flex-col gap-0 p-0 md:w-[80vw] sm:max-w-none"
      >
        <RuntimeLogDetailHeader
          title={t('appLogs.dialogTitle')}
          description={
            effectiveRunId
              ? t('appLogs.dialogDescription', { id: effectiveRunId })
              : t('appLogs.selectRunDescription')
          }
          runId={effectiveRunId}
          status={summary?.status}
          sourceLabel={sourceLabel}
          closeLabel={tCommon('close')}
          onClose={() => onOpenChange(false)}
        />

        {summary ? (
          <div className="flex shrink-0 flex-wrap items-center gap-x-7 gap-y-2 border-b px-5 py-3 text-xs">
            <div className="flex items-center gap-2">
              <span className="text-muted-foreground">{t('appLogs.columns.createdAt')}</span>
              <span className="font-medium text-foreground">
                {summary.created_at ? formatDate(summary.created_at) : '-'}
              </span>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-muted-foreground">{tAgents('workflow.elapsed')}</span>
              <span className="font-medium text-foreground">
                {typeof summary.elapsed_time === 'number'
                  ? formatWorkflowElapsedMs(summary.elapsed_time)
                  : '-'}
              </span>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-muted-foreground">{tAgents('workflow.steps')}</span>
              <span className="font-medium text-foreground">
                {typeof summary.total_steps === 'number' ? summary.total_steps : '-'}
              </span>
            </div>
            {summary.conversation_id ? (
              <div className="flex min-w-0 items-center gap-2">
                <span className="text-muted-foreground">{t('appLogs.columns.conversation')}</span>
                <span className="font-medium text-foreground" title={summary.conversation_id}>
                  {shortenIdentifier(summary.conversation_id)}
                </span>
              </div>
            ) : null}
            {version ? (
              <div className="flex min-w-0 items-center gap-2">
                <span className="text-muted-foreground">{t('appLogs.columns.version')}</span>
                <span className="font-medium text-foreground" title={version}>
                  {shortenIdentifier(version)}
                </span>
              </div>
            ) : null}
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
              <Tabs
                value={conversationDetailTab}
                onValueChange={value =>
                  setConversationDetailTab(value as 'conversation' | 'execution')
                }
                className="flex min-h-0 flex-1 flex-col"
              >
                <div className="flex shrink-0 items-center justify-between gap-3 border-b px-5 py-2.5">
                  <TabsList className="h-8">
                    <TabsTrigger value="conversation" className="h-7 gap-1.5 px-3 text-xs">
                      <MessageSquareText className="size-3.5" />
                      {t('appLogs.conversationContent')}
                    </TabsTrigger>
                    <TabsTrigger value="execution" className="h-7 gap-1.5 px-3 text-xs">
                      <ListTree className="size-3.5" />
                      {t('appLogs.executionDetails')}
                    </TabsTrigger>
                  </TabsList>
                  <div className="flex items-center gap-2">
                    {selectedMessageRunId ? (
                      <Button
                        type="button"
                        variant="ghost"
                        size="xs"
                        onClick={onBackToSelectedRun}
                      >
                        <ChevronLeft className="size-4" />
                        {t('appLogs.backToSelectedRun')}
                      </Button>
                    ) : null}
                    <Button
                      type="button"
                      variant="outline"
                      size="xs"
                      onClick={() => setConversationDialogOpen(true)}
                    >
                      <MessageSquareText className="size-4" />
                      {t('appLogs.viewConversation')}
                    </Button>
                  </div>
                </div>

                <TabsContent
                  value="conversation"
                  className="mt-0 h-0 min-h-0 grow overflow-y-auto px-5 py-4 outline-none"
                >
                  <div className="mx-auto max-w-5xl">
                    <ConversationContext
                      messages={sortedMessages}
                      activeRunId={effectiveRunId}
                      isLoading={isMessagesLoading}
                      error={messagesError}
                    />
                  </div>
                </TabsContent>

                <TabsContent
                  value="execution"
                  className="mt-0 h-0 min-h-0 grow overflow-hidden outline-none data-[state=active]:flex data-[state=active]:flex-col"
                >
                  {executionContent}
                </TabsContent>
              </Tabs>
            ) : (
              executionContent
            )}
          </div>
        )}
      </SheetContent>
      <ConversationLogDialog
        open={conversationDialogOpen}
        onOpenChange={setConversationDialogOpen}
        messages={sortedMessages}
        activeRunId={effectiveRunId}
        isLoading={isMessagesLoading}
        error={messagesError}
        onInspect={onInspectMessage}
      />
    </Sheet>
  );
}
