import { formatDate, formatDurationSeconds } from '@/utils/format';
import { isValidEmail } from '@/utils/validation';
import { generateClientId } from '@/utils/client-id';
import { isNotificationSMSLinkCodeValid } from '@/components/notification-sms/validation';
import { NOTIFICATION_SMS_TEMPLATE } from '@/lib/features/notification-sms';
import type { FormInputs } from '@/components/workflow/common/workflow-input-form';
import { InputVarType, type InputVar } from '@/components/workflow/types/input-var';
import type {
  AutomationRunWorkflowActionConfig,
  AutomationNotificationActionConfig,
  AutomationWorkflowVersionStrategy,
  AutomationRunStatus,
  AutomationTask,
  AutomationTaskAction,
  AutomationTaskDetailData,
  AutomationTaskListItem,
  AutomationTaskRun,
  AutomationTaskStatus,
  CreateAutomationTaskRequest,
  GeneratedAutomationTaskDraft,
  UpdateAutomationTaskRequest,
} from '@/services/types/automation';
import { actionTypeRegistry, TASK_STATUS_FILTERS, TASK_WEEKDAYS } from './registry';
import type {
  TaskDraft,
  TaskDraftAction,
  TaskDraftActionErrors,
  TaskPanelTab,
  TaskRecurringMode,
  TaskScheduleSummary,
  TaskStatusFilterKey,
  TaskValidationErrors,
  TaskWeekdayKey,
} from './types';

const DEFAULT_TIME = '09:00';
const DEFAULT_WORKFLOW_TIMEOUT_SECONDS = 120;
const MIN_WORKFLOW_TIMEOUT_SECONDS = 30;
const MAX_WORKFLOW_TIMEOUT_SECONDS = 1800;
const DEFAULT_WORKFLOW_INPUTS_JSON = '{}';
const SENSITIVE_WORKFLOW_INPUT_KEY_PATTERN =
  /(api[_-]?key|authorization|credential|password|secret|token)/i;

const RUN_STATUS_VARIANT: Record<
  AutomationRunStatus,
  'secondary' | 'success' | 'warning' | 'destructive' | 'info'
> = {
  queued: 'info',
  running: 'warning',
  succeeded: 'success',
  failed: 'destructive',
  cancelled: 'secondary',
};

export function getBrowserTimezone(): string {
  if (typeof Intl === 'undefined') {
    return '';
  }

  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone ?? '';
  } catch {
    return '';
  }
}

export function isValidTimeZone(timeZone?: string | null): boolean {
  const normalized = timeZone?.trim();

  if (!normalized || typeof Intl === 'undefined') {
    return false;
  }

  try {
    new Intl.DateTimeFormat('en-US', { timeZone: normalized }).format(new Date());
    return true;
  } catch {
    return false;
  }
}

function pad(value: number): string {
  return String(value).padStart(2, '0');
}

function parseLocalDateTime(localDateTime: string): {
  year: number;
  month: number;
  day: number;
  hour: number;
  minute: number;
} | null {
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/.exec(localDateTime.trim());

  if (!match) {
    return null;
  }

  const [, year, month, day, hour, minute] = match;

  return {
    year: Number(year),
    month: Number(month),
    day: Number(day),
    hour: Number(hour),
    minute: Number(minute),
  };
}

function serializeCronWeekdays(days: TaskWeekdayKey[]): string {
  const values = TASK_WEEKDAYS.filter(day => days.includes(day.key)).map(day => day.cronValue);
  return values.join(',');
}

function parseCronWeekdays(token: string): TaskWeekdayKey[] {
  const normalizedTokens = token
    .split(',')
    .map(item => item.trim().toUpperCase())
    .filter(Boolean);

  const lookup: Record<string, TaskWeekdayKey> = {
    MON: 'mon',
    TUE: 'tue',
    WED: 'wed',
    THU: 'thu',
    FRI: 'fri',
    SAT: 'sat',
    SUN: 'sun',
    '0': 'sun',
    '7': 'sun',
    '1': 'mon',
    '2': 'tue',
    '3': 'wed',
    '4': 'thu',
    '5': 'fri',
    '6': 'sat',
  };

  return normalizedTokens
    .map(item => lookup[item])
    .filter((item): item is TaskWeekdayKey => Boolean(item));
}

