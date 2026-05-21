import type { AutomationBodyType } from '@/services/types/automation';
import type { ValidationError, ValidationResult } from '../common/validation';
import {
  getMissingRequiredNotificationSMSTemplateParams,
  normalizeNotificationSMSTemplateKey,
  NOTIFICATION_SMS_TEMPLATE,
  type NotificationSMSTemplate,
  type NotificationSMSTemplateParam,
} from '@/lib/features/notification-sms';
import { generateClientId } from '@/utils/client-id';

export type CreateScheduledTaskScheduleType = 'once' | 'cron';
export type CreateScheduledTaskOnceInputMode = 'fixed' | 'variable';
export type CreateScheduledTaskActionType = 'send_notification' | 'run_workflow';
export type CreateScheduledTaskChannelType = 'email' | 'sms' | 'webhook';

export interface ScheduledTaskOnceConfig {
  input_mode: CreateScheduledTaskOnceInputMode;
  run_at: string;
}

export interface ScheduledTaskCronConfig {
  expr: string;
}

export interface ScheduledTaskNotificationDraft {
  recipients: string[];
  subject: string;
  body: string;
  body_type: AutomationBodyType;
  template: string;
  template_params: Record<string, string>;
}

export interface CreateScheduledTaskActionData {
  client_id: string;
  action_type: CreateScheduledTaskActionType;
  enabled: boolean;
  channel_type?: CreateScheduledTaskChannelType;
  notification?: ScheduledTaskNotificationDraft;
  raw_config?: Record<string, unknown> | null;
}

export interface CreateScheduledTaskActionValidationErrors {
  actionType?: ValidationError;
  channelType?: ValidationError;
  recipients?: ValidationError;
  template?: ValidationError;
  subject?: ValidationError;
  bodyType?: ValidationError;
  body?: ValidationError;
  templateParams?: Record<string, ValidationError | undefined>;
}

export interface CreateScheduledTaskNodeData {
  type: 'create-scheduled-task';
  title: string;
  desc: string;
  task: {
    name: string;
    description: string;
    schedule: {
      type: CreateScheduledTaskScheduleType;
      timezone: string;
      once: ScheduledTaskOnceConfig;
      cron: ScheduledTaskCronConfig;
    };
    actions: CreateScheduledTaskActionData[];
  };
  isInLoop: boolean;
  isInIteration: boolean;
}

export function createDefaultScheduledTaskNotificationDraft(): ScheduledTaskNotificationDraft {
  return {
    recipients: [''],
    subject: '',
    body: '',
    body_type: 'text/html',
    template: NOTIFICATION_SMS_TEMPLATE,
    template_params: {
      notification_title: '',
      link_code: '',
    },
  };
}

export function createDefaultScheduledTaskActionDraft(): CreateScheduledTaskActionData {
  return {
    client_id: generateClientId('scheduled-task-action'),
    action_type: 'send_notification',
    enabled: true,
    channel_type: 'email',
    notification: createDefaultScheduledTaskNotificationDraft(),
    raw_config: null,
  };
}

export function createDefaultCreateScheduledTaskNodeData(): CreateScheduledTaskNodeData {
  return {
    type: 'create-scheduled-task',
    title: '',
    desc: '',
    task: {
      name: '',
      description: '',
      schedule: {
        type: 'once',
        timezone: 'Asia/Shanghai',
        once: {
          input_mode: 'fixed',
          run_at: '',
        },
        cron: {
          expr: '',
        },
      },
      actions: [createDefaultScheduledTaskActionDraft()],
    },
    isInLoop: false,
    isInIteration: false,
  };
}

export const DEFAULT_CREATE_SCHEDULED_TASK_NODE_DATA = createDefaultCreateScheduledTaskNodeData();

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}

function isBodyType(value: unknown): value is AutomationBodyType {
  return value === 'text/html' || value === 'text/plain';
}

