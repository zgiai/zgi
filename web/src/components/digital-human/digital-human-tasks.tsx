'use client';

import { useDeferredValue, useEffect, useState, type ReactNode } from 'react';
import Link from 'next/link';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import {
  AlertCircle,
  CalendarClock,
  CheckCircle2,
  ListTodo,
  Loader2,
  MoreHorizontal,
  Pause,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  RotateCcw,
  Search,
  XCircle,
} from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { Skeleton } from '@/components/ui/skeleton';
import { Textarea } from '@/components/ui/textarea';
import { digitalHumanChannelProvider } from '@/components/digital-human/digital-human-channel-providers';
import {
  useDigitalHumanTaskDetail,
  useDigitalHumanTaskMutations,
  useDigitalHumanTasks,
} from '@/hooks/digital-human/use-digital-human-tasks';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type {
  CreateDigitalHumanTaskRequest,
  DigitalHumanTask,
  DigitalHumanTaskPriority,
  DigitalHumanTaskScheduleType,
  DigitalHumanTaskStatus,
} from '@/services/types/digital-human';

type StatusFilter = 'open' | 'all' | DigitalHumanTaskStatus;
type PriorityFilter = 'all' | DigitalHumanTaskPriority;
type ScheduleFilter = 'all' | DigitalHumanTaskScheduleType;

const summaryStatuses: DigitalHumanTaskStatus[] = [
  'proposed',
  'pending',
  'in_progress',
  'blocked',
  'completed',
];
const taskParam = 'task';

