'use client';

import { useMemo } from 'react';
import {
  sanitizeAIChatContextText,
  usePageContextRegistration,
  type AIChatCapabilityDescriptor,
  type AIChatPageContextItem,
} from '@/components/aichat/page-context';
import { AGENT_KEYS } from '@/hooks/query-keys';
import { AgentType, type Agent } from '@/services/types/agent';
import { getAgentDetailEditHref } from '@/utils/agent-detail-routes';

const AGENTS_CONTEXT_VISIBLE_LIMIT = 20;
const CONTEXT_FIELD_MAX_LENGTH = 900;

interface AgentsAIChatContextRegistrationProps {
  agents: Agent[];
  pageSize: number;
  searchKeyword: string;
  pageTitle?: string;
  workspaceId?: string;
  workspaceName?: string;
  canView: boolean;
  canManage: boolean;
  isLoading: boolean;
  isFetching: boolean;
  permissionsSettled?: boolean;
  hasNextPage: boolean;
}

function compactContextField(value: string, maxLength = CONTEXT_FIELD_MAX_LENGTH): string {
  const text = sanitizeAIChatContextText(value).replace(/\s+/g, ' ').trim();
  if (text.length <= maxLength) return text;
  return `${text.slice(0, maxLength).trim()}...`;
}

function agentTypeLabel(type: AgentType): string {
  switch (type) {
    case AgentType.AGENT:
      return 'agent';
    case AgentType.WORKFLOW:
      return 'workflow';
    case AgentType.CONVERSATIONAL_AGENT:
      return 'conversational workflow';
    default:
      return String(type).toLowerCase();
  }
}

function isAgentRuntimeItem(agent: Agent) {
  return agent.agent_type === AgentType.AGENT;
}

function contextItemTypeForAgent(agent: Agent): AIChatPageContextItem['type'] {
  return isAgentRuntimeItem(agent) ? 'agent' : 'workflow';
}

function publishedStatus(agent: Agent) {
  if (agent.is_published && agent.web_app_status === 'inactive') return 'published_offline';
  if (agent.is_published) return 'published';
  return 'draft';
}

function buildAgentListDescription(agents: Agent[], isLoading: boolean, hasNextPage: boolean) {
  if (isLoading) {
    return 'The Agents page is loading. Wait for the visible Agent list before answering list-specific questions.';
  }
  if (agents.length === 0) {
    return 'No Agents are visible with the current filters.';
  }

  const visibleSummary = agents
    .slice(0, AGENTS_CONTEXT_VISIBLE_LIMIT)
    .map((agent, index) =>
      [
        `visible_index=${index + 1}`,
        `name=${agent.name}`,
        `type=${agentTypeLabel(agent.agent_type)}`,
        `status=${publishedStatus(agent)}`,
      ].join(', ')
    )
    .join(' | ');
  const pagination = hasNextPage
    ? 'More Agents may exist beyond the currently loaded list.'
    : 'The currently loaded list has no next page.';
  const firstRuntimeAgentIndex = agents.findIndex(isAgentRuntimeItem);
  const firstRuntimeAgent = firstRuntimeAgentIndex >= 0 ? agents[firstRuntimeAgentIndex] : null;
  const runtimeAgentHint = firstRuntimeAgent
    ? [
        'For requests like "open the first Agent configuration", prefer the first visible item where type=agent, not workflow.',
        `first_visible_type_agent_index=${firstRuntimeAgentIndex + 1}`,
        `first_visible_type_agent_name=${firstRuntimeAgent.name}`,
        `first_visible_type_agent_href=${getAgentDetailEditHref(
          firstRuntimeAgent.id,
          firstRuntimeAgent.agent_type
        )}`,
      ].join(', ')
    : 'No visible item with type=agent is currently loaded; workflow rows are workflow assets, not Agent Runtime editor pages.';

  return compactContextField(
    `${pagination} ${runtimeAgentHint}. Visible resource index: ${visibleSummary}`
  );
}

