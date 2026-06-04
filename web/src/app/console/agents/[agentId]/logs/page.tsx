'use client';

import Link from 'next/link';
import { use, useEffect, useMemo, useState } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { AlertCircle, ExternalLink, History, Loader2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { TableCell, TableRow } from '@/components/ui/table';
import { StickyDataTable } from '@/components/common/sticky-data-table';
import { useAgent } from '@/hooks/agent/use-agents';
import { useLatestWorkflowVersion } from '@/hooks/workflow/use-workflow';
import { useWorkflowChatMessages } from '@/hooks/workflow/use-workflow-chat-messages';
import { useWorkflowRunDetail } from '@/hooks/workflow/use-workflow-run-detail';
import { useWorkflowRunNodeExecutions } from '@/hooks/workflow/use-workflow-run-node-executions';
import { useWorkflowRunsInfinite } from '@/hooks/workflow/use-workflow-runs';
import { useT } from '@/i18n/translations';
import { AgentType } from '@/services/types/agent';
import type { WorkflowChatMessageItem, WorkflowRunItem } from '@/services/types/workflow';
import { formatDate, formatWorkflowElapsedMs } from '@/utils/format';
import { canShowAgentRuntimeLogs } from '@/utils/agent-detail-routes';
import { getErrorMessage } from '@/utils/error-notifications';
import {
  buildWorkflowRunExecutionItems,
  buildWorkflowRunHistoryResult,
  buildWorkflowRunSummary,
} from '@/components/workflow/ui/workflow-run-panel/utils/history-view-data';
import { cn } from '@/lib/utils';
import { LogDetailDrawer, type HistoryTab } from './_components/log-detail-drawer';
import { LogStatusBadge } from './_components/log-status-badge';

interface AgentLogsPageProps {
  params: Promise<{ agentId: string }>;
}

function normalizeTimestamp(value?: number | null): number {
  if (typeof value !== 'number' || Number.isNaN(value)) return 0;
  return value < 1_000_000_000_000 ? value * 1000 : value;
}

function normalizeUnknownTimestamp(value?: number | string | null): number {
  if (typeof value === 'number') {
    return normalizeTimestamp(value);
  }
  if (typeof value === 'string') {
    const parsed = Date.parse(value);
    return Number.isNaN(parsed) ? 0 : parsed;
  }
  return 0;
}

function buildFallbackSummaryFromExecutions(
  runId: string,
  executions: ReturnType<typeof buildWorkflowRunExecutionItems>
) {
  if (!runId || executions.length === 0) return null;

  const createdAtMs = executions
    .map(item => normalizeUnknownTimestamp(item.createdAtMs))
    .filter(Boolean)
    .sort((a, b) => a - b);
  const elapsedTime = executions.reduce(
    (total, item) => total + (typeof item.elapsedTime === 'number' ? item.elapsedTime : 0),
    0
  );
  const hasFailed = executions.some(item => item.status === 'failed');
  const hasRunning = executions.some(item => item.status === 'running');
  const status = hasFailed ? 'failed' : hasRunning ? 'running' : 'succeeded';

  return {
    id: runId,
    status,
    created_at: createdAtMs[0],
    elapsed_time: elapsedTime,
    total_steps: executions.length,
  };
}

function LogTableSkeleton() {
  return (
    <div className="rounded-xl border">
      <div className="space-y-2 p-4">
        {Array.from({ length: 8 }).map((_, index) => (
          <Skeleton key={`log-row-${index}`} className="h-12 w-full" />
        ))}
      </div>
    </div>
  );
}

/**
 * @component AgentLogsPage
 * @category Feature
 * @status Stable
 * @description Displays webapp-originated workflow run history under the agent detail area.
 * @usage Use in the agent detail route alongside workflow and api pages.
 */
export default function AgentLogsPage({ params }: AgentLogsPageProps) {
  const t = useT('webapp');
  const tAgents = useT('agents');
  const { agentId } = use(params);
  const router = useRouter();
  const searchParams = useSearchParams();
  const focusRunId = searchParams.get('runId');
  const focusTab = searchParams.get('tab');

  const [selectedLogId, setSelectedLogId] = useState<string | null>(null);
  const [selectedMessageRunId, setSelectedMessageRunId] = useState<string | null>(null);
  const [isDetailOpen, setIsDetailOpen] = useState(false);
  const [activeTab, setActiveTab] = useState<HistoryTab>('execution');

  const { agent, isLoading: isAgentLoading, error: agentError } = useAgent(agentId);
  const agentDetail = agent?.data ?? null;
  const isPublished = agentDetail?.is_published === true;
  const isWorkflowAgent = canShowAgentRuntimeLogs(agentDetail?.agent_type);
  const isConversationWorkflow = agentDetail?.agent_type === AgentType.CONVERSATIONAL_AGENT;

  const { data: latest } = useLatestWorkflowVersion(
    isWorkflowAgent && isPublished ? agentId : null
  );

  const {
    pages,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    isLoading: isRunsLoading,
    error: runsError,
    reload,
  } = useWorkflowRunsInfinite(
    {
      agentId: isWorkflowAgent && isPublished ? agentId : null,
      limit: 50,
      query: { triggered_from: 'web-app' },
    },
    {
      enabled: isWorkflowAgent && isPublished,
      staleTime: 30_000,
      refetchOnWindowFocus: false,
    }
  );

  const runItems = useMemo(() => pages.flat(), [pages]);
  const selectedLog = useMemo(
    () => runItems.find(item => item.id === selectedLogId) ?? null,
    [runItems, selectedLogId]
  );

  useEffect(() => {
    if (!selectedLogId || runItems.some(item => item.id === selectedLogId)) return;
    setSelectedLogId(null);
    setSelectedMessageRunId(null);
    setIsDetailOpen(false);
    setActiveTab('execution');
  }, [runItems, selectedLogId]);

  useEffect(() => {
    if (!focusRunId || isRunsLoading) return;

    const inList = runItems.some(item => item.id === focusRunId);
    if (inList) {
      setSelectedLogId(focusRunId);
      setSelectedMessageRunId(null);
    } else {
      setSelectedLogId(null);
      setSelectedMessageRunId(focusRunId);
    }
    setIsDetailOpen(true);
    setActiveTab(
      focusTab === 'details' || focusTab === 'results' || focusTab === 'inputs'
        ? focusTab
        : 'execution'
    );
  }, [focusRunId, focusTab, isRunsLoading, runItems]);

  const effectiveRunId = selectedMessageRunId ?? selectedLogId;
  const isDetailDrawerOpen = isDetailOpen && Boolean(effectiveRunId);

  const {
    detail,
    isLoading: isDetailLoading,
    error: detailError,
  } = useWorkflowRunDetail(
    { agentId: isWorkflowAgent ? agentId : null, runId: effectiveRunId },
    {
      enabled: Boolean(isWorkflowAgent && isPublished && isDetailOpen && effectiveRunId),
      staleTime: 60_000,
      refetchOnWindowFocus: false,
      suppressErrorToast: Boolean(focusRunId),
    }
  );

  const {
    records: nodeExecutions,
    isLoading: isNodeExecutionsLoading,
    error: nodeExecutionsError,
  } = useWorkflowRunNodeExecutions(
    { agentId: isWorkflowAgent ? agentId : null, runId: effectiveRunId },
    {
      enabled: Boolean(isWorkflowAgent && isPublished && isDetailOpen && effectiveRunId),
      staleTime: 60_000,
      refetchOnWindowFocus: false,
    }
  );

  const conversationId = selectedLog?.conversation_id ?? detail?.conversation_id ?? null;
  const {
    messages,
    isLoading: isMessagesLoading,
    error: messagesError,
  } = useWorkflowChatMessages(
    {
      agentId: isWorkflowAgent ? agentId : null,
      conversationId,
      page: 1,
      limit: 100,
    },
    {
      enabled: Boolean(isPublished && isConversationWorkflow && isDetailOpen && conversationId),
      staleTime: 60_000,
      refetchOnWindowFocus: false,
    }
  );

  const sortedMessages = useMemo(
    () =>
      [...messages].sort(
        (a, b) => normalizeTimestamp(a.created_at) - normalizeTimestamp(b.created_at)
      ),
    [messages]
  );

  const summary = useMemo(() => {
    const detailSummary = buildWorkflowRunSummary(detail);
    if (detailSummary) return detailSummary;
    if (isConversationWorkflow && selectedMessageRunId) return null;
    if (!selectedLog) return null;

    return {
      id: selectedLog.id,
      status: String(selectedLog.status),
      created_at: selectedLog.created_at,
      finished_at: selectedLog.finished_at,
      elapsed_time: selectedLog.elapsed_time,
      total_steps: selectedLog.total_steps,
      total_tokens: selectedLog.total_tokens,
      conversation_id: selectedLog.conversation_id,
      message_id: selectedLog.message_id,
    };
  }, [detail, isConversationWorkflow, selectedLog, selectedMessageRunId]);

  const executionItems = useMemo(
    () => buildWorkflowRunExecutionItems(nodeExecutions),
    [nodeExecutions]
  );
  const result = useMemo(() => buildWorkflowRunHistoryResult(detail), [detail]);
  const fallbackSummary = useMemo(
    () => buildFallbackSummaryFromExecutions(effectiveRunId || '', executionItems),
    [effectiveRunId, executionItems]
  );
  const effectiveSummary = summary ?? fallbackSummary;
  const detailLoading = isDetailLoading || isNodeExecutionsLoading;
  const effectiveDetailError =
    !detailLoading && !summary && fallbackSummary ? null : detailError;
  const webAppId = latest?.data?.web_app_id;
  const webAppHref =
    webAppId && agentDetail
      ? `/webapp/${webAppId}/${agentDetail.agent_type === AgentType.CONVERSATIONAL_AGENT ? 'chat' : 'run'}`
      : null;

  useEffect(() => {
    if (isAgentLoading || !agentDetail || isPublished) return;
    router.replace(`/console/agents/${agentId}/workflow`);
  }, [agentDetail, agentId, isAgentLoading, isPublished, router]);

  const handleSelectLog = (item: WorkflowRunItem) => {
    setSelectedLogId(item.id);
    setSelectedMessageRunId(null);
    setIsDetailOpen(true);
    setActiveTab('execution');
  };

  const handleInspectMessage = (message: WorkflowChatMessageItem) => {
    if (!message.workflow_run_id) return;
    if (runItems.some(item => item.id === message.workflow_run_id)) {
      setSelectedLogId(message.workflow_run_id);
      setSelectedMessageRunId(null);
    } else {
      setSelectedMessageRunId(message.workflow_run_id);
    }
    setIsDetailOpen(true);
    setActiveTab('execution');
  };

  const handleBackToSelectedRun = () => {
    setSelectedMessageRunId(null);
    setActiveTab('execution');
  };

  const handleDetailOpenChange = (open: boolean) => {
    setIsDetailOpen(open);
    if (open) return;
    setSelectedMessageRunId(null);
    setActiveTab('execution');
  };

  if (isAgentLoading) {
    return (
      <div className="h-full w-full p-6">
        <div className="space-y-4">
          <Skeleton className="h-8 w-48" />
          <LogTableSkeleton />
        </div>
      </div>
    );
  }

  if (agentError || !agentDetail) {
    return (
      <div className="flex h-full w-full items-center justify-center p-6">
        <div className="max-w-xl rounded-2xl border border-dashed bg-background p-8 text-center">
          <div className="mx-auto mb-4 flex size-12 items-center justify-center rounded-full bg-muted">
            <AlertCircle className="size-5 text-muted-foreground" />
          </div>
          <div className="text-lg font-semibold">{tAgents('workflow.loadFailedTitle')}</div>
          <div className="mt-2 text-sm text-muted-foreground">
            {agentError ? getErrorMessage(agentError) : tAgents('workflow.notFoundDesc')}
          </div>
        </div>
      </div>
    );
  }

  if (!isWorkflowAgent) {
    return (
      <div className="flex h-full w-full items-center justify-center p-6">
        <div className="max-w-xl rounded-2xl border border-dashed bg-background p-8 text-center">
          <div className="mx-auto mb-4 flex size-12 items-center justify-center rounded-full bg-muted">
            <AlertCircle className="size-5 text-muted-foreground" />
          </div>
          <div className="text-lg font-semibold">{t('appCenter.appUnavailableTitle')}</div>
          <div className="mt-2 text-sm text-muted-foreground">
            {t('appCenter.appUnavailableDescription')}
          </div>
        </div>
      </div>
    );
  }

  if (!isPublished) {
    return (
      <div className="flex h-full w-full items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="flex h-full w-full min-h-0 flex-col overflow-hidden bg-background">
      <div className="border-b px-6 py-4">
        <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div className="min-w-0">
            <div className="flex items-center gap-2 text-lg font-semibold">
              <History className="size-5 text-primary" />
              <span>{t('appLogs.title')}</span>
            </div>
            <div className="mt-1 text-sm text-muted-foreground">
              {agentDetail.name} · {t('appLogs.subtitle')}
            </div>
          </div>
          {webAppHref ? (
            <Button asChild variant="outline" size="sm" className="w-fit">
              <Link href={webAppHref} target="_blank" rel="noreferrer">
                <ExternalLink className="size-4" />
                {t('appLogs.openApp')}
              </Link>
            </Button>
          ) : null}
        </div>
      </div>

      <div className="h-0 min-h-0 flex-1 overflow-hidden p-4">
        <section className="flex h-full min-h-0 flex-col overflow-hidden rounded-xl border bg-background">
          <div className="flex shrink-0 flex-col gap-1 border-b px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="text-sm font-semibold">{t('appLogs.recentRuns')}</div>
            <div className="text-xs text-muted-foreground">{t('appLogs.selectRunDescription')}</div>
          </div>

          <div className="min-h-0 flex-1 overflow-hidden">
            {runsError ? (
              <div className="m-4 space-y-3 rounded-xl border border-dashed p-4 text-sm">
                <div className="text-destructive">{runsError}</div>
                <Button size="sm" variant="outline" onClick={() => void reload()}>
                  {tAgents('workflow.retry')}
                </Button>
              </div>
            ) : isRunsLoading && runItems.length === 0 ? (
              <LogTableSkeleton />
            ) : runItems.length === 0 ? (
              <div className="m-4 rounded-2xl border border-dashed px-4 py-10 text-center">
                <div className="text-sm font-medium">{t('appLogs.noLogsTitle')}</div>
                <div className="mt-2 text-sm text-muted-foreground">
                  {t('appLogs.noLogsDescription')}
                </div>
              </div>
            ) : (
              <StickyDataTable
                className="h-full"
                columns={[
                  { key: 'runId', header: t('appLogs.columns.runId'), className: 'pl-4' },
                  { key: 'status', header: t('appLogs.columns.status') },
                  { key: 'steps', header: tAgents('workflow.steps') },
                  { key: 'elapsed', header: tAgents('workflow.elapsed') },
                  { key: 'createdAt', header: t('appLogs.columns.createdAt') },
                  { key: 'conversation', header: t('appLogs.columns.conversation') },
                ]}
                data={runItems}
                getRowKey={item => item.id}
                pagination={
                  hasNextPage ? (
                    <div className="flex shrink-0 items-center justify-center border-t px-4 py-3">
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        disabled={isFetchingNextPage}
                        onClick={() => void fetchNextPage()}
                      >
                        {isFetchingNextPage ? (
                          <Loader2 className="size-4 animate-spin" />
                        ) : null}
                        {isFetchingNextPage ? t('appLogs.loadingMore') : t('appLogs.loadMore')}
                      </Button>
                    </div>
                  ) : null
                }
              >
                {runItems.map(item => {
                  const isSelected = item.id === selectedLogId;
                  return (
                    <TableRow
                      key={item.id}
                      role="button"
                      tabIndex={0}
                      aria-pressed={isSelected}
                      className={cn(
                        'group cursor-pointer border-b border-border/10 transition-colors interactive-subtle',
                        isSelected ? 'bg-primary/5 hover:bg-primary/10' : 'hover:bg-bg-canvas/40'
                      )}
                      onClick={() => handleSelectLog(item)}
                      onKeyDown={event => {
                        if (event.key !== 'Enter' && event.key !== ' ') return;
                        event.preventDefault();
                        handleSelectLog(item);
                      }}
                    >
                      <TableCell className="max-w-[280px] py-4 pl-4 font-medium">
                        <div className="truncate" title={item.id}>
                          {item.id}
                        </div>
                      </TableCell>
                      <TableCell className="py-4">
                        <LogStatusBadge status={item.status} />
                      </TableCell>
                      <TableCell className="py-4">
                        {typeof item.total_steps === 'number' ? item.total_steps : '-'}
                      </TableCell>
                      <TableCell className="py-4">
                        {typeof item.elapsed_time === 'number'
                          ? formatWorkflowElapsedMs(item.elapsed_time)
                          : '-'}
                      </TableCell>
                      <TableCell className="py-4">
                        {typeof item.created_at === 'number' ? formatDate(item.created_at) : '-'}
                      </TableCell>
                      <TableCell className="max-w-[260px] py-4">
                        <div className="truncate" title={item.conversation_id ?? '-'}>
                          {item.conversation_id ?? '-'}
                        </div>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </StickyDataTable>
            )}
          </div>
        </section>

        <LogDetailDrawer
          open={isDetailDrawerOpen}
          onOpenChange={handleDetailOpenChange}
          selectedLogId={selectedLogId}
          effectiveRunId={effectiveRunId}
          summary={effectiveSummary}
          activeTab={activeTab}
          setActiveTab={setActiveTab}
          detailLoading={detailLoading}
          executionItems={executionItems}
          result={result}
          detailError={effectiveDetailError}
          nodeExecutionsError={nodeExecutionsError}
          isConversationWorkflow={isConversationWorkflow}
          sortedMessages={sortedMessages}
          isMessagesLoading={isMessagesLoading}
          messagesError={messagesError}
          selectedMessageRunId={selectedMessageRunId}
          onInspectMessage={handleInspectMessage}
          onBackToSelectedRun={handleBackToSelectedRun}
          showDeepLinkedHint={Boolean(focusRunId && !selectedLog && fallbackSummary)}
        />
      </div>
    </div>
  );
}