export function DigitalHumanTasks({ id }: { id: string }) {
  const t = useT('digitalHumans');
  const pathname = usePathname();
  const router = useRouter();
  const searchParams = useSearchParams();
  const permissions = useAccountPermissions();
  const canUpdate = permissions.hasPermission('digital_human.update');
  const canRun = permissions.hasPermission('digital_human.run');
  const [search, setSearch] = useState('');
  const deferredSearch = useDeferredValue(search);
  const [status, setStatus] = useState<StatusFilter>('open');
  const [priority, setPriority] = useState<PriorityFilter>('all');
  const [schedule, setSchedule] = useState<ScheduleFilter>('all');
  const selectedTaskID = searchParams.get(taskParam) ?? '';
  const [editorTask, setEditorTask] = useState<DigitalHumanTask | null | undefined>();
  const query = useDigitalHumanTasks(id, {
    query: deferredSearch.trim() || undefined,
    status:
      status === 'all'
        ? undefined
        : status === 'open'
          ? ['proposed', 'pending', 'in_progress', 'waiting', 'blocked']
          : [status],
    priority: priority === 'all' ? undefined : [priority],
    schedule_type: schedule === 'all' ? undefined : schedule,
  });
  const items = query.data?.data.items ?? [];
  const counts = query.data?.data.counts ?? {};
  const hasFilters = Boolean(
    search.trim() || status !== 'open' || priority !== 'all' || schedule !== 'all'
  );
  const setSelectedTaskID = (taskId?: string) => {
    const params = new URLSearchParams(searchParams.toString());
    if (taskId) params.set(taskParam, taskId);
    else params.delete(taskParam);
    const query = params.toString();
    router.replace(query ? `${pathname}?${query}` : pathname, { scroll: false });
  };

  return (
    <main className="mx-auto flex min-h-full w-full max-w-[1600px] flex-col gap-5 px-4 py-6 md:px-8 md:py-8">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <ListTodo className="size-5 text-primary" />
            <h1 className="text-2xl font-semibold tracking-tight">{t('tasks.title')}</h1>
          </div>
          <p className="max-w-3xl text-sm text-muted-foreground">{t('tasks.description')}</p>
        </div>
        <div className="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => void query.refetch()}
            disabled={query.isFetching}
          >
            <RefreshCw className={cn('size-4', query.isFetching && 'animate-spin')} />
            {t('common.refresh')}
          </Button>
          {canUpdate ? (
            <Button size="sm" onClick={() => setEditorTask(null)}>
              <Plus className="size-4" />
              {t('tasks.create')}
            </Button>
          ) : null}
        </div>
      </div>

      <div className="grid grid-cols-2 gap-3 lg:grid-cols-5">
        {summaryStatuses.map(item => (
          <button
            key={item}
            type="button"
            className={cn(
              'rounded-xl border bg-card p-4 text-left outline-none transition-colors hover:border-primary/30 hover:bg-primary/[0.02] focus-visible:ring-2 focus-visible:ring-ring',
              status === item && 'border-primary/40 bg-primary/5'
            )}
            onClick={() => setStatus(current => (current === item ? 'open' : item))}
          >
            <p className="text-xs text-muted-foreground">{t(`tasks.status.${item}`)}</p>
            <p className="mt-1 text-2xl font-semibold tabular-nums">{counts[item] ?? 0}</p>
          </button>
        ))}
      </div>

      <section className="overflow-hidden rounded-xl border bg-background">
        <div className="flex flex-col gap-3 border-b p-4 xl:flex-row xl:items-center">
          <Input
            value={search}
            onChange={event => setSearch(event.target.value)}
            placeholder={t('tasks.filters.search')}
            leftIcon={<Search />}
            className="h-9"
            containerClassName="min-w-0 xl:w-[420px]"
          />
          <div className="grid flex-1 grid-cols-1 gap-2 sm:grid-cols-3 xl:flex xl:justify-end">
            <TaskFilter
              value={status}
              onChange={value => setStatus(value as StatusFilter)}
              label={t('tasks.filters.status')}
            >
              <SelectItem value="open">{t('tasks.filters.openStatuses')}</SelectItem>
              <SelectItem value="all">{t('tasks.filters.allStatuses')}</SelectItem>
              {taskStatuses.map(item => (
                <SelectItem key={item} value={item}>
                  {t(`tasks.status.${item}`)}
                </SelectItem>
              ))}
            </TaskFilter>
            <TaskFilter
              value={priority}
              onChange={value => setPriority(value as PriorityFilter)}
              label={t('tasks.filters.priority')}
            >
              <SelectItem value="all">{t('tasks.filters.allPriorities')}</SelectItem>
              {taskPriorities.map(item => (
                <SelectItem key={item} value={item}>
                  {t(`tasks.priority.${item}`)}
                </SelectItem>
              ))}
            </TaskFilter>
            <TaskFilter
              value={schedule}
              onChange={value => setSchedule(value as ScheduleFilter)}
              label={t('tasks.filters.schedule')}
            >
              <SelectItem value="all">{t('tasks.filters.allSchedules')}</SelectItem>
              {taskSchedules.map(item => (
                <SelectItem key={item} value={item}>
                  {t(`tasks.schedule.${item}`)}
                </SelectItem>
              ))}
            </TaskFilter>
          </div>
          {hasFilters ? (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setSearch('');
                setStatus('open');
                setPriority('all');
                setSchedule('all');
              }}
            >
              {t('tasks.filters.clear')}
            </Button>
          ) : null}
        </div>

        {query.isLoading ? (
          <div className="space-y-3 p-4">
            {[0, 1, 2].map(item => (
              <Skeleton key={item} className="h-20 rounded-xl" />
            ))}
          </div>
        ) : query.isError ? (
          <TaskState
            icon={<AlertCircle className="size-5" />}
            title={t('tasks.loadError')}
            description={t('tasks.loadErrorDescription')}
            action={
              <Button variant="outline" size="sm" onClick={() => void query.refetch()}>
                {t('tasks.retry')}
              </Button>
            }
          />
        ) : items.length === 0 ? (
          <TaskState
            icon={hasFilters ? <Search className="size-5" /> : <ListTodo className="size-5" />}
            title={hasFilters ? t('tasks.noMatches') : t('tasks.empty')}
            description={hasFilters ? t('tasks.noMatchesDescription') : t('tasks.emptyDescription')}
          />
        ) : (
          <>
            <div className="hidden grid-cols-[minmax(260px,2fr)_110px_110px_minmax(180px,1fr)_170px_170px] gap-4 border-b bg-muted/20 px-5 py-3 text-xs font-medium text-muted-foreground lg:grid">
              <span>{t('tasks.columns.name')}</span>
              <span>{t('tasks.columns.status')}</span>
              <span>{t('tasks.columns.priority')}</span>
              <span>{t('tasks.columns.schedule')}</span>
              <span>{t('tasks.columns.lastResult')}</span>
              <span>{t('tasks.columns.updated')}</span>
            </div>
            <div className="divide-y">
              {items.map(task => (
                <button
                  key={task.id}
                  type="button"
                  className="grid w-full gap-3 px-4 py-4 text-left outline-none transition-colors hover:bg-muted/25 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring lg:grid-cols-[minmax(260px,2fr)_110px_110px_minmax(180px,1fr)_170px_170px] lg:items-center lg:gap-4 lg:px-5"
                  onClick={() => setSelectedTaskID(task.id)}
                >
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{task.name}</p>
                    <p className="mt-1 line-clamp-1 text-xs text-muted-foreground">{task.goal}</p>
                    <p className="mt-1 truncate text-[11px] text-muted-foreground/80">
                      {sourceLabel(task, t)}
                    </p>
                  </div>
                  <TaskListCell label={t('tasks.columns.status')}>
                    <TaskStatusBadge task={task} />
                  </TaskListCell>
                  <TaskListCell label={t('tasks.columns.priority')}>
                    <PriorityBadge priority={task.priority} />
                  </TaskListCell>
                  <TaskListCell label={t('tasks.columns.schedule')}>
                    <TaskScheduleLabel task={task} />
                  </TaskListCell>
                  <TaskListCell label={t('tasks.columns.lastResult')}>
                    {task.last_run_status
                      ? t(`tasks.runStatus.${normalizeRunStatus(task.last_run_status)}`)
                      : t('tasks.neverRun')}
                  </TaskListCell>
                  <TaskListCell label={t('tasks.columns.updated')}>
                    {formatDate(task.updated_at)}
                  </TaskListCell>
                </button>
              ))}
            </div>
          </>
        )}
      </section>

      <TaskEditorDialog
        id={id}
        task={editorTask}
        open={editorTask !== undefined}
        onOpenChange={open => {
          if (!open) setEditorTask(undefined);
        }}
      />
      <TaskDetailSheet
        id={id}
        taskId={selectedTaskID}
        open={Boolean(selectedTaskID)}
        canUpdate={canUpdate}
        canRun={canRun}
        onEdit={task => setEditorTask(task)}
        onOpenChange={open => {
          if (!open) setSelectedTaskID();
        }}
      />
    </main>
  );
}