function buildAgentListCapabilities(
  canView: boolean,
  canManage: boolean
): AIChatCapabilityDescriptor[] {
  return [
    {
      id: 'agent.list_visible',
      title: 'List visible Agents',
      description: 'Answer questions about Agents currently visible on the Agents page.',
      risk: 'low',
      status: canView ? 'available' : 'unavailable',
      permissions: ['agent.view'],
    },
    {
      id: 'agent.open_visible',
      title: 'Open visible Agent',
      description:
        'Navigate to a visible Agent detail page when the user asks to inspect or configure it.',
      risk: 'low',
      status: canView ? 'available' : 'unavailable',
      permissions: ['agent.view'],
    },
    {
      id: 'agent.create_from_page',
      title: 'Create Agent from page',
      description: 'Create a draft Agent in the current workspace through governed AIChat tools.',
      risk: 'medium',
      requiresConfirmation: true,
      status: canManage ? 'available' : 'disabled',
      permissions: ['agent.manage'],
      metadata: {
        supported_by_aichat_tool: true,
        tool_skill_id: 'agent-management',
        tool_name: 'create_agent',
      },
    },
    {
      id: 'agent.update_identity',
      title: 'Edit visible Agent',
      description: 'Update a visible Agent name, description, or icon through governed AIChat tools.',
      risk: 'medium',
      requiresConfirmation: true,
      status: canManage ? 'available' : 'disabled',
      permissions: ['agent.manage'],
      metadata: {
        supported_by_aichat_tool: true,
        tool_skill_id: 'agent-management',
        tool_name: 'update_agent_identity',
      },
    },
    {
      id: 'agent.delete_visible',
      title: 'Delete visible Agent',
      description: 'Delete a visible Agent through governed AIChat tools. Deletion always asks first.',
      risk: 'high',
      requiresConfirmation: true,
      status: canManage ? 'available' : 'disabled',
      permissions: ['agent.manage'],
      metadata: {
        supported_by_aichat_tool: true,
        tool_skill_id: 'agent-management',
        tool_name: 'delete_agent',
      },
    },
  ];
}

function buildVisibleAgentMetadata(agent: Agent, visibleIndex: number) {
  const href = getAgentDetailEditHref(agent.id, agent.agent_type);
  return {
    page: 'console.agents',
    resource_kind: 'agent',
    agent_id: agent.id,
    href,
    visible_index: visibleIndex,
    visible_ordinal: visibleIndex,
    visible_rank: visibleIndex,
    display_name: agent.name,
    name: agent.name,
    agent_type: agent.agent_type,
    agent_type_label: agentTypeLabel(agent.agent_type),
    is_published: agent.is_published,
    web_app_status: agent.web_app_status,
    can_edit: agent.can_edit,
    created_at: agent.created_at,
    updated_at: agent.updated_at,
  };
}

