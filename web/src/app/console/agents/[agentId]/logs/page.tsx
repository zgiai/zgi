'use client';

import Link from 'next/link';
import { use, useEffect, useMemo, useState, type ReactNode } from 'react';
import { useRouter, useSearchParams } from 'next/navigation';
import { AlertCircle, Copy, ExternalLink, Filter, History, Loader2, Search, X } from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { TableCell, TableRow } from '@/components/ui/table';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { StickyDataTable } from '@/components/common/sticky-data-table';
import { useAgentRuntimeRunDetail } from '@/hooks/agent/use-agent-runtime-run-detail';
import { useAgentRuntimeRunSteps } from '@/hooks/agent/use-agent-runtime-run-steps';
import { useAgentRuntimeRunsInfinite } from '@/hooks/agent/use-agent-runtime-runs';
import { useAgent } from '@/hooks/agent/use-agents';
import { useLatestWorkflowVersion } from '@/hooks/workflow/use-workflow';
import { useWorkflowChatMessages } from '@/hooks/workflow/use-workflow-chat-messages';
import { useWorkflowRunDetail } from '@/hooks/workflow/use-workflow-run-detail';
import { useWorkflowRunNodeExecutions } from '@/hooks/workflow/use-workflow-run-node-executions';
import { useWorkflowRunsInfinite } from '@/hooks/workflow/use-workflow-runs';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n/translations';
import { AgentType } from '@/services/types/agent';
import type { AgentRuntimeRunItem } from '@/services/types/agent-runtime-log';
import type { WorkflowChatMessageItem, WorkflowRunItem } from '@/services/types/workflow';
import { formatDate, formatWorkflowElapsedMs } from '@/utils/format';
import {
  canShowAgentRuntimeLogs,
  getAgentDetailEditHref,
  getWebAppRunHref,
  supportsAgentRuntimeLogs,
} from '@/utils/agent-detail-routes';
import { getErrorMessage } from '@/utils/error-notifications';
import {
  buildWorkflowRunExecutionItems,
  buildWorkflowRunHistoryResult,
  buildWorkflowRunSummary,
} from '@/components/workflow/ui/workflow-run-panel/utils/history-view-data';
import { cn } from '@/lib/utils';
import { AgentRuntimeLogDetailDrawer } from './_components/agent-runtime-log-detail-drawer';
import { AgentLogsAIChatContextRegistration } from './_components/agent-logs-aichat-context';
import { LogDetailDrawer, type HistoryTab } from './_components/log-detail-drawer';
import { RunStatusBadge } from '@/components/workflow/ui/run-status-badge';
import { AGENT_PERMISSION_ACTIONS, WORKFLOW_PERMISSION_ACTIONS } from '@/constants/permissions';

interface AgentLogsPageProps {
  params: Promise<{ agentId: string }>;
}

type LogRunListItem = WorkflowRunItem | AgentRuntimeRunItem;
type AgentRuntimeLogSource = 'webapp' | 'console' | 'external-api';

const UUID_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

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

function isAgentRuntimeRunItem(item: LogRunListItem): item is AgentRuntimeRunItem {
  return 'query' in item;
}

function summarizeLogText(value?: string | null) {
  return value?.replace(/\s+/g, ' ').trim() ?? '';
}