function parseCronExpression(expr: string): {
  minutes: string;
  hours: string;
  dayOfMonth: string;
  month: string;
  dayOfWeek: string;
} | null {
  const parts = expr
    .trim()
    .split(/\s+/)
    .map(item => item.trim())
    .filter(Boolean);

  if (parts.length !== 5) {
    return null;
  }

  const [minutes, hours, dayOfMonth, month, dayOfWeek] = parts;

  return {
    minutes,
    hours,
    dayOfMonth,
    month,
    dayOfWeek,
  };
}

function getRecurringModeFromCron(expr: string): {
  recurringMode: TaskRecurringMode;
  recurringTime: string;
  recurringDays: TaskWeekdayKey[];
} {
  const parsed = parseCronExpression(expr);

  if (!parsed) {
    return {
      recurringMode: 'customCron',
      recurringTime: DEFAULT_TIME,
      recurringDays: ['mon'],
    };
  }

  const hasSimpleTime = /^\d{1,2}$/.test(parsed.hours) && /^\d{1,2}$/.test(parsed.minutes);
  const recurringTime = hasSimpleTime
    ? `${pad(Number(parsed.hours))}:${pad(Number(parsed.minutes))}`
    : DEFAULT_TIME;

  if (
    hasSimpleTime &&
    parsed.dayOfMonth === '*' &&
    parsed.month === '*' &&
    parsed.dayOfWeek === '*'
  ) {
    return {
      recurringMode: 'daily',
      recurringTime,
      recurringDays: ['mon'],
    };
  }

  const recurringDays = parseCronWeekdays(parsed.dayOfWeek);

  if (
    hasSimpleTime &&
    parsed.dayOfMonth === '*' &&
    parsed.month === '*' &&
    recurringDays.length > 0
  ) {
    return {
      recurringMode: 'weekly',
      recurringTime,
      recurringDays,
    };
  }

  return {
    recurringMode: 'customCron',
    recurringTime,
    recurringDays: recurringDays.length > 0 ? recurringDays : ['mon'],
  };
}

function getCronExpressionFromDraft(draft: TaskDraft): string {
  const { recurringMode, recurringTime, recurringDays, cronExpr } = draft.schedule;

  if (recurringMode === 'customCron') {
    return cronExpr.trim();
  }

  const [hoursText, minutesText] = recurringTime.split(':');
  const hours = pad(Number(hoursText ?? '0'));
  const minutes = pad(Number(minutesText ?? '0'));

  if (recurringMode === 'daily') {
    return `${Number(minutes)} ${Number(hours)} * * *`;
  }

  return `${Number(minutes)} ${Number(hours)} * * ${serializeCronWeekdays(recurringDays)}`;
}

function extractRawObject(value: unknown): Record<string, unknown> | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return null;
  }

  return value as Record<string, unknown>;
}

function isKnownScheduleType(value: string): value is 'once' | 'cron' {
  return value === 'once' || value === 'cron';
}

function isKnownActionType(value: string): value is 'send_notification' | 'run_workflow' {
  return value === 'send_notification' || value === 'run_workflow';
}

function isKnownChannelType(value: string): value is 'email' | 'sms' {
  return value === 'email' || value === 'sms';
}

function normalizeRecipients(recipients: string[]): string[] {
  return recipients.map(item => item.trim()).filter(Boolean);
}

function isWorkflowVersionStrategy(value: unknown): value is AutomationWorkflowVersionStrategy {
  return value === 'latest_published' || value === 'pinned';
}

function stringifyWorkflowInputs(value: unknown): string {
  const inputValue = extractRawObject(value) ?? {};
  return JSON.stringify(inputValue, null, 2);
}

function parseWorkflowInputsJson(value: string): Record<string, unknown> | null {
  const trimmed = value.trim();

  if (!trimmed) {
    return {};
  }

  try {
    const parsed = JSON.parse(trimmed);
    return extractRawObject(parsed);
  } catch {
    return null;
  }
}

function hasSensitiveWorkflowInputKey(value: Record<string, unknown>): boolean {
  return Object.keys(value).some(key => SENSITIVE_WORKFLOW_INPUT_KEY_PATTERN.test(key));
}

function getWorkflowTimeoutSeconds(value: string): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? Math.floor(parsed) : DEFAULT_WORKFLOW_TIMEOUT_SECONDS;
}

export function workflowFormValuesToJson(values: FormInputs): string {
  const normalized = Object.entries(values).reduce<Record<string, unknown>>((acc, [key, value]) => {
    if (value === undefined) {
      return acc;
    }

    acc[key] = value;
    return acc;
  }, {});

  return JSON.stringify(normalized, null, 2);
}

