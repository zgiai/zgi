'use client';

import { use } from 'react';
import { AlertCircle, RefreshCcw } from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { BatchResultItemDetailPage } from '@/components/workflow-test/batch-result-item-detail-page';
import { useAgent } from '@/hooks/agent/use-agents';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';

interface BatchResultItemPageProps {
  params: Promise<{
    agentId: string;
    batchId: string;
    itemId: string;
  }>;
}

export default function BatchResultItemPage({ params }: BatchResultItemPageProps) {
  const t = useT('agents.workflowTest.page');
  const { agentId, batchId, itemId } = use(params);
  const { agent, isLoading, error, refetch } = useAgent(agentId);

  if (isLoading) {
    return (
      <div className="space-y-6 bg-slate-50 p-8">
        <Skeleton className="h-56 rounded-2xl" />
        <Skeleton className="h-[720px] rounded-2xl" />
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

  return <BatchResultItemDetailPage agentId={agentId} batchId={batchId} itemId={itemId} agentName={agent.data.name} />;
}
