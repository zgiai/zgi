'use client';

import { useCallback, useDeferredValue, useEffect, useRef, useState, type ReactNode } from 'react';
import Link from 'next/link';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';
import {
  AlertCircle,
  ArrowLeft,
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
  Webhook,
  XCircle,
} from 'lucide-react';
import { isAxiosError } from 'axios';
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
import { digitalHumanTaskDisplayStatus, digitalHumanTaskScheduleSummary } from '@/components/digital-human/digital-human-task-presentation';
import { DigitalHumanTaskActivityItem, DigitalHumanTaskHistoryPager, DigitalHumanTaskRunSummary } from '@/components/digital-human/digital-human-task-history';
import { TaskLatestReport, TaskReportFeed, TaskReportView } from '@/components/digital-human/digital-human-task-reports';
import { TaskReportDestinations } from '@/components/digital-human/digital-human-task-destinations';
import { NotificationChannelManager } from '@/components/notifications/notification-channel-manager';
import { TEAM_NOTIFICATION_CHANNELS_PATH } from '@/services/notification-channel.service';
import { useLocale } from '@/hooks/use-locale';
import {
  useDigitalHumanTaskDetail,
  useDigitalHumanTaskMutations,
  useDigitalHumanTasks,
} from '@/hooks/digital-human/use-digital-human-tasks';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { digitalHumanService } from '@/services/digital-human.service';
import type {
  CreateDigitalHumanTaskRequest,
  DigitalHumanTask,
  DigitalHumanTaskPriority,
  DigitalHumanTaskVisibility,
  DigitalHumanTaskScheduleType,
  DigitalHumanTaskSchedulePreview,
  DigitalHumanTaskStatus,
} from '@/services/types/digital-human';

type StatusFilter = 'open' | 'all' | DigitalHumanTaskStatus;
type PriorityFilter = 'all' | DigitalHumanTaskPriority;
type ScheduleFilter = 'all' | DigitalHumanTaskScheduleType;

const summaryStatuses: DigitalHumanTaskStatus[] = [
  'proposed',
  'pending',
  'in_progress',
  'waiting',
  'blocked',
  'completed',
  'canceled',
];
const taskParam = 'task';
const taskTimezones = ['UTC', ...(typeof Intl.supportedValuesOf === 'function'
  ? Intl.supportedValuesOf('timeZone')
  : ['Asia/Shanghai', 'Asia/Tokyo', 'Europe/London', 'America/New_York'])];