export function parseWorkflowInputsJsonToFormValues(value: string): FormInputs {
  return (parseWorkflowInputsJson(value) ?? {}) as FormInputs;
}

export function getUnsupportedWorkflowInputVariables(variables: InputVar[]): InputVar[] {
  return variables.filter(
    variable => variable.type === InputVarType.FILE || variable.type === InputVarType.FILE_LIST
  );
}

function hasWorkflowInputValue(value: FormInputs[string]): boolean {
  if (value === undefined || value === null) {
    return false;
  }

  if (typeof value === 'string') {
    return value.trim().length > 0;
  }

  if (Array.isArray(value)) {
    return value.length > 0;
  }

  if (typeof value === 'boolean') {
    return value === true;
  }

  return true;
}

export function getMissingRequiredWorkflowInputVariables(
  variables: InputVar[],
  values: FormInputs
): InputVar[] {
  return variables.filter(variable => {
    if (!variable.required) {
      return false;
    }

    return !hasWorkflowInputValue(values[variable.variable]);
  });
}

function formatTaskDateTimeWithPattern(
  value: number | string,
  pattern: string,
  timeZone?: string
): string {
  const normalizedTimeZone = timeZone?.trim();

  if (normalizedTimeZone && isValidTimeZone(normalizedTimeZone)) {
    try {
      return formatDate(value, pattern, { timezone: normalizedTimeZone });
    } catch {
      // Fall through to the user's local timezone.
    }
  }

  try {
    return formatDate(value, pattern);
  } catch {
    return 'Invalid Date';
  }
}

function getRunTimestamp(run: AutomationTaskRun): number | string | null {
  return run.finished_at ?? run.started_at ?? run.scheduled_for ?? run.created_at ?? null;
}

export function formatTaskDateTime(value?: number | string | null, timeZone?: string): string {
  if (!value) {
    return '-';
  }

  return formatTaskDateTimeWithPattern(value, 'YYYY-MM-DD HH:mm', timeZone);
}

export function formatTaskDateTimeLocal(value?: number | string | null, timeZone?: string): string {
  if (!value) {
    return '';
  }

  return formatTaskDateTimeWithPattern(value, 'YYYY-MM-DDTHH:mm', timeZone);
}

function getTimeZoneWallParts(
  date: Date,
  timeZone: string
): {
  year: number;
  month: number;
  day: number;
  hour: number;
  minute: number;
  second: number;
} | null {
  try {
    const parts = new Intl.DateTimeFormat('en-US', {
      timeZone,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
      hourCycle: 'h23',
    }).formatToParts(date);
    const values = Object.fromEntries(parts.map(part => [part.type, part.value]));

    return {
      year: Number(values.year),
      month: Number(values.month),
      day: Number(values.day),
      hour: Number(values.hour),
      minute: Number(values.minute),
      second: Number(values.second),
    };
  } catch {
    return null;
  }
}

function getTimeZoneOffsetMs(date: Date, timeZone: string): number | null {
  const parts = getTimeZoneWallParts(date, timeZone);

  if (!parts) {
    return null;
  }

  const wallTimeAsUtc = Date.UTC(
    parts.year,
    parts.month - 1,
    parts.day,
    parts.hour,
    parts.minute,
    parts.second
  );

  return wallTimeAsUtc - date.getTime();
}

function zonedLocalDateTimeToUnixSeconds(localDateTime: string, timeZone: string): number | null {
  const parsed = parseLocalDateTime(localDateTime);

  if (!parsed || !isValidTimeZone(timeZone)) {
    return null;
  }

  const wallTimeAsUtc = Date.UTC(
    parsed.year,
    parsed.month - 1,
    parsed.day,
    parsed.hour,
    parsed.minute,
    0,
    0
  );
  let instantMs = wallTimeAsUtc;

  for (let index = 0; index < 3; index += 1) {
    const offsetMs = getTimeZoneOffsetMs(new Date(instantMs), timeZone);
    if (offsetMs === null) {
      return null;
    }
    const nextInstantMs = wallTimeAsUtc - offsetMs;
    if (nextInstantMs === instantMs) {
      break;
    }
    instantMs = nextInstantMs;
  }

  return Math.floor(instantMs / 1000);
}

