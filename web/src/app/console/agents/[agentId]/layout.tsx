'use client';

import React from 'react';
import AgentSidebar from '@/components/agents/agent-sidebar';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { ShieldAlert, Loader2 } from 'lucide-react';
import { useAgent } from '@/hooks/agent/use-agents';
import { useParams } from 'next/navigation';
import { WorkspaceMismatchGuard } from '@/components/common/workspace-mismatch-guard';
import { useT } from '@/i18n';
import { AGENT_ASSET_VISIBLE_PERMISSION_CODES } from '@/constants/permissions';

export default function AgentLayout({ children }: { children: React.ReactNode }) {
  const t = useT();
  const params = useParams<{ agentId: string }>();
  const agentId = params?.agentId ?? '';

  const { hasAnyPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const { agent, isLoading: isAgentLoading } = useAgent(agentId);

  const canView = hasAnyPermission(AGENT_ASSET_VISIBLE_PERMISSION_CODES);
  const isLoading = isPermissionsLoading || isAgentLoading;

  if (isLoading) {
    return (
      <div className="flex h-full w-full items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (!canView) {
    return (
      <div className="flex flex-col items-center justify-center h-full w-full p-4 text-center">
        <ShieldAlert className="h-12 w-12 text-muted-foreground mb-4" />
        <h2 className="text-xl font-semibold mb-2">{t('common.accessDenied')}</h2>
        <p className="text-muted-foreground max-w-md">{t('common.unauthorizedDescription')}</p>
      </div>
    );
  }

  return (
    <WorkspaceMismatchGuard
      isLoading={isAgentLoading}
      targetWorkspaceId={agent?.data?.workspace?.id || ''}
      targetWorkspaceName={agent?.data?.workspace?.name}
    >
      <div className="flex h-full min-w-0 min-h-0">
        <AgentSidebar />
        <div className="w-0 grow h-full min-w-0 min-h-0 overflow-auto">{children}</div>
      </div>
    </WorkspaceMismatchGuard>
  );
}