const taskStatuses: DigitalHumanTaskStatus[] = [
  'proposed',
  'pending',
  'in_progress',
  'waiting',
  'blocked',
  'completed',
  'canceled',
];
const taskPriorities: DigitalHumanTaskPriority[] = ['low', 'normal', 'high', 'urgent'];
const taskSchedules: DigitalHumanTaskScheduleType[] = ['manual', 'once', 'cron'];

function TaskFilter({
  value,
  onChange,
  label,
  children,
}: {
  value: string;
  onChange: (value: string) => void;
  label: string;
  children: ReactNode;
}) {
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger className="h-9 min-w-0 xl:w-[170px]" aria-label={label}>
        <SelectValue />
      </SelectTrigger>
      <SelectContent>{children}</SelectContent>
    </Select>
  );
}

function TaskListCell({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex min-w-0 items-center justify-between gap-4 text-xs text-muted-foreground lg:block">
      <span className="shrink-0 font-medium text-foreground/70 lg:hidden">{label}</span>
      <div className="min-w-0 text-right lg:text-left">{children}</div>
    </div>
  );
}

function TaskStatusBadge({ task }: { task: DigitalHumanTask }) {
  const t = useT('digitalHumans');
  const variant =
    task.work_status === 'completed'
      ? 'success'
      : task.work_status === 'blocked' || task.work_status === 'canceled'
        ? 'destructive'
        : task.work_status === 'in_progress'
          ? 'info'
          : task.work_status === 'proposed'
            ? 'warning'
            : 'subtle';
  return (
    <Badge variant={variant}>
      {task.lifecycle_status === 'paused' && task.work_status === 'pending'
        ? t('tasks.status.paused')
        : t(`tasks.status.${task.work_status}`)}
    </Badge>
  );
}

