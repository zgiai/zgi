import { useMutation, useQueryClient } from '@tanstack/react-query';
import { workflowService } from '@/services/workflow.service';
import type { WorkflowNodeRunRequest, WorkflowNodeRunResponse } from '@/services/types/workflow';
import { WORKFLOW_KEYS } from '@/hooks/query-keys';

interface RunWorkflowNodeDraftParams {
  agentId: string;
  nodeId: string;
  payload: WorkflowNodeRunRequest;
}

export function useRunWorkflowNodeDraft() {
  const queryClient = useQueryClient();

  return useMutation<WorkflowNodeRunResponse, Error, RunWorkflowNodeDraftParams>({
    mutationFn: ({ agentId, nodeId, payload }) =>
      workflowService.runDraftWorkflowNode(agentId, nodeId, payload),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: WORKFLOW_KEYS.runs(variables.agentId) });
    },
  });
}

export default useRunWorkflowNodeDraft;
