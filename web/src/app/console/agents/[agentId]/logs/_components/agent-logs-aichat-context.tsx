'use client';

import { useMemo } from 'react';
import {
  sanitizeAIChatContextText,
  usePageContextRegistration,
  type AIChatCapabilityDescriptor,
  type AIChatContextRelation,
  type AIChatPageContextItem,
} from '@/components/aichat/page-context';
import { AGENT_KEYS, WORKFLOW_KEYS } from '@/hooks/query-keys';
import type { WorkflowFinishedData } from '@/components/workflow/ui/workflow-run-panel/types';
import type { WorkflowRunNodeListItem } from '@/components/workflow/ui/workflow-run-nodes-list';
import type { AgentDetail } from '@/services/types/agent';
import type {
  AgentRuntimeRunDetail,
  AgentRuntimeRunItem,
  AgentRuntimeStep,
} from '@/services/types/agent-runtime-log';
import type { WorkflowRunDetail, WorkflowRunItem } from '@/services/types/workflow';

const DESCRIPTION_MAX_LENGTH = 700;
const METADATA_TEXT_MAX_LENGTH = 220;
const SUMMARY_LIST_LIMIT = 8;

type LogRunListItem = WorkflowRunItem | AgentRuntimeRunItem;

interface AgentLogsAIChatContextRegistrationProps {
  agent: AgentDetail;
  isAgentRuntime: boolean;
  runtimeLogSource: string;
  searchFilter: string;
  conversationFilter: string;
  displayRunItems: LogRunListItem[];
  effectiveRunId: string | null;
  selectedWorkflowRun: WorkflowRunItem | null;
  workflowDetail: WorkflowRunDetail | null;
  workflowSummary: WorkflowFinishedData | null;
  workflowExecutionItems: WorkflowRunNodeListItem[];
  selectedAgentRuntimeRun: AgentRuntimeRunItem | null;
  agentRuntimeDetail: AgentRuntimeRunDetail | null;
  agentRuntimeSteps: AgentRuntimeStep[];
  selectedMessageRunId: string | null;
  publishedWorkflowId?: string | null;
}

function compactText(value: unknown, maxLength = DESCRIPTION_MAX_LENGTH): string {
  if (value === null || value === undefined) return '';

  const text =
    typeof value === 'string'
      ? value
      : (() => {
          try {
            return JSON.stringify(value);
          } catch {
            return String(value);
          }
        })();
  const normalized = sanitizeAIChatContextText(text).replace(/\s+/g, ' ').trim();
  if (normalized.length <= maxLength) return normalized;
  return `${normalized.slice(0, maxLength).trim()}...`;
}

function compactMetadataText(value: unknown): string | undefined {
  const text = compactText(value, METADATA_TEXT_MAX_LENGTH);
  return text || undefined;
}

function stringValue(value: unknown): string | undefined {
  if (typeof value === 'string' && value.trim()) return value.trim();
  if (typeof value === 'number' && Number.isFinite(value)) return String(value);
  return undefined;
}

function isFailureStatus(status?: string | null): boolean {
  const normalized = status?.toLowerCase() ?? '';
  return (
    normalized.includes('fail') ||
    normalized.includes('error') ||
    normalized.includes('exception') ||
    normalized.includes('timeout')
  );
}

function summarizeValues(values: Array<string | null | undefined>, limit = SUMMARY_LIST_LIMIT) {
  const unique = Array.from(
    new Set(values.map(value => value?.trim()).filter((value): value is string => Boolean(value)))
  );
  if (unique.length === 0) return undefined;

  const visible = unique.slice(0, limit).join(', ');
  const omitted = unique.length > limit ? `, +${unique.length - limit} more` : '';
  return compactMetadataText(`${visible}${omitted}`);
}

