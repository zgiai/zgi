import type { DigitalHumanRun, DigitalHumanRunStatus } from '@/services/types/digital-human';

/**
 * Every status a DigitalHuman surface can observe while work is still in
 * flight. It covers the durable run statuses plus the ChatRuntime message
 * statuses the runtime-log projection returns for a suspended turn, so the
 * runs list, run detail, dashboard, and chat transport all keep polling a
 * `waiting` run instead of treating it as finished.
 */
const ACTIVE_DIGITAL_HUMAN_RUN_STATUSES: ReadonlySet<string> = new Set([
  'queued',
  'running',
  'waiting',
  'canceling',
  'streaming',
  'loading',
  'pending',
  'pending_approval',
  'waiting_approval',
  'pending_question',
  'waiting_question',
  'pending_client_action',
  'waiting_client_action',
  'waiting_budget',
]);

export function isActiveDigitalHumanRunStatus(status?: string | null): boolean {
  const normalized = status?.trim().toLowerCase();
  return normalized ? ACTIVE_DIGITAL_HUMAN_RUN_STATUSES.has(normalized) : false;
}

const WAITING_DIGITAL_HUMAN_RUN_STATUSES: ReadonlySet<string> = new Set([
  'waiting',
  'pending_approval',
  'waiting_approval',
  'pending_question',
  'waiting_question',
  'pending_client_action',
  'waiting_client_action',
  'waiting_budget',
]);

/**
 * A run parked on a person: an approval, a question, a client action, or more
 * budget. It is still polled, but it is not working and holds no place among
 * running work; it belongs with what needs attention.
 */
export function isWaitingDigitalHumanRunStatus(status?: string | null): boolean {
  const normalized = status?.trim().toLowerCase();
  return normalized ? WAITING_DIGITAL_HUMAN_RUN_STATUSES.has(normalized) : false;
}

/** A run that is actually working now, as opposed to waiting on someone. */
export function isRunningDigitalHumanRunStatus(status?: string | null): boolean {
  return isActiveDigitalHumanRunStatus(status) && !isWaitingDigitalHumanRunStatus(status);
}

/**
 * Translation keys under the `digitalHumans.status` namespace. A waiting run
 * with a known reason gets a reason-specific label so the console says what it
 * is waiting for; every other status falls back to its plain status label.
 */
export type DigitalHumanRunStatusLabelKey =
  | `status.${DigitalHumanRunStatus}`
  | 'status.waitingApproval'
  | 'status.waitingQuestion'
  | 'status.waitingClientAction';

export function digitalHumanRunStatusLabelKey(
  status: DigitalHumanRunStatus,
  waitingReason?: DigitalHumanRun['waiting_reason']
): DigitalHumanRunStatusLabelKey {
  if (status !== 'waiting') return `status.${status}`;
  switch (waitingReason) {
    case 'waiting_approval':
      return 'status.waitingApproval';
    case 'waiting_question':
      return 'status.waitingQuestion';
    case 'waiting_client_action':
      return 'status.waitingClientAction';
    default:
      return 'status.waiting';
  }
}

/**
 * The receipt a future approval card must submit to resume this run. The
 * current console intentionally has no confirmation action yet: this accessor
 * is the typed seam between the durable waiting state and that future card.
 */
export function digitalHumanRunApprovalReceipt(run: DigitalHumanRun): string | null {
  if (run.status !== 'waiting' || run.waiting_reason !== 'waiting_approval') return null;
  return run.waiting_receipt ?? null;
}

function runEventContext(run: DigitalHumanRun): Record<string, unknown> | null {
  const context = run.invocation_event?.context;
  return context && typeof context === 'object' && !Array.isArray(context) ? (context as Record<string, unknown>) : null;
}

/**
 * The name of the task a run executes, or null for other runs. A task run's
 * query is the instruction written for the model, so people see the task.
 */
export function digitalHumanRunTaskName(run: DigitalHumanRun): string | null {
  const name = runEventContext(run)?.task_name;
  return typeof name === 'string' && name.trim() ? name.trim() : null;
}

/** The task a run executes, when the run carries it. */
export function digitalHumanRunTaskId(run: DigitalHumanRun): string | null {
  const taskId = runEventContext(run)?.task_id;
  return typeof taskId === 'string' && taskId.trim() ? taskId.trim() : null;
}

/** Whether a person, rather than the schedule, started this task run. */
export function digitalHumanRunStartedByPerson(run: DigitalHumanRun): boolean {
  return runEventContext(run)?.task_trigger === 'manual_run';
}

/** A one-line plain-text preview of Markdown, for lists and summaries. */
export function plainTextPreview(text?: string | null): string {
  if (!text) return '';
  return text
    .replace(/```[\s\S]*?```/g, ' ')
    .replace(/!\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/^\s{0,3}(#{1,6}|>|[-*+]|\d+\.)\s+/gm, '')
    .replace(/(\*\*|__|\*|_|`|~~)/g, '')
    .replace(/\s+/g, ' ')
    .trim();
}
