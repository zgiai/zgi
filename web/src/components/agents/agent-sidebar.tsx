'use client';

import * as React from 'react';
import { usePathname, useParams } from 'next/navigation';
import { BookOpen, History, KeyRound, PanelsTopLeft, RotateCcw, ScanSearch } from 'lucide-react';
import { useAgent } from '@/hooks/agent/use-agents';
import { useT } from '@/i18n';
import {
  ENABLE_AGENT_API_PAGE,
  ENABLE_AGENT_BATCH_TEST_PAGE,
  ENABLE_AGENT_RUNTIME_LOGS_PAGE,
  ICON_BG,
  ICON_TEXT,
} from '@/lib/config';
import {
  ResourceSidebar,
  ResourceSidebarHeader,
  type ResourceSidebarNavItem,
} from '@/components/common/resource-sidebar';
import AgentDialog from '@/components/agents/agent-dialog';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useWorkflowDebugFocusMode } from '@/components/workflow/hooks/use-debug-focus-mode';
import { usePersistentSidebarCollapse } from '@/hooks/use-persistent-sidebar-collapse';

interface AgentSidebarProps {
  /** When true, hide navigation items (workspace mismatch mode) */
  isMismatch?: boolean;
}

/**
 * AgentSidebar — collapsible agent-specific sidebar.
 * - Shows agent summary (icon, name, desc) on top; collapsed shows only icon (smaller size)
 * - First nav item is always Edit and links to /workflow
 * - Feature-gated unfinished Agent pages stay hidden until enabled
 * - Collapsed state persisted to localStorage
 */
export function AgentSidebar({ isMismatch = false }: AgentSidebarProps) {
  const pathname = usePathname();
  const params = useParams<{ agentId: string }>();
  const agentId = params?.agentId ?? '';
  const { agent, isLoading } = useAgent(agentId);
  const t = useT();
  const { hasPermission } = useAccountPermissions();
  const canManage = hasPermission('agent.manage');
  const [editOpen, setEditOpen] = React.useState(false);
  const isDebugFocusMode = useWorkflowDebugFocusMode();
  const [isCollapsed, setIsCollapsed] = usePersistentSidebarCollapse(
    'agent',
    true,
    isDebugFocusMode
  );

  const toggleCollapse = () => setIsCollapsed(prev => !prev);
  const agentData = agent?.data;
  const editHref =
    agentData?.agent_type === 'AGENT'
      ? `/console/agents/${agentId}/agent`
      : `/console/agents/${agentId}/workflow`;

  const navItems: ResourceSidebarNavItem[] = React.useMemo(() => {
    const items: ResourceSidebarNavItem[] = [
      { title: t('agents.actions.edit'), href: editHref, icon: PanelsTopLeft },
    ];

    if (ENABLE_AGENT_RUNTIME_LOGS_PAGE && agentData?.is_published) {
      items.push({
        title: t('agents.workflow.webappLogs'),
        href: `/console/agents/${agentId}/logs`,
        icon: History,
      });
    }

    if (ENABLE_AGENT_API_PAGE) {
      items.push({
        title: t('agents.apiKeys.navTitle'),
        href: `/console/agents/${agentId}/api`,
        icon: KeyRound,
      });
    }

    if (ENABLE_AGENT_BATCH_TEST_PAGE) {
      items.push({
        title: t('agents.workflowTest.navTitle'),
        href: `/console/agents/${agentId}/batch-test`,
        icon: ScanSearch,
        children: [
          {
            title: t('agents.workflowTest.subnav.caseLibrary'),
            href: `/console/agents/${agentId}/batch-test`,
            icon: BookOpen,
            isActive: currentPathname =>
              currentPathname === `/console/agents/${agentId}/batch-test`,
          },
          {
            title: t('agents.workflowTest.subnav.batches'),
            href: `/console/agents/${agentId}/batch-test/batches`,
            icon: RotateCcw,
            isActive: currentPathname =>
              currentPathname === `/console/agents/${agentId}/batch-test/batches` ||
              currentPathname.startsWith(`/console/agents/${agentId}/batch-test/`),
          },
        ],
      });
    }

    return items;
  }, [agentData, agentId, editHref, t]);

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
            iconActionLabel={t('agents.actions.edit')}
            onIconClick={
              canManage && !isMismatch && agentData ? () => setEditOpen(true) : undefined
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
