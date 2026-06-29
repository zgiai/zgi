'use client';

import { use } from 'react';
import { AlertCircle, RefreshCcw } from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { CreateBatchPage } from '@/components/workflow-test/create-batch-page';
import { useAgent } from '@/hooks/agent/use-agents';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import { canShowAgentBatchTest, supportsWorkflowDetailPages } from '@/utils/agent-detail-routes';
import { getErrorMessage } from '@/utils/error-notifications';
import { WORKFLOW_PERMISSION_ACTIONS } from '@/constants/permissions';

interface NewBatchTestPageProps {
  params: Promise<{
    agentId: string;
  }>;
}

export default function NewBatchTestPage({ params }: NewBatchTestPageProps) {
  const t = useT('agents.workflowTest.page');
  const tWebapp = useT('webapp');
  const tRoot = useT();
  const { agentId } = use(params);
  const { hasAnyPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canViewBatchTestLibrary = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.view);
  const canUpdateBatchTest = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.update);
  const canDebugBatchTest = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.debug);
  const canViewBatchTestLogs = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.logsView);
  const canCreateAndRunBatch =
    canViewBatchTestLibrary && canViewBatchTestLogs && canUpdateBatchTest && canDebugBatchTest;
  const { agent, isLoading, error, refetch } = useAgent(agentId, canCreateAndRunBatch);

  if (isPermissionsLoading || (canCreateAndRunBatch && isLoading)) {
    return (
      <div className="space-y-6 bg-slate-50 p-8">
        <Skeleton className="h-36 rounded-2xl" />
        <Skeleton className="h-96 rounded-2xl" />
      </div>
    );
  }

  if (!canCreateAndRunBatch) {
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

  if (error || !agent?.data) {
    return (
      <div className="flex h-full w-full items-center justify-center p-6">
        <div className="w-full max-w-xl">
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>{t('overviewLoadFailed')}</AlertTitle>
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

  const supportsBatchTest = supportsWorkflowDetailPages(agent.data.agent_type);
  const canShowBatchTest = canShowAgentBatchTest(agent.data.agent_type, {
      canView: true,
      canViewBatchTest: canCreateAndRunBatch,
      canRunBatchTest: canDebugBatchTest,
  });
  if (!supportsBatchTest || !canShowBatchTest) {
    return (
      <div className="flex h-full w-full items-center justify-center p-6">
        <div className="max-w-xl rounded-2xl border border-dashed bg-background p-8 text-center">
          <div className="mx-auto mb-4 flex size-12 items-center justify-center rounded-full bg-muted">
            <AlertCircle className="size-5 text-muted-foreground" />
          </div>
          <div className="text-lg font-semibold">
            {supportsBatchTest ? tRoot('common.accessDenied') : tWebapp('appCenter.appUnavailableTitle')}
          </div>
          <div className="mt-2 text-sm text-muted-foreground">
            {supportsBatchTest
              ? tRoot('common.unauthorizedDescription')
              : tWebapp('appCenter.appUnavailableDescription')}
          </div>
        </div>
      </div>
    );
  }

  return (
    <CreateBatchPage
      agentId={agentId}
      agentName={agent.data.name}
      agentDescription={agent.data.description}
    />
  );
}