function summarizeStatuses(items: LogRunListItem[]) {
  const counts = new Map<string, number>();
  items.forEach(item => {
    const status = stringValue(item.status) ?? 'unknown';
    counts.set(status, (counts.get(status) ?? 0) + 1);
  });

  return Array.from(counts.entries())
    .sort((left, right) => right[1] - left[1])
    .slice(0, SUMMARY_LIST_LIMIT)
    .map(([status, count]) => `${status}:${count}`)
    .join(', ');
}

function summarizeSelectedRunDescription(parts: Array<string | null | undefined>) {
  const text = parts
    .map(part => compactText(part, 260))
    .filter(Boolean)
    .join(' | ');

  return compactText(text || 'Selected run context is available on the Agent logs page.');
}

function selectedRunMetric(label: string, value: unknown): string | null {
  const text = stringValue(value);
  return text ? `${label}: ${text}` : null;
}

function buildSelectedRunMetricSummary({
  status,
  elapsedTime,
  totalTokens,
  totalSteps,
  loadedCount,
  failedCount,
  loadedLabel,
  failedLabel,
}: {
  status: string;
  elapsedTime?: string | number | null;
  totalTokens?: string | number | null;
  totalSteps?: string | number | null;
  loadedCount: number;
  failedCount: number;
  loadedLabel: string;
  failedLabel: string;
}): string[] {
  return [
    selectedRunMetric('Status', status),
    selectedRunMetric('Elapsed time', elapsedTime),
    selectedRunMetric('Total tokens', totalTokens),
    selectedRunMetric('Total steps', totalSteps),
    selectedRunMetric(loadedLabel, loadedCount),
    selectedRunMetric(failedLabel, failedCount),
  ].filter((part): part is string => Boolean(part));
}

function buildPageCapabilities(hasSelectedRun: boolean): AIChatCapabilityDescriptor[] {
  return [
    {
      id: 'inspect_agent_logs_context',
      title: 'Inspect Agent logs context',
      description: 'Read the current Agent logs page context, filters, and selected run metadata.',
      risk: 'low',
      status: 'available',
    },
    {
      id: 'summarize_loaded_agent_runs',
      title: 'Summarize loaded runs',
      description: 'Summarize the loaded run list for author debugging and trend spotting.',
      risk: 'low',
      status: 'available',
    },
    {
      id: 'summarize_selected_run',
      title: 'Summarize selected run',
      description:
        'Summarize the current run, including status, timing, tokens, and notable steps.',
      risk: 'low',
      status: hasSelectedRun ? 'available' : 'unavailable',
      metadata: {
        has_selected_run: hasSelectedRun,
      },
    },
  ];
}

function buildSelectedRunCapabilities(
  hasSelectedRun: boolean,
  hasFailureSignal: boolean
): AIChatCapabilityDescriptor[] {
  return [
    {
      id: 'explain_selected_run_failure',
      title: 'Explain selected failure',
      description:
        'Explain the likely failure cause using the selected run status, error text, and failed step or node summaries.',
      risk: 'low',
      status: hasSelectedRun && hasFailureSignal ? 'available' : 'unavailable',
      metadata: {
        has_selected_run: hasSelectedRun,
        has_failure_signal: hasFailureSignal,
      },
    },
    {
      id: 'generate_selected_run_retrospective',
      title: 'Generate run retrospective',
      description:
        'Create a concise post-run retrospective covering what happened, impact, and next debugging checks.',
      risk: 'low',
      status: hasSelectedRun ? 'available' : 'unavailable',
      metadata: {
        has_selected_run: hasSelectedRun,
      },
    },
    {
      id: 'draft_fix_recommendations_from_log',
      title: 'Draft fix recommendations',
      description:
        'Draft author-focused remediation suggestions grounded in the selected log and visible execution details.',
      risk: 'low',
      status: hasSelectedRun ? 'available' : 'unavailable',
      metadata: {
        has_selected_run: hasSelectedRun,
        uses_agent_memory: false,
      },
    },
  ];
}