function PriorityBadge({ priority }: { priority: DigitalHumanTaskPriority }) {
  const t = useT('digitalHumans');
  return (
    <Badge
      variant={priority === 'urgent' ? 'destructive' : priority === 'high' ? 'warning' : 'outline'}
    >
      {t(`tasks.priority.${priority}`)}
    </Badge>
  );
}

function TaskScheduleLabel({ task }: { task: DigitalHumanTask }) {
  const t = useT('digitalHumans');
  if (task.schedule.type === 'manual') {
    return <span>{t('tasks.schedule.manual')}</span>;
  }
  if (task.schedule.type === 'once') {
    return (
      <span>
        {task.schedule.run_at ? formatDate(task.schedule.run_at) : t('tasks.schedule.once')}
      </span>
    );
  }
  return (
    <span className="flex flex-col">
      <span>{task.schedule.cron}</span>
      <span>{task.schedule.timezone}</span>
    </span>
  );
}

function TaskState({
  icon,
  title,
  description,
  action,
}: {
  icon: ReactNode;
  title: string;
  description: string;
  action?: ReactNode;
}) {
  return (
    <div className="flex min-h-64 flex-col items-center justify-center px-6 text-center">
      <span className="mb-3 flex size-10 items-center justify-center rounded-full bg-muted text-muted-foreground">
        {icon}
      </span>
      <p className="font-medium">{title}</p>
      <p className="mt-1 max-w-lg text-sm text-muted-foreground">{description}</p>
      {action ? <div className="mt-4">{action}</div> : null}
    </div>
  );
}

