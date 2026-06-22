'use client';

import React from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { WorkflowValueEditor } from '@/components/workflow/ui';
import {
  toLocalDateTimeInputValue,
  WorkflowDateTimeInput,
} from '@/components/workflow/common/workflow-date-time-input';
import { useT } from '@/i18n';
import type { CreateScheduledTaskNodeData } from '../config';

interface ScheduleEditorProps {
  nodeId: string;
  schedule: CreateScheduledTaskNodeData['task']['schedule'];
  readOnly?: boolean;
  onChange: (next: CreateScheduledTaskNodeData['task']['schedule']) => void;
}

type ScheduleRecurringMode = 'daily' | 'weekly' | 'customCron';
type ScheduleWeekdayKey = 'mon' | 'tue' | 'wed' | 'thu' | 'fri' | 'sat' | 'sun';

const DEFAULT_RECURRING_TIME = '09:00';
const SCHEDULE_TABS_LIST_CLASS =
  'h-8 w-fit rounded-xl border border-border bg-muted/35 p-1 shadow-none';
const SCHEDULE_TABS_TRIGGER_CLASS = 'h-6 min-w-[72px] rounded-lg px-3 text-[12px]';
const SCHEDULE_WEEKDAYS: Array<{
  key: ScheduleWeekdayKey;
  cronValue: string;
  labelKey: string;
}> = [
  { key: 'mon', cronValue: '1', labelKey: 'createScheduledTask.weekdays.mon' },
  { key: 'tue', cronValue: '2', labelKey: 'createScheduledTask.weekdays.tue' },
  { key: 'wed', cronValue: '3', labelKey: 'createScheduledTask.weekdays.wed' },
  { key: 'thu', cronValue: '4', labelKey: 'createScheduledTask.weekdays.thu' },
  { key: 'fri', cronValue: '5', labelKey: 'createScheduledTask.weekdays.fri' },
  { key: 'sat', cronValue: '6', labelKey: 'createScheduledTask.weekdays.sat' },
  { key: 'sun', cronValue: '0', labelKey: 'createScheduledTask.weekdays.sun' },
];

function pad(value: number): string {
  return String(value).padStart(2, '0');
}

function parseCronWeekdays(token: string): ScheduleWeekdayKey[] {
  const normalizedTokens = token
    .split(',')
    .map(item => item.trim())
    .filter(Boolean);

  const lookup: Record<string, ScheduleWeekdayKey> = {
    '0': 'sun',
    '1': 'mon',
    '2': 'tue',
    '3': 'wed',
    '4': 'thu',
    '5': 'fri',
    '6': 'sat',
    '7': 'sun',
  };

  return normalizedTokens
    .map(item => lookup[item])
    .filter((item): item is ScheduleWeekdayKey => Boolean(item));
}

function serializeCronWeekdays(days: ScheduleWeekdayKey[]): string {
  const values = SCHEDULE_WEEKDAYS.filter(day => days.includes(day.key)).map(day => day.cronValue);
  return values.length > 0 ? values.join(',') : '1';
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
  return { minutes, hours, dayOfMonth, month, dayOfWeek };
}

