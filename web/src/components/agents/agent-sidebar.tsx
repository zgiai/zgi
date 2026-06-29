'use client';

import * as React from 'react';
import { usePathname, useParams } from 'next/navigation';
import { BookOpen, History, KeyRound, PanelsTopLeft, RotateCcw, ScanSearch } from 'lucide-react';
import { useAgent } from '@/hooks/agent/use-agents';
import { useT } from '@/i18n';
import { ICON_BG, ICON_TEXT } from '@/lib/config';
import {
  ResourceSidebar,
  ResourceSidebarHeader,
  type ResourceSidebarNavItem,
} from '@/components/common/resource-sidebar';
import AgentDialog from '@/components/agents/agent-dialog';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useWorkflowDebugFocusMode } from '@/components/workflow/hooks/use-debug-focus-mode';
import { usePersistentSidebarCollapse } from '@/hooks/use-persistent-sidebar-collapse';
import {
  getAgentDetailRouteAccess,
  isAgentRuntimeType,
  isWorkflowRuntimeType,
} from '@/utils/agent-detail-routes';
import {
  AGENT_ASSET_VISIBLE_PERMISSION_CODES,
  AGENT_PERMISSION_ACTIONS,
  WORKFLOW_PERMISSION_ACTIONS,
} from '@/constants/permissions';
import { markAgentListRestoreIntentFromDetail } from '@/utils/agent-list-state';

interface AgentSidebarProps {
  /** When true, hide navigation items (workspace mismatch mode) */
  isMismatch?: boolean;
}

/**
 * AgentSidebar — collapsible agent-specific sidebar.
 * - Shows agent summary (icon, name, desc) on top; collapsed shows only icon (smaller size)
 * - First nav item links to the editor for the current agent type.
 * - Collapsed state persisted to localStorage
 */