function TaskDetailSheet({
  id,
  taskId,
  open,
  canUpdate,
  canRun,
  onEdit,
  onOpenChange,
}: {
  id: string;
  taskId: string;
  open: boolean;
  canUpdate: boolean;
  canRun: boolean;
  onEdit: (task: DigitalHumanTask) => void;
  onOpenChange: (open: boolean) => void;
}) {
  const t = useT('digitalHumans');
  const detail = useDigitalHumanTaskDetail(id, taskId);
  const mutations = useDigitalHumanTaskMutations(id);
  const task = detail.task.data?.data;
  const [confirmAction, setConfirmAction] = useState<'decline' | 'cancel' | 'archive' | null>(null);
  const perform = async (action: string) => {
    if (!task) return;
    await mutations.action.mutateAsync({ taskId: task.id, action, revision: task.revision });
    setConfirmAction(null);
    toast.success(t('tasks.actionApplied'));
  };

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side="right"
        className="flex h-full w-[min(720px,100vw)] max-w-none flex-col gap-0 p-0 sm:max-w-none"
      >
        <SheetHeader className="shrink-0 border-b px-6 py-5 pr-12 text-left">
          <SheetTitle>{task?.name ?? t('tasks.detail.title')}</SheetTitle>
          <SheetDescription>{task?.goal ?? t('tasks.detail.description')}</SheetDescription>
        </SheetHeader>
        {detail.task.isLoading || !task ? (
          <div className="space-y-4 p-6">
            <Skeleton className="h-28" />
            <Skeleton className="h-44" />
          </div>
        ) : (
          <div className="min-h-0 flex-1 overflow-y-auto">
            <div className="space-y-6 p-6">
              <div className="flex flex-wrap items-center gap-2">
                <TaskStatusBadge task={task} />
                <PriorityBadge priority={task.priority} />
                <Badge variant="outline">
                  <CalendarClock className="size-3" />
                  {t(`tasks.schedule.${task.schedule.type}`)}
                </Badge>
              </div>
              <section className="grid gap-4 rounded-xl border p-4 sm:grid-cols-2">
                <DetailValue
                  label={t('tasks.fields.schedule')}
                  value={<TaskScheduleLabel task={task} />}
                />
                <DetailValue
                  label={t('tasks.fields.delivery')}
                  value={t(`tasks.delivery.${task.delivery_mode}`)}
                />
                <DetailValue
                  label={t('tasks.fields.nextRun')}
                  value={task.next_run_at ? formatDate(task.next_run_at) : '—'}
                />
                <DetailValue
                  label={t('tasks.fields.timeout')}
                  value={t('tasks.seconds', { value: task.execution_timeout_seconds })}
                />
                <DetailValue
                  label={t('tasks.fields.retry')}
                  value={String(task.max_attempts - 1)}
                />
                <DetailValue label={t('tasks.fields.source')} value={sourceLabel(task, t)} />
              </section>
              {task.success_criteria.length ? (
                <section>
                  <h3 className="text-sm font-semibold">{t('tasks.fields.successCriteria')}</h3>
                  <ul className="mt-2 space-y-2">
                    {task.success_criteria.map((item, index) => (
                      <li key={`${index}-${item}`} className="flex gap-2 text-sm">
                        <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-success" />
                        {item}
                      </li>
                    ))}
                  </ul>
                </section>
              ) : null}
              <section>
                <h3 className="text-sm font-semibold">{t('tasks.detail.runs')}</h3>
                <div className="mt-3 space-y-2">
                  {detail.runs.isLoading ? (
                    <Skeleton className="h-20" />
                  ) : (detail.runs.data?.data.items ?? []).length === 0 ? (
                    <p className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
                      {t('tasks.detail.noRuns')}
                    </p>
                  ) : (
                    (detail.runs.data?.data.items ?? []).map(run => (
                      <div
                        key={run.id}
                        className="flex items-center justify-between gap-3 rounded-lg border p-3"
                      >
                        <div className="min-w-0">
                          <p className="text-sm font-medium">
                            {t('tasks.detail.attempt', { value: run.attempt })}
                          </p>
                          <p className="text-xs text-muted-foreground">
                            {formatDate(run.created_at)}
                          </p>
                          {run.error_summary ? (
                            <p className="mt-1 line-clamp-2 text-xs text-destructive">
                              {run.error_summary}
                            </p>
                          ) : null}
                        </div>
                        <div className="flex items-center gap-2">
                          <Badge
                            variant={
                              run.status === 'succeeded'
                                ? 'success'
                                : run.status === 'failed'
                                  ? 'destructive'
                                  : 'subtle'
                            }
                          >
                            {t(`tasks.runStatus.${normalizeRunStatus(run.status)}`)}
                          </Badge>
                          {run.digital_human_run_id ? (
                            <Button asChild variant="ghost" size="sm">
                              <Link
                                href={`/console/digital-humans/${id}/runs?run=${run.digital_human_run_id}`}
                              >
                                {t('tasks.detail.viewLog')}
                              </Link>
                            </Button>
                          ) : null}
                        </div>
                      </div>
                    ))
                  )}
                </div>
              </section>
              <section>
                <h3 className="text-sm font-semibold">{t('tasks.detail.activities')}</h3>
                <div className="mt-3 border-l pl-4">
                  {detail.activities.isLoading ? (
                    <Skeleton className="h-28" />
                  ) : (
                    (detail.activities.data?.data.items ?? []).map(activity => (
                      <div key={activity.id} className="relative pb-4 last:pb-0">
                        <span className="absolute -left-[21px] top-1 size-2 rounded-full bg-primary" />
                        <p className="text-sm">{activityLabel(activity.action, t)}</p>
                        <p className="mt-0.5 text-xs text-muted-foreground">
                          {formatDate(activity.created_at)}
                        </p>
                      </div>
                    ))
                  )}
                </div>
              </section>
            </div>
          </div>
        )}
        {task ? (
          <div className="flex shrink-0 flex-wrap justify-end gap-2 border-t bg-background px-6 py-4">
            {canUpdate && task.work_status !== 'canceled' ? (
              <Button variant="outline" size="sm" onClick={() => onEdit(task)}>
                <Pencil className="size-4" />
                {t('common.edit')}
              </Button>
            ) : null}
            {task.work_status === 'proposed' && canUpdate ? (
              <>
                <Button variant="outline" size="sm" onClick={() => setConfirmAction('decline')}>
                  <XCircle className="size-4" />
                  {t('tasks.actions.decline')}
                </Button>
                <Button size="sm" onClick={() => void perform('confirm')}>
                  <CheckCircle2 className="size-4" />
                  {t('tasks.actions.confirm')}
                </Button>
              </>
            ) : null}
            {task.lifecycle_status === 'active' && task.work_status === 'pending' && canUpdate ? (
              <Button variant="outline" size="sm" onClick={() => void perform('pause')}>
                <Pause className="size-4" />
                {t('tasks.actions.pause')}
              </Button>
            ) : null}
            {task.lifecycle_status === 'paused' &&
            !['canceled', 'completed'].includes(task.work_status) &&
            canUpdate ? (
              <Button variant="outline" size="sm" onClick={() => void perform('resume')}>
                <Play className="size-4" />
                {t('tasks.actions.resume')}
              </Button>
            ) : null}
            {task.work_status === 'blocked' && canRun ? (
              <Button size="sm" onClick={() => void perform('retry')}>
                <RotateCcw className="size-4" />
                {t('tasks.actions.retry')}
              </Button>
            ) : null}
            {!['proposed', 'in_progress', 'canceled'].includes(task.work_status) && canRun ? (
              <Button
                size="sm"
                onClick={() => void perform('run_now')}
                disabled={mutations.action.isPending}
              >
                <Play className="size-4" />
                {t('tasks.actions.runNow')}
              </Button>
            ) : null}
            {!['completed', 'canceled'].includes(task.work_status) && canRun ? (
              <Button variant="outline" size="sm" onClick={() => setConfirmAction('cancel')}>
                <XCircle className="size-4" />
                {t('tasks.actions.cancel')}
              </Button>
            ) : null}
            {['completed', 'canceled'].includes(task.work_status) && canUpdate ? (
              <Button variant="outline" size="sm" onClick={() => setConfirmAction('archive')}>
                <MoreHorizontal className="size-4" />
                {t('tasks.actions.archive')}
              </Button>
            ) : null}
          </div>
        ) : null}
        <ConfirmDialog
          open={confirmAction !== null}
          onOpenChange={nextOpen => {
            if (!nextOpen) setConfirmAction(null);
          }}
          title={confirmationCopy(confirmAction, 'title', t)}
          description={confirmationCopy(confirmAction, 'description', t)}
          confirmText={confirmAction ? t(`tasks.actions.${confirmAction}`) : ''}
          cancelText={t('common.cancel')}
          onConfirm={() => {
            if (confirmAction) void perform(confirmAction);
          }}
          loading={mutations.action.isPending}
          closeOnConfirm={false}
          variant={confirmAction === 'archive' ? 'default' : 'danger'}
        />
      </SheetContent>
    </Sheet>
  );
}

