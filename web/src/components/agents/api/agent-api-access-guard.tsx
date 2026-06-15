'use client';

import type { ReactNode } from 'react';
import { AlertCircle, Loader2 } from 'lucide-react';
import { useAgent } from '@/hooks/agent/use-agents';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import type { AgentType } from '@/services/types/agent';
import { useT } from '@/i18n';
import { canShowAgentApiKeys, supportsAgentApiKeyPages } from '@/utils/agent-detail-routes';
import { getErrorMessage } from '@/utils/error-notifications';

interface AgentApiAccessGuardProps {
  agentId: string;
  children: (context: { agentType: AgentType | string | undefined }) => ReactNode;
}

export function AgentApiAccessGuard({ agentId, children }: AgentApiAccessGuardProps) {
  const t = useT();
  const tWebapp = useT('webapp');
  const { agent, isLoading, error } = useAgent(agentId);
  const { hasPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canManage = hasPermission('agent.manage');
  const agentType = (agent?.data?.agent_type as AgentType | string | undefined) ?? undefined;

  if (isLoading || isPermissionsLoading) {
    return (
      <div className="flex h-full w-full items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (error || !agent?.data) {
    return (
      <div className="flex h-full w-full items-center justify-center p-6">
        <div className="max-w-xl rounded-2xl border border-dashed bg-background p-8 text-center">
          <div className="mx-auto mb-4 flex size-12 items-center justify-center rounded-full bg-muted">
            <AlertCircle className="size-5 text-muted-foreground" />
          </div>
          <div className="text-lg font-semibold">{t('agents.workflow.loadFailedTitle')}</div>
          <div className="mt-2 text-sm text-muted-foreground">
            {error ? getErrorMessage(error) : t('agents.workflow.notFoundDesc')}
          </div>
        </div>
      </div>
    );
  }

  if (!supportsAgentApiKeyPages(agentType)) {
    return (
      <div className="flex h-full w-full items-center justify-center p-6">
        <div className="max-w-xl rounded-2xl border border-dashed bg-background p-8 text-center">
          <div className="mx-auto mb-4 flex size-12 items-center justify-center rounded-full bg-muted">
            <AlertCircle className="size-5 text-muted-foreground" />
          </div>
          <div className="text-lg font-semibold">{tWebapp('appCenter.appUnavailableTitle')}</div>
          <div className="mt-2 text-sm text-muted-foreground">
            {tWebapp('appCenter.appUnavailableDescription')}
          </div>
        </div>
      </div>
    );
  }

  if (!canShowAgentApiKeys(agentType, { canView: true, canManage })) {
    return (
      <div className="flex h-full w-full items-center justify-center p-6">
        <div className="max-w-xl rounded-2xl border border-dashed bg-background p-8 text-center">
          <div className="mx-auto mb-4 flex size-12 items-center justify-center rounded-full bg-muted">
            <AlertCircle className="size-5 text-muted-foreground" />
          </div>
          <div className="text-lg font-semibold">{t('common.accessDenied')}</div>
          <div className="mt-2 text-sm text-muted-foreground">
            {t('common.unauthorizedDescription')}
          </div>
        </div>
      </div>
    );
  }

  return <>{children({ agentType })}</>;
}

export default AgentApiAccessGuard;
