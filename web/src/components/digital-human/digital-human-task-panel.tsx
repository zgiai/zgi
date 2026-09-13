'use client';

import { useCallback, useDeferredValue, useEffect, useMemo, useRef, useState } from 'react';
import {
  Archive,
  ArrowRight,
  AlertTriangle,
  CalendarClock,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  CircleDot,
  Inbox,
  Info,
  ListTodo,
  Loader2,
  Pause,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  RotateCcw,
  X,
} from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Textarea } from '@/components/ui/textarea';
import { Sheet, SheetContent, SheetTitle } from '@/components/ui/sheet';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useT } from '@/i18n';
import { useLocale } from '@/hooks/use-locale';
import { getErrorMessage } from '@/utils/error-notifications';
import { translateTimezone } from '@/utils/timezone';
import { digitalHumanTaskDisplayStatus } from '@/components/digital-human/digital-human-task-presentation';
import type {
  CreateDigitalHumanTaskRequest,
  DigitalHumanTask,
  DigitalHumanTaskActivity,
  DigitalHumanTaskList,
  DigitalHumanTaskPage,
  DigitalHumanTaskRun,
  UpdateDigitalHumanTaskRequest,
} from '@/services/types/digital-human';
import type { ApiResponseData } from '@/services/types/common';

const taskStatusFilters = [
  'open',
  'proposed',
  'pending',
  'in_progress',
  'waiting',
  'blocked',
  'completed',
  'canceled',
] as const;
const taskPriorityFilters = ['low', 'normal', 'high', 'urgent'] as const;

export interface DigitalHumanTaskPanelClient {
  list: (params?: Record<string, unknown>) => Promise<ApiResponseData<DigitalHumanTaskList>>;
  create: (request: CreateDigitalHumanTaskRequest) => Promise<ApiResponseData<DigitalHumanTask>>;
  get: (taskId: string) => Promise<ApiResponseData<DigitalHumanTask>>;
  update: (
    taskId: string,
    request: UpdateDigitalHumanTaskRequest
  ) => Promise<ApiResponseData<DigitalHumanTask>>;
  action: (
    taskId: string,
    action: string,
    revision: number
  ) => Promise<ApiResponseData<DigitalHumanTask>>;
  runs: (taskId: string) => Promise<ApiResponseData<DigitalHumanTaskPage<DigitalHumanTaskRun>>>;
  activities: (
    taskId: string
  ) => Promise<ApiResponseData<DigitalHumanTaskPage<DigitalHumanTaskActivity>>>;
}

