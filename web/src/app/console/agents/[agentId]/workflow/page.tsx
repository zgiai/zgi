'use client';

import { use } from 'react';
import { useSearchParams } from 'next/navigation';
import WorkflowEditor from '@/components/workflow';
import { useAgent } from '@/hooks/agent/use-agents';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { WorkflowSkeleton } from '@/components/workflow/ui/workflow-skeleton';
import { useT } from '@/i18n';
import { Alert, AlertTitle, AlertDescription } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { RefreshCcw, AlertCircle } from 'lucide-react';
import { getErrorMessage } from '@/utils/error-notifications';
import { PermissionDeniedState } from '@/components/common/permission-gate-state';
import { WORKFLOW_PERMISSION_ACTIONS } from '@/constants/permissions';
import { supportsWorkflowDetailPages } from '@/utils/agent-detail-routes';

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
  const { hasAnyPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canCreateWorkflow = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.create);
  const canImportWorkflow = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.import);
  const canOpenWorkflowEditor =
    canCreateWorkflow ||
    canImportWorkflow ||
    hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.update) ||
    hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.runDraft) ||
    hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.runStop) ||
    hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.debug) ||
    hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.publish) ||
    hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.runtimeConfigManage) ||
    hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.runtimeAccessManage);
  const { agent, isLoading, error, refetch } = useAgent(agentId, canOpenWorkflowEditor);

  // Initial loading skeleton
  if (isPermissionsLoading || (canOpenWorkflowEditor && isLoading)) {
    return <WorkflowSkeleton />;
  }

  if (!canOpenWorkflowEditor) {
    return <PermissionDeniedState />;
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
  if (!supportsWorkflowDetailPages(agent.data.agent_type)) {
    return <PermissionDeniedState />;
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
