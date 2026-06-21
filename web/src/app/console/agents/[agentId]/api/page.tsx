'use client';

import { useParams } from 'next/navigation';
import { AlertCircle, Loader2 } from 'lucide-react';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs';
import ApiKeysTab from '@/components/agents/api/api-keys-tab';
import ApiDocsTab from '@/components/agents/api/api-docs-tab';
import RuntimeAccessTab from '@/components/agents/api/runtime-access-tab';
import { useAgent } from '@/hooks/agent/use-agents';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import type { AgentType } from '@/services/types/agent';
import { useT } from '@/i18n';
import {
  canShowAgentApiKeys,
  canShowAgentRuntimeAccess,
  supportsAgentRuntimeLogs,
} from '@/utils/agent-detail-routes';
import { getErrorMessage } from '@/utils/error-notifications';

export default function AgentApiPage() {
  const { agentId } = useParams<{ agentId: string }>();
  const t = useT();
  const tWebapp = useT('webapp');

  const { agent, isLoading, error } = useAgent(agentId);
  const { hasPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canManage = hasPermission('agent.manage');
  const agentType = (agent?.data?.agent_type as AgentType | undefined) ?? undefined;
  const canShowWorkflowApiTabs = canShowAgentApiKeys(agentType, { canView: true, canManage });
  const canShowRuntimeAccess = canShowAgentRuntimeAccess(agentType, { canView: true, canManage });

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

  if (!supportsAgentRuntimeLogs(agentType)) {
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

  if (!canShowRuntimeAccess) {
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

  return (
    <div className="p-4 space-y-4">
      <Tabs
        defaultValue={canShowWorkflowApiTabs ? 'api-keys' : 'runtime-access'}
        className="w-full"
      >
        <TabsList>
          {canShowWorkflowApiTabs ? (
            <TabsTrigger value="api-keys">{t('agents.apiKeys.navTitle')}</TabsTrigger>
          ) : null}
          <TabsTrigger value="runtime-access">{t('agents.runtimeAccess.navTitle')}</TabsTrigger>
          {canShowWorkflowApiTabs ? (
            <TabsTrigger value="api-docs">{t('agents.apiTitle')}</TabsTrigger>
          ) : null}
        </TabsList>
        {canShowWorkflowApiTabs ? (
          <TabsContent value="api-keys">
            <ApiKeysTab agentId={agentId} />
          </TabsContent>
        ) : null}
        <TabsContent value="runtime-access">
          <RuntimeAccessTab agentId={agentId} canManage={canManage} />
        </TabsContent>
        {canShowWorkflowApiTabs ? (
          <TabsContent value="api-docs">
            <ApiDocsTab agentType={agentType} />
          </TabsContent>
        ) : null}
      </Tabs>
    </div>
  );
}
