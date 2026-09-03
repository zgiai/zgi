'use client';

import { useMemo } from 'react';
import { useAccountCapabilities } from '@/hooks/use-account-capabilities';
import { useDashboardRecentWork, useDashboardStats } from '@/hooks/dashboard/use-dashboard';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import {
  getZGIConsoleNavigationAccess,
  getZGIConsoleNavigationDisplayState,
  type ZGIConsoleNavigationDisplayState,
  type ZGIConsoleNavigationAccessContext,
} from '@/routes/console-navigation';
import {
  useCurrentWorkspace,
  usePermissions,
  useWorkspaceContextStatus,
} from '@/store/workspace-store';

export type WorkspaceHomeAccessState = ZGIConsoleNavigationDisplayState;

function toHomeAccessState(
  href: string,
  context: ZGIConsoleNavigationAccessContext,
  permissionsFailed: boolean
): WorkspaceHomeAccessState {
  const access = getZGIConsoleNavigationAccess(href, context);
  return getZGIConsoleNavigationDisplayState(access, permissionsFailed);
}

export function useWorkspaceHome() {
  const currentWorkspace = useCurrentWorkspace();
  const contextStatus = useWorkspaceContextStatus();
  const permissionState = usePermissions();
  const capabilities = useAccountCapabilities();
  const accountPermissions = useAccountPermissions();
  const isContextSettled = contextStatus !== 'loading';
  const statsQuery = useDashboardStats(
    {
      scope: currentWorkspace ? 'workspace' : 'overview',
      workspace_id: currentWorkspace?.id,
    },
    { enabled: isContextSettled, refetchOnWindowFocus: false, staleTime: 0 }
  );
  const recentWorkQuery = useDashboardRecentWork(
    {
      scope: currentWorkspace ? 'workspace' : 'overview',
      workspace_id: currentWorkspace?.id,
      limit: 8,
    },
    { enabled: isContextSettled, refetchOnWindowFocus: false, staleTime: 0 }
  );

  const stats = statsQuery.data?.data;
  const recentWork = useMemo(
    () => recentWorkQuery.data?.data.items ?? [],
    [recentWorkQuery.data?.data.items]
  );
  const navigationContext = useMemo<ZGIConsoleNavigationAccessContext>(
    () => ({
      workspaceStatus: contextStatus,
      permissionsSettled: !accountPermissions.isLoading && accountPermissions.error === null,
      organizationRole: accountPermissions.organizationRole,
      workspaceRole: accountPermissions.workspaceRole,
      permissions: accountPermissions.permissions,
    }),
    [
      accountPermissions.error,
      accountPermissions.isLoading,
      accountPermissions.organizationRole,
      accountPermissions.permissions,
      accountPermissions.workspaceRole,
      contextStatus,
    ]
  );
  const actionAccess = useMemo(() => {
    const permissionsFailed = accountPermissions.error !== null;
    const isOrganizationAdmin =
      navigationContext.organizationRole === 'owner' ||
      navigationContext.organizationRole === 'admin';
    const createAccess = (href: string, permission: string): WorkspaceHomeAccessState => {
      const pageAccess = toHomeAccessState(href, navigationContext, permissionsFailed);
      if (pageAccess !== 'available') return pageAccess;
      return isOrganizationAdmin || navigationContext.permissions.includes(permission)
        ? 'available'
        : 'forbidden';
    };

    return {
      chat: toHomeAccessState('/console/work/chat', navigationContext, permissionsFailed),
      agent: toHomeAccessState('/console/agents', navigationContext, permissionsFailed),
      agentCreate: createAccess('/console/agents', 'agent.create'),
      knowledge: toHomeAccessState('/console/dataset', navigationContext, permissionsFailed),
      knowledgeCreate: createAccess('/console/dataset', 'knowledge_base.create'),
      workflow: toHomeAccessState('/console/workflows', navigationContext, permissionsFailed),
      workflowCreate: createAccess('/console/workflows', 'workflow.create'),
      apps: toHomeAccessState('/console/work/app', navigationContext, permissionsFailed),
      workspace: toHomeAccessState('/console/workspace', navigationContext, permissionsFailed),
      modelConfig: capabilities.isLoading
        ? 'loading'
        : capabilities.error
          ? 'error'
          : capabilities.canManageModelConfig
            ? 'available'
            : 'forbidden',
    } satisfies Record<string, WorkspaceHomeAccessState>;
  }, [
    accountPermissions.error,
    capabilities.canManageModelConfig,
    capabilities.error,
    capabilities.isLoading,
    navigationContext,
  ]);
  const checklist = useMemo(() => {
    const hasWorkspace = contextStatus === 'ready' && Boolean(currentWorkspace);
    const hasChatModel = (stats?.models.by_usecase?.['text-chat'] ?? 0) > 0;
    const hasConversation = (stats?.activity?.direct_conversations ?? 0) > 0;
    const hasAgent = (stats?.activity?.created_agents ?? 0) > 0;
    const hasKnowledge = (stats?.activity?.created_datasets ?? 0) > 0;

    return {
      workspace: hasWorkspace,
      model: hasChatModel,
      chat: hasConversation,
      agent: hasAgent,
      knowledge: hasKnowledge,
      completed: [hasWorkspace, hasChatModel, hasConversation, hasAgent, hasKnowledge].filter(
        Boolean
      ).length,
      total: 5,
    };
  }, [contextStatus, currentWorkspace, stats]);

  const refresh = () => {
    void statsQuery.refetch();
    void recentWorkQuery.refetch();
    void capabilities.refetch();
  };

  return {
    currentWorkspace,
    contextStatus,
    organizationRole: permissionState.organizationRole,
    canManageModelConfig: capabilities.canManageModelConfig,
    actionAccess,
    stats,
    recentWork,
    checklist,
    isLoading: !isContextSettled || statsQuery.isLoading || recentWorkQuery.isLoading,
    isStatsLoading: !isContextSettled || statsQuery.isLoading,
    isRecentWorkLoading: !isContextSettled || recentWorkQuery.isLoading,
    isReadinessLoading: !isContextSettled || statsQuery.isLoading || capabilities.isLoading,
    isChecklistLoading:
      !isContextSettled ||
      statsQuery.isLoading ||
      capabilities.isLoading ||
      (contextStatus === 'ready' && accountPermissions.isLoading),
    isRefreshing:
      !isContextSettled ||
      statsQuery.isFetching ||
      recentWorkQuery.isFetching ||
      capabilities.isFetching,
    hasError: statsQuery.isError || recentWorkQuery.isError || capabilities.error !== null,
    hasStatsError: statsQuery.isError,
    hasRecentWorkError: recentWorkQuery.isError,
    hasReadinessError: statsQuery.isError || capabilities.error !== null,
    refresh,
  };
}