export function localDateTimeToUnixSeconds(localDateTime: string, timeZone?: string): number {
  const normalizedTimeZone = timeZone?.trim();
  const zonedValue = normalizedTimeZone
    ? zonedLocalDateTimeToUnixSeconds(localDateTime, normalizedTimeZone)
    : null;

  if (typeof zonedValue === 'number') {
    return zonedValue;
  }

  const parsed = parseLocalDateTime(localDateTime);

  if (!parsed) {
    const direct = new Date(localDateTime).getTime();
    return Number.isFinite(direct) ? Math.floor(direct / 1000) : 0;
  }

  return Math.floor(
    new Date(
      parsed.year,
      parsed.month - 1,
      parsed.day,
      parsed.hour,
      parsed.minute,
      0,
      0
    ).getTime() / 1000
  );
}

export function localDateTimeToOffsetDateTime(localDateTime: string): string {
  const parsed = parseLocalDateTime(localDateTime);
  const date = parsed
    ? new Date(parsed.year, parsed.month - 1, parsed.day, parsed.hour, parsed.minute, 0, 0)
    : new Date(localDateTime);

  if (!Number.isFinite(date.getTime())) {
    return '';
  }

  const offsetMinutes = -date.getTimezoneOffset();
  const sign = offsetMinutes >= 0 ? '+' : '-';
  const absOffsetMinutes = Math.abs(offsetMinutes);
  const offsetHours = Math.floor(absOffsetMinutes / 60);
  const offsetRemainderMinutes = absOffsetMinutes % 60;

  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(
    date.getHours()
  )}:${pad(date.getMinutes())}:${pad(date.getSeconds())}${sign}${pad(offsetHours)}:${pad(
    offsetRemainderMinutes
  )}`;
}

export function getStatusFilterQuery(filterKey: TaskStatusFilterKey): string {
  return TASK_STATUS_FILTERS.find(item => item.key === filterKey)?.query ?? '';
}

export function createDefaultTaskDraft(): TaskDraft {
  return {
    name: '',
    description: '',
    schedule: {
      scheduleType: 'cron',
      timezone: getBrowserTimezone(),
      onceRunAt: '',
      recurringMode: 'daily',
      recurringTime: DEFAULT_TIME,
      recurringDays: ['mon'],
      cronExpr: '0 9 * * 1',
      rawConfig: null,
    },
    actions: [],
    support: {
      hasUnknownAction: false,
      hasUnknownChannel: false,
      hasUnknownSchedule: false,
      isEditable: true,
    },
  };
}

export function createDefaultTaskActionDraft(
  actionType: string = 'send_notification'
): TaskDraftAction {
  const isRunWorkflow = actionType === 'run_workflow';

  return {
    clientId: generateClientId(),
    actionType,
    channelType: isRunWorkflow ? '' : (actionTypeRegistry[actionType]?.channelTypes[0] ?? 'email'),
    enabled: true,
    recipients: [''],
    subject: '',
    bodyType: 'text/html',
    body: '',
    smsNotificationTitle: '',
    smsLinkCode: '',
    workflowAgentId: '',
    workflowVersionStrategy: 'latest_published',
    workflowVersionUuid: '',
    workflowInputsJson: DEFAULT_WORKFLOW_INPUTS_JSON,
    workflowTimeoutSeconds: String(DEFAULT_WORKFLOW_TIMEOUT_SECONDS),
    rawConfig: null,
  };
}

export function taskDetailToDraft(taskDetail: AutomationTaskDetailData): TaskDraft {
  const task = taskDetail.task;
  const orderedActions = [...taskDetail.actions].sort(
    (left, right) => (left.action_order ?? 0) - (right.action_order ?? 0)
  );
  const action = orderedActions[0];
  const rawScheduleType = String((task as { schedule_type?: string }).schedule_type ?? '');
  const rawActionType = String((action as { action_type?: string } | undefined)?.action_type ?? '');
  const rawChannelType = String(
    (action as { config?: { channel_type?: string } } | undefined)?.config?.channel_type ?? ''
  );

  const hasUnknownSchedule = !isKnownScheduleType(rawScheduleType);
  const hasUnknownAction = Boolean(action) && !isKnownActionType(rawActionType);
  const hasUnknownChannel =
    Boolean(action) && rawActionType === 'send_notification' && !isKnownChannelType(rawChannelType);
  const draft = createDefaultTaskDraft();

  draft.name = task.name;
  draft.description = task.description ?? '';
  draft.schedule.timezone = task.timezone ?? '';
  draft.support = {
    hasUnknownSchedule,
    hasUnknownAction,
    hasUnknownChannel,
    isEditable: !hasUnknownSchedule && !hasUnknownAction && !hasUnknownChannel,
  };

  if (rawScheduleType === 'once') {
    draft.schedule.scheduleType = 'once';
    draft.schedule.onceRunAt = formatTaskDateTimeLocal(
      (task.schedule_config as { run_at?: number | string }).run_at ?? '',
      draft.schedule.timezone
    );
    draft.schedule.rawConfig = extractRawObject(task.schedule_config);
  } else if (rawScheduleType === 'cron') {
    const cronExpr = (task.schedule_config as { cron_expr?: string }).cron_expr ?? '';
    const recurring = getRecurringModeFromCron(cronExpr);

    draft.schedule.scheduleType = 'cron';
    draft.schedule.cronExpr = cronExpr;
    draft.schedule.recurringMode = recurring.recurringMode;
    draft.schedule.recurringTime = recurring.recurringTime;
    draft.schedule.recurringDays = recurring.recurringDays;
    draft.schedule.rawConfig = extractRawObject(task.schedule_config);
  } else {
    draft.schedule.scheduleType = 'cron';
    draft.schedule.rawConfig = extractRawObject(task.schedule_config);
  }

  draft.actions = orderedActions.length
    ? orderedActions.map(item => {
        const actionType = String((item as { action_type?: string }).action_type ?? '');

        if (actionType === 'run_workflow') {
          const config = item.config as AutomationRunWorkflowActionConfig;
          const versionStrategy = isWorkflowVersionStrategy(config.workflow_ref?.version_strategy)
            ? config.workflow_ref.version_strategy
            : 'latest_published';

          return {
            ...createDefaultTaskActionDraft('run_workflow'),
            clientId: item.id || generateClientId(),
            enabled: item.enabled ?? true,
            workflowAgentId: config.workflow_ref?.agent_id ?? '',
            workflowVersionStrategy: versionStrategy,
            workflowVersionUuid: config.workflow_ref?.version_uuid ?? '',
            workflowInputsJson: stringifyWorkflowInputs(config.inputs),
            workflowTimeoutSeconds: String(
              config.execution?.timeout_seconds ?? DEFAULT_WORKFLOW_TIMEOUT_SECONDS
            ),
            rawConfig: extractRawObject(item.config),
          };
        }

        const notificationConfig =
          item.action_type === 'send_notification'
            ? item.config
            : {
                channel_type: 'email' as const,
                to: [],
                subject: '',
                body_type: 'text/html' as const,
                body: '',
              };
        const channelType = String(
          (item as { config?: { channel_type?: string } }).config?.channel_type ?? ''
        );
        const templateParams =
          channelType === 'sms' && 'template_params' in notificationConfig
            ? notificationConfig.template_params
            : null;

        return {
          ...createDefaultTaskActionDraft('send_notification'),
          clientId: item.id || generateClientId(),
          actionType,
          channelType,
          enabled: item.enabled ?? true,
          recipients: notificationConfig.to?.length ? [...notificationConfig.to] : [''],
          subject: 'subject' in notificationConfig ? (notificationConfig.subject ?? '') : '',
          bodyType:
            'body_type' in notificationConfig
              ? (notificationConfig.body_type ?? 'text/html')
              : 'text/html',
          body:
            ('body' in notificationConfig ? notificationConfig.body : undefined) ??
            (notificationConfig as { content?: string } | undefined)?.content ??
            '',
          smsNotificationTitle: templateParams?.notification_title ?? '',
          smsLinkCode: templateParams?.link_suffix ?? '',
          rawConfig: extractRawObject(item.config),
        };
      })
    : draft.actions;

  return draft;
}

export function draftToCreateRequest(
  draft: TaskDraft,
  workspaceId?: string
): CreateAutomationTaskRequest {
  const actions: CreateAutomationTaskRequest['actions'] = draft.actions.map((action, index) => {
    const base = {
      action_order: index + 1,
      enabled: action.enabled,
    };

    if (action.actionType === 'run_workflow') {
      const inputs = parseWorkflowInputsJson(action.workflowInputsJson) ?? {};
      const timeoutSeconds = getWorkflowTimeoutSeconds(action.workflowTimeoutSeconds);

      return {
        ...base,
        action_type: 'run_workflow',
        config: {
          workflow_ref: {
            agent_id: action.workflowAgentId.trim(),
            version_strategy: action.workflowVersionStrategy,
            ...(action.workflowVersionStrategy === 'pinned'
              ? { version_uuid: action.workflowVersionUuid.trim() }
              : {}),
          },
          inputs,
          execution: {
            timeout_seconds: timeoutSeconds,
          },
        },
      };
    }

    if (action.channelType === 'sms') {
      return {
        ...base,
        action_type: 'send_notification',
        config: {
          channel_type: 'sms',
          to: normalizeRecipients(action.recipients),
          template: NOTIFICATION_SMS_TEMPLATE,
          template_params: {
            notification_title: action.smsNotificationTitle.trim(),
            link_suffix: action.smsLinkCode.trim(),
          },
        },
      };
    }

    return {
      ...base,
      action_type: 'send_notification',
      config: {
        channel_type: 'email',
        to: normalizeRecipients(action.recipients),
        subject: action.subject.trim(),
        body_type: action.bodyType,
        body: action.body,
      },
    };
  });

  const base = {
    workspace_id: workspaceId,
    name: draft.name.trim(),
    description: draft.description.trim() || null,
    schedule_type: draft.schedule.scheduleType,
    ...(draft.schedule.timezone.trim() ? { timezone: draft.schedule.timezone.trim() } : {}),
    actions,
  } as const;

  if (draft.schedule.scheduleType === 'once') {
    return {
      ...base,
      schedule_config: {
        run_at: localDateTimeToUnixSeconds(draft.schedule.onceRunAt, draft.schedule.timezone),
        // run_at: localDateTimeToOffsetDateTime(draft.schedule.onceRunAt),
      },
    };
  }

  return {
    ...base,
    schedule_config: {
      cron_expr: getCronExpressionFromDraft(draft),
    },
  };
}

export function draftToUpdateRequest(
  draft: TaskDraft,
  workspaceId?: string
): UpdateAutomationTaskRequest {
  return draftToCreateRequest(draft, workspaceId);
}

export function generatedTaskDraftToDraft(generated: GeneratedAutomationTaskDraft): TaskDraft {
  return taskDetailToDraft({
    task: {
      id: 'generated',
      name: generated.name ?? '',
      description: generated.description ?? '',
      status: 'draft',
      schedule_type: generated.schedule_type,
      timezone: generated.timezone ?? '',
      schedule_config: generated.schedule_config,
    },
    actions: (generated.actions ?? []).map((action, index): AutomationTaskAction => {
      const base = {
        id: generateClientId(),
        task_id: 'generated',
        action_order: action.action_order ?? index + 1,
        enabled: action.enabled ?? true,
      };

      if (action.action_type === 'run_workflow') {
        return {
          ...base,
          action_type: 'run_workflow',
          config: action.config as AutomationRunWorkflowActionConfig,
        };
      }

      return {
        ...base,
        action_type: 'send_notification',
        config: action.config as AutomationNotificationActionConfig,
      };
    }),
  });
}

export function validateTaskDraft(
  draft: TaskDraft,
  t: (key: string) => string
): TaskValidationErrors {
  const errors: TaskValidationErrors = {};

  if (!draft.name.trim()) {
    errors.name = t('editor.validation.nameRequired');
  }

  if (draft.schedule.scheduleType === 'once' && !draft.schedule.onceRunAt.trim()) {
    errors.onceRunAt = t('editor.validation.runAtRequired');
  }

  if (draft.schedule.timezone.trim() && !isValidTimeZone(draft.schedule.timezone)) {
    errors.timezone = t('editor.validation.timezoneInvalid');
  }

  if (draft.schedule.scheduleType === 'cron') {
    if (!draft.schedule.recurringTime.trim()) {
      errors.recurringTime = t('editor.validation.timeRequired');
    }

    if (draft.schedule.recurringMode === 'weekly' && draft.schedule.recurringDays.length === 0) {
      errors.recurringDays = t('editor.validation.weeklyDaysRequired');
    }

    if (draft.schedule.recurringMode === 'customCron') {
      if (!draft.schedule.cronExpr.trim()) {
        errors.cronExpr = t('editor.validation.cronRequired');
      } else if (draft.schedule.cronExpr.trim().split(/\s+/).length !== 5) {
        errors.cronExpr = t('editor.validation.cronInvalid');
      }
    }
  }

  if (draft.actions.length === 0) {
    errors.actionsRequired = t('editor.validation.actionsRequired');
    return errors;
  }

  const actionErrors: Record<string, TaskDraftActionErrors> = {};

  draft.actions.forEach(action => {
    const currentErrors: TaskDraftActionErrors = {};

    if (!action.actionType.trim()) {
      currentErrors.actionType = t('editor.validation.actionTypeRequired');
    }

    if (action.actionType === 'run_workflow') {
      if (!action.workflowAgentId.trim()) {
        currentErrors.workflowAgentId = t('editor.validation.workflowAgentRequired');
      }

      if (action.workflowVersionStrategy === 'pinned' && !action.workflowVersionUuid.trim()) {
        currentErrors.workflowVersionUuid = t('editor.validation.workflowVersionUuidRequired');
      }

      const inputs = parseWorkflowInputsJson(action.workflowInputsJson);

      if (inputs === null) {
        currentErrors.workflowInputsJson = t('editor.validation.workflowInputsInvalid');
      } else if (hasSensitiveWorkflowInputKey(inputs)) {
        currentErrors.workflowInputsJson = t('editor.validation.workflowInputsSensitive');
      }

      const timeoutSeconds = Number(action.workflowTimeoutSeconds);

      if (
        !Number.isInteger(timeoutSeconds) ||
        timeoutSeconds < MIN_WORKFLOW_TIMEOUT_SECONDS ||
        timeoutSeconds > MAX_WORKFLOW_TIMEOUT_SECONDS
      ) {
        currentErrors.workflowTimeoutSeconds = t('editor.validation.workflowTimeoutInvalid');
      }
    } else {
      if (!action.channelType.trim()) {
        currentErrors.channelType = t('editor.validation.channelTypeRequired');
      }

      const recipients = normalizeRecipients(action.recipients ?? []);

      if (recipients.length === 0) {
        currentErrors.recipients = t('editor.validation.recipientsRequired');
      } else if (action.channelType === 'email' && recipients.some(email => !isValidEmail(email))) {
        currentErrors.recipients = t('editor.validation.recipientsInvalid');
      }

      if (action.channelType === 'sms') {
        if (!action.smsNotificationTitle.trim()) {
          currentErrors.smsNotificationTitle = t('editor.validation.smsNotificationTitleRequired');
        }

        if (!action.smsLinkCode.trim()) {
          currentErrors.smsLinkCode = t('editor.validation.smsLinkCodeRequired');
        } else if (!isNotificationSMSLinkCodeValid(action.smsLinkCode)) {
          currentErrors.smsLinkCode = t('editor.validation.smsLinkCodeInvalid');
        }
      } else {
        if (!action.subject.trim()) {
          currentErrors.subject = t('editor.validation.subjectRequired');
        }

        if (!action.body.trim()) {
          currentErrors.body = t('editor.validation.contentRequired');
        }
      }
    }

    if (Object.keys(currentErrors).length > 0) {
      actionErrors[action.clientId] = currentErrors;
    }
  });

  if (Object.keys(actionErrors).length > 0) {
    errors.actionErrors = actionErrors;
  }

  return errors;
}

export function getTaskStatusBadgeVariant(status: string) {
  switch (status) {
    case 'active':
    case 'succeeded':
      return 'success';
    case 'paused':
    case 'queued':
      return 'warning';
    case 'archived':
    case 'cancelled':
      return 'secondary';
    case 'failed':
      return 'destructive';
    default:
      return 'outline';
  }
}

export function getRunStatusBadgeVariant(status: AutomationRunStatus) {
  return RUN_STATUS_VARIANT[status] ?? 'outline';
}

export function summarizeRecipients(recipients: string[], t: (key: string) => string): string {
  const normalized = normalizeRecipients(recipients);

  if (normalized.length === 0) {
    return t('actions.noRecipients');
  }

  if (normalized.length === 1) {
    return normalized[0];
  }

  return `${normalized[0]} +${normalized.length - 1}`;
}

export function getScheduleSummary(
  task: Pick<
    AutomationTask,
    'schedule_type' | 'schedule_config' | 'next_run_at' | 'last_run_at' | 'status' | 'timezone'
  >,
  t: (key: string, values?: Record<string, string | number>) => string
): TaskScheduleSummary {
  const rawScheduleType = String((task as { schedule_type?: string }).schedule_type ?? '');

  if (rawScheduleType === 'once') {
    const scheduledAt =
      (task.schedule_config as { run_at?: number | string }).run_at ?? task.next_run_at ?? null;
    const completedAt = task.last_run_at ?? scheduledAt;
    const isCompleted = task.status === 'completed';
    const time = formatTaskDateTime(isCompleted ? completedAt : scheduledAt, task.timezone);

    return {
      title: t('schedule.once'),
      description: t(isCompleted ? 'schedule.summary.onceCompleted' : 'schedule.summary.once', {
        time,
      }),
      badges: [],
    };
  }

  if (rawScheduleType === 'cron') {
    const cronExpr = (task.schedule_config as { cron_expr?: string }).cron_expr ?? '';
    const recurring = getRecurringModeFromCron(cronExpr);
    const weekdayLabels = recurring.recurringDays.map(dayKey => {
      const weekday = TASK_WEEKDAYS.find(item => item.key === dayKey);
      return weekday ? t(weekday.labelKey) : dayKey;
    });

    if (recurring.recurringMode === 'daily') {
      return {
        title: t('schedule.daily'),
        description: t('schedule.summary.daily', { time: recurring.recurringTime }),
        badges: [],
      };
    }

    if (recurring.recurringMode === 'weekly') {
      return {
        title: t('schedule.weekly'),
        description: t('schedule.summary.weekly', {
          days: weekdayLabels.join(' / '),
          time: recurring.recurringTime,
        }),
        badges: [],
      };
    }

    return {
      title: t('schedule.customCron'),
      description: t('schedule.summary.custom', { expr: cronExpr }),
      badges: [],
    };
  }

  return {
    title: t('fallback.unknownSchedule'),
    description: t('detail.unsupportedReadonly'),
    badges: [],
  };
}

export function getTaskNextRunLabel(
  task: Pick<AutomationTask, 'next_run_at' | 'status' | 'timezone'>,
  t: (key: string) => string
): string {
  if (task.status === 'paused') {
    return t('schedule.pausedNoNextRun');
  }

  return task.next_run_at
    ? formatTaskDateTime(task.next_run_at, task.timezone)
    : t('schedule.noNextRun');
}

export function getPrimaryAction(
  item: Pick<AutomationTaskListItem, 'actions'> | Pick<AutomationTaskDetailData, 'actions'>
): AutomationTaskAction | null {
  return item.actions[0] ?? null;
}

export function getDraftPrimaryAction(draft: TaskDraft): TaskDraftAction {
  return draft.actions[0] ?? createDefaultTaskActionDraft('send_notification');
}

function normalizeToMilliseconds(value?: number | string | null): number | null {
  if (value === undefined || value === null || value === '') {
    return null;
  }

  if (typeof value === 'number') {
    return value < 1e12 ? value * 1000 : value;
  }

  const numeric = Number(value);
  if (!Number.isNaN(numeric) && Number.isFinite(numeric)) {
    return numeric < 1e12 ? numeric * 1000 : numeric;
  }

  const parsed = new Date(value).getTime();
  return Number.isFinite(parsed) ? parsed : null;
}

export function formatRunDuration(run: AutomationTaskRun): string {
  const startedAtMs = normalizeToMilliseconds(run.started_at);
  const finishedAtMs = normalizeToMilliseconds(run.finished_at);

  if (!startedAtMs || !finishedAtMs) {
    return '-';
  }

  const durationSeconds = (finishedAtMs - startedAtMs) / 1000;

  if (!Number.isFinite(durationSeconds) || durationSeconds < 0) {
    return '-';
  }

  return formatDurationSeconds(durationSeconds);
}

export function getLatestRunTimestamp(task: AutomationTask): number | string | null {
  return task.last_run_at ?? task.updated_at ?? task.created_at ?? null;
}

export function shouldShowArchivedNotice(
  task: AutomationTask,
  selectedStatuses: AutomationTaskStatus[]
): boolean {
  return task.status === 'archived' && !selectedStatuses.includes('archived');
}

export function getRunsPanelTab(defaultTab: TaskPanelTab | null): TaskPanelTab {
  return defaultTab === 'runs' ? 'runs' : 'overview';
}

export function safeJson(value: unknown): string {
  if (value === null || value === undefined) {
    return '';
  }

  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

export function getMostRelevantRunTimestamp(run: AutomationTaskRun): number | string | null {
  return getRunTimestamp(run);
}
