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