export function DigitalHumanTasks({ id }: { id: string }) {
  const t = useT('digitalHumans');
  const pathname = usePathname();
  const router = useRouter();
  const searchParams = useSearchParams();
  const permissions = useAccountPermissions();
  const canUpdate = permissions.hasPermission('digital_human.update');
  const canRun = permissions.hasPermission('digital_human.run');
  const urlSearch = searchParams.get('q') ?? '';
  const [search, setSearch] = useState(urlSearch);
  const pendingSearchRef = useRef<string | null>(null);
  const deferredSearch = useDeferredValue(search);
  const rawStatus = searchParams.get('status');
  const status: StatusFilter = rawStatus === 'all' || taskStatuses.includes(rawStatus as DigitalHumanTaskStatus)
    ? rawStatus as StatusFilter : 'open';
  const rawPriority = searchParams.get('priority');
  const priority: PriorityFilter = taskPriorities.includes(rawPriority as DigitalHumanTaskPriority)
    ? rawPriority as DigitalHumanTaskPriority : 'all';
  const rawSchedule = searchParams.get('schedule');
  const schedule: ScheduleFilter = taskSchedules.includes(rawSchedule as DigitalHumanTaskScheduleType)
    ? rawSchedule as DigitalHumanTaskScheduleType : 'all';
  const rawPage = Number(searchParams.get('page') ?? 1);
  const page = Number.isSafeInteger(rawPage) && rawPage > 0 && rawPage <= 10_000 ? rawPage : 1;
  const selectedTaskID = searchParams.get(taskParam) ?? '';
  const tab = searchParams.get('tab') === 'reports' ? 'reports' : 'tasks';
  const [editorTask, setEditorTask] = useState<DigitalHumanTask | null | undefined>();
  const [teamChannelsOpen, setTeamChannelsOpen] = useState(false);
  const query = useDigitalHumanTasks(id, {
    page,
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
  const updateParams = useCallback((updates: Record<string, string | null>) => {
    const params = new URLSearchParams(searchParams.toString());
    if (!('q' in updates) && search !== urlSearch) {
      if (search) params.set('q', search);
      else params.delete('q');
    }
    for (const [key, value] of Object.entries(updates)) {
      if (value) params.set(key, value);
      else params.delete(key);
    }
    const next = params.toString();
    if (params.get('q') !== searchParams.get('q')) pendingSearchRef.current = params.get('q') ?? '';
    router.replace(next ? `${pathname}?${next}` : pathname, { scroll: false });
  }, [pathname, router, search, searchParams, urlSearch]);
  useEffect(() => {
    if (pendingSearchRef.current === urlSearch) {
      pendingSearchRef.current = null;
      return;
    }
    setSearch(urlSearch);
  }, [urlSearch]);
  useEffect(() => {
    if (search === urlSearch) return;
    const timer = window.setTimeout(() => updateParams({ q: search || null, page: null }), 300);
    return () => window.clearTimeout(timer);
  }, [search, updateParams, urlSearch]);
  const setSelectedTaskID = (taskId?: string) => updateParams({ [taskParam]: taskId ?? null });
  const openReport = (reportId: string) => updateParams({ report: reportId });

  return (
    <main className="mx-auto flex min-h-full w-full max-w-[1600px] flex-col gap-5 px-4 py-6 md:px-8 md:py-8">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            <ListTodo className="size-5 text-primary" />
            <h1 className="text-2xl font-semibold tracking-tight">{t('tasks.title')}</h1>
          </div>
          <p className="max-w-3xl text-sm text-muted-foreground">
            {canUpdate ? t('tasks.descriptionManager') : t('tasks.description')}
          </p>
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
            <Button variant="outline" size="sm" onClick={() => setTeamChannelsOpen(true)}>
              <Webhook className="size-4" />
              {t('tasks.destinations.manage')}
            </Button>
          ) : null}
          {canUpdate ? (
            <Button size="sm" onClick={() => setEditorTask(null)}>
              <Plus className="size-4" />
              {t('tasks.create')}
            </Button>
          ) : null}
        </div>
      </div>

      <div role="tablist" className="flex gap-1 border-b">
        {(['tasks', 'reports'] as const).map(item => (
          <button
            key={item}
            type="button"
            role="tab"
            aria-selected={tab === item}
            className={cn(
              '-mb-px border-b-2 px-3 py-2 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground',
              tab === item && 'border-primary text-foreground',
              tab !== item && 'border-transparent'
            )}
            onClick={() => updateParams({ tab: item === 'tasks' ? null : item, page: null })}
          >
            {item === 'tasks' ? t('tasks.reports.tasksTab') : t('tasks.reports.tab')}
          </button>
        ))}
      </div>

      {tab === 'reports' ? (
        <TaskReportFeed digitalHumanId={id} onOpen={openReport} />
      ) : (
      <>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-7">
        {summaryStatuses.map(item => (
          <button
            key={item}
            type="button"
            className={cn(
              'rounded-xl border bg-card p-4 text-left outline-none transition-colors hover:border-primary/30 hover:bg-primary/[0.02] focus-visible:ring-2 focus-visible:ring-ring',
              status === item && 'border-primary/40 bg-primary/5'
            )}
            onClick={() => updateParams({ status: status === item ? null : item, page: null })}
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
              onChange={value => updateParams({ status: value === 'open' ? null : value, page: null })}
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
              onChange={value => updateParams({ priority: value === 'all' ? null : value, page: null })}
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
              onChange={value => updateParams({ schedule: value === 'all' ? null : value, page: null })}
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
                updateParams({ q: null, status: null, priority: null, schedule: null, page: null });
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
            <div className="hidden grid-cols-[minmax(240px,2fr)_105px_90px_130px_160px_130px] gap-4 border-b bg-muted/20 px-5 py-3 text-xs font-medium text-muted-foreground xl:grid">
              <span>{t('tasks.columns.name')}</span>
              <span>{t('tasks.columns.status')}</span>
              <span>{t('tasks.columns.priority')}</span>
              <span>{t('tasks.columns.schedule')}</span>
              <span>{t('tasks.columns.nextRun')}</span>
              <span>{t('tasks.columns.lastResult')}</span>
            </div>
            <div className="divide-y">
              {items.map(task => (
                <button
                  key={task.id}
                  type="button"
                  className="grid w-full gap-3 px-4 py-4 text-left outline-none transition-colors hover:bg-muted/25 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring xl:grid-cols-[minmax(240px,2fr)_105px_90px_130px_160px_minmax(160px,1fr)] xl:items-center xl:gap-4 xl:px-5"
                  onClick={() => setSelectedTaskID(task.id)}
                >
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{task.name}</p>
                    <p className="mt-1 line-clamp-1 text-xs text-muted-foreground">{task.goal}</p>
                    <p className="mt-1 flex min-w-0 items-center gap-1.5 text-[11px] text-muted-foreground/80">
                      <span className="truncate">{sourceLabel(task, t)}</span>
                      <TaskVisibilityBadge task={task} />
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
                  <TaskListCell label={t('tasks.columns.nextRun')}>
                    {task.next_run_at ? formatDate(task.next_run_at) : '—'}
                  </TaskListCell>
                  <TaskListCell label={t('tasks.columns.lastResult')}>
                    {task.latest_report ? (
                      <TaskLatestReport report={task.latest_report} onOpen={openReport} />
                    ) : task.last_run_status ? (
                      t(`tasks.runStatus.${normalizeRunStatus(task.last_run_status)}`)
                    ) : (
                      t('tasks.neverRun')
                    )}
                  </TaskListCell>
                </button>
              ))}
            </div>
          </>
        )}
        {query.data && (query.data.data.total > 0 || page > 1) ? (
          <div className="flex flex-wrap items-center justify-between gap-3 border-t px-4 py-3 text-sm text-muted-foreground">
            <span>{t('tasks.pagination.range', {
              from: items.length ? (page - 1) * 100 + 1 : 0,
              to: items.length ? (page - 1) * 100 + items.length : 0,
              total: query.data.data.total,
            })}</span>
            <div className="flex gap-2">
              <Button variant="outline" size="sm" disabled={page <= 1 || query.isFetching}
                onClick={() => updateParams({ page: page === 2 ? null : String(page - 1) })}>
                {t('tasks.pagination.previous')}
              </Button>
              <Button variant="outline" size="sm" disabled={!query.data.data.has_more || query.isFetching}
                onClick={() => updateParams({ page: String(page + 1) })}>
                {t('tasks.pagination.next')}
              </Button>
            </div>
          </div>
        ) : null}
      </section>
      </>
      )}

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
      {canUpdate ? (
        <Sheet open={teamChannelsOpen} onOpenChange={setTeamChannelsOpen}>
          <SheetContent side="right" className="w-[min(560px,100vw)] max-w-none overflow-y-auto sm:max-w-none">
            <SheetHeader className="text-left">
              <SheetTitle>{t('tasks.destinations.manage')}</SheetTitle>
              <SheetDescription>{t('tasks.destinations.manageDescription')}</SheetDescription>
            </SheetHeader>
            <div className="mt-4">
              <NotificationChannelManager basePath={TEAM_NOTIFICATION_CHANNELS_PATH} types={['webhook']} />
            </div>
          </SheetContent>
        </Sheet>
      ) : null}
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
    <div className="flex min-w-0 items-center justify-between gap-4 text-xs text-muted-foreground xl:block">
      <span className="shrink-0 font-medium text-foreground/70 xl:hidden">{label}</span>
      <div className="min-w-0 text-right xl:text-left">{children}</div>
    </div>
  );
}

function TaskStatusBadge({ task }: { task: DigitalHumanTask }) {
  const t = useT('digitalHumans');
  const status = digitalHumanTaskDisplayStatus(task);
  const variant =
    status === 'completed' || status === 'recurring_active'
      ? 'success'
      : status === 'blocked' || status === 'canceled'
        ? 'destructive'
        : status === 'in_progress'
          ? 'info'
          : status === 'proposed' || status === 'waiting'
            ? 'warning'
            : 'subtle';
  return <Badge variant={variant}>{t(`tasks.status.${status}`)}</Badge>;
}

// Workspace tasks are the default in the task center, so only narrower
// visibility is called out.
function TaskVisibilityBadge({ task }: { task: DigitalHumanTask }) {
  const t = useT('digitalHumans');
  if (!task.visibility || task.visibility === 'workspace') return null;
  return (
    <Badge variant="subtle" className="shrink-0 text-[10px]" title={t('tasks.visibilityNote')}>
      {t(`tasks.visibility.${task.visibility}`)}
    </Badge>
  );
}

function PriorityBadge({ priority }: { priority: DigitalHumanTaskPriority }) {
  const t = useT('digitalHumans');
  return (
    <Badge
      title={t('tasks.priorityNote')}
      variant={priority === 'urgent' ? 'destructive' : priority === 'high' ? 'warning' : 'outline'}
    >
      {t(`tasks.priority.${priority}`)}
    </Badge>
  );
}

function TaskScheduleLabel({ task }: { task: DigitalHumanTask }) {
  const t = useT('digitalHumans');
  const { locale } = useLocale();
  const label = digitalHumanTaskScheduleSummary(task, t, locale);
  return <span className="block max-w-full truncate" title={label}>{label}</span>;
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
  const { locale } = useLocale();
  const [runPage, setRunPage] = useState(1);
  const [activityPage, setActivityPage] = useState(1);
  // A report opens inside this sheet, in place of the task, rather than in a
  // second sheet stacked over it.
  const [reportView, setReportView] = useState<string | null>(null);
  useEffect(() => { setRunPage(1); setActivityPage(1); setReportView(null); }, [taskId]);
  const detail = useDigitalHumanTaskDetail(id, taskId, runPage, activityPage);
  const mutations = useDigitalHumanTaskMutations(id);
  const task = detail.task.data?.data;
  const actionPendingRef = useRef(false);
  const [confirmAction, setConfirmAction] = useState<'decline' | 'cancel' | 'archive' | null>(null);
  // A task requested by another account is read-only unless the reader manages
  // the task center; the server reports this per task.
  const taskChangeable = task?.can_change !== false;
  const allowUpdate = canUpdate && taskChangeable;
  const allowRun = canRun && taskChangeable;
  const perform = async (action: string) => {
    if (!open || !task || actionPendingRef.current) return;
    actionPendingRef.current = true;
    try {
      const latest = await detail.task.refetch();
      if (latest.isError || !latest.data?.data || latest.data.data.id !== taskId) {
        toast.error(t('tasks.loadError'));
        return;
      }
      if (latest.data.data.revision !== task.revision ||
        latest.data.data.work_status !== task.work_status ||
        latest.data.data.lifecycle_status !== task.lifecycle_status) {
        toast.error(t('tasks.actionStale'));
        return;
      }
      if (latest.data.data.can_change === false) {
        toast.error(t('tasks.sharedReadOnly'));
        return;
      }
      await mutations.action.mutateAsync({ taskId, action, revision: latest.data.data.revision });
      setConfirmAction(null);
      toast.success(t('tasks.actionApplied'));
    } catch {
      void detail.task.refetch();
    } finally {
      actionPendingRef.current = false;
    }
  };

  // The requester chooses the team channels of their shared task; a task
  // manager may only remove them. The server enforces both.
  const changeChannels = async (channelIds: string[]) => {
    if (!open || !task?.notify_channel_access || actionPendingRef.current) return;
    actionPendingRef.current = true;
    try {
      const latest = await detail.task.refetch();
      if (latest.isError || !latest.data?.data || latest.data.data.id !== taskId) {
        toast.error(t('tasks.loadError'));
        return;
      }
      if (latest.data.data.revision !== task.revision) {
        toast.error(t('tasks.actionStale'));
        return;
      }
      await mutations.update.mutateAsync({
        taskId,
        request: { expected_revision: latest.data.data.revision, notify_channel_ids: channelIds },
      });
      toast.success(t('tasks.destinations.updated'));
    } catch {
      void detail.task.refetch();
    } finally {
      actionPendingRef.current = false;
    }
  };

  // Visibility belongs to task center managers, including for tasks another
  // account requested. The server reports it per task, because it also
  // depends on whether task visibility is enabled.
  const changeVisibility = async (visibility: DigitalHumanTaskVisibility) => {
    if (
      !open ||
      !task ||
      !task.visibility_options?.includes(visibility) ||
      visibility === effectiveTaskVisibility(task) ||
      actionPendingRef.current
    ) {
      return;
    }
    actionPendingRef.current = true;
    try {
      const latest = await detail.task.refetch();
      if (latest.isError || !latest.data?.data || latest.data.data.id !== taskId) {
        toast.error(t('tasks.loadError'));
        return;
      }
      if (latest.data.data.revision !== task.revision) {
        toast.error(t('tasks.actionStale'));
        return;
      }
      await mutations.update.mutateAsync({
        taskId,
        request: { expected_revision: latest.data.data.revision, visibility },
      });
      toast.success(t('tasks.visibilityControl.updated'));
    } catch {
      void detail.task.refetch();
    } finally {
      actionPendingRef.current = false;
    }
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
        {reportView ? (
          <div className="min-h-0 flex-1 overflow-y-auto">
            <div className="space-y-4 p-6">
              <Button variant="ghost" size="sm" className="-ml-2" onClick={() => setReportView(null)}>
                <ArrowLeft className="size-4" />
                {t('tasks.reports.backToTask')}
              </Button>
              <TaskReportView digitalHumanId={id} reportId={reportView} mine={false} />
            </div>
          </div>
        ) : detail.task.isLoading || !task ? (
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
                <TaskVisibilityBadge task={task} />
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
                <DetailValue
                  label={t('tasks.visibilityControl.label')}
                  value={
                    <TaskVisibilityControl
                      task={task}
                      onChange={visibility => void changeVisibility(visibility)}
                    />
                  }
                />
              </section>
              <TaskReportDestinations task={task} onChange={changeChannels} />
              {(task.success_criteria ?? []).length ? (
                <section>
                  <h3 className="text-sm font-semibold">{t('tasks.fields.successCriteria')}</h3>
                  <ul className="mt-2 space-y-2">
                    {(task.success_criteria ?? []).map((item, index) => (
                      <li key={`${index}-${item}`} className="flex gap-2 text-sm">
                        <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-success" />
                        {item}
                      </li>
                    ))}
                  </ul>
                </section>
              ) : null}
              <section>
                <h3 className="text-sm font-semibold">{t('tasks.reports.history')}</h3>
                <div className="mt-3">
                  <TaskReportFeed digitalHumanId={id} taskId={task.id} onOpen={setReportView} />
                </div>
              </section>
              <section>
                <h3 className="text-sm font-semibold">{t('tasks.detail.runs')}</h3>
                <div className="mt-3 space-y-2">
                  {detail.runs.isLoading ? (
                    <Skeleton className="h-20" />
                  ) : detail.runs.isError ? (
                    <Button variant="outline" size="sm" onClick={() => void detail.runs.refetch()}>{t('tasks.retry')}</Button>
                  ) : (detail.runs.data?.data.items ?? []).length === 0 ? (
                    <p className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
                      {t('tasks.detail.noRuns')}
                    </p>
                  ) : (
                    (detail.runs.data?.data.items ?? []).map(run => (
                      <DigitalHumanTaskRunSummary key={run.id} run={run}>
                        {run.digital_human_run_id ? (
                          <Button asChild variant="ghost" size="sm" className="mt-2">
                            <Link href={`/console/digital-humans/${id}/runs?run=${run.digital_human_run_id}`}>
                              {t('tasks.detail.viewLog')}
                            </Link>
                          </Button>
                        ) : null}
                      </DigitalHumanTaskRunSummary>
                    ))
                  )}
                </div>
                <DigitalHumanTaskHistoryPager data={detail.runs.data?.data ?? null} page={runPage} onPage={setRunPage} />
              </section>
              <section>
                <h3 className="text-sm font-semibold">{t('tasks.detail.activities')}</h3>
                <ol className="mt-3 space-y-0">
                  {detail.activities.isLoading ? (
                    <Skeleton className="h-28" />
                  ) : detail.activities.isError ? (
                    <Button variant="outline" size="sm" onClick={() => void detail.activities.refetch()}>{t('tasks.retry')}</Button>
                  ) : (detail.activities.data?.data.items ?? []).length === 0 ? (
                    <p className="text-sm text-muted-foreground">{t('tasks.detail.noActivities')}</p>
                  ) : (
                    (detail.activities.data?.data.items ?? []).map((activity, index, items) => (
                      <DigitalHumanTaskActivityItem key={activity.id} activity={activity} locale={locale}
                        isLast={index === items.length - 1} defaultOpen={index === 0} />
                    ))
                  )}
                </ol>
                <DigitalHumanTaskHistoryPager data={detail.activities.data?.data ?? null} page={activityPage} onPage={setActivityPage} />
              </section>
            </div>
          </div>
        )}
        {task ? (
          <div className="flex shrink-0 flex-wrap items-center justify-end gap-2 border-t bg-background px-6 py-4">
            {!taskChangeable ? (
              <p className="mr-auto text-xs text-muted-foreground">{t('tasks.sharedReadOnly')}</p>
            ) : null}
            {allowUpdate && task.work_status !== 'canceled' ? (
              <Button variant="outline" size="sm" onClick={() => onEdit(task)}>
                <Pencil className="size-4" />
                {t('common.edit')}
              </Button>
            ) : null}
            {task.work_status === 'proposed' && allowUpdate ? (
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
            {task.lifecycle_status === 'active' && task.work_status === 'pending' && allowUpdate ? (
              <Button variant="outline" size="sm" onClick={() => void perform('pause')}>
                <Pause className="size-4" />
                {t('tasks.actions.pause')}
              </Button>
            ) : null}
            {task.lifecycle_status === 'paused' &&
            !['canceled', 'completed'].includes(task.work_status) &&
            allowUpdate ? (
              <Button variant="outline" size="sm" onClick={() => void perform('resume')}>
                <Play className="size-4" />
                {t('tasks.actions.resume')}
              </Button>
            ) : null}
            {task.work_status === 'blocked' && allowRun ? (
              <Button size="sm" onClick={() => void perform('retry')}>
                <RotateCcw className="size-4" />
                {t('tasks.actions.retry')}
              </Button>
            ) : null}
            {!['proposed', 'in_progress', 'canceled', 'blocked'].includes(task.work_status) && allowRun ? (
              <Button
                size="sm"
                onClick={() => void perform('run_now')}
                disabled={mutations.action.isPending}
              >
                <Play className="size-4" />
                {t('tasks.actions.runNow')}
              </Button>
            ) : null}
            {!['completed', 'canceled'].includes(task.work_status) && allowRun ? (
              <Button variant="outline" size="sm" onClick={() => setConfirmAction('cancel')}>
                <XCircle className="size-4" />
                {t('tasks.actions.cancel')}
              </Button>
            ) : null}
            {['completed', 'canceled'].includes(task.work_status) && allowUpdate ? (
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

function effectiveTaskVisibility(task: DigitalHumanTask): DigitalHumanTaskVisibility {
  if (task.visibility) return task.visibility;
  if (task.owner_kind === 'conversation') return 'conversation';
  if (task.owner_kind === 'user_subject') return 'requester';
  return 'workspace';
}

// The server lists the visibilities this reader may set: the requester shares
// or withdraws their task, and a task manager only narrows someone else's.
function TaskVisibilityControl({
  task,
  onChange,
}: {
  task: DigitalHumanTask;
  onChange: (visibility: DigitalHumanTaskVisibility) => void;
}) {
  const t = useT('digitalHumans');
  const options = task.visibility_options ?? [];
  const stored = effectiveTaskVisibility(task);
  // A requester-only visibility from an older build shows as this task's
  // requester-only option.
  const current = options.includes(stored)
    ? stored
    : (options.find(option => option !== 'workspace' && stored !== 'workspace') ?? stored);
  const hint = t(`tasks.visibilityControl.hint.${current}`);
  if (options.length < 2) {
    return (
      <div>
        <span>{t(`tasks.visibility.${current}`)}</span>
        <p className="mt-0.5 text-xs text-muted-foreground">{hint}</p>
      </div>
    );
  }
  return (
    <div>
      <Select value={current} onValueChange={value => onChange(value as DigitalHumanTaskVisibility)}>
        <SelectTrigger className="h-8 w-full sm:w-56" aria-label={t('tasks.visibilityControl.label')}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {options.map(option => (
            <SelectItem key={option} value={option}>
              {t(`tasks.visibility.${option}`)}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <p className="mt-1 text-xs text-muted-foreground">{hint}</p>
    </div>
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
  const [schedulePreview, setSchedulePreview] = useState<{
    key: string;
    value?: DigitalHumanTaskSchedulePreview;
    error?: 'invalid' | 'unavailable';
  }>({ key: '' });
  const [previewRetry, setPreviewRetry] = useState(0);
  useEffect(() => {
    if (open) {
      setForm(task ? taskForm(task) : emptyTaskForm());
      setSchedulePreview({ key: '' });
    }
  }, [open, task]);
  const set = <K extends keyof TaskFormState>(key: K, value: TaskFormState[K]) =>
    setForm(current => ({ ...current, [key]: value }));
  const timezone = form.timezone.trim() || 'UTC';
  const scheduleInput = form.scheduleType === 'once' ? form.runAt : form.cron.trim();
  const scheduleKey = JSON.stringify([form.scheduleType, scheduleInput, timezone]);
  useEffect(() => {
    if (!open || form.scheduleType === 'manual' || !scheduleInput) return;
    let active = true;
    const timer = window.setTimeout(() => {
      void digitalHumanService.previewTaskSchedule(id, {
        type: form.scheduleType === 'once' ? 'once' : 'cron',
        timezone,
        ...(form.scheduleType === 'once' ? { local_run_at: scheduleInput } : { cron: scheduleInput }),
      }).then(response => {
        if (active) setSchedulePreview({ key: scheduleKey, value: response.data });
      }).catch(error => {
        if (active) {
          setSchedulePreview({
            key: scheduleKey,
            error: isAxiosError(error) && error.response?.status === 400 ? 'invalid' : 'unavailable',
          });
        }
      });
    }, 350);
    return () => { active = false; window.clearTimeout(timer); };
  }, [form.scheduleType, id, open, previewRetry, scheduleInput, scheduleKey, timezone]);
  const previewReady = form.scheduleType === 'manual' ||
    (schedulePreview.key === scheduleKey && Boolean(schedulePreview.value?.next_run_at));
  const timeout = Number(form.timeout);
  const attempts = Number(form.attempts);
  const valid = Boolean(
    form.name.trim() && form.goal.trim() && previewReady &&
    Number.isInteger(timeout) && timeout >= 30 && timeout <= 1800 &&
    Number.isInteger(attempts) && attempts >= 1 && attempts <= 3
  );
  const submit = async () => {
    if (!valid) return;
    const schedule = form.scheduleType === 'manual'
      ? { type: 'manual' as const }
      : schedulePreview.value?.schedule;
    if (!schedule) return;
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
      let latest;
      try {
        latest = await digitalHumanService.getTask(id, task.id);
      } catch {
        toast.error(t('tasks.loadError'));
        return;
      }
      if (latest.data.revision !== task.revision) {
        toast.error(t('tasks.editor.conflict'));
        return;
      }
    }
    try {
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
    } catch {
      return;
    }
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
              <p className="text-xs text-muted-foreground">{t('tasks.priorityNote')}</p>
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
              <Select value={timezone} onValueChange={value => set('timezone', value)}>
                <SelectTrigger><SelectValue /></SelectTrigger>
                <SelectContent className="max-h-72">
                  {!taskTimezones.includes(timezone) ? <SelectItem value={timezone}>{timezone}</SelectItem> : null}
                  {taskTimezones.map(value => <SelectItem key={value} value={value}>{value}</SelectItem>)}
                </SelectContent>
              </Select>
            </Field>
          ) : null}
          {form.scheduleType !== 'manual' && scheduleInput ? (
            <div role="status" className={cn('flex flex-wrap items-center gap-2 text-sm',
              schedulePreview.key === scheduleKey && schedulePreview.error === 'invalid' ? 'text-destructive' : 'text-muted-foreground')}>
              <span>{schedulePreview.key !== scheduleKey
                ? t('tasks.editor.checkingSchedule')
                : schedulePreview.error === 'invalid'
                  ? t('tasks.editor.invalidSchedule')
                  : schedulePreview.error === 'unavailable'
                    ? t('tasks.editor.previewUnavailable')
                    : schedulePreview.value?.next_run_at
                      ? t('tasks.editor.nextRunPreview', { value: new Date(schedulePreview.value.next_run_at).toLocaleString(undefined, { timeZone: timezone }) })
                      : t('tasks.editor.invalidSchedule')}</span>
              {schedulePreview.key === scheduleKey && schedulePreview.error === 'unavailable' ? (
                <Button variant="outline" size="sm" onClick={() => {
                  setSchedulePreview({ key: '' });
                  setPreviewRetry(value => value + 1);
                }}>{t('tasks.retry')}</Button>
              ) : null}
            </div>
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
                {(!Number.isInteger(timeout) || timeout < 30 || timeout > 1800) ? (
                  <p role="alert" className="text-xs text-destructive">{t('tasks.editor.timeoutRange')}</p>
                ) : null}
              </Field>
              <Field label={t('tasks.fields.attempts')}>
                <Input
                  type="number"
                  min={1}
                  max={3}
                  value={form.attempts}
                  onChange={event => set('attempts', event.target.value)}
                />
                {(!Number.isInteger(attempts) || attempts < 1 || attempts > 3) ? (
                  <p role="alert" className="text-xs text-destructive">{t('tasks.editor.attemptsRange')}</p>
                ) : null}
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
    successCriteria: (task.success_criteria ?? []).join('\n'),
    priority: task.priority,
    scheduleType: task.schedule.type,
    runAt: task.schedule.run_at ? localDateTime(task.schedule.run_at, task.schedule.timezone || 'UTC') : '',
    cron: task.schedule.cron ?? '0 9 * * 1-5',
    timezone: task.schedule.timezone || 'UTC',
    deliveryMode: task.delivery_mode,
    timeout: String(task.execution_timeout_seconds),
    attempts: String(task.max_attempts),
  };
}
function localDateTime(value: string, timezone: string) {
  const parts = Object.fromEntries(new Intl.DateTimeFormat('en-US', {
    timeZone: timezone, year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', hourCycle: 'h23',
  }).formatToParts(new Date(value)).map(part => [part.type, part.value]));
  return `${parts.year}-${parts.month}-${parts.day}T${parts.hour}:${parts.minute}`;
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