function isScheduleType(value: unknown): value is CreateScheduledTaskScheduleType {
  return value === 'once' || value === 'cron';
}

function isInputMode(value: unknown): value is CreateScheduledTaskOnceInputMode {
  return value === 'fixed' || value === 'variable';
}

function isActionType(value: unknown): value is CreateScheduledTaskActionType {
  return value === 'send_notification' || value === 'run_workflow';
}

function isChannelType(value: unknown): value is CreateScheduledTaskChannelType {
  return value === 'email' || value === 'sms' || value === 'webhook';
}

function normalizeRecipients(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return [''];
  }

  const recipients = value.filter((item): item is string => typeof item === 'string');
  return recipients.length > 0 ? recipients : [''];
}

function normalizeTemplateParams(value: Record<string, unknown>): Record<string, string> {
  return Object.fromEntries(
    Object.entries(value).filter((entry): entry is [string, string] => typeof entry[1] === 'string')
  );
}

function normalizeAction(action: unknown): CreateScheduledTaskActionData {
  const fallback = createDefaultScheduledTaskActionDraft();

  if (!isRecord(action)) {
    return fallback;
  }

  const notificationSource = isRecord(action.notification) ? action.notification : {};
  const templateParams = isRecord(notificationSource.template_params)
    ? notificationSource.template_params
    : {};
  const normalizedTemplateParams = normalizeTemplateParams(templateParams);
  return {
    client_id:
      typeof action.client_id === 'string' && action.client_id.trim()
        ? action.client_id
        : fallback.client_id,
    action_type: isActionType(action.action_type) ? action.action_type : fallback.action_type,
    enabled: typeof action.enabled === 'boolean' ? action.enabled : fallback.enabled,
    channel_type: isChannelType(action.channel_type) ? action.channel_type : fallback.channel_type,
    notification: {
      recipients: normalizeRecipients(
        notificationSource.recipients ?? action.recipients ?? fallback.notification?.recipients
      ),
      subject:
        typeof notificationSource.subject === 'string'
          ? notificationSource.subject
          : typeof action.subject === 'string'
            ? action.subject
            : (fallback.notification?.subject ?? ''),
      body:
        typeof notificationSource.body === 'string'
          ? notificationSource.body
          : typeof action.body === 'string'
            ? action.body
            : (fallback.notification?.body ?? ''),
      body_type: isBodyType(notificationSource.body_type)
        ? notificationSource.body_type
        : isBodyType(action.body_type)
          ? action.body_type
          : (fallback.notification?.body_type ?? 'text/html'),
      template: normalizeNotificationSMSTemplateKey(
        typeof notificationSource.template === 'string'
          ? notificationSource.template
          : fallback.notification?.template
      ),
      template_params: normalizedTemplateParams,
    },
    raw_config: isRecord(action.raw_config) ? action.raw_config : null,
  };
}

export function cloneCreateScheduledTaskActionData(
  action: CreateScheduledTaskActionData
): CreateScheduledTaskActionData {
  return normalizeAction({
    ...action,
    notification: action.notification
      ? {
          ...action.notification,
          recipients: [...action.notification.recipients],
        }
      : action.notification,
    raw_config: isRecord(action.raw_config) ? { ...action.raw_config } : action.raw_config,
  });
}

function normalizeActions(value: unknown): CreateScheduledTaskActionData[] {
  if (!Array.isArray(value)) {
    return [createDefaultScheduledTaskActionDraft()];
  }

  const actions = value.map(normalizeAction);
  return actions.length > 0 ? actions : [createDefaultScheduledTaskActionDraft()];
}

function isValidFixedOnceRunAt(value: string): boolean {
  if (!value.trim()) {
    return false;
  }

  const date = new Date(value);
  return !Number.isNaN(date.getTime());
}

