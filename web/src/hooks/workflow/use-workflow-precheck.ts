'use client';

import { useMutation } from '@tanstack/react-query';
import { workflowService } from '@/services/workflow.service';
import type { WorkflowRunInputValues } from '@/services/workflow.service';
import type { ChatAttachment } from '@/components/chat/types';

/**
 * @hook useWorkflowDraftPrecheck
 * @description Runs workflow draft precheck before a manual debug execution.
 */
export function useWorkflowDraftPrecheck(agentId: string) {
  return useMutation({
    mutationFn: (payload: { inputs?: WorkflowRunInputValues }) =>
      workflowService.precheckWorkflowDraft(agentId, payload),
  });
}

/**
 * @hook useWorkflowChatDraftPrecheck
 * @description Runs advanced-chat draft precheck before a manual debug send.
 */
export function useWorkflowChatDraftPrecheck(agentId: string) {
  return useMutation({
    mutationFn: (payload: {
      query: string;
      conversation_id?: string;
      history_window_size?: number;
      files?: ChatAttachment[];
      inputs?: Record<string, unknown>;
    }) => workflowService.precheckWorkflowChatDraft(agentId, payload),
  });
}