function buildAgentRelation(agent: AgentDetail): AIChatContextRelation {
  return {
    type: 'belongs_to_agent',
    resourceType: 'agent',
    resourceId: agent.id,
    title: agent.name,
    metadata: {
      agent_id: agent.id,
      page: 'agent_logs',
    },
  };
}

function buildWorkflowRelation(workflowId?: string | null): AIChatContextRelation | null {
  if (!workflowId) return null;
  return {
    type: 'runs_workflow',
    resourceType: 'workflow',
    resourceId: workflowId,
    title: 'Published workflow',
    metadata: {
      workflow_id: workflowId,
    },
  };
}

function buildPageHints({
  agentId,
  isAgentRuntime,
  effectiveRunId,
}: {
  agentId: string;
  isAgentRuntime: boolean;
  effectiveRunId: string | null;
}): NonNullable<AIChatPageContextItem['hints']> {
  if (isAgentRuntime) {
    return {
      handledAssetTypes: ['agent_runtime_run'],
      refreshHints: [
        { assetType: 'agent_runtime_run', queryKey: [...AGENT_KEYS.detail(agentId), 'runtime-runs'] },
        ...(effectiveRunId
          ? [
              {
                assetType: 'agent_runtime_run',
                resourceId: effectiveRunId,
                queryKey: AGENT_KEYS.runtimeRunDetail(agentId, effectiveRunId),
              },
              {
                assetType: 'agent_runtime_run',
                resourceId: effectiveRunId,
                queryKey: AGENT_KEYS.runtimeRunSteps(agentId, effectiveRunId),
              },
            ]
          : []),
      ],
    };
  }

  return {
    handledAssetTypes: ['workflow_run'],
    refreshHints: [
      { assetType: 'workflow_run', queryKey: WORKFLOW_KEYS.runs(agentId) },
      { assetType: 'workflow_run', queryKey: WORKFLOW_KEYS.runDetails() },
      { assetType: 'workflow_run', queryKey: WORKFLOW_KEYS.executions() },
    ],
  };
}

function buildAgentRuntimeSelectedRunItem({
  agent,
  effectiveRunId,
  selectedRun,
  detail,
  steps,
}: {
  agent: AgentDetail;
  effectiveRunId: string;
  selectedRun: AgentRuntimeRunItem | null;
  detail: AgentRuntimeRunDetail | null;
  steps: AgentRuntimeStep[];
}): AIChatPageContextItem {
  const status = detail?.status ?? selectedRun?.status ?? 'unknown';
  const failedSteps = steps.filter(step => isFailureStatus(step.status) || Boolean(step.error));
  const firstFailedStep = failedSteps[0] ?? null;
  const error = detail?.error ?? selectedRun?.error ?? firstFailedStep?.error;
  const hasFailureSignal = isFailureStatus(status) || Boolean(error) || failedSteps.length > 0;
  const query = detail?.query ?? selectedRun?.query;
  const answer = detail?.answer ?? selectedRun?.answer_preview;
  const elapsedTime = detail?.elapsed_time ?? selectedRun?.elapsed_time;
  const totalTokens = detail?.total_tokens ?? selectedRun?.total_tokens;
  const totalSteps = detail?.total_steps ?? selectedRun?.total_steps ?? steps.length;

  return {
    id: effectiveRunId,
    type: 'log',
    title: 'Selected Agent runtime run',
    subtitle: `${status} runtime run`,
    description: summarizeSelectedRunDescription([
      ...buildSelectedRunMetricSummary({
        status,
        elapsedTime,
        totalTokens,
        totalSteps,
        loadedCount: steps.length,
        failedCount: failedSteps.length,
        loadedLabel: 'Loaded steps',
        failedLabel: 'Failed steps',
      }),
      query ? `Query: ${query}` : null,
      error ? `Error: ${compactText(error, 320)}` : null,
      answer ? `Answer: ${answer}` : null,
    ]),
    href: `/console/agents/${agent.id}/logs?runId=${effectiveRunId}`,
    source: 'Agent Logs',
    status,
    risk: hasFailureSignal ? 'medium' : 'low',
    relations: [buildAgentRelation(agent)],
    capabilities: buildSelectedRunCapabilities(true, hasFailureSignal),
    metadata: {
      agent_id: agent.id,
      page: 'agent_logs',
      run_id: effectiveRunId,
      run_kind: 'agent_runtime',
      conversation_id: detail?.conversation_id ?? selectedRun?.conversation_id,
      source: detail?.source ?? selectedRun?.source,
      source_web_app_id: detail?.source_web_app_id ?? selectedRun?.source_web_app_id,
      model_provider: detail?.model_provider ?? selectedRun?.model_provider,
      model_name: detail?.model_name ?? selectedRun?.model_name,
      elapsed_time: elapsedTime,
      total_tokens: totalTokens,
      total_steps: totalSteps,
      loaded_step_count: steps.length,
      failed_step_count: failedSteps.length,
      step_types: summarizeValues(steps.map(step => step.type)),
      first_failed_step: firstFailedStep?.title,
      first_failed_step_type: firstFailedStep?.type,
      error: compactMetadataText(error),
      ai_chat_memory_shared_with_agent: false,
    },
  };
}