function buildAgentsAIChatContextItems({
  agents,
  pageSize,
  searchKeyword,
  pageTitle,
  workspaceId,
  workspaceName,
  canView,
  canManage,
  isLoading,
  isFetching,
  permissionsSettled = true,
  hasNextPage,
}: AgentsAIChatContextRegistrationProps): AIChatPageContextItem[] {
  const visibleAgents = agents.slice(0, AGENTS_CONTEXT_VISIBLE_LIMIT);
  const capabilities = buildAgentListCapabilities(canView, canManage);
  const contextReady = permissionsSettled && canView && !isLoading && !isFetching;
  const queryStatus = !permissionsSettled
    ? 'loading'
    : !canView
      ? 'unavailable'
      : contextReady
        ? 'ready'
        : 'loading';
  const querySettled = permissionsSettled && (!canView || contextReady);
  const orderedVisibleAgentIds = visibleAgents.map(agent => agent.id).join(',');
  const visibleRuntimeAgents = visibleAgents.filter(isAgentRuntimeItem);
  const orderedVisibleRuntimeAgentIds = visibleRuntimeAgents.map(agent => agent.id).join(',');
  const firstRuntimeAgentIndex = visibleAgents.findIndex(isAgentRuntimeItem);
  const firstRuntimeAgent =
    firstRuntimeAgentIndex >= 0 ? visibleAgents[firstRuntimeAgentIndex] : undefined;
  const resolvedPageTitle = pageTitle?.trim() || 'Agent Management';
  const agentTypeCounts = visibleAgents.reduce<Record<string, number>>((counts, agent) => {
    const key = agentTypeLabel(agent.agent_type);
    counts[key] = (counts[key] ?? 0) + 1;
    return counts;
  }, {});

  return [
    {
      id: 'console.agents',
      type: 'page',
      title: resolvedPageTitle,
      subtitle: workspaceName
        ? `${workspaceName} ${resolvedPageTitle}`
        : `Current workspace ${resolvedPageTitle}`,
      description: buildAgentListDescription(visibleAgents, isLoading, hasNextPage),
      href: '/console/agents',
      source: resolvedPageTitle,
      status: canView ? 'available' : 'readonly',
      capabilities,
      hints: {
        handledAssetTypes: ['agent'],
        refreshHints: [
          { assetType: 'agent', queryKey: AGENT_KEYS.all },
          { assetType: 'agent', queryKey: AGENT_KEYS.lists() },
        ],
      },
      metadata: {
        page: 'console.agents',
        route: '/console/agents',
        resource_kind: 'page',
        context_ready: contextReady,
        agents_query_status: queryStatus,
        agents_query_settled: querySettled,
        permissions_settled: permissionsSettled,
        permissions_query_status: permissionsSettled ? 'ready' : 'loading',
        ordered_visible_agent_ids: orderedVisibleAgentIds || null,
        ordered_visible_runtime_agent_ids: orderedVisibleRuntimeAgentIds || null,
        visible_agent_count: visibleAgents.length,
        visible_runtime_agent_count: visibleRuntimeAgents.length,
        loaded_agent_count: agents.length,
        omitted_context_agent_count: Math.max(agents.length - visibleAgents.length, 0),
        first_visible_runtime_agent_id: firstRuntimeAgent?.id,
        first_visible_runtime_agent_name: firstRuntimeAgent?.name,
        first_visible_runtime_agent_href: firstRuntimeAgent
          ? getAgentDetailEditHref(firstRuntimeAgent.id, firstRuntimeAgent.agent_type)
          : undefined,
        first_visible_runtime_agent_index:
          firstRuntimeAgentIndex >= 0 ? firstRuntimeAgentIndex + 1 : undefined,
        has_next_page: hasNextPage,
        page_size: pageSize,
        search: searchKeyword.trim(),
        workspace_id: workspaceId,
        workspace_name: workspaceName,
        can_view_agents: canView,
        can_manage_agents: canManage,
        agent_type_counts: Object.entries(agentTypeCounts)
          .map(([type, count]) => `${type}=${count}`)
          .join(','),
      },
    },
    ...visibleAgents.map((agent, index) => {
      const href = getAgentDetailEditHref(agent.id, agent.agent_type);
      const itemType = contextItemTypeForAgent(agent);
      return {
        id: agent.id,
        type: itemType,
        title: agent.name,
        subtitle: `${agentTypeLabel(agent.agent_type)} - ${publishedStatus(agent)}`,
        description: compactContextField(
          agent.description || 'No description is set for this Agent.'
        ),
        href,
        source: 'Agents page',
        risk: 'low' as const,
        status: agent.is_published ? ('published' as const) : ('draft' as const),
        capabilities: [
          {
            id: 'agent.open',
            title: isAgentRuntimeItem(agent) ? 'Open Agent' : 'Open workflow',
            description: isAgentRuntimeItem(agent)
              ? 'Navigate to this Agent Runtime detail page.'
              : 'Navigate to this workflow detail page.',
            risk: 'low' as const,
            status: 'available' as const,
            permissions: ['agent.view'],
          },
          {
            id: 'agent.inspect_summary',
            title: 'Inspect Agent summary',
            description: 'Answer from the visible Agent card metadata.',
            risk: 'low' as const,
            status: 'available' as const,
          },
          ...(isAgentRuntimeItem(agent)
            ? [
                {
                  id: 'agent.update_identity',
                  title: 'Edit Agent identity',
                  description: 'Update this Agent name, description, or icon.',
                  risk: 'medium' as const,
                  requiresConfirmation: true,
                  status: agent.can_edit && canManage ? ('available' as const) : ('disabled' as const),
                  permissions: ['agent.manage'],
                  metadata: {
                    supported_by_aichat_tool: true,
                    tool_skill_id: 'agent-management',
                    tool_name: 'update_agent_identity',
                    agent_id: agent.id,
                  },
                },
                {
                  id: 'agent.delete',
                  title: 'Delete Agent',
                  description: 'Delete this Agent. Deletion always asks first.',
                  risk: 'high' as const,
                  requiresConfirmation: true,
                  status: agent.can_edit && canManage ? ('available' as const) : ('disabled' as const),
                  permissions: ['agent.manage'],
                  metadata: {
                    supported_by_aichat_tool: true,
                    tool_skill_id: 'agent-management',
                    tool_name: 'delete_agent',
                    agent_id: agent.id,
                  },
                },
              ]
            : []),
        ],
        permissions: agent.can_edit ? ['agent.view', 'agent.manage'] : ['agent.view'],
        metadata: buildVisibleAgentMetadata(agent, index + 1),
      };
    }),
  ];
}

export function AgentsAIChatContextRegistration(props: AgentsAIChatContextRegistrationProps) {
  const {
    agents,
    pageSize,
    searchKeyword,
    pageTitle,
    workspaceId,
    workspaceName,
    canView,
    canManage,
    isLoading,
    isFetching,
    permissionsSettled,
    hasNextPage,
  } = props;
  const items = useMemo(
    () =>
      buildAgentsAIChatContextItems({
        agents,
        pageSize,
        searchKeyword,
        pageTitle,
        workspaceId,
        workspaceName,
        canView,
        canManage,
        isLoading,
        isFetching,
        permissionsSettled,
        hasNextPage,
      }),
    [
      agents,
      canManage,
      canView,
      hasNextPage,
      isFetching,
      isLoading,
      pageSize,
      pageTitle,
      permissionsSettled,
      searchKeyword,
      workspaceId,
      workspaceName,
    ]
  );

  usePageContextRegistration(items, { scopeId: 'console-agents' });
  return null;
}
