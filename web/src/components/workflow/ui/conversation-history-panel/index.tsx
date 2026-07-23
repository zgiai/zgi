'use client';

import React from 'react';
import { Panel } from '@xyflow/react';
import { ArrowLeft, MessageSquareText } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import MarkdownViewer from '@/components/common/markdown-viewer';
import { usePanelStackItem } from '../../hooks';
import { useWorkflowStore } from '../../store';
import type { WorkflowEdge, WorkflowNode } from '../../store/type';
import { useWorkflowChatMessages } from '@/hooks';
import { useT } from '@/i18n';
import type { ScopedTranslations } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import { formatDate } from '@/utils/format';
import { isSensitiveOutputBlockedValue } from '@/utils/model-output-filter';
import type { WorkflowChatMessageItem } from '@/services/types/workflow';
import ExecutionTab from '../workflow-run-panel/components/workflow-run-panel-execution';
import DetailsTab from '../workflow-run-panel/components/workflow-run-panel-details';
import { useHistoryView } from '../workflow-run-panel/hooks/use-history-view';
import type { WorkflowFinishedData } from '../workflow-run-panel/types';
import type { ConversationHistoryMessageItem, SelectedMessageRunState } from './types';
import { getRightPanelMotionClassName, getRightPanelMotionStyle } from '../right-panel-motion';
import { RunStatusBadge } from '../run-status-badge';

interface ConversationHistoryPanelProps {
  open: boolean;
  temporarilyHidden?: boolean;
  agentId: string;
}

type ConversationHistoryView = 'messages' | 'detail';

const HISTORY_PANEL_WIDTH = 480;

function normalizeTimestamp(value?: number | null): number {
  if (typeof value !== 'number' || Number.isNaN(value)) return 0;
  return value < 1_000_000_000_000 ? value * 1000 : value;
}

function mapConversationMessages(
  messages: WorkflowChatMessageItem[]
): ConversationHistoryMessageItem[] {
  return [...messages]
    .sort((a, b) => normalizeTimestamp(a.created_at) - normalizeTimestamp(b.created_at))
    .map(message => ({
      id: message.id,
      conversationId: message.conversation_id,
      query: message.query,
      answer: message.answer,
      createdAt: message.created_at,
      workflowRunId: message.workflow_run_id,
      invokeFrom: message.invoke_from,
      parentMessageId: message.parent_message_id,
    }));
}

function getStringValue(value: unknown): string | null {
  if (typeof value !== 'string') return null;

  const trimmed = value.trim();
  return trimmed.length > 0 ? trimmed : null;
}

function getInputQuery(inputs?: Record<string, unknown>): string | null {
  if (!inputs) return null;

  return getStringValue(inputs.query);
}

function getErrorMessage(error: unknown): string | null {
  const direct = getStringValue(error);
  if (direct) return direct;

  if (!error || typeof error !== 'object' || Array.isArray(error)) return null;

  const payload = error as Record<string, unknown>;
  return (
    getStringValue(payload.message) ||
    getStringValue(payload.error) ||
    getStringValue(payload.reason) ||
    JSON.stringify(payload)
  );
}

function createVirtualRunMessage(
  runId: string,
  summary: WorkflowFinishedData | null | undefined,
  t: ScopedTranslations<'agents'>
): ConversationHistoryMessageItem | null {
  if (!summary) return null;

  const error = getErrorMessage(summary.error);
  const fallbackAnswer = t('workflow.conversationHistory.virtualRunAnswer');
  const answer = error
    ? `${fallbackAnswer}\n\n${t('workflow.conversationHistory.virtualRunErrorPrefix')}: ${error}`
    : fallbackAnswer;

  return {
    id: `virtual-run-message-${runId}`,
    conversationId: summary.conversation_id ?? '',
    query: getInputQuery(summary.inputs) ?? t('workflow.conversationHistory.virtualRunQuery'),
    answer,
    createdAt: summary.created_at ?? Date.now(),
    workflowRunId: runId,
    status: summary.status,
    error,
    isVirtual: true,
  };
}

function isFailedRunStatus(status: string | undefined) {
  const normalized = (status || '').toLowerCase();
  return normalized === 'failed' || normalized === 'error' || normalized === 'stopped';
}

function InspectorError({ message }: { message: string }) {
  return (
    <div className="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">
      {message}
    </div>
  );
}

/**
 * @component ConversationHistoryPanel
 * @category Feature
 * @status Stable
 * @description Conversation workflow history panel that follows run-log-first navigation and swaps between message and run detail views.
 * @usage Open after selecting a conversation workflow run from the history dropdown.
 */
