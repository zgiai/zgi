import { useMutation } from '@tanstack/react-query';
import { workflowService } from '@/services/workflow.service';
import { toast } from 'sonner';
import { useT } from '@/i18n';

interface StopWorkflowTaskParams {
  agentId: string;
  workflowRunId: string;
}

/**
 * Hook for stopping a running workflow task
 * POST /console/api/agents/{agent_id}/workflow-runs/tasks/{workflow_run_id}/stop
 */
export function useStopWorkflowTask() {
  const t = useT('agents');

  return useMutation({
    mutationFn: ({ agentId, workflowRunId }: StopWorkflowTaskParams) =>
      workflowService.stopWorkflowTask(agentId, workflowRunId),
    onError: error => {
      const message = (error as { message?: string }).message || t('workflow.stopFailed');
      toast.error(message);
      console.error('Stop workflow task error:', error);
    },
    onSuccess: () => {
      toast.success(t('workflow.stopSuccess'));
    },
  });
}

export default useStopWorkflowTask;