export function normalizeCreateScheduledTaskNodeData(
  value: Partial<CreateScheduledTaskNodeData> | Record<string, unknown> | null | undefined
): CreateScheduledTaskNodeData {
  const fallback = createDefaultCreateScheduledTaskNodeData();

  if (!isRecord(value)) {
    return fallback;
  }

  const legacyValue = value as Record<string, unknown>;
  const task = isRecord(value.task) ? value.task : {};
  const schedule = isRecord(task.schedule) ? task.schedule : {};
  const once = isRecord(schedule.once) ? schedule.once : {};
  const cron = isRecord(schedule.cron) ? schedule.cron : {};

  const legacyScheduleType = legacyValue.schedule_type;
  const legacyRunAt = legacyValue.run_at;
  const legacyCronExpr = legacyValue.cron_expr;
  const legacyTimezone = legacyValue.timezone;

  const normalized: CreateScheduledTaskNodeData = {
    ...fallback,
    title: typeof value.title === 'string' ? value.title : fallback.title,
    desc: typeof value.desc === 'string' ? value.desc : fallback.desc,
    task: {
      name:
        typeof task.name === 'string'
          ? task.name
          : typeof legacyValue.task_name === 'string'
            ? legacyValue.task_name
            : fallback.task.name,
      description:
        typeof task.description === 'string'
          ? task.description
          : typeof legacyValue.task_description === 'string'
            ? legacyValue.task_description
            : fallback.task.description,
      schedule: {
        type: isScheduleType(schedule.type)
          ? schedule.type
          : isScheduleType(legacyScheduleType)
            ? legacyScheduleType
            : fallback.task.schedule.type,
        timezone:
          typeof schedule.timezone === 'string'
            ? schedule.timezone
            : typeof legacyTimezone === 'string'
              ? legacyTimezone
              : fallback.task.schedule.timezone,
        once: {
          input_mode: isInputMode(once.input_mode)
            ? once.input_mode
            : fallback.task.schedule.once.input_mode,
          run_at:
            typeof once.run_at === 'string'
              ? once.run_at
              : typeof legacyRunAt === 'string'
                ? legacyRunAt
                : fallback.task.schedule.once.run_at,
        },
        cron: {
          expr:
            typeof cron.expr === 'string'
              ? cron.expr
              : typeof legacyCronExpr === 'string'
                ? legacyCronExpr
                : fallback.task.schedule.cron.expr,
        },
      },
      actions: normalizeActions(task.actions ?? legacyValue.actions),
    },
    isInLoop: Boolean(value.isInLoop),
    isInIteration: Boolean(value.isInIteration),
  };

  return normalized;
}

const SUPPORTED_EMAIL_BODY_TYPES: AutomationBodyType[] = ['text/html', 'text/plain'];

export function getCreateScheduledTaskActionValidationErrors(
  action: CreateScheduledTaskActionData,
  index: number,
  smsTemplates: NotificationSMSTemplate[] = []
): CreateScheduledTaskActionValidationErrors {
  const params = { index: index + 1 };
  const errors: CreateScheduledTaskActionValidationErrors = {};

  if (action.action_type !== 'send_notification') {
    errors.actionType = {
      code: 'createScheduledTask.validation.unsupportedActionType',
      params,
    };
    return errors;
  }

  if (action.channel_type !== 'email' && action.channel_type !== 'sms') {
    errors.channelType = {
      code: 'createScheduledTask.validation.unsupportedChannelType',
      params,
    };
    return errors;
  }

  const notification = action.notification;

  if (!notification) {
    errors.body = { code: 'createScheduledTask.validation.bodyRequired', params };
    return errors;
  }

  const recipients = notification.recipients.map(item => item.trim()).filter(Boolean);

  if (recipients.length === 0) {
    errors.recipients = { code: 'createScheduledTask.validation.recipientsRequired', params };
  }

  if (action.channel_type === 'sms') {
    if (!notification.template.trim()) {
      errors.template = {
        code: 'createScheduledTask.validation.templateRequired',
        params,
      };
    }

    const missingParams = getMissingRequiredNotificationSMSTemplateParams(
      notification.template,
      notification.template_params,
      smsTemplates
    );
    if (missingParams.length > 0) {
      errors.templateParams = Object.fromEntries(
        missingParams.map(param => [
          param.key,
          getCreateScheduledTaskTemplateParamRequiredError(index, notification.template, param),
        ])
      );
    }

    return errors;
  }

  if (!notification.subject.trim()) {
    errors.subject = { code: 'createScheduledTask.validation.subjectRequired', params };
  }

  if (!SUPPORTED_EMAIL_BODY_TYPES.includes(notification.body_type)) {
    errors.bodyType = { code: 'createScheduledTask.validation.bodyTypeRequired', params };
  }

  if (!notification.body.trim()) {
    errors.body = { code: 'createScheduledTask.validation.bodyRequired', params };
  }

  return errors;
}