function DetailValue({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div>
      <p className="text-xs text-muted-foreground">{label}</p>
      <div className="mt-1 text-sm">{value}</div>
    </div>
  );
}

interface TaskFormState {
  name: string;
  goal: string;
  successCriteria: string;
  priority: DigitalHumanTaskPriority;
  scheduleType: DigitalHumanTaskScheduleType;
  runAt: string;
  cron: string;
  timezone: string;
  deliveryMode: 'task_only' | 'source_conversation';
  timeout: string;
  attempts: string;
}

function TaskEditorDialog({
  id,
  task,
  open,
  onOpenChange,
}: {
  id: string;
  task: DigitalHumanTask | null | undefined;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const t = useT('digitalHumans');
  const mutations = useDigitalHumanTaskMutations(id);
  const [form, setForm] = useState<TaskFormState>(() => emptyTaskForm());
  useEffect(() => {
    if (open) setForm(task ? taskForm(task) : emptyTaskForm());
  }, [open, task]);
  const set = <K extends keyof TaskFormState>(key: K, value: TaskFormState[K]) =>
    setForm(current => ({ ...current, [key]: value }));
  const valid =
    form.name.trim() &&
    form.goal.trim() &&
    (form.scheduleType !== 'once' || form.runAt) &&
    (form.scheduleType !== 'cron' || form.cron.trim());
  const submit = async () => {
    if (!valid) return;
    const schedule = {
      type: form.scheduleType,
      timezone: form.timezone.trim() || 'UTC',
      ...(form.scheduleType === 'once' ? { run_at: new Date(form.runAt).toISOString() } : {}),
      ...(form.scheduleType === 'cron' ? { cron: form.cron.trim() } : {}),
    };
    const request: CreateDigitalHumanTaskRequest = {
      name: form.name.trim(),
      goal: form.goal.trim(),
      success_criteria: splitLines(form.successCriteria),
      priority: form.priority,
      schedule,
      delivery_mode: form.deliveryMode,
      execution_timeout_seconds: Number(form.timeout),
      max_attempts: Number(form.attempts),
    };
    if (task) {
      await mutations.update.mutateAsync({
        taskId: task.id,
        request: { ...request, expected_revision: task.revision },
      });
    } else {
      await mutations.create.mutateAsync(request);
    }
    toast.success(task ? t('tasks.updated') : t('tasks.created'));
    onOpenChange(false);
  };
  const pending = mutations.create.isPending || mutations.update.isPending;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg">
        <DialogHeader>
          <DialogTitle>
            {task ? t('tasks.editor.editTitle') : t('tasks.editor.createTitle')}
          </DialogTitle>
          <DialogDescription>{t('tasks.editor.description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-5 pb-4">
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label={t('tasks.fields.name')} required>
              <Input
                value={form.name}
                onChange={event => set('name', event.target.value)}
                placeholder={t('tasks.editor.namePlaceholder')}
              />
            </Field>
            <Field label={t('tasks.fields.priority')}>
              <Select
                value={form.priority}
                onValueChange={value => set('priority', value as DigitalHumanTaskPriority)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {taskPriorities.map(item => (
                    <SelectItem key={item} value={item}>
                      {t(`tasks.priority.${item}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
          </div>
          <Field label={t('tasks.fields.goal')} required>
            <Textarea
              value={form.goal}
              onChange={event => set('goal', event.target.value)}
              placeholder={t('tasks.editor.goalPlaceholder')}
              className="min-h-24"
            />
          </Field>
          <Field label={t('tasks.fields.successCriteria')}>
            <Textarea
              value={form.successCriteria}
              onChange={event => set('successCriteria', event.target.value)}
              placeholder={t('tasks.editor.criteriaPlaceholder')}
              className="min-h-24"
            />
          </Field>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label={t('tasks.fields.schedule')}>
              <Select
                value={form.scheduleType}
                onValueChange={value => set('scheduleType', value as DigitalHumanTaskScheduleType)}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {taskSchedules.map(item => (
                    <SelectItem key={item} value={item}>
                      {t(`tasks.schedule.${item}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            {form.scheduleType === 'once' ? (
              <Field label={t('tasks.fields.runAt')} required>
                <Input
                  type="datetime-local"
                  value={form.runAt}
                  onChange={event => set('runAt', event.target.value)}
                />
              </Field>
            ) : null}
            {form.scheduleType === 'cron' ? (
              <Field label={t('tasks.fields.cron')} required>
                <Input
                  value={form.cron}
                  onChange={event => set('cron', event.target.value)}
                  placeholder="0 9 * * 1-5"
                />
              </Field>
            ) : null}
          </div>
          {form.scheduleType !== 'manual' ? (
            <Field label={t('tasks.fields.timezone')}>
              <Input
                value={form.timezone}
                onChange={event => set('timezone', event.target.value)}
              />
            </Field>
          ) : null}
          <details className="rounded-xl border">
            <summary className="cursor-pointer px-4 py-3 text-sm font-medium">
              {t('tasks.editor.advanced')}
            </summary>
            <div className="grid gap-4 border-t p-4 sm:grid-cols-3">
              <Field label={t('tasks.fields.delivery')}>
                <Select
                  value={form.deliveryMode}
                  onValueChange={value =>
                    set('deliveryMode', value as TaskFormState['deliveryMode'])
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="task_only">{t('tasks.delivery.task_only')}</SelectItem>
                    {task?.delivery_mode === 'source_conversation' ? (
                      <SelectItem value="source_conversation">
                        {t('tasks.delivery.source_conversation')}
                      </SelectItem>
                    ) : null}
                  </SelectContent>
                </Select>
              </Field>
              <Field label={t('tasks.fields.timeout')}>
                <Input
                  type="number"
                  min={30}
                  max={1800}
                  value={form.timeout}
                  onChange={event => set('timeout', event.target.value)}
                />
              </Field>
              <Field label={t('tasks.fields.attempts')}>
                <Input
                  type="number"
                  min={1}
                  max={3}
                  value={form.attempts}
                  onChange={event => set('attempts', event.target.value)}
                />
              </Field>
            </div>
          </details>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button onClick={() => void submit()} disabled={!valid || pending}>
            {pending ? <Loader2 className="size-4 animate-spin" /> : null}
            {t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Field({
  label,
  required,
  children,
}: {
  label: string;
  required?: boolean;
  children: ReactNode;
}) {
  return (
    <div className="space-y-2">
      <Label>
        {label}
        {required ? <span className="ml-1 text-destructive">*</span> : null}
      </Label>
      {children}
    </div>
  );
}
function emptyTaskForm(): TaskFormState {
  return {
    name: '',
    goal: '',
    successCriteria: '',
    priority: 'normal',
    scheduleType: 'manual',
    runAt: '',
    cron: '0 9 * * 1-5',
    timezone:
      typeof Intl === 'undefined'
        ? 'UTC'
        : Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC',
    deliveryMode: 'task_only',
    timeout: '300',
    attempts: '1',
  };
}
function taskForm(task: DigitalHumanTask): TaskFormState {
  return {
    name: task.name,
    goal: task.goal,
    successCriteria: task.success_criteria.join('\n'),
    priority: task.priority,
    scheduleType: task.schedule.type,
    runAt: task.schedule.run_at ? localDateTime(task.schedule.run_at) : '',
    cron: task.schedule.cron ?? '0 9 * * 1-5',
    timezone: task.schedule.timezone || 'UTC',
    deliveryMode: task.delivery_mode,
    timeout: String(task.execution_timeout_seconds),
    attempts: String(task.max_attempts),
  };
}
function localDateTime(value: string) {
  const date = new Date(value);
  const offset = date.getTimezoneOffset() * 60_000;
  return new Date(date.getTime() - offset).toISOString().slice(0, 16);
}
function splitLines(value: string) {
  return value
    .split(/\r?\n/)
    .map(item => item.trim())
    .filter(Boolean);
}
function formatDate(value: string) {
  return new Date(value).toLocaleString();
}
function normalizeRunStatus(
  value: string
): 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled' | 'unknown' {
  return ['queued', 'running', 'succeeded', 'failed', 'canceled'].includes(value)
    ? (value as 'queued' | 'running' | 'succeeded' | 'failed' | 'canceled')
    : 'unknown';
}
function sourceLabel(task: DigitalHumanTask, t: ReturnType<typeof useT<'digitalHumans'>>) {
  if (task.source === 'console') return t('tasks.source.console');
  if (!task.source.startsWith('channel:')) return task.source || t('tasks.source.unknown');
  const providerID = task.source.slice('channel:'.length);
  const provider = digitalHumanChannelProvider(providerID);
  const providerName = provider ? t(provider.nameKey) : providerID;
  return task.source_conversation_type === 'group'
    ? t('tasks.source.channelGroup', { provider: providerName })
    : task.source_conversation_type === 'direct'
      ? t('tasks.source.channelDirect', { provider: providerName })
      : t('tasks.source.channel', { provider: providerName });
}
const taskActivityActions = [
  'created',
  'updated',
  'status_changed',
  'confirm',
  'decline',
  'run_now',
  'pause',
  'resume',
  'retry',
  'cancel',
  'archive',
  'run_finished',
  'proposal_expired',
] as const;
type TaskActivityAction = (typeof taskActivityActions)[number];
function isTaskActivityAction(value: string): value is TaskActivityAction {
  return (taskActivityActions as readonly string[]).includes(value);
}
function activityLabel(action: string, t: ReturnType<typeof useT<'digitalHumans'>>) {
  return isTaskActivityAction(action) ? t(`tasks.activity.${action}`) : action.replaceAll('_', ' ');
}
function confirmationCopy(
  action: 'decline' | 'cancel' | 'archive' | null,
  field: 'title' | 'description',
  t: ReturnType<typeof useT<'digitalHumans'>>
) {
  if (!action) return '';
  if (action === 'decline') {
    return field === 'title'
      ? t('tasks.confirm.declineTitle')
      : t('tasks.confirm.declineDescription');
  }
  if (action === 'cancel') {
    return field === 'title'
      ? t('tasks.confirm.cancelTitle')
      : t('tasks.confirm.cancelDescription');
  }
  return field === 'title'
    ? t('tasks.confirm.archiveTitle')
    : t('tasks.confirm.archiveDescription');
}
