import type {
  DigitalHumanTask,
  DigitalHumanTaskStatus,
} from '@/services/types/digital-human';

export type DigitalHumanTaskDisplayStatus =
  | DigitalHumanTaskStatus
  | 'paused'
  | 'recurring_active'
  | 'manual_ready';

export function digitalHumanTaskDisplayStatus(
  task: DigitalHumanTask
): DigitalHumanTaskDisplayStatus {
  if (task.lifecycle_status === 'paused' && task.work_status === 'pending') {
    return 'paused';
  }
  if (task.lifecycle_status === 'active' && task.work_status === 'pending') {
    if (task.schedule.type === 'cron') return 'recurring_active';
    if (task.schedule.type === 'manual') return 'manual_ready';
  }
  return task.work_status;
}