export function AgentSidebar({ isMismatch = false }: AgentSidebarProps) {
  const pathname = usePathname();
  const params = useParams<{ agentId: string }>();
  const agentId = params?.agentId ?? '';
  const t = useT();
  const { hasAnyPermission } = useAccountPermissions();
  const canView = hasAnyPermission(AGENT_ASSET_VISIBLE_PERMISSION_CODES);
  const { agent, isLoading } = useAgent(agentId, canView);
  const canCreateAgent = hasAnyPermission(AGENT_PERMISSION_ACTIONS.create);
  const canImportAgent = hasAnyPermission(AGENT_PERMISSION_ACTIONS.import);
  const canUpdateAgent = hasAnyPermission(AGENT_PERMISSION_ACTIONS.update);
  const canConfigureAgentRuntime = hasAnyPermission(AGENT_PERMISSION_ACTIONS.runtimeConfigManage);
  const canPublishAgent = hasAnyPermission(AGENT_PERMISSION_ACTIONS.publish);
  const canCreateWorkflow = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.create);
  const canImportWorkflow = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.import);
  const canUpdateWorkflow = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.update);
  const canRunWorkflowDraft = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.runDraft);
  const canStopWorkflowRun = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.runStop);
  const canDebugWorkflow = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.debug);
  const canPublishWorkflow = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.publish);
  const canConfigureWorkflowRuntime = hasAnyPermission(
    WORKFLOW_PERMISSION_ACTIONS.runtimeConfigManage
  );
  const canManageAgentRuntimeAccess = hasAnyPermission(AGENT_PERMISSION_ACTIONS.runtimeAccessManage);
  const canManageWorkflowRuntimeAccess = hasAnyPermission(
    WORKFLOW_PERMISSION_ACTIONS.runtimeAccessManage
  );
  const canViewAgentLogs = hasAnyPermission(AGENT_PERMISSION_ACTIONS.logsView);
  const canViewWorkflowLogs = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.logsView);
  const canViewWorkflowTestLibrary = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.view);
  const canViewWorkflowTestBatches = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.logsView);
  const canRunWorkflowBatchTest = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.debug);
  const [editOpen, setEditOpen] = React.useState(false);
  const isDebugFocusMode = useWorkflowDebugFocusMode();
  const [isCollapsed, setIsCollapsed] = usePersistentSidebarCollapse(
    'agent',
    true,
    isDebugFocusMode
  );

  const toggleCollapse = () => setIsCollapsed(prev => !prev);
  const agentData = agent?.data;
  const isAgentRuntime = isAgentRuntimeType(agentData?.agent_type);
  const isWorkflowRuntime = isWorkflowRuntimeType(agentData?.agent_type);
  const canEditIdentity = isAgentRuntime
    ? canUpdateAgent
    : isWorkflowRuntime
      ? canUpdateWorkflow
      : false;
  const canEditRuntime = isAgentRuntime
    ? canCreateAgent ||
      canImportAgent ||
      canUpdateAgent ||
      canConfigureAgentRuntime ||
      canPublishAgent ||
      canManageAgentRuntimeAccess
    : isWorkflowRuntime
      ? canCreateWorkflow ||
        canImportWorkflow ||
        canUpdateWorkflow ||
        canRunWorkflowDraft ||
        canStopWorkflowRun ||
        canDebugWorkflow ||
        canPublishWorkflow ||
        canConfigureWorkflowRuntime ||
        canManageWorkflowRuntimeAccess
      : false;
  const canManageRuntimeAccess = isAgentRuntime
    ? canManageAgentRuntimeAccess
    : isWorkflowRuntime
      ? canManageWorkflowRuntimeAccess
      : false;
  const canViewRuntimeLogs = isAgentRuntime
    ? canViewAgentLogs
    : isWorkflowRuntime
      ? canViewWorkflowLogs
      : false;
  const routeAccess = React.useMemo(
    () =>
      getAgentDetailRouteAccess(agentId, agentData?.agent_type, {
        canView,
        canOpenEditor: canEditRuntime,
        canEditRuntime,
        canManageRuntimeAccess,
        canViewRuntimeLogs,
        canViewBatchTest:
          isWorkflowRuntime &&
          (canViewWorkflowTestLibrary || canViewWorkflowTestBatches || canRunWorkflowBatchTest),
        canRunBatchTest: isWorkflowRuntime && canRunWorkflowBatchTest,
      }),
    [
      agentData?.agent_type,
      agentId,
      canEditRuntime,
      canManageRuntimeAccess,
      canRunWorkflowBatchTest,
      canViewWorkflowTestBatches,
      canViewWorkflowTestLibrary,
      canView,
      canViewRuntimeLogs,
      isWorkflowRuntime,
    ]
  );

  const navItems: ResourceSidebarNavItem[] = React.useMemo(() => {
    const items: ResourceSidebarNavItem[] = [];

    if (routeAccess.canShowEditor) {
      items.push({
        title: t('agents.actions.edit'),
        href: routeAccess.editHref,
        icon: PanelsTopLeft,
      });
    }

    if (routeAccess.canShowRuntimeLogs && agentData?.is_published) {
      items.push({
        title: t('agents.workflow.webappLogs'),
        href: `/console/agents/${agentId}/logs`,
        icon: History,
      });
    }

    if (routeAccess.canShowApiKeys) {
      items.push({
        title: t('agents.apiKeys.navTitle'),
        href: `/console/agents/${agentId}/api`,
        icon: KeyRound,
      });
    }

    if (routeAccess.canShowBatchTest) {
      const batchTestHref = canViewWorkflowTestLibrary
        ? `/console/agents/${agentId}/batch-test`
        : `/console/agents/${agentId}/batch-test/batches`;
      const batchTestChildren: ResourceSidebarNavItem[] = [];

      if (canViewWorkflowTestLibrary) {
        batchTestChildren.push({
          title: t('agents.workflowTest.subnav.caseLibrary'),
          href: `/console/agents/${agentId}/batch-test`,
          icon: BookOpen,
          isActive: currentPathname =>
            currentPathname === `/console/agents/${agentId}/batch-test`,
        });
      }

      if (canViewWorkflowTestBatches) {
        batchTestChildren.push({
          title: t('agents.workflowTest.subnav.batches'),
          href: `/console/agents/${agentId}/batch-test/batches`,
          icon: RotateCcw,
          isActive: currentPathname =>
            currentPathname === `/console/agents/${agentId}/batch-test/batches` ||
            currentPathname.startsWith(`/console/agents/${agentId}/batch-test/`),
        });
      }

      items.push({
        title: t('agents.workflowTest.navTitle'),
        href: batchTestHref,
        icon: ScanSearch,
        children: batchTestChildren,
      });
    }

    return items;
  }, [
    agentData?.is_published,
    agentId,
    canViewWorkflowTestBatches,
    canViewWorkflowTestLibrary,
    routeAccess,
    t,
  ]);

  const iconType = agentData?.icon_type;
  let textIcon = agentData?.name?.slice(0, 2).toUpperCase() || ICON_TEXT;
  let iconBackground = ICON_BG;
  let imgSrc: string | undefined = undefined;

  if (iconType === 'image') {
    imgSrc = agentData?.icon_url || '';
  } else if (iconType === 'text') {
    try {
      const parsed = JSON.parse(agentData?.icon || '{}');
      textIcon = parsed?.icon || textIcon;
      iconBackground = parsed?.icon_background || iconBackground;
    } catch {
      // ignore parse errors
    }
  } else if (agentData?.icon) {
    try {
      const parsed = JSON.parse(agentData.icon);
      if (parsed?.icon) textIcon = parsed.icon;
      if (parsed?.icon_background) iconBackground = parsed.icon_background;
    } catch {
      // ignore parse errors
    }
  }

  return (
    <>
      <ResourceSidebar
        isCollapsed={isCollapsed}
        onToggleCollapse={toggleCollapse}
        expandLabel={t('navigation.expand')}
        collapseLabel={t('navigation.collapse')}
        header={
          <ResourceSidebarHeader
            isCollapsed={isCollapsed}
            iconType={iconType}
            iconSrc={imgSrc}
            icon={textIcon}
            iconBackground={iconBackground}
            name={agentData?.name || (isLoading ? t('agents.loading') : '-')}
            description={agentData?.description || ''}
            showIdentity={false}
            backHref="/console/agents"
            backLabel={t('agents.backToAgentList')}
            onBackClick={() => markAgentListRestoreIntentFromDetail(agentId)}
            iconActionLabel={t('agents.actions.edit')}
            onIconClick={
              canEditIdentity && !isMismatch && agentData
                ? () => setEditOpen(true)
                : undefined
            }
          />
        }
        navItems={navItems}
        pathname={pathname}
        isNavigationHidden={isMismatch}
      />
      <AgentDialog open={editOpen} mode="edit" agentId={agentId} onOpenChange={setEditOpen} />
    </>
  );
}

export default AgentSidebar;
