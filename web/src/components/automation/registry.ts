import type { LucideIcon } from 'lucide-react';
import { BellRing, Clock3, Mail, Repeat2, Smartphone, Workflow } from 'lucide-react';
import type { TaskRecurringMode, TaskStatusFilterKey, TaskWeekdayKey } from './types';

export const TASK_STATUS_FILTERS: Array<{
  key: TaskStatusFilterKey;
  query: string;
  labelKey: string;
}> = [
  { key: 'all', query: 'all', labelKey: 'filters.all' },
  { key: 'active', query: 'active', labelKey: 'filters.active' },
  { key: 'paused', query: 'paused', labelKey: 'filters.paused' },
  { key: 'completed', query: 'completed', labelKey: 'filters.completed' },
  { key: 'archived', query: 'archived', labelKey: 'filters.archived' },
];

export const TASK_WEEKDAYS: Array<{
  key: TaskWeekdayKey;
  cronValue: string;
  labelKey: string;
}> = [
  { key: 'mon', cronValue: '1', labelKey: 'schedule.weekdays.mon' },
  { key: 'tue', cronValue: '2', labelKey: 'schedule.weekdays.tue' },
  { key: 'wed', cronValue: '3', labelKey: 'schedule.weekdays.wed' },
  { key: 'thu', cronValue: '4', labelKey: 'schedule.weekdays.thu' },
  { key: 'fri', cronValue: '5', labelKey: 'schedule.weekdays.fri' },
  { key: 'sat', cronValue: '6', labelKey: 'schedule.weekdays.sat' },
  { key: 'sun', cronValue: '0', labelKey: 'schedule.weekdays.sun' },
];

export const scheduleTypeRegistry: Record<
  'once' | 'cron',
  { icon: LucideIcon; labelKey: string }
> = {
  once: {
    icon: Clock3,
    labelKey: 'schedule.once',
  },
  cron: {
    icon: Repeat2,
    labelKey: 'schedule.recurring',
  },
};

export const recurringModeRegistry: Record<
  TaskRecurringMode,
  { icon: LucideIcon; labelKey: string }
> = {
  daily: {
    icon: Repeat2,
    labelKey: 'schedule.daily',
  },
  weekly: {
    icon: Repeat2,
    labelKey: 'schedule.weekly',
  },
  customCron: {
    icon: Repeat2,
    labelKey: 'schedule.customCron',
  },
};

export interface TaskActionTypeOption {
  value: string;
  icon: LucideIcon;
  labelKey: string;
  descriptionKey: string;
  channelTypes: string[];
}

export interface TaskChannelTypeOption {
  value: string;
  icon: LucideIcon;
  labelKey: string;
  descriptionKey: string;
}

export const actionTypeRegistry: Record<string, TaskActionTypeOption> = {
  send_notification: {
    value: 'send_notification',
    icon: BellRing,
    labelKey: 'actions.sendNotification',
    descriptionKey: 'actions.sendNotificationDescription',
    channelTypes: ['email', 'sms'],
  },
  run_workflow: {
    value: 'run_workflow',
    icon: Workflow,
    labelKey: 'actions.runWorkflow',
    descriptionKey: 'actions.runWorkflowDescription',
    channelTypes: [],
  },
};

export const channelTypeRegistry: Record<string, TaskChannelTypeOption> = {
  email: {
    value: 'email',
    icon: Mail,
    labelKey: 'actions.email',
    descriptionKey: 'actions.emailDescription',
  },
  sms: {
    value: 'sms',
    icon: Smartphone,
    labelKey: 'actions.sms',
    descriptionKey: 'actions.smsDescription',
  },
};

export const actionTypeOptions = Object.values(actionTypeRegistry);

export const channelTypeOptions = Object.values(channelTypeRegistry);