function shortenID(value?: string | null) {
  if (!value) return '-';
  if (value.length <= 12) return value;
  return `${value.slice(0, 8)}...${value.slice(-4)}`;
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

function LogListState({
  icon,
  title,
  description,
  action,
  tone = 'default',
}: {
  icon: ReactNode;
  title: string;
  description: string;
  action?: ReactNode;
  tone?: 'default' | 'error';
}) {
  return (
    <div className="m-4 flex min-h-52 flex-col items-center justify-center rounded-2xl border border-dashed bg-muted/10 px-6 py-10 text-center">
      <div
        className={cn(
          'mb-4 flex size-11 items-center justify-center rounded-xl border',
          tone === 'error'
            ? 'border-rose-200/80 bg-rose-50 text-rose-600 dark:border-rose-500/25 dark:bg-rose-500/10 dark:text-rose-300'
            : 'border-border/70 bg-background text-muted-foreground shadow-sm'
        )}
      >
        {icon}
      </div>
      <div className="text-sm font-semibold text-foreground">{title}</div>
      <div className="mt-1.5 max-w-md text-sm leading-6 text-muted-foreground">
        {description}
      </div>
      {action ? <div className="mt-4">{action}</div> : null}
    </div>
  );
}

/**
 * @component AgentLogsPage
 * @category Feature
 * @status Stable
 * @description Displays webapp-originated runtime logs under the agent detail area.
 * @usage Use in the agent detail route alongside workflow and api pages.
 */
export default function AgentLogsPage({ params }: AgentLogsPageProps) {
  const t = useT('webapp');
  const tAgents = useT('agents');
  const tRoot = useT();
  const { agentId } = use(params);
  const router = useRouter();
  const searchParams = useSearchParams();
  const focusRunId = searchParams.get('runId');
  const focusTab = searchParams.get('tab');
  const focusConversationId = searchParams.get('conversation_id');

  const [selectedLogId, setSelectedLogId] = useState<string | null>(null);
  const [selectedMessageRunId, setSelectedMessageRunId] = useState<string | null>(null);
  const [isDetailOpen, setIsDetailOpen] = useState(false);
  const [activeTab, setActiveTab] = useState<HistoryTab>('execution');
  const [conversationFilterInput, setConversationFilterInput] = useState('');
  const [conversationFilter, setConversationFilter] = useState('');
  const [runtimeLogSource, setRuntimeLogSource] = useState<AgentRuntimeLogSource>('webapp');
  const [searchFilterInput, setSearchFilterInput] = useState('');
  const [searchFilter, setSearchFilter] = useState('');

  const { hasAnyPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canViewAgentRuntimeLogs = hasAnyPermission(AGENT_PERMISSION_ACTIONS.logsView);
  const canViewWorkflowRuntimeLogs = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.logsView);
  const canOpenRuntimeLogs = canViewAgentRuntimeLogs || canViewWorkflowRuntimeLogs;
  const {
    agent,
    isLoading: isAgentLoading,
    error: agentError,
  } = useAgent(agentId, canOpenRuntimeLogs);
  const agentDetail = agent?.data ?? null;
  const isPublished = agentDetail?.is_published === true;
  const supportsRuntimeLogs = supportsAgentRuntimeLogs(agentDetail?.agent_type);
  const isAgentRuntime = agentDetail?.agent_type === AgentType.AGENT;
  const canViewRuntimeLogs = isAgentRuntime ? canViewAgentRuntimeLogs : canViewWorkflowRuntimeLogs;
  const canAccessRuntimeLogs = canShowAgentRuntimeLogs(agentDetail?.agent_type, {
    canView: true,
    canViewRuntimeLogs,
  });
  const canQueryWorkflowLogs = canAccessRuntimeLogs && isPublished && !isAgentRuntime;
  const canQueryAgentRuntimeLogs = canAccessRuntimeLogs && isPublished && isAgentRuntime;
  const isConversationWorkflow = agentDetail?.agent_type === AgentType.CONVERSATIONAL_AGENT;
  const normalizedConversationFilter = conversationFilter.trim();
  const normalizedSearchFilter = searchFilter.trim();
  const agentRuntimeRunsQuery = useMemo(
    () => ({
      source: runtimeLogSource,
      ...(runtimeLogSource === 'webapp' ? { triggered_from: 'web-app' } : {}),
      ...(normalizedConversationFilter ? { conversation_id: normalizedConversationFilter } : {}),
      ...(normalizedSearchFilter ? { q: normalizedSearchFilter } : {}),
    }),
    [normalizedConversationFilter, normalizedSearchFilter, runtimeLogSource]
  );

  const { data: latest } = useLatestWorkflowVersion(
    canAccessRuntimeLogs && isPublished ? agentId : null
  );

  useEffect(() => {
    const nextConversationFilter = focusConversationId?.trim() ?? '';
    if (!nextConversationFilter || !UUID_PATTERN.test(nextConversationFilter)) {
      return;
    }
    setConversationFilterInput(nextConversationFilter);
    setConversationFilter(nextConversationFilter);
  }, [focusConversationId]);

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
      agentId: canQueryWorkflowLogs ? agentId : null,
      limit: 50,
      query: { triggered_from: 'web-app' },
    },
    {
      enabled: canQueryWorkflowLogs,
      staleTime: 30_000,
      refetchOnWindowFocus: false,
    }
  );

  const runItems = useMemo(() => pages.flat(), [pages]);
  const {
    pages: agentRuntimePages,
    fetchNextPage: fetchNextAgentRuntimePage,
    hasNextPage: hasNextAgentRuntimePage,
    isFetchingNextPage: isFetchingNextAgentRuntimePage,
    isLoading: isAgentRuntimeRunsLoading,
    error: agentRuntimeRunsError,
    reload: reloadAgentRuntimeRuns,
  } = useAgentRuntimeRunsInfinite(
    {
      agentId: canQueryAgentRuntimeLogs ? agentId : null,
      limit: 50,
      query: agentRuntimeRunsQuery,
    },
    {
      enabled: canQueryAgentRuntimeLogs,
      staleTime: 30_000,
      refetchOnWindowFocus: false,
    }
  );
  const agentRuntimeRunItems = useMemo(() => agentRuntimePages.flat(), [agentRuntimePages]);
  const displayRunItems = useMemo<LogRunListItem[]>(
    () => (isAgentRuntime ? agentRuntimeRunItems : runItems),
    [agentRuntimeRunItems, isAgentRuntime, runItems]
  );
  const selectedAgentRuntimeRun = useMemo(
    () => agentRuntimeRunItems.find(item => item.id === selectedLogId) ?? null,
    [agentRuntimeRunItems, selectedLogId]
  );
  const selectedLog = useMemo(
    () => runItems.find(item => item.id === selectedLogId) ?? null,
    [runItems, selectedLogId]
  );

  useEffect(() => {
    if (!selectedLogId || displayRunItems.some(item => item.id === selectedLogId)) return;
    setSelectedLogId(null);
    setSelectedMessageRunId(null);
    setIsDetailOpen(false);
    setActiveTab('execution');
  }, [displayRunItems, selectedLogId]);

  useEffect(() => {
    const listLoading = isAgentRuntime ? isAgentRuntimeRunsLoading : isRunsLoading;
    if (!focusRunId || listLoading) return;

    const inList = displayRunItems.some(item => item.id === focusRunId);
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
  }, [
    displayRunItems,
    focusRunId,
    focusTab,
    isAgentRuntime,
    isAgentRuntimeRunsLoading,
    isRunsLoading,
  ]);

  const effectiveRunId = selectedMessageRunId ?? selectedLogId;
  const isDetailDrawerOpen = isDetailOpen && Boolean(effectiveRunId);

  const {
    detail,
    isLoading: isDetailLoading,
    error: detailError,
  } = useWorkflowRunDetail(
    { agentId: canQueryWorkflowLogs ? agentId : null, runId: effectiveRunId },
    {
      enabled: Boolean(canQueryWorkflowLogs && isDetailOpen && effectiveRunId),
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
    { agentId: canQueryWorkflowLogs ? agentId : null, runId: effectiveRunId },
    {
      enabled: Boolean(canQueryWorkflowLogs && isDetailOpen && effectiveRunId),
      staleTime: 60_000,
      refetchOnWindowFocus: false,
    }
  );
  const {
    detail: agentRuntimeDetail,
    isLoading: isAgentRuntimeDetailLoading,
    error: agentRuntimeDetailLoadError,
  } = useAgentRuntimeRunDetail(
    { agentId: canQueryAgentRuntimeLogs ? agentId : null, messageId: effectiveRunId },
    {
      enabled: Boolean(canQueryAgentRuntimeLogs && isDetailOpen && effectiveRunId),
      staleTime: 60_000,
      refetchOnWindowFocus: false,
      suppressErrorToast: Boolean(focusRunId),
    }
  );
  const {
    steps: agentRuntimeSteps,
    isLoading: isAgentRuntimeStepsLoading,
    error: agentRuntimeStepsLoadError,
  } = useAgentRuntimeRunSteps(
    { agentId: canQueryAgentRuntimeLogs ? agentId : null, messageId: effectiveRunId },
    {
      enabled: Boolean(canQueryAgentRuntimeLogs && isDetailOpen && effectiveRunId),
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
      agentId: canQueryWorkflowLogs ? agentId : null,
      conversationId,
      page: 1,
      limit: 100,
    },
    {
      enabled: Boolean(
        canQueryWorkflowLogs && isConversationWorkflow && isDetailOpen && conversationId
      ),
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
  const effectiveDetailError = !detailLoading && !summary && fallbackSummary ? null : detailError;
  const agentRuntimeDetailLoading = isAgentRuntimeDetailLoading || isAgentRuntimeStepsLoading;
  const agentRuntimeDetailError = agentRuntimeDetailLoadError || agentRuntimeStepsLoadError;
  const webAppId = latest?.data?.web_app_id;
  const webAppHref =
    webAppId && agentDetail ? getWebAppRunHref(webAppId, agentDetail.agent_type) : null;
  const listError = isAgentRuntime ? agentRuntimeRunsError : runsError;
  const listLoading = isAgentRuntime ? isAgentRuntimeRunsLoading : isRunsLoading;
  const listHasNextPage = isAgentRuntime ? hasNextAgentRuntimePage : hasNextPage;
  const listIsFetchingNextPage = isAgentRuntime
    ? isFetchingNextAgentRuntimePage
    : isFetchingNextPage;
  const fetchNextListPage = isAgentRuntime ? fetchNextAgentRuntimePage : fetchNextPage;
  const reloadList = isAgentRuntime ? reloadAgentRuntimeRuns : reload;
  const hasActiveRuntimeFilters = Boolean(normalizedConversationFilter || normalizedSearchFilter);
  const hasPendingRuntimeFilterChanges =
    conversationFilterInput.trim() !== normalizedConversationFilter ||
    searchFilterInput.trim() !== normalizedSearchFilter;
  const logPageTitle = isAgentRuntime
    ? t('appLogs.agentTitle')
    : t('appLogs.workflowTitle');
  const logPageSubtitle = isAgentRuntime
    ? t('appLogs.agentSubtitle', { name: agentDetail?.name ?? '' })
    : t('appLogs.workflowSubtitle', { name: agentDetail?.name ?? '' });
  const emptyLogCopy = hasActiveRuntimeFilters
    ? {
        title: t('appLogs.empty.filteredTitle'),
        description: t('appLogs.empty.filteredDescription'),
      }
    : !isAgentRuntime
      ? {
          title: t('appLogs.empty.workflowTitle'),
          description: t('appLogs.empty.workflowDescription'),
        }
      : runtimeLogSource === 'console'
        ? {
            title: t('appLogs.empty.consoleTitle'),
            description: t('appLogs.empty.consoleDescription'),
          }
        : runtimeLogSource === 'external-api'
          ? {
              title: t('appLogs.empty.externalApiTitle'),
              description: t('appLogs.empty.externalApiDescription'),
            }
          : {
              title: t('appLogs.empty.agentTitle'),
              description: t('appLogs.empty.agentDescription'),
            };
  const logTableColumns = useMemo(() => {
    const columns = [
      { key: 'runId', header: t('appLogs.columns.runId'), className: 'pl-4' },
      { key: 'status', header: t('appLogs.columns.status') },
      { key: 'steps', header: tAgents('workflow.steps') },
      { key: 'elapsed', header: tAgents('workflow.elapsed') },
      { key: 'createdAt', header: t('appLogs.columns.createdAt') },
      { key: 'conversation', header: t('appLogs.columns.conversation') },
    ];
    if (!isAgentRuntime) return columns;
    return [
      columns[0],
      { key: 'query', header: t('appLogs.columns.query') },
      { key: 'answer', header: t('appLogs.columns.answer') },
      ...columns.slice(1),
    ];
  }, [isAgentRuntime, t, tAgents]);

  useEffect(() => {
    if (
      isAgentLoading ||
      isPermissionsLoading ||
      !agentDetail ||
      !canAccessRuntimeLogs ||
      isPublished
    ) {
      return;
    }
    router.replace(getAgentDetailEditHref(agentId, agentDetail.agent_type));
  }, [
    agentDetail,
    agentId,
    canAccessRuntimeLogs,
    isAgentLoading,
    isPermissionsLoading,
    isPublished,
    router,
  ]);

  const handleSelectLog = (item: WorkflowRunItem | AgentRuntimeRunItem) => {
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

  const resetRuntimeLogSelection = () => {
    setSelectedLogId(null);
    setSelectedMessageRunId(null);
    setIsDetailOpen(false);
    setActiveTab('execution');
  };

  const handleApplyRuntimeFilters = () => {
    const nextConversationFilter = conversationFilterInput.trim();
    const nextSearchFilter = searchFilterInput.trim();
    if (nextConversationFilter && !UUID_PATTERN.test(nextConversationFilter)) {
      toast.error(t('appLogs.filters.invalidConversationId'));
      return;
    }
    setConversationFilterInput(nextConversationFilter);
    setConversationFilter(nextConversationFilter);
    setSearchFilterInput(nextSearchFilter);
    setSearchFilter(nextSearchFilter);
    resetRuntimeLogSelection();
  };

  const handleClearRuntimeFilters = () => {
    setConversationFilterInput('');
    setConversationFilter('');
    setSearchFilterInput('');
    setSearchFilter('');
    resetRuntimeLogSelection();
  };

  const handleClearConversationFilter = () => {
    setConversationFilterInput('');
    setConversationFilter('');
    resetRuntimeLogSelection();
  };

  const handleClearSearchFilter = () => {
    setSearchFilterInput('');
    setSearchFilter('');
    resetRuntimeLogSelection();
  };

  const handleRuntimeSourceChange = (value: string) => {
    const nextSource: AgentRuntimeLogSource =
      value === 'console' || value === 'external-api' ? value : 'webapp';
    setRuntimeLogSource(nextSource);
    resetRuntimeLogSelection();
  };

  const handleFilterByConversation = (conversationId?: string | null) => {
    if (!conversationId) return;
    setConversationFilterInput(conversationId);
    setConversationFilter(conversationId);
    resetRuntimeLogSelection();
  };

  const handleCopyConversation = async (conversationId?: string | null) => {
    if (!conversationId || typeof navigator === 'undefined') return;
    try {
      await navigator.clipboard.writeText(conversationId);
      toast.success(t('appLogs.filters.copiedConversationId'));
    } catch {
      toast.error(t('appLogs.filters.copyConversationFailed'));
    }
  };

  const handleDetailOpenChange = (open: boolean) => {
    setIsDetailOpen(open);
    if (open) return;
    setSelectedMessageRunId(null);
    setActiveTab('execution');
  };

  if (isPermissionsLoading || (canOpenRuntimeLogs && isAgentLoading)) {
    return (
      <div className="h-full w-full p-6">
        <div className="space-y-4">
          <Skeleton className="h-8 w-48" />
          <LogTableSkeleton />
        </div>
      </div>
    );
  }

  if (!canOpenRuntimeLogs) {
    return (
      <div className="flex h-full w-full items-center justify-center p-6">
        <div className="max-w-xl rounded-2xl border border-dashed bg-background p-8 text-center">
          <div className="mx-auto mb-4 flex size-12 items-center justify-center rounded-full bg-muted">
            <AlertCircle className="size-5 text-muted-foreground" />
          </div>
          <div className="text-lg font-semibold">{tRoot('common.accessDenied')}</div>
          <div className="mt-2 text-sm text-muted-foreground">
            {tRoot('common.unauthorizedDescription')}
          </div>
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

  if (!supportsRuntimeLogs || !canAccessRuntimeLogs) {
    return (
      <div className="flex h-full w-full items-center justify-center p-6">
        <div className="max-w-xl rounded-2xl border border-dashed bg-background p-8 text-center">
          <div className="mx-auto mb-4 flex size-12 items-center justify-center rounded-full bg-muted">
            <AlertCircle className="size-5 text-muted-foreground" />
          </div>
          <div className="text-lg font-semibold">
            {supportsRuntimeLogs ? tRoot('common.accessDenied') : t('appCenter.appUnavailableTitle')}
          </div>
          <div className="mt-2 text-sm text-muted-foreground">
            {supportsRuntimeLogs
              ? tRoot('common.unauthorizedDescription')
              : t('appCenter.appUnavailableDescription')}
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
      <AgentLogsAIChatContextRegistration
        agent={agentDetail}
        isAgentRuntime={isAgentRuntime}
        runtimeLogSource={runtimeLogSource}
        searchFilter={normalizedSearchFilter}
        conversationFilter={normalizedConversationFilter}
        displayRunItems={displayRunItems}
        effectiveRunId={effectiveRunId}
        selectedWorkflowRun={selectedLog}
        workflowDetail={detail}
        workflowSummary={effectiveSummary}
        workflowExecutionItems={executionItems}
        selectedAgentRuntimeRun={selectedAgentRuntimeRun}
        agentRuntimeDetail={agentRuntimeDetail}
        agentRuntimeSteps={agentRuntimeSteps}
        selectedMessageRunId={selectedMessageRunId}
        publishedWorkflowId={latest?.data?.workflow_id}
      />
      <div className="border-b px-6 py-4">
        <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div className="min-w-0">
            <div className="flex items-center gap-2 text-lg font-semibold">
              <History className="size-5 text-primary" />
              <span>{logPageTitle}</span>
            </div>
            <div className="mt-1 text-sm text-muted-foreground">{logPageSubtitle}</div>
          </div>
          {webAppHref ? (
            <Button asChild variant="outline" size="sm" className="w-fit">
              <Link href={webAppHref} target="_blank" rel="noreferrer">
                <ExternalLink className="size-4" />
                {isAgentRuntime ? t('appLogs.openAgent') : t('appLogs.openWorkflowApp')}
              </Link>
            </Button>
          ) : null}
        </div>
      </div>

      <div className="h-0 min-h-0 flex-1 overflow-hidden p-4">
        <section className="flex h-full min-h-0 flex-col overflow-hidden rounded-xl border bg-background">
          <div className="flex shrink-0 flex-col gap-3 border-b px-4 py-3">
            <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div className="min-w-0">
                <div className="text-sm font-semibold">{t('appLogs.recentRuns')}</div>
                <div className="text-xs text-muted-foreground">
                  {t('appLogs.selectRunDescription')}
                </div>
              </div>
              {isAgentRuntime ? (
                <Tabs value={runtimeLogSource} onValueChange={handleRuntimeSourceChange}>
                  <TabsList className="h-8">
                    <TabsTrigger value="webapp" className="h-6 text-xs">
                      {t('appLogs.filters.sources.webapp')}
                    </TabsTrigger>
                    <TabsTrigger value="console" className="h-6 text-xs">
                      {t('appLogs.filters.sources.console')}
                    </TabsTrigger>
                    <TabsTrigger value="external-api" className="h-6 text-xs">
                      {t('appLogs.filters.sources.externalApi')}
                    </TabsTrigger>
                  </TabsList>
                </Tabs>
              ) : null}
            </div>

            {isAgentRuntime ? (
              <div className="flex flex-col gap-2">
                <div className="flex w-full flex-col gap-2 xl:flex-row xl:items-center">
                  <Input
                    value={searchFilterInput}
                    onChange={event => setSearchFilterInput(event.target.value)}
                    onKeyDown={event => {
                      if (event.key !== 'Enter') return;
                      event.preventDefault();
                      handleApplyRuntimeFilters();
                    }}
                    placeholder={t('appLogs.filters.searchPlaceholder')}
                    leftIcon={<Search />}
                    className="h-8"
                    containerClassName="min-w-0 xl:w-[360px]"
                  />
                  <Input
                    value={conversationFilterInput}
                    onChange={event => setConversationFilterInput(event.target.value)}
                    onKeyDown={event => {
                      if (event.key !== 'Enter') return;
                      event.preventDefault();
                      handleApplyRuntimeFilters();
                    }}
                    placeholder={t('appLogs.filters.conversationPlaceholder')}
                    leftIcon={<Filter />}
                    className="h-8"
                    containerClassName="min-w-0 xl:w-[360px]"
                  />
                  <div className="flex gap-2">
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      disabled={!hasPendingRuntimeFilterChanges}
                      onClick={handleApplyRuntimeFilters}
                    >
                      {t('appLogs.filters.apply')}
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      disabled={!hasActiveRuntimeFilters && !hasPendingRuntimeFilterChanges}
                      onClick={handleClearRuntimeFilters}
                    >
                      {t('appLogs.filters.clear')}
                    </Button>
                  </div>
                </div>
                {hasActiveRuntimeFilters ? (
                  <div className="flex flex-wrap gap-2">
                    {normalizedSearchFilter ? (
                      <Badge variant="subtle" className="gap-1">
                        <span>
                          {t('appLogs.filters.searchChip', { keyword: normalizedSearchFilter })}
                        </span>
                        <button
                          type="button"
                          className="rounded-full p-0.5 hover:bg-background"
                          onClick={handleClearSearchFilter}
                          aria-label={t('appLogs.filters.clearSearch')}
                        >
                          <X className="size-3" />
                        </button>
                      </Badge>
                    ) : null}
                    {normalizedConversationFilter ? (
                      <Badge variant="subtle" className="gap-1">
                        <span>
                          {t('appLogs.filters.conversationChip', {
                            conversationId: shortenID(normalizedConversationFilter),
                          })}
                        </span>
                        <button
                          type="button"
                          className="rounded-full p-0.5 hover:bg-background"
                          onClick={handleClearConversationFilter}
                          aria-label={t('appLogs.filters.clearConversation')}
                        >
                          <X className="size-3" />
                        </button>
                      </Badge>
                    ) : null}
                  </div>
                ) : null}
              </div>
            ) : null}
          </div>

          <div className="min-h-0 flex-1 overflow-hidden">
            {listError ? (
              <LogListState
                tone="error"
                icon={<AlertCircle className="size-5" />}
                title={t('appLogs.loadErrorTitle')}
                description={t('appLogs.loadErrorDescription')}
                action={
                  <Button size="sm" variant="outline" onClick={() => void reloadList()}>
                    {tAgents('workflow.retry')}
                  </Button>
                }
              />
            ) : listLoading && displayRunItems.length === 0 ? (
              <LogTableSkeleton />
            ) : displayRunItems.length === 0 ? (
              <LogListState
                icon={
                  hasActiveRuntimeFilters ? (
                    <Search className="size-5" />
                  ) : (
                    <History className="size-5" />
                  )
                }
                title={emptyLogCopy.title}
                description={emptyLogCopy.description}
                action={
                  hasActiveRuntimeFilters ? (
                    <Button size="sm" variant="outline" onClick={handleClearRuntimeFilters}>
                      {t('appLogs.empty.clearFilters')}
                    </Button>
                  ) : undefined
                }
              />
            ) : (
              <StickyDataTable<LogRunListItem>
                className="h-full"
                columns={logTableColumns}
                data={displayRunItems}
                getRowKey={item => item.id}
                pagination={
                  listHasNextPage ? (
                    <div className="flex shrink-0 items-center justify-center border-t px-4 py-3">
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        disabled={listIsFetchingNextPage}
                        onClick={() => void fetchNextListPage()}
                      >
                        {listIsFetchingNextPage ? (
                          <Loader2 className="size-4 animate-spin" />
                        ) : null}
                        {listIsFetchingNextPage ? t('appLogs.loadingMore') : t('appLogs.loadMore')}
                      </Button>
                    </div>
                  ) : null
                }
              >
                {displayRunItems.map(item => {
                  const isSelected = item.id === selectedLogId;
                  const querySummary = isAgentRuntimeRunItem(item)
                    ? summarizeLogText(item.query)
                    : '';
                  const answerSummary = isAgentRuntimeRunItem(item)
                    ? summarizeLogText(item.answer_preview)
                    : '';
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
                      {isAgentRuntime ? (
                        <>
                          <TableCell className="max-w-[300px] py-4">
                            <div
                              className="line-clamp-2 text-xs leading-5"
                              title={querySummary || t('appLogs.noQuery')}
                            >
                              {querySummary || t('appLogs.noQuery')}
                            </div>
                          </TableCell>
                          <TableCell className="max-w-[340px] py-4">
                            <div
                              className="line-clamp-2 text-xs leading-5 text-muted-foreground"
                              title={answerSummary || t('appLogs.noAnswerYet')}
                            >
                              {answerSummary || t('appLogs.noAnswerYet')}
                            </div>
                          </TableCell>
                        </>
                      ) : null}
                      <TableCell className="py-4">
                        <RunStatusBadge status={item.status} />
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
                        <div className="flex min-w-0 items-center gap-1">
                          <div className="min-w-0 truncate" title={item.conversation_id ?? '-'}>
                            {shortenID(item.conversation_id)}
                          </div>
                          {isAgentRuntime && item.conversation_id ? (
                            <div className="flex opacity-0 transition-opacity group-hover:opacity-100">
                              <Button
                                type="button"
                                size="xs"
                                variant="ghost"
                                isIcon
                                aria-label={t('appLogs.filters.copyConversation')}
                                title={t('appLogs.filters.copyConversation')}
                                onClick={event => {
                                  event.stopPropagation();
                                  void handleCopyConversation(item.conversation_id);
                                }}
                              >
                                <Copy className="size-3.5" />
                              </Button>
                              <Button
                                type="button"
                                size="xs"
                                variant="ghost"
                                isIcon
                                aria-label={t('appLogs.filters.filterConversation')}
                                title={t('appLogs.filters.filterConversation')}
                                onClick={event => {
                                  event.stopPropagation();
                                  handleFilterByConversation(item.conversation_id);
                                }}
                              >
                                <Filter className="size-3.5" />
                              </Button>
                            </div>
                          ) : null}
                        </div>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </StickyDataTable>
            )}
          </div>
        </section>

        {isAgentRuntime ? (
          <AgentRuntimeLogDetailDrawer
            open={isDetailDrawerOpen}
            onOpenChange={handleDetailOpenChange}
            selectedRun={selectedAgentRuntimeRun}
            detail={agentRuntimeDetail}
            steps={agentRuntimeSteps}
            isLoading={agentRuntimeDetailLoading}
            error={agentRuntimeDetailError}
          />
        ) : (
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
        )}
      </div>
    </div>
  );
}
