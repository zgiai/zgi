'use client';

import { useMutation } from '@tanstack/react-query';
import { workflowService } from '@/services/workflow.service';
import type {
  WorkflowChatDraftPrecheckRequest,
  WorkflowDraftPrecheckRequest,
} from '@/services/workflow.service';

/**
 * @hook useWorkflowDraftPrecheck
 * @description Runs workflow draft precheck before a manual debug execution.
 */
export function useWorkflowDraftPrecheck(agentId: string) {
  return useMutation({
    mutationFn: (payload: WorkflowDraftPrecheckRequest) =>
      workflowService.precheckWorkflowDraft(agentId, payload),
  });
}

/**
 * @hook useWorkflowChatDraftPrecheck
 * @description Runs advanced-chat draft precheck before a manual debug send.
 */
export function useWorkflowChatDraftPrecheck(agentId: string) {
  return useMutation({
    mutationFn: (payload: WorkflowChatDraftPrecheckRequest) =>
      workflowService.precheckWorkflowChatDraft(agentId, payload),
  });
}
