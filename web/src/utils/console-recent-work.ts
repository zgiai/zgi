import type { DashboardRecentWorkItem, DashboardRecentWorkType } from '@/services/types/dashboard';
import { AgentType } from '@/services/types/agent';
import { getAgentDetailDefaultHref } from '@/utils/agent-detail-routes';

export type RecentWorkAgentAccess = Pick<
  DashboardRecentWorkItem,
  | 'agent_type'
  | 'is_published'
  | 'can_open_editor'
  | 'can_view_logs'
  | 'can_view_batch_test'
  | 'prefer_batch_test_library'
>;

export function getRecentWorkHref(
  type: DashboardRecentWorkType,
  resourceId: string,
  parentId?: string
) {
  if (type === 'conversation') {
    if (!parentId) {
      return `/console/work/chat?convId=${encodeURIComponent(resourceId)}`;
    }
    const query = `conversation_id=${encodeURIComponent(resourceId)}`;
    return `/console/agents/${parentId}/logs?${query}`;
  }

  if (type === 'workflow') {
    return `/console/workflows/${resourceId}`;
  }

  if (type === 'agent') {
    return `/console/agents/${resourceId}`;
  }

  if (type === 'dataset') {
    return `/console/dataset/${resourceId}`;
  }

  return `/console/db/${resourceId}`;
}

function hasAgentAccess(access?: RecentWorkAgentAccess): access is RecentWorkAgentAccess {
  return (
    access !== undefined &&
    (access.is_published !== undefined || access.can_open_editor !== undefined)
  );
}

/**
 * Agent and workflow items reuse the agent card's route choice, so a member
 * who can only view published assets is never linked to an editor it cannot
 * open. Returns null when no detail page is available.
 */
function getRecentAgentHref(
  type: 'agent' | 'workflow',
  resourceId: string,
  access: RecentWorkAgentAccess
): string | null {
  const agentType = access.agent_type || (type === 'workflow' ? AgentType.WORKFLOW : AgentType.AGENT);
  return getAgentDetailDefaultHref(resourceId, agentType, {
    canView: true,
    canOpenEditor: Boolean(access.can_open_editor),
    canManageRuntimeAccess: false,
    canViewRuntimeLogs: Boolean(access.can_view_logs),
    canViewBatchTest: Boolean(access.can_view_batch_test),
    canRunBatchTest: false,
    isPublished: Boolean(access.is_published),
    preferBatchTestLibrary: Boolean(access.prefer_batch_test_library),
  });
}

export function getRecentWorkNavigationHref(
  hasWorkspace: boolean,
  type: DashboardRecentWorkType,
  resourceId: string,
  parentId?: string,
  workspaceId?: string,
  agentAccess?: RecentWorkAgentAccess
): string | null {
  const isOrganizationConversation = type === 'conversation' && !parentId && !workspaceId;
  if (!hasWorkspace && !isOrganizationConversation) {
    return null;
  }
  if ((type === 'agent' || type === 'workflow') && hasAgentAccess(agentAccess)) {
    return getRecentAgentHref(type, resourceId, agentAccess);
  }
  return getRecentWorkHref(type, resourceId, parentId);
}