function buildWorkflowSelectedRunItem({
  agent,
  effectiveRunId,
  selectedRun,
  detail,
  summary,
  executionItems,
  publishedWorkflowId,
  selectedMessageRunId,
}: {
  agent: AgentDetail;
  effectiveRunId: string;
  selectedRun: WorkflowRunItem | null;
  detail: WorkflowRunDetail | null;
  summary: WorkflowFinishedData | null;
  executionItems: WorkflowRunNodeListItem[];
  publishedWorkflowId?: string | null;
  selectedMessageRunId: string | null;
}): AIChatPageContextItem {
  const status = summary?.status ?? detail?.status ?? selectedRun?.status ?? 'unknown';
  const failedNodes = executionItems.filter(
    item => isFailureStatus(item.status) || Boolean(item.error)
  );
  const firstFailedNode = failedNodes[0] ?? null;
  const error = detail?.error ?? firstFailedNode?.error;
  const hasFailureSignal = isFailureStatus(status) || Boolean(error) || failedNodes.length > 0;
  const workflowRelation = buildWorkflowRelation(publishedWorkflowId);
  const relations = [buildAgentRelation(agent), ...(workflowRelation ? [workflowRelation] : [])];
  const elapsedTime = summary?.elapsed_time ?? detail?.elapsed_time ?? selectedRun?.elapsed_time;
  const totalTokens = summary?.total_tokens ?? detail?.total_tokens ?? selectedRun?.total_tokens;
  const totalSteps = summary?.total_steps ?? detail?.total_steps ?? selectedRun?.total_steps;

  return {
    id: effectiveRunId,
    type: 'log',
    title: 'Selected workflow run',
    subtitle: `${status} workflow run`,
    description: summarizeSelectedRunDescription([
      ...buildSelectedRunMetricSummary({
        status,
        elapsedTime,
        totalTokens,
        totalSteps,
        loadedCount: executionItems.length,
        failedCount: failedNodes.length,
        loadedLabel: 'Loaded node executions',
        failedLabel: 'Failed node executions',
      }),
      error ? `Error: ${compactText(error, 320)}` : null,
      firstFailedNode
        ? `First failed node: ${firstFailedNode.title || firstFailedNode.nodeId}`
        : null,
      executionItems.length > 0 ? `Loaded node executions: ${executionItems.length}` : null,
    ]),
    href: `/console/agents/${agent.id}/logs?runId=${effectiveRunId}`,
    source: 'Agent Logs',
    status,
    risk: hasFailureSignal ? 'medium' : 'low',
    relations,
    capabilities: buildSelectedRunCapabilities(true, hasFailureSignal),
    metadata: {
      agent_id: agent.id,
      page: 'agent_logs',
      run_id: effectiveRunId,
      run_kind: 'workflow',
      workflow_id: detail?.workflow_id ?? publishedWorkflowId,
      conversation_id:
        summary?.conversation_id ?? detail?.conversation_id ?? selectedRun?.conversation_id,
      message_id: summary?.message_id ?? detail?.message_id ?? selectedRun?.message_id,
      selected_message_run_id: selectedMessageRunId,
      elapsed_time: elapsedTime,
      total_tokens: totalTokens,
      total_steps: totalSteps,
      loaded_node_count: executionItems.length,
      failed_node_count: failedNodes.length,
      node_types: summarizeValues(executionItems.map(item => item.nodeType)),
      first_failed_node: firstFailedNode?.title || firstFailedNode?.nodeId,
      first_failed_node_type: firstFailedNode?.nodeType,
      error: compactMetadataText(error),
      ai_chat_memory_shared_with_agent: false,
    },
  };
}

