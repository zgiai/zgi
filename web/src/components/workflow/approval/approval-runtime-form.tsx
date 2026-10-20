'use client';

import React from 'react';
import { AlertTriangle, ArrowRight, ChevronDown, ClipboardList, Loader2 } from 'lucide-react';

import MarkdownViewer from '@/components/common/markdown-viewer';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import type {
  ApprovalRuntimeAction,
  ApprovalRuntimeForm as ApprovalRuntimeFormData,
} from '@/services/approval.service';
import { useT } from '@/i18n';
import { useLocale } from '@/hooks/use-locale';
import { cn } from '@/lib/utils';
import { translateTimezone } from '@/utils/timezone';

export interface ApprovalRuntimeFormProps {
  form: ApprovalRuntimeFormData;
  isSubmitting?: boolean;
  submittedAction?: string | null;
  onSubmit: (payload: { inputs: Record<string, unknown>; action: string }) => void | Promise<void>;
  secondaryAction?: React.ReactNode;
  className?: string;
}

function stringifyDefault(value: unknown): string {
  if (value === null || typeof value === 'undefined') return '';
  if (typeof value === 'string') return value;
  if (typeof value === 'number' || typeof value === 'boolean') return String(value);
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function getActionClassName(action: ApprovalRuntimeAction): string {
  switch (action.style) {
    case 'danger':
      return 'bg-destructive text-destructive-foreground hover:bg-destructive/90';
    case 'primary':
      return '';
    default:
      return '';
  }
}

/**
 * @component ApprovalRuntimeForm
 * @category Feature
 * @status Beta
 * @description Runtime form for submitting a paused human approval workflow node.
 * @usage Reuse in workflow run panel and public token approval page.
 * @example
 * <ApprovalRuntimeForm form={form} onSubmit={submit} />
 */
export function ApprovalRuntimeForm({
  form,
  isSubmitting = false,
  submittedAction,
  onSubmit,
  secondaryAction,
  className,
}: ApprovalRuntimeFormProps) {
  const t = useT('nodes');
  const webT = useT('webapp');
  const { locale } = useLocale();
  const isDigitalHumanTaskApproval = form.node_id === 'digital_human_task';
  const taskApproval = isDigitalHumanTaskApproval ? form.task_approval : undefined;
  const taskOperation = taskApproval?.operation ?? 'action';
  const title = isDigitalHumanTaskApproval
    ? taskApprovalTitle(taskOperation, webT)
    : form.node_title;
  const content = isDigitalHumanTaskApproval
    ? taskApprovalDescription(taskOperation, webT)
    : form.content;
  const visibleFields = React.useMemo(
    () =>
      (form.fields || []).filter(
        field => !isDigitalHumanTaskApproval || field.key === 'supplement'
      ),
    [form.fields, isDigitalHumanTaskApproval]
  );
  const [inputs, setInputs] = React.useState<Record<string, string>>({});
  const [errors, setErrors] = React.useState<Record<string, string>>({});
  // A task proposal is reviewed first; the change request box only opens when
  // the user asks for changes, so it does not compete with the decision.
  const [revising, setRevising] = React.useState(false);
  const renderedFields = isDigitalHumanTaskApproval && !revising ? [] : visibleFields;

  React.useEffect(() => {
    const defaults = form.resolved_default_values || {};
    const next: Record<string, string> = {};
    for (const field of visibleFields) {
      next[field.key] = stringifyDefault(defaults[field.key]);
    }
    setInputs(next);
    setErrors({});
    setRevising(false);
  }, [form, visibleFields]);

  const submit = React.useCallback(
    async (action: ApprovalRuntimeAction) => {
      const nextErrors: Record<string, string> = {};
      for (const field of visibleFields) {
        if (field.required && !String(inputs[field.key] ?? '').trim()) {
          nextErrors[field.key] = t('approval.runtime.required');
        }
      }
      if (
        isDigitalHumanTaskApproval &&
        action.id === 'revise' &&
        !String(inputs.supplement ?? '').trim()
      ) {
        nextErrors.supplement = webT('consoleChat.taskApproval.supplementRequired');
      }
      setErrors(nextErrors);
      if (Object.keys(nextErrors).length > 0) return;
      await onSubmit({ inputs, action: action.id });
    },
    [inputs, isDigitalHumanTaskApproval, onSubmit, t, visibleFields, webT]
  );

  return (
    <div className={cn('space-y-5', className)}>
      {isDigitalHumanTaskApproval && taskApproval ? (
        <TaskApprovalHeader
          title={title}
          operation={taskOperation}
          presentation={taskApproval}
          webT={webT}
        />
      ) : (
        <div className="space-y-1">
          <h2 className="text-lg font-semibold">{title}</h2>
          {form.expiration_at ? (
            <p className="text-xs text-muted-foreground">
              {t('approval.runtime.expiresAt', {
                time: new Date(form.expiration_at * 1000).toLocaleString(),
              })}
            </p>
          ) : null}
        </div>
      )}

      {isDigitalHumanTaskApproval && taskApproval ? (
        <DigitalHumanTaskApprovalDetails presentation={taskApproval} locale={locale} webT={webT} />
      ) : (
        <div className="rounded-lg border bg-background p-3 sm:p-4">
          <MarkdownViewer
            content={content || ''}
            className="md-viewer break-words whitespace-pre-wrap"
          />
        </div>
      )}

      <div className="space-y-4">
        {renderedFields.map(field => (
          <div key={field.key} className="space-y-1.5">
            <Label htmlFor={`approval-${form.id}-${field.key}`} className="text-sm font-medium">
              {isDigitalHumanTaskApproval && field.key === 'name'
                ? webT('consoleChat.taskApproval.name')
                : isDigitalHumanTaskApproval && field.key === 'goal'
                  ? webT('consoleChat.taskApproval.goal')
                  : isDigitalHumanTaskApproval && field.key === 'supplement'
                    ? webT('consoleChat.taskApproval.supplement')
                    : field.label || field.key}
              {field.required ? <span className="ml-1 text-destructive">*</span> : null}
            </Label>
            {field.type === 'textarea' ? (
              <Textarea
                id={`approval-${form.id}-${field.key}`}
                value={inputs[field.key] ?? ''}
                onChange={event =>
                  setInputs(current => ({ ...current, [field.key]: event.target.value }))
                }
                aria-invalid={Boolean(errors[field.key])}
                autoFocus={isDigitalHumanTaskApproval && field.key === 'supplement'}
                placeholder={
                  isDigitalHumanTaskApproval && field.key === 'supplement'
                    ? webT('consoleChat.taskApproval.supplementPlaceholder')
                    : undefined
                }
                className={
                  isDigitalHumanTaskApproval && field.key === 'supplement'
                    ? 'min-h-[88px]'
                    : 'min-h-[120px]'
                }
              />
            ) : (
              <Input
                id={`approval-${form.id}-${field.key}`}
                value={inputs[field.key] ?? ''}
                onChange={event =>
                  setInputs(current => ({ ...current, [field.key]: event.target.value }))
                }
                aria-invalid={Boolean(errors[field.key])}
              />
            )}
            {errors[field.key] ? (
              <p className="text-xs text-destructive">{errors[field.key]}</p>
            ) : isDigitalHumanTaskApproval && field.key === 'supplement' ? (
              <p className="text-xs leading-5 text-muted-foreground">
                {webT('consoleChat.taskApproval.supplementHint')}
              </p>
            ) : null}
          </div>
        ))}
      </div>

      <div className="flex flex-col gap-3 border-t pt-4 sm:flex-row sm:items-center sm:justify-between">
        {secondaryAction ? <div className="shrink-0">{secondaryAction}</div> : <span />}
        {isDigitalHumanTaskApproval ? (
          <TaskApprovalActions
            actions={form.actions || []}
            operation={taskOperation}
            revising={revising}
            isSubmitting={isSubmitting}
            submittedAction={submittedAction}
            onRevise={() => setRevising(true)}
            onBack={() => {
              setRevising(false);
              setErrors({});
            }}
            onSubmit={submit}
            webT={webT}
          />
        ) : (
          <div className="flex flex-wrap justify-end gap-2">
            {(form.actions || []).map(action => (
              <Button
                key={action.id}
                type="button"
                variant={action.style === 'secondary' ? 'outline' : 'default'}
                className={getActionClassName(action)}
                disabled={isSubmitting}
                onClick={() => submit(action)}
              >
                {isSubmitting && submittedAction === action.id ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : null}
                {action.label || action.id}
              </Button>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

function TaskApprovalActions({
  actions,
  operation,
  revising,
  isSubmitting,
  submittedAction,
  onRevise,
  onBack,
  onSubmit,
  webT,
}: {
  actions: ApprovalRuntimeAction[];
  operation: string;
  revising: boolean;
  isSubmitting: boolean;
  submittedAction?: string | null;
  onRevise: () => void;
  onBack: () => void;
  onSubmit: (action: ApprovalRuntimeAction) => void | Promise<void>;
  webT: ReturnType<typeof useT<'webapp'>>;
}) {
  const approve = actions.find(action => action.id === 'approve');
  const revise = actions.find(action => action.id === 'revise');
  const decline = actions.find(action => action.id === 'decline');
  const spinner = (action: ApprovalRuntimeAction) =>
    isSubmitting && submittedAction === action.id ? (
      <Loader2 className="size-4 animate-spin" />
    ) : null;

  if (revising && revise) {
    return (
      <div className="flex flex-wrap justify-end gap-2">
        <Button type="button" variant="ghost" disabled={isSubmitting} onClick={onBack}>
          {webT('consoleChat.taskApproval.backToReview')}
        </Button>
        <Button type="button" disabled={isSubmitting} onClick={() => onSubmit(revise)}>
          {spinner(revise)}
          {webT('consoleChat.taskApproval.regenerate')}
        </Button>
      </div>
    );
  }

  return (
    <div className="flex flex-wrap justify-end gap-2">
      {decline ? (
        <Button
          type="button"
          variant="ghost"
          className="text-muted-foreground hover:text-foreground"
          disabled={isSubmitting}
          onClick={() => onSubmit(decline)}
        >
          {spinner(decline)}
          {webT('consoleChat.taskApproval.decline')}
        </Button>
      ) : null}
      {revise ? (
        <Button type="button" variant="outline" disabled={isSubmitting} onClick={onRevise}>
          {webT('consoleChat.taskApproval.requestChanges')}
        </Button>
      ) : null}
      {approve ? (
        <Button type="button" disabled={isSubmitting} onClick={() => onSubmit(approve)}>
          {spinner(approve)}
          {taskApprovalApproveLabel(operation, webT)}
        </Button>
      ) : null}
    </div>
  );
}

function TaskApprovalHeader({
  title,
  operation,
  presentation,
  webT,
}: {
  title: string;
  operation: string;
  presentation: NonNullable<ApprovalRuntimeFormData['task_approval']>;
  webT: ReturnType<typeof useT<'webapp'>>;
}) {
  const taskName =
    readTaskString(presentation.task_after, 'name') ||
    readTaskString(presentation.task_before, 'name') ||
    webT('consoleChat.taskApproval.unnamedTask');

  return (
    <div role="region" aria-label={title} className="flex items-start gap-3">
      <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-warning/15 text-warning-foreground">
        <ClipboardList className="size-4" aria-hidden="true" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="rounded-full bg-warning/15 px-2 py-0.5 text-xs font-medium text-warning-foreground">
            {webT('consoleChat.taskApproval.pending')}
          </span>
          <span className="text-xs text-muted-foreground">
            {taskApprovalOperationLabel(operation, webT)}
          </span>
        </div>
        <h2 className="mt-1 break-words text-base font-semibold leading-6 text-foreground">
          {taskName}
        </h2>
      </div>
    </div>
  );
}

function taskApprovalTitle(operation: string, t: ReturnType<typeof useT<'webapp'>>): string {
  switch (operation) {
    case 'create':
      return t('consoleChat.taskApproval.createTitle');
    case 'update':
      return t('consoleChat.taskApproval.updateTitle');
    case 'cancel':
      return t('consoleChat.taskApproval.cancelTitle');
    case 'run_now':
      return t('consoleChat.taskApproval.runNowTitle');
    case 'pause':
      return t('consoleChat.taskApproval.pauseTitle');
    case 'resume':
      return t('consoleChat.taskApproval.resumeTitle');
    case 'retry':
      return t('consoleChat.taskApproval.retryTitle');
    case 'archive':
      return t('consoleChat.taskApproval.archiveTitle');
    default:
      return t('consoleChat.taskApproval.title');
  }
}

function taskApprovalOperationLabel(
  operation: string,
  t: ReturnType<typeof useT<'webapp'>>
): string {
  const labels: Record<string, string> = {
    create: t('consoleChat.taskApproval.operationCreate'),
    update: t('consoleChat.taskApproval.operationUpdate'),
    cancel: t('consoleChat.taskApproval.operationCancel'),
    run_now: t('consoleChat.taskApproval.operationRunNow'),
    pause: t('consoleChat.taskApproval.operationPause'),
    resume: t('consoleChat.taskApproval.operationResume'),
    retry: t('consoleChat.taskApproval.operationRetry'),
    archive: t('consoleChat.taskApproval.operationArchive'),
  };
  return labels[operation] ?? t('consoleChat.taskApproval.operation');
}

function taskApprovalApproveLabel(operation: string, t: ReturnType<typeof useT<'webapp'>>): string {
  const labels: Record<string, string> = {
    create: t('consoleChat.taskApproval.confirmCreate'),
    update: t('consoleChat.taskApproval.confirmUpdate'),
    cancel: t('consoleChat.taskApproval.confirmCancel'),
    run_now: t('consoleChat.taskApproval.confirmRunNow'),
    pause: t('consoleChat.taskApproval.confirmPause'),
    resume: t('consoleChat.taskApproval.confirmResume'),
    retry: t('consoleChat.taskApproval.confirmRetry'),
    archive: t('consoleChat.taskApproval.confirmArchive'),
  };
  return labels[operation] ?? t('consoleChat.taskApproval.approve');
}

function readTaskString(task: Record<string, unknown> | null | undefined, key: string): string {
  const value = task?.[key];
  return typeof value === 'string' ? value.trim() : '';
}

function taskApprovalDescription(operation: string, t: ReturnType<typeof useT<'webapp'>>): string {
  switch (operation) {
    case 'create':
      return t('consoleChat.taskApproval.createDescription');
    case 'update':
      return t('consoleChat.taskApproval.updateDescription');
    case 'cancel':
      return t('consoleChat.taskApproval.cancelDescription');
    case 'run_now':
      return t('consoleChat.taskApproval.runNowDescription');
    case 'pause':
      return t('consoleChat.taskApproval.pauseDescription');
    case 'resume':
      return t('consoleChat.taskApproval.resumeDescription');
    case 'retry':
      return t('consoleChat.taskApproval.retryDescription');
    case 'archive':
      return t('consoleChat.taskApproval.archiveDescription');
    default:
      return t('consoleChat.taskApproval.description');
  }
}

function DigitalHumanTaskApprovalDetails({
  presentation,
  locale,
  webT,
}: {
  presentation: NonNullable<ApprovalRuntimeFormData['task_approval']>;
  locale?: string;
  webT: ReturnType<typeof useT<'webapp'>>;
}) {
  const [showUnchanged, setShowUnchanged] = React.useState(false);
  const changes = presentation.changes ?? [];
  const before = presentation.task_before ?? null;
  const after = presentation.task_after ?? null;
  const timezone = taskApprovalScheduleTimezone(after);
  const format = (key: string, value: unknown) =>
    formatTaskApprovalValue(key, value, locale, webT, timezone);
  const changedKeys = new Set(changes.map(change => change.field));
  // The task name is the card heading, and the work/lifecycle states are
  // internal bookkeeping; neither helps the approver decide.
  const detailRows = taskApprovalDetailKeys
    .filter(key => after && key in after && !changedKeys.has(key))
    .map(key => ({
      key,
      label: taskApprovalFieldLabel(key, webT),
      value: format(key, after?.[key]),
    }));
  const scheduleChanged = changedKeys.has('schedule');
  const nextRunRecalculated =
    scheduleChanged &&
    Boolean(before && 'next_run_at' in before) &&
    !(after && 'next_run_at' in after);
  const relatedFieldsStale =
    scheduleChanged &&
    !changedKeys.has('goal') &&
    !changedKeys.has('success_criteria') &&
    Boolean(readTaskString(after, 'goal') || readTaskString(after, 'success_criteria'));
  const hasChanges = changes.length > 0;

  return (
    <div className="space-y-3">
      {hasChanges ? (
        <dl className="divide-y rounded-lg border">
          {changes.map(change => (
            <TaskApprovalRow key={change.field} label={taskApprovalChangeLabel(change, webT)}>
              <span className="text-muted-foreground line-through decoration-muted-foreground/50">
                {format(change.field, change.before)}
              </span>
              <ArrowRight
                className="mx-1.5 inline size-3.5 align-[-2px] text-muted-foreground"
                aria-hidden="true"
              />
              <span className="font-medium text-foreground">
                {format(change.field, change.after)}
              </span>
            </TaskApprovalRow>
          ))}
          {nextRunRecalculated ? (
            <TaskApprovalRow label={taskApprovalFieldLabel('next_run_at', webT)}>
              <span className="text-muted-foreground">
                {webT('consoleChat.taskApproval.nextRunRecalculated')}
              </span>
            </TaskApprovalRow>
          ) : null}
        </dl>
      ) : null}
      {relatedFieldsStale ? (
        <p className="flex items-start gap-2 rounded-md bg-warning/10 px-3 py-2 text-xs leading-5 text-warning-foreground">
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
          {webT('consoleChat.taskApproval.relatedFieldsHint')}
        </p>
      ) : null}
      {detailRows.length > 0 ? (
        hasChanges ? (
          <div>
            <button
              type="button"
              className="inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline"
              aria-expanded={showUnchanged}
              onClick={() => setShowUnchanged(value => !value)}
            >
              <ChevronDown
                className={cn('size-3.5 transition-transform', showUnchanged && 'rotate-180')}
                aria-hidden="true"
              />
              {showUnchanged
                ? webT('consoleChat.taskApproval.hideUnchanged')
                : webT('consoleChat.taskApproval.showUnchanged', { count: detailRows.length })}
            </button>
            {showUnchanged ? (
              <dl className="mt-2 divide-y rounded-lg border bg-muted/20">
                {detailRows.map(row => (
                  <TaskApprovalRow key={row.key} label={row.label} muted>
                    {row.value}
                  </TaskApprovalRow>
                ))}
              </dl>
            ) : null}
          </div>
        ) : (
          <dl className="divide-y rounded-lg border">
            {detailRows.map(row => (
              <TaskApprovalRow key={row.key} label={row.label}>
                <span className="font-medium text-foreground">{row.value}</span>
              </TaskApprovalRow>
            ))}
          </dl>
        )
      ) : null}
    </div>
  );
}

const taskApprovalDetailKeys = [
  'goal',
  'schedule',
  'next_run_at',
  'priority',
  'delivery_mode',
  'success_criteria',
];

function TaskApprovalRow({
  label,
  muted = false,
  children,
}: {
  label: string;
  muted?: boolean;
  children: React.ReactNode;
}) {
  return (
    <div className="grid gap-1 px-3 py-2 text-sm sm:grid-cols-[7.5rem_minmax(0,1fr)] sm:gap-3">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className={cn('min-w-0 break-words', muted ? 'text-foreground/80' : 'text-foreground')}>
        {children}
      </dd>
    </div>
  );
}

function taskApprovalChangeLabel(
  change: { field: string; label?: string },
  t: ReturnType<typeof useT<'webapp'>>
): string {
  // The server label is a legacy single-locale fallback; prefer the
  // localized label for every field the UI knows about.
  const label = taskApprovalFieldLabel(change.field, t);
  return label !== change.field ? label : change.label || change.field;
}

function taskApprovalScheduleTimezone(task: Record<string, unknown> | null): string | undefined {
  const schedule = task?.schedule;
  if (!schedule || typeof schedule !== 'object') return undefined;
  const timezone = (schedule as Record<string, unknown>).timezone;
  return typeof timezone === 'string' && timezone.trim() ? timezone.trim() : undefined;
}

function formatTaskDateTime(value: string, locale: string | undefined, timezone?: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const options: Intl.DateTimeFormatOptions = { dateStyle: 'medium', timeStyle: 'short' };
  if (timezone) {
    try {
      // dateStyle/timeStyle cannot be combined with timeZoneName.
      return date.toLocaleString(locale, {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
        hour: 'numeric',
        minute: '2-digit',
        timeZone: timezone,
        timeZoneName: 'short',
      });
    } catch {
      // Fall back to the viewer's zone when the task zone is unknown here.
    }
  }
  return date.toLocaleString(locale, options);
}

function taskApprovalFieldLabel(key: string, t: ReturnType<typeof useT<'webapp'>>): string {
  const labels: Record<string, string> = {
    name: t('consoleChat.taskApproval.name'),
    goal: t('consoleChat.taskApproval.goal'),
    schedule: t('consoleChat.taskApproval.schedule'),
    priority: t('consoleChat.taskApproval.priority'),
    delivery_mode: t('consoleChat.taskApproval.delivery'),
    success_criteria: t('consoleChat.taskApproval.successCriteria'),
    work_status: t('consoleChat.taskApproval.workStatus'),
    lifecycle_status: t('consoleChat.taskApproval.lifecycleStatus'),
    next_run_at: t('consoleChat.taskApproval.nextRunAt'),
  };
  return labels[key] ?? key;
}

function formatTaskApprovalValue(
  key: string,
  value: unknown,
  locale: string | undefined,
  t: ReturnType<typeof useT<'webapp'>>,
  timezone?: string
): string {
  if (value === null || typeof value === 'undefined' || value === '') {
    return t('consoleChat.taskApproval.emptyValue');
  }
  if (key === 'schedule' && value && typeof value === 'object') {
    const schedule = value as Record<string, unknown>;
    const type = String(schedule.type ?? '');
    if (type === 'manual') return t('consoleChat.taskApproval.manual');
    if (type === 'once') {
      const runAt =
        typeof schedule.run_at === 'string'
          ? formatTaskDateTime(schedule.run_at, locale, taskApprovalScheduleTimezone({ schedule }))
          : '';
      return runAt
        ? t('consoleChat.taskApproval.onceAt', { time: runAt })
        : t('consoleChat.taskApproval.once');
    }
    const timezone = typeof schedule.timezone === 'string' ? schedule.timezone : '';
    const scheduleDescription = describeApprovalCron(schedule.cron, t);
    return timezone
      ? t('consoleChat.taskApproval.recurringAtTimezone', {
          schedule: scheduleDescription,
          timezone: translateTimezone(timezone, locale),
        })
      : scheduleDescription;
  }
  if (key === 'priority') {
    const labels: Record<string, string> = {
      low: t('consoleChat.taskApproval.priorityLow'),
      normal: t('consoleChat.taskApproval.priorityNormal'),
      high: t('consoleChat.taskApproval.priorityHigh'),
      urgent: t('consoleChat.taskApproval.priorityUrgent'),
    };
    return labels[String(value)] ?? String(value);
  }
  if (key === 'delivery_mode') {
    return value === 'source_conversation'
      ? t('consoleChat.taskApproval.deliveryConversation')
      : t('consoleChat.taskApproval.deliveryTask');
  }
  if (key === 'work_status') {
    const labels: Record<string, string> = {
      proposed: t('consoleChat.taskApproval.statusProposed'),
      pending: t('consoleChat.taskApproval.statusPending'),
      in_progress: t('consoleChat.taskApproval.statusInProgress'),
      waiting: t('consoleChat.taskApproval.statusWaiting'),
      blocked: t('consoleChat.taskApproval.statusBlocked'),
      completed: t('consoleChat.taskApproval.statusCompleted'),
      canceled: t('consoleChat.taskApproval.statusCanceled'),
    };
    return labels[String(value)] ?? String(value);
  }
  if (key === 'lifecycle_status') {
    const labels: Record<string, string> = {
      active: t('consoleChat.taskApproval.lifecycleActive'),
      paused: t('consoleChat.taskApproval.lifecyclePaused'),
      archived: t('consoleChat.taskApproval.lifecycleArchived'),
    };
    return labels[String(value)] ?? String(value);
  }
  if (key === 'next_run_at' && typeof value === 'string') {
    return formatTaskDateTime(value, locale, timezone);
  }
  if (Array.isArray(value)) return value.join('、');
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
}

function describeApprovalCron(value: unknown, t: ReturnType<typeof useT<'webapp'>>): string {
  const expression = typeof value === 'string' ? value.trim() : '';
  const fields = expression.split(/\s+/);
  if (fields.length !== 5) return t('consoleChat.taskApproval.recurring');

  const [minutes, hours, dayOfMonth, month, dayOfWeek] = fields;
  if (/^\d{1,2}$/.test(minutes) && /^\d{1,2}$/.test(hours) && dayOfMonth === '*' && month === '*') {
    const time = `${hours.padStart(2, '0')}:${minutes.padStart(2, '0')}`;
    if (dayOfWeek === '*') return t('consoleChat.taskApproval.dailyAt', { time });
    if (dayOfWeek === '1-5') return t('consoleChat.taskApproval.weekdaysAt', { time });
    if (/^[0-7]$/.test(dayOfWeek)) {
      const weekdayLabels: Record<string, string> = {
        '0': t('consoleChat.taskApproval.sunday'),
        '1': t('consoleChat.taskApproval.monday'),
        '2': t('consoleChat.taskApproval.tuesday'),
        '3': t('consoleChat.taskApproval.wednesday'),
        '4': t('consoleChat.taskApproval.thursday'),
        '5': t('consoleChat.taskApproval.friday'),
        '6': t('consoleChat.taskApproval.saturday'),
        '7': t('consoleChat.taskApproval.sunday'),
      };
      return t('consoleChat.taskApproval.weeklyAt', {
        day: weekdayLabels[dayOfWeek],
        time,
      });
    }
  }
  return t('consoleChat.taskApproval.recurring');
}

export default ApprovalRuntimeForm;
