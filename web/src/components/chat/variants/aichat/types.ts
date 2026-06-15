import type { ModelSelectorValue } from '@/components/common/model-selector';
import type { ApprovalRuntimeForm } from '@/services/approval.service';

export const MAX_AICHAT_BRANCHES = 5;

export interface AIChatModelValue extends ModelSelectorValue {
  params?: Record<string, number | string | boolean | string[]>;
}

export interface AIChatSuggestion {
  key: string;
  text: string;
}

export interface AIChatWorkflowApprovalRequest {
  conversationId: string;
  messageId: string;
  workflowRunId?: string;
  approvalToken: string;
  approvalUrl?: string;
  approvalFormId?: string;
  approvalForm?: ApprovalRuntimeForm | null;
}

export interface AIChatWorkflowApprovalSubmitPayload {
  inputs: Record<string, unknown>;
  action: string;
}