export function ConversationHistoryPanel({
  open,
  temporarilyHidden = false,
  agentId,
}: ConversationHistoryPanelProps) {
  const { panelStyle } = usePanelStackItem({
    id: 'conversation-history',
    position: 'top-right',
    order: 3,
    visible: open,
    width: HISTORY_PANEL_WIDTH,
    gap: 8,
  });
  const t = useT('agents');
  const tCommon = useT('common');
  const mode = useWorkflowStore.use.mode();
  const selectedRunId = useWorkflowStore.use.selectedRunId();
  const enterHistoryMode = useWorkflowStore.use.enterHistoryMode();
  const resetRunStatus = useWorkflowStore.use.resetRunStatus();
  const setHistorySnapshot = useWorkflowStore.use.setHistorySnapshot();

  const [shake, setShake] = React.useState(false);
  const [view, setView] = React.useState<ConversationHistoryView>('messages');
  const [selectedBaseRunId, setSelectedBaseRunId] = React.useState<string | null>(null);
  const [selectedConversationId, setSelectedConversationId] = React.useState<string | null>(null);
  const [selectedMessageRun, setSelectedMessageRun] = React.useState<SelectedMessageRunState>({
    messageId: null,
    runId: null,
  });
  const [inspectorTab, setInspectorTab] = React.useState<'execution' | 'details'>('execution');

  const effectiveRunId = selectedMessageRun.runId ?? selectedBaseRunId;

  const {
    messages,
    isLoading: isMessagesLoading,
    isFetching: isMessagesFetching,
    error: messagesError,
  } = useWorkflowChatMessages(
    {
      agentId,
      conversationId: selectedConversationId,
      page: 1,
      limit: 100,
    },
    {
      enabled: open && Boolean(selectedConversationId),
      staleTime: 60_000,
      refetchOnWindowFocus: false,
    }
  );

  const messageItems = React.useMemo(() => mapConversationMessages(messages), [messages]);

  const hasSelectedRunMessage = React.useMemo(() => {
    if (!selectedBaseRunId) return false;
    return messageItems.some(message => message.workflowRunId === selectedBaseRunId);
  }, [messageItems, selectedBaseRunId]);

  const {
    loading: isInspectorLoading,
    summary: inspectorSummary,
    items: inspectorItems,
    graph: inspectorGraph,
    detailError,
    nodeExecutionsError,
  } = useHistoryView({
    agentId,
    open,
    isHistory: open && mode === 'history' && Boolean(effectiveRunId),
    selectedRunId: effectiveRunId,
  });

  React.useEffect(() => {
    if (!open) return;

    const win = window as Window & {
      __workflowConversationHistoryPanelOpen?: boolean;
      __workflowConversationHistoryPanelShake?: () => void;
    };

    win.__workflowConversationHistoryPanelOpen = true;
    win.__workflowConversationHistoryPanelShake = () => {
      setShake(true);
      window.setTimeout(() => setShake(false), 600);
    };

    return () => {
      win.__workflowConversationHistoryPanelOpen = false;
      win.__workflowConversationHistoryPanelShake = undefined;
    };
  }, [open]);

  React.useEffect(() => {
    if (!open || mode !== 'history' || !effectiveRunId || !inspectorGraph) return;

    const graphNodes = (
      Array.isArray(inspectorGraph.nodes) ? inspectorGraph.nodes : []
    ) as WorkflowNode[];
    const graphEdges = (
      Array.isArray(inspectorGraph.edges) ? inspectorGraph.edges : []
    ) as WorkflowEdge[];
    const graphViewport = inspectorGraph.viewport ?? { x: 0, y: 0, zoom: 1 };

    setHistorySnapshot(effectiveRunId, {
      nodes: graphNodes,
      edges: graphEdges,
      viewport: graphViewport,
    });
  }, [effectiveRunId, inspectorGraph, mode, open, setHistorySnapshot]);

  React.useEffect(() => {
    if (!open || !selectedRunId) return;
    if (selectedMessageRun.runId === selectedRunId) return;

    setSelectedBaseRunId(selectedRunId);
    setSelectedConversationId(null);
    setSelectedMessageRun({ messageId: null, runId: null });
    setInspectorTab('execution');
    setView('messages');
  }, [open, selectedRunId, selectedMessageRun.runId]);

  React.useEffect(() => {
    if (!selectedBaseRunId || selectedMessageRun.runId || !inspectorSummary?.conversation_id) {
      return;
    }
    setSelectedConversationId(inspectorSummary.conversation_id);
  }, [inspectorSummary?.conversation_id, selectedBaseRunId, selectedMessageRun.runId]);

  const displayMessageItems = React.useMemo(() => {
    if (!selectedBaseRunId || hasSelectedRunMessage) return messageItems;

    const virtualMessage = createVirtualRunMessage(selectedBaseRunId, inspectorSummary, t);
    if (!virtualMessage) return messageItems;

    return [...messageItems, virtualMessage].sort(
      (a, b) => normalizeTimestamp(a.createdAt) - normalizeTimestamp(b.createdAt)
    );
  }, [hasSelectedRunMessage, inspectorSummary, messageItems, selectedBaseRunId, t]);

  React.useEffect(() => {
    if (!selectedBaseRunId || selectedMessageRun.runId || view !== 'messages') return;
    if (isMessagesLoading || isMessagesFetching || messagesError) return;
    if (messageItems.length > 0 || !isFailedRunStatus(inspectorSummary?.status)) return;

    setInspectorTab(inspectorItems.length > 0 ? 'execution' : 'details');
    setView('detail');
  }, [
    inspectorItems.length,
    inspectorSummary?.status,
    isMessagesFetching,
    isMessagesLoading,
    messageItems.length,
    messagesError,
    selectedBaseRunId,
    selectedMessageRun.runId,
    view,
  ]);

  React.useEffect(() => {
    if (mode === 'history') return;
    resetRunStatus();
    setSelectedBaseRunId(null);
    setSelectedConversationId(null);
    setView('messages');
    setSelectedMessageRun({ messageId: null, runId: null });
  }, [mode, resetRunStatus]);

  const handleInspectMessage = React.useCallback(
    (message: ConversationHistoryMessageItem) => {
      if (!message.workflowRunId) return;

      setSelectedMessageRun({
        messageId: message.id,
        runId: message.workflowRunId,
      });
      setInspectorTab('execution');
      setView('detail');
      enterHistoryMode(message.workflowRunId);
    },
    [enterHistoryMode]
  );

  const handleBackToMessages = React.useCallback(() => {
    if (selectedBaseRunId) {
      enterHistoryMode(selectedBaseRunId);
    }
    resetRunStatus();
    setInspectorTab('execution');
    setView('messages');
    setSelectedMessageRun({ messageId: null, runId: null });
  }, [enterHistoryMode, resetRunStatus, selectedBaseRunId]);

  if (!open) {
    return null;
  }

  return (
    <Panel
      position="top-right"
      aria-hidden={temporarilyHidden}
      className={getRightPanelMotionClassName(
        cn(
          'p-0 bg-primary-foreground border border-muted rounded-lg shadow-lg w-[480px] h-[calc(100%-120px)] overflow-hidden',
          shake ? 'workflow-panel-attention' : ''
        ),
        temporarilyHidden
      )}
      style={getRightPanelMotionStyle(panelStyle, temporarilyHidden)}
    >
      <div
        className="flex h-full flex-col"
        onContextMenu={event => {
          event.stopPropagation();
        }}
      >
        <div className="flex items-center border-b border-border/50 px-3 py-2">
          <div className="font-medium">{t('workflow.conversationHistory.title')}</div>
        </div>

        {view === 'messages' ? (
          <>
            <div className="border-b border-border/50 px-3 py-3">
              {!selectedBaseRunId ? (
                <div className="text-sm text-muted-foreground">
                  {t('workflow.conversationHistory.selectRunFromDropdown')}
                </div>
              ) : inspectorSummary ? (
                <div className="flex flex-wrap gap-2">
                  <RunStatusBadge status={inspectorSummary.status} />
                  <Badge variant="outline">
                    {t('workflow.workflowRunId')}: {selectedBaseRunId}
                  </Badge>
                  {inspectorSummary.conversation_id ? (
                    <Badge variant="outline">
                      {t('workflow.conversationHistory.conversations')}:{' '}
                      {inspectorSummary.conversation_id}
                    </Badge>
                  ) : null}
                </div>
              ) : (
                <div className="space-y-2">
                  <Skeleton className="h-6 w-28 rounded-full" />
                  <Skeleton className="h-6 w-full rounded-full" />
                </div>
              )}
            </div>

            <div className="min-h-0 flex-1 overflow-y-auto bg-muted/10 px-3 py-4">
              {!selectedBaseRunId ? (
                <div className="flex h-full flex-col items-center justify-center gap-3 px-6 text-center text-muted-foreground">
                  <div className="rounded-2xl bg-muted p-4">
                    <MessageSquareText className="h-8 w-8" />
                  </div>
                  <div className="text-sm">
                    {t('workflow.conversationHistory.selectRunFromDropdown')}
                  </div>
                </div>
              ) : isMessagesLoading || isMessagesFetching ? (
                <div className="space-y-4">
                  {Array.from({ length: 4 }).map((_, index) => (
                    <Skeleton key={index} className="h-28 w-full rounded-2xl" />
                  ))}
                </div>
              ) : messagesError ? (
                <InspectorError message={messagesError} />
              ) : displayMessageItems.length === 0 ? (
                <div className="flex h-full items-center justify-center text-sm text-muted-foreground">
                  {t('workflow.noMessagesYet')}
                </div>
              ) : (
                <div className="space-y-5">
                  {displayMessageItems.map(message => (
                    <div key={message.id} className="space-y-2">
                      <div className="flex justify-end">
                        <div className="max-w-[85%] rounded-2xl bg-primary px-4 py-3 text-sm text-primary-foreground shadow-sm">
                          <div className="mb-1 flex items-center justify-between gap-2 text-[11px] text-primary-foreground/70">
                            <span>{formatDate(message.createdAt)}</span>
                            {message.isVirtual ? (
                              <Badge
                                variant="secondary"
                                className="h-5 border-0 bg-primary-foreground/15 px-1.5 text-[10px] text-primary-foreground"
                              >
                                {t('workflow.conversationHistory.virtualMessageBadge')}
                              </Badge>
                            ) : null}
                          </div>
                          <div className="whitespace-pre-wrap break-words">
                            {message.query || '-'}
                          </div>
                        </div>
                      </div>

                      <div className="flex justify-start">
                        <div className="max-w-[92%] rounded-2xl border bg-background px-4 py-3 text-sm shadow-sm">
                          <div className="prose prose-sm max-w-none dark:prose-invert">
                            <MarkdownViewer
                              content={
                                isSensitiveOutputBlockedValue(message.answer)
                                  ? tCommon('sensitiveOutput.blocked')
                                  : message.answer || ''
                              }
                            />
                          </div>

                          <div className="mt-3 flex items-center justify-between gap-3 border-t pt-3">
                            <div className="min-w-0 text-xs text-muted-foreground">
                              {message.isVirtual ? (
                                <span className="truncate">
                                  {t('workflow.conversationHistory.virtualMessageBadge')}
                                </span>
                              ) : message.invokeFrom ? (
                                <span className="truncate">
                                  {t('workflow.invokeFrom')}: {message.invokeFrom}
                                </span>
                              ) : (
                                <span>{t('workflow.messageHistory')}</span>
                              )}
                            </div>

                            {message.workflowRunId ? (
                              <Button
                                size="sm"
                                variant="outline"
                                onClick={() => handleInspectMessage(message)}
                              >
                                {t('workflow.conversationHistory.inspectRun')}
                              </Button>
                            ) : (
                              <span className="text-xs text-muted-foreground">
                                {t('workflow.conversationHistory.noRunForMessage')}
                              </span>
                            )}
                          </div>
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </>
        ) : (
          <>
            <div className="flex items-center justify-between border-b border-border/50 px-3 py-3">
              <Button variant="ghost" size="sm" onClick={handleBackToMessages}>
                <ArrowLeft className="h-4 w-4" />
                {t('workflow.conversationHistory.backToMessages')}
              </Button>

              <div className="min-w-0 text-right text-xs text-muted-foreground">
                {effectiveRunId
                  ? t('workflow.viewingRunHistoryWithId', { id: effectiveRunId })
                  : t('workflow.conversationHistory.runInspector')}
              </div>
            </div>

            <div className="flex h-0 grow flex-col overflow-hidden">
              {(detailError || nodeExecutionsError) && !isInspectorLoading ? (
                <div className="px-4 pt-4">
                  <InspectorError
                    message={detailError || nodeExecutionsError || t('workflow.failed')}
                  />
                </div>
              ) : null}

              <Tabs
                value={inspectorTab}
                onValueChange={value => setInspectorTab(value as 'execution' | 'details')}
                className="flex h-full flex-col"
              >
                <div className="px-4 pt-4">
                  <TabsList className="w-full">
                    <TabsTrigger value="execution" className="flex-1">
                      {t('workflow.execution')}
                    </TabsTrigger>
                    <TabsTrigger value="details" className="flex-1">
                      {t('workflow.runDetails')}
                    </TabsTrigger>
                  </TabsList>
                </div>

                <TabsContent
                  value="execution"
                  className="mt-3 flex-1 overflow-y-auto px-4 pb-4 outline-none"
                >
                  {isInspectorLoading ? (
                    <div className="space-y-3">
                      {Array.from({ length: 4 }).map((_, index) => (
                        <Skeleton key={index} className="h-24 w-full rounded-xl" />
                      ))}
                    </div>
                  ) : (
                    <ExecutionTab items={inspectorItems} showDetail />
                  )}
                </TabsContent>

                <TabsContent
                  value="details"
                  className="mt-3 flex-1 overflow-y-auto px-4 pb-4 outline-none"
                >
                  {isInspectorLoading ? (
                    <div className="space-y-3">
                      <Skeleton className="h-16 w-full rounded-xl" />
                      <Skeleton className="h-16 w-full rounded-xl" />
                      <Skeleton className="h-40 w-full rounded-xl" />
                    </div>
                  ) : (
                    <DetailsTab runSummary={inspectorSummary} />
                  )}
                </TabsContent>
              </Tabs>
            </div>
          </>
        )}
      </div>
    </Panel>
  );
}
