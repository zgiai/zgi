/**
 * What the person must resolve before the conversation can go on (ADR 0050).
 *
 * The composer, the queue and the message actions all read this one value,
 * so none of them can send, regenerate or edit over a turn that waits for the
 * person.
 */
export type PendingInteraction =
  | { kind: 'workflow_approval' }
  | { kind: 'tool_approval'; approvalId?: string }
  | { kind: 'question'; requestId?: string }
  | { kind: 'page_action' }
  | { kind: 'budget' };

export interface PendingInteractionInput {
  /** Status of the conversation's current leaf message. */
  leafStatus?: string | null;
  /** A workflow approval form is waiting. */
  workflowApproval: boolean;
  /** The tool approval the composer would show, if any. */
  toolApprovalId?: string | null;
  /** The question the composer would show, if any. */
  questionRequestId?: string | null;
  /** The question has questions to answer. */
  hasQuestion: boolean;
}

/**
 * Returns the one interaction to resolve, in the order the composer shows
 * them: a workflow approval, a tool approval, a question. A leaf message that
 * waits without a panel to show (a page action, more budget, or an approval or
 * question whose panel is not available on this surface) still counts.
 */
export function selectPendingInteraction(
  input: PendingInteractionInput
): PendingInteraction | null {
  if (input.workflowApproval) return { kind: 'workflow_approval' };
  if (input.toolApprovalId) return { kind: 'tool_approval', approvalId: input.toolApprovalId };
  if (input.hasQuestion) {
    return {
      kind: 'question',
      ...(input.questionRequestId ? { requestId: input.questionRequestId } : {}),
    };
  }
  switch (input.leafStatus) {
    case 'waiting_approval':
      return { kind: 'tool_approval' };
    case 'waiting_question':
      return { kind: 'question' };
    case 'waiting_client_action':
      return { kind: 'page_action' };
    case 'waiting_budget':
      return { kind: 'budget' };
    default:
      return null;
  }
}
