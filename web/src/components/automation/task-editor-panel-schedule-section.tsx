'use client';

import * as React from 'react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { RadioCard, RadioCardGroup } from '@/components/ui/radio-card';
import { Separator } from '@/components/ui/separator';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { TASK_WEEKDAYS } from './registry';
import type { TaskDraftSchedule, TaskValidationErrors, TaskWeekdayKey } from './types';

type CronTemplateKey = 'dailyMorning' | 'weekdaysMorning' | 'hourly' | 'monthlyMorning';

interface TaskEditorScheduleSectionProps {
  schedule: TaskDraftSchedule;
  editable: boolean;
  errors: TaskValidationErrors;
  onScheduleChange: (updater: (current: TaskDraftSchedule) => TaskDraftSchedule) => void;
  onToggleWeekday: (weekday: TaskWeekdayKey) => void;
}

const CRON_TEMPLATES: Array<{ key: CronTemplateKey; cronExpr: string }> = [
  { key: 'dailyMorning', cronExpr: '0 9 * * *' },
  { key: 'weekdaysMorning', cronExpr: '0 9 * * MON,TUE,WED,THU,FRI' },
  { key: 'hourly', cronExpr: '0 * * * *' },
  { key: 'monthlyMorning', cronExpr: '0 9 1 * *' },
];

function parseCronExpression(expr: string): {
  minute: string;
  hour: string;
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

  const [minute, hour, dayOfMonth, month, dayOfWeek] = parts;
  return { minute, hour, dayOfMonth, month, dayOfWeek };
}

function normalizeTime(hour: string, minute: string): string | null {
  if (!/^\d{1,2}$/.test(hour) || !/^\d{1,2}$/.test(minute)) {
    return null;
  }

  const hourNumber = Number(hour);
  const minuteNumber = Number(minute);
  if (hourNumber > 23 || minuteNumber > 59) {
    return null;
  }

  return `${String(hourNumber).padStart(2, '0')}:${String(minuteNumber).padStart(2, '0')}`;
}

function formatCronWeekdays(
  token: string,
  translate: (key: string, values?: Record<string, string | number>) => string
): string {
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
  const labels = token
    .split(',')
    .map(item => lookup[item.trim().toUpperCase()])
    .filter((item): item is TaskWeekdayKey => Boolean(item))
    .map(item => translate(`schedule.weekdays.${item}`));

  return labels.join(' / ');
}

/**
 * @component TaskEditorScheduleSection
 * @category Feature
 * @status Stable
 * @description Schedule form card for the scheduled task editor, covering one-time and recurring modes.
 * @usage Render inside the task editor panel and wire it to the local draft schedule state.
 */
