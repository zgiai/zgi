/**
 * The closed status vocabulary of the chat client (ADR 0050).
 *
 * The backend and older stored messages use many words for the same state:
 * `success`, `succeeded`, `completed` and `allowed` all mean the call
 * finished. Every place that interprets a status maps it here first, so an
 * unknown word reads as unknown instead of falling into a default branch that
 * means something else.
 */

/** Where a tool call, skill load or connector operation stands. */
export type InvocationOutcome =
  | 'running'
  | 'waiting'
  | 'succeeded'
  | 'failed'
  | 'cancelled'
  | 'unknown';

const INVOCATION_OUTCOMES: Record<string, InvocationOutcome> = {
  pending: 'running',
  queued: 'running',
  loading: 'running',
  running: 'running',
  streaming: 'running',
  in_progress: 'running',
  submitting: 'running',

  needs_approval: 'waiting',
  needs_resolution: 'waiting',
  waiting_approval: 'waiting',
  waiting_question: 'waiting',
  waiting_client_action: 'waiting',
  waiting_budget: 'waiting',
  requires_confirmation: 'waiting',
  reload_required: 'waiting',

  success: 'succeeded',
  succeeded: 'succeeded',
  completed: 'succeeded',
  complete: 'succeeded',
  ok: 'succeeded',
  allowed: 'succeeded',
  approved: 'succeeded',
  loaded: 'succeeded',
  already_loaded: 'succeeded',
  auto_restored: 'succeeded',
  advisory: 'succeeded',
  skipped: 'succeeded',

  error: 'failed',
  failed: 'failed',
  failure: 'failed',
  blocked: 'failed',
  denied: 'failed',
  rejected: 'failed',
  timed_out: 'failed',
  timeout: 'failed',
  expired: 'failed',

  cancelled: 'cancelled',
  canceled: 'cancelled',
  stopped: 'cancelled',
  interrupted: 'cancelled',
};

/** Maps any status word to its outcome; a word not listed here is `unknown`. */
export function invocationOutcome(status: unknown): InvocationOutcome {
  if (typeof status !== 'string') return 'unknown';
  return INVOCATION_OUTCOMES[status.trim().toLowerCase()] ?? 'unknown';
}

/**
 * The outcomes a call may move to. Live events, the message_end snapshot and
 * a reloaded copy of the message can reach the client in any order, so an
 * older report must not undo a newer one: a waiting call does not go back to
 * running (the work after an approval is a call of its own), and a finished
 * call only changes when a later report says it failed.
 */
const INVOCATION_TRANSITIONS: Record<InvocationOutcome, ReadonlySet<InvocationOutcome>> = {
  unknown: new Set(['unknown', 'running', 'waiting', 'succeeded', 'failed', 'cancelled']),
  running: new Set(['running', 'waiting', 'succeeded', 'failed', 'cancelled']),
  waiting: new Set(['waiting', 'succeeded', 'failed', 'cancelled']),
  succeeded: new Set(['succeeded', 'failed']),
  cancelled: new Set(['cancelled', 'succeeded', 'failed']),
  failed: new Set(['failed']),
};

/** Reports that a call whose status is `from` may take the status `to`. */
export function canInvocationMoveTo(from: unknown, to: unknown): boolean {
  return INVOCATION_TRANSITIONS[invocationOutcome(from)].has(invocationOutcome(to));
}

/** A tool governance decision the client acts on. */
export type GovernanceDecisionStatus =
  | 'allowed'
  | 'needs_approval'
  | 'needs_resolution'
  | 'denied'
  | 'blocked'
  | 'error';

const GOVERNANCE_DECISIONS: ReadonlySet<string> = new Set<GovernanceDecisionStatus>([
  'allowed',
  'needs_approval',
  'needs_resolution',
  'denied',
  'blocked',
  'error',
]);

/**
 * Reads a governance decision from the words a decision event carries.
 * `null` means the event names no decision the client knows; callers must not
 * read that as a request for approval unless the event says approval is
 * required.
 */
export function governanceDecisionStatus(
  decision: unknown,
  approvalStatus?: unknown
): GovernanceDecisionStatus | null {
  const approval = typeof approvalStatus === 'string' ? approvalStatus.trim().toLowerCase() : '';
  if (approval === 'approved') return 'allowed';
  if (approval === 'rejected') return 'denied';
  const word = typeof decision === 'string' ? decision.trim().toLowerCase() : '';
  if (GOVERNANCE_DECISIONS.has(word)) return word as GovernanceDecisionStatus;
  switch (invocationOutcome(word)) {
    case 'succeeded':
      return 'allowed';
    case 'failed':
      return 'error';
    case 'waiting':
      return 'needs_approval';
    default:
      return null;
  }
}