function getRecurringModeFromCron(expr: string): {
  recurringMode: ScheduleRecurringMode;
  recurringTime: string;
  recurringDays: ScheduleWeekdayKey[];
} {
  const parsed = parseCronExpression(expr);

  if (!parsed) {
    return {
      recurringMode: 'customCron',
      recurringTime: DEFAULT_RECURRING_TIME,
      recurringDays: ['mon'],
    };
  }

  const hasSimpleTime = /^\d{1,2}$/.test(parsed.hours) && /^\d{1,2}$/.test(parsed.minutes);
  const recurringTime = hasSimpleTime
    ? `${pad(Number(parsed.hours))}:${pad(Number(parsed.minutes))}`
    : DEFAULT_RECURRING_TIME;

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

function getCronExpressionFromPreset(
  recurringMode: ScheduleRecurringMode,
  recurringTime: string,
  recurringDays: ScheduleWeekdayKey[],
  cronExpr: string
): string {
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

/**
 * @component ScheduleEditor
 * @category Feature
 * @status Beta
 * @description Editor for the create-scheduled-task node schedule draft with fixed and variable once modes.
 * @usage Render inside the node manager to edit `task.schedule`.
 * @example
 * <ScheduleEditor nodeId={id} schedule={task.schedule} onChange={setSchedule} />
 */
export function ScheduleEditor({
  nodeId,
  schedule,
  readOnly = false,
  onChange,
}: ScheduleEditorProps) {
  const t = useT('nodes');
  const derivedRecurring = React.useMemo(
    () => getRecurringModeFromCron(schedule.cron.expr),
    [schedule.cron.expr]
  );
  const emittedCronExprRef = React.useRef(schedule.cron.expr);
  const [selectedRecurringMode, setSelectedRecurringMode] = React.useState<ScheduleRecurringMode>(
    derivedRecurring.recurringMode
  );

  React.useEffect(() => {
    if (schedule.cron.expr !== emittedCronExprRef.current) {
      setSelectedRecurringMode(derivedRecurring.recurringMode);
    }
  }, [derivedRecurring.recurringMode, schedule.cron.expr]);

  const handleScheduleTypeChange = React.useCallback(
    (value: string) => {
      const nextType = value as CreateScheduledTaskNodeData['task']['schedule']['type'];
      const nextCronExpr =
        nextType === 'cron' && !schedule.cron.expr.trim()
          ? getCronExpressionFromPreset(
              'daily',
              DEFAULT_RECURRING_TIME,
              ['mon'],
              schedule.cron.expr
            )
          : schedule.cron.expr;

      if (nextType === 'cron') {
        emittedCronExprRef.current = nextCronExpr;
        setSelectedRecurringMode(getRecurringModeFromCron(nextCronExpr).recurringMode);
      }

      onChange({
        ...schedule,
        type: nextType,
        cron: {
          ...schedule.cron,
          expr: nextCronExpr,
        },
      });
    },
    [onChange, schedule]
  );

  const handleRecurringModeChange = React.useCallback(
    (value: string) => {
      const nextMode = value as ScheduleRecurringMode;
      const nextCronExpr =
        nextMode === 'customCron'
          ? selectedRecurringMode === 'customCron'
            ? schedule.cron.expr
            : ''
          : getCronExpressionFromPreset(
              nextMode,
              derivedRecurring.recurringTime,
              derivedRecurring.recurringDays,
              schedule.cron.expr
            );

      emittedCronExprRef.current = nextCronExpr;
      setSelectedRecurringMode(nextMode);

      onChange({
        ...schedule,
        cron: {
          ...schedule.cron,
          expr: nextCronExpr,
        },
      });
    },
    [
      derivedRecurring.recurringDays,
      derivedRecurring.recurringTime,
      onChange,
      schedule,
      selectedRecurringMode,
    ]
  );

  const handleRecurringTimeChange = React.useCallback(
    (value: string) => {
      const nextCronExpr = getCronExpressionFromPreset(
        selectedRecurringMode,
        value,
        derivedRecurring.recurringDays,
        schedule.cron.expr
      );
      emittedCronExprRef.current = nextCronExpr;

      onChange({
        ...schedule,
        cron: {
          ...schedule.cron,
          expr: nextCronExpr,
        },
      });
    },
    [derivedRecurring.recurringDays, onChange, schedule, selectedRecurringMode]
  );

  const toggleWeekday = React.useCallback(
    (day: ScheduleWeekdayKey) => {
      const checked = derivedRecurring.recurringDays.includes(day);
      const nextDays = checked
        ? derivedRecurring.recurringDays.filter(item => item !== day)
        : [...derivedRecurring.recurringDays, day];

      if (nextDays.length === 0) {
        return;
      }

      const nextCronExpr = getCronExpressionFromPreset(
        'weekly',
        derivedRecurring.recurringTime,
        nextDays,
        schedule.cron.expr
      );
      emittedCronExprRef.current = nextCronExpr;
      setSelectedRecurringMode('weekly');

      onChange({
        ...schedule,
        cron: {
          ...schedule.cron,
          expr: nextCronExpr,
        },
      });
    },
    [derivedRecurring.recurringDays, derivedRecurring.recurringTime, onChange, schedule]
  );

  return (
    <section className="space-y-4">
      <h3 className="text-sm font-semibold text-foreground">
        {t('createScheduledTask.section.schedule')}
      </h3>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <Label>{t('createScheduledTask.fields.scheduleType')}</Label>
        <Tabs value={schedule.type} onValueChange={handleScheduleTypeChange}>
          <TabsList className={SCHEDULE_TABS_LIST_CLASS}>
            <TabsTrigger value="once" className={SCHEDULE_TABS_TRIGGER_CLASS} disabled={readOnly}>
              {t('createScheduledTask.scheduleTypeOnce')}
            </TabsTrigger>
            <TabsTrigger value="cron" className={SCHEDULE_TABS_TRIGGER_CLASS} disabled={readOnly}>
              {t('createScheduledTask.scheduleTypeCron')}
            </TabsTrigger>
          </TabsList>
        </Tabs>
      </div>

      {schedule.type === 'once' ? (
        <div className="space-y-4 rounded-2xl border border-border bg-muted/20 p-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <Label>{t('createScheduledTask.fields.onceInputMode')}</Label>
            <Tabs
              value={schedule.once.input_mode}
              onValueChange={value => {
                const nextInputMode =
                  value as CreateScheduledTaskNodeData['task']['schedule']['once']['input_mode'];
                const nextRunAt =
                  nextInputMode === 'fixed' &&
                  schedule.once.run_at &&
                  !toLocalDateTimeInputValue(schedule.once.run_at)
                    ? ''
                    : schedule.once.run_at;

                onChange({
                  ...schedule,
                  once: {
                    ...schedule.once,
                    input_mode: nextInputMode,
                    run_at: nextRunAt,
                  },
                });
              }}
            >
              <TabsList className={SCHEDULE_TABS_LIST_CLASS}>
                <TabsTrigger
                  value="fixed"
                  className={SCHEDULE_TABS_TRIGGER_CLASS}
                  disabled={readOnly}
                >
                  {t('createScheduledTask.onceInputFixed')}
                </TabsTrigger>
                <TabsTrigger
                  value="variable"
                  className={SCHEDULE_TABS_TRIGGER_CLASS}
                  disabled={readOnly}
                >
                  {t('createScheduledTask.onceInputVariable')}
                </TabsTrigger>
              </TabsList>
            </Tabs>
          </div>

          <div className="rounded-2xl border border-border bg-background p-4">
            {schedule.once.input_mode === 'fixed' ? (
              <div className="space-y-2">
                <Label htmlFor="workflow-task-run-at">
                  {t('createScheduledTask.fields.runAt')}
                </Label>
                <WorkflowDateTimeInput
                  id="workflow-task-run-at"
                  value={schedule.once.run_at}
                  onChange={value =>
                    onChange({
                      ...schedule,
                      once: {
                        ...schedule.once,
                        run_at: value,
                      },
                    })
                  }
                  disabled={readOnly}
                />
              </div>
            ) : (
              <div className="space-y-2">
                <Label>{t('createScheduledTask.fields.runAt')}</Label>
                <WorkflowValueEditor
                  nodeId={nodeId}
                  value={schedule.once.run_at}
                  onChange={value =>
                    onChange({
                      ...schedule,
                      once: {
                        ...schedule.once,
                        run_at: value,
                      },
                    })
                  }
                  readOnly={readOnly}
                  placeholder={t('createScheduledTask.placeholders.runAtVariable')}
                  editorClassName="min-h-[72px] rounded-xl border-border bg-background px-3 py-2.5 shadow-none hover:border-border focus-within:border-primary/70"
                />
              </div>
            )}
          </div>
        </div>
      ) : (
        <div className="space-y-4 rounded-2xl border border-border bg-muted/20 p-4">
          <div className="rounded-2xl border border-border bg-background p-4">
            <div className="space-y-2">
              <Label htmlFor="workflow-task-timezone">
                {t('createScheduledTask.fields.timezone')}
              </Label>
              <Input
                id="workflow-task-timezone"
                value={schedule.timezone}
                onChange={event =>
                  onChange({
                    ...schedule,
                    timezone: event.target.value,
                  })
                }
                placeholder={t('createScheduledTask.placeholders.timezone')}
                disabled={readOnly}
              />
            </div>
          </div>

          <div className="space-y-3">
            <Label>{t('createScheduledTask.fields.recurringPreset')}</Label>
            <Select
              value={selectedRecurringMode}
              onValueChange={handleRecurringModeChange}
              disabled={readOnly}
            >
              <SelectTrigger className="h-10 rounded-xl border-border bg-background shadow-none hover:border-border sm:max-w-[220px]">
                <SelectValue placeholder={t('createScheduledTask.fields.recurringPreset')} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="daily">{t('createScheduledTask.scheduleDaily')}</SelectItem>
                <SelectItem value="weekly">{t('createScheduledTask.scheduleWeekly')}</SelectItem>
                <SelectItem value="customCron">
                  {t('createScheduledTask.scheduleCustomCron')}
                </SelectItem>
              </SelectContent>
            </Select>
          </div>

          {selectedRecurringMode !== 'customCron' ? (
            <div className="rounded-2xl border border-border bg-background p-4">
              <div className="space-y-2">
                <Label htmlFor="workflow-task-recurring-time">
                  {t('createScheduledTask.fields.time')}
                </Label>
                <Input
                  id="workflow-task-recurring-time"
                  type="time"
                  value={derivedRecurring.recurringTime}
                  onChange={event => handleRecurringTimeChange(event.target.value)}
                  disabled={readOnly}
                />
              </div>
            </div>
          ) : null}

          {selectedRecurringMode === 'weekly' ? (
            <div className="rounded-2xl border border-border bg-background p-4">
              <div className="space-y-2">
                <Label>{t('createScheduledTask.fields.weeklyDays')}</Label>
                <div className="flex flex-wrap gap-2">
                  {SCHEDULE_WEEKDAYS.map(day => {
                    const checked = derivedRecurring.recurringDays.includes(day.key);

                    return (
                      <Button
                        key={day.key}
                        type="button"
                        size="sm"
                        variant={checked ? 'default' : 'outline'}
                        onClick={() => toggleWeekday(day.key)}
                        disabled={readOnly}
                      >
                        {t(day.labelKey as never)}
                      </Button>
                    );
                  })}
                </div>
              </div>
            </div>
          ) : null}

          {selectedRecurringMode === 'customCron' ? (
            <div className="rounded-2xl border border-border bg-background p-4">
              <div className="space-y-2">
                <Label htmlFor="workflow-task-cron">
                  {t('createScheduledTask.fields.cronExpr')}
                </Label>
                <Input
                  id="workflow-task-cron"
                  value={schedule.cron.expr}
                  onChange={event => {
                    emittedCronExprRef.current = event.target.value;
                    onChange({
                      ...schedule,
                      cron: {
                        ...schedule.cron,
                        expr: event.target.value,
                      },
                    });
                  }}
                  placeholder={t('createScheduledTask.placeholders.cronExpr')}
                  disabled={readOnly}
                />
                <p className="text-[11px] leading-5 text-muted-foreground">
                  {t('createScheduledTask.help.customCron')}
                </p>
              </div>
            </div>
          ) : null}
        </div>
      )}
    </section>
  );
}

export default ScheduleEditor;