export function hasCreateScheduledTaskActionValidationErrors(
  errors: CreateScheduledTaskActionValidationErrors
): boolean {
  return Object.values(errors).some(Boolean);
}

export const checkValid = (
  data: CreateScheduledTaskNodeData,
  smsTemplates: NotificationSMSTemplate[] = []
): ValidationResult => {
  const normalized = normalizeCreateScheduledTaskNodeData(data);
  const errors: ValidationError[] = [];
  const warnings: ValidationError[] = [];

  if (!normalized.task.name.trim()) {
    errors.push({ code: 'createScheduledTask.validation.taskNameRequired' });
  }

  if (normalized.task.schedule.type === 'once') {
    if (
      !normalized.task.schedule.once.run_at.trim() ||
      (normalized.task.schedule.once.input_mode === 'fixed' &&
        !isValidFixedOnceRunAt(normalized.task.schedule.once.run_at))
    ) {
      errors.push({ code: 'createScheduledTask.validation.runAtRequired' });
    }
  } else if (!normalized.task.schedule.cron.expr.trim()) {
    errors.push({ code: 'createScheduledTask.validation.cronExprRequired' });
  }

  if (!Array.isArray(normalized.task.actions) || normalized.task.actions.length === 0) {
    errors.push({ code: 'createScheduledTask.validation.actionRequired' });
    return { isValid: false, errors, warnings };
  }

  const enabledActions = normalized.task.actions
    .map((action, index) => ({ action, index }))
    .filter(item => item.action.enabled);

  if (enabledActions.length === 0) {
    errors.push({ code: 'createScheduledTask.validation.enabledActionRequired' });
  }

  enabledActions.forEach(({ action, index }) => {
    const actionErrors = getCreateScheduledTaskActionValidationErrors(action, index, smsTemplates);
    errors.push(...flattenCreateScheduledTaskActionValidationErrors(actionErrors));
  });

  return { isValid: errors.length === 0, errors, warnings };
};

function getCreateScheduledTaskTemplateParamRequiredError(
  index: number,
  templateKey: string,
  param: NotificationSMSTemplateParam
): ValidationError {
  const params = { index: index + 1 };
  if (templateKey === NOTIFICATION_SMS_TEMPLATE) {
    if (param.key === 'notification_title') {
      return { code: 'createScheduledTask.validation.notificationTitleRequired', params };
    }
    if (param.key === 'link_code') {
      return { code: 'createScheduledTask.validation.linkCodeRequired', params };
    }
  }
  return {
    code: 'createScheduledTask.validation.templateParamRequired',
    params: { ...params, label: param.label?.trim() || param.key },
  };
}

function flattenCreateScheduledTaskActionValidationErrors(
  actionErrors: CreateScheduledTaskActionValidationErrors
): ValidationError[] {
  return [
    actionErrors.actionType,
    actionErrors.channelType,
    actionErrors.recipients,
    actionErrors.template,
    actionErrors.subject,
    actionErrors.bodyType,
    actionErrors.body,
    ...Object.values(actionErrors.templateParams ?? {}),
  ].filter((error): error is ValidationError => typeof error !== 'undefined');
}
