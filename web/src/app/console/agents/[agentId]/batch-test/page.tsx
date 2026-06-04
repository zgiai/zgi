'use client';

import { use } from 'react';
import { AlertCircle, RefreshCcw } from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { BatchTestOverview } from '@/components/workflow-test/batch-test-overview';
import { useAgent } from '@/hooks/agent/use-agents';
import { useT } from '@/i18n';
import { canShowWorkflowDetailPages } from '@/utils/agent-detail-routes';
import { getErrorMessage } from '@/utils/error-notifications';

interface BatchTestPageProps {
  params: Promise<{
    agentId: string;
  }>;
}

export default function BatchTestPage({ params }: BatchTestPageProps) {
  const t = useT('agents.workflowTest.page');
  const tWebapp = useT('webapp');
  const { agentId } = use(params);
  const { agent, isLoading, error, refetch } = useAgent(agentId);

  if (isLoading) {
    return (
      <div className="space-y-6 bg-slate-50 p-8">
        <Skeleton className="h-44 rounded-2xl" />
        <Skeleton className="h-52 rounded-2xl" />
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

  if (!canShowWorkflowDetailPages(agent.data.agent_type)) {
    return (
      <div className="flex h-full w-full items-center justify-center p-6">
        <div className="max-w-xl rounded-2xl border border-dashed bg-background p-8 text-center">
          <div className="mx-auto mb-4 flex size-12 items-center justify-center rounded-full bg-muted">
            <AlertCircle className="size-5 text-muted-foreground" />
          </div>
          <div className="text-lg font-semibold">
            {tWebapp('appCenter.appUnavailableTitle')}
          </div>
          <div className="mt-2 text-sm text-muted-foreground">
            {tWebapp('appCenter.appUnavailableDescription')}
          </div>
        </div>
      </div>
    );
  }

  return (
    <BatchTestOverview
      agentId={agentId}
      agentName={agent.data.name}
      agentDescription={agent.data.description}
    />
  );
}
