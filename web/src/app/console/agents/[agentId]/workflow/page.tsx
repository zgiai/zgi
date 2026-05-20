'use client';

import { use } from 'react';
import { useSearchParams } from 'next/navigation';
import WorkflowEditor from '@/components/workflow';
import { useAgent } from '@/hooks/agent/use-agents';
import { WorkflowSkeleton } from '@/components/workflow/ui/workflow-skeleton';
import { useT } from '@/i18n';
import { Alert, AlertTitle, AlertDescription } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { RefreshCcw, AlertCircle } from 'lucide-react';
import { getErrorMessage } from '@/utils/error-notifications';

interface WorkflowPageProps {
  params: Promise<{
    agentId: string;
  }>;
}

const WorkflowPage: React.FC<WorkflowPageProps> = ({ params }) => {
  const t = useT();
  const { agentId } = use(params);
  const searchParams = useSearchParams();
  const focusNodeId = searchParams.get('nodeId') || undefined;
  const { agent, isLoading, error, refetch } = useAgent(agentId);

  // Initial loading skeleton
  if (isLoading) {
    return <WorkflowSkeleton />;
  }

  // Error or empty state with i18n and retry
  if (error || !agent?.data) {
    const message = error ? getErrorMessage(error) : '';
    return (
      <div className="w-full h-full flex items-center justify-center p-6">
        <div className="max-w-xl w-full">
          <Alert variant="destructive">
            <AlertCircle className="h-4 w-4" />
            <AlertTitle>
              {error ? t('agents.workflow.loadFailedTitle') : t('agents.workflow.notFoundTitle')}
            </AlertTitle>
            <AlertDescription>
              {error
                ? message || t('agents.workflow.loadFailedDesc')
                : t('agents.workflow.notFoundDesc')}
            </AlertDescription>
          </Alert>
          <div className="mt-4 flex gap-2">
            <Button
              variant="default"
              onClick={() => {
                void refetch();
              }}
            >
              <RefreshCcw className="h-4 w-4 mr-2" />
              {t('agents.actions.retry')}
            </Button>
          </div>
        </div>
      </div>
    );
  }
  return (
    <div className="w-full h-full">
      <WorkflowEditor
        key={`${agentId}:${focusNodeId || 'default'}`}
        agentDetail={agent?.data}
        isDetailLoading={isLoading}
        focusNodeId={focusNodeId}
      />
    </div>
  );
};

export default WorkflowPage;