export function TaskEditorScheduleSection({
  schedule,
  editable,
  errors,
  onScheduleChange,
  onToggleWeekday,
}: TaskEditorScheduleSectionProps) {
  const t = useT('automation');
  const translate = React.useCallback(
    (key: string, values?: Record<string, string | number>) => t(key as never, values as never),
    [t]
  );
  const cronPreview = React.useMemo(() => {
    if (schedule.scheduleType !== 'cron') {
      return '';
    }

    if (schedule.recurringMode === 'daily') {
      return translate('schedule.preview.daily', { time: schedule.recurringTime || '--:--' });
    }

    if (schedule.recurringMode === 'weekly') {
      const days = schedule.recurringDays
        .map(day => translate(`schedule.weekdays.${day}`))
        .join(' / ');
      return translate('schedule.preview.weekly', {
        days: days || translate('schedule.preview.noWeekdays'),
        time: schedule.recurringTime || '--:--',
      });
    }

    const parsed = parseCronExpression(schedule.cronExpr);
    if (!parsed) {
      return translate('schedule.preview.invalid');
    }

    const time = normalizeTime(parsed.hour, parsed.minute);
    if (time && parsed.dayOfMonth === '*' && parsed.month === '*' && parsed.dayOfWeek === '*') {
      return translate('schedule.preview.daily', { time });
    }

    if (time && parsed.dayOfMonth === '*' && parsed.month === '*' && parsed.dayOfWeek !== '*') {
      const days = formatCronWeekdays(parsed.dayOfWeek, translate);
      if (days) {
        return translate('schedule.preview.weekly', { days, time });
      }
    }

    if (
      parsed.minute === '0' &&
      parsed.hour === '*' &&
      parsed.dayOfMonth === '*' &&
      parsed.month === '*' &&
      parsed.dayOfWeek === '*'
    ) {
      return translate('schedule.preview.hourly');
    }

    if (time && parsed.dayOfMonth !== '*' && parsed.month === '*' && parsed.dayOfWeek === '*') {
      return translate('schedule.preview.monthly', {
        day: parsed.dayOfMonth,
        time,
      });
    }

    return translate('schedule.preview.custom', { expr: schedule.cronExpr.trim() });
  }, [schedule, translate]);

  return (
    <Card className="border-border/70" padding="none">
      <CardHeader className="pb-3">
        <CardTitle className="text-base">{t('schedule.title')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-5">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <Label>{t('schedule.modeLabel')}</Label>
          <Tabs
            value={schedule.scheduleType}
            onValueChange={value =>
              onScheduleChange(current => ({
                ...current,
                scheduleType: value as 'once' | 'cron',
              }))
            }
          >
            <TabsList>
              <TabsTrigger value="once" disabled={!editable}>
                {t('schedule.once')}
              </TabsTrigger>
              <TabsTrigger value="cron" disabled={!editable}>
                {t('schedule.recurring')}
              </TabsTrigger>
            </TabsList>
          </Tabs>
        </div>

        {schedule.scheduleType === 'once' ? (
          <div className="space-y-2">
            <Label htmlFor="automation-run-at">{t('schedule.runAt')}</Label>
            <Input
              id="automation-run-at"
              type="datetime-local"
              value={schedule.onceRunAt}
              onChange={event =>
                onScheduleChange(current => ({
                  ...current,
                  onceRunAt: event.target.value,
                }))
              }
              errorText={errors.onceRunAt}
              disabled={!editable}
            />
          </div>
        ) : (
          <div className="space-y-2">
            <Label htmlFor="automation-recurring-time">{t('schedule.time')}</Label>
            <Input
              id="automation-recurring-time"
              type="time"
              value={schedule.recurringTime}
              onChange={event =>
                onScheduleChange(current => ({
                  ...current,
                  recurringTime: event.target.value,
                }))
              }
              errorText={errors.recurringTime}
              disabled={!editable}
            />
          </div>
        )}

        <div className="space-y-2">
          <Label htmlFor="automation-timezone">{t('schedule.timezone')}</Label>
          <Input
            id="automation-timezone"
            value={schedule.timezone}
            onChange={event =>
              onScheduleChange(current => ({
                ...current,
                timezone: event.target.value,
              }))
            }
            placeholder={t('schedule.timezonePlaceholder')}
            errorText={errors.timezone}
            disabled={!editable}
          />
          <p className="text-xs leading-6 text-muted-foreground">{t('schedule.timezoneHelp')}</p>
        </div>

        {schedule.scheduleType === 'cron' ? (
          <>
            <Separator />

            <div className="space-y-3">
              <Label>{t('schedule.recurringPreset')}</Label>
              <RadioCardGroup
                value={schedule.recurringMode}
                onValueChange={value =>
                  onScheduleChange(current => ({
                    ...current,
                    recurringMode: value as 'daily' | 'weekly' | 'customCron',
                  }))
                }
                className={cn('md:grid-cols-3', !editable && 'pointer-events-none opacity-70')}
              >
                <RadioCard value="daily" title={t('schedule.daily')} hiddenRadio />
                <RadioCard value="weekly" title={t('schedule.weekly')} hiddenRadio />
                <RadioCard value="customCron" title={t('schedule.customCron')} hiddenRadio />
              </RadioCardGroup>
            </div>

            <div className="space-y-2">
              <Label>{t('schedule.templatePresets')}</Label>
              <div className="flex flex-wrap gap-2">
                {CRON_TEMPLATES.map(template => (
                  <Button
                    key={template.key}
                    type="button"
                    variant="outline"
                    size="sm"
                    disabled={!editable}
                    onClick={() =>
                      onScheduleChange(current => ({
                        ...current,
                        recurringMode: 'customCron',
                        cronExpr: template.cronExpr,
                      }))
                    }
                  >
                    {t(`schedule.templates.${template.key}` as never)}
                  </Button>
                ))}
              </div>
            </div>

            {schedule.recurringMode === 'weekly' ? (
              <div className="space-y-2">
                <Label>{t('schedule.weeklyDays')}</Label>
                <div className="flex flex-wrap gap-2">
                  {TASK_WEEKDAYS.map(weekday => {
                    const checked = schedule.recurringDays.includes(weekday.key);

                    return (
                      <Button
                        key={weekday.key}
                        type="button"
                        size="sm"
                        variant={checked ? 'default' : 'outline'}
                        onClick={() => onToggleWeekday(weekday.key)}
                        disabled={!editable}
                      >
                        {t(weekday.labelKey as never)}
                      </Button>
                    );
                  })}
                </div>
                {errors.recurringDays ? (
                  <p className="text-xs font-medium text-destructive">{errors.recurringDays}</p>
                ) : null}
              </div>
            ) : null}

            {schedule.recurringMode === 'customCron' ? (
              <div className="space-y-2">
                <Label htmlFor="automation-cron">{t('schedule.cronExpression')}</Label>
                <Input
                  id="automation-cron"
                  value={schedule.cronExpr}
                  onChange={event =>
                    onScheduleChange(current => ({
                      ...current,
                      cronExpr: event.target.value,
                    }))
                  }
                  errorText={errors.cronExpr}
                  placeholder="0 9 * * 1"
                  disabled={!editable}
                />
                <p className="text-xs leading-6 text-muted-foreground">
                  {t('schedule.customHelp')}
                </p>
              </div>
            ) : null}

            <div className="flex flex-col gap-1 rounded-lg border border-border/70 bg-muted/20 px-3 py-2 text-sm leading-6 sm:flex-row sm:items-center">
              <span className="shrink-0 font-medium text-foreground">
                {t('schedule.previewLabel')}
              </span>
              <span className="text-muted-foreground sm:ml-2">{cronPreview}</span>
            </div>
          </>
        ) : null}
      </CardContent>
    </Card>
  );
}
