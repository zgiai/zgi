import type { ConversationTransport } from '@/components/chat/controllers/types';
import type { Message } from '@/components/chat/types';
import type { ApprovalRuntimeForm as ApprovalRuntimeFormData } from '@/services/approval.service';
import type { QuestionAnswerChoice, WorkflowPrecheckWarning } from '@/services/types/workflow';

export interface UseWebappConversationTransportOptions {
  enablePrecheck?: boolean;
}

export interface UseWebappConversationTransportResult {
  transport: ConversationTransport;
  precheckWarnings: WorkflowPrecheckWarning[];
  clearPrecheckWarnings: () => void;
  latestTaskId: string | null;
  approvalForm: ApprovalRuntimeFormData | null;
  approvalToken: string | null;
  approvalLoading: boolean;
  approvalError: unknown;
  approvalSubmitting: boolean;
  approvalSubmittedAction: string | null;
  questionAnswerPrompt: {
    question: string;
    choices: QuestionAnswerChoice[];
    round?: number;
  } | null;
  questionAnswerSubmitting: boolean;
  syncQuestionAnswerRuntime: (conversationId?: string) => void;
  submitApproval: (payload: { inputs: Record<string, unknown>; action: string }) => Promise<void>;
  submitQuestionAnswerChoice: (
    conversationId: string,
    choice: QuestionAnswerChoice
  ) => Promise<void>;
  retryApprovalForm: () => void;
  resumeWorkflowRun: (conversationId: string, message: Message) => void;
  continueWorkflowRun: (conversationId: string, message: Message) => void;
}
