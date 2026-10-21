import type { useT } from '@/i18n';
import { translateTimezone } from '@/utils/timezone';
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

/** The viewer's own time zone; task times are shown in it. */
export function viewerTimeZone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  } catch {
    return 'UTC';
  }
}

/**
 * A task time in the viewer's zone, such as "Oct 21, 7:30 PM". The year is
 * shown only when it is not the current one.
 */
export function formatTaskTime(value: string | null | undefined, locale?: string): string {
  if (!value) return '—';
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) return value;
  const options: Intl.DateTimeFormatOptions = { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' };
  if (date.getFullYear() !== new Date().getFullYear()) options.year = 'numeric';
  return new Intl.DateTimeFormat(locale, options).format(date);
}

export function digitalHumanTaskScheduleSummary(
  task: DigitalHumanTask,
  t: ReturnType<typeof useT<'digitalHumans'>>,
  locale?: string
): string {
  const schedule = task.schedule;
  if (schedule.type === 'manual') return t('tasks.schedule.manual');
  if (schedule.type === 'once') {
    return `${t('tasks.schedule.once')} · ${formatTaskTime(schedule.run_at, locale)}`;
  }
  const description = describeCronSchedule(schedule.cron, t);
  // A recurring rule keeps its own zone; name it only when it differs from
  // the viewer's, since every other time on the page is in the viewer's.
  return schedule.timezone && schedule.timezone !== viewerTimeZone()
    ? `${description} · ${translateTimezone(schedule.timezone, locale)}`
    : description;
}

function describeCronSchedule(
  expression: string | undefined,
  t: ReturnType<typeof useT<'digitalHumans'>>
): string {
  const fields = expression?.trim().split(/\s+/) ?? [];
  if (fields.length !== 5) return t('tasks.schedule.custom');

  const [minutes, hours, dayOfMonth, month, dayOfWeek] = fields;
  const simpleTime = parseSimpleCronTime(minutes, hours);
  if (simpleTime && dayOfMonth === '*' && month === '*') {
    if (dayOfWeek === '*') return t('tasks.schedule.daily', { time: simpleTime });

    const weekdays = parseCronWeekdays(dayOfWeek);
    if (weekdays.length > 0) {
      const weekdayLabels: Record<string, string> = {
        sun: t('tasks.weekdays.sun'), mon: t('tasks.weekdays.mon'),
        tue: t('tasks.weekdays.tue'), wed: t('tasks.weekdays.wed'),
        thu: t('tasks.weekdays.thu'), fri: t('tasks.weekdays.fri'), sat: t('tasks.weekdays.sat'),
      };
      if (weekdays.join(',') === 'mon,tue,wed,thu,fri') {
        return t('tasks.schedule.weekdays', { time: simpleTime });
      }
      return t('tasks.schedule.weekly', {
        days: weekdays.map(day => weekdayLabels[day]).join(', '), time: simpleTime,
      });
    }
  }

  const everyMinutes = /^\*(\/(\d+))?$/.exec(minutes);
  if (everyMinutes?.[2] && hours === '*' && dayOfMonth === '*' && month === '*' && dayOfWeek === '*') {
    return t('tasks.schedule.everyMinutes', { value: everyMinutes[2] });
  }
  const everyHours = minutes === '0' ? /^\*(\/(\d+))?$/.exec(hours) : null;
  if (everyHours?.[2] && dayOfMonth === '*' && month === '*' && dayOfWeek === '*') {
    return t('tasks.schedule.everyHours', { value: everyHours[2] });
  }
  if (simpleTime && /^\d{1,2}$/.test(dayOfMonth) && month === '*' && dayOfWeek === '*') {
    return t('tasks.schedule.monthly', { day: String(Number(dayOfMonth)), time: simpleTime });
  }
  return t('tasks.schedule.custom');
}

function parseSimpleCronTime(minutes: string, hours: string): string | null {
  if (!/^\d{1,2}$/.test(minutes) || !/^\d{1,2}$/.test(hours)) return null;
  const minute = Number(minutes);
  const hour = Number(hours);
  if (minute > 59 || hour > 23) return null;
  return `${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')}`;
}

function parseCronWeekdays(value: string): string[] {
  const aliases = ['sun', 'mon', 'tue', 'wed', 'thu', 'fri', 'sat'];
  const result = new Set<number>();
  for (const token of value.split(',')) {
    const normalized = token.trim().toLowerCase();
    const range = normalized.split('-');
    if (range.length === 2) {
      const start = parseCronWeekday(range[0]);
      const end = parseCronWeekday(range[1]);
      if (start !== null && end !== null && start <= end) {
        for (let day = start; day <= end; day += 1) result.add(day);
      }
      continue;
    }
    const day = parseCronWeekday(normalized);
    if (day !== null) result.add(day);
  }
  return [...result].sort((a, b) => a - b).map(day => aliases[day]);
}

function parseCronWeekday(value: string): number | null {
  const aliases: Record<string, number> = {
    sun: 0, mon: 1, tue: 2, wed: 3, thu: 4, fri: 5, sat: 6,
  };
  if (value in aliases) return aliases[value];
  if (!/^\d$/.test(value)) return null;
  const day = Number(value);
  return day === 7 ? 0 : day <= 6 ? day : null;
}