function buildAgentLogsAIChatContextItems({
  agent,
  isAgentRuntime,
  runtimeLogSource,
  searchFilter,
  conversationFilter,
  displayRunItems,
  effectiveRunId,
  selectedWorkflowRun,
  workflowDetail,
  workflowSummary,
  workflowExecutionItems,
  selectedAgentRuntimeRun,
  agentRuntimeDetail,
  agentRuntimeSteps,
  selectedMessageRunId,
  publishedWorkflowId,
}: AgentLogsAIChatContextRegistrationProps): AIChatPageContextItem[] {
  const pageHref = `/console/agents/${agent.id}/logs`;
  const statusSummary = summarizeStatuses(displayRunItems);
  const failedRunCount = displayRunItems.filter(item => isFailureStatus(item.status)).length;
  const mode = isAgentRuntime ? 'agent_runtime' : 'workflow';
  const hasSelectedRun = Boolean(effectiveRunId);
  const pageHints = buildPageHints({ agentId: agent.id, isAgentRuntime, effectiveRunId });
  const selectedRunItem =
    effectiveRunId && isAgentRuntime
      ? buildAgentRuntimeSelectedRunItem({
          agent,
          effectiveRunId,
          selectedRun: selectedAgentRuntimeRun,
          detail: agentRuntimeDetail,
          steps: agentRuntimeSteps,
        })
      : effectiveRunId
        ? buildWorkflowSelectedRunItem({
            agent,
            effectiveRunId,
            selectedRun: selectedWorkflowRun,
            detail: workflowDetail,
            summary: workflowSummary,
            executionItems: workflowExecutionItems,
            publishedWorkflowId,
            selectedMessageRunId,
          })
        : null;

  return [
    {
      id: pageHref,
      type: 'page',
      title: 'Agent logs',
      subtitle: `${agent.name} author diagnostics`,
      description:
        'Agent author log page with high-detail run context for debugging, retrospectives, and fix planning. AIChat memory is separate from Agent memory.',
      href: pageHref,
      source: 'Console',
      risk: 'low',
      status: 'available',
      capabilities: buildPageCapabilities(hasSelectedRun),
      hints: pageHints,
      metadata: {
        agent_id: agent.id,
        page: 'agent_logs',
        log_mode: mode,
        runtime_log_source: isAgentRuntime ? runtimeLogSource : undefined,
        loaded_run_count: displayRunItems.length,
        failed_run_count: failedRunCount,
        selected_run_id: effectiveRunId,
        search_filter_active: Boolean(searchFilter),
        conversation_filter_active: Boolean(conversationFilter),
        ai_chat_memory_shared_with_agent: false,
      },
    },
    {
      id: agent.id,
      type: 'agent',
      title: agent.name,
      subtitle: `${agent.agent_type} logs`,
      description: compactText(agent.description || 'Agent log context for author debugging.'),
      href: pageHref,
      source: 'Agent Logs',
      risk: 'low',
      status: agent.is_published ? 'published' : 'draft',
      capabilities: buildPageCapabilities(hasSelectedRun),
      hints: {
        handledAssetTypes: ['agent'],
        refreshHints: [
          { assetType: 'agent', queryKey: AGENT_KEYS.detail(agent.id) },
          { assetType: 'agent', queryKey: AGENT_KEYS.config(agent.id) },
        ],
      },
      metadata: {
        agent_id: agent.id,
        agent_type: agent.agent_type,
        is_published: Boolean(agent.is_published),
        web_app_status: agent.web_app_status,
        can_edit_agent: agent.can_edit,
        page: 'agent_logs',
        selected_run_id: effectiveRunId,
      },
    },
    {
      id: `${agent.id}:loaded-agent-logs`,
      type: 'log',
      title: 'Loaded Agent log list',
      subtitle: `${displayRunItems.length} runs loaded`,
      description: compactText(
        [
          statusSummary ? `Statuses: ${statusSummary}` : null,
          searchFilter ? `Search filter: ${searchFilter}` : null,
          conversationFilter ? `Conversation filter: ${conversationFilter}` : null,
        ]
          .filter(Boolean)
          .join(' | ') || 'No runs are currently loaded in the list.'
      ),
      href: pageHref,
      source: 'Agent Logs',
      risk: failedRunCount > 0 ? 'medium' : 'low',
      status: failedRunCount > 0 ? 'error' : 'available',
      relations: [buildAgentRelation(agent)],
      capabilities: [
        {
          id: 'summarize_loaded_agent_runs',
          title: 'Summarize loaded runs',
          description: 'Summarize the loaded log rows and identify notable failed or slow runs.',
          risk: 'low',
          status: 'available',
        },
      ],
      metadata: {
        agent_id: agent.id,
        page: 'agent_logs',
        log_mode: mode,
        loaded_run_count: displayRunItems.length,
        failed_run_count: failedRunCount,
        status_summary: compactMetadataText(statusSummary),
        recent_run_ids: summarizeValues(displayRunItems.map(item => item.id)),
        selected_run_id: effectiveRunId,
      },
    },
    ...(selectedRunItem ? [selectedRunItem] : []),
  ];
}