export function DigitalHumanTaskPanel({
  client,
  title = 'my',
  compact = false,
  refreshKey = 0,
}: {
  client: DigitalHumanTaskPanelClient;
  title?: 'my' | 'shared';
  compact?: boolean;
  refreshKey?: number;
}) {
  const t = useT('digitalHumans');
  const { locale } = useLocale();
  const [tasks, setTasks] = useState<DigitalHumanTaskList | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [detailOpen, setDetailOpen] = useState(false);
  const [runs, setRuns] = useState<DigitalHumanTaskRun[]>([]);
  const [activities, setActivities] = useState<DigitalHumanTaskActivity[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState({ name: '', goal: '' });
  const [search, setSearch] = useState('');
  const [status, setStatus] = useState('open');
  const [priority, setPriority] = useState('all');
  const deferredSearch = useDeferredValue(search);
  const loadErrorText = t('tasks.loadError');
  const clientRef = useRef(client);
  const loadErrorTextRef = useRef(loadErrorText);
  const lastLoadFilterKeyRef = useRef<string | null>(null);
  clientRef.current = client;
  loadErrorTextRef.current = loadErrorText;
  const selected = useMemo(
    () => tasks?.items.find(item => item.id === selectedId) ?? null,
    [selectedId, tasks]
  );

  const load = useCallback(async () => {
    try {
      const statusValues =
        status === 'open'
          ? ['proposed', 'pending', 'in_progress', 'waiting', 'blocked']
          : status !== 'all'
            ? [status]
            : undefined;
      const response = await clientRef.current.list({
        page: 1,
        page_size: 100,
        ...(deferredSearch.trim() ? { query: deferredSearch.trim() } : {}),
        ...(statusValues ? { status: statusValues } : {}),
        ...(priority !== 'all' ? { priority: [priority] } : {}),
      });
      setTasks(response.data);
      setSelectedId(current =>
        response.data.items.some(item => item.id === current)
          ? current
          : (response.data.items[0]?.id ?? null)
      );
    } catch (error) {
      toast.error(getErrorMessage(error) || loadErrorTextRef.current);
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, [deferredSearch, priority, status]);

  const loadFilterKey = `${deferredSearch}\u0000${priority}\u0000${status}\u0000${refreshKey}`;
  useEffect(() => {
    if (lastLoadFilterKeyRef.current === loadFilterKey) return;
    if (refreshKey > 0) setRefreshing(true);
    lastLoadFilterKeyRef.current = loadFilterKey;
    void load();
  }, [load, loadFilterKey, refreshKey]);

  // Scheduler state changes happen outside the chat turn. Keep the panel
  // live while a task can still transition, then stop polling once it reaches
  // a terminal/blocked state so completed history does not create background
  // traffic.
  const shouldPoll = useMemo(
    () =>
      tasks?.items.some(task =>
        task.work_status === 'in_progress' ||
        ((task.work_status === 'pending' || task.work_status === 'waiting') &&
          Boolean(task.next_run_at))
      ) ?? false,
    [tasks]
  );

  useEffect(() => {
    if (!shouldPoll) return;
    const timer = window.setInterval(() => {
      void load();
    }, 5000);
    return () => window.clearInterval(timer);
  }, [load, shouldPoll]);

  useEffect(() => {
    if (!selected) {
      setRuns([]);
      setActivities([]);
      return;
    }
    void Promise.all([
      clientRef.current.runs(selected.id),
      clientRef.current.activities(selected.id),
    ])
      .then(([runResponse, activityResponse]) => {
        setRuns(runResponse.data.items);
        setActivities(activityResponse.data.items);
      })
      .catch(() => {
        setRuns([]);
        setActivities([]);
      });
  }, [selectedId, selected]);

  const visibleTasks = useMemo(() => tasks?.items ?? [], [tasks]);

  const select = (task: DigitalHumanTask) => {
    setSelectedId(task.id);
    setEditing(false);
    setDraft({ name: task.name, goal: task.goal });
    if (compact) setDetailOpen(true);
  };

  const create = async () => {
    if (!draft.name.trim() || !draft.goal.trim()) return;
    try {
      await client.create({
        name: draft.name.trim(),
        goal: draft.goal.trim(),
        schedule: { type: 'manual' },
        delivery_mode: 'task_only',
      });
      toast.success(t('tasks.created'));
      setDraft({ name: '', goal: '' });
      setEditing(false);
      await load();
    } catch (error) {
      toast.error(getErrorMessage(error) || t('tasks.loadError'));
    }
  };

  const update = async () => {
    if (!selected || !draft.name.trim() || !draft.goal.trim()) return;
    try {
      await clientRef.current.update(selected.id, {
        name: draft.name.trim(),
        goal: draft.goal.trim(),
        expected_revision: selected.revision,
      });
      toast.success(t('tasks.updated'));
      setEditing(false);
      await load();
    } catch (error) {
      toast.error(getErrorMessage(error) || t('tasks.loadError'));
      await load();
    }
  };

  const action = async (name: string) => {
    if (!selected) return;
    try {
      await clientRef.current.action(selected.id, name, selected.revision);
      toast.success(t('tasks.actionApplied'));
      await load();
    } catch (error) {
      toast.error(getErrorMessage(error) || t('tasks.loadError'));
      await load();
    }
  };

  const titleText = title === 'my' ? t('tasks.myTitle') : t('tasks.title');
  // Lifecycle operations are deterministic user controls. Keep content edits
  // and proposal confirmation in the conversation, where the model can
  // explain the change and collect any missing details.
  const allowDirectMutations = title === 'shared' || title === 'my';
  const allowDirectEdit = title === 'shared' || !compact;
  return (
    <section
      className={
        compact ? 'flex h-full min-h-0 flex-col bg-card' : 'rounded-xl border bg-background'
      }
    >
      <div className="flex items-start justify-between gap-3 border-b px-4 py-3.5">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <span className="flex size-7 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <ListTodo className="size-4" />
            </span>
            <h2 className="truncate font-semibold">{titleText}</h2>
            {compact && title === 'my' ? (
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    type="button"
                    className="inline-flex size-5 shrink-0 items-center justify-center rounded-full text-muted-foreground/80 transition-colors hover:bg-muted hover:text-foreground"
                    aria-label={t('tasks.createFromChat')}
                  >
                    <Info className="size-3.5" />
                  </button>
                </TooltipTrigger>
                <TooltipContent side="left" className="max-w-64 leading-5">
                  <span className="block">{t('tasks.myDescription')}</span>
                  <span className="mt-1 block">{t('tasks.createFromChat')}</span>
                </TooltipContent>
              </Tooltip>
            ) : null}
          </div>
          {!compact ? (
            <p className="mt-1 text-xs text-muted-foreground">
              {title === 'my' ? t('tasks.myDescription') : t('tasks.description')}
            </p>
          ) : null}
        </div>
        <div className="flex shrink-0 gap-1">
          <Button
            variant="ghost"
            size="sm"
            className="h-8 w-8 p-0"
            onClick={() => {
              setRefreshing(true);
              void load();
            }}
            disabled={refreshing}
            aria-label={t('common.refresh')}
          >
            <RefreshCw className={refreshing ? 'size-4 animate-spin' : 'size-4'} />
          </Button>
          {!compact ? (
            <Button
              variant="ghost"
              size="sm"
              className="h-8 w-8 p-0"
              onClick={() => {
                setSelectedId(null);
                setEditing(true);
                setDraft({ name: '', goal: '' });
              }}
              aria-label={t('tasks.create')}
            >
              <Plus className="size-4" />
            </Button>
          ) : null}
        </div>
      </div>
      <div
        className={
          compact
            ? 'grid gap-2 border-b p-3'
            : 'grid gap-2 border-b p-3 sm:grid-cols-[minmax(0,1fr)_auto_auto]'
        }
      >
        <Input
          value={search}
          onChange={event => setSearch(event.target.value)}
          placeholder={t('tasks.filters.search')}
          aria-label={t('tasks.filters.search')}
        />
        <div className={compact ? 'grid grid-cols-2 gap-2' : 'contents'}>
          <Select value={status} onValueChange={setStatus}>
            <SelectTrigger className="h-9 min-w-0" aria-label={t('tasks.filters.status')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t('tasks.filters.allStatuses')}</SelectItem>
              {taskStatusFilters.map(value => (
                <SelectItem key={value} value={value}>
                  {t(`tasks.status.${value}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Select value={priority} onValueChange={setPriority}>
            <SelectTrigger className="h-9 min-w-0" aria-label={t('tasks.filters.priority')}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t('tasks.filters.allPriorities')}</SelectItem>
              {taskPriorityFilters.map(value => (
                <SelectItem key={value} value={value}>
                  {t(`tasks.priority.${value}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>
      <div
        className={
          compact
            ? 'flex min-h-0 flex-1 flex-col'
            : 'grid gap-4 p-4 md:grid-cols-[minmax(12rem,18rem)_minmax(0,1fr)]'
        }
      >
        <div
          className={
            compact
              ? 'min-h-0 flex-1 overflow-y-auto border-b p-2'
              : 'min-h-0 overflow-y-auto border-r p-2'
          }
        >
          {loading ? (
            <Loader2 className="mx-auto mt-6 size-4 animate-spin" />
          ) : visibleTasks.length === 0 ? (
            <div className="flex min-h-28 flex-col items-center justify-center gap-2 px-3 py-5 text-center">
              <Inbox className="size-6 text-muted-foreground/60" />
              <p className="text-xs text-muted-foreground">
                {title === 'my' ? t('tasks.myEmpty') : t('tasks.empty')}
              </p>
              {!compact ? (
                <Button
                  variant="ghost"
                  size="sm"
                  className="h-8 gap-1.5 text-xs"
                  onClick={() => {
                    setSelectedId(null);
                    setEditing(true);
                    setDraft({ name: '', goal: '' });
                  }}
                >
                  <Plus className="size-3.5" />
                  {t('tasks.create')}
                </Button>
              ) : null}
            </div>
          ) : (
            visibleTasks.map(task => {
              const displayStatus = digitalHumanTaskDisplayStatus(task);
              return (
                <button
                  key={task.id}
                  type="button"
                  onClick={() => select(task)}
                  className={`group mb-1 w-full rounded-lg border p-2.5 text-left text-sm transition-colors hover:border-primary/30 hover:bg-muted/70 ${selected?.id === task.id ? 'border-primary/40 bg-muted shadow-sm' : 'border-transparent'}`}
                  aria-label={`${task.name}, ${t(`tasks.status.${displayStatus}`)}`}
                >
                  <div className="flex min-w-0 items-start gap-2">
                    <div className="min-w-0 flex-1">
                      <div className="truncate font-medium">{task.name}</div>
                      <div className="mt-1 flex flex-wrap items-center gap-1">
                        <Badge variant="outline" className="text-[10px]">
                          {t(`tasks.status.${displayStatus}`)}
                        </Badge>
                        {task.priority !== 'normal' ? (
                          <Badge variant="secondary" className="text-[10px]">
                            {t(`tasks.priority.${task.priority}`)}
                          </Badge>
                        ) : null}
                      </div>
                    </div>
                    <ChevronRight className="mt-0.5 size-4 shrink-0 text-muted-foreground/50 transition-transform group-hover:translate-x-0.5 group-hover:text-foreground" />
                  </div>
                  <div className="mt-1.5 flex min-w-0 items-center gap-1 text-[11px] text-muted-foreground">
                    <CalendarClock className="size-3 shrink-0" />
                    <span className="truncate">{taskScheduleSummary(task, t, locale)}</span>
                  </div>
                  {task.next_run_at ? (
                    <div className="mt-0.5 truncate pl-4 text-[11px] text-muted-foreground/80">
                      {t('tasks.fields.nextRun')} · {formatTaskDate(task.next_run_at)}
                    </div>
                  ) : null}
                  {task.last_run_at ? (
                    <div className="mt-0.5 truncate pl-4 text-[11px] text-muted-foreground/70">
                      {t('tasks.fields.lastRun')} · {formatTaskDate(task.last_run_at)}
                    </div>
                  ) : null}
                </button>
              );
            })
          )}
        </div>
        {!compact ? (
          <div className="min-w-0 p-4">
            <TaskDetailContent
              selected={selected}
              editing={editing}
              draft={draft}
              runs={runs}
              activities={activities}
              title={title}
              onDraftChange={setDraft}
              onCreate={create}
              onUpdate={update}
              onCancelEdit={() => setEditing(false)}
              onAction={action}
              allowProposalApproval
              allowDirectMutations={allowDirectMutations}
              allowDirectEdit={allowDirectEdit}
              onStartEdit={task => {
                setDraft({ name: task.name, goal: task.goal });
                setEditing(true);
              }}
            />
          </div>
        ) : null}
      </div>
      {compact ? (
        <Sheet open={detailOpen && Boolean(selected)} onOpenChange={setDetailOpen}>
          <SheetContent side="right" className="w-full overflow-y-auto p-0 sm:max-w-lg">
            <SheetTitle className="sr-only">{selected?.name ?? t('tasks.myTitle')}</SheetTitle>
            <div className="p-4">
              <TaskDetailContent
                selected={selected}
                editing={editing}
                draft={draft}
                runs={runs}
                activities={activities}
                title={title}
                onDraftChange={setDraft}
                onCreate={create}
                onUpdate={update}
                onCancelEdit={() => setEditing(false)}
                onAction={action}
                allowProposalApproval={false}
                allowDirectMutations={allowDirectMutations}
                allowDirectEdit={allowDirectEdit}
                onStartEdit={task => {
                  setDraft({ name: task.name, goal: task.goal });
                  setEditing(true);
                }}
              />
            </div>
          </SheetContent>
        </Sheet>
      ) : null}
    </section>
  );
}

interface TaskDraft {
  name: string;
  goal: string;
}

function TaskDetailContent({
  selected,
  editing,
  draft,
  runs,
  activities,
  title,
  onDraftChange,
  onCreate,
  onUpdate,
  onCancelEdit,
  onAction,
  allowProposalApproval,
  allowDirectMutations,
  allowDirectEdit,
  onStartEdit,
}: {
  selected: DigitalHumanTask | null;
  editing: boolean;
  draft: TaskDraft;
  runs: DigitalHumanTaskRun[];
  activities: DigitalHumanTaskActivity[];
  title: 'my' | 'shared';
  onDraftChange: (draft: TaskDraft) => void;
  onCreate: () => Promise<void>;
  onUpdate: () => Promise<void>;
  onCancelEdit: () => void;
  onAction: (name: string) => Promise<void>;
  allowProposalApproval: boolean;
  allowDirectMutations: boolean;
  allowDirectEdit: boolean;
  onStartEdit: (task: DigitalHumanTask) => void;
}) {
  const t = useT('digitalHumans');
  const { locale } = useLocale();
  const [confirmingAction, setConfirmingAction] = useState<'cancel' | 'archive' | null>(null);

  if (editing) {
    return (
      <div className="space-y-4">
        <div>
          <p className="text-sm font-semibold">
            {selected ? t('tasks.editor.editTitle') : t('tasks.editor.createTitle')}
          </p>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            {t('tasks.editor.description')}
          </p>
        </div>
        <Input
          value={draft.name}
          onChange={event => onDraftChange({ ...draft, name: event.target.value })}
          placeholder={t('tasks.editor.namePlaceholder')}
        />
        <Textarea
          value={draft.goal}
          onChange={event => onDraftChange({ ...draft, goal: event.target.value })}
          placeholder={t('tasks.editor.goalPlaceholder')}
          className="min-h-28 resize-none"
        />
        <div className="flex gap-2">
          <Button className="flex-1" onClick={() => void (selected ? onUpdate() : onCreate())}>
            {selected ? t('common.save') : t('tasks.create')}
          </Button>
          <Button variant="ghost" onClick={onCancelEdit}>
            <X className="mr-1 size-4" />
            {t('common.cancel')}
          </Button>
        </div>
      </div>
    );
  }

  if (!selected) {
    return (
      <div className="flex h-full min-h-40 flex-col items-center justify-center gap-2 text-center">
        <CheckCircle2 className="size-7 text-muted-foreground/50" />
        <p className="text-sm font-medium text-foreground/80">{t('tasks.empty')}</p>
        <p className="max-w-[18rem] text-xs leading-5 text-muted-foreground">
          {title === 'my' ? t('tasks.myEmptyDescription') : t('tasks.emptyDescription')}
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div>
        <h3 className="font-semibold">{selected.name}</h3>
        <p className="mt-1 text-sm text-muted-foreground">{selected.goal}</p>
      </div>
      <div className="flex flex-wrap gap-2">
        <Badge>{t(`tasks.status.${digitalHumanTaskDisplayStatus(selected)}`)}</Badge>
        {allowDirectMutations && selected.work_status === 'proposed' && allowProposalApproval ? (
          <Button size="sm" onClick={() => void onAction('confirm')}>
            {t('tasks.actions.confirm')}
          </Button>
        ) : null}
        {!allowDirectMutations && selected.work_status === 'proposed' ? (
          <span className="self-center text-xs text-muted-foreground">
            {t('tasks.approvalInChat')}
          </span>
        ) : null}
        {allowDirectMutations &&
        ['pending', 'waiting', 'blocked'].includes(selected.work_status) ? (
          <Button size="sm" onClick={() => void onAction('run_now')}>
            <Play className="mr-1 size-3" />
            {t('tasks.actions.runNow')}
          </Button>
        ) : null}
        {allowDirectMutations && selected.lifecycle_status === 'active' ? (
          <Button size="sm" variant="outline" onClick={() => void onAction('pause')}>
            <Pause className="mr-1 size-3" />
            {t('tasks.actions.pause')}
          </Button>
        ) : allowDirectMutations && selected.lifecycle_status === 'paused' ? (
          <Button size="sm" variant="outline" onClick={() => void onAction('resume')}>
            <Play className="mr-1 size-3" />
            {t('tasks.actions.resume')}
          </Button>
        ) : null}
        {allowDirectMutations && selected.work_status === 'blocked' ? (
          <Button size="sm" variant="outline" onClick={() => void onAction('retry')}>
            <RotateCcw className="mr-1 size-3" />
            {t('tasks.actions.retry')}
          </Button>
        ) : null}
        {allowDirectMutations &&
        !['completed', 'canceled'].includes(selected.work_status) &&
        selected.lifecycle_status !== 'archived' ? (
          <Button size="sm" variant="outline" onClick={() => setConfirmingAction('cancel')}>
            {t('tasks.actions.cancel')}
          </Button>
        ) : null}
        {allowDirectMutations && selected.lifecycle_status !== 'archived' ? (
          <Button size="sm" variant="ghost" onClick={() => setConfirmingAction('archive')}>
            <Archive className="mr-1 size-3" />
            {title === 'my' ? t('tasks.actions.remove') : t('tasks.actions.archive')}
          </Button>
        ) : null}
        {allowDirectEdit ? (
          <Button size="sm" variant="ghost" onClick={() => onStartEdit(selected)}>
            {t('tasks.editor.editTitle')}
          </Button>
        ) : null}
      </div>
      <ConfirmDialog
        open={confirmingAction !== null}
        onOpenChange={open => {
          if (!open) setConfirmingAction(null);
        }}
        title={
          confirmingAction === 'cancel'
            ? t('tasks.confirm.cancelTitle')
            : t(title === 'my' ? 'tasks.confirm.removeTitle' : 'tasks.confirm.archiveTitle')
        }
        description={
          confirmingAction === 'cancel'
            ? t('tasks.confirm.cancelDescription')
            : t(title === 'my' ? 'tasks.confirm.removeDescription' : 'tasks.confirm.archiveDescription')
        }
        confirmText={
          confirmingAction === 'cancel'
            ? t('tasks.actions.cancel')
            : t(title === 'my' ? 'tasks.actions.remove' : 'tasks.actions.archive')
        }
        variant="warning"
        onConfirm={() => {
          const action = confirmingAction;
          setConfirmingAction(null);
          if (action) void onAction(action);
        }}
      />
      <section className="rounded-lg border bg-muted/20 p-3">
        <div className="flex items-center gap-2 text-sm font-medium">
          <CalendarClock className="size-4 text-primary" />
          {t('tasks.fields.schedule')}
        </div>
        <div className="mt-3 grid gap-2 text-xs sm:grid-cols-2">
          <TaskScheduleDetail
            label={t('tasks.fields.schedule')}
            value={taskScheduleSummary(selected, t, locale)}
          />
          {selected.schedule.type === 'once' && selected.schedule.run_at ? (
            <TaskScheduleDetail
              label={t('tasks.fields.runAt')}
              value={formatTaskDate(selected.schedule.run_at)}
            />
          ) : null}
          {selected.schedule.timezone ? (
            <TaskScheduleDetail
              label={t('tasks.fields.timezone')}
              value={translateTimezone(selected.schedule.timezone, locale)}
            />
          ) : null}
          {selected.next_run_at ? (
            <TaskScheduleDetail
              label={t('tasks.fields.nextRun')}
              value={formatTaskDate(selected.next_run_at)}
            />
          ) : null}
        </div>
      </section>
      <section className="rounded-lg border bg-muted/20 p-3">
        <div className="flex items-center gap-2 text-sm font-medium">
          <ArrowRight className="size-4 text-primary" />
          {t('tasks.fields.delivery')}
        </div>
        <div className="mt-3 grid gap-2 text-xs sm:grid-cols-2">
          <TaskScheduleDetail
            label={t('tasks.fields.delivery')}
            value={t(
              selected.delivery_target === 'source_conversation' ||
                selected.delivery_mode === 'source_conversation'
                ? 'tasks.delivery.source_conversation'
                : 'tasks.delivery.task_only'
            )}
          />
          {selected.delivery_fallback === 'task_center' ||
          selected.delivery_mode === 'source_conversation' ? (
            <TaskScheduleDetail
              label={t('tasks.fields.deliveryFallback')}
              value={t('tasks.delivery.task_only')}
            />
          ) : null}
        </div>
      </section>
      <div className="border-t pt-3">
        <h4 className="text-xs font-medium text-muted-foreground">{t('tasks.detail.runs')}</h4>
        {runs.length === 0 ? (
          <p className="mt-2 text-xs text-muted-foreground">{t('tasks.detail.noRuns')}</p>
        ) : (
          runs.slice(0, 3).map(run => (
            <div key={run.id} className="mt-2 rounded-md bg-muted/50 p-2 text-xs">
              <div className="flex justify-between">
                <span>{runStatusLabel(run.status, t)}</span>
                <span>{run.finished_at ? new Date(run.finished_at).toLocaleString() : '—'}</span>
              </div>
              {run.result_delivery_status ? (
                <div className="mt-1 text-muted-foreground">
                  {t('tasks.detail.resultDelivery')}：
                  {runDeliveryStatusLabel(run.result_delivery_status, t)}
                  {run.result_delivery_error ? ` · ${run.result_delivery_error}` : ''}
                </div>
              ) : null}
              {run.result_summary ? (
                <p className="mt-1 whitespace-pre-wrap text-muted-foreground">
                  {run.result_summary}
                </p>
              ) : null}
            </div>
          ))
        )}
      </div>
      {activities.length > 0 ? (
        <div className="border-t pt-4">
          <div className="flex items-center justify-between gap-2">
            <h4 className="text-xs font-medium text-muted-foreground">
              {t('tasks.detail.activities')}
            </h4>
            <span className="text-[11px] text-muted-foreground">{activities.length}</span>
          </div>
          <ol className="mt-3 space-y-0">
            {activities.slice(0, 8).map((activity, index) => {
              return (
                <li key={activity.id} className="relative pl-6">
                  {index < Math.min(activities.length, 8) - 1 ? (
                    <span
                      aria-hidden="true"
                      className="absolute bottom-0 left-[7px] top-4 w-px bg-border"
                    />
                  ) : null}
                  <span
                    aria-hidden="true"
                    className={`absolute left-0 top-0.5 flex size-4 items-center justify-center rounded-full bg-background ${activityTone(activity.action)}`}
                  >
                    <ActivityIcon action={activity.action} />
                  </span>
                  <div className="pb-4 last:pb-0">
                    <div className="flex flex-wrap items-baseline justify-between gap-x-2 gap-y-0.5">
                      <span className="text-xs font-medium text-foreground">
                        {activityLabel(activity.action, t)}
                      </span>
                      <time
                        dateTime={activity.created_at}
                        title={formatTaskDate(activity.created_at)}
                        className="text-[11px] text-muted-foreground"
                      >
                        {formatActivityTime(activity.created_at, locale)}
                      </time>
                    </div>
                    <div className="mt-0.5 text-[11px] text-muted-foreground">
                      {activityActorLabel(activity.actor_type, t)}
                    </div>
                    <ActivityChangeDetails
                      activity={activity}
                      locale={locale}
                      t={t}
                      defaultOpen={index === 0}
                    />
                  </div>
                </li>
              );
            })}
          </ol>
        </div>
      ) : null}
    </div>
  );
}

function TaskScheduleDetail({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <div className="text-muted-foreground">{label}</div>
      <div className="mt-0.5 break-words font-medium text-foreground">{value}</div>
    </div>
  );
}

type TaskActivityTranslator = ReturnType<typeof useT<'digitalHumans'>>;

function activityLabel(action: string, t: TaskActivityTranslator): string {
  const labels: Record<string, string> = {
    created: t('tasks.activity.created'),
    updated: t('tasks.activity.updated'),
    status_changed: t('tasks.activity.status_changed'),
    confirm: t('tasks.activity.confirm'),
    decline: t('tasks.activity.decline'),
    run_now: t('tasks.activity.run_now'),
    pause: t('tasks.activity.pause'),
    resume: t('tasks.activity.resume'),
    retry: t('tasks.activity.retry'),
    cancel: t('tasks.activity.cancel'),
    archive: t('tasks.activity.archive'),
    blocked: t('tasks.activity.blocked'),
    run_finished: t('tasks.activity.run_finished'),
    proposal_expired: t('tasks.activity.proposal_expired'),
    status_repaired: t('tasks.activity.status_repaired'),
  };
  if (labels[action]) return labels[action];
  return action.replace(/[_-]+/g, ' ').replace(/\b\w/g, character => character.toUpperCase());
}

function activityActorLabel(actorType: string, t: TaskActivityTranslator): string {
  return actorType === 'system' ? t('tasks.activity.system') : t('tasks.activity.actorSelf');
}

function activityTone(action: string): string {
  if (action === 'created' || action === 'confirm' || action === 'run_finished') {
    return 'text-emerald-600';
  }
  if (action === 'decline' || action === 'cancel' || action === 'archive' || action === 'blocked') {
    return 'text-destructive';
  }
  return 'text-primary';
}

function ActivityIcon({ action }: { action: string }) {
  if (action === 'created') {
    return <CircleDot className="size-2.5" />;
  }
  if (action === 'confirm' || action === 'run_finished') {
    return <CheckCircle2 className="size-2.5" />;
  }
  if (action === 'blocked') {
    return <AlertTriangle className="size-2.5" />;
  }
  if (action === 'updated' || action === 'status_changed') {
    return <Pencil className="size-2.5" />;
  }
  return <CircleDot className="size-2.5" />;
}

function ActivityChangeDetails({
  activity,
  locale,
  t,
  defaultOpen,
}: {
  activity: DigitalHumanTaskActivity;
  locale?: string;
  t: TaskActivityTranslator;
  defaultOpen: boolean;
}) {
  const [open, setOpen] = useState(defaultOpen);
  const changes = formatActivityChanges(activity.changes, activity.action, locale);
  if (changes.length === 0) return null;

  return (
    <div className="mt-1.5">
      <button
        type="button"
        className="inline-flex min-h-7 items-center gap-1 rounded-md px-1.5 text-[11px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        aria-expanded={open}
        onClick={() => setOpen(value => !value)}
      >
        <ChevronDown className={`size-3 transition-transform ${open ? '' : '-rotate-90'}`} />
        {open ? t('tasks.activity.hideChanges') : t('tasks.activity.viewChanges')}
      </button>
      {open ? (
        <div className="mt-1.5 space-y-1 rounded-md bg-muted/50 px-2.5 py-2 text-[11px] leading-5">
          {changes.map(change => (
            <div key={change.key} className="grid gap-0.5 sm:grid-cols-[minmax(5rem,8rem)_minmax(0,1fr)] sm:gap-2">
              <span className="text-muted-foreground">{change.label}</span>
              <span className="min-w-0 break-words font-medium text-foreground">
                {change.before !== undefined ? (
                  <>
                    <span className="text-muted-foreground line-through">
                      {formatActivityValue(change.key, change.before, locale)}
                    </span>
                    <ArrowRight className="mx-1 inline-block size-3 align-[-2px] text-muted-foreground" />
                  </>
                ) : null}
                {change.after !== undefined
                  ? formatActivityValue(change.key, change.after, locale)
                  : t('tasks.activity.removed')}
              </span>
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function formatActivityTime(value: string, locale?: string): string {
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) return value;
  const elapsed = Math.max(0, Date.now() - timestamp);
  const minute = 60 * 1000;
  const hour = 60 * minute;
  const day = 24 * hour;
  if (elapsed < minute) return locale?.startsWith('zh') ? '刚刚' : 'Just now';
  if (elapsed < hour) {
    const count = Math.max(1, Math.floor(elapsed / minute));
    return locale?.startsWith('zh') ? `${count} 分钟前` : `${count}m ago`;
  }
  if (elapsed < day) {
    const count = Math.max(1, Math.floor(elapsed / hour));
    return locale?.startsWith('zh') ? `${count} 小时前` : `${count}h ago`;
  }
  if (elapsed < 7 * day) {
    const count = Math.max(1, Math.floor(elapsed / day));
    return locale?.startsWith('zh') ? `${count} 天前` : `${count}d ago`;
  }
  return new Intl.DateTimeFormat(locale === 'zh-Hans' ? 'zh-CN' : locale, {
    month: 'numeric',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(new Date(timestamp));
}

function formatActivityChanges(
  changes: Record<string, unknown> | undefined,
  action: string,
  locale?: string
): Array<{ key: string; label: string; before?: unknown; after?: unknown }> {
  if (!changes) return [];
  const labels: Record<string, string> = locale?.startsWith('zh')
    ? {
        name: '名称',
        goal: '目标',
        priority: '优先级',
        work_status: '状态',
        lifecycle_status: '生命周期',
        delivery_mode: '交付方式',
        schedule: '执行计划',
        reason: '原因',
        run_status: '执行状态',
        digital_human_status: '数字人状态',
      }
    : {
        name: 'Name',
        goal: 'Goal',
        priority: 'Priority',
        work_status: 'Status',
        lifecycle_status: 'Lifecycle',
        delivery_mode: 'Delivery',
        schedule: 'Schedule',
        reason: 'Reason',
        run_status: 'Run status',
        digital_human_status: 'Digital human status',
      };
  const entries: Array<{ key: string; label: string; before?: unknown; after?: unknown }> = [];
  for (const [key, rawValue] of Object.entries(changes)) {
    if (rawValue === undefined || rawValue === null) continue;
    if (isActivityChange(rawValue)) {
      entries.push({
        key,
        label: labels[key] ?? key,
        before: rawValue.before,
        after: rawValue.after,
      });
      continue;
    }
    if (typeof rawValue === 'object') {
      entries.push({ key, label: labels[key] ?? key, after: activityObjectSummary(key, locale) });
      continue;
    }
    entries.push({
      key,
      label: labels[key] ?? key,
      before: inferredActivityBefore(action, key, rawValue),
      after: rawValue,
    });
  }
  return entries;
}

function isActivityChange(value: unknown): value is { before?: unknown; after?: unknown } {
  return typeof value === 'object' && value !== null && ('before' in value || 'after' in value);
}

function inferredActivityBefore(action: string, key: string, after: unknown): unknown {
  if (action !== 'confirm') return undefined;
  if (key === 'work_status' && after === 'pending') return 'proposed';
  if (key === 'status' && after === 'active') return 'draft';
  return undefined;
}

function activityObjectSummary(key: string, locale?: string): string {
  if (key === 'schedule') {
    return locale?.startsWith('zh') ? '执行计划已更新' : 'Execution plan updated';
  }
  return locale?.startsWith('zh') ? '已更新' : 'Updated';
}

function formatActivityValue(key: string, value: unknown, locale?: string): string {
  const zh = locale?.startsWith('zh');
  const maps: Record<string, Record<string, string>> = {
    work_status: zh
      ? {
          proposed: '待确认',
          pending: '待执行',
          in_progress: '执行中',
          waiting: '等待输入',
          blocked: '执行受阻',
          completed: '已完成',
          canceled: '已取消',
        }
      : {
          proposed: 'Needs confirmation',
          pending: 'Ready to run',
          in_progress: 'In progress',
          waiting: 'Waiting for input',
          blocked: 'Blocked',
          completed: 'Completed',
          canceled: 'Canceled',
        },
    status: zh
      ? { draft: '未启用', active: '已启用', paused: '已暂停', archived: '已归档' }
      : { draft: 'Not active', active: 'Enabled', paused: 'Paused', archived: 'Archived' },
    priority: zh
      ? { low: '低', normal: '普通', high: '高', urgent: '紧急' }
      : { low: 'Low', normal: 'Normal', high: 'High', urgent: 'Urgent' },
    delivery_mode: zh
      ? { task_only: '仅保留在待办详情', source_conversation: '回复到来源会话' }
      : { task_only: 'Keep in task details', source_conversation: 'Reply to source conversation' },
    reason: zh
      ? { execution_unavailable: '执行环境不可用' }
      : { execution_unavailable: 'Execution environment unavailable' },
    run_status: zh
      ? { queued: '排队中', running: '执行中', succeeded: '已完成', failed: '失败', canceled: '已取消' }
      : { queued: 'Queued', running: 'Running', succeeded: 'Succeeded', failed: 'Failed', canceled: 'Canceled' },
  };
  if (typeof value === 'string') return maps[key]?.[value] ?? value;
  if (typeof value === 'boolean') return value ? (zh ? '是' : 'Yes') : zh ? '否' : 'No';
  if (Array.isArray(value)) return value.join(zh ? '、' : ', ');
  if (value === null || value === undefined) return zh ? '无' : 'None';
  return String(value);
}

function taskScheduleSummary(
  task: DigitalHumanTask,
  t: ReturnType<typeof useT<'digitalHumans'>>,
  locale?: string
): string {
  const schedule = task.schedule;
  if (schedule.type === 'manual') return t('tasks.schedule.manual');
  if (schedule.type === 'once') {
    return `${t('tasks.schedule.once')} · ${schedule.run_at ? formatTaskDate(schedule.run_at) : '—'}`;
  }
  const description = describeCronSchedule(schedule.cron, t);
  return schedule.timezone
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
        sun: t('tasks.weekdays.sun'),
        mon: t('tasks.weekdays.mon'),
        tue: t('tasks.weekdays.tue'),
        wed: t('tasks.weekdays.wed'),
        thu: t('tasks.weekdays.thu'),
        fri: t('tasks.weekdays.fri'),
        sat: t('tasks.weekdays.sat'),
      };
      const isWeekdays = weekdays.join(',') === 'mon,tue,wed,thu,fri';
      if (isWeekdays) return t('tasks.schedule.weekdays', { time: simpleTime });
      return t('tasks.schedule.weekly', {
        days: weekdays.map(day => weekdayLabels[day]).join(', '),
        time: simpleTime,
      });
    }
  }

  const everyMinutes = /^\*(\/(\d+))?$/.exec(minutes);
  if (
    everyMinutes?.[2] &&
    hours === '*' &&
    dayOfMonth === '*' &&
    month === '*' &&
    dayOfWeek === '*'
  ) {
    return t('tasks.schedule.everyMinutes', { value: everyMinutes[2] });
  }

  const everyHours = minutes === '0' ? /^\*(\/(\d+))?$/.exec(hours) : null;
  if (everyHours?.[2] && dayOfMonth === '*' && month === '*' && dayOfWeek === '*') {
    return t('tasks.schedule.everyHours', { value: everyHours[2] });
  }

  if (simpleTime && /^\d{1,2}$/.test(dayOfMonth) && month === '*' && dayOfWeek === '*') {
    return t('tasks.schedule.monthly', {
      day: String(Number(dayOfMonth)),
      time: simpleTime,
    });
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
    sun: 0,
    mon: 1,
    tue: 2,
    wed: 3,
    thu: 4,
    fri: 5,
    sat: 6,
  };
  if (value in aliases) return aliases[value];
  if (!/^\d$/.test(value)) return null;
  const day = Number(value);
  return day === 7 ? 0 : day <= 6 ? day : null;
}

function formatTaskDate(value: string): string {
  const timestamp = Date.parse(value);
  if (!Number.isFinite(timestamp)) return value;
  return new Date(timestamp).toLocaleString(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  });
}

function runStatusLabel(status: string, t: ReturnType<typeof useT<'digitalHumans'>>) {
  switch (status) {
    case 'queued':
      return t('tasks.runStatus.queued');
    case 'running':
      return t('tasks.runStatus.running');
    case 'succeeded':
      return t('tasks.runStatus.succeeded');
    case 'failed':
      return t('tasks.runStatus.failed');
    case 'canceled':
      return t('tasks.runStatus.canceled');
    default:
      return t('tasks.runStatus.unknown');
  }
}

function runDeliveryStatusLabel(
  status: NonNullable<DigitalHumanTaskRun['result_delivery_status']>,
  t: ReturnType<typeof useT<'digitalHumans'>>
) {
  const key = ['pending', 'processing', 'succeeded', 'failed', 'unknown', 'dismissed'].includes(
    status
  )
    ? status
    : 'unknown';
  return t(`tasks.deliveryStatus.${key}` as const);
}
