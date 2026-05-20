'use client';

import { use } from 'react';
import { AlertCircle, RefreshCcw } from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { CreateBatchPage } from '@/components/workflow-test/create-batch-page';
import { useAgent } from '@/hooks/agent/use-agents';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';

interface NewBatchTestPageProps {
  params: Promise<{
    agentId: string;
  }>;
}

export default function NewBatchTestPage({ params }: NewBatchTestPageProps) {
  const t = useT('agents.workflowTest.page');
  const { agentId } = use(params);
  const { agent, isLoading, error, refetch } = useAgent(agentId);

  if (isLoading) {
    return (
      <div className="space-y-6 bg-slate-50 p-8">
        <Skeleton className="h-36 rounded-2xl" />
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

  return (
    <CreateBatchPage
      agentId={agentId}
      agentName={agent.data.name}
      agentDescription={agent.data.description}
    />
  );
}
