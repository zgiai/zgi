'use client';

import React, { useEffect } from 'react';
import { useParams, usePathname, useRouter, useSearchParams } from 'next/navigation';
import { Loader2, ShieldAlert } from 'lucide-react';
import AgentSidebar from '@/components/agents/agent-sidebar';
import { WorkspaceMismatchGuard } from '@/components/common/workspace-mismatch-guard';
import { useAgent } from '@/hooks/agent/use-agents';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import {
  AGENT_ASSET_VISIBLE_PERMISSION_CODES,
  AGENT_PERMISSION_ACTIONS,
  WORKFLOW_PERMISSION_ACTIONS,
} from '@/constants/permissions';
import {
  getAgentDetailBaseHref,
  getAgentDetailRouteKind,
  type AgentDetailRouteKind,
} from '@/utils/agent-detail-routes';

interface AgentDetailLayoutProps {
  children: React.ReactNode;
  routeKind: AgentDetailRouteKind;
}

function buildCanonicalHref(
  pathname: string,
  searchParams: Pick<URLSearchParams, 'toString'>,
  agentId: string,
  targetKind: AgentDetailRouteKind
) {
  const currentBase = pathname.startsWith(`/console/workflows/${agentId}`)
    ? `/console/workflows/${agentId}`
    : `/console/agents/${agentId}`;
  const targetBase = getAgentDetailBaseHref(agentId, targetKind);
  let suffix = pathname.startsWith(currentBase) ? pathname.slice(currentBase.length) : '';

  if (suffix === '/agent' || suffix === '/workflow') {
    suffix = '';
  }

  const query = searchParams.toString();
  return `${targetBase}${suffix}${query ? `?${query}` : ''}`;
}

export function AgentDetailLayout({ children, routeKind }: AgentDetailLayoutProps) {
  const t = useT();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const params = useParams<{ agentId: string }>();
  const agentId = params?.agentId ?? '';

  const { hasAnyPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canViewAnyAgentAsset = hasAnyPermission(AGENT_ASSET_VISIBLE_PERMISSION_CODES);
  const { agent, isLoading: isAgentLoading } = useAgent(agentId, canViewAnyAgentAsset);

  const actualRouteKind = getAgentDetailRouteKind(agent?.data?.agent_type);
  const canViewAgentDetail = hasAnyPermission(AGENT_PERMISSION_ACTIONS.page);
  const canViewWorkflowDetail = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.page);
  const canViewRouteKind = routeKind === 'agent' ? canViewAgentDetail : canViewWorkflowDetail;
  const canViewActualKind =
    actualRouteKind === 'agent'
      ? canViewAgentDetail
      : actualRouteKind === 'workflow'
        ? canViewWorkflowDetail
        : false;
  const shouldRedirectToCanonical =
    Boolean(actualRouteKind) && actualRouteKind !== routeKind && canViewActualKind;
  const isLoading = isPermissionsLoading || isAgentLoading;

  useEffect(() => {
    if (!shouldRedirectToCanonical || !actualRouteKind) {
      return;
    }

    router.replace(buildCanonicalHref(pathname, searchParams, agentId, actualRouteKind));
  }, [
    actualRouteKind,
    agentId,
    pathname,
    router,
    searchParams,
    shouldRedirectToCanonical,
  ]);

  if (isLoading || shouldRedirectToCanonical) {
    return (
      <div className="flex h-full w-full items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  const canView =
    actualRouteKind === routeKind ? canViewRouteKind : Boolean(actualRouteKind && canViewActualKind);

  if (!canView) {
    return (
      <div className="flex h-full w-full flex-col items-center justify-center p-4 text-center">
        <ShieldAlert className="mb-4 h-12 w-12 text-muted-foreground" />
        <h2 className="mb-2 text-xl font-semibold">{t('common.accessDenied')}</h2>
        <p className="max-w-md text-muted-foreground">{t('common.unauthorizedDescription')}</p>
      </div>
    );
  }

  return (
    <WorkspaceMismatchGuard
      isLoading={isAgentLoading}
      targetWorkspaceId={agent?.data?.workspace?.id || ''}
      targetWorkspaceName={agent?.data?.workspace?.name}
    >
      <div className="flex h-full min-h-0 min-w-0">
        <AgentSidebar routeKind={routeKind} />
        <div className="h-full min-h-0 w-0 min-w-0 grow overflow-auto">{children}</div>
      </div>
    </WorkspaceMismatchGuard>
  );
}

export default AgentDetailLayout;
