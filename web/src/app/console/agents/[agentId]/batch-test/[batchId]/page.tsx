'use client';

import { use } from 'react';
import { AlertCircle, RefreshCcw } from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { BatchResultDetail } from '@/components/workflow-test/batch-result-detail';
import { useAgent } from '@/hooks/agent/use-agents';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import { canShowAgentBatchTest, supportsWorkflowDetailPages } from '@/utils/agent-detail-routes';
import { getErrorMessage } from '@/utils/error-notifications';
import { AGENT_MANAGE_PERMISSION_CODES } from '@/constants/permissions';

interface BatchResultPageProps {
  params: Promise<{
    agentId: string;
    batchId: string;
  }>;
}

export default function BatchResultPage({ params }: BatchResultPageProps) {
  const t = useT('agents.workflowTest.page');
  const tWebapp = useT('webapp');
  const tRoot = useT();
  const { agentId, batchId } = use(params);
  const { agent, isLoading, error, refetch } = useAgent(agentId);
  const { hasAnyPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canManage = hasAnyPermission(AGENT_MANAGE_PERMISSION_CODES);

  if (isLoading || isPermissionsLoading) {
    return (
      <div className="space-y-6 bg-slate-50 p-8">
        <Skeleton className="h-56 rounded-2xl" />
        <Skeleton className="h-96 rounded-2xl" />
      </div>
    );
  }

  if (error || !agent?.data) {
    return (
      <div className="flex h-full w-full items-center justify-center p-6">
        <div className="w-full max-w-xl">
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>{t('resultLoadFailed')}</AlertTitle>
            <AlertDescription>
              {error ? getErrorMessage(error) || t('agentLoadFailed') : t('agentNotFound')}
            </AlertDescription>
          </Alert>
          <Button className="mt-4" onClick={() => void refetch()}>
            <RefreshCcw className="mr-2 size-4" />
            {t('retry')}
          </Button>
        </div>
      </div>
    );
  }

  if (!supportsWorkflowDetailPages(agent.data.agent_type)) {
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

  if (!canShowAgentBatchTest(agent.data.agent_type, { canView: true, canManage })) {
    return (
      <div className="flex h-full w-full items-center justify-center p-6">
        <div className="max-w-xl rounded-2xl border border-dashed bg-background p-8 text-center">
          <div className="mx-auto mb-4 flex size-12 items-center justify-center rounded-full bg-muted">
            <AlertCircle className="size-5 text-muted-foreground" />
          </div>
          <div className="text-lg font-semibold">{tRoot('common.accessDenied')}</div>
          <div className="mt-2 text-sm text-muted-foreground">
            {tRoot('common.unauthorizedDescription')}
          </div>
        </div>
      </div>
    );
  }

  return <BatchResultDetail agentId={agentId} batchId={batchId} agentName={agent.data.name} />;
}