export function AgentLogsAIChatContextRegistration(props: AgentLogsAIChatContextRegistrationProps) {
  const {
    agent,
    isAgentRuntime,
    runtimeLogSource,
    searchFilter,
    conversationFilter,
    displayRunItems,
    effectiveRunId,
    selectedWorkflowRun,
    workflowDetail,
    workflowSummary,
    workflowExecutionItems,
    selectedAgentRuntimeRun,
    agentRuntimeDetail,
    agentRuntimeSteps,
    selectedMessageRunId,
    publishedWorkflowId,
  } = props;

  const items = useMemo(
    () =>
      buildAgentLogsAIChatContextItems({
        agent,
        isAgentRuntime,
        runtimeLogSource,
        searchFilter,
        conversationFilter,
        displayRunItems,
        effectiveRunId,
        selectedWorkflowRun,
        workflowDetail,
        workflowSummary,
        workflowExecutionItems,
        selectedAgentRuntimeRun,
        agentRuntimeDetail,
        agentRuntimeSteps,
        selectedMessageRunId,
        publishedWorkflowId,
      }),
    [
      agent,
      agentRuntimeDetail,
      agentRuntimeSteps,
      conversationFilter,
      displayRunItems,
      effectiveRunId,
      isAgentRuntime,
      publishedWorkflowId,
      runtimeLogSource,
      searchFilter,
      selectedAgentRuntimeRun,
      selectedMessageRunId,
      selectedWorkflowRun,
      workflowDetail,
      workflowExecutionItems,
      workflowSummary,
    ]
  );

  usePageContextRegistration(items, { scopeId: `agent-logs:${agent.id}` });

  return null;
}
