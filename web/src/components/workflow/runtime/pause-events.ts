import {
  parseApprovalPausedEvent,
  type ParsedApprovalPausedEvent,
} from '../approval/runtime-events';
import {
  parseQuestionAnswerPausedEvent,
  type ParsedQuestionAnswerPaused,
} from '../question-answer/runtime-events';

export interface ParsedWorkflowPausedEvent {
  approval: ParsedApprovalPausedEvent;
  questionAnswer: ParsedQuestionAnswerPaused;
  hasApproval: boolean;
  hasQuestionAnswer: boolean;
  preferredStatus: 'pending_approval' | 'pending_question' | null;
}

/**
 * Parses every actionable reason from a workflow pause.
 *
 * A single V2 pause may contain approval and question-answer reasons from
 * parallel branches. Consumers must apply both projections instead of treating
 * the pause as one mutually exclusive interaction type.
 */
export function parseWorkflowPausedEvent(payload: unknown): ParsedWorkflowPausedEvent {
  const approval = parseApprovalPausedEvent(payload);
  const questionAnswer = parseQuestionAnswerPausedEvent(payload);
  const hasApproval = approval.isApproval;
  const hasQuestionAnswer = questionAnswer.isQuestionAnswer;

  return {
    approval,
    questionAnswer,
    hasApproval,
    hasQuestionAnswer,
    // Keep the existing persisted-message compatibility rule: an actionable
    // question takes precedence over approval in the scalar runtime status.
    preferredStatus: hasQuestionAnswer
      ? 'pending_question'
      : hasApproval
        ? 'pending_approval'
        : null,
  };
}
